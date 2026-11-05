# Shared setup: a fresh fixture, an isolated cache, a fake gh and no Bloom.
# shellcheck disable=SC2154,SC2034 # bats sets $output; the .bats files use $W
load fixture

setup() {
  WS=$BATS_TEST_TMPDIR/ws
  W=$WS/worktrees
  export LOPPER_CACHE_DIR=$BATS_TEST_TMPDIR/cache
  export LOPPER_BLOOM_DB=$BATS_TEST_TMPDIR/no-bloom.sqlite
  export LOPPER_BLOOM_DIR=$BATS_TEST_TMPDIR/bloom
  export FAKE_GH_DIR=$WS/.prs
  export PATH="$BATS_TEST_DIRNAME/bin:$PATH"
  make_fixture "$WS"
}

# CI puts the macOS /bin/bash 3.2 first on PATH, so this runs the tool with it
lopper() { bash "$BATS_TEST_DIRNAME/../bin/git-lopper" -C "$WS" "$@"; }

# bash 3.2 does not fail a test on a false [[ ]], so assert with functions
contains() { [[ $output == *"$1"* ]] || { printf 'output does not contain: %s\n--\n%s\n' "$1" "$output"; return 1; }; }
lacks() { [[ $output != *"$1"* ]] || { printf 'output contains: %s\n--\n%s\n' "$1" "$output"; return 1; }; }

class_of() { # PATH-BELOW-worktrees/ -> class in the saved scan
  lopper list --tsv | awk -F'\t' -v p="/worktrees/$1" 'substr($4, length($4) - length(p) + 1) == p {print $1}'
}
