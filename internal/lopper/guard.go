package lopper

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// Guards say which worktrees must not be removed right now: active Bloom
// workspaces, and folders that a running process uses.
type Guards struct {
	bloomDirs       []string // the Bloom workspace folder, as given and without symlinks
	bloomActive     []string // the workspaces that Bloom has not archived
	bloomUnreadable bool     // Bloom's database exists, but the query failed
	cwds            []string // the working directories of all processes
	seesProcesses   bool     // cwds is complete enough to trust
}

// SeesProcesses reports whether the in-use check worked when the guards were loaded.
func (g Guards) SeesProcesses() bool { return g.seesProcesses }

// LoadGuards reads Bloom's database (when bloomDB exists) and the working
// directories of all processes. All paths are in NFC.
//
// shortcut: reads Bloom's private sqlite schema through the sqlite3 command; if
// the query fails, the whole Bloom folder counts as active.
func LoadGuards(bloomDB, bloomDir string) Guards {
	g := Guards{bloomDirs: withRealPaths([]string{bloomDir})}
	if fi, err := os.Stat(bloomDB); err == nil && fi.Mode().IsRegular() {
		// any state but 'archived' counts as active, so a new Bloom state fails safe
		cmd, cancel := command(10*time.Second, "sqlite3", "-readonly", bloomDB,
			"select path from workspaces where state <> 'archived'")
		out, err := cmd.Output()
		cancel()
		if err != nil {
			g.bloomUnreadable = true
		} else {
			g.bloomActive = withRealPaths(lines(string(out)))
		}
	}
	cwds, ok := processCwds()
	g.cwds = withRealPaths(cwds)
	g.seesProcesses = ok && listsSelf(g.cwds)
	return g
}

// listsSelf reports whether cwds holds the working directory of this process: if
// it does not, lsof or /proc failed in silence, and the in-use check is blind.
func listsSelf(cwds []string) bool {
	wd, err := os.Getwd()
	if err != nil {
		return false
	}
	for _, p := range withRealPaths([]string{wd}) {
		if slices.Contains(cwds, p) {
			return true
		}
	}
	return false
}

// CanSeeProcesses reports whether this system has lsof or /proc for the in-use
// check. Guards.SeesProcesses tells whether the check really worked.
func CanSeeProcesses() bool {
	_, err := exec.LookPath("lsof")
	return err == nil || exists("/proc/self/cwd")
}

// processCwds lists the working directory of every process it can see. ok is
// false when there is no lsof and no /proc, or when lsof did not finish in time.
func processCwds() (cwds []string, ok bool) {
	if _, err := exec.LookPath("lsof"); err == nil {
		cmd, cancel := command(30*time.Second, "lsof", "-d", "cwd", "-Fn")
		defer cancel()
		// lsof exits with 1 when it cannot read some processes; its output is still right
		out, _ := cmd.Output()
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			return nil, false // did not start, or killed by the time limit
		}
		for _, l := range lines(string(out)) {
			if strings.HasPrefix(l, "n") {
				cwds = append(cwds, l[1:])
			}
		}
		return cwds, true
	}
	links, _ := filepath.Glob("/proc/[0-9]*/cwd")
	for _, l := range links {
		if p, err := os.Readlink(l); err == nil {
			cwds = append(cwds, p)
		}
	}
	return cwds, len(links) > 0
}

// Protected says why the worktree at path must stay, or "" when nothing protects it.
func (g Guards) Protected(path string) string {
	for _, p := range withRealPaths([]string{path}) {
		switch {
		case g.bloomUnreadable && overlaps(g.bloomDirs, p, true):
			return "Bloom workspace (Bloom db unreadable)"
		case overlaps(g.bloomActive, p, true):
			return "active Bloom workspace"
		case overlaps(g.cwds, p, false):
			return "in use by a running process"
		}
	}
	return ""
}

// overlaps reports whether an entry of list is p or inside p. With either, it
// also reports whether p is inside an entry.
func overlaps(list []string, p string, either bool) bool {
	for _, e := range list {
		if e != "" && (under(e, p) || either && under(p, e)) {
			return true
		}
	}
	return false
}

func under(a, b string) bool { return a == b || strings.HasPrefix(a, strings.TrimSuffix(b, "/")+"/") }

// withRealPaths gives each path in NFC, plus its form without symlinks
// (/var -> /private/var). Bloom and HFS+ store paths in NFD, git prints NFC.
func withRealPaths(paths []string) []string {
	var list []string
	for _, p := range paths {
		list = append(list, norm.NFC.String(p))
		if r := realPath(p); r != "" && r != p {
			list = append(list, norm.NFC.String(r))
		}
	}
	return list
}

// realPath resolves the symlinks in path. For a missing path, it resolves the parent.
func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	if d, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		return filepath.Join(d, filepath.Base(p))
	}
	return ""
}

// nested says why removing path would also remove something else: a repository
// or worktree inside it (to 6 levels down), or a folder that cannot be read, so
// it cannot be checked. Dependency folders below the top level are not searched.
func nested(path string) string {
	why := ""
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			why = "has a folder it cannot read"
			return filepath.SkipAll
		}
		rel, _ := filepath.Rel(path, p)
		if rel == "." {
			return nil
		}
		level := strings.Count(rel, string(filepath.Separator)) + 1
		switch {
		case d.Name() == ".git" && level >= 2: // the worktree's own .git is level 1
			why = "holds another repository or worktree"
			return filepath.SkipAll
		case d.IsDir() && (level >= 6 || level >= 2 && isDeps(d.Name())):
			return filepath.SkipDir
		}
		return nil
	})
	return why
}

func isDeps(name string) bool { return name == "node_modules" || name == "vendor" || name == ".venv" }
