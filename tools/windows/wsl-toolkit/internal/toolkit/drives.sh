#!/bin/sh
# Remove the empty drive mount points a base with `automount off` still carries.
#
# It runs as root, delivered on stdin, after the restart that applies the
# provisioner's /etc/wsl.conf and before the verifier reads the drives back.
#
# ⛔ AN EMPTY /mnt/c IS NOT AN ABSENT ONE. The first start of a fresh import
# automounts every Windows drive before provisioning has written `automount off`,
# and the restart that applies the setting leaves each mount point behind as an
# empty 0777 directory. Nothing is reachable through them, but `ls /mnt/c`
# succeeds, the host's drive letters are listed, and guest root has a place ready
# to mount one. Measured on a rebuilt Arch base on 2026-09-13: nine of them.
#
# ⛔ AFTER THE RESTART, AND ONLY THERE. This sweep ran inside provision.sh, before
# the restart, and two things followed from that:
#
#   - a drive WSL mounted after the sweep left its mount point, and the verifier
#     then refused the base: a fedora build removed nine of this host's ten and
#     met /mnt/e. Finding 18, WSL-155.
#   - every session started between the sweep and the restart asked WSL to
#     translate each Windows PATH entry onto a drive it had just unmounted, so
#     the grants step relayed one `wsl: Failed to translate` line per entry, 93 of
#     them on this host. Finding 13, WSL-153.
#
# After the restart `automount off` and `appendWindowsPath=false` are both in
# force, so nothing is mounted under the sweep and nothing is translated.
#
# ⚠ Only a single-letter directory, only when it is empty, and a live drive mount
# is unmounted first, in case a start did not apply the setting.
set -eu
cd /

die() { printf 'drives: %s\n' "$*" >&2; exit 3; }

drive_points=0
for drive_dir in /mnt/?; do
  [ -d "$drive_dir" ] || continue
  while read -r _source live_target live_type live_options _rest; do
    [ "$live_target" = "$drive_dir" ] || continue
    case "$live_type:$live_options" in
      drvfs:*|9p:*aname=drvfs*)
        umount "$drive_dir" 2>/dev/null || umount -l "$drive_dir" ||
          die "the automounted drive at $drive_dir could not be unmounted"
        ;;
    esac
  done < /proc/mounts
  rmdir "$drive_dir" 2>/dev/null || die "$drive_dir is not an empty directory, so it was left and automount off cannot be true"
  drive_points=$((drive_points + 1))
done
printf '  * drive mount points removed: %s\n' "$drive_points"
printf 'drives-complete\n'
