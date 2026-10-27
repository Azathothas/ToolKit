#!/bin/sh
# pi adapter, install: the Pi coding agent for the base's account, from pi's own
# release while its digest is the one pinned here, from npm only when that release
# cannot be fetched, and herdr's own Pi integration so herdr has lifecycle authority
# over its panes.
#
# THE DEFECT IT EXISTS TO REMOVE: an agent herdr can only classify by reading its
# screen. Pi publishes lifecycle hooks and herdr ships an integration for them, so
# `idle`, `working` and `blocked` come from the agent itself rather than from a
# guess about what its terminal looks like.
#
# ⭐ NOTHING IS PIPED INTO A SHELL. The release asset is saved, its digest is read,
# and only the saved file is unpacked. The fallback is `npm install -g
# --ignore-scripts`, which pi documents and which runs no lifecycle script. WSL-88,
# WSL-138.
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
# same_file A B is true when B exists and holds A's bytes. ⚠ NOT cmp: cmp is
# diffutils, which a base with no toolset does not carry, and a missing cmp read
# as "the files differ" on every run. WSL-105.
same_file() {
  [ -f "$2" ] || return 1
  [ "$(sha256sum < "$1")" = "$(sha256sum < "$2")" ]
}

# -- what this adapter installs, and from where ------------------------------------
# ⭐ PI'S OWN RELEASE FIRST, AND npm ONLY WHEN THAT RELEASE CANNOT BE FETCHED. Ruling
# 20, as ruling 36 amended it: an adapter prefers the provider's official installer or
# release asset. Every pi release on GitHub carries a standalone build per platform,
# beside a SHA256SUMS that covers it, and pi's own README calls those the official
# standalone binaries. WSL-138.
#
# ⛔ THE ASSET IS UNPACKED ONLY WHILE ITS DIGEST IS THE ONE PINNED HERE, the route ruling
# 4 set for Muse: download, save, print the length and SHA-256, and unpack the saved
# file. Each digest is the line pi v1.4.0's own SHA256SUMS carries. The x86_64 one was
# also measured on download: 42,645,744 bytes. ⛔ Any other digest stops the ensure with
# exit 2, keeps the file to be read, and says how to approve it. It never takes npm.
#
# ⚠ NOT pi.dev/install.sh, BECAUSE OF WHAT A DIGEST ON IT WOULD PIN. That installer asks
# pi.dev's API for the newest release at run time and installs it with npm, so a digest
# on the script pins nothing it installs. It also stops at a menu when it has a terminal.
PI_RELEASE=1.4.0
PI_X86_64_PINNED_SHA256=f2e4e4329603223d03260cdb8bd5033dbdc1f73ba58ab8939db2e56546e115d2
PI_AARCH64_PINNED_SHA256=fa5829c7ef83ffeaebba015715c8306220f89b2aa8a00154efbba98dd5440156
PI_URL_BASE=https://github.com/earendil-works/pi/releases/download

# ⭐ A NEWER RELEASE IS A CONFIGURATION CHANGE, NOT AN EDIT TO THIS FILE, as herdr's is.
# `version` and `sha256` on this adapter's entry arrive as TK_ADAPTER_VERSION and
# TK_ADAPTER_SHA256_<ARCH>, and the executable refuses one without the other. ⭐ The npm
# fallback installs the same version, so the two routes never disagree on which pi.
if [ -n "${TK_ADAPTER_VERSION:-}" ]; then
  PI_RELEASE=$TK_ADAPTER_VERSION
  PI_X86_64_PINNED_SHA256=${TK_ADAPTER_SHA256_X86_64:-}
  PI_AARCH64_PINNED_SHA256=${TK_ADAPTER_SHA256_AARCH64:-}
fi
# ⚠ THE FALLBACK'S PACKAGE. npm checks it against the registry's own integrity value.
PI_PACKAGE=${TK_ADAPTER_PACKAGE:-@earendil-works/pi-coding-agent}
PI_BIN=pi

