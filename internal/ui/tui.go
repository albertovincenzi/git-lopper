package ui

import (
	"errors"
	"os"
	"os/exec"
	"slices"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/albertovincenzi/git-lopper/v2/internal/lopper"
)

type screen int

const (
	homeScreen screen = iota // the three groups
	listScreen               // the worktrees of one group
	busyScreen               // a scan, plan or clean runs
	planScreen               // a clean plan, waiting for y
	pageScreen               // a text to scroll: the help or a plan
	noteScreen               // a message; any key goes back
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
	back     screen // the home or list screen under a dialog
	group    int    // the selected group on the home screen, or the group of the list
	curPath  string // the selected worktree of the list
	sortBy   sortOrder
	query    string
	typing   bool            // the keys go to the search
	excluded map[string]bool // READY worktrees taken out of the clean
	chosen   map[string]bool // REVIEW worktrees put in the clean
	flash    string          // a hint in place of the keys, until the next key

	sizes   map[string]int64 // size on disk by path, measured in the background
	sizeGen int
	sizeCh  chan tea.Msg
	sized   int // measured so far, of sizeAll
	sizeAll int

	task, detail string // the busy or page title, and what the busy task does
	prog         progressMsg
	spin         spinner.Model
	wanted       []lopper.Class
	steps        []lopper.Step
	planSees     bool // the in-use check worked for this plan
	note         string
	noteHead     string
	page         viewport.Model
	pageBack     screen
	events       chan tea.Msg
	width        int
	height       int
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
	sizeMsg struct {
		gen  int
		path string
		size int64
	}
)

