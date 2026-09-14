#!/bin/sh
# Bring the base's explicit Windows directory grants to what the configuration
# says: the tool-owned /etc/fstab block, and, when asked, the live mounts.
#
# THE DEFECT IT EXISTS TO REMOVE: a grant that changed only across a restart. A
# restart stops every agent in the base, so adding a project meant stopping the
# work already running there. `base grant` and `base revoke` run this live, and
# provisioning runs it to write the block before the restart that mounts it.
#
# It runs as root, delivered on stdin. TK_USER, TK_GRANTS_MODE, TK_FSTAB_B64 and
# TK_MOUNT_CHECKS arrive as exported variables, and every Windows path in them is
# base64 made by Go, so none of it becomes shell source.
#
#   TK_GRANTS_MODE=write   rewrite the block and create each target, and mount nothing
#   TK_GRANTS_MODE=live    rewrite the block, unmount what it no longer names, mount
#                          what it names, and read every grant back as the account
set -eu
umask 022
cd /

: "${TK_USER:?TK_USER is required}"
: "${TK_GRANTS_MODE:?TK_GRANTS_MODE is required}"
: "${TK_FSTAB_B64?TK_FSTAB_B64 is required, and may be empty}"
: "${TK_MOUNT_CHECKS?TK_MOUNT_CHECKS is required, and may be empty}"

say() { printf '  * %s\n' "$*"; }
die() { printf 'grants: %s\n' "$*" >&2; exit 3; }

case "$TK_GRANTS_MODE" in
  write|live) ;;
  *) die "TK_GRANTS_MODE is $TK_GRANTS_MODE; it must be write or live" ;;
esac

FSTAB_BEGIN='# wsl-toolkit mounts begin'
FSTAB_END='# wsl-toolkit mounts end'
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' 0 1 2 15

# decode64 NAME VALUE: base64 read from a file, so its exit code is not hidden by a
# pipeline, into $WORK/NAME.
decode64() {
  printf '%s' "$2" > "$WORK/$1.b64"
  base64 -d "$WORK/$1.b64" > "$WORK/$1" || die "the $1 payload could not be decoded"
}

# -- the new block ------------------------------------------------------------------
# One marked block is replaced as a unit, so a grant removed from the configuration
# is removed from the file.
: > "$WORK/fstab"
if [ -f /etc/fstab ]; then
  in_block=false
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      "$FSTAB_BEGIN") in_block=true; continue ;;
      "$FSTAB_END")   in_block=false; continue ;;
    esac
    if [ "$in_block" = false ]; then
      printf '%s\n' "$line" >> "$WORK/fstab"
    fi
  done < /etc/fstab
fi
printf '%s\n' "$FSTAB_BEGIN" >> "$WORK/fstab"
if [ -n "$TK_FSTAB_B64" ]; then
  decode64 block "$TK_FSTAB_B64"
  cat "$WORK/block" >> "$WORK/fstab"
fi
printf '%s\n' "$FSTAB_END" >> "$WORK/fstab"

# install_fstab FILE: the whole file, through a rename, so a killed run leaves one.
install_fstab() {
  cp "$1" /etc/fstab.wsl-toolkit.new
  mv /etc/fstab.wsl-toolkit.new /etc/fstab
}

# -- the targets the configuration names ------------------------------------------
: > "$WORK/targets"
# The payload is a validated sequence of base64-target/mode pairs.
# shellcheck disable=SC2086
set -- $TK_MOUNT_CHECKS
while [ "$#" -gt 0 ]; do
  [ "$#" -ge 2 ] || die "the explicit mount check table is incomplete"
  decode64 target "$1"
  printf '%s %s\n' "$2" "$(cat "$WORK/target")" >> "$WORK/targets"
  mkdir -p "$(cat "$WORK/target")"
  chmod 755 "$(cat "$WORK/target")"
  shift 2
done
count=$(grep -c . "$WORK/targets" || :)
say "explicit Windows directories: ${count:-0}"

if [ "$TK_GRANTS_MODE" = write ]; then
  install_fstab "$WORK/fstab"
  printf 'grants-complete\n'
  exit 0
fi

