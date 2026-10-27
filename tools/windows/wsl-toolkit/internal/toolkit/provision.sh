#!/bin/sh
# Provision the wsl-toolkit base distribution: a rootless container engine and
# one unprivileged account to run it as.
#
# It is delivered on stdin by the executable and runs as root at create or
# repair time. Normal work runs as the configured account; an explicit profile
# setting may let that account elevate without a password.
#
# ⭐ packages.sh ARRIVES AHEAD OF IT, in the same payload. That is the shared
# package table from scripts/common/bootstrap.sh, and this script calls its
# detect_provider, detect_os_id and package_for rather than keeping a second map of
# package names. TODO/RULES.md section 4 owns the rule.
#
# ⚠ TWELVE MANAGERS ARE DETECTED AND FOUR HAVE HAD A BASE BUILT. A base has been
# built and verified from apk, apt, dnf and pacman, the alpine, debian, fedora and
# arch presets. tdnf and xbps have engine packages below and no build. emerge, yum
# and zypper are refused by name, because nothing here installs an engine through
# them, and pkg, pkg_add and pkgin are BSD managers no WSL distribution has.
#
# TK_USER and TK_UID arrive as exported variables rather than being substituted
# into this text. Substituting a value into a script is the defect the command
# channel exists to remove, and doing it here would undo that one layer up.
set -eu

# ⛔ THE CREATION MASK IS SET HERE, NOT INHERITED. The unprivileged account reads
# and traverses what this script creates: /etc/fstab, /etc/subuid and every
# directory above a mount target. A restrictive mask taken for the sudoers
# candidate once leaked into every later step, so a fresh build made /workspaces
# 0700 and the account could not reach its own checkout. A step that needs a
# tighter mask takes it inside a subshell, and a test holds that shape.
umask 022

# ⚠ OUT OF WHATEVER DIRECTORY WSL STARTED IN. With no --cd, wsl.exe starts the
# shell in the caller's Windows directory translated under /mnt, so a drive this
# script unmounts below would be the one it is standing in.
cd /

say() { printf '  * %s\n' "$*"; }
die() { printf 'provision: %s\n' "$*" >&2; exit 3; }

# >>> run deadline: begin
# ⛔ NO MANAGER CALL OUTLIVES THE RUN. The executable stops provisioning after
# TK_PROVISION_SECONDS. A stalled alpine mirror's second bounded try was cut
# there with no name, and the new base was rolled back. Each bound below is cut
# to the time the run has left, less PROVISION_MARGIN for the script to say why
# it stopped. With no TK_PROVISION_SECONDS, nothing is cut. WSL-204.
PROVISION_MARGIN=120
provision_deadline=
if [ -n "${TK_PROVISION_SECONDS:-}" ]; then
  provision_deadline=$(($(date +%s) + TK_PROVISION_SECONDS - PROVISION_MARGIN))
fi
# <<< run deadline: end

: "${TK_USER:?TK_USER is required}"
: "${TK_UID:?TK_UID is required}"
: "${TK_AUTOMOUNT:?TK_AUTOMOUNT is required}"
: "${TK_INTEROP:?TK_INTEROP is required}"
: "${TK_SYSTEMD:?TK_SYSTEMD is required}"
: "${TK_SHARED_TMPFS:?TK_SHARED_TMPFS is required}"
: "${TK_PASSWORDLESS_SUDO:?TK_PASSWORDLESS_SUDO is required}"
: "${TK_TOOLSET:?TK_TOOLSET is required}"
: "${TK_BOOT_B64:?TK_BOOT_B64 is required}"

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

case "$TK_SHARED_TMPFS" in
  on|off) ;;
  *) die "TK_SHARED_TMPFS is $TK_SHARED_TMPFS; it must be on or off" ;;
esac
say "shared /mnt/wsl: $TK_SHARED_TMPFS"

case "$TK_PASSWORDLESS_SUDO" in
  true|false) ;;
  *) die "TK_PASSWORDLESS_SUDO is $TK_PASSWORDLESS_SUDO; it must be true or false" ;;
esac

# -- which userland is this ---------------------------------------------------
# The package manager is read from what is installed, by the shared table's
# detection, because a derivative names itself in /etc/os-release and keeps its
# parent's manager. The system's ID is read too, because the table spells some
# names per distribution.
# >>> package manager: begin
PROVIDER=$(detect_provider)
OS_ID=$(detect_os_id)
FAMILY=$PROVIDER
# PROVIDERS is the shared table's list, set by packages.sh ahead of this script.
# shellcheck disable=SC2153
case "$FAMILY" in
  apk|apt|dnf|pacman|tdnf|xbps) ;;
  '') die "no package manager this script knows: tried $PROVIDERS" ;;
  *)  die "$FAMILY is installed, and this script installs a container engine only through apk, apt, dnf, pacman, tdnf or xbps" ;;
esac
say "package manager: $FAMILY on $OS_ID"
# <<< package manager: end

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
# >>> engine packages: begin
case "$FAMILY" in
  apt) export DEBIAN_FRONTEND=noninteractive ;;
esac

