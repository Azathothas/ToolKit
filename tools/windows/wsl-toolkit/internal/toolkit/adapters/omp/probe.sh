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
# The two paths install.sh writes outside the account's home. A case in the suite points
# both into a temporary directory, and nothing else sets them.
STAGE_DIR=${TK_OMP_STAGE_DIR:-/var/lib/wsl-toolkit/omp}
WRAPPER=${TK_OMP_WRAPPER:-/usr/local/bin/omp}

as_account() {
  runuser -u "$TK_USER" -- env -i HOME="$TK_HOME" USER="$TK_USER" LOGNAME="$TK_USER" \
    SHELL=/bin/bash LANG=C.UTF-8 npm_config_prefix="$PREFIX" \
    PATH="$PREFIX/bin:/usr/local/sbin:/usr/local/bin:/usr/bin:/bin" "$@"
}
# >>> the account's reads: begin
# account_read CMD ARG... is as_account CMD ARG..., stopped after 30 seconds where the
# userland carries timeout. ⛔ A FILE OF THE ACCOUNT'S CAN HOLD ITS READER: a named pipe
# swapped in after a `[ -f ]` test, or a file of its own FUSE mount that never answers.
# ⭐ Root's timeout runs runuser. ⚠ The hang is bounded, not removed. WSL-201.
account_read() (
  if command -v timeout >/dev/null 2>&1; then
    runuser() { timeout 30 runuser "$@"; }
  fi
  as_account "$@"
)

# is_omp_wrapper PATH is true when PATH is the adapter's separating wrapper. ⛔ The account
# reads it, and only once it is a regular file. Root reading a FIFO the account planted
# there would hold the probe until its timeout.
is_omp_wrapper() {
  # shellcheck disable=SC2016  # the single quotes hold the account's own script
  account_read sh -c '[ -f "$1" ] && head -3 "$1"' sh "$1" 2>/dev/null |
    grep -qF "# wsl-toolkit omp adapter wrapper"
}
# first_lines N PATH prints the first N lines of PATH, read by the account, when it is a
# file. ⛔ Root's head and sed followed a path the account's login shell named, as
# is_omp_wrapper says. WSL-201.
first_lines() {
  # shellcheck disable=SC2016  # the single quotes hold the account's own script
  account_read sh -c '[ -f "$2" ] && exec head -n "$1" "$2"' sh "$1" "$2" 2>/dev/null
}
# <<< the account's reads: end
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
  account_read sh -lc "printf '%s' \"\${$1:-}\"" 2>/dev/null
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

# ⛔ ASKED FIRST: IS OMP IN THIS BASE AT ALL. Without it, an omp that never installed,
# because an adapter before it stopped `base ensure`, read as five problems: no bun,
# "installed and a login shell cannot find it", and three more that only mean
# something for an omp that is there. The adapter installs bun itself. WSL-107,
# issue 37.
if [ -e "$WRAPPER" ] || [ -e "$PREFIX/bin/omp" ]; then installed=yes; else installed=no; fi
printf 'installed %s\n' "$installed"
if [ "${TK_PROBE_FOOTPRINT:-}" = 1 ]; then
  # ⚠ The npm shim is moved aside to omp-npm when the adapter separates omp's
  # directory, so the package is found through whichever launcher is npm's.
  shim=$PREFIX/bin/omp
  [ -e "$PREFIX/bin/omp-npm" ] && shim=$PREFIX/bin/omp-npm
  # ⚠ npm's package is counted where a launcher points into it, or where an omp from npm
  # left it. A release binary a refusal kept is counted too. Each is the adapter's.
  # WSL-138.
  # linksafe: npm_package_dir reads where the launcher links with readlink, and opens nothing
  npm_dir=$(npm_package_dir "$shim")
  [ -n "$npm_dir" ] || npm_dir=$PREFIX/lib/node_modules/@oh-my-pi/pi-coding-agent
  # linksafe: footprint tests each path and reads its size with du, which follows no link
  printf 'footprint %s\n' "$(footprint "$WRAPPER" "$PREFIX/bin/omp" "$PREFIX/bin/omp-npm" "$npm_dir" "$STAGE_DIR" "$TK_HOME/.omp")"
  exit 0
