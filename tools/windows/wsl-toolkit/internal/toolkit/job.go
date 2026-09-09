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
	"time"
)

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
	StagedFrom  string
	Excludes    []string
	ArtifactDir string // a host directory to write /out back to. Empty means none.
	Env         map[string]string
	Timeout     time.Duration
	Network     bool
	Limits      WorkspaceLimits
	Label       string // what a report calls this row
	// User is what the container runs as, in podman's own spelling: a name, a
	// uid, or uid:gid. Empty means the image's own default, which is usually
	// root INSIDE the container and is not root on this machine.
	User string
}

// JobResult is what one unit of work produced.
type JobResult struct {
	Label      string        `json:"label"`
	Image      string        `json:"image"`
	ID         string        `json:"id"`
	Exit       int           `json:"exit"`
	Duration   time.Duration `json:"duration_ns"`
	Started    time.Time     `json:"started"`
	Stdout     string        `json:"stdout,omitempty"`
	Stderr     string        `json:"stderr,omitempty"`
	Artifacts  int           `json:"artifacts"`
	Error      string        `json:"error,omitempty"`
	TimedOut   bool          `json:"timed_out"`
	Unreached  bool          `json:"unreached"`
	Transcript string        `json:"transcript,omitempty"`
}

// Runner executes jobs in the owned distribution.
type Runner struct {
	cfg      Config
	base     *Base
	wsl      *Wsl
	home     string
	ledger   *Ledger
	log      func(string)
	homeOnce sync.Once
	homeDir  string
	homeErr  error
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
	home, err := EnsureHome()
	if err != nil {
		return nil, err
	}
	return &Runner{cfg: cfg, base: base, wsl: base.wsl, home: home, ledger: led, log: log}, nil
}

// guestHome asks the distribution where the account's home is, once.
//
// ⚠ Read, never assumed. /home/<user> is a convention: an image whose useradd
// defaults differ puts it elsewhere, and a path built from the convention then
// resolves to a directory nobody owns.
func (r *Runner) guestHome(ctx context.Context) (string, error) {
	r.homeOnce.Do(func() {
		out, stderr, code, err := r.wsl.Capture(ctx, r.cfg.Base.Name, r.cfg.Base.User,
			[]byte("printf '%s\\n' \"$HOME\"\n"), 2*time.Minute)
		if err != nil || code != 0 {
			r.homeErr = fmt.Errorf("could not read the guest home directory (exit %d): %s", code, firstLine(stderr+out))
			return
		}
		h := strings.TrimSpace(firstLine(out))
		if !strings.HasPrefix(h, "/") {
			r.homeErr = fmt.Errorf("the guest reported a home directory of %q, which is not an absolute path", h)
			return
		}
		if err := AssertArgvSafe([]string{h}); err != nil {
			r.homeErr = err
			return
		}
		r.homeDir = h
	})
	return r.homeDir, r.homeErr
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
func (r *Runner) Run(ctx context.Context, spec JobSpec) JobResult {
	started := time.Now()
	res := JobResult{Label: spec.Label, Image: spec.Image, Started: started.UTC()}
	if res.Label == "" {
		res.Label = spec.Image
	}
	if err := ValidateImageRef(spec.Image); err != nil {
		res.Exit, res.Error, res.Unreached = 2, err.Error(), true
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

	defer func() {
		// Teardown runs whatever happened, including a cancelled context, so it
		// gets its own budget rather than inheriting a dead one.
		tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
		defer cancel()
		if err := r.teardown(tctx, id, jobsRoot, guestJob); err != nil {
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
		if _, _, err := r.wsl.SendWorkspace(ctx, r.cfg.Base.Name, user, guestWork, spec.Workspace, limits, spec.Excludes, r.log); err != nil {
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

	container := "wtk-" + id
	runScript := r.containerScript(spec, container, guestWork, guestOut, guestScript)

	out := &boundedBuffer{max: 8 << 20}
	errBuf := &boundedBuffer{max: 2 << 20}
	runCtx := ctx
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	code, execErr := r.wsl.Exec(runCtx, ExecRequest{
		Distro: r.cfg.Base.Name, User: user,
		Script: append(guestRuntimePrologue(), runScript...),
		Stdout: out, Stderr: errBuf,
	})
	res.Duration = time.Since(started)
	res.Stdout, res.Stderr = out.String(), errBuf.String()
	res.Exit = code

	if runCtx.Err() != nil && ctx.Err() == nil {
		// ⭐ 124, as coreutils' timeout and -CommandTimeoutSeconds both report.
		res.TimedOut, res.Exit = true, 124
		res.Error = fmt.Sprintf("the job passed its %s deadline", spec.Timeout)
		r.killContainer(context.WithoutCancel(ctx), container)
	} else if execErr != nil && code == 0 {
		res.Exit, res.Error = 2, execErr.Error()
	}

	if spec.ArtifactDir != "" {
		n, _, err := r.wsl.FetchArtifacts(ctx, r.cfg.Base.Name, user, guestOut, spec.ArtifactDir, limits, nil)
		res.Artifacts = n
		if err != nil && res.Error == "" {
			res.Error = "artifacts: " + err.Error()
		}
	}
	return res
}

// containerScript builds the engine invocation.
//
// ⛔ Every value is POSIX-quoted and nothing is substituted into the caller's
// script, which is a file the container reads. This text never contains it.
func (r *Runner) containerScript(spec JobSpec, container, guestWork, guestOut, guestScript string) []byte {
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
	args := []string{
		"run", "--rm", "--name", container,
		"--label", JobLabel,
		"--label", "wsl-toolkit.image=" + spec.Image,
		"--pull=missing",
		"--volume", guestWork + ":/work" + opts,
		"--volume", guestOut + ":/out" + opts,
		// ⚠ The read-only option joins the same comma list rather than adding a
		// second colon: podman reads `:ro:U,Z` as a directory name and refuses
		// with "incorrect volume format", which reads as a bad path.
		"--volume", guestScript + ":/job.sh:ro" + strings.Replace(opts, ":", ",", 1),
		"--workdir", "/work",
		"--env", "WSL_TOOLKIT_JOB=1",
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
	args = append(args, spec.Image, "/bin/sh", "/job.sh")

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
	script := []byte("podman rm -f " + shellQuote(name) + " >/dev/null 2>&1 || :\n")
	if _, err := r.wsl.Exec(bounded, ExecRequest{
		Distro: r.cfg.Base.Name, User: r.cfg.Base.User,
		Script: append(guestRuntimePrologue(), script...), Timeout: 2 * time.Minute,
	}); err != nil {
		r.log("could not remove container " + name + ": " + err.Error())
	}
}

// teardown removes the job's guest directory and any container holding its
// name. It reads the state back rather than reporting what it attempted.
func (r *Runner) teardown(ctx context.Context, id, jobsRoot, guestJob string) error {
	script := "podman rm -f " + shellQuote("wtk-"+id) + " >/dev/null 2>&1 || :\n" +
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
