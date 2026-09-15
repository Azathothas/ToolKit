#!/bin/sh
# muse adapter, install: Muse Code for the base's account, from Meta's own installer,
# run only while that file's SHA-256 is one the operator approved after reading it.
#
# THE DEFECT IT EXISTS TO REMOVE: a provider CLI installed by hand, from a URL whose
# content can change under the same name, in an order written down once. This saves
# the installer, prints its length and digest, and runs the saved file only while
# the digest is approved; any other stops before it runs and says how to approve it.
#
# It runs as root inside a wsl-toolkit base during `base ensure`, delivered on stdin,
# and it looks before it changes anything, so a second run changes nothing. TK_USER,
# TK_DISTRO and, when the operator approved another installer, TK_INSTALLER_SHA256
# arrive as exported variables rather than as text substituted into this file.
set -eu
umask 022
cd /

: "${TK_USER:?TK_USER is required}"
: "${TK_DISTRO:?TK_DISTRO is required}"

say() { printf '  * %s\n' "$*"; }
die() { printf 'muse adapter: %s\n' "$*" >&2; exit 3; }

# -- what this adapter pins ----------------------------------------------------------
# ⛔ THE INSTALLER RUNS ONLY WHILE ITS DIGEST IS APPROVED. The operator approved this
# digest on 2026-09-14, for a file of 314 lines and 9,314 bytes that installs Muse for
# one account with no root. It fetches Muse's launcher and checks that file only when
# the server sends a digest, and the launcher checks the binary against a manifest from
# the same host, so both checks prove transport and not authorship. WSL-77.
MUSE_INSTALLER_URL=https://dev.meta.ai/install.sh
MUSE_INSTALLER_PINNED_SHA256=5196d820127a241211c96cf38f0b2e30cff8506a82e9da9508cd5f826632a0ca

# The two paths this writes outside the account's home. A case in the suite points
# both into a temporary directory, and nothing else sets them.
STAGE_DIR=${TK_MUSE_STAGE_DIR:-/var/lib/wsl-toolkit/muse}
WRAPPER=${TK_MUSE_WRAPPER:-/usr/local/bin/muse}

# ⛔ MEASURED ON THE ARCH PRESET ONLY, AND ANYTHING ELSE IS REFUSED.
command -v pacman >/dev/null 2>&1 ||
  die "this adapter is measured on the arch preset, and this base has no pacman. Build the base from the arch preset"

TK_HOME=$(getent passwd "$TK_USER" | cut -d: -f6)
if [ -z "$TK_HOME" ] || [ ! -d "$TK_HOME" ]; then
  die "the account $TK_USER has no home directory"
fi
LAUNCHER=$TK_HOME/.local/bin/muse

case $TK_DISTRO in
  wsl-toolkit-*) tool="wsl-toolkit --instance ${TK_DISTRO#wsl-toolkit-}" ;;
  *) tool=wsl-toolkit ;;
esac

# as_account runs a command as the account, with the environment a fresh shell of its
# own would have and nothing of this script's.
as_account() {
  runuser -u "$TK_USER" -- env -i HOME="$TK_HOME" USER="$TK_USER" LOGNAME="$TK_USER" \
    SHELL=/bin/bash LANG=C.UTF-8 PATH=/usr/local/sbin:/usr/local/bin:/usr/bin:/bin "$@"
}

# muse_version answers what the launcher says it is, and "" when it says nothing.
# ⚠ MUSE_NO_AUTO_UPDATE, so asking downloads nothing.
muse_version() {
  [ -x "$LAUNCHER" ] || return 0
  as_account MUSE_NO_AUTO_UPDATE=1 "$LAUNCHER" --version 2>/dev/null | sed -n 's/^Muse Code //p' || :
}

# -- the commands the installer and its launcher call ---------------------------------
missing=
for want in bash curl sha256sum mktemp uname wc date runuser cmp; do
  command -v "$want" >/dev/null 2>&1 || missing="$missing $want"
done
if [ -n "$missing" ]; then
  say "installing what Meta's installer and its launcher call:$missing"
  pacman -S --noconfirm --needed bash curl coreutils util-linux diffutils >/dev/null
fi
for want in bash curl sha256sum mktemp uname wc date runuser cmp; do
  command -v "$want" >/dev/null 2>&1 || die "$want is still absent after the package step"
done

# -- Muse ------------------------------------------------------------------------------
installed=$(muse_version)
if [ -n "$installed" ]; then
  say "Muse Code $installed is installed for $TK_USER, so the installer is not fetched"
