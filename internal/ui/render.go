package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/albertovincenzi/git-lopper/v2/internal/lopper"
)

// Usage is the help text.
func Usage(version string) string {
	return fmt.Sprintf(`git-lopper %s — find stale git worktrees and remove them safely

Usage:
  git lopper [options]                    open the interactive app
  git lopper [options] scan [--no-fetch]  scan, save the result, print a summary
  git lopper [options] list [CLASS...]    print the saved scan (--tsv for raw rows)
  git lopper [options] clean CLASS...     remove PRUNE, SAFE or LIKELY worktrees
                                          (--dry-run to preview, --yes to skip the prompt)
  git lopper install-skill [AGENT...]     teach coding agents to use these commands
                                          (asks where and for which agents, or name them)
Options:
  -C, --root DIR   folder that holds your repositories (default: current folder)
  -d, --depth N    how deep to look for repositories (default: 3)
  -h, --help       show this help (use -h with 'git lopper': git maps --help to man)
  -V, --version    show the version

Classes:
  PRUNE   folder already gone, only git metadata left
  SAFE    clean, HEAD already in the default branch
  LIKELY  clean, PR merged or closed, or remote branch deleted
  KEEP    changes, unpushed commits, a rebase in progress, or locked  (never cleaned)
  REVIEW  open PR, no PR, never pushed, or commits only here          (never cleaned)

Safety: never uses --force and never deletes branches. Skips worktrees that a
running process uses, active Bloom workspaces, and worktrees that hold another
repository. Checks every worktree again right before it removes it.

Environment: LOPPER_DEPTH, LOPPER_CACHE_DIR, LOPPER_BLOOM_DB, LOPPER_BLOOM_DIR, NO_COLOR
`, version)
}

// Summary has one line per class: the count, a bar and, when width allows, the meaning.
func (l Look) Summary(rows []lopper.Row, width int) string {
	counts, most := map[lopper.Class]int{}, 1
	for _, r := range rows {
		counts[r.Class]++
		most = max(most, counts[r.Class])
	}
	var b strings.Builder
	for _, c := range lopper.Classes {
		st := l.Class(c)
		fmt.Fprintf(&b, "  %s %s %5d  %s", st.Render(l.Dot), l.Bold.Render(fmt.Sprintf("%-7s", c)), counts[c], l.Bar(counts[c], most, 22, st))
		if width >= 96 {
			b.WriteString("  " + l.Grey.Render(c.Describe()))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// List shows rows grouped by repository: branch, reason, last commit and path.
func (l Look) List(rows []lopper.Row) string {
	var b strings.Builder
	repo := ""
	for _, r := range rows {
		if r.Repo != repo {
			repo = r.Repo
			fmt.Fprintf(&b, "\n %s  %s\n", l.Bold.Render(filepath.Base(repo)), l.Grey.Render(Tilde(repo)))
		}
		st := l.Class(r.Class)
		fmt.Fprintf(&b, "   %s %-40.40s %s %s\n     %s\n", st.Render(fmt.Sprintf("%s %-6s", l.Dot, r.Class)), r.Branch,
			st.Render(fmt.Sprintf("%-30.30s", r.Reason)), l.Grey.Render(r.LastCommit), l.Grey.Render(Tilde(r.Path)))
	}
	return b.String()
}

// Plan shows each step of a clean plan: the action, the reason and the path.
func (l Look) Plan(steps []lopper.Step) string {
	var b strings.Builder
	for _, s := range steps {
		st := l.Green
		if s.Action == lopper.Skip {
			st = l.Yellow
		}
		fmt.Fprintf(&b, "  %s %-46.46s %s\n", st.Render(fmt.Sprintf("%-7s", s.Action)), s.Reason, l.Grey.Render(Tilde(s.Row.Path)))
	}
	return b.String()
}

// PlanSummary counts the removals, and the skips by reason.
func (l Look) PlanSummary(steps []lopper.Step, canSeeProcesses bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "    %s  %-34s %4d\n", l.Green.Render(l.OK), "remove", lopper.Todo(steps))
	skips := map[string]int{}
	for _, s := range steps {
		if s.Action == lopper.Skip {
			why, _, _ := strings.Cut(s.Reason, " (") // "changed since the scan (now ...)"
			skips[why]++
		}
	}
	reasons := make([]string, 0, len(skips))
	for why := range skips {
		reasons = append(reasons, why)
	}
	slices.SortFunc(reasons, func(a, b string) int {
		if skips[a] != skips[b] {
			return skips[b] - skips[a]
		}
		return strings.Compare(a, b)
	})
	for _, why := range reasons {
		fmt.Fprintf(&b, "    %s  %-34s %4d\n", l.Yellow.Render(l.Pause), "skip: "+why, skips[why])
	}
	if !canSeeProcesses {
		fmt.Fprintf(&b, "\n    %s\n", l.Red.Render(l.Fail+" lsof or /proc did not work: running sessions cannot be detected"))
	}
	return b.String()
}

// Result tells what a clean did, with git's errors and the log path.
func (l Look) Result(res lopper.Result, logPath string) string {
	var b strings.Builder
	b.WriteString("\n  " + l.Green.Bold(true).Render(fmt.Sprintf("%s removed %d", l.OK, res.Done)))
	if res.Skipped > 0 {
		b.WriteString("   " + l.Yellow.Bold(true).Render(fmt.Sprintf("%s skipped %d (changed since the plan)", l.Pause, res.Skipped)))
	}
	if res.Refused > 0 {
		b.WriteString("   " + l.Red.Bold(true).Render(fmt.Sprintf("%s refused by git %d", l.Fail, res.Refused)))
	}
	b.WriteString("\n")
	for _, e := range res.Errors[:min(len(res.Errors), 12)] {
		b.WriteString("    " + e + "\n")
	}
	b.WriteString("  " + l.Grey.Render("log: "+Tilde(logPath)) + "\n")
	return b.String()
}
