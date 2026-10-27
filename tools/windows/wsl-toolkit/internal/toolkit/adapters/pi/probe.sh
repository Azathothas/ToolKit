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
# Where install.sh unpacks each pi release it takes from pi's own GitHub release. WSL-138.
RELEASES=$TK_HOME/.local/share/wsl-toolkit/pi
# The two paths install.sh writes outside the account's home. A case in the suite points
# both into a temporary directory, and nothing else sets them.
STAGE_DIR=${TK_PI_STAGE_DIR:-/var/lib/wsl-toolkit/pi}
WRAPPER=${TK_PI_WRAPPER:-/usr/local/bin/pi}

as_account() {
  runuser -u "$TK_USER" -- env -i HOME="$TK_HOME" USER="$TK_USER" LOGNAME="$TK_USER" \
    SHELL=/bin/bash LANG=C.UTF-8 npm_config_prefix="$PREFIX" \
    PATH="$PREFIX/bin:/usr/local/sbin:/usr/local/bin:/usr/bin:/bin" "$@"
}

# ⭐ WHAT THIS ADAPTER LEAVES IN THE BASE, in bytes on disk. `base status` asks it of
# an adapter the configuration no longer names, which keeps its files until `base
# recreate`. POSIX `du -sk`, so it answers on any userland. WSL-109.
footprint() {
  total=0
  for p in "$@"; do
    [ -n "$p" ] || continue
    [ -e "$p" ] || [ -L "$p" ] || continue
    k=$(du -sk "$p" 2>/dev/null | cut -f1)
    total=$((total + ${k:-0} * 1024))
  done
  printf '%s' "$total"
}
# The npm package a launcher in $PREFIX/bin points into, or nothing.
npm_package_dir() {
  target=$(readlink -f "$1" 2>/dev/null) || return 0
  printf '%s' "$target" | sed -n 's#^\(.*/node_modules/\(@[^/]*/\)\{0,1\}[^/]*\)/.*$#\1#p'
}

# ⛔ ASKED FIRST: IS PI IN THIS BASE AT ALL. Without it, a pi that never installed,
# because an adapter before it stopped `base ensure`, read as "installed and a login
# shell cannot find it", beside problems that only mean something for a pi that is
# there. WSL-107, issue 37.
if [ -e "$WRAPPER" ] || [ -e "$PREFIX/bin/pi" ]; then installed=yes; else installed=no; fi
printf 'installed %s\n' "$installed"
if [ "${TK_PROBE_FOOTPRINT:-}" = 1 ]; then
  # ⚠ npm's package is counted where the launcher points into it, or where a pi from npm
  # left it. A release asset a refusal kept is counted too. Each is the adapter's. WSL-138.
  npm_dir=$(npm_package_dir "$PREFIX/bin/pi")
  [ -n "$npm_dir" ] || npm_dir=$PREFIX/lib/node_modules/@earendil-works/pi-coding-agent
  printf 'footprint %s\n' "$(footprint "$WRAPPER" "$PREFIX/bin/pi" "$npm_dir" "$RELEASES" "$STAGE_DIR" "$TK_HOME/.pi")"
  exit 0
fi
if [ "$installed" = no ]; then
  problem "pi is not installed in this base. Run: base ensure"
  exit 0
fi

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

# ⛔ AND IT MUST RESOLVE TO THIS TOOL'S WRAPPER, NOT MERELY RESOLVE. Measured on
# 2026-09-17: `bootstrap.sh` wrote `export PATH="$HOME/.local/bin:$PATH"` into the
# account's profile, so after `base bootstrap` a login shell found the vendor's own
# launcher FIRST and the wrapper on the system path was never reached. It now puts
# its prefix last and rewrites that line. WSL-147. pi keeps its startup model and effort in its own settings file, so those survive; the account guard does not.
# ⚠ A probe that only asked whether the name resolved called that base healthy.
if [ -n "$pane_path" ]; then
  if head -3 "$pane_path" 2>/dev/null | grep -q "wsl-toolkit's pi adapter"; then
    printf 'wrapper_on_login_path yes\n'
  else
    printf 'wrapper_on_login_path no\n'
    # ⭐ THE PROBLEM NAMES ITS REMEDY. bootstrap.sh now puts its prefix after the
    # system path and rewrites the line an earlier run wrote. WSL-147.
    problem "a login shell resolves pi to $pane_path, which is not this tool's wrapper. The account's own prefix is ahead of /usr/local/bin, which an earlier bootstrap.sh put there, so the wrapper and everything it sets are bypassed. Run: base bootstrap, which moves its own line after the system path. A line the account wrote itself is moved by hand"
  fi
fi

version=$(as_account pi --version 2>/dev/null | tr -d '\r' | head -1)
if [ -n "$version" ]; then
  printf 'version %s\n' "$version"
else
  problem "pi answers no version for $TK_USER, so it is not usable by that account"
fi

