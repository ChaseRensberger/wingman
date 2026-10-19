// Package instruction discovers directory guidance and prepares instruction snapshots.
package instruction

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chaserensberger/wingman/models"
)

// Read records a successful file read that can expose directory guidance.
type Read struct {
	RequestedPath string `json:"requested_path"`
	ResolvedPath  string `json:"resolved_path"`
	ToolUseID     string `json:"tool_use_id,omitempty"`
	CallID        string `json:"call_id,omitempty"`
}

// Discover loads global and ancestor guidance, then guidance exposed by successful reads.
func Discover(ctx context.Context, globalPath, workDir string, reads []Read) ([]models.InstructionSnapshot, error) {
	var sources []models.InstructionSnapshot
	seen := make(map[string]bool)
	add := func(path, kind string, read Read) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == "" {
			return nil
		}
		canonical, err := filepath.EvalSymlinks(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("resolve instructions %s: %w", path, err)
		}
		canonical, err = filepath.Abs(canonical)
		if err != nil {
			return err
		}
		if seen[canonical] {
			return nil
		}
		data, err := os.ReadFile(canonical)
		if err != nil {
			return fmt.Errorf("read %s instructions %s: %w", kind, canonical, err)
		}
		seen[canonical] = true
		sources = append(sources, models.InstructionSnapshot{Kind: kind, Path: canonical, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Content: string(data), RequestedPath: read.RequestedPath, ResolvedPath: read.ResolvedPath, ToolUseID: read.ToolUseID, CallID: read.CallID})
		return nil
	}
	ancestors := func(dir, kind string, read Read) error {
		var dirs []string
		for {
			dirs = append(dirs, dir)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		for i := len(dirs) - 1; i >= 0; i-- {
			if err := add(filepath.Join(dirs[i], "AGENTS.md"), kind, read); err != nil {
				return err
			}
		}
		return nil
	}
	if err := add(globalPath, "global", Read{}); err != nil {
		return nil, err
	}
	if workDir != "" {
		dir, err := filepath.EvalSymlinks(workDir)
		if os.IsNotExist(err) {
			dir = workDir
		} else if err != nil {
			return nil, err
		}
		dir, err = filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		if err := ancestors(dir, "project", Read{}); err != nil {
			return nil, err
		}
	}
	for _, read := range reads {
		if !filepath.IsAbs(read.ResolvedPath) {
			continue
		}
		if err := ancestors(filepath.Dir(read.ResolvedPath), "directory", read); err != nil {
			return nil, err
		}
	}
	return sources, nil
}

// Render appends source-labelled guidance to a base system prompt.
func Render(base string, sources []models.InstructionSnapshot) string {
	parts := []string{base}
	for _, source := range sources {
		parts = append(parts, fmt.Sprintf("## Wingman instructions: %s\nSource: %s\n\n%s", source.Kind, source.Path, source.Content))
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