fi
if [ "$installed" = no ]; then
  problem "omp is not installed in this base. Run: base ensure"
  exit 0
fi

# ⭐ WHERE THE ACCOUNT'S omp CAME FROM, read from the file that is omp itself: beside the
# separating wrapper when there is one. install.sh takes omp's own release binary and
# takes npm only when that release could not be fetched. WSL-138.
omp_real=$PREFIX/bin/omp
if is_omp_wrapper "$omp_real"; then
  omp_real=$PREFIX/bin/omp-npm
fi
case $(account_read readlink -f "$omp_real" 2>/dev/null || :) in
  */node_modules/*) source=npm ;;
  *)
    # ⭐ A PREBUILT omp IS ONE ELF FILE, and it carries its own runtime. ⛔ Read only once it
    # is a regular file, so a FIFO there holds nothing.
    # shellcheck disable=SC2016  # the single quotes hold the account's own script
    if [ "$(account_read sh -c '[ -f "$1" ] && head -c 4 "$1"' sh "$omp_real" 2>/dev/null | od -An -c | tr -d ' \n')" = '177ELF' ]; then
      source=release
    else
      source=other
    fi
    ;;
esac
printf 'source %s\n' "$source"
if [ "$source" = npm ]; then
  printf "note omp came from npm, not from omp's own release. base ensure installs the release when it can fetch it\n"
fi

# ⭐ THE RUNTIME IS A FACT HERE, because omp from npm is a Bun program. A base with Node
# alone installs it and cannot run it. Reporting the version alone would have said
# "no version" for a missing runtime and a broken install alike, which are different
# problems with different fixes. Measured 2026-09-17.
# ⚠ omp's release binary carries its own runtime, so only an omp that is not one needs bun.
bun_version=$(as_account bun --version 2>/dev/null | tr -d '\r' | head -1)
if [ -n "$bun_version" ]; then
  printf 'bun %s\n' "$bun_version"
elif [ "$source" != release ]; then
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

# ⛔ AND IT MUST RESOLVE TO THIS TOOL'S WRAPPER, NOT MERELY RESOLVE. Measured on
# 2026-09-17: `bootstrap.sh` wrote `export PATH="$HOME/.local/bin:$PATH"` into the
# account's profile, so after `base bootstrap` a login shell found the vendor's own
# launcher FIRST and the wrapper on the system path was never reached. It now puts
# its prefix last and rewrites that line. WSL-147. omp keeps its startup model and effort in its own configuration, so those survive; the account guard does not.
# ⚠ A probe that only asked whether the name resolved called that base healthy.
if [ -n "$pane_path" ]; then
  if first_lines 3 "$pane_path" | grep -q "wsl-toolkit's omp adapter"; then
    printf 'wrapper_on_login_path yes\n'
  else
    printf 'wrapper_on_login_path no\n'
    # ⭐ THE PROBLEM NAMES ITS REMEDY. bootstrap.sh now puts its prefix after the
    # system path and rewrites the line an earlier run wrote. WSL-147.
    problem "a login shell resolves omp to $pane_path, which is not this tool's wrapper. The account's own prefix is ahead of /usr/local/bin, which an earlier bootstrap.sh put there, so the wrapper and everything it sets are bypassed. Run: base bootstrap, which moves its own line after the system path. A line the account wrote itself is moved by hand"
  fi
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
if [ -n "$omp_path" ] && is_omp_wrapper "$omp_path"; then
  # shellcheck disable=SC2016  # the single quotes hold the account's own script
  wrapped_dir=$(account_read sh -c '[ -f "$1" ] && exec sed -n "s/^PI_CODING_AGENT_DIR=//p" "$1"' sh "$omp_path" 2>/dev/null | head -1)
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
