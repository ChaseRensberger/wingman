package server

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chaserensberger/wingman/store"
)

const agentsFileName = "AGENTS.md"

func (s *Server) resolveInstructions(agent *store.Agent, workDir string) (string, []store.InstructionSource, error) {
	paths := []struct {
		kind string
		path string
	}{
		{kind: "global", path: s.globalInstructionsPath},
	}
	if workDir != "" {
		paths = append(paths, struct {
			kind string
			path string
		}{kind: "project", path: filepath.Join(workDir, agentsFileName)})
	}

	var sections []string
	var sources []store.InstructionSource
	for _, candidate := range paths {
		if candidate.path == "" {
			continue
		}
		content, canonicalPath, found, err := readInstructionFile(candidate.path)
		if err != nil {
			return "", nil, fmt.Errorf("read %s instructions %q: %w", candidate.kind, candidate.path, err)
		}
		if !found {
			continue
		}
		sum := sha256.Sum256(content)
		sources = append(sources, store.InstructionSource{
			Kind: candidate.kind, Path: canonicalPath, SHA256: fmt.Sprintf("%x", sum),
			ResolvedAt: time.Now().UTC(), Order: len(sources) + 1,
		})
		sections = append(sections, fmt.Sprintf("## Wingman instructions: %s\nSource: %s\n\n%s", candidate.kind, canonicalPath, content))
	}
	parts := []string{}
	if agent.Instructions != "" {
		parts = append(parts, agent.Instructions)
	}
	parts = append(parts, "Current date: "+time.Now().Format(time.DateOnly)+".")
	parts = append(parts, sections...)
	return strings.Join(parts, "\n\n"), sources, nil
}

func readInstructionFile(path string) ([]byte, string, bool, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, "", false, err
	}
	content, err := os.ReadFile(absolute)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, "", false, err
	}
	return content, canonical, true, nil
}
