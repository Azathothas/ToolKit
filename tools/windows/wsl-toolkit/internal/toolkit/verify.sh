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
