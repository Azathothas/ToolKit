#!/bin/sh
# check-record.sh - do the record's counts still agree with its rows, and does
# every entry have a row and every row an entry?
#
# The defect this exists to catch is a record that contradicts itself. Closing
# one entry moves several numbers: the index total line, the priority table row,
# that table's total row, and the status beside the entry itself. Nothing is
# wrong with any single file afterwards. What is missing is anything that
# compares two of them.
#
# ⭐ THIS WAS PAID FOR BEFORE IT WAS WRITTEN. docs/methodology/work-todo.md
# records the incident: a session closed two high-priority entries, wrote it
# into the entries, the index and the record, pushed, then rewrote a fourth file
# and never pushed again. The published state said those entries were open,
# beside entries saying done, for the whole of the next session.
#
# ── WHAT IT ASSERTS ─────────────────────────────────────────────────────────
#   • every id in the index table has an entry heading in the named file;
#   • every entry heading has a row in the index;
#   • the status in the index equals the status in the entry;
#   • the `total N open N blocked N done N` line agrees with the rows;
#   • the priority table's per-row and total figures agree with the rows.
#
# ⛔ IT CANNOT CHECK THAT AN ENTRY IS TRUE. That is a reading and it belongs to
# the review pass. It checks that the bookkeeping is consistent, which is the
# part that rots silently.
#
# Usage:
#   sh scripts/common/check-record.sh
#   sh scripts/common/check-record.sh --json
#   sh scripts/common/check-record.sh --dir TODO
#
# Exit codes: 0 consistent, 1 inconsistent, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

set -u

JSON=0
DIR="TODO"

while [ $# -gt 0 ]; do
  case "$1" in
    --json) JSON=1 ;;
    --dir) shift; DIR="${1:-}" ;;
    -h|--help) awk 'NR>1 { if (/^#/) { sub(/^# ?/, ""); print } else exit }' "$0"; exit 0 ;;
    *) printf 'check-record: unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
  shift
done

command -v git >/dev/null 2>&1 || { printf 'check-record: git not found\n' >&2; exit 2; }
command -v awk >/dev/null 2>&1 || { printf 'check-record: awk not found\n' >&2; exit 2; }
git rev-parse --show-toplevel >/dev/null 2>&1 || { printf 'check-record: not a git repository\n' >&2; exit 2; }
REPO_ROOT=$(git rev-parse --show-toplevel)

# ⛔ Every path below is relative to the repository root. `git ls-files` and a
# bare relative path both follow the process working directory, so without this
# a run from a subdirectory silently scopes itself and reports clean over
# everything else. The scope of a guard must not depend on who called it.
cd "$REPO_ROOT" || { printf 'check-record: cannot enter %s\n' "$REPO_ROOT" >&2; exit 2; }

INDEX="$DIR/INDEX.md"
[ -f "$INDEX" ] || { printf 'check-record: no index at %s\n' "$INDEX" >&2; exit 2; }

TMP="${TMPDIR:-/tmp}/.checkrecord.$$"
mkdir -p "$TMP" || { printf 'check-record: cannot write to %s\n' "$TMP" >&2; exit 2; }
trap 'rm -rf "$TMP"' EXIT INT TERM

PROBLEMS=""
COUNT=0
report() { PROBLEMS="$PROBLEMS  $1
"; COUNT=$((COUNT + 1)); }

# ── the index rows ──────────────────────────────────────────────────────────
# A row is: | ID | PRI | EFF | STATUS | title | [`file`](file) |
# ⚠ The header and the separator both start with `|`, so rows are selected on
# an id shaped like LETTERS-DIGITS rather than on position.
awk -F'|' '
  /^\|[[:space:]]*[A-Z][A-Z]*-[0-9][0-9]*[[:space:]]*\|/ {
    id = $2; pri = $3; eff = $4; st = $5; file = $7
    gsub(/^[ \t]+|[ \t]+$/, "", id)
    gsub(/^[ \t]+|[ \t]+$/, "", pri)
    gsub(/^[ \t]+|[ \t]+$/, "", eff)
    gsub(/^[ \t]+|[ \t]+$/, "", st)
    # the file cell is a markdown link; take what is inside the parentheses
    if (match(file, /\([^)]+\)/)) file = substr(file, RSTART + 1, RLENGTH - 2)
    gsub(/^[ \t]+|[ \t]+$/, "", file)
    print id "\t" pri "\t" eff "\t" st "\t" file
  }
' "$INDEX" > "$TMP/rows"

ROWS=$(wc -l < "$TMP/rows" | tr -d ' ')
[ "$ROWS" -gt 0 ] || { printf 'check-record: no entry rows found in %s\n' "$INDEX" >&2; exit 2; }

