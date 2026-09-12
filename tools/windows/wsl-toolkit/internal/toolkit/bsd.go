package toolkit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ⭐ A BSD USERLAND, ON THIS WINDOWS HOST'S OWN HYPERVISOR, WITH NO NESTING.
//
// ⛔ A BSD BINARY CANNOT RUN ON A LINUX KERNEL and that is not a bug to route
// around. Measured 2026-08-27: FreeBSD's own image under Linux podman exits 139,
// a SIGSEGV on its first syscall. It is NOT `Exec format error`, so binfmt_misc
// and qemu-user are both irrelevant - they solve a foreign ARCHITECTURE
// presenting LINUX syscalls, and nothing presents BSD syscalls on a Linux
// kernel. A BSD userland needs a BSD kernel, so the only question is which
// hypervisor boots it.
//
// ⭐ THE ANSWER IS THE HOST'S OWN, AND IT NEEDS NO ELEVATION. `qemu-system-x86_64
// -accel whpx` runs the guest on the Windows Hypervisor Platform, which is the
// same hypervisor WSL2 already uses, so a BSD guest sits BESIDE the podman
// machine rather than inside it. Measured on this machine: FreeBSD 15.1-RELEASE
// reaches a login prompt in 113.6 s, 117.4 s and 117.7 s over three independent
// boots, with the WSL2 podman machine running throughout.
//
// ⚠ THE RANKING IN `TODO/bsd.md` INVERTS HERE AND THE REASON IS NOT SPEED. That
// table calls a Hyper-V `.vhd` guest the low-friction option and this one the
// fallback. Measured: Hyper-V and the Host Compute System both refuse an
// unelevated caller on this host, and `WHvGetCapability` answers one. The
// "fallback" is the only one of the two that runs without an administrator.
//
// ⛔ WHAT THIS DOES NOT DO. A long-running `podman system service` inside the
// guest panics the guest KERNEL in `_umtx_op`, which is what Go's scheduler
// parks threads on. So this reaches a BSD SHELL, which is what issue 29 asked
// for, and it does not offer a BSD container endpoint.
//
// ⚠ 108 OF THE 114 SECONDS ARE DEVICE PROBING, between the kernel banner and
// mounting root. Not the loader, not rc, not the filesystem, and not the
// network: removing the NIC entirely changed the total by under four seconds.
// A boot is the cost of this feature and it is paid per run.

// BsdImage is the guest this tool boots.
//
// ⛔ PINNED BY RELEASE AND VERIFIED BY DIGEST. The BASIC-CI image is the
// smallest published FreeBSD that needs no installer: it boots to a serial
// console and its root account has an EMPTY password, which is what makes it
// provisionable and is also a door. ⭐ The door is never opened: the console is
// this process's own pipe and the guest is given no network unless a caller asks.
const (
	BsdRelease           = "15.1-RELEASE"
	BsdImageName         = "FreeBSD-15.1-RELEASE-amd64-BASIC-CI-ufs.raw"
	BsdImageURL          = "https://download.freebsd.org/releases/CI-IMAGES/15.1-RELEASE/amd64/Latest/FreeBSD-15.1-RELEASE-amd64-BASIC-CI-ufs.raw.xz"
	BsdImagePinnedSha256 = "908e735f18ba192eaf48c2703b549225c9b96e74e3784bd9278c7120b4139962"

	// BsdCPU is a NAMED model rather than `host` or `max`.
	//
	// ⚠ Published advice says this host's CPU generation wedges QEMU under WHPX
	// with a newer model. Measured on QEMU 11.1.0: five models including the two
	// that advice forbids all behaved identically and none wedged, so the
	// prediction is false HERE. A named model is still what is passed, because
	// it costs nothing and the failure it avoids is expensive.
	BsdCPU = "Icelake-Server-v7"
)