# ⛔ A MANAGER THAT HANGS IS STOPPED, AND THE STOP IS NAMED. A first build of the
# alpine preset hung twice in `apk update`, and nothing bounded it, so the build
# waited until it was stopped by hand. Each manager call below gets
# ENGINE_STEP_SECONDS where the userland carries timeout, as coreutils and
# BusyBox both do. GNU timeout answers 124; BusyBox's leaves the TERM the
# command died of, 143. A stopped call then counts as a failed attempt. WSL-203.
#
# ⚠ THE BOUND IS CUT TO THE RUN'S DEADLINE, and a call the run has no time left
# for is not started. Three tries of 900 seconds outlast the run's 30 minutes.
# WSL-204.
ENGINE_STEP_SECONDS=900
engine_timeout=
if command -v timeout >/dev/null 2>&1; then
  engine_timeout=timeout
fi
bounded() {
  bounded_status=0
  bounded_for=$ENGINE_STEP_SECONDS
  if [ -n "${provision_deadline:-}" ]; then
    bounded_left=$((provision_deadline - $(date +%s)))
    if [ "$bounded_left" -lt "$bounded_for" ]; then
      bounded_for=$bounded_left
    fi
    if [ "$bounded_for" -lt 1 ]; then
      printf '%s was not started: provisioning has no time left\n' "$1" >&2
      return 124
    fi
  fi
  if [ -n "$engine_timeout" ]; then
    timeout "$bounded_for" "$@" || bounded_status=$?
    case "$bounded_status" in
      124|143) printf '%s ran past %ss and was stopped\n' "$1" "$bounded_for" >&2 ;;
    esac
  else
    "$@" || bounded_status=$?
  fi
  return "$bounded_status"
}

# engine_transaction is one attempt at the family's engine packages, the index
# refresh included where the family has one.
engine_transaction() {
  case "$FAMILY" in
    apk)
      bounded apk update >/dev/null &&
        bounded apk add --no-cache podman crun fuse-overlayfs slirp4netns shadow shadow-uidmap \
          iptables ip6tables ca-certificates tar iproute2 >/dev/null
      ;;
    pacman)
      # ⛔ ARCH DOES NOT SUPPORT PARTIAL UPGRADES. Refreshing the package database
      # with -Sy and installing into an older rootfs produced an unsatisfiable
      # dependency set on a fresh base on 2026-09-12. Upgrade the rootfs and
      # install from one synchronized transaction.
      bounded pacman -Syu --noconfirm --needed podman crun fuse-overlayfs slirp4netns shadow \
        iptables-nft ca-certificates tar iproute2 >/dev/null
      ;;
    apt)
      # ⛔ nftables SUPPLIES nft, AND netavark SHELLS OUT TO IT BY THAT NAME.
      # Measured on this host on 2026-09-15: a Debian base carrying no firewall
      # package refused the QEMU binary-format installer's rootful run with
      # `netavark: nftables error: unable to execute nft: No such file or
      # directory`, and the build rolled the distribution back. The apk arm carries
      # iptables and the pacman arm iptables-nft; this arm carried neither. WSL-86.
      bounded apt-get update -qq >/dev/null &&
        bounded apt-get install -y -qq --no-install-recommends podman uidmap fuse-overlayfs \
          slirp4netns ca-certificates iproute2 nftables >/dev/null
      ;;
    dnf)
      # ⛔ util-linux SUPPLIES runuser AND su, AND PROVISIONING BECOMES THE ACCOUNT. Fedora's
      # image carries util-linux-core, which has neither, and podman pulls in neither.
      # Measured on a first build of the fedora preset: provisioning stopped at the
      # account's XDG directories, and the build was removed. WSL-201.
      bounded dnf -y --setopt=install_weak_deps=False install podman fuse-overlayfs \
        slirp4netns shadow-utils iproute tar util-linux >/dev/null
      ;;
    # ⚠ AND HERE, under the same name, on Photon, Azure Linux and Void. Neither preset is
    # built yet, and a build without it would stop at the XDG directories. WSL-201.
    tdnf)
      bounded tdnf install -y podman shadow-utils tar iproute2 util-linux >/dev/null
      ;;
    xbps)
      bounded xbps-install -Sy podman crun fuse-overlayfs slirp4netns shadow \
        iproute2 ca-certificates tar util-linux >/dev/null
      ;;
  esac
}

# ⛔ A STALLED MIRROR IS ASKED AGAIN, AND NAMED. Two builds from nothing on
# 2026-09-14 failed in pacman on `geo.mirror.pkgbuild.com : Operation too slow`
# and rolled their distributions back, and the next attempt passed. Finding 10,
# WSL-140. So each family's transaction gets three attempts, the manager's last
# line names what stopped each one, and the pause between them grows.
#
# ⚠ The manager's stderr is relayed after each attempt rather than as it comes,
# because the last line of it is what names a stall. stdin is /dev/null: this
# script arrives on stdin, and a manager that read it would eat the rest.
ENGINE_ATTEMPTS=3
engine_attempt=1
while :; do
  engine_status=0
  engine_said=$(engine_transaction 2>&1 >/dev/null </dev/null) || engine_status=$?
  if [ -n "$engine_said" ]; then
    printf '%s\n' "$engine_said" >&2
  fi
  if [ "$engine_status" -eq 0 ]; then
    break
  fi
  engine_why='it printed no reason'
  while IFS= read -r engine_line; do
    if [ -n "$engine_line" ]; then
      engine_why=$engine_line
    fi
  done <<ENGINE_SAID
