package toolkit

import (
	"context"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Engine is a container engine on the HOST, used for exactly one job: turning
// an OCI image into a rootfs archive that wsl --import can read.
//
// ⚠ It is not the engine jobs run in. Jobs run in a rootless podman INSIDE the
// owned distribution, so nothing a job does can reach the machine's own engine
// or the images somebody else put in it.
type Engine struct {
	Name string // podman or docker
	Path string
	Arch string // the platform token this engine accepts, normalised
}

// FindEngine picks the host engine, preferring podman.
func FindEngine(ctx context.Context) (*Engine, error) {
	// ⛔ EVERY CANDIDATE'S REASON IS KEPT. Reporting only the last one names
	// whichever engine happened to be tried second, so a broken podman reads as
	// a missing docker and the next reader looks in the wrong place. That is
	// exactly what this function did on its first run here.
	var problems []string
	for _, name := range []string{"podman", "docker"} {
		exe, err := ResolveExecutable(name)
		if err != nil {
			problems = append(problems, name+": "+err.Error())
			continue
		}
		e := &Engine{Name: name, Path: exe.Resolved}
		arch, err := e.readArch(ctx)
		if err != nil {
			why := fmt.Sprintf("%s is installed at %s and did not answer: %v", name, exe.Resolved, err)
			// ⭐ PODMAN GETS A SECOND SENTENCE, because its own advice for this
			// state sends the reader to two commands that cannot help.
			if name == "podman" {
				if hint := DiagnosePodman(ctx, exe.Resolved, err); hint != "" {
					why += ". " + hint
				}
			}
			problems = append(problems, why)
			continue
		}
		e.Arch = arch
		return e, nil
	}
	if len(problems) == 0 {
		problems = append(problems, "neither podman nor docker is installed")
	}
	return nil, fmt.Errorf("no usable container engine on this host: %s", strings.Join(problems, "; "))
}

// readArch asks the engine what architecture it runs, and normalises the answer.
//
// ⛔ THE TWO ENGINES SPELL THE FIELD DIFFERENTLY AND ASKING THE WRONG ONE FAILS
// THE WHOLE CALL. podman answers to {{.Host.Arch}} and docker to
// {{.Architecture}}; asking podman for the lower-case spelling returns
// "can't evaluate field host", which reads as a broken engine and is a wrong
// template.
//
// ⚠ They also disagree on the VALUE. podman answers amd64 and docker answers
// x86_64, and only the first is a token --platform accepts. A value neither
// recognises is passed through rather than guessed at, so the engine refuses it
// by name instead of this code inventing one.
func (e *Engine) readArch(ctx context.Context) (string, error) {
	bounded, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	field := "{{.Host.Arch}}"
	if e.Name == "docker" {
		field = "{{.Architecture}}"
	}
	out, stderr, err := Output(bounded, e.Path, "info", "--format", field)
	if err != nil {
		return "", fmt.Errorf("%s info --format %s: %w: %s", e.Name, field, err, firstLine(stderr))
	}
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("%s info --format %s printed nothing: %s", e.Name, field, firstLine(stderr))
	}
	arch := strings.TrimSpace(firstLine(out))
	switch arch {
	case "x86_64":
		return "amd64", nil
	case "aarch64":
		return "arm64", nil
	default:
		return arch, nil
	}
}

// Platform is the --platform value every pull and create passes.
//
// ⛔ NAMED ON EVERY CALL, NEVER INHERITED. The local image store is keyed by tag
// and not by architecture, so one `pull --platform linux/riscv64 alpine`
// repoints the shared local alpine:latest at that image and every later
// unqualified pull hands it back. Imported into WSL, that rootfs registers
// cleanly and then nothing in it executes.
func (e *Engine) Platform() string { return "linux/" + e.Arch }

