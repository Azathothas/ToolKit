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

# first_lines N PATH prints the first N lines of PATH, read by the account, when it is a
# file. ⛔ Root's head followed a link the account placed there, and a named pipe there
# held the probe. WSL-201.
first_lines() {
  # shellcheck disable=SC2016  # the single quotes hold the account's own script
  account_read sh -c '[ -f "$2" ] && exec head -n "$1" "$2"' sh "$1" "$2" 2>/dev/null
}
# <<< the account's reads: end

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
  # ⚠ AND herdr's RULES FOR MUSE, when they are this adapter's. A file of the
  # operator's own at that path is not this adapter's to count. WSL-158.
  rules=''
  if first_lines 1 "$TK_HOME/.config/herdr/agent-detection/muse.toml" | grep -q "from wsl-toolkit's muse adapter"; then
    rules=$TK_HOME/.config/herdr/agent-detection/muse.toml
  fi
  # linksafe: footprint tests each path and reads its size with du, which follows no link
  printf 'footprint %s\n' "$(footprint "$WRAPPER" "$LAUNCHER" "$versions" "$TK_HOME/.config/muse" /var/lib/wsl-toolkit/muse "$rules")"
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

# linksafe: the launcher's path is the pattern, and grep reads the wrapper outside the home
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
  if first_lines 3 "$pane_path" | grep -q "wsl-toolkit's muse adapter"; then
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
  if ! account_read jq -e 'type == "object" and has("schema_version")' "$MUSE_SETTINGS" >/dev/null 2>&1; then
    problem "$MUSE_SETTINGS is not a JSON object carrying schema_version, and Muse refuses to start over it"
  fi
  # shellcheck disable=SC2016  # the single quotes hold a jq program, not shell
  registered=$(account_read jq -r --arg h "$HOOK" \
    '[(.hooks // {}) | to_entries[] | select(any(.value[]?.hooks[]?; (.command // "") == $h)) | .key] | sort | join(",")' \
    "$MUSE_SETTINGS" 2>/dev/null)
  printf 'herdr_reporter_events %s\n' "${registered:-none}"
fi

