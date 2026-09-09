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
    pacman -Sy --noconfirm --needed podman crun fuse-overlayfs slirp4netns shadow \
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

# ⛔ THE EFFECT IS VERIFIED, NOT THE EXIT CODE TRUSTED. A package manager can
# print an error from a hook and still exit 0, which measurement on this host
# showed pacman doing. What matters is whether the binary is on PATH.
command -v podman >/dev/null 2>&1 || die "podman is still not installed after the package step"
say "podman: $(podman --version 2>&1 | head -1)"
for tool in newuidmap newgidmap; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is missing, so a rootless engine cannot map more than one id"
done

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
# cgroup_manager: this distribution runs WSL's own init rather than systemd, so
# there is no user slice for the systemd manager to place a container in. Left
# on the default the engine reports "systemd cgroup flag passed, but systemd
# support for managing cgroups is not available", which reads as a kernel
# problem and is a manager setting.
#
# events_logger: journald is not running here either, and the default logger
# fails closed rather than falling back, so every run ends in an error about a
# log nobody asked for.
mkdir -p "$TK_HOME/.config/containers"
CONF_PATH=$TK_HOME/.config/containers/containers.conf
{
  echo '# Written by wsl-toolkit. This distribution runs WSL init, not systemd.'
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
systemd=false

[automount]
enabled=true

[interop]
enabled=true
appendWindowsPath=false
CONF
say "wrote /etc/wsl.conf"

printf 'provision-complete\n'
