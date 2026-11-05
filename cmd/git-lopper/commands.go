package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
	"github.com/albertovincenzi/git-lopper/internal/ui"
)

// app holds what every command needs.
type app struct {
	cfg         lopper.Config
	root        string
	look        ui.Look
	width       int
	hasProgress bool // stderr is a terminal
}

func newApp(cfg lopper.Config, root string) *app {
	width := 80
	if w, _, err := term.GetSize(os.Stdout.Fd()); err == nil && w > 0 {
		width = w
	}
	return &app{cfg: cfg, root: root, look: ui.NewLook(), width: width, hasProgress: isTerminal(os.Stderr)}
}

// progress draws a progress line on stderr, when stderr is a terminal.
func (a *app) progress(done, total int, label string) {
	if a.hasProgress {
		fmt.Fprint(os.Stderr, "\r\033[K")
		_, _ = lipgloss.Fprint(os.Stderr, a.look.Progress(done, total, label, a.width))
	}
}

func (a *app) progressDone() {
	if a.hasProgress {
		fmt.Fprint(os.Stderr, "\r\033[K")
	}
}

// load reads the saved scan of the root folder.
func (a *app) load() (lopper.Saved, error) {
	s, err := lopper.Load(a.cfg.ScanPath(a.root))
	if errors.Is(err, lopper.ErrNoScan) {
		return s, fmt.Errorf("no scan for %s yet: run 'git lopper scan' first", ui.Tilde(a.root))
	}
	return s, err
}

func (a *app) scan(fetch bool) int {
	saved, warn, err := a.cfg.ScanAndSave(a.root, fetch, a.progress)
	a.progressDone()
	if err != nil {
		return fail(err)
	}
	_, _ = lipgloss.Fprint(os.Stdout, a.look.Summary(saved.Rows, a.width))
	if warn != "" {
		_, _ = lipgloss.Fprint(os.Stderr, "\n"+a.look.Yellow.Render(warn)+"\n")
	}
	return 0
}

func (a *app) list(tsv bool, classes []lopper.Class) int {
	saved, err := a.load()
	if err != nil {
		return fail(err)
	}
	rows := lopper.Of(saved.Rows, classes)
	if tsv {
		if err := lopper.WriteTSV(os.Stdout, rows); err != nil {
			return fail(err)
		}
		return 0
	}
	_, _ = lipgloss.Fprint(os.Stdout, a.look.List(rows))
	return 0
}

// clean exits with 1 when git refused a removal.
func (a *app) clean(dry, yes bool, classes []lopper.Class) int {
	saved, err := a.load()
	if err != nil {
		return fail(err)
	}
	guards := lopper.LoadGuards(a.cfg.BloomDB, a.cfg.BloomDir)
	steps := lopper.MakePlan(classes, saved.Rows, guards, a.progress)
	a.progressDone()
	todo := lopper.Todo(steps)
	_, _ = lipgloss.Fprint(os.Stdout, a.look.Bold.Render("clean "+join(classes))+"  "+ui.Tilde(a.root)+"\n"+
		a.look.PlanSummary(steps, guards.SeesProcesses()))
	if dry {
		_, _ = lipgloss.Fprint(os.Stdout, "\n"+a.look.Plan(steps))
		return 0
	}
	if todo == 0 {
		return 0
	}
	if !yes {
		if !isTerminal(os.Stdin) {
			return fail(errors.New("no terminal to confirm: add --yes, or --dry-run to preview"))
		}
		fmt.Printf("\nRemove %d worktrees? [y/N] ", todo)
		answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if a := strings.TrimSpace(answer); a != "y" && a != "Y" && a != "yes" {
			fmt.Println("nothing removed.")
			return 0
		}
	}
	res, _, err := a.cfg.Clean(a.root, saved, classes, steps, a.progress)
	a.progressDone()
	_, _ = lipgloss.Fprint(os.Stdout, a.look.Result(res, a.cfg.LogPath()))
	if err != nil {
		return fail(err)
	}
	if res.Refused > 0 {
		return 1
	}
	return 0
}

func (a *app) tui() int {
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		return fail(errors.New("the interactive app needs a terminal (see -h for commands)"))
	}
	if err := ui.RunTUI(a.cfg, a.root, version); err != nil {
		return fail(err)
	}
	return 0
}
