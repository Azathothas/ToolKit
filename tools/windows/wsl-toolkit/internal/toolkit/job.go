package toolkit

import (
	"archive/tar"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// StopGraceFlag is what every removal of a container this tool started carries.
//
// ⛔ IT IS WHERE WSL-45's MISSING SECONDS WERE, and it is a measurement rather
// than a theory. `podman rm -f` sends SIGTERM and waits out podman's default
// ten second stop timeout before SIGKILL, so removing a container whose payload
// is still running cost 10.63s on the development host on 2026-09-10. The same
// removal with `-t 0` cost 0.56s, of which 0.46s is the wsl.exe round trip
// itself. That is the whole difference between a 2s deadline returning in about
// sixteen seconds and returning in about five.
//
// ⚠ IT IS CORRECT AND NOT MERELY FAST. Every container removed through this has
// finished, has passed a deadline the caller set, or has been named by a caller
// asking for it to go. A payload already told its time is up does not get
// another ten seconds to ignore a signal.
const StopGraceFlag = "-t 0"

// CleanupGrace is how long a job that passed its deadline may spend stopping.
//
// ⛔ A DEADLINE BOUNDS THE CALLER, NOT ONLY THE CHILD. WSL-19 put the deadline
// in and bounded the child, which was the problem it was given; WSL-45 is the
// discovery that a caller who asked for two seconds still waited fifteen,
// because killing the container and removing the guest directory each cost a
// fresh wsl.exe invocation with a ceiling measured in minutes. A caller's wall
// time is now the deadline plus this and no more, and the manual says so.
//
// ⚠ TEN SECONDS IS A CEILING, NOT A BUDGET ANYTHING SPENDS. With StopGraceFlag
// the kill and the teardown after a timed-out job cost about 0.6s and 0.5s on
// the development host, so this is roughly nine times the measured cost and
// exists for a machine under load. A machine where it is still not enough
// leaves the container for `gc`, which is the outcome that was always available
// and is now the bounded one.
const CleanupGrace = 10 * time.Second

// cleanupBudget is ONE allowance shared by everything a timed-out job still has
// to do, rather than a fresh ceiling per step.
//
// ⚠ IT IS A TYPE RATHER THAN A PAIR OF LOCALS because the two are used in
// different scopes: the kill starts the budget and a deferred teardown spends
// the rest of it. A bare context and cancel split across those two reads to
// `go vet` as a cancel that is not called on every path, which is a warning
// worth not teaching anybody to ignore.
type cleanupBudget struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func newCleanupBudget(parent context.Context, d time.Duration) *cleanupBudget {
	ctx, cancel := context.WithTimeout(parent, d)
	return &cleanupBudget{ctx: ctx, cancel: cancel}
}

// Context is the budget, or the fallback where no deadline ever fired. A nil
// receiver is the ordinary case and answers the fallback.
func (b *cleanupBudget) Context(fallback context.Context) context.Context {
	if b == nil {
		return fallback
	}
	return b.ctx
}

func (b *cleanupBudget) Release() {
	if b != nil {
		b.cancel()
	}
}

// JobLabel is stamped on every container this executable starts, so cleanup can
// find one whose run was killed.
//
// ⛔ The ONLY thing cleanup matches on. A name prefix is a convention a person
// can reproduce by accident; a label is something this executable put there.
const JobLabel = "wsl-toolkit.owner=wsl-toolkit"

// GuestRoot is the directory under the running account's home where a job's
// files live, so two accounts cannot collide and nothing needs root.
//
// ⛔ EVERY GUEST PATH THIS FILE BUILDS IS ABSOLUTE. A relative one resolves
// against wsl.exe's default working directory, which is the caller's Windows
// directory over drvfs, so a job directory would land in somebody's checkout.
const GuestRoot = ".wsl-toolkit"

// JobSpec is one unit of isolated work.
type JobSpec struct {
	Image     string
	Script    []byte
	Workspace string // a host directory, copied in. Empty means no workspace.
	// StagedFrom is a guest directory a fleet already unpacked the workspace
	// into, copied locally instead of sent again. ⛔ It is a GUEST path and
	// never a host one: a host path here would be a mount by another name.
	StagedFrom         string
	Excludes           []string
	ArtifactDir        string // a host directory to write /out back to. Empty means none.
	Env                map[string]string
	Timeout            time.Duration
	Network            bool
	Platform           string
	ContainerLifecycle string
	Limits             WorkspaceLimits
	Label              string // what a report calls this row
	// User is what the container runs as, in podman's own spelling: a name, a
	// uid, or uid:gid. Empty means the image's own default, which is usually
	// root INSIDE the container and is not root on this machine.
	User string
	// Stdout and Stderr receive the container's bytes AS THEY ARRIVE.
	//
	// ⛔ Nil means nothing is written live, which is what a caller wants when
	// its own stdout carries a structured answer. It does not mean the output is
	// lost: the bounded copy and the transcript on disk are written either way.
	Stdout io.Writer
	Stderr io.Writer
	// MaxOutput is how many bytes of stdout the in-memory copy keeps, with
	// stderr getting a quarter of it. Zero means the defaults.
	//
	// ⚠ IT DOES NOT BOUND THE TRANSCRIPT, which is complete whatever this
	// says. It bounds the string a structured answer carries, because an answer
	// holding a gigabyte of output is a document nothing can parse.
	MaxOutput int64
	// OnTick receives a heartbeat while this job runs. Nil means none, which is
	// what a caller that did not ask for one passes.
	//
	// ⛔ IT IS AN EVENT AND NOT A RENDERING. Whatever draws it belongs to the
	// caller; this side emits a machine-readable fact with a timestamp on it.
	// WSL-50.
	OnTick func(TickEvent)
	// TickInterval overrides the default. Zero means TickInterval, and anything
	// under MinTickInterval is raised to it.
	TickEvery time.Duration
}

// JobResult is what one unit of work produced.
type JobResult struct {
	Label    string        `json:"label"`
	Image    string        `json:"image"`
	ID       string        `json:"id"`
	Exit     int           `json:"exit"`
	Duration time.Duration `json:"duration_ns"`
	Started  time.Time     `json:"started"`
	Stdout   string        `json:"stdout,omitempty"`
	Stderr   string        `json:"stderr,omitempty"`
	// Artifacts is what was DELIVERED to the directory the caller named.
	//
	// ⛔ IT USED TO BE WHAT WAS ENCOUNTERED. The extractor incremented as each
	// entry was read, so a transfer refused at its third entry answered
	// "artifacts: 3" beside the failure that stopped it, and a caller reading
	// the number believed three files had arrived. WSL-46, issue 24. The count
	// that changed meaning would have been worse than a count that gained a
	// sibling, so the sibling is what this is.
	Artifacts int `json:"artifacts"`
	// ArtifactsAttempted is what the guest offered, delivered or not.
	ArtifactsAttempted int    `json:"artifacts_attempted"`
	Error              string `json:"error,omitempty"`
	// ArtifactError is the OUTPUT TRANSFER's outcome, separate from the
	// command's. ⛔ The two were one field, and the verdict read the
	// command's exit code alone, so a job that succeeded and could not deliver
	// what it was asked for exited 0.
	ArtifactError string `json:"artifact_error,omitempty"`
	// GuestDir is set only when something is still in it worth fetching by
	// hand. An empty value means the job was torn down and there is nothing to
	// go back for.
	GuestDir string `json:"guest_dir,omitempty"`
	// RetainedKind and Retained name a copy of the output that could not be
	// delivered and was KEPT, wherever it lives.
	//
	// ⛔ THE HELPER ROUTE NAMED NOTHING. The helper deliberately does not
	// acknowledge an artifact set whose download failed, so the set is still
	// there and a caller can go back for it - and the result said only that the
	// transfer had failed, so nobody could. WSL-46, issue 24. Kind is "guest"
	// or "helper"; Retained is a guest path for the first and an artifact set
	// id for the second, and both are what `artifacts retry` takes.
	RetainedKind string `json:"retained_kind,omitempty"`
	Retained     string `json:"retained,omitempty"`
	TimedOut     bool   `json:"timed_out"`
	Unreached    bool   `json:"unreached"`
	Transcript   string `json:"transcript,omitempty"`
	// WorkspaceOmitted is how many entries the upload LEFT OUT, and
	// WorkspaceOmission names the first few with the reason.
	//
	// ⛔ AN INCOMPLETE INPUT IS A FACT A CALLER NEEDS. A Windows junction
	// pointing outside a workspace was skipped in silence and the job exited 0
	// having never seen it, so a build ran against a tree that was missing
	// something and reported on it as the real one. WSL-47, issue 26. It is
	// counted rather than refused: a junction somewhere in a large tree is a
	// normal thing to have.
	WorkspaceOmitted  int                 `json:"workspace_omitted,omitempty"`
	WorkspaceOmission []WorkspaceOmission `json:"workspace_omission,omitempty"`
	// EffectiveExit is what THIS PROCESS returns for this job, which is not
	// always the container's own code.
	//
	// ⛔ `exit` KEEPS MEANING THE CONTAINER'S CODE. A caller already reads it
	// and redefining it would break them silently, so the verdict is a second,
	// clearly named field rather than a new meaning for an old one. WSL-46,
	// issue 24: a failed transfer exited 1 while the JSON said exit 0, and
	// nothing in the object carried the 1.
	EffectiveExit int `json:"effective_exit"`
	// StdoutBytes is what the command WROTE, which is not always what Stdout
	// holds. ⛔ The pair exists because the difference used to be invisible: a
	// 9 MiB stdout came back as 8 MiB with exit 0 and no field said so.
	StdoutBytes int64 `json:"stdout_bytes"`
	StderrBytes int64 `json:"stderr_bytes"`
	// StdoutTruncated says the copy above is shorter than the command's output.
	// The complete text is under Transcript.
	StdoutTruncated bool `json:"stdout_truncated,omitempty"`
	StderrTruncated bool `json:"stderr_truncated,omitempty"`
	// Container names the stopped container that a persistent job keeps.
	Container          string `json:"container,omitempty"`
	ContainerLifecycle string `json:"container_lifecycle"`
	Platform           string `json:"platform,omitempty"`
	Cancelled          bool   `json:"cancelled,omitempty"`
}

// Failed says whether this row counts against the run: the command's own
// exit code, a deadline, or requested output that did not arrive.
//
// ⛔ ONE DEFINITION, read by the single-job verdict and by the fleet's
// counts. There were two, they read different fields, and they disagreed
// about a transfer that failed: the job exited 0 and the fleet counted a pass.
func (j JobResult) Failed() bool {
	return j.Exit != 0 || j.TimedOut || j.ArtifactError != ""
}

// The exit codes this tool answers with, and they mean four different things.
//
//	0    it ran and it agreed
//	1    it ran and it disagreed
//	2    it could not run: bad usage, a missing base, a refusal
//	124  a deadline was reached, as coreutils' timeout reports it
//
// ⛔ ONE HOME. They were named constants in main and bare literals in here,
// which is a value in two places with nothing checking that they agree.
const (
	ExitOK      = 0
	ExitFailed  = 1
	ExitCannot  = 2
	ExitTimeout = 124
)

// Verdict is the code the process returns for this job.
//
// ⛔ ONE DEFINITION, for the same reason Failed has one: the rule lived in the
// command layer, so nothing inside a result could state its own verdict and the
// structured answer could not carry it.
func (j JobResult) Verdict() int {
	switch {
	case j.Unreached:
		return ExitCannot
	case j.TimedOut:
		return ExitTimeout
	case j.Exit != 0:
		// ⭐ The container's own exit code is forwarded verbatim, which is the
		// whole point of running one. A wrapper that flattened it to 1 would
		// make every downstream test read the same. It also WINS over a failed
		// transfer: a job that exited 7 and delivered nothing exited 7, and
		// that is the more specific fact.
		return j.Exit
	case j.ArtifactError != "":
		// ⛔ The command succeeded and what it was asked to deliver did not
		// arrive. Exiting 0 here is how a green pipeline lost its build output.
		return ExitFailed
	default:
		return ExitOK
	}
}

// Seal fills in the fields derived from the rest, immediately before the result
// is written out.
//
// ⛔ IT IS CALLED AT THE POINT OF RENDERING, not at the point of production. A
// helper-run result is produced on one machine and then AMENDED on another when
// the artifact download fails, so a verdict computed where the job ran would be
// stale in exactly the case the field exists for.
func (j *JobResult) Seal() { j.EffectiveExit = j.Verdict() }

// Runner executes jobs in the owned distribution.
type Runner struct {
	cfg    Config
	base   *Base
	wsl    *Wsl
	home   string
	ledger *Ledger
	log    func(string)
	homeMu sync.Mutex
	// homeDir is the guest account's home once it has been read. ⛔ Empty
	// means "not known yet", and there is deliberately no cached error beside
	// it: see guestHome.
	homeDir string
}

// NewRunner binds a runner to this host. It does not create the base; a caller
// that needs one calls Ensure first, so "the base is missing" is a message
// rather than a surprise minutes-long build inside another command.
func NewRunner(cfg Config, log func(string)) (*Runner, error) {
	if log == nil {
		log = func(string) {}
	}
	base, err := NewBase(cfg, log)
	if err != nil {
		return nil, err
	}
	led, err := OpenLedger()
	if err != nil {
		return nil, err
	}
	// ⛔ Home, not EnsureHome. A Runner is built by `resources` and by `ready`,
	// both of which only read. WSL-55.
	home, err := Home()
	if err != nil {
		return nil, err
	}
	return &Runner{cfg: cfg, base: base, wsl: base.wsl, home: home, ledger: led, log: log}, nil
}

// guestHome asks the distribution where the account's home is, and remembers
// the answer.
//
// ⚠ Read, never assumed. /home/<user> is a convention: an image whose useradd
// defaults differ puts it elsewhere, and a path built from the convention then
// resolves to a directory nobody owns.
//
// ⛔ IT CACHES SUCCESS AND NEVER FAILURE, and that is a rule this tool holds
// everywhere rather than a fix to one function. It was a sync.Once wrapping BOTH
// the value and the error, so one lookup made while the base was absent poisoned
// the helper for its whole lifetime: the base was rebuilt, `base status --probe`
// reported healthy, and every later job still answered "There is no distribution
// with the supplied name" until somebody restarted the helper. WSL-44, issue 19.
// WSL-32 was a probe cache with the same shape, which is why this is written as
// a rule: A NEGATIVE RESULT IS NEVER CACHED ANYWHERE IN THIS TOOL.
func (r *Runner) guestHome(ctx context.Context) (string, error) {
	r.homeMu.Lock()
	defer r.homeMu.Unlock()
	if r.homeDir != "" {
		return r.homeDir, nil
	}
	out, stderr, code, err := r.wsl.Capture(ctx, r.cfg.Base.Name, r.cfg.Base.User,
		[]byte("printf '%s\\n' \"$HOME\"\n"), 2*time.Minute)
	if err != nil || code != 0 {
		return "", fmt.Errorf("could not read the guest home directory (exit %d): %s", code, firstLine(stderr+out))
	}
	h := strings.TrimSpace(firstLine(out))
	if !strings.HasPrefix(h, "/") {
		return "", fmt.Errorf("the guest reported a home directory of %q, which is not an absolute path", h)
	}
	if err := AssertArgvSafe([]string{h}); err != nil {
		return "", err
	}
	r.homeDir = h
	return r.homeDir, nil
}

// ForgetGuestHome drops the remembered answer, for the operations that can
// change it.
//
// ⚠ Caching only success is not enough on its own: a base rebuilt from another
// rootfs can put the account's home somewhere else, and a remembered value would
// then be a stale SUCCESS rather than a stale failure. Rebuilding the base
// clears it.
func (r *Runner) ForgetGuestHome() {
	r.homeMu.Lock()
	defer r.homeMu.Unlock()
	r.homeDir = ""
}

// EnsureBase is the ONE way a caller brings the base up through a Runner, so
// the remembered guest home cannot outlive the distribution it describes.
func (r *Runner) EnsureBase(ctx context.Context, force bool) (BaseState, error) {
	return r.EnsureBaseWith(ctx, force, false)
}

// EnsureBaseWith carries the repair switch through to the base. ⛔ It is a
// separate entry point rather than a changed signature because the helper
// protocol and every existing caller pass two arguments, and a third one that
// defaults to true is exactly the shape a deletion nobody asked for arrives in.
func (r *Runner) EnsureBaseWith(ctx context.Context, force, repair bool) (BaseState, error) {
	st, err := r.base.EnsureWith(ctx, force, repair)
	r.ForgetGuestHome()
	return st, err
}

// Base exposes the lifecycle, so a caller can ensure it before running.
func (r *Runner) Base() *Base { return r.base }

func newJobID() (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// ⛔ Not a timestamp and not a counter. An identifier two concurrent
		// runs can produce is two runs writing into one directory.
		return "", fmt.Errorf("no cryptographic randomness for a job id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// Run executes one job and returns what it produced.
//
// ⛔ NO HOST PATH REACHES THE CONTAINER. It gets two directories, both inside
// the distribution's own filesystem: a copy of the workspace, and an empty one
// to hand things back in.
func (r *Runner) Run(ctx context.Context, spec JobSpec) (res JobResult) {
	started := time.Now()
	trace := newJobTrace()
	res = JobResult{Label: spec.Label, Image: spec.Image, Started: started.UTC(),
		ContainerLifecycle: spec.ContainerLifecycle, Platform: spec.Platform}
	// ⛔ REGISTERED FIRST, SO IT RUNS LAST. Defers run in reverse, and the
	// teardown below is a defer too: a duration assigned before it ran was the
	// interval up to the point the container stopped, not the interval the
	// caller waited. WSL-45 measured 3.9s reported against 15.2s waited, and
	// the eleven second difference was entirely inside the teardown this now
	// runs after.
	// budget is non-nil only once a deadline has fired, and it is what makes the
	// grace ONE allowance rather than one per step. It is declared here so the
	// teardown defer below can spend what the kill starts.
	var budget *cleanupBudget
	defer func() {
		budget.Release()
		res.Duration = time.Since(started)
		if res.TimedOut || TraceEnabled() {
			r.log("timing: " + trace.Render())
		}
	}()
	if res.Label == "" {
		res.Label = spec.Image
	}
	if err := ValidateImageRef(spec.Image); err != nil {
		res.Exit, res.Error, res.Unreached = 2, err.Error(), true
		return res
	}
	if _, err := NormalizePlatform(spec.Platform); err != nil {
		res.Exit, res.Error, res.Unreached = 2, err.Error(), true
		return res
	}
	if spec.ContainerLifecycle != ContainerPersistent && spec.ContainerLifecycle != ContainerEphemeral {
		res.Exit, res.Error, res.Unreached = 2,
			fmt.Sprintf("container lifecycle %q must be %q or %q", spec.ContainerLifecycle, ContainerPersistent, ContainerEphemeral), true
		return res
	}
	id, err := newJobID()
	if err != nil {
		res.Exit, res.Error = 2, err.Error()
		return res
	}
	res.ID = id

	user := r.cfg.Base.User
	guestHome, err := r.guestHome(ctx)
	if err != nil {
		res.Exit, res.Error, res.Unreached = 2, err.Error(), true
		return res
	}
	jobsRoot := guestHome + "/" + GuestRoot + "/jobs"
	guestJob := jobsRoot + "/" + id
	guestWork := guestJob + "/work"
	guestOut := guestJob + "/out"
	guestScript := guestJob + "/job.sh"
	container := "wtk-" + id
	deadline := time.Time{}
	if spec.Timeout > 0 {
		deadline = time.Now().Add(spec.Timeout).UTC()
	}

	// ⭐ Recorded BEFORE anything exists. A record written after a successful
	// create cannot describe the create that was killed half way.
	if err := r.ledger.Append(LedgerEntry{
		Event: "open", Kind: "job", ID: id, Distro: r.cfg.Base.Name,
		Image: spec.Image, GuestDir: guestJob, HostDir: spec.ArtifactDir, Deadline: deadline,
	}); err != nil {
		res.Exit, res.Error = 2, "could not record the job before starting it: "+err.Error()
		return res
	}

	// ⛔ keepGuest IS THE ONE REASON THE JOB DIRECTORY SURVIVES. A transfer
	// that failed leaves the only copy of the output inside the guest, and a
	// teardown that ran anyway made the failure unrecoverable rather than merely
	// reported.
	keepGuest := false
	keepContainer := false
	defer func() {
		if keepGuest || keepContainer {
			note := "the job finished and its directory was kept"
			if keepGuest {
				r.log("the job directory is kept because its artifacts could not be fetched: " + guestJob)
				r.log("fetch them by hand, then: wsl-toolkit gc --apply")
				note += ": its artifacts could not be fetched"
			}
			if keepContainer {
				r.log("the stopped container and its job directory are kept: " + container)
				note += ": the container lifecycle is persistent"
			}
			// ⛔ THE RECORD STILL CLOSES. The directory is kept on purpose and
			// the job is over, and cleanup reads an open record as work in
			// flight. Leaving it open would mean gc spares this directory
			// forever, which turns a deliberate hold into a permanent leak.
			if err := r.ledger.Append(LedgerEntry{
				Event: "close", Kind: "job", ID: id,
				Note: note,
			}); err != nil {
				r.log("could not close the ledger record for job " + id + ": " + err.Error())
			}
			return
		}
		// Teardown runs whatever happened, including a cancelled context, so it
		// gets its own budget rather than inheriting a dead one.
		//
		// ⛔ EXCEPT WHERE A DEADLINE FIRED, in which case it SHARES the one the
		// kill above already started. Five minutes is the right ceiling for an
		// ordinary job whose caller is not waiting on a clock; it is the wrong
		// one for a caller who asked for two seconds, and the two ceilings added
		// together are where WSL-45's missing eleven seconds were.
		ordinary, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		err := r.teardown(budget.Context(ordinary), id, jobsRoot, guestJob)
		trace.Mark("teardown")
		if err != nil {
			r.log("cleanup after job " + id + ": " + err.Error())
			return
		}
		if err := r.ledger.Append(LedgerEntry{Event: "close", Kind: "job", ID: id}); err != nil {
			r.log("could not close the ledger record for job " + id + ": " + err.Error())
		}
	}()

	limits := spec.Limits
	if limits.MaxBytes == 0 {
		limits = DefaultWorkspaceLimits()
	}

	// The job directory, with the two mount points and nothing else in it.
	if code, err := r.wsl.ExecDirect(ctx, r.cfg.Base.Name, user, "",
		[]string{"/bin/mkdir", "-p", guestWork, guestOut}, nil, io.Discard, io.Discard, 2*time.Minute); err != nil || code != 0 {
		res.Exit, res.Error, res.Unreached = 2, fmt.Sprintf("could not create the job directory in the guest (exit %d): %v", code, err), true
		return res
	}

	switch {
	case spec.StagedFrom != "":
		// ⭐ A local copy inside the distribution, not a second trip across
		// wsl.exe. Each row still gets its OWN copy: two rows sharing a
		// directory is two rows able to change each other's result.
		if err := r.copyStaged(ctx, spec.StagedFrom, guestWork); err != nil {
			res.Exit, res.Error, res.Unreached = 2, err.Error(), true
			return res
		}
	case spec.Workspace != "":
		up, err := r.wsl.SendWorkspace(ctx, r.cfg.Base.Name, user, guestWork, spec.Workspace, limits, spec.Excludes, r.log)
		// ⛔ RECORDED BEFORE THE ERROR IS READ. A partial upload that then
		// failed still left entries out, and a caller reading the result of a
		// failed job is exactly who wants to know which ones.
		res.WorkspaceOmitted, res.WorkspaceOmission = up.Omitted, up.Omission
		if err != nil {
			res.Exit, res.Error, res.Unreached = 2, err.Error(), true
			return res
		}
	}

	// ⛔ The job script travels as a FILE. Not an argument, which wsl.exe
	// expands; not stdin, which a job that reads its own would consume.
	if err := r.sendFile(ctx, user, guestJob, "job.sh", spec.Script); err != nil {
		res.Exit, res.Error, res.Unreached = 2, err.Error(), true
		return res
	}

	token, err := newMarkerToken()
	if err != nil {
		res.Exit, res.Error, res.Unreached = 2, err.Error(), true
		return res
	}
	runScript := r.containerScript(spec, container, guestWork, guestOut, guestScript, token)

	streams := newJobStreams(r.home, id, spec.Stdout, spec.Stderr, spec.MaxOutput, r.log)
	defer streams.Close()
	marker := newMarkerStripper(streams.Err, token)

	// ⭐ THE HEARTBEAT COUNTS THE BYTES THE STREAMS CARRY, which is what makes a
	// tick the difference between work and a stall. It holds a number and never
	// a copy: the bounded buffer and the transcript already hold the bytes.
	var outCount, errCount atomic.Int64
	interval := spec.TickEvery
	if interval == 0 {
		interval = TickInterval
	}
	tick := startTicker(ctx, interval,
		TickEvent{ID: id, Label: res.Label, Container: container},
		deadline, &outCount, &errCount, spec.OnTick)
	// ⛔ STOPPED BEFORE THE RESULT IS BUILT, and Stop waits: a tick serialised
	// after the result would tell a caller reading events in order that a job
	// finished and is still running.
	defer tick.Stop()
	jobOut := &countingWriter{to: streams.Out, count: &outCount}
	jobErr := &countingWriter{to: marker, count: &errCount}
	runCtx := ctx
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	code, execErr := r.wsl.Exec(runCtx, ExecRequest{
		Distro: r.cfg.Base.Name, User: user,
		Script: append(guestRuntimePrologue(), runScript...),
		Stdout: jobOut, Stderr: jobErr,
	})
	trace.Mark("exec")
	tick.Stop()
	if err := marker.Flush(); err != nil {
		r.log("could not flush the job's error stream: " + err.Error())
	}
	streams.Close()
	trace.Mark("streams")
	streams.Apply(&res)
	res.Exit = code
	if marker.Seen() && spec.ContainerLifecycle == ContainerPersistent {
		keepContainer = true
		res.Container = container
	}

	switch {
	case runCtx.Err() != nil && ctx.Err() == nil:
		// ⭐ 124, as coreutils' timeout and -CommandTimeoutSeconds both report.
		res.TimedOut, res.Exit = true, 124
		res.Error = fmt.Sprintf("the job passed its %s deadline", spec.Timeout)
		// ⛔ ONE BUDGET FOR EVERYTHING THAT REMAINS. The kill had two minutes of
		// its own and the teardown had five, which are ceilings and not budgets:
		// a caller who asked for two seconds waited for the sum of whatever they
		// each cost. This context is shared with the teardown defer below, so
		// the caller's wall time past the deadline is bounded once.
		budget = newCleanupBudget(context.WithoutCancel(ctx), CleanupGrace)
		if keepContainer {
			r.stopContainer(budget.Context(ctx), container)
		} else {
			r.killContainer(budget.Context(ctx), container)
		}
		trace.Mark("kill")
	case ctx.Err() != nil:
		res.Cancelled, res.Exit = true, 130
		res.Error = "the job was cancelled"
		budget = newCleanupBudget(context.WithoutCancel(ctx), CleanupGrace)
		if keepContainer {
			r.stopContainer(budget.Context(ctx), container)
		} else {
			r.killContainer(budget.Context(ctx), container)
		}
		trace.Mark("cancel")
	case !marker.Seen() && (code != 0 || execErr != nil):
		// ⛔ NOTHING RAN, so this is not the payload's exit code. The engine
		// could not acquire the image, could not create the container, or could
		// not become what --user named, and every one of those has a status a
		// real payload could also return.
		res.Unreached, res.Exit = true, 2
		res.Error = "the container never started: " + engineFailure(res.Stderr, execErr)
	case execErr != nil && code == 0:
		res.Exit, res.Error = 2, execErr.Error()
	}

	if spec.ArtifactDir != "" && !res.Unreached {
		got, err := r.wsl.FetchArtifacts(ctx, r.cfg.Base.Name, user, guestOut, spec.ArtifactDir, limits, nil)
		res.Artifacts, res.ArtifactsAttempted = got.Delivered, got.Attempted
		if err != nil {
			// ⛔ A FAILED TRANSFER IS ITS OWN FIELD. Folding it into Error left
			// the verdict reading Exit alone, so a container that succeeded and
			// could not deliver its output exited 0 and a fleet counted the row
			// as a pass.
			res.ArtifactError = err.Error()
			// ⚠ AND THE GUEST DIRECTORY STAYS. It is what the output can still
			// be recovered from, and the teardown below is the half of that
			// defect nothing could undo afterwards. gc collects it later under
			// its own age policy.
			res.GuestDir = guestJob
			res.RetainedKind, res.Retained = "guest", guestJob
			keepGuest = true
		}
	}
	return res
}

// engineFailure picks the line worth reporting out of the engine's own noise.
// ⚠ podman writes PROGRESS to stderr as well as errors, so the last
// non-empty line is the one that says why; the first is usually a pull that
// started. TrimSpace also removes the carriage return a Windows-side pipe adds.
func engineFailure(stderr string, execErr error) string {
	lines := strings.Split(stderr, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(lines[i]); t != "" {
			return t
		}
	}
	if execErr != nil {
		return execErr.Error()
	}
	return "the engine gave no reason"
}

// containerScript builds the engine invocation.
//
// ⛔ Every value is POSIX-quoted and nothing is substituted into the caller's
// script, which is a file the container reads. This text never contains it.
func (r *Runner) containerScript(spec JobSpec, container, guestWork, guestOut, guestScript, token string) []byte {
	// ⚠ :Z is not SELinux theatre on a host that has none: podman ignores it
	// where there is no policy and it is required where there is one.
	//
	// ⛔ :U IS WHAT MAKES --user WORK. The mounted directories belong to the
	// guest account, which the container's user namespace maps to uid 0 inside,
	// so a container told to run as any other uid cannot write /out or read
	// /job.sh. Measured here: `--user 1000:1000` failed with
	// "can't open '/job.sh': Permission denied" until podman was asked to
	// re-own the mounts for the user it is about to become. It is applied only
	// when a user was named, because re-owning costs a walk of the tree.
	opts := ":Z"
	if spec.User != "" {
		opts = ":U,Z"
	}
	args := []string{"run"}
	if spec.ContainerLifecycle == ContainerEphemeral {
		args = append(args, "--rm")
	}
	args = append(args,
		"--name", container,
		"--label", JobLabel,
		"--label", "wsl-toolkit.image="+spec.Image,
		"--pull=missing",
		"--volume", guestWork+":/work"+opts,
		"--volume", guestOut+":/out"+opts,
		// ⚠ The read-only option joins the same comma list rather than adding a
		// second colon: podman reads `:ro:U,Z` as a directory name and refuses
		// with "incorrect volume format", which reads as a bad path.
		"--volume", guestScript+":/job.sh:ro"+strings.Replace(opts, ":", ",", 1),
		"--workdir", "/work",
		"--env", "WSL_TOOLKIT_JOB=1",
	)
	if spec.Platform != "" {
		args = append(args, "--platform", spec.Platform)
	}
	if !spec.Network {
		args = append(args, "--network", "none")
	}
	if spec.User != "" {
		args = append(args, "--user", spec.User)
	}
	envKeys := make([]string, 0, len(spec.Env))
	for k := range spec.Env {
		if isShellName(k) {
			envKeys = append(envKeys, k)
		}
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		args = append(args, "--env", k+"="+spec.Env[k])
	}
	// ⛔ THE PAYLOAD IS ENTERED THROUGH A WRAPPER THAT ANNOUNCES ITSELF.
	// Everything before this point can fail with a status a real payload could
	// also return: resolving the reference, pulling it, creating the container,
	// becoming --user, finding /bin/sh. The marker is written from INSIDE the
	// container, so its presence is the one fact that separates an image that
	// never ran from a program that exited 125.
	wrapper := "printf '\\n%s\\n' '" + token + "' >&2\nexec /bin/sh /job.sh\n"
	args = append(args, spec.Image, "/bin/sh", "-c", wrapper)

	var b strings.Builder
	b.WriteString("set -u\n")
	b.WriteString("exec podman")
	for _, a := range args {
		b.WriteString(" ")
		b.WriteString(shellQuote(a))
	}
	b.WriteString("\n")
	return []byte(b.String())
}

// sendFile puts one file into a guest directory through the archive channel,
// so it carries the same guarantees a workspace does.
func (r *Runner) sendFile(ctx context.Context, user, guestDir, name string, content []byte) error {
	pr, pw := io.Pipe()
	go func() {
		tw := tar.NewWriter(pw)
		err := tw.WriteHeader(&tar.Header{
			Name: name, Typeflag: tar.TypeReg, Mode: 0o644,
			Size: int64(len(content)), ModTime: time.Now(),
		})
		if err == nil {
			_, err = tw.Write(content)
		}
		if err == nil {
			err = tw.Close()
		}
		_ = pw.CloseWithError(err)
	}()
	errBuf := &boundedBuffer{max: 32 << 10}
	code, err := r.wsl.ExecDirect(ctx, r.cfg.Base.Name, user, "",
		[]string{"/bin/tar", "-xf", "-", "-C", guestDir}, pr, io.Discard, errBuf, 2*time.Minute)
	if err != nil || code != 0 {
		return fmt.Errorf("could not place %s in the guest (exit %d): %s", name, code, firstLine(errBuf.String()))
	}
	return nil
}

// copyStaged duplicates a staged workspace inside the guest.
func (r *Runner) copyStaged(ctx context.Context, from, to string) error {
	if err := AssertArgvSafe([]string{from, to}); err != nil {
		return err
	}
	errBuf := &boundedBuffer{max: 32 << 10}
	code, err := r.wsl.ExecDirect(ctx, r.cfg.Base.Name, r.cfg.Base.User, "",
		[]string{"/bin/cp", "-a", from + "/.", to + "/"}, nil, io.Discard, errBuf, 30*time.Minute)
	if err != nil || code != 0 {
		return fmt.Errorf("could not copy the staged workspace (exit %d): %s", code, firstLine(errBuf.String()))
	}
	return nil
}

func (r *Runner) killContainer(ctx context.Context, name string) {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	script := []byte("podman rm -f " + StopGraceFlag + " " + shellQuote(name) + " >/dev/null 2>&1 || :\n")
	if _, err := r.wsl.Exec(bounded, ExecRequest{
		Distro: r.cfg.Base.Name, User: r.cfg.Base.User,
		Script: append(guestRuntimePrologue(), script...), Timeout: 2 * time.Minute,
	}); err != nil {
		r.log("could not remove container " + name + ": " + err.Error())
	}
}

// stopContainer stops a persistent container and keeps its filesystem.
func (r *Runner) stopContainer(ctx context.Context, name string) {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	script := []byte("podman stop " + StopGraceFlag + " " + shellQuote(name) + " >/dev/null 2>&1 || :\n")
	if _, err := r.wsl.Exec(bounded, ExecRequest{
		Distro: r.cfg.Base.Name, User: r.cfg.Base.User,
		Script: append(guestRuntimePrologue(), script...), Timeout: 2 * time.Minute,
	}); err != nil {
		r.log("could not stop container " + name + ": " + err.Error())
	}
}

// teardown removes the job's guest directory and any container holding its
// name. It reads the state back rather than reporting what it attempted.
func (r *Runner) teardown(ctx context.Context, id, jobsRoot, guestJob string) error {
	script := "podman rm -f " + StopGraceFlag + " " + shellQuote("wtk-"+id) + " >/dev/null 2>&1 || :\n" +
		GuestRemoveScript(jobsRoot, guestJob)
	out, stderr, code, err := r.baseCapture(ctx, []byte(script), 5*time.Minute)
	if err != nil || code != 0 {
		return fmt.Errorf("exit %d: %s", code, firstLine(stderr+out))
	}
	if !strings.Contains(out, "removed") {
		return errors.New("the guest reported no removal")
	}
	return nil
}

func (r *Runner) baseCapture(ctx context.Context, script []byte, timeout time.Duration) (string, string, int, error) {
	out := &boundedBuffer{max: 4 << 20}
	errBuf := &boundedBuffer{max: 256 << 10}
	code, err := r.wsl.Exec(ctx, ExecRequest{
		Distro: r.cfg.Base.Name, User: r.cfg.Base.User,
		Script:  append(guestRuntimePrologue(), script...),
		Timeout: timeout, Stdout: out, Stderr: errBuf,
	})
	return out.String(), errBuf.String(), code, err
}

// WriteTranscript saves one job's streams, so a fleet of twelve does not have
// to render twelve outputs to be readable.
func (r *Runner) WriteTranscript(dir string, res *JobResult) error {
	return WriteJobTranscript(dir, res)
}

// WriteJobTranscript is the one implementation, so a row run through the helper
// and a row run directly produce the same file.
func WriteJobTranscript(dir string, res *JobResult) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	safe := strings.Map(func(r rune) rune {
		if strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.", r) {
			return r
		}
		return '-'
	}, res.Label)
	path := filepath.Join(dir, safe+".txt")
	var b strings.Builder
	fmt.Fprintf(&b, "image    %s\n", res.Image)
	fmt.Fprintf(&b, "job      %s\n", res.ID)
	fmt.Fprintf(&b, "started  %s\n", res.Started.Format(time.RFC3339))
	fmt.Fprintf(&b, "duration %s\n", res.Duration.Round(time.Millisecond))
	fmt.Fprintf(&b, "exit     %d\n", res.Exit)
	if res.Error != "" {
		fmt.Fprintf(&b, "error    %s\n", res.Error)
	}
	b.WriteString("\n-- stdout --\n")
	b.WriteString(res.Stdout)
	b.WriteString("\n-- stderr --\n")
	b.WriteString(res.Stderr)
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return err
	}
	res.Transcript = path
	return nil
}