// ExportRootfs turns an image reference into a rootfs archive on the host.
//
// It is the one place this executable talks to the host engine, and it removes
// the container it created whether or not the export succeeded.
func (e *Engine) ExportRootfs(ctx context.Context, ref, tarPath string, log func(string)) error {
	if err := ValidateImageRef(ref); err != nil {
		return err
	}
	pullCtx, cancelPull := context.WithTimeout(ctx, 30*time.Minute)
	defer cancelPull()
	log(fmt.Sprintf("pulling %s for %s", ref, e.Platform()))
	// ⛔ NEVER EMIT NOTHING: RENDER SILENCE, WITH A TIME ON IT. The pull is
	// bounded at thirty minutes above, which is the right ceiling and the whole
	// of what this used to say. Between the line before and the pull returning,
	// a caller saw NOTHING, so a stalled pull and a slow one were the same
	// picture for up to half an hour.
	//
	// ⚠ MEASURED 2026-09-17, and it is why this exists. A
	// `podman pull ghcr.io/pkgforge-dev/archlinux:latest` on this host sat for
	// 28 minutes with ZERO bytes read, ZERO written and ZERO processor time over
	// a 25-second window; the same pull, run again minutes later, finished in
	// 221 seconds. The stall is not reproducible and is not diagnosed. The
	// silence was ours, and this is WSL-18's own rule applied where it was not.
	stopBeat := beat(pullCtx, log, "still pulling "+ref)
	out, stderr, err := Output(pullCtx, e.Path, "pull", "--platform", e.Platform(), ref)
	stopBeat()
	if err != nil {
		return fmt.Errorf("%s pull %s: %w: %s", e.Name, ref, err, firstLine(out+stderr))
	}

	createCtx, cancelCreate := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelCreate()
	out, stderr, err = Output(createCtx, e.Path, "create", "--platform", e.Platform(), ref)
	if err != nil {
		return fmt.Errorf("%s create %s: %w: %s", e.Name, ref, err, firstLine(out+stderr))
	}
	id := strings.TrimSpace(firstLine(out))
	if id == "" {
		return fmt.Errorf("%s create %s printed no container id", e.Name, ref)
	}
	defer func() {
		rmCtx, cancelRm := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancelRm()
		// The container is this function's to remove and nothing else reads it,
		// so a failure here is reported and does not change the export's
		// verdict. It is discarded explicitly rather than by omission.
		if _, _, rmErr := Output(rmCtx, e.Path, "rm", "-f", id); rmErr != nil {
			log("could not remove the export container " + id + ": " + rmErr.Error())
		}
	}()

	log("exporting the rootfs of container " + id[:min(12, len(id))])
	f, err := os.OpenFile(tarPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	exportCtx, cancelExport := context.WithTimeout(ctx, 30*time.Minute)
	defer cancelExport()
	errBuf := &boundedBuffer{max: 64 << 10}
	cmd := newCommand(exportCtx, e.Path, "export", id)
	runErr := runCommand(exportCtx, cmd, nil, f, errBuf)
	closeErr := f.Close()
	if runErr != nil {
		_ = os.Remove(tarPath)
		return fmt.Errorf("%s export %s: %w: %s", e.Name, id, runErr, firstLine(errBuf.String()))
	}
	if closeErr != nil {
		return closeErr
	}
	size, ok := FileSize(tarPath)
	if !ok || size == 0 {
		// ⛔ An export that wrote nothing and exited 0 is the "step that exits 0
		// having done nothing" pattern. wsl --import would accept the empty
		// archive and register a distribution with no userland in it.
		_ = os.Remove(tarPath)
		return fmt.Errorf("%s export wrote an empty archive for %s", e.Name, ref)
	}
	log(fmt.Sprintf("rootfs: %s", HumanBytes(size)))
	return nil
}

// PullBeatEvery is how often a long engine call says it is still there.
//
// ⚠ IT IS NOT A PROGRESS BAR AND CANNOT BECOME ONE. podman writes its progress
// to stderr and Output CAPTURES that, so nothing here knows how many bytes have
// moved. What it can say honestly is how long it has been waiting, which is the
// difference between "this is slow" and "this has stopped".
const PullBeatEvery = 30 * time.Second

// beat writes one line every PullBeatEvery until the returned function is
// called, and returns a function that is safe to call exactly once.
//
// ⭐ THE ELAPSED TIME IS THE CONTENT. A heartbeat that only said "still working"
// would be the watcher whose output is the thing it is watching, which
// docs/conventions/forbidden-patterns.md already has a row for.
func beat(ctx context.Context, log func(string), what string) func() {
	if log == nil {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	started := time.Now()
	go func() {
		defer close(done)
		t := time.NewTicker(PullBeatEvery)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case now := <-t.C:
				log(fmt.Sprintf("%s: %s so far, no output yet", what, FormatSpan(now.Sub(started))))
			}
		}
	}()
	// ⛔ THE STOP WAITS, for the reason tick.go gives: a line already in flight
	// would otherwise print after the one saying the step finished.
	var once sync.Once
	return func() {
		once.Do(func() { close(stop) })
		<-done
	}
}

