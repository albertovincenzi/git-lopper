package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
)

// press sends a key: a printable one ("q") or a named one ("down", "enter").
func press(m *Model, key string) tea.Cmd {
	named := map[string]rune{"down": tea.KeyDown, "up": tea.KeyUp, "right": tea.KeyRight, "left": tea.KeyLeft,
		"esc": tea.KeyEscape, "enter": tea.KeyEnter, "tab": tea.KeyTab, "space": tea.KeySpace, "backspace": tea.KeyBackspace}
	msg := tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
	if code, ok := named[key]; ok {
		msg = tea.KeyPressMsg{Code: code}
		if key == "space" {
			msg.Text = " "
		}
	}
	_, cmd := m.Update(msg)
	return cmd
}

func typeText(m *Model, text string) {
	for _, r := range text {
		press(m, string(r))
	}
}

// screenText is the screen without colours.
func screenText(m *Model) string { return ansi.Strip(m.render()) }

func testModel(t *testing.T) *Model {
	t.Helper()
	cfg := lopper.Config{Depth: 3, CacheDir: t.TempDir(), BloomDB: "/nonexistent"}
	m := NewModel(cfg, t.TempDir(), "test")
	m.saved, m.hasScan = lopper.Saved{ScannedAt: time.Now(), Rows: []lopper.Row{
		{Class: lopper.Safe, Reason: "already in the default branch", Repo: "/r/api", Path: "/w/login", Branch: "feat/login", LastCommit: "2026-10-30"},
		{Class: lopper.Likely, Reason: "PR merged", Repo: "/r/web", Path: "/w/deps", Branch: "chore/deps", LastCommit: "2026-09-01"},
		{Class: lopper.Review, Reason: "no PR", Repo: "/r/api", Path: "/w/idea", Branch: "local/idea", LastCommit: "2026-11-01"},
		{Class: lopper.Keep, Reason: "3 changed files", Repo: "/r/api", Path: "/w/typo", Branch: "fix/typo", LastCommit: "-"},
	}}, true
	m.sizes = map[string]int64{"/w/login": 2 << 30, "/w/deps": 300 << 20, "/w/idea": 1 << 20, "/w/typo": 5 << 20}
	m.sized, m.sizeAll = 4, 4
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

func TestTUIHomeExplainsTheGroups(t *testing.T) {
	m := testModel(t)
	out := screenText(m)
	for _, want := range []string{
		"READY TO REMOVE", "2 · 2.3 GB", "PR merged", "in the default branch", "remove 2 after a check",
		"NEEDS YOUR DECISION", "no PR", "open a PR, or remove it",
		"HAS YOUR WORK", "changed files", "commit, stash or discard",
		"4 worktrees in 2 repos", "scanned today",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the home screen does not show %q:\n%s", want, out)
		}
	}
	if cmd := press(m, "q"); cmd == nil || cmd() != tea.Quit() {
		t.Fatal("q on the home screen does not quit")
	}
}

