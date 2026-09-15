#!/bin/sh
# pi adapter, probe: what install.sh put in place, read back from the machine.
#
# THE DEFECT IT EXISTS TO CATCH: an agent reported installed because npm exited 0.
# A package removed by a later install, a herdr integration that was never written,
# and a version that answers nothing are all invisible to an exit code.
#
# It runs as root inside the base, delivered on stdin. It prints one fact per line
# as `name value`, `problem TEXT` for each thing wrong, and exits 0 whatever it
# finds: the lines are the answer.
set -u
: "${TK_USER:?TK_USER is required}"

problem() { printf 'problem %s\n' "$*"; }

TK_HOME=$(getent passwd "$TK_USER" | cut -d: -f6)
PREFIX=$TK_HOME/.local

as_account() {
  runuser -u "$TK_USER" -- env -i HOME="$TK_HOME" USER="$TK_USER" LOGNAME="$TK_USER" \
    SHELL=/bin/bash LANG=C.UTF-8 npm_config_prefix="$PREFIX" \
    PATH="$PREFIX/bin:/usr/local/sbin:/usr/local/bin:/usr/bin:/bin" "$@"
}

version=$(as_account pi --version 2>/dev/null | tr -d '\r' | head -1)
if [ -n "$version" ]; then
  printf 'version %s\n' "$version"
else
  problem "pi answers no version for $TK_USER, so it is not usable by that account"
fi

# ⭐ THE AGENT DIRECTORY IS REPORTED AS A FACT, because it is the thing that decides
# whether herdr will accept an omp integration beside this one: herdr refuses omp
# when the two resolve to the same directory. WSL-89.
agent_dir=$TK_HOME/.pi/agent
[ -n "${PI_CODING_AGENT_DIR:-}" ] && agent_dir=$PI_CODING_AGENT_DIR
printf 'agent_dir %s\n' "$agent_dir"

extension=$agent_dir/extensions/herdr-agent-state.ts
if [ -f "$extension" ]; then
  printf 'herdr_integration installed\n'
else
  printf 'herdr_integration absent\n'
  # ⚠ A FACT, NOT A PROBLEM. pi runs without it; what is lost is herdr reporting
  # state from the agent rather than from its screen, and a base with no herdr at
  # all is a perfectly good base for pi.
fi

if command -v herdr >/dev/null 2>&1 || [ -x /usr/local/bin/herdr ]; then
  herdr_bin=$(command -v herdr 2>/dev/null || printf '/usr/local/bin/herdr')
  status=$(as_account "$herdr_bin" integration status 2>/dev/null | sed -n 's/^[[:space:]]*pi[[:space:]]*//p' | head -1)
  [ -n "$status" ] && printf 'herdr_integration_status %s\n' "$status"
fi
