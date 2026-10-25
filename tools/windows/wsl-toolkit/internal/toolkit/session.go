// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A detached base session is a payload the guest runs with no client attached.
// A supervisor started with setsid runs it in a process group of its own, puts
// its two streams in files, holds its deadline, and writes its exit code last.
// Nothing on Windows has to stay alive while it runs. WSL-95.
//
// ⭐ MEASURED ON WSL 2.7.12 BEFORE IT WAS BUILT: a process in its own session
// outlives the client that started it, whether the client exits cleanly or is
// killed with its whole tree, on a base with systemd and on one without. A
// plain background process does not survive the killed client.
//
// ⛔ A PROCESS GROUP IS NOT A FENCE. A child that calls setsid, or a daemon
// that double-forks, leaves the payload's group, so `stop` and the deadline
// signalled the group and an emulator podbox started outlived both. Where the
// account's systemd user manager answers, the payload runs in a scope of its
// own, `wtk-session-ID.scope`, and every process it starts stays in that
// scope's cgroup whatever it does to its session. Without one, the group is
// still what is signalled, and the record says which. WSL-126.

// SessionSchema versions a session's host record.
const SessionSchema = "wsl-toolkit-session/1"

const sessionRecordName = "session.json"

// SessionRecord is what the host keeps about one detached session: enough to
// find it in the guest again.
type SessionRecord struct {
	Schema    string    `json:"schema"`
	ID        string    `json:"id"`
	Distro    string    `json:"distro"`
	User      string    `json:"user"`
	GuestDir  string    `json:"guest_dir"`
	Dir       string    `json:"dir"`
	Started   time.Time `json:"started"`
	TimeoutMS int64     `json:"timeout_ms,omitempty"`
	// ArtifactDir is the host directory the session's out directory is delivered
	// to, by `wait` or `logs --follow` once it ends. WSL-124.
	ArtifactDir string `json:"artifact_dir,omitempty"`
	// Unit is the systemd scope the payload runs in, where the account's user
	// manager answered at the start. Empty means its process group is all a
	// stop reaches. WSL-126.
	Unit string `json:"unit,omitempty"`
}

// Containment names how a stop reaches a session: its systemd scope, or its
// process group. WSL-126.
func (rec SessionRecord) Containment() string {
	if rec.Unit != "" {
		return ContainedByScope
	}
	return ContainedByGroup
}

// The two answers Containment gives.
const (
	ContainedByScope = "systemd-scope"
	ContainedByGroup = "process-group"
)

// ReadSessionRecord reads jobs/ID/session.json, if there is one.
func ReadSessionRecord(home, id string) (SessionRecord, bool, error) {
	var rec SessionRecord
	b, err := os.ReadFile(filepath.Join(home, "jobs", id, sessionRecordName))
	if errors.Is(err, os.ErrNotExist) {
		return rec, false, nil
	}
	if err != nil {
		return rec, false, err
	}
	if err := json.Unmarshal(b, &rec); err != nil {
		return rec, false, fmt.Errorf("the session record for %s does not parse: %w", id, err)
	}
	if rec.Schema != SessionSchema {
		return rec, false, fmt.Errorf("the session record for %s has schema %q, and this build reads %q", id, rec.Schema, SessionSchema)
	}
	if err := checkSessionRecord(id, rec); err != nil {
		return rec, false, err
	}
	return rec, true, nil
}

// checkSessionRecord holds a record read from disk to the one layout a session
// has.
//
// ⛔ THE GUEST DIRECTORY IS REMOVED BY PATH, so a record that names any other
// directory is refused here, before a stop, a follow or a removal reads it.
func checkSessionRecord(id string, rec SessionRecord) error {
	want := "/" + GuestRoot + "/sessions/" + id
	switch {
	case rec.ID != id:
		return fmt.Errorf("the session record in %s names session %q", id, rec.ID)
	case !strings.HasPrefix(rec.GuestDir, "/") || !strings.HasSuffix(rec.GuestDir, want) || len(rec.GuestDir) == len(want):
		return fmt.Errorf("the session record for %s names %q, which is not a session directory", id, rec.GuestDir)
	case strings.Contains(rec.GuestDir, "/../") || strings.Contains(rec.GuestDir, "/./") || strings.ContainsAny(rec.GuestDir, "\x00\n\r"):
		return fmt.Errorf("the session record for %s names %q, which is not a plain path", id, rec.GuestDir)
	case rec.Distro == "" || rec.User == "":
		return fmt.Errorf("the session record for %s names no distribution or no account", id)
	case rec.Unit != "" && rec.Unit != sessionUnit(id):
		return fmt.Errorf("the session record for %s names the unit %q, which is not its own scope", id, rec.Unit)
	}
	return nil
}

// sessionUnit is the one name a session's scope has. WSL-126.
func sessionUnit(id string) string { return "wtk-session-" + id + ".scope" }

// sessionsRoot is the guest directory every session of one account is under.
func sessionsRoot(rec SessionRecord) string {
	return strings.TrimSuffix(rec.GuestDir, "/"+rec.ID)
}

func writeSessionRecord(home string, rec SessionRecord) error {
	dir := filepath.Join(home, "jobs", rec.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, sessionRecordName), append(b, '\n'), 0o600)
}

