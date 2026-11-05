# Builds a folder of throwaway repositories with one linked worktree per case.
# Used by the tests and by `make demo`. Needs git 2.32+ (GIT_CONFIG_GLOBAL).

# isolate from the user's git config (hooks, signing, default branch)
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=lopper GIT_AUTHOR_EMAIL=lopper@example.com
export GIT_COMMITTER_NAME=lopper GIT_COMMITTER_EMAIL=lopper@example.com

commit() { # DIR MESSAGE
  printf '%s\n' "$2" >>"$1/notes.txt"
  git -C "$1" add notes.txt && git -C "$1" commit -qm "$2"
}

new_repo() { # ROOT NAME — a clone of a local bare "origin" with one commit on main
  git init -q --bare -b main "$1/.origins/$2.git"
  git clone -q "$1/.origins/$2.git" "$1/$2" 2>/dev/null
  git -C "$1/$2" checkout -q -b main 2>/dev/null
  commit "$1/$2" init
  git -C "$1/$2" push -q -u origin main 2>/dev/null
  git -C "$1/$2" remote set-head origin main
}

add_wt() { # REPO BRANCH DIR
  git -C "$1" worktree add -q -b "$2" "$3" 2>/dev/null
}

pushed() { # DIR BRANCH — push and track
  git -C "$1" push -q -u origin "$2" 2>/dev/null
}

make_fixture() { # ROOT
  local root=$1 api=$1/api web=$1/web w=$1/worktrees
  mkdir -p "$root/.prs" "$w"

  new_repo "$root" api
  # SAFE: the work is merged into main
  add_wt "$api" feat/login "$w/login" && commit "$w/login" login
  git -C "$api" merge -q --ff-only feat/login && git -C "$api" push -q origin main 2>/dev/null
  # KEEP: uncommitted change
  add_wt "$api" fix/typo "$w/typo" && echo typo >"$w/typo/draft.txt"
  # KEEP: a commit that is not pushed
  add_wt "$api" feat/cache "$w/cache" && commit "$w/cache" cache-1
  pushed "$w/cache" feat/cache && commit "$w/cache" cache-2
  # KEEP: a rebase in progress
  add_wt "$api" feat/rebasing "$w/rebasing"
  mkdir "$(git -C "$w/rebasing" rev-parse --absolute-git-dir)/rebase-merge"
  # KEEP: locked
  add_wt "$api" feat/locked "$w/locked" && git -C "$api" worktree lock "$w/locked"
  # KEEP: locked, folder missing (like a disk that is not mounted)
  add_wt "$api" feat/on-disk "$w/on-disk" && git -C "$api" worktree lock "$w/on-disk" && rm -rf "$w/on-disk"
  # KEEP: folder missing, and its detached HEAD is the only reference to a commit
  git -C "$api" worktree add -q --detach "$w/detached" 2>/dev/null && commit "$w/detached" lost && rm -rf "$w/detached"
  # PRUNE: folder deleted by hand
  add_wt "$api" spike/old "$w/old" && rm -rf "$w/old"
  # LIKELY: PR merged, branch still on the remote
  add_wt "$api" feat/search "$w/search" && commit "$w/search" search && pushed "$w/search" feat/search
  # LIKELY: PR squash-merged, remote branch deleted, HEAD is the PR head
  add_wt "$api" exp/squashed "$w/squashed" && commit "$w/squashed" squashed && pushed "$w/squashed" exp/squashed
  git -C "$api" push -q origin --delete exp/squashed 2>/dev/null
  # REVIEW: remote branch deleted, but a later commit exists only here
  add_wt "$api" exp/gone "$w/gone" && commit "$w/gone" gone-1 && pushed "$w/gone" exp/gone && commit "$w/gone" gone-2
  git -C "$api" push -q origin --delete exp/gone 2>/dev/null
  git -C "$api" fetch -q --prune
  # REVIEW: PR open
  add_wt "$api" feat/billing "$w/billing" && commit "$w/billing" billing && pushed "$w/billing" feat/billing
  # REVIEW: never pushed
  add_wt "$api" local/idea "$w/idea" && commit "$w/idea" idea
  # SAFE outside, KEEP inside: a worktree in an ignored folder of another worktree
  echo .nested/ >>"$(git -C "$api" rev-parse --path-format=absolute --git-common-dir)/info/exclude"
  add_wt "$api" feat/host "$w/host"
  add_wt "$api" feat/inner "$w/host/.nested/inner" && echo wip >"$w/host/.nested/inner/work.txt"
  printf 'feat/search\tMERGED\t%s\nexp/squashed\tMERGED\t%s\nfeat/billing\tOPEN\t%s\n' \
    "$(git -C "$w/search" rev-parse HEAD)" "$(git -C "$w/squashed" rev-parse HEAD)" \
    "$(git -C "$w/billing" rev-parse HEAD)" >"$root/.prs/api"

  new_repo "$root" web
  # SAFE: branch created and never used
  add_wt "$web" chore/deps "$w/deps"
  # SAFE: a path with a space and a non-ASCII letter
  add_wt "$web" chore/space "$w/with space "$'\xc3\xa0'
  # PRUNE: folder deleted by hand
  add_wt "$web" fix/old-css "$w/old-css" && rm -rf "$w/old-css"
  # KEEP: detached HEAD with a commit that no ref contains
  git -C "$web" worktree add -q --detach "$w/orphan" 2>/dev/null && commit "$w/orphan" orphan
  # REVIEW: detached HEAD on a commit of a pushed branch
  add_wt "$web" feat/peek "$w/peek-branch" && commit "$w/peek-branch" peek && pushed "$w/peek-branch" feat/peek
  git -C "$web" worktree add -q --detach "$w/peek" feat/peek 2>/dev/null
  : >"$root/.prs/web"
}
