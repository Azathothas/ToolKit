#!/bin/sh
# omp adapter, install: Oh My Pi for the base's account, and herdr's own OMP
# integration so herdr has lifecycle authority over its panes. omp comes from its own
# release while that digest is the one pinned here. npm is taken only when the release
# cannot be fetched.
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

# -- what this adapter installs, and from where ------------------------------------
# ⭐ omp'S OWN RELEASE FIRST, AND npm ONLY WHEN THAT RELEASE CANNOT BE FETCHED. Ruling
# 20, as ruling 36 amended it. omp's own installer, `curl -fsSL https://omp.sh/install |
# sh`, takes a prebuilt binary whenever the machine has no Bun. Every omp release on
# GitHub carries that binary per platform, beside a SHA256SUMS.txt that covers it.
# WSL-138.
#
# ⛔ THE BINARY IS INSTALLED ONLY WHILE ITS DIGEST IS THE ONE PINNED HERE. That is the
# route ruling 4 set for Muse: download, save, print the length and SHA-256, and install
# the saved file. Each digest is the line omp v18.13.1's own SHA256SUMS.txt carries, and
# GitHub's own asset digest agrees. Both files were measured on download. The x86_64 one
# is 275,986,608 bytes, and the aarch64 one is 237,423,608 bytes. The x86_64 one answered
# `omp/18.13.1` once a scratch base installed it. ⛔ Any other digest
# stops the ensure with exit 2 and keeps the file to be read. It never takes npm.
#
# ⚠ NOT omp.sh/install, BECAUSE OF WHAT A DIGEST ON IT WOULD PIN. It is served from the
# main branch, so its digest moves with every commit to that script. It asks a third
# service, build.stencil.so, for the newest build at run time, and with a Bun present it
# installs from npm instead.
OMP_RELEASE=18.13.1
OMP_X86_64_PINNED_SHA256=ee2961c7da02e514a2475ee958c2fd85dbabf4d0907949ba5022edb2951f84b5
OMP_AARCH64_PINNED_SHA256=803db725f9e5bd177ce5274f7718081481f367829f773d984a0a9611f4713244
OMP_URL_BASE=https://github.com/can1357/oh-my-pi/releases/download

# ⭐ A NEWER RELEASE IS A CONFIGURATION CHANGE, NOT AN EDIT TO THIS FILE, as herdr's is.
# `version` and `sha256` on this adapter's entry arrive as TK_ADAPTER_VERSION and
# TK_ADAPTER_SHA256_<ARCH>, and the executable refuses one without the other. ⭐ The npm
# fallback installs the same version, so the two routes never disagree on which omp.
if [ -n "${TK_ADAPTER_VERSION:-}" ]; then
  OMP_RELEASE=$TK_ADAPTER_VERSION
  OMP_X86_64_PINNED_SHA256=${TK_ADAPTER_SHA256_X86_64:-}
  OMP_AARCH64_PINNED_SHA256=${TK_ADAPTER_SHA256_AARCH64:-}
fi
# ⚠ THE FALLBACK'S PACKAGE. npm checks it against the registry's own integrity value.
OMP_PACKAGE=${TK_ADAPTER_PACKAGE:-@oh-my-pi/pi-coding-agent}
TK_ADAPTER_SEPARATE_AGENT_DIR=${TK_ADAPTER_SEPARATE_AGENT_DIR:-}
OMP_SEPARATED=no
OMP_BIN=omp

# The two paths this writes outside the account's home, and the file that marks a musl
# base. A case in the suite points each into a temporary directory, and nothing else
# sets them.
STAGE_DIR=${TK_OMP_STAGE_DIR:-/var/lib/wsl-toolkit/omp}
WRAPPER=${TK_OMP_WRAPPER:-/usr/local/bin/omp}
MUSL_MARK=${TK_OMP_MUSL_MARK:-/etc/alpine-release}

TK_HOME=$(getent passwd "$TK_USER" | cut -d: -f6)
if [ -z "$TK_HOME" ] || [ ! -d "$TK_HOME" ]; then
  die "the account $TK_USER has no home directory"
fi
PREFIX=$TK_HOME/.local

case $TK_DISTRO in
  # The default base: the canonical name and the older one. WSL-167.
  wsl-toolkit | wsl-toolkit-jobs) tool=wsl-toolkit ;;
  wsl-toolkit-*) tool="wsl-toolkit --instance ${TK_DISTRO#wsl-toolkit-}" ;;
  *) tool=wsl-toolkit ;;
