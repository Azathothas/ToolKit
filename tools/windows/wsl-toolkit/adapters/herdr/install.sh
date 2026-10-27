#!/bin/sh
# herdr adapter, install: herdr in the base, its server as a system unit, the
# tracked configuration for the account, and an SSH door that listens on nothing.
#
# THE DEFECT IT EXISTS TO REMOVE: a base whose agents run in a multiplexer that
# was set up by hand, and a Windows client that reaches it through a port any
# local process could reach. Everything here is written by this file and read back
# by probe.sh.
#
# It runs as root inside a wsl-toolkit base during `base ensure`, delivered on
# stdin, and every step looks before it changes anything, so a second run changes
# nothing. TK_USER, TK_DISTRO, TK_SSH_CLIENT_KEY and TK_FILE_CONFIG_TOML_B64 arrive
# as exported variables rather than as text substituted into this file.
set -eu
umask 022
cd /

: "${TK_USER:?TK_USER is required}"
: "${TK_DISTRO:?TK_DISTRO is required}"
: "${TK_SSH_CLIENT_KEY:?TK_SSH_CLIENT_KEY is required}"
: "${TK_FILE_CONFIG_TOML_B64:?TK_FILE_CONFIG_TOML_B64 is required}"

say() { printf '  * %s\n' "$*"; }
die() { printf 'herdr adapter: %s\n' "$*" >&2; exit 3; }
# same_file A B is true when B exists and holds A's bytes. ⚠ NOT cmp: cmp is
# diffutils, which a base with no toolset does not carry, and a missing cmp read
# as "the files differ" on every run. WSL-105.
same_file() {
  [ -f "$2" ] || return 1
  [ "$(sha256sum < "$1")" = "$(sha256sum < "$2")" ]
}

# -- what this adapter pins, and how a configuration moves it ------------------
# ⛔ PINNED BY VERSION AND VERIFIED BY DIGEST BEFORE USE, and a file that does not
# match is deleted without being run. The release is herdr 0.9.5 as this repository's
# wsl-toolkit-v7.0.0 publishes it: built by herdr-build.yml from herdr's v0.9.5 tag,
# and each digest is the line that release's own SHA256SUMS carries. WSL-141.
# ⚠ SO IT IS DOWNLOADED FROM THAT RELEASE, under the name release.yml gives it. herdr's
# own aarch64 asset is a different build, and its digest is not this one.
HERDR_VERSION=0.9.5
HERDR_X86_64_PINNED_SHA256=14d9e8bc8b4a9e0118d77c32266f76d461f09b40342eb23752960bd89f59929d
HERDR_AARCH64_PINNED_SHA256=c9b4b513f1e86426acf2313db2a8a0987899217ee45cc437e047e93a2da1e21e
HERDR_URL_BASE=https://github.com/Azathothas/ToolKit/releases/download/wsl-toolkit-v7.0.0
HERDR_ASSET_STEM=herdr-$HERDR_VERSION

# ⭐ A NEWER RELEASE IS A CONFIGURATION CHANGE, NOT AN EDIT TO THIS FILE. `version`
# and `sha256` on this adapter's entry in base.adapters arrive as TK_ADAPTER_VERSION
# and TK_ADAPTER_SHA256_<ARCH>, and the executable refuses one without the other, so
# a moved version always brings its own digests. ⛔ An architecture the configuration
# does not name is refused rather than falling back to a digest for a release this is
# no longer installing.
# ⛔ A CONFIGURED VERSION IS herdr's OWN RELEASE, under herdr's own asset names, as it
# has been since the field existed. Only the default comes from this repository.
if [ -n "${TK_ADAPTER_VERSION:-}" ]; then
  HERDR_VERSION=$TK_ADAPTER_VERSION
  HERDR_X86_64_PINNED_SHA256=${TK_ADAPTER_SHA256_X86_64:-}
  HERDR_AARCH64_PINNED_SHA256=${TK_ADAPTER_SHA256_AARCH64:-}
  HERDR_URL_BASE=https://github.com/herdrdev/herdr/releases/download/v$HERDR_VERSION
  HERDR_ASSET_STEM=herdr
fi