$engine_said
ENGINE_SAID
  if [ "$engine_attempt" -ge "$ENGINE_ATTEMPTS" ]; then
    die "the engine packages did not install through $FAMILY in $ENGINE_ATTEMPTS attempts. The last one ended: $engine_why"
  fi
  engine_pause=$((engine_attempt * 15))
  # ⚠ A try the run has no time left for is not waited for. WSL-204.
  if [ -n "${provision_deadline:-}" ] && [ $(($(date +%s) + engine_pause)) -ge "$provision_deadline" ]; then
    die "the engine packages did not install through $FAMILY, and provisioning has no time left for attempt $((engine_attempt + 1)). The last one ended: $engine_why"
  fi
  say "the engine packages did not install, attempt $engine_attempt of $ENGINE_ATTEMPTS: $engine_why. Trying again in ${engine_pause}s"
  sleep "$engine_pause"
  engine_attempt=$((engine_attempt + 1))
done

case "$FAMILY" in
  apk)    bounded apk add --no-cache passt >/dev/null 2>&1 || : ;;
  pacman) bounded pacman -S --noconfirm --needed passt >/dev/null 2>&1 || : ;;
  apt)    bounded apt-get install -y -qq --no-install-recommends passt >/dev/null 2>&1 || : ;;
  dnf)    bounded dnf -y --setopt=install_weak_deps=False install passt >/dev/null 2>&1 || : ;;
  tdnf)   bounded tdnf install -y passt >/dev/null 2>&1 || : ;;
  xbps)   bounded xbps-install -Sy passt >/dev/null 2>&1 || : ;;
esac
# <<< engine packages: end

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
      bounded pacman -S --noconfirm --needed systemd >/dev/null
      ;;
    apt)
      bounded apt-get install -y -qq --no-install-recommends systemd systemd-sysv >/dev/null
      ;;
    dnf)
      bounded dnf -y --setopt=install_weak_deps=False install systemd >/dev/null
      ;;
    tdnf)
      bounded tdnf install -y systemd >/dev/null
      ;;
  esac
  command -v systemctl >/dev/null 2>&1 || die "systemd was requested and systemctl is still absent"
fi

# -- reproducible base tools --------------------------------------------------
# The commands promised by `developer` are this script's. Their package names are
# the shared table's. Keep this list small: provider-specific installers remain an
# explicit act by the unprivileged account and are not fetched or executed as root
# here.
case "$TK_TOOLSET" in
  none)
    ;;
  developer)
    # >>> developer packages: begin
    # ⚠ A NAME THE TABLE SAYS THIS SYSTEM DOES NOT CARRY IS NAMED AND LEFT OUT, and
    # the check after the install then refuses the command it would have supplied.
    developer_packages=
    for developer_name in bash build curl git jq node npm openssh ripgrep tmux unzip; do
      developer_resolved=$(package_for "$developer_name") ||
        die "the shared package table has no row for $developer_name"
      if [ -z "$developer_resolved" ]; then
        say "$OS_ID's $FAMILY carries no package for $developer_name"
        continue
      fi
      developer_packages="$developer_packages $developer_resolved"
    done
    # The list is package names from the table, split on purpose.
    # shellcheck disable=SC2086
    case "$FAMILY" in
      apk)    bounded apk add --no-cache $developer_packages >/dev/null ;;
      pacman) bounded pacman -S --noconfirm --needed $developer_packages >/dev/null ;;
      apt)    bounded apt-get install -y -qq --no-install-recommends $developer_packages >/dev/null ;;
      dnf)    bounded dnf -y --setopt=install_weak_deps=False install $developer_packages >/dev/null ;;
      tdnf)   bounded tdnf install -y $developer_packages >/dev/null ;;
      xbps)   bounded xbps-install -Sy $developer_packages >/dev/null ;;
    esac
    # <<< developer packages: end
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
# >>> rootless id mapping: begin
# ⛔ THE PRIVILEGE IS CHECKED, NOT THE PATH. newuidmap and newgidmap raise their
# own privilege through a file capability, or through a setuid bit where a
# distribution still ships one. A rootfs imported from a tar unpacked without
# extended attributes keeps the bytes and drops the capability, and the binary is
# then on PATH and inert.
#
# ⛔ MEASURED ON THIS HOST ON 2026-09-15. Fedora's imported newuidmap carried
# neither. Its base built, and verification then answered `newuidmap: write to
# uid_map failed: Operation not permitted` and `cannot set up namespace using
# "/usr/sbin/newuidmap": should have setuid or have filecaps setuid`, which reads
# as a broken image and is a dropped capability. The working Arch base's carries
# cap_setuid=ep. A check that asked only whether the binary was on PATH passed
# over it. WSL-86.
cap_tool() {
  # ⚠ /usr/sbin AND /sbin ARE NAMED. setcap and getcap live there on most of
  # these distributions, and an imported rootfs's root PATH does not always carry
  # them; this runs before any profile is read.
  for ct_name in "$1" /usr/sbin/"$1" /sbin/"$1" /usr/bin/"$1" /bin/"$1"; do
    if ct_found=$(command -v "$ct_name" 2>/dev/null); then
      printf '%s' "$ct_found"
      return 0
    fi
  done
  return 1
}

# ⚠ The capability tools are the family's own spelling, and the install is
# allowed to fail: a spelling this host has not measured is then named by the
# refusals below, with the package in the message, rather than ending the build
# inside a package manager.
case "$FAMILY" in
  apt)  CAP_PACKAGE=libcap2-bin ;;
  xbps) CAP_PACKAGE=libcap-progs ;;
  *)    CAP_PACKAGE=libcap ;;