// NewModel opens the app on the saved scan of root, when there is one.
func NewModel(cfg lopper.Config, root, version string) *Model {
	_, ghErr := exec.LookPath("gh")
	_, dbErr := os.Stat(cfg.BloomDB)
	m := &Model{cfg: cfg, root: root, version: version, look: NewLook(),
		hasGh: ghErr == nil, canSee: lopper.CanSeeProcesses(), hasBloom: dbErr == nil, width: 80, height: 24,
		excluded: map[string]bool{}, chosen: map[string]bool{}, sizes: map[string]int64{}}
	sp := spinner.Line
	if utf8Locale() {
		sp = spinner.Dot
	}
	m.spin = spinner.New(spinner.WithSpinner(sp), spinner.WithStyle(m.look.Acc))
	saved, err := lopper.Load(cfg.ScanPath(root))
	switch {
	case err == nil:
		m.saved, m.hasScan = saved, true
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

// Init asks the terminal for its background colour (to pick the light or dark
// palette), and starts to measure the worktrees.
func (m *Model) Init() tea.Cmd { return tea.Batch(tea.RequestBackgroundColor, m.startSizing()) }

// Update handles keys, the window size and the messages of background work.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.look = newLook(msg.IsDark())
		m.spin.Style = m.look.Acc
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.page.SetWidth(m.width - 4)
		m.page.SetHeight(m.height - 4)
	case spinner.TickMsg:
		if m.screen == busyScreen {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
	case sizeMsg:
		if msg.gen != m.sizeGen {
			return m, nil // from the scan before this one
		}
		m.sizes[msg.path] = msg.size
		if m.sized++; m.sized < m.sizeAll {
			return m, wait(m.sizeCh)
		}
	case progressMsg:
		m.prog = msg
		return m, wait(m.events)
	case scanDoneMsg:
		m.screen = m.back
		if msg.err != nil || msg.warn != "" {
			m.showNote("SCAN", m.look.Yellow.Render(errOr(msg.err, msg.warn)))
		}
		if msg.err == nil {
			m.saved, m.hasScan = msg.saved, true
			return m, m.startSizing()
		}
	case planDoneMsg:
		m.steps, m.planSees, m.screen = msg.steps, msg.seesProcesses, planScreen
	case cleanDoneMsg:
		m.saved = msg.saved
		note := m.look.Result(msg.res, m.cfg.LogPath())
		if freed := m.sizeOf(msg.res.Removed); freed > 0 {
			note = m.look.Green.Bold(true).Render("  freed "+humanBytes(freed)) + "\n" + note
		}
		if msg.err != nil {
			note += m.look.Red.Render(msg.err.Error())
		}
		m.showNote("CLEAN", note)
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

func (m *Model) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	switch m.screen {
	case busyScreen: // only ctrl+c stops background work
	case homeScreen:
		return m.homeKey(key)
	case listScreen:
		return m.listKey(k)
	case planScreen:
		todo := lopper.Todo(m.steps)
		switch {
		case key == "y" && todo > 0:
			return m, m.startClean()
		case key == "v" && todo > 0:
			m.showPage("PLAN", m.look.Plan(m.steps), planScreen)
		default:
			m.screen = m.back
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
		m.screen = m.back
	}
	return m, nil
}

func (m *Model) homeKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		return m, tea.Quit
	case "s", "f":
		return m, m.startScan(key == "s")
	case "?":
		m.showPage("HELP", appKeys+Usage(m.version), homeScreen)
	case "c":
		return m, m.clean()
	case "up", "k", "shift+tab":
		m.group = (m.group + len(groups) - 1) % len(groups)
	case "down", "j", "tab":
		m.group = (m.group + 1) % len(groups)
	case "enter", "right", "l":
		m.openList(m.group)
	case "1", "2", "3":
		m.openList(int(key[0] - '1'))
	}
	return m, nil
}

func (m *Model) listKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := k.String()
	if m.typing {
		switch key {
		case "enter", "down", "up":
			m.typing = false
		case "esc":
			m.query, m.typing = "", false
		case "backspace":
			if r := []rune(m.query); len(r) > 0 {
				m.query = string(r[:len(r)-1])
			}
		default:
			m.query += k.Text
		}
		return m, nil
	}
	switch key {
	case "esc", "q", "left", "h":
		if m.query != "" && key == "esc" {
			m.query = ""
			break
		}
		m.screen = homeScreen
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup":
		m.move(-m.listRows())
	case "pgdown":
		m.move(m.listRows())
	case "home", "g":
		m.move(-len(m.saved.Rows))
	case "end", "G":
		m.move(len(m.saved.Rows))
	case "space":
		m.toggle()
	case "a":
		m.toggleAll()
	case "/":
		m.typing = true
	case "s":
		m.sortBy = (m.sortBy + 1) % sortOrders
	case "tab":
		m.openList((m.group + 1) % len(groups))
	case "shift+tab":
		m.openList((m.group + len(groups) - 1) % len(groups))
	case "c":
		return m, m.clean()
	case "?":
		m.showPage("HELP", appKeys+Usage(m.version), listScreen)
	}
	return m, nil
}

func (m *Model) openList(g int) {
	m.group, m.screen, m.curPath, m.query, m.typing = g, listScreen, "", "", false
}

// listRows is the number of worktrees that the list shows at once.
func (m *Model) listRows() int { return max(m.height-8, 1) }

// move moves the selection in the list by delta rows.
func (m *Model) move(delta int) {
	rows := m.visible()
	if len(rows) == 0 {
		return
	}
	i := max(min(m.cursor(rows)+delta, len(rows)-1), 0)
	m.curPath = rows[i].Path
}

func (m *Model) cursor(rows []lopper.Row) int {
	return max(slices.IndexFunc(rows, func(r lopper.Row) bool { return r.Path == m.curPath }), 0)
}

// toggle puts the selected worktree in the clean, or takes it out.
func (m *Model) toggle() {
	rows := m.visible()
	if len(rows) == 0 {
		return
	}
	r := rows[m.cursor(rows)]
	m.curPath = r.Path
	switch m.group {
	case readyGroup:
		m.excluded[r.Path] = !m.excluded[r.Path]
	case decideGroup:
		m.chosen[r.Path] = !m.chosen[r.Path]
	default:
		m.flash = "this worktree has your work: " + adviceFor(r.Reason).todo
	}
}

// toggleAll puts all the worktrees of the list in the clean, or takes them out.
// It works only for READY TO REMOVE: the others are chosen one by one.
func (m *Model) toggleAll() {
	if m.group != readyGroup {
		m.flash = "select these worktrees one by one, with space"
		return
	}
	rows := m.visible()
	anyOut := slices.ContainsFunc(rows, func(r lopper.Row) bool { return m.excluded[r.Path] })
	for _, r := range rows {
		m.excluded[r.Path] = !anyOut // one or more out: all in; else all out
	}
}