# The two paths this writes outside the account's home. A case in the suite points both
# into a temporary directory, and nothing else sets them.
STAGE_DIR=${TK_PI_STAGE_DIR:-/var/lib/wsl-toolkit/pi}
WRAPPER=${TK_PI_WRAPPER:-/usr/local/bin/pi}

TK_HOME=$(getent passwd "$TK_USER" | cut -d: -f6)
if [ -z "$TK_HOME" ] || [ ! -d "$TK_HOME" ]; then
  die "the account $TK_USER has no home directory"
fi
TK_GROUP=$(id -gn "$TK_USER")
PREFIX=$TK_HOME/.local
# ⭐ EACH RELEASE IS UNPACKED HERE, BY THE ACCOUNT, and $PREFIX/bin/pi links to the one in
# use. The build reads its themes and native helpers beside its own real path, so it is
# kept whole rather than copied out of its directory.
RELEASES=$TK_HOME/.local/share/wsl-toolkit/pi

case $TK_DISTRO in
  # The default base: the canonical name and the older one. WSL-167.
  wsl-toolkit | wsl-toolkit-jobs) tool=wsl-toolkit ;;
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

# -- pi --------------------------------------------------------------------------------
pi_version() {
  as_account "$PI_BIN" --version 2>/dev/null | tr -d '\r' | head -1
}

case "$(uname -m)" in
  x86_64) PI_ASSET=pi-linux-x64.tar.gz; PI_WANT=$PI_X86_64_PINNED_SHA256 ;;
  aarch64) PI_ASSET=pi-linux-arm64.tar.gz; PI_WANT=$PI_AARCH64_PINNED_SHA256 ;;
  *) PI_ASSET=''; PI_WANT='' ;;
esac
# ⛔ AN EMPTY DIGEST IS A REFUSAL, NOT AN UNCHECKED DOWNLOAD AND NOT A FALLBACK. It means
# a configuration moved the version and gave no digest for THIS architecture.
if [ -n "$PI_ASSET" ] && [ -z "$PI_WANT" ]; then
  die "base.adapters moves pi to $PI_RELEASE and names no sha256 for $(uname -m). Add it, or remove the version to use the one this adapter pins"
fi
RELEASE_DIR=$RELEASES/$PI_RELEASE
RELEASE_BIN=$RELEASE_DIR/pi/pi

# from_release is true when the account's pi is this release, unpacked from the asset
# whose digest is the one in force. ⚠ Read as the account, because the files are its own.
from_release() {
  if [ -z "$PI_ASSET" ] || [ ! -x "$RELEASE_BIN" ]; then
    return 1
  fi
  linked=$(as_account readlink -f "$PREFIX/bin/pi" 2>/dev/null) || return 1
  [ "$linked" = "$(as_account readlink -f "$RELEASE_BIN" 2>/dev/null)" ] || return 1
  [ "$(as_account cat "$RELEASE_DIR/asset.sha256" 2>/dev/null)" = "$PI_WANT" ]
}