// BsdStatus is what `bsd status` answers.
type BsdStatus struct {
	Schema      string   `json:"schema"`
	Qemu        string   `json:"qemu,omitempty"`
	QemuVersion string   `json:"qemu_version,omitempty"`
	Whpx        bool     `json:"whpx"`
	WhpxDetail  string   `json:"whpx_detail"`
	Image       string   `json:"image,omitempty"`
	ImageBytes  int64    `json:"image_bytes,omitempty"`
	Ready       bool     `json:"ready"`
	Problems    []string `json:"problems,omitempty"`
}

// BsdDir is where the guest image lives.
//
// ⭐ IN THE SHARED CACHE, NOT THE INSTANCE'S OWN STATE. The image is about 6 GB
// expanded, and an agent running under `--instance two` must not pay for it a
// second time. CacheDir carries the reasoning.
func BsdDir() (string, error) {
	cache, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "bsd"), nil
}

// BsdImagePath is the raw disk this tool boots.
func BsdImagePath() (string, error) {
	dir, err := BsdDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, BsdImageName), nil
}

// FindQemu locates the emulator.
//
// ⚠ A machine-wide scoop install is not under the user's home, so both are
// looked at rather than only the one this host happens to use.
func FindQemu() (string, error) {
	if p, err := exec.LookPath("qemu-system-x86_64"); err == nil {
		return p, nil
	}
	for _, env := range []struct{ base, rest string }{
		{"USERPROFILE", `scoop\apps\qemu\current\qemu-system-x86_64.exe`},
		{"ProgramData", `scoop\apps\qemu\current\qemu-system-x86_64.exe`},
		{"ProgramFiles", `qemu\qemu-system-x86_64.exe`},
	} {
		if dir := os.Getenv(env.base); dir != "" {
			cand := filepath.Join(dir, env.rest)
			if _, err := os.Stat(cand); err == nil {
				return cand, nil
			}
		}
	}
	return "", errors.New("qemu-system-x86_64 was not found. Install it with: scoop install qemu")
}

// BsdProbe reports whether this host can boot the guest.
func BsdProbe(ctx context.Context) BsdStatus {
	st := BsdStatus{Schema: "wsl-toolkit-bsd-status/1"}
	if runtime.GOOS != "windows" {
		st.Problems = append(st.Problems, "this interface runs on a Windows host, because the accelerator it uses is the Windows Hypervisor Platform")
		return st
	}
	if qemu, err := FindQemu(); err != nil {
		st.Problems = append(st.Problems, err.Error())
	} else {
		st.Qemu = qemu
		st.QemuVersion = qemuVersion(ctx, qemu)
	}
	st.Whpx, st.WhpxDetail = whpxAvailable()
	if !st.Whpx {
		st.Problems = append(st.Problems,
			"the Windows Hypervisor Platform is not available: "+st.WhpxDetail+
				". Enable the optional feature named Windows Hypervisor Platform, then restart")
	}
	img, err := BsdImagePath()
	if err != nil {
		st.Problems = append(st.Problems, err.Error())
		st.Ready = false
		return st
	}
	if info, err := os.Stat(img); err == nil {
		st.Image, st.ImageBytes = img, info.Size()
	} else {
		st.Problems = append(st.Problems,
			"the guest image is not on this machine. Run: wsl-toolkit bsd fetch")
	}
	st.Ready = len(st.Problems) == 0
	return st
}

func qemuVersion(ctx context.Context, qemu string) string {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, qemu, "--version").Output()
	if err != nil {
		return ""
	}
	return firstLine(string(out))
}

// BsdRunSpec is one guest session.
type BsdRunSpec struct {
	Script  []byte        // the payload, run at a root shell in the guest
	Timeout time.Duration // the whole session, boot included
	Network bool          // outbound user-mode networking. Nothing is forwarded in
	MemMiB  int
	VCpus   int
	Stdout  io.Writer // the guest console, as it arrives
}

// BsdResult is what one session produced.
type BsdResult struct {
	Exit     int           `json:"exit"`
	Output   string        `json:"output"`
	BootTime time.Duration `json:"boot_ns"`
	Duration time.Duration `json:"duration_ns"`
	Error    string        `json:"error,omitempty"`
}