else
  install -d -m 0755 "$STAGE_DIR"
  saved=$STAGE_DIR/install.sh
  fetched=$(mktemp "$STAGE_DIR/install.sh.XXXXXX")
  trap 'rm -f "$fetched"' 0 1 2 15
  curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location --max-redirs 3 \
    "$MUSE_INSTALLER_URL" --output "$fetched" || die "downloading $MUSE_INSTALLER_URL failed"
  chmod 0644 "$fetched"
  mv -f "$fetched" "$saved"
  trap - 0 1 2 15
  length=$(wc -c < "$saved" | tr -d ' ')
  digest=$(sha256sum "$saved" | cut -d' ' -f1)
  say "saved Meta's installer at $saved: $length bytes, SHA-256 $digest"

  approved=
  if [ "$digest" = "$MUSE_INSTALLER_PINNED_SHA256" ]; then
    approved="the digest this adapter pins"
  elif [ -n "${TK_INSTALLER_SHA256:-}" ] && [ "$digest" = "$TK_INSTALLER_SHA256" ]; then
    approved="the installer_sha256 in this base's configuration"
  fi
  # ⛔ A FILE NOBODY APPROVED IS KEPT TO BE READ AND NEVER RUN.
  if [ -z "$approved" ]; then
    printf 'muse adapter: the installer Meta serves now is not one the operator approved, so it was not run.\n' >&2
    printf '  saved at    %s, %s bytes, SHA-256 %s\n' "$saved" "$length" "$digest" >&2
    printf '  read it     %s base exec --root -c %s\n' "$tool" "'cat $saved'" >&2
    printf '  approve it  add "installer_sha256": "%s" to the muse entry in base.adapters, then run base ensure again\n' "$digest" >&2
    exit 2
  fi
  say "running it as $TK_USER, approved by $approved"
  # ⚠ MUSE_NO_MODIFY_PATH: the account's profiles stay as they are, and the wrapper
  # below puts muse on PATH for every shell instead.
  as_account MUSE_NO_MODIFY_PATH=1 bash "$saved" || die "Meta's installer exited non-zero"
  installed=$(muse_version)
  [ -n "$installed" ] || die "the installer finished, and $LAUNCHER --version answers with no version"
  say "installed Muse Code $installed for $TK_USER"
fi

# -- muse by name ----------------------------------------------------------------------
# ⭐ `base exec` starts a shell that reads no profile, so a launcher in ~/.local/bin is
# not found by name there. This file puts it on PATH for every shell. ⛔ It runs Muse
# only as the account it is installed for: the launcher updates itself in that home,
# and another account would write there as itself.
wrapper_tmp=$(mktemp)
cat > "$wrapper_tmp" <<WRAPPER
#!/bin/sh
# Written by wsl-toolkit's muse adapter. base ensure rewrites it.
if [ "\$(id -un)" != "$TK_USER" ]; then
  printf 'muse is installed for $TK_USER, and runs only as $TK_USER\n' >&2
  exit 126
fi
exec "$LAUNCHER" "\$@"
WRAPPER
if cmp -s "$wrapper_tmp" "$WRAPPER"; then
  rm -f "$wrapper_tmp"
else
  install -d -m 0755 "$(dirname "$WRAPPER")"
  install -m 0755 "$wrapper_tmp" "$WRAPPER"
  rm -f "$wrapper_tmp"
  say "wrote $WRAPPER, which runs Muse as $TK_USER"
fi

# -- the herdr reporter --------------------------------------------------------------
# ⭐ herdr CLASSIFIES A MUSE PANE BY READING ITS SCREEN, and this gives it the events
# instead. herdr ships a Muse detection manifest but no Muse integration: its own was
# proposed as herdrdev/herdr#4163 to #4166 and every part was closed WITHOUT merging on
# 2026-09-15, so a third-party hook is the only route and this is ours.
#
# ⛔ NOTHING IS INSTALLED WHEN THE PIECES ARE NOT THERE, and each refusal says which.
# A hook registered against a missing herdr is a hook that runs on every tool call of
# every turn and does nothing.
HOOK_DIR=$TK_HOME/.local/share/wsl-toolkit
HOOK=$HOOK_DIR/herdr-agent-state.sh
HOOK_MARK=/wsl-toolkit/herdr-agent-state.sh
MUSE_SETTINGS=$TK_HOME/.muse/settings.json
HOOK_EVENTS='SessionStart UserPromptSubmit PreToolUse PermissionRequest Stop SessionEnd'

herdr_reporter_skip=
if [ -z "${TK_FILE_HERDR_AGENT_STATE_SH_B64:-}" ]; then
  herdr_reporter_skip='this adapter carries no reporter file'
elif ! command -v herdr >/dev/null 2>&1 && [ ! -x /usr/local/bin/herdr ]; then
  herdr_reporter_skip='no herdr is installed in this base, so there is nothing to report to. Add the herdr adapter to base.adapters'
