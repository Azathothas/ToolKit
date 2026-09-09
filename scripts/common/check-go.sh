#!/bin/sh
# check-go.sh - does the wsl-toolkit executable still build, vet and pass its
# own tests?
#
# The defect this exists to catch is a repository that gained a compiled tool
# and left the gate reading only the PowerShell half of it. Before this file,
# every check here was about scripts and documents, so a Go change that did not
# compile reached a commit and the first thing to notice was CI, or a release.
#
# -- WHAT IT CHECKS, AND WHY EACH ONE IS SEPARATE ----------------------------
#   1. gofmt. A formatting difference is not a defect and an unformatted tree
#      is a diff nobody can read, so it is reported by NAME rather than fixed.
#   2. go vet. It catches the class a compiler does not: a printf verb that
#      disagrees with its argument, a lock copied by value, an unreachable
#      branch.
#   3. go build. The obvious one, and the only one of the four that a reader
#      would think of on their own.
#   4. go test. The suite over the parts that decide what a caller sees: the
#      archive validation that keeps a container away from the host, the
#      containment guard, the version reader.
#
# ⛔ THE MODULE IS NOT AT THE REPOSITORY ROOT and this resolves it rather than
# assuming. `go build ./...` run from the root of this tree finds no module at
# all and exits 1, which reads as a broken build and is a wrong directory.
#
# ⚠ NO go ON PATH IS "COULD NOT RUN", NOT "PASS". A machine without a Go
# toolchain has not proved this tree builds, and reporting green over an absent
# compiler is how a check quietly stops applying. Exit 2.
#
# Usage:
#   sh scripts/common/check-go.sh
#   sh scripts/common/check-go.sh --json
#
# Exit codes: 0 clean, 1 a step failed, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.
set -eu

JSON=0
for arg in "$@"; do
  case "$arg" in
    --json) JSON=1 ;;
    *) printf 'check-go: unknown argument %s\n' "$arg" >&2; exit 2 ;;
  esac
done

# ⛔ RESOLVED FROM THIS SCRIPT'S OWN LOCATION, never from the working directory.
# The check contract in scripts/README.md requires it, and a check that only
# works from the repository root is a check somebody runs from a subdirectory
# and misreads.
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH='' cd -- "$SCRIPT_DIR/../.." && pwd)
MODULE_DIR="$REPO_ROOT/tools/windows/wsl-toolkit"

fail() {
  if [ "$JSON" = 1 ]; then
    printf '{"schema":"check-go/1","ok":false,"reason":%s,"steps":[]}\n' "\"$1\""
  else
    printf 'check-go: %s\n' "$1" >&2
  fi
  exit "$2"
}

[ -d "$MODULE_DIR" ] || fail "no Go module at tools/windows/wsl-toolkit" 2
[ -f "$MODULE_DIR/go.mod" ] || fail "tools/windows/wsl-toolkit has no go.mod" 2
command -v go >/dev/null 2>&1 || fail "go is not on PATH" 2

# ⚠ A SCRATCH BUILD CACHE INSIDE THE TREE. The default cache is under the user's
# home, which a restricted process may not be able to write, and the failure
# arrives as "failed to initialize build cache" from a command that looks like a
# compiler problem. .tmp/ is gitignored.
GOCACHE_DIR="$REPO_ROOT/.tmp/go-build"
mkdir -p "$GOCACHE_DIR"
export GOCACHE="$GOCACHE_DIR"
export GOFLAGS=-mod=mod

STEPS=''
PROBLEMS=0

record() {
  # $1 name, $2 status, $3 detail
  if [ -n "$STEPS" ]; then STEPS="$STEPS,"; fi
  STEPS="$STEPS{\"step\":\"$1\",\"status\":\"$2\"}"
  if [ "$JSON" = 0 ]; then
    printf '  %-6s %s%s\n' "$2" "$1" "$3"
  fi
  if [ "$2" = "FAIL" ]; then PROBLEMS=$((PROBLEMS + 1)); fi
}

if [ "$JSON" = 0 ]; then
  printf 'check-go: %s\n' "$MODULE_DIR"
fi

# ⛔ EVERY EXIT CODE IS READ FROM THE PROCESS THAT PRODUCED IT. `if ! cmd; then`
# rather than `cmd; rc=$?`, because under `set -e` the second form's guard is
# unreachable: a failing command exits the shell before the test runs.
# The `|| true` belongs to gofmt alone, in a group. Written as
# `cd ... && gofmt ... || true` it belongs to the whole chain, so a cd that
# failed would run `true`, leave this empty and report the tree formatted.
# That is SC2015, and CI is where it was read.
UNFORMATTED=$(cd "$MODULE_DIR" && { gofmt -l . 2>/dev/null || true; })
if [ -n "$UNFORMATTED" ]; then
  record 'gofmt' 'FAIL' " -- $(printf '%s' "$UNFORMATTED" | tr '\n' ' ')"
else
  record 'gofmt' 'ok' ''
fi

BUILD_OUT="$REPO_ROOT/.tmp/check-go-build.txt"
if (cd "$MODULE_DIR" && go build ./... >"$BUILD_OUT" 2>&1); then
  record 'build' 'ok' ''
else
  record 'build' 'FAIL' " -- $(head -3 "$BUILD_OUT" | tr '\n' ' ')"
fi

VET_OUT="$REPO_ROOT/.tmp/check-go-vet.txt"
if (cd "$MODULE_DIR" && go vet ./... >"$VET_OUT" 2>&1); then
  record 'vet' 'ok' ''
else
  record 'vet' 'FAIL' " -- $(head -3 "$VET_OUT" | tr '\n' ' ')"
fi

TEST_OUT="$REPO_ROOT/.tmp/check-go-test.txt"
if (cd "$MODULE_DIR" && go test ./... >"$TEST_OUT" 2>&1); then
  record 'test' 'ok' ''
else
  record 'test' 'FAIL' " -- $(grep -m3 -E '^(---|FAIL|\s+[a-z_]+\.go)' "$TEST_OUT" | tr '\n' ' ')"
fi

if [ "$JSON" = 1 ]; then
  if [ "$PROBLEMS" = 0 ]; then OK=true; else OK=false; fi
  printf '{"schema":"check-go/1","ok":%s,"problems":%s,"steps":[%s]}\n' "$OK" "$PROBLEMS" "$STEPS"
elif [ "$PROBLEMS" = 0 ]; then
  printf 'check-go: clean.\n'
else
  printf 'check-go FAILED: %s step(s).\n' "$PROBLEMS" >&2
fi

[ "$PROBLEMS" = 0 ] || exit 1
exit 0
