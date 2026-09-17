#!/bin/sh
# Attack the base's doors from the inside, as the unprivileged account, and
# report each one as something that was TRIED rather than something that was read
# back from a configuration file.
#
# ⛔ EVERY LINE IS AN ATTEMPT. A door is `open` because something actually got
# through, `closed` because the attempt was refused, and `unknown` because this
# guest carries no way to make the attempt. ⛔ `unknown` IS NOT `closed`, and the
# caller is forbidden from treating it as one: a probe that reports a door shut
# because it could not reach the handle is the exact theatre this file exists to
# refuse. WSL-68.
#
# It runs as TK_USER inside the owned distribution and is delivered on stdin.
#
# The answer is machine-read. One line per door:
#
#   DOOR|<id>|<verdict>|<detail>
#
# and one last line, DOORS-COMPLETE|<n>, whose n is counted by the emitter below
# rather than by the reader. ⭐ That is the point of it: if the reader's parse ever
# stops matching what this file writes, the two numbers disagree and the command
# refuses, instead of rendering an empty table over a base it never measured.
set -u

: "${TK_USER:?TK_USER is required}"
: "${TK_HOSTADDR?TK_HOSTADDR is required, and may be empty}"
: "${TK_DOORS_MARK:?TK_DOORS_MARK is required}"

case "$TK_DOORS_MARK" in
  *[!0-9a-f]*|'') printf 'doors: TK_DOORS_MARK is not hexadecimal\n' >&2; exit 3 ;;
esac

doors=0
# say ID VERDICT DETAIL. The detail is flattened: a newline or a pipe in it would
# forge a row, and the count above is what makes a forged row detectable anyway.
say() {
  s_detail=$(printf '%s' "$3" | tr '\n|' '  ' | cut -c1-200)
  doors=$((doors + 1))
  printf 'DOOR|%s|%s|%s\n' "$1" "$2" "$s_detail"
}

# ---------------------------------------------------------------- identity ---
say identity info "account=$(id -un) uid=$(id -u) distro=${WSL_DISTRO_NAME:-unset} pid1=$(cat /proc/1/comm 2>/dev/null)"

# ------------------------------------------------------------- filesystem ----
drives=$(awk '$2 ~ /^\/mnt\/[A-Za-z]$/ {printf "%s ", $2}' /proc/mounts 2>/dev/null)
if [ -n "$drives" ]; then
  say fs.windows-drives open "mounted: $drives"
else
  say fs.windows-drives closed 'no /mnt/<letter> in /proc/mounts'
fi

att=/tmp/doors-$TK_DOORS_MARK
mkdir -p "$att" 2>/dev/null
if out=$(mount -t drvfs 'C:' "$att" 2>&1); then
  say fs.mount-drvfs open "the account mounted C: at $att"
  umount "$att" 2>/dev/null
else
  say fs.mount-drvfs closed "$(printf '%s' "$out" | head -1)"
fi
rmdir "$att" 2>/dev/null

wsldrv=$(awk '$2 == "/usr/lib/wsl/drivers" {print $3}' /proc/mounts 2>/dev/null)
if [ -n "$wsldrv" ]; then
  if touch "/usr/lib/wsl/drivers/.doors-$TK_DOORS_MARK" 2>/dev/null; then
    rm -f "/usr/lib/wsl/drivers/.doors-$TK_DOORS_MARK" 2>/dev/null
    say fs.wsl-drivers open "$wsldrv and WRITABLE"
  else
    say fs.wsl-drivers readonly "$wsldrv, a write was refused"
  fi
else
  say fs.wsl-drivers absent 'not mounted'
fi

# ⛔ /mnt/wsl IS ONE tmpfs SHARED BY EVERY DISTRIBUTION IN THE UTILITY VM, mounted
# drwxrwxrwt, and uids are not namespaced across it. A file written here is a
# channel out of a base that is otherwise sealed, and it is the door this entry's
# own list did not contain until the attack found it.
if [ -d /mnt/wsl ]; then
  if printf 'doors %s\n' "$TK_DOORS_MARK" > "/mnt/wsl/.doors-$TK_DOORS_MARK" 2>/dev/null; then
    rm -f "/mnt/wsl/.doors-$TK_DOORS_MARK" 2>/dev/null
    left=present
    [ -e "/mnt/wsl/.doors-$TK_DOORS_MARK" ] || left=removed
    say fs.mnt-wsl-shared open "wrote /mnt/wsl/.doors-$TK_DOORS_MARK, visible to every distribution; own file $left"
  else
    say fs.mnt-wsl-shared readonly 'present and a write was refused'
  fi
else
  say fs.mnt-wsl-shared closed 'not mounted'
fi
say fs.mnt-wsl-contents info "$(find /mnt/wsl -mindepth 1 -maxdepth 1 2>/dev/null | sed 's|.*/||' | tr '\n' ' ')"

