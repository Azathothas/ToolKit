#!/bin/sh
# Provision the wsl-toolkit base distribution: a rootless container engine and
# one unprivileged account to run it as.
#
# It is delivered on stdin by the executable and runs as root ONCE, at create or
# repair time. Nothing afterwards runs as root.
#
# TK_USER and TK_UID arrive as exported variables rather than being substituted
# into this text. Substituting a value into a script is the defect the command
# channel exists to remove, and doing it here would undo that one layer up.
set -eu

say() { printf '  * %s\n' "$*"; }
die() { printf 'provision: %s\n' "$*" >&2; exit 3; }

: "${TK_USER:?TK_USER is required}"
: "${TK_UID:?TK_UID is required}"
: "${TK_BINFMT_IMAGE:?TK_BINFMT_IMAGE is required}"
: "${TK_AUTOMOUNT:?TK_AUTOMOUNT is required}"
: "${TK_INTEROP:?TK_INTEROP is required}"
: "${TK_SYSTEMD:?TK_SYSTEMD is required}"
: "${TK_TOOLSET:?TK_TOOLSET is required}"
: "${TK_FSTAB_B64?TK_FSTAB_B64 is required, and may be empty}"
: "${TK_MOUNT_CHECKS?TK_MOUNT_CHECKS is required, and may be empty}"

# -- how the Windows drives appear ---------------------------------------------
# ⛔ READ ONLY BY DEFAULT. WSL mounts every fixed drive under /mnt, and a job that
# can WRITE there can destroy the real checkout on the Windows host. That is the
# one thing this tool's copy-never-mount rule exists to make impossible, and the
# automount was leaving it open behind the rule.
#
# ⚠ The `ro` goes in the MOUNT OPTIONS, not in `enabled`. `enabled=false` is a
# third setting with a different meaning, and conflating them would silently turn
# a read-only request into no mount at all.
case "$TK_AUTOMOUNT" in
  ro)  AUTOMOUNT_BLOCK='enabled=true
options="metadata,ro"' ;;
  rw)  AUTOMOUNT_BLOCK='enabled=true
options="metadata"' ;;
  off) AUTOMOUNT_BLOCK='enabled=false' ;;
  *)   die "TK_AUTOMOUNT is $TK_AUTOMOUNT; it must be ro, rw or off" ;;
esac
say "windows drives: $TK_AUTOMOUNT"

case "$TK_INTEROP" in
  on)  INTEROP_ENABLED=true ;;
  off) INTEROP_ENABLED=false ;;
  *)   die "TK_INTEROP is $TK_INTEROP; it must be on or off" ;;
esac
say "windows interop: $TK_INTEROP"

case "$TK_SYSTEMD" in
  true|false) SYSTEMD_ENABLED=$TK_SYSTEMD ;;
  *)          die "TK_SYSTEMD is $TK_SYSTEMD; it must be true or false" ;;
esac
say "systemd: $SYSTEMD_ENABLED"

# -- which userland is this ---------------------------------------------------
# Read from what is installed rather than from /etc/os-release's ID, because a
# derivative names itself and keeps its parent's package manager.
if   command -v apk          >/dev/null 2>&1; then FAMILY=apk
elif command -v pacman       >/dev/null 2>&1; then FAMILY=pacman
elif command -v apt-get      >/dev/null 2>&1; then FAMILY=apt
elif command -v dnf          >/dev/null 2>&1; then FAMILY=dnf
elif command -v tdnf         >/dev/null 2>&1; then FAMILY=tdnf
elif command -v xbps-install >/dev/null 2>&1; then FAMILY=xbps
else die "no package manager this script knows: tried apk, pacman, apt-get, dnf, tdnf, xbps-install"
fi
say "package manager: $FAMILY"

