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

: "${TK_USER:?TK_USER is required}"
: "${TK_UID:?TK_UID is required}"
: "${TK_BINFMT_IMAGE:?TK_BINFMT_IMAGE is required}"
: "${TK_AUTOMOUNT:?TK_AUTOMOUNT is required}"
: "${TK_INTEROP:?TK_INTEROP is required}"
: "${TK_SYSTEMD:?TK_SYSTEMD is required}"
: "${TK_SHARED_TMPFS:?TK_SHARED_TMPFS is required}"
: "${TK_PASSWORDLESS_SUDO:?TK_PASSWORDLESS_SUDO is required}"
: "${TK_TOOLSET:?TK_TOOLSET is required}"

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
    # ⛔ nftables SUPPLIES nft, AND netavark SHELLS OUT TO IT BY THAT NAME.
    # Measured on this host on 2026-09-15: a Debian base carrying no firewall
    # package refused the QEMU binary-format installer's rootful run with
    # `netavark: nftables error: unable to execute nft: No such file or
    # directory`, and the build rolled the distribution back. The apk arm carries
    # iptables and the pacman arm iptables-nft; this arm carried neither. WSL-86.
    apt-get install -y -qq --no-install-recommends podman uidmap fuse-overlayfs \
      slirp4netns ca-certificates iproute2 nftables >/dev/null
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
      apk)    apk add --no-cache $developer_packages >/dev/null ;;
      pacman) pacman -S --noconfirm --needed $developer_packages >/dev/null ;;
      apt)    apt-get install -y -qq --no-install-recommends $developer_packages >/dev/null ;;
      dnf)    dnf -y --setopt=install_weak_deps=False install $developer_packages >/dev/null ;;
      tdnf)   tdnf install -y $developer_packages >/dev/null ;;
      xbps)   xbps-install -Sy $developer_packages >/dev/null ;;
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
    apk)    apk add --no-cache "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
    pacman) pacman -S --noconfirm --needed "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
    apt)    apt-get install -y -qq --no-install-recommends "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
    dnf)    dnf -y --setopt=install_weak_deps=False install "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
    tdnf)   tdnf install -y "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
    xbps)   xbps-install -Sy "$CAP_PACKAGE" >/dev/null 2>&1 || : ;;
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
TK_GROUP=$(id -gn "$TK_USER")
chown "$TK_USER:$TK_GROUP" "$TK_HOME"
chmod 0700 "$TK_HOME"
for xdg_dir in "$TK_HOME/.config" "$TK_HOME/.cache" "$TK_HOME/.local" "$TK_HOME/.local/share" "$TK_HOME/.local/state"; do
  mkdir -p "$xdg_dir"
  chown "$TK_USER:$TK_GROUP" "$xdg_dir"
  chmod 0700 "$xdg_dir"
done
say "account home: $TK_HOME; persistent XDG directories: ready"

# -- optional agent privilege escalation -------------------------------------
# This is deliberately a tool-owned drop-in: turning the profile setting off
# removes only the authority this tool created. Validate the candidate before
# its atomic move so a malformed rule never becomes active.
sudoers_path=/etc/sudoers.d/wsl-toolkit-$TK_USER
case "$TK_PASSWORDLESS_SUDO" in
  true)
    case "$FAMILY" in
      apk)    apk add --no-cache sudo >/dev/null ;;
      pacman) pacman -S --noconfirm --needed sudo >/dev/null ;;
      apt)    apt-get install -y -qq --no-install-recommends sudo >/dev/null ;;
      dnf)    dnf -y --setopt=install_weak_deps=False install sudo >/dev/null ;;
      tdnf)   tdnf install -y sudo >/dev/null ;;
      xbps)   xbps-install -Sy sudo >/dev/null ;;
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
SEAL_BOOT=/usr/local/lib/wsl-toolkit/seal-boot.sh
if [ "$TK_SHARED_TMPFS" = off ]; then
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

  # ⭐ THE BOOT COMMAND NAMES A SCRIPT AND CARRIES NO LOGIC. Measured on
  # 2026-09-17: a `command=` whose value held nested quotes did not run at all,
  # silently, and the only evidence was a marker file that never appeared. A path
  # has nothing for wsl.conf's parser to get wrong.
  install -d -m 0755 /usr/local/lib/wsl-toolkit
  cat > "$SEAL_BOOT" <<'BOOTSH'