# --------------------------------------------------------------- interop -----
# ⛔ THIS ROW IS A READING AND CARRIES NO VERDICT, because on 2026-09-17 it was
# measured WRONG IN BOTH DIRECTIONS on this host: wsl-toolkit-base, configured
# `[interop] enabled=false`, carries a registered handler; wsl-toolkit, configured
# `enabled=true`, has none at all and cannot execute a Windows binary. A door
# decided from this file would have answered backwards on both.
if [ -e /proc/sys/fs/binfmt_misc/WSLInterop ]; then
  say interop.binfmt info "a handler is registered: $(sed -n 1p /proc/sys/fs/binfmt_misc/WSLInterop 2>/dev/null). A reading, not an attempt"
else
  say interop.binfmt info 'no WSLInterop handler in binfmt_misc. A reading, not an attempt'
fi

# ⭐ THE ATTEMPT THAT NEEDS NOTHING FROM WINDOWS. A two-byte PE header is enough
# to match the handler's magic, so executing one asks the kernel the only question
# that has an answer on every base: will it hand a PE image to an interpreter at
# all? Measured: with no handler the exec is refused outright and the shell falls
# back to reading the file as a script.
stub=/tmp/doors-$TK_DOORS_MARK.exe
printf 'MZ' > "$stub" 2>/dev/null
chmod +x "$stub" 2>/dev/null
stub_out=$("$stub" 2>&1)
stub_rc=$?
rm -f "$stub" 2>/dev/null
case "$stub_rc:$stub_out" in
  127:*|126:*)
    say interop.exec-pe closed "the kernel refused to execute a PE image: $(printf '%s' "$stub_out" | head -1)" ;;
  *)
    say interop.exec-pe open "a PE image was accepted and handed to an interpreter: $(printf '%s' "$stub_out" | head -1)" ;;
esac

# ⛔ A WINDOWS EXECUTABLE MUST BE REACHED BEFORE IT CAN BE TRIED, and a base with
# no Windows path has none. The first version of this row ran a hardcoded
# /mnt/c/... path, got `No such file or directory`, and reported the door CLOSED
# over an attempt that never happened. That is the false closed this file exists
# to refuse, and driving it on 2026-09-16 is what found it.
winexe=
for candidate in /mnt/c/Windows/System32/cmd.exe /mnt/*/Windows/System32/cmd.exe; do
  [ -f "$candidate" ] || continue
  winexe=$candidate
  break
done
if [ -z "$winexe" ]; then
  for candidate in /workspaces/*/*.exe; do
    [ -f "$candidate" ] || continue
    winexe=$candidate
    break
  done
fi
if [ -z "$winexe" ]; then
  say interop.run-exe unknown 'no Windows executable is reachable from this guest, so none was run'
elif out=$("$winexe" /c echo DOORS 2>&1); then
  say interop.run-exe open "$winexe ran: $(printf '%s' "$out" | tr -d '\r' | head -1)"
else
  say interop.run-exe closed "$winexe did not run: $(printf '%s' "$out" | tr -d '\r' | head -1)"
fi