esac

as_account() {
  runuser -u "$TK_USER" -- env -i HOME="$TK_HOME" USER="$TK_USER" LOGNAME="$TK_USER" \
    SHELL=/bin/bash LANG=C.UTF-8 \
    npm_config_prefix="$PREFIX" \
    PATH="$PREFIX/bin:/usr/local/sbin:/usr/local/bin:/usr/bin:/bin" "$@"
}

# >>> the account's own writes: begin
# ⛔ ROOT WRITES NOTHING IN THE ACCOUNT'S HOME. This script runs as root, and chown,
# install -d, cp and a redirect each follow a link the account placed there. Where
# base.passwordless_sudo is off, or in a herdr pane under private_net, the account is
# not root. Root's write through that link was one the account could not make. As the
# account, a link reaches only what the account could already write. WSL-201.
#
# account_dirs MODE DIR... makes each DIR as the account and gives it MODE. ⚠ A DIR that
# is a link keeps the mode of what it names, which is the account's own to set.
account_dirs() {
  # shellcheck disable=SC2016  # the single quotes hold the account's own script
  as_account sh -c '
    mode=$1
    shift
    for dir do
      mkdir -p "$dir" || exit 1
      [ -L "$dir" ] || chmod "$mode" "$dir" || exit 1
    done' sh "$@"
}

# account_put DEST MODE writes its standard input to DEST as the account. ⭐ A file of
# the account's own beside DEST takes the bytes and is renamed over it, so a link at
# DEST is replaced and never written through. ⛔ A directory at DEST is refused.
account_put() {
  # shellcheck disable=SC2016  # the single quotes hold the account's own script
  as_account sh -c '
    [ ! -d "$1" ] || exit 1
    tmp=$(mktemp "${1%/*}/.wsl-toolkit.XXXXXX") || exit 1
    if cat > "$tmp" && chmod "$2" "$tmp" && mv -f "$tmp" "$1"; then
      exit 0
    fi
    rm -f "$tmp"
    exit 1' sh "$1" "$2"
}

# account_same SRC DEST is true when DEST, read by the account, is a file holding SRC's
# bytes. ⛔ ROOT READS ONLY SRC, its own file outside the home. Root's read of DEST
# followed a link the account placed there, and a named pipe there held base ensure.
account_same() {
  as_src=$(sha256sum < "$1") || return 1
  # shellcheck disable=SC2016  # the single quotes hold the account's own script
  [ "$as_src" = "$(as_account sh -c '[ -f "$1" ] && exec sha256sum < "$1"' sh "$2" 2>/dev/null)" ]
}
# <<< the account's own writes: end

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

# -- omp, from its own release ------------------------------------------------------------
omp_version() {
  as_account "$OMP_BIN" --version 2>/dev/null | tr -d '\r' | head -1
}

case "$(uname -m)" in
  x86_64) OMP_ASSET=omp-linux-x64; OMP_WANT=$OMP_X86_64_PINNED_SHA256 ;;
  aarch64) OMP_ASSET=omp-linux-arm64; OMP_WANT=$OMP_AARCH64_PINNED_SHA256 ;;
  *) OMP_ASSET=''; OMP_WANT='' ;;
esac
# ⚠ A musl BASE TAKES ANOTHER BUILD, and none is pinned here, so it takes npm.
if [ -f "$MUSL_MARK" ]; then
  OMP_ASSET=''
  OMP_WANT=''
fi
# ⛔ AN EMPTY DIGEST IS A REFUSAL, NOT AN UNCHECKED DOWNLOAD AND NOT A FALLBACK. It means
# a configuration moved the version and gave no digest for THIS architecture.
if [ -n "$OMP_ASSET" ] && [ -z "$OMP_WANT" ]; then
  die "base.adapters moves omp to $OMP_RELEASE and names no sha256 for $(uname -m). Add it, or remove the version to use the one this adapter pins"
fi

# is_omp_wrapper PATH is true when PATH is the separating wrapper further down. ⛔ The
# account reads it, and only once it is a regular file. Root reading a FIFO the account
# planted there would hold base ensure until its timeout.
is_omp_wrapper() {
  # shellcheck disable=SC2016  # the single quotes hold the account's own script
  as_account sh -c '[ -f "$1" ] && head -3 "$1"' sh "$1" 2>/dev/null |
    grep -qF "# wsl-toolkit omp adapter wrapper"
}

