#!/bin/sh
# deslop.sh - which files in this tree address a reader as an agent?
#
# ⭐ AN INVENTORY, NOT A GATE, and the distinction matters here more than
# anywhere else in this directory. It exits 0 whether it finds twenty
# agent-facing files or none, exactly as scripts/doctor/ does, because in the
# repository that SHIPS them their presence is correct. Only `--apply` changes
# anything, and only then can it fail.
#
# ⚠ IT IS NOT A CHECK, so scripts/README.md's five-point contract does not
# apply in full. What it keeps is the header rule, the exit-code rule, and
# read-only-unless-a-flag-is-passed.
#
# -- WHAT IT IS FOR ----------------------------------------------------------
#
# A project adopts the engineering of `Azathothas/TEMPLATE` and wants none of
# the content written for a machine: a compliance rule forbids it, or the
# maintainer does not want it, which is a complete reason on its own.
#
# ⛔ IT IS AIMED AT ANOTHER TREE, NOT AT THIS ONE. It reports on whichever
# repository it is run from, and every path it matches below is one that
# template ships. ⚠ Run with `--apply` here and it removes THIS repository's
# own router and methodology, which are content it wants rather than content it
# regrets.
#
# ⭐ THE INTENDED PATH IS TO NEVER INSTALL IT, which is a SELECTION made at
# adoption and cheaper than any removal:
# https://github.com/Azathothas/TEMPLATE/blob/main/docs/methodology/lean-adoption.md
# This script is the retrofit for a project that already took everything.
#
# -- ⛔ THE THREE THINGS IT WILL NOT DO --------------------------------------
#
# 1. ⛔ IT NEVER TOUCHES HISTORY. No rebase, no amend, no filter, no force
#    push. Removing a file going forward is complete and reversible; rewriting
#    published history un-publishes nothing, because every fork, mirror, cache
#    and archive keeps its copy, and it breaks every clone and every open
#    contribution. docs/security/remote-ops.md calls that a red line.
# 2. ⛔ IT NEVER DELETES WITHOUT --apply, and --apply refuses on a dirty tree.
#    A deletion mixed into uncommitted work cannot be reviewed and cannot be
#    undone with one command.
# 3. ⛔ IT DELETES NOTHING OUTSIDE THE LIST IT PRINTED. What you read is what
#    it removes.
#
# -- ⚠ WHAT IT CANNOT DECIDE -------------------------------------------------
#
# ⚠ Whether a file addresses an agent is a READING, and this matches names and
# greps for phrases. It will miss a file named something else, and it will
# flag a file that merely mentions the word. ⭐ The list is a starting point
# for a person, never an answer. That is why the default mode only prints.
#
# ⛔ AND IT CANNOT LIFT THE ENGINEERING OUT FIRST. The methodology documents
# carry four practices that are engineering rather than agent instruction, and
# a project that deletes them without reading them loses the part that was
# actually paid for. The lean-adoption page linked above names all four. Read
# them, write them into the project's own contributing guide, THEN run this.
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

JSON=0
APPLY=0

while [ $# -gt 0 ]; do
  case "$1" in
    --json)    JSON=1 ;;
    --apply)   APPLY=1 ;;
    --dry-run) APPLY=0 ;;   # the default; accepted so the intent can be written out
    -h|--help) awk 'NR>1 { if (/^#/) { sub(/^# ?/, ""); print } else exit }' "$0"; exit 0 ;;
    *) printf 'deslop: unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
  shift
done

command -v git >/dev/null 2>&1 || { printf 'deslop: git not found\n' >&2; exit 2; }
git rev-parse --show-toplevel >/dev/null 2>&1 || { printf 'deslop: not a git repository\n' >&2; exit 2; }
REPO_ROOT=$(git rev-parse --show-toplevel)
cd "$REPO_ROOT" || { printf 'deslop: cannot enter %s\n' "$REPO_ROOT" >&2; exit 2; }