# ⭐ A NIGHTLY COMES FROM THIS REPOSITORY'S OWN RELEASES. For the `nightly` channel the
# executable resolves the newest herdr-nightly-* prerelease and passes its tag as the
# version, each digest from that release's SHA256SUMS, and its download base; the digest
# check below does not change. ⛔ Only a GitHub release download, and only beside the
# version it belongs to. WSL-90.
if [ -n "${TK_ADAPTER_URL_BASE:-}" ]; then
  [ -n "${TK_ADAPTER_VERSION:-}" ] ||
    die "a download base arrived without the version it belongs to"
  case $TK_ADAPTER_URL_BASE in
    https://github.com/*/releases/download/*) HERDR_URL_BASE=$TK_ADAPTER_URL_BASE ;;
    *) die "the download base $TK_ADAPTER_URL_BASE is not a GitHub release download" ;;
  esac
fi

# The one path this checks and writes the download at. A case in the suite points it
# into a temporary directory, and nothing else sets it.
HERDR_BIN=${TK_HERDR_BIN:-/usr/local/bin/herdr}
# The two directories this writes its own files and its units in. A case in the suite
# points each into a temporary directory, and nothing else sets them. WSL-159.
LIB_DIR=${TK_HERDR_LIB_DIR:-/usr/local/lib/wsl-toolkit}
UNIT_DIR=${TK_HERDR_UNIT_DIR:-/etc/systemd/system}
# The release the installed binary came from, which probe.sh reports and this machine's
# half reads to fetch the Windows client of the same build.
HERDR_RELEASE_FILE=$LIB_DIR/herdr-release
DOOR_DIR=/etc/wsl-toolkit/ssh
DOOR_WRAPPER=$LIB_DIR/sshd-stdio
UNIT=wsl-toolkit-herdr.service
# What base.adapters' private_net adds, and removes when it goes. WSL-159.
NET_UNIT=wsl-toolkit-herdr-net.service
NET_SCRIPT=$LIB_DIR/herdr-private-net
NET_DROPIN=$UNIT_DIR/$UNIT.d/private-net.conf
# The namespace's name, and the directory `ip netns` keeps it in. A case in the suite
# points the directory into a temporary one, and nothing else sets it.
NETNS=wsl-toolkit-herdr
NETNS_DIR=${TK_NETNS_DIR:-/run/netns}

# ⛔ MEASURED ON THE ARCH PRESET ONLY, AND ANYTHING ELSE IS REFUSED. The package
# that carries sshd, whether installing it starts a listener, and whether its sshd
# honours PAM all differ by distribution, and none of the others was driven.
command -v pacman >/dev/null 2>&1 ||
  die "this adapter is measured on the arch preset, and this base has no pacman. Build the base from the arch preset"

pid_one=$(cat /proc/1/comm 2>/dev/null || echo unknown)
[ "$pid_one" = systemd ] ||
  die "herdr's server runs as a system unit, and PID 1 here is $pid_one. Set base.systemd to true"

TK_HOME=$(getent passwd "$TK_USER" | cut -d: -f6)
if [ -z "$TK_HOME" ] || [ ! -d "$TK_HOME" ]; then
  die "the account $TK_USER has no home directory"
fi
TK_GROUP=$(id -gn "$TK_USER")

# as_account runs a command as the account, with the environment a fresh shell of its
# own would have and nothing of this script's.
as_account() {
  runuser -u "$TK_USER" -- env -i HOME="$TK_HOME" USER="$TK_USER" LOGNAME="$TK_USER" \
    SHELL=/bin/bash LANG=C.UTF-8 PATH=/usr/local/sbin:/usr/local/bin:/usr/bin:/bin "$@"
}

# >>> the account's own writes: begin
# ⛔ ROOT WRITES NOTHING IN THE ACCOUNT'S HOME. This script runs as root, and chown,
# install -d, cp and a redirect each follow a link the account placed there. Where
# base.passwordless_sudo is off, or in a herdr pane under private_net, the account is
# not root. Root's write through that link was one the account could not make. As the
# account, a link reaches only what the account could already write. WSL-201.
#
# account_dirs MODE DIR... makes each DIR as the account and gives it MODE. ⚠ A DIR that
# is a link keeps the mode of what it names, which is the account's own to set.
account_dirs() {
  # shellcheck disable=SC2016  # the single quotes hold the account's own script
  as_account sh -c '
    mode=$1
    shift
    for dir do
      mkdir -p "$dir" || exit 1
      [ -L "$dir" ] || chmod "$mode" "$dir" || exit 1
    done' sh "$@"
}

