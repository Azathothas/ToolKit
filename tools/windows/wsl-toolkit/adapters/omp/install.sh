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
TK_ADAPTER_SEPARATE_AGENT_DIR=${TK_ADAPTER_SEPARATE_AGENT_DIR:-}
OMP_SEPARATED=no
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
  if [ -z "$TK_ADAPTER_SEPARATE_AGENT_DIR" ]; then
    printf 'omp adapter: pi and omp resolve to one extension directory, so herdr would refuse the omp integration.\n' >&2
    printf '  both resolve to  %s\n' "$OMP_AGENT_DIR" >&2
    printf '  the cause        PI_CODING_AGENT_DIR is read by BOTH, and it is set for %s\n' "$TK_USER" >&2
    printf '  the fix          unset it for that account, or set PI_CONFIG_DIR so omp resolves elsewhere\n' >&2
    printf '  or               set "separate_agent_dir": true on this adapter and omp keeps one of its own\n' >&2
    printf '  read it          %s base exec -c %s\n' "$tool" "'printenv PI_CODING_AGENT_DIR PI_CONFIG_DIR'" >&2
    exit 3
  fi
  # ⭐ THE OPT-IN, RULED BY THE OPERATOR ON 2026-09-17. The refusal above stays the
  # default; this is the other half they asked for.
  #
  # ⛔ AND IT IS ONLY TRUE WITH THE WRAPPER BELOW. Resolving omp's directory here
  # and installing the integration into it would leave omp reading
  # PI_CODING_AGENT_DIR again at run time and looking in pi's directory, so herdr
  # would find no extension where the install said it put one. Separating at
  # install time alone is a claim; the wrapper is what makes it a fact.
  OMP_AGENT_DIR=$TK_HOME/.omp/agent
  OMP_SEPARATED=yes
  say "pi and omp collided at $PI_AGENT_DIR, and separate_agent_dir is set"
  say "omp takes $OMP_AGENT_DIR, and PI_CODING_AGENT_DIR is left exactly as the operator set it"
fi

# -- omp is a Bun program, and a base built as documented has Node ----------------------
# ⛔ MEASURED ON 2026-09-17, NOT READ. The package's shim is `#!/usr/bin/env bun` and its
# package.json requires `bun >= 1.3.14`. Installed on a base with Node alone, omp answers
# `env: 'bun': No such file or directory`, rc 127, and the adapter reported a version of
# nothing. WSL-89's premise never recorded this, because nothing had run it.
#
# ⛔ NOT FROM npm. `npm install -g --ignore-scripts bun` exits 0 and leaves a bun that
# refuses to run: `Error: Bun's postinstall script was not run.` The postinstall is what
# downloads the real binary, so getting a working bun that way means running a
# third-party install script that fetches an unverified binary - the class this
# repository refuses, and the reason omp's own installer is not piped into a shell here.
#
# ⭐ FROM THE DISTRIBUTION, signed and verified by its own package manager. Arch carries
# bun 1.4.2 in `extra`, and with it omp answers `omp/18.2.3`. Ruled by the operator on
# 2026-09-17, "yes install bun".
#
# ⚠ NOT ADDED TO bootstrap.sh's SHARED TABLE. That file is fetched by URL and is not
# where a runtime one adapter needs belongs, which is the rule WSL-88 states for pi.
if as_account sh -c 'command -v bun' >/dev/null 2>&1; then
  say "bun $(as_account bun --version 2>/dev/null | tr -d '\r' | head -1) is installed, and omp needs it"
else
  say "omp is a Bun program and this base has none; installing bun from the distribution"
  if command -v pacman >/dev/null 2>&1; then
    pacman -Sy --noconfirm >/dev/null 2>&1 || :
    pacman -S --noconfirm bun >/dev/null 2>&1 ||
      die "pacman could not install bun, which omp needs. Read it with: $tool base exec --root -c 'pacman -S bun'"
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache bun >/dev/null 2>&1 ||
      die "apk could not install bun, which omp needs. Read it with: $tool base exec --root -c 'apk add bun'"
  else
    # ⛔ NAMED, NOT GUESSED. omp is driven on the arch preset, and a base on a
    # manager this adapter has not measured gets a refusal that says so rather than
    # an install nobody has seen work.
    die "omp needs the Bun runtime and this base's package manager is not one this adapter has installed it with. Install bun in the base, then run this again"
  fi
  bun_version=$(as_account bun --version 2>/dev/null | tr -d '\r' | head -1)
  [ -n "$bun_version" ] || die "bun installed and the account cannot run it, so omp still will not start"
  say "installed bun $bun_version"
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

