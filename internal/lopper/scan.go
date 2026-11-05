package lopper

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Row is one linked worktree in a scan.
type Row struct {
	Class      Class  `json:"class"`
	Reason     string `json:"reason"`
	Repo       string `json:"repo"`
	Path       string `json:"path"`
	Branch     string `json:"branch"`
	Base       string `json:"base"`
	PR         string `json:"pr"`
	PRHead     string `json:"pr_head"`
	Upstream   string `json:"upstream"`
	LastCommit string `json:"last_commit"`
}

// Progress receives "done of total" and a label of the current item.
type Progress func(done, total int, label string)

// GhWarning is the scan warning when gh cannot give the PR state.
const GhWarning = "gh is missing or not logged in: PR state is unknown, so fewer worktrees are LIKELY"

// parallel is how many git commands run at the same time.
const parallel = 8

// Scan finds the repositories under root (to depth folders down, plus the one
// around root) and classifies all their linked worktrees. With fetch, it runs
// `git fetch --prune` in each repository first. warn is set when gh does not work.
func Scan(root string, depth int, fetch bool, report Progress) (rows []Row, warn string) {
	noPR := PRNone
	if !GhReady() {
		noPR, warn = PRUnknown, GhWarning
	}
	repos := FindRepos(root, depth)
	infos := make([]*repoInfo, len(repos))
	c := &counter{total: len(repos), report: report}
	forEach(len(repos), func(i int) {
		infos[i] = readRepo(repos[i], fetch, noPR)
		c.step(filepath.Base(repos[i]))
	})

	var targets []Target
	for _, info := range infos {
		if info != nil {
			targets = append(targets, info.targets...)
		}
	}
	rows = make([]Row, len(targets))
	c = &counter{total: len(targets), report: report}
	forEach(len(targets), func(i int) {
		t := targets[i]
		class, reason, upstream := Inspect(t)
		last, err := git(t.Path, "log", "-1", "--format=%cd", "--date=short")
		if err != nil || last == "" {
			last = "-"
		}
		rows[i] = Row{class, reason, t.Repo, t.Path, t.Branch, t.Base, t.PR, t.PRHead, upstream, last}
		c.step(filepath.Base(t.Repo) + " · " + t.Branch)
	})
	return rows, warn
}

// GhReady reports whether gh is installed and logged in.
func GhReady() bool {
	if _, err := exec.LookPath("gh"); err != nil {
		return false
	}
	cmd, cancel := command(30*time.Second, "gh", "auth", "status")
	defer cancel()
	return cmd.Run() == nil
}

// FindRepos lists the repositories under root, to depth folders down, plus the
// repository around root. It does not follow symbolic links.
func FindRepos(root string, depth int) []string {
	var repos []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil // an unreadable folder is not searched
		}
		if d.Name() == ".git" && p != root {
			repos = append(repos, filepath.Dir(p))
			return fs.SkipDir
		}
		if rel, _ := filepath.Rel(root, p); rel != "." && strings.Count(rel, string(filepath.Separator))+1 >= depth {
			return fs.SkipDir
		}
		return nil
	})
	if common, err := git(root, "rev-parse", "--path-format=absolute", "--git-common-dir"); err == nil {
		repos = append(repos, strings.TrimSuffix(common, "/.git"))
	}
	slices.Sort(repos)
	return slices.Compact(repos)
}

type repoInfo struct{ targets []Target }

type pullRequest struct{ state, head string }

// readRepo collects the linked worktrees of repo, with its default branch and
// PR states. It returns nil for a repository without linked worktrees.
func readRepo(repo string, fetch bool, noPR string) *repoInfo {
	out, err := git(repo, "worktree", "list", "--porcelain")
	entries := parsePorcelain(out)
	if err != nil || len(entries) < 2 {
		return nil
	}
	remote := pickRemote(repo)
	if fetch && remote != "" {
		fetchRemote(repo, remote)
	}
	base := defaultBase(repo, remote)
	prs, noPR := pullRequests(repo, noPR)
	info := &repoInfo{}
	for _, e := range entries[1:] { // the first entry is the main worktree
		t := Target{Repo: repo, Path: e.path, Branch: e.branch, Base: base, PR: noPR, PRHead: "-", Locked: e.locked, Head: e.head}
		if pr, ok := prs[e.branch]; ok {
			t.PR, t.PRHead = pr.state, pr.head
		}
		info.targets = append(info.targets, t)
	}
	return info
}

