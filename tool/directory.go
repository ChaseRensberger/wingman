package tool

import (
	"container/heap"
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type nameHeap []string

func (h nameHeap) Len() int           { return len(h) }
func (h nameHeap) Less(i, j int) bool { return h[i] > h[j] }
func (h nameHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *nameHeap) Push(v any)        { *h = append(*h, v.(string)) }
func (h *nameHeap) Pop() any          { a := *h; v := a[len(a)-1]; *h = a[:len(a)-1]; return v }

// Select the offset by name prefix, so deep pages do not retain or repeatedly
// sort every preceding page. Memory depends on page size, not directory size.
func directoryPage(ctx context.Context, path string, offset, limit int) ([]string, int, error) {
	prefix, rank, total := "", offset-1, 0
	for rank > 0 {
		var counts [257]int
		err := directoryEntries(ctx, path, func(entry os.DirEntry) error {
			name := directoryName(entry)
			if prefix == "" {
				total++
			}
			if strings.HasPrefix(name, prefix) {
				if len(name) == len(prefix) {
					counts[0]++
				} else {
					counts[int(name[len(prefix)])+1]++
				}
			}
			return nil
		})
		if err != nil {
			return nil, 0, err
		}
		bucket := 0
		for bucket < len(counts) && rank >= counts[bucket] {
			rank -= counts[bucket]
			bucket++
		}
		if bucket == len(counts) {
			return nil, total, nil
		}
		if bucket == 0 {
			break
		}
		prefix += string([]byte{byte(bucket - 1)})
	}
	names := &nameHeap{}
	total = 0
	err := directoryEntries(ctx, path, func(entry os.DirEntry) error {
		total++
		name := directoryName(entry)
		if name < prefix {
			return nil
		}
		if names.Len() < limit {
			heap.Push(names, name)
		} else if name < (*names)[0] {
			(*names)[0] = name
			heap.Fix(names, 0)
		}
		return nil
	})
	sort.Strings(*names)
	return *names, total, err
}

func directoryName(entry os.DirEntry) string {
	if entry.IsDir() {
		return entry.Name() + "/"
	}
	return entry.Name()
}

func directoryEntries(ctx context.Context, path string, visit func(os.DirEntry) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := f.ReadDir(128)
		for _, entry := range entries {
			if err := visit(entry); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func walkFiles(ctx context.Context, path string, skip func(string) bool, visit func(string) error) error {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := f.ReadDir(128)
		for _, entry := range entries {
			next := filepath.Join(path, entry.Name())
			if entry.IsDir() {
				if skip != nil && skip(entry.Name()) {
					continue
				}
				err = walkFiles(ctx, next, skip, visit)
			} else {
				err = visit(next)
			}
			if err != nil {
				return err
			}
		}
		// Like filesystem search tools, skip directories that cannot be read.
		if readErr != nil {
			return nil
		}
	}
}

func globFiles(ctx context.Context, pattern string, visit func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := filepath.Match(pattern, ""); err != nil {
		return err
	}
	if !globHasMeta(pattern) {
		if _, err := os.Lstat(pattern); err == nil {
			return visit(pattern)
		}
		return nil
	}
	dir, file := filepath.Split(pattern)
	dir = filepath.Clean(dir)
	matchDir := func(path string) error {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() {
			return nil
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || resolved != path {
			return nil
		}
		err = directoryEntries(ctx, path, func(entry os.DirEntry) error {
			match, err := filepath.Match(file, entry.Name())
			if err != nil {
				return err
			}
			if match {
				return visit(filepath.Join(path, entry.Name()))
			}
			return nil
		})
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if globHasMeta(dir) {
		return globFiles(ctx, dir, matchDir)
	}
	return matchDir(dir)
}

func globHasMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[") || (filepath.Separator != '\\' && strings.Contains(pattern, `\`))
}
