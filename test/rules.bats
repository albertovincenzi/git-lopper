#!/usr/bin/env bats
# Unit tests of the rules that decide what may be removed.

setup() {
  # shellcheck source=../bin/git-lopper
  source "$BATS_TEST_DIRNAME/../bin/git-lopper"
}

# classify LOCKED BUSY DIRTY MERGED UPSTREAM PR ON_REMOTE

@test "classify: locks, work in progress, changes and unpushed commits are KEEP" {
  [ "$(classify 1 0 0 yes ok MERGED yes)" = "KEEP${TAB}locked" ]
  [ "$(classify 0 1 0 yes ok MERGED yes)" = "KEEP${TAB}rebase or merge in progress" ]
  [ "$(classify 0 0 2 yes ok MERGED yes)" = "KEEP${TAB}2 changed files" ]
  [ "$(classify 0 0 1 yes ok MERGED yes)" = "KEEP${TAB}1 changed file" ]
  [ "$(classify 0 0 0 yes '[ahead 1, behind 3]' MERGED yes)" = "KEEP${TAB}unpushed commits" ]
}

@test "classify: HEAD in the default branch is SAFE" {
  [ "$(classify 0 0 0 yes ok none no)" = "SAFE${TAB}already in the default branch" ]
  [ "$(classify 0 0 0 yes none OPEN no)" = "SAFE${TAB}already in the default branch" ]
}

@test "classify: done upstream with HEAD on a remote is LIKELY" {
  [ "$(classify 0 0 0 no ok MERGED yes)" = "LIKELY${TAB}PR merged" ]
  [ "$(classify 0 0 0 no ok CLOSED yes)" = "LIKELY${TAB}PR closed" ]
  [ "$(classify 0 0 0 no '[gone]' none yes)" = "LIKELY${TAB}remote branch deleted" ]
}

@test "classify: done upstream with commits only here is REVIEW" {
  [ "$(classify 0 0 0 no '[gone]' none no)" = "REVIEW${TAB}commits not on any remote" ]
  [ "$(classify 0 0 0 no none MERGED no)" = "REVIEW${TAB}commits not on any remote" ]
}

@test "classify: everything else is REVIEW" {
  [ "$(classify 0 0 0 no ok OPEN no)" = "REVIEW${TAB}PR open" ]
  [ "$(classify 0 0 0 no none none no)" = "REVIEW${TAB}never pushed" ]
  [ "$(classify 0 0 0 no - none no)" = "REVIEW${TAB}detached HEAD" ]
  [ "$(classify 0 0 0 no ok unknown no)" = "REVIEW${TAB}PR state unknown" ]
  [ "$(classify 0 0 0 no '[behind 2]' none no)" = "REVIEW${TAB}no PR" ]
}

@test "overlaps: inside, both directions, siblings and odd characters" {
  overlaps in $'/p/busy/sub\n/p/x' /p/busy
  overlaps in $'/p/busy/sub\n/p/x' /p/x
  ! overlaps in $'/p/busy/sub\n/p/x' /p/bus
  ! overlaps in '' /p
  ! overlaps in /b/ws /b/ws/inner
  overlaps both /b/ws /b/ws/inner
  overlaps in '/p/a\b c' '/p/a\b c'
}

@test "protected: active Bloom workspaces, and paths inside them" {
  BLOOM_DIR=/b BLOOM_ACTIVE=/b/r/live CWDS=''
  [ "$(protected /b/r/live)" = "active Bloom workspace" ]
  [ "$(protected /b/r/live/nested)" = "active Bloom workspace" ]
  [ -z "$(protected /b/r/archived)" ]
}

@test "protected: an unreadable Bloom db protects the whole Bloom folder" {
  BLOOM_DIR=/b BLOOM_DIR_REAL=/b BLOOM_ACTIVE='*' CWDS=''
  [ "$(protected /b/r/any)" = "Bloom workspace (Bloom db unreadable)" ]
  [ -z "$(protected /elsewhere/wt)" ]
}

@test "protected: Bloom paths in NFD match git paths in NFC" {
  printf a | iconv -f UTF-8-MAC -t UTF-8 >/dev/null 2>&1 || skip "this iconv has no UTF-8-MAC"
  BLOOM_DIR=/b BLOOM_ACTIVE=$'/b/r/dovr\xc3\xa0' CWDS=''
  [ "$(protected $'/b/r/dovra\xcc\x80')" = "active Bloom workspace" ]
}

@test "protected: a process cwd in the worktree, not in a sibling with the same prefix" {
  BLOOM_ACTIVE='' CWDS=$'/p/busy/sub\n/p/x'
  [ "$(protected /p/busy)" = "in use by a running process" ]
  [ "$(protected /p/x)" = "in use by a running process" ]
  [ -z "$(protected /p/bus)" ]
}
