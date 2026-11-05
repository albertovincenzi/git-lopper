package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/albertovincenzi/git-lopper/v2/internal/lopper"
)

// wideWidth is the width from which the two lower cards of the home screen sit side by side.
const wideWidth = 110

var emptyCard = [3]string{"Nothing to remove now.", "Nothing to decide.", "No work that exists only here."}

// home is the three groups: what they hold, why, and what to do. It is h lines high.
func (m *Model) home(l Look, w, h int) string {
	if !m.hasScan {
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, m.welcome(l, w, h))
	}
	// the most that fits: fewer reasons per card first, then fewer descriptions
	fits := [][2]int{{6, 3}, {6, 1}, {4, 1}, {3, 1}, {3, 0}, {2, 0}, {1, 0}}
	var body string
	for _, f := range fits {
		if body = m.cards(l, w, f[0], f[1]); lipgloss.Height(body) <= h {
			break
		}
	}
	lines := strings.Split(body, "\n")
	return strings.Join(lines[:min(len(lines), h)], "\n") + strings.Repeat("\n", max(h-len(lines), 0))
}

// cards draws the three cards with at most reasons lines of reasons each, and
// the description of the first blurbs cards.
func (m *Model) cards(l Look, w, reasons, blurbs int) string {
	card := func(g, w, h int) string {
		lines := m.cardLines(l, g, w-4, reasons, g < blurbs)
		if h == 0 {
			h = len(lines) + 2
		}
		rows := lopper.Of(m.saved.Rows, groups[g].classes)
		st := l.group(g)
		return l.box(frame{title: st.Bold(true).Render(fmt.Sprintf("%d  %s", g+1, groups[g].title)),
			badge: st.Render(fmt.Sprintf("%d · %s", len(rows), m.sizeText(rows))), lines: lines, w: w, h: h, edge: st, heavy: g == m.group})
	}
	if w < wideWidth {
		return card(readyGroup, w, 0) + "\n" + card(decideGroup, w, 0) + "\n" + card(workGroup, w, 0)
	}
	half := w / 2
	h := max(len(m.cardLines(l, decideGroup, half-4, reasons, decideGroup < blurbs)),
		len(m.cardLines(l, workGroup, w-half-4, reasons, workGroup < blurbs))) + 2
	return card(readyGroup, w, 0) + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, card(decideGroup, half, h), card(workGroup, w-half, h))
}

// reasonCount is how many worktrees of a group share a reason, and their size.
type reasonCount struct {
	advice
	n    int
	rows []lopper.Row
}

// cardLines are the description (with blurb), the reasons and the keys of a card, iw cells wide.
func (m *Model) cardLines(l Look, g, iw, reasons int, blurb bool) []string {
	rows, st := lopper.Of(m.saved.Rows, groups[g].classes), l.group(g)
	var lines []string
	if blurb {
		for line := range strings.Lines(ansi.Wordwrap(groups[g].blurb, iw, "")) {
			lines = append(lines, l.Grey.Render(strings.TrimRight(line, "\n")))
		}
		lines = append(lines, "")
	}
	if len(rows) == 0 {
		return append(lines, st.Render(l.OK+" "+emptyCard[g]))
	}
	byLabel := map[string]*reasonCount{}
	for _, r := range rows {
		a := adviceFor(r.Reason)
		if byLabel[a.label] == nil {
			byLabel[a.label] = &reasonCount{advice: a}
		}
		byLabel[a.label].n++
		byLabel[a.label].rows = append(byLabel[a.label].rows, r)
	}
	list := make([]*reasonCount, 0, len(byLabel))
	for _, rc := range byLabel {
		list = append(list, rc)
	}
	slices.SortFunc(list, func(a, b *reasonCount) int { return cmp.Or(b.n-a.n, strings.Compare(a.label, b.label)) })
	for i, rc := range list {
		if i == reasons-1 && len(list) > reasons {
			rest := 0
			for _, more := range list[i:] {
				rest += more.n
			}
			lines = append(lines, l.Grey.Render(fmt.Sprintf("%4d  for %d more reasons: see the list", rest, len(list)-i)))
			break
		}
		what := l.Grey.Render(rc.todo)
		if g == readyGroup {
			what = l.Grey.Render(m.sizeText(rc.rows))
		}
		lines = append(lines, st.Bold(true).Render(fmt.Sprintf("%4d", rc.n))+"  "+fit(rc.label, 22)+" "+what)
	}
	return append(lines, "", m.cardKeys(l, g))
}

// cardKeys says what the keys do for a card; in colour only on the selected card.
func (m *Model) cardKeys(l Look, g int) string {
	key := func(k, what string) string {
		if g != m.group {
			return l.Faint.Render(k + " " + what)
		}
		return l.Key(k, what)
	}
	switch g {
	case readyGroup:
		in := 0
		for _, r := range lopper.Of(m.saved.Rows, lopper.Cleanable) {
			if m.isIn(r) {
				in++
			}
		}
		keys := key(l.Enter, "see the list") + "   " + key("c", fmt.Sprintf("remove %d after a check", in))
		if out := len(lopper.Of(m.saved.Rows, lopper.Cleanable)) - in; out > 0 {
			keys += l.Grey.Render(fmt.Sprintf("   %d kept out", out))
		}
		return keys
	case decideGroup:
		if n := len(m.chosenRows()); n > 0 {
			return key(l.Enter, "see the list") + l.Grey.Render(fmt.Sprintf("   %d selected for the clean", n))
		}
		return key(l.Enter, "see the list and select")
	}
	return key(l.Enter, "see the list")
}

func (m *Model) chosenRows() []lopper.Row {
	var rows []lopper.Row
	for _, r := range lopper.Of(m.saved.Rows, []lopper.Class{lopper.Review}) {
		if m.chosen[r.Path] {
			rows = append(rows, r)
		}
	}
	return rows
}

func (m *Model) welcome(l Look, w, h int) string {
	lines := []string{"", l.Bold.Render("No scan of this folder yet."), "",
		l.Grey.Render("A scan reads every worktree below this folder. It changes nothing."), "",
		l.Key("s", "scan") + "   " + l.Key("f", "fast scan, no git fetch"), ""}
	return dialogBox(l, l.Acc.Bold(true).Render("WELCOME"), lines, w, h, l.Acc)
}
