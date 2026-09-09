#!/bin/sh
# check-no-secrets.sh - does any file carry something that must not be published?
#
# ⭐ A WRAPPER, and the rule lives in tools/check. Every rule this repository
# enforces over its own tree is one Go program now: it runs natively on either
# host, so there is no second implementation to keep in step and no third check
# comparing them. scripts/common/check.sh is the entry point and this forwards
# to one named check of it.
#
# Usage:
#   sh scripts/common/check-no-secrets.sh
#   sh scripts/common/check-no-secrets.sh --json
#
# Exit codes: 0 agreed, 1 disagreed, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

set -u
CDPATH=''
export CDPATH
HERE=$(cd -- "$(dirname -- "$0")" && pwd)
# ⚠ --public is accepted and dropped. It used to turn ON the rules that only
# matter for a public repository; this one IS public, so those rules are always
# on and the flag names a distinction that no longer exists here.
ARGS=""
for a in "$@"; do
  case "$a" in
    --public) continue ;;
    *) ARGS="$ARGS $a" ;;
  esac
done

# shellcheck disable=SC2086
# Deliberate: the accumulated arguments are meant to split into words.
exec sh "$HERE/check.sh" secrets $ARGS
