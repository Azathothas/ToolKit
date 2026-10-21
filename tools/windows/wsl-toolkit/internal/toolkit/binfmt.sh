#!/bin/sh
# binfmt.sh: the QEMU handlers this base registers in the WSL kernel, and the
# watcher that registers them again when another distribution removes them.
#
# ⛔ THE TABLE IS NOT THIS BASE'S. `binfmt_misc` is one table for every
# distribution in the WSL kernel, and a distribution that stops can empty it:
# the Podman machine's shutdown unregisters every entry, this base's included,
# while the VM and this base keep running. WSL-105, issue 37.
#
# It runs as root, delivered on stdin by `base ensure`, which passes the image as
# TK_BINFMT_IMAGE, whether the base runs systemd as TK_SYSTEMD, and this script's
# own bytes as TK_SELF_B64. With `watch` as its first argument it is the
# watcher's body: the copy the ensure installed, run by systemd.
#
# ⭐ THE BASE'S OWN SET IS DERIVED, NOT TYPED: the emulators the image carries
# that the kernel holds after the install. The image carries qemu-i386, and on
# an amd64 host the installer leaves it out because the kernel runs 386 itself,
# so a table typed here would claim a handler nothing registers.
set -u

DIR=/proc/sys/fs/binfmt_misc
RECORD=/var/lib/wsl-toolkit/binfmt-handlers
SELF=/usr/local/lib/wsl-toolkit/binfmt.sh
UNIT=wsl-toolkit-binfmt.service

: "${TK_BINFMT_IMAGE:?TK_BINFMT_IMAGE is required}"

say() { printf '  * %s\n' "$*"; }
die() { printf 'binfmt: %s\n' "$*" >&2; exit 3; }
# same_file A B is true when B exists and holds A's bytes. ⚠ NOT cmp: cmp is
# diffutils, which a base with no toolset does not carry, and a missing cmp read
# as "the files differ" on every run. WSL-105.
same_file() {
  [ -f "$2" ] || return 1
  [ "$(sha256sum < "$1")" = "$(sha256sum < "$2")" ]
}

# missing prints each recorded handler the kernel does not hold, one per line.
missing() {
  [ -r "$RECORD" ] || return 0
  while IFS= read -r name; do
    case $name in '' | image\ *) continue ;; esac
    [ -e "$DIR/$name" ] || printf '%s\n' "$name"
  done < "$RECORD"
}

# recorded_image is the image the record was derived from, or nothing.
recorded_image() {
  [ -r "$RECORD" ] || return 0
  sed -n 's/^image //p' "$RECORD" | head -1
}

# register runs the image's installer, which writes every handler it carries
# that the kernel lacks, and leaves the ones that are there alone.
register() {
  podman run --rm --privileged --pull=missing "$TK_BINFMT_IMAGE" --install all >/dev/null
}

# record writes the base's own set: the image's emulators the kernel now holds.
record() {
  mnt=$(podman image mount "$TK_BINFMT_IMAGE") || die "could not read the emulators $TK_BINFMT_IMAGE carries"
  carried=''
  for f in "$mnt"/usr/bin/qemu-*; do
    [ -e "$f" ] && carried="$carried ${f##*/}"
  done
  podman image unmount "$TK_BINFMT_IMAGE" >/dev/null 2>&1 || :
  [ -n "$carried" ] || die "$TK_BINFMT_IMAGE carries no qemu emulator under /usr/bin"
  mkdir -p "${RECORD%/*}"
  tmp=$(mktemp "$RECORD.XXXXXX") || die "no temporary file beside $RECORD"
  printf 'image %s\n' "$TK_BINFMT_IMAGE" > "$tmp"
  for name in $carried; do
    [ -e "$DIR/$name" ] && printf '%s\n' "$name" >> "$tmp"
  done
  chmod 0644 "$tmp"
  mv "$tmp" "$RECORD"
}