# ⭐ THE ONE INTEROP DOOR THIS TOOL CAN ACTUALLY PROMISE. appendWindowsPath is
# written by the provisioner and is decidable on every base, with nothing fetched
# and nothing reachable required.
case ":$PATH:" in
  *:/mnt/*) say interop.windows-path open "a Windows directory is on PATH: $PATH" ;;
  *) say interop.windows-path closed 'no Windows directory on PATH' ;;
esac
say interop.env info "WSLENV=${WSLENV:-unset}"

# ---------------------------------------------------- the WSL plumbing -------
# ⚠ PRESENT IS NOT ATTACKED. Each of these is reported with its mode because a
# sealed base's threat model has to answer for them, and because the enumeration
# that named them measured nothing about what they reach. Finding 22.
for p in /init /run/WSL /dev/kvm /dev/dxg /dev/vsock; do
  p_id="plumbing.$(printf '%s' "$p" | sed 's#^/##; s#/#-#g')"
  if [ -e "$p" ]; then
    # stat, not ls: the mode and owner are the point, and parsing ls is what
    # SC2012 refuses. A guest with no stat reports the path alone.
    say "$p_id" info "present, $(stat -c '%A %U %G' "$p" 2>/dev/null || echo 'mode unread: no stat in this guest')"
  else
    say "$p_id" absent 'not present'
  fi
done
say plumbing.netns info "net=$(readlink /proc/self/ns/net 2>/dev/null) pid=$(readlink /proc/self/ns/pid 2>/dev/null)"

# --------------------------------------------------------------- network -----
say net.addresses info "$(ip -4 -o addr show 2>/dev/null | awk '{printf "%s=%s ", $2, $4}')"
say net.default-route info "$(ip -4 route show default 2>/dev/null | head -1)"

# ⛔ HOW THE CONNECTION IS TRIED IS PART OF THE ANSWER, and a guest with no way to
# try it reports `unknown`. The scratch probe this replaces used bash's /dev/tcp
# through `sh`, which is dash on Debian and busybox ash on Alpine: on either of
# those every TCP door would have read CLOSED without one packet being sent.
#
# ⛔ AND EVERY ROUTE CARRIES ITS OWN DEADLINE. bash's /dev/tcp has none: the first
# driven run of this file spent 277 SECONDS on two filtered ports, because a
# connect to a port that neither answers nor refuses runs to the kernel's own
# timeout. `nc -w` and python3's settimeout have one; bash is wrapped in
# `timeout`, and without `timeout` the bash route is not used at all.
connect_method=none
if command -v nc >/dev/null 2>&1; then
  connect_method=nc
elif command -v timeout >/dev/null 2>&1 && command -v bash >/dev/null 2>&1; then
  connect_method=bash
elif command -v python3 >/dev/null 2>&1; then
  connect_method=python3
fi

# try_tcp ID HOST PORT
try_tcp() {
  t_id=$1; t_host=$2; t_port=$3
  if [ -z "$t_host" ]; then
    say "$t_id" unknown 'no address to try'
    return
  fi
  case "$connect_method" in
    nc)
      if nc -z -w 4 "$t_host" "$t_port" >/dev/null 2>&1; then
        say "$t_id" open "$t_host:$t_port connected, via nc"
      else
        say "$t_id" closed "$t_host:$t_port refused or unreachable, via nc"
      fi
      ;;
    bash)
      if timeout 4 bash -c "exec 3<>/dev/tcp/$t_host/$t_port" >/dev/null 2>&1; then
        say "$t_id" open "$t_host:$t_port connected, via bash /dev/tcp"
      else
        say "$t_id" closed "$t_host:$t_port refused, unreachable or silent for 4s, via bash /dev/tcp"
      fi
      ;;
    python3)
      if python3 -c "import socket,sys; s=socket.socket(); s.settimeout(4); sys.exit(s.connect_ex((sys.argv[1], int(sys.argv[2]))))" "$t_host" "$t_port" >/dev/null 2>&1; then
        say "$t_id" open "$t_host:$t_port connected, via python3"
      else
        say "$t_id" closed "$t_host:$t_port refused or unreachable, via python3"
      fi
      ;;
    *)
      # ⛔ NOT `closed`. Nothing was tried.
      say "$t_id" unknown 'this guest has no nc, bash or python3 to open a socket with'
      ;;
  esac
}

try_tcp net.windows-host-smb "$TK_HOSTADDR" 445
try_tcp net.windows-host-rdp "$TK_HOSTADDR" 3389
if [ -z "$TK_HOSTADDR" ]; then
  say net.windows-host-icmp unknown 'no address to try'
elif ! command -v ping >/dev/null 2>&1; then
  say net.windows-host-icmp unknown 'this guest has no ping'
elif ping -c1 -W2 "$TK_HOSTADDR" >/dev/null 2>&1; then
  say net.windows-host-icmp open "$TK_HOSTADDR answers ICMP"
else
  say net.windows-host-icmp closed "$TK_HOSTADDR does not answer ICMP"
fi
try_tcp net.internet 1.1.1.1 443
say net.dns info "$(sed -n 's/^nameserver //p' /etc/resolv.conf 2>/dev/null | tr '\n' ' ')"
if [ -L /etc/resolv.conf ]; then
  say net.resolv-conf info "/etc/resolv.conf -> $(readlink /etc/resolv.conf 2>/dev/null)"
else
  say net.resolv-conf info "/etc/resolv.conf is a regular file"
fi
# ⛔ ONE SHARED NETWORK NAMESPACE, so a listener another distribution started is
# reachable from here at 127.0.0.1. Measured 2026-09-14: every distribution on
# this host answered net:[4026531833].
if command -v ss >/dev/null 2>&1; then
  say net.listeners info "$(ss -H -ltn 2>/dev/null | awk '{printf "%s ", $4}')"
else
  say net.listeners info 'no ss in this guest'
fi

# ------------------------------------------------------------- privilege -----
if sudo -n true 2>/dev/null; then
  say priv.passwordless-sudo open 'sudo -n true succeeded'
else
  say priv.passwordless-sudo closed 'sudo -n true was refused'
fi
if ! command -v unshare >/dev/null 2>&1; then
  say priv.unshare-netns unknown 'this guest has no unshare'
  say priv.unshare-userns unknown 'this guest has no unshare'
  say priv.unshare-user-plus-net unknown 'this guest has no unshare'
else
  if unshare -n true 2>/dev/null; then
    say priv.unshare-netns open 'an unprivileged network namespace was made'
  else
    say priv.unshare-netns closed 'unshare -n was refused'
  fi
  if unshare -U true 2>/dev/null; then
    say priv.unshare-userns open 'a user namespace was made'
  else
    say priv.unshare-userns closed 'unshare -U was refused'
  fi
  # ⭐ THIS IS THE ONE THE RULING TURNS ON. `unshare -Un` succeeding is what lets a
  # sealed base get a network namespace of its own without a rule in the shared one.
  if unshare -Un true 2>/dev/null; then
    say priv.unshare-user-plus-net open 'a private network namespace was made inside a user namespace'
  else
    say priv.unshare-user-plus-net closed 'unshare -Un was refused'
  fi
fi

printf 'DOORS-COMPLETE|%d\n' "$doors"