esac
if ! cap_tool getcap >/dev/null || ! cap_tool setcap >/dev/null; then
  case "$FAMILY" in
    apk)    bounded apk add --no-cache "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
    pacman) bounded pacman -S --noconfirm --needed "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
    apt)    bounded apt-get install -y -qq --no-install-recommends "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
    dnf)    bounded dnf -y --setopt=install_weak_deps=False install "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
    tdnf)   bounded tdnf install -y "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
    xbps)   bounded xbps-install -Sy "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
  esac
fi
GETCAP=$(cap_tool getcap) || GETCAP=''
SETCAP=$(cap_tool setcap) || SETCAP=''

for tool in newuidmap newgidmap; do
  case "$tool" in
    newuidmap) want_cap=cap_setuid ;;
    *)         want_cap=cap_setgid ;;
  esac
  tool_path=$(command -v "$tool" 2>/dev/null) || tool_path=''
  [ -n "$tool_path" ] || die "$tool is missing, so a rootless engine cannot map more than one id"
  # A distribution that ships it setuid root needs no capability at all.
  if [ -u "$tool_path" ]; then
    say "$tool_path is setuid"
    continue
  fi
  # ⛔ EVERY SUBSTITUTION HAS ITS OWN FALLBACK. Under `set -e` an assignment from
  # a command that exits non-zero ends the script, and getcap exits non-zero on a
  # file with no capability at all, which is precisely the case being read.
  [ -n "$GETCAP" ] || die "$tool_path is not setuid and $CAP_PACKAGE supplied no getcap, so whether it carries $want_cap cannot be read"
  tool_caps=$("$GETCAP" "$tool_path" 2>/dev/null) || tool_caps=''
  case "$tool_caps" in
    *"$want_cap"*) continue ;;
  esac
  [ -n "$SETCAP" ] || die "$tool_path carries neither the setuid bit nor $want_cap, and $CAP_PACKAGE supplied no setcap to give it one"
  "$SETCAP" "$want_cap+ep" "$tool_path" >/dev/null 2>&1 || :
  tool_caps=$("$GETCAP" "$tool_path" 2>/dev/null) || tool_caps=''
  case "$tool_caps" in
    *"$want_cap"*)
      say "$tool_path arrived without $want_cap and now carries it"
      continue
      ;;
  esac
  die "$tool_path carries neither the setuid bit nor $want_cap, so a rootless engine cannot map more than one id"
done
# <<< rootless id mapping: end

# -- other Linux architectures ----------------------------------------------
# ⛔ NOT HERE ANY MORE. The handlers live in a table every distribution shares,
# and another distribution can empty it while this one runs, so registering them
# once at provisioning left a base that never got them back. binfmt.sh registers
# them in every `base ensure`, after this script, and on a systemd base installs
# the watcher that registers them again. WSL-105.

# -- the unprivileged account -------------------------------------------------
# ⭐ CREATED WITH bash WHERE THE GUEST HAS BASH. wanted_login_shell is
# loginshell.sh's, which arrives ahead of this script, and `base ensure` moves an
# account an older release created with /bin/sh. Finding 44. WSL-143.
# >>> the account's creation: begin
if id -u "$TK_USER" >/dev/null 2>&1; then
  say "account $TK_USER already exists"
else
  TK_LOGIN_SHELL=$(wanted_login_shell)
  # ⛔ TK_UID ONLY WHERE IT IS FREE. An image whose own account holds 1000, as
  # ubuntu's does, refused the build outright. The account then takes the next
  # free id, and the grants read the ids it got. WSL-188.
  uid_taken=$(getent passwd "$TK_UID" 2>/dev/null | cut -d: -f1) || uid_taken=
  if [ -n "$uid_taken" ]; then
    say "uid $TK_UID is $uid_taken's, so $TK_USER takes the next free one"
  fi
  if command -v useradd >/dev/null 2>&1; then
    if [ -n "$uid_taken" ]; then
      useradd --create-home --shell "$TK_LOGIN_SHELL" "$TK_USER"
    else
      useradd --create-home --uid "$TK_UID" --shell "$TK_LOGIN_SHELL" "$TK_USER"
    fi
  elif command -v adduser >/dev/null 2>&1; then
    if [ -n "$uid_taken" ]; then
      adduser -D -s "$TK_LOGIN_SHELL" "$TK_USER"
    else
      adduser -D -u "$TK_UID" -s "$TK_LOGIN_SHELL" "$TK_USER"
    fi
  else
    die "no useradd and no adduser"
  fi
  say "created $TK_USER as uid $(id -u "$TK_USER"), login shell $TK_LOGIN_SHELL"
fi
# <<< the account's creation: end
# ⚠ AN ACCOUNT THAT ALREADY EXISTS IS MOVED HERE TOO, so a re-provisioning repairs
# a login shell the verifier refused because nothing can start it.
login_said=$(apply_login_shell "$TK_USER") || login_said="the login shell of $TK_USER was not set"
say "$login_said"
TK_HOME=$(getent passwd "$TK_USER" 2>/dev/null | cut -d: -f6) || TK_HOME=
[ -n "$TK_HOME" ] || TK_HOME=/home/$TK_USER
# ⭐ THE HOME ITSELF IS ROOT'S TO MAKE AND GIVE. Its parent is root's, so the account
# cannot put a link in its place.
[ -d "$TK_HOME" ] || { mkdir -p "$TK_HOME"; chown "$TK_USER" "$TK_HOME"; }
TK_GROUP=$(id -gn "$TK_USER")
chown "$TK_USER:$TK_GROUP" "$TK_HOME"
chmod 0700 "$TK_HOME"

