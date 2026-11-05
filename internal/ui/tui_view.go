package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
)

const (
	minWidth  = 30
	minHeight = 12
)

const appKeys = `The home screen
  ↑ ↓  tab             choose a group
  ⏎  1 2 3             see the list of a group
  c                    remove the selected worktrees: you see a plan first,
                       then y removes them
  s  f                 scan with git fetch, scan without it

The list
  ↑ ↓  PgUp PgDn       move
  space                READY TO REMOVE: keep a worktree out of the clean, or put it back
                       NEEDS YOUR DECISION: select a worktree for the clean
  a                    READY TO REMOVE: all in, or all out
  /                    search in branch, repository, path and reason
  s                    sort: oldest first, largest first, by repository
  tab                  the next group
  esc                  back to the home screen

`

// View draws the app on the alternate screen.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m *Model) render() string {
	w, h, l := m.width, m.height, m.look
	if w < minWidth || h < minHeight {
		return "make the window larger"
	}
	keys := m.keys(l, w)
	switch m.screen {
	case homeScreen, listScreen:
		return m.base(l, m.screen, w, h) + "\n" + keys
	case pageScreen:
		return m.header(l, w) + "\n" + l.box(frame{title: l.Bold.Render(m.task), lines: strings.Split(m.page.View(), "\n"),
			w: w, h: h - 2, edge: l.Acc}) + "\n" + keys
	}
	return overlay(m.base(l.dim(), m.back, w, h), m.dialog(l, w, h-1), w, h-1) + "\n" + keys
}

// base is the home or the list screen without the keys: h-1 lines.
func (m *Model) base(l Look, s screen, w, h int) string {
	if s == listScreen && m.hasScan {
		return m.header(l, w) + "\n" + m.listView(l, w, h-5) + "\n" + m.details(l, w)
	}
	return m.header(l, w) + "\n\n" + m.home(l, w, h-3)
}

// overlay puts dialog in the middle of a w×h body.
func overlay(body, dialog string, w, h int) string {
	x, y := max((w-lipgloss.Width(dialog))/2, 0), max((h-lipgloss.Height(dialog))/2, 0)
	return lipgloss.NewCompositor(lipgloss.NewLayer(body), lipgloss.NewLayer(dialog).X(x).Y(y).Z(1)).Render()
}

// header is the name, the folder, the scan and what works on this system. When
// the window is narrow, it drops the badges, then the scan.
func (m *Model) header(l Look, w int) string {
	logo := " " + l.Acc.Bold(true).Render(l.Logo+" git lopper") + "  "
	scan, old := "", false
	if m.hasScan {
		repos := map[string]bool{}
		for _, r := range m.saved.Rows {
			repos[r.Repo] = true
		}
		scan = fmt.Sprintf(" · %d worktrees in %d repos · %s", len(m.saved.Rows), len(repos), scanned(m.saved.ScannedAt, time.Now()))
		old = time.Since(m.saved.ScannedAt) > 24*time.Hour
	}
	badges := l.Badge(m.hasGh, "gh") + " " + l.Badge(m.canSee, "in-use")
	if m.hasBloom {
		badges += " " + l.Badge(true, "Bloom")
	}
	sizing := ""
	if m.sized < m.sizeAll {
		sizing = l.Yellow.Render(fmt.Sprintf("sizes %d/%d", m.sized, m.sizeAll)) + "  "
	}
	right := sizing + badges
	need := min(lipgloss.Width(Tilde(m.root)), 24)
	room := func() int { return w - 2 - lipgloss.Width(logo) - lipgloss.Width(scan) - lipgloss.Width(right) }
	if room() < need {
		right = sizing
	}
	if room() < need {
		scan = ""
	}
	scanSt := l.Faint
	if old && scan != "" {
		scanSt, scan = l.Yellow, scan+": old, press s"
	}
	left := logo + l.Grey.Render(tail(Tilde(m.root), max(room(), 8))) + scanSt.Render(scan)
	return fit(left+strings.Repeat(" ", max(w-1-lipgloss.Width(left)-lipgloss.Width(right), 1))+right, w)
}

// scanned says when the scan ran: "scanned today 10:05", "scanned Mon 02 Nov".
func scanned(at, now time.Time) string {
	switch at.Format(time.DateOnly) {
	case now.Format(time.DateOnly):
		return "scanned today " + at.Format("15:04")
	case now.AddDate(0, 0, -1).Format(time.DateOnly):
		return "scanned yesterday " + at.Format("15:04")
	}
	return "scanned " + at.Format("Mon 02 Jan")
}

type keyHint struct {
	key, what   string
	off, danger bool
}

