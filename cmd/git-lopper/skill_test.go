package main

import (
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