# >>> as the account: begin
# as_account SCRIPT NAME ARG...: SCRIPT run by sh as the account. NAME is its $0,
# and each ARG is one of its positional parameters. So a file in its home is
# written by its owner, and not by root following whatever that directory holds.
# It goes through runuser, or through su where there is no runuser.
#
# ⛔ NOT setpriv. BusyBox's has no --reuid, so every ensure on alpine printed its
# usage, and root then removed the account's file itself. su is grants.sh's form,
# its options before the account, and no ARG begins with a dash. WSL-188, WSL-199.
as_account() {
  aa_script=$1
  shift
  if command -v runuser >/dev/null 2>&1; then
    runuser -u "$TK_USER" -- /bin/sh -c "$aa_script" "$@"
  elif command -v su >/dev/null 2>&1; then
    su -s /bin/sh -c "$aa_script" "$TK_USER" "$@"
  else
    return 127
  fi
}
# <<< as the account: end

# >>> the account's XDG directories: begin
# ⛔ EVERYTHING INSIDE THE HOME IS THE ACCOUNT'S TO MAKE. A re-provisioning runs after
# the account has run agents and jobs, and this script cannot tell it from a first
# build. Root's mkdir, chown and chmod each followed a link the account left at one of
# these paths, so a link at .config had root give a directory outside the home to the
# account. WSL-201.
#
# ⚠ ROOT STILL GIVES BACK A DIRECTORY THAT IS NOT THE ACCOUNT'S, AND ONLY A DIRECTORY.
# Root enters each with cd -P and reads it back with pwd -P. Where it stands is then the
# directory at that path in the home, and root gives that directory, `.`. An older
# release left .config/containers root's when its provisioning died before its chown -R,
# and the account's own mktemp there then failed every run.
#
# ⛔ NEVER A NAME. chown -h on a name changed a hard link the account made there, and a
# hard link is the file itself. So a hard link at .cache to a file of root's gave the
# account that file. A directory cannot be hard-linked, and the one root changes is the
# one it read back. WSL-201.
# linksafe: root enters the home itself, and reads where it is
tk_home_real=$(cd -P "$TK_HOME" && pwd -P) || die "the home $TK_HOME cannot be entered"
for xdg_rel in .config .cache .local .local/share .local/state .config/containers .config/containers/containers.conf.d; do
  (
    # linksafe: cd -P resolves every link, and pwd -P reads back that this is the home's own
    cd -P "$TK_HOME/$xdg_rel" 2>/dev/null || exit 0
    [ "$(pwd -P)" = "$tk_home_real/$xdg_rel" ] || exit 0
    # linksafe: `.` is the directory pwd -P just read back as the home's own, and a directory has no hard link
    chown "$TK_USER:$TK_GROUP" .
  )
done
# ⛔ A NAME THAT IS NOT A DIRECTORY IS NAMED, AND LEFT AS IT IS. The verifier refuses
# the base over it, and only the account may move a file of its home aside.
# ⛔ THE ENGINE SETTINGS' TWO DIRECTORIES TOO. A file, a pipe or a dangling link at
# either failed the settings step below with a message that named neither the path nor
# the remedy. WSL-201.
xdg_code=0
# shellcheck disable=SC2016  # the single quotes hold the account's own script
xdg_said=$(as_account '
  for d do
    if [ -L "$d" ] || [ -e "$d" ]; then
      [ -d "$d" ] || { printf "%s" "$d"; exit 2; }
    fi
    mkdir -p "$d" || exit 1
    [ -L "$d" ] || chmod 0700 "$d" || exit 1
  done' wtk-xdg \
  "$TK_HOME/.config" "$TK_HOME/.config/containers" "$TK_HOME/.config/containers/containers.conf.d" \
  "$TK_HOME/.cache" "$TK_HOME/.local" "$TK_HOME/.local/share" "$TK_HOME/.local/state") ||
  xdg_code=$?
case $xdg_code in
  0) ;;
  2) die "$xdg_said is not a directory, and a directory of $TK_USER's belongs there. Move it aside as $TK_USER, then run base ensure again" ;;
  127) die "neither runuser nor su is installed, so nothing can act as $TK_USER in its home" ;;
  *) die "$TK_USER could not make its XDG directories in $TK_HOME, so the verifier would refuse the base" ;;
esac
# <<< the account's XDG directories: end
say "account home: $TK_HOME; persistent XDG directories: ready"