# -- the engine and what rootless needs ---------------------------------------
# shadow supplies newuidmap and newgidmap. Without them a rootless engine cannot
# map more than one id, and every image with a non-root user in it fails at
# unpack time reporting a permission error that reads like a broken image.
#
# ⛔ passt SUPPLIES pasta, AND podman 5.4 AND LATER REACH FOR IT FIRST. Measured
# on this host on 2026-09-09: a Debian base carrying slirp4netns and no passt
# refused every rootless run with "could not find pasta, the network namespace
# can't be configured", which reads as a broken image and is a missing package.
# It is installed where the distribution has it, and the containers.conf below
# pins the older path where it does not.
#
# ⚠ The passt install is allowed to fail. A distribution that does not package
# it is a distribution the fallback covers, and refusing the whole build over an
# optional package would turn a working base into no base at all.
case "$FAMILY" in
  apk)
    apk update >/dev/null
    apk add --no-cache podman crun fuse-overlayfs slirp4netns shadow shadow-uidmap \
      iptables ip6tables ca-certificates tar iproute2 >/dev/null
    apk add --no-cache passt >/dev/null 2>&1 || :
    ;;
  pacman)
    # ⛔ ARCH DOES NOT SUPPORT PARTIAL UPGRADES. Refreshing the package database
    # with -Sy and installing into an older rootfs produced an unsatisfiable
    # dependency set on a fresh base on 2026-09-12. Upgrade the rootfs and
    # install from one synchronized transaction.
    pacman -Syu --noconfirm --needed podman crun fuse-overlayfs slirp4netns shadow \
      iptables-nft ca-certificates tar iproute2 >/dev/null
    pacman -S --noconfirm --needed passt >/dev/null 2>&1 || :
    ;;
  apt)
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq >/dev/null
    apt-get install -y -qq --no-install-recommends podman uidmap fuse-overlayfs \
      slirp4netns ca-certificates iproute2 >/dev/null
    apt-get install -y -qq --no-install-recommends passt >/dev/null 2>&1 || :
    ;;
  dnf)
    dnf -y --setopt=install_weak_deps=False install podman fuse-overlayfs \
      slirp4netns shadow-utils iproute tar >/dev/null
    dnf -y --setopt=install_weak_deps=False install passt >/dev/null 2>&1 || :
    ;;
  tdnf)
    tdnf install -y podman shadow-utils tar iproute2 >/dev/null
    tdnf install -y passt >/dev/null 2>&1 || :
    ;;
  xbps)
    xbps-install -Sy podman crun fuse-overlayfs slirp4netns shadow \
      iproute2 ca-certificates tar >/dev/null
    xbps-install -Sy passt >/dev/null 2>&1 || :
    ;;
esac

# -- optional systemd ---------------------------------------------------------
# WSL starts systemd only when both halves exist: wsl.conf asks for it and the
# userland carries the manager. Refuse a userland this provisioner cannot make
# true rather than writing a setting that boot will ignore.
if [ "$SYSTEMD_ENABLED" = true ]; then
  case "$FAMILY" in
    apk|xbps)
      die "systemd was requested, and this userland does not package it as its init system"
      ;;
    pacman)
      pacman -S --noconfirm --needed systemd >/dev/null
      ;;
    apt)
      apt-get install -y -qq --no-install-recommends systemd systemd-sysv >/dev/null
      ;;
    dnf)
      dnf -y --setopt=install_weak_deps=False install systemd >/dev/null
      ;;
    tdnf)
      tdnf install -y systemd >/dev/null
      ;;
  esac
  command -v systemctl >/dev/null 2>&1 || die "systemd was requested and systemctl is still absent"
fi

# -- reproducible base tools --------------------------------------------------
# Package names differ, but the commands promised by `developer` do not. Keep
# this list small: provider-specific installers remain an explicit act by the
# unprivileged account and are not fetched or executed as root here.
case "$TK_TOOLSET" in
  none)
    ;;
  developer)
    case "$FAMILY" in
      apk)
        apk add --no-cache bash build-base curl git jq nodejs npm openssh-client ripgrep tmux unzip >/dev/null
        ;;
      pacman)
        pacman -S --noconfirm --needed bash base-devel curl git jq nodejs npm openssh ripgrep tmux unzip >/dev/null
        ;;
      apt)
        apt-get install -y -qq --no-install-recommends bash build-essential curl git jq nodejs npm openssh-client ripgrep tmux unzip >/dev/null
        ;;
      dnf)
        dnf -y --setopt=install_weak_deps=False install bash gcc gcc-c++ make curl git jq nodejs npm openssh-clients ripgrep tmux unzip >/dev/null
        ;;
      tdnf)
        tdnf install -y bash gcc gcc-c++ make curl git jq nodejs npm openssh-clients ripgrep tmux unzip >/dev/null
        ;;
      xbps)
        xbps-install -Sy bash base-devel curl git jq nodejs npm openssh ripgrep tmux unzip >/dev/null
        ;;
    esac
    for tool in bash cc c++ curl git jq make node npm rg ssh tmux unzip; do
      command -v "$tool" >/dev/null 2>&1 || die "the developer toolset promised $tool and it is absent"
    done
    ;;
  *) die "TK_TOOLSET is $TK_TOOLSET; it must be none or developer" ;;