// GuestRemoveScript is the one guest-side removal, and every path that deletes
// something inside the distribution uses it.
//
// ⛔ The guest re-checks containment rather than trusting the path it was
// handed, for the reason RemoveInside guards the host side.
//
// ⛔ IT FALLS BACK INTO THE USER NAMESPACE, and that is not belt and braces.
// A container run with --user gets its mounts re-owned to the uid it becomes,
// which lands in this account's subuid range, and the account then cannot unlink
// what is inside them: measured here as
// "rm: cannot remove '.../out/who.txt': Permission denied" after a job that ran
// as 1000:1000. `podman unshare` enters the namespace where this account IS root
// over that range, which is the only place those files can be removed from.
//
// It reads the state back and reports what is true rather than what was
// attempted.
func GuestRemoveScript(root, dir string) string {
	return fmt.Sprintf(`tk_dir=%s
tk_root=%s
case "$tk_dir" in
  "$tk_root"/*) : ;;
  *) echo "wsl-toolkit: $tk_dir is not under $tk_root and will not be removed" >&2; exit 2 ;;
esac
case "$tk_dir" in
  *..*) echo "wsl-toolkit: $tk_dir climbs out of $tk_root" >&2; exit 2 ;;
esac
rm -rf "$tk_dir" 2>/dev/null || :
if [ -e "$tk_dir" ]; then
  podman unshare rm -rf "$tk_dir" >/dev/null 2>&1 || :
fi
if [ -e "$tk_dir" ]; then echo "still-present: $tk_dir" >&2; exit 1; fi
printf 'removed\n'
`, shellQuote(dir), shellQuote(root))
}

