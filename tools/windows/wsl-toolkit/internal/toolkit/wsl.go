package toolkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// ProtectedDistros must never be unregistered, whatever this executable is
// asked. Destroying one takes the machine's container engine down with it.
//
// ⚠ The list is not the guard. AssertOwnedDistro refuses everything that is not
// the base by exact name, so a name absent here is already refused. This exists
// so a plausible name's refusal says WHY.
var ProtectedDistros = []string{
	"podman-machine-default",
	"docker-desktop",
	"docker-desktop-data",
	"rancher-desktop",
	"rancher-desktop-data",
}

// ErrWslDenied is returned when wsl.exe is present and this process may not
// talk to it. ⛔ A different fact from wsl.exe being absent: one is a sandbox,
// the other is a missing feature, and they need different next moves.
var ErrWslDenied = errors.New("this process is not permitted to call wsl.exe")

// ErrWslMissing is returned when there is no wsl.exe to call at all.
var ErrWslMissing = errors.New("wsl.exe was not found on this host")

// deniedMarkers are what a blocked wsl.exe says. ⚠ Matched on the child's own
// output rather than on an exit code, because the codes differ by what denied
// it.
var deniedMarkers = []string{
	"e_accessdenied",
	"access is denied",
	"0x80070005",
	"operation not permitted",
	"the requested operation requires elevation",
}

// Wsl is the handle to this host's wsl.exe.
type Wsl struct {
	Path string
}

// FindWsl resolves wsl.exe once. Every question goes through the returned
// handle, so there is one place that knows how to talk to it and one place a
// denial is recognised.
func FindWsl() (*Wsl, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("%w: this host is %s", ErrWslMissing, runtime.GOOS)
	}
	exe, err := ResolveExecutable("wsl.exe")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWslMissing, err)
	}
	return &Wsl{Path: exe.Resolved}, nil
}

// probeOnce holds the access answer for the life of the process.
//
// ⚠ A fleet asks this once, not once per row. The probe costs one wsl.exe
// invocation, and a twelve-row matrix paying for twelve of them would be a
// second's latency bought for an answer that cannot change mid-run.
var probeOnce struct {
	sync.Once
	err error
}

// ProbeWsl answers whether THIS PROCESS may talk to wsl.exe, by talking to it.
//
// ⛔ FindWsl RESOLVES A PATH AND NOTHING ELSE. It answers "there is a
// wsl.exe" and was being read as "this process can use it", so a sandboxed
// caller with a live helper took the direct route, met
// Wsl/EnumerateDistros/Service/E_ACCESSDENIED, and was then advised to start the
// helper that was already running and answering. The routing decision has to
// make the call whose refusal it is trying to detect.
//
// ⭐ The probe is READ ONLY and creates nothing. --list --quiet enumerates,
// which is the exact call a sandbox refuses, so the probe fails in the same
// place the work would.
//
// ⚠ An empty machine is not a denial. `wsl --list --quiet` exits nonzero
// with "no installed distributions" where WSL works perfectly, and a probe that
// read any failure as a refusal would route every fresh host to a helper that is
// not there.
func ProbeWsl(ctx context.Context) error {
	probeOnce.Do(func() { probeOnce.err = probeWsl(ctx) })
	return probeOnce.err
}

func probeWsl(ctx context.Context) error {
	w, err := FindWsl()
	if err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, stderr, err := Output(bounded, w.Path, "--list", "--quiet")
	if err == nil {
		return nil
	}
	if classified := classifyWslFailure(out, stderr, err); errors.Is(classified, ErrWslDenied) {
		return classified
	}
	// ⛔ ANYTHING ELSE IS NOT AN ANSWER THIS MAY ACT ON. A timeout, an empty
	// machine, a broken install: none of them says this process is refused, and
	// routing on a guess would send work to a helper for reasons the helper does
	// not fix. The direct path runs and reports its own refusal, which is the
	// behaviour that was correct before this probe existed.
	return nil
}

func classifyWslFailure(out, stderr string, err error) error {
	joined := strings.ToLower(out + " " + stderr)
	for _, marker := range deniedMarkers {
		if strings.Contains(joined, marker) {
			return fmt.Errorf("%w: %s", ErrWslDenied, strings.TrimSpace(firstLine(out+stderr)))
		}
	}
	return err
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}

// Distro is one registered WSL distribution.
type Distro struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Owned   bool   `json:"owned"`
}