# ── the entry headings, from every category file in the directory ───────────
# A heading is: ## ID. Title
for f in "$DIR"/*.md; do
  case "$f" in
    "$INDEX"|"$DIR/PROGRESS.md"|"$DIR/RULES.md") continue ;;
  esac
  [ -f "$f" ] || continue
  awk -v F="$f" '
    /^##[[:space:]]+[A-Z][A-Z]*-[0-9][0-9]*\./ {
      line = $0
      sub(/^##[[:space:]]+/, "", line)
      id = line
      sub(/\..*$/, "", id)
      print id "\t" F
    }
  ' "$f" >> "$TMP/entries"
done
[ -f "$TMP/entries" ] || : > "$TMP/entries"

# ── every row has an entry, in the file the row names ───────────────────────
# ⚠ Fed by redirect, not by a pipe. A `while read` on the right of a pipe runs
# in a subshell and every count it increments is discarded on exit.
# docs/conventions/shell.md section 4.
#
# shellcheck disable=SC2034
# pri and eff are read only to consume their positions in the row, so that st
# and file land in the right variables. The priority table is checked in awk
# further down, which is the only place those two columns are used.
while IFS="$(printf '\t')" read -r id pri eff st file; do
  [ -n "${id:-}" ] || continue
  hit=$(awk -F'\t' -v I="$id" '$1 == I { print $2 }' "$TMP/entries" | head -1)
  if [ -z "$hit" ]; then
    report "$INDEX: row '$id' has no entry heading '## $id.' in any file under $DIR/"
    continue
  fi
  if [ "$hit" != "$DIR/$file" ] && [ "$hit" != "$file" ]; then
    report "$INDEX: row '$id' names $file but the entry is in $hit"
  fi
  # the status recorded beside the entry, if the entry states one
  est=$(awk -v I="$id" '
    $0 ~ "^##[[:space:]]+" I "\\." { inb = 1; next }
    inb && /^##[[:space:]]/ { exit }
    inb && /\*\*Status\*\*/ {
      line = $0
      sub(/.*\*\*Status\*\*[^A-Za-z]*/, "", line)
      sub(/[^A-Za-z].*$/, "", line)
      print line; exit
    }
  ' "$hit")
  if [ -n "$est" ] && [ "$est" != "$st" ]; then
    report "$id: index says '$st', $hit says '$est'"
  fi
done < "$TMP/rows"

# ── every entry has a row ───────────────────────────────────────────────────
while IFS="$(printf '\t')" read -r id file; do
  [ -n "${id:-}" ] || continue
  if ! awk -F'\t' -v I="$id" '$1 == I { found = 1 } END { exit !found }' "$TMP/rows"; then
    report "$file: entry '$id' has no row in $INDEX"
  fi
done < "$TMP/entries"

# ── the declared counts agree with the rows ─────────────────────────────────
D_TOTAL=$(awk '/^total[[:space:]]+[0-9]/ { print $2; exit }' "$INDEX")
D_OPEN=$(awk '/^total[[:space:]]+[0-9]/ { for (i = 1; i < NF; i++) if ($i == "open") print $(i+1); exit }' "$INDEX")
D_BLOCKED=$(awk '/^total[[:space:]]+[0-9]/ { for (i = 1; i < NF; i++) if ($i == "blocked") print $(i+1); exit }' "$INDEX")
D_DONE=$(awk '/^total[[:space:]]+[0-9]/ { for (i = 1; i < NF; i++) if ($i == "done") print $(i+1); exit }' "$INDEX")

A_TOTAL="$ROWS"
A_OPEN=$(awk -F'\t' '$4 == "open"' "$TMP/rows" | wc -l | tr -d ' ')
A_BLOCKED=$(awk -F'\t' '$4 == "blocked"' "$TMP/rows" | wc -l | tr -d ' ')
A_DONE=$(awk -F'\t' '$4 == "done"' "$TMP/rows" | wc -l | tr -d ' ')

if [ -z "${D_TOTAL:-}" ]; then
  report "$INDEX: no 'total N open N blocked N done N' line. Nothing can be checked against the rows."
else
  [ "$D_TOTAL" = "$A_TOTAL" ]     || report "$INDEX: declares total $D_TOTAL, rows say $A_TOTAL"
  [ "${D_OPEN:-}" = "$A_OPEN" ]   || report "$INDEX: declares open ${D_OPEN:-none}, rows say $A_OPEN"
  [ "${D_BLOCKED:-}" = "$A_BLOCKED" ] || report "$INDEX: declares blocked ${D_BLOCKED:-none}, rows say $A_BLOCKED"
  [ "${D_DONE:-}" = "$A_DONE" ]   || report "$INDEX: declares done ${D_DONE:-none}, rows say $A_DONE"
fi