func TestTUISelection(t *testing.T) {
	m := testModel(t)
	if got := len(m.selection()); got != 2 {
		t.Fatalf("at the start, %d worktrees are selected, want the 2 READY ones", got)
	}
	press(m, "enter") // the READY list, oldest first: chore/deps, then feat/login
	if m.screen != listScreen || m.group != readyGroup {
		t.Fatalf("enter: screen %d group %d, want the READY list", m.screen, m.group)
	}
	press(m, "space")
	if m.isIn(m.saved.Rows[1]) || len(m.selection()) != 1 || !strings.Contains(screenText(m), "kept out of the clean") {
		t.Fatalf("space did not keep chore/deps out of the clean:\n%s", screenText(m))
	}
	press(m, "a") // one is out: all in
	if len(m.selection()) != 2 {
		t.Fatalf("a with one worktree out: %d selected, want 2", len(m.selection()))
	}
	press(m, "tab") // NEEDS YOUR DECISION: nothing selected, a refused
	press(m, "a")
	if m.flash == "" || len(m.selection()) != 2 {
		t.Fatalf("a in NEEDS YOUR DECISION: flash %q, %d selected; want a hint and no change", m.flash, len(m.selection()))
	}
	press(m, "space")
	sel := m.selection()
	if len(sel) != 3 || !slices.Contains(wantedFor(sel), lopper.Review) || slices.Contains(wantedFor(sel), lopper.Keep) {
		t.Fatalf("after space on a REVIEW worktree: %d selected, wanted %v", len(sel), wantedFor(sel))
	}
	press(m, "tab") // HAS YOUR WORK: space never selects
	press(m, "space")
	if len(m.selection()) != 3 || !strings.Contains(m.flash, "has your work") {
		t.Fatalf("space on a KEEP worktree: flash %q, %d selected", m.flash, len(m.selection()))
	}
	if slices.Contains(wantedFor(m.saved.Rows[:2]), lopper.Review) {
		t.Fatal("without a REVIEW worktree, the clean may remove REVIEW")
	}
}

func TestTUISearchAndSort(t *testing.T) {
	m := testModel(t)
	press(m, "1")
	press(m, "/")
	typeText(m, "LOG")
	if rows := m.visible(); len(rows) != 1 || rows[0].Branch != "feat/login" {
		t.Fatalf("search LOG: %v, want feat/login only", rows)
	}
	press(m, "esc")
	if m.query != "" || m.typing || len(m.visible()) != 2 {
		t.Fatalf("esc in the search: query %q typing %v, want it cleared", m.query, m.typing)
	}
	if rows := m.visible(); rows[0].Branch != "chore/deps" {
		t.Fatalf("oldest first: %s comes first", rows[0].Branch)
	}
	press(m, "s")
	if rows := m.visible(); rows[0].Branch != "feat/login" || !strings.Contains(screenText(m), "largest first") {
		t.Fatalf("largest first: %s comes first", rows[0].Branch)
	}
	press(m, "esc")
	if m.screen != homeScreen {
		t.Fatalf("esc in the list: screen %d, want home", m.screen)
	}
}

func TestTUIPlanNeedsWorkToConfirm(t *testing.T) {
	m := testModel(t)
	m.back, m.screen = homeScreen, planScreen
	m.steps = []lopper.Step{{Action: lopper.Skip, Reason: "in use by a running process", Row: m.saved.Rows[0]}}
	if !strings.Contains(screenText(m), "nothing to remove") {
		t.Fatalf("a plan without removals does not say so:\n%s", screenText(m))
	}
	if cmd := press(m, "y"); cmd != nil || m.screen != homeScreen {
		t.Fatalf("y with nothing to remove: screen %d, want home and no work", m.screen)
	}
	m.screen = planScreen
	m.steps = []lopper.Step{{Action: lopper.Remove, Row: m.saved.Rows[0]}, {Action: lopper.Remove, Row: m.saved.Rows[2]}}
	out := screenText(m)
	if !strings.Contains(out, "frees 2.0 GB") || !strings.Contains(out, "1 of them you selected") {
		t.Fatalf("the plan does not show the space and the REVIEW worktree:\n%s", out)
	}
}

func TestTUINoScanYet(t *testing.T) {
	m := testModel(t)
	m.saved, m.hasScan = lopper.Saved{}, false
	if !strings.Contains(screenText(m), "No scan of this folder yet") {
		t.Fatalf("no welcome without a scan:\n%s", screenText(m))
	}
	if cmd := press(m, "c"); cmd != nil || m.screen != homeScreen {
		t.Fatal("c without a scan starts work")
	}
}

