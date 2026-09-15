#!/bin/sh
# omp adapter, install: Oh My Pi for the base's account, from npm, and herdr's own
# OMP integration so herdr has lifecycle authority over its panes.
#
# THE DEFECT IT EXISTS TO REMOVE: the same as the pi adapter's, and one more that
# only appears when both are installed.
#
# ⛔ herdr REFUSES THE OMP INTEGRATION WHEN PI AND OMP RESOLVE TO THE SAME
# EXTENSION DIRECTORY, so the omp extension cannot be loaded by pi. The variable
# that causes it, PI_CODING_AGENT_DIR, is read by BOTH: one exported for pi
# silently redirects omp onto pi's directory. This adapter resolves both the way
# herdr resolves them and refuses FIRST, naming the two paths and the variable,
# because herdr's own refusal names only itself. WSL-89.
#
# It runs as root inside a wsl-toolkit base during `base ensure`, delivered on
# stdin, and it looks before it changes anything, so a second run changes nothing.
set -eu
umask 022
cd /

: "${TK_USER:?TK_USER is required}"
: "${TK_DISTRO:?TK_DISTRO is required}"

say() { printf '  * %s\n' "$*"; }
die() { printf 'omp adapter: %s\n' "$*" >&2; exit 3; }

OMP_PACKAGE=${TK_ADAPTER_PACKAGE:-@oh-my-pi/pi-coding-agent}
OMP_VERSION=${TK_ADAPTER_VERSION:-}
OMP_BIN=omp

TK_HOME=$(getent passwd "$TK_USER" | cut -d: -f6)
if [ -z "$TK_HOME" ] || [ ! -d "$TK_HOME" ]; then
  die "the account $TK_USER has no home directory"
fi
TK_GROUP=$(id -gn "$TK_USER")
PREFIX=$TK_HOME/.local

case $TK_DISTRO in
  wsl-toolkit-*) tool="wsl-toolkit --instance ${TK_DISTRO#wsl-toolkit-}" ;;
  *) tool=wsl-toolkit ;;
esac

as_account() {
  runuser -u "$TK_USER" -- env -i HOME="$TK_HOME" USER="$TK_USER" LOGNAME="$TK_USER" \
    SHELL=/bin/bash LANG=C.UTF-8 \
    npm_config_prefix="$PREFIX" \
    PATH="$PREFIX/bin:/usr/local/sbin:/usr/local/bin:/usr/bin:/bin" "$@"
}

command -v npm >/dev/null 2>&1 ||
  die "npm is not installed in this base, and omp is an npm package. Set base.toolset to developer or later"

# -- the collision, refused before anything is installed -------------------------------
# ⭐ RESOLVED EXACTLY AS herdr RESOLVES IT, from herdr's own integrations page:
# PI_CODING_AGENT_DIR when set, else $HOME/$PI_CONFIG_DIR/agent when PI_CONFIG_DIR is
# set, else ~/.omp/agent. Pi's is PI_CODING_AGENT_DIR when set, else ~/.pi/agent.
account_env() {
  as_account sh -c "printf '%s' \"\${$1:-}\""
}
PI_CODING_AGENT_DIR=$(account_env PI_CODING_AGENT_DIR)
PI_CONFIG_DIR=$(account_env PI_CONFIG_DIR)

if [ -n "$PI_CODING_AGENT_DIR" ]; then
  OMP_AGENT_DIR=$PI_CODING_AGENT_DIR
elif [ -n "$PI_CONFIG_DIR" ]; then
  OMP_AGENT_DIR=$TK_HOME/$PI_CONFIG_DIR/agent
else
  OMP_AGENT_DIR=$TK_HOME/.omp/agent
fi
if [ -n "$PI_CODING_AGENT_DIR" ]; then
  PI_AGENT_DIR=$PI_CODING_AGENT_DIR
else
  PI_AGENT_DIR=$TK_HOME/.pi/agent
fi

if [ "$OMP_AGENT_DIR" = "$PI_AGENT_DIR" ]; then
  printf 'omp adapter: pi and omp resolve to one extension directory, so herdr would refuse the omp integration.\n' >&2
  printf '  both resolve to  %s\n' "$OMP_AGENT_DIR" >&2
  printf '  the cause        PI_CODING_AGENT_DIR is read by BOTH, and it is set for %s\n' "$TK_USER" >&2
  printf '  the fix          unset it for that account, or set PI_CONFIG_DIR so omp resolves elsewhere\n' >&2
  printf '  read it          %s base exec -c %s\n' "$tool" "'printenv PI_CODING_AGENT_DIR PI_CONFIG_DIR'" >&2
  exit 3
fi

# -- omp --------------------------------------------------------------------------------
omp_version() {
  as_account "$OMP_BIN" --version 2>/dev/null | tr -d '\r' | head -1
}

want=$OMP_PACKAGE
[ -n "$OMP_VERSION" ] && want="$OMP_PACKAGE@$OMP_VERSION"

installed=$(omp_version || :)
if [ -n "$installed" ] && [ -z "$OMP_VERSION" ]; then
  say "omp $installed is installed for $TK_USER"
elif [ -n "$installed" ] && [ "$installed" = "$OMP_VERSION" ]; then
  say "omp $installed is installed for $TK_USER, at the version this base pins"
else
  say "installing $want for $TK_USER, with no lifecycle scripts"
  as_account npm install -g --ignore-scripts "$want" >/dev/null 2>&1 ||
    die "npm could not install $want. Read it with: $tool base exec -c 'npm install -g --ignore-scripts $want'"
  installed=$(omp_version || :)
  [ -n "$installed" ] || die "npm installed $want and $OMP_BIN --version answers nothing"
  say "installed omp $installed"
fi

# -- herdr's own OMP integration --------------------------------------------------------
if command -v herdr >/dev/null 2>&1 || [ -x /usr/local/bin/herdr ]; then
  herdr_bin=$(command -v herdr 2>/dev/null || printf '/usr/local/bin/herdr')
  install -d -o "$TK_USER" -g "$TK_GROUP" -m 0755 "$(dirname "$OMP_AGENT_DIR")" "$OMP_AGENT_DIR"
  if as_account "$herdr_bin" integration install omp >/dev/null 2>&1; then
    say "installed herdr's omp integration into $OMP_AGENT_DIR"
  else
    say "herdr refused its omp integration; omp works and herdr will read its screen instead"
  fi
else
  say "no herdr in this base, so no integration was installed. Add the herdr adapter to base.adapters"
fi

say "omp is ready: $tool base agent omp"
printf 'adapter-complete omp\n'