# ── the priority table agrees with the rows ─────────────────────────────────
for p in P0 P1 P2 P3; do
  a_open=$(awk -F'\t' -v P="$p" '$2 == P && $4 == "open"' "$TMP/rows" | wc -l | tr -d ' ')
  a_blk=$(awk -F'\t' -v P="$p" '$2 == P && $4 == "blocked"' "$TMP/rows" | wc -l | tr -d ' ')
  a_done=$(awk -F'\t' -v P="$p" '$2 == P && $4 == "done"' "$TMP/rows" | wc -l | tr -d ' ')
  a_tot=$(awk -F'\t' -v P="$p" '$2 == P' "$TMP/rows" | wc -l | tr -d ' ')
  line=$(awk -F'|' -v P="$p" '
    $0 ~ /^\|/ {
      c = $2; gsub(/^[ \t]+|[ \t]+$/, "", c)
      if (c == P) { print $3 "\t" $4 "\t" $5 "\t" $6; exit }
    }' "$INDEX")
  [ -n "$line" ] || { report "$INDEX: priority table has no row for $p"; continue; }
  d_open=$(printf '%s' "$line" | cut -f1 | tr -d ' ')
  d_blk=$(printf '%s' "$line" | cut -f2 | tr -d ' ')
  d_done=$(printf '%s' "$line" | cut -f3 | tr -d ' ')
  d_tot=$(printf '%s' "$line" | cut -f4 | tr -d ' ')
  [ "$d_open" = "$a_open" ] || report "$INDEX: $p declares open $d_open, rows say $a_open"
  [ "$d_blk"  = "$a_blk"  ] || report "$INDEX: $p declares blocked $d_blk, rows say $a_blk"
  [ "$d_done" = "$a_done" ] || report "$INDEX: $p declares done $d_done, rows say $a_done"
  [ "$d_tot"  = "$a_tot"  ] || report "$INDEX: $p declares total $d_tot, rows say $a_tot"
done

# ── the RECORD's own count line agrees too ─────────────────────────────────
# ⛔ THIS WAS THE GAP AND IT WAS FOUND BY A REVIEW, NOT BY THE CHECK. The first
# version compared the index against the entries and stopped there, so
# PROGRESS.md sat declaring "open 14 blocked 1" beside an index saying
# "open 15 blocked 0" and this reported clean. work-todo.md is explicit that
# closing an entry moves "the record's own count lines" as well, and a checker
# that reads two of the three files is exactly the shape whose absence that
# document blames for the incident behind it.
RECORD="$DIR/PROGRESS.md"
if [ -f "$RECORD" ]; then
  R_LINE=$(awk '/total[[:space:]]+[0-9]+[[:space:]]+open[[:space:]]+[0-9]/ { print; exit }' "$RECORD")
  if [ -n "$R_LINE" ]; then
    r_total=$(printf '%s' "$R_LINE" | awk '{for (i=1;i<NF;i++) if ($i=="total") print $(i+1)}')
    r_open=$(printf '%s' "$R_LINE" | awk '{for (i=1;i<NF;i++) if ($i=="open") print $(i+1)}')
    r_blocked=$(printf '%s' "$R_LINE" | awk '{for (i=1;i<NF;i++) if ($i=="blocked") print $(i+1)}')
    r_done=$(printf '%s' "$R_LINE" | awk '{for (i=1;i<NF;i++) if ($i=="done") print $(i+1)}')
    [ "${r_total:-}" = "$A_TOTAL" ]     || report "$RECORD: declares total ${r_total:-none}, rows say $A_TOTAL"
    [ "${r_open:-}" = "$A_OPEN" ]       || report "$RECORD: declares open ${r_open:-none}, rows say $A_OPEN"
    [ "${r_blocked:-}" = "$A_BLOCKED" ] || report "$RECORD: declares blocked ${r_blocked:-none}, rows say $A_BLOCKED"
    [ "${r_done:-}" = "$A_DONE" ]       || report "$RECORD: declares done ${r_done:-none}, rows say $A_DONE"
  fi
  # ⚠ A record with no count line is not an error. Not every project states one,
  # and inventing a requirement here would fail a correct tree.
fi

# ── report ──────────────────────────────────────────────────────────────────
if [ "$JSON" = "1" ]; then
  printf '{"schema":"check-record/1","problems":%s,"entries":%s,"open":%s,"blocked":%s,"done":%s}\n' \
    "$COUNT" "$A_TOTAL" "$A_OPEN" "$A_BLOCKED" "$A_DONE"
  [ "$COUNT" -gt 0 ] && exit 1
  exit 0
fi

if [ "$COUNT" -gt 0 ]; then
  printf 'record check failed, %s problem(s):\n\n' "$COUNT"
  printf '%s\n' "$PROBLEMS"
  printf 'The rules are in docs/methodology/work-todo.md. Fix the file that is\n'
  printf 'wrong, not the count that reports it.\n'
  exit 1
fi

printf 'record ok: %s entries (%s open, %s blocked, %s done), counts agree with rows\n' \
  "$A_TOTAL" "$A_OPEN" "$A_BLOCKED" "$A_DONE"
exit 0
