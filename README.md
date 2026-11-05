# git-lopper

[![CI](https://github.com/albertovincenzi/git-lopper/actions/workflows/ci.yml/badge.svg)](https://github.com/albertovincenzi/git-lopper/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Find the stale git worktrees in all your repositories and remove them safely.**

Coding agents, review tools and `git worktree add` leave worktrees behind. After a few
weeks you have hundreds of them, spread over many repositories, and you cannot tell
which ones still hold work. `git lopper` scans a folder of repositories, sorts every
linked worktree into one of five classes, and removes only the ones that are safe to
remove — after it checks each one again.

```
  ✂  GIT LOPPER  ~/code   ✔ gh ✔ in-use
  ──────────────────────────────────────────────────────────────────
  11 worktrees in 2 repos · scanned Thu 05 Nov 07:23

  ● PRUNE       2  ██████████████░░░░░░░░  folder already gone, only git metadata left
  ● SAFE        2  ██████████████░░░░░░░░  clean, HEAD already in the default branch
  ● LIKELY      2  ██████████████░░░░░░░░  clean, PR merged/closed or remote branch deleted
  ● KEEP        3  ██████████████████████  changes, unpushed commits or locked
  ● REVIEW      2  ██████████████░░░░░░░░  open PR, no PR or never pushed

   s  scan    f  fast scan    l  list    c  clean    ?  help    q  quit
```

- **One scan for many repositories.** Point it at `~/code` and it finds every repository
  with linked worktrees, including the one around the current folder.
- **Knows about squash merges.** It asks GitHub (through `gh`) for the PR state, so a
  branch that was squash-merged is not reported as unmerged work.
- **Safe by design.** No `--force`, no branch deletion, a plan before every removal and a
  fresh check of each worktree right before it goes. See [Safety](#safety).
- **Interactive or scriptable.** A colour terminal app, plus `scan`, `list` and `clean`
  commands with `--tsv`, `--dry-run` and `--yes`.
- **One file, no dependencies.** Plain bash 3.2+ (the macOS default) and git.

## Install

`git-lopper` is a single script. Put it anywhere on your `PATH`; git then runs it as
`git lopper`.

```sh
curl -fsSL https://raw.githubusercontent.com/albertovincenzi/git-lopper/main/bin/git-lopper \
  -o ~/.local/bin/git-lopper && chmod +x ~/.local/bin/git-lopper
```

Or from a clone:

```sh
git clone https://github.com/albertovincenzi/git-lopper && cd git-lopper
make install                # into /usr/local/bin; use PREFIX=~/.local for your home
```

## Usage

```sh
cd ~/code && git lopper       # open the interactive app
```

| Key | Action |
| --- | --- |
| `s` | scan: `git fetch --prune` in every repository, then read the PR state |
| `f` | fast scan: no fetch |
| `l` | list the worktrees of one class |
| `c` | clean one class: shows the plan, removes only after `y` |
| `?` | help |
| `q` | quit |

The same work from a script or a cron job:

```sh
git lopper -C ~/code scan                       # scan and print the summary
git lopper -C ~/code list KEEP                  # what still holds work
git lopper -C ~/code list --tsv > worktrees.tsv # raw rows for your own tools
git lopper -C ~/code clean SAFE PRUNE --dry-run # show the plan, change nothing
git lopper -C ~/code clean SAFE PRUNE --yes     # remove without a prompt
```

`clean` exits with `0` when every planned removal succeeded, `1` when git refused one,
and `2` on a usage error. Use `git lopper -h` for help: `git lopper --help` makes git
look for a man page.

## Classes

| Class | Meaning | Cleaned by |
| --- | --- | --- |
| `PRUNE` | The folder is gone; only git's metadata is left. | `git worktree remove` on that entry only |
| `SAFE` | No changes, and `HEAD` is already in the default branch. | `git worktree remove` |
| `LIKELY` | No changes, no unpushed commits, `HEAD` is on a remote branch or in the PR, and the PR is merged or closed, or the remote branch was deleted (typical after a squash merge). | `git worktree remove` |
| `KEEP` | Uncommitted changes, unpushed commits, a rebase, merge or bisect in progress, or a locked worktree. A missing folder is `KEEP` too when the entry is locked, or when its detached `HEAD` is the only reference to its commits. | never |
| `REVIEW` | An open PR, no PR, never pushed, a detached `HEAD`, or a merged PR / deleted remote branch with commits that exist only in this worktree. | never |

The default branch is `origin/HEAD` (or the first of `main`, `master`, `develop`, `trunk`
that exists on the remote). PRs from forks are ignored. Without `gh`, when `gh` is not
logged in, or when `gh pr list` fails, the PR state is unknown, so fewer worktrees reach
`LIKELY`.

A folder on a disk that is not mounted looks like a missing folder, so its worktree is
`PRUNE`. Lock such worktrees with `git worktree lock`: `git lopper` never touches a
locked one.

## Safety

`git lopper` removes worktree folders, so it is careful about it:

- **It never uses `--force`.** git itself refuses a worktree with changes, untracked
  files or a lock, and `git lopper` reports the refusal.
- **It never deletes branches.** Commits stay reachable from their branch after the
  worktree is removed.
- **It checks every worktree again right before it removes it.** If the worktree changed
  after the scan (a new file, a new commit), it is skipped.
- **It skips worktrees that a running process uses**: a process whose working
  directory is in the worktree (a shell, an agent, a dev server), found with `lsof` or
  `/proc`. Editors are not detected, because they usually do not run from that folder.
- **It skips a worktree that holds another repository or worktree** in one of its
  folders, because the removal would delete that one too.
- **It shows the plan first.** Nothing is removed before you press `y` or pass `--yes`.

What a removal does delete: files that git **ignores** in that worktree, such as `.env`,
`node_modules` or build output. Copy what you need first.

Each run writes a log to `~/.cache/git-lopper/last-clean.log`.

## Integrations

**Bloom** workspaces are git worktrees too. When Bloom's
database is present, `git lopper` skips every workspace that Bloom has not archived, and
every worktree inside one, so archive those in Bloom first. If the database cannot be
read, all of Bloom's workspace folder is skipped. This reads Bloom's internal schema and
may break with a Bloom update.

## Configuration

| Variable | Default |
| --- | --- |
| `LOPPER_DEPTH` | `3` — how deep to look for repositories (also `-d`) |
| `LOPPER_CACHE_DIR` | `${XDG_CACHE_HOME:-~/.cache}/git-lopper` — saved scans and the log |
| `LOPPER_BLOOM_DB` | `~/Library/Application Support/Bloom/bloom.sqlite` |
| `LOPPER_BLOOM_DIR` | `~/bloom/workspaces.noindex` |
| `NO_COLOR` | set it to turn colours off |

## Requirements

- bash 3.2 or later, git 2.17 or later (2.31+ to find the repository around the current folder)
- optional: `gh` for the PR state, `lsof` or `/proc` for the in-use check, `sqlite3` for
  Bloom, `less` for paging

CI runs the tests on Ubuntu and on macOS with its bash 3.2.

## Development

```sh
make test    # bats test suite on throwaway repositories (uses npx)
make lint    # ShellCheck
make demo    # open the app on a throwaway set of repositories
```

The tests build real repositories with one worktree for each case (see
[`test/fixture.bash`](test/fixture.bash)) and use a fake `gh`. Issues and pull requests
are welcome; please add a test for any change to the rules.

## License

[MIT](LICENSE)
