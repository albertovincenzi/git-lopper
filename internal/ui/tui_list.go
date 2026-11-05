package ui

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
)

type sortOrder int

const (
	byAge  sortOrder = iota // the oldest last commit first
	bySize                  // the largest first; the ones not measured yet last
	byName                  // repository, then branch
	sortOrders
)

var sortNames = [sortOrders]string{"oldest first", "largest first", "by repository"}

// visible is the list of the open group, searched and sorted.
func (m *Model) visible() []lopper.Row {
	rows := lopper.Of(m.saved.Rows, groups[m.group].classes)
	if q := strings.ToLower(m.query); q != "" {
		rows = slices.DeleteFunc(rows, func(r lopper.Row) bool {
			return !strings.Contains(strings.ToLower(r.Branch+" "+r.Path+" "+r.Repo+" "+r.Reason), q)
		})
	}
	slices.SortStableFunc(rows, m.compare)
	return rows
}

func (m *Model) compare(a, b lopper.Row) int {
	switch m.sortBy {
	case bySize:
		sa, okA := m.sizes[a.Path]
		sb, okB := m.sizes[b.Path]
		if okA != okB {
			if okA {
				return -1
			}
			return 1
		}
		return cmp.Compare(sb, sa)
	case byName:
		return cmp.Or(cmp.Compare(filepath.Base(a.Repo), filepath.Base(b.Repo)), cmp.Compare(a.Branch, b.Branch))
	}
	return cmp.Compare(a.LastCommit, b.LastCommit)
}

// isIn reports whether the clean removes the worktree.
func (m *Model) isIn(r lopper.Row) bool {
	switch groupOf(r.Class) {
	case readyGroup:
		return !m.excluded[r.Path]
	case decideGroup:
		return m.chosen[r.Path]
	}
	return false
}

// sizeText is the size of rows on disk; "…" marks a sum that still grows.
func (m *Model) sizeText(rows []lopper.Row) string {
	var n int64
	all := true
	for _, r := range rows {
		size, ok := m.sizes[r.Path]
		n, all = n+size, all && ok
	}
	if all {
		return humanBytes(n)
	}
	return humanBytes(n) + "…"
}

// listView is the box with the worktrees of the open group.
func (m *Model) listView(l Look, w, h int) string {
	g, st := groups[m.group], l.group(m.group)
	all, rows := lopper.Of(m.saved.Rows, g.classes), m.visible()
	count := fmt.Sprintf("%d · %s", len(all), m.sizeText(all))
	if m.group != workGroup {
		in := slices.DeleteFunc(slices.Clone(all), func(r lopper.Row) bool { return !m.isIn(r) })
		count = fmt.Sprintf("%d of %d selected · %s", len(in), len(all), m.sizeText(in))
	}
	f := frame{title: st.Bold(true).Render(g.title) + l.Grey.Render(" · "+count), badge: l.Grey.Render(sortNames[m.sortBy]),
		w: w, h: h, edge: st, heavy: true}
	if m.typing || m.query != "" {
		caret := ""
		if m.typing {
			caret = "_"
		}
		f.badge = l.Acc.Bold(true).Render("/"+m.query+caret) + l.Grey.Render(fmt.Sprintf(" · %d found", len(rows)))
	}
	if len(rows) == 0 {
		f.lines = []string{"", l.Faint.Render("no worktrees here")}
		return l.box(f)
	}
	lines, at := m.listLines(l, rows, w-4)
	vis := h - 2
	f.lines, f.off = lines, max(min(at-vis/3, len(lines)-vis), 0)
	if f.off > 0 && f.off != at {
		f.lines[f.off] = lines[0] // the column names stay on top
	}
	return l.box(f)
}

// listLines draws the column names, then one line per row; at is the line of the selected row.
func (m *Model) listLines(l Look, rows []lopper.Row, iw int) (lines []string, at int) {
	const markW, ageW, sizeW = 3, 5, 7
	repoW, whyW := 18, 22
	branchW := func() int {
		n := iw - markW - ageW - sizeW - 2
		if repoW > 0 {
			n -= repoW + 1
		}
		if whyW > 0 {
			n -= whyW + 1
		}
		return n
	}
	if branchW() < 16 {
		repoW = 0
	}
	if branchW() < 16 {
		whyW = 0
	}
	bw := branchW()
	col := func(s string, w int) string {
		if w == 0 {
			return ""
		}
		return fit(s, w) + " "
	}
	right := func(s string, w int) string { return strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) + s }
	lines = append(lines, l.Faint.Render(strings.Repeat(" ", markW)+col("BRANCH", bw)+col("REPOSITORY", repoW)+
		col("WHY", whyW)+right("AGE", ageW)+" "+right("SIZE", sizeW)))
	now, cur := time.Now(), m.cursor(rows)
	for i, r := range rows {
		sel, in := i == cur, m.isIn(r)
		paint := func(st lipgloss.Style, s string) string {
			if sel {
				st = st.Background(l.SelBg)
			}
			return st.Render(s)
		}
		mark, markSt, text := " ", l.group(m.group).Bold(true), l.Text
		if m.group != workGroup {
			mark = l.On
			if !in {
				mark, markSt = l.Off, l.Faint
			}
		}
		if !in && m.group == readyGroup {
			text = l.Faint // kept out of the clean
		}
		pointer := " "
		if sel {
			pointer, at = l.Cursor, len(lines)
		}
		size := "…"
		if n, ok := m.sizes[r.Path]; ok {
			size = humanBytes(n)
		}
		lines = append(lines, paint(l.Acc.Bold(true), pointer)+paint(markSt, mark+" ")+paint(text.Bold(sel), col(r.Branch, bw))+
			paint(l.Grey, col(filepath.Base(r.Repo), repoW))+paint(l.Class(r.Class), col(adviceFor(r.Reason).label, whyW))+
			paint(l.Grey, right(age(r.LastCommit, now), ageW)+" ")+paint(text, right(size, sizeW)))
	}
	return lines, at
}

// details are three lines on the selected worktree: its path, what the scan
// found, and what to do.
func (m *Model) details(l Look, w int) string {
	rows := m.visible()
	if len(rows) == 0 {
		return "\n\n"
	}
	r := rows[m.cursor(rows)]
	a := adviceFor(r.Reason)
	if a.why == "" {
		a.why = r.Reason
	}
	facts := fmt.Sprintf(" · %s · branch %s · base %s · upstream %s · PR %s · last commit %s",
		r.Class, r.Branch, r.Base, r.Upstream, r.PR, r.LastCommit)
	var hint string
	switch in := m.isIn(r); {
	case m.group == workGroup:
		hint = " · git lopper never removes it"
	case m.group == readyGroup && in:
		hint = " · space: keep it out of the clean"
	case m.group == readyGroup:
		hint = " · kept out of the clean · space: put it back"
	case in:
		hint = " · selected for the clean · space: unselect it"
	default:
		hint = " · space: select it for the clean"
	}
	return " " + fit(l.Bold.Render(tail(Tilde(r.Path), w-1)), w-1) + "\n" +
		" " + fit(l.Class(r.Class).Render(l.Dot+" "+r.Reason)+l.Grey.Render(facts), w-1) + "\n" +
		" " + fit(l.group(m.group).Render(l.Arrow+" "+a.todo)+l.Grey.Render(" · "+a.why+hint), w-1)
}
