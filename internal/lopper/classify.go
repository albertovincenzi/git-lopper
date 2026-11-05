// Package lopper finds the linked git worktrees in a folder of repositories,
// sorts them into classes and removes the ones that are safe to remove.
package lopper

import (
	"fmt"
	"slices"
	"strings"
)

// Class says what clean may do with a worktree.
type Class string

// The classes, from "nothing left to lose" to "look at it first".
const (
	Prune  Class = "PRUNE"
	Safe   Class = "SAFE"
	Likely Class = "LIKELY"
	Keep   Class = "KEEP"
	Review Class = "REVIEW"
)

var (
	// Classes lists every class in display order.
	Classes = []Class{Prune, Safe, Likely, Keep, Review}
	// Cleanable lists the classes that clean accepts.
	Cleanable = []Class{Prune, Safe, Likely}
)

// Describe gives the meaning of a class in one short line.
func (c Class) Describe() string {
	switch c {
	case Prune:
		return "folder already gone, only git metadata left"
	case Safe:
		return "clean, HEAD already in the default branch"
	case Likely:
		return "clean, PR merged/closed or remote branch deleted"
	case Keep:
		return "changes, unpushed commits, rebase or lock"
	case Review:
		return "open PR, no PR, never pushed or local commits"
	}
	return ""
}

// ParseClass accepts a class name in any case.
func ParseClass(s string) (Class, bool) {
	c := Class(strings.ToUpper(s))
	return c, slices.Contains(Classes, c)
}

// PR states that gh does not give: gh says MERGED, CLOSED or OPEN.
const (
	PRNone    = "none"    // gh works, and no PR has this branch
	PRUnknown = "unknown" // gh is missing, not logged in, or failed
)

// Upstream states besides git's own track text ("[gone]", "[ahead 1]", ...).
const (
	UpNone     = "none" // the branch has no upstream
	UpOK       = "ok"   // the branch is in sync with its upstream
	UpDetached = "-"    // a detached HEAD has no branch
)

// Facts are what Classify needs to know about a worktree whose folder exists.
type Facts struct {
	Locked, Busy bool   // Busy: a rebase, merge, cherry-pick, revert or bisect
	Dirty        int    // changed and untracked files
	Merged       bool   // HEAD is in the default branch
	Upstream     string // an Up* value, or git's track text
	PR           string // MERGED, CLOSED, OPEN, PRNone or PRUnknown
	OnRemote     bool   // HEAD is on a remote branch, or in the PR
}

// Classify applies the rules in order; the first rule that matches wins.
func Classify(f Facts) (Class, string) {
	switch {
	case f.Locked:
		return Keep, "locked"
	case f.Busy:
		return Keep, "rebase or merge in progress"
	case f.Dirty == 1:
		return Keep, "1 changed file"
	case f.Dirty > 1:
		return Keep, fmt.Sprintf("%d changed files", f.Dirty)
	case strings.Contains(f.Upstream, "ahead"):
		return Keep, "unpushed commits"
	case f.Merged:
		return Safe, "already in the default branch"
	case doneUpstream(f.PR, f.Upstream):
		// done upstream, but only LIKELY when no commit exists only here
		switch {
		case !f.OnRemote:
			return Review, "commits not on any remote"
		case f.PR == "MERGED":
			return Likely, "PR merged"
		case f.PR == "CLOSED":
			return Likely, "PR closed"
		}
		return Likely, "remote branch deleted"
	case f.PR == "OPEN":
		return Review, "PR open"
	case f.Upstream == UpNone:
		return Review, "never pushed"
	case f.Upstream == UpDetached:
		return Review, "detached HEAD"
	case f.PR == PRUnknown:
		return Review, "PR state unknown"
	}
	return Review, "no PR"
}

func doneUpstream(pr, upstream string) bool {
	return pr == "MERGED" || pr == "CLOSED" || upstream == "[gone]"
}
