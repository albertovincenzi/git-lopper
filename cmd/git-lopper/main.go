// Command git-lopper finds stale git worktrees in many repositories and removes them safely.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/albertovincenzi/git-lopper/v2/internal/lopper"
	"github.com/albertovincenzi/git-lopper/v2/internal/ui"
)

// version is set at release time with -ldflags "-X main.version=...".
var version = "2.2.0"

func main() { os.Exit(run(os.Args[1:])) }

type options struct {
	root, command        string
	args                 []string
	depth                int // 0: from the environment
	fetch, tsv, dry, yes bool
	help, version        bool
}

// parseArgs accepts options before and after the command, like the git commands do.
func parseArgs(argv []string) (options, error) {
	o := options{root: ".", fetch: true}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch a {
		case "-C", "--root", "-d", "--depth":
			if i+1 >= len(argv) {
				return o, fmt.Errorf("%s needs a value", a)
			}
			i++
			if a == "-C" || a == "--root" {
				o.root = argv[i]
				continue
			}
			d, err := lopper.ParseDepth(argv[i])
			if err != nil {
				return o, fmt.Errorf("%s: %w", a, err)
			}
			o.depth = d
		case "--no-fetch":
			o.fetch = false
		case "--tsv":
			o.tsv = true
		case "-n", "--dry-run":
			o.dry = true
		case "-y", "--yes":
			o.yes = true
		case "-h", "--help":
			o.help = true
		case "-V", "--version":
			o.version = true
		default:
			switch {
			case strings.HasPrefix(a, "-"):
				return o, fmt.Errorf("unknown option: %s (try -h)", a)
			case o.command == "":
				o.command = a
			default:
				o.args = append(o.args, a)
			}
		}
	}
	return o, nil
}

func run(argv []string) int {
	o, err := parseArgs(argv)
	if err != nil {
		return fail(err)
	}
	if o.help || o.command == "help" {
		fmt.Print(ui.Usage(version))
		return 0
	}
	if o.version {
		fmt.Printf("git-lopper %s\n", version)
		return 0
	}
	if o.command == "install-skill" {
		return installSkill(o.args, isTerminal(os.Stdin) && isTerminal(os.Stdout))
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fail(errors.New("git is not installed"))
	}
	classes, err := parseClasses(o.command, o.args)
	if err != nil {
		return fail(err)
	}
	cfg, err := lopper.ConfigFromEnv()
	if err != nil {
		return fail(err)
	}
	if o.depth > 0 {
		cfg.Depth = o.depth
	}
	root, err := filepath.Abs(o.root)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if fi, statErr := os.Stat(root); err != nil || statErr != nil || !fi.IsDir() {
		return fail(fmt.Errorf("not a folder: %s", o.root))
	}
	a := newApp(cfg, root)
	switch o.command {
	case "":
		return a.tui()
	case "scan":
		return a.scan(o.fetch)
	case "list":
		return a.list(o.tsv, classes)
	case "clean":
		return a.clean(o.dry, o.yes, classes)
	}
	return fail(fmt.Errorf("unknown command: %s (try -h)", o.command))
}

// parseClasses checks the class arguments of list and clean.
func parseClasses(command string, args []string) ([]lopper.Class, error) {
	if command != "list" && command != "clean" {
		if len(args) > 0 {
			return nil, fmt.Errorf("%s takes no arguments: %s", command, strings.Join(args, " "))
		}
		return nil, nil
	}
	var classes []lopper.Class
	for _, s := range args {
		c, ok := lopper.ParseClass(s)
		if !ok {
			return nil, fmt.Errorf("unknown class: %s (one of: %s)", s, join(lopper.Classes))
		}
		classes = append(classes, c)
	}
	if command == "list" && len(classes) == 0 {
		return lopper.Classes, nil
	}
	if command == "clean" && len(classes) == 0 {
		return nil, fmt.Errorf("clean needs one or more of: %s", join(lopper.Cleanable))
	}
	for _, c := range classes {
		if command == "clean" && !slices.Contains(lopper.Cleanable, c) {
			return nil, fmt.Errorf("%s is never cleaned (only %s)", c, join(lopper.Cleanable))
		}
	}
	return classes, nil
}

func join(classes []lopper.Class) string {
	s := make([]string, len(classes))
	for i, c := range classes {
		s[i] = string(c)
	}
	return strings.Join(s, " ")
}

// fail reports a usage or setup error; the exit code is 2.
func fail(err error) int {
	fmt.Fprintf(os.Stderr, "git-lopper: %s\n", err)
	return 2
}

func isTerminal(f *os.File) bool { return term.IsTerminal(f.Fd()) }
