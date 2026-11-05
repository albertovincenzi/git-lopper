package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
)

const (
	sideWidth   = 22 // the classes panel
	footerLines = 4  // two lines of details, the status line and the keys
	narrowWidth = 64 // below this width, the classes panel is hidden
	minWidth    = 30
	minHeight   = 10
)

const appKeys = `Keys
  ↑ ↓  j k  PgUp PgDn  move in the list of worktrees
  ← →  h l  tab  1-5   change the class
  c                    clean the class (PRUNE, SAFE or LIKELY): you see the plan,
                       then y removes the worktrees
  s  f                 scan with git fetch, scan without it
  ?  q                 this help, quit

`

// View draws the app on the alternate screen.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m *Model) render() string {
	w, h := m.width, m.height
	if w < minWidth || h < minHeight {
		return "make the window larger"
	}
	l, bodyH := m.look, h-footerLines
	var body string
	switch m.screen {
	case browseScreen:
		body = m.browser(l, w, bodyH)
		if !m.hasScan {
			body = overlay(body, m.noScanBox(l, w, bodyH), w, bodyH)
		}
	case pageScreen:
		body = l.box(l.Bold.Render(m.task), strings.Split(m.page.View(), "\n"), 0, w, bodyH, l.Acc)
	default:
		body = overlay(m.browser(l.dim(), w, bodyH), m.dialog(l, w, bodyH), w, bodyH)
	}
	return body + "\n" + m.details(l, w) + "\n" + m.status(l, w) + "\n" + m.keys(l, w)
}

// overlay puts dialog in the middle of a w×h body.
func overlay(body, dialog string, w, h int) string {
	x, y := max((w-lipgloss.Width(dialog))/2, 0), max((h-lipgloss.Height(dialog))/2, 0)
	return lipgloss.NewCompositor(lipgloss.NewLayer(body), lipgloss.NewLayer(dialog).X(x).Y(y).Z(1)).Render()
}

// browser is the classes panel and the list of the selected class; in a narrow window, only the list.
func (m *Model) browser(l Look, w, h int) string {
	if !m.hasScan {
		return lipgloss.Place(w, h, lipgloss.Left, lipgloss.Top, "")
	}
	if w < narrowWidth {
		return m.listBox(l, w, h, true)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.classBox(l, sideWidth, h), m.listBox(l, w-sideWidth, h, false))
}

func (m *Model) classBox(l Look, w, h int) string {
	iw := w - 4
	counts, repos := map[lopper.Class]int{}, map[string]bool{}
	for _, r := range m.saved.Rows {
		counts[r.Class]++
		repos[r.Repo] = true
	}
	var lines []string
	for i, c := range lopper.Classes {
		st, mark := l.Class(c), "  "
		if counts[c] == 0 {
			st = l.Faint
		}
		if i == m.class {
			st, mark = st.Bold(true).Background(l.SelBg), l.Cursor+" "
		}
		lines = append(lines, st.Render(fit(fmt.Sprintf("%s%-7s %5d", mark, c, counts[c]), iw)))
	}
	cleanable := 0
	for _, c := range lopper.Cleanable {
		cleanable += counts[c]
	}
	lines = append(lines, "", stack(l, counts, len(m.saved.Rows), iw),
		l.Grey.Render(fmt.Sprintf("%d worktrees", len(m.saved.Rows))),
		l.Grey.Render(fmt.Sprintf("%d repos", len(repos))), "",
		l.Green.Bold(true).Render(fmt.Sprint(cleanable))+l.Grey.Render(" cleanable"))
	return l.box(l.Bold.Render("Classes"), lines, 0, w, h, l.Faint)
}

// stack draws one bar with a part for each class, in the class colours.
func stack(l Look, counts map[lopper.Class]int, total, width int) string {
	var b strings.Builder
	sum, from := 0, 0
	for _, c := range lopper.Classes {
		sum += counts[c]
		to := sum * width / max(total, 1)
		b.WriteString(l.Class(c).Render(strings.Repeat(l.Full, to-from)))
		from = to
	}
	return b.String() + l.Faint.Render(strings.Repeat(l.Empty, width-from))
}

func (m *Model) listBox(l Look, w, h int, narrow bool) string {
	c, rows := m.selected(), m.rows()
	st := l.Class(c)
	name := st.Bold(true).Render(fmt.Sprintf("%s · %d", c, len(rows)))
	title := name + l.Grey.Render(" · "+c.Describe())
	if narrow {
		title = l.Faint.Render(l.Prev+" ") + name + l.Faint.Render(" "+l.Next)
	}
	if len(rows) == 0 {
		return l.box(title, []string{"", l.Faint.Render("no worktrees in this class")}, 0, w, h, st)
	}
	lines, heads, at := m.listLines(l, rows, w-4)
	vis := h - 2
	off := max(min(at-vis/3, len(lines)-vis), 0)
	if off > 0 && off != at && heads[off] != off {
		lines[off] = lines[heads[off]] // the repository of the first rows shown stays on top
	}
	return l.box(title, lines, off, w, h, st)
}

