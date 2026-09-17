#!/bin/sh
# Run one payload in a network namespace of the account's own, with the Windows
# host and the private ranges refused by a rule INSIDE that namespace.
#
# ⭐ THE RULE IS NOT IN THE SHARED NAMESPACE, and that is the condition the
# operator's ruling of 2026-09-14 carries. Every WSL distribution on a host shares
# one network namespace, so a rule written there would change the network of the
# podman machine and of every other base. Measured 2026-09-17: inside
# `pasta --config-net` the namespace is a different one, the shared one is
# unchanged before and after, and its ruleset is not even readable to the account.
#
# ⛔ A CONTAINER CANNOT RUN IN HERE, and this is not a detail. `pasta --config-net`
# puts the payload in a USER namespace where it is uid 0, so podman takes itself
# for rootful and chooses system paths it cannot write. Measured three ways on
# 2026-09-17: plain, with --root and --runroot pointed at the account's own
# directories, and with _CONTAINERS_USERNS_CONFIGURED set. Each failed, moving the
# refusal from /var/lib/containers to /run/libpod and back. That is why this is a
# flag on one command and not how every command in the base starts.
#
# It runs as the configured account. TK_PRIVATE_NET_PAYLOAD_B64 carries the
# payload, base64 made by Go, so none of it becomes shell source here.
set -u

: "${TK_PRIVATE_NET_PAYLOAD_B64:?TK_PRIVATE_NET_PAYLOAD_B64 is required}"
: "${TK_HOSTADDR?TK_HOSTADDR is required, and may be empty}"

die() { printf 'private-net: %s\n' "$*" >&2; exit 3; }

command -v pasta >/dev/null 2>&1 ||
  die "pasta is not installed in this base, and it is what makes a network namespace without privilege"
command -v nft >/dev/null 2>&1 ||
  die "nft is not installed in this base, and the refusals are nftables rules"

WORK=$(mktemp -d) || die "no temporary directory"
trap 'rm -rf "$WORK"' 0 1 2 15

# ⛔ DECODED THROUGH A FILE so the exit code is the decoder's and not a pipeline's.
printf '%s' "$TK_PRIVATE_NET_PAYLOAD_B64" > "$WORK/payload.b64"
base64 -d "$WORK/payload.b64" > "$WORK/payload.sh" || die "the payload could not be decoded"

# ⛔ THE RESOLVER IS EXCEPTED BEFORE THE PRIVATE RANGES ARE DROPPED, and the order
# in an nftables chain is the order of the rules. WSL's resolver lives at a 10/8
# address, so a drop of the private ranges with no exception ahead of it takes DNS
# with it, and the payload then fails for a reason that looks nothing like a
# firewall.
{
  printf '#!/bin/sh\nset -u\n'
  printf 'nft add table inet wsl_toolkit_private || exit 3\n'
  printf 'nft add chain inet wsl_toolkit_private out "{ type filter hook output priority 0 ; }" || exit 3\n'
  # Every nameserver this base resolves through stays reachable, named one by one
  # rather than by a range, so nothing else in that range is let through with it.
  sed -n 's/^nameserver  *\([0-9.]*\).*/\1/p' /etc/resolv.conf 2>/dev/null |
    while IFS= read -r ns; do
      [ -n "$ns" ] || continue
      printf 'nft add rule inet wsl_toolkit_private out ip daddr %s accept || exit 3\n' "$ns"
    done
  if [ -n "$TK_HOSTADDR" ]; then
    printf 'nft add rule inet wsl_toolkit_private out ip daddr %s drop || exit 3\n' "$TK_HOSTADDR"
  fi
  printf 'nft add rule inet wsl_toolkit_private out ip daddr "{ 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16 }" drop || exit 3\n'
  # ⭐ THE RULESET IS PRINTED WHEN ASKED, so what refused a connection is readable
  # rather than inferred from a timeout.
  #
  # ⛔ SINGLE QUOTES ARE THE POINT ON THE NEXT TWO LINES, not an oversight. These
  # are the literal text of the script being generated, and `$0` there is that
  # script's own path when it runs inside the namespace. Expanding them here would
  # bake in this shell's values and the generated script would read nothing.
  # shellcheck disable=SC2016
  printf 'if [ -n "${TK_PRIVATE_NET_SHOW:-}" ]; then nft list ruleset >&2; fi\n'
  # shellcheck disable=SC2016
  printf 'exec /bin/sh "$0.payload"\n'
} > "$WORK/enter.sh"
cp "$WORK/payload.sh" "$WORK/enter.sh.payload"
chmod 0700 "$WORK/enter.sh" "$WORK/enter.sh.payload"

# ⛔ --quiet, BECAUSE THIS WRAPS SOMEBODY ELSE'S COMMAND. Without it pasta writes
# `No interfaces with usable IPv6 routes` and a second line to stderr on every
# run, which lands in the middle of the payload's own output and is the class
# shell-profile.sh was measured against: a wrapper that adds bytes to a stream a
# caller is parsing.
exec pasta --quiet --config-net -- /bin/sh "$WORK/enter.sh"
