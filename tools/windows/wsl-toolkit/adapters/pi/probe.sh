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

# ⛔ RESOLVED ON THE PATH A PANE ACTUALLY HAS, not on this probe's own. A login shell
# is what herdr starts an agent in, and $HOME/.local/bin is NOT on it: measured
# 2026-09-17, npm put pi there, a pane answered `command not found`, and
# `herdr agent start` timed out on an agent that could never appear. A probe that
# looked only at its own curated PATH called that base healthy.
pane_path=$(runuser -l "$TK_USER" -c "command -v pi" 2>/dev/null | tr -d '\r' | head -1)
if [ -n "$pane_path" ]; then
  printf 'on_login_path %s\n' "$pane_path"
else
  problem "pi is installed and a login shell cannot find it, so herdr cannot start it in a pane. Run: base ensure"
fi

version=$(as_account pi --version 2>/dev/null | tr -d '\r' | head -1)
if [ -n "$version" ]; then
  printf 'version %s\n' "$version"
else
  problem "pi answers no version for $TK_USER, so it is not usable by that account"
fi

# -- the startup model, read back from pi rather than from the file that set it ---------
# ⛔ A STARTUP MODEL PI CANNOT RESOLVE IS NOT AN ERROR TO PI: it falls back to its own
# built-in default, on a DIFFERENT provider, and says nothing. Measured 2026-09-17: with
# defaultProvider muse-gateway and defaultModel muse-spark-1.3-contributor set correctly,
# pi's own status line read `(anthropic) claude-opus-4-8`, because models.json declared
# four spark ids and not that one. The gateway serving a model and pi's catalogue knowing
# it are two different facts.
#
# ⚠ AND max CLAMPS TO high WITHOUT A thinkingLevelMap. pi's own docs: with the map
# omitted, `xhigh` and `max` are unsupported and clamped away. A base can therefore be set
# to max and start every session at high with nothing said.
#
# ⭐ SO BOTH ARE READ BACK FROM PI'S OWN CATALOGUE, and a mismatch is a problem rather
# than a fact.
settings=$TK_HOME/.pi/agent/settings.json
if [ -f "$settings" ] && command -v jq >/dev/null 2>&1; then
  want_provider=$(as_account jq -r '.defaultProvider // ""' "$settings" 2>/dev/null)
  want_model=$(as_account jq -r '.defaultModel // ""' "$settings" 2>/dev/null)
  want_effort=$(as_account jq -r '.defaultThinkingLevel // ""' "$settings" 2>/dev/null)
  [ -n "$want_model" ] && printf 'startup_model %s/%s\n' "${want_provider:-?}" "$want_model"
  [ -n "$want_effort" ] && printf 'startup_effort %s\n' "$want_effort"
  if [ -n "$want_model" ]; then
    if as_account pi --list-models 2>/dev/null | tr -d '\r' |
        awk -v p="$want_provider" -v m="$want_model" '$1 == p && $2 == m { found = 1 } END { exit !found }'; then
      : # pi's own catalogue resolves it
    else
      problem "pi's startup model is $want_provider/$want_model and its own catalogue does not list it, so every session starts on pi's built-in default instead. Declare the model under that provider in $TK_HOME/.pi/agent/models.json"
    fi
    case $want_effort in
      xhigh|max)
        models=$TK_HOME/.pi/agent/models.json
        # shellcheck disable=SC2016  # the single quotes hold a jq program, not shell
        if [ -f "$models" ] && ! as_account jq -e --arg p "$want_provider" --arg m "$want_model" --arg e "$want_effort" \
            '(.providers[$p].models // []) | any(.id == $m and ((.thinkingLevelMap // {}) | has($e)))' "$models" >/dev/null 2>&1; then
          problem "pi's startup effort is $want_effort and $want_provider/$want_model declares no thinkingLevelMap exposing it, so pi clamps every session to high. Add \"thinkingLevelMap\": {\"$want_effort\": \"$want_effort\"} to that model"
        fi
        ;;
    esac
  fi
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
