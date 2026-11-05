package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func pickKeys(p *picker, keys ...string) {
	for _, k := range keys {
		p.Update(keyMsg(k))
	}
}

func TestPickMany(t *testing.T) {
	p := newPicker("Which agents?", []Choice{{Label: "Claude Code", Badge: "found", On: true}, {Label: "Codex"}, {Label: "Cursor"}}, true)
	pickKeys(p, "down", "space", "down", "space", "space") // Codex in, Cursor in and out
	if got := p.chosen(); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("chosen %v, want [0 1]", got)
	}
	screen := ansi.Strip(p.render())
	for _, want := range []string{"Which agents?", "Claude Code", "found", "space select"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}

	pickKeys(p, "a", "a", "enter") // all in, all out: enter needs one
	if p.done || !strings.Contains(ansi.Strip(p.render()), "select one") {
		t.Fatalf("enter with nothing selected must ask for one:\n%s", ansi.Strip(p.render()))
	}
	pickKeys(p, "a", "enter")
	if !p.done || len(p.chosen()) != 3 {
		t.Fatalf("done %v, chosen %v", p.done, p.chosen())
	}
	if got := ansi.Strip(p.render()); !strings.Contains(got, "Claude Code, Codex, Cursor") {
		t.Errorf("the last screen must sum up the choice: %q", got)
	}
}

func TestPickOne(t *testing.T) {
	p := newPicker("Where?", []Choice{{Label: "your user"}, {Label: "this repository"}}, false)
	pickKeys(p, "down", "space", "enter") // space does nothing in a single choice
	if !p.done || !slices.Equal(p.chosen(), []int{1}) {
		t.Fatalf("done %v, chosen %v", p.done, p.chosen())
	}
}

func TestPickCancel(t *testing.T) {
	p := newPicker("Where?", []Choice{{Label: "your user"}}, false)
	pickKeys(p, "esc")
	if p.done || !p.cancelled {
		t.Fatalf("esc must cancel: done %v, cancelled %v", p.done, p.cancelled)
	}
}
