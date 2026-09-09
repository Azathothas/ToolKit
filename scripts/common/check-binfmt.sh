#!/bin/sh
# check-binfmt.sh - are binfmt_misc handlers actually registered in the kernel
# that containers on this machine run against?
#
# ⭐ A WRAPPER. The tool is `repo binfmt` in tools/repo.
#
# The defect it exists to catch is cross-architecture execution that has never
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
# In order: /proc/sys/fs/binfmt_misc directly, when this host has one (Linux,
# WSL); `wsl.exe -d DISTRO` when it does not (a Windows host); then exit 2,
# because no Linux kernel is reachable from here.
#
# ⚠ ON A WINDOWS HOST THIS STARTS A WSL DISTRIBUTION to read its kernel, and
# the default one is `podman-machine-default` because that is where the engine
# usually lives. It only READS; it registers nothing and stops nothing. On a
# machine where that distribution belongs to somebody else, name another with
# --distro.
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

unset CDPATH
DIR=$(cd -- "$(dirname -- "$0")" && pwd) || {
  printf 'check-binfmt: cannot resolve my own directory\n' >&2
  exit 2
}
exec sh "$DIR/repo.sh" binfmt "$@"
