#!/bin/sh
# Bring the base's explicit Windows directory grants to what the configuration
# says: the tool-owned /etc/fstab block, and, when asked, the live mounts.
#
# THE DEFECT IT EXISTS TO REMOVE: a grant that changed only across a restart. A
# restart stops every agent in the base, so adding a project meant stopping the
# work already running there. `base grant` and `base revoke` run this live, and
# provisioning runs it to write the block before the restart that mounts it.
#
# It runs as root, delivered on stdin. TK_USER, TK_GRANTS_MODE, TK_FSTAB_B64,
# TK_MOUNT_CHECKS, TK_GRANT_TARGET, TK_GRANT_SOURCE, TK_GRANT_OWNER and
# TK_GRANT_OWNER_NAME arrive as exported variables, and every path in them is
# base64 made by Go, so none of it becomes shell source.
#
# The owners ledger, /var/lib/wsl-toolkit/grant-owners, says which configuration
# file made each live grant: one line of target, owner key and owner name, the two
# in base64. verify.sh reads it. WSL-179.
#
#   TK_GRANTS_MODE=write   rewrite the block and create each target, and mount nothing
#   TK_GRANTS_MODE=live    rewrite the block, unmount or mount TK_GRANT_TARGET alone,
#                          as the block names it, and read it back as the account
#   TK_GRANTS_MODE=block   rewrite the block alone: no directory made, no mount changed
#   TK_GRANTS_MODE=held    change nothing, and name each live mount the ledger gives
#                          another configuration file
set -eu
umask 022
cd /

: "${TK_USER:?TK_USER is required}"
: "${TK_GRANTS_MODE:?TK_GRANTS_MODE is required}"
: "${TK_FSTAB_B64?TK_FSTAB_B64 is required, and may be empty}"
: "${TK_MOUNT_CHECKS?TK_MOUNT_CHECKS is required, and may be empty}"
: "${TK_GRANT_TARGET:=}"
: "${TK_GRANT_SOURCE:=}"
: "${TK_GRANT_OWNER:=}"
: "${TK_GRANT_OWNER_NAME:=}"

say() { printf '  * %s\n' "$*"; }
die() { printf 'grants: %s\n' "$*" >&2; exit 3; }

case "$TK_GRANTS_MODE" in
  write|live|block|held) ;;
  *) die "TK_GRANTS_MODE is $TK_GRANTS_MODE; it must be write, live, block or held" ;;
esac

FSTAB_BEGIN='# wsl-toolkit mounts begin'
FSTAB_END='# wsl-toolkit mounts end'
OWNERS=/var/lib/wsl-toolkit/grant-owners
LOCK=/var/lib/wsl-toolkit/grants.lock
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' 0 1 2 15

# ⛔ ONE RUN AT A TIME IN THE GUEST, WHICHEVER HOST ROUTE IT CAME BY. The host's
# grant lock lives under the state root, and the WSL-118 package copy and a
# --home split each resolve another one. Two grants through two routes each wrote
# a ledger without the other's entry, and that mount became unconfigured. ⚠ Not
# `flock -w`: BusyBox's flock takes no timeout, so the wait is polled with -n, as
# repair.sh does. A base with no flock runs as it did before the lock. WSL-179.
if command -v flock >/dev/null 2>&1; then
  mkdir -p "${LOCK%/*}"
  exec 9>>"$LOCK"
  tk_waited=0
  until flock -n 9 2>/dev/null; do
    [ "$tk_waited" -lt 120 ] ||
      die "another grant held the base's grant lock for 120s, so nothing was changed. Run this again once it has finished"
    sleep 1
    tk_waited=$((tk_waited + 1))
  done
fi

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

# install_owners FILE: the owners ledger, through a rename, readable by the account
# verify.sh runs as.
install_owners() {
  mkdir -p /var/lib/wsl-toolkit
  cp "$1" "$OWNERS.new"
  chmod 644 "$OWNERS.new"
  mv "$OWNERS.new" "$OWNERS"
}

# ⛔ BLOCK, NOTHING LIVE IS TOUCHED. It takes back the block of a live change that
# mounted and unmounted nothing, because its target was already as asked. A live
# take-back unmounted that target, and another configuration had mounted it. WSL-179.
if [ "$TK_GRANTS_MODE" = block ]; then
  install_fstab "$WORK/fstab"
  printf 'grants-complete\n'
  exit 0
fi

# ⭐ HELD, NOTHING IS CHANGED. It names each live DrvFS mount the ledger gives a
# file other than TK_GRANT_OWNER, one `held-mount TARGET FILE64` row each, for
# `base ensure` to refuse the restart that would drop them. It reads under the
# lock, so a grant still running is read once it has finished. WSL-179.
if [ "$TK_GRANTS_MODE" = held ]; then
  if [ -f "$OWNERS" ]; then
    while read -r held_target held_key held_name _rest; do
      if [ -z "$held_target" ] || [ "$held_key" = "$TK_GRANT_OWNER" ]; then
        continue
      fi
      while read -r _source live_target live_type live_options _live_rest; do
        [ "$live_target" = "$held_target" ] || continue
        case "$live_type:$live_options" in
          drvfs:*|9p:*aname=drvfs*) printf 'held-mount %s %s\n' "$held_target" "$held_name"; break ;;
        esac
      done < /proc/mounts
    done < "$OWNERS"
  fi
  printf 'grants-complete\n'
  exit 0