type entry struct {
	path, branch, head string
	locked             bool
}

// parsePorcelain reads the output of `git worktree list --porcelain`.
//
// shortcut: a path with a newline breaks this line format, and then git worktree
// remove refuses the cut path; use --porcelain -z when git 2.36 is the minimum.
func parsePorcelain(out string) []entry {
	var list []entry
	e := entry{head: "-"}
	for _, line := range strings.Split(out+"\n", "\n") {
		switch {
		case line == "":
			if e.path != "" {
				list = append(list, e)
			}
			e = entry{head: "-"}
		case strings.HasPrefix(line, "worktree "):
			e.path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			e.head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			e.branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		case line == "detached":
			e.branch = detached
		case line == "locked" || strings.HasPrefix(line, "locked "):
			e.locked = true
		}
	}
	return list
}

// fetchRemote runs `git fetch --prune`. It runs in a new session without a
// terminal, so ssh cannot ask for a passphrase or a host key over the app.
func fetchRemote(repo, remote string) {
	cmd, cancel := command(2*time.Minute, "git", "-C", repo, "fetch", "--prune", "--quiet", remote)
	defer cancel()
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	_ = cmd.Run() // offline, refused or too slow: scan what is there
}

// pickRemote returns origin, else the first remote, else "".
func pickRemote(repo string) string {
	out, _ := git(repo, "remote")
	remotes := lines(out)
	if slices.Contains(remotes, "origin") {
		return "origin"
	}
	if len(remotes) > 0 {
		return remotes[0]
	}
	return ""
}

// defaultBase returns the remote's HEAD, else the first of main, master,
// develop or trunk on the remote, else "-".
func defaultBase(repo, remote string) string {
	if remote == "" {
		return "-"
	}
	if head, err := git(repo, "symbolic-ref", "--short", "refs/remotes/"+remote+"/HEAD"); err == nil && head != "" {
		return head
	}
	for _, b := range []string{"main", "master", "develop", "trunk"} {
		if gitOK(repo, "rev-parse", "-q", "--verify", "refs/remotes/"+remote+"/"+b) {
			return remote + "/" + b
		}
	}
	return "-"
}

// pullRequests asks gh for the PRs of repo, by branch. When gh fails, the PR
// state of all its worktrees is unknown.
func pullRequests(repo, noPR string) (map[string]pullRequest, string) {
	if noPR != PRNone {
		return nil, noPR
	}
	cmd, cancel := command(time.Minute, "gh", "pr", "list", "--state", "all", "--limit", "2000",
		"--json", "headRefName,state,headRefOid,isCrossRepository",
		// PRs from forks are skipped: their branch names say nothing about ours
		"-q", `.[] | select(.isCrossRepository | not) | "\(.headRefName)\t\(.state)\t\(.headRefOid)"`)
	defer cancel()
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return nil, PRUnknown
	}
	prs := map[string]pullRequest{}
	for _, l := range lines(string(out)) {
		f := strings.Split(l, "\t")
		if _, seen := prs[f[0]]; len(f) == 3 && !seen { // gh lists the newest PR first
			prs[f[0]] = pullRequest{f[1], f[2]}
		}
	}
	return prs, PRNone
}

// forEach runs fn for 0..n-1, at most `parallel` at the same time.
func forEach(n int, fn func(i int)) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, parallel)
	for i := range n {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer func() { <-slots; wg.Done() }()
			fn(i)
		}()
	}
	wg.Wait()
}

// counter reports progress from many goroutines, one report at a time.
type counter struct {
	mu          sync.Mutex
	done, total int
	report      Progress
}

func (c *counter) step(label string) {
	if c.report == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.done++
	c.report(c.done, c.total, label)
}