// List enumerates registered distributions.
//
// ⭐ --list --quiet, never --verbose. The verbose form prints a LOCALISED header
// and state word, so a parser keyed to "NAME" or "Running" answers differently
// on a machine whose Windows is not in English. --running --quiet is the same
// list filtered, so neither fact carries a language.
func (w *Wsl) List(ctx context.Context, baseName string) ([]Distro, error) {
	names, err := w.listNames(ctx)
	if err != nil {
		return nil, err
	}
	runningNames, err := w.listRunningNames(ctx)
	if err != nil {
		return nil, err
	}
	running := map[string]bool{}
	for _, n := range runningNames {
		running[strings.ToLower(n)] = true
	}
	out := make([]Distro, 0, len(names))
	for _, n := range names {
		out = append(out, Distro{
			Name:    n,
			Running: running[strings.ToLower(n)],
			Owned:   strings.EqualFold(n, baseName),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (w *Wsl) listNames(ctx context.Context) ([]string, error) {
	return w.nameQuery(ctx, "--list", "--quiet")
}

func (w *Wsl) listRunningNames(ctx context.Context) ([]string, error) {
	return w.nameQuery(ctx, "--list", "--running", "--quiet")
}

func (w *Wsl) nameQuery(ctx context.Context, args ...string) ([]string, error) {
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, stderr, err := Output(bounded, w.Path, args...)
	if err != nil {
		// ⚠ "no installed distributions" is an ANSWER, not a failure: an empty
		// machine and a machine this process cannot ask are different facts.
		if strings.Contains(strings.ToLower(out+stderr), "no installed distributions") {
			return nil, nil
		}
		return nil, classifyWslFailure(out, stderr, err)
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(strings.Trim(line, "\r\x00"))
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// Exists reports whether a distribution is registered under this exact name.
func (w *Wsl) Exists(ctx context.Context, name string) (bool, error) {
	names, err := w.listNames(ctx)
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if strings.EqualFold(n, name) {
			return true, nil
		}
	}
	return false, nil
}

// AssertOwnedDistro is the single choke point in front of every destructive WSL
// call this executable can make, and it lives in identity.go with the rest of
// what ownership means. AssertOwnedDistro is the structural half; Unregister
// below is the one call that also demands the guest's own proof.

// Import registers a distribution from a rootfs archive.
func (w *Wsl) Import(ctx context.Context, name, dir, tarball string) error {
	if err := AssertOwnedDistro(name); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	out, stderr, err := Output(bounded, w.Path, "--import", name, dir, tarball, "--version", "2")
	if err != nil {
		return fmt.Errorf("wsl --import failed: %w: %s", classifyWslFailure(out, stderr, err), firstLine(out+stderr))
	}
	return nil
}

// Terminate stops a distribution. It is not destructive: the disk stays.
//
// ⛔ There is no `wsl --shutdown` anywhere in this executable. It is
// machine-wide and takes every distribution down, including somebody else's.
func (w *Wsl) Terminate(ctx context.Context, name string) error {
	if err := AssertOwnedDistro(name); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out, stderr, err := Output(bounded, w.Path, "--terminate", name)
	if err != nil {
		return fmt.Errorf("wsl --terminate %s: %w", name, classifyWslFailure(out, stderr, err))
	}
	return nil
}

// Unregister removes a distribution and its disk.
//
// ⛔ THIS IS THE IRREVERSIBLE ONE, so it is the one that asks the GUEST rather
// than trusting the name. The name rule stops a typo naming somebody else's
// distribution; the marker stops a distribution this tool did not build being
// adopted by editing a configuration file. WSL-42, issue 16.
//
// ⚠ `wantIdentity` is false for exactly one caller: a base whose provisioning
// failed so early that no marker was written is still this tool's mess to clear
// up, and refusing to remove it would leave a distribution nothing can touch.
// Base.Remove passes true whenever the base was ever usable.
func (w *Wsl) Unregister(ctx context.Context, name string, wantIdentity bool) error {
	if err := AssertOwnedDistro(name); err != nil {
		return err
	}
	if wantIdentity {
		if _, err := w.ReadIdentity(ctx, name); err != nil {
			return fmt.Errorf("refusing to unregister %s: %w", name, err)
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	out, stderr, err := Output(bounded, w.Path, "--unregister", name)
	if err != nil {
		return fmt.Errorf("wsl --unregister %s: %w", name, classifyWslFailure(out, stderr, err))
	}
	return nil
}

// ExecRequest is one command sent into a distribution.
type ExecRequest struct {
	Distro  string
	User    string
	Script  []byte
	Dir     string // working directory inside the guest, optional
	Env     map[string]string
	Timeout time.Duration
	Stdout  io.Writer
	Stderr  io.Writer
}

// Exec runs a shell script inside a distribution and returns its exit code.
//
// ⛔ THE SCRIPT TRAVELS ON STDIN AND NEVER AS AN ARGUMENT. Measured 2026-09-09
// against a real Alpine distro with one payload carrying a dollar sign, a
// backtick, both quotes, a tab and a parenthesis: on stdin every character
// arrived and exit 7 propagated; as an argument to /bin/sh -lc the dollar name
// expanded to nothing, the backtick was EXECUTED, and the command reported
// exit 0 over that failure. docs/conventions/shell.md section 7 carries the same
// measurement from PowerShell.
//
// ⚠ A script that reads its own stdin consumes the rest of itself. A caller's
// job is delivered as a FILE instead, which leaves stdin free.
func (w *Wsl) Exec(ctx context.Context, req ExecRequest) (int, error) {
	if strings.TrimSpace(req.Distro) == "" {
		return 2, errors.New("Exec needs a distribution name")
	}
	user := req.User
	if user == "" {
		user = "root"
	}
	args := []string{"-d", req.Distro, "-u", user}
	if req.Dir != "" {
		args = append(args, "--cd", req.Dir)
	}
	args = append(args, "--", "/bin/sh")

	bounded := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		bounded, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	script := req.Script
	if len(req.Env) > 0 {
		script = append(shellAssignments(req.Env), script...)
	}

	cmd := newCommand(bounded, w.Path, args...)
	stdout := req.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := req.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	err := runCommand(bounded, cmd, strings.NewReader(string(script)), stdout, stderr)
	if err == nil {
		return 0, nil
	}
	return ExitCode(err), err
}

// Capture runs a script and returns its streams, for a question this tool asks
// for itself rather than a caller's job.
func (w *Wsl) Capture(ctx context.Context, distro, user string, script []byte, timeout time.Duration) (string, string, int, error) {
	out := &boundedBuffer{max: 4 << 20}
	errBuf := &boundedBuffer{max: 256 << 10}
	code, err := w.Exec(ctx, ExecRequest{
		Distro: distro, User: user, Script: script, Timeout: timeout,
		Stdout: out, Stderr: errBuf,
	})
	if err != nil {
		err = classifyWslFailure(out.String(), errBuf.String(), err)
	}
	return out.String(), errBuf.String(), code, err
}

// shellAssignments renders environment as POSIX assignments the guest evaluates
// before the caller's script.
//
// ⛔ Values are single-quoted, never substituted into the caller's script. A
// text replacement into somebody's script is the defect -ScriptArg removes one
// layer down.
func shellAssignments(env map[string]string) []byte {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		if !isShellName(k) {
			continue
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(shellQuote(env[k]))
		b.WriteString("; export ")
		b.WriteString(k)
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// ValidEnvName is the rule the container invocation applies to an environment
// name, exported so a caller can be REFUSED at the point they typed it.
//
// ⛔ The check used to live only at the point of use, where the loop building
// `--env` arguments dropped a name it did not like and said nothing. A job then
// ran without a variable the caller passed, and the only way to find out was to
// read the container's own environment.
func ValidEnvName(s string) bool { return isShellName(s) }

func isShellName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// WindowsPathToGuest converts a Windows path to the drvfs path a distribution
// sees. ⛔ Reporting only: nothing running in a container is given a host path.
func WindowsPathToGuest(p string) (string, error) {
	// ⛔ A WINDOWS PATH IS PARSED AS ONE, WHATEVER HOST IS ASKING.
	// filepath.Abs answers in the RUNNING host's grammar, so on Linux it read
	// a drive-rooted path as a relative name and prepended a working directory.
	// The tool runs on Windows and its suite runs on both, which is the second
	// host every check here earns; a function whose answer depends on where it
	// is asked cannot be checked there. Found by the ubuntu CI job, 2026-09-09.
	abs := p
	if !isWindowsAbsolute(abs) {
		resolved, err := filepath.Abs(p)
		if err != nil {
			return "", err
		}
		abs = resolved
	}
	if !isWindowsAbsolute(abs) {
		return "", fmt.Errorf("%q has no drive letter, so it has no /mnt path", p)
	}
	drive := strings.ToLower(abs[:1])
	rest := strings.ReplaceAll(abs[2:], `\`, "/")
	return "/mnt/" + drive + rest, nil
}

// isWindowsAbsolute reports whether a string is a drive-rooted Windows path,
// by its own grammar rather than the running host's.
func isWindowsAbsolute(p string) bool {
	if len(p) < 3 || p[1] != ':' {
		return false
	}
	if p[2] != '\\' && p[2] != '/' {
		return false
	}
	c := p[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// FileSize is the size of a file, and whether it could be read. ⚠ A total that
// counts an unreadable file as zero is a number somebody acts on, so callers
// name what they could not measure and withhold the total.
func FileSize(path string) (int64, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return st.Size(), true
}