// listLines draws rows grouped by repository. heads gives, for each line, the
// line of its repository; at is the line of the selected row.
func (m *Model) listLines(l Look, rows []lopper.Row, iw int) (lines []string, heads []int, at int) {
	dw := 5 // MM-DD
	if iw >= 72 {
		dw = 10
	}
	rw := min(30, iw/3)
	bw := iw - rw - dw - 4
	if bw < 12 {
		rw, bw = 0, iw-dw-3
	}
	repo := ""
	for i, r := range rows {
		if r.Repo != repo {
			repo = r.Repo
			name := filepath.Base(repo)
			lines = append(lines, l.Bold.Render(name)+" "+l.Faint.Render(tail(Tilde(filepath.Dir(repo)), iw-len(name)-1)))
			heads = append(heads, len(lines)-1)
		}
		heads = append(heads, heads[len(heads)-1])
		sel := i == m.cursor
		paint := func(st lipgloss.Style, s string) string {
			if sel {
				st = st.Background(l.SelBg)
			}
			return st.Render(s)
		}
		mark := "  "
		if sel {
			mark, at = l.Cursor+" ", len(lines)
		}
		date := r.LastCommit
		if dw == 5 && len(date) == 10 {
			date = date[5:]
		}
		line := paint(l.Acc.Bold(true), mark) + paint(l.Text.Bold(sel), fit(r.Branch, bw)+" ")
		if rw > 0 {
			line += paint(l.Class(r.Class), fit(r.Reason, rw)+" ")
		}
		lines = append(lines, line+paint(l.Grey, fit(date, dw)))
	}
	return lines, heads, at
}

// details are two lines on the selected worktree: its path, then what the scan found.
func (m *Model) details(l Look, w int) string {
	rows := m.rows()
	if !m.hasScan {
		return "\n"
	}
	if len(rows) == 0 {
		return " " + l.Grey.Render(fit(m.selected().Describe(), w-1)) + "\n"
	}
	r := rows[m.cursor]
	facts := fmt.Sprintf(" · branch %s · base %s · upstream %s · PR %s · last commit %s", r.Branch, r.Base, r.Upstream, r.PR, r.LastCommit)
	return " " + fit(l.Bold.Render(tail(Tilde(r.Path), w-1)), w-1) + "\n " +
		fit(l.Class(r.Class).Render(l.Dot+" "+r.Reason)+l.Grey.Render(facts), w-1)
}

// status is the name, the folder, the scan time and what works on this system;
// or a hint. When the window is narrow, it drops the badges, then the time.
func (m *Model) status(l Look, w int) string {
	logo := l.Acc.Bold(true).Render(l.Logo+" git lopper") + "  "
	when, right := "", l.Badge(m.hasGh, "gh")+" "+l.Badge(m.canSee, "in-use")
	if m.hasScan {
		when = " · scanned " + m.saved.ScannedAt.Format("Mon 02 Jan 15:04")
	}
	if m.hasBloom {
		right += " " + l.Badge(true, "Bloom")
	}
	room := func() int { return w - 4 - lipgloss.Width(logo) - len(when) - lipgloss.Width(right) }
	if room() < 16 {
		right = ""
	}
	if room() < 16 {
		when = ""
	}
	left := logo + l.Grey.Render(tail(Tilde(m.root), room())) + l.Faint.Render(when)
	if m.flash != "" {
		left = l.Yellow.Bold(true).Render(m.flash)
	}
	gap := w - 2 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return " " + fit(left, w-1)
	}
	return " " + left + strings.Repeat(" ", gap) + right + " "
}

type keyHint struct {
	key, what   string
	off, danger bool
}

// keys is the bar of the keys that work on this screen. It drops the last ones when they do not fit.
func (m *Model) keys(l Look, w int) string {
	var hints []keyHint
	switch m.screen {
	case browseScreen:
		c := m.selected()
		canClean := m.hasScan && slices.Contains(lopper.Cleanable, c) && len(m.rows()) > 0
		hints = []keyHint{{key: l.UpDown, what: "move", off: !m.hasScan}, {key: l.LeftRight, what: "class", off: !m.hasScan},
			{key: "c", what: "clean " + string(c), off: !canClean}, {key: "s", what: "scan"}, {key: "f", what: "fast scan"},
			{key: "?", what: "help"}, {key: "q", what: "quit"}}
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

func (m *Model) noScanBox(l Look, w, h int) string {
	lines := []string{"", l.Bold.Render("No scan of this folder yet."), "",
		l.Key("s", "scan") + "   " + l.Key("f", "fast scan, no git fetch"), ""}
	return dialogBox(l, l.Acc.Bold(true).Render("WELCOME"), lines, w, h, l.Acc)
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
		for line := range strings.Lines(l.PlanSummary(m.steps, m.planSees)) {
			lines = append(lines, strings.TrimPrefix(strings.TrimRight(line, "\n"), "  "))
		}
		lines = append(lines, "", l.Faint.Render("  never --force · branches stay · each worktree is checked again"), "")
		if todo := lopper.Todo(m.steps); todo > 0 {
			lines = append(lines, "  "+l.Red.Bold(true).Reverse(true).Render(" y ")+" "+l.Red.Render(fmt.Sprintf("remove %d", todo))+
				"   "+l.Key("v", "view plan")+"   "+l.Key("esc", "back"))
		} else {
			lines = append(lines, "  "+l.Grey.Render("nothing to remove"))
		}
		st := l.Class(m.picked)
		title, edge = st.Bold(true).Render("CLEAN "+string(m.picked)), st
	default:
		title = l.Acc.Bold(true).Render(m.noteHead)
		for line := range strings.Lines(strings.Trim(m.note, "\n")) {
			lines = append(lines, strings.TrimRight(line, "\n"))
		}
	}
	return dialogBox(l, title, lines, w, h, edge)
}

// dialogBox frames lines: as wide as the longest line (44 cells at least), at
// most w-4 wide and h high.
func dialogBox(l Look, title string, lines []string, w, h int, edge lipgloss.Style) string {
	most := 40
	for _, line := range lines {
		most = max(most, lipgloss.Width(line))
	}
	return l.box(title, lines, 0, max(min(most+4, w-4), minWidth-4), min(len(lines)+2, h), edge)
}
