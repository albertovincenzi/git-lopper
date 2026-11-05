package ui

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/albertovincenzi/git-lopper/v2/internal/lopper"
)

const allRepos = "all repositories"

// rows is the scan as the app shows it: every worktree, or only the ones of
// the repository in view. The cards, the lists and the clean all use it.
func (m *Model) rows() []lopper.Row { return m.rowsOf(m.repo) }

func (m *Model) rowsOf(repo string) []lopper.Row {
	if repo == "" {
		return m.saved.Rows
	}
	return slices.DeleteFunc(slices.Clone(m.saved.Rows), func(r lopper.Row) bool { return r.Repo != repo })
}

// repoName is a repository path in short: relative to the folder of the scan
// when it is inside it.
func (m *Model) repoName(repo string) string {
	rel, err := filepath.Rel(m.root, repo)
	switch {
	case err != nil || rel == ".." || strings.HasPrefix(rel, "../"):
		return Tilde(repo)
	case rel == ".":
		return filepath.Base(repo)
	}
	return rel
}

// repoList is "" (all repositories), then the repositories of the scan by name.
func (m *Model) repoList() []string {
	list := []string{""}
	for _, r := range m.saved.Rows {
		if !slices.Contains(list, r.Repo) {
			list = append(list, r.Repo)
		}
	}
	slices.SortFunc(list[1:], func(a, b string) int { return cmp.Compare(m.repoName(a), m.repoName(b)) })
	return list
}

// openRepos shows the dialog to choose the repository in view.
func (m *Model) openRepos() {
	if !m.hasScan {
		m.flash = "no scan yet: press s"
		return
	}
	m.back, m.screen = m.screen, repoScreen
	m.repoCur = max(slices.Index(m.repoList(), m.repo), 0)
}

func (m *Model) repoKey(key string) {
	list := m.repoList()
	switch key {
	case "up", "k":
		m.repoCur = max(m.repoCur-1, 0)
	case "down", "j":
		m.repoCur = min(m.repoCur+1, len(list)-1)
	case "home", "g":
		m.repoCur = 0
	case "end", "G":
		m.repoCur = len(list) - 1
	case "enter", "space":
		m.repo, m.curPath, m.screen = list[min(m.repoCur, len(list)-1)], "", m.back
	case "esc", "q", "r", "left", "h":
		m.screen = m.back
	}
}

// repoLines are the column names, then one line per repository: its worktrees,
// how many of them are ready to remove, and its size on disk. at is the line of
// the cursor.
func (m *Model) repoLines(l Look) (lines []string, at int) {
	list := m.repoList()
	nameW := len(allRepos)
	for _, repo := range list[1:] {
		nameW = max(nameW, ansi.StringWidth(m.repoName(repo)))
	}
	nameW = min(nameW, 40)
	lines = append(lines, l.Faint.Render("    "+fit("REPOSITORY", nameW)+" "+alignRight("WORKTREES", 9)+" "+
		alignRight("READY", 6)+" "+alignRight("SIZE", 8)))
	for i, repo := range list {
		rows, name := m.rowsOf(repo), allRepos
		if repo != "" {
			name = m.repoName(repo)
		}
		pointer, mark, text := "  ", "  ", l.Text
		if i == m.repoCur {
			pointer, at, text = l.Acc.Bold(true).Render(l.Cursor)+" ", len(lines), l.Bold
		}
		if repo == m.repo {
			mark = l.Acc.Render(l.Dot) + " "
		}
		ready := len(lopper.Of(rows, lopper.Cleanable))
		readySt := l.Grey
		if ready > 0 {
			readySt = l.Green
		}
		lines = append(lines, pointer+mark+text.Render(fit(name, nameW))+" "+l.Grey.Render(alignRight(fmt.Sprint(len(rows)), 9))+" "+
			readySt.Render(alignRight(fmt.Sprint(ready), 6))+" "+l.Grey.Render(alignRight(m.sizeText(rows), 8)))
	}
	return lines, at
}
