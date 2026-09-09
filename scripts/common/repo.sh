#!/bin/sh
# repo.sh - build this repository's tool box and run one of its tools.
#
# ⭐ THE TOOLS ARE ONE BINARY. tools/repo holds the ones that are not gate
# checks: a host probe, a commit path, a licence writer and two remote readers.
# Everything under scripts/common that used to be a sh script and a PowerShell
# twin of the same tool is a wrapper around one subcommand of it.
#
# ⛔ WHY THEY ARE NOT SHELL ANY MORE. Each was written twice, in sh and in
# PowerShell, because the default host here is Windows and a POSIX script cannot
# be assumed to run on it. Keeping the two in step needed a third check that ran
# both halves of every pair and compared their answers, and that check was most
# of a gate taking about twelve minutes. One implementation that runs natively
# on either host has no halves to compare.
#
# ⛔ WHY NOT tools/check. That binary holds the rules this repository enforces
# over its own tree, and check-gate runs all of them. Folding a commit path and
# a licence writer in would make the gate do things that are not checks.
#
# Usage:
#   sh scripts/common/repo.sh                 the tools, listed
#   sh scripts/common/repo.sh deslop          one tool
#   sh scripts/common/repo.sh deslop --json   one tool, as one object
#
# Exit codes: 0 it ran and agreed, 1 it ran and disagreed, 2 it could not run.
#
# ⛔ Read the exit code from this process, unpiped. Piping it into anything
# reports the pipeline's status, so a run that failed reads as green.

set -u

command -v git >/dev/null 2>&1 || { printf 'repo: git not found\n' >&2; exit 2; }
git rev-parse --show-toplevel >/dev/null 2>&1 || { printf 'repo: not a git repository\n' >&2; exit 2; }
REPO_ROOT=$(git rev-parse --show-toplevel)
command -v go >/dev/null 2>&1 || {
  printf 'repo: no Go toolchain on PATH, and these tools are a Go program.\n' >&2
  printf '  That is "could not run" rather than a pass: nothing was done.\n' >&2
  exit 2
}

# ⚠ The binary goes under .tmp/, which is gitignored. Building it into the tree
# would make a tool's own output a tracked file another tool then reads.
BIN="$REPO_ROOT/.tmp/repo"
case "$(uname -s 2>/dev/null || echo unknown)" in
  MINGW*|MSYS*|CYGWIN*|Windows_NT) BIN="$BIN.exe" ;;
esac
mkdir -p "$REPO_ROOT/.tmp" || { printf 'repo: cannot write to %s/.tmp\n' "$REPO_ROOT" >&2; exit 2; }

# ⛔ REBUILT EVERY RUN, not cached by hand. Go's own build cache makes an
# unchanged tree a fraction of a second, and a hand-rolled staleness test is how
# a session ends up running the tool it had yesterday.
if ! (cd "$REPO_ROOT/tools/repo" && go build -o "$BIN" . 2>&1); then
  printf 'repo: the tool box did not build\n' >&2
  exit 2
fi

cd "$REPO_ROOT" || { printf 'repo: cannot enter %s\n' "$REPO_ROOT" >&2; exit 2; }
exec "$BIN" "$@"
