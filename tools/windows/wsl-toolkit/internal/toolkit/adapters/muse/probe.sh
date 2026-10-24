#!/bin/sh
# muse adapter, probe: what install.sh put in place, read back from the machine.
#
# THE DEFECT IT EXISTS TO CATCH: Muse reported installed because an installer exited
# 0. A launcher that no longer answers, a wrapper edited by hand and a name that
# resolves to something else are invisible to an exit code and visible here.
#
# It runs as root inside the base, delivered on stdin. It prints one fact per line
# as `name value`, and `problem TEXT` for each thing that is wrong, and exits 0
# whatever it finds: the lines are the answer.
set -u
: "${TK_USER:?TK_USER is required}"

problem() { printf 'problem %s\n' "$*"; }

TK_HOME=$(getent passwd "$TK_USER" | cut -d: -f6)
LAUNCHER=$TK_HOME/.local/bin/muse
WRAPPER=/usr/local/bin/muse

as_account() {
  runuser -u "$TK_USER" -- env -i HOME="$TK_HOME" USER="$TK_USER" LOGNAME="$TK_USER" \
    SHELL=/bin/bash LANG=C.UTF-8 PATH=/usr/local/sbin:/usr/local/bin:/usr/bin:/bin "$@"
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

# ⛔ ASKED FIRST: IS MUSE IN THIS BASE AT ALL. Without it, a Muse that never
# installed, because an adapter before it stopped `base ensure`, read as
# "installed and a login shell cannot find it". WSL-107, issue 37.
if [ -e "$WRAPPER" ] || [ -e "$LAUNCHER" ]; then installed=yes; else installed=no; fi
printf 'installed %s\n' "$installed"
if [ "${TK_PROBE_FOOTPRINT:-}" = 1 ]; then
  # ⚠ Meta's launcher may be a link into the versions it keeps, so the directory it
  # resolves into is counted when that is under the account's own share.
  versions=''
  if real=$(readlink -f "$LAUNCHER" 2>/dev/null); then
    case $real in
      "$TK_HOME"/.local/share/*) versions=$(dirname "$real") ;;
    esac
  fi
  printf 'footprint %s\n' "$(footprint "$WRAPPER" "$LAUNCHER" "$versions" "$TK_HOME/.config/muse" /var/lib/wsl-toolkit/muse)"
  exit 0
fi
if [ "$installed" = no ]; then
  problem "Muse is not installed in this base. Run: base ensure"
  exit 0
fi

if [ -x "$LAUNCHER" ]; then
  printf 'launcher %s\n' "$LAUNCHER"
  # ⚠ MUSE_NO_AUTO_UPDATE, so reading the version downloads nothing.
  version=$(as_account MUSE_NO_AUTO_UPDATE=1 "$LAUNCHER" --version 2>/dev/null | sed -n 's/^Muse Code //p')
  if [ -n "$version" ]; then
    printf 'version %s\n' "$version"
  else
    problem "$LAUNCHER --version answers with no version"
  fi
else
  problem "Muse is not installed for $TK_USER at $LAUNCHER"
fi

if [ -x "$WRAPPER" ] && grep -qxF "exec \"$LAUNCHER\" \"\$@\"" "$WRAPPER"; then
  printf 'wrapper %s\n' "$WRAPPER"
else
  problem "$WRAPPER does not run $LAUNCHER as this tool writes it"
fi
# ⚠ THE NAME AS base exec's SHELL FINDS IT, which is the lookup an agent's command makes.
resolved=$(as_account sh -c 'command -v muse' 2>/dev/null || :)
printf 'resolves %s\n' "${resolved:-nothing}"
[ "$resolved" = "$WRAPPER" ] || problem "muse resolves to ${resolved:-nothing} for $TK_USER, not to $WRAPPER"

# ⛔ AND THE SAME QUESTION ON A LOGIN SHELL'S PATH, WHICH IS A DIFFERENT PATH. The
# check above reads base exec's curated environment; a herdr pane and `base shell` read
# a LOGIN shell's, and those two disagreed once an earlier `base bootstrap` ran.
# That `bootstrap.sh` wrote `export PATH="$HOME/.local/bin:$PATH"` into the
# account's profile, so the vendor's own launcher was found FIRST and this wrapper
# was never reached. It now puts its prefix last and rewrites that line. WSL-147. ⛔ Muse Code has no settings key for a model or a reasoning effort, so what
# is lost is BOTH of them, on every session, with nothing said.
#
# ⚠ Finding 49's shape, a second time: a guard proved on a curated PATH is a guard
# proved on a path nobody uses.
pane_path=$(runuser -l "$TK_USER" -c "command -v muse" 2>/dev/null | tr -d '\r' | head -1)
if [ -z "$pane_path" ]; then
  problem "Muse is installed and a login shell cannot find it, so herdr cannot start it in a pane. Run: base ensure"
else
  printf 'on_login_path %s\n' "$pane_path"
  if head -3 "$pane_path" 2>/dev/null | grep -q "wsl-toolkit's muse adapter"; then
    printf 'wrapper_on_login_path yes\n'
  else
    printf 'wrapper_on_login_path no\n'
    # ⭐ THE PROBLEM NAMES ITS REMEDY. bootstrap.sh now puts its prefix after the
    # system path and rewrites the line an earlier run wrote. WSL-147.
    problem "a login shell resolves muse to $pane_path, which is not this tool's wrapper, so the configured model and reasoning effort reach no session started from a pane. The account's own prefix is ahead of /usr/local/bin, which an earlier bootstrap.sh put there. Run: base bootstrap, which moves its own line after the system path. A line the account wrote itself is moved by hand"
  fi
fi

# ⛔ THE SANDBOX MUSE REFUSES TO RUN WITHOUT. Without bwrap, Muse installs, signs in,
# answers questions, and refuses EVERY command with "the execution environment is
# broken". A probe that reported only a version called that base healthy. Measured
# 2026-09-17.
if command -v bwrap >/dev/null 2>&1; then
  printf 'sandbox bwrap %s\n' "$(bwrap --version 2>/dev/null | awk '{print $2}')"
else
  problem "Muse enforces a sandbox and this base has no bwrap, so it will refuse every command it is asked to run"
fi

# Whether the operator has signed in: the credential file's presence, never its content.

if [ -f "$TK_HOME/.config/muse/auth.json" ]; then
  printf 'credential present\n'
else
  printf 'credential absent\n'
fi

# -- the herdr reporter, read back ------------------------------------------------
# ⭐ install.sh WRITES IT, SO THE PROBE READS IT. A gap here is how "the integration
# is installed" becomes something an operator believes rather than something the
# machine says. ⚠ Its absence is a FACT and not a problem: muse runs without it, and
# a base with no herdr is a perfectly good base for muse.
HOOK=$TK_HOME/.local/share/wsl-toolkit/herdr-agent-state.sh
# The file Muse reads, as install.sh names it and for the same measured reason.
MUSE_SETTINGS=$TK_HOME/.config/muse/settings.json
if [ -x "$HOOK" ]; then
  printf 'herdr_reporter installed\n'
  # ⛔ AND IT STILL PASSES ITS OWN CASES. A file that is present and broken reports
  # the same as one that works, until something asks it.
  if as_account sh "$HOOK" --selftest >/dev/null 2>&1; then
    printf 'herdr_reporter_selftest pass\n'
  else
    problem "the herdr reporter at $HOOK fails its own self-test"
  fi
else
  printf 'herdr_reporter absent\n'
fi
if [ -f "$MUSE_SETTINGS" ] && command -v jq >/dev/null 2>&1; then
  # ⛔ A SETTINGS FILE MUSE WILL NOT READ STOPS MUSE, not only the reporter: it refuses
  # to start over one that is not an object carrying schema_version.
  if ! as_account jq -e 'type == "object" and has("schema_version")' "$MUSE_SETTINGS" >/dev/null 2>&1; then
    problem "$MUSE_SETTINGS is not a JSON object carrying schema_version, and Muse refuses to start over it"
  fi
  # shellcheck disable=SC2016  # the single quotes hold a jq program, not shell
  registered=$(as_account jq -r --arg h "$HOOK" \
    '[(.hooks // {}) | to_entries[] | select(any(.value[]?.hooks[]?; (.command // "") == $h)) | .key] | sort | join(",")' \
    "$MUSE_SETTINGS" 2>/dev/null)
  printf 'herdr_reporter_events %s\n' "${registered:-none}"
fi