# account_put DEST MODE writes its standard input to DEST as the account. ⭐ A file of
# the account's own beside DEST takes the bytes and is renamed over it, so a link at
# DEST is replaced and never written through. ⛔ A directory at DEST is refused.
account_put() {
  # shellcheck disable=SC2016  # the single quotes hold the account's own script
  as_account sh -c '
    [ ! -d "$1" ] || exit 1
    tmp=$(mktemp "${1%/*}/.wsl-toolkit.XXXXXX") || exit 1
    if cat > "$tmp" && chmod "$2" "$tmp" && mv -f "$tmp" "$1"; then
      exit 0
    fi
    rm -f "$tmp"
    exit 1' sh "$1" "$2"
}
# <<< the account's own writes: end

# -- the packages ----------------------------------------------------------------
missing=
for tool in sshd ssh-keygen curl sha256sum ss; do
  command -v "$tool" >/dev/null 2>&1 || missing="$missing $tool"
done
if [ -n "$missing" ]; then
  say "installing what the door and the download need:$missing"
  # ⛔ -Syu, NOT -S. A bare -S installs from a package database as old as the
  # rootfs, which a mirror may no longer serve, or into a partial upgrade. WSL-188.
  pacman -Syu --noconfirm --needed openssh curl coreutils iproute2 >/dev/null
fi
for tool in sshd ssh-keygen curl sha256sum ss; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is still absent after the package step"
done

# ⛔ THE DISTRIBUTION'S OWN LISTENER IS MASKED, NOT MERELY LEFT DISABLED. The
# operator's ruling is that nothing in the base accepts a connection but the herdr
# client's key through wsl.exe, and a disabled unit is one `systemctl enable` away
# from a port.
for unit in sshd.service sshd.socket; do
  if [ "$(systemctl is-enabled "$unit" 2>/dev/null || :)" != masked ]; then
    systemctl mask --now "$unit" >/dev/null 2>&1 || :
  fi
done

# -- herdr ------------------------------------------------------------------------
case "$(uname -m)" in
  x86_64)  asset=$HERDR_ASSET_STEM-linux-x86_64;  want=$HERDR_X86_64_PINNED_SHA256 ;;
  aarch64) asset=$HERDR_ASSET_STEM-linux-aarch64; want=$HERDR_AARCH64_PINNED_SHA256 ;;
  *) die "herdr publishes no Linux build for $(uname -m)" ;;
esac
# ⛔ AN EMPTY DIGEST IS A REFUSAL, NOT AN UNCHECKED DOWNLOAD. It means a
# configuration moved the version and gave no digest for THIS architecture, and
# checking a new release against the old one's digest would be worse than either.
[ -n "$want" ] ||
  die "base.adapters moves herdr to $HERDR_VERSION and names no sha256 for $(uname -m). Add it, or remove the version to use the one this adapter pins"
have=
[ -f "$HERDR_BIN" ] && have=$(sha256sum "$HERDR_BIN" | cut -d' ' -f1)
if [ "$have" = "$want" ]; then
  say "herdr $HERDR_VERSION is installed and matches its pinned digest"
else
  tmp=$(mktemp "$HERDR_BIN.XXXXXX")
  trap 'rm -f "$tmp"' 0 1 2 15
  curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location \
    "$HERDR_URL_BASE/$asset" --output "$tmp" ||
    die "downloading $asset failed"
  got=$(sha256sum "$tmp" | cut -d' ' -f1)
  [ "$got" = "$want" ] || die "$asset does not match its pinned digest: expected $want, measured $got. It was deleted and not run"
  chmod 0755 "$tmp"
  mv "$tmp" "$HERDR_BIN"
  trap - 0 1 2 15
  say "installed herdr $HERDR_VERSION from $asset, digest verified"
  # ⚠ A RUNNING SERVER KEEPS THE herdr IT STARTED WITH. This file never restarts it, as
  # the server step below says, so the release just installed serves nothing until the
  # server next starts, and the ensure says so. WSL-141.
  if systemctl is-active --quiet "$UNIT" 2>/dev/null; then
    say "the running herdr server keeps the herdr it started with until it restarts, which ends every pane. When no pane is needed, run: base exec --root -c 'systemctl restart $UNIT'"
  fi
