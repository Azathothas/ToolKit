#!/bin/sh
# pi adapter, install: the Pi coding agent for the base's account, from npm, and
# herdr's own Pi integration so herdr has lifecycle authority over its panes.
#
# THE DEFECT IT EXISTS TO REMOVE: an agent herdr can only classify by reading its
# screen. Pi publishes lifecycle hooks and herdr ships an integration for them, so
# `idle`, `working` and `blocked` come from the agent itself rather than from a
# guess about what its terminal looks like.
#
# ⭐ NOTHING IS PIPED INTO A SHELL. Pi documents `npm install -g --ignore-scripts`
# as its normal install and says it needs no lifecycle script, so this repository's
# refusal to pipe a remote script costs nothing here. WSL-88.
#
# It runs as root inside a wsl-toolkit base during `base ensure`, delivered on
# stdin, and it looks before it changes anything, so a second run changes nothing.
set -eu
umask 022
cd /

: "${TK_USER:?TK_USER is required}"
: "${TK_DISTRO:?TK_DISTRO is required}"

say() { printf '  * %s\n' "$*"; }
die() { printf 'pi adapter: %s\n' "$*" >&2; exit 3; }

# -- what this adapter installs ------------------------------------------------------
# ⚠ THE PACKAGE IS NAMED HERE AND THE VERSION IS NOT PINNED, and that is a
# deliberate difference from the herdr adapter. npm resolves and verifies a package
# against the registry's own integrity value, which is the check a digest written
# here would duplicate and then go stale against; `version` on this adapter's entry
# in base.adapters pins one when an operator wants that.
PI_PACKAGE=${TK_ADAPTER_PACKAGE:-@earendil-works/pi-coding-agent}
PI_VERSION=${TK_ADAPTER_VERSION:-}
PI_BIN=pi

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

# as_account runs a command as the account, with a fresh environment of its own.
as_account() {
  runuser -u "$TK_USER" -- env -i HOME="$TK_HOME" USER="$TK_USER" LOGNAME="$TK_USER" \
    SHELL=/bin/bash LANG=C.UTF-8 \
    npm_config_prefix="$PREFIX" \
    PATH="$PREFIX/bin:/usr/local/sbin:/usr/local/bin:/usr/bin:/bin" "$@"
}

command -v npm >/dev/null 2>&1 ||
  die "npm is not installed in this base, and pi is an npm package. Set base.toolset to developer or later"

# -- pi --------------------------------------------------------------------------------
pi_version() {
  as_account "$PI_BIN" --version 2>/dev/null | tr -d '\r' | head -1
}

want=$PI_PACKAGE
[ -n "$PI_VERSION" ] && want="$PI_PACKAGE@$PI_VERSION"

installed=$(pi_version || :)
if [ -n "$installed" ] && [ -z "$PI_VERSION" ]; then
  say "pi $installed is installed for $TK_USER"
elif [ -n "$installed" ] && [ "$installed" = "$PI_VERSION" ]; then
  say "pi $installed is installed for $TK_USER, at the version this base pins"
else
  say "installing $want for $TK_USER, with no lifecycle scripts"
  # ⛔ --ignore-scripts. Pi's own documentation says it needs none for a normal
  # install, and a dependency's install script runs as this account with its home.
  as_account npm install -g --ignore-scripts "$want" >/dev/null 2>&1 ||
    die "npm could not install $want. Read it with: $tool base exec -c 'npm install -g --ignore-scripts $want'"
  installed=$(pi_version || :)
  [ -n "$installed" ] || die "npm installed $want and $PI_BIN --version answers nothing"
  say "installed pi $installed"
fi

# -- herdr's own Pi integration -----------------------------------------------------
# ⭐ herdr'S, NOT OURS. Pi is one of the agents herdr has a real integration for, so
# there is nothing here to write: `herdr integration install pi` puts herdr's own
# extension in Pi's extensions directory, and herdr owns its contents and its version.
# ⛔ The muse adapter carries a hook of this repository's own ONLY because herdr has
# no Muse integration and the one proposed upstream was closed unmerged.
if command -v herdr >/dev/null 2>&1 || [ -x /usr/local/bin/herdr ]; then
  herdr_bin=$(command -v herdr 2>/dev/null || printf '/usr/local/bin/herdr')
  # ⚠ herdr CREATES THE EXTENSIONS DIRECTORY ONLY WHEN PI'S AGENT DIRECTORY EXISTS,
  # and a fresh install has never been run, so it does not. Making it is this
  # adapter's job and it is the only directory it makes.
  install -d -o "$TK_USER" -g "$TK_GROUP" -m 0755 "$TK_HOME/.pi" "$TK_HOME/.pi/agent"
  if as_account "$herdr_bin" integration install pi >/dev/null 2>&1; then
    say "installed herdr's pi integration, which gives herdr lifecycle authority"
  else
    # ⚠ NOT FATAL, AND SAID RATHER THAN SWALLOWED. pi runs without it; what is lost
    # is herdr reporting state from the agent rather than from its screen.
    say "herdr refused its pi integration; pi works and herdr will read its screen instead"
  fi
else
  say "no herdr in this base, so no integration was installed. Add the herdr adapter to base.adapters"
fi

# -- the wrapper that puts the agent on a PANE's PATH ------------------------------------
# ⛔ npm INSTALLS INTO $HOME/.local/bin AND A herdr PANE DOES NOT HAVE IT ON PATH.
# Measured 2026-09-17 on the operator's base: a login shell's PATH is
# /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:... with no ~/.local/bin, and
# /etc/profile appends /usr/local/bin and nothing else. So `herdr agent start pi`
# ran `pi` in the pane, the shell answered `command not found`, and the start timed
# out waiting for an agent that was never going to appear.
#
# ⭐ THE FIX IS THE ONE muse ALREADY USES: a root-owned wrapper on the system PATH.
# It is what makes an agent reachable by the NAME herdr launches it by, from a pane,
# from SSH, and from `base agent`, without changing the account's environment.
install -d -m 0755 /usr/local/bin
cat > "/usr/local/bin/pi" <<WRAPPER
#!/bin/sh
# Written by wsl-toolkit's pi adapter. base ensure rewrites it.
if [ "\$(id -un)" != "$TK_USER" ]; then
  printf 'pi is installed for $TK_USER, and runs only as $TK_USER\n' >&2
  exit 126
fi
exec "$TK_HOME/.local/bin/pi" "\$@"
WRAPPER
chmod 0755 "/usr/local/bin/pi"
say "wrote /usr/local/bin/pi, so a herdr pane can start it by name"

# ⛔ AND IT IS READ BACK ON THE PATH A PANE ACTUALLY HAS, not on this script's own.
# A wrapper that a login shell never reaches is the same as no wrapper, and that is
# exactly the failure this section exists to remove.
resolved_on_pane_path=$(runuser -l "$TK_USER" -c "command -v pi" 2>/dev/null | tr -d '\r' | head -1)
[ -n "$resolved_on_pane_path" ] ||
  die "pi is installed and a login shell still cannot find it, so herdr could not start it in a pane"
say "a login shell resolves pi to $resolved_on_pane_path"

say "pi is ready: $tool base agent pi"

printf 'adapter-complete pi\n'
