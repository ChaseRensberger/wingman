package tool

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chaserensberger/wingman/models"
)

// MaxOutputBytes is the maximum model-facing text output, including its truncation notice.
const MaxOutputBytes = 50 * 1024

const outputPrefix = "tool-output:"

// Reserve room for the reference and notice without truncating the preview twice.
const outputPreviewBytes = MaxOutputBytes - 512

type outputCollector struct {
	inv     Invocation
	preview strings.Builder
	file    *os.File
	err     error
}

func (w *outputCollector) WriteString(text string) (int, error) {
	written := 0
	for len(text) > 0 {
		size := min(len(text), 32*1024)
		n, err := w.Write([]byte(text[:size]))
		written += n
		if err != nil {
			return written, err
		}
		text = text[size:]
	}
	return written, nil
}

func (w *outputCollector) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if w.file == nil && len(p) > outputPreviewBytes-w.preview.Len() {
		dir, err := outputDirectory(w.inv)
		if err == nil {
			err = os.MkdirAll(dir, 0700)
		}
		if err == nil {
			cleanOutputFiles(dir)
			w.file, err = os.CreateTemp(dir, "output-")
		}
		if err == nil {
			_, err = io.WriteString(w.file, w.preview.String())
		}
		if err != nil {
			w.err = fmt.Errorf("save full tool output: %w", err)
			return 0, w.err
		}
	}
	if w.file != nil {
		if _, err := w.file.Write(p); err != nil {
			w.err = fmt.Errorf("save full tool output: %w", err)
			return 0, w.err
		}
	}
	w.preview.Write(p[:min(len(p), outputPreviewBytes-w.preview.Len())])
	return len(p), nil
}

func (w *outputCollector) finish(metadata map[string]any) (Result, error) {
	text := w.preview.String()
	if w.file != nil || w.err != nil {
		if w.file != nil {
			w.err = errors.Join(w.err, w.file.Close())
		}
		metadata = maps.Clone(metadata)
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadata["truncated"] = true
		if w.err == nil {
			ref := outputPrefix + filepath.Base(w.file.Name())
			metadata["output_ref"] = ref
			metadata["output_path"] = w.file.Name()
			text = validPrefix(text) + fmt.Sprintf("\n[Output truncated. Full output: %s. Use read with this filePath and offset/limit. Available for seven days.]", ref)
		} else {
			delete(metadata, "output_ref")
			delete(metadata, "output_path")
			text = validPrefix(text) + "\n[Output truncated; saving full output failed.]"
		}
	}
	return Result{Text: text, Metadata: metadata}, w.err
}

func validPrefix(text string) string {
	for len(text) > 0 && !utf8.ValidString(text) {
		// Only a split final rune is trimmed; arbitrary binary output is retained.
		start := len(text) - 1
		for start > 0 && !utf8.RuneStart(text[start]) {
			start--
		}
		if utf8.FullRuneInString(text[start:]) {
			break
		}
		text = text[:start]
	}
	return text
}

func outputDirectory(inv Invocation) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	workDir, err := filepath.Abs(inv.WorkDir)
	if err != nil {
		return "", err
	}
	scope := sha256.Sum256([]byte(workDir + "\x00" + inv.SessionID))
	return filepath.Join(base, "wingman", "tool-output", fmt.Sprintf("%x", scope)), nil
}

func outputPath(inv Invocation, ref string) (string, error) {
	name := strings.TrimPrefix(ref, outputPrefix)
	if !strings.HasPrefix(name, "output-") || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("invalid tool output reference")
	}
	dir, err := outputDirectory(inv)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("invalid tool output file")
	}
	if time.Since(info.ModTime()) > 7*24*time.Hour {
		return "", fmt.Errorf("tool output has expired")
	}
	return path, nil
}

func cleanOutputFiles(dir string) {
	f, err := os.Open(dir)
	if err != nil {
		return
	}
	defer f.Close()
	for {
		entries, err := f.ReadDir(128)
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), "output-") {
				continue
			}
			if info, err := entry.Info(); err == nil && info.Mode().IsRegular() && time.Since(info.ModTime()) > 7*24*time.Hour {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
			}
		}
		if err != nil {
			return
		}
	}
}

// BoundResult saves oversized text and returns a bounded preview, preserving non-text output.
func BoundResult(inv Invocation, result Result) (Result, error) {
	size := len(result.Text)
	for _, part := range result.OutputParts {
		if text, ok := part.(models.TextPart); ok {
			size += len(text.Text)
		}
	}
	if size <= MaxOutputBytes {
		return result, nil
	}
	w := &outputCollector{inv: inv}
	_, _ = io.WriteString(w, result.Text)
	for _, part := range result.OutputParts {
		if text, ok := part.(models.TextPart); ok {
			_, _ = io.WriteString(w, text.Text)
		}
	}
	bounded, err := w.finish(result.Metadata)
	remaining := outputPreviewBytes
	result.Text = strings.Clone(validPrefix(result.Text[:min(len(result.Text), remaining)]))
	remaining -= len(result.Text)
	parts := append(models.Content(nil), result.OutputParts...)
	for i, part := range parts {
		if text, ok := part.(models.TextPart); ok {
			text.Text = strings.Clone(validPrefix(text.Text[:min(len(text.Text), remaining)]))
			remaining -= len(text.Text)
			parts[i] = text
		}
	}
	notice := bounded.Text[len(validPrefix(w.preview.String())):]
	if len(parts) == 0 {
		result.Text += notice
	} else {
		parts = append(parts, models.TextPart{Text: notice})
	}
	result.OutputParts, result.Metadata = parts, bounded.Metadata
	return result, err
}
