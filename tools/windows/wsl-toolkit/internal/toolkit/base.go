package toolkit

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed provision.sh
var provisionScript []byte

//go:embed verify.sh
var verifyScript []byte

// BaseSpaceFloor is what the volume must have free before an import starts.
//
// ⚠ Far above wsl-toolkit.ps1's 256 MiB floor, because this distribution will
// hold an engine and a dozen images. Running out midway leaves a partial disk
// and a registered distribution that does not work.
const BaseSpaceFloor = int64(6) << 30

// BaseState is what `base status` answers.
type BaseState struct {
	Name       string    `json:"name"`
	Image      string    `json:"image"`
	BuiltFrom  string    `json:"built_from,omitempty"`
	User       string    `json:"user"`
	Registered bool      `json:"registered"`
	Running    bool      `json:"running"`
	Healthy    bool      `json:"healthy"`
	Engine     string    `json:"engine,omitempty"`
	DiskPath   string    `json:"disk_path,omitempty"`
	DiskBytes  int64     `json:"disk_bytes,omitempty"`
	DiskKnown  bool      `json:"disk_known"`
	Created    time.Time `json:"created,omitempty"`
	// Identity is what the GUEST says it is, read under --probe. ⛔ It is the
	// authority: `built_from` above comes from a file on this machine that an
	// editor can reach, and this comes from inside the distribution. WSL-42.
	Identity *Identity `json:"identity,omitempty"`
	Problems []string  `json:"problems,omitempty"`
}

// Base is the owned distribution's lifecycle.
type Base struct {
	cfg  Config
	home string
	wsl  *Wsl
	log  func(string)
	// unmarked says this process built the distribution now registered and did
	// not get as far as stamping it, so Remove may unregister it without the
	// guest's proof. ⛔ It is set by the build path and by nothing else: a
	// flag a caller could set would be a way past the guard.
	unmarked bool
}

// NewBase binds the lifecycle to this host.
//
// ⛔ IT DOES NOT CREATE THE STATE DIRECTORY, and it used to. `paths.go` states
// the rule in the function it is about - "a report that creates a directory on a
// machine it is only describing has changed the thing it was asked to measure" -
// and `base status`, a read-only report, reached EnsureHome through here. So did
// `ready`, which is the command an agent runs FIRST, on a machine it has not
// touched. WSL-55. The paths that WRITE ensure; reading a record from a
// directory that does not exist is already "no record".
func NewBase(cfg Config, log func(string)) (*Base, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	w, err := FindWsl()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = func(string) {}
	}
	return &Base{cfg: cfg, home: home, wsl: w, log: log}, nil
}

// Dir is where the distribution's disk lives.
func (b *Base) Dir() string { return filepath.Join(b.home, "base") }