// SessionStart is what `base exec --detach` asks for.
type SessionStart struct {
	Distro  string
	User    string
	Dir     string // "~" or an absolute guest path
	Payload []byte // the command, repaired, with any environment prefixed
	Timeout time.Duration
	Grace   time.Duration
	// Inputs go to the session's in directory, ArtifactDir names the host
	// directory its out directory is delivered to, and Stdin is what the payload
	// reads, nil for /dev/null. WSL-124.
	Inputs      []JobInput
	ArtifactDir string
	Stdin       []byte
}

// SessionPayload is what a detached session runs: the request's environment as
// assignments, then its script, which is what Exec would send framed.
func SessionPayload(req ExecRequest) []byte {
	return append(shellAssignments(req.Env), req.Script...)
}

// SessionGrace is how long a session's payload has between the TERM its
// deadline or a stop sends and the KILL after it.
const SessionGrace = 10 * time.Second

// StartSession places the payload in the guest, starts the supervisor, and
// returns once the payload is running, with its record written on this side.
//
// ⛔ THE PAYLOAD TRAVELS AS A FILE, through the archive channel a job's script
// uses, never on a command line. The supervisor is this tool's own text and
// carries no byte the caller wrote.
func StartSession(ctx context.Context, w *Wsl, home string, s SessionStart) (SessionRecord, error) {
	var rec SessionRecord
	id, err := newJobID()
	if err != nil {
		return rec, err
	}
	guestHome, err := accountHome(ctx, w, s.Distro, s.User)
	if err != nil {
		return rec, err
	}
	guestDir := guestHome + "/" + GuestRoot + "/sessions/" + id
	if err := AssertArgvSafe([]string{guestDir}); err != nil {
		return rec, err
	}
	rec = SessionRecord{Schema: SessionSchema, ID: id, Distro: s.Distro, User: s.User, GuestDir: guestDir,
		Dir: s.Dir, Started: time.Now().UTC(), TimeoutMS: s.Timeout.Milliseconds(), ArtifactDir: s.ArtifactDir}
	// ⭐ RECORDED BEFORE ANYTHING EXISTS in the guest, for the reason a job's
	// ledger record is: a start that dies half way still leaves a record that
	// names where its leftovers are.
	if err := writeSessionRecord(home, rec); err != nil {
		return rec, fmt.Errorf("the session record: %w", err)
	}
	made := []string{"/bin/mkdir", "-p", guestDir}
	if s.ArtifactDir != "" {
		made = append(made, guestDir+"/out")
	}
	errBuf := &boundedBuffer{max: 64 << 10}
	if code, err := w.ExecDirect(ctx, s.Distro, s.User, "", made, nil, io.Discard, errBuf, 2*time.Minute); err != nil || code != 0 {
		return rec, fmt.Errorf("could not create %s in the guest (exit %d): %s", guestDir, code, guestFailure(errBuf.String()))
	}
	files := sessionFiles(guestDir, s)
	errBuf = &boundedBuffer{max: 64 << 10}
	if _, err := pumpArchive(
		func(pw io.Writer) (WorkspaceUpload, error) {
			return writeInputsTar(pw, files)
		},
		func(pr io.Reader) (int, error) {
			return w.ExecDirect(ctx, s.Distro, s.User, "", []string{"/bin/tar", "-xf", "-", "-C", guestDir}, pr, io.Discard, errBuf, 5*time.Minute)
		},
		func() string { return errBuf.String() }); err != nil {
		return rec, fmt.Errorf("the session's payload: %w", err)
	}
	grace := s.Grace
	if grace <= 0 {
		grace = SessionGrace
	}
	out, stderr, code, err := w.Capture(ctx, s.Distro, s.User, sessionLaunchScript(guestDir, s.Dir, s.Timeout, grace), 2*time.Minute)
	if err != nil || code != 0 || !strings.Contains(out, "SESSION|") {
		return rec, fmt.Errorf("the session did not start (exit %d): %s", code, guestFailure(stderr, out))
	}
	// ⭐ THE SCOPE IS RECORDED ONCE IT EXISTS, so the answer and a later stop say
	// which of the two reaches this session. WSL-126.
	if unit := launchedUnit(out, id); unit != "" {
		rec.Unit = unit
		if err := writeSessionRecord(home, rec); err != nil {
			return rec, fmt.Errorf("the session record: %w", err)
		}
	}
	return rec, nil
}

// sessionFiles is what one archive places in a session's directory: the payload,
// with the in and out directories it has exported in front, each input under
// in/, and the bytes its stdin reads. WSL-124.
//
// ⛔ THE EXPORTS GO IN FRONT OF THE CALLER'S OWN ASSIGNMENTS, so a --env that
// names one of the two wins, as a caller's explicit value does everywhere else.
func sessionFiles(guestDir string, s SessionStart) []JobInput {
	var exports strings.Builder
	if len(s.Inputs) > 0 {
		exports.WriteString(ExecInEnv + "=" + shellQuote(guestDir+"/in") + "; export " + ExecInEnv + "\n")
	}
	if s.ArtifactDir != "" {
		exports.WriteString(ExecOutEnv + "=" + shellQuote(guestDir+"/out") + "; export " + ExecOutEnv + "\n")
	}
	files := []JobInput{
		{Name: "payload.sh", Bytes: append([]byte(exports.String()), s.Payload...)},
	}
	for _, in := range s.Inputs {
		files = append(files, JobInput{Name: "in/" + in.Name, Path: in.Path, Bytes: in.Bytes})
	}
	if s.Stdin != nil {
		files = append(files, JobInput{Name: "stdin", Bytes: s.Stdin})
	}
	return files
}

