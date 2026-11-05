package ui

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strconv"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
)

type screen int

const (
	browseScreen screen = iota // the classes and the worktrees of one class
	busyScreen                 // a scan, plan or clean runs
	planScreen                 // a clean plan, waiting for y
	pageScreen                 // a text to scroll: the help or a plan
	noteScreen                 // a message; any key goes back
)

// Model is the interactive app.
type Model struct {
	cfg                     lopper.Config
	root, version           string
	look                    Look
	hasGh, canSee, hasBloom bool

	saved   lopper.Saved
	hasScan bool

	screen   screen
	class    int    // the selected class: an index in lopper.Classes
	cursor   int    // the selected worktree of that class
	flash    string // a hint in the status line, until the next key
	picked   lopper.Class
	task     string // the busy or page title
	detail   string // what the busy task does
	prog     progressMsg
	spin     spinner.Model
	steps    []lopper.Step
	planSees bool // the in-use check worked for this plan
	note     string
	noteHead string
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
	sp := spinner.Line
	if utf8Locale() {
		sp = spinner.Dot
	}
	m.spin = spinner.New(spinner.WithSpinner(sp), spinner.WithStyle(m.look.Acc))
	saved, err := lopper.Load(cfg.ScanPath(root))
	switch {
	case err == nil:
		m.setScan(saved)
	case !errors.Is(err, lopper.ErrNoScan):
		m.showNote("ERROR", m.look.Red.Render(err.Error()))
	}
	return m
}

// RunTUI runs the app until the user quits.
func RunTUI(cfg lopper.Config, root, version string) error {
	_, err := tea.NewProgram(NewModel(cfg, root, version)).Run()
	return err
}

// Init asks the terminal for its background colour, to pick the light or dark palette.
func (m *Model) Init() tea.Cmd { return tea.RequestBackgroundColor }

// Update handles keys, the window size and the messages of background work.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.look = newLook(msg.IsDark())
		m.spin.Style = m.look.Acc
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.page.SetWidth(m.width - 4)
		m.page.SetHeight(m.listHeight())
	case spinner.TickMsg:
		if m.screen == busyScreen {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
	case progressMsg:
		m.prog = msg
		return m, m.wait()
	case scanDoneMsg:
		m.screen = browseScreen
		if msg.err == nil {
			m.setScan(msg.saved)
		}
		if note := errOr(msg.err, msg.warn); note != "" {
			m.showNote("SCAN", m.look.Yellow.Render(note))
		}
	case planDoneMsg:
		m.steps, m.planSees, m.screen = msg.steps, msg.seesProcesses, planScreen
	case cleanDoneMsg:
		m.saved = msg.saved
		m.cursor = max(min(m.cursor, len(m.rows())-1), 0)
		note := m.look.Result(msg.res, m.cfg.LogPath())
		if msg.err != nil {
			note += m.look.Red.Render(msg.err.Error())
		}
		m.showNote("CLEAN "+string(m.picked), note)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		m.flash = ""
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

// setScan shows a scan. When the selected class is empty, it selects the first class with worktrees.
func (m *Model) setScan(s lopper.Saved) {
	m.saved, m.hasScan = s, true
	for i := 0; len(m.rows()) == 0 && i < len(lopper.Classes); i++ {
		m.class = i
	}
	if len(m.rows()) == 0 {
		m.class = 0
	}
	m.cursor = max(min(m.cursor, len(m.rows())-1), 0)
}

func (m *Model) selected() lopper.Class { return lopper.Classes[m.class] }

// rows are the worktrees of the selected class.
func (m *Model) rows() []lopper.Row { return lopper.Of(m.saved.Rows, []lopper.Class{m.selected()}) }

// listHeight is the number of lines inside the panels.
func (m *Model) listHeight() int { return max(m.height-footerLines-2, 1) }

func (m *Model) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	switch m.screen {
	case busyScreen: // only ctrl+c stops background work
	case browseScreen:
		return m.browseKey(key)
	case planScreen:
		todo := lopper.Todo(m.steps)
		switch {
		case key == "y" && todo > 0:
			return m, m.startClean()
		case key == "v" && todo > 0:
			m.showPage("PLAN "+string(m.picked), m.look.Plan(m.steps), planScreen)
		default:
			m.screen = browseScreen
		}
	case pageScreen:
		switch key {
		case "q", "b", "esc", "left", "h":
			m.screen = m.pageBack
		default:
			var cmd tea.Cmd
			m.page, cmd = m.page.Update(k)
			return m, cmd
		}
	case noteScreen:
		m.screen = browseScreen
	}
	return m, nil
}

