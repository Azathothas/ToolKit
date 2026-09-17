#!/bin/sh
# shell-matrix.sh - prove text-tool writes the SAME BYTES from every shell.
#
# ⛔ THE CLAIM THIS TOOL MAKES IS ABOUT SHELLS, so it is the one claim that
# cannot be proved by a Go test. The Go suite calls Run() directly and never
# crosses a shell boundary at all, which is exactly the boundary where a payload
# loses its quoting. This runs the SAME base64 through every shell on the host
# and compares the digests.
#
# ⭐ THE PAYLOAD IS THE ONE THAT BROKE A QUOTED HEREDOC. docs/conventions/shell.md
# carries the measurement: this line, passed to `bash -c` inside a heredoc that
# was quoted, had its backticks EXECUTED.
#
# Usage: sh shell-matrix.sh PATH_TO_TEXT_TOOL [WORKDIR]
#
# It exits 0 when every shell that exists on this host produced one digest, and
# 1 when any of them disagreed or the tool refused. A shell that is not
# installed is SKIPPED and named, never silently passed.
#
# SPDX-License-Identifier: 0BSD
set -eu

TOOL=${1:?usage: sh shell-matrix.sh PATH_TO_TEXT_TOOL [WORKDIR]}
WORK=${2:-./.text-tool-matrix}

# ⚠ base64 OF THE HOSTILE PAYLOAD, not the payload itself. Putting the raw bytes
# in this file would make THIS file the thing that has to survive quoting, which
# is the defect it is measuring.
B64='YSBsaW5lIHdpdGggYGJhY2t0aWNrc2AsICRET0xMQVJTLCAicXVvdGVzIiwgJ2Fwb3N0cm9waGVzJyBhbmQgRDpcdG9vbHNceFxkKwo='

rm -rf "$WORK"
mkdir -p "$WORK"

digest() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

RAN=0
SKIPPED=0
FAILED=0
FIRST=''
FIRST_SHELL=''

# try SHELL_NAME COMMAND... - run one shell and record its digest.
try() {
  name=$1
  shift
  if ! command -v "$name" >/dev/null 2>&1; then
    printf '  skip  %-12s not installed\n' "$name"
    SKIPPED=$((SKIPPED + 1))
    return 0
  fi
  out="$WORK/$name.txt"
  rm -f "$out"
  if ! "$@" >/dev/null 2>&1; then
    printf '  FAIL  %-12s the tool exited non-zero\n' "$name"
    FAILED=$((FAILED + 1))
    return 0
  fi
  if [ ! -f "$out" ]; then
    printf '  FAIL  %-12s wrote no file\n' "$name"
    FAILED=$((FAILED + 1))
    return 0
  fi
  d=$(digest "$out")
  RAN=$((RAN + 1))
  if [ -z "$FIRST" ]; then
    FIRST=$d
    FIRST_SHELL=$name
    printf '  ok    %-12s %s\n' "$name" "$d"
  elif [ "$d" = "$FIRST" ]; then
    printf '  ok    %-12s same bytes\n' "$name"
  else
    printf '  FAIL  %-12s %s, and %s gave %s\n' "$name" "$d" "$FIRST_SHELL" "$FIRST"
    FAILED=$((FAILED + 1))
  fi
}

printf 'text-tool shell matrix: %s\n\n' "$TOOL"

# ⚠ EACH SHELL IS GIVEN THE COMMAND AS ONE STRING, because that is how a shell
# actually receives one from a harness, and it is where quoting is lost.
for sh_name in sh dash bash zsh ksh mksh busybox ash yash; do
  case $sh_name in
    busybox)
      try busybox busybox sh -c "$TOOL write $WORK/busybox.txt --b64 $B64"
      ;;
    *)
      try "$sh_name" "$sh_name" -c "$TOOL write $WORK/$sh_name.txt --b64 $B64"
      ;;
  esac
done

printf '\n'
if [ "$FAILED" -gt 0 ]; then
  printf '%d shell(s) ran, %d skipped, %d FAILED\n' "$RAN" "$SKIPPED" "$FAILED"
  exit 1
fi
if [ "$RAN" -lt 2 ]; then
  # ⛔ ONE SHELL PROVES NOTHING. The whole claim is that they AGREE, and a run
  # that found one shell has compared nothing and must not report success.
  printf '%d shell(s) ran and at least 2 are needed to compare anything\n' "$RAN"
  exit 1
fi
printf '%d shell(s) ran and agreed, %d skipped, 0 failed\n' "$RAN" "$SKIPPED"