esac
say "toolset: $TK_TOOLSET"

# ⛔ THE EFFECT IS VERIFIED, NOT THE EXIT CODE TRUSTED. A package manager can
# print an error from a hook and still exit 0, which measurement on this host
# showed pacman doing. What matters is whether the binary is on PATH.
command -v podman >/dev/null 2>&1 || die "podman is still not installed after the package step"
say "podman: $(podman --version 2>&1 | head -1)"
for tool in newuidmap newgidmap; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is missing, so a rootless engine cannot map more than one id"
done

# -- other Linux architectures ----------------------------------------------
# The WSL kernel is common to its distributions. These handlers let the
# rootless engine run a Linux image for another architecture. The installer
# container is privileged because it writes the kernel's binfmt_misc table. It
# is removed when registration is complete.
say "registering QEMU binary formats from $TK_BINFMT_IMAGE"
podman run --rm --privileged --pull=missing "$TK_BINFMT_IMAGE" --install all >/dev/null || \
  die "the QEMU binary-format installer did not complete"
set -- /proc/sys/fs/binfmt_misc/qemu-*
[ -e "$1" ] || die "the QEMU binary-format installer created no handlers"
say "QEMU binary formats: $# handler(s)"

# -- the unprivileged account -------------------------------------------------
if id -u "$TK_USER" >/dev/null 2>&1; then
  say "account $TK_USER already exists"
else
  if command -v useradd >/dev/null 2>&1; then
    useradd --create-home --uid "$TK_UID" --shell /bin/sh "$TK_USER"
  elif command -v adduser >/dev/null 2>&1; then
    adduser -D -u "$TK_UID" -s /bin/sh "$TK_USER"
  else
    die "no useradd and no adduser"
  fi
  say "created $TK_USER as uid $TK_UID"
fi
TK_HOME=$(getent passwd "$TK_USER" 2>/dev/null | cut -d: -f6) || TK_HOME=
[ -n "$TK_HOME" ] || TK_HOME=/home/$TK_USER
[ -d "$TK_HOME" ] || { mkdir -p "$TK_HOME"; chown "$TK_USER" "$TK_HOME"; }

# -- the id ranges a rootless engine maps -------------------------------------
# One range each, well clear of any real uid. A user with no range gets
# "cannot set up namespace" from every run, which reads as a container failure
# and is a missing line in a file.
for f in /etc/subuid /etc/subgid; do
  [ -f "$f" ] || : > "$f"
  if grep -q "^$TK_USER:" "$f" 2>/dev/null; then
    say "$f already has a range for $TK_USER"
  else
    printf '%s:100000:65536\n' "$TK_USER" >> "$f"
    say "added a 65536 id range to $f"
  fi
done

# -- engine configuration, for this account -----------------------------------
# cgroup_manager: cgroupfs works in both supported init modes. It is required
# when systemd is off because there is no user slice, and avoids making rootless
# containers depend on a lingering user manager when systemd is on.
#
# events_logger: journald is not running here either, and the default logger
# fails closed rather than falling back, so every run ends in an error about a
# log nobody asked for.
mkdir -p "$TK_HOME/.config/containers"
CONF_PATH=$TK_HOME/.config/containers/containers.conf
{
  echo '# Written by wsl-toolkit. cgroupfs works with either configured init.'
  echo '[engine]'
  echo 'cgroup_manager = "cgroupfs"'
  echo 'events_logger = "file"'
} > "$CONF_PATH"

# ⛔ THE FALLBACK IS WRITTEN ONLY WHEN pasta IS REALLY ABSENT, and the test is
# for the BINARY rather than for the package name. Pinning slirp4netns on a host
# that has pasta chooses the slower path for no reason, and asserting that a
# package installed would answer about the package manager rather than about
# what is on PATH.
if command -v pasta >/dev/null 2>&1; then
  say "rootless networking: pasta"
