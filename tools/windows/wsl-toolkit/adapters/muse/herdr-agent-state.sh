#!/bin/sh
# herdr-agent-state.sh - report Muse Code's lifecycle to herdr, as a Muse command
# hook. Installed by the muse adapter into the account's home, and named in Muse's
# settings.json for six events.
#
# THE DEFECT IT EXISTS TO REMOVE: herdr classifies a Muse pane by reading its
# screen, so an approval prompt is `blocked` only while herdr recognises that
# screen's shape, and a Muse session has no identity herdr can restore. This gives
# herdr the events themselves.
#
# ⛔ IT IS A STOPGAP AND SAYS SO. herdr's own Muse integration was proposed as
# herdrdev/herdr#4163 to #4166 and every part was closed WITHOUT merging on
# 2026-09-15. When herdr ships `herdr integration install muse`, this is deleted
# and that is used; docs/reference-sweeps/usable.md carries the contract both
# implement.
#
# ⛔ THREE RULES IT NEVER BREAKS, and each has a reference that broke it:
#
#   1. NOTHING REACHES STDOUT. Muse reads a hook's stdout and it can influence the
#      agent, so every message goes to the log file and nowhere else.
#   2. IT ALWAYS EXITS 0. A hook that fails must not fail the agent.
#   3. IT NEVER GUESSES A PANE. A reference fell back to the most recently updated
#      session of ANY workspace when none matched, so a pane was confidently
#      labelled with another project's session. Here, no match is no report.
#
# ⚠ MUSE HOOKS RUN WITH A CLEARED ENVIRONMENT. HERDR_ENV, HERDR_PANE_ID and
# HERDR_BIN_PATH are not visible here, which is why the pane is resolved rather
# than read. Three ways are tried, most precise first.
#
# Self-test, which needs no Muse and no herdr:
#   sh herdr-agent-state.sh --selftest
set -u

SOURCE=custom:wsl-toolkit-muse
AGENT=muse
EVENT=
MESSAGE_MAX=500

STATE_DIR=${WSL_TOOLKIT_HERDR_MUSE_STATE:-${XDG_STATE_HOME:-$HOME/.local/state}/wsl-toolkit/herdr-muse}
BINDINGS=$STATE_DIR/bindings
LOCK=$STATE_DIR/lock
LOG=$STATE_DIR/log

log() {
  [ -d "$STATE_DIR" ] || mkdir -p "$STATE_DIR" 2>/dev/null || return 0
  printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo -)" "$*" >> "$LOG" 2>/dev/null || :
  # ⚠ BOUNDED, because this file is appended to on every tool call of every turn
  # and nothing else ever truncates it.
  if [ -f "$LOG" ]; then
    lines=$(wc -l < "$LOG" 2>/dev/null || echo 0)
    if [ "${lines:-0}" -gt 2000 ] 2>/dev/null; then
      if tail -n 500 "$LOG" > "$LOG.trim" 2>/dev/null; then
        mv "$LOG.trim" "$LOG" 2>/dev/null || rm -f "$LOG.trim" 2>/dev/null || :
      fi
    fi
  fi
}

# ---------------------------------------------------------------------- herdr --

# herdr_bin answers the herdr to call, or nothing.
#
# ⛔ NO GUESSED INSTALL PATHS. A reference tried /opt/homebrew/bin and
# /usr/local/bin by name; this asks the environment, then PATH, then the one path
# this repository's own adapter writes, which it knows because it wrote it.
herdr_bin() {
  if [ -n "${HERDR_BIN_PATH:-}" ] && [ -x "${HERDR_BIN_PATH}" ]; then
    printf '%s' "$HERDR_BIN_PATH"
    return 0
  fi
  if [ -n "${WSL_TOOLKIT_HERDR_BIN:-}" ] && [ -x "${WSL_TOOLKIT_HERDR_BIN}" ]; then
    printf '%s' "$WSL_TOOLKIT_HERDR_BIN"
    return 0
  fi
  hb=$(command -v herdr 2>/dev/null) && [ -n "$hb" ] && { printf '%s' "$hb"; return 0; }
  [ -x /usr/local/bin/herdr ] && { printf '%s' /usr/local/bin/herdr; return 0; }
  printf ''
}

