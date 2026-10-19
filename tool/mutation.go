package tool

import (
	"context"
	"sort"
	"strings"
	"sync"
)

type mutationLock struct {
	gate chan struct{}
	refs int
}

// Cooperating tools share locks across sessions; external editors do not.
var mutations = struct {
	sync.Mutex
	paths map[string]*mutationLock
}{paths: make(map[string]*mutationLock)}

func lockMutation(ctx context.Context, paths ...string) (func(), error) {
	keys := make(map[string]bool)
	for _, path := range paths {
		keys[path] = true
		canonical, err := canonicalPath(path)
		if err != nil {
			return nil, err
		}
		keys[canonical] = true
	}
	ordered := make([]string, 0, len(keys))
	for path := range keys {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	var releases []func()
	unlock := func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}
	for _, path := range ordered {
		mutations.Lock()
		lock := mutations.paths[path]
		if lock == nil {
			lock = &mutationLock{gate: make(chan struct{}, 1)}
			mutations.paths[path] = lock
		}
		lock.refs++
		mutations.Unlock()
		drop := func() {
			mutations.Lock()
			defer mutations.Unlock()
			lock.refs--
			if lock.refs == 0 {
				delete(mutations.paths, path)
			}
		}
		select {
		case lock.gate <- struct{}{}:
			releases = append(releases, func() { <-lock.gate; drop() })
		case <-ctx.Done():
			drop()
			unlock()
			return nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		unlock()
		return nil, err
	}
	return unlock, nil
}

const utf8BOM = "\ufeff"

func normalizedText(text string) string {
	return strings.ReplaceAll(strings.TrimPrefix(text, utf8BOM), "\r\n", "\n")
}

func retainTextFormat(original, next string) string {
	bom := strings.HasPrefix(original, utf8BOM) || strings.HasPrefix(next, utf8BOM)
	next = strings.TrimPrefix(next, utf8BOM)
	if strings.Contains(original, "\r\n") && !strings.Contains(strings.ReplaceAll(original, "\r\n", ""), "\n") {
		next = strings.ReplaceAll(strings.ReplaceAll(next, "\r\n", "\n"), "\n", "\r\n")
	}
	if bom {
		next = utf8BOM + next
	}
	return next
}

func textLineEnding(text string) string {
	if strings.Contains(text, "\r\n") && !strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
		return "\r\n"
	}
	return "\n"
}

func replaceNormalizedText(original, old, next string, count int) string {
	var out strings.Builder
	next = strings.ReplaceAll(next, "\n", textLineEnding(original))
	if strings.HasPrefix(original, utf8BOM) {
		out.WriteString(utf8BOM)
		original = strings.TrimPrefix(original, utf8BOM)
	}
	text := strings.ReplaceAll(original, "\r\n", "\n")
	// Map normalized match boundaries back to the original bytes.
	offset := func(raw string, length int) int {
		i := 0
		for range length {
			if raw[i] == '\r' && i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
			i++
		}
		return i
	}
	for count != 0 {
		index := strings.Index(text, old)
		if index < 0 {
			break
		}
		start := offset(original, index)
		end := start + offset(original[start:], len(old))
		out.WriteString(original[:start])
		out.WriteString(next)
		original = original[end:]
		text = text[index+len(old):]
		count--
	}
	out.WriteString(original)
	return out.String()
}