# -- optional agent privilege escalation -------------------------------------
# This is deliberately a tool-owned drop-in: turning the profile setting off
# removes only the authority this tool created. Validate the candidate before
# its atomic move so a malformed rule never becomes active.
sudoers_path=/etc/sudoers.d/wsl-toolkit-$TK_USER
case "$TK_PASSWORDLESS_SUDO" in
  true)
    case "$FAMILY" in
      apk)    bounded apk add --no-cache sudo >/dev/null ;;
      pacman) bounded pacman -S --noconfirm --needed sudo >/dev/null ;;
      apt)    bounded apt-get install -y -qq --no-install-recommends sudo >/dev/null ;;
      dnf)    bounded dnf -y --setopt=install_weak_deps=False install sudo >/dev/null ;;
      tdnf)   bounded tdnf install -y sudo >/dev/null ;;
      xbps)   bounded xbps-install -Sy sudo >/dev/null ;;
    esac
    command -v sudo >/dev/null 2>&1 || die "passwordless sudo was requested and sudo is absent"
    command -v visudo >/dev/null 2>&1 || die "passwordless sudo was requested and visudo is absent"
    mkdir -p /etc/sudoers.d
    chmod 0750 /etc/sudoers.d
    sudoers_tmp=$sudoers_path.tmp.$$
    ( umask 077; printf '%s ALL=(ALL:ALL) NOPASSWD: ALL\n' "$TK_USER" > "$sudoers_tmp" ) ||
      die "the passwordless sudo candidate could not be written"
    chmod 0440 "$sudoers_tmp"
    if ! visudo -cf "$sudoers_tmp" >/dev/null 2>&1; then
      rm -f "$sudoers_tmp"
      die "the passwordless sudo rule did not pass visudo"
    fi
    mv "$sudoers_tmp" "$sudoers_path"
    say "passwordless sudo: true"
    ;;
  false)
    rm -f "$sudoers_path"
    say "passwordless sudo: false"
    ;;
esac

# -- the id ranges a rootless engine maps -------------------------------------
# One range each, well clear of any real uid. A user with no range gets
# "cannot set up namespace" from every run, which reads as a container failure
# and is a missing line in a file.
#
# ⛔ A RANGE NO OTHER ACCOUNT HOLDS. 100000 was written whoever held it, so an
# image whose own account had that range shared its ids with this account's
# containers. The range starts past the highest one in the file, and never below
# 100000. WSL-188.
# >>> the id ranges: begin
for f in /etc/subuid /etc/subgid; do
  [ -f "$f" ] || : > "$f"
  if grep -q "^$TK_USER:" "$f" 2>/dev/null; then
    say "$f already has a range for $TK_USER"
  else
    range_start=$(awk -F: 'BEGIN { s = 100000 } $2 ~ /^[0-9]+$/ && $3 ~ /^[0-9]+$/ && $2 + $3 > s { s = $2 + $3 } END { print s }' "$f")
    printf '%s:%s:65536\n' "$TK_USER" "$range_start" >> "$f"
    say "added the 65536 ids from $range_start to $f"
  fi
done
# <<< the id ranges: end

# -- engine configuration, for this account -----------------------------------
# cgroup_manager: cgroupfs works in both supported init modes, and is required
# when systemd is off because there is no user slice. ⭐ On a systemd base,
# engines.sh, which every `base ensure` runs, delegates the controllers to the
# account's user manager, keeps that manager lingering, and switches the account
# to the systemd manager with a drop-in beside this file once its session bus
# answers. WSL-136.
#
# events_logger: journald is not running here either, and the default logger
# fails closed rather than falling back, so every run ends in an error about a
# log nobody asked for.
#
# ⛔ DROP-INS, NOT THE MAIN FILES. Every re-provisioning wrote the account's
# containers.conf and /etc/containers/registries.conf whole, so a mirror or a
# proxy set in either was gone after the next ensure. The main files are the
# operator's now. ⚠ The account's drop-in is 10-, so engines.sh's 50- switch to
# the systemd manager still sorts after it and wins. WSL-188.
# >>> the engine's settings: begin
# ⛔ THE ACCOUNT WRITES ITS OWN SETTINGS. Root built them in a file of its own outside
# the home, and the account reads, removes and writes in its home. Root's mkdir,
# redirect and chown -R followed a link the account left there. WSL-201.
CONF_DIR=$TK_HOME/.config/containers/containers.conf.d
CONF_PATH=$CONF_DIR/10-wsl-toolkit.conf
# ⚠ THE MAIN FILE AN EARLIER RELEASE WROTE IS THIS TOOL'S OWN, AND IT GOES, read
# by its first line. Kept, its slirp4netns pin would outlast a pasta installed
# since. A main file anyone else wrote is left as it is. WSL-188.
OLD_CONF=$TK_HOME/.config/containers/containers.conf
# shellcheck disable=SC2016  # the single quotes hold the account's own script
old_conf=$(as_account '
  if [ -f "$1" ] && [ "$(head -n 1 "$1")" = "$2" ]; then
    rm -f "$1" && echo removed
  fi' wtk-old-conf "$OLD_CONF" '# Written by wsl-toolkit. cgroupfs works with either configured init.') || old_conf=
if [ "$old_conf" = removed ]; then
  say "removed the containers.conf an earlier release wrote; its settings are now $CONF_PATH"
fi
conf_tmp=$(mktemp) || die "no temporary file for $CONF_PATH"
{
  echo '# Written by wsl-toolkit. cgroupfs works with either configured init.'
  echo '[engine]'
  echo 'cgroup_manager = "cgroupfs"'
  echo 'events_logger = "file"'
} > "$conf_tmp"

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
  } >> "$conf_tmp"
  say "rootless networking: pasta is absent, pinned to slirp4netns"
else
  rm -f "$conf_tmp"
  die "neither pasta nor slirp4netns is installed, so a rootless container has no way to get a network"
