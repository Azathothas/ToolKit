#!/bin/sh
# check-docs.sh - do the documents still resolve, and are they written the way
# this repository writes documents?
#
# The defect this exists to catch is a document that was true when it was
# written. Four shapes of it, and every one is invisible to every other check:
#
#   - a link or a path that stopped resolving when something was renamed;
#   - a fenced shell block that does not parse, which is a block nobody can
#     copy and paste;
#   - an angle-bracket placeholder inside a shell block: a human reads it as
#     "fill this in" and bash reads it as a redirect, so the reader gets a
#     cryptic syntax error instead of an obvious instruction.
#
# ⚠ CONTROL BYTES ARE NOT CHECKED HERE. That rule scanned markdown only while
# every .ts, .py, .rs and .sh in the tree went unchecked, so it moved to
# check-control-bytes.sh, which reads every text file. Run both.
#
# It also enforces the mechanical half of the prose rule: no em dash, and no
# emoji outside the five this repository defines. docs/conventions/prose.md is
# the rule; this is the part a machine can hold.
#
# ⛔ WHAT IT DOES NOT CHECK IS WHETHER A CLAIM IS TRUE. That is a reading, and
# it belongs to the review pass. A guard that tried to verify prose would
# either pass vacuously or refuse legitimate writing, and both are worse than
# an honest scope.
#
# ⚠ EVERY PER-LINE TEST IS DONE IN awk, NOT IN A SHELL LOOP. The first version
# ran a pipeline per line of every file, which is tens of thousands of process
# spawns, and it did not finish in two minutes on Windows. One awk pass per
# file replaced it. The shell only touches the filesystem, which is the one
# thing awk should not be doing here.
#
# Usage:
#   sh scripts/common/check-docs.sh
#   sh scripts/common/check-docs.sh --json
#   sh scripts/common/check-docs.sh --path docs
#
# Exit codes: 0 clean, 1 something is wrong, 2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

set -u

JSON=0
SCOPE=""

while [ $# -gt 0 ]; do
  case "$1" in
    --json) JSON=1 ;;
    --path) shift; SCOPE="${1:-}" ;;
    -h|--help) awk 'NR>1 { if (/^#/) { sub(/^# ?/, ""); print } else exit }' "$0"; exit 0 ;;
    *) printf 'check-docs: unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
  shift
done

command -v git >/dev/null 2>&1 || { printf 'check-docs: git not found\n' >&2; exit 2; }
git rev-parse --show-toplevel >/dev/null 2>&1 || { printf 'check-docs: not a git repository\n' >&2; exit 2; }
command -v awk >/dev/null 2>&1 || { printf 'check-docs: awk not found\n' >&2; exit 2; }
SELF=check-docs
REPO_ROOT=$(git rev-parse --show-toplevel)

# ⛔ EVERY git QUERY BELOW RUNS FROM THE REPOSITORY ROOT. `git ls-files` is
# relative to the process working directory, so without this a run from a
# subdirectory silently scopes itself to that subtree and reports clean over
# everything else. The scope of a guard must not depend on who called it.
cd "$REPO_ROOT" || { printf '%s: cannot enter %s\n' "$SELF" "$REPO_ROOT" >&2; exit 2; }

# ⛔ TRACKED **PLUS UNTRACKED-BUT-NOT-IGNORED**. `git ls-files` alone cannot see
# a file that has never been staged, which is exactly when a new file is most
# likely to carry a defect and exactly what the next `git add -A` will take.
# Ignored files stay out: they are ignored on purpose.
list_files() {
  {
    git ls-files -- "$@" 2>/dev/null
    git ls-files --others --exclude-standard -- "$@" 2>/dev/null
  } | sort -u
}


# ⚠ THE TEMPLATE DIRECTORIES ARE EXEMPT FROM THE LINK CHECK, AND MUST BE.
# A template's links are written relative to where the file will live in the
# PROJECT, not where it lives here: docs/templates/AGENTS.md links to
# docs/methodology/gate.md because in a project that file sits at the root.
# Checking those here reports thirty-odd failures on a correct tree, and a
# check that fails on a correct tree gets switched off within a week.
# ⭐ The PROSE rules still apply to templates. Only link resolution is exempt,
# because only that one is position-dependent.
# The exempt paths are matched by a `case` in the loop below, so the test costs
# no process spawn.

if [ -n "$SCOPE" ]; then
  FILES=$(list_files "$SCOPE" | grep '\.md$' || true)
else
  FILES=$(list_files '*.md' || true)
fi
[ -z "$FILES" ] && { printf 'check-docs: no markdown files in scope\n' >&2; exit 2; }