fi

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
  # ⭐ THE RESTART THAT FOLLOWS MOUNTS THIS BLOCK ALONE, so the ledger becomes this
  # configuration's grants. Another configuration's live grant ends with it. WSL-179.
  if [ -n "$TK_GRANT_OWNER" ]; then
    : > "$WORK/owners"
    while read -r _mode target; do
      if [ -n "$target" ]; then
        printf '%s %s %s\n' "$target" "$TK_GRANT_OWNER" "$TK_GRANT_OWNER_NAME" >> "$WORK/owners"
      fi
    done < "$WORK/targets"
    install_owners "$WORK/owners"
  fi
  printf 'grants-complete\n'
  exit 0
fi

# ⛔ LIVE, ONE TARGET IS CHANGED: the grant or the revoke this call makes. Another
# caller's configuration can hold a grant this one does not name, and a sweep of
# every unnamed /workspaces mount unmounted that caller's checkout. WSL-179.
[ -n "$TK_GRANT_TARGET" ] || die "TK_GRANT_TARGET is required in live mode"
decode64 own "$TK_GRANT_TARGET"
OWN=$(cat "$WORK/own")
case "$OWN" in
  /workspaces/?*) ;;
  *) die "the live target $OWN is not under /workspaces" ;;
esac
# GRANT_FROM is the Windows directory this call's grant names, as fstab escapes
# it: the grant's own for a grant, and the one taken away for a revoke.
GRANT_FROM=
if [ -n "$TK_GRANT_SOURCE" ]; then
  decode64 grant_from "$TK_GRANT_SOURCE"
  GRANT_FROM=$(cat "$WORK/grant_from")
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
# changed is whether this run unmounted or mounted its target. ⛔ A run that did
# neither says so, and its take-back then leaves the target alone: another
# configuration may hold that mount. WSL-179.
changed=false

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

# live_source TARGET: the Windows directory a live DrvFS mount at TARGET comes
# from, as /proc/mounts escapes it, or nothing. Measured on 2026-10-24: a mount
# from the block reads `D:/src/A\040Dir/ToolKit`, the fstab field unchanged.
live_source() {
  while read -r live_from live_target live_type live_options _rest; do
    [ "$live_target" = "$1" ] || continue
    case "$live_type:$live_options" in
      drvfs:*|9p:*aname=drvfs*) printf '%s\n' "$live_from"; return 0 ;;
    esac
  done < /proc/mounts
  return 0
}

# block_source TARGET: the Windows directory the new block mounts at TARGET, as
# fstab escapes it, or nothing.
block_source() {
  [ -f "$WORK/block" ] || return 0
  while read -r block_from block_target _rest; do
    if [ "$block_target" = "$1" ]; then
      printf '%s\n' "$block_from"
      return 0
    fi
  done < "$WORK/block"
  return 0
}

# source_key PATH: an escaped Windows path in one spelling. fstab and /proc/mounts
# escape differently, a separator can be either slash, and Windows compares
# without case.
source_key() {
  printf '%s\n' "$1" |
    sed -e 's/\\134/\//g' -e 's/\\043/#/g' -e 's/\\040/ /g' -e 's/\\011/	/g' \
      -e 's/\\/\//g' -e 's/\/*$//' |
    tr '[:upper:]' '[:lower:]'
}

# holder TARGET: the owner key and owner name the ledger gives TARGET, or nothing.
holder() {
  [ -f "$OWNERS" ] || return 0
  while read -r held_target held_key held_name _rest; do
    if [ "$held_target" = "$1" ]; then
      printf '%s %s\n' "$held_key" "$held_name"
      return 0
    fi
  done < "$OWNERS"
  return 0
}

# ⛔ A LIVE TARGET ANOTHER CONFIGURATION HOLDS IS REFUSED, NOT CHANGED. The ledger
# names the file that granted it, and a grant or a revoke from this one unmounted
# or replaced that file's checkout. The refusal names the file. WSL-179.
held=$(holder "$OWN")
held_key=${held%% *}
if [ -n "$held" ] && [ -n "$TK_GRANT_OWNER" ] && [ "$held_key" != "$TK_GRANT_OWNER" ] &&
  [ -n "$(live_mode "$OWN")" ]; then
  decode64 held_name "${held#* }"
  die "$OWN is mounted for $(cat "$WORK/held_name"), another configuration, so it stays as it is. Revoke it with that file first: wsl-toolkit --config $(cat "$WORK/held_name") base revoke --target $OWN"
fi

