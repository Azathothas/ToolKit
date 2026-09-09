#!/bin/sh
# check-go.sh - gofmt, vet, build and test over every Go module here
#
# ⭐ A WRAPPER, and the rule lives in tools/check. Every rule this repository
# enforces over its own tree is one Go program now: it runs natively on either
# host, so there is no second implementation to keep in step and no third check
# comparing them. scripts/common/check.sh is the entry point and this forwards
# to one named check of it.
#
# Usage:
#   sh scripts/common/check-go.sh
#   sh scripts/common/check-go.sh --json
#
# Exit codes: 0 agreed, 1 disagreed, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

set -u
CDPATH=''
export CDPATH
HERE=$(cd -- "$(dirname -- "$0")" && pwd)
exec sh "$HERE/check.sh" go "$@"