// HumanBytes renders a byte count in binary units, labelled as binary.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 4; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTP"[exp])
}

// ParseHumanBytes reads a size the way an engine prints one, and it lives
// beside HumanBytes because it is its inverse and the two vocabularies have to
// agree.
//
// ⛔ THE TWO UNIT FAMILIES MEAN DIFFERENT NUMBERS AND BOTH ARE IN USE. podman's
// `{{.MemUsage}}` goes through docker's units.HumanSize, which is DECIMAL: the
// `33.44GB` measured on this host on 2026-09-17 is 33,440,000,000 bytes, which
// is 31.1 GiB on a 32 GiB machine. Reading it as binary would report a tenth
// less memory than there is, with nothing saying so. A `iB` suffix is binary,
// a bare `B` suffix is decimal, and that is the rule both tools follow.
func ParseHumanBytes(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	if err != nil || n < 0 {
		return 0, false
	}
	suffix := strings.TrimSpace(s[i:])
	binary := strings.HasSuffix(suffix, "iB")
	letter := strings.TrimSuffix(strings.TrimSuffix(suffix, "iB"), "B")
	step := float64(1000)
	if binary {
		step = 1024
	}
	mul := float64(1)
	switch strings.ToUpper(strings.TrimSpace(letter)) {
	case "":
	case "K":
		mul = step
	case "M":
		mul = step * step
	case "G":
		mul = step * step * step
	case "T":
		mul = step * step * step * step
	case "P":
		mul = step * step * step * step * step
	default:
		return 0, false
	}
	// ⛔ ROUNDED, NOT TRUNCATED. 33.44 times a billion is 33439999999.999996 in
	// binary floating point, and int64() of that is 33,439,999,999 - a byte
	// short of the figure the engine printed, every time, with nothing saying
	// so. It is docs/conventions/shell.md section 8's [int](2.65) in Go, and
	// the case is what found it.
	return int64(math.Round(n * mul)), true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// podmanSocketFailure matches a podman that started and cannot be talked to.
//
// ⚠ THE TWO SPELLINGS ARE ONE STATE. The transport error surfaces as an SSH
// channel refusal when the machine is up and the socket behind it is not
// served, and as a plain dial refusal when the forward itself is gone.
var podmanSocketFailure = regexp.MustCompile(`(?i)unable to connect to podman socket|ssh: rejected|connection refused|actively refused|cannot connect to podman`)

// DiagnosePodman explains a podman that is installed and will not answer, and
// names a recovery it has MEASURED rather than one it believes.
//
// ⛔ PODMAN'S OWN ADVICE IS WRONG FOR THIS STATE AND A CONSUMER WILL BLAME US.
// When the default connection's socket is not served, podman says to try
// `podman machine init` and `podman machine start`. The machine is already
// running, so start does nothing, and init would build a second machine. The
// reader runs both, neither helps, and the tool that relayed that advice is the
// one they came from. Measured on this host 2026-09-17: the machine reported
// Running, `podman machine ssh` worked, and `user@1000.service` had failed with
// "Failed to spawn executor: Device or resource busy" on systemd 259, so the
// ROOTLESS socket was never created while the ROOTFUL one was healthy.
//
// ⭐ IT NAMES A CONNECTION IT HAS JUST DRIVEN. podman registers a root
// connection beside the rootless one for every machine, and when the rootless
// user manager is dead the root one usually answers. Trying each and reporting
// the one that worked turns a dead end into one command.
//
// ⚠ IT RUNS ONLY ON THE FAILURE PATH and is bounded, so a healthy host pays
// nothing for it. An empty answer means nothing was recognised, and the caller
// then reports podman's own words alone rather than a guess of ours.
func DiagnosePodman(ctx context.Context, exe string, failure error) string {
	if failure == nil || !podmanSocketFailure.MatchString(failure.Error()) {
		return ""
	}
	bounded, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	running, machine := podmanMachineRunning(bounded, exe)
	return podmanHint(running, machine, podmanWorkingConnection(bounded, exe))
}

// podmanHint is what the diagnosis SAYS, with no process in it.
//
// ⛔ SEPARATED FROM THE MEASURING so the sentences can be driven on a host with
// no podman at all. They could not be before, and repo mutate said so: removing
// the whole guard left the cases green, because every one of them asserted only
// that the diagnosis stayed SILENT and none that it ever spoke.
func podmanHint(running bool, machine, conn string) string {
	var notes []string
	if running {
		notes = append(notes, fmt.Sprintf("the machine %q reports Running, so `podman machine start` will not help and "+
			"`podman machine init` would build a SECOND machine", machine))
	}
	if conn != "" {
		notes = append(notes, fmt.Sprintf("connection %q DOES answer, so the socket is fine and the default connection is "+
			"the broken half. Run: podman system connection default %s", conn, conn))
	} else {
		notes = append(notes, "no registered connection answers. Inside the machine, `systemctl is-active user@1000.service` "+
			"reports whether the user manager that serves the rootless socket came up at all; a failed one leaves no socket "+
			"to connect to and no amount of restarting the machine creates one")
	}
	return strings.Join(notes, "; ")
}

// podmanMachineRunning reports whether any machine is running, and its name.
func podmanMachineRunning(ctx context.Context, exe string) (bool, string) {
	out, _, err := Output(ctx, exe, "machine", "list", "--format", "{{.Name}} {{.Running}}")
	if err != nil {
		return false, ""
	}
	for _, ln := range strings.Split(out, "\n") {
		fields := strings.Fields(ln)
		if len(fields) == 2 && strings.EqualFold(fields[1], "true") {
			return true, strings.TrimSuffix(fields[0], "*")
		}
	}
	return false, ""
}

// podmanWorkingConnection returns the first registered connection that answers.
//
// ⛔ IT DRIVES EACH ONE rather than reading a status field. A connection listed
// as present says nothing about whether the socket behind it is served, which is
// the entire failure being diagnosed.
func podmanWorkingConnection(ctx context.Context, exe string) string {
	out, _, err := Output(ctx, exe, "system", "connection", "list", "--format", "{{.Name}}")
	if err != nil {
		return ""
	}
	for _, ln := range strings.Split(out, "\n") {
		name := strings.TrimSpace(ln)
		if name == "" || strings.EqualFold(name, "name") {
			continue
		}
		probe, cancel := context.WithTimeout(ctx, 15*time.Second)
		_, _, probeErr := Output(probe, exe, "--connection", name, "info", "--format", "{{.Host.Arch}}")
		cancel()
		if probeErr == nil {
			return name
		}
	}
	return ""
}
