#!/bin/sh
# Clear engine run state that this boot invalidated, and nothing else.
#
# It runs as TK_USER inside the owned distribution and is delivered on stdin,
# the same way verify.sh and provision.sh are.
#
# ⛔ WHY THIS EXISTS AT ALL. After a host reboot podman refuses every container
# with `current system boot ID differs from cached boot ID` and NAMES the two
# directories to delete. `base ensure` used to re-provision, report honestly that
# it still could not run a container, and stop, leaving the operator to do by
# hand a thing the engine had already written down. WSL-61.
#
# ⛔ IT IS NOT REACHED WITHOUT --repair. An unflagged `base ensure` prints the
# command and takes no deletion, because a tool that removes engine state to make
# a probe pass is one deletion away from removing something else.
#
# ⚠ THE ROOT COMES FROM THE ENGINE, NOT FROM $XDG_RUNTIME_DIR, AND THAT IS A
# MEASUREMENT AND NOT A PREFERENCE. Measured in this distribution on 2026-09-10:
# $XDG_RUNTIME_DIR is /mnt/wslg/runtime-dir, which WSLg owns, while podman's own
# run root is /tmp/wsl-toolkit-run-1000/containers. A version of this script that
# asked the environment cleared two directories that do not exist, found nothing
# to remove, and would have reported a successful repair over having done
# nothing.
#
# THE FOUR RULES THIS DELETION IS HELD TO:
#
#   1. ⛔ THE ROOT IS ASKED OF PODMAN, never typed here and never assumed from
#      the environment. `podman info` answers with the run root it is actually
#      using, so an account configured differently is cleared correctly instead
#      of confidently wrongly.
#   2. ⛔ ONLY TWO FIXED LEAF NAMES ARE APPENDED TO IT. Nothing here takes a path
#      from a caller, so there is no path to contain and no way to widen it.
#   3. ⛔ A ROOT THAT IS NOT A REAL SUBDIRECTORY IS REFUSED BY NAME, before
#      anything is removed.
#   4. ⛔ THE STATE IS READ BACK, and a run that removed nothing says so rather
#      than reporting a repair. A delete that did not happen reported as a delete
#      that did is `WSL-04`, the defect this tool's safety model was built
#      around.
set -eu

# RULE 1. Ask the engine. ⚠ `podman info` still answers in the stale-boot-id
# state, which is the only state this script runs in: podman refuses to RUN a
# container and remains happy to describe itself.
runroot=$(podman info --format '{{.Store.RunRoot}}' 2>/dev/null || echo '')
source=engine
if [ -z "$runroot" ]; then
  # ⚠ THE FALLBACK IS NAMED IN THE OUTPUT rather than used silently, because it
  # is the value that was measured to be wrong on this machine and a reader has
  # to know which one produced the answer.
  runroot="${XDG_RUNTIME_DIR:-}/containers"
  source=environment
fi

# podman's run root is <base>/containers and its libpod tmp is <base>/libpod/tmp,
# so the base is the run root's parent.
base=$(dirname "$runroot")

# RULE 3. Refuse a root that cannot mean what the two paths below need it to.
case "$base" in
  ''|'.'|'/') echo "repair: the engine's run base resolved to '$base', which is refused" >&2; exit 1 ;;
  /*/*) ;;
  *) echo "repair: the engine's run base is '$base', which is not an absolute subdirectory" >&2; exit 1 ;;
esac
case "$base" in
  *..*) echo "repair: the engine's run base contains '..', which is refused" >&2; exit 1 ;;
esac

printf 'run-base %s (from the %s)\n' "$base" "$source"

# RULE 2. Two fixed leaf names, and nothing a caller supplied.
removed=0
failed=0
for leaf in containers libpod/tmp; do
  d="$base/$leaf"
  if [ ! -e "$d" ]; then
    printf 'absent %s\n' "$d"
    continue
  fi
  rm -rf "$d" || true
  # RULE 4. Read it back rather than trusting the exit code of the remover.
  if [ -e "$d" ]; then
    printf 'still-present %s\n' "$d"
    failed=1
  else
    printf 'removed %s\n' "$d"
    removed=$((removed + 1))
  fi
done

if [ "$failed" -ne 0 ]; then
  echo "repair: at least one directory is still there, so nothing has been fixed" >&2
  exit 1
fi
if [ "$removed" -eq 0 ]; then
  # ⛔ NOTHING WAS THERE, SO NOTHING WAS REPAIRED, and this exits non-zero to say
  # so. A repair that finds an empty tree and reports success is the shape that
  # sends a caller away believing a problem is gone.
  #
  # ⚠ MEASURED 2026-09-10: THIS BRANCH RARELY FIRES ON THE ENGINE PATH, because
  # the `podman info` above RECREATES the run root before this loop reaches it.
  # It is the fallback path's guard, and it is kept for the case where podman
  # cannot answer and the environment's directories are not there either.
  echo "repair: found nothing to remove under $base, so the run state was not what was wrong" >&2
  exit 1
fi
# ⛔ THIS TOKEN MEANS A DELETION HAPPENED. It does NOT mean the base works: the
# run root is recreated by the next thing that touches the engine, so removing it
# always succeeds whether or not stale state was the problem. `base ensure`
# re-runs the health probe afterwards and reports THAT, which is the only reading
# that answers the question a caller asked.
printf 'repaired run-state\n'