elif command -v slirp4netns >/dev/null 2>&1; then
  {
    echo '[network]'
    echo 'default_rootless_network_cmd = "slirp4netns"'
  } >> "$CONF_PATH"
  say "rootless networking: pasta is absent, pinned to slirp4netns"
else
  die "neither pasta nor slirp4netns is installed, so a rootless container has no way to get a network"
fi

# ⛔ SHORT NAMES ARE REFUSED INSIDE THE GUEST TOO. An unqualified reference is
# resolved through the engine's own alias table, so the same string is two
# different images on two machines. Enforcing here means an agent that types a
# short name gets a refusal naming the rule rather than a surprise image.
mkdir -p /etc/containers
cat > /etc/containers/registries.conf <<'CONF'
# Written by wsl-toolkit. Every reference is fully qualified, everywhere.
unqualified-search-registries = []
short-name-mode = "enforcing"
CONF
chown -R "$TK_USER" "$TK_HOME/.config"

# -- explicit Windows directory grants ---------------------------------------
# One marked block is replaced as a unit, so changing the configuration removes
# a stale grant on the next provision. The payload is base64 made by Go: no host
# path becomes shell source, and base64 reads from a file so its exit code is not
# hidden by a pipeline.
FSTAB_BEGIN='# wsl-toolkit mounts begin'
FSTAB_END='# wsl-toolkit mounts end'
FSTAB_TMP="/etc/fstab.wsl-toolkit.$$"
FSTAB_ENCODED="/tmp/wsl-toolkit-fstab.$$"
: > "$FSTAB_TMP"
if [ -f /etc/fstab ]; then
  in_toolkit_block=false
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      "$FSTAB_BEGIN") in_toolkit_block=true; continue ;;
      "$FSTAB_END")   in_toolkit_block=false; continue ;;
    esac
    if [ "$in_toolkit_block" = false ]; then
      printf '%s\n' "$line" >> "$FSTAB_TMP"
    fi
  done < /etc/fstab
fi
printf '%s\n' "$FSTAB_BEGIN" >> "$FSTAB_TMP"
if [ -n "$TK_FSTAB_B64" ]; then
  printf '%s' "$TK_FSTAB_B64" > "$FSTAB_ENCODED"
  if ! base64 -d "$FSTAB_ENCODED" >> "$FSTAB_TMP"; then
    rm -f "$FSTAB_TMP" "$FSTAB_ENCODED"
    die "the explicit mount table could not be decoded"
  fi
  rm -f "$FSTAB_ENCODED"
fi
printf '%s\n' "$FSTAB_END" >> "$FSTAB_TMP"
mv "$FSTAB_TMP" /etc/fstab

mount_count=0
# The payload is a validated sequence of base64-target/mode pairs.
# shellcheck disable=SC2086
set -- $TK_MOUNT_CHECKS
while [ "$#" -gt 0 ]; do
  [ "$#" -ge 2 ] || die "the explicit mount check table is incomplete"
  target_b64=$1
  shift 2
  printf '%s' "$target_b64" > "$FSTAB_ENCODED"
  if ! target=$(base64 -d "$FSTAB_ENCODED"); then
    rm -f "$FSTAB_ENCODED"
    die "an explicit mount target could not be decoded"
  fi
  rm -f "$FSTAB_ENCODED"
  mkdir -p "$target"
  chmod 755 "$target"
  mount_count=$((mount_count + 1))
done
say "explicit Windows directories: $mount_count"

# -- how WSL starts this distribution -----------------------------------------
# appendWindowsPath=false is the half that answers the reported complaint. With
# it left on, every Windows PATH entry is appended to the guest's, so a guest
# `find` on PATH can resolve to a Windows executable and a tool an agent
# installed inside is shadowed by one it did not.
cat > /etc/wsl.conf <<CONF
# Written by wsl-toolkit.
[user]
default=$TK_USER

[boot]
systemd=$SYSTEMD_ENABLED

[automount]
$AUTOMOUNT_BLOCK
mountFsTab=true

[interop]
enabled=$INTEROP_ENABLED
appendWindowsPath=false
CONF
say "wrote /etc/wsl.conf"

printf 'provision-complete\n'
