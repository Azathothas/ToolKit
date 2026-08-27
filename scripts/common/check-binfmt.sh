#!/bin/sh
# check-binfmt.sh - are binfmt_misc handlers actually registered in the kernel
# that containers on this machine run against?
#
# The defect this exists to catch is cross-architecture execution that has never
# once worked while every visible signal says the machine is healthy. Measured
# on the reporting machine on 2026-08-27: `systemd-binfmt.service` reported
# `status=0/SUCCESS` having registered ZERO handlers, because the path it writes
# to had a systemd autofs stacked on the binfmt_misc mount and every read of it
# returned ELOOP. The unit was green, the config was complete, the emulators
# were installed, and `podman run --platform linux/arm64` failed with
# `Exec format error` that reads like an unrelated breakage.
#
# ⭐ It reads the KERNEL, not a unit's exit code. That is the whole point: the
# unit is the thing that lied.
#
# ── ⚠ WHERE IT LOOKS, AND WHY NOT `podman machine ssh` ──────────────────────
#
# The issue that asked for this assumed `podman machine ssh`. It is not used,
# and the reason is measured: on Windows that command passes
# `-o UserKnownHostsFile=NUL` to its own ssh, and under Git Bash NUL is a
# FILENAME rather than the null device, so it writes a 99-byte file called NUL
# into whatever directory you ran it from. A diagnostic that litters the
# repository it is diagnosing is a worse defect than the one it detects.
#
# ⭐ It is also unnecessary. Measured on 2026-08-27, kernel 7.2.0-WSL2-STABLE:
# every WSL2 distro on a machine shares ONE kernel, so the handlers registered
# by the podman machine are visible from any distro, and `wsl -d <distro>` reads
# them with no ssh, no key file and nothing written anywhere.
#
# So, in order:
#   1. /proc/sys/fs/binfmt_misc directly, when this host has one (Linux, WSL);
#   2. `wsl.exe -d DISTRO` when it does not (a Windows host);
#   3. exit 2, because no Linux kernel is reachable from here.
#
# Usage:
#   sh scripts/common/check-binfmt.sh
#   sh scripts/common/check-binfmt.sh --json
#   sh scripts/common/check-binfmt.sh --distro podman-machine-default
#   sh scripts/common/check-binfmt.sh --require 1
#
# ⚠ --require N is what turns this from a report into an assertion. WITHOUT it a
# count of zero is reported and exits 0, because a machine that never wanted
# cross-architecture execution is not broken. scripts/README.md: a check that
# measures an open defect must not fail the build for that defect alone, and it
# judges only past a stated ceiling.
#
# Exit codes: 0 read it, 1 the kernel state is broken or below --require,
#             2 could not run.
#
# ⛔ Read the exit code from this process, unpiped.

set -u

JSON=0
DISTRO="podman-machine-default"
REQUIRE=0
BINFMT_DIR="/proc/sys/fs/binfmt_misc"

while [ $# -gt 0 ]; do
  case "$1" in
    --json) JSON=1 ;;
    --distro) shift; DISTRO="${1:-}" ;;
    --require) shift; REQUIRE="${1:-0}" ;;
    -h|--help) awk 'NR>1 { if (/^#/) { sub(/^# ?/, ""); print } else exit }' "$0"; exit 0 ;;
    *) printf 'check-binfmt: unknown argument: %s\n' "$1" >&2; exit 2 ;;
  esac
  shift
done

case "$REQUIRE" in
  ''|*[!0-9]*) printf 'check-binfmt: --require wants a number, got "%s"\n' "$REQUIRE" >&2; exit 2 ;;
esac

SOURCE=""
LISTING=""
KERNEL=""
READ_ERR=""

if [ -d "$BINFMT_DIR" ]; then
  SOURCE="local"
  KERNEL=$(uname -r 2>/dev/null || printf 'unknown')
  # ⚠ stderr is kept, not discarded. ELOOP is the whole diagnosis and it arrives
  # on stderr; a version of this that sent it to /dev/null would report "no
  # handlers" over the one state it exists to name.
  LISTING=$(ls -1 "$BINFMT_DIR" 2>&1) || READ_ERR="$LISTING"