fi
# ⭐ A FILE OF THE ACCOUNT'S OWN BESIDE IT TAKES THE BYTES AND IS RENAMED OVER IT, so a
# link at that path is replaced and never written through.
# shellcheck disable=SC2016  # the single quotes hold the account's own script
if ! as_account '
  mkdir -p "${1%/*}" || exit 1
  t=$(mktemp "$1.XXXXXX") || exit 1
  if cat > "$t" && chmod 0644 "$t" && mv -f "$t" "$1"; then
    exit 0
  fi
  rm -f "$t"
  exit 1' wtk-engine-conf "$CONF_PATH" < "$conf_tmp"; then
  rm -f "$conf_tmp"
  die "$TK_USER could not write its engine settings at $CONF_PATH"
fi
rm -f "$conf_tmp"

# ⛔ SHORT NAMES ARE REFUSED INSIDE THE GUEST TOO. An unqualified reference is
# resolved through the engine's own alias table, so the same string is two
# different images on two machines. Enforcing here means an agent that types a
# short name gets a refusal naming the rule rather than a surprise image.
REG_PATH=/etc/containers/registries.conf.d/99-wsl-toolkit.conf
mkdir -p /etc/containers/registries.conf.d
# ⛔ A MAIN FILE IN THE V1 FORMAT TAKES NO DROP-IN. The engine refuses to mix the
# two formats, so every pull would fail. That file is written whole, as before.
if grep -q '^[[:space:]]*\[registries\.' /etc/containers/registries.conf 2>/dev/null; then
  REG_PATH=/etc/containers/registries.conf
  say "/etc/containers/registries.conf is in the v1 format, which takes no drop-in, so it is written whole"
fi
cat > "$REG_PATH" <<'CONF'
# Written by wsl-toolkit. Every reference is fully qualified, everywhere.
unqualified-search-registries = []
short-name-mode = "enforcing"
CONF
# ⭐ base.registry's mirrors and proxy, from mirrors.sh, which arrives ahead of
# this script. Written here, so the health probe's first pull takes them. A base
# without the key gets neither file and no line. WSL-196.
apply_registry_settings || die "base.registry's drop-ins could not be written"
# ⛔ NO chown -R OVER THE ACCOUNT'S .config. It gave the account what root wrote above,
# and the account writes that itself now. ⚠ BusyBox's changes a link in that tree
# rather than what it names, measured on Alpine, and GNU's does too by default. A walk
# by root through a tree the account owns can still meet a link swapped in while it
# runs. WSL-201.
# <<< the engine's settings: end

# -- explicit Windows directory grants ---------------------------------------
# ⭐ grants.sh writes the tool-owned /etc/fstab block, run by the executable right
# after this script and before the restart that mounts it. It is also what
# `base grant` and `base revoke` run live, so the block has one home.

# -- the shared tmpfs every distribution in the utility VM writes to ----------
# ⛔ /mnt/wsl IS ONE tmpfs, COMMON TO EVERY DISTRIBUTION, mounted world-writable,
# and uids are not namespaced across it. A base with no grants at all wrote a file
# there that another distribution read, measured 2026-09-15. WSL-68.
#
# ⛔ CLOSING IT TAKES DNS WITH IT, and that is the whole difficulty. WSL points
# /etc/resolv.conf at /mnt/wsl/resolv.conf, so a base that unmounts the directory
# and does nothing else answers `getent hosts` exit 2. The order below is the fix
# and it is not interchangeable: a real resolver is written HERE, while the shared
# file is still reachable, and the boot script refreshes it at every start BEFORE
# it unmounts.
if [ "$TK_SHARED_TMPFS" = off ]; then
  # >>> the shared resolver: begin
  # ⛔ WAITED FOR, NOT READ ONCE. WSL writes the shared file as a distribution
  # starts, and it was measured empty that early, so a base kept the resolver it
  # had, empty or old. Ten seconds, a second at a time. WSL-188.
  # ⚠ ONLY WHILE /mnt/wsl IS MOUNTED. A sealed base that is provisioned again
  # closed it at its last start, so nothing writes the file there, and every
  # re-provisioning paid the ten seconds for nothing. WSL-188.
  resolv_wait=0
  while [ ! -s /mnt/wsl/resolv.conf ] && [ "$resolv_wait" -lt 10 ] && grep -q ' /mnt/wsl ' /proc/mounts 2>/dev/null; do
    sleep 1
    resolv_wait=$((resolv_wait + 1))
  done
  # ⛔ THE SYMLINK IS REMOVED BEFORE ANYTHING IS WRITTEN. /etc/resolv.conf points
  # INTO the shared directory, so writing through it would put this base's
  # resolver in the tmpfs every other distribution can read and replace.
  if [ -s /mnt/wsl/resolv.conf ]; then
    rm -f /etc/resolv.conf
    grep -E '^(nameserver|search|options|domain) ' /mnt/wsl/resolv.conf > /etc/resolv.conf ||
      die "the shared resolver held no usable line, so closing /mnt/wsl would leave this base unable to resolve anything"
    chmod 0644 /etc/resolv.conf
  elif [ -f /etc/resolv.conf ] && [ ! -L /etc/resolv.conf ] && grep -q '^nameserver ' /etc/resolv.conf; then
    say "kept the real /etc/resolv.conf this base already had"
  else
    die "base.shared_tmpfs is off and there is no resolver to keep: /mnt/wsl/resolv.conf is absent or empty and /etc/resolv.conf carries no nameserver"
  fi
  resolvers=$(grep -c '^nameserver ' /etc/resolv.conf)
  say "wrote /etc/resolv.conf, $resolvers nameserver(s), before closing the shared tmpfs"
  # <<< the shared resolver: end

  RESOLV_BLOCK='
