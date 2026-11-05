package lopper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNested(t *testing.T) {
	tests := []struct {
		name string
		dirs []string // folders to create below the worktree
		want string
	}{
		{"only its own .git file", nil, ""},
		{"a repository two levels down", []string{"a/b/.git"}, "holds another repository or worktree"},
		{"a repository in a top-level node_modules", []string{"node_modules/x/.git"}, "holds another repository or worktree"},
		{"a repository in a deeper vendor folder", []string{"src/vendor/x/.git"}, ""},
		{"a repository at level 6", []string{"a/b/c/d/e/.git"}, "holds another repository or worktree"},
		{"a repository at level 7", []string{"a/b/c/d/e/f/.git"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wt := t.TempDir()
			if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, d := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(wt, d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if got := nested(wt); got != tt.want {
				t.Errorf("nested() = %q, want %q", got, tt.want)
			}
		})
	}
}

// unreadable makes a folder that this user cannot list, and restores it after the test.
func unreadable(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root can read every folder")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

func TestNestedUnreadableFolder(t *testing.T) {
	wt := t.TempDir()
	unreadable(t, filepath.Join(wt, "secret"))
	if got := nested(wt); got != "has a folder it cannot read" {
		t.Errorf("nested() = %q, want the unreadable-folder reason", got)
	}
}

func TestInspectUnreadableIsNotMissing(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "locked")
	unreadable(t, parent)
	class, reason, _ := Inspect(Target{Repo: "/nonexistent", Path: filepath.Join(parent, "wt"), Branch: "feat/x", Head: "-"})
	if class != Keep || reason != "cannot read the folder" {
		t.Errorf("Inspect() = %s %q, want KEEP \"cannot read the folder\"", class, reason)
	}
}

func TestGuardsSeeThisProcess(t *testing.T) {
	if !CanSeeProcesses() {
		t.Skip("no lsof and no /proc")
	}
	g := LoadGuards("/nonexistent", "/nonexistent")
	if !g.SeesProcesses() {
		t.Fatal("the in-use check did not list the working directory of this process")
	}
	wd, _ := os.Getwd()
	if got := g.Protected(wd); got != "in use by a running process" {
		t.Errorf("Protected(cwd) = %q, want in use", got)
	}
}