fi
install -d -m 0755 "${HERDR_RELEASE_FILE%/*}"
printf '%s\n' "$HERDR_VERSION" > "$HERDR_RELEASE_FILE.tmp"
mv "$HERDR_RELEASE_FILE.tmp" "$HERDR_RELEASE_FILE"

# -- the account's configuration ----------------------------------------------------
# ⭐ THE TRACKED FILE IS THE CONFIGURATION, and it is written whole on every ensure.
# ⛔ THE ACCOUNT WRITES IT, from a copy root decoded outside the home. Root's temporary
# file sat in a directory the account owns, so the account could put a link in its
# place before root's redirect and chown reached it. WSL-201.
account_dirs 0700 "$TK_HOME/.config" "$TK_HOME/.config/herdr" ||
  die "the account could not make $TK_HOME/.config/herdr"
config_tmp=$(mktemp)
encoded=$(mktemp)
printf '%s' "$TK_FILE_CONFIG_TOML_B64" > "$encoded"
if ! base64 -d "$encoded" > "$config_tmp"; then
  rm -f "$encoded" "$config_tmp"
  die "the tracked herdr configuration could not be decoded"
fi
rm -f "$encoded"
if ! account_put "$TK_HOME/.config/herdr/config.toml" 0644 < "$config_tmp"; then
  rm -f "$config_tmp"
  die "the account could not write $TK_HOME/.config/herdr/config.toml"
fi
rm -f "$config_tmp"
say "wrote the tracked herdr configuration"

# -- the server ---------------------------------------------------------------------
unit_tmp=$(mktemp)
cat > "$unit_tmp" <<UNIT
# Written by wsl-toolkit's herdr adapter. base ensure rewrites it.
[Unit]
Description=herdr server for $TK_USER, written by wsl-toolkit
After=network-online.target

[Service]
Type=simple
User=$TK_USER
Group=$TK_GROUP
WorkingDirectory=$TK_HOME
Environment=HOME=$TK_HOME SHELL=/bin/bash LANG=C.UTF-8
ExecStart=$HERDR_BIN server
Restart=on-failure

[Install]
WantedBy=multi-user.target
UNIT
if same_file "$unit_tmp" "$UNIT_DIR/$UNIT"; then
  rm -f "$unit_tmp"
else
  mv "$unit_tmp" "$UNIT_DIR/$UNIT"
  chmod 0644 "$UNIT_DIR/$UNIT"
  systemctl daemon-reload
  say "wrote $UNIT_DIR/$UNIT"
fi

