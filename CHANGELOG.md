# Changelog

All notable changes to this project are documented in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the
project uses [Semantic Versioning](https://semver.org/).

## [0.1.0] - Unreleased

### Added

- Interactive terminal app: dashboard, list, clean with a plan, help.
- `scan`, `list` (with `--tsv`) and `clean` (with `--dry-run` and `--yes`) commands.
- Five classes: `PRUNE`, `SAFE`, `LIKELY`, `KEEP`, `REVIEW`, with PR state from `gh`.
- Safety: no `--force`, branches are kept, every worktree is checked again before
  removal. Skipped: worktrees in use by a running process, active Bloom workspaces,
  worktrees that hold another repository or worktree.
- `LIKELY` only when `HEAD` is on a remote branch or in the PR; PRs from forks are ignored.
- A missing folder is removed entry by entry, never with `git worktree prune`; a locked
  entry, or a detached one whose commits exist only there, is `KEEP`.
- A rebase, merge, cherry-pick, revert or bisect in progress is `KEEP`.
- A single Go binary for macOS and Linux; it checks up to 8 worktrees in parallel.
- Go unit tests, bats end-to-end tests on real throwaway repositories, CI on Ubuntu
  and macOS.