# ------------------------------------------------------------------- the JSON --

# field reads one top-level string from the hook payload.
#
# ⛔ jq IS REQUIRED, AND ITS ABSENCE IS A NO-OP RATHER THAN A GUESS. An earlier
# draft carried a hand-rolled reader for the case where jq is missing, and it
# could not be made right: a value with an escape in it, a nested object, or a
# comma inside a string each defeat a line-at-a-time parser, and a WRONG session
# id here is worse than no report at all. The base this installs into carries jq
# in its developer toolset, and install.sh refuses to register the hook without
# one.
#
# shellcheck disable=SC2016  # the single quotes hold a jq program, not shell
field() {
  printf '%s' "$2" | "$JQ" -r --arg k "$1" '.[$k] // empty' 2>/dev/null
}

# --------------------------------------------------------------- the bindings --

# ⛔ ONE WRITER AT A TIME. Two hooks can run at once - a subagent's PreToolUse
# beside the lead's - and a lost update means a stale seq, which herdr accepts and
# then ignores. `mkdir` is the atomic primitive every shell has.
lock_take() {
  lt_waited=0
  while [ "$lt_waited" -lt 50 ]; do
    if mkdir "$LOCK" 2>/dev/null; then
      return 0
    fi
    sleep 0.1 2>/dev/null || sleep 1
    lt_waited=$((lt_waited + 1))
  done
  return 1
}
lock_drop() { rmdir "$LOCK" 2>/dev/null || :; }

binding_pane() {
  [ -f "$BINDINGS" ] || return 0
  while read -r b_session b_pane _b_seq; do
    [ "$b_session" = "$1" ] && { printf '%s' "$b_pane"; return 0; }
  done < "$BINDINGS"
}

binding_seq() {
  [ -f "$BINDINGS" ] || return 0
  while read -r b_session _b_pane b_seq; do
    [ "$b_session" = "$1" ] && { printf '%s' "$b_seq"; return 0; }
  done < "$BINDINGS"
}

pane_is_owned() {
  [ -f "$BINDINGS" ] || return 1
  while read -r _b_session b_pane _b_seq; do
    [ "$b_pane" = "$1" ] && return 0
  done < "$BINDINGS"
  return 1
}

binding_put() {
  bp_session=$1
  bp_pane=$2
  bp_seq=$3
  mkdir -p "$STATE_DIR" 2>/dev/null || return 1
  : >> "$BINDINGS"
  bp_tmp=$BINDINGS.$$
  while read -r b_session b_pane b_seq; do
    [ "$b_session" = "$bp_session" ] && continue
    printf '%s %s %s\n' "$b_session" "$b_pane" "$b_seq"
  done < "$BINDINGS" > "$bp_tmp"
  [ -n "$bp_pane" ] && printf '%s %s %s\n' "$bp_session" "$bp_pane" "$bp_seq" >> "$bp_tmp"
  mv "$bp_tmp" "$BINDINGS" 2>/dev/null || { rm -f "$bp_tmp" 2>/dev/null; return 1; }
  return 0
}

# ------------------------------------------------------------ resolving a pane --

# pane_runs_muse says whether herdr sees a muse process in the foreground of a pane.
# ⚠ A read that FAILS answers yes, because refusing on an unreadable answer would
# drop a report for a pane that is probably right.
pane_runs_muse() {
  pm_info=$("$HERDR" pane process-info --pane "$1" 2>/dev/null) || return 0
  [ -n "$pm_info" ] || return 0
  case $pm_info in
    *muse*) return 0 ;;
  esac
  return 1
}

