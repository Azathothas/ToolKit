#!/bin/sh
# mount-helper.sh puts WSL's mount.drvfs where BusyBox's mount finds it, and
# makes it hand on the `ro` that BusyBox drops. grants.sh runs this before it
# writes the fstab block. engines.sh runs it at every owner's `base ensure`, so a
# base an earlier release granted gets it too. Each runs it as root, handed it as
# TK_MOUNT_HELPER_B64. WSL-198.
#
# ⛔ WSL RUNS /bin/mount -a WITH NO PATH AT A START. BusyBox's mount looks for
# mount.drvfs on PATH, and musl's default path has no /sbin, where WSL keeps it.
# Measured on alpine: every grant failed with `No such device`, so a restart
# mounted none. WSL also printed that mount -a failed on every call into the base.
# musl's default path starts with /usr/local/bin, so the helper goes there.
#
# ⛔ AND BUSYBOX HANDS THE HELPER NO ro. Measured on alpine: it ran
# `mount.drvfs SOURCE TARGET -o metadata,uid=1000,...`, and a read-only grant came
# up writable. WSL's helper keeps an `ro` it is handed, also measured. So the one
# written here hands on the `ro` of the target's fstab line. A read-only grant is
# then read-only from its mount, inside WSL's own mount -a. A remount after the
# start left a window in which the grant was writable.
#
# ⚠ ONLY WHERE /bin/mount IS BUSYBOX'S. util-linux's mount finds the helper in /sbin
# itself, and hands it `ro`.
#
# ⚠ A FILE THERE THAT THIS TOOL DID NOT WRITE IS LEFT AS IT IS, and named. A link to
# WSL's helper is replaced, since it finds the helper and still drops the `ro`.
#
# It says nothing where the helper is already in place. It exits 0 even where it
# could not write the helper, so a grant made now is not refused for it. A restart
# then mounts no grant, and `base ensure` names each grant that is not mounted.
set -u
HELPER=/sbin/mount.drvfs
WRAPPER=/usr/local/bin/mount.drvfs
MOUNT_BIN=/bin/mount

say() { printf '  * %s\n' "$*"; }

[ -e "$HELPER" ] || exit 0
case "$(readlink -f "$MOUNT_BIN" 2>/dev/null)" in
  */busybox) ;;
  *) exit 0 ;;
esac

# The second line of a helper this tool wrote.
MARK='# Written by wsl-toolkit.'
if [ -L "$WRAPPER" ]; then
  if [ "$(readlink "$WRAPPER" 2>/dev/null)" != "$HELPER" ]; then
    say "⚠ mount helper: $WRAPPER links elsewhere, so it was left as it is. A restart may mount no grant"
    exit 0
  fi
elif [ -e "$WRAPPER" ]; then
  mh_second=
  { read -r _mh_first && read -r mh_second; } < "$WRAPPER" 2>/dev/null || :
  if [ "$mh_second" != "$MARK" ]; then
    say "⚠ mount helper: $WRAPPER is not this tool's, so it was left as it is. A read-only grant may come up writable at a start"
    exit 0
  fi
fi

if ! mkdir -p "${WRAPPER%/*}" 2>/dev/null || ! tmp=$(mktemp "$WRAPPER.XXXXXX" 2>/dev/null); then
  say "⚠ mount helper: no temporary file beside $WRAPPER, so a restart mounts no grant until base ensure can write it"
  exit 0
fi
cat > "$tmp" <<'WSL_TOOLKIT_MOUNT_HELPER'
#!/bin/sh
# Written by wsl-toolkit.
# WSL's mount.drvfs, where BusyBox's mount looks for it, handed the `ro` that
# BusyBox drops. wsl-toolkit writes this where /bin/mount is BusyBox's. WSL-198.
# >>> the mount helper: begin
PATH=/usr/sbin:/usr/bin:/sbin:/bin
export PATH
helper=/sbin/mount.drvfs
fstab=/etc/fstab
mounts=/proc/mounts
# The target is the second word that is neither an option nor an option's value.
mh_target=
mh_words=0
mh_value=false
for mh_a in "$@"; do
  if [ "$mh_value" = true ]; then
    mh_value=false
    continue
  fi
  case "$mh_a" in
    -o) mh_value=true ;;
    -*) ;;
    *)
      mh_words=$((mh_words + 1))
      [ "$mh_words" != 2 ] || mh_target=$mh_a
      ;;
  esac
