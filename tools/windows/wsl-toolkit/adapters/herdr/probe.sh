#!/bin/sh
# herdr adapter, probe: what install.sh wrote, read back from the machine.
#
# THE DEFECT IT EXISTS TO CATCH: an adapter reported healthy because its install
# exited 0. A unit that failed after starting, a door whose configuration no longer
# parses, a second key added by hand and a distribution's own sshd listening are all
# invisible to an exit code and visible here.
#
# It runs as root inside the base, delivered on stdin. It prints one fact per line
# as `name value`, and `problem TEXT` for each thing that is wrong, and exits 0
# whatever it finds: the lines are the answer.
set -u
: "${TK_USER:?TK_USER is required}"

problem() { printf 'problem %s\n' "$*"; }

HERDR_BIN=/usr/local/bin/herdr
DOOR_DIR=/etc/wsl-toolkit/ssh
DOOR_WRAPPER=/usr/local/lib/wsl-toolkit/sshd-stdio
UNIT=wsl-toolkit-herdr.service

if [ -x "$HERDR_BIN" ]; then
  version=$("$HERDR_BIN" --version 2>/dev/null | sed -n 's/^herdr //p')
  printf 'version %s\n' "${version:-unknown}"
  # ⚠ THE DIGEST AND THE RELEASE, because a build of herdr's development branch answers
  # the same --version as the release it follows.
  printf 'sha256 %s\n' "$(sha256sum "$HERDR_BIN" | cut -d' ' -f1)"
  release=$(cat /usr/local/lib/wsl-toolkit/herdr-release 2>/dev/null || :)
  printf 'release %s\n' "${release:-unknown}"
else
  problem "herdr is not installed at $HERDR_BIN"
fi

state=$(systemctl is-active "$UNIT" 2>/dev/null || :)
printf 'server %s\n' "${state:-unknown}"
[ "$state" = active ] || problem "the herdr server unit $UNIT is ${state:-unknown}"
# ⚠ THE UNIT BEING ACTIVE IS NOT THE SERVER ANSWERING, so the account asks it.
answer=$(runuser -u "$TK_USER" -- "$HERDR_BIN" status server 2>/dev/null | sed -n 's/^status: //p')
printf 'server-answers %s\n' "${answer:-nothing}"
[ "$answer" = running ] || problem "herdr status server answers ${answer:-nothing} for $TK_USER"
# ⚠ A FACT AND NOT A PROBLEM. base ensure never restarts a running server, so a newer
# binary serves nothing until the server next starts, and herdr says whether that is so.
stale=$(runuser -u "$TK_USER" -- "$HERDR_BIN" status 2>/dev/null | sed -n 's/^[[:space:]]*server_binary_stale: //p')
printf 'server-binary-stale %s\n' "${stale:-unknown}"

if [ -r "$DOOR_DIR/ssh_host_ed25519_key.pub" ]; then
  printf 'ssh-host-key %s\n' "$(cut -d' ' -f1,2 "$DOOR_DIR/ssh_host_ed25519_key.pub")"
else
  problem "the SSH door has no host key"
fi
[ -x "$DOOR_WRAPPER" ] || problem "the SSH door's wrapper $DOOR_WRAPPER is missing"
sshd -t -f "$DOOR_DIR/sshd_config" >/dev/null 2>&1 || problem "the SSH door's sshd configuration does not pass sshd -t"
keys=$(grep -c . "$DOOR_DIR/authorized_keys" 2>/dev/null || :)
printf 'ssh-client-keys %s\n' "${keys:-0}"
[ "${keys:-0}" = 1 ] || problem "the SSH door accepts ${keys:-0} keys, and it must accept the herdr client's one"
# ⚠ WSL2 DISTRIBUTIONS SHARE ONE NETWORK NAMESPACE, so a listener without a process
# name here may be another distribution's. What this base promises is that no sshd
# of its own listens.
listeners=$(ss -Htlnp 2>/dev/null | grep -c '"sshd' || :)
printf 'ssh-listeners %s\n' "${listeners:-0}"
[ "${listeners:-0}" = 0 ] || problem "an sshd in this base listens on a TCP port"
for unit in sshd.service sshd.socket; do
  unit_state=$(systemctl is-enabled "$unit" 2>/dev/null || :)
  [ "$unit_state" = masked ] || problem "$unit is ${unit_state:-unknown}, and it must be masked"
done