// launchedUnit reads the scope the launch script says the payload runs in, and
// accepts only this session's own. WSL-126.
func launchedUnit(out, id string) string {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "UNIT|"); ok && v == sessionUnit(id) {
			return v
		}
	}
	return ""
}

// accountHome asks the guest where an account's home is. It is read, never
// built from a convention, for the reason Runner.guestHome gives.
func accountHome(ctx context.Context, w *Wsl, distro, user string) (string, error) {
	out, stderr, code, err := w.Capture(ctx, distro, user, []byte("printf '%s\\n' \"$HOME\"\n"), 2*time.Minute)
	if err != nil || code != 0 {
		return "", fmt.Errorf("could not read %s's home in %s (exit %d): %s", user, distro, code, guestFailure(stderr, out))
	}
	h := strings.TrimSpace(firstLine(out))
	if !strings.HasPrefix(h, "/") {
		return "", fmt.Errorf("the guest reported a home directory of %q for %s, which is not an absolute path", h, user)
	}
	return h, nil
}

// sessionLaunchScript starts the supervisor and waits for the payload to run.
//
// The supervisor, in its own session:
//
//	payload.sh    runs under setsid, so its process group is its own and a stop
//	              reaches it and its children without reaching the supervisor.
//	              Its stdin is the session's stdin file where there is one, and
//	              /dev/null otherwise. WSL-124
//	scope         where the account's user manager answers, payload.sh runs in
//	              wtk-session-ID.scope too, and a child that leaves the group
//	              stays in it. The supervisor is outside it. WSL-126
//	watchdog      TERM at the deadline, KILL a grace later; it marks timedout
//	exit          written last, through a rename, so a reader never sees half
//
// ⚠ THE SCOPE IS ASKED FOR ONLY AFTER A PROBE SCOPE STARTED. A base with no
// systemd, or one whose user manager does not answer, keeps the process group
// alone, and the launch prints no UNIT line, so the record says which.
func sessionLaunchScript(guestDir, dir string, timeout, grace time.Duration) []byte {
	cd := `cd "$HOME"`
	if dir != "" && dir != "~" {
		cd = "cd " + shellQuote(dir)
	}
	to := int64(0)
	if timeout > 0 {
		to = int64(timeout.Round(time.Second) / time.Second)
		if to < 1 {
			to = 1
		}
	}
	g := int64(grace.Round(time.Second) / time.Second)
	if g < 1 {
		g = 1
	}
	var b strings.Builder
	b.WriteString("set -u\n")
	b.WriteString("d=" + shellQuote(guestDir) + "\n")
	b.WriteString(`[ -f "$d/payload.sh" ] || { echo "wsl-toolkit: the session's payload is not in $d" >&2; exit 2; }` + "\n")
	b.WriteString(`command -v setsid >/dev/null 2>&1 || { echo "wsl-toolkit: this base has no setsid, so a session cannot run apart from its client" >&2; exit 2; }` + "\n")
	b.WriteString(cd + ` || { echo "wsl-toolkit: the session cannot start in its directory" >&2; exit 2; }` + "\n")
	b.WriteString(sessionScopeProbe)
	b.WriteString(`setsid /bin/sh -c '
d=$1; to=$2; grace=$3; u=${4:-}
` + sessionProcStart + `bootmark > "$d/boot" || rm -f "$d/boot"
pstart "$$" > "$d/supervisor.start" || rm -f "$d/supervisor.start"
printf "%s\n" "$$" > "$d/supervisor.pid"
date -u +%Y-%m-%dT%H:%M:%SZ > "$d/started"
i=/dev/null; if [ -f "$d/stdin" ]; then i="$d/stdin"; fi
if [ -n "$u" ]; then
  setsid systemd-run --user --scope --unit="$u" --quiet --collect -- /bin/sh "$d/payload.sh" <"$i" >"$d/stdout" 2>"$d/stderr" &
else
  setsid /bin/sh "$d/payload.sh" <"$i" >"$d/stdout" 2>"$d/stderr" &
fi
p=$!
pstart "$p" > "$d/payload.start" || rm -f "$d/payload.start"
printf "%s\n" "$p" > "$d/payload.pid"
w=
if [ "$to" -gt 0 ]; then
  ( trap "kill \$s 2>/dev/null; exit 0" TERM
    sleep "$to" & s=$!; wait "$s"
    late=
    if [ -e "$d/exit" ] || [ ! -d "$d" ]; then late=1; else : > "$d/timedout"; kill -TERM "-$p" 2>/dev/null; fi
    if [ -n "$u" ]; then systemctl --user kill --signal=TERM "$u" >/dev/null 2>&1; fi
    sleep "$grace" & s=$!; wait "$s"
    if [ -z "$late" ]; then kill -KILL "-$p" 2>/dev/null; fi
    if [ -n "$u" ]; then systemctl --user kill --signal=KILL "$u" >/dev/null 2>&1; fi ) </dev/null >/dev/null 2>&1 &
  w=$!
fi
wait "$p"; code=$?
if [ -n "$w" ] && { [ -z "$u" ] || { [ ! -e "$d/timedout" ] && ! systemctl --user is-active --quiet "$u" 2>/dev/null; }; }; then kill "$w" 2>/dev/null; fi
date -u +%Y-%m-%dT%H:%M:%SZ > "$d/ended"
printf "%s\n" "$code" > "$d/exit.tmp" && mv -f "$d/exit.tmp" "$d/exit"
' wtk-session "$d" ` + strconv.FormatInt(to, 10) + " " + strconv.FormatInt(g, 10) + ` "$u" </dev/null >/dev/null 2>&1 &
i=0
while [ ! -s "$d/payload.pid" ] && [ "$i" -lt 100 ]; do sleep 0.1 2>/dev/null || sleep 1; i=$((i+1)); done
[ -s "$d/payload.pid" ] || { echo "wsl-toolkit: the session did not start" >&2; exit 2; }
printf 'SESSION|%s\n' "$(cat "$d/payload.pid")"
if [ -n "$u" ]; then printf 'UNIT|%s\n' "$u"; fi
`)
	return []byte(b.String())
}

