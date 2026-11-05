package main

import (
	"cmp"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/albertovincenzi/git-lopper/v2/internal/ui"
)

// skill teaches a coding agent to use the commands of git-lopper.
//
//go:embed SKILL.md
var skill []byte

const skillName = "git-lopper"

// agent is a coding agent that reads personal skills from a folder.
type agent struct {
	name, title string
	home        string // the agent is on this machine when this folder exists
	skills      string
}

func agents() ([]agent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	claude := cmp.Or(os.Getenv("CLAUDE_CONFIG_DIR"), filepath.Join(home, ".claude"))
	return []agent{
		{name: "claude", title: "Claude Code", home: claude, skills: filepath.Join(claude, "skills")},
		// Codex reads user skills from ~/.agents/skills; $CODEX_HOME/skills is deprecated.
		{name: "codex", title: "Codex", home: cmp.Or(os.Getenv("CODEX_HOME"), filepath.Join(home, ".codex")),
			skills: filepath.Join(home, ".agents", "skills")},
	}, nil
}

// installSkill writes the skill for the agents named in args, or for every agent
// that is on this machine. It replaces an older copy of the skill.
func installSkill(args []string) int {
	all, err := agents()
	if err != nil {
		return fail(err)
	}
	var picked []agent
	for _, name := range args {
		i := slices.IndexFunc(all, func(a agent) bool { return a.name == name })
		if i < 0 {
			return fail(fmt.Errorf("unknown agent: %s (one of: claude codex)", name))
		}
		picked = append(picked, all[i])
	}
	if len(args) == 0 {
		for _, a := range all {
			if fi, err := os.Stat(a.home); err == nil && fi.IsDir() {
				picked = append(picked, a)
			}
		}
	}
	if len(picked) == 0 {
		return fail(errors.New("found no Claude Code (~/.claude) or Codex (~/.codex): name one, as in 'git lopper install-skill claude'"))
	}
	for _, a := range picked {
		dir := filepath.Join(a.skills, skillName)
		path := filepath.Join(dir, "SKILL.md")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(path, skill, 0o600); err != nil {
			return fail(err)
		}
		fmt.Printf("installed the skill for %s: %s\n", a.title, ui.Tilde(path))
	}
	return 0
}
