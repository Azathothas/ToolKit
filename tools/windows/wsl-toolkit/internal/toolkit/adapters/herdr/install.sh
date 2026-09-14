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

# -- what this adapter pins ----------------------------------------------------
# ⛔ PINNED BY VERSION AND VERIFIED BY DIGEST BEFORE USE. The digests are the ones
# GitHub publishes for the v0.9.0 release assets, read on 2026-09-14, and a file
# that does not match is deleted without being run.
HERDR_VERSION=0.9.0
HERDR_X86_64_PINNED_SHA256=4fa1a01158dd8043da92d31b270780b0dcc10603038d9b61cac4d81ab63fb71f
HERDR_AARCH64_PINNED_SHA256=9c8db20fb7e7427b138d5367113f1621ffd319f2f65d6f009e2594029115f0d2

HERDR_BIN=/usr/local/bin/herdr
DOOR_DIR=/etc/wsl-toolkit/ssh
DOOR_WRAPPER=/usr/local/lib/wsl-toolkit/sshd-stdio
UNIT=wsl-toolkit-herdr.service

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

# -- the packages ----------------------------------------------------------------
missing=
for tool in sshd ssh-keygen curl sha256sum ss; do
  command -v "$tool" >/dev/null 2>&1 || missing="$missing $tool"
done
if [ -n "$missing" ]; then
  say "installing what the door and the download need:$missing"
  pacman -S --noconfirm --needed openssh curl coreutils iproute2 >/dev/null
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
  x86_64)  asset=herdr-linux-x86_64;  want=$HERDR_X86_64_PINNED_SHA256 ;;
  aarch64) asset=herdr-linux-aarch64; want=$HERDR_AARCH64_PINNED_SHA256 ;;
  *) die "herdr publishes no Linux build for $(uname -m)" ;;
esac
have=
[ -f "$HERDR_BIN" ] && have=$(sha256sum "$HERDR_BIN" | cut -d' ' -f1)
if [ "$have" = "$want" ]; then
  say "herdr $HERDR_VERSION is installed and matches its pinned digest"
else
  tmp=$(mktemp "$HERDR_BIN.XXXXXX")
  trap 'rm -f "$tmp"' 0 1 2 15
  curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location \
    "https://github.com/herdrdev/herdr/releases/download/v$HERDR_VERSION/$asset" --output "$tmp" ||
    die "downloading $asset failed"
  got=$(sha256sum "$tmp" | cut -d' ' -f1)
  [ "$got" = "$want" ] || die "$asset does not match its pinned digest: expected $want, measured $got. It was deleted and not run"
  chmod 0755 "$tmp"
  mv "$tmp" "$HERDR_BIN"
  trap - 0 1 2 15
  say "installed herdr $HERDR_VERSION from $asset, digest verified"
fi

# -- the account's configuration ----------------------------------------------------
# ⭐ THE TRACKED FILE IS THE CONFIGURATION, and it is written whole on every ensure.
install -d -o "$TK_USER" -g "$TK_GROUP" -m 0700 "$TK_HOME/.config" "$TK_HOME/.config/herdr"
config_tmp=$(mktemp "$TK_HOME/.config/herdr/config.toml.XXXXXX")
encoded=$(mktemp)
printf '%s' "$TK_FILE_CONFIG_TOML_B64" > "$encoded"
if ! base64 -d "$encoded" > "$config_tmp"; then
  rm -f "$encoded" "$config_tmp"
  die "the tracked herdr configuration could not be decoded"
fi
rm -f "$encoded"
chown "$TK_USER:$TK_GROUP" "$config_tmp"
chmod 0644 "$config_tmp"
mv "$config_tmp" "$TK_HOME/.config/herdr/config.toml"
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
if cmp -s "$unit_tmp" "/etc/systemd/system/$UNIT"; then
  rm -f "$unit_tmp"
else
  mv "$unit_tmp" "/etc/systemd/system/$UNIT"
  chmod 0644 "/etc/systemd/system/$UNIT"
  systemctl daemon-reload
  say "wrote /etc/systemd/system/$UNIT"
fi
systemctl enable "$UNIT" >/dev/null 2>&1 || die "systemctl could not enable $UNIT"
# ⛔ A RUNNING SERVER IS NEVER RESTARTED HERE. Stopping it ends every pane and every
# agent in it, and an ensure run to change something else must not do that.
if systemctl is-active --quiet "$UNIT"; then
  say "the herdr server is running, and is left running"
else
  systemctl start "$UNIT" || die "systemctl could not start $UNIT"
  say "started the herdr server"
fi

# -- the SSH door ---------------------------------------------------------------------
# ⭐ NOTHING LISTENS. The client's ProxyCommand starts `sshd -i` through wsl.exe for
# one connection, so the only way in is a process that can already run wsl.exe as
# this Windows account, holding the one key below.
install -d -m 0755 /etc/wsl-toolkit "$DOOR_DIR" /usr/local/lib/wsl-toolkit
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