# ⭐ THE FILE THAT IS omp ITSELF. On a base that took separate_agent_dir, the account's
# omp is the wrapper further down. The program it runs is beside it, so a new release goes
# there and the wrapper stays.
OMP_LAUNCHER=$PREFIX/bin/omp
OMP_TARGET=$OMP_LAUNCHER
if is_omp_wrapper "$OMP_LAUNCHER"; then
  OMP_TARGET=$PREFIX/bin/omp-npm
fi

# omp_from_release is true when the account's omp is the release whose digest is in force.
# ⚠ Read as the account, because the file is its own.
omp_from_release() {
  if [ -z "$OMP_ASSET" ] || [ ! -f "$OMP_TARGET" ]; then
    return 1
  fi
  [ "$(as_account sha256sum "$OMP_TARGET" 2>/dev/null | cut -d' ' -f1)" = "$OMP_WANT" ]
}

# omp_release_install fetches omp's release binary, checks it and installs it. It answers
# 1 when the binary cannot be fetched, which is the one case npm is taken for.
omp_release_install() {
  for need in curl sha256sum; do
    if ! command -v "$need" >/dev/null 2>&1; then
      say "this base has no $need, so omp's release binary cannot be fetched"
      return 1
    fi
  done
  # ⛔ set -e DOES NOT REACH IN HERE, because this runs as a condition, so every step
  # that can fail says so itself.
  install -d -m 0755 "$STAGE_DIR" || die "no directory $STAGE_DIR to save omp's release binary in"
  saved=$STAGE_DIR/$OMP_ASSET
  url=$OMP_URL_BASE/v$OMP_RELEASE/$OMP_ASSET
  fetched=$(mktemp "$STAGE_DIR/$OMP_ASSET.XXXXXX") || die "no temporary file in $STAGE_DIR"
  # ⛔ A SIGNAL ENDS THE RUN, and the exit takes the partial file with it. A handler that
  # only removed the file let the run go on. The killed fetch then read as a fetch failure,
  # and that is the one case npm is taken for.
  trap 'rm -f "$fetched"' 0
  trap 'exit 129' 1
  trap 'exit 130' 2
  trap 'exit 143' 15
  if ! fetch_error=$(curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location --max-redirs 5 \
      "$url" --output "$fetched" 2>&1); then
    rm -f "$fetched"
    trap - 0 1 2 15
    say "omp's release binary could not be fetched from $url: $(printf '%s' "$fetch_error" | tail -1)"
    return 1
  fi
  if ! chmod 0644 "$fetched" || ! mv -f "$fetched" "$saved"; then
    die "omp's release binary could not be saved at $saved"
  fi
  trap - 0 1 2 15
  length=$(wc -c < "$saved" | tr -d ' ')
  digest=$(sha256sum "$saved" | cut -d' ' -f1)
  say "saved omp's release binary at $saved: $length bytes, SHA-256 $digest"
  # ⛔ A FILE WHOSE DIGEST IS NOT THE PINNED ONE IS KEPT TO BE READ AND NEVER RUN. npm is
  # NOT taken in its place, because a changed digest is not a download failure. Whoever
  # reads the refusal compares the file with the release's own list. Where they agree,
  # they pin it. Ruling 36.
  if [ "$digest" != "$OMP_WANT" ]; then
    printf "omp adapter: omp's release binary is not the one this adapter pins, so it was not installed and npm was not tried.\n" >&2
    printf '  saved at    %s, %s bytes, SHA-256 %s\n' "$saved" "$length" "$digest" >&2
    printf '  pinned      omp %s, SHA-256 %s\n' "$OMP_RELEASE" "$OMP_WANT" >&2
    printf "  compare it  with the release's own list, %s\n" "$OMP_URL_BASE/v$OMP_RELEASE/SHA256SUMS.txt" >&2
    printf '  pin it      where they agree, add "version": "%s", "sha256": {"%s": "%s"} to the omp entry in base.adapters, then run base ensure again\n' \
      "$OMP_RELEASE" "$(uname -m)" "$digest" >&2
    printf '  stop        where they disagree. The file is not the one omp published\n' >&2
    exit 2
  fi
  # ⭐ COPIED BY THE ACCOUNT, beside the file it replaces, and moved over it in one step.
  as_account mkdir -p "$PREFIX/bin" || die "the account could not make $PREFIX/bin"
  staged=$(as_account mktemp "$PREFIX/bin/.omp.XXXXXX") || die "the account could not make a file in $PREFIX/bin"
  if ! as_account cp "$saved" "$staged" || ! as_account chmod 0755 "$staged" || ! as_account mv -f "$staged" "$OMP_TARGET"; then
    as_account rm -f "$staged"
    die "the account could not put omp's release binary at $OMP_TARGET"
  fi
  # ⚠ The installed copy holds the same bytes, so the saved one goes.
  rm -f "$saved"
  return 0
}