# prune_releases removes every release this adapter unpacked but the one in use, and any
# unpacking a stopped run left. ⚠ As the account, so a link it planted reaches nothing.
prune_releases() {
  for old in "$RELEASES"/* "$RELEASES"/.unpack.*; do
    if [ ! -e "$old" ] && [ ! -L "$old" ]; then
      continue
    fi
    if [ "$old" = "$RELEASE_DIR" ]; then
      continue
    fi
    if as_account rm -rf "$old"; then
      say "removed $old, which the pi in use does not read"
    else
      say "could not remove $old, which the pi in use does not read"
    fi
  done
  return 0
}

# release_install fetches pi's release asset, checks it and unpacks it. It answers 1 when
# the asset cannot be fetched, which is the one case npm is taken for.
release_install() {
  missing=
  for want in curl sha256sum tar gzip; do
    command -v "$want" >/dev/null 2>&1 || missing="$missing $want"
  done
  if [ -n "$missing" ] && command -v pacman >/dev/null 2>&1; then
    say "installing what the release download needs:$missing"
    # ⛔ -Syu, NOT -S, for the reason herdr's installer gives. WSL-188.
    pacman -Syu --noconfirm --needed curl coreutils tar gzip >/dev/null 2>&1 || :
  fi
  for want in curl sha256sum tar gzip; do
    if ! command -v "$want" >/dev/null 2>&1; then
      say "this base has no $want, so pi's release asset cannot be fetched and unpacked"
      return 1
    fi
  done
  # ⛔ set -e DOES NOT REACH IN HERE, because this runs as a condition, so every step
  # that can fail says so itself.
  install -d -m 0755 "$STAGE_DIR" || die "no directory $STAGE_DIR to save pi's release asset in"
  saved=$STAGE_DIR/$PI_ASSET
  url=$PI_URL_BASE/v$PI_RELEASE/$PI_ASSET
  fetched=$(mktemp "$STAGE_DIR/$PI_ASSET.XXXXXX") || die "no temporary file in $STAGE_DIR"
  trap 'rm -f "$fetched"' 0 1 2 15
  if ! fetch_error=$(curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location --max-redirs 5 \
      "$url" --output "$fetched" 2>&1); then
    rm -f "$fetched"
    trap - 0 1 2 15
    say "pi's release asset could not be fetched from $url: $(printf '%s' "$fetch_error" | tail -1)"
    return 1
  fi
  if ! chmod 0644 "$fetched" || ! mv -f "$fetched" "$saved"; then
    die "pi's release asset could not be saved at $saved"
  fi
  trap - 0 1 2 15
  length=$(wc -c < "$saved" | tr -d ' ')
  digest=$(sha256sum "$saved" | cut -d' ' -f1)
  say "saved pi's release asset at $saved: $length bytes, SHA-256 $digest"
  # ⛔ A FILE WHOSE DIGEST IS NOT THE PINNED ONE IS KEPT TO BE READ AND NEVER UNPACKED,
  # and npm is NOT taken in its place: a release that changed under its own name is a
  # question for the operator, not a download failure.
  if [ "$digest" != "$PI_WANT" ]; then
    printf "pi adapter: pi's release asset is not the one this adapter pins, so it was not unpacked and npm was not tried.\n" >&2
    printf '  saved at    %s, %s bytes, SHA-256 %s\n' "$saved" "$length" "$digest" >&2
    printf '  pinned      pi %s, SHA-256 %s\n' "$PI_RELEASE" "$PI_WANT" >&2
    printf "  compare it  with the release's own list, %s\n" "$PI_URL_BASE/v$PI_RELEASE/SHA256SUMS" >&2
    printf '  approve it  add "version": "%s", "sha256": {"%s": "%s"} to the pi entry in base.adapters, then run base ensure again\n' \
      "$PI_RELEASE" "$(uname -m)" "$digest" >&2
    exit 2
  fi
  # ⭐ UNPACKED BY THE ACCOUNT, into a directory of its own, from the saved file alone.
  as_account mkdir -p "$RELEASES" "$PREFIX/bin" || die "the account could not make $RELEASES"
  unpack=$(as_account mktemp -d "$RELEASES/.unpack.XXXXXX") || die "the account could not make a directory under $RELEASES"
  if ! as_account tar -xzf "$saved" -C "$unpack"; then
    as_account rm -rf "$unpack"
    die "pi's release asset at $saved did not unpack"
  fi
  if [ ! -x "$unpack/pi/pi" ]; then
    as_account rm -rf "$unpack"
    die "pi's release asset holds no pi/pi, which is where every release since 1.0 keeps the program"
  fi
  # shellcheck disable=SC2016  # the single quotes hold the account's own script
  printf '%s\n' "$digest" | as_account sh -c 'cat > "$1"' sh "$unpack/asset.sha256" ||
    die "the account could not record the digest its release was unpacked from"
  as_account rm -rf "$RELEASE_DIR"
  as_account mv "$unpack" "$RELEASE_DIR" || die "the account could not move pi's release into $RELEASE_DIR"
  as_account ln -sfn "$RELEASE_BIN" "$PREFIX/bin/pi" || die "the account could not link $PREFIX/bin/pi to its release"
  # ⚠ The unpacked release holds the same bytes, so the saved copy goes.
  rm -f "$saved"
  return 0
}

installed=$(pi_version || :)
if from_release; then
  say "pi $installed is installed for $TK_USER from its release, unpacked from the asset this adapter pins"
  prune_releases
elif [ -n "$PI_ASSET" ] && release_install; then
  installed=$(pi_version || :)
  [ "$installed" = "$PI_RELEASE" ] ||
    die "pi's release $PI_RELEASE is unpacked and $PI_BIN --version answers '${installed:-nothing}'"
  say "installed pi $installed for $TK_USER from its release asset, digest verified"
  prune_releases
else
  [ -n "$PI_ASSET" ] || say "pi publishes no Linux build for $(uname -m)"
  # -- the fallback: npm, and only because the release could not be fetched ------------
  command -v npm >/dev/null 2>&1 ||
    die "pi's release could not be fetched, and npm is not installed in this base for the fallback. Run base ensure again when the release can be fetched, or set base.toolset to developer or later"
  want=$PI_PACKAGE@$PI_RELEASE
  current=$(as_account readlink -f "$PREFIX/bin/pi" 2>/dev/null || :)
  case $current in
    */node_modules/*) ;;
    *) installed= ;;
  esac
  if [ -n "$installed" ] && [ "$installed" = "$PI_RELEASE" ]; then
    say "pi $installed from npm is installed for $TK_USER, at the version this adapter pins"
  else
    # ⚠ npm WILL NOT REPLACE A LAUNCHER IT DID NOT MAKE, so a release's link goes first,
    # and comes back when npm fails, so a failed fallback leaves the pi that was there.
    earlier_link=
    if [ -L "$PREFIX/bin/pi" ]; then
      earlier_link=$(as_account readlink "$PREFIX/bin/pi" 2>/dev/null || :)
      as_account rm -f "$PREFIX/bin/pi"
    fi
    say "installing $want from npm for $TK_USER, with no lifecycle scripts"
    # ⛔ --ignore-scripts. Pi's own documentation says it needs none for a normal
    # install, and a dependency's install script runs as this account with its home.
    if ! as_account npm install -g --ignore-scripts "$want" >/dev/null 2>&1; then
      [ -z "$earlier_link" ] || as_account ln -sfn "$earlier_link" "$PREFIX/bin/pi" || :
      die "npm could not install $want. Read it with: $tool base exec -c 'npm install -g --ignore-scripts $want'"
    fi
    installed=$(pi_version || :)
    [ -n "$installed" ] || die "npm installed $want and $PI_BIN --version answers nothing"
    say "installed pi $installed from npm"
  fi
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

# -- the startup model and reasoning effort --------------------------------------------
# ⛔ IT GOES IN PI'S OWN SETTINGS FILE AND NOT IN AN ENVIRONMENT. A herdr pane inherits
# the herdr SERVICE's environment, not a login shell's: measured on 2026-09-17 by reading
# /proc/PID/environ for every pane process, which is why a key exported in ~/.profile
# never reached pi and `pi auth check` answered not_ready. A value the agent itself reads
# at startup is the only kind that reaches an agent herdr starts.
#
# ⭐ PI'S OWN DOOR, from pi's own docs/settings.md in this base: ~/.pi/agent/settings.json
# takes defaultProvider, defaultModel and defaultThinkingLevel, and they are what /model
# and /thinking write when a person saves a choice there.
#
# ⚠ THE FILE IS MERGED, NEVER REPLACED. It is the account's, it carries their own
# settings, and one that cannot be parsed stops this rather than being overwritten with a
# guess - the rule the Muse trust store taught on 2026-09-17.
if [ -n "${TK_ADAPTER_MODEL:-}${TK_ADAPTER_EFFORT:-}" ]; then
  if ! command -v jq >/dev/null 2>&1; then
    say "no jq in this base, so pi's startup model and effort were not set. Set base.toolset to developer or later"
  else
    install -d -o "$TK_USER" -g "$TK_GROUP" -m 0755 "$TK_HOME/.pi" "$TK_HOME/.pi/agent"
    pi_settings=$TK_HOME/.pi/agent/settings.json
    [ -f "$pi_settings" ] || as_account sh -c "printf '{}\n' > '$pi_settings'"
    as_account jq -e 'type == "object"' "$pi_settings" >/dev/null 2>&1 ||
      die "$pi_settings is not a JSON object, and pi reads its settings from one. It was left exactly as it is; read it, then run base ensure again"
    # ⭐ A SLASH SPLITS PROVIDER FROM MODEL, because pi reaches a model through a named
    # provider and spells that pair `provider/id` on its own --model flag. A bare id
    # leaves defaultProvider alone, so a base that only moves the effort keeps its
    # provider.
    pi_provider=
    pi_model=${TK_ADAPTER_MODEL:-}
    case $pi_model in
      */*) pi_provider=${pi_model%%/*}; pi_model=${pi_model#*/} ;;
    esac
    settings_tmp=$(mktemp)
    # shellcheck disable=SC2016  # the single quotes hold a jq program, not shell
    if as_account jq \
        --arg provider "$pi_provider" --arg model "$pi_model" --arg effort "${TK_ADAPTER_EFFORT:-}" '
        (if $provider == "" then . else .defaultProvider = $provider end)
        | (if $model == "" then . else .defaultModel = $model end)
        | (if $effort == "" then . else .defaultThinkingLevel = $effort end)
      ' "$pi_settings" > "$settings_tmp" 2>/dev/null && [ -s "$settings_tmp" ]; then
      if same_file "$settings_tmp" "$pi_settings"; then
        rm -f "$settings_tmp"
      else
        install -o "$TK_USER" -g "$TK_GROUP" -m 0600 "$settings_tmp" "$pi_settings"
        rm -f "$settings_tmp"
      fi
      say "pi starts on ${TK_ADAPTER_MODEL:-its own model} at effort ${TK_ADAPTER_EFFORT:-its own}, from $pi_settings"
    else
      rm -f "$settings_tmp"
      die "pi's settings could not be updated, and $pi_settings was left as it was"
    fi
  fi