// sessionProcStart defines pstart, which prints when a process started, in clock
// ticks after boot: field 22 of /proc/PID/stat, counted after the name, which
// may hold spaces. A reused process id names a process that started later.
// WSL-184.
//
// It defines bootmark too, which names this start of the base: the kernel's boot
// id, and when the distribution's own init started.
//
// ⛔ THE BOOT ID ALONE MISSES THE COMMON RESTART. `wsl --terminate`, and this
// tool's own restart after provisioning, keep WSL's VM and its boot id. The
// distribution's init starts again, and its process ids count up from the
// bottom again, so init's start is what such a restart changes.
//
// ⚠ NO SINGLE QUOTE IN IT, because the supervisor's own text is one.
const sessionProcStart = `pstart() {
  ps_l=$(cat "/proc/$1/stat" 2>/dev/null) || return 1
  ps_l=${ps_l##*") "}
  set -f; set -- $ps_l; set +f
  [ "$#" -ge 20 ] || return 1
  printf "%s\n" "${20}"
}
bootmark() {
  bm_b=$(cat /proc/sys/kernel/random/boot_id 2>/dev/null) || return 1
  bm_i=$(pstart 1) || return 1
  [ -n "$bm_b" ] && [ -n "$bm_i" ] || return 1
  printf "%s %s\n" "$bm_b" "$bm_i"
}
`

// sessionOwnerRead is the guest half that holds a session's process ids to the
// processes its launch recorded. sup answers whether the supervisor named in
// supervisor.pid runs, and sets lost where it cannot be this session's: restart
// where the base started again since the launch, reused where the id now names a
// process that started later. WSL-184.
//
// ⛔ A RESTART ENDS EVERY SESSION AND KEEPS EVERY FILE. The pid files then name
// whatever the account runs next under those ids, so `follow` waited forever on
// an unrelated process and `stop` signalled its group. The start of the base and
// the start of the process are what a reused id cannot carry.
//
// ⚠ A SESSION AN OLDER BUILD STARTED HAS NONE OF THESE FILES, and is read as
// before.
// ⚠ Its names carry a prefix: a shell function shares every variable with the
// script that calls it, and the follow script's t is its tail.
const sessionOwnerRead = sessionProcStart + `sup() {
  lost=
  sup_s=$(cat "$d/supervisor.pid" 2>/dev/null) || sup_s=
  [ -n "$sup_s" ] || return 1
  sup_b=$(cat "$d/boot" 2>/dev/null) || sup_b=
  if [ -n "$sup_b" ] && sup_n=$(bootmark) && [ "$sup_b" != "$sup_n" ]; then lost=restart; return 1; fi
  kill -0 "$sup_s" 2>/dev/null || return 1
  sup_t=$(cat "$d/supervisor.start" 2>/dev/null) || sup_t=
  if [ -n "$sup_t" ] && [ "$sup_t" != "$(pstart "$sup_s")" ]; then lost=reused; return 1; fi
  return 0
}
`

// sessionScopeProbe decides whether a session runs in a scope of its own: the
// account's user manager must start a probe scope first. It sets u to the
// session's scope and records it in the guest directory, or leaves u empty.
//
// ⛔ THE WATCHDOG IS LEFT RUNNING PAST A DEADLINE ONLY IN A SCOPE. There its KILL
// after the grace reaches what ignored the TERM and outlived the payload's own
// shell; a process group keeps the behaviour it had. WSL-126.
//
// ⛔ AND IN A SCOPE IT OUTLIVES A PAYLOAD THAT ENDED FIRST, while the scope still
// holds a process. A daemon the payload left there outlived the deadline until
// someone ran stop. At the deadline it signals the scope alone: the session's
// own answer stands, and timedout is not written. WSL-126.
//
// ⛔ A SESSION DIRECTORY THAT IS GONE AT THE DEADLINE IS LATE TOO. gc removes an
// ended session's directory, exit file included, and the watchdog then signalled
// the ended payload's process-group id, which another process may hold. WSL-126.
const sessionScopeProbe = `u=
if command -v systemd-run >/dev/null 2>&1 && command -v systemctl >/dev/null 2>&1 &&
  systemd-run --user --scope --quiet --collect -- true </dev/null >/dev/null 2>&1; then
  u="wtk-session-${d##*/}.scope"
  printf '%s\n' "$u" > "$d/unit" || u=
fi
`