done
# ⛔ READ-ONLY WHERE THE TARGET'S drvfs LINE IN fstab SAYS ro. BusyBox drops the
# `ro` before it runs this, measured on alpine. The last line for the target wins.
mh_want=
if [ -n "$mh_target" ] && [ -r "$fstab" ]; then
  while read -r mh_from mh_at mh_type mh_options _mh_rest; do
    case "$mh_from" in '#'*|'') continue ;; esac
    if [ "$mh_at" = "$mh_target" ] && [ "$mh_type" = drvfs ]; then
      case ",$mh_options," in *,ro,*) mh_want=ro ;; *) mh_want= ;; esac
    fi
  done < "$fstab"
fi
[ "$mh_want" = ro ] || exec "$helper" "$@"
# ⛔ THE HELPER IS HANDED ro FIRST AMONG ITS OPTIONS, so the mount is read-only from
# the start. Measured on alpine: WSL's helper keeps it.
mh_left=$#
mh_given=false
while [ "$mh_left" -gt 0 ]; do
  mh_a=$1
  shift
  mh_left=$((mh_left - 1))
  if [ "$mh_a" = -o ] && [ "$mh_left" -gt 0 ]; then
    set -- "$@" -o "ro,$1"
    shift
    mh_left=$((mh_left - 1))
    mh_given=true
  else
    set -- "$@" "$mh_a"
  fi
done
[ "$mh_given" = true ] || set -- "$@" -o ro
"$helper" "$@" || exit
# mh_live sets mh_mode to the first option of the last Windows mount at the
# target, ro or rw. The mount on top is the last row.
mh_live() {
  mh_mode=
  while read -r _mh_from mh_at mh_type mh_options _mh_rest; do
    [ "$mh_at" = "$mh_target" ] || continue
    case "$mh_type:$mh_options" in
      drvfs:*|9p:*aname=drvfs*) mh_mode=${mh_options%%,*} ;;
    esac
  done < "$mounts"
}
# ⛔ READ BACK. A MOUNT LEFT WRITABLE IS MADE READ-ONLY, OR IT IS TAKEN AWAY. A plain
# remount makes the filesystem read-only, so a bind of it made since goes too. A
# bind remount covers its own mount point alone, measured on alpine. So it is
# asked only where the plain one fails. One that stays writable is unmounted, and
# the mount fails.
mh_live
[ "$mh_mode" = rw ] || exit 0
mount -o remount,ro "$mh_target" 2>/dev/null || mount -o remount,bind,ro "$mh_target" 2>/dev/null || :
mh_live
[ "$mh_mode" = rw ] || exit 0
if umount "$mh_target" 2>/dev/null; then
  mh_gone=unmounted
elif umount -l "$mh_target" 2>/dev/null; then
  mh_gone="detached, since it was busy"
else
  mh_gone="NOT unmounted, and it is still writable"
fi
printf 'mount.drvfs: %s came up writable and would not go read-only, so it was %s\n' "$mh_target" "$mh_gone" >&2
exit 32
# <<< the mount helper: end
WSL_TOOLKIT_MOUNT_HELPER

if [ -f "$WRAPPER" ] && [ ! -L "$WRAPPER" ] && [ "$(cat "$tmp")" = "$(cat "$WRAPPER")" ]; then
  rm -f "$tmp"
  exit 0
fi
if chmod 0755 "$tmp" && mv -f "$tmp" "$WRAPPER"; then
  say "mount helper: wrote $WRAPPER, so WSL's mount -a finds $HELPER and hands it ro"
else
  rm -f "$tmp"
  say "⚠ mount helper: $WRAPPER could not be written, so a restart mounts no grant until base ensure can write it"
fi
exit 0