else
  WSL=""
  for c in wsl.exe wsl; do
    if command -v "$c" >/dev/null 2>&1; then WSL=$c; break; fi
  done
  if [ -z "$WSL" ]; then
    printf 'check-binfmt: no %s on this host and no wsl.exe to reach one.\n' "$BINFMT_DIR" >&2
    printf 'check-binfmt: nothing to read. This is not a failure, it is not applicable here.\n' >&2
    exit 2
  fi
  SOURCE="wsl:$DISTRO"

  # WSL emits UTF-16LE unless this is set, and a redirected stdout then reads as
  # empty or as mojibake. docs/conventions/shell.md section 7.
  WSL_UTF8=1
  export WSL_UTF8

  # ⛔ BOTH of these, and they cover different things. Git Bash rewrites any
  # argument that looks like a POSIX path into a Windows path before the target
  # process sees it, so `/bin/sh` arrives at the Linux side as
  # `C:/Program Files/Git/bin/sh` and the distro reports as unstartable on a
  # machine where it is running fine. Measured here: without them this check
  # exited 2 over a healthy podman machine.
  # MSYS_NO_PATHCONV disables the leading-path heuristic; MSYS2_ARG_CONV_EXCL is
  # a per-argument exclusion list and '*' excludes everything.
  MSYS_NO_PATHCONV=1
  MSYS2_ARG_CONV_EXCL='*'
  export MSYS_NO_PATHCONV MSYS2_ARG_CONV_EXCL

  if ! "$WSL" -d "$DISTRO" -u root -- /bin/sh -lc 'exit 0' >/dev/null 2>&1; then
    printf 'check-binfmt: distro "%s" is not registered or would not start.\n' "$DISTRO" >&2
    printf 'check-binfmt: start it, or name another with --distro. Could not run.\n' >&2
    exit 2
  fi

  KERNEL=$("$WSL" -d "$DISTRO" -u root -- /bin/sh -lc 'uname -r' 2>/dev/null | tr -d '\r')
  LISTING=$("$WSL" -d "$DISTRO" -u root -- /bin/sh -lc "ls -1 $BINFMT_DIR" 2>&1 | tr -d '\r')
  case "$LISTING" in
    *'Too many levels of symbolic links'*|*ELOOP*|*'No such file'*|*'Not a directory'*)
      READ_ERR="$LISTING"
      ;;
  esac
fi

COUNT=0
ENABLED="unknown"
STACKED=0

if [ -z "$READ_ERR" ]; then
  COUNT=$(printf '%s\n' "$LISTING" | grep -c '^qemu-' || true)
  case "$LISTING" in
    *status*) ENABLED="present" ;;
    *) ENABLED="absent" ;;
  esac
else
  case "$READ_ERR" in
    *'Too many levels of symbolic links'*|*ELOOP*) STACKED=1 ;;
  esac
fi

# ── the verdict ────────────────────────────────────────────────────────────
PROBLEM=""
if [ -n "$READ_ERR" ]; then
  if [ "$STACKED" = 1 ]; then
    PROBLEM="stacked-mount"
  else
    PROBLEM="unreadable"
  fi
elif [ "$COUNT" -lt "$REQUIRE" ]; then
  PROBLEM="below-require"
fi

if [ "$JSON" = 1 ]; then
  printf '{"schema":"check-binfmt/1","source":"%s","kernel":"%s","handlers":%s,"status_file":"%s","stacked":%s,"problem":"%s"}\n' \
    "$SOURCE" "$KERNEL" "$COUNT" "$ENABLED" "$STACKED" "$PROBLEM"
  [ -n "$PROBLEM" ] && exit 1
  exit 0
fi

printf 'check-binfmt\n'
printf '  read from      %s\n' "$SOURCE"
printf '  kernel         %s\n' "$KERNEL"

if [ "$STACKED" = 1 ]; then
  printf '  handlers       UNREADABLE\n\n'
  printf '⛔ %s exists and CANNOT BE READ: ELOOP.\n' "$BINFMT_DIR"
  printf '   That is a second filesystem stacked on the same path, and it is the\n'
  printf '   state this check exists to name. systemd-binfmt.service writes into\n'
  printf '   the path underneath and reports status=0/SUCCESS while registering\n'
  printf '   nothing, so the unit is green and cross-architecture execution has\n'
  printf '   never once worked.\n\n'
  printf '   The reading, verbatim:\n'
  printf '%s\n' "$READ_ERR" | sed 's/^/     /'
  exit 1
fi

if [ -n "$READ_ERR" ]; then
  printf '  handlers       UNREADABLE\n\n'
  printf '⛔ %s could not be read, and NOT with the ELOOP this check knows:\n' "$BINFMT_DIR"
  printf '%s\n' "$READ_ERR" | sed 's/^/     /'
  exit 1
fi

printf '  qemu handlers  %s\n' "$COUNT"
printf '  status file    %s\n' "$ENABLED"

if [ "$COUNT" = 0 ]; then
  printf '\n'
  printf '⚠ ZERO handlers are registered. Nothing is broken if this machine never\n'
  printf '  wanted cross-architecture execution. If it did, this is why\n'
  printf '  "podman run --platform linux/ARCH" fails with Exec format error, and\n'
  printf '  a green systemd-binfmt.service does not contradict it.\n'
fi

if [ "$PROBLEM" = "below-require" ]; then
  printf '\n⛔ %s handler(s) registered, --require asked for %s.\n' "$COUNT" "$REQUIRE"
  exit 1
fi

printf '\n'
printf '⚠ Registered is not the same as reaching a container. A handler registered\n'
printf '  WITHOUT the F flag needs its interpreter to exist inside the mount\n'
printf '  namespace that runs; with F the kernel holds the interpreter open and it\n'
printf '  does not. Read one to see which:\n'
printf '    cat %s/qemu-aarch64\n' "$BINFMT_DIR"
exit 0