fi

# -- the wrapper that puts the agent on a PANE's PATH ------------------------------------
# ⛔ pi IS LINKED INTO $HOME/.local/bin AND A herdr PANE DOES NOT HAVE IT ON PATH.
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
# Written by wsl-toolkit's pi adapter. base ensure rewrites it.
if [ "\$(id -un)" != "$TK_USER" ]; then
  printf 'pi is installed for $TK_USER, and runs only as $TK_USER\n' >&2
  exit 126
fi
exec "$TK_HOME/.local/bin/pi" "\$@"
WRAPPER
chmod 0755 "$WRAPPER"
say "wrote $WRAPPER, so a herdr pane can start it by name"

# ⛔ AND IT IS READ BACK ON THE PATH A PANE ACTUALLY HAS, not on this script's own.
# A wrapper that a login shell never reaches is the same as no wrapper, and that is
# exactly the failure this section exists to remove.
resolved_on_pane_path=$(runuser -l "$TK_USER" -c "command -v pi" 2>/dev/null | tr -d '\r' | head -1)
[ -n "$resolved_on_pane_path" ] ||
  die "pi is installed and a login shell still cannot find it, so herdr could not start it in a pane"
say "a login shell resolves pi to $resolved_on_pane_path"

say "pi is ready: $tool base agent pi"

printf 'adapter-complete pi\n'