// selection is the worktrees that the clean will remove.
func (m *Model) selection() []lopper.Row {
	var rows []lopper.Row
	for _, r := range m.saved.Rows {
		if g := groupOf(r.Class); g == readyGroup && !m.excluded[r.Path] || g == decideGroup && m.chosen[r.Path] {
			rows = append(rows, r)
		}
	}
	return rows
}

// clean starts the plan for the selection, or says why it cannot.
func (m *Model) clean() tea.Cmd {
	rows := m.selection()
	switch {
	case !m.hasScan:
		m.flash = "no scan yet: press s"
	case len(rows) == 0:
		m.flash = "nothing is selected for the clean"
	default:
		m.wanted = wantedFor(rows)
		return m.startPlan(rows)
	}
	return nil
}

// wantedFor gives the classes that a clean of rows may remove: REVIEW only
// when the user selected a REVIEW worktree.
func wantedFor(rows []lopper.Row) []lopper.Class {
	wanted := slices.Clone(lopper.Cleanable)
	if slices.ContainsFunc(rows, func(r lopper.Row) bool { return r.Class == lopper.Review }) {
		wanted = append(wanted, lopper.Review)
	}
	return wanted
}

func (m *Model) sizeOf(paths []string) int64 {
	var n int64
	for _, p := range paths {
		n += m.sizes[p]
	}
	return n
}

func (m *Model) showPage(title, text string, back screen) {
	m.page = viewport.New(viewport.WithWidth(m.width-4), viewport.WithHeight(m.height-4))
	m.page.SetContent(text)
	m.task, m.pageBack, m.screen = title, back, pageScreen
}

func (m *Model) showNote(head, text string) {
	if m.screen == homeScreen || m.screen == listScreen {
		m.back = m.screen
	}
	m.noteHead, m.note, m.screen = head, text, noteScreen
}

// startSizing measures every worktree of the scan in the background.
func (m *Model) startSizing() tea.Cmd {
	if !m.hasScan || len(m.saved.Rows) == 0 {
		return nil
	}
	m.sizeGen++
	m.sizes, m.sized, m.sizeAll = map[string]int64{}, 0, len(m.saved.Rows)
	gen, paths := m.sizeGen, make([]string, 0, len(m.saved.Rows))
	for _, r := range m.saved.Rows {
		paths = append(paths, r.Path)
	}
	ch := make(chan tea.Msg, len(paths)) // never blocks: the measures of an old scan end on their own
	m.sizeCh = ch
	go lopper.DiskSizes(paths, func(p string, n int64) { ch <- sizeMsg{gen, p, n} })
	return wait(ch)
}

// start runs job in the background; its progress and its result come back as messages.
func (m *Model) start(task, detail string, job func(report lopper.Progress) tea.Msg) tea.Cmd {
	if m.screen == homeScreen || m.screen == listScreen {
		m.back = m.screen
	}
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
	return tea.Batch(wait(events), m.spin.Tick)
}

func wait(ch chan tea.Msg) tea.Cmd { return func() tea.Msg { return <-ch } }

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

func (m *Model) startPlan(rows []lopper.Row) tea.Cmd {
	cfg, wanted := m.cfg, m.wanted
	return m.start("CLEAN", "checking every worktree again", func(report lopper.Progress) tea.Msg {
		guards := lopper.LoadGuards(cfg.BloomDB, cfg.BloomDir)
		return planDoneMsg{lopper.MakePlan(wanted, rows, guards, report), guards.SeesProcesses()}
	})
}

func (m *Model) startClean() tea.Cmd {
	cfg, root, saved, steps, wanted := m.cfg, m.root, m.saved, m.steps, m.wanted
	return m.start("CLEAN", "removing, one worktree at a time", func(report lopper.Progress) tea.Msg {
		res, saved, err := cfg.Clean(root, saved, wanted, steps, report)
		return cleanDoneMsg{res, err, saved}
	})
}
