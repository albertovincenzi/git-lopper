package lopper

import (
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name   string
		facts  Facts
		class  Class
		reason string
	}{
		{"locked wins", Facts{Locked: true, Merged: true, Upstream: UpOK, PR: "MERGED", OnRemote: true}, Keep, "locked"},
		{"work in progress", Facts{Busy: true, Merged: true, Upstream: UpOK, PR: "MERGED", OnRemote: true}, Keep, "rebase or merge in progress"},
		{"changes", Facts{Dirty: 2, Merged: true, Upstream: UpOK, PR: "MERGED", OnRemote: true}, Keep, "2 changed files"},
		{"one change", Facts{Dirty: 1, Merged: true, Upstream: UpOK, PR: "MERGED", OnRemote: true}, Keep, "1 changed file"},
		{"unpushed", Facts{Merged: true, Upstream: "[ahead 1, behind 3]", PR: "MERGED", OnRemote: true}, Keep, "unpushed commits"},
		{"merged", Facts{Merged: true, Upstream: UpOK, PR: PRNone}, Safe, "already in the default branch"},
		{"merged beats an open PR", Facts{Merged: true, Upstream: UpNone, PR: "OPEN"}, Safe, "already in the default branch"},
		{"PR merged", Facts{Upstream: UpOK, PR: "MERGED", OnRemote: true}, Likely, "PR merged"},
		{"PR closed", Facts{Upstream: UpOK, PR: "CLOSED", OnRemote: true}, Likely, "PR closed"},
		{"remote branch deleted", Facts{Upstream: "[gone]", PR: PRNone, OnRemote: true}, Likely, "remote branch deleted"},
		{"gone, commits only here", Facts{Upstream: "[gone]", PR: PRNone}, Review, "commits not on any remote"},
		{"merged PR, commits only here", Facts{Upstream: UpNone, PR: "MERGED"}, Review, "commits not on any remote"},
		{"PR open", Facts{Upstream: UpOK, PR: "OPEN"}, Review, "PR open"},
		{"never pushed", Facts{Upstream: UpNone, PR: PRNone}, Review, "never pushed"},
		{"detached", Facts{Upstream: UpDetached, PR: PRNone}, Review, "detached HEAD"},
		{"gh failed", Facts{Upstream: UpOK, PR: PRUnknown}, Review, "PR state unknown"},
		{"behind, no PR", Facts{Upstream: "[behind 2]", PR: PRNone}, Review, "no PR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			class, reason := Classify(tt.facts)
			if class != tt.class || reason != tt.reason {
				t.Errorf("Classify() = %s %q, want %s %q", class, reason, tt.class, tt.reason)
			}
		})
	}
}

func TestOverlaps(t *testing.T) {
	cwds := []string{"/p/busy/sub", "/p/x"}
	tests := []struct {
		name   string
		list   []string
		path   string
		either bool
		want   bool
	}{
		{"a cwd inside the path", cwds, "/p/busy", false, true},
		{"a cwd equal to the path", cwds, "/p/x", false, true},
		{"a sibling with the same prefix", cwds, "/p/bus", false, false},
		{"an empty list", nil, "/p", false, false},
		{"the path inside an entry, one direction", []string{"/b/ws"}, "/b/ws/inner", false, false},
		{"the path inside an entry, both directions", []string{"/b/ws"}, "/b/ws/inner", true, true},
		{"backslash and space", []string{`/p/a\b c`}, `/p/a\b c`, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := overlaps(tt.list, tt.path, tt.either); got != tt.want {
				t.Errorf("overlaps(%q, %q, %v) = %v, want %v", tt.list, tt.path, tt.either, got, tt.want)
			}
		})
	}
}

func TestProtected(t *testing.T) {
	tests := []struct {
		name   string
		guards Guards
		path   string
		want   string
	}{
		{"active Bloom workspace", Guards{bloomActive: []string{"/b/r/live"}}, "/b/r/live", "active Bloom workspace"},
		{"inside an active Bloom workspace", Guards{bloomActive: []string{"/b/r/live"}}, "/b/r/live/nested", "active Bloom workspace"},
		{"archived Bloom workspace", Guards{bloomActive: []string{"/b/r/live"}}, "/b/r/archived", ""},
		{"Bloom db unreadable", Guards{bloomDirs: []string{"/b"}, bloomUnreadable: true}, "/b/r/any", "Bloom workspace (Bloom db unreadable)"},
		{"Bloom db unreadable, outside Bloom", Guards{bloomDirs: []string{"/b"}, bloomUnreadable: true}, "/elsewhere/wt", ""},
		// Bloom stores NFD ("a" + combining grave), git prints NFC ("à")
		{"NFD in Bloom, NFC from git", withNFC(Guards{bloomActive: []string{"/b/r/dovrà"}}), "/b/r/dovrà", "active Bloom workspace"},
		{"in use", Guards{cwds: []string{"/p/busy/sub"}}, "/p/busy", "in use by a running process"},
		{"not in use: sibling with the same prefix", Guards{cwds: []string{"/p/busy/sub"}}, "/p/bus", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.guards.Protected(tt.path); got != tt.want {
				t.Errorf("Protected(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// withNFC normalises the Bloom paths the way LoadGuards does.
func withNFC(g Guards) Guards {
	g.bloomActive = withRealPaths(g.bloomActive)
	return g
}

func TestParsePorcelain(t *testing.T) {
	out := "worktree /r\nHEAD aaa\nbranch refs/heads/main\n\n" +
		"worktree /w/a b\nHEAD bbb\nbranch refs/heads/feat/x\nlocked reason here\n\n" +
		"worktree /w/d\nHEAD ccc\ndetached\nprunable gitdir file points to non-existent location\n"
	got := parsePorcelain(out)
	want := []entry{
		{path: "/r", branch: "main", head: "aaa"},
		{path: "/w/a b", branch: "feat/x", head: "bbb", locked: true},
		{path: "/w/d", branch: detached, head: "ccc"},
	}
	if len(got) != len(want) {
		t.Fatalf("parsePorcelain() gave %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// parsePorcelain reads text from git: it must never panic, and every entry must
// come from one "worktree " line.
func FuzzParsePorcelain(f *testing.F) {
	f.Add("worktree /r\nHEAD abc\nbranch refs/heads/main\n\nworktree /w\nHEAD def\ndetached\nlocked why\n")
	f.Add("worktree \n\nworktree /a b\tc\"d\nbranch refs/heads/x\n")
	f.Fuzz(func(t *testing.T, out string) {
		list := parsePorcelain(out)
		if len(list) > strings.Count(out, "worktree ") {
			t.Fatalf("%d entries from %d worktree lines", len(list), strings.Count(out, "worktree "))
		}
		for _, e := range list {
			if e.path == "" || strings.Contains(e.path, "\n") {
				t.Fatalf("bad path %q", e.path)
			}
		}
	})
}