# ancestor_pids prints this process's ancestors, innermost first.
ancestor_pids() {
  ap_pid=${PPID:-0}
  ap_seen=0
  while [ "$ap_pid" -gt 1 ] && [ "$ap_seen" -lt 64 ]; do
    printf '%s\n' "$ap_pid"
    [ -r "/proc/$ap_pid/stat" ] || return 0
    ap_stat=$(cat "/proc/$ap_pid/stat" 2>/dev/null) || return 0
    # ⚠ AFTER THE LAST `) `, because a process name can contain a space or a
    # parenthesis and splitting on the first one reads the wrong field.
    ap_rest=${ap_stat##*) }
    ap_next=$(printf '%s' "$ap_rest" | cut -d' ' -f2)
    case $ap_next in
      ''|*[!0-9]*) return 0 ;;
    esac
    [ "$ap_next" = "$ap_pid" ] && return 0
    ap_pid=$ap_next
    ap_seen=$((ap_seen + 1))
  done
}

# resolve_pane finds the pane this hook is running inside, or prints nothing.
#
# ⭐ THREE WAYS, MOST PRECISE FIRST, and a tie is never broken by recency:
#   1. an ancestor of this process is the pane's shell or one of its foreground
#      processes, which is exact;
#   2. the pane's working directory is the one the payload carries AND its
#      foreground is muse;
#   3. that working directory, and EXACTLY ONE pane has it.
#
# ⛔ TWO PANES ON ONE CHECKOUT IS AN ORDINARY THING TO DO, and picking either of
# them is picking wrong half the time, so way 3 requires a unique match.
#
# ⚠ THE LOOP READS FROM A HERE-DOCUMENT, not from a pipe. A `while` on the right
# of a pipe runs in a subshell, so everything it learned would be discarded at the
# `done` - which is the defect that made an earlier draft write its candidates to
# a temporary file and read them back.
resolve_pane() {
  rp_cwd=$1
  rp_ancestors=$(ancestor_pids)
  rp_by_cwd=
  rp_by_cwd_n=0
  rp_by_muse=

  rp_workspaces=$("$HERDR" workspace list 2>/dev/null) || return 0
  [ -n "$rp_workspaces" ] || return 0
  rp_wids=$(printf '%s' "$rp_workspaces" | "$JQ" -r '.result.workspaces[]?.workspace_id // empty' 2>/dev/null)
  [ -n "$rp_wids" ] || return 0

  for rp_w in $rp_wids; do
    rp_panes=$("$HERDR" pane list --workspace "$rp_w" 2>/dev/null) || continue
    rp_rows=$(printf '%s' "$rp_panes" | "$JQ" -r '.result.panes[]? | "\(.pane_id) \(.foreground_cwd // .cwd // "-")"' 2>/dev/null)
    [ -n "$rp_rows" ] || continue
    while read -r rp_pane rp_pcwd; do
      [ -n "$rp_pane" ] || continue
      rp_info=$("$HERDR" pane process-info --pane "$rp_pane" 2>/dev/null) || rp_info=
      rp_pids=$(printf '%s' "$rp_info" | "$JQ" -r '[.result.process_info.shell_pid?, (.result.process_info.foreground_processes[]?.pid)] | map(select(. != null)) | .[]' 2>/dev/null)
      for rp_a in $rp_ancestors; do
        for rp_p in $rp_pids; do
          if [ "$rp_a" = "$rp_p" ]; then
            printf '%s' "$rp_pane"
            return 0
          fi
        done
      done
      if [ -n "$rp_cwd" ] && [ "$rp_pcwd" = "$rp_cwd" ]; then
        rp_by_cwd=$rp_pane
        rp_by_cwd_n=$((rp_by_cwd_n + 1))
        case $rp_info in
          *muse*) rp_by_muse=$rp_pane ;;
        esac
      fi
    done <<ROWS
$rp_rows
ROWS
  done

  if [ -n "$rp_by_muse" ]; then
    printf '%s' "$rp_by_muse"
    return 0
  fi
  if [ "$rp_by_cwd_n" = 1 ] && [ -n "$rp_by_cwd" ]; then
    printf '%s' "$rp_by_cwd"
    return 0
  fi
  printf ''
}

# ------------------------------------------------------------------ reporting --

