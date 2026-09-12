#!/bin/sh
# Prove the base distribution can actually run a rootless container, as the
# unprivileged account, right now.
#
# It runs as TK_USER inside the owned distribution and is delivered on stdin.
#
# ⛔ THE ENGINE ANSWERING `info` IS NOT THE ENGINE RUNNING A CONTAINER. Rootless
# podman reports a complete configuration and then fails at the first run with a
# namespace, a cgroup or a storage error, so a check that reads `info` passes
# over a base nothing can use. This runs a container and reads what came back.
set -eu

: "${TK_IMAGE:?TK_IMAGE is required}"
: "${TK_M1:?TK_M1 is required}"
: "${TK_M2:?TK_M2 is required}"
: "${TK_AUTOMOUNT:?TK_AUTOMOUNT is required}"
: "${TK_INTEROP:?TK_INTEROP is required}"
: "${TK_SYSTEMD:?TK_SYSTEMD is required}"
: "${TK_TOOLSET:?TK_TOOLSET is required}"
: "${TK_MOUNT_CHECKS?TK_MOUNT_CHECKS is required, and may be empty}"

# The configuration files are intentions. These checks run after WSL has been
# terminated and started again, so they answer whether WSL honored them.
case "$TK_AUTOMOUNT" in
  off)
    if grep -Eq '[[:space:]]/mnt/[[:alpha:]]([[:space:]]|/)' /proc/mounts; then
      printf 'verify: a Windows drive is mounted below /mnt even though automount is off\n' >&2
      exit 3
    fi
    ;;
  ro|rw) ;;
  *) printf 'verify: unknown automount setting %s\n' "$TK_AUTOMOUNT" >&2; exit 3 ;;
esac
printf 'automount %s\n' "$TK_AUTOMOUNT"

case "$TK_INTEROP" in
  off)
    if [ -n "${WSL_INTEROP:-}" ]; then
      printf 'verify: WSL_INTEROP exists even though Windows interop is off\n' >&2
      exit 3
    fi
    ;;
  on) ;;
  *) printf 'verify: unknown interop setting %s\n' "$TK_INTEROP" >&2; exit 3 ;;
esac
printf 'interop %s\n' "$TK_INTEROP"

pid_one=$(cat /proc/1/comm 2>/dev/null || echo unknown)
case "$TK_SYSTEMD:$pid_one" in
  true:systemd|false:systemd-shutdown) ;;
  true:*)
    printf 'verify: systemd was requested but PID 1 is %s\n' "$pid_one" >&2
    exit 3
    ;;
  false:systemd)
    printf 'verify: systemd is PID 1 even though it was disabled\n' >&2
    exit 3
    ;;
  false:*) ;;
  *) printf 'verify: unknown systemd setting %s\n' "$TK_SYSTEMD" >&2; exit 3 ;;
esac
printf 'systemd %s pid-one %s\n' "$TK_SYSTEMD" "$pid_one"

case "$TK_TOOLSET" in
  none) ;;
  developer)
    for tool in bash cc c++ curl git jq make node npm rg ssh tmux unzip; do
      command -v "$tool" >/dev/null 2>&1 || {
        printf 'verify: the developer toolset is missing %s\n' "$tool" >&2
        exit 3
      }
    done
    ;;
  *) printf 'verify: unknown toolset %s\n' "$TK_TOOLSET" >&2; exit 3 ;;
esac
printf 'toolset %s\n' "$TK_TOOLSET"

mount_count=0
mount_decode="${TMPDIR:-/tmp}/wsl-toolkit-mount-check.$$"
mount_allowlist="${TMPDIR:-/tmp}/wsl-toolkit-mount-allowlist.$$"
: > "$mount_allowlist"
trap 'rm -f "$mount_decode" "$mount_allowlist"' 0 1 2 15
# The payload is a validated sequence of base64-target/mode pairs.
# shellcheck disable=SC2086
set -- $TK_MOUNT_CHECKS
while [ "$#" -gt 0 ]; do
  [ "$#" -ge 2 ] || { printf 'verify: the explicit mount check table is incomplete\n' >&2; exit 3; }
  target_b64=$1
  mode=$2
  shift 2
  printf '%s' "$target_b64" > "$mount_decode"
  if ! target=$(base64 -d "$mount_decode"); then
    rm -f "$mount_decode"
    printf 'verify: an explicit mount target could not be decoded\n' >&2
    exit 3
  fi
  rm -f "$mount_decode"
  printf '%s\n' "$target" >> "$mount_allowlist"
  [ -d "$target" ] || { printf 'verify: explicit mount %s is absent\n' "$target" >&2; exit 3; }
  if ! awk -v expected="$target" '
    $2 == expected && ($3 == "drvfs" || ($3 == "9p" && $4 ~ /aname=drvfs/)) { found=1 }
    END { exit !found }
  ' /proc/mounts; then
    printf 'verify: explicit target %s is a guest directory, not a live Windows mount\n' "$target" >&2
    exit 3
  fi
  ls "$target" >/dev/null 2>&1 || { printf 'verify: explicit mount %s is unreadable\n' "$target" >&2; exit 3; }
  write_probe="$target/.wsl-toolkit-write-probe.$$"
  case "$mode" in
    rw)
      ( set -C; : > "$write_probe" ) 2>/dev/null || {
        printf 'verify: explicit mount %s is not writable\n' "$target" >&2
        exit 3
      }
      rm -f "$write_probe"
      ;;
    ro)
      if ( set -C; : > "$write_probe" ) 2>/dev/null; then
        rm -f "$write_probe"
        printf 'verify: explicit mount %s is writable but configured read-only\n' "$target" >&2
        exit 3
      fi
      ;;
    *) printf 'verify: unknown explicit mount mode %s\n' "$mode" >&2; exit 3 ;;
  esac
  mount_count=$((mount_count + 1))
  printf 'mount %s %s\n' "$mode" "$target"
