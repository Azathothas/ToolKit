#!/bin/sh
# check.sh - build the gate and run it.
#
# ⭐ THE GATE IS ONE BINARY. tools/check holds every rule this repository
# enforces over its own tree, and this is the shortest path from a session to
# an answer. Everything under scripts/common/check-*.sh is a wrapper around one
# named check of it.
#
# ⛔ WHY THE RULES ARE NOT SHELL ANY MORE. Each one used to be written twice,
# in sh and in PowerShell, because the default host here is Windows and a POSIX
# check cannot be assumed to run on it. Keeping the two in step needed a third
# check that ran both halves of every pair and compared their answers, and that
# check was most of a gate that took 13m20s. One implementation that runs
# natively on either host has no halves to compare, and the whole gate now
# takes about 30 seconds.
#
# Usage:
#   sh scripts/common/check.sh              every check, with a verdict
#   sh scripts/common/check.sh docs         one check
#   sh scripts/common/check.sh docs --json  one check, as one object
#
# Exit codes: 0 it ran and agreed, 1 it ran and disagreed, 2 it could not run.
#
# ⛔ Read the exit code from this process, unpiped. Piping it into anything
# reports the pipeline's status, so a run that failed reads as green.

set -u

command -v git >/dev/null 2>&1 || { printf 'check: git not found\n' >&2; exit 2; }
git rev-parse --show-toplevel >/dev/null 2>&1 || { printf 'check: not a git repository\n' >&2; exit 2; }
REPO_ROOT=$(git rev-parse --show-toplevel)
command -v go >/dev/null 2>&1 || {
  printf 'check: no Go toolchain on PATH, and the gate is a Go program.\n' >&2
  printf '  That is "could not run" rather than a pass: nothing was checked.\n' >&2
  exit 2
}

# ⚠ The binary goes under .tmp/, which is gitignored. Building it into the tree
# would make the gate's own output a tracked file the gate then reads.
BIN="$REPO_ROOT/.tmp/check"
case "$(uname -s 2>/dev/null || echo unknown)" in
  MINGW*|MSYS*|CYGWIN*|Windows_NT) BIN="$BIN.exe" ;;
esac
mkdir -p "$REPO_ROOT/.tmp" || { printf 'check: cannot write to %s/.tmp\n' "$REPO_ROOT" >&2; exit 2; }

# ⛔ REBUILT EVERY RUN, not cached by hand. Go's own build cache makes an
# unchanged tree a fraction of a second, and a hand-rolled staleness test is
# how a session ends up running the gate it had yesterday.
if ! (cd "$REPO_ROOT/tools/check" && go build -o "$BIN" . 2>&1); then
  printf 'check: the gate did not build\n' >&2
  exit 2
fi

cd "$REPO_ROOT" || { printf 'check: cannot enter %s\n' "$REPO_ROOT" >&2; exit 2; }
exec "$BIN" "$@"
