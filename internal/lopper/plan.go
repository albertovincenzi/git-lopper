package lopper

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Action is what clean does with one worktree.
type Action string

// The actions of a clean plan.
const (
	Remove     Action = "remove" // git worktree remove
	PruneEntry Action = "prune"  // the folder is gone: drop git's entry
	Skip       Action = "skip"
)

// Step is one row of a clean plan.
type Step struct {
	Action Action
	Reason string
	Row    Row
}

// Decide checks a scanned worktree again, right before clean acts on it.
func Decide(wanted []Class, r Row, g Guards) (Action, string) {
	if why := g.Protected(r.Path); why != "" {
		return Skip, why
	}
	if r.Class == Prune {
		if !missing(r.Path) {
			return Skip, "folder exists again"
		}
		return PruneEntry, "folder missing"
	}
	// not locked: git itself refuses a locked worktree
	class, reason, _ := Inspect(Target{Repo: r.Repo, Path: r.Path, Branch: r.Branch, Base: r.Base, PR: r.PR, PRHead: r.PRHead, Head: "-"})
	if !slices.Contains(wanted, class) {
		return Skip, fmt.Sprintf("changed since the scan (now %s: %s)", class, reason)
	}
	if why := nested(r.Path); why != "" {
		return Skip, why
	}
	return Remove, reason
}

// MakePlan decides what to do with each row of a wanted class.
func MakePlan(wanted []Class, rows []Row, g Guards, report Progress) []Step {
	var picked []Row
	for _, r := range rows {
		if slices.Contains(wanted, r.Class) {
			picked = append(picked, r)
		}
	}
	steps := make([]Step, len(picked))
	c := &counter{total: len(picked), report: report}
	forEach(len(picked), func(i int) {
		action, reason := Decide(wanted, picked[i], g)
		steps[i] = Step{action, reason, picked[i]}
		c.step("checking " + filepath.Base(picked[i].Repo) + " · " + picked[i].Branch)
	})
	return steps
}

// Todo counts the steps that remove something.
func Todo(steps []Step) int {
	n := 0
	for _, s := range steps {
		if s.Action != Skip {
			n++
		}
	}
	return n
}

// Result is the outcome of RunPlan.
type Result struct {
	Done, Skipped, Refused int
	Removed                []string // paths
	Errors                 []string // git's error lines
}

// RunPlan removes the planned worktrees one at a time. It checks each one again
// first, with fresh guards. git's output goes to log.
func RunPlan(wanted []Class, steps []Step, g Guards, log io.Writer, report Progress) Result {
	var res Result
	todo, n := Todo(steps), 0
	for _, s := range steps {
		if s.Action == Skip {
			continue
		}
		n++
		if report != nil {
			report(n, todo, filepath.Base(s.Row.Repo)+" · "+filepath.Base(s.Row.Path))
		}
		if action, why := Decide(wanted, s.Row, g); action == Skip {
			res.Skipped++
			fmt.Fprintf(log, "SKIPPED %s: %s\n", s.Row.Path, why)
			continue
		}
		// also for a missing folder: unlike `worktree prune`, it drops only this entry
		cmd := exec.Command("git", "-C", s.Row.Repo, "worktree", "remove", s.Row.Path)
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		out, err := cmd.CombinedOutput()
		_, _ = log.Write(out)
		if err != nil {
			res.Refused++
			fmt.Fprintf(log, "REFUSED %s\n", s.Row.Path)
			res.Errors = append(res.Errors, "REFUSED "+s.Row.Path)
			for _, l := range lines(string(out)) {
				if strings.HasPrefix(l, "fatal") || strings.HasPrefix(l, "error") {
					res.Errors = append(res.Errors, "  "+l)
				}
			}
			continue
		}
		res.Done++
		res.Removed = append(res.Removed, s.Row.Path)
	}
	return res
}

// Clean runs a plan with fresh guards, writes git's output to the clean log, and
// drops the removed worktrees from the saved scan of root.
func (c Config) Clean(root string, saved Saved, wanted []Class, steps []Step, report Progress) (Result, Saved, error) {
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return Result{}, saved, fmt.Errorf("cannot create %s: %w", c.CacheDir, err)
	}
	log, err := os.Create(c.LogPath())
	if err != nil {
		return Result{}, saved, fmt.Errorf("cannot write the log: %w", err)
	}
	defer log.Close()
	// guards again: things may have changed while the plan was on screen
	res := RunPlan(wanted, steps, LoadGuards(c.BloomDB, c.BloomDir), log, report)
	saved = saved.Without(res.Removed)
	return res, saved, Save(c.ScanPath(root), saved)
}
