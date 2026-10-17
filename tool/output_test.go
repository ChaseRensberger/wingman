package tool

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/chaserensberger/wingman/models"
)

func setOutputCacheDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)
}

func TestOutputCollectorSpillsDuringCollection(t *testing.T) {
	setOutputCacheDir(t, t.TempDir())
	inv := Invocation{WorkDir: t.TempDir(), SessionID: "session"}
	w := &outputCollector{inv: inv}
	chunk := strings.Repeat("line\n", 1000)
	for i := 0; i < 200; i++ {
		if _, err := io.WriteString(w, chunk); err != nil {
			t.Fatal(err)
		}
		if w.preview.Len() > MaxOutputBytes {
			t.Fatal("unbounded preview")
		}
	}
	result, err := w.finish(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Text) > MaxOutputBytes+512 || result.Metadata["truncated"] != true {
		t.Fatalf("result size=%d metadata=%v", len(result.Text), result.Metadata)
	}
	ref := result.Metadata["output_ref"].(string)
	path, err := outputPath(inv, ref)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil || string(saved) != strings.Repeat(chunk, 200) {
		t.Fatalf("full output size=%d err=%v", len(saved), err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%v", info.Mode())
	}
	inv.Input = map[string]any{"filePath": ref, "offset": 10000, "limit": 2}
	page, err := NewReadTool().Execute(t.Context(), inv)
	if err != nil || !strings.Contains(page.Text, "10000: line\n10001: line") {
		t.Fatalf("page=%v err=%v", page, err)
	}
	inv.SessionID = "another"
	if _, err := NewReadTool().Execute(t.Context(), inv); err == nil {
		t.Fatal("cross-session output read")
	}
	inv.SessionID = "session"
	inv.WorkDir = t.TempDir()
	if _, err := NewReadTool().Execute(t.Context(), inv); err == nil {
		t.Fatal("cross-workspace output read")
	}
}

func TestOutputFailureIsExplicitAndBounded(t *testing.T) {
	for _, env := range []string{"XDG_CACHE_HOME", "HOME"} {
		t.Run(env, func(t *testing.T) {
			dir := t.TempDir()
			block := filepath.Join(dir, "file")
			if err := os.WriteFile(block, nil, 0600); err != nil {
				t.Fatal(err)
			}
			setOutputCacheDir(t, block)
			if env == "HOME" {
				t.Setenv("XDG_CACHE_HOME", "")
			}
			result, err := BoundResult(Invocation{}, Result{Text: strings.Repeat("x", MaxOutputBytes*2)})
			if err == nil || result.Metadata["output_ref"] != nil || !strings.Contains(result.Text, "saving full output failed") || len(result.Text) > MaxOutputBytes+512 {
				t.Fatalf("size=%d metadata=%v err=%v", len(result.Text), result.Metadata, err)
			}
		})
	}
}

func TestBoundResultPreservesImagesAndStructuredOutput(t *testing.T) {
	setOutputCacheDir(t, t.TempDir())
	inv := Invocation{WorkDir: t.TempDir()}
	image := models.ImagePart{Base64: "image", MediaType: "image/png"}
	input := Result{Text: strings.Repeat("€", MaxOutputBytes/3+1), OutputParts: models.Content{image, models.TextPart{Text: "tail"}}, Structured: map[string]any{"ok": true}}
	result, err := BoundResult(inv, input)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(result.Text) || !reflect.DeepEqual(result.OutputParts[0], image) || !reflect.DeepEqual(result.Structured, input.Structured) {
		t.Fatalf("result not preserved")
	}
	path, _ := outputPath(inv, result.Metadata["output_ref"].(string))
	data, err := os.ReadFile(path)
	if err != nil || string(data) != input.Text+"tail" {
		t.Fatalf("full output lost: %v", err)
	}
}

func TestOutputExpiryAndReferenceValidation(t *testing.T) {
	setOutputCacheDir(t, t.TempDir())
	inv := Invocation{WorkDir: t.TempDir()}
	w := &outputCollector{inv: inv}
	_, _ = io.WriteString(w, strings.Repeat("x", MaxOutputBytes+1))
	result, err := w.finish(nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := result.Metadata["output_ref"].(string)
	path, _ := outputPath(inv, ref)
	old := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := outputPath(inv, ref); err == nil {
		t.Fatal("expired output readable")
	}
	for _, ref := range []string{"tool-output:output-../../secret", "tool-output:output-/secret", "tool-output:secret"} {
		if _, err := outputPath(inv, ref); err == nil {
			t.Fatalf("accepted %q", ref)
		}
	}
	cleanOutputFiles(filepath.Dir(path))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expired output retained: %v", err)
	}
}

func TestProgressBoundsConcurrentUpdates(t *testing.T) {
	for _, delta := range []string{"x", strings.Repeat("x", MaxOutputBytes*2)} {
		var bytes, count, markers int
		p := NewProgress(func(delta string, metadata map[string]any) {
			bytes += len(delta)
			count++
			if metadata["progress_truncated"] == true {
				markers++
			}
		})
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 1000; j++ {
					p.Report(delta, nil)
				}
			}()
		}
		wg.Wait()
		if bytes > MaxOutputBytes+128 || count > 257 || markers != 1 {
			t.Fatalf("bytes=%d count=%d markers=%d", bytes, count, markers)
		}
	}
	var seen map[string]any
	p := NewProgress(func(_ string, metadata map[string]any) { seen = metadata })
	p.Report("", map[string]any{"huge": strings.Repeat("x", MaxOutputBytes)})
	if seen["progress_truncated"] != true || seen["huge"] != nil {
		t.Fatalf("oversized metadata: %v", seen)
	}
}

