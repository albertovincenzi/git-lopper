package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/albertovincenzi/git-lopper/v2/internal/lopper"
)

// The Agent Skills format: YAML frontmatter with a name equal to the folder and a
// description of at most 1024 characters, then the instructions.
func TestSkillFile(t *testing.T) {
	text := string(skill)
	head, body, ok := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
	if !strings.HasPrefix(text, "---\n") || !ok {
		t.Fatal("SKILL.md needs a frontmatter between two --- lines")
	}
	fields := map[string]string{}
	for line := range strings.SplitSeq(head, "\n") {
		k, v, _ := strings.Cut(line, ": ")
		fields[k] = v
	}
	if fields["name"] != skillName {
		t.Errorf("name: %q, want %q (the folder install-skill writes)", fields["name"], skillName)
	}
	d := fields["description"]
	if d == "" || len(d) > 1024 || strings.Contains(d, ": ") {
		t.Errorf("description: 1 to 1024 characters, no ': ' (plain YAML); got %d characters", len(d))
	}
	if cols := strings.ReplaceAll(lopper.TSVHeader, "\t", " "); !strings.Contains(body, cols) {
		t.Errorf("the skill must list the TSV columns: %s", cols)
	}
}

// Agents that read the same folder get one copy of the skill.
func TestSkillTargets(t *testing.T) {
	all := []agent{
		{title: "Claude Code", user: "/h/.claude/skills", project: ".claude/skills"},
		{title: "Codex", user: "/h/.agents/skills", project: ".agents/skills"},
		{title: "Cursor", user: "/h/.cursor/skills", project: ".agents/skills"},
	}
	got := targets(all, "/repo")
	want := []target{
		{dir: "/repo/.claude/skills/git-lopper", titles: []string{"Claude Code"}},
		{dir: "/repo/.agents/skills/git-lopper", titles: []string{"Codex", "Cursor"}},
	}
	same := func(a, b target) bool { return a.dir == b.dir && slices.Equal(a.titles, b.titles) }
	if !slices.EqualFunc(got, want, same) {
		t.Errorf("in a repository: %v, want %v", got, want)
	}
	if got := targets(all, ""); len(got) != 3 || got[2].dir != "/h/.cursor/skills/git-lopper" {
		t.Errorf("for the user: %v", got)
	}
}
