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
 ✂ git lopper  ~/code · 21 worktrees in 2 repos · scanned today 14:58 ✔ gh ✔ in-use

┏━ 1  READY TO REMOVE ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 8 · 60 KB ━┓
┃ Their work is in the default branch, or in a merged or closed PR. The branches   ┃
┃ stay, and git refuses to remove a worktree with changed files.                   ┃
┃                                                                                  ┃
┃    4  in the default branch  44 KB                                               ┃
┃    2  PR merged              16 KB                                               ┃
┃    2  folder already deleted 0 KB                                                ┃
┃                                                                                  ┃
┃  ⏎  see the list    c  remove 8 after a check                                    ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
╭─ 2  NEEDS YOUR DECISION ───────────────────────────────────────────── 5 · 40 KB ─╮
│ Their commits are safe in a branch, but you may still use them. Select the ones  │
│ you do not need.                                                                 │
│                                                                                  │
│    1  PR open                wait for the merge                                  │
│    1  commits not pushed     push them, or remove it                             │
│    1  detached HEAD          remove it if not needed                             │
│    1  never pushed           push it, or remove it                               │
│    1  no PR                  open a PR, or remove it                             │
│                                                                                  │
│ ⏎ see the list and select                                                        │
╰──────────────────────────────────────────────────────────────────────────────────╯
╭─ 3  HAS YOUR WORK ─────────────────────────────────────────────────── 8 · 56 KB ─╮
│ Changes, commits or operations that exist only here. git lopper never removes    │
│ them.                                                                            │
│                                                                                  │
│    2  changed files          commit, stash or discard                            │
│    2  commits only here      put them on a branch                                │
│    2  locked                 unlock it if not needed                             │
│    1  rebase or merge        finish or abort it                                  │
│    1  unpushed commits       push the branch                                     │
│                                                                                  │
│ ⏎ see the list                                                                   │
╰──────────────────────────────────────────────────────────────────────────────────╯
 ↑↓ group  ⏎ open  c remove 8  r repo  s scan  f fast scan  ? help  q quit
```

- **One scan for many repositories.** Point it at `~/code` and it finds every repository
  with linked worktrees, including the one around the current folder.
- **Knows about squash merges.** It asks GitHub (through `gh`) for the PR state, so a
  branch that was squash-merged is not reported as unmerged work.
- **Safe by design.** No `--force`, no branch deletion, a plan before every removal and a
  fresh check of each worktree right before it goes. See [Safety](#safety).
- **Tells you what to do.** The terminal app puts the worktrees in three groups (ready to
  remove, needs your decision, has your work), with the size on disk, the age and the next
  step for each one. For scripts: `scan`, `list` and `clean` with `--tsv`, `--dry-run` and
  `--yes`.
- **One binary.** Written in Go: nothing to install but git, and it checks many
  worktrees in parallel.

## Quickstart

```sh
brew install albertovincenzi/tap/git-lopper
cd ~/code        # the folder that holds your repositories
git lopper       # open the app
```

1. Press `s` to scan. The scan reads every worktree below the folder and changes nothing.
2. Read the three groups. **Ready to remove** is selected for you. **Needs your decision**
   is not: open it and select the worktrees you do not need. **Has your work** is never
   removed; its list tells you how to save the work.
3. Press `⏎` to see a list, with the size and the age of each worktree. Press `space` to
   keep a worktree out of the clean, or to select it.
4. Press `c`. You see the plan and the space it frees. Press `y` to remove.

The branches stay. `git lopper` never uses `--force`, and it checks each worktree again
right before it removes it. From a script: `git lopper scan`, then
`git lopper clean SAFE LIKELY --dry-run`.

## Install

`git-lopper` is a single binary. Put it anywhere on your `PATH`; git then runs it as
`git lopper`. With [Homebrew](https://brew.sh), on macOS or Linux:

```sh
brew install albertovincenzi/tap/git-lopper
```

Or download the archive for your system from the
[releases](https://github.com/albertovincenzi/git-lopper/releases), and put `git-lopper`
on your `PATH`. Each release after 2.0.0 has a Sigstore signature of its checksum file.
To check a download with [cosign](https://github.com/sigstore/cosign):

```sh
v=2.0.1   # the version you downloaded
cosign verify-blob \
  --certificate-identity "https://github.com/albertovincenzi/git-lopper/.github/workflows/release.yml@refs/tags/v$v" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --bundle "git-lopper_${v}_checksums.txt.sigstore.json" "git-lopper_${v}_checksums.txt"