TMP="${TMPDIR:-/tmp}/.checkdocs.$$"
mkdir -p "$TMP" || { printf 'check-docs: cannot write to %s\n' "$TMP" >&2; exit 2; }
trap 'rm -rf "$TMP"' EXIT INT TERM

PROBLEMS=""
COUNT=0
NFILES=0
NLINKS=0
NBLOCKS=0

report() { PROBLEMS="$PROBLEMS  $1
"; COUNT=$((COUNT + 1)); }

for f in $FILES; do
  NFILES=$((NFILES + 1))
  dir=$(dirname "$f")

  # ⚠ Decided ONCE per file, with a case statement, which is a shell builtin.
  # An earlier version ran `grep` against the filename inside the per-link
  # loop: one process spawn per link, which on Windows is most of the runtime.
  case "$f" in
    docs/templates/*|bootstrap/prompts/*) link_check=0 ;;
    *) link_check=1 ;;
  esac

  # ── one pass: strip fences and code spans, then emit every finding ────────
  # ⚠ Stripping code spans is why `[int](2.65)` inside backticks is not
  # reported as a broken link. Markdown does not linkify a code span, and an
  # earlier ad-hoc version of this check reported exactly that as broken.
  awk '
    BEGIN { FS = "\n" }
    /^[ \t]*```/ { fence = !fence; next }
    fence { next }
    {
      line = $0
      while (match(line, /`[^`]*`/))
        line = substr(line, 1, RSTART - 1) substr(line, RSTART + RLENGTH)

      if (index(line, "\342\200\224") > 0) print "EMDASH\t" NR "\t"

      rest = line
      while (match(rest, /\]\([^)\t ]+/)) {
        t = substr(rest, RSTART + 2, RLENGTH - 2)
        print "LINK\t" NR "\t" t
        rest = substr(rest, RSTART + RLENGTH)
      }
    }
  ' "$f" > "$TMP/find" 2>/dev/null || true

  while IFS="$(printf '\t')" read -r kind ln detail; do
    case "${kind:-}" in
      EMDASH)
        report "$f:$ln em dash. docs/conventions/prose.md" ;;
      LINK)
        [ "$link_check" = "0" ] && continue
        case "$detail" in http://*|https://*|mailto:*|'') continue ;; esac
        NLINKS=$((NLINKS + 1))
        target=${detail%%#*}
        [ -z "$target" ] && continue
        [ -e "$dir/$target" ] || report "$f:$ln broken link -> $detail" ;;
    esac
  done < "$TMP/find"

  # ⚠ THE CONTROL-BYTE RULE MOVED, IT WAS NOT DROPPED. It used to live here and
  # scanned markdown only, which left every .ts, .py, .rs, .sh and .yml in the
  # tree unchecked for the one defect that makes a file invisible to review.
  # It now lives in check-control-bytes.sh over EVERY text file. Two checks
  # enforcing one rule is two places for it to be wrong, so this one no longer
  # does it. ⛔ Run both: this one for documents, that one for the whole tree.

  # ── fenced shell blocks: extracted in one pass, then checked ─────────────
  rm -f "$TMP"/blk.*
  awk -v D="$TMP" '
    /^[ \t]*```(bash|sh)[ \t]*$/ { inb = 1; n++; start[n] = NR; next }
    inb && /^[ \t]*```/          { inb = 0; next }
    inb                          { print $0 > (D "/blk." n) }
    END { for (i = 1; i <= n; i++) print i "\t" start[i] }
  ' "$f" > "$TMP/blocks" 2>/dev/null || true

  while IFS="$(printf '\t')" read -r idx bstart; do
    [ -z "${idx:-}" ] && continue
    blk="$TMP/blk.$idx"
    [ -f "$blk" ] || continue
    NBLOCKS=$((NBLOCKS + 1))
    tr -d '\r' < "$blk" > "$blk.clean"
    sh -n "$blk.clean" 2>/dev/null || report "$f:$bstart shell block does not parse"
    if grep -qE '<[a-z][a-z0-9-]*>' "$blk.clean" 2>/dev/null; then
      report "$f:$bstart shell-unsafe placeholder. bash reads it as a redirect; use UPPER_SNAKE"
    fi
  done < "$TMP/blocks"
done

# ── a page nothing links to ─────────────────────────────────────────────────
# ⛔ AN UNLINKED PAGE IS NOT READ, SO IT IS NOT CORRECTED, and that is the state
# every stale document passes through on the way to being wrong.
#
# ⚠ This rule was written in docs/conventions/prose.md with nothing enforcing
# it, and within the hour a new prompt file was added that nothing referenced.
# A door sweep found it by hand. A rule that can be checked should be a check.
#
# Roots are exempt: a README is an entry point, and the files at the repository
# root are what a reader or a raw URL arrives at directly.
LINKED="$TMP/linked"
: > "$LINKED"
for f in $FILES; do
  d=$(dirname "$f")
  awk '
    /^[ \t]*```/ { fence = !fence; next }
    fence { next }
    {
      rest = $0
      while (match(rest, /\]\([^)\t ]+/)) {
        print substr(rest, RSTART + 2, RLENGTH - 2)
        rest = substr(rest, RSTART + RLENGTH)
      }
    }
  ' "$f" 2>/dev/null | while IFS= read -r tgt; do
      case "$tgt" in http://*|https://*|mailto:*|'') continue ;; esac
      t=${tgt%%#*}
      [ -z "$t" ] && continue
      # Normalise to a repo-relative path so a link from a subdirectory and one
      # from the root both name the same file.
      if [ "$d" = "." ]; then printf '%s\n' "$t"; else printf '%s/%s\n' "$d" "$t"; fi
    done >> "$LINKED"
done
# Collapse `a/../b` so ../ links resolve to the same string as a direct one.
sed -e ':a' -e 's![^/][^/]*/\.\./!!;ta' -e 's!^\./!!' "$LINKED" | sort -u > "$LINKED.n"

for f in $FILES; do
  case "$f" in
    */README.md|README.md) continue ;;
    */*) ;;
    *) continue ;;   # a root-level file is an entry point
  esac
  grep -qxF "$f" "$LINKED.n" || report "$f is linked from nowhere. An unlinked page is not read, so it is not corrected."
done

# ── only the five defined characters ────────────────────────────────────────
# Three prose markers and two status glyphs. docs/conventions/prose.md is the
# rule and says why the set is a list rather than a principle.
#
# ⚠ Needs a grep that speaks unicode ranges. BSD grep does not, so this
# degrades honestly rather than silently passing.
#
# ⛔ THE ALLOWLIST IS A LOOKAHEAD, NOT A SECOND grep -v. It used to be
# `grep -nP <banned> | grep -vP <allowed>`, and grep -v drops the whole LINE, so
# a line carrying an allowed marker hid every banned emoji beside it: `⛔ never
# use <party emoji>` passed the check. The lookahead excludes the CHARACTER, so
# the line is still reported for the character that is wrong.
EMOJI_NOTE=""
# ⚠ The probe tests the ACTUAL pattern shape, not merely whether -P exists.
# This machine's grep has -P but its PCRE is not in UTF-8 mode by default, so
# every `\x{...}` above U+00FF is refused with "character value too large". The
# `(*UTF)` prefix turns it on. Testing for -P alone reported the check as
# available and then printed that error on every run.
if printf '\342\255\220' | grep -qP '(*UTF)[\x{2B50}]' 2>/dev/null; then
  bad=$(printf '%s\n' "$FILES" | tr '\n' '\0' \
    | xargs -0 grep -nP '(*UTF)(?![\x{26D4}\x{2B50}\x{26A0}\x{2705}\x{274C}])[\x{1F300}-\x{1FAFF}\x{2190}-\x{21FF}\x{2300}-\x{23FF}\x{2500}-\x{27BF}\x{2B00}-\x{2BFF}\x{FE0F}]' 2>/dev/null \
    | head -5 || true)
  if [ -n "$bad" ]; then
    printf '%s\n' "$bad" | while IFS= read -r b; do
      printf '  %s\n' "$(printf '%s' "$b" | cut -c1-110)"
    done > "$TMP/emoji"
    while IFS= read -r e; do
      report "an emoji outside the five defined characters: $e"
    done < "$TMP/emoji"
  fi
else
  EMOJI_NOTE="⚠ the emoji check needs a grep with -P and was skipped"
fi

# ── report ──────────────────────────────────────────────────────────────────
if [ "$JSON" = "1" ]; then
  printf '{"schema":"check-docs/1","problems":%s,"files":%s,"links":%s,"shell_blocks":%s}\n' \
    "$COUNT" "$NFILES" "$NLINKS" "$NBLOCKS"
  [ "$COUNT" -gt 0 ] && exit 1
  exit 0
fi

if [ "$COUNT" -gt 0 ]; then
  printf 'documentation check failed, %s problem(s):\n\n%s\n' "$COUNT" "$PROBLEMS"
  [ -n "$EMOJI_NOTE" ] && printf '%s\n' "$EMOJI_NOTE"
  exit 1
fi

printf 'docs ok: %s files, %s relative links, %s shell blocks. Links and prose clean.\n' \
  "$NFILES" "$NLINKS" "$NBLOCKS"
[ -n "$EMOJI_NOTE" ] && printf '%s\n' "$EMOJI_NOTE"
exit 0
