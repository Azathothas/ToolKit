#!/bin/sh
# check-gate.sh - run every local gate this host can run, in one command.
#
# The defect this exists to catch is a gate that was skipped because it was the
# ninth thing to remember. Part (a) of docs/methodology/gate.md is a LIST, and a
# list run by hand is a list run in the order somebody remembers it, missing
# whichever entry was added last. The session that wrote this ran that list once
# per work item and re-typed it every time, which is exactly how one of them
# quietly stops being run.
#
# ⛔ IT IS NOT A SECOND SET OF RULES. Every line below shells out to a check
# that already exists and reads that check's own exit code. When this file and
# .github/workflows/ci.yml disagree about what runs, CI is the one that gates a
# push and this one is the defect.
#
# -- ⚠ A SKIPPED CHECK IS NOT A PASSED CHECK ---------------------------------
#
# Some of these need a tool that is not everywhere: shellcheck, jq, pwsh,
# PSScriptAnalyzer. A gate that silently dropped one of them and still printed
# green would be the "step that exits 0 having done nothing it was asked to do"
# row in docs/conventions/forbidden-patterns.md.
#
# So a missing tool is reported as SKIP, counted separately, named in the
# summary line and carried in --json as `skipped`. ⭐ The exit code is still 0,
# because "this host cannot run that one" is not a failure of the tree; the
# caller reads `skipped` to decide whether it wants that answer. CI runs on two
# hosts that between them have every tool, which is where the coverage comes
# from.
#
# -- ⚠ --fast, AND WHY IT IS A FLAG RATHER THAN THE DEFAULT ------------------
#
# Measured on one Windows 11 Pro 26200 machine, 2026-08-29, with 13 twin pairs:
# the full run took 379s and check-twins ALONE took 270s, because that check
# runs both halves of every pair. Without it the sh half is 66s and the
# PowerShell half 41s. That is the right price before a push and the wrong one
# before each of a dozen commits, and a gate too slow to run is a gate that gets
# run once at the end.
#
# ⚠ The three numbers are separate runs on a machine doing other things, so
# they do not add up and are not meant to. Each carries its own conditions,
# which is what makes any of them comparable to a later one.
# ⚠ The figures carried before this were 208s and 171s over TEN pairs, taken
# on 2026-08-27. Two pairs were added on 2026-08-29 and the old numbers stopped
# describing this tree, which is why they were re-taken rather than adjusted.
#
# ⛔ --fast SKIPS check-twins. It does not weaken anything else, it is reported
# as a SKIP like every other, and the summary says so. The full run is what a
# push is gated on.
#
# Usage:
#   sh scripts/common/check-gate.sh
#   sh scripts/common/check-gate.sh --fast
#   sh scripts/common/check-gate.sh --json
#
# Exit codes: 0 everything that ran passed, 1 something failed, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

set -u

JSON=0
FAST=0
while [ $# -gt 0 ]; do
  case "$1" in
    --json) JSON=1 ;;
    --fast) FAST=1 ;;
    -h|--help) awk 'NR>1 { if (/^#/) { sub(/^# ?/, ""); print } else exit }' "$0"; exit 0 ;;
    *) printf 'check-gate: unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
  shift
done

command -v git >/dev/null 2>&1 || { printf 'check-gate: git not found\n' >&2; exit 2; }
git rev-parse --show-toplevel >/dev/null 2>&1 || { printf 'check-gate: not a git repository\n' >&2; exit 2; }
REPO_ROOT=$(git rev-parse --show-toplevel)

# ⛔ Every path below is relative to the repository root, so the scope of the
# gate does not depend on who called it. check-record.sh carries the same rule
# and the same reason.
cd "$REPO_ROOT" || { printf 'check-gate: cannot enter %s\n' "$REPO_ROOT" >&2; exit 2; }

HERE="scripts/common"
PASSED=0
FAILED=0
SKIPPED=0
FAILED_NAMES=""
SKIPPED_NAMES=""

# ⚠ Called directly, never on the right of a pipe. A function that increments a
# counter inside a pipeline runs in a subshell and every count it made is
# discarded on exit. docs/conventions/shell.md section 4.
record_pass() { PASSED=$((PASSED + 1)); [ "$JSON" = 1 ] || printf '  ok    %s\n' "$1"; }
record_fail() {
  FAILED=$((FAILED + 1))
  FAILED_NAMES="$FAILED_NAMES $1"
  [ "$JSON" = 1 ] || printf '  FAIL  %s (exit %s)\n' "$1" "$2"
}
record_skip() {
  SKIPPED=$((SKIPPED + 1))
  SKIPPED_NAMES="$SKIPPED_NAMES $1"
  [ "$JSON" = 1 ] || printf '  SKIP  %s -- %s\n' "$1" "$2"
}