// Status reads the base without changing it. ⭐ `probe` decides whether it runs
// a container: always would cost a pull on a cold machine, never could only
// report that a distribution is registered, which is not the same as usable.
func (b *Base) Status(ctx context.Context, probe bool) (BaseState, error) {
	st := BaseState{Name: b.cfg.Base.Name, Image: b.cfg.Base.Image, User: b.cfg.Base.User}
	distros, err := b.wsl.List(ctx, b.cfg.Base.Name)
	if err != nil {
		return st, err
	}
	for _, d := range distros {
		if strings.EqualFold(d.Name, b.cfg.Base.Name) {
			st.Registered, st.Running = true, d.Running
		}
	}
	st.DiskPath = filepath.Join(b.Dir(), "ext4.vhdx")
	if size, ok := FileSize(st.DiskPath); ok {
		st.DiskBytes, st.DiskKnown = size, true
	}
	if rec, err := b.readRecord(); err == nil {
		st.Created = rec.Created
		// ⛔ The configured image and the one it was built from are two places
		// for one fact, so they are compared. They drift the moment somebody
		// runs `--preset alpine` without `--save`.
		st.BuiltFrom = rec.Image
		// ⚠ THIS IS THE RECORD, and the record is not the authority. Under
		// --probe the guest's own marker replaces both this value and this
		// problem below; without one, the record is the best answer available
		// and the message says which it is.
		if rec.Image != "" && rec.Image != st.Image {
			st.Problems = append(st.Problems, fmt.Sprintf(
				"this machine's record says it was built from %s and the configuration says %s. "+
					"Run `base status --probe` to ask the guest, then `base recreate` to rebuild",
				rec.Image, st.Image))
		}
	}
	if !st.Registered {
		st.Problems = append(st.Problems, "not registered. Create it with: wsl-toolkit base ensure")
		return st, nil
	}
	if !probe {
		return st, nil
	}
	// ⭐ ASKED OF THE GUEST, and only under --probe because it costs a wsl.exe
	// round trip. A status that reports the record alone reports what this
	// machine believes, which is exactly what issue 18 showed to be wrong.
	if id, err := b.wsl.ReadIdentity(ctx, b.cfg.Base.Name); err == nil {
		st.Identity = &id
		st.BuiltFrom = id.Image
		// ⛔ THE RECORD-BASED PROBLEM IS DROPPED, not added to. The record and
		// the marker say the same thing whenever `ensure` has run, so keeping
		// both produced two lines about one disagreement and left a reader
		// wondering which of the two the tool believed. The marker is the
		// authority, so it is the one that speaks.
		st.Problems = withoutRecordDrift(st.Problems)
		if id.Image != st.Image {
			st.Problems = append(st.Problems, fmt.Sprintf(
				"the guest was built from %s and the configuration says %s. Run `base recreate` to rebuild it, or change the configuration back",
				id.Image, st.Image))
		}
	} else if errors.Is(err, ErrNoIdentity) {
		st.Problems = append(st.Problems, fmt.Sprintf(
			"%s carries no identity marker, so this tool cannot confirm it built it. `base ensure` adopts one it has a record for", b.cfg.Base.Name))
	} else {
		st.Problems = append(st.Problems, err.Error())
	}
	engine, verifyErr := b.verify(ctx)
	st.Engine = engine
	if verifyErr != nil {
		st.Problems = append(st.Problems, verifyErr.Error())
		return st, nil
	}
	st.Healthy = true
	return st, nil
}