// bsdPromptRE is the shell prompt this image presents. ⚠ Unanchored on purpose;
// the caller matches from a POSITION instead, which is what makes "the command
// finished" mean the prompt AFTER the command rather than the one before it.
var bsdPromptRE = regexp.MustCompile(`root@[^\r\n]*# `)

// BsdRun boots the guest, runs one payload at a root shell, and powers it off.
func BsdRun(ctx context.Context, spec BsdRunSpec) (BsdResult, error) {
	var res BsdResult
	started := time.Now()
	img, err := BsdImagePath()
	if err != nil {
		return res, err
	}
	if _, err := os.Stat(img); err != nil {
		return res, fmt.Errorf("the guest image is not on this machine. Run: wsl-toolkit bsd fetch")
	}
	qemu, err := FindQemu()
	if err != nil {
		return res, err
	}
	if spec.Timeout <= 0 {
		spec.Timeout = 15 * time.Minute
	}
	if spec.MemMiB <= 0 {
		spec.MemMiB = 2048
	}
	if spec.VCpus <= 0 {
		spec.VCpus = 2
	}
	ctx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	args := []string{
		"-accel", "whpx",
		"-M", "q35",
		"-cpu", BsdCPU,
		"-smp", strconv.Itoa(spec.VCpus),
		"-m", strconv.Itoa(spec.MemMiB),
		// if=none plus an explicit device, so the transport is named rather
		// than left to QEMU's if= heuristics.
		"-drive", "if=none,file=" + BsdImageName + ",format=raw,id=root0",
		"-device", "virtio-blk-pci,drive=root0",
		"-display", "none",
		"-no-reboot",
		// ⭐ stdio, NOT mon:stdio. The monitor multiplexed onto the same pipe
		// puts its own banner into the stream this parses.
		"-serial", "stdio",
		"-rtc", "base=utc,clock=host,driftfix=slew",
	}
	if spec.Network {
		// ⛔ No hostfwd. Outbound only. This image's root has an empty password
		// and nothing should be able to reach it.
		args = append(args, "-netdev", "user,id=n0,ipv6=off", "-device", "virtio-net-pci,netdev=n0")
	} else {
		// ⛔ NOT DECORATION. Without it QEMU attaches a DEFAULT NIC and the
		// guest takes a DHCP lease, so a report saying "network none" is false.
		args = append(args, "-nic", "none")
	}

	g, err := startGuest(ctx, qemu, args, filepath.Dir(img), spec.Stdout)
	if err != nil {
		return res, err
	}
	defer g.stop()

	if !g.wait(ctx, regexp.MustCompile(`login:`)) {
		res.Error = "the guest did not reach a login prompt within " + spec.Timeout.String()
		res.Duration = time.Since(started)
		return res, errors.New(res.Error)
	}
	res.BootTime = time.Since(started)
	// ⛔ Let the tty settle. login(1) reopens and reconfigures the line, and
	// anything sent while it does is lost.
	select {
	case <-ctx.Done():
		return res, ctx.Err()
	case <-time.After(750 * time.Millisecond):
	}
	if err := g.send(ctx, "root"); err != nil {
		return res, err
	}
	if !g.wait(ctx, bsdPromptRE) {
		res.Error = "the guest did not present a root shell"
		res.Duration = time.Since(started)
		return res, errors.New(res.Error)
	}

	exit, out, err := g.run(ctx, string(spec.Script))
	res.Exit, res.Output, res.Duration = exit, out, time.Since(started)
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	g.graceful = true
	return res, nil
}

// guest is a running QEMU with its serial console on a pipe.
type guest struct {
	cmd      *exec.Cmd
	in       io.WriteCloser
	mu       sync.Mutex
	text     strings.Builder
	graceful bool
}

