package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
)

// press sends a key: a printable one ("q") or a named one ("down", "right").
func press(m *Model, key string) tea.Cmd {
	named := map[string]rune{"down": tea.KeyDown, "up": tea.KeyUp, "right": tea.KeyRight, "left": tea.KeyLeft, "esc": tea.KeyEscape}
	msg := tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
	if code, ok := named[key]; ok {
		msg = tea.KeyPressMsg{Code: code}
	}
	_, cmd := m.Update(msg)
	return cmd
}

// screenText is the screen without colours.
func screenText(m *Model) string { return ansi.Strip(m.render()) }

func testModel(t *testing.T) *Model {
	t.Helper()
	cfg := lopper.Config{Depth: 3, CacheDir: t.TempDir(), BloomDB: "/nonexistent"}
	m := NewModel(cfg, t.TempDir(), "test")
	m.setScan(lopper.Saved{Rows: []lopper.Row{
		{Class: lopper.Safe, Reason: "already in the default branch", Repo: "/r/api", Path: "/w/login", Branch: "feat/login", LastCommit: "2026-10-30"},
		{Class: lopper.Safe, Reason: "already in the default branch", Repo: "/r/web", Path: "/w/deps", Branch: "chore/deps", LastCommit: "-"},
		{Class: lopper.Keep, Reason: "1 changed file", Repo: "/r/api", Path: "/w/typo", Branch: "fix/typo", LastCommit: "-"},
	}})
	return m
}

func TestTUIBrowse(t *testing.T) {
	m := testModel(t)
	if m.selected() != lopper.Safe {
		t.Fatalf("the app opens on %s, want the first class with worktrees (SAFE)", m.selected())
	}
	out := screenText(m)
	for _, want := range []string{"Classes", "SAFE · 2", "feat/login", "chore/deps", "/w/login", "2 cleanable"} {
		if !strings.Contains(out, want) {
			t.Errorf("the first screen does not show %q:\n%s", want, out)
		}
	}
	press(m, "down")
	if m.cursor != 1 || !strings.Contains(screenText(m), "/w/deps") {
		t.Fatalf("down: cursor %d, want the details of the second worktree", m.cursor)
	}
	press(m, "down") // stays on the last row
	press(m, "right")
	press(m, "right")
	if m.selected() != lopper.Keep || m.cursor != 0 || !strings.Contains(screenText(m), "fix/typo") {
		t.Fatalf("right twice: class %s cursor %d, want KEEP and the first row", m.selected(), m.cursor)
	}
	press(m, "1")
	if m.selected() != lopper.Prune || !strings.Contains(screenText(m), "no worktrees in this class") {
		t.Fatalf("1: class %s, want an empty PRUNE list", m.selected())
	}
	press(m, "left") // wraps to the last class
	if m.selected() != lopper.Review {
		t.Fatalf("left from the first class: %s, want REVIEW", m.selected())
	}
	if cmd := press(m, "q"); cmd == nil || cmd() != tea.Quit() {
		t.Fatal("q does not quit")
	}
}

func TestTUICleanRefusesKeepAndReview(t *testing.T) {
	m := testModel(t)
	for _, key := range []string{"4", "5"} {
		press(m, key)
		if cmd := press(m, "c"); cmd != nil || m.screen != browseScreen {
			t.Fatalf("c on %s starts work", m.selected())
		}
		if !strings.Contains(screenText(m), "is never cleaned") {
			t.Fatalf("c on %s gives no hint:\n%s", m.selected(), screenText(m))
		}
		press(m, "j") // any key clears the hint
		if m.flash != "" {
			t.Fatal("the hint stays after the next key")
		}
	}
}

func TestTUIPlanNeedsWorkToConfirm(t *testing.T) {
	m := testModel(t)
	m.picked, m.screen = lopper.Safe, planScreen
	m.steps = []lopper.Step{{Action: lopper.Skip, Reason: "in use by a running process", Row: m.saved.Rows[0]}}
	if !strings.Contains(screenText(m), "nothing to remove") {
		t.Fatalf("a plan without removals does not say so:\n%s", screenText(m))
	}
	if cmd := press(m, "y"); cmd != nil || m.screen != browseScreen {
		t.Fatalf("y with nothing to remove: screen %d, want the browser and no work", m.screen)
	}
}

func TestTUINoScanYet(t *testing.T) {
	m := testModel(t)
	m.saved, m.hasScan = lopper.Saved{}, false
	if !strings.Contains(screenText(m), "No scan of this folder yet") {
		t.Fatalf("no welcome without a scan:\n%s", screenText(m))
	}
	if cmd := press(m, "c"); cmd != nil || m.screen != browseScreen {
		t.Fatal("c without a scan starts work")
	}
}

func TestTUIKeepsTheRepositoryOnTop(t *testing.T) {
	m := testModel(t)
	for i := range 30 {
		m.saved.Rows = append(m.saved.Rows, lopper.Row{Class: lopper.Likely, Reason: "PR merged", Repo: "/r/big", Path: fmt.Sprint("/w/b", i), Branch: fmt.Sprint("feat/b", i)})
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	press(m, "3")
	for range 20 {
		press(m, "down")
	}
	top := strings.Split(screenText(m), "\n")[1]
	if !strings.Contains(top, "big") || strings.Contains(top, "feat/b") {
		t.Fatalf("after scrolling, the first line of the list is %q, want the repository", top)
	}
}

// Every screen fills the window exactly: no line is wider, and there is one line per row.
func TestTUIFitsTheWindow(t *testing.T) {
	m := testModel(t)
	long := strings.Repeat("very-long-branch-name/", 8)
	m.saved.Rows = append(m.saved.Rows, lopper.Row{Class: lopper.Safe, Reason: strings.Repeat("reason ", 10), Repo: "/r/" + long,
		Path: "/w/" + long + "à", Branch: long, LastCommit: "2026-11-05"})
	m.steps = []lopper.Step{{Action: lopper.Remove, Reason: "already in the default branch", Row: m.saved.Rows[0]}}
	m.picked = lopper.Safe
	screens := map[string]func(){
		"browse": func() { m.screen = browseScreen },
		"busy":   func() { m.screen, m.task, m.detail, m.prog = busyScreen, "SCAN", "no fetch", progressMsg{3, 7, long} },
		"plan":   func() { m.screen = planScreen },
		"note": func() {
			m.showNote("CLEAN SAFE", m.look.Result(lopper.Result{Done: 1, Errors: []string{long + long}}, "/log"))
		},
		"help": func() { m.showPage("HELP", appKeys+Usage("test"), browseScreen) },
	}
	for _, size := range [][2]int{{30, 10}, {50, 14}, {63, 20}, {64, 20}, {80, 24}, {160, 50}} {
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
		}
	}
}
