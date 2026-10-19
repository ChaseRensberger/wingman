package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRipgrepSearchSemantics(t *testing.T) {
	dir := t.TempDir()
	for path, text := range map[string]string{
		".gitignore": "ignored/\n", "ignored/no.go": "needle", ".hidden.go": "needle", "binary.go": "needle\x00",
		"src/one.go": "Needle\nneedle\n", "src/two.ts": "needle\n", "src/unknown.xyz": "needle\n", "src/literal.go": "a.b\naXb\n",
	} {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	grep := func(input map[string]any) Result {
		t.Helper()
		result, err := NewGrepTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: input})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	result := grep(map[string]any{"pattern": "needle"})
	if result.Metadata["matches"] != 3 || strings.Contains(result.Text, "ignored") || strings.Contains(result.Text, "hidden") || strings.Contains(result.Text, "binary") {
		t.Fatalf("search = %#v", result)
	}
	result = grep(map[string]any{"pattern": "needle", "include": "src/*.{go,ts}", "caseSensitive": false, "limit": 2})
	if result.Metadata["matches"] != 2 || result.Metadata["limit_reached"] != true {
		t.Fatalf("limited = %#v", result)
	}
	result = grep(map[string]any{"pattern": "a.b", "literal": true})
	if result.Metadata["matches"] != 1 || !strings.Contains(result.Text, ":1: a.b") {
		t.Fatalf("literal = %#v", result)
	}
	result = grep(map[string]any{"pattern": "needle", "hidden": true})
	if !strings.Contains(result.Text, ".hidden.go") || strings.Contains(result.Text, "ignored/no.go") {
		t.Fatalf("hidden = %#v", result)
	}
	files, err := NewGlobTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"pattern": "src/**/*.{go,ts}"}})
	if err != nil || files.Metadata["count"] != 3 {
		t.Fatalf("glob = %#v, %v", files, err)
	}
	if _, err := NewGrepTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"pattern": "["}}); err == nil {
		t.Fatal("invalid regex accepted")
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := NewGlobTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"pattern": "*"}}); err == nil || !strings.Contains(err.Error(), "install ripgrep") {
		t.Fatalf("missing dependency: %v", err)
	}
}

func TestPatchPreflightAndTextFormats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "one.txt")
	original := utf8BOM + "first\r\nsecond\r\nlast"
	if err := os.WriteFile(path, []byte(original), 0755); err != nil {
		t.Fatal(err)
	}
	apply := func(patch string) error {
		_, err := NewApplyPatchTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"patchText": patch}})
		return err
	}
	if err := apply("*** Begin Patch\n*** Update File: one.txt\n@@\n-first\n+new\n*** Delete File: missing.txt\n*** End Patch"); err == nil {
		t.Fatal("expected preflight failure")
	}
	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Fatal("first file changed before later failure")
	}
	if err := apply("*** Begin Patch\n*** Update File: one.txt\n@@\n first\n-last\n+changed\n*** End Patch"); err == nil {
		t.Fatal("matched disjoint context")
	}
	if err := apply("*** Begin Patch\n*** Update File: one.txt\n*** Move to: moved.txt\n@@\n-first\n+changed\n*** End Patch"); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(dir, "moved.txt")
	got, _ = os.ReadFile(moved)
	if string(got) != utf8BOM+"changed\r\nsecond\r\nlast" {
		t.Fatalf("format = %q", got)
	}
	info, _ := os.Stat(moved)
	if info.Mode().Perm() != 0755 {
		t.Fatalf("mode = %v", info.Mode())
	}
	_, err := NewEditTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"filePath": moved, "oldString": "changed\nsecond", "newString": "edited\nsecond"}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(moved)
	if string(got) != utf8BOM+"edited\r\nsecond\r\nlast" {
		t.Fatalf("edit format = %q", got)
	}
}

func TestEditPreservesUntouchedLineEndings(t *testing.T) {
	for _, tc := range []struct {
		name, original, old, next, want string
		all                             bool
	}{
		{"mixed", "first\r\nsecond\nthird\r\n", "second", "SECOND", "first\r\nSECOND\nthird\r\n", false},
		{"normalized match", utf8BOM + "first\r\nsecond\nthird\r\nlast", "first\nsecond", "FIRST\nSECOND", utf8BOM + "FIRST\nSECOND\nthird\r\nlast", false},
		{"all matches", "é\r\nold\nold\r\nlast", "old", "NEW", "é\r\nNEW\nNEW\r\nlast", true},
		{"CRLF replacement", "first\r\nsecond\r\n", "first\nsecond", "one\ntwo", "one\r\ntwo\r\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "text")
			if err := os.WriteFile(path, []byte(tc.original), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := NewEditTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{
				"filePath": path, "oldString": tc.old, "newString": tc.next, "replaceAll": tc.all,
			}})
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestPatchPreservesUntouchedLineEndings(t *testing.T) {
	for _, tc := range []struct {
		name, original, hunk, want string
	}{
		{"mixed", "first\r\nsecond\nthird\r\n", "@@\n first\n-second\n+SECOND\n third", "first\r\nSECOND\nthird\r\n"},
		{"anchor", utf8BOM + "first\r\nsecond\nthird\r\n", "@@ first\n-second\n+SECOND", utf8BOM + "first\r\nSECOND\nthird\r\n"},
		{"delete unterminated last", "first\nlast", "@@\n-last\n*** End of File", "first\n"},
		{"delete CRLF last", "first\r\nlast", "@@\n-last\n*** End of File", "first\r\n"},
		{"replace unterminated last", "first\nlast", "@@\n-last\n+LAST\n*** End of File", "first\nLAST"},
		{"keep unterminated context", "first\nlast", "@@\n-first\n+FIRST\n last", "FIRST\nlast"},
		{"append unterminated last", "first\nlast", "@@\n last\n+new\n*** End of File", "first\nlast\nnew"},
		{"delete whole file", "last", "@@\n-last", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "text")
			if err := os.WriteFile(path, []byte(tc.original), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := NewApplyPatchTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{
				"patchText": "*** Begin Patch\n*** Update File: text\n" + tc.hunk + "\n*** End Patch",
			}})
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestMutationLocksSerializeAliasesAndCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target")
	if err := os.WriteFile(path, []byte("one two"), 0644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockMutation(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if _, err := lockMutation(ctx, alias); err == nil {
		t.Fatal("alias bypassed lock")
	}
	unlock()
	var wg sync.WaitGroup
	for _, word := range []string{"one", "two"} {
		wg.Go(func() {
			_, err := NewEditTool().Execute(t.Context(), Invocation{WorkDir: dir, Input: map[string]any{"filePath": alias, "oldString": word, "newString": strings.ToUpper(word)}})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	got, _ := os.ReadFile(path)
	if string(got) != "ONE TWO" {
		t.Fatalf("lost edit: %q", got)
	}
	mutations.Lock()
	defer mutations.Unlock()
	if mutations.paths[path] != nil || mutations.paths[alias] != nil {
		t.Fatal("file locks retained")
	}
}
