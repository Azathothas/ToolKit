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
# watcher's body: the copy the ensure installed, run by systemd. With TK_ARCH and
# TK_HANDLER it registers one job's platform and nothing else.
#
# ⭐ THE BASE'S OWN SET IS DERIVED, NOT TYPED: the emulators the image carries
# that the kernel holds after the install. The image carries qemu-i386, and on
# an amd64 host the installer leaves it out because the kernel runs 386 itself,
# so a table typed here would claim a handler nothing registers.
set -u

DIR=/proc/sys/fs/binfmt_misc
RECORD=/var/lib/wsl-toolkit/binfmt-handlers
WATCH_RECORD=/var/lib/wsl-toolkit/binfmt-watch
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

# root_podman is every podman this script runs, and it runs them as root.
#
# ⛔ ROOT'S PODMAN IS TOLD ITS CGROUP MANAGER AND ITS EVENTS BACKEND ON THE
# COMMAND LINE. On a base that boots systemd, root's podman picks the systemd
# cgroup manager, which reaches systemd over D-Bus, and provision.sh installs
# systemd without recommends, so a Debian base has no D-Bus. crun answered
# `cannot open sd-bus` and every `base ensure` exited 2. cgroupfs and the file
# backend need neither D-Bus nor journald, and a global option outranks whatever
# /etc/containers says. The account's engine reads the same two from the
# containers.conf provision.sh writes for it. WSL-168.
root_podman() {
  podman --cgroup-manager=cgroupfs --events-backend=file "$@"
}

# fetch makes the installer's image present, and fetch_said is why it could not.
#
# ⛔ THE PULL IS ITS OWN STEP, AND BOUNDED. The run pulled with --pull=missing
# under the ensure's 20 minute ceiling, so a registry that stalled held every
# `base ensure`, and on the helper route every `run`, for 20 minutes over a step
# that fails no ensure. A stored image is never pulled again. WSL-182.
#
# ⚠ THE BOUND NAMES podman ITSELF, with root_podman's two options: timeout runs a
# program, and root_podman is a function.
fetch_said=''
fetch() {
  fetch_said=''
  root_podman image exists "$TK_BINFMT_IMAGE" >/dev/null 2>&1 && return 0
  fetch_code=0
  if command -v timeout >/dev/null 2>&1; then
    fetch_out=$(timeout 120 podman --cgroup-manager=cgroupfs --events-backend=file pull -q "$TK_BINFMT_IMAGE" 2>&1 >/dev/null) || fetch_code=$?
  else
    fetch_out=$(root_podman pull -q "$TK_BINFMT_IMAGE" 2>&1 >/dev/null) || fetch_code=$?
  fi
  [ "$fetch_code" = 0 ] && return 0
  if [ "$fetch_code" = 124 ]; then
    fetch_said="the pull of $TK_BINFMT_IMAGE did not finish within 120 seconds"
  else
    fetch_said=$(printf '%s\n' "$fetch_out" | sed '/^[[:space:]]*$/d' | tail -n 1)
  fi
  [ -n "$fetch_said" ] || fetch_said="the pull exited $fetch_code"
  return "$fetch_code"
}

# register runs the image's installer, which writes every handler it carries
# that the kernel lacks, and leaves the ones that are there alone.
register() {
  fetch || return
  root_podman run --rm --privileged --pull=never "$TK_BINFMT_IMAGE" --install all >/dev/null
}

# note_watch records the watcher's state as root measured it, for the verifier.
#
# ⛔ THE VERIFIER RUNS AS THE ACCOUNT, and on a systemd base with no D-Bus the
# account's systemctl cannot reach the manager, so a watcher root started read
# `none`. WSL-181. ⚠ ITS OWN FILE, NOT A LINE OF $RECORD: a watcher still running
# an older copy of this script reads every line of $RECORD but `image` as a
# handler, and would register again every 30 seconds.
note_watch() {
  mkdir -p "${WATCH_RECORD%/*}"
  watch_tmp=$(mktemp "$WATCH_RECORD.XXXXXX") || return 0
  printf '%s\n' "$1" > "$watch_tmp"
  chmod 0644 "$watch_tmp"
  mv "$watch_tmp" "$WATCH_RECORD"
}

# record writes the base's own set: the image's emulators the kernel now holds.
record() {
  mnt=$(root_podman image mount "$TK_BINFMT_IMAGE") || die "could not read the emulators $TK_BINFMT_IMAGE carries"
  carried=''
  for f in "$mnt"/usr/bin/qemu-*; do
    [ -e "$f" ] && carried="$carried ${f##*/}"
  done
  root_podman image unmount "$TK_BINFMT_IMAGE" >/dev/null 2>&1 || :
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

# -- one platform, for the job that asked for it ---------------------------------
# ⭐ TK_ARCH NAMES ONE PLATFORM, and EnsurePlatform sends it with the handler that
# platform needs. Its root podman was a second inline copy in base.go, which had
# to be given the WSL-168 options a second time. This is now its one home. WSL-181.
if [ -n "${TK_ARCH:-}" ]; then
  : "${TK_HANDLER:?TK_HANDLER is required with TK_ARCH}"
  if [ ! -e "$DIR/$TK_HANDLER" ]; then
    fetch || die "the QEMU installer image was not pulled: $fetch_said"
    # ⚠ podman's own failure is the last line, and its code is the exit.
    root_podman run --rm --privileged --pull=never "$TK_BINFMT_IMAGE" --install "$TK_ARCH" || exit
  fi
  [ -e "$DIR/$TK_HANDLER" ] || die "handler $TK_HANDLER was not registered"
  printf 'binfmt-ready %s\n' "$TK_HANDLER"
  exit 0
fi

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
  register || die "the QEMU binary-format installer did not complete${fetch_said:+: $fetch_said}"
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
  note_watch none
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
watch_state=$(systemctl is-active "$UNIT" 2>/dev/null) || :
note_watch "${watch_state:-unknown}"
[ "$watch_state" = active ] || die "$UNIT is not active after it was started"
say "the watcher $UNIT is active"
printf 'binfmt-complete\n'