# -- the wrapper that makes a separated directory true at RUN time -----------------------
# ⛔ WITHOUT THIS THE SEPARATION IS A CLAIM. The integration would sit in omp's own
# directory while omp, started later, reads the account's PI_CODING_AGENT_DIR again and
# looks in pi's. herdr would then find no extension where the install said it put one.
#
# ⭐ A WRAPPER COSTS herdr NOTHING FOR omp, and that is why it is allowed here and is
# not the answer for pi. omp reports state and session through herdr's socket API and,
# in herdr's own words, "does not require native process detection for the omp
# executable"; pi's authority is lifecycle hooks whose agent herdr must still identify.
if [ "$OMP_SEPARATED" = yes ]; then
  OMP_WRAPPER=$TK_HOME/.local/bin/omp
  OMP_REAL=$TK_HOME/.local/bin/omp-npm
  OMP_MARK="# wsl-toolkit omp adapter wrapper"
  # ⛔ DETECTED BY CONTENT, NOT BY PATH. npm's own prefix for this account IS
  # $HOME/.local, so npm's shim and any wrapper want the same filename and a guard
  # that read the PATH alone called the real shim a wrapper and refused. Measured
  # 2026-09-17: `this adapter will not wrap a wrapper`, over npm's shim.
  if [ -f "$OMP_WRAPPER" ] && head -3 "$OMP_WRAPPER" 2>/dev/null | grep -qF "$OMP_MARK"; then
    [ -x "$OMP_REAL" ] || die "the wrapper at $OMP_WRAPPER is in place and $OMP_REAL is gone, so omp has nothing to run"
    say "the omp wrapper is already in place, and $OMP_REAL is what it runs"
  else
    [ -f "$OMP_WRAPPER" ] || die "omp installed and there is no $OMP_WRAPPER to wrap, so npm put it somewhere this adapter does not know"
    mv -f "$OMP_WRAPPER" "$OMP_REAL" || die "npm's omp could not be moved aside to $OMP_REAL"
    chown "$TK_USER:$TK_GROUP" "$OMP_REAL"
    cat > "$OMP_WRAPPER" <<WRAPPER
#!/bin/sh
$OMP_MARK
# Written because pi and omp resolved to one extension directory and this base set
# "separate_agent_dir": true. It changes PI_CODING_AGENT_DIR for OMP ALONE; the
# account's own value is untouched and pi keeps reading it.
#
# \$0.npm is npm's own shim, moved aside by the adapter. A reinstall of the package
# puts a fresh shim back at this path, and the next base ensure wraps it again.
PI_CODING_AGENT_DIR=$OMP_AGENT_DIR
export PI_CODING_AGENT_DIR
exec $OMP_REAL "\$@"
WRAPPER
    chown "$TK_USER:$TK_GROUP" "$OMP_WRAPPER"
    chmod 0755 "$OMP_WRAPPER"
    say "moved npm's omp to $OMP_REAL and wrote the wrapper, which gives omp alone $OMP_AGENT_DIR"
  fi
  # ⛔ AND THE SEPARATING WRAPPER HAS TO WIN WHERE npm PUT omp. ⚠ This is deliberately
  # NOT the login-shell check further down: a login shell resolves omp through
  # /usr/local/bin, which execs THIS wrapper, so both are true and they answer
  # different questions. This one asks whether the npm prefix's omp is the wrapper.
  OMP_RESOLVED=$(as_account sh -c "command -v omp" 2>/dev/null | tr -d '\r' | head -1)
  [ "$OMP_RESOLVED" = "$OMP_WRAPPER" ] ||
    die "the wrapper is at $OMP_WRAPPER and the account's prefix resolves omp to $OMP_RESOLVED, so the separation would not hold at run time"
  as_account omp --version >/dev/null 2>&1 ||
    die "the wrapper is in place and omp will not run through it"
  say "the account's omp resolves to the wrapper and runs"
fi

# -- herdr's own OMP integration --------------------------------------------------------
if command -v herdr >/dev/null 2>&1 || [ -x /usr/local/bin/herdr ]; then
  herdr_bin=$(command -v herdr 2>/dev/null || printf '/usr/local/bin/herdr')
  install -d -o "$TK_USER" -g "$TK_GROUP" -m 0755 "$(dirname "$OMP_AGENT_DIR")" "$OMP_AGENT_DIR"
  # ⭐ THE INSTALL RUNS UNDER THE SAME OVERRIDE THE WRAPPER APPLIES, so install time
  # and run time resolve to one directory rather than two.
  #
  # ⛔ AND ONLY WHEN SEPARATING. PI_CODING_AGENT_DIR is read by BOTH agents, so
  # exporting it for this command on a base that did NOT separate makes herdr resolve
  # pi's directory to omp's value too, see one directory for two agents, and refuse
  # the very install it was asked for. Measured on 2026-09-17: the adapter reported
  # "herdr refused its omp integration" on a base with no collision at all, and the
  # same command by hand, without the override, exited 0.
  if [ "$OMP_SEPARATED" = yes ]; then
    set -- as_account env PI_CODING_AGENT_DIR="$OMP_AGENT_DIR" "$herdr_bin" integration install omp
  else
    set -- as_account "$herdr_bin" integration install omp
  fi
  if "$@" >/dev/null 2>&1; then
    say "installed herdr's omp integration into $OMP_AGENT_DIR"
  else
    say "herdr refused its omp integration; omp works and herdr will read its screen instead"
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
cat > "/usr/local/bin/omp" <<WRAPPER
#!/bin/sh
# Written by wsl-toolkit's omp adapter. base ensure rewrites it.
if [ "\$(id -un)" != "$TK_USER" ]; then
  printf 'omp is installed for $TK_USER, and runs only as $TK_USER\n' >&2
  exit 126
fi
exec "$TK_HOME/.local/bin/omp" "\$@"
WRAPPER
chmod 0755 "/usr/local/bin/omp"
say "wrote /usr/local/bin/omp, so a herdr pane can start it by name"

# ⛔ AND IT IS READ BACK ON THE PATH A PANE ACTUALLY HAS, not on this script's own.
# A wrapper that a login shell never reaches is the same as no wrapper, and that is
# exactly the failure this section exists to remove.
resolved_on_pane_path=$(runuser -l "$TK_USER" -c "command -v omp" 2>/dev/null | tr -d '\r' | head -1)
[ -n "$resolved_on_pane_path" ] ||
  die "omp is installed and a login shell still cannot find it, so herdr could not start it in a pane"
say "a login shell resolves omp to $resolved_on_pane_path"

say "omp is ready: $tool base agent omp"

printf 'adapter-complete omp\n'