elif ! command -v jq >/dev/null 2>&1; then
  herdr_reporter_skip='jq is not installed, and the reporter reads its payload with it. Set base.toolset to developer or later'
fi

if [ -n "$herdr_reporter_skip" ]; then
  say "no herdr reporter: $herdr_reporter_skip"
else
  # ⛔ RESOLVED HERE, NOT AT THE TOP OF THE FILE. An earlier draft set this beside
  # TK_HOME, and under `set -e` an `id` for an account that does not exist then
  # ended the script before ANY path below it - including the digest refusal, which
  # a case drives with no such account. A variable a feature needs belongs inside
  # the branch that has the feature.
  TK_GROUP=$(id -gn "$TK_USER")
  install -d -o "$TK_USER" -g "$TK_GROUP" -m 0755 "$HOOK_DIR"
  hook_tmp=$(mktemp)
  hook_encoded=$(mktemp)
  printf '%s' "$TK_FILE_HERDR_AGENT_STATE_SH_B64" > "$hook_encoded"
  base64 -d < "$hook_encoded" > "$hook_tmp" || die "the reporter did not decode"
  rm -f "$hook_encoded"
  # ⭐ ITS OWN SELF-TEST IS THE GATE ON INSTALLING IT. A reporter that cannot pass
  # its own cases is not registered, because a broken hook runs on every event.
  if ! sh "$hook_tmp" --selftest >/dev/null 2>&1; then
    rm -f "$hook_tmp"
    die "the herdr reporter failed its own self-test in this base, so it was not installed"
  fi
  if cmp -s "$hook_tmp" "$HOOK"; then
    rm -f "$hook_tmp"
  else
    install -o "$TK_USER" -g "$TK_GROUP" -m 0755 "$hook_tmp" "$HOOK"
    rm -f "$hook_tmp"
    say "wrote $HOOK, and its self-test passed"
  fi

  # ⛔ MUSE'S settings.json IS MERGED, NEVER REPLACED. It is the operator's file and
  # carries their own settings; herdr's own unmerged plumbing did the same. jq does
  # the merge, so a value this adapter does not name is preserved exactly.
  install -d -o "$TK_USER" -g "$TK_GROUP" -m 0700 "$TK_HOME/.muse"
  [ -f "$MUSE_SETTINGS" ] || as_account sh -c "printf '{}\n' > '$MUSE_SETTINGS'"
  if ! as_account jq -e . "$MUSE_SETTINGS" >/dev/null 2>&1; then
    die "$MUSE_SETTINGS is not valid JSON, so it was left untouched. Read it, then run base ensure again"
  fi
  settings_tmp=$(mktemp)
  chown "$TK_USER" "$settings_tmp"
  # ⚠ ONE ENTRY PER EVENT, and every OLD one of ours is dropped first. ⛔ Keying on
  # the current path alone was measured leaving a dead entry behind when the path
  # moved, and a hook that no longer exists still runs on every event of every turn.
  # The marker is the trailing path this adapter always writes, so our entry is
  # recognised wherever the account's home is, and nobody else's is touched.
  # shellcheck disable=SC2016  # the single quotes hold a jq program, not shell
  if as_account jq --arg hook "$HOOK" --arg mark "$HOOK_MARK" --arg events "$HOOK_EVENTS" '
      ($events | split(" ")) as $names
      | .hooks = ((.hooks // {}) | reduce $names[] as $n (.;
          .[$n] = (((.[$n] // []) | map(select(((.command // "") | endswith($mark)) | not))) + [{"type":"command","command":$hook}])))
    ' "$MUSE_SETTINGS" > "$settings_tmp" 2>/dev/null && [ -s "$settings_tmp" ]; then
    if cmp -s "$settings_tmp" "$MUSE_SETTINGS"; then
      rm -f "$settings_tmp"
      say "Muse already reports to herdr on $(printf '%s' "$HOOK_EVENTS" | wc -w) events"
    else
      install -o "$TK_USER" -g "$TK_GROUP" -m 0600 "$settings_tmp" "$MUSE_SETTINGS"
      rm -f "$settings_tmp"
      say "registered the herdr reporter in $MUSE_SETTINGS for: $HOOK_EVENTS"
    fi
  else
    rm -f "$settings_tmp"
    die "merging the reporter into $MUSE_SETTINGS failed, and that file was left as it was"
  fi
fi

# ⛔ SIGNING IN IS THE OPERATOR'S, and nothing here reads or writes the credential.
say "signing in is the operator's: $tool base shell, then muse login"

printf 'adapter-complete muse\n'