# -- unmount this call's target, when the configuration no longer names it -----------
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
  [ "$old" = "$OWN" ] || continue
  if ! cut -d' ' -f2- "$WORK/targets" | grep -Fxq "$old"; then
    # ⛔ ONLY A MOUNT FROM THE DIRECTORY THIS CONFIGURATION GRANTED IS UNMOUNTED.
    # The ledger refusal above needs an entry. A mount made before the ledger has
    # none, and one a restart mounted from another file's block has a stale one,
    # and a revoke then unmounted that other file's checkout. WSL-179.
    old_from=$(live_source "$old")
    if [ -z "$GRANT_FROM" ] || [ "$(source_key "$old_from")" != "$(source_key "$GRANT_FROM")" ]; then
      fail_back "$old is mounted from $old_from, not from ${GRANT_FROM:-the directory this configuration granted}, so it stays mounted. Revoke it with the file that granted it"
    fi
    # ⛔ NOT A LAZY UNMOUNT. A process standing in the directory keeps it busy, and
    # detaching the mount under it would leave that process editing a directory
    # nobody can see is still mounted.
    umount "$old" 2>"$WORK/umount.err" ||
      fail_back "$old is still in use, so it stays mounted: $(head -1 "$WORK/umount.err")"
    printf '%s\n' "$old" >> "$WORK/unmounted"
    changed=true
    say "unmounted $old"
  fi
done < "$WORK/live-targets"

install_fstab "$WORK/fstab"

# -- mount this call's target, when the configuration names it ------------------------
while read -r mode target; do
  [ -n "$target" ] || continue
  [ "$target" = "$OWN" ] || continue
  current=$(live_mode "$target")
  # ⛔ A LIVE MOUNT IS "AS ASKED" ONLY FROM THE SAME DIRECTORY. The mode alone was
  # compared, so a grant over another configuration's checkout of the same name
  # said "unchanged", and its agent wrote into that checkout. WSL-179.
  if [ -n "$current" ]; then
    live_from=$(live_source "$target")
    want_from=$(block_source "$target")
    [ "$(source_key "$live_from")" = "$(source_key "$want_from")" ] ||
      fail_back "$target is already mounted from $live_from, not from $want_from, so it stays as it is. Revoke it where it was granted first"
  fi
  # ⛔ A MOUNT ALREADY AS ASKED IS NOT CLAIMED UNLESS THE LEDGER GIVES IT THIS FILE.
  # A mount made before the ledger has no entry. This file was recorded as its
  # owner, the file that made it was refused its own revoke, and this file's
  # revoke unmounted that file's checkout. WSL-179.
  if [ -n "$current" ] && [ "$current" = "$mode" ] &&
    { [ -z "$TK_GRANT_OWNER" ] || [ "$held_key" != "$TK_GRANT_OWNER" ]; }; then
    fail_back "$target is already mounted $current from $live_from, and the grant ledger does not say this configuration mounted it, so it stays as it is. Revoke it with the file that granted it, or grant this directory at another --target"
  fi
  if [ -n "$current" ] && [ "$current" != "$mode" ]; then
    # ⛔ ONLY THIS CONFIGURATION'S OWN MOUNT IS REMOUNTED. A mount in another mode
    # that the ledger does not give this file was unmounted and replaced. WSL-179.
    if [ -z "$TK_GRANT_OWNER" ] || [ "$held_key" != "$TK_GRANT_OWNER" ]; then
      fail_back "$target is already mounted $current, not $mode, and this configuration did not mount it, so it stays as it is. Revoke it where it was granted first"
    fi
    umount "$target" 2>"$WORK/umount.err" ||
      fail_back "$target is mounted $current, is still in use, and cannot become $mode: $(head -1 "$WORK/umount.err")"
    # ⛔ RECORDED, so a mount that then fails puts this one back from the old block.
    # Unrecorded, a failed switch left the target unmounted. WSL-179.
    printf '%s\n' "$target" >> "$WORK/unmounted"
    changed=true
    current=
  fi
  if [ -z "$current" ]; then
    mount "$target" 2>"$WORK/mount.err" || fail_back "$target could not be mounted: $(head -1 "$WORK/mount.err")"
    changed=true
    say "mounted $target $mode"
  fi
done < "$WORK/targets"

# -- read this call's grant back, as the account ---------------------------------------
while read -r mode target; do
  [ -n "$target" ] || continue
  [ "$target" = "$OWN" ] || continue
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
# -- the ledger: this call's target, as this configuration now has it -----------------
# ⭐ WRITTEN LAST, once nothing can still fail back, so it never names a grant the
# base does not hold. WSL-179.
if [ -n "$TK_GRANT_OWNER" ]; then
  : > "$WORK/owners"
  if [ -f "$OWNERS" ]; then
    while IFS= read -r line; do
      case "$line" in
        "$OWN "*|'') ;;
        *) printf '%s\n' "$line" >> "$WORK/owners" ;;
      esac
    done < "$OWNERS"
  fi
  if cut -d' ' -f2- "$WORK/targets" | grep -Fxq "$OWN"; then
    printf '%s %s %s\n' "$OWN" "$TK_GRANT_OWNER" "$TK_GRANT_OWNER_NAME" >> "$WORK/owners"
  fi
  install_owners "$WORK/owners"
fi
if [ "$changed" = false ]; then
  printf 'unchanged %s\n' "$OWN"
fi
printf 'grants-complete\n'
