#!/bin/sh
# Clear the engine's run state when, and only when, a restart of WSL left it
# stale. `run`, `matrix` and `base exec` send this before their first container
# or engine call. WSL-137. Each start of the base runs it too, and it tells a
# restart of the distribution alone from a state of this start. WSL-178.
#
# It runs as TK_USER inside the owned distribution, after the runtime prologue
# has set XDG_RUNTIME_DIR, and repair.sh follows it in the same payload. Where
# the state is current this exits 0 before repair.sh is reached.
#
# ⛔ WHY IT IS AUTOMATIC HERE AND OPT IN FOR `base ensure`. `wsl --shutdown` and
# a Windows restart start a new boot of WSL, and podman then refuses every
# container with `current system boot ID differs from cached boot ID` in every
# base that holds an engine. Only `base ensure --repair` cleared it, one base at
# a time, so every job after a restart failed until somebody ran that by hand.
# This removes nothing on a guess. It acts only on the engine's OWN record: the
# boot id podman wrote into its alive file, read beside the one the kernel holds
# now. That is the exact condition podman refuses over and prescribes the
# deletion for.
#
# ⛔ NO ENGINE OF THIS BOOT IS LIVE ON ANOTHER BOOT'S RECORD. podman reads the
# same record before it starts anything and refuses while it names another boot,
# so while it does, nothing of this boot is running on the state removed here.
# ⚠ An earlier start's record on this boot is not refused, so repair.sh keeps it
# while a container of this start runs on it. WSL-178.
#
# ⛔ THE CHECK IS MADE AGAIN UNDER THE LOCK `base ensure --repair` TAKES, and
# repair.sh takes it and makes it. A matrix starts its rows at once, and without
# the lock a second row could clear the state a first row's container had just
# started on. A caller that waited finds the record gone, or rewritten for this
# boot, and removes nothing.
set -eu

tk_stale_root=${XDG_RUNTIME_DIR:-}
tk_stale_alive=$tk_stale_root/libpod/tmp/alive
# ⭐ THE START OF THE BASE THE STATE WAS LAST HELD TO, beside the lock repair.sh
# takes. bootmark, which the payload defines before this text, names a start:
# the kernel's boot id and when the distribution's init started. WSL-178.
tk_stale_mark=$tk_stale_root/.wsl-toolkit-run-state.start
tk_stale_now=$(bootmark 2>/dev/null) || tk_stale_now=

# ⚠ An absent record, an empty one and one naming this boot all read as current.
# podman writes the record itself when it starts on no record, and a record this
# cannot read as another boot's is never a reason to delete.
stale_boot_id() {
  [ -n "$tk_stale_root" ] || return 1
  [ -s "$tk_stale_alive" ] || return 1
  sbi_now=$(cat /proc/sys/kernel/random/boot_id 2>/dev/null) || return 1
  [ -n "$sbi_now" ] || return 1
  sbi_cached=$(cat "$tk_stale_alive" 2>/dev/null) || return 1
  [ -n "$sbi_cached" ] || return 1
  [ "$sbi_cached" != "$sbi_now" ]
}

# ⛔ A RESTART OF THE DISTRIBUTION ALONE KEEPS THE BOOT ID. `wsl --terminate`, an
# idle stop of the distribution and this tool's own restart after provisioning
# keep WSL's VM, so podman's record still names this boot. podman then reads the
# dead start's containers, its pause process and its network as live. A record
# held to another start of the base is that start's state. WSL-178.
#
# ⚠ NO MARK IS NOT STALE. A state an older build left, or one no check has seen
# yet, is taken as this start's and held to it, so the next start can tell.
stale_start() {
  [ -n "$tk_stale_root" ] || return 1
  [ -s "$tk_stale_alive" ] || return 1
  [ -n "$tk_stale_now" ] || return 1
  ss_was=$(cat "$tk_stale_mark" 2>/dev/null) || return 1
  [ -n "$ss_was" ] && [ "$ss_was" != "$tk_stale_now" ]
}

if stale_boot_id; then
  printf 'run-state stale: the engine recorded another boot of WSL\n'
elif stale_start; then
  printf 'run-state stale: the engine recorded an earlier start of this base\n'
else
  # ⭐ A CURRENT STATE IS HELD TO THIS START. A mark that cannot be written is
  # removed, so a later check takes the state as current and never clears it on
  # a mark this start failed to move.
  if [ -n "$tk_stale_root" ] && [ -n "$tk_stale_now" ] &&
    [ "$(cat "$tk_stale_mark" 2>/dev/null || :)" != "$tk_stale_now" ]; then
    { printf '%s\n' "$tk_stale_now" > "$tk_stale_mark.$$" && mv -f "$tk_stale_mark.$$" "$tk_stale_mark"; } 2>/dev/null ||
      rm -f "$tk_stale_mark.$$" "$tk_stale_mark" 2>/dev/null || :
  fi
  printf 'run-state current\n'
  exit 0
fi

# ⛔ NO LOCK, NO DELETION. A base without flock keeps the state, and `base
# ensure --repair` is still the way through. ⚠ BusyBox's flock counts: repair.sh
# waits with `flock -n`, which it takes, and not with -w, which it does not.
if ! command -v flock >/dev/null 2>&1; then
  printf 'run-state kept: this base has no flock to clear it safely\n'
  exit 0
fi

# repair.sh refuses to remove anything outside this root. Its own rule 1 asks
# podman where its run state is, and its rule 5 holds the two to agree. ⚠ It is
# read by repair.sh, which follows in the same shell, and nothing here reads it.
# shellcheck disable=SC2034
TK_STALE_BASE=$tk_stale_root
# repair.sh follows in the same payload, and its last line is `repaired run-state`.
