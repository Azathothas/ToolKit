#!/bin/sh
# git-sync.sh - the sanctioned way to commit and push in a project that
# started from this template.
#
# ⭐ A WRAPPER. The tool is `repo git-sync` in tools/repo.
#
# The defect it exists to catch is a rule that everybody agreed to and nobody
# enforces. docs/conventions/git.md states the identity rule and the
# attribution rule; before this existed the template DOCUMENTED both and
# ENFORCED neither, so the only thing standing between a project and a commit
# crediting a tool was whether the agent that session had read the file.
#
# ⭐ WHAT IT MAKES MECHANICAL, and each one has cost a real session:
#
#   1. Author AND committer are pinned per invocation, so a machine whose
#      global config says something else still produces the right commit.
#      ⚠ `git commit --author` sets only the author, which is why both are
#      set: a commit can carry two different identities and the one shown in a
#      log is not the one a checker reads.
#   2. An AI-attribution line is REFUSED, never stripped. Silently rewriting
#      somebody's commit message is worse than declining to commit it: the
#      author never learns the rule and the next message has the same line.
#   3. A CI-skip marker is refused unless the flag was passed. A message that
#      merely MENTIONS `[skip ci]` skips CI, because GitHub does not read the
#      sentence around it. That shipped a commit with no run once.
#   4. The body is read from a FILE, never from a shell string. A body with an
#      apostrophe in it does not survive a shell, and the failure is silent:
#      see docs/conventions/shell.md section 1.
#
# ⛔ NOTHING ABOUT THIS KNOWS WHO YOU ARE. The identity comes from
# --name/--email or from git config, and if neither has one it refuses rather
# than guessing. A template must never carry a person baked into it.
#
# ⚠ IT IS A HELPER, NOT A CHECK. It writes: that is its job. `--check` is the
# read-only half and satisfies the check contract in scripts/README.md; the
# rest deliberately does not.
#
# Usage:
#   sh scripts/common/git-sync.sh --check
#   sh scripts/common/git-sync.sh --message "Subject" --body-file msg.txt
#   sh scripts/common/git-sync.sh --message "Subject" --no-push
#   sh scripts/common/git-sync.sh --push-only
#   sh scripts/common/git-sync.sh --message "Subject" --path README.md --path docs
#   sh scripts/common/git-sync.sh --message "Subject" --gate "sh scripts/common/check-gate.sh"
#   sh scripts/common/git-sync.sh --message "Docs only" --no-ci
#
# Exit codes: 0 done, 1 a rule was broken or a gate failed, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

set -u

unset CDPATH
DIR=$(cd -- "$(dirname -- "$0")" && pwd) || {
  printf 'git-sync: cannot resolve my own directory
' >&2
  exit 2
}
exec sh "$DIR/repo.sh" git-sync "$@"
