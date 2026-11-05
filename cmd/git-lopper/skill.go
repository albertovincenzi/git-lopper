package main

import (
	"cmp"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/albertovincenzi/git-lopper/v2/internal/ui"
)

// skill teaches a coding agent to use the commands of git-lopper.
//
//go:embed SKILL.md
var skill []byte

const skillName = "git-lopper"

// agent is a coding agent that reads Agent Skills from a folder.
type agent struct {
	name, title string
	home        string // the agent is on this machine when this folder exists
	user        string // its folder of skills for the user
	project     string // its folder of skills in a repository, relative to the root
}

func (a agent) found() bool {
	fi, err := os.Stat(a.home)
	return err == nil && fi.IsDir()
}

// agents are the coding agents that read Agent Skills, with their folders as the
// skills CLI of Vercel lists them (github.com/vercel-labs/skills, src/agents.ts).
func agents() ([]agent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	at := func(p ...string) string { return filepath.Join(append([]string{home}, p...)...) }
	config := cmp.Or(os.Getenv("XDG_CONFIG_HOME"), at(".config"))
	claude := cmp.Or(os.Getenv("CLAUDE_CONFIG_DIR"), at(".claude"))
	const shared = ".agents/skills" // the folder in a repository that most agents read
	return []agent{
		{"claude", "Claude Code", claude, filepath.Join(claude, "skills"), ".claude/skills"},
		// Codex reads user skills from ~/.agents/skills; $CODEX_HOME/skills is deprecated.
		{"codex", "Codex", cmp.Or(os.Getenv("CODEX_HOME"), at(".codex")), at(".agents", "skills"), shared},
		{"cursor", "Cursor", at(".cursor"), at(".cursor", "skills"), shared},
		{"gemini", "Gemini CLI", at(".gemini"), at(".gemini", "skills"), shared},
		{"copilot", "GitHub Copilot", at(".copilot"), at(".copilot", "skills"), shared},
		{"opencode", "OpenCode", filepath.Join(config, "opencode"), filepath.Join(config, "opencode", "skills"), shared},
		{"amp", "Amp", filepath.Join(config, "amp"), filepath.Join(config, "agents", "skills"), shared},
		{"cline", "Cline", at(".cline"), at(".agents", "skills"), shared},
		{"windsurf", "Windsurf", at(".codeium", "windsurf"), at(".codeium", "windsurf", "skills"), ".windsurf/skills"},
		{"goose", "Goose", filepath.Join(config, "goose"), filepath.Join(config, "goose", "skills"), ".goose/skills"},
		{"kiro", "Kiro CLI", at(".kiro"), at(".kiro", "skills"), ".kiro/skills"},
		{"roo", "Roo Code", at(".roo"), at(".roo", "skills"), ".roo/skills"},
		{"junie", "Junie", at(".junie"), at(".junie", "skills"), ".junie/skills"},
	}, nil
}

// target is a folder for the skill, and the agents that read it.
type target struct {
	dir    string
	titles []string
}

// targets are the folders of the agents for the user or, with a project, in that
// repository. Agents that read the same folder share one target.
func targets(picked []agent, project string) []target {
	var ts []target
	for _, a := range picked {
		dir := a.user
		if project != "" {
			dir = filepath.Join(project, a.project)
		}
		dir = filepath.Join(dir, skillName)
		if i := slices.IndexFunc(ts, func(t target) bool { return t.dir == dir }); i >= 0 {
			ts[i].titles = append(ts[i].titles, a.title)
			continue
		}
		ts = append(ts, target{dir: dir, titles: []string{a.title}})
	}
	return ts
}

// installSkill writes the skill for the agents named in args. Without names, it asks
// where and for which agents in a terminal; outside a terminal, it installs for the
// user of every agent on this machine. It replaces an older copy of the skill.
func installSkill(args []string, interactive bool) int {
	all, err := agents()
	if err != nil {
		return fail(err)
	}
	var picked []agent
	for _, name := range args {
		i := slices.IndexFunc(all, func(a agent) bool { return a.name == name })
		if i < 0 {
			names := make([]string, len(all))
			for j, a := range all {
				names[j] = a.name
			}
			return fail(fmt.Errorf("unknown agent: %s (one of: %s)", name, strings.Join(names, " ")))
		}
		picked = append(picked, all[i])
	}
	project := ""
	switch {
	case len(args) > 0:
	case interactive:
		if picked, project, err = askSkill(all); err != nil {
			return fail(err)
		}
		if picked == nil {
			fmt.Println("nothing installed.")
			return 0
		}
	default:
		if picked = slices.DeleteFunc(all, func(a agent) bool { return !a.found() }); len(picked) == 0 {
			return fail(errors.New("found no coding agent on this machine: name one, as in 'git lopper install-skill claude'"))
		}
	}
	for _, t := range targets(picked, project) {
		path := filepath.Join(t.dir, "SKILL.md")
		if err := os.MkdirAll(t.dir, 0o750); err != nil {
			return fail(err)
		}
		if err := os.WriteFile(path, skill, 0o600); err != nil {
			return fail(err)
		}
		fmt.Printf("installed the skill for %s: %s\n", strings.Join(t.titles, ", "), ui.Tilde(path))
	}
	return 0
}

// askSkill asks where the skill goes, then which agents use it. The agents on this
// machine come first and are selected. It returns no agents when the user cancels.
func askSkill(all []agent) (picked []agent, project string, err error) {
	repo, what, err := currentProject()
	if err != nil {
		return nil, "", err
	}
	where, err := ui.Pick("Where should the skill go?", []ui.Choice{
		{Label: "your user", Note: "every project on this computer"},
		{Label: what, Note: ui.Tilde(repo) + " · commit it to share it with your team"},
	}, false)
	if err != nil || where == nil {
		return nil, "", err
	}
	if where[0] == 1 {
		project = repo
	}
	sorted := append(slices.DeleteFunc(slices.Clone(all), func(a agent) bool { return !a.found() }),
		slices.DeleteFunc(slices.Clone(all), agent.found)...)
	choices := make([]ui.Choice, len(sorted))
	for i, a := range sorted {
		c := ui.Choice{Label: a.title, Note: ui.Tilde(a.user), On: a.found()}
		if project != "" {
			c.Note = a.project
		}
		if c.On {
			c.Badge = "found"
		}
		choices[i] = c
	}
	chosen, err := ui.Pick("Which agents should use it?", choices, true)
	if err != nil || chosen == nil {
		return nil, "", err
	}
	for _, i := range chosen {
		picked = append(picked, sorted[i])
	}
	return picked, project, nil
}

// currentProject is the repository around the current folder, or the folder itself
// outside a repository.
func currentProject() (path, what string, err error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if top := strings.TrimSpace(string(out)); err == nil && top != "" {
		return top, "this repository", nil
	}
	dir, err := os.Getwd()
	return dir, "this folder", err
}