# -- the private network namespace -----------------------------------------------------
# ⭐ base.adapters' private_net PUTS THE SERVER, AND SO EVERY AGENT IT STARTS, IN A
# NETWORK NAMESPACE OF ITS OWN. The internet stays reachable, and the Windows host and
# the private ranges are refused. private-net.sh says how root makes it. WSL-159.
#
# ⭐ THE SERVER'S OWN UNIT ABOVE IS THE SAME BYTES EITHER WAY. The key adds a drop-in
# beside it, a unit that links the namespace, and the script that unit runs. A base
# without the key has none of the three, and one whose key went has all three removed.
#
# ⛔ IT COSTS SOMETHING, AND EVERY READ SAYS WHAT. Those agents give up rootless
# containers and sudo. base status prints it on a line of its own, and probe.sh as a
# note.
net_changed=
# ⚠ THE DROP-IN ALONE DECIDES WHERE THE SERVER'S NEXT START IS, so only a change to it
# is one the running server waits for.
dropin_changed=
# ⚠ A CHANGED SCRIPT CARRIES CHANGED RULES, and a running namespace takes them below.
script_changed=
# place_file NAME_B64 DEST MODE writes one carried file where it belongs, when its
# bytes differ, and sets pf_placed when it wrote it. ⚠ It always answers 0, so set -e
# stays in force inside it: a function called as a condition runs without it.
place_file() {
  pf_placed=
  pf_tmp=$(mktemp)
  pf_encoded=$(mktemp)
  printf '%s' "$1" > "$pf_encoded"
  if ! base64 -d "$pf_encoded" > "$pf_tmp"; then
    rm -f "$pf_encoded" "$pf_tmp"
    die "$2 could not be decoded"
  fi
  rm -f "$pf_encoded"
  if same_file "$pf_tmp" "$2"; then
    rm -f "$pf_tmp"
    return 0
  fi
  install -d -m 0755 "${2%/*}"
  chmod "$3" "$pf_tmp"
  mv "$pf_tmp" "$2"
  net_changed=1
  pf_placed=1
  say "wrote $2"
}
if [ "${TK_ADAPTER_PRIVATE_NET:-}" = 1 ]; then
  : "${TK_FILE_PRIVATE_NET_SH_B64:?TK_FILE_PRIVATE_NET_SH_B64 is required with private_net}"
  : "${TK_FILE_WSL_TOOLKIT_HERDR_NET_SERVICE_B64:?TK_FILE_WSL_TOOLKIT_HERDR_NET_SERVICE_B64 is required with private_net}"
  : "${TK_FILE_PRIVATE_NET_CONF_B64:?TK_FILE_PRIVATE_NET_CONF_B64 is required with private_net}"
  missing=
  for tool in pasta nft ip; do
    command -v "$tool" >/dev/null 2>&1 || missing="$missing $tool"
  done
  if [ -n "$missing" ]; then
    say "installing what the private network namespace needs:$missing"
    # ⛔ -Syu, NOT -S, for the reason the packages step above gives. WSL-188.
    pacman -Syu --noconfirm --needed passt nftables iproute2 >/dev/null
  fi
  for tool in pasta nft ip; do
    command -v "$tool" >/dev/null 2>&1 || die "$tool is still absent after the package step, and private_net needs it"
  done
  place_file "$TK_FILE_PRIVATE_NET_SH_B64" "$NET_SCRIPT" 0755
  script_changed=$pf_placed
  place_file "$TK_FILE_WSL_TOOLKIT_HERDR_NET_SERVICE_B64" "$UNIT_DIR/$NET_UNIT" 0644
  place_file "$TK_FILE_PRIVATE_NET_CONF_B64" "$NET_DROPIN" 0644
  dropin_changed=$pf_placed
else
  # ⭐ THE DROP-IN GOES FIRST AND ALWAYS, so the server's next start is in the shared
  # namespace. ⛔ The namespace's own unit goes only once no process is left in it.
  # Stopping it under a running server would take that server's only way out.
  if [ -e "$NET_DROPIN" ]; then
    rm -f "$NET_DROPIN"
    rmdir "${NET_DROPIN%/*}" 2>/dev/null || :
    # ⛔ RELOADED AT ONCE, BEFORE THE STOP BELOW. Until systemd reads the units again,
    # the running server still requires the namespace's unit. A stop of that unit then
    # stops the server too, and ends every pane.
    systemctl daemon-reload
    dropin_changed=1
    say "removed $NET_DROPIN, because private_net is off"
  fi
  if [ -e "$UNIT_DIR/$NET_UNIT" ] || [ -e "$NET_SCRIPT" ]; then
    if [ -n "$(ip netns pids wsl-toolkit-herdr 2>/dev/null || :)" ]; then
      say "the herdr server's namespace still holds it, so $NET_UNIT stays until the server restarts and the next base ensure"
    else
      systemctl stop "$NET_UNIT" >/dev/null 2>&1 || :
      rm -f "$UNIT_DIR/$NET_UNIT" "$NET_SCRIPT"
      net_changed=1
      say "removed $NET_UNIT and its script, because private_net is off"
    fi
  fi
fi
[ -z "$net_changed" ] || systemctl daemon-reload

# ⭐ A NAMESPACE A PROCESS IS IN IS KEPT CURRENT, AND NO PANE ENDS FOR IT. WSL-159.
# - Its link is started again when it is down. The unit relinks the same namespace,
#   and starting a unit the server requires does not restart the server.
# - A changed script writes its rules into the namespace in place. pasta keeps its
#   link, and the rules otherwise wait for WSL's VM to restart.
if [ "${TK_ADAPTER_PRIVATE_NET:-}" = 1 ] && [ -e "$NETNS_DIR/$NETNS" ]; then
  if [ -n "$(ip netns pids "$NETNS" 2>/dev/null || :)" ] && ! systemctl is-active --quiet "$NET_UNIT"; then
    if systemctl start "$NET_UNIT"; then
      say "started $NET_UNIT again, which links the herdr server's namespace and ends no pane"
    else
      say "$NET_UNIT did not start, so the herdr server's namespace has no way out"
    fi
  elif [ -n "$script_changed" ]; then
    if "$NET_SCRIPT" rules; then
      say "wrote the changed rules into the running namespace $NETNS"
    else
      say "the changed rules did not reach the running namespace $NETNS, which keeps the rules it had until it restarts"
    fi
  fi
