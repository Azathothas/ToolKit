#!/bin/sh
# check-remote-items.sh - what is open against this repository, and does it
# say anything that survives being checked?
#
# ⭐ A WRAPPER. The tool is `repo remote-items` in tools/repo.
#
# The defect it exists to catch is a change accepted on the strength of its
# own description. A bot's pull request title says what it believes it is
# doing. A contributor's issue says what they believe is wrong. Both are
# CLAIMS, and both are usually right, which is exactly what makes the wrong one
# expensive: nobody is looking by the hundredth bump.
#
# ⭐ THIS WAS PAID FOR ON THIS REPOSITORY, TWICE, IN ONE HOUR.
#   1. `actions/checkout` was pinned to v4, and v4 targets Node 20, which
#      GitHub had deprecated. The runs were being force-migrated with a warning
#      in a log nobody reads. Resolving a tag is not the same as checking what
#      it declares.
#   2. The replacement pin was v5, chosen by looking only at v5 and v4. v7
#      already existed. A tag resolving cleanly says nothing about whether it
#      is current.
#
# ⛔ IT IS READ ONLY. It never merges, never closes, never comments, never
# approves. Deciding is the operator's. docs/security/remote-ops.md.
#
# ⚠ IT CANNOT TELL YOU WHETHER A CHANGE IS A GOOD IDEA. It checks the facts an
# item asserts about the world. Whether you want the change is a reading.
#
# Usage:
#   sh scripts/common/check-remote-items.sh
#   sh scripts/common/check-remote-items.sh --json
#   sh scripts/common/check-remote-items.sh --repo OWNER/NAME
#
# Exit codes: 0 nothing open, or nothing open failed a check;
#             1 an item's claim did not survive checking;
#             2 could not run.
#
# ⚠ AN UNREAD ITEM IS NOT A FAILED CHECK, and this used to exit 1 for one. Any
# repository with an open issue was then permanently red, which is how a check
# stops being read: the one state it cannot report is the one it exists for.
#
# ⛔ `--json` PUTS THE JSON DOCUMENT ON STDOUT AND NOTHING ELSE. The report
# still goes out, on stderr, where a human reading a terminal sees it and a
# gate runner reading stdout does not.
#
# ⛔ Read the exit code from this process, unpiped.

set -u

unset CDPATH
DIR=$(cd -- "$(dirname -- "$0")" && pwd) || {
  printf 'check-remote-items: cannot resolve my own directory
' >&2
  exit 2
}
exec sh "$DIR/repo.sh" remote-items "$@"