shasum -a 256 --check --ignore-missing "git-lopper_${v}_checksums.txt"
```

With Go 1.26 or later:

```sh
go install github.com/albertovincenzi/git-lopper/v2/cmd/git-lopper@latest
```

Or from a clone:

```sh
git clone https://github.com/albertovincenzi/git-lopper && cd git-lopper
make install                # into /usr/local/bin; use PREFIX=$HOME/.local for your home
```

## Usage

```sh
cd ~/code && git lopper       # open the interactive app
```

The home screen shows three groups, with their size on disk and what to do:

- **Ready to remove** (`PRUNE`, `SAFE`, `LIKELY`): all selected. Keep one out with `space`.
- **Needs your decision** (`REVIEW`): none selected. Select the ones you do not need with
  `space`, one by one.
- **Has your work** (`KEEP`): never removed. The list says how to save the work.

| Key | Action |
| --- | --- |
| `↑` `↓` `⏎` `1` `2` `3` | choose a group and open its list |
| `space` / `a` | select or unselect a worktree / all the ready ones |
| `/` | search in branch, repository, path and reason |
| `r` | show one repository: the groups, the lists and the clean then hold only its worktrees; `esc` on the home screen shows all of them again |
| `s` | on the home screen: scan (`git fetch --prune`, then the PR state); in a list: sort by age, size or repository |
| `f` | fast scan: no fetch |
| `c` | clean the selected worktrees: shows the plan and the space it frees, removes only after `y` |
| `?` / `q` | help / quit |

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

## AI agents

`git lopper install-skill` installs a skill that teaches coding agents to use the
commands above. With the skill, the agent scans, reports each class, shows the
`--dry-run` plan, and runs `clean --yes` only after you agree to that plan.

In a terminal, it asks where the skill goes (your user, for every project, or the
current repository, to commit and share with your team) and which agents use it. The
agents on this computer are selected for you:

```
✔ Where should the skill go? your user
Which agents should use it?
› ✔ Claude Code     found  ~/.claude/skills
  ✔ Codex           found  ~/.agents/skills
  ○ Cursor                 ~/.cursor/skills
  ○ Gemini CLI             ~/.gemini/skills
  …
↑↓ move  space select  a all  ⏎ install  esc cancel
```

From a script, name the agents. The skill then goes to their user folders:

```sh
git lopper install-skill claude codex
```

| Agent | Name | User folder | Repository folder |
| --- | --- | --- | --- |
| Claude Code | `claude` | `~/.claude/skills` (`$CLAUDE_CONFIG_DIR/skills` when set) | `.claude/skills` |
| Codex | `codex` | `~/.agents/skills` | `.agents/skills` |
| Cursor | `cursor` | `~/.cursor/skills` | `.agents/skills` |
| Gemini CLI | `gemini` | `~/.gemini/skills` | `.agents/skills` |
| GitHub Copilot | `copilot` | `~/.copilot/skills` | `.agents/skills` |
| OpenCode | `opencode` | `~/.config/opencode/skills` | `.agents/skills` |
| Amp | `amp` | `~/.config/agents/skills` | `.agents/skills` |
| Cline | `cline` | `~/.agents/skills` | `.agents/skills` |
| Windsurf | `windsurf` | `~/.codeium/windsurf/skills` | `.windsurf/skills` |
| Goose | `goose` | `~/.config/goose/skills` | `.goose/skills` |
| Kiro CLI | `kiro` | `~/.kiro/skills` | `.kiro/skills` |
| Roo Code | `roo` | `~/.roo/skills` | `.roo/skills` |
| Junie | `junie` | `~/.junie/skills` | `.junie/skills` |

Agents that read the same folder share one copy. Outside a terminal and without names,
the skill goes to the user folder of every agent on this computer. Run it again after an
upgrade. Then ask the agent, for example: "which worktrees in ~/code can I remove?"

## Classes

| Class | Meaning | Cleaned by |
| --- | --- | --- |
| `PRUNE` | The folder is gone; only git's metadata is left. | `git worktree remove` on that entry only |
| `SAFE` | No changes, and `HEAD` is already in the default branch. | `git worktree remove` |
| `LIKELY` | No changes, no unpushed commits, `HEAD` is on a remote branch or in the PR, and the PR is merged or closed, or the remote branch was deleted (typical after a squash merge). | `git worktree remove` |
| `KEEP` | Uncommitted changes, unpushed commits, a rebase, merge or bisect in progress, or a locked worktree, or a detached `HEAD` whose commits no branch or tag contains. A missing folder is `KEEP` too when the entry is locked, or when its detached `HEAD` is the only reference to its commits. | never |
| `REVIEW` | An open PR, no PR, never pushed, a detached `HEAD`, or a merged PR / deleted remote branch with commits that exist only in this worktree. | only in the app, when you select it; `clean REVIEW` is refused |

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

- macOS or Linux, git 2.17 or later (2.31+ to find the repository around the current folder)
- optional: `gh` for the PR state, `lsof` or `/proc` for the in-use check, `sqlite3` for
  Bloom

Windows is not supported yet: there, `git lopper` cannot see which worktrees are in use.

## Development

```sh
make build   # bin/git-lopper
make test    # Go unit tests (with -race), then the bats end-to-end tests (uses npx)
make lint    # go vet, gofmt, ShellCheck for the test scripts
make demo    # open the app on a throwaway set of repositories
```

The rules are in [`internal/lopper`](internal/lopper), the output and the terminal app
(built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)) in
[`internal/ui`](internal/ui). The end-to-end tests run the binary on real repositories
with one worktree for each case (see [`test/fixture.bash`](test/fixture.bash)) and use a
fake `gh`. CI runs all tests on Ubuntu and macOS. Issues and pull requests
are welcome; please add a test for any change to the rules.

## License

[MIT](LICENSE)