fi

systemctl enable "$UNIT" >/dev/null 2>&1 || die "systemctl could not enable $UNIT"
# ⛔ A RUNNING SERVER IS NEVER RESTARTED HERE. Stopping it ends every pane and every
# agent in it, and an ensure run to change something else must not do that.
if systemctl is-active --quiet "$UNIT"; then
  say "the herdr server is running, and is left running"
  # ⚠ SO A CHANGED private_net WAITS FOR THE NEXT START, and the probe says which
  # namespace the server is in until then. WSL-159.
  if [ -n "$dropin_changed" ]; then
    say "the running herdr server keeps the network namespace it started in until it restarts, which ends every pane. When no pane is needed, run: base exec --root -c 'systemctl restart $UNIT'"
  fi
else
  systemctl start "$UNIT" || die "systemctl could not start $UNIT"
  say "started the herdr server"
fi

# -- the SSH door ---------------------------------------------------------------------
# ⭐ NOTHING LISTENS. The client's ProxyCommand starts `sshd -i` through wsl.exe for
# one connection, so the only way in is a process that can already run wsl.exe as
# this Windows account, holding the one key below.
install -d -m 0755 /etc/wsl-toolkit "$DOOR_DIR" "$LIB_DIR"
if [ ! -f "$DOOR_DIR/ssh_host_ed25519_key" ]; then
  ssh-keygen -q -t ed25519 -N '' -C "$TK_DISTRO" -f "$DOOR_DIR/ssh_host_ed25519_key"
  say "generated the door's host key"
fi
door_tmp=$(mktemp)
cat > "$door_tmp" <<CONF
# Written by wsl-toolkit's herdr adapter. base ensure rewrites it.
# sshd reads this only as \`sshd -i\`, started per connection by wsl.exe.
HostKey $DOOR_DIR/ssh_host_ed25519_key
AuthorizedKeysFile $DOOR_DIR/authorized_keys
AllowUsers $TK_USER
AuthenticationMethods publickey
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
PermitEmptyPasswords no
# The account's password field is locked, and sshd without PAM refuses a locked
# account even for a key. PAM's account check does not.
UsePAM yes
AllowTcpForwarding no
AllowStreamLocalForwarding no
AllowAgentForwarding no
X11Forwarding no
PermitTunnel no
GatewayPorts no
PermitUserEnvironment no
PrintMotd no
PidFile none
CONF
mv "$door_tmp" "$DOOR_DIR/sshd_config"
chmod 0644 "$DOOR_DIR/sshd_config"
# ⛔ ONE KEY, AND restrict ON IT. No forwarding, no terminal, no agent, whatever the
# configuration above says, and a key added by hand is replaced on the next ensure.
printf 'restrict %s\n' "$TK_SSH_CLIENT_KEY" > "$DOOR_DIR/authorized_keys"
chmod 0644 "$DOOR_DIR/authorized_keys"
printf '#!/bin/sh\n# Written by wsl-toolkit. One SSH connection on stdin and stdout, listening on nothing.\nexec %s -i -f %s/sshd_config -E /var/log/wsl-toolkit-sshd.log\n' \
  "$(command -v sshd)" "$DOOR_DIR" > "$DOOR_WRAPPER.tmp"
chmod 0755 "$DOOR_WRAPPER.tmp"
mv "$DOOR_WRAPPER.tmp" "$DOOR_WRAPPER"
sshd -t -f "$DOOR_DIR/sshd_config" || die "the door's sshd configuration does not pass sshd -t"
sshd -T -f "$DOOR_DIR/sshd_config" 2>/dev/null | grep -qix 'usepam yes' ||
  die "this base's sshd does not honour UsePAM, so it would refuse the locked account"
say "the SSH door accepts one key, for $TK_USER, and listens on nothing"

printf 'adapter-complete herdr\n'