# -- herdr's detection rules for Muse, read back -------------------------------------
# ⭐ WHETHER THE FILE IS THERE, AND WHETHER herdr LOADS IT. install.sh writes the file.
# A herdr whose manifest format moved ignores a file it cannot read, with a warning and
# its own rules instead. ⚠ Facts and a note, not problems: the reporter still reports,
# and `base ensure` writes the same file again and cannot cure it. WSL-158.
#
# ⚠ AND WHETHER IT SHADOWS A NEWER MANIFEST OF herdr's. A local file wins whenever herdr
# holds a remote one, so herdr cannot deliver a fix for Muse's screen past it. A remote
# version other than the one these rules were measured against is a note naming it.
HERDR_RULES=$TK_HOME/.config/herdr/agent-detection/muse.toml
HERDR_RULES_MEASURED=2026.08.26.1
if [ -f "$HERDR_RULES" ] && first_lines 1 "$HERDR_RULES" | grep -q "from wsl-toolkit's muse adapter"; then
  printf 'herdr_detection_rules installed\n'
  if command -v jq >/dev/null 2>&1; then
    # ⚠ THE WARNING GOES LAST, because it is herdr's own text and may carry a `|`.
    loaded=$(as_account herdr server agent-manifests --json 2>/dev/null |
      account_read jq -r '.result.manifests[]? | select(.agent == "muse") | "\(.source_kind)|\(.local_override_shadowing_remote // false)|\(.cached_remote_version // "")|\(.warning // "it gave no warning")"' 2>/dev/null | head -n 1)
    kind=${loaded%%|*}
    rest=${loaded#*|}
    shadowing=${rest%%|*}
    rest=${rest#*|}
    remote=${rest%%|*}
    warning=${rest#*|}
    if [ -n "$kind" ]; then
      printf 'herdr_detection_source %s\n' "$kind"
      [ "$kind" = "local override" ] ||
        printf 'note herdr reads its own Muse rules and not %s: %s\n' "$HERDR_RULES" "$warning"
      if [ "$kind" = "local override" ] && [ "$shadowing" = true ] && [ -n "$remote" ]; then
        printf 'herdr_detection_shadows %s\n' "$remote"
        [ "$remote" = "$HERDR_RULES_MEASURED" ] ||
          printf 'note %s\n' "this adapter's Muse rules shadow herdr's Muse manifest $remote, and were measured against $HERDR_RULES_MEASURED. Measure $remote without them: move $HERDR_RULES aside, then run base herdr -- agent explain PANE"
      fi
    fi
  fi
elif [ -f "$HERDR_RULES" ]; then
  printf 'herdr_detection_rules other\n'
else
  printf 'herdr_detection_rules absent\n'
fi

# -- and Muse runs it ----------------------------------------------------------------
# ⛔ THE LINE ABOVE READS THE FILE install.sh WROTE, AND THAT PROVES install.sh WROTE IT.
# A hook in a file Muse does not read, or outside a matcher group, read back as six
# registered events and never ran. So the probe runs one session of Muse's credential-free
# echo provider, and looks for that session's SessionStart in the reporter's own log.
# Finding 26, WSL-157.
#
# ⛔ IN A DIRECTORY NO PANE HAS. The reporter binds a session to the herdr pane whose
# working directory is the session's, so a session started in the account's home could
# claim one of the operator's panes. A fresh directory is no pane's, so the reporter
# logs the session and reports nothing to herdr.
#
# ⭐ AND IT LEAVES NOTHING OF MUSE'S. --no-session-log keeps the session out of the
# account's history, and Muse's data and cache directories are the scratch directory's,
# which goes when the session ends. Measured on Muse Code 1.4.11: the reporter's log
# line was then the only change under the account's home.
# ⛔ THE CONFIGURATION AND STATE DIRECTORIES STAY THE ACCOUNT'S. The first is where the
# hook is registered, and the reporter writes its log under the second.
#
# ⚠ STDIN IS /dev/null. This script reaches the shell on its stdin, and a command that
# read stdin would take the rest of it.
#
# ⛔ A MUSE THAT CANNOT ANSWER IS UNMEASURED, AND ONE THAT ANSWERS AND RUNS NO HOOK IS A
# NOTE. A Muse that cannot run the echo session, or answers no session id, leaves the
# question unmeasured, and says why as a fact. One whose session ran over a file that
# registers SessionStart, and never reached the reporter, is a note naming its version:
# `base ensure` writes the same file again and cannot cure it. Muse comes from an
# installer whose version is not pinned, so either one as a problem would fail every
# `base ensure` a 7.0.0 tool passed, with a remedy that loops. WSL-157.
#
# ⛔ ONLY `base ensure` RUNS THE SESSION. It runs every hook the account's settings
# register, the operator's own included, and it can take a minute. `base status --probe`
# and `ready` are reads, so they answer without the fact, and `base ensure` carries it
# on the state it answers. WSL-157.
REPORTER_LOG=$TK_HOME/.local/state/wsl-toolkit/herdr-muse/log
# >>> the probe's Muse session: begin
if [ "${TK_PROBE_SESSION:-}" != 1 ]; then
  # A read: no session, and no fact that would read like a measurement.
  :
elif [ ! -x "$HOOK" ] || [ ! -x "$LAUNCHER" ]; then
  # Nothing to run, and the lines above have said why.
  :
elif ! command -v herdr >/dev/null 2>&1 && [ ! -x /usr/local/bin/herdr ]; then
  printf 'herdr_reporter_session_start unmeasured: no herdr in this base to report to\n'
elif ! command -v jq >/dev/null 2>&1; then
  printf 'herdr_reporter_session_start unmeasured: no jq to read Muse with\n'
else
  scratch=$(as_account mktemp -d /tmp/wsl-toolkit-muse-probe.XXXXXX 2>/dev/null) || scratch=
  case $scratch in
    /tmp/wsl-toolkit-muse-probe.?*) ;;
    *) scratch= ;;
  esac
  # ⛔ ROOT OPENS NOTHING UNDER THE ACCOUNT'S DIRECTORY. This script runs as root, and
  # the scratch directory is the account's, so a process of the account can put a link
  # where root writes and turn the write onto any file. Muse's stderr goes to a file
  # root's own mktemp makes, outside it. WSL-157.
  errors=$(mktemp /tmp/wsl-toolkit-muse-probe-stderr.XXXXXX 2>/dev/null) || errors=
  # ⛔ AND THE ACCOUNT ENTERS ITS SCRATCH DIRECTORY AND REMOVES IT. Root's cd and rm -rf
  # there walked a directory the account owns, through any link it put there. WSL-202.
  if [ -z "$scratch" ] || [ -z "$errors" ] || ! as_account mkdir "$scratch/work" "$scratch/data" "$scratch/cache"; then
    printf 'herdr_reporter_session_start unmeasured: no scratch directory for a Muse session in /tmp\n'
    [ -z "$scratch" ] || as_account rm -rf "$scratch"
    [ -z "$errors" ] || rm -f "$errors"
  else
    deadline=
    command -v timeout >/dev/null 2>&1 && deadline='timeout 60'
    # shellcheck disable=SC2086,SC2016  # $deadline is a command and its argument, or nothing; the single quotes hold the account's own script
    session_out=$(as_account sh -c 'cd "$1" || exit 1; shift; exec "$@"' sh "$scratch/work" \
      env XDG_DATA_HOME="$scratch/data" XDG_CACHE_HOME="$scratch/cache" \
      MUSE_NO_AUTO_UPDATE=1 $deadline "$LAUNCHER" exec --json --provider echo --no-session-log \
      'wsl-toolkit probe' </dev/null 2>"$errors")
    ran=$?
    said=$(head -1 "$errors" 2>/dev/null)
    # ⚠ EVERY RECORD CARRIES THE SESSION'S STREAM, and the hook's payload names the same id.
    session=$(printf '%s\n' "$session_out" | jq -rR 'fromjson? | select(.stream.kind? == "session") | .stream.id' 2>/dev/null | head -1)
    as_account rm -rf "$scratch"
    rm -f "$errors"
    # ⚠ A HOOK MAY STILL BE RUNNING when `exec` returns, so a miss is read again for a
    # few seconds. The log is read as the account, whose file it is.
    reached=no
    if [ "$ran" -eq 0 ] && [ -n "$session" ]; then
      for _ in 1 2 3 4; do
        if account_read grep -qF "SessionStart for $session " "$REPORTER_LOG" 2>/dev/null; then
          reached=yes
          break
        fi
        sleep 1
      done
    fi
    if [ "$ran" -ne 0 ]; then
      printf 'herdr_reporter_session_start unmeasured: Muse could not run a session of its echo provider, exit %s: %s\n' "$ran" "${said:-it said nothing}"
    elif [ -z "$session" ]; then
      printf 'herdr_reporter_session_start unmeasured: Muse ran a session of its echo provider and its answer named no session\n'
    elif [ "$reached" = yes ]; then
      printf 'herdr_reporter_session_start reached\n'
    else
      case ",${registered:-}," in
        *,SessionStart,*)
          printf 'herdr_reporter_session_start missing\n'
          printf 'note %s\n' "Muse ${version:-of no version} ran session $session, and its SessionStart never reached the herdr reporter at $HOOK, so herdr shows no state for Muse. $MUSE_SETTINGS registers the hook as install.sh writes it, so base ensure cannot cure this: Muse's installer is not pinned, and this Muse runs its hooks some other way. Report it with this Muse version, WSL-157"
          ;;
        *)
          problem "Muse ran session $session and its SessionStart never reached the herdr reporter at $HOOK, because $MUSE_SETTINGS does not register it for SessionStart. Run: base ensure"
          ;;
      esac
    fi
  fi
fi
# <<< the probe's Muse session: end