# drop_npm_package removes the package npm put omp in, once omp's release is in use. A
# base built before WSL-138 holds one, and nothing reads it after that. ⚠ As the account,
# whose files they are.
drop_npm_package() {
  case $OMP_PACKAGE in
    '' | /* | *..*) return 0 ;;
  esac
  npm_dir=$PREFIX/lib/node_modules/$OMP_PACKAGE
  if [ ! -e "$npm_dir" ] && [ ! -L "$npm_dir" ]; then
    return 0
  fi
  if as_account rm -rf "$npm_dir"; then
    # ⚠ AND ITS SCOPE, once nothing else is in it. A scratch base kept an empty @oh-my-pi.
    case $OMP_PACKAGE in
      @*/*) as_account rmdir "${npm_dir%/*}" 2>/dev/null || : ;;
    esac
    say "removed $npm_dir, npm's omp, which the release in use does not read"
  else
    say "could not remove $npm_dir, npm's omp, which the release in use does not read"
  fi
  return 0
}

omp_from=npm
installed=$(omp_version || :)
if omp_from_release; then
  omp_from=release
  say "omp ${installed#omp/} is installed for $TK_USER from its release, whose digest this adapter pins"
  # ⚠ A BINARY A REFUSAL KEPT IS MOOT ONCE THE ONE IN USE IS THE PINNED ONE, and it is
  # 276 MB. pi's adapter met the same leftover on a scratch base.
  if [ -e "$STAGE_DIR/$OMP_ASSET" ]; then
    rm -f "$STAGE_DIR/$OMP_ASSET"
    say "removed $STAGE_DIR/$OMP_ASSET, which an earlier refusal kept and the omp in use does not need"
  fi
elif [ -n "$OMP_ASSET" ] && omp_release_install; then
  omp_from=release
  installed=$(omp_version || :)
  [ "${installed#omp/}" = "$OMP_RELEASE" ] ||
    die "omp's release $OMP_RELEASE is installed and $OMP_BIN --version answers '${installed:-nothing}'"
  say "installed omp ${installed#omp/} for $TK_USER from its release binary, digest verified"
else
  [ -n "$OMP_ASSET" ] || say "omp's releases carry no build this adapter pins for this machine"
fi
if [ "$omp_from" = release ]; then
  drop_npm_package
fi

# -- the fallback: omp from npm is a Bun program, and a base built as documented has Node --
# ⛔ MEASURED ON 2026-09-17, NOT READ. The package's shim is `#!/usr/bin/env bun` and its
# package.json requires `bun >= 1.3.14`. Installed on a base with Node alone, omp answers
# `env: 'bun': No such file or directory`, rc 127, and the adapter reported a version of
# nothing. WSL-89's premise never recorded this, because nothing had run it.
#
# ⭐ THE RELEASE BINARY CARRIES ITS OWN RUNTIME, so a base that took it needs no Bun.
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
if [ "$omp_from" = release ]; then
  say "omp's release binary carries its own runtime, so no Bun is installed for it"
elif as_account sh -c 'command -v bun' >/dev/null 2>&1; then
  say "bun $(as_account bun --version 2>/dev/null | tr -d '\r' | head -1) is installed, and omp from npm needs it"
else
  say "omp from npm is a Bun program and this base has none; installing bun from the distribution"
  if command -v pacman >/dev/null 2>&1; then
    # ⛔ ONE TRANSACTION THAT SYNCS AND UPGRADES. `-Sy` and then `-S` is the
    # partial upgrade Arch does not support: bun built against a newer library
    # than the base holds. provision.sh's engine step says the same. WSL-188.
    pacman -Syu --noconfirm --needed bun >/dev/null 2>&1 ||
      die "pacman could not install bun, which omp needs. Read it with: $tool base exec --root -c 'pacman -Syu bun'"
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

