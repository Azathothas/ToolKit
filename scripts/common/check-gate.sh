#!/bin/sh
# check-gate.sh - run every check this repository has over its own tree.
#
# ⭐ ONE BINARY, AND THAT IS THE WHOLE CHANGE. tools/check holds every rule, so
# this is a wrapper rather than a runner: there is no list of checks here to
# fall out of step with the list there.
#
# ⛔ --fast IS GONE AND NOTHING WAS LOST. It existed to skip check-twins, which
# ran both halves of every sh/PowerShell pair and compared their answers, and
# which was most of a gate that took 13m20s on this host. The rules are one
# implementation now, so there are no halves to compare and no reason to skip
# anything: the full run is about 30 seconds. A caller still passing --fast is
# told that rather than silently getting something different.
#
# Usage:
#   sh scripts/common/check-gate.sh
#   sh scripts/common/check-gate.sh --json
#
# Exit codes: 0 agreed, 1 disagreed, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped. Piping it into anything
# reports the pipeline's status, so a run that failed reads as green.

set -u

CDPATH=''
export CDPATH
HERE=$(cd -- "$(dirname -- "$0")" && pwd)

for a in "$@"; do
  case "$a" in
    --fast)
      printf 'check-gate: --fast no longer means anything.\n' >&2
      printf '  It skipped check-twins, which compared the sh and PowerShell halves of\n' >&2
      printf '  every rule. The rules are one Go program now and the full run is about\n' >&2
      printf '  30 seconds. Drop the flag.\n' >&2
      exit 2
      ;;
  esac
done

exec sh "$HERE/check.sh" "$@"
