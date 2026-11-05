package lopper

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// detached is the branch name of a worktree with a detached HEAD.
const detached = "(detached)"

// Target is a worktree to inspect, with what the scan knows about it.
type Target struct {
	Repo, Path, Branch, Base string
	PR, PRHead               string // PRHead is the sha of the PR head, or "-"
	Locked                   bool
	Head                     string // HEAD from `git worktree list`, or "-"
}

// Inspect classifies a worktree. It also returns the upstream state.
func Inspect(t Target) (class Class, reason, upstream string) {
	if t.Locked {
		return Keep, "locked", UpDetached
	}
	fi, err := os.Stat(t.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// a missing detached worktree may hold the only reference to its commits
		if t.Branch == detached && t.Head != "-" && !reachable(t.Repo, t.Head) {
			return Keep, "detached commits only here", UpDetached
		}
		return Prune, "folder missing", UpDetached
	case err != nil:
		return Keep, "cannot read the folder", UpDetached
	case !fi.IsDir():
		return Keep, "not a folder", UpDetached
	}
	status, err := git(t.Path, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return Keep, "git status failed", UpDetached
	}
	f := Facts{PR: t.PR, Upstream: UpDetached, Dirty: len(lines(status)), Busy: inProgress(t.Path)}
	f.Merged = t.Base != "-" && gitOK(t.Path, "merge-base", "--is-ancestor", "HEAD", t.Base)
	if t.Branch != detached {
		f.Upstream, _ = git(t.Path, "for-each-ref",
			"--format=%(if)%(upstream)%(then)%(upstream:track)%(else)none%(end)", "refs/heads/"+t.Branch)
		if f.Upstream == "" {
			f.Upstream = UpOK
		}
	}
	if doneUpstream(f.PR, f.Upstream) {
		head, _ := git(t.Path, "rev-parse", "HEAD")
		f.OnRemote = onRemote(t.Path, head, t.PRHead)
	}
	class, reason = Classify(f)
	// without a branch, a commit that no ref contains is lost with the worktree
	if class != Keep && t.Branch == detached {
		if head, err := git(t.Path, "rev-parse", "HEAD"); err != nil || !reachable(t.Path, head) {
			return Keep, "detached commits only here", f.Upstream
		}
	}
	return class, reason, f.Upstream
}

// onRemote reports whether commit is on a remote ref, or in the PR head.
func onRemote(dir, commit, prHead string) bool {
	if reachable(dir, commit, "refs/remotes") {
		return true
	}
	if prHead == "-" || prHead == "" {
		return false
	}
	return commit == prHead || gitOK(dir, "merge-base", "--is-ancestor", commit, prHead)
}

// reachable reports whether a ref (under the given prefixes, or any ref) contains commit.
func reachable(dir, commit string, prefixes ...string) bool {
	out, err := git(dir, append([]string{"for-each-ref", "--contains", commit, "--count=1", "--format=x"}, prefixes...)...)
	return err == nil && out != ""
}

// inProgress reports a rebase, merge, cherry-pick, revert or bisect in the worktree.
func inProgress(wt string) bool {
	gitDir, err := git(wt, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return false
	}
	for _, f := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG"} {
		if exists(filepath.Join(gitDir, f)) {
			return true
		}
	}
	return false
}

// git runs git in dir with a fixed locale, so that its output can be parsed.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	return strings.TrimRight(string(out), "\n"), err
}

func gitOK(dir string, args ...string) bool {
	_, err := git(dir, args...)
	return err == nil
}

// lines splits output into its non-empty lines.
func lines(s string) []string {
	var list []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			list = append(list, l)
		}
	}
	return list
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// missing is true only when path does not exist; an unreadable path is not missing.
func missing(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, fs.ErrNotExist)
}

// command makes a command that is killed after limit; call cancel when done.
func command(limit time.Duration, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	return exec.CommandContext(ctx, name, args...), cancel
}