func startGuest(ctx context.Context, qemu string, args []string, workdir string, mirror io.Writer) (*guest, error) {
	// ⚠ The argument list is passed as a LIST. Joining it into one string and
	// letting something re-split it is how a value with a space in it arrives as
	// two arguments and QEMU dies naming an option nobody passed.
	cmd := exec.Command(qemu, args...)
	// ⚠ A native Windows binary gets a bare filename, never a path. Run from the
	// directory the image is in.
	cmd.Dir = workdir
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting the guest: %w", err)
	}
	g := &guest{cmd: cmd, in: in}
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := out.Read(buf)
			if n > 0 {
				g.mu.Lock()
				g.text.Write(buf[:n])
				g.mu.Unlock()
				if mirror != nil {
					_, _ = mirror.Write(buf[:n])
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return g, nil
}

// seen returns the console text so far.
func (g *guest) seen() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.text.String()
}

// wait pumps until a pattern matches anywhere, or the context ends.
func (g *guest) wait(ctx context.Context, re *regexp.Regexp) bool {
	return g.waitFrom(ctx, re, 0)
}

// waitFrom waits for a match at or after a position in the stream.
//
// ⛔ A POSITION, NOT A COUNT OF PROMPTS. Waiting for the prompt pattern anywhere
// matches the prompt the command was TYPED at and returns immediately, so a
// command that is still running reads as finished and its output is read as
// somebody else's.
func (g *guest) waitFrom(ctx context.Context, re *regexp.Regexp, from int) bool {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		text := g.seen()
		if from <= len(text) && re.MatchString(text[from:]) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-tick.C:
		}
	}
}

