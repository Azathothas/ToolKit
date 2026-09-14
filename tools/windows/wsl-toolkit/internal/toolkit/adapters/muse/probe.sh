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
