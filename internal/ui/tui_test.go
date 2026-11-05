package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
)

func press(m *Model, key string) tea.Cmd {
	_, cmd := m.Update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
	return cmd
}

func testModel(t *testing.T) *Model {
	t.Helper()
	cfg := lopper.Config{Depth: 3, CacheDir: t.TempDir(), BloomDB: "/nonexistent"}
	m := NewModel(cfg, t.TempDir(), "test")
	m.saved, m.hasScan = lopper.Saved{Rows: []lopper.Row{
		{Class: lopper.Safe, Reason: "already in the default branch", Repo: "/r/api", Path: "/w/login", Branch: "feat/login", LastCommit: "-"},
		{Class: lopper.Keep, Reason: "1 changed file", Repo: "/r/api", Path: "/w/typo", Branch: "fix/typo", LastCommit: "-"},
	}}, true
	return m
}

func TestTUIListAndBack(t *testing.T) {
	m := testModel(t)
	if !strings.Contains(m.render(), "2 worktrees in 1 repos") {
		t.Fatalf("the dashboard does not show the scan:\n%s", m.render())
	}
	press(m, "l")
	if m.screen != pickScreen || len(m.choices) != len(lopper.Classes) {
		t.Fatalf("l: screen %d with %d choices, want the pick screen with all classes", m.screen, len(m.choices))
	}
	press(m, "2") // SAFE
	if m.screen != pageScreen || !strings.Contains(m.render(), "feat/login") || strings.Contains(m.render(), "fix/typo") {
		t.Fatalf("2: want the list of SAFE only, got:\n%s", m.render())
	}
	press(m, "q")
	if m.screen != dashScreen {
		t.Fatalf("q on a page: screen %d, want the dashboard", m.screen)
	}
	if cmd := press(m, "q"); cmd == nil || cmd() != tea.Quit() {
		t.Fatal("q on the dashboard does not quit")
	}
}

func TestTUICleanOffersOnlyCleanableClasses(t *testing.T) {
	m := testModel(t)
	press(m, "c")
	if m.screen != pickScreen || len(m.choices) != len(lopper.Cleanable) {
		t.Fatalf("c: screen %d with %d choices, want the pick screen with the cleanable classes", m.screen, len(m.choices))
	}
	press(m, "9")
	if m.screen != dashScreen {
		t.Fatalf("a wrong number: screen %d, want the dashboard", m.screen)
	}
}

func TestTUIPlanNeedsWorkToConfirm(t *testing.T) {
	m := testModel(t)
	m.picked, m.screen = lopper.Safe, planScreen
	m.steps = []lopper.Step{{Action: lopper.Skip, Reason: "in use by a running process", Row: m.saved.Rows[0]}}
	if cmd := press(m, "y"); cmd != nil || m.screen != dashScreen {
		t.Fatalf("y with nothing to remove: screen %d, want the dashboard and no work", m.screen)
	}
}

func TestTUINoScanYet(t *testing.T) {
	m := testModel(t)
	m.hasScan = false
	press(m, "l")
	if m.screen != dashScreen || !strings.Contains(m.render(), "No scan yet") {
		t.Fatalf("l without a scan: screen %d, want the dashboard with a hint", m.screen)
	}
}
