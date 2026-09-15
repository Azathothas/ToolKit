#!/bin/sh
# omp adapter, probe: what install.sh put in place, read back from the machine.
#
# THE DEFECT IT EXISTS TO CATCH: the pi probe's, plus the one that only appears
# with both installed. ⛔ The resolved agent directory is a FACT here rather than a
# detail, because it is what decides whether herdr accepts this integration beside
# pi's: herdr refuses omp when the two resolve to the same directory, and an
# environment that changed after install is invisible until something breaks.
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
account_env() {
  as_account sh -c "printf '%s' \"\${$1:-}\""
}

version=$(as_account omp --version 2>/dev/null | tr -d '\r' | head -1)
if [ -n "$version" ]; then
  printf 'version %s\n' "$version"
else
  problem "omp answers no version for $TK_USER, so it is not usable by that account"
fi

pi_coding_agent_dir=$(account_env PI_CODING_AGENT_DIR)
pi_config_dir=$(account_env PI_CONFIG_DIR)
if [ -n "$pi_coding_agent_dir" ]; then
  agent_dir=$pi_coding_agent_dir
elif [ -n "$pi_config_dir" ]; then
  agent_dir=$TK_HOME/$pi_config_dir/agent
else
  agent_dir=$TK_HOME/.omp/agent
fi
printf 'agent_dir %s\n' "$agent_dir"

if [ -n "$pi_coding_agent_dir" ]; then
  pi_dir=$pi_coding_agent_dir
else
  pi_dir=$TK_HOME/.pi/agent
fi
if [ "$agent_dir" = "$pi_dir" ]; then
  problem "pi and omp both resolve to $agent_dir, so herdr refuses the omp integration. PI_CODING_AGENT_DIR is read by both and is set for $TK_USER"
fi

extension=$agent_dir/extensions/herdr-omp-agent-state.ts
if [ -f "$extension" ]; then
  printf 'herdr_integration installed\n'
else
  printf 'herdr_integration absent\n'
fi

if command -v herdr >/dev/null 2>&1 || [ -x /usr/local/bin/herdr ]; then
  herdr_bin=$(command -v herdr 2>/dev/null || printf '/usr/local/bin/herdr')
  status=$(as_account "$herdr_bin" integration status 2>/dev/null | sed -n 's/^[[:space:]]*omp[[:space:]]*//p' | head -1)
  [ -n "$status" ] && printf 'herdr_integration_status %s\n' "$status"
fi