func TestProgressPreservesSplitUTF8BelowLimit(t *testing.T) {
	var received strings.Builder
	p := NewProgress(func(delta string, _ map[string]any) { received.WriteString(delta) })
	p.Report("\xe2\x82", nil)
	p.Report("\xac", nil)
	if received.String() != "€" {
		t.Fatalf("received %q", received.String())
	}
}

func TestReadBoundsHugeLinesAndPages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	data := strings.Repeat("x", 1<<20) + "\r\nsecond\rthird\n" + strings.Repeat("next\n", 3000)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := NewReadTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"filePath": path, "limit": 1 << 30}})
	if err != nil || len(result.Text) > MaxOutputBytes || len(result.Metadata["preview"].(string)) > MaxOutputBytes || !strings.Contains(result.Text, "2: second\n3: third") || !strings.Contains(result.Text, "offset=2001") {
		t.Fatalf("size=%d err=%v", len(result.Text), err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := readLinePrefix(ctx, bufio.NewReader(strings.NewReader(data)), 2000); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel err=%v", err)
	}
}

func TestDirectoryPagingAndStreamingGlob(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 300; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d.txt", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	page, total, err := directoryPage(t.Context(), dir, 151, 3)
	if err != nil || total != 300 || !reflect.DeepEqual(page, []string{"150.txt", "151.txt", "152.txt"}) {
		t.Fatalf("page=%v total=%d err=%v", page, total, err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "file.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{"*.txt", "??0.txt", "[12]*", "missing/*", `\150.txt`, "*/*.txt", "nested/*"} {
		full := filepath.Join(dir, pattern)
		want, err := filepath.Glob(full)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		if err := globFiles(t.Context(), full, func(path string) error { got = append(got, path); return nil }); err != nil {
			t.Fatal(err)
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("pattern=%q got=%v want=%v", pattern, got, want)
		}
	}
}

func TestDirectoryPagesMatchSortedEntries(t *testing.T) {
	dir := t.TempDir()
	want := []string{"a", "aa", "ab", "b", "β", "€", "€€", "😊"}
	for _, name := range want {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "a-dir"), 0700); err != nil {
		t.Fatal(err)
	}
	want = append(want, "a-dir/")
	sort.Strings(want)
	for offset := 1; offset <= len(want)+2; offset++ {
		page, total, err := directoryPage(t.Context(), dir, offset, 2)
		start := min(offset-1, len(want))
		expected := want[start:min(start+2, len(want))]
		if err != nil || total != len(want) || strings.Join(page, "\n") != strings.Join(expected, "\n") {
			t.Fatalf("offset=%d page=%v want=%v total=%d err=%v", offset, page, expected, total, err)
		}
	}
}

func TestBashAndGrepRetainFullOutput(t *testing.T) {
	setOutputCacheDir(t, t.TempDir())
	dir := t.TempDir()
	for _, fail := range []bool{false, true} {
		command := "for ((i=0;i<20000;i++)); do echo line; done"
		if fail {
			command += "; exit 3"
		}
		result, err := NewBashTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"command": command}})
		if (err != nil) != fail || len(result.Text) > MaxOutputBytes+512 {
			t.Fatalf("size=%d err=%v", len(result.Text), err)
		}
		data, err := os.ReadFile(result.Metadata["output_path"].(string))
		if err != nil || string(data) != strings.Repeat("line\n", 20000) {
			t.Fatalf("saved bytes=%d err=%v", len(data), err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("match\n", 20000)), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := NewGrepTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"pattern": "match"}})
	if err != nil || len(result.Text) > MaxOutputBytes+512 || result.Metadata["matches"] != 20000 {
		t.Fatalf("size=%d metadata=%v err=%v", len(result.Text), result.Metadata, err)
	}
	data, err := os.ReadFile(result.Metadata["output_path"].(string))
	if err != nil || !strings.Contains(string(data), "big.txt:20000: match\n") {
		t.Fatalf("full grep output lost: %v", err)
	}
}

func TestGlobStreamsLargeResultToSavedOutput(t *testing.T) {
	setOutputCacheDir(t, t.TempDir())
	dir := t.TempDir()
	for i := 0; i < 300; i++ {
		name := fmt.Sprintf("%03d-%s.txt", i, strings.Repeat("x", 210))
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, pattern := range []string{"*.txt", "**/*.txt"} {
		result, err := NewGlobTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"pattern": pattern}})
		if err != nil || len(result.Text) > MaxOutputBytes || result.Metadata["count"] != 300 {
			t.Fatalf("size=%d metadata=%v err=%v", len(result.Text), result.Metadata, err)
		}
		data, err := os.ReadFile(result.Metadata["output_path"].(string))
		if err != nil || len(strings.Split(string(data), "\n")) != 300 {
			t.Fatalf("saved bytes=%d err=%v", len(data), err)
		}
	}
}

func TestGrepContinuesPastOversizedLines(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "minified.js"), []byte(strings.Repeat("x", 128*1024)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "source.go"), []byte("find me\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := NewGrepTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"pattern": "find me"}})
	if err != nil || !strings.Contains(result.Text, "source.go:1: find me") {
		t.Fatalf("result=%v err=%v", result, err)
	}
}