// sessionUnitRead is the guest half that reads a session's scope back, and
// accepts only a session's own scope name. live answers whether it still holds
// a process. WSL-126.
const sessionUnitRead = `u=$(cat "$d/unit" 2>/dev/null) || u=
case "$u" in wtk-session-*.scope) ;; *) u= ;; esac
live() { [ -n "$u" ] && systemctl --user is-active --quiet "$u" 2>/dev/null; }
`

// SessionState is what the guest says about one session.
type SessionState struct {
	Alive    bool   `json:"alive"`
	Exit     *int   `json:"exit,omitempty"`
	TimedOut bool   `json:"timed_out,omitempty"`
	Stopped  bool   `json:"stopped,omitempty"`
	Started  string `json:"started,omitempty"`
	Ended    string `json:"ended,omitempty"`
	Stdout   int64  `json:"stdout_bytes"`
	Stderr   int64  `json:"stderr_bytes"`
	// Gone is true where the guest holds nothing for this session.
	Gone bool `json:"gone,omitempty"`
	// Lost says why a session with no exit is not this session any more:
	// restart, where the base started again since its launch, or reused, where
	// its id names a process that started later. WSL-184.
	Lost string `json:"lost,omitempty"`
}

// The two answers SessionState.Lost carries. WSL-184.
const (
	SessionLostRestart = "restart"
	SessionLostReused  = "reused"
)

// LostReason is the sentence a reader gets for a session that is gone without
// an exit, or an empty string. WSL-184.
func (s SessionState) LostReason() string {
	switch s.Lost {
	case SessionLostRestart:
		return "the base started again after this session did, and its supervisor went without an exit"
	case SessionLostReused:
		return "its supervisor's process id now names another process, and the supervisor went without an exit"
	}
	return ""
}

// Verdict is the code a caller gets for a session, on the rules a job keeps:
// 124 for its own deadline, 130 for a stop, the payload's own code otherwise.
func (s SessionState) Verdict() int {
	switch {
	case s.TimedOut:
		return ExitTimeout
	case s.Stopped:
		return 130
	case s.Exit != nil:
		return *s.Exit
	}
	return ExitFailed
}

// sessionStateScript reports on each session directory it is given, each report
// headed by a DIR line, so one guest call answers for many sessions.
func sessionStateScript(guestDirs ...string) []byte {
	var b strings.Builder
	b.WriteString(sessionOwnerRead)
	b.WriteString(`yn() { if [ -e "$1" ]; then echo yes; else echo no; fi; }
state() {
d=$1
printf 'DIR|%s\n' "$d"
if [ ! -d "$d" ]; then echo 'GONE|yes'; return 0; fi
alive=no; if sup; then alive=yes; fi
printf 'ALIVE|%s\n' "$alive"
if [ -n "$lost" ] && [ ! -e "$d/exit" ]; then printf 'LOST|%s\n' "$lost"; fi
printf 'EXIT|%s\n' "$(cat "$d/exit" 2>/dev/null)"
printf 'TIMEDOUT|%s\n' "$(yn "$d/timedout")"
printf 'STOPPED|%s\n' "$(yn "$d/stopped")"
printf 'STARTED|%s\n' "$(cat "$d/started" 2>/dev/null)"
printf 'ENDED|%s\n' "$(cat "$d/ended" 2>/dev/null)"
printf 'BYTES|%s|%s\n' "$(wc -c < "$d/stdout" 2>/dev/null)" "$(wc -c < "$d/stderr" 2>/dev/null)"
}
`)
	for _, d := range guestDirs {
		b.WriteString("state " + shellQuote(d) + "\n")
	}
	return []byte(b.String())
}

// SessionNow is what is true of one session now. Where its distribution is no
// longer registered, the session is gone with it, and that is an answer rather
// than a failure to ask.
func SessionNow(ctx context.Context, w *Wsl, rec SessionRecord) (SessionState, error) {
	registered, err := w.Exists(ctx, rec.Distro)
	if err != nil {
		return SessionState{}, fmt.Errorf("could not ask whether %s is registered: %w", rec.Distro, err)
	}
	if !registered {
		return SessionState{Gone: true}, nil
	}
	return ReadSessionState(ctx, w, rec)
}

// ReadSessionState asks the guest about one session.
func ReadSessionState(ctx context.Context, w *Wsl, rec SessionRecord) (SessionState, error) {
	states, err := readSessionStates(ctx, w, rec.Distro, rec.User, []string{rec.GuestDir})
	if err != nil {
		return SessionState{}, fmt.Errorf("could not read session %s: %w", rec.ID, err)
	}
	return states[rec.GuestDir], nil
}

// readSessionStates asks one account in one distribution about many sessions,
// in one call.
func readSessionStates(ctx context.Context, w *Wsl, distro, user string, guestDirs []string) (map[string]SessionState, error) {
	if err := AssertArgvSafe(guestDirs); err != nil {
		return nil, err
	}
	out, stderr, code, err := w.Capture(ctx, distro, user, sessionStateScript(guestDirs...), 2*time.Minute)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("the guest %s could not be asked (exit %d): %s", distro, code, guestFailure(stderr, out))
	}
	states := parseSessionStates(out)
	for _, d := range guestDirs {
		if _, ok := states[d]; !ok {
			return nil, fmt.Errorf("the guest %s gave no answer for %s", distro, d)
		}
	}
	return states, nil
}