# ⛔ THE LOG NAMES THE EVENT, because without it two reports of `working` in one turn
# cannot be told apart and `PreToolUse` cannot be shown to have fired at all. WSL-76's
# fifth outstanding item was exactly that: registered and never driven. Measured on
# 2026-09-17, a real turn logged UserPromptSubmit and then PreToolUse.
#
# ⚠ EVENT IS THE CALLER'S, read from the payload, and empty outside main().
report() {
  r_pane=$1
  r_state=$2
  r_session=$3
  r_seq=$4
  r_message=${5:-}
  set -- pane report-agent "$r_pane" --source "$SOURCE" --agent "$AGENT" \
    --state "$r_state" --agent-session-id "$r_session" --seq "$r_seq"
  if [ -n "$r_message" ]; then
    set -- "$@" --message "$(printf '%.'"$MESSAGE_MAX"'s' "$r_message")"
  fi
  if "$HERDR" "$@" >/dev/null 2>&1; then
    log "reported $r_state on ${EVENT:-?} pane=$r_pane seq=$r_seq"
  else
    log "herdr refused $r_state on ${EVENT:-?} pane=$r_pane seq=$r_seq"
  fi
}

# ⛔ A RELEASE NEEDS ITS OWN SEQ. herdr ignores a pane lifecycle call whose seq is
# not above the last one it recorded, and a release sent without one is dropped in
# silence: the agent row then sits in `herdr agent list` for ever.
release() {
  if "$HERDR" pane release-agent "$1" --source "$SOURCE" --agent "$AGENT" --seq "$2" >/dev/null 2>&1; then
    log "released pane=$1 seq=$2"
  else
    log "herdr refused the release pane=$1 seq=$2"
  fi
}

permission_message() {
  for pm_key in question prompt message title; do
    pm_value=$(field "$pm_key" "$1")
    [ -n "$pm_value" ] && { printf '%s' "$pm_value"; return 0; }
  done
  pm_tool=$(field tool_name "$1")
  [ -n "$pm_tool" ] && { printf 'approval requested: %s' "$pm_tool"; return 0; }
  printf 'approval requested'
}

# ------------------------------------------------------------------ self-test --

selftest() {
  st_fail=0
  st_tmp=${TMPDIR:-/tmp}/herdr-muse-selftest.$$
  mkdir -p "$st_tmp" || { printf 'selftest: no temporary directory\n' >&2; return 1; }
  STATE_DIR=$st_tmp
  BINDINGS=$st_tmp/bindings
  LOCK=$st_tmp/lock
  LOG=$st_tmp/log

  check() {
    if [ "$2" = "$3" ]; then
      printf 'ok    %s\n' "$1"
    else
      printf 'FAIL  %s: got %s, want %s\n' "$1" "$2" "$3"
      st_fail=$((st_fail + 1))
    fi
  }

  body='{"hook_event_name":"PreToolUse","tool_name":"Bash","session_id":"abc-123","cwd":"/w/p","model":"unknown"}'
  check 'field reads an event'        "$(field hook_event_name "$body")" 'PreToolUse'
  check 'field reads a session'       "$(field session_id "$body")"      'abc-123'
  check 'field reads a cwd'           "$(field cwd "$body")"             '/w/p'
  check 'field on an absent key'      "$(field nope "$body")"            ''

  binding_put s1 w1:p1 4 || :
  check 'a binding round-trips'       "$(binding_pane s1)"               'w1:p1'
  check 'its seq round-trips'         "$(binding_seq s1)"                '4'
  binding_put s2 w1:p2 1 || :
  check 'a second binding is kept'    "$(binding_pane s1)"               'w1:p1'
  check 'and so is the first'         "$(binding_pane s2)"               'w1:p2'
  if pane_is_owned w1:p2; then check 'an owned pane is owned' yes yes; else check 'an owned pane is owned' no yes; fi
  if pane_is_owned w1:p9; then check 'an unowned pane is not' yes no; else check 'an unowned pane is not' no no; fi
  binding_put s1 '' 0 || :
  check 'a binding is removable'      "$(binding_pane s1)"               ''
  check 'and the other survives'      "$(binding_pane s2)"               'w1:p2'

  if lock_take; then
    check 'the lock is taken' yes yes
    lock_drop
  else
    check 'the lock is taken' no yes
  fi

  check 'a permission message'        "$(permission_message '{"question":"Allow?"}')"   'Allow?'
  check 'falls back to the tool'      "$(permission_message '{"tool_name":"Bash"}')"    'approval requested: Bash'
  check 'and then to a constant'      "$(permission_message '{}')"                     'approval requested'

  rm -rf "$st_tmp" 2>/dev/null || :
  if [ "$st_fail" = 0 ]; then
    printf 'selftest: every case passed\n'
    return 0
  fi
  printf 'selftest: %s case(s) failed\n' "$st_fail"
  return 1
}

