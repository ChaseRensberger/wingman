package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnifiedPatchInsertionNotChangingFollowingLines(t *testing.T) {
	old := "line1\nline2\nline3\n"
	new := "line1\ninserted\nline2\nline3\n"

	patch, additions, deletions := unifiedPatch("file.txt", old, new)
	if additions != 1 || deletions != 0 {
		t.Fatalf("additions=%d deletions=%d, want 1 0", additions, deletions)
	}
	// The patch should preserve context: line2 and line3 should appear
	// as context lines (prefixed with space), not be re-emitted as
	// additions or deletions.
	lines := strings.Split(patch, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, " line2") || strings.HasPrefix(line, " line3") {
			return // found correct context line
		}
	}
	t.Fatalf("patch lost context lines:\n%s", patch)
}

func TestApplyPatchMoveMetadataUsesRelativePaths(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "old.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := NewApplyPatchTool().Execute(context.Background(), Invocation{
		WorkDir: workDir,
		Input: map[string]any{"patchText": strings.Join([]string{
			"*** Begin Patch",
			"*** Update File: old.txt",
			"*** Move to: nested/new.txt",
			"-old",
			"+new",
			"*** End Patch",
		}, "\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	files, ok := result.Metadata["files"].([]map[string]any)
	if !ok || len(files) != 1 {
		t.Fatalf("files = %#v", result.Metadata["files"])
	}
	if files[0]["relativePath"] != "old.txt" || files[0]["movePath"] != "nested/new.txt" {
		t.Fatalf("move metadata = %#v", files[0])
	}
}

func TestUnifiedPatchDeletion(t *testing.T) {
	old := "keep\nremove\nkeep\n"
	new := "keep\nkeep\n"

	patchText, additions, deletions := unifiedPatch("file.txt", old, new)
	if additions != 0 || deletions != 1 {
		t.Fatalf("additions=%d deletions=%d, want 0 1", additions, deletions)
	}
	if !strings.Contains(patchText, "-remove") {
		t.Fatalf("patch missing deletion:\n%s", patchText)
	}
}

func TestUnifiedPatchReplace(t *testing.T) {
	old := "a\nb\nc\n"
	new := "a\nB\nc\n"

	_, additions, deletions := unifiedPatch("file.txt", old, new)
	if additions != 1 || deletions != 1 {
		t.Fatalf("additions=%d deletions=%d, want 1 1", additions, deletions)
	}
}

func TestUnifiedPatchEmptyOldContent(t *testing.T) {
	old := ""
	new := "hello\nworld\n"

	_, additions, deletions := unifiedPatch("file.txt", old, new)
	if additions != 2 || deletions != 0 {
		t.Fatalf("additions=%d deletions=%d, want 2 0", additions, deletions)
	}
}

func TestUnifiedPatchDoesNotAddTrailingBlankLine(t *testing.T) {
	patch, _, _ := unifiedPatch("file.txt", "one\ntwo\n", "one\nchanged\n")
	if strings.Contains(patch, "@@ -1,3 +1,3 @@") {
		t.Fatalf("patch includes a synthetic trailing line:\n%s", patch)
	}
}

func TestUnifiedPatchCountsContentThatLooksLikeHeaders(t *testing.T) {
	patch, additions, deletions := unifiedPatch("file.txt", "--old\n", "++new\n")
	if additions != 1 || deletions != 1 {
		t.Fatalf("additions=%d deletions=%d, want 1 1\n%s", additions, deletions, patch)
	}
}

func TestUnifiedPatchSeparatesLinesWithoutTrailingNewline(t *testing.T) {
	patch, additions, deletions := unifiedPatch("file.txt", "old", "new")
	if additions != 1 || deletions != 1 {
		t.Fatalf("additions=%d deletions=%d, want 1 1\n%s", additions, deletions, patch)
	}
	if !strings.Contains(patch, "-old\n") || !strings.Contains(patch, "+new\n") || !strings.Contains(patch, "\\ No newline at end of file") {
		t.Fatalf("invalid no-newline patch:\n%s", patch)
	}
}
