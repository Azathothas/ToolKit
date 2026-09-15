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
