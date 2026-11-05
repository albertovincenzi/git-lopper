package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
)

// The three groups of the home screen, in plain words.
const (
	readyGroup  = iota // PRUNE, SAFE, LIKELY: selected for the clean
	decideGroup        // REVIEW: the user selects them one by one
	workGroup          // KEEP: never removed
)

type group struct {
	title, blurb string
	classes      []lopper.Class
}

var groups = [3]group{
	{"READY TO REMOVE", "Their work is in the default branch, or in a merged or closed PR. " +
		"The branches stay, and git refuses to remove a worktree with changed files.", lopper.Cleanable},
	{"NEEDS YOUR DECISION", "Their commits are safe in a branch, but you may still use them. " +
		"Select the ones you do not need.", []lopper.Class{lopper.Review}},
	{"HAS YOUR WORK", "Changes, commits or operations that exist only here. git lopper never removes them.",
		[]lopper.Class{lopper.Keep}},
}

func groupOf(c lopper.Class) int {
	switch c {
	case lopper.Review:
		return decideGroup
	case lopper.Keep:
		return workGroup
	}
	return readyGroup
}

func (l Look) group(g int) lipgloss.Style {
	return [3]lipgloss.Style{l.Green, l.Yellow, l.Red}[g]
}

// advice is a reason in short words, what to do (short, for the cards) and why
// (for the details of a worktree).
type advice struct{ label, todo, why string }

// adviceFor explains the reason that the scan gave for a worktree.
func adviceFor(reason string) advice {
	switch reason {
	case "folder missing":
		return advice{"folder already deleted", "remove git's entry", "the folder is gone; only git's entry is left"}
	case "already in the default branch":
		return advice{"in the default branch", "remove it", "its commits are in the default branch"}
	case "PR merged":
		return advice{"PR merged", "remove it", "the PR is merged"}
	case "PR closed":
		return advice{"PR closed", "remove it", "the PR was closed without a merge; the branch keeps the commits"}
	case "remote branch deleted":
		return advice{"remote branch deleted", "remove it", "the branch keeps the commits"}
	case "PR open":
		return advice{"PR open", "wait for the merge", "or remove it if you work on it elsewhere; the branch stays"}
	case "no PR":
		return advice{"no PR", "open a PR, or remove it", "the branch keeps the commits"}
	case "never pushed":
		return advice{"never pushed", "push it, or remove it", "the local branch keeps the commits"}
	case "commits not on any remote":
		return advice{"commits not pushed", "push them, or remove it", "the local branch keeps them"}
	case "detached HEAD":
		return advice{"detached HEAD", "remove it if not needed", "its commits are also in other branches"}
	case "PR state unknown":
		return advice{"PR state unknown", "gh auth login, scan again", "without gh, git lopper cannot see the PRs"}
	case "unpushed commits":
		return advice{"unpushed commits", "push the branch", "some commits exist only here"}
	case "rebase or merge in progress":
		return advice{"rebase or merge", "finish or abort it", "removing it would lose the operation"}
	case "locked":
		return advice{"locked", "unlock it if not needed", "git worktree unlock PATH"}
	case "detached commits only here":
		return advice{"commits only here", "put them on a branch", "git branch NAME COMMIT, or they are lost"}
	}
	if strings.HasSuffix(reason, " changed file") || strings.HasSuffix(reason, " changed files") {
		return advice{"changed files", "commit, stash or discard", "git refuses to remove it with changes"}
	}
	return advice{reason, "check it by hand", ""}
}

// humanBytes writes a size on disk in KB, MB, GB or TB (powers of 1024).
func humanBytes(n int64) string {
	const k = 1024
	switch {
	case n < k*k:
		return fmt.Sprintf("%d KB", (n+k-1)/k)
	case n < k*k*k:
		return fmt.Sprintf("%d MB", n/(k*k))
	case n < k*k*k*k:
		return fmt.Sprintf("%.1f GB", float64(n)/(k*k*k))
	}
	return fmt.Sprintf("%.1f TB", float64(n)/(k*k*k*k))
}

// age writes how long ago a YYYY-MM-DD date was: today, 5d, 3w, 4mo, 2y.
func age(date string, now time.Time) string {
	t, err := time.ParseInLocation(time.DateOnly, date, now.Location())
	if err != nil {
		return "-"
	}
	switch days := int(now.Sub(t).Hours() / 24); {
	case days < 1:
		return "today"
	case days < 14:
		return fmt.Sprintf("%dd", days)
	case days < 60:
		return fmt.Sprintf("%dw", days/7)
	case days < 365:
		return fmt.Sprintf("%dmo", days/30)
	default:
		return fmt.Sprintf("%dy", days/365)
	}
}