// parseSessionStates splits the script's answer at its DIR lines.
func parseSessionStates(out string) map[string]SessionState {
	states := map[string]SessionState{}
	dir, section := "", []string{}
	flush := func() {
		if dir != "" {
			states[dir] = parseSessionState(strings.Join(section, "\n"))
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "DIR|"); ok {
			flush()
			dir, section = v, nil
			continue
		}
		section = append(section, line)
	}
	flush()
	return states
}

// parseSessionState reads one session's lines. ⭐ It is a function with no
// process in it, so its case runs on any host.
func parseSessionState(out string) SessionState {
	var st SessionState
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(strings.TrimSpace(strings.TrimRight(line, "\r")), "|")
		if !ok {
			continue
		}
		switch key {
		case "GONE":
			st.Gone = val == "yes"
		case "ALIVE":
			st.Alive = val == "yes"
		case "LOST":
			if v := strings.TrimSpace(val); v == SessionLostRestart || v == SessionLostReused {
				st.Lost = v
			}
		case "EXIT":
			if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil {
				st.Exit = &n
			}
		case "TIMEDOUT":
			st.TimedOut = val == "yes"
		case "STOPPED":
			st.Stopped = val == "yes"
		case "STARTED":
			st.Started = strings.TrimSpace(val)
		case "ENDED":
			st.Ended = strings.TrimSpace(val)
		case "BYTES":
			o, e, _ := strings.Cut(val, "|")
			st.Stdout, _ = strconv.ParseInt(strings.TrimSpace(o), 10, 64)
			st.Stderr, _ = strconv.ParseInt(strings.TrimSpace(e), 10, 64)
		}
	}
	return st
}

// StopSession sends the payload's process group TERM, then KILL a grace later,
// and waits for the supervisor to record the exit.
func StopSession(ctx context.Context, w *Wsl, rec SessionRecord, grace time.Duration) (SessionState, error) {
	budget := grace + 30*time.Second
	out, stderr, code, err := w.Capture(ctx, rec.Distro, rec.User, stopSessionScript(rec.GuestDir, grace), budget)
	if err != nil || code != 0 {
		return SessionState{}, fmt.Errorf("could not stop session %s (exit %d): %s", rec.ID, code, guestFailure(stderr, out))
	}
	return ReadSessionState(ctx, w, rec)
}

// stopSessionScript marks the session stopped, sends its process group TERM,
// and sends KILL where the group outlives the grace. It returns once the
// supervisor has written the exit, or ten seconds after the KILL.
//
// ⛔ NO `--` BEFORE THE GROUP, here or in the supervisor's watchdog. dash, the
// /bin/sh of a Debian or Ubuntu base, reads `--` as a process id, refuses it
// and signals nothing; `kill -TERM "-$pg"` is read as a group by dash, bash and
// busybox alike.
//
// ⭐ A SESSION IN A SCOPE IS SIGNALLED THROUGH THE SCOPE AS WELL, and the grace
// runs until the scope is empty, so a child that left the group and ignores the
// TERM still gets the KILL. WSL-126.
//
// ⛔ NOTHING IS SIGNALLED FOR A SESSION THE BASE OUTLIVED. A supervisor from
// another boot, or one whose id now names a later process, makes the stop print
// LOST and send nothing. A group whose leader started later than the payload
// did is another process's group, and it is left alone too. WSL-184.
func stopSessionScript(guestDir string, grace time.Duration) []byte {
	g := int64(grace.Round(time.Second) / time.Second)
	if g < 1 {
		g = 1
	}
	return []byte("d=" + shellQuote(guestDir) + "\ng=" + strconv.FormatInt(g, 10) + "\n" + sessionUnitRead + sessionOwnerRead + `
if [ -e "$d/exit" ]; then exit 0; fi
if ! sup && [ -n "$lost" ]; then printf 'LOST|%s\n' "$lost"; exit 0; fi
pg=$(cat "$d/payload.pid" 2>/dev/null)
if [ -z "$pg" ]; then echo "wsl-toolkit: the session records no process group" >&2; exit 2; fi
ps=$(cat "$d/payload.start" 2>/dev/null) || ps=
if [ -n "$ps" ] && pn=$(pstart "$pg") && [ "$pn" != "$ps" ]; then printf 'LOST|%s\n' reused; exit 0; fi
: > "$d/stopped"
if [ -n "$u" ]; then systemctl --user kill --signal=TERM "$u" >/dev/null 2>&1; fi
kill -TERM "-$pg" 2>/dev/null
i=0; while [ "$i" -lt "$g" ] && { [ ! -e "$d/exit" ] || live; }; do sleep 1; i=$((i+1)); done
if [ ! -e "$d/exit" ]; then kill -KILL "-$pg" 2>/dev/null; fi
if live; then systemctl --user kill --signal=KILL "$u" >/dev/null 2>&1; fi
i=0; while [ "$i" -lt 10 ] && [ ! -e "$d/exit" ]; do sleep 1; i=$((i+1)); done
`)
}

