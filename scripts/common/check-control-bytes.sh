#!/bin/sh
# check-control-bytes.sh - does any tracked text file carry a literal control byte?
#
# ⭐ A WRAPPER, and the rule lives in tools/check. Every rule this repository
# enforces over its own tree is one Go program now: it runs natively on either
# host, so there is no second implementation to keep in step and no third check
# comparing them. scripts/common/check.sh is the entry point and this forwards
# to one named check of it.
#
# Usage:
#   sh scripts/common/check-control-bytes.sh
#   sh scripts/common/check-control-bytes.sh --json
#
# Exit codes: 0 agreed, 1 disagreed, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

set -u
CDPATH=''
export CDPATH
HERE=$(cd -- "$(dirname -- "$0")" && pwd)
exec sh "$HERE/check.sh" control-bytes "$@"