# ⛔ LIVE, THE BLOCK IS WRITTEN ONLY ONCE NOTHING CAN STILL REFUSE THE REMOVALS, and
# a failure after it puts the old block back. Written first, a revoke refused over a
# busy directory left that grant mounted, still in the configuration, and gone from
# the block, so the next restart would have dropped it. Measured on 2026-09-14.
: > "$WORK/fstab.before"
if [ -f /etc/fstab ]; then
  cp /etc/fstab "$WORK/fstab.before"
fi
: > "$WORK/unmounted"

# fail_back MESSAGE: the old block back, what this run unmounted mounted again from
# it, and the reason.
fail_back() {
  install_fstab "$WORK/fstab.before"
  while IFS= read -r again; do
    if [ -n "$again" ]; then
      mount "$again" 2>/dev/null || :
    fi
  done < "$WORK/unmounted"
  die "$1"
}

# live_mode TARGET: the mode a live DrvFS mount at TARGET has, or nothing.
live_mode() {
  while read -r _source live_target live_type live_options _rest; do
    [ "$live_target" = "$1" ] || continue
    case "$live_type:$live_options" in
      drvfs:*|9p:*aname=drvfs*) printf '%s\n' "${live_options%%,*}"; return 0 ;;
    esac
  done < /proc/mounts
  return 0
}

# -- unmount what the configuration no longer names -----------------------------------
# ⛔ READ FROM THE LIVE MOUNTS, NOT FROM THE OLD BLOCK. A block that had drifted from
# what was mounted, as one did on 2026-09-14, answered "nothing to unmount" and the
# revoke exited 0 over a directory that stayed mounted. ⚠ Only under /workspaces,
# where the configuration confines every grant: guest root can mount a Windows path
# anywhere else, and this is not the place that decides about it.
: > "$WORK/live-targets"
while read -r _source live_target live_type live_options _rest; do
  case "$live_type:$live_options:$live_target" in
    drvfs:*:/workspaces/*|9p:*aname=drvfs*:/workspaces/*) printf '%s\n' "$live_target" >> "$WORK/live-targets" ;;
  esac
done < /proc/mounts
while IFS= read -r old; do
  [ -n "$old" ] || continue
  if ! cut -d' ' -f2- "$WORK/targets" | grep -Fxq "$old"; then
    # ⛔ NOT A LAZY UNMOUNT. A process standing in the directory keeps it busy, and
    # detaching the mount under it would leave that process editing a directory
    # nobody can see is still mounted.
    umount "$old" 2>"$WORK/umount.err" ||
      fail_back "$old is still in use, so it stays mounted: $(head -1 "$WORK/umount.err")"
    printf '%s\n' "$old" >> "$WORK/unmounted"
    say "unmounted $old"
  fi
done < "$WORK/live-targets"

install_fstab "$WORK/fstab"

# -- mount what it names, in the mode it names ----------------------------------------
while read -r mode target; do
  [ -n "$target" ] || continue
  current=$(live_mode "$target")
  if [ -n "$current" ] && [ "$current" != "$mode" ]; then
    umount "$target" 2>"$WORK/umount.err" ||
      fail_back "$target is mounted $current, is still in use, and cannot become $mode: $(head -1 "$WORK/umount.err")"
    current=
  fi
  if [ -z "$current" ]; then
    mount "$target" 2>"$WORK/mount.err" || fail_back "$target could not be mounted: $(head -1 "$WORK/mount.err")"
    say "mounted $target $mode"
  fi
done < "$WORK/targets"

# -- read every grant back, as the account --------------------------------------------
while read -r mode target; do
  [ -n "$target" ] || continue
  [ "$(live_mode "$target")" = "$mode" ] || fail_back "$target is not a live $mode Windows mount after mounting it"
  probe="$target/.wsl-toolkit-write-probe.$$"
  # The probe path reaches the account's shell as $1, so no Windows path is ever
  # shell source; the single quotes are what keep it an argument.
  # shellcheck disable=SC2016
  case "$mode" in
    rw)
      runuser -u "$TK_USER" -- sh -c ': > "$1" && rm -f "$1"' sh "$probe" ||
        fail_back "$TK_USER cannot write in $target"
      ;;
    ro)
      if runuser -u "$TK_USER" -- sh -c ': > "$1"' sh "$probe" 2>/dev/null; then
        rm -f "$probe"
        fail_back "$target is writable by $TK_USER and granted read-only"
      fi
      ;;
  esac
  printf 'grant %s %s\n' "$mode" "$target"
done < "$WORK/targets"
printf 'grants-complete\n'