if [ "$omp_from" = npm ]; then
  command -v npm >/dev/null 2>&1 ||
    die "omp's release could not be fetched, and npm is not installed in this base for the fallback. Run base ensure again when the release can be fetched, or set base.toolset to developer or later"
  want=$OMP_PACKAGE@$OMP_RELEASE
  case $(as_account readlink -f "$OMP_TARGET" 2>/dev/null || :) in
    */node_modules/*) ;;
    *) installed= ;;
  esac
  if [ -n "$installed" ] && [ "${installed#omp/}" = "$OMP_RELEASE" ]; then
    say "omp ${installed#omp/} from npm is installed for $TK_USER, at the version this adapter pins"
  else
    # ⚠ npm WILL NOT REPLACE A LAUNCHER IT DID NOT MAKE. So a release's binary or the
    # wrapper is moved aside first, and it comes back when npm fails.
    aside=
    if [ -e "$OMP_LAUNCHER" ] || [ -L "$OMP_LAUNCHER" ]; then
      case $(as_account readlink -f "$OMP_LAUNCHER" 2>/dev/null || :) in
        */node_modules/*) ;;
        *)
          aside=$PREFIX/bin/.omp.before-npm
          as_account mv -f "$OMP_LAUNCHER" "$aside" || die "the account could not move $OMP_LAUNCHER aside for npm"
          ;;
      esac
    fi
    say "installing $want from npm for $TK_USER, with no lifecycle scripts"
    if ! as_account npm install -g --ignore-scripts "$want" >/dev/null 2>&1; then
      [ -z "$aside" ] || as_account mv -f "$aside" "$OMP_LAUNCHER" || :
      die "npm could not install $want. Read it with: $tool base exec -c 'npm install -g --ignore-scripts $want'"
    fi
    [ -z "$aside" ] || as_account rm -f "$aside"
    installed=$(omp_version || :)
    [ -n "$installed" ] || die "npm installed $want and $OMP_BIN --version answers nothing"
    say "installed omp ${installed#omp/} from npm"
  fi
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
  if is_omp_wrapper "$OMP_WRAPPER"; then
    [ -x "$OMP_REAL" ] || die "the wrapper at $OMP_WRAPPER is in place and $OMP_REAL is gone, so omp has nothing to run"
    say "the omp wrapper is already in place, and $OMP_REAL is what it runs"
  else
    [ -f "$OMP_WRAPPER" ] || die "omp installed and there is no $OMP_WRAPPER to wrap, so it went somewhere this adapter does not know"
    # ⛔ THE ACCOUNT MOVES ITS OWN omp AND WRITES THE WRAPPER. Root's chown here followed
    # the link the account could leave at that path. WSL-201.
    as_account mv -f "$OMP_WRAPPER" "$OMP_REAL" || die "omp could not be moved aside to $OMP_REAL"
    account_put "$OMP_WRAPPER" 0755 <<WRAPPER || die "the account could not write the wrapper at $OMP_WRAPPER"
#!/bin/sh
$OMP_MARK
# Written because pi and omp resolved to one extension directory and this base set
# "separate_agent_dir": true. It changes PI_CODING_AGENT_DIR for OMP ALONE; the
# account's own value is untouched and pi keeps reading it.
#
# omp-npm beside it is omp itself, its release binary or npm's shim, moved aside by
# the adapter. A reinstall puts omp back at this path, and the next base ensure wraps
# it again.
PI_CODING_AGENT_DIR=$OMP_AGENT_DIR
export PI_CODING_AGENT_DIR
exec $OMP_REAL "\$@"
WRAPPER
    say "moved omp to $OMP_REAL and wrote the wrapper, which gives omp alone $OMP_AGENT_DIR"
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

# -- the startup model and reasoning effort --------------------------------------------
# ⛔ IT GOES IN OMP'S OWN CONFIGURATION AND NOT IN AN ENVIRONMENT. A herdr pane inherits
# the herdr SERVICE's environment, not a login shell's, measured on 2026-09-17, so a
# variable exported in a profile never reaches an agent herdr starts.
#
# ⭐ OMP'S OWN DOOR, AND ITS OWN COMMAND WRITES IT. `omp config` reads and writes
# ~/.omp/agent/config.yml, so nothing here parses or rewrites YAML by hand. Measured in
# the base on 2026-09-17: `defaultThinkingLevel` is a settings key omp lists, and the
# startup model is the `default` entry of the `modelRoles` record, which omp's own code
# reads as `defaultModelPattern ?? getModelRole("default")`.
#
# ⚠ THE RECORD IS MERGED, NEVER REPLACED, so a role the operator set for smol, slow or
# plan keeps its model.
if [ -n "${TK_ADAPTER_MODEL:-}${TK_ADAPTER_EFFORT:-}" ]; then
  if [ -n "${TK_ADAPTER_EFFORT:-}" ]; then
    as_account "$OMP_BIN" config set defaultThinkingLevel "$TK_ADAPTER_EFFORT" >/dev/null 2>&1 ||
      die "omp refused the effort $TK_ADAPTER_EFFORT. Read what it takes with: $tool base exec -c 'omp config list'"
  fi
  if [ -n "${TK_ADAPTER_MODEL:-}" ]; then
    if ! command -v jq >/dev/null 2>&1; then
      say "no jq in this base, so omp's startup model was not set. Set base.toolset to developer or later"
    else
      roles=$(as_account "$OMP_BIN" config get modelRoles 2>/dev/null | tr -d '\r')
      printf '%s' "$roles" | jq -e 'type == "object"' >/dev/null 2>&1 || roles='{}'
      merged=$(printf '%s' "$roles" | jq -c --arg m "$TK_ADAPTER_MODEL" '. + {"default": $m}' 2>/dev/null) ||
        die "omp's modelRoles could not be merged, and it was left as it was"
      as_account "$OMP_BIN" config set modelRoles "$merged" >/dev/null 2>&1 ||
        die "omp refused the model $TK_ADAPTER_MODEL. Read what it has with: $tool base exec -c 'omp config get modelRoles'"
    fi
  fi
  # ⛔ READ BACK FROM omp ITSELF, not from the file this wrote. A setting written and
  # never read back is a claim; omp normalises and validates what it stores, so its own
  # answer is the only proof the value is in force.
  back=$(as_account "$OMP_BIN" config get defaultThinkingLevel 2>/dev/null | tr -d '\r' | head -1)
  say "omp starts on ${TK_ADAPTER_MODEL:-its own model} at effort ${back:-unread}"
fi

# -- herdr's own OMP integration --------------------------------------------------------
if command -v herdr >/dev/null 2>&1 || [ -x /usr/local/bin/herdr ]; then
  herdr_bin=$(command -v herdr 2>/dev/null || printf '/usr/local/bin/herdr')
  # ⛔ AS THE ACCOUNT. PI_CONFIG_DIR is the account's to set, and it can name a directory
  # outside the home, which root made and gave away. WSL-201.
  account_dirs 0755 "$(dirname "$OMP_AGENT_DIR")" "$OMP_AGENT_DIR" ||
    die "the account could not make $OMP_AGENT_DIR, where herdr installs its omp integration"
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
    # linksafe: set -- builds the account's own command line, and as_account runs it
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
# ⛔ omp IS INSTALLED INTO $HOME/.local/bin AND A herdr PANE DOES NOT HAVE IT ON PATH.
# Measured 2026-09-17 on the operator's base: a login shell's PATH is
# /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:... with no ~/.local/bin, and
# /etc/profile appends /usr/local/bin and nothing else. So `herdr agent start pi`
# ran `pi` in the pane, the shell answered `command not found`, and the start timed
# out waiting for an agent that was never going to appear.
#
# ⭐ THE FIX IS THE ONE muse ALREADY USES: a root-owned wrapper on the system PATH.
# It is what makes an agent reachable by the NAME herdr launches it by, from a pane,
# from SSH, and from `base agent`, without changing the account's environment.
install -d -m 0755 "$(dirname "$WRAPPER")"
cat > "$WRAPPER" <<WRAPPER
#!/bin/sh
# Written by wsl-toolkit's omp adapter. base ensure rewrites it.
if [ "\$(id -un)" != "$TK_USER" ]; then
  printf 'omp is installed for $TK_USER, and runs only as $TK_USER\n' >&2
  exit 126
fi
exec "$TK_HOME/.local/bin/omp" "\$@"
WRAPPER
chmod 0755 "$WRAPPER"
say "wrote $WRAPPER, so a herdr pane can start it by name"

# ⛔ AND IT IS READ BACK ON THE PATH A PANE ACTUALLY HAS, not on this script's own.
# A wrapper that a login shell never reaches is the same as no wrapper, and that is
# exactly the failure this section exists to remove.
resolved_on_pane_path=$(runuser -l "$TK_USER" -c "command -v omp" 2>/dev/null | tr -d '\r' | head -1)
[ -n "$resolved_on_pane_path" ] ||
  die "omp is installed and a login shell still cannot find it, so herdr could not start it in a pane"
say "a login shell resolves omp to $resolved_on_pane_path"

say "omp is ready: $tool base agent omp"

printf 'adapter-complete omp\n'