# ⭐ WHERE THE ACCOUNT'S pi CAME FROM. install.sh takes pi's own release, and npm only when
# that release could not be fetched. So `npm` here says the official route did not run. A
# base built before WSL-138 reads `npm` too, until its next base ensure.
pi_real=$(as_account readlink -f "$PREFIX/bin/pi" 2>/dev/null || :)
releases_real=$(as_account readlink -f "$RELEASES" 2>/dev/null || :)
if [ -n "$pi_real" ] && [ -n "$releases_real" ] && [ "${pi_real#"$releases_real"/}" != "$pi_real" ]; then
  printf 'source release\n'
else
  case $pi_real in
    */node_modules/*)
      printf 'source npm\n'
      printf "note pi came from npm, not from pi's own release. base ensure installs the release when it can fetch it\n"
      ;;
    *) printf 'source other\n' ;;
  esac
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
    # ⛔ pi LISTS ONLY THE MODELS OF A PROVIDER IT HOLDS A KEY FOR. Measured with pi 1.4.0:
    # with no key it prints `No models available`, and with the provider's key it lists
    # muse-gateway/muse-spark-1.3-contributor. So a model the guide declared before its
    # key was stored read as undeclared, and the probe told the operator to declare it
    # again. A provider pi lists nothing for is a missing key, read from models.json and
    # from pi itself. WSL-135.
    undeclared="pi's startup model is $want_provider/$want_model and its own catalogue does not list it, so every session starts on pi's built-in default instead. Declare the model under that provider in $TK_HOME/.pi/agent/models.json"
    listed=$(as_account pi --list-models 2>/dev/null | tr -d '\r')
    models=$TK_HOME/.pi/agent/models.json
    if printf '%s\n' "$listed" | awk -v p="$want_provider" -v m="$want_model" '$1 == p && $2 == m { found = 1 } END { exit !found }'; then
      printf 'credential present\n' # pi's own catalogue resolves it, which takes a key
    elif [ -z "$want_provider" ] || printf '%s\n' "$listed" | awk -v p="$want_provider" '$1 == p { found = 1 } END { exit !found }'; then
      # ⛔ pi HOLDS A KEY FOR THIS PROVIDER AND STILL DOES NOT LIST THE MODEL. So it is not
      # declared, and every session starts on another model.
      [ -z "$want_provider" ] || printf 'credential present\n'
      problem "$undeclared"
    else
      printf 'credential absent\n'
      # ⭐ THE CHECK THIS PROBE TRUSTS IS THE ONE ITS NOTES NAME. pi 1.4.0 answers
      # --provider with not_ready for any name, a provider it does not know included.
      check="pi auth check --model $want_provider/$want_model"
      # shellcheck disable=SC2016  # the single quotes hold a jq program, not shell
      if [ -f "$models" ] && as_account jq -e --arg p "$want_provider" --arg m "$want_model" \
          '(.providers[$p].models // []) | any(.id == $m)' "$models" >/dev/null 2>&1; then
        # ⭐ DECLARED, AND ITS KEY IS THE OPERATOR'S TO STORE. True, and it stops nothing.
        printf "note pi's startup model %s/%s is declared in %s, and pi holds no key for %s yet, so pi lists none of its models. Store the key, and %s answers ready\n" \
          "$want_provider" "$want_model" "$models" "$want_provider" "$check"
      elif [ -f "$models" ] && as_account jq -e --arg p "$want_provider" '.providers | has($p)' "$models" >/dev/null 2>&1; then
        # ⛔ THE PROVIDER IS DECLARED AND THE MODEL IS NOT, so no key would make pi list it.
        problem "$undeclared"
      else
        # ⚠ A PROVIDER pi CARRIES ITSELF IS ONE models.json DOES NOT NAME. Measured with pi
        # 1.4.0 and no key: `pi auth check --model` answers not_ready for anthropic's
        # models. It answers invalid for a provider pi does not know. The models of a
        # provider pi carries cannot be listed without the key, so the model waits for it.
        verdict=$(as_account pi auth check --model "$want_provider/$want_model" 2>/dev/null | tr -d '\r' | head -1)
        if [ "$verdict" = not_ready ]; then
          printf "note pi holds no key for %s, a provider pi carries itself, so its startup model %s/%s is not checked until the key is stored. Store the key, and %s answers ready\n" \
            "$want_provider" "$want_provider" "$want_model" "$check"
        elif [ "$verdict" = invalid ] && [ ! -e "$models" ] && [ ! -L "$models" ]; then
          # ⭐ NOTHING IS DECLARED YET. That is every new base whose profile names a provider
          # of its own, until its models.json is written. A note, so base ensure exits 0
          # there. A models.json that exists and lacks the provider stays a problem. WSL-135.
          printf "note pi's startup model %s/%s names a provider pi does not carry, and %s does not exist yet. Declare the provider and the model there, as step 5 of the muse-code guide does\n" \
            "$want_provider" "$want_model" "$models"
        else
          problem "pi answers ${verdict:-nothing} for its startup model $want_provider/$want_model, so it knows no such provider. Declare the provider and the model in $models"
        fi
      fi
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
