#!/bin/sh
# herdr adapter, private network: the namespace herdr's server, and so every pane and
# agent it starts, runs in when base.adapters gives herdr "private_net": true. WSL-159.
#
# THE DEFECT IT EXISTS TO REMOVE: `base exec --private-net` confines one command, and
# an agent herdr starts is a child of herdr's server, so no flag on a command reaches
# it. This puts the server itself in a namespace. The internet stays reachable there,
# and the Windows host and the private ranges are refused, as that flag's wrapper does.
#
# ⭐ ROOT MAKES THIS NAMESPACE, AND NOT THE ACCOUNT. The flag's wrapper runs pasta as
# the account, which puts the payload in a user namespace where it is uid 0. Every
# agent wrapper refuses to run there, because each runs only as the account by name.
# And uid 0 can delete the very rules that refuse the host. Here the namespace belongs
# to the base's own user namespace. So herdr and its agents keep the account's uid,
# and the account holds no privilege over the rules. Measured on an Arch base: the
# account's `nft flush ruleset` in here answers `Operation not permitted`.
#
# It runs as root from wsl-toolkit-herdr-net.service, which the herdr server's unit
# requires while the key is on. `start` makes the namespace when it is not there,
# writes the rules into it whole, and starts pasta. pasta links it to the shared
# namespace and forks once it has. `stop` removes the namespace once no process is
# left in it. `rules` writes the rules into a namespace that is there and touches
# nothing else. install.sh runs it when this file changes under a running server.
set -eu
umask 022

NETNS=wsl-toolkit-herdr
# The three paths a case in the suite points into a temporary directory, and that
# nothing else sets.
NETNS_DIR=${TK_NETNS_DIR:-/run/netns}
PIDFILE=${TK_HERDR_NET_PIDFILE:-/run/wsl-toolkit-herdr-net.pid}
RESOLV=${TK_RESOLV_CONF:-/etc/resolv.conf}

die() { printf 'herdr private network: %s\n' "$*" >&2; exit 3; }

case ${1:-} in
  start | rules) ;;
  stop)
    # ⚠ ONLY AN EMPTY NAMESPACE GOES. pasta stops on its own when it fails, and the
    # herdr server is still in the namespace then. Removing the name would leave that
    # server where a new pasta cannot reach it, so the next start relinks it instead.
    [ -e "$NETNS_DIR/$NETNS" ] || exit 0
    if [ -z "$(ip netns pids "$NETNS" 2>/dev/null)" ]; then
      ip netns delete "$NETNS"
    fi
    exit 0
    ;;
  *) die "usage: start | stop | rules" ;;
esac
mode=$1

for tool in ip nft pasta; do
  command -v "$tool" >/dev/null 2>&1 ||
    die "$tool is not installed in this base, and the namespace needs it. Run base ensure"
done

if [ "$mode" = rules ]; then
  [ -e "$NETNS_DIR/$NETNS" ] || die "there is no namespace $NETNS to write the rules into"
else
  [ -e "$NETNS_DIR/$NETNS" ] || ip netns add "$NETNS" ||
    die "ip netns add $NETNS failed"
fi

# ⛔ THE RULES ARE WRITTEN WHOLE ON EVERY START, in one nft transaction. The first two
# lines make the table exist and then remove it. So the third defines it from nothing,
# and a rule an earlier start wrote cannot outlive a change here.
#
# ⛔ THE RESOLVER IS EXCEPTED FOR DNS ALONE, AND BEFORE THE DROPS. An nftables chain
# runs in order, and WSL's resolver lives at a 10/8 address. So a drop of the ranges
# ahead of it takes DNS with it. ⚠ Port 53 only, where `base exec --private-net`
# accepts the address whole: a WSL in NAT mode without DNS tunnelling names the
# Windows host itself as the resolver, and accepting that address whole opens the host.
#
# ⭐ THE GATEWAY IS READ HERE, AT EVERY START. In NAT mode it is the Windows host, and
# its address moves when WSL's VM restarts, which is also when this runs again.
#
# ⛔ 100.64.0.0/10 IS DROPPED WITH THE PRIVATE RANGES. No public network routes that
# shared address space. Tailscale gives the Windows host an address there, and the
# host answers on it through WSL's NAT. WSL-159.
rules=$(mktemp)
trap 'rm -f "$rules"' 0 1 2 15
{
  printf 'table inet wsl_toolkit_private\n'
  printf 'delete table inet wsl_toolkit_private\n'
  printf 'table inet wsl_toolkit_private {\n'
  printf '  chain out {\n'
  printf '    type filter hook output priority 0; policy accept;\n'
  sed -n 's/^nameserver[[:space:]][[:space:]]*\([0-9][0-9.]*\)[[:space:]]*$/\1/p' "$RESOLV" 2>/dev/null |
    while IFS= read -r ns; do
      printf '    ip daddr %s udp dport 53 accept\n' "$ns"
      printf '    ip daddr %s tcp dport 53 accept\n' "$ns"
    done
  ip -4 route show default 2>/dev/null |
    sed -n 's/.* via \([0-9][0-9.]*\).*/\1/p' |
    while IFS= read -r gw; do
      printf '    ip daddr %s drop\n' "$gw"
    done
  printf '    ip daddr { 10.0.0.0/8, 100.64.0.0/10, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16 } drop\n'
  printf '  }\n'
  printf '}\n'
} > "$rules"
if ! ip netns exec "$NETNS" nft -f "$rules"; then
  [ "$mode" = start ] ||
    die "the rules could not be written into the running namespace, so it keeps the rules it had"
  die "the rules that refuse the Windows host could not be written, so pasta was not started and the namespace has no way out"
fi
rm -f "$rules"
trap - 0 1 2 15
# ⚠ rules STOPS HERE. pasta keeps the link it made, and every process in the namespace
# stays where it is.
[ "$mode" = start ] || exit 0

# ⭐ ping ANSWERS WHAT THE RULES ALLOW. A new namespace allows ICMP echo sockets to no
# group. NoNewPrivileges takes ping's own capability away. So without this line, every
# ping in a pane fails for a reason that is not the rules.
ip netns exec "$NETNS" sysctl -q -w net.ipv4.ping_group_range='0 2147483647' >/dev/null 2>&1 || :

# ⛔ EACH FLAG CLOSES A WAY PAST THE RULES, read from pasta's own --help:
#   -4                    IPv4 alone. The rules are IPv4 rules, so an IPv6 path would
#                         pass them by.
#   -t -u -T -U none      pasta forwards nothing in either direction. Its default,
#                         auto, binds in here every port bound in the shared
#                         namespace. A connection to this namespace's own loopback
#                         then reached any distribution's local service.
#   --no-map-gw           the gateway address stays the gateway, and is not mapped to
#                         the shared namespace's loopback.
#   --runas 0:0           ⚠ pasta stays root. Started as root it drops to nobody, and
#                         nobody cannot enter a namespace root made: measured, it
#                         answered `Couldn't switch to pasta namespaces: Operation not
#                         permitted`. Rootful podman runs pasta as root the same way.
# It forks once the link is up, which is when systemd takes the unit as started.
exec pasta --quiet -4 --config-net --netns "$NETNS_DIR/$NETNS" \
  -t none -u none -T none -U none --no-map-gw --runas 0:0 --pid "$PIDFILE"
