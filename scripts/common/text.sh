#!/bin/sh
# text.sh - write and edit a file without the shell touching the payload.
#
# A WRAPPER. The tool is `text` in tools/text.
#
# WHAT IT IS FOR. A payload crossing a shell boundary loses its quoting SILENTLY:
# the file is written, nothing returns non-zero, and the damage is a substituted
# fragment in the middle of a long document. docs/conventions/shell.md section 1
# carries the measurement, where a quoted heredoc executed the backticks in a line
# of prose.
#
# USE YOUR HARNESS'S OWN WRITE AND EDIT TOOLS FIRST. They put bytes on disk with
# no shell in the path at all. This is for a harness that has none, and for what
# those tools cannot express: a substitution whose match count you want asserted,
# a line range, or a file that is not valid UTF-8.
#
# Usage:
#   sh scripts/common/text.sh write PATH --b64 BASE64
#   sh scripts/common/text.sh edit PATH --replace FIND --text NEW --expect 1
#   sh scripts/common/text.sh --help
#
# Exit codes: 0 it did it, 1 it refused, 2 it could not run.
#
# Read the exit code from this process, unpiped. Piping it into anything
# reports the pipeline's status, so a run that failed reads as green.

set -u

command -v git >/dev/null 2>&1 || { printf 'text: git not found\n' >&2; exit 2; }

# THE REPOSITORY IS RESOLVED FROM THIS FILE, NOT FROM THE CALLER'S DIRECTORY,
# for the reason findings 3 and 34 in TODO/PROGRESS.md state.
HERE=$(cd -- "$(dirname -- "$0")" && pwd) || { printf 'text: could not resolve this script\n' >&2; exit 2; }
git -C "$HERE" rev-parse --show-toplevel >/dev/null 2>&1 || { printf 'text: not a git repository\n' >&2; exit 2; }
REPO_ROOT=$(git -C "$HERE" rev-parse --show-toplevel)
command -v go >/dev/null 2>&1 || {
  printf 'text: no Go toolchain on PATH, and this tool is a Go program.\n' >&2
  printf '  That is "could not run" rather than a pass: nothing was done.\n' >&2
  exit 2
}

BIN="$REPO_ROOT/.tmp/text"
case "$(uname -s 2>/dev/null || echo unknown)" in
  MINGW*|MSYS*|CYGWIN*|Windows_NT) BIN="$BIN.exe" ;;
esac
mkdir -p "$REPO_ROOT/.tmp" || { printf 'text: cannot write to %s/.tmp\n' "$REPO_ROOT" >&2; exit 2; }

if ! (cd "$REPO_ROOT/tools/text" && go build -o "$BIN" . 2>&1); then
  printf 'text: the tool did not build\n' >&2
  exit 2
fi

# THE CALLER'S DIRECTORY IS KEPT, unlike the other wrappers here. A path this
# tool is given is the caller's own, so entering the repository root would make
# every relative path mean something else.
exec "$BIN" "$@"
