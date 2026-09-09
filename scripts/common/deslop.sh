#!/bin/sh
# deslop.sh - which files in this tree address a reader as an agent?
#
# ⭐ A WRAPPER. The tool is `repo deslop` in tools/repo, and this exists so the
# documented command keeps working and so a consumer fetching one raw URL still
# gets something runnable. Its own header carries what the tool refuses and why.
#
# ⭐ AN INVENTORY, NOT A GATE. It exits 0 whether it finds twenty agent-facing
# files or none, because in the repository that SHIPS them their presence is
# correct. Only `--apply` changes anything, and only then can it fail.
#
# ⛔ IT IS AIMED AT ANOTHER TREE, NOT AT THIS ONE. Every path it matches is one
# that `Azathothas/TEMPLATE` ships. ⚠ Run with `--apply` here and it removes
# THIS repository's own router and methodology, which are content it wants
# rather than content it regrets.
#
# ⭐ THE INTENDED PATH IS TO NEVER INSTALL IT, which is a SELECTION made at
# adoption and cheaper than any removal:
# https://github.com/Azathothas/TEMPLATE/blob/main/docs/methodology/lean-adoption.md
#
# ⛔ IT NEVER TOUCHES HISTORY. No rebase, no amend, no filter, no force push.
# Removing a file going forward is complete and reversible; rewriting published
# history un-publishes nothing, because every fork, mirror, cache and archive
# keeps its copy. docs/security/remote-ops.md calls that a red line.
#
# Usage:
#   sh scripts/common/deslop.sh
#   sh scripts/common/deslop.sh --json
#   sh scripts/common/deslop.sh --apply
#
# Exit codes: 0 the inventory ran, or the removal succeeded;
#             1 --apply was asked for and could not be done safely;
#             2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

set -u

# ⚠ CDPATH is unset rather than blanked on the cd line: `CDPATH= cd` is the
# usual idiom and shellcheck reads it as a missing assignment (SC1007).
unset CDPATH
DIR=$(cd -- "$(dirname -- "$0")" && pwd) || {
  printf 'deslop: cannot resolve my own directory\n' >&2
  exit 2
}
exec sh "$DIR/repo.sh" deslop "$@"