// -- WSL-52: reaching what the tool already holds -----------------------------

// RetainedGuestDir answers where a job's kept output is, or why there is none.
//
// ⛔ IT READS THE LEDGER RATHER THAN GUESSING A PATH. A guest directory built
// from a convention would name somewhere plausible for a job that never ran, and
// the answer to "was anything retained" would then be "something is missing
// there", which is not the same fact.
func (r *Runner) RetainedGuestDir(id string) (string, string) {
	entries, err := r.ledger.All()
	if err != nil {
		return "", "the ledger could not be read: " + err.Error()
	}
	guestDir, closed, kept := "", false, false
	for _, e := range entries {
		if e.ID != id || e.Kind != "job" {
			continue
		}
		if e.Event == "open" && e.GuestDir != "" {
			guestDir = e.GuestDir
		}
		if e.Event == "close" {
			closed = true
			// ⭐ THE NOTE IS WHAT SEPARATES A KEPT DIRECTORY FROM A TORN-DOWN
			// ONE, and both close the record. Run writes that note precisely so
			// a later reader can tell them apart without asking the guest.
			kept = strings.Contains(e.Note, "could not be fetched")
		}
	}
	switch {
	case guestDir == "":
		return "", "no job with that id was recorded here"
	case closed && !kept:
		return "", "that job was torn down, so nothing was retained. Its transcript is still readable with: wsl-toolkit logs " + id
	default:
		return guestDir, ""
	}
}