[network]
generateResolvConf=false'
else
  RESOLV_BLOCK=''
fi

# -- the boot script, WSL-68 and WSL-178 ---------------------------------------
# ⭐ THE BOOT COMMAND NAMES A SCRIPT AND CARRIES NO LOGIC. Measured on
# 2026-09-17: a `command=` whose value held nested quotes did not run at all,
# silently, and the only evidence was a marker file that never appeared. A path
# has nothing for wsl.conf's parser to get wrong.
#
# ⛔ EVERY BASE HAS ONE NOW, NOT ONLY A SEALED ONE. The engine's run state lives
# on the disk and names the boot it was written in, so every restart of WSL's VM
# left podman refusing every container until a job cleared it. A base with no
# systemd has no other way to clear it at a start. The seal's half still runs
# only where base.shared_tmpfs is off. WSL-178.
#
# ⚠ THE TWO LINES BEFORE ITS TEXT ARE THIS CONFIGURATION'S, and they are the only
# part provisioning writes into it. The account name was checked before it came.
# The text is boot.sh, handed over as TK_BOOT_B64, which every `base ensure`
# hands engines.sh too. WSL-178.
# >>> the boot script's install: begin
BOOT_SCRIPT=/usr/local/lib/wsl-toolkit/boot.sh
install -d -m 0755 /usr/local/lib/wsl-toolkit
{
  printf '#!/bin/sh\nseal=%s\naccount=%s\n' "$TK_SHARED_TMPFS" "$TK_USER"
  printf '%s' "$TK_BOOT_B64" | base64 -d
} > "$BOOT_SCRIPT.new" || die "could not write $BOOT_SCRIPT"
grep -qx '# <<< the boot script: end' "$BOOT_SCRIPT.new" ||
  { rm -f "$BOOT_SCRIPT.new"; die "the boot script handed to provisioning was cut short"; }
chmod 0755 "$BOOT_SCRIPT.new" || die "could not make $BOOT_SCRIPT executable"
mv -f "$BOOT_SCRIPT.new" "$BOOT_SCRIPT" || die "could not install $BOOT_SCRIPT"
BOOT_LINE="command=$BOOT_SCRIPT"
say "installed $BOOT_SCRIPT, run at every start"
# <<< the boot script's install: end

# -- how WSL starts this distribution -----------------------------------------

# appendWindowsPath=false is the half that answers the reported complaint. With
# it left on, every Windows PATH entry is appended to the guest's, so a guest
# `find` on PATH can resolve to a Windows executable and a tool an agent
# installed inside is shadowed by one it did not.
# ⛔ WRITTEN BESIDE IT AND COMPARED, so the executable restarts the base only when
# this file changed. Every re-provision used to end in a restart, which stopped
# every job and agent in the base over settings that had not moved. WSL-175.
cat > /etc/wsl.conf.wsl-toolkit.new <<CONF
# Written by wsl-toolkit.
[user]
default=$TK_USER

[boot]
systemd=$SYSTEMD_ENABLED
$BOOT_LINE

[automount]
$AUTOMOUNT_BLOCK
mountFsTab=true

[interop]
enabled=$INTEROP_ENABLED
appendWindowsPath=false
$RESOLV_BLOCK
CONF
# >>> install_changed: begin
# install_changed NEW DEST: DEST becomes NEW unless it already holds the same bytes,
# and the answer is `changed` or `unchanged`. ⚠ The x keeps a trailing newline
# that command substitution would strip, so two files differing only there differ.
# ⛔ EACH STEP RETURNS ITS OWN FAILURE. The caller runs this inside `$(...) ||`,
# where bash, the default base's /bin/sh, suspends errexit, so a failed mv went on
# to say `changed` over a file it never wrote. WSL-175.
install_changed() {
  if [ -f "$2" ] && [ "$(cat "$1"; printf x)" = "$(cat "$2"; printf x)" ]; then
    rm -f "$1" || return 1
    printf 'unchanged\n'
    return 0
  fi
  mv "$1" "$2" || return 1
  printf 'changed\n'
}
# <<< install_changed: end
wsl_conf=$(install_changed /etc/wsl.conf.wsl-toolkit.new /etc/wsl.conf) ||
  die "/etc/wsl.conf could not be written"
say "/etc/wsl.conf: $wsl_conf"
printf 'wsl-conf %s\n' "$wsl_conf"
# ⛔ THE NAME AN EARLIER RELEASE GAVE THE BOOT SCRIPT GOES ONLY NOW, once the
# wsl.conf that names boot.sh is in place. Removed before it, a provisioning that
# died between the two left a sealed base naming a script that was gone, and its
# next start kept /mnt/wsl mounted with nothing logged. WSL-178.
rm -f /usr/local/lib/wsl-toolkit/seal-boot.sh

# -- the drive mount points automount leaves behind ---------------------------
# ⛔ NOT HERE ANY MORE. drives.sh removes them after the restart that applies
# `automount off`. Swept here, a drive mounted after the sweep was left behind,
# and every session before the restart relayed one `wsl: Failed to translate`
# line per Windows PATH entry. Findings 13 and 18, WSL-153 and WSL-155.

printf 'provision-complete\n'