// StopSessionLeftovers ends what an ended session left running in its scope:
// a daemon its payload started and did not wait for. It answers whether the
// scope held anything, and whether it still does. WSL-126.
//
// ⚠ A SESSION WITH NO SCOPE HAS NOTHING TO ASK. Its process group was the only
// handle, and the group ended with the payload's own shell.
func StopSessionLeftovers(ctx context.Context, w *Wsl, rec SessionRecord, grace time.Duration) (held, still bool, err error) {
	if rec.Unit == "" {
		return false, false, nil
	}
	out, stderr, code, err := w.Capture(ctx, rec.Distro, rec.User, sessionLeftoversScript(rec.GuestDir, grace), grace+30*time.Second)
	if err != nil || code != 0 {
		return false, false, fmt.Errorf("could not end what session %s left in its scope (exit %d): %s", rec.ID, code, guestFailure(stderr, out))
	}
	return parseLeftovers(out)
}

// parseLeftovers reads the leftovers script's one line. ⭐ A function with no
// process in it, so its case runs on any host.
func parseLeftovers(out string) (held, still bool, err error) {
	for _, line := range strings.Split(out, "\n") {
		switch strings.TrimSpace(line) {
		case "LEFT|none":
			return false, false, nil
		case "LEFT|ended":
			return true, false, nil
		case "LEFT|running":
			return true, true, nil
		}
	}
	return false, false, fmt.Errorf("the guest gave no answer about the session's scope: %s", guestFailure(out))
}

// sessionLeftoversScript TERMs a scope that still holds a process, KILLs it a
// grace later, and says what is left.
func sessionLeftoversScript(guestDir string, grace time.Duration) []byte {
	g := int64(grace.Round(time.Second) / time.Second)
	if g < 1 {
		g = 1
	}
	return []byte("d=" + shellQuote(guestDir) + "\ng=" + strconv.FormatInt(g, 10) + "\n" + sessionUnitRead + `
if ! live; then echo 'LEFT|none'; exit 0; fi
systemctl --user kill --signal=TERM "$u" >/dev/null 2>&1
i=0; while [ "$i" -lt "$g" ] && live; do sleep 1; i=$((i+1)); done
if live; then systemctl --user kill --signal=KILL "$u" >/dev/null 2>&1; fi
i=0; while [ "$i" -lt 10 ] && live; do sleep 1; i=$((i+1)); done
if live; then echo 'LEFT|running'; else echo 'LEFT|ended'; fi
`)
}

// FollowSession writes a session's streams as they grow, from the start or from
// the last tail lines, until the supervisor records the exit or is gone.
func FollowSession(ctx context.Context, w *Wsl, rec SessionRecord, tail int, stdout, stderr io.Writer) (SessionState, error) {
	if st, err := SessionNow(ctx, w, rec); err != nil || st.Gone {
		return st, err
	}
	code, err := w.Exec(ctx, ExecRequest{Distro: rec.Distro, User: rec.User, Script: followSessionScript(rec.GuestDir, tail), Stdout: stdout, Stderr: stderr})
	if ctx.Err() != nil {
		return SessionState{}, ctx.Err()
	}
	if err != nil || code != 0 {
		return SessionState{}, fmt.Errorf("could not follow session %s (exit %d): %v", rec.ID, code, err)
	}
	return ReadSessionState(ctx, w, rec)
}

// ReadSessionStreams writes a session's recorded streams, named "stdout" or
// "stderr", in the order given, whole or their last tail lines. It is `logs ID`
// for a session, whose streams are in the guest.
func ReadSessionStreams(ctx context.Context, w *Wsl, rec SessionRecord, names []string, tail int, out io.Writer) error {
	script, err := sessionStreamsScript(rec.GuestDir, names, tail)
	if err != nil {
		return err
	}
	errBuf := &boundedBuffer{max: 64 << 10}
	code, err := w.Exec(ctx, ExecRequest{Distro: rec.Distro, User: rec.User, Script: script, Stdout: out, Stderr: errBuf, Timeout: 10 * time.Minute})
	if err != nil || code != 0 {
		return fmt.Errorf("could not read session %s (exit %d): %s", rec.ID, code, guestFailure(errBuf.String()))
	}
	return nil
}

// sessionStreamsScript writes the named streams, whole or their last tail
// lines, in order.
func sessionStreamsScript(guestDir string, names []string, tail int) ([]byte, error) {
	var b strings.Builder
	b.WriteString("d=" + shellQuote(guestDir) + "\n")
	b.WriteString(`[ -d "$d" ] || { echo "wsl-toolkit: the guest holds nothing for this session" >&2; exit 2; }` + "\n")
	for _, n := range names {
		if n != "stdout" && n != "stderr" {
			return nil, fmt.Errorf("a session keeps stdout and stderr, not %q", n)
		}
		if tail > 0 {
			fmt.Fprintf(&b, "tail -n %d \"$d/%s\" 2>/dev/null || :\n", tail, n)
		} else {
			fmt.Fprintf(&b, "cat \"$d/%s\" 2>/dev/null || :\n", n)
		}
	}
	return []byte(b.String()), nil
}