// FetchRetained copies a retained guest directory to the host.
//
// ⛔ IT DOES NOT REMOVE THE GUEST COPY AFTERWARDS. `gc` collects it under its
// own age policy, and a retrieval that deleted the only remaining copy on a
// partial success would be the defect WSL-33 was filed for.
func (r *Runner) FetchRetained(ctx context.Context, guestDir, hostDir string) (ArtifactTransfer, error) {
	out := guestDir
	if !strings.HasSuffix(out, "/out") {
		out += "/out"
	}
	return r.wsl.FetchArtifacts(ctx, r.cfg.Base.Name, r.cfg.Base.User, out, hostDir, DefaultWorkspaceLimits(), r.log)
}

// ReachImage answers whether a reference is here, and optionally puts it here.
//
// ⛔ THREE ANSWERS, NOT TWO. Cached, pulled and unreachable are different facts,
// and folding "already here" into "reachable" would make a warm run on a machine
// with no network look identical to one that fetched everything.
func (r *Runner) ReachImage(ctx context.Context, ref string, pull bool) (cached, pulled bool, reason string) {
	if err := ValidateImageRef(ref); err != nil {
		return false, false, err.Error()
	}
	script := "podman image exists " + shellQuote(ref) + " && printf 'cached\n' || printf 'absent\n'\n"
	out, stderr, code, err := r.baseCapture(ctx, []byte(script), 2*time.Minute)
	if err != nil || code != 0 {
		return false, false, "the engine could not be asked: " + firstLine(stderr+out)
	}
	if strings.Contains(out, "cached") {
		return true, false, ""
	}
	if !pull {
		// ⚠ NOT CACHED IS NOT UNREACHABLE. Without --pull this reports what is
		// here and does not go to a registry to find out about the rest, because
		// a report that quietly downloaded a gigabyte is not a report.
		return false, false, ""
	}
	pullScript := "podman pull " + shellQuote(ref) + " >/dev/null\n"
	out, stderr, code, err = r.baseCapture(ctx, []byte(pullScript), 30*time.Minute)
	if err != nil || code != 0 {
		return false, false, engineFailure(stderr+out, err)
	}
	return false, true, ""
}