if [ "${1:-}" = watch ]; then
  # ⚠ EVERY 30 SECONDS, AND SILENT UNTIL IT ACTS. A handler cannot be watched
  # for: binfmt_misc is a pseudo filesystem that inotify does not reach. The
  # journal gets a line only when this registers, and a failure waits five
  # minutes so an image that cannot be pulled does not fill the journal.
  while :; do
    gone=$(missing | tr '\n' ' ')
    if [ -e "$DIR/status" ] && [ -n "$gone" ]; then
      echo "binfmt: these QEMU handlers of this base are gone: $gone; registering them again"
      if register && [ -z "$(missing)" ]; then
        echo "binfmt: registered again"
      else
        echo "binfmt: registering them again failed; trying again in five minutes"
        sleep 270
      fi
    fi
    sleep 30
  done
fi

[ -e "$DIR/register" ] || die "$DIR is not mounted, so no handler can be registered"

# -- the handlers ----------------------------------------------------------------
if [ "$(recorded_image)" != "$TK_BINFMT_IMAGE" ] || [ -n "$(missing)" ]; then
  gone=$(missing | tr '\n' ' ')
  if [ -n "$gone" ]; then
    say "these QEMU handlers of this base are gone: $gone; registering them again"
  else
    say "registering QEMU binary formats from $TK_BINFMT_IMAGE"
  fi
  register || die "the QEMU binary-format installer did not complete"
  record
fi
own=$(grep -cv '^image ' "$RECORD" 2>/dev/null || :)
[ "${own:-0}" -gt 0 ] || die "the QEMU binary-format installer left none of the image's handlers registered"
left=$(missing | tr '\n' ' ')
[ -z "$left" ] || die "registered, and these are still not held: $left"
say "QEMU binary formats: $own handler(s) of this base's own"

# -- the watcher -----------------------------------------------------------------
if [ "${TK_SYSTEMD:-false}" != true ] || [ ! -d /run/systemd/system ]; then
  say "no systemd, so no watcher: a job registers its own platform, and base ensure the rest"
  printf 'binfmt-complete\n'
  exit 0
fi
: "${TK_SELF_B64:?TK_SELF_B64 is required}"
mkdir -p "${SELF%/*}"
tmp=$(mktemp "$SELF.XXXXXX") || die "no temporary file beside $SELF"
printf '%s' "$TK_SELF_B64" | base64 -d > "$tmp" || die "could not write $SELF"
chmod 0755 "$tmp"
changed=0
if ! same_file "$tmp" "$SELF"; then
  mv "$tmp" "$SELF"
  changed=1
else
  rm -f "$tmp"
fi
unit_tmp=$(mktemp "/etc/systemd/system/$UNIT.XXXXXX") || die "no temporary file for $UNIT"
cat > "$unit_tmp" <<UNIT_FILE
[Unit]
Description=wsl-toolkit: register this base's QEMU handlers again when another distribution removes them
After=local-fs.target

[Service]
Environment=TK_BINFMT_IMAGE=$TK_BINFMT_IMAGE
ExecStart=$SELF watch
Restart=on-failure
RestartSec=30

[Install]
WantedBy=multi-user.target
UNIT_FILE
chmod 0644 "$unit_tmp"
if ! same_file "$unit_tmp" "/etc/systemd/system/$UNIT"; then
  mv "$unit_tmp" "/etc/systemd/system/$UNIT"
  changed=1
else
  rm -f "$unit_tmp"
fi
systemctl daemon-reload || die "systemctl daemon-reload failed"
systemctl enable "$UNIT" >/dev/null 2>&1 || die "systemctl could not enable $UNIT"
if [ "$changed" = 1 ]; then
  systemctl restart "$UNIT" || die "systemctl could not start $UNIT"
else
  systemctl start "$UNIT" || die "systemctl could not start $UNIT"
fi
[ "$(systemctl is-active "$UNIT" 2>/dev/null)" = active ] || die "$UNIT is not active after it was started"
say "the watcher $UNIT is active"
printf 'binfmt-complete\n'
