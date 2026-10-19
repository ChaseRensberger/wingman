package tool

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var errSearchLimit = errors.New("search result limit reached")

func searchOptions(inv Invocation) (string, int, error) {
	if inv.WorkDir == "" {
		return "", 0, fmt.Errorf("workDir is required for search tools")
	}
	path, _ := inv.Input["path"].(string)
	if path == "" {
		path = "."
	}
	path, _, err := ResolvePath(inv.WorkDir, path)
	if err != nil {
		return "", 0, err
	}
	path, err = canonicalPath(path)
	limit := 100
	if raw, ok := inv.Input["limit"]; ok {
		switch value := raw.(type) {
		case int:
			limit = value
		case float64:
			limit = int(value)
			if float64(limit) != value {
				return "", 0, fmt.Errorf("limit must be a positive integer")
			}
		default:
			return "", 0, fmt.Errorf("limit must be a positive integer")
		}
		if limit <= 0 {
			return "", 0, fmt.Errorf("limit must be a positive integer")
		}
	}
	return path, limit, err
}

func searchVisibility(inv Invocation) []string {
	args := []string{}
	if hidden, _ := inv.Input["hidden"].(bool); hidden {
		args = append(args, "--hidden")
	} else {
		args = append(args, "--glob=!**/.*")
	}
	return append(args, "--glob=!**/.git/**")
}

func runRipgrep(ctx context.Context, dir string, args []string, split bufio.SplitFunc, visit func([]byte) error) error {
	binary, err := exec.LookPath("rg")
	if err != nil {
		return fmt.Errorf("ripgrep (rg) is required for grep and glob; install ripgrep and ensure rg is on PATH")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, append([]string{"--no-config", "--no-require-git", "--color=never"}, args...)...)
	cmd.Dir = dir
	var stderr searchErrors
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	scanner.Split(split)
	var readErr error
	for scanner.Scan() {
		if readErr = visit(scanner.Bytes()); readErr != nil {
			cancel()
			break
		}
	}
	if readErr == nil {
		readErr = scanner.Err()
	}
	if readErr != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	if readErr != nil {
		return readErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var exit *exec.ExitError
	if errors.As(waitErr, &exit) && exit.ExitCode() == 1 {
		return nil
	}
	if waitErr != nil {
		return fmt.Errorf("ripgrep: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type searchErrors struct{ bytes.Buffer }

func (w *searchErrors) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := 8192 - w.Len(); remaining > 0 {
		_, _ = w.Buffer.Write(p[:min(remaining, n)])
	}
	return n, nil
}

func finishSearch(output *outputCollector, searchErr error, key string, count int, empty string) (Result, error) {
	limited := errors.Is(searchErr, errSearchLimit)
	if limited {
		_, _ = output.WriteString("\n[Result limit reached. Narrow the search or increase limit.]")
	}
	result, err := output.finish(map[string]any{key: count, "limit_reached": limited})
	if searchErr != nil && !limited {
		return result, searchErr
	}
	if err != nil {
		return result, err
	}
	if count == 0 {
		result.Text = empty
	}
	return result, nil
}
