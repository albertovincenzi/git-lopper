package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
)

type screen int

const (
	dashScreen screen = iota
	pickScreen        // choose a class to list or clean
	busyScreen        // a scan, plan or clean runs
	planScreen        // a clean plan, waiting for y
	pageScreen        // a text to scroll: list, plan or help
	noteScreen        // a message; any key goes back to the dashboard
)

// header is the logo line, the rule and the scan info, with blank lines around.
const headerLines = 5

// Model is the interactive app.
type Model struct {
	cfg                     lopper.Config
	root, version           string
	look                    Look
	hasGh, canSee, hasBloom bool

	saved   lopper.Saved
	hasScan bool

	screen   screen
	cleaning bool // the pick screen is for clean, else for list
	choices  []lopper.Class
	picked   lopper.Class
	title    string // of the busy and page screens
	prog     progressMsg
	steps    []lopper.Step
	planSees bool // the in-use check worked for this plan
	note     string
	page     viewport.Model
	pageBack screen
	events   chan tea.Msg
	width    int
	height   int
}

type (
	progressMsg struct {
		done, total int
		label       string
	}
	scanDoneMsg struct {
		saved lopper.Saved
		warn  string
		err   error
	}
	planDoneMsg struct {
		steps         []lopper.Step
		seesProcesses bool
	}
	cleanDoneMsg struct {
		res   lopper.Result
		err   error
		saved lopper.Saved
	}
)

// NewModel opens the app on the saved scan of root, when there is one.
func NewModel(cfg lopper.Config, root, version string) *Model {
	_, ghErr := exec.LookPath("gh")
	_, dbErr := os.Stat(cfg.BloomDB)
	m := &Model{cfg: cfg, root: root, version: version, look: NewLook(),
		hasGh: ghErr == nil, canSee: lopper.CanSeeProcesses(), hasBloom: dbErr == nil, width: 80, height: 24}
	saved, err := lopper.Load(cfg.ScanPath(root))
	switch {
	case err == nil:
		m.saved, m.hasScan = saved, true
	case !errors.Is(err, lopper.ErrNoScan):
		m.note, m.screen = m.look.Red.Render("  "+err.Error())+"\n", noteScreen
	}
	return m
}

// RunTUI runs the app until the user quits.
func RunTUI(cfg lopper.Config, root, version string) error {
	_, err := tea.NewProgram(NewModel(cfg, root, version)).Run()
	return err
}

// Init starts nothing: the app waits for a key.
func (m *Model) Init() tea.Cmd { return nil }

// Update handles keys, the window size and the messages of background work.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.page.SetWidth(msg.Width)
		m.page.SetHeight(m.pageHeight())
	case progressMsg:
		m.prog = msg
		return m, m.wait()
	case scanDoneMsg:
		m.screen = dashScreen
		if msg.err == nil {
			m.saved, m.hasScan = msg.saved, true
		}
		if note := errOr(msg.err, msg.warn); note != "" {
			m.note, m.screen = "  "+m.look.Yellow.Render(note)+"\n", noteScreen
		}
	case planDoneMsg:
		m.steps, m.planSees, m.screen = msg.steps, msg.seesProcesses, planScreen
	case cleanDoneMsg:
		m.saved, m.screen = msg.saved, noteScreen
		m.note = m.look.Result(msg.res, m.cfg.LogPath())
		if msg.err != nil {
			m.note += "  " + m.look.Red.Render(msg.err.Error()) + "\n"
		}
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m.key(msg)
	}
	return m, nil
}

func errOr(err error, warn string) string {
	if err != nil {
		return err.Error()
	}
	return warn
}

func (m *Model) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	switch m.screen {
	case busyScreen: // only ctrl+c stops background work
	case dashScreen:
		switch key {
		case "q":
			return m, tea.Quit
		case "s", "f":
			return m, m.startScan(key == "s")
		case "l", "c":
			if m.hasScan {
				m.cleaning, m.screen = key == "c", pickScreen
				m.choices = lopper.Classes
				if m.cleaning {
					m.choices = lopper.Cleanable
				}
			}
		case "?":
			m.showPage("HELP", Usage(m.version), dashScreen)
		}
	case pickScreen:
		i, err := strconv.Atoi(key)
		if err != nil || i < 1 || i > len(m.choices) {
			m.screen = dashScreen
			break
		}
		m.picked = m.choices[i-1]
		if m.cleaning {
			return m, m.startPlan()
		}
		m.showPage("LIST "+string(m.picked), m.look.List(lopper.Of(m.saved.Rows, []lopper.Class{m.picked})), dashScreen)
	case planScreen:
		todo := lopper.Todo(m.steps)
		switch {
		case key == "y" && todo > 0:
			return m, m.startClean()
		case key == "v" && todo > 0:
			m.showPage("PLAN "+string(m.picked), m.look.Plan(m.steps), planScreen)
		default:
			m.screen = dashScreen
		}
	case pageScreen:
		switch key {
		case "q", "b", "esc", "left":
			m.screen = m.pageBack
		default:
			var cmd tea.Cmd
			m.page, cmd = m.page.Update(k)
			return m, cmd
		}
	case noteScreen:
		m.screen = dashScreen
	}
	return m, nil
}

func (m *Model) pageHeight() int { return max(m.height-headerLines-3, 3) }

func (m *Model) showPage(title, text string, back screen) {
	m.page = viewport.New(viewport.WithWidth(m.width), viewport.WithHeight(m.pageHeight()))
	m.page.SetContent(text)
	m.title, m.pageBack, m.screen = title, back, pageScreen
}

