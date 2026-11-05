package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Choice is one line of the list that Pick shows.
type Choice struct {
	Label string
	Badge string // in green after the label, as "found"
	Note  string // in grey at the end of the line
	On    bool   // selected at the start, in a multiple choice
}

// Pick shows a list below the prompt and lets the user choose one line, or with
// many, any number of lines. It returns the indexes of the chosen lines, or nil
// when the user cancels. One line that sums up the choice stays on the screen.
func Pick(question string, choices []Choice, many bool) ([]int, error) {
	p := newPicker(question, choices, many)
	if _, err := tea.NewProgram(p).Run(); err != nil {
		return nil, err
	}
	if !p.done {
		return nil, nil
	}
	return p.chosen(), nil
}

type picker struct {
	look            Look
	question        string
	choices         []Choice
	many            bool
	cur             int
	warn            string
	done, cancelled bool
}

func newPicker(question string, choices []Choice, many bool) *picker {
	return &picker{look: NewLook(), question: question, choices: slices.Clone(choices), many: many}
}

func (p *picker) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (p *picker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		p.look = newLook(msg.IsDark())
	case tea.KeyPressMsg:
		p.warn = ""
		switch msg.String() {
		case "up", "k":
			p.cur = max(p.cur-1, 0)
		case "down", "j":
			p.cur = min(p.cur+1, len(p.choices)-1)
		case "space":
			if p.many {
				p.choices[p.cur].On = !p.choices[p.cur].On
			}
		case "a":
			if p.many {
				allOn := !slices.ContainsFunc(p.choices, func(c Choice) bool { return !c.On })
				for i := range p.choices {
					p.choices[i].On = !allOn
				}
			}
		case "enter":
			if len(p.chosen()) == 0 {
				p.warn = "select one with space"
				return p, nil
			}
			p.done = true
			return p, tea.Quit
		case "esc", "q", "ctrl+c":
			p.cancelled = true
			return p, tea.Quit
		}
	}
	return p, nil
}

// chosen are the selected lines, or in a single choice the line of the cursor.
func (p *picker) chosen() []int {
	if !p.many {
		return []int{p.cur}
	}
	var on []int
	for i, c := range p.choices {
		if c.On {
			on = append(on, i)
		}
	}
	return on
}

func (p *picker) View() tea.View { return tea.NewView(p.render()) }

func (p *picker) render() string {
	l := p.look
	if p.cancelled {
		return l.Grey.Render(l.Fail+" "+p.question+" cancelled") + "\n"
	}
	if p.done {
		var labels []string
		for _, i := range p.chosen() {
			labels = append(labels, p.choices[i].Label)
		}
		return l.Green.Render(l.OK) + " " + p.question + " " + l.Acc.Render(strings.Join(labels, ", ")) + "\n"
	}
	labelW, badgeW := 0, 0
	for _, c := range p.choices {
		labelW = max(labelW, ansi.StringWidth(c.Label))
		badgeW = max(badgeW, ansi.StringWidth(c.Badge))
	}
	var b strings.Builder
	b.WriteString(l.Bold.Render(p.question) + "\n")
	for i, c := range p.choices {
		pointer, text := "  ", l.Text
		if i == p.cur {
			pointer, text = l.Acc.Bold(true).Render(l.Cursor)+" ", l.Bold
		}
		mark := ""
		if p.many {
			mark = l.Faint.Render(l.Off) + " "
			if c.On {
				mark = l.Acc.Render(l.On) + " "
			}
		}
		line := pointer + mark + text.Render(fit(c.Label, labelW))
		if badgeW > 0 {
			line += "  " + l.Green.Render(fit(c.Badge, badgeW))
		}
		if c.Note != "" {
			line += "  " + l.Grey.Render(c.Note)
		}
		b.WriteString(line + "\n")
	}
	keys := l.UpDown + " move  " + l.Enter + " choose  esc cancel"
	if p.many {
		keys = l.UpDown + " move  space select  a all  " + l.Enter + " install  esc cancel"
	}
	b.WriteString(l.Faint.Render(keys))
	if p.warn != "" {
		b.WriteString("  " + l.Yellow.Render(p.warn))
	}
	return b.String() + "\n"
}