# Run one check ONCE, read its exit code from the process that produced it, and
# show the output only when it failed. ⛔ Once: re-running it to inspect the
# output would be a second execution whose result could differ from the one that
# was scored.
check_simple() {
  name=$1
  shift
  out=$("$@" 2>&1)
  rc=$?
  if [ "$rc" = 0 ]; then
    record_pass "$name"
  else
    record_fail "$name" "$rc"
    [ "$JSON" = 1 ] || printf '%s\n' "$out" | sed 's/^/  | /'
  fi
}

[ "$JSON" = 1 ] || printf 'check-gate: %s\n\n' "$REPO_ROOT"

# -- the checks that are pure sh and always available ------------------------
check_simple 'check-docs'            sh "$HERE/check-docs.sh"
# ⛔ BOTH OF THESE, NOT ONE. check-docs reads markdown; check-markers reads
# every tracked text file and owns the character rule; check-one-home reads the
# documents against each other. The first was green on this tree while the
# second had 164 findings and the third 17, which is what "run both" costs when
# it is advice rather than a line in the runner.
check_simple 'check-markers'         sh "$HERE/check-markers.sh"
check_simple 'check-one-home'        sh "$HERE/check-one-home.sh"
check_simple 'check-placeholders'    sh "$HERE/check-placeholders.sh"
check_simple 'check-control-bytes'   sh "$HERE/check-control-bytes.sh"
check_simple 'check-record'          sh "$HERE/check-record.sh"
check_simple 'check-no-secrets'      sh "$HERE/check-no-secrets.sh" --public

# ⚠ 2 is "could not run", which is the honest answer in a project with no
# CHANGELOG.md, and it is a pass here rather than a failure. ⛔ Collapsing 2 into
# 0 with `|| true` would hide a genuine exit 1 as well.
cl_out=$(sh "$HERE/check-changelog.sh" 2>&1)
rc=$?
if [ "$rc" = 0 ] || [ "$rc" = 2 ]; then
  record_pass 'check-changelog'
else
  record_fail 'check-changelog' "$rc"
  [ "$JSON" = 1 ] || printf '%s\n' "$cl_out" | sed 's/^/  | /'
fi

# -- line endings, against git's own answer rather than a second table -------
bad=$(git ls-files --eol | grep -v 'i/lf' | grep -v 'i/-text')
if [ -z "$bad" ]; then
  record_pass 'line-endings'
else
  record_fail 'line-endings' 1
  [ "$JSON" = 1 ] || printf '%s\n' "$bad" | sed 's/^/  | /'
fi

# -- every tracked shell script parses ---------------------------------------
# ⛔ Every tracked file, not a hardcoded list. A list is a thing somebody
# forgets to extend, and the script they forgot is the one that breaks.
fail=0
for f in $(git ls-files '*.sh'); do
  sh -n "$f" 2>/dev/null || { fail=1; [ "$JSON" = 1 ] || printf '  | parse FAIL %s\n' "$f"; }
done
if [ "$fail" = 0 ]; then record_pass 'sh -n'; else record_fail 'sh -n' 1; fi

# -- shellcheck, when it is here ---------------------------------------------
if command -v shellcheck >/dev/null 2>&1; then
  fail=0
  for f in $(git ls-files '*.sh'); do
    if ! sc_out=$(shellcheck -s sh "$f" 2>&1); then
      fail=1
      [ "$JSON" = 1 ] || { printf '  | shellcheck %s\n' "$f"; printf '%s\n' "$sc_out" | sed 's/^/  | /'; }
    fi
  done
  if [ "$fail" = 0 ]; then record_pass 'shellcheck'; else record_fail 'shellcheck' 1; fi
else
  record_skip 'shellcheck' 'shellcheck is not on PATH'
fi

# -- the PowerShell half, which needs a PowerShell ---------------------------
PWSH=""
for c in pwsh pwsh.exe powershell.exe; do
  if command -v "$c" >/dev/null 2>&1; then PWSH=$c; break; fi
done