// start runs job in the background; its progress and its result come back as messages.
func (m *Model) start(title string, job func(report lopper.Progress) tea.Msg) tea.Cmd {
	m.screen, m.title, m.prog = busyScreen, title, progressMsg{}
	events := make(chan tea.Msg, 64)
	m.events = events
	go func() {
		events <- job(func(done, total int, label string) {
			select { // drop a progress step when the screen is behind; never the result
			case events <- progressMsg{done, total, label}:
			default:
			}
		})
	}()
	return m.wait()
}

func (m *Model) wait() tea.Cmd {
	events := m.events
	return func() tea.Msg { return <-events }
}

func (m *Model) startScan(fetch bool) tea.Cmd {
	title := "SCAN  no fetch, PR state from gh"
	if fetch {
		title = "SCAN  git fetch --prune, then PR state from gh"
	}
	cfg, root := m.cfg, m.root
	return m.start(title, func(report lopper.Progress) tea.Msg {
		saved, warn, err := cfg.ScanAndSave(root, fetch, report)
		return scanDoneMsg{saved, warn, err}
	})
}

func (m *Model) startPlan() tea.Cmd {
	cfg, rows, wanted := m.cfg, m.saved.Rows, []lopper.Class{m.picked}
	return m.start("CLEAN "+string(m.picked)+"  checking every worktree", func(report lopper.Progress) tea.Msg {
		guards := lopper.LoadGuards(cfg.BloomDB, cfg.BloomDir)
		return planDoneMsg{lopper.MakePlan(wanted, rows, guards, report), guards.SeesProcesses()}
	})
}

func (m *Model) startClean() tea.Cmd {
	cfg, root, saved, steps, wanted := m.cfg, m.root, m.saved, m.steps, []lopper.Class{m.picked}
	return m.start("CLEAN "+string(m.picked)+"  removing", func(report lopper.Progress) tea.Msg {
		res, saved, err := cfg.Clean(root, saved, wanted, steps, report)
		return cleanDoneMsg{res, err, saved}
	})
}

// View draws the header and the current screen on the alternate screen.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m *Model) render() string {
	var b strings.Builder
	b.WriteString(m.header())
	l := m.look
	switch m.screen {
	case dashScreen:
		if m.hasScan {
			b.WriteString(l.Summary(m.saved.Rows, m.width))
		} else {
			b.WriteString("  " + l.Yellow.Render("No scan yet.") + " Press " + l.Key("s", "scan") + " to start.\n")
		}
		b.WriteString("\n  " + strings.Join([]string{l.Key("s", "scan"), l.Key("f", "fast scan"), l.Key("l", "list"),
			l.Key("c", "clean"), l.Key("?", "help"), l.Key("q", "quit")}, "   ") + "\n")
	case pickScreen:
		if m.cleaning {
			b.WriteString("  " + l.Bold.Render("CLEAN") + "  which class?  " + l.Grey.Render("(KEEP and REVIEW are never cleaned)") + "\n\n")
		} else {
			b.WriteString("  " + l.Bold.Render("LIST") + "  which class?\n\n")
		}
		for i, c := range m.choices {
			fmt.Fprintf(&b, "    %s  %s %s %s\n", l.Bold.Render(strconv.Itoa(i+1)), l.Class(c).Render(l.Dot),
				l.Bold.Render(fmt.Sprintf("%-7s", c)), l.Grey.Render(c.Describe()))
		}
		b.WriteString("\n  " + l.Grey.Render("pick a number · any other key: back") + "\n")
	case busyScreen:
		b.WriteString("  " + l.Bold.Render(m.title) + "\n\n" + l.Progress(m.prog.done, m.prog.total, m.prog.label, m.width) + "\n")
	case planScreen:
		b.WriteString("  " + l.Bold.Render("CLEAN") + "  " + l.Class(m.picked).Bold(true).Render(string(m.picked)) + "\n\n")
		b.WriteString(l.PlanSummary(m.steps, m.planSees))
		if todo := lopper.Todo(m.steps); todo > 0 {
			b.WriteString("\n  " + l.Key("y", fmt.Sprintf("remove %d", todo)) + "   " + l.Key("v", "view plan") + "   " + l.Key("b", "back") + "\n")
		} else {
			b.WriteString("\n  nothing to remove.  " + l.Grey.Render("any key: back") + "\n")
		}
	case pageScreen:
		b.WriteString("  " + l.Bold.Render(m.title) + "\n" + m.page.View() + "\n  " + l.Grey.Render("↑/↓ scroll · q back") + "\n")
	case noteScreen:
		b.WriteString(m.note + "\n  " + l.Grey.Render("press any key") + "\n")
	}
	return b.String()
}

func (m *Model) header() string {
	l := m.look
	info := "no scan yet"
	if m.hasScan {
		repos := map[string]bool{}
		for _, r := range m.saved.Rows {
			repos[r.Repo] = true
		}
		info = fmt.Sprintf("%d worktrees in %d repos · scanned %s", len(m.saved.Rows), len(repos), m.saved.ScannedAt.Format("Mon 02 Jan 15:04"))
	}
	badges := l.Badge(m.hasGh, "gh") + " " + l.Badge(m.canSee, "in-use")
	if m.hasBloom {
		badges += " " + l.Badge(true, "Bloom")
	}
	return "\n  " + l.Acc.Bold(true).Render(l.Logo+"  GIT LOPPER") + "  " + l.Grey.Render(Tilde(m.root)) + "   " + badges +
		"\n  " + l.Faint.Render(strings.Repeat(l.Rule, 66)) + "\n  " + l.Grey.Render(info) + "\n\n"
}