// withoutRecordDrift removes the problem the host record raised, for the case
// where the guest has since answered the same question with authority.
func withoutRecordDrift(problems []string) []string {
	out := problems[:0]
	for _, p := range problems {
		if strings.HasPrefix(p, recordDriftPrefix) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// recordDriftPrefix is how that problem is recognised. ⚠ A prefix rather than
// a whole-string match, because the message names two images and neither is
// known here.
const recordDriftPrefix = "this machine's record says it was built from "

type baseRecord struct {
	Schema  string    `json:"schema"`
	Name    string    `json:"name"`
	Image   string    `json:"image"`
	User    string    `json:"user"`
	Created time.Time `json:"created"`
}

func (b *Base) recordPath() string { return filepath.Join(b.home, "base.json") }

func (b *Base) readRecord() (baseRecord, error) {
	var rec baseRecord
	data, err := os.ReadFile(b.recordPath())
	if err != nil {
		return rec, err
	}
	return rec, jsonUnmarshal(data, &rec)
}

func (b *Base) writeRecord() error {
	if _, err := EnsureHome(); err != nil {
		return err
	}
	rec := baseRecord{
		Schema: "wsl-toolkit-base/1", Name: b.cfg.Base.Name,
		Image: b.cfg.Base.Image, User: b.cfg.Base.User, Created: time.Now().UTC(),
	}
	data, err := jsonMarshalIndent(rec)
	if err != nil {
		return err
	}
	return writeFileAtomic(b.recordPath(), data, 0o600)
}

// Ensure brings the base into a state where a job can run, doing the least that
// achieves it.
//
// ⭐ The recovery path as well as the create path. The base exists to be wrecked:
// a registered-but-unusable distribution is re-provisioned in place, and only
// one that will not provision is removed and rebuilt.
func (b *Base) Ensure(ctx context.Context, force bool) (BaseState, error) {
	st, err := b.Status(ctx, false)
	if err != nil {
		return st, err
	}
	if st.Registered && force {
		b.log("rebuilding from nothing, as asked")
		if err := b.Remove(ctx); err != nil {
			return st, err
		}
		st.Registered = false
	}
	if st.Registered {
		b.log(b.cfg.Base.Name + " is registered; checking what it is")
		// ⛔ WHAT IT IS COMES BEFORE WHETHER IT WORKS. A health probe runs an
		// Alpine CONTAINER successfully inside whatever the distribution is; it
		// proves the engine works and identifies nothing. WSL-42, issue 18: a
		// base built from Arch, with the config since changed to Alpine, was
		// RELABELLED Alpine because the probe passed, and `base status` then
		// reported no drift over a guest whose /etc/os-release still said Arch.
		if err := b.reconcileIdentity(ctx); err != nil {
			return st, err
		}
		if engine, err := b.verify(ctx); err == nil {
			b.log("the engine answers: " + engine)
			return b.Status(ctx, true)
		} else {
			b.log("it cannot: " + err.Error())
			b.log("re-provisioning in place")
		}
		if err := b.provision(ctx); err != nil {
			b.log("re-provisioning failed: " + err.Error())
			b.log("removing and rebuilding")
			if err := b.Remove(ctx); err != nil {
				return st, err
			}
		} else {
			if engine, err := b.verify(ctx); err != nil {
				return st, fmt.Errorf("re-provisioned and it still cannot run a container: %w", err)
			} else {
				b.log("the engine answers: " + engine)
			}
			return b.Status(ctx, true)
		}
	}
	// ⛔ FROM HERE THIS PROCESS OWNS THE DISTRIBUTION IT IS BUILDING, marker or
	// not, so a build that dies before stamping can still be cleared up.
	b.unmarked = true
	if err := b.create(ctx); err != nil {
		return st, err
	}
	if err := b.provision(ctx); err != nil {
		return st, err
	}
	// ⭐ STAMPED BEFORE IT IS VERIFIED. The marker says what this tool BUILT,
	// which is a fact by the time provisioning has finished; whether it can run
	// a container is a different question and the answer to it is not identity.
	if err := b.wsl.WriteIdentity(ctx, b.cfg.Base.Name, Identity{
		Image: b.cfg.Base.Image, User: b.cfg.Base.User, Tool: toolVersion(),
	}); err != nil {
		return st, err
	}
	b.unmarked = false
	engine, err := b.verify(ctx)
	if err != nil {
		return st, fmt.Errorf("built and it cannot run a container: %w", err)
	}
	b.log("the engine answers: " + engine)
	if err := b.writeRecord(); err != nil {
		return st, err
	}
	return b.Status(ctx, true)
}

// reconcileIdentity compares what the guest says it is against the record and
// the configuration, and refuses rather than papering over a disagreement.
//
// ⭐ THE COMMENT THAT USED TO BE HERE ARGUED FOR ITSELF THREE LINES ABOVE THE
// DEFECT. It read: "Refreshed on every path that leaves a usable base. A record
// written once describes the first build forever." Somebody reasoned about this
// and reached a conclusion that is half right. A record refreshed on every
// healthy path follows the CONFIG, and the thing it claims to describe is the
// GUEST. It is replaced rather than deleted, because the next reader will have
// the same thought. WSL-42, issue 18.
//
// Three outcomes and no fourth:
//
//	the guest carries a marker      the record is rewritten from THE MARKER
//	it carries none and we built it the marker is written and it is said out loud
//	anything else                   a refusal naming the command that rebuilds
func (b *Base) reconcileIdentity(ctx context.Context) error {
	id, err := b.wsl.ReadIdentity(ctx, b.cfg.Base.Name)
	switch {
	case err == nil:
		// ⛔ THE MARKER WINS. The record lives on the host where an editor can
		// reach it; the marker is inside the distribution and was written by a
		// provisioning run. They can disagree, and this is which one is true.
		if err := b.writeRecordFrom(id); err != nil {
			return err
		}
		if id.Image != b.cfg.Base.Image {
			// ⛔ REPORTED AND LEFT VISIBLE. It does not rebuild: a rebuild
			// destroys the thing the operator needs to look at, and `recreate`
			// already exists for the case where they want one.
			b.log(fmt.Sprintf("this distribution was built from %s and the configuration says %s",
				id.Image, b.cfg.Base.Image))
			b.log("run `wsl-toolkit base recreate` to rebuild it, or change the configuration back")
		}
		return nil

	case errors.Is(err, ErrNoIdentity):
		// The upgrade path, and it is part of this entry rather than a follow-up:
		// a base built before markers existed has none.
		rec, recErr := b.readRecord()
		if recErr != nil {
			return fmt.Errorf("%s is registered, carries no wsl-toolkit identity marker and has no record here either, "+
				"so this tool cannot tell whether it built it. Remove it yourself, or run: wsl-toolkit base recreate",
				b.cfg.Base.Name)
		}
		b.log(b.cfg.Base.Name + " predates the identity marker and this tool's own record describes it; stamping it")
		if err := b.wsl.WriteIdentity(ctx, b.cfg.Base.Name, Identity{
			Image: rec.Image, User: rec.User, Built: rec.Created, Tool: toolVersion(),
		}); err != nil {
			return err
		}
		b.log("wrote " + IdentityPath)
		return nil

	default:
		return err
	}
}

// writeRecordFrom writes the host record from what the GUEST said, so the two
// cannot drift apart by the record following the configuration.
func (b *Base) writeRecordFrom(id Identity) error {
	if _, err := EnsureHome(); err != nil {
		return err
	}
	rec := baseRecord{
		Schema: "wsl-toolkit-base/1", Name: b.cfg.Base.Name,
		Image: id.Image, User: id.User, Created: id.Built,
	}
	data, err := jsonMarshalIndent(rec)
	if err != nil {
		return err
	}
	return writeFileAtomic(b.recordPath(), data, 0o600)
}

// toolVersion is the product version where it can be read, and an empty string
// where it cannot. ⚠ A marker with no version is still a marker; refusing to
// stamp a distribution because a version string could not be read would trade a
// working base for a cosmetic field.
func toolVersion() string {
	if ScriptVersion == nil {
		return ""
	}
	v, err := ScriptVersion()
	if err != nil {
		return ""
	}
	return v
}

func (b *Base) create(ctx context.Context) error {
	// ⭐ THE FIRST THING THAT WRITES, so this is where the state directory is
	// brought into existence. Everything above it reads.
	if _, err := EnsureHome(); err != nil {
		return err
	}
	dir := b.Dir()
	if _, err := os.Stat(dir); err == nil {
		// ⛔ Never import over a leftover directory: wsl --import into one that
		// already holds a disk is how two distributions share one file.
		if err := RemoveInside(b.home, dir); err != nil {
			return fmt.Errorf("%s is left over from an earlier build and could not be removed: %w", dir, err)
		}
		b.log("removed a leftover " + dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	engine, err := FindEngine(ctx)
	if err != nil {
		return fmt.Errorf("the base is built from an OCI image, so a host engine is required: %w", err)
	}
	b.log(fmt.Sprintf("host engine: %s, platform %s", engine.Name, engine.Platform()))

	tarPath := filepath.Join(b.home, "base-rootfs.tar")
	defer func() {
		if err := os.Remove(tarPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			b.log("the rootfs archive is still on disk: " + tarPath)
		}
	}()
	if err := engine.ExportRootfs(ctx, b.cfg.Base.Image, tarPath, b.log); err != nil {
		return err
	}
	if err := b.assertSpace(tarPath); err != nil {
		return err
	}
	b.log("importing as WSL2 distribution " + b.cfg.Base.Name)
	if err := b.wsl.Import(ctx, b.cfg.Base.Name, dir, tarPath); err != nil {
		return err
	}
	return nil
}

// assertSpace refuses an import the volume cannot hold, BEFORE anything is
// registered.
func (b *Base) assertSpace(tarPath string) error {
	size, ok := FileSize(tarPath)
	if !ok {
		return fmt.Errorf("cannot measure %s", tarPath)
	}
	need := BaseSpaceFloor + 2*size
	free, ok, err := FreeSpace(b.Dir())
	if err != nil {
		return err
	}
	if !ok {
		// ⚠ "could not measure" is a third answer, and treating it as either of
		// the other two is a lie or a needless refusal.
		b.log("free space on this volume could not be read; importing anyway")
		return nil
	}
	b.log(fmt.Sprintf("space: %s wanted, %s free", HumanBytes(need), HumanBytes(free)))
	if free < need {
		return fmt.Errorf("NOT ENOUGH DISK SPACE for %s. About %s is wanted and %s is free on the volume holding it. "+
			"Nothing has been imported and nothing is registered", b.Dir(), HumanBytes(need), HumanBytes(free))
	}
	return nil
}

func (b *Base) provision(ctx context.Context) error {
	b.log("provisioning: a rootless engine and the " + b.cfg.Base.User + " account")
	out := &prefixWriter{prefix: "", to: b.logWriter()}
	code, err := b.wsl.Exec(ctx, ExecRequest{
		Distro: b.cfg.Base.Name,
		User:   "root",
		Script: provisionScript,
		Env: map[string]string{
			"TK_USER": b.cfg.Base.User,
			"TK_UID":  "1000",
		},
		Timeout: 30 * time.Minute,
		Stdout:  out,
		Stderr:  out,
	})
	out.Flush()
	if err != nil || code != 0 {
		return fmt.Errorf("provisioning exited %d: %w", code, err)
	}
	if !strings.Contains(out.seen.String(), "provision-complete") {
		// ⛔ The script prints that line last, so its absence over a zero exit is
		// a step that exited 0 having done nothing.
		return errors.New("provisioning exited 0 without reaching its last line")
	}
	// WSL reads /etc/wsl.conf at start, so without this restart the settings
	// just written appear to have been applied and are not.
	b.log("restarting so WSL re-reads /etc/wsl.conf")
	if err := b.wsl.Terminate(ctx, b.cfg.Base.Name); err != nil {
		return err
	}
	return nil
}

// verify runs a real container as the unprivileged account and returns the
// engine's own version line.
func (b *Base) verify(ctx context.Context) (string, error) {
	half := func() string {
		var raw [6]byte
		if _, err := rand.Read(raw[:]); err != nil {
			// A failure to read randomness is not a reason to refuse to verify,
			// and it is reported through the marker rather than swallowed.
			return "nrandom"
		}
		return hex.EncodeToString(raw[:])
	}
	m1, m2 := half(), half()
	out, stderr, code, err := b.captureAs(ctx, b.cfg.Base.User, verifyScript, map[string]string{
		"TK_IMAGE": VerifyImage,
		"TK_M1":    m1,
		"TK_M2":    m2,
	}, 20*time.Minute)
	if err != nil || code != 0 {
		return "", fmt.Errorf("a container did not run as %s (exit %d): %s", b.cfg.Base.User, code, firstLine(stderr+out))
	}
	// ⛔ Compared with whitespace removed: a tty wraps a long line, so a marker
	// that arrived correctly can fail an exact match.
	flat := strings.Join(strings.Fields(out), "")
	if !strings.Contains(flat, m1+m2) {
		return "", fmt.Errorf("the container ran and did not return the marker: %s", firstLine(out+stderr))
	}
	engine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "engine ") {
			engine = strings.TrimSpace(strings.TrimPrefix(line, "engine "))
		}
	}
	return engine, nil
}

// VerifyImage is what the health check runs: small, in the catalog, and pulled
// with --pull=missing so a warm base costs nothing.
const VerifyImage = "docker.io/library/alpine:latest"

// captureAs runs a script as an account inside the base with the runtime
// directory every rootless engine needs already set.
func (b *Base) captureAs(ctx context.Context, user string, script []byte, env map[string]string, timeout time.Duration) (string, string, int, error) {
	full := map[string]string{}
	for k, v := range env {
		full[k] = v
	}
	out := &boundedBuffer{max: 8 << 20}
	errBuf := &boundedBuffer{max: 512 << 10}
	code, err := b.wsl.Exec(ctx, ExecRequest{
		Distro: b.cfg.Base.Name, User: user,
		Script: append(guestRuntimePrologue(), script...),
		Env:    full, Timeout: timeout, Stdout: out, Stderr: errBuf,
	})
	return out.String(), errBuf.String(), code, err
}

// guestRuntimePrologue gives the account the runtime directory rootless podman
// needs, which neither WSL nor runuser sets.
//
// ⚠ Not a copy of wsl-toolkit.ps1's -UserEnv prologue. That one prepares an
// arbitrary imported distribution; this runs in one this executable provisioned,
// where the account and the paths are known.
func guestRuntimePrologue() []byte {
	return []byte(`_tk_uid=$(id -u)
_tk_run=/tmp/wsl-toolkit-run-$_tk_uid
(umask 077; mkdir "$_tk_run") 2>/dev/null || :
if [ -L "$_tk_run" ] || [ ! -d "$_tk_run" ]; then
    echo "wsl-toolkit: $_tk_run is not a private directory" >&2; exit 2
fi
chmod 700 "$_tk_run" || exit 2
XDG_RUNTIME_DIR=$_tk_run; export XDG_RUNTIME_DIR
TMPDIR=${TMPDIR:-/tmp}; export TMPDIR
unset _tk_uid _tk_run
`)
}

// Remove unregisters the base and deletes its disk.
func (b *Base) Remove(ctx context.Context) error {
	exists, err := b.wsl.Exists(ctx, b.cfg.Base.Name)
	if err != nil {
		return err
	}
	if exists {
		b.log("unregistering " + b.cfg.Base.Name)
		// ⚠ The marker is DEMANDED unless this tool is clearing up a build of
		// its own that never got far enough to write one. `b.unmarked` is set by
		// the one path that knows that: a provisioning run that failed.
		if err := b.wsl.Unregister(ctx, b.cfg.Base.Name, !b.unmarked); err != nil {
			return err
		}
	}
	dir := b.Dir()
	if _, err := os.Stat(dir); err == nil {
		// wsl --unregister releases the disk asynchronously, so a removal right
		// afterwards can lose the race against a handle about to close.
		var lastErr error
		for attempt := 1; attempt <= 5; attempt++ {
			if lastErr = RemoveInside(b.home, dir); lastErr == nil {
				b.log("deleted " + dir)
				break
			}
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		if lastErr != nil {
			return fmt.Errorf("the disk is still on disk after five attempts: %w", lastErr)
		}
	}
	// ⛔ Through the one deletion. The record outlives the call that wrote it,
	// which is the line RemoveInside's own comment draws.
	if err := RemoveInside(b.home, b.recordPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// prefixWriter relays a child's output through the logger and keeps a copy, so
// a caller can assert on what was said rather than on the exit code.
//
// ⛔ IT IS WRITTEN TO FROM TWO GOROUTINES. provision passes the same
// instance as both Stdout and Stderr, and os/exec runs one copier per stream
// when the writer is not an *os.File. `seen` is a boundedBuffer and locks
// itself; `partial` did not, so the two copiers raced on the same slice. The
// symptom would be an interleaved or dropped provisioning line, in the one
// place whose output decides whether the base is usable.
//
// ⚠ `partial` IS BOUNDED TOO. It holds whatever has arrived since the last
// newline, and a child that writes megabytes without one would grow it without
// limit. Past the ceiling the held text is flushed as its own line rather than
// dropped: a long line is still information, and losing it silently is the
// failure this whole type exists to avoid.
type prefixWriter struct {
	mu      sync.Mutex
	prefix  string
	to      func(string)
	seen    boundedBuffer
	partial []byte
}

// maxPartialLine is how much unterminated output is held before it is flushed
// as a line of its own.
const maxPartialLine = 64 << 10

func (w *prefixWriter) logTo(s string) {
	if w.to != nil {
		w.to(s)
	}
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	if w.seen.max == 0 {
		w.seen.max = 4 << 20
	}
	// ⚠ The bounded copy cannot fail the write. Its whole job is to stop
	// early, and telling the child its output could not be written because a
	// diagnostic buffer is full would kill a provisioning run over nothing.
	_, _ = w.seen.Write(p)
	w.partial = append(w.partial, p...)
	var lines []string
	for {
		i := indexByte(w.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(w.partial[:i]), "\r")
		w.partial = w.partial[i+1:]
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(w.partial) > maxPartialLine {
		lines = append(lines, string(w.partial))
		w.partial = w.partial[:0]
	}
	w.mu.Unlock()

	// ⛔ THE LOGGER IS CALLED OUTSIDE THE LOCK. It is a caller-supplied
	// function, and holding a lock across one is how an unrelated callback
	// deadlocks a provisioning run.
	for _, line := range lines {
		w.logTo(w.prefix + line)
	}
	return len(p), nil
}

// Flush emits whatever arrived after the last newline.
//
// ⚠ A child whose final line has no terminator would otherwise have it held
// forever, which is exactly the line a failing step tends to end on.
func (w *prefixWriter) Flush() {
	w.mu.Lock()
	rest := strings.TrimRight(string(w.partial), "\r")
	w.partial = w.partial[:0]
	w.mu.Unlock()
	if rest != "" {
		w.logTo(w.prefix + rest)
	}
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func (b *Base) logWriter() func(string) { return b.log }