func TestTUIKeepsTheColumnNamesOnTop(t *testing.T) {
	m := testModel(t)
	for i := range 40 {
		m.saved.Rows = append(m.saved.Rows, lopper.Row{Class: lopper.Likely, Reason: "PR merged", Repo: "/r/big",
			Path: "/w/b" + string(rune('A'+i)), Branch: "feat/b" + string(rune('A'+i)), LastCommit: "2026-10-01"})
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	press(m, "1")
	for range 25 {
		press(m, "down")
	}
	top := strings.Split(screenText(m), "\n")[2]
	if !strings.Contains(top, "BRANCH") {
		t.Fatalf("after scrolling, the first line of the list is %q, want the column names", top)
	}
}

// Every screen fills the window exactly: no line is wider, and there is one line per row.
func TestTUIFitsTheWindow(t *testing.T) {
	m := testModel(t)
	long := strings.Repeat("very-long-branch-name/", 8)
	m.saved.Rows = append(m.saved.Rows, lopper.Row{Class: lopper.Safe, Reason: strings.Repeat("reason ", 10), Repo: "/r/" + long,
		Path: "/w/" + long + "à", Branch: long, LastCommit: "2026-11-05"})
	m.steps = []lopper.Step{{Action: lopper.Remove, Reason: "already in the default branch", Row: m.saved.Rows[0]}}
	screens := map[string]func(){
		"home":   func() { m.screen = homeScreen },
		"list":   func() { m.openList(readyGroup) },
		"search": func() { m.openList(decideGroup); m.typing, m.query = true, long },
		"busy": func() {
			m.back, m.screen, m.task, m.detail, m.prog = listScreen, busyScreen, "SCAN", "no fetch", progressMsg{3, 7, long}
		},
		"plan": func() { m.back, m.screen = homeScreen, planScreen },
		"note": func() {
			m.showNote("CLEAN", m.look.Result(lopper.Result{Done: 1, Errors: []string{long + long}}, "/log"))
		},
		"help":  func() { m.showPage("HELP", appKeys+Usage("test"), homeScreen) },
		"flash": func() { m.screen, m.flash = homeScreen, long },
	}
	for _, size := range [][2]int{{30, 12}, {50, 14}, {63, 20}, {80, 24}, {109, 30}, {110, 30}, {160, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for name, show := range screens {
			show()
			lines := strings.Split(m.render(), "\n")
			if len(lines) != size[1] {
				t.Errorf("%s at %dx%d: %d lines, want %d", name, size[0], size[1], len(lines), size[1])
			}
			for i, line := range lines {
				if lipgloss.Width(line) > size[0] {
					t.Errorf("%s at %dx%d: line %d is %d cells wide:\n%s", name, size[0], size[1], i, lipgloss.Width(line), line)
				}
			}
			m.flash, m.typing, m.query = "", false, ""
		}
	}
}

func TestAdviceCoversEveryReason(t *testing.T) {
	reasons := []string{"folder missing", "already in the default branch", "PR merged", "PR closed", "remote branch deleted",
		"PR open", "no PR", "never pushed", "commits not on any remote", "detached HEAD", "PR state unknown",
		"unpushed commits", "rebase or merge in progress", "locked", "detached commits only here", "1 changed file", "12 changed files"}
	for _, r := range reasons {
		if a := adviceFor(r); a.todo == "check it by hand" || a.label == "" {
			t.Errorf("no advice for %q", r)
		}
	}
}

func TestHumanBytesAndAge(t *testing.T) {
	for n, want := range map[int64]string{0: "0 KB", 1: "1 KB", 300 << 20: "300 MB", 3 << 29: "1.5 GB", 2 << 40: "2.0 TB"} {
		if got := humanBytes(n); got != want {
			t.Errorf("humanBytes(%d) = %q, want %q", n, got, want)
		}
	}
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.Local)
	for date, want := range map[string]string{"2026-11-05": "today", "2026-11-01": "4d", "2026-10-15": "3w",
		"2026-06-01": "5mo", "2024-11-01": "2y", "-": "-"} {
		if got := age(date, now); got != want {
			t.Errorf("age(%s) = %q, want %q", date, got, want)
		}
	}
}
