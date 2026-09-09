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
