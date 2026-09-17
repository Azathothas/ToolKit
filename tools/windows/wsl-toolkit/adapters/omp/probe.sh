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
# ⛔ A LOGIN SHELL, AND THAT IS THE WHOLE POINT OF THIS FUNCTION. as_account runs
# `env -i`, which clears the environment on purpose so an install is reproducible,
# and a plain `sh -c` under it therefore reports EVERY variable as unset. Measured
# on 2026-09-17: with `export PI_CODING_AGENT_DIR` in the account's .profile, a
# login shell answered the account's own .pi/agent and this function answered nothing, so
# the collision refusal below could never fire on any base. It was dead code in the
# one condition WSL-89 said most wanted driving.
#
# ⭐ `sh -lc` reads /etc/profile and the account's own profile, which is how the
# agent itself will get the variable when herdr starts it.
account_env() {
  as_account sh -lc "printf '%s' \"\${$1:-}\"" 2>/dev/null
}


# ⭐ THE RUNTIME IS A FACT HERE, because omp is a Bun program and a base with Node
# alone installs it and cannot run it. Reporting the version alone would have said
# "no version" for a missing runtime and a broken install alike, which are different
# problems with different fixes. Measured 2026-09-17.
bun_version=$(as_account bun --version 2>/dev/null | tr -d '\r' | head -1)
if [ -n "$bun_version" ]; then
  printf 'bun %s\n' "$bun_version"
else
  problem "omp is a Bun program and this base has no bun the account can run"
fi

# ⛔ RESOLVED ON THE PATH A PANE ACTUALLY HAS, not on this probe's own. A login shell
# is what herdr starts an agent in, and $HOME/.local/bin is NOT on it: measured
# 2026-09-17, npm put omp there, a pane answered `command not found`, and
# `herdr agent start` timed out on an agent that could never appear. A probe that
# looked only at its own curated PATH called that base healthy.
pane_path=$(runuser -l "$TK_USER" -c "command -v omp" 2>/dev/null | tr -d '\r' | head -1)
if [ -n "$pane_path" ]; then
  printf 'on_login_path %s\n' "$pane_path"
else
  problem "omp is installed and a login shell cannot find it, so herdr cannot start it in a pane. Run: base ensure"
fi
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
# ⭐ THE WRAPPER IS READ BEFORE THE COLLISION IS JUDGED, because a base that took the
# separation resolves omp through a wrapper and its directory is NOT the account's
# PI_CODING_AGENT_DIR. Judging the collision without looking would report a problem on
# exactly the base that fixed it.
omp_path=$(runuser -l "$TK_USER" -c "command -v omp" 2>/dev/null | tr -d '\r' | head -1)
separated=no
# ⛔ BY CONTENT, NOT BY PATH. npm's prefix for this account is $HOME/.local, so its
# own shim and the wrapper share a filename and only the marker tells them apart.
if [ -n "$omp_path" ] && head -3 "$omp_path" 2>/dev/null | grep -qF "# wsl-toolkit omp adapter wrapper"; then
  wrapped_dir=$(sed -n 's/^PI_CODING_AGENT_DIR=//p' "$omp_path" 2>/dev/null | head -1)
  if [ -n "$wrapped_dir" ]; then
    separated=yes
    agent_dir=$wrapped_dir
  fi
fi
printf 'agent_dir %s\n' "$agent_dir"
printf 'separate_agent_dir %s\n' "$separated"

if [ -n "$pi_coding_agent_dir" ]; then
  pi_dir=$pi_coding_agent_dir
else
  pi_dir=$TK_HOME/.pi/agent
fi
printf 'pi_agent_dir %s\n' "$pi_dir"
if [ "$agent_dir" = "$pi_dir" ]; then
  problem "pi and omp both resolve to $agent_dir, so herdr refuses the omp integration. PI_CODING_AGENT_DIR is read by both and is set for $TK_USER"
fi
# ⛔ A SEPARATION THE ACCOUNT'S PATH NO LONGER REACHES IS NOT ONE. The wrapper can be
# shadowed by a later npm install putting omp ahead of it, and then install time and
# run time disagree again with nothing saying so.
if [ "$separated" = yes ] && [ ! -x "$omp_path" ]; then
  problem "omp resolves to $omp_path and it is not executable, so the separated directory would not hold at run time"
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