// followSessionScript writes each stream's new bytes, once a second, until the
// exit is recorded or the supervisor is gone. The last pass after the exit
// reads what the payload wrote before it ended.
//
// ⛔ A SUPERVISOR IS GONE WHERE ITS ID NAMES ANOTHER PROCESS. `follow` across a
// restart waited on whatever the account ran next under that id, forever.
// WSL-184.
func followSessionScript(guestDir string, tail int) []byte {
	return []byte("d=" + shellQuote(guestDir) + "\nt=" + strconv.Itoa(tail) + "\n" + sessionOwnerRead + `
size() { n=$(wc -c 2>/dev/null < "$1"); echo $((n + 0)); }
start() {
  if [ "$t" -gt 0 ]; then k=$(tail -n "$t" "$1" 2>/dev/null | wc -c); echo $(($(size "$1") - k)); else echo 0; fi
}
so=$(start "$d/stdout"); se=$(start "$d/stderr")
while :; do
  ended=no; if [ -e "$d/exit" ]; then ended=yes; fi
  n=$(size "$d/stdout")
  if [ "$n" -gt "$so" ]; then tail -c +$((so + 1)) "$d/stdout" | head -c $((n - so)); so=$n; fi
  n=$(size "$d/stderr")
  if [ "$n" -gt "$se" ]; then tail -c +$((se + 1)) "$d/stderr" | head -c $((n - se)) >&2; se=$n; fi
  if [ "$ended" = yes ]; then break; fi
  if ! sup; then
    if [ ! -e "$d/exit" ]; then break; fi
  fi
  sleep 1
done
`)
}

// SessionDelivery is what became of a session's out directory once it ended.
// WSL-124.
type SessionDelivery struct {
	To        string          `json:"to"`
	Delivered int             `json:"delivered"`
	Attempted int             `json:"attempted"`
	Bytes     int64           `json:"bytes"`
	List      []ArtifactEntry `json:"list,omitempty"`
	Unlisted  int             `json:"unlisted,omitempty"`
	Error     string          `json:"error,omitempty"`
	// Earlier says an earlier `wait` or `logs --follow` delivered it, and this is
	// that delivery's record rather than a second copy.
	Earlier bool `json:"earlier,omitempty"`
}

const sessionDeliveryName = "artifacts.json"

// DeliverSessionArtifacts copies an ended session's out directory to the host
// directory its record names, once. Nil where the session asked for none.
//
// ⭐ THE FIRST DELIVERY THAT SUCCEEDS IS RECORDED, and a later `wait` reads the
// record rather than writing the files again over whatever the caller did with
// them. A delivery that failed is not recorded, so the next one tries again.
//
// ⚠ NOBODY HAS TO WAIT, AND THEN NOTHING IS DELIVERED. The output stays in the
// guest with the session, and `gc` removes it with the session.
func DeliverSessionArtifacts(ctx context.Context, w *Wsl, home string, rec SessionRecord, st SessionState) *SessionDelivery {
	if rec.ArtifactDir == "" {
		return nil
	}
	recordPath := filepath.Join(home, "jobs", rec.ID, sessionDeliveryName)
	if b, err := os.ReadFile(recordPath); err == nil {
		var earlier SessionDelivery
		if json.Unmarshal(b, &earlier) == nil {
			earlier.Earlier = true
			return &earlier
		}
	}
	d := &SessionDelivery{To: rec.ArtifactDir}
	if st.Gone {
		d.Error = "the guest holds nothing for this session, so its output went with it"
		return d
	}
	got, err := w.FetchArtifacts(ctx, rec.Distro, rec.User, rec.GuestDir+"/out", rec.ArtifactDir, DefaultWorkspaceLimits(), nil)
	d.Delivered, d.Attempted, d.Bytes, d.List, d.Unlisted = got.Delivered, got.Attempted, got.Bytes, got.List, got.Unlisted
	if err != nil {
		d.Error = err.Error()
		return d
	}
	if b, err := json.MarshalIndent(d, "", "  "); err == nil {
		_ = writeFileAtomic(recordPath, append(b, '\n'), 0o600)
	}
	return d
}

// RemoveSession removes one ended session's guest directory, checking
// containment in the guest as every removal of state does, then its host
// directory through the one host deletion. Where the distribution is no longer
// registered, the guest half is gone with it and only the host half is removed.
func RemoveSession(ctx context.Context, w *Wsl, home string, rec SessionRecord) error {
	registered, err := w.Exists(ctx, rec.Distro)
	if err != nil {
		return fmt.Errorf("could not ask whether %s is registered: %w", rec.Distro, err)
	}
	if registered {
		// ⛔ WHAT THE SESSION LEFT IN ITS SCOPE IS ENDED FIRST, and a removal it
		// outlives is refused. The record holds the scope's name, and without it
		// stop could no longer reach a daemon the payload left there. WSL-126.
		_, still, err := StopSessionLeftovers(ctx, w, rec, SessionGrace)
		if err != nil {
			return fmt.Errorf("%w, so session %s is kept for stop to reach it", err, rec.ID)
		}
		if still {
			return fmt.Errorf("session %s left processes in its scope %s that did not end, so it is kept for stop to reach them", rec.ID, rec.Unit)
		}
		out, stderr, code, err := w.Capture(ctx, rec.Distro, rec.User, []byte(GuestRemoveScript(sessionsRoot(rec), rec.GuestDir)), 2*time.Minute)
		if err != nil || code != 0 || !strings.Contains(out, "removed") {
			return fmt.Errorf("could not remove session %s in the guest (exit %d): %s", rec.ID, code, guestFailure(stderr, out))
		}
	}
	return RemoveInside(home, filepath.Join(home, "jobs", rec.ID))
}
