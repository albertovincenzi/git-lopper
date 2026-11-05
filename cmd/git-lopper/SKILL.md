---
name: git-lopper
description: Find and remove stale git worktrees in many repositories with the git-lopper CLI. Use when the user asks which git worktrees are old, merged or safe to delete, wants to clean up the worktrees that agents or reviews left behind, or wants to free the disk space they take.
---

# git-lopper

`git lopper` scans a folder of repositories, puts every linked worktree in a class, and
removes only the worktrees that are safe to remove. Use its commands below. Do not run
`git lopper` without a command: that opens an interactive app, which needs a terminal.

## Classes

| Class | Meaning | Removed by `clean` |
| --- | --- | --- |
| `PRUNE` | The folder is gone; only git's metadata is left. | yes |
| `SAFE` | No changes, and `HEAD` is already in the default branch. | yes |
| `LIKELY` | No changes, and the PR is merged or closed, or the remote branch was deleted (a squash merge). | yes |
| `REVIEW` | An open PR, no PR, never pushed, a detached `HEAD`, or commits that exist only here. | no: the user decides |
| `KEEP` | Changed files, edits hidden from `git status` (assume-unchanged, skip-worktree), unpushed commits, a rebase or merge in progress, or a lock. | never |

## Steps

1. Scan the folder that holds the repositories. If the user names no folder, use the
   current one: the scan also finds the repository around it.

   ```sh
   git lopper -C <folder> scan              # git fetch --prune, then the PR state from gh
   git lopper -C <folder> scan --no-fetch   # faster, no network
   ```

   The scan changes no worktree. It prints the count of each class and saves the result.
   `list` and `clean` read the saved result, so scan again when it may be old.

2. Read the worktrees:

   ```sh
   git lopper -C <folder> list --tsv [CLASS...]
   ```

   The columns are: class reason repo path branch base pr pr_head upstream last_commit.
   Tell the user what each class holds and why (the `reason` column). For `KEEP`, say
   what work is in the worktree.

3. Show the plan. This changes nothing:

   ```sh
   git lopper -C <folder> clean PRUNE SAFE LIKELY --dry-run
   ```

4. Only after the user agrees to that plan, remove with the same classes:

   ```sh
   git lopper -C <folder> clean PRUNE SAFE LIKELY --yes
   ```

   Exit code 0: every removal worked. 1: git refused a removal; the output says why.
   2: a usage error. The output names the log file of the removals.

## Rules

- Never run `clean --yes` before the user agrees to the plan of `--dry-run`. A removal also
  deletes the files that git ignores in the worktree. The plan lists them under each
  worktree ("also deletes ..."): tell the user, above all about `.env` files, keys and notes.
- `clean` refuses `KEEP` and `REVIEW`. Do not work around it with
  `git worktree remove --force`, `rm -rf` or a branch deletion.
- For `REVIEW`, give the user the list with the reasons. To remove some of them, the user
  runs `git lopper` in their own terminal and selects them.
- `clean` skips a worktree that a running process uses (also the one you run in), an
  active Bloom workspace, and a worktree that holds another repository. The plan shows
  each skip and its reason.
- Without `gh`, or when `gh` is not logged in, the PR state is unknown, so fewer worktrees
  are `LIKELY`. The scan prints a warning then; tell the user.
- `-d N` sets how deep the scan looks for repositories (default 3).
- For help, run `git lopper -h`. With `--help`, git looks for a man page and fails.
