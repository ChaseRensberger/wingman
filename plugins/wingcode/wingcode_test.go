package wingcode

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
)

func TestRepositoryContextRefreshesAndSupportsWorktrees(t *testing.T) {
	root := t.TempDir()
	generation, err := plugin.ActivateAll(New())
	if err != nil {
		t.Fatal(err)
	}
	defer generation.Close(t.Context())
	transform := generation.Runtime().Hooks.TransformSystem
	info := run.TransformSystemInfo{System: "plan: do not edit", WorkDir: root}
	before, err := transform(t.Context(), info)
	if err != nil || !strings.Contains(before, "Git repository: not detected") {
		t.Fatalf("before=%q err=%v", before, err)
	}
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-q")
	after, err := transform(t.Context(), info)
	canonical, _ := filepath.EvalSymlinks(root)
	if err != nil || !strings.Contains(after, "Repository root: "+canonical) || !strings.HasPrefix(after, "plan: do not edit") {
		t.Fatalf("after=%q err=%v", after, err)
	}
	git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-qm", "initial")
	worktree := filepath.Join(t.TempDir(), "linked")
	git("worktree", "add", "--detach", worktree)
	nested := filepath.Join(worktree, "nested")
	if err := os.Mkdir(nested, 0755); err != nil {
		t.Fatal(err)
	}
	info.WorkDir = nested
	text, err := transform(t.Context(), info)
	canonical, _ = filepath.EvalSymlinks(worktree)
	if err != nil || !strings.Contains(text, "Repository root: "+canonical) {
		t.Fatalf("linked=%q err=%v", text, err)
	}
}
