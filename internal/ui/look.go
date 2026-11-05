// Package ui draws git-lopper's output: the CLI text and the terminal app.
package ui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/albertovincenzi/git-lopper/internal/lopper"
)

// Look holds the styles and glyphs. Write styled text with lipgloss.Fprint (or
// through Bubble Tea): it drops the colours when the output is not a terminal,
// or when NO_COLOR is set.
type Look struct {
	Bold, Acc, Grey, Faint, Green, Cyan, Yellow, Red lipgloss.Style

	Dot, OK, Pause, Fail, Full, Empty, Rule, Logo string
}

// NewLook makes the look. Outside a UTF-8 locale, the glyphs are ASCII.
func NewLook() Look {
	fg := func(c string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }
	l := Look{
		Bold: lipgloss.NewStyle().Bold(true), Acc: fg("141"), Grey: fg("245"), Faint: fg("240"),
		Green: fg("42"), Cyan: fg("45"), Yellow: fg("214"), Red: fg("203"),
		Dot: "*", OK: "+", Pause: "-", Fail: "x", Full: "#", Empty: ".", Rule: "-", Logo: ">8",
	}
	locale := strings.ToUpper(firstSet("LC_ALL", "LC_CTYPE", "LANG"))
	if strings.Contains(locale, "UTF-8") || strings.Contains(locale, "UTF8") {
		l.Dot, l.OK, l.Pause, l.Fail, l.Full, l.Empty, l.Rule, l.Logo = "●", "✔", "⏸", "✖", "█", "░", "─", "✂"
	}
	return l
}

func firstSet(names ...string) string {
	for _, n := range names {
		if v := os.Getenv(n); v != "" {
			return v
		}
	}
	return ""
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