func (m *Model) browseKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, tea.Quit
	case "s", "f":
		return m, m.startScan(key == "s")
	case "c":
		return m, m.clean()
	case "?":
		m.showPage("HELP", appKeys+Usage(m.version), browseScreen)
	case "left", "h", "shift+tab":
		m.setClass(m.class - 1)
	case "right", "l", "tab":
		m.setClass(m.class + 1)
	case "up", "k":
		m.cursor--
	case "down", "j":
		m.cursor++
	case "pgup":
		m.cursor -= m.listHeight()
	case "pgdown":
		m.cursor += m.listHeight()
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = len(m.rows()) - 1
	default:
		if i, err := strconv.Atoi(key); err == nil && i >= 1 && i <= len(lopper.Classes) {
			m.setClass(i - 1)
		}
	}
	m.cursor = max(min(m.cursor, len(m.rows())-1), 0)
	return m, nil
}

func (m *Model) setClass(i int) {
	n := len(lopper.Classes)
	m.class, m.cursor = (i%n+n)%n, 0
}

// clean starts the plan for the selected class, or says why it cannot.
func (m *Model) clean() tea.Cmd {
	c := m.selected()
	switch {
	case !m.hasScan:
		m.flash = "no scan yet: press s"
	case !slices.Contains(lopper.Cleanable, c):
		m.flash = string(c) + " is never cleaned"
	case len(m.rows()) == 0:
		m.flash = "no " + string(c) + " worktrees to clean"
	default:
		m.picked = c
		return m.startPlan()
	}
	return nil
}

func (m *Model) showPage(title, text string, back screen) {
	m.page = viewport.New(viewport.WithWidth(m.width-4), viewport.WithHeight(m.listHeight()))
	m.page.SetContent(text)
	m.task, m.pageBack, m.screen = title, back, pageScreen
}

func (m *Model) showNote(head, text string) {
	m.noteHead, m.note, m.screen = head, text, noteScreen
}

// start runs job in the background; its progress and its result come back as messages.
func (m *Model) start(task, detail string, job func(report lopper.Progress) tea.Msg) tea.Cmd {
	m.screen, m.task, m.detail, m.prog = busyScreen, task, detail, progressMsg{}
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
	return tea.Batch(m.wait(), m.spin.Tick)
}

func (m *Model) wait() tea.Cmd {
	events := m.events
	return func() tea.Msg { return <-events }
}

func (m *Model) startScan(fetch bool) tea.Cmd {
	detail := "no fetch, PR state from gh"
	if fetch {
		detail = "git fetch --prune, then PR state from gh"
	}
	cfg, root := m.cfg, m.root
	return m.start("SCAN", detail, func(report lopper.Progress) tea.Msg {
		saved, warn, err := cfg.ScanAndSave(root, fetch, report)
		return scanDoneMsg{saved, warn, err}
	})
}

func (m *Model) startPlan() tea.Cmd {
	cfg, rows, wanted := m.cfg, m.saved.Rows, []lopper.Class{m.picked}
	return m.start("CLEAN "+string(m.picked), "checking every worktree again", func(report lopper.Progress) tea.Msg {
		guards := lopper.LoadGuards(cfg.BloomDB, cfg.BloomDir)
		return planDoneMsg{lopper.MakePlan(wanted, rows, guards, report), guards.SeesProcesses()}
	})
}

func (m *Model) startClean() tea.Cmd {
	cfg, root, saved, steps, wanted := m.cfg, m.root, m.saved, m.steps, []lopper.Class{m.picked}
	return m.start("CLEAN "+string(m.picked), "removing, one worktree at a time", func(report lopper.Progress) tea.Msg {
		res, saved, err := cfg.Clean(root, saved, wanted, steps, report)
		return cleanDoneMsg{res, err, saved}
	})
}