#!/bin/sh
# Written by wsl-toolkit. WSL runs this as root at every start of this
# distribution, named by `command` in /etc/wsl.conf's [boot] section.
#
# ⛔ THE RESOLVER IS REFRESHED BEFORE THE UNMOUNT, AND A BAD READ CHANGES
# NOTHING. The shared file is not always populated this early - measured empty on
# 2026-09-17 - so a refresh that comes back short leaves the working file alone.
# A resolver that works beats a fresher one that does not.
set -u
log=/run/wsl-toolkit-seal.log
{
  printf 'seal-boot %s\n' "$(date -u +%FT%TZ 2>/dev/null)"
  if [ -s /mnt/wsl/resolv.conf ]; then
    t=$(mktemp /etc/.resolv.XXXXXX 2>/dev/null) || t=
    if [ -n "$t" ]; then
      if grep -E '^(nameserver|search|options|domain) ' /mnt/wsl/resolv.conf > "$t" 2>/dev/null && [ -s "$t" ]; then
        chmod 0644 "$t" && mv "$t" /etc/resolv.conf && printf 'resolver refreshed\n'
      else
        rm -f "$t"
        printf 'resolver kept, the shared file gave nothing usable\n'
      fi
    fi
  else
    printf 'resolver kept, no shared file to read yet\n'
  fi
  if ! grep -q ' /mnt/wsl ' /proc/mounts 2>/dev/null; then
    printf 'shared tmpfs was not mounted\n'
  elif umount /mnt/wsl 2>/dev/null || umount -l /mnt/wsl 2>/dev/null; then
    printf 'shared tmpfs unmounted\n'
  else
    printf 'shared tmpfs NOT unmounted\n'
  fi
} > "$log" 2>&1
# ⛔ ALWAYS 0. WSL treats a failing boot command as a start it should complain
# about, and a base that will not start is worse than one whose tmpfs is open.
# The log above is where the answer is, and `base doors` is what reads the result.
exit 0
BOOTSH
  chmod 0755 "$SEAL_BOOT"
  SEAL_BOOT_LINE="command=$SEAL_BOOT"
  RESOLV_BLOCK='
[network]
generateResolvConf=false'
  say "installed $SEAL_BOOT, run at every start"
else
  rm -f "$SEAL_BOOT" 2>/dev/null || :
  SEAL_BOOT_LINE=''
  RESOLV_BLOCK=''
fi

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
$SEAL_BOOT_LINE

[automount]
$AUTOMOUNT_BLOCK
mountFsTab=true

[interop]
enabled=$INTEROP_ENABLED
appendWindowsPath=false
$RESOLV_BLOCK
CONF
say "wrote /etc/wsl.conf"

# -- the drive mount points automount leaves behind ---------------------------
# ⛔ AN EMPTY /mnt/c IS NOT AN ABSENT ONE. The first start of a fresh import
# automounts every Windows drive before this script has written `automount off`,
# and the restart that applies the setting leaves each mount point behind as an
# empty 0777 directory. Nothing is reachable through them, but `ls /mnt/c`
# succeeds, the host's drive letters are listed, and guest root has a place ready
# to mount one. Measured on a rebuilt Arch base on 2026-09-13: nine of them.
# ⚠ Only a single-letter directory, only when it is empty, and a live drive
# mount is unmounted first: on a first start this runs before that restart.
if [ "$TK_AUTOMOUNT" = off ]; then
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
  say "drive mount points removed: $drive_points"
fi

printf 'provision-complete\n'