// keys is the bar of the keys that work on this screen, or a hint after a key
// that did nothing. It drops the last keys when they do not fit.
func (m *Model) keys(l Look, w int) string {
	if m.flash != "" {
		return " " + fit(l.Yellow.Bold(true).Render(m.flash), w-1)
	}
	selected := len(m.selection())
	removeN := fmt.Sprintf("remove %d", selected)
	var hints []keyHint
	switch m.screen {
	case homeScreen:
		hints = []keyHint{{key: l.UpDown, what: "group", off: !m.hasScan}, {key: l.Enter, what: "open", off: !m.hasScan},
			{key: "c", what: removeN, off: selected == 0}, {key: "s", what: "scan"}, {key: "f", what: "fast scan"},
			{key: "?", what: "help"}, {key: "q", what: "quit"}}
	case listScreen:
		if m.typing {
			hints = []keyHint{{key: "type", what: "to search"}, {key: l.Enter, what: "done"}, {key: "esc", what: "clear"}}
			break
		}
		hints = []keyHint{{key: l.UpDown, what: "move"}, {key: "space", what: "select", off: m.group == workGroup},
			{key: "a", what: "all", off: m.group != readyGroup}, {key: "/", what: "search"}, {key: "s", what: "sort"},
			{key: "c", what: removeN, off: selected == 0}, {key: "esc", what: "back"}, {key: "?", what: "help"}}
	case busyScreen:
		hints = []keyHint{{key: "ctrl+c", what: "stop"}}
	case planScreen:
		if todo := lopper.Todo(m.steps); todo > 0 {
			hints = []keyHint{{key: "y", what: fmt.Sprintf("remove %d", todo), danger: true}, {key: "v", what: "view plan"}, {key: "esc", what: "back"}}
		} else {
			hints = []keyHint{{key: "any key", what: "back"}}
		}
	case pageScreen:
		hints = []keyHint{{key: l.UpDown, what: "scroll"}, {key: "esc", what: "back"}}
	case noteScreen:
		hints = []keyHint{{key: "any key", what: "back"}}
	}
	var b strings.Builder
	used := 0
	for _, k := range hints {
		ks, ws := l.Acc.Bold(true), l.Grey
		switch {
		case k.off:
			ks, ws = l.Faint, l.Faint
		case k.danger:
			ks, ws = l.Red.Bold(true), l.Red
		}
		part := " " + ks.Render(k.key) + " " + ws.Render(k.what) + " "
		if used+lipgloss.Width(part) > w {
			break
		}
		b.WriteString(part)
		used += lipgloss.Width(part)
	}
	return b.String()
}

// dialog is the box of the busy, plan or note screen, at most w×h.
func (m *Model) dialog(l Look, w, h int) string {
	var lines []string
	title, edge := l.Acc.Bold(true).Render(m.task), l.Acc
	switch m.screen {
	case busyScreen:
		count := ""
		if m.prog.total > 0 {
			count = fmt.Sprintf(" %d/%d", m.prog.done, m.prog.total)
		}
		bw := min(w-8, 60)
		lines = []string{m.spin.View() + " " + m.detail, "",
			l.gradBar(m.prog.done, m.prog.total, bw-len(count)) + l.Bold.Render(count), l.Grey.Render(fit(m.prog.label, bw))}
	case planScreen:
		lines = m.planLines(l)
	default:
		title = l.Acc.Bold(true).Render(m.noteHead)
		for line := range strings.Lines(strings.Trim(m.note, "\n")) {
			lines = append(lines, strings.TrimRight(line, "\n"))
		}
	}
	return dialogBox(l, title, lines, w, h, edge)
}

// planLines tell what the clean will do: removals and the space they free,
// skips by reason, and the keys.
func (m *Model) planLines(l Look) []string {
	var lines []string
	for line := range strings.Lines(l.PlanSummary(m.steps, m.planSees)) {
		lines = append(lines, strings.TrimPrefix(strings.TrimRight(line, "\n"), "  "))
	}
	var going []string
	picked := 0
	for _, s := range m.steps {
		if s.Action != lopper.Skip {
			going = append(going, s.Row.Path)
			if s.Row.Class == lopper.Review {
				picked++
			}
		}
	}
	lines = append(lines, "")
	if free := m.sizeOf(going); free > 0 {
		lines = append(lines, "  "+l.Green.Bold(true).Render("frees "+humanBytes(free)))
	}
	if picked > 0 {
		lines = append(lines, "  "+l.Yellow.Render(fmt.Sprintf("%d of them you selected in NEEDS YOUR DECISION", picked)))
	}
	lines = append(lines, "  "+l.Faint.Render("never --force · branches stay · each worktree is checked again"), "")
	if todo := lopper.Todo(m.steps); todo > 0 {
		lines = append(lines, "  "+l.Red.Bold(true).Reverse(true).Render(" y ")+" "+l.Red.Render(fmt.Sprintf("remove %d", todo))+
			"   "+l.Key("v", "view plan")+"   "+l.Key("esc", "back"))
	} else {
		lines = append(lines, "  "+l.Grey.Render("nothing to remove"))
	}
	return lines
}

// dialogBox frames lines: as wide as the longest line (44 cells at least), at
// most w-4 wide and h high.
func dialogBox(l Look, title string, lines []string, w, h int, edge lipgloss.Style) string {
	most := 40
	for _, line := range lines {
		most = max(most, lipgloss.Width(line))
	}
	return l.box(frame{title: title, lines: lines, w: max(min(most+4, w-4), minWidth-4), h: min(len(lines)+2, h), edge: edge})
}