# ----------------------------------------------------------------------- main --

JQ=$(command -v jq 2>/dev/null) || JQ=

case ${1:-} in
  --selftest) selftest; exit $? ;;
esac

main() {
  payload=$(cat 2>/dev/null) || return 0
  [ -n "$payload" ] || return 0

  event=$(field hook_event_name "$payload")
  EVENT=$event
  session=$(field session_id "$payload")
  [ -n "$event" ] && [ -n "$session" ] || return 0

  HERDR=$(herdr_bin)
  [ -n "$HERDR" ] || return 0
  # ⛔ NO jq, NO REPORT. See field() above: a guessed session id is worse than
  # silence, and this is logged once per event rather than swallowed.
  if [ -z "$JQ" ]; then
    log "jq is not installed, so no state was reported for $event"
    return 0
  fi

  cwd=$(field cwd "$payload")
  mkdir -p "$STATE_DIR" 2>/dev/null || return 0
  lock_take || { log "the lock was busy; $event for $session was dropped"; return 0; }

  pane=$(binding_pane "$session")
  seq=$(binding_seq "$session")
  [ -n "$seq" ] || seq=0

  if [ "$event" = SessionStart ]; then
    if [ -n "$pane" ]; then
      lock_drop
      return 0
    fi
    pane=$(resolve_pane "$cwd")
    if [ -z "$pane" ]; then
      log "SessionStart for $session resolved no pane"
      lock_drop
      return 0
    fi
    # ⛔ A PANE ANOTHER SESSION HOLDS IS NOT TAKEN while that session's muse is
    # still there. This is what stops a subagent's SessionStart from seizing the
    # lead conversation's pane.
    if pane_is_owned "$pane" && pane_runs_muse "$pane"; then
      log "SessionStart for $session found $pane already owned and still running muse"
      lock_drop
      return 0
    fi
    binding_put "$session" "$pane" 1 || :
    lock_drop
    report "$pane" idle "$session" 1
    return 0
  fi

  if [ -z "$pane" ]; then
    # ⛔ ADOPTION, for the session that never sent a SessionStart. `muse resume`
    # reuses the original session id, so muse treats the restored conversation as
    # a continuation and emits none; a session already running when this hook was
    # installed sent none either. Without this every later event bails out and the
    # pane stays invisible for the rest of its life.
    case $event in
      UserPromptSubmit|PreToolUse|PermissionRequest) ;;
      *) lock_drop; return 0 ;;
    esac
    pane=$(resolve_pane "$cwd")
    if [ -z "$pane" ] || pane_is_owned "$pane"; then
      lock_drop
      return 0
    fi
    # ⚠ A WALL-CLOCK SEED, because the pane being adopted may already carry a high
    # seq from the session being taken over, and herdr ignores anything at or
    # below it.
    seq=$(date -u +%s 2>/dev/null || echo 1)
    log "adopted $pane for $session at seq $seq"
  fi

  seq=$((seq + 1))
  case $event in
    UserPromptSubmit|PreToolUse)
      binding_put "$session" "$pane" "$seq" || :
      lock_drop
      report "$pane" working "$session" "$seq"
      ;;
    PermissionRequest)
      binding_put "$session" "$pane" "$seq" || :
      lock_drop
      report "$pane" blocked "$session" "$seq" "$(permission_message "$payload")"
      ;;
    Stop)
      binding_put "$session" "$pane" "$seq" || :
      lock_drop
      report "$pane" idle "$session" "$seq"
      ;;
    SessionEnd)
      binding_put "$session" '' 0 || :
      lock_drop
      report "$pane" idle "$session" "$seq"
      release "$pane" "$((seq + 1))"
      ;;
    *)
      lock_drop
      ;;
  esac
  return 0
}

main
exit 0