done
printf 'mount-count %s\n' "$mount_count"

# With drive automount disabled, every live DrvFS mount must be one of the
# configured targets. This catches a grant removed from configuration but still
# present in the running guest, forcing ensure to rewrite fstab and restart.
if [ "$TK_AUTOMOUNT" = off ]; then
  while read -r _source live_target live_type live_options _rest; do
    case "$live_type:$live_options" in
      drvfs:*|9p:*aname=drvfs*)
        if ! grep -Fx "$live_target" "$mount_allowlist" >/dev/null 2>&1; then
          rm -f "$mount_allowlist"
          printf 'verify: unconfigured Windows directory is mounted at %s\n' "$live_target" >&2
          exit 3
        fi
        ;;
    esac
  done < /proc/mounts
fi
rm -f "$mount_allowlist"

printf 'engine %s\n' "$(podman --version 2>&1 | head -1)"
printf 'runtime-dir %s\n' "${XDG_RUNTIME_DIR:-unset}"
printf 'user %s uid %s\n' "$(id -un)" "$(id -u)"

# ⛔ THE MARKER IS NEVER WRITTEN WHOLE ON THIS COMMAND LINE. The two halves go in
# as separate variables and the container joins them, so the command's own echo
# cannot satisfy the test. A console check that searched its captured output for
# a marker the command also contained reported a container had run over a run
# that had failed.
podman run --rm --pull=missing \
  -e TK_A="$TK_M1" -e TK_B="$TK_M2" \
  "$TK_IMAGE" /bin/sh -c 'printf "%s%s\n" "$TK_A" "$TK_B"'

# ⭐ WHAT THE CGROUP TREE CAN DO FOR THIS ACCOUNT, measured rather than read off
# a version number. Everything below this line is best effort: a base that runs
# a container is usable whether or not it can account for one, so nothing here
# may fail the script. `set -eu` is on, so every command carries its own guard.
#
# ⛔ THE MECHANISM IS NAMED, NOT INFERRED FROM THE HOST. Delegation can arrive
# four ways and this repository may use any of them later: systemd's
# user@.service, an explicitly handed-over subtree, a rootful engine, or a full
# virtual machine that simply has one. A row that said "WSL, so no" would be
# wrong the day the base changes shape.
cg_version=none
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
  cg_version=v2
elif [ -d /sys/fs/cgroup/memory ]; then
  cg_version=v1
fi
printf 'cgroup-version %s\n' "$cg_version"
printf 'cgroup-controllers %s\n' "$(cat /sys/fs/cgroup/cgroup.controllers 2>/dev/null || echo -)"

cg_self=$(head -1 /proc/self/cgroup 2>/dev/null || echo -)
printf 'cgroup-self %s\n' "$cg_self"

# ⛔ DELEGATION IS PROVED BY CREATING A CGROUP, not by reading a mode bit. The
# directory is root-owned and world-readable in both the delegated and the
# undelegated case, so a permission read answers the same either way.
cg_rel=${cg_self#0::}
case "$cg_rel" in /*) ;; *) cg_rel=/ ;; esac
cg_probe="/sys/fs/cgroup${cg_rel%/}/wsl-toolkit-delegation-probe.$$"
if mkdir "$cg_probe" 2>/dev/null; then
  printf 'cgroup-delegated yes\n'
  rmdir "$cg_probe" 2>/dev/null || true
else
  printf 'cgroup-delegated no\n'
fi

case "$cg_self" in
  *user@*.service*|*user.slice*) printf 'cgroup-under systemd\n' ;;
  *)                             printf 'cgroup-under none\n' ;;
esac
printf 'engine-rootless %s\n' "$(podman info --format '{{.Host.Security.Rootless}}' 2>/dev/null || echo unknown)"

binfmt_handlers=0
for handler in /proc/sys/fs/binfmt_misc/qemu-*; do
  [ -e "$handler" ] || continue
  binfmt_handlers=$((binfmt_handlers + 1))
done
printf 'binfmt-handlers %s\n' "$binfmt_handlers"

# ⭐ THE ENFORCEMENT CLAIM IS THE ONE A CALLER ACTS ON, so it is measured inside
# a container and not derived from the rows above. A separate run from the
# marker one on purpose: a base that runs a container is healthy, and a base
# that cannot account for one is a capability finding, not a health failure.
#
# ⚠ THREE ANSWERS, NOT TWO, AND THE THIRD IS THE ONE THIS BASE GIVES. A
# container here HAS /sys/fs/cgroup mounted and has no `memory.max` in it,
# because it is sitting in the ROOT cgroup, which has no memory limit file by
# definition. `missing` therefore means the limit was not applied; `nocgroupfs`
# means nobody could look, and those are different answers.
# ⛔ BOUNDED. This runs on the health path of every `base status --probe` and
# every `base ensure`, which cost about a second before it existed. An engine
# that wedges here would turn that into the script's whole 20 minute ceiling, so
# the container carries its own kill.
cg_limit=$(podman run --rm --pull=missing --timeout 30 --memory 64m "$TK_IMAGE" /bin/sh -c '
  if [ ! -d /sys/fs/cgroup ]; then echo nocgroupfs
  elif [ -r /sys/fs/cgroup/memory.max ]; then cat /sys/fs/cgroup/memory.max
  else echo missing; fi' 2>/dev/null || echo unreadable)
printf 'cgroup-limit %s\n' "$(printf '%s' "$cg_limit" | tr -d '\r\n' | head -c 32)"
