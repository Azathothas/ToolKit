#!/bin/sh
# fill-license.sh - write LICENSE from a template, with the holder filled in.
#
# ⭐ A WRAPPER. The tool is `repo license` in tools/repo, and this exists so the
# documented command keeps working and so a consumer fetching one raw URL still
# gets something runnable.
#
# The defect it exists to catch is a licence file with a placeholder still in
# it, or worse, one whose copyright line was rewritten when it should not have
# been.
#
# ⛔ A NAIVE "REPLACE THE COPYRIGHT LINE" SCRIPT CORRUPTS FIVE OF THESE TWELVE,
# and that is why the tool carries a table instead of a regex:
#
#   - The GPL, AGPL and LGPL texts open with the FREE SOFTWARE FOUNDATION's
#     copyright on the licence DOCUMENT ITSELF. It is not yours and rewriting
#     it is both wrong and a licence violation. Your copyright goes in the
#     per-file header, never in LICENSE.
#   - SPDX's ISC text is a licence INSTANCE, not a template: it carries
#     Internet Systems Consortium's own copyright, and shipping it unedited
#     attributes your software to them.
#   - MPL-2.0, CC0-1.0 and Unlicense have no copyright line to fill at all.
#
# Four placeholder styles appear across the set. Each licence names its own.
#
# Usage:
#   sh scripts/common/fill-license.sh --id MIT --holder "Some Name"
#   sh scripts/common/fill-license.sh --id MIT --holder "Some Name" --year 2026
#   sh scripts/common/fill-license.sh --id Apache-2.0 --holder "Some Name" --out LICENSE
#   sh scripts/common/fill-license.sh --list
#
# With no --holder, it is read from git config. Nothing is invented: if git has
# no user.name either, it refuses rather than guessing.
#
# Exit codes: 0 written, 1 refused or a placeholder survived, 2 could not run.

set -u

unset CDPATH
DIR=$(cd -- "$(dirname -- "$0")" && pwd) || {
  printf 'fill-license: cannot resolve my own directory\n' >&2
  exit 2
}
exec sh "$DIR/repo.sh" license "$@"
