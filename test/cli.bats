#!/usr/bin/env bats
# End-to-end tests on a fixture of real repositories and worktrees.

load test_helper

@test "scan puts every worktree in the right class" {
  run lopper scan --no-fetch
  [ "$status" -eq 0 ]
  [ "$(class_of login)" = SAFE ]
  [ "$(class_of deps)" = SAFE ]
  [ "$(class_of "with space à")" = SAFE ]
  [ "$(class_of host)" = SAFE ]
  [ "$(class_of typo)" = KEEP ]
  [ "$(class_of cache)" = KEEP ]
  [ "$(class_of rebasing)" = KEEP ]
  [ "$(class_of locked)" = KEEP ]
  [ "$(class_of on-disk)" = KEEP ]
  [ "$(class_of detached)" = KEEP ]
  [ "$(class_of host/.nested/inner)" = KEEP ]
  [ "$(class_of old)" = PRUNE ]
  [ "$(class_of old-css)" = PRUNE ]
  [ "$(class_of search)" = LIKELY ]
  [ "$(class_of squashed)" = LIKELY ]
  [ "$(class_of gone)" = REVIEW ]
  [ "$(class_of billing)" = REVIEW ]
  [ "$(class_of idea)" = REVIEW ]
  [ "$(class_of orphan)" = KEEP ]
  [ "$(class_of peek)" = REVIEW ]
}

@test "scan without gh says the PR state is unknown" {
  mkdir "$BATS_TEST_TMPDIR/nogh"
  printf '#!/bin/sh\nexit 1\n' >"$BATS_TEST_TMPDIR/nogh/gh"
  chmod +x "$BATS_TEST_TMPDIR/nogh/gh"
  PATH="$BATS_TEST_TMPDIR/nogh:$PATH" run lopper scan --no-fetch
  [ "$status" -eq 0 ]
  contains "PR state is unknown"
  [ "$(class_of search)" = REVIEW ]
  [ "$(class_of squashed)" = REVIEW ]
}

@test "a failing 'gh pr list' gives an unknown PR state, not 'no PR'" {
  FAKE_GH_FAIL=1 lopper scan --no-fetch
  run lopper list --tsv REVIEW
  contains "$(printf 'PR state unknown\t')"
  [ "$(class_of squashed)" = REVIEW ]
}

@test "scan finds the repository around the current folder" {
  lopper -C "$W/typo" scan --no-fetch
  run lopper -C "$W/typo" list --tsv
  contains "feat/search"
  lacks "chore/deps"
}

@test "list needs a scan first, and known classes" {
  run lopper list
  [ "$status" -eq 2 ]
  contains "no scan"
  run lopper list FOO
  [ "$status" -eq 2 ]
  contains "unknown class"
}

@test "list --tsv prints the header and only the classes asked for" {
  lopper scan --no-fetch
  run lopper list --tsv keep
  [ "${lines[0]}" = "$(printf 'class\treason\trepo\tpath\tbranch\tbase\tpr\tpr_head\tupstream\tlast_commit')" ]
  [ "${#lines[@]}" -eq 9 ]
}

@test "clean --dry-run changes nothing" {
  lopper scan --no-fetch
  run lopper clean prune safe likely --dry-run
  [ "$status" -eq 0 ]
  contains "remove"
  [ -d "$W/login" ]
  [ "$(git -C "$WS/api" worktree list | wc -l)" -eq 16 ]
}

@test "clean without --yes and without a terminal does nothing" {
  lopper scan --no-fetch
  run lopper clean SAFE </dev/null
  [ "$status" -eq 2 ]
  [ -d "$W/login" ]
}

@test "clean never accepts KEEP or REVIEW" {
  lopper scan --no-fetch
  run lopper clean KEEP --yes
  [ "$status" -eq 2 ]
  run lopper clean REVIEW --yes
  [ "$status" -eq 2 ]
}

@test "clean removes PRUNE, SAFE and LIKELY, keeps the rest and all branches" {
  lopper scan --no-fetch
  run lopper clean PRUNE SAFE LIKELY --yes
  [ "$status" -eq 0 ]
  for d in login deps "with space à" search squashed; do [ ! -e "$W/$d" ]; done
  for d in typo cache rebasing locked gone billing idea host host/.nested/inner orphan peek; do [ -d "$W/$d" ]; done
  [ -f "$W/host/.nested/inner/work.txt" ]
  contains "holds another repository or worktree"
  # only the planned entries left git's list; the KEEP ones with a missing folder stay
  run git -C "$WS/api" worktree list --porcelain
  lacks "$W/old"$'\n'
  contains "$W/on-disk"
  contains "$W/detached"
  git -C "$WS/api" rev-parse -q --verify feat/search
  git -C "$WS/api" rev-parse -q --verify exp/squashed
  [ -z "$(class_of login)" ]
  [ "$(class_of typo)" = KEEP ]
}

@test "clean skips worktrees that changed after the scan" {
  lopper scan --no-fetch
  echo late >"$W/login/late.txt"
  commit "$W/deps" late
  run lopper clean SAFE --yes
  [ "$status" -eq 0 ]
  contains "changed since the scan"
  [ -d "$W/login" ]
  [ -d "$W/deps" ]
  [ ! -e "$W/with space à" ]
}

@test "clean skips a worktree that a running process uses" {
  command -v lsof >/dev/null || [ -e /proc/self/cwd ] || skip "no lsof and no /proc"
  lopper scan --no-fetch
  (cd "$W/login" && exec sleep 30) 3>&- &
  local pid=$!
  sleep 0.3
  run lopper clean SAFE --yes
  kill "$pid"
  contains "in use by a running process"
  [ -d "$W/login" ]
  [ ! -e "$W/deps" ]
}

@test "clean skips active Bloom workspaces" {
  command -v sqlite3 >/dev/null || skip "no sqlite3"
  sqlite3 "$LOPPER_BLOOM_DB" "create table workspaces (path text, state text);
    insert into workspaces values ('$W/login', 'active'), ('$W/old-css', 'setting_up'), ('$W/deps', 'archived');"
  lopper scan --no-fetch
  run lopper clean SAFE PRUNE --yes
  contains "active Bloom workspace"
  [ -d "$W/login" ]
  run git -C "$WS/web" worktree list --porcelain
  contains "$W/old-css"
  [ ! -e "$W/deps" ]
}

@test "version, help and bad input" {
  run lopper --version
  contains "git-lopper "
  run lopper -h
  contains "Usage:"
  run lopper --nope
  [ "$status" -eq 2 ]
  run lopper nope
  [ "$status" -eq 2 ]
  run lopper -d x scan
  [ "$status" -eq 2 ]
  run lopper scan SAFE
  [ "$status" -eq 2 ]
  contains "takes no arguments"
}