// send types one line into the guest, one character at a time.
//
// ⛔ SLOWLY, AND THE REASON IS MEASURED. A serial console is a real tty with a
// real input queue. Writing a whole line at once while login(1) or the shell is
// still setting up the line discipline silently DROPS characters: a marker of
// `TOOLKIT-READY-789f28b0` reached the shell as `TOO789f28b`, never matched, and
// read as "the guest never answered" over a guest that had answered correctly.
func (g *guest) send(ctx context.Context, line string) error {
	for _, ch := range append([]byte(line), '\n') {
		if err := ctx.Err(); err != nil {
			return err
		}
		// ⚠ The write is bounded like everything else here. Against a guest
		// that has stopped draining its console the tty queue fills, the pipe
		// behind it fills, and an unbounded write parks in the kernel where no
		// caller's timeout reaches it.
		done := make(chan error, 1)
		go func(b byte) {
			_, err := g.in.Write([]byte{b})
			done <- err
		}(ch)
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("typing into the guest console: %w", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		if ch != '\n' {
			time.Sleep(5 * time.Millisecond)
		}
	}
	return nil
}

// run executes one payload at a root shell and reads back its exit code.
//
// ⭐ THE EXIT CODE IS CARRIED OUT OF THE GUEST, not inferred from the output. A
// console gives a caller text and nothing else, so the payload is followed by a
// marker carrying `$?` and that is what the verdict reads.
//
// ⛔ TWO MARKERS, NOT ONE, AND THE ECHO IS WHY. A tty ECHOES the command it was
// given, so the echo sits in the same stream as the output. Matching it by its
// own text does not work: a console WRAPS a long line, and measured here the
// wrap landed inside a word, so no line held the whole command and the entire
// command line was reported as program output. Bracketing between a marker the
// payload prints BEFORE it runs and one it prints after removes the echo by
// POSITION, which no wrapping can defeat.
//
// ⛔ EACH MARKER IS ASSEMBLED INSIDE THE GUEST from two halves, so the echo of
// the command cannot contain either one whole. Without that the parser matches
// the command line that mentions the marker and reports a success the guest
// never had.
func (g *guest) run(ctx context.Context, payload string) (int, string, error) {
	head, err := bsdToken()
	if err != nil {
		return 0, "", err
	}
	tail, err := bsdToken()
	if err != nil {
		return 0, "", err
	}
	// One line. A payload with newlines in it is sent as a single command
	// sequence, which is what a shell already understands.
	single := strings.ReplaceAll(strings.TrimRight(payload, "\r\n"), "\r\n", "\n")
	single = strings.ReplaceAll(single, "\n", "; ")
	// ⛔ THE PAYLOAD RUNS IN A SUBSHELL, and a measured hang is why. A script
	// that ends in `exit 42` is an ordinary script, and run at the login shell
	// it exits THAT: the closing marker never prints, this waits for text the
	// guest will never send, and the session burns its whole budget before
	// reporting a timeout over a command that did exactly what it was told.
	// ⭐ Parentheses confine it, and `$?` still carries the code the subshell
	// exited with, so nothing is lost by containing it.
	line := `printf '\n%s%s\n' '` + head[:6] + `' '` + head[6:] + `'; ( ` + single +
		` ); printf '\n%s%s %s\n' '` + tail[:6] + `' '` + tail[6:] + `' "$?"`

	before := len(g.seen())
	if err := g.send(ctx, line); err != nil {
		return 0, "", err
	}
	doneRE := regexp.MustCompile(`(?m)^` + tail + ` ([0-9]+)\s*$`)
	if !g.waitFrom(ctx, doneRE, before) {
		return 0, cleanConsole(g.after(before)), errors.New("the guest did not finish the command within the budget")
	}
	chunk := g.after(before)
	m := doneRE.FindStringSubmatch(chunk)
	code, _ := strconv.Atoi(m[1])
	// ⚠ The LAST occurrence of the start marker, because the echo of the command
	// carries the two halves and a guest that printed them itself is the only
	// one that can produce them joined.
	if i := strings.LastIndex(chunk, head); i >= 0 {
		chunk = chunk[i+len(head):]
	}
	if i := strings.Index(chunk, m[0]); i >= 0 {
		chunk = chunk[:i]
	}
	return code, cleanConsole(chunk), nil
}

// after returns the console text written since a position.
func (g *guest) after(before int) string {
	text := g.seen()
	if before > len(text) {
		return ""
	}
	return text[before:]
}

// cleanConsole drops the shell prompts and the blank lines a console leaves
// around output that has already been bracketed by markers.
//
// ⚠ A BARE CARRIAGE RETURN IS A LINE BOUNDARY HERE. A tty emits one when it
// wraps, and treating it as ordinary text joins two display lines into one
// string with the break buried inside a word.
func cleanConsole(chunk string) string {
	chunk = strings.ReplaceAll(chunk, "\r\n", "\n")
	chunk = strings.ReplaceAll(chunk, "\r", "\n")
	var keep []string
	for _, raw := range strings.Split(chunk, "\n") {
		line := strings.TrimRight(bsdPromptRE.ReplaceAllString(raw, ""), " \t")
		if strings.TrimSpace(line) == "" {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}

// bsdPoweredOffRE is what the guest prints on its way down.
var bsdPoweredOffRE = regexp.MustCompile(`Uptime|rebooting|Powering off`)

// stop asks the guest to power off, then makes sure QEMU is gone.
//
// ⛔ ONE Wait, AND ONLY ONE. An earlier version waited in a goroutine, gave up
// after a timeout, and then called Wait again on the same command. `os/exec`
// answers the second call with an error, which is the harmless half; the real
// problem is two goroutines reading one process state at once. The single wait
// is started before anything else and every path here joins it.
func (g *guest) stop() {
	if g.cmd.Process == nil {
		return
	}
	done := make(chan struct{})
	go func() { _ = g.cmd.Wait(); close(done) }()

	if g.graceful {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := g.send(ctx, "poweroff"); err == nil {
			g.waitFrom(ctx, bsdPoweredOffRE, 0)
		}
		cancel()
		select {
		case <-done:
			return
		case <-time.After(45 * time.Second):
		}
	}
	_ = g.in.Close()
	_ = g.cmd.Process.Kill()
	<-done
}

func bsdToken() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "TK" + strings.ToUpper(hex.EncodeToString(b[:])), nil
}