# ⛔ SCORED AS TWO ENTRIES, because they can have different answers. The parse
# either ran or it did not; the analyzer is a MODULE that may be absent, and
# check-powershell exits 0 either way. One verdict for both is how a skipped
# analyzer reads as a passed check, which is what it did here once, before the
# fixed status line below existed.
if [ -n "$PWSH" ]; then
  ps_out=$("$PWSH" -NoProfile -File "$HERE/check-powershell.ps1" 2>&1)
  rc=$?
  case "$rc" in
    0) record_pass 'powershell parse' ;;
    2) record_skip 'powershell parse' 'the host reported it could not run' ;;
    *)
      record_fail 'powershell parse' "$rc"
      [ "$JSON" = 1 ] || printf '%s\n' "$ps_out" | sed 's/^/  | /'
      ;;
  esac
  # ⛔ The FIXED status line, never the prose above it. check-powershell.ps1
  # prints `analyzer=clean|skipped|issues:N` as its last line and documents that
  # this is the contract.
  case "$ps_out" in
    *'analyzer=skipped'*) record_skip 'PSScriptAnalyzer' 'not installed on this host' ;;
    *'analyzer=clean'*)   record_pass 'PSScriptAnalyzer' ;;
    *'analyzer=issues'*)  record_fail 'PSScriptAnalyzer' 1 ;;
    *) record_skip 'PSScriptAnalyzer' 'check-powershell printed no analyzer status line' ;;
  esac
  # ⭐ THE ONE TEST IN THIS TREE. Part (a) of docs/methodology/gate.md is the
  # suite as well as the checks, and this is the suite: wsl-toolkit.ps1's
  # pure functions against a table of cases. It needs no WSL and no container
  # engine, so it runs wherever a PowerShell does, including the ubuntu job.
  check_simple 'wsl-toolkit selftest' \
    "$PWSH" -NoProfile -File scripts/windows/wsl-toolkit/selftest.ps1
  # ⛔ THE PRODUCT AGREES WITH ITS SOURCES. wsl-toolkit.ps1 is BUILT from the
  # parts under scripts/windows/wsl-toolkit/{src,core,libs}, and it is tracked
  # because a consumer fetching one raw URL cannot run a build step. That makes
  # it the one file here that can silently stop matching what anybody wrote: an
  # edit to a part that was never rebuilt, or an edit to the product that no
  # part carries. Both are invisible without this.
  check_simple 'wsl-toolkit bundle' \
    "$PWSH" -NoProfile -File scripts/windows/wsl-toolkit/build.ps1 -Check
else
  record_skip 'powershell parse' 'no pwsh, pwsh.exe or powershell.exe on PATH'
  record_skip 'PSScriptAnalyzer' 'no pwsh, pwsh.exe or powershell.exe on PATH'
  record_skip 'wsl-toolkit selftest' 'no pwsh, pwsh.exe or powershell.exe on PATH'
  record_skip 'wsl-toolkit bundle' 'no pwsh, pwsh.exe or powershell.exe on PATH'
fi

# -- the probe is not a gate, but it exiting non-zero is a real failure ------
check_simple 'doctor probe' sh scripts/doctor/doctor.sh --fast

# -- the twins, which need jq and a PowerShell ------------------------------
#
# ⛔ THIS PAIR RUNS THIS SCRIPT. check-twins.sh compares both halves of every
# twin and check-gate is one of them, so an unguarded call here is an infinite
# recursion: gate runs twins runs gate runs twins. It hung for ten minutes and
# left twenty stray shells holding their own files open, which is how this guard
# came to exist.
#
# CHECK_GATE_INNER is set for the child and honoured when this script finds it
# already set in its own environment. check-twins.sh exports it too, so a
# session that runs check-twins directly gets a gate one level deep rather than
# three. ⚠ It is an internal recursion guard and nothing else reads it.
if [ "$FAST" = 1 ]; then
  record_skip 'check-twins' '--fast was passed; check-twins alone is 270s, measured 2026-08-29'
elif [ "${CHECK_GATE_INNER:-}" = "1" ]; then
  record_skip 'check-twins' 'already inside check-twins; calling it here would recurse'
elif [ -z "$PWSH" ]; then
  record_skip 'check-twins' 'no PowerShell on PATH; it runs both halves of every pair'
elif ! command -v jq >/dev/null 2>&1; then
  record_skip 'check-twins' 'jq is not on PATH; it compares json'
else
  CHECK_GATE_INNER=1 check_simple 'check-twins' sh "$HERE/check-twins.sh"
fi

# -- report -----------------------------------------------------------------
TOTAL=$((PASSED + FAILED + SKIPPED))

if [ "$JSON" = 1 ]; then
  printf '{"schema":"check-gate/1","total":%s,"passed":%s,"failed":%s,"skipped":%s}\n' \
    "$TOTAL" "$PASSED" "$FAILED" "$SKIPPED"
  [ "$FAILED" -gt 0 ] && exit 1
  exit 0
fi

printf '\n'
if [ "$FAILED" -gt 0 ]; then
  printf 'GATE FAILED: %s of %s. Failed:%s\n' "$FAILED" "$TOTAL" "$FAILED_NAMES"
  [ "$SKIPPED" -gt 0 ] && printf 'Also skipped %s:%s\n' "$SKIPPED" "$SKIPPED_NAMES"
  exit 1
fi

if [ "$SKIPPED" -gt 0 ]; then
  printf 'gate ok: %s passed, but %s SKIPPED on this host:%s\n' "$PASSED" "$SKIPPED" "$SKIPPED_NAMES"
  printf 'A skipped check is not a passed check. CI runs on two hosts that between\n'
  printf 'them have every tool; that is where the coverage for these comes from.\n'
  exit 0
fi

printf 'gate ok: all %s checks passed\n' "$PASSED"
exit 0
