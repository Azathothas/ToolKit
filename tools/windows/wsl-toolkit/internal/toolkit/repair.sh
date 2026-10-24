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
# ⚠ THE ONE OTHER WAY IN IS stale.sh, which `run`, `matrix` and `base exec` send
# ahead of this file, and it reaches here only when the engine's own alive record
# names another boot of WSL. It sets TK_STALE_BASE, and rule 5 holds the deletion
# to the root that record was read under. WSL-137.
#
# ⛔ BOTH WAYS IN TAKE ONE LOCK, HERE, AND READ THE RECORD AGAIN UNDER IT before
# anything is removed. Each decided to clear before it held the lock, so a caller
# that cleared first and started an engine has left a record naming this boot,
# and that record stops the second clear. WSL-137.
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

# THE LOCK, which both ways in take here. A base with no flock clears as it did
# before the lock existed, and stale.sh never reaches here without one. WSL-137.
#
# ⛔ NOT `flock -w`. BusyBox's flock, the only one the alpine preset carries,
# takes -s -x -u and -n and no timeout. `-w 120` failed there at once, so
# --repair cleared nothing and blamed a lock holder that did not exist. Both
# flocks take -n, so the 120s wait is polled with it.
if [ -n "${XDG_RUNTIME_DIR:-}" ] && command -v flock >/dev/null 2>&1; then
  exec 9>>"$XDG_RUNTIME_DIR/.wsl-toolkit-run-state.lock"
  tk_waited=0
  until flock -n 9 2>/dev/null; do
    if [ "$tk_waited" -ge 120 ]; then
      echo "repair: the run-state lock was not free within 120s, so nothing was removed" >&2
      exit 1
    fi
    sleep 1
    tk_waited=$((tk_waited + 1))
  done

  # ⛔ THE SECOND READ, UNDER THE LOCK, ON BOTH WAYS IN. A caller that held the
  # lock meanwhile may have cleared the state and started a container, and podman
  # then wrote a record naming this boot. Nothing is removed under that engine.
  tk_record=missing
  if [ -s "$XDG_RUNTIME_DIR/libpod/tmp/alive" ]; then
    tk_cached=$(cat "$XDG_RUNTIME_DIR/libpod/tmp/alive" 2>/dev/null || :)
    tk_now=$(cat /proc/sys/kernel/random/boot_id 2>/dev/null || :)
    if [ -n "$tk_cached" ] && [ "$tk_cached" = "$tk_now" ]; then
      tk_record=this-boot
    elif [ -n "$tk_cached" ] && [ -n "$tk_now" ]; then
      tk_record=other-boot
    fi
  fi
  # ⚠ stale.sh proved another boot's record, so anything else is now another
  # caller's clear. `base ensure` proved nothing about the record, so only one
  # naming this boot stops it, and a missing one clears as it did in 7.0.0.
  if [ "$tk_record" = this-boot ]; then
    printf 'run-state current: another caller cleared it first\n'
    exit 0
  fi
  if [ -n "${TK_STALE_BASE:-}" ] && [ "$tk_record" != other-boot ]; then
    printf 'run-state current: another caller cleared it first\n'
    exit 0
  fi
fi

# RULE 1. Ask the engine. ⚠ `podman info` still answers in the stale-boot-id
# state, which is the only state this script runs in: podman refuses to RUN a
# container and remains happy to describe itself.
#
# ⛔ IT IS NOT HANDED THE LOCK. A rootless engine can start a process that
# outlives this call, and one that held descriptor 9 would keep the lock after
# this script ended, so every later caller would wait 120s and remove nothing.
#
# ⛔ IT IS BOUNDED, because this shell holds the lock while it asks. A hung
# `podman info` outlived the 3-minute budget of the call that sent this, which
# kills wsl.exe and not this shell, and every later job waited 120s on the lock
# it held. A question that is not answered in 60s falls to the named fallback.
# WSL-137.
if command -v timeout >/dev/null 2>&1; then
  runroot=$(timeout 60 podman info --format '{{.Store.RunRoot}}' 9>&- 2>/dev/null || echo '')
else
  runroot=$(podman info --format '{{.Store.RunRoot}}' 9>&- 2>/dev/null || echo '')
fi
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

# RULE 5, for stale.sh alone. ⛔ The stale record was read under one root and the
# engine named this one, so a disagreement removes nothing: the state under the
# engine's root was never shown to be stale. WSL-137.
if [ -n "${TK_STALE_BASE:-}" ] && [ "$base" != "$TK_STALE_BASE" ]; then
  echo "repair: the engine's run base is $base and the stale boot id was read under $TK_STALE_BASE, so nothing was removed" >&2
  exit 1
fi

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
    # ⛔ THE CLEAR STOPS AT THE FIRST DIRECTORY STILL THERE. libpod/tmp holds the
    # stale record, and removing it under a half-removed run root would read as
    # current to every later check while podman refreshed over the remains. So
    # the record survives, and the next check still names the state. WSL-137.
    break
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