# ⛔ MATCHED ON THE WHOLE PATH, ANCHORED. An unanchored match on "agent" would
# take src/agents/ in a project that happens to build one, which is a deletion
# of somebody's source code. Every pattern below names a path this template
# ships, and nothing else.
#
# ⚠ THE FAMILY OF ROUTER FILENAMES IS DELIBERATELY WIDE. Several tools read a
# file of this shape under their own name from anywhere in a tree, so a project
# that wants none of them wants all of these gone.
is_agent_facing() {
  case "$1" in
    AGENTS.md|*/AGENTS.md)         return 0 ;;
    CLAUDE.md|*/CLAUDE.md)         return 0 ;;
    GEMINI.md|*/GEMINI.md)         return 0 ;;
    .cursorrules|*/.cursorrules)   return 0 ;;
    .windsurfrules|*/.windsurfrules) return 0 ;;
    ROUTE.md|ADOPT.md|MAINTAIN.md) return 0 ;;
    bootstrap/*)                   return 0 ;;
    docs/methodology/*)            return 0 ;;
    docs/templates/*)              return 0 ;;
    .github/copilot-instructions.md) return 0 ;;
    *) return 1 ;;
  esac
}

FILES=$(
  {
    git ls-files 2>/dev/null
    git ls-files --others --exclude-standard 2>/dev/null
  } | sort -u
)

HITS=""
NHITS=0
for f in $FILES; do
  if is_agent_facing "$f"; then
    HITS="$HITS$f
"
    NHITS=$((NHITS + 1))
  fi
done

# ⭐ THE REFERENCES MATTER MORE THAN THE FILES. Deleting a document is easy;
# the expensive part is the twenty links elsewhere that now resolve to nothing,
# and check-docs is what finds those. Counting them here means the size of the
# real job is visible BEFORE anything is removed.
NREFS=0
if [ "$NHITS" -gt 0 ]; then
  PAT=$(printf '%s' "$HITS" | sed 's/[.[\*^$]/\\&/g' | tr '\n' '|' | sed 's/|$//')
  if [ -n "$PAT" ]; then
    NREFS=$(
      printf '%s\n' "$FILES" | while IFS= read -r f; do
        is_agent_facing "$f" && continue
        [ -f "$f" ] || continue
        printf '%s\n' "$f"
      done | tr '\n' '\0' | xargs -0 grep -lE "$PAT" 2>/dev/null | wc -l | tr -d ' '
    )
  fi
fi

if [ "$APPLY" = "0" ]; then
  if [ "$JSON" = "1" ]; then
    printf '{"schema":"deslop/1","agent_facing":%s,"referencing_files":%s,"applied":false}\n' \
      "$NHITS" "${NREFS:-0}"
    exit 0
  fi
  if [ "$NHITS" -eq 0 ]; then
    printf 'no agent-facing files in this tree.\n'
    exit 0
  fi
  printf 'agent-facing files, %s:\n\n%s\n' "$NHITS" "$HITS"
  printf '%s other file(s) reference one of them, and every such link breaks on removal.\n' "${NREFS:-0}"
  printf '\n'
  printf -- '⛔ Read the lean-adoption page in Azathothas/TEMPLATE before removing any\n'
  printf 'of this. Four practices under docs/methodology/ are engineering rather than\n'
  printf 'agent instruction. Lift them into the project'"'"'s own contributing guide first.\n'
  printf '\n'
  printf 'Nothing was changed. Pass --apply to remove the list above.\n'
  exit 0
fi

# -- --apply -----------------------------------------------------------------
# ⛔ REFUSES ON A DIRTY TREE. A deletion of this size mixed into uncommitted
# work cannot be reviewed, and cannot be undone with one command.
if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
  printf 'deslop: the tree is dirty. Commit or stash first.\n' >&2
  printf 'deslop: a removal this size has to be reviewable on its own.\n' >&2
  exit 1
fi

if [ "$NHITS" -eq 0 ]; then
  printf 'nothing to remove.\n'
  exit 0
fi

# ⛔ THE STATE IS READ BACK, AND THE REPORT IS WHAT IS TRUE rather than what was
# attempted. The predecessor ran `git rm ... || rm -f ... || true` in a loop and
# then printed the number it had PLANNED to remove, so a file something held
# open read as a file that had gone. docs/conventions/forbidden-patterns.md
# carries that exact shape, under a delete that reported success.
#
# ⚠ FED BY A HERE-DOCUMENT, NOT A PIPE. A `while read` on the right of a pipe
# runs in a subshell and every count it increments is discarded on exit, which
# is how the predecessor came to report a constant in the first place.
# docs/conventions/shell.md section 4.
REMOVED=0
SURVIVED=""
while IFS= read -r f; do
  [ -n "$f" ] || continue
  git rm -q -- "$f" >/dev/null 2>&1 || rm -f -- "$f" >/dev/null 2>&1 || true
  if [ -e "$f" ]; then
    SURVIVED="$SURVIVED  $f
"
  else
    REMOVED=$((REMOVED + 1))
  fi
done <<EOF
$HITS
EOF

printf 'removed %s of %s agent-facing file(s).\n\n' "$REMOVED" "$NHITS"
if [ -n "$SURVIVED" ]; then
  printf -- '⛔ STILL PRESENT after the removal:\n%s\n' "$SURVIVED"
  printf 'Something is holding them open, or the path is not writable. Nothing was\n'
  printf 'reported as removed that is still there.\n'
  exit 1
fi
printf -- '⛔ NOT DONE YET. %s file(s) referenced them and those links now resolve\n' "${NREFS:-0}"
printf 'to nothing. Run the documentation check and fix every one:\n\n'
printf '    sh scripts/common/check-docs.sh\n\n'
printf -- '⛔ History was NOT touched, and rewriting it would not un-publish anything.\n'
printf 'Every fork, mirror and archive keeps its copy. docs/security/remote-ops.md.\n'
exit 0
