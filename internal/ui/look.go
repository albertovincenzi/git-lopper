// Package ui draws git-lopper's output: the CLI text and the terminal app.
package ui

import (
	"fmt"
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
)

// Look holds the styles and glyphs. Write styled text with lipgloss.Fprint (or
// through Bubble Tea): it drops the colours when the output is not a terminal,
// or when NO_COLOR is set.
type Look struct {
	Text, Bold, Acc, Grey, Faint, Green, Cyan, Yellow, Red lipgloss.Style

	SelBg color.Color    // the background of the selected row
	Grad  [2]color.Color // the two ends of the progress bar

	Dot, OK, Pause, Fail, Full, Empty, Rule, Logo      string
	Side, Thumb, Cursor, Prev, Next, UpDown, LeftRight string
	Corners                                            [4]string // top left, top right, bottom left, bottom right
}

// palette is a set of ANSI 256 colours: accent, grey, faint, green, cyan,
// yellow, red, the selection background and the end of the progress bar.
type palette [9]string

var (
	darkPalette  = palette{"141", "245", "240", "42", "45", "214", "203", "236", "45"}
	lightPalette = palette{"92", "242", "250", "28", "31", "130", "160", "254", "31"}
)

// NewLook makes the look for a dark terminal. Outside a UTF-8 locale, the glyphs are ASCII.
func NewLook() Look { return newLook(true) }

func newLook(dark bool) Look {
	p := lightPalette
	if dark {
		p = darkPalette
	}
	fg := func(c string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }
	l := Look{
		Text: lipgloss.NewStyle(), Bold: lipgloss.NewStyle().Bold(true), Acc: fg(p[0]), Grey: fg(p[1]), Faint: fg(p[2]),
		Green: fg(p[3]), Cyan: fg(p[4]), Yellow: fg(p[5]), Red: fg(p[6]),
		SelBg: lipgloss.Color(p[7]), Grad: [2]color.Color{lipgloss.Color(p[0]), lipgloss.Color(p[8])},
		Dot: "*", OK: "+", Pause: "-", Fail: "x", Full: "#", Empty: ".", Rule: "-", Logo: ">8",
		Side: "|", Thumb: "#", Cursor: ">", Prev: "<", Next: ">", UpDown: "j/k", LeftRight: "h/l",
		Corners: [4]string{"+", "+", "+", "+"},
	}
	if utf8Locale() {
		l.Dot, l.OK, l.Pause, l.Fail, l.Full, l.Empty, l.Rule, l.Logo = "●", "✔", "⏸", "✖", "█", "░", "─", "✂"
		l.Side, l.Thumb, l.Cursor, l.Prev, l.Next, l.UpDown, l.LeftRight = "│", "┃", "›", "‹", "›", "↑↓", "←→"
		l.Corners = [4]string{"╭", "╮", "╰", "╯"}
	}
	return l
}

func utf8Locale() bool {
	for _, n := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := strings.ToUpper(os.Getenv(n)); v != "" {
			return strings.Contains(v, "UTF-8") || strings.Contains(v, "UTF8")
		}
	}
	return false
}

// dim gives the same look in one faint colour, for the screen behind a dialog.
func (l Look) dim() Look {
	f := l.Faint
	l.Text, l.Bold, l.Acc, l.Grey, l.Green, l.Cyan, l.Yellow, l.Red = f, f, f, f, f, f, f, f
	l.SelBg = lipgloss.NoColor{}
	return l
}

// Class gives the colour of a class.
func (l Look) Class(c lopper.Class) lipgloss.Style {
	switch c {
	case lopper.Prune:
		return l.Grey
	case lopper.Safe:
		return l.Green
	case lopper.Likely:
		return l.Cyan
	case lopper.Keep:
		return l.Red
	}
	return l.Yellow
}

// Bar draws value of max in width cells; a value above 0 gets one cell at least.
func (l Look) Bar(value, max, width int, st lipgloss.Style) string {
	full := 0
	if max > 0 {
		full = min(value*width/max, width)
	}
	if value > 0 && full == 0 {
		full = 1
	}
	return st.Render(strings.Repeat(l.Full, full)) + l.Faint.Render(strings.Repeat(l.Empty, width-full))
}

// gradBar draws done of total in width cells, the done part in the accent gradient.
func (l Look) gradBar(done, total, width int) string {
	if width <= 0 {
		return ""
	}
	full := 0
	if total > 0 {
		full = min(done*width/total, width)
	}
	var b strings.Builder
	for _, c := range lipgloss.Blend1D(width, l.Grad[0], l.Grad[1])[:full] {
		b.WriteString(lipgloss.NewStyle().Foreground(c).Render(l.Full))
	}
	return b.String() + l.Faint.Render(strings.Repeat(l.Empty, width-full))
}

// Progress is one line: a bar, "done/total" and the label, cut to fit width.
func (l Look) Progress(done, total int, label string, width int) string {
	room := max(width-44, 10)
	if r := []rune(label); len(r) > room {
		label = string(r[:room])
	}
	return "  " + l.Bar(done, total, 28, l.Acc) + " " + l.Bold.Render(fmt.Sprintf("%3d/%-3d", done, total)) + " " + label
}

// Key draws a key and what it does, for the app's menus.
func (l Look) Key(key, what string) string {
	return l.Acc.Bold(true).Reverse(true).Render(" "+key+" ") + " " + what
}

// Badge shows a feature that works (or not) on this system.
func (l Look) Badge(ok bool, name string) string {
	if ok {
		return l.Green.Render(l.OK + " " + name)
	}
	return l.Grey.Render(l.Fail + " " + name)
}

// fit cuts s (styled or not) to w cells with "…", and pads it to w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// tail cuts the start of a path to w cells, so that the folder name stays: "…/code/api".
func tail(p string, w int) string {
	if n := ansi.StringWidth(p); n > w {
		return ansi.TruncateLeft(p, n-w+1, "…")
	}
	return p
}

// box draws lines in a w×h frame, with title in the top edge and the frame in
// the edge style. With more lines than room, off is the first line shown, and
// the right edge shows where the view is.
func (l Look) box(title string, lines []string, off, w, h int, edge lipgloss.Style) string {
	iw, vis := w-4, h-2
	title = ansi.Truncate(title, max(iw-1, 0), "…")
	tw := ansi.StringWidth(title)
	var b strings.Builder
	b.WriteString(edge.Render(l.Corners[0]+l.Rule+" ") + title + edge.Render(" "+strings.Repeat(l.Rule, max(w-tw-5, 0))+l.Corners[1]) + "\n")
	thumbFrom, thumbTo := -1, -1
	if len(lines) > vis && vis > 0 {
		size := max(vis*vis/len(lines), 1)
		thumbFrom = off * (vis - size) / (len(lines) - vis)
		thumbTo = thumbFrom + size
	}
	for i := range vis {
		line := ""
		if j := off + i; j < len(lines) {
			line = lines[j]
		}
		right := edge.Render(l.Side)
		if i >= thumbFrom && i < thumbTo {
			right = edge.Render(l.Thumb)
		}
		b.WriteString(edge.Render(l.Side) + " " + fit(line, iw) + " " + right + "\n")
	}
	b.WriteString(edge.Render(l.Corners[2] + strings.Repeat(l.Rule, w-2) + l.Corners[3]))
	return b.String()
}

// Tilde writes a path in the home folder as ~/...
func Tilde(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+"/"); ok {
		return "~/" + rest
	}
	return p
}
