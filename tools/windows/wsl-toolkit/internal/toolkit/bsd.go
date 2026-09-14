package toolkit

import (
	"archive/tar"
	"bytes"
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
// machine rather than inside it, with the WSL2 podman machine running throughout.
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
// ⚠ A BOOT IS PAID PER RUN. It is seconds rather than minutes because of how
// BsdCPU presents the processor, which carries the measurement; nothing keeps a
// guest running between runs.

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
	//
	// ⭐ THE HYPERVISOR BIT IS HIDDEN, AND THAT IS WHAT TOOK THE BOOT FROM TWO
	// MINUTES TO NINE SECONDS. WHPX shows the guest the host's own signature,
	// `Microsoft Hv`, so FreeBSD attaches its Hyper-V VMBus driver and holds root
	// mount for about 105 seconds waiting on a VMBus QEMU does not provide.
	// Measured on 2026-09-14, three boots each: login at 115.0 s, 114.8 s and 115.0
	// s as the image boots; at 7.5 s to 7.8 s on a copy with the VMBus driver
	// disabled and the signature still shown; at 8.3 s to 9.5 s with the bit hidden.
	// The bit is hidden rather than the driver disabled because disabling it means
	// writing into the shared image, and a freshly fetched one would still pay the
	// two minutes. ⚠ CLFLUSH goes with it: a guest that believes it has the hardware
	// flushes a device's memory with it, and QEMU's WHPX emulator printed
	// `Unimplemented handler` onto the console 256 times a boot. WSL-79.
	BsdCPU = "Icelake-Server-v7,-hypervisor,-clflush,-clflushopt"

	// BsdDefaultVCPUs is one because the FreeBSD image panics under concurrent
	// package work on this WHPX host. Five fresh-image runs with one processor
	// completed without a panic; the earlier two-processor matrix panicked under
	// every CPU model and memory size it tried. Measured 2026-09-14. WSL-81.
	BsdDefaultVCPUs = 1

	// BsdDefaultDiskGiB is the guest disk a run grows the image to.
	//
	// ⭐ THE OPERATOR RULED A TRUE 10 GiB ROOT, "raise it to 12/13 however much
	// necessary to provide true 10GiB", and the smallest whole number that gives it
	// is measured, not computed. The image carries a boot partition, an EFI
	// partition and 1 GiB of swap ahead of root, so the root is smaller than the
	// disk. Measured by `df -k /` in the guest against the 10,485,760 KiB the ruling
	// asks for: a 10 GiB disk left 8.7 GiB, 11 GiB left 10,110,092 KiB on a fresh
	// copy of the published image, and 12 GiB leaves 11,138,540 KiB. The published
	// image is 6.0 GiB with a 4.8 GiB root, which a toolchain install fills. ⛔ Not
	// larger than the ruling: a default nobody chose is a ceiling somebody else pays
	// for, and `--disk` exists for anything else. WSL-72.
	BsdDefaultDiskGiB = 12
)

// BsdStatus is what `bsd status` answers.
type BsdStatus struct {
	Schema      string `json:"schema"`
	Qemu        string `json:"qemu,omitempty"`
	QemuVersion string `json:"qemu_version,omitempty"`
	Whpx        bool   `json:"whpx"`
	WhpxDetail  string `json:"whpx_detail"`
	Image       string `json:"image,omitempty"`
	ImageBytes  int64  `json:"image_bytes,omitempty"`
	// DiskDefaultBytes is what the next run grows a smaller image to. The image
	// file IS the guest disk, so ImageBytes is the disk's current size.
	DiskDefaultBytes int64    `json:"disk_default_bytes"`
	Ready            bool     `json:"ready"`
	Problems         []string `json:"problems,omitempty"`
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
	st := BsdStatus{Schema: "wsl-toolkit-bsd-status/1", DiskDefaultBytes: int64(BsdDefaultDiskGiB) << 30}
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
	DiskGiB int       // the guest disk; the image grows to it and never shrinks
	Stdout  io.Writer // the guest console, as it arrives
}

// BsdResult is what one session produced.
type BsdResult struct {
	Exit     int           `json:"exit"`
	Output   string        `json:"output"`
	BootTime time.Duration `json:"boot_ns"`
	Duration time.Duration `json:"duration_ns"`
	// DiskBytes is the guest disk and RootBytes the root filesystem ON it, read
	// back in the guest. ⚠ Two numbers because growing the file is half the job:
	// UFS does not notice a larger disk until the partition and the filesystem
	// are both extended.
	DiskBytes int64  `json:"disk_bytes"`
	RootBytes int64  `json:"root_bytes"`
	Error     string `json:"error,omitempty"`
	// ShutdownPanic is a kernel panic the guest printed while it powered off,
	// after the payload had answered. ⚠ The payload's exit stands, and the next
	// boot writes a core dump into the shared image. WSL-81.
	ShutdownPanic string `json:"shutdown_panic,omitempty"`
}

// growBsdImage extends the guest disk to want bytes.
//
// ⛔ IT NEVER SHRINKS. The image is shared state that outlives the session that
// touched it, and a shorter file cuts off the filesystem inside it, so a smaller
// request is refused rather than honoured or silently ignored. An equal one is
// nothing to do. ⚠ Growing keeps every byte a previous session left.
func growBsdImage(path string, want int64) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	have := info.Size()
	if have > want {
		return have, fmt.Errorf("the guest disk is already %s and this run asks for %s. It never shrinks, because a shorter file cuts off the filesystem inside it. Pass --disk %d or larger, or run `wsl-toolkit bsd fetch --force` to start again from the published image",
			HumanBytes(have), HumanBytes(want), (have+(1<<30)-1)>>30)
	}
	if have == want {
		return have, nil
	}
	// ⚠ A file another QEMU holds open cannot be extended on Windows, and that
	// refusal is the right answer: growing a disk under a running guest is how
	// a filesystem is damaged.
	if err := os.Truncate(path, want); err != nil {
		return have, fmt.Errorf("could not grow the guest disk to %s, and it is unchanged: %w. Is another `bsd run` using it?", HumanBytes(want), err)
	}
	after, err := os.Stat(path)
	if err != nil {
		return have, err
	}
	if after.Size() != want {
		return after.Size(), fmt.Errorf("the guest disk was asked to grow to %s and is %s", HumanBytes(want), HumanBytes(after.Size()))
	}
	return after.Size(), nil
}

// bsdGrowRootScript extends the last UFS partition and its filesystem to the
// end of the disk, and prints the root filesystem's size in KiB.
//
// ⭐ FreeBSD's own tools, in the guest, on every boot. The image enables
// `growfs_enable` and it does nothing here: that rc script runs only on a first
// boot, and a shared image has had its first boot. `gpart recover` moves the
// backup GPT header to the new end, and nothing grows while it sits at the old
// one. ⛔ ONE LINE, because it is typed at the console, and a typed line that
// carries a newline is refused rather than joined.
const bsdGrowRootScript = `gpart recover vtbd0 >/dev/null 2>&1; tk_g=$(gpart show vtbd0 | awk 'NR==1 {e=$2+$3} $4=="freebsd-ufs" {i=$3; p=$1+$2} END {print i, e-p}'); set -- $tk_g; if [ -z "${1:-}" ]; then echo "no freebsd-ufs partition on vtbd0"; exit 3; fi; if [ "$2" -gt 2048 ]; then gpart resize -i "$1" vtbd0 >/dev/null || exit 3; growfs -y / >/dev/null || exit 3; fi; df -k / | awk 'NR==2 {print "tk-root-kib", $2}'`

var bsdRootKiBRE = regexp.MustCompile(`tk-root-kib ([0-9]+)`)

// ⭐ A CALLER'S SCRIPT REACHES THE GUEST AS BYTES, ON A DISK OF ITS OWN.
//
// ⛔ IT USED TO BE TYPED AT THE CONSOLE AS ONE LINE, with every newline turned into
// `; `. A `#` comment then swallowed every command after it and the run still
// exited 0, a blank line became `; ;` and a syntax error, and a compound command
// split across lines did not parse. Measured in the FreeBSD 15.1 guest:
// `echo joined-a; # a comment; echo joined-b` printed only `joined-a`. WSL-79.
//
// The script travels as a tar on a second read-only virtio disk, the guest
// extracts it to a file and runs that file, so no shell on the host parses it and
// the console's line length bounds nothing but three short lines this tool types.
const (
	// bsdPayloadMember is the one file the payload disk holds.
	bsdPayloadMember = "payload.sh"
	// bsdPayloadDevice is the payload disk inside the guest. ⚠ The second
	// virtio-blk device, because its drive follows the root disk's in the
	// argument list and the guest numbers them in that order.
	bsdPayloadDevice = "/dev/vtbd1"
	// bsdPayloadRecord is what the archive is padded to: tar's default record,
	// so a read of the device in whole records never has to be shorter.
	bsdPayloadRecord = 10240
)

// bsdPayloadPrefix names every payload disk, so a run can find the ones an earlier
// run could not remove.
const bsdPayloadPrefix = "tk-payload-"

// sweepBsdPayloads removes payload disks earlier runs left beside the image.
//
// ⚠ ONLY ONES AN HOUR OLD. Another run writes its disk before its QEMU opens it,
// and a sweep in that gap would take the file from under it. A file an older run
// still holds open cannot be removed on Windows, and that refusal is left alone.
func sweepBsdPayloads(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasPrefix(name, bsdPayloadPrefix) || !strings.HasSuffix(name, ".tar") {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) >= time.Hour {
			_ = RemoveInside(dir, filepath.Join(dir, name))
		}
	}
}

// bsdPayloadArchive is the script as a tar holding one file, byte for byte.
func bsdPayloadArchive(script []byte) ([]byte, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Name: bsdPayloadMember, Mode: 0o600, Size: int64(len(script)), Typeflag: tar.TypeReg, Format: tar.FormatUSTAR}
	if err := tw.WriteHeader(hdr); err != nil {
		return nil, err
	}
	if _, err := tw.Write(script); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if pad := (bsdPayloadRecord - buf.Len()%bsdPayloadRecord) % bsdPayloadRecord; pad > 0 {
		buf.Write(make([]byte, pad))
	}
	return buf.Bytes(), nil
}

// bsdPayloadSteps are the two lines typed for a payload that arrived on a disk:
// take the script off the disk into a file in dir, then run that file and remove
// it. name is a token this run drew, `tk` and sixteen hex digits.
//
// ⚠ THE SCRIPT'S STDIN IS /dev/null, as it is for every command this tool runs. A
// command in it that reads stdin would otherwise read the console, and wait there.
//
// ⛔ THE COPY IS REMOVED WHATEVER THE SCRIPT EXITS WITH, and the exit is still the
// script's. The image is shared and FreeBSD does not clear /tmp at boot, so a copy
// left by a script that failed would stay in every later session. A copy a killed
// run left is swept by the next extract, by its exact name shape and nothing wider.
func bsdPayloadSteps(device, dir, name string) (extract, run string) {
	file := dir + "/" + name + ".sh"
	return "rm -f " + dir + "/tk????????????????.sh; tar -xOf " + device + " " + bsdPayloadMember + " > " + file,
		"sh " + file + " </dev/null; tk_rc=$?; rm -f " + file + "; exit $tk_rc"
}

// bsdBootFailureRE is what a guest prints when it will never reach a login.
//
// ⛔ WAITING FOR `login:` ALONE CANNOT TELL A SLOW BOOT FROM A DEAD ONE. A
// damaged image stopped at the loader with `can't load 'kernel'` and sat at its
// `?` prompt, and the run spent its whole ten-minute budget before reporting
// that the guest "did not reach a login prompt", naming nothing. Measured on
// 2026-09-13. WSL-72.
//
// ⚠ A KERNEL PANIC STARTS ITS LINE, AND A REPORT OF AN OLD ONE DOES NOT. The first
// version matched `panic: ` anywhere and stopped a healthy boot at `savecore 880 -
// - reboot after panic: page fault`, which is rc noting the PREVIOUS boot's panic
// on its way to a login prompt.
var bsdBootFailureRE = regexp.MustCompile(`(?m)can't load 'kernel'|^mountroot>|^Enter full pathname of shell or RETURN|^panic: `)

// bsdBootFailure answers the console line that says the boot is over, or "".
func bsdBootFailure(console string) string {
	loc := bsdBootFailureRE.FindStringIndex(console)
	if loc == nil {
		return ""
	}
	start := strings.LastIndexAny(console[:loc[0]], "\r\n") + 1
	end := loc[1] + strings.IndexAny(console[loc[1]:], "\r\n")
	if end < loc[1] {
		end = len(console)
	}
	return strings.TrimSpace(console[start:end])
}

// bsdPromptRE is the shell prompt this image presents. ⚠ Unanchored on purpose;
// the caller matches from a POSITION instead, which is what makes "the command
// finished" mean the prompt AFTER the command rather than the one before it.
var bsdPromptRE = regexp.MustCompile(`root@[^\r\n]*# `)

// BsdRun boots the guest, runs one payload at a root shell, and powers it off.
func BsdRun(ctx context.Context, spec BsdRunSpec) (res BsdResult, err error) {
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
		spec.VCpus = BsdDefaultVCPUs
	}
	if spec.DiskGiB <= 0 {
		spec.DiskGiB = BsdDefaultDiskGiB
	}
	disk, err := growBsdImage(img, int64(spec.DiskGiB)<<30)
	if err != nil {
		return res, err
	}
	res.DiskBytes = disk
	ctx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()

	archive, err := bsdPayloadArchive(spec.Script)
	if err != nil {
		return res, err
	}
	token, err := bsdToken()
	if err != nil {
		return res, err
	}
	dir := filepath.Dir(img)
	sweepBsdPayloads(dir)
	payloadName := bsdPayloadPrefix + strings.ToLower(token) + ".tar"
	payloadPath := filepath.Join(dir, payloadName)
	if err := os.WriteFile(payloadPath, archive, 0o600); err != nil {
		return res, fmt.Errorf("writing the payload disk: %w", err)
	}
	// ⛔ THROUGH THE ONE DELETION, after QEMU has let go of the file: the guest's
	// stop is deferred below this, so it runs first. ⚠ A file that could not be
	// removed is swept by the next run rather than failing this one.
	defer func() { _ = RemoveInside(dir, payloadPath) }()

	g, err := startGuest(ctx, qemu, bsdQemuArgs(spec, payloadName), filepath.Dir(img), spec.Stdout)
	if err != nil {
		return res, err
	}
	// ⚠ THE RESULT CARRIES A PANIC AT POWEROFF, because nothing else would. The
	// payload has answered by then, so its exit stands, and the guest has already
	// synced its buffers; measured on 2026-09-14, a panic in VOP_RECLAIM after `All
	// buffers synced` on a run that exited 0. WSL-81.
	defer func() { res.ShutdownPanic = g.stopAndReadPanic() }()

	if failure, ok := g.waitBoot(ctx); !ok {
		res.Error = "the guest did not reach a login prompt within " + spec.Timeout.String()
		if failure != "" {
			res.Error = "the guest cannot boot, and stopped at: " + failure +
				". If the image is damaged, `wsl-toolkit bsd fetch --force` restores the published one"
		}
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
	if err := g.wait(ctx, bsdPromptRE); err != nil {
		res.Error = "the guest did not present a root shell"
		var gone *guestGoneError
		if errors.As(err, &gone) {
			res.Error += ": " + gone.reason
		}
		res.Duration = time.Since(started)
		return res, errors.New(res.Error)
	}

	// ⭐ THE FILESYSTEM FOLLOWS THE DISK BEFORE THE PAYLOAD RUNS, so a payload
	// never meets a grown file with the old root still inside it.
	growExit, growOut, err := g.run(ctx, bsdGrowRootScript)
	if err != nil || growExit != 0 {
		res.Error = stepError("the root filesystem did not grow to the "+HumanBytes(disk)+" disk", growExit, growOut, err)
		res.Duration = time.Since(started)
		g.graceful = err == nil
		return res, errors.New(res.Error)
	}
	if m := bsdRootKiBRE.FindStringSubmatch(growOut); m != nil {
		if kib, perr := strconv.ParseInt(m[1], 10, 64); perr == nil {
			res.RootBytes = kib << 10
		}
	}

	extract, runScript := bsdPayloadSteps(bsdPayloadDevice, "/tmp", strings.ToLower(token))
	if code, out, err := g.run(ctx, extract); err != nil || code != 0 {
		res.Error = stepError("the script did not come off its disk in the guest", code, out, err)
		res.Duration = time.Since(started)
		g.graceful = err == nil
		return res, errors.New(res.Error)
	}
	exit, out, err := g.run(ctx, runScript)
	res.Exit, res.Output, res.Duration = exit, out, time.Since(started)
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	g.graceful = true
	return res, nil
}

// bsdQemuArgs is the command line one guest session boots with.
func bsdQemuArgs(spec BsdRunSpec, payloadName string) []string {
	args := []string{
		"-accel", "whpx",
		// ⚠ No default devices: a DVD drive and a VGA card the guest never uses
		// each cost a probe, and together about 1.7 s of every boot.
		"-nodefaults",
		"-M", "q35",
		"-cpu", BsdCPU,
		"-smp", strconv.Itoa(spec.VCpus),
		"-m", strconv.Itoa(spec.MemMiB),
		// if=none plus an explicit device, so the transport is named rather
		// than left to QEMU's if= heuristics.
		"-drive", "if=none,file=" + BsdImageName + ",format=raw,id=root0",
		"-device", "virtio-blk-pci,drive=root0",
		// ⚠ AFTER the root disk, which is what makes it vtbd1 in the guest.
		"-drive", "if=none,file=" + payloadName + ",format=raw,id=payload0,readonly=on",
		"-device", "virtio-blk-pci,drive=payload0",
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
		return append(args, "-netdev", "user,id=n0,ipv6=off", "-device", "virtio-net-pci,netdev=n0")
	}
	// ⛔ NOT DECORATION. Without it QEMU attaches a DEFAULT NIC and the guest
	// takes a DHCP lease, so a report saying "network none" is false. `-nodefaults`
	// removes that NIC too, and the absence is still asserted here rather than
	// inferred from another flag.
	return append(args, "-nic", "none")
}

// guest is a running QEMU with its serial console on a pipe.
type guest struct {
	cmd      *exec.Cmd
	in       io.WriteCloser
	mu       sync.Mutex
	text     strings.Builder
	graceful bool
	// gone is closed when QEMU closes its console, which is QEMU exiting.
	gone chan struct{}
}

// guestGoneError is a wait that ended because the guest can no longer answer.
type guestGoneError struct{ reason string }

func (e *guestGoneError) Error() string { return e.reason }

// bsdKernelPanicRE is a FreeBSD kernel panic as its console prints one: the panic
// line, and the CPU that took it on the next.
//
// ⛔ NOT `panic: ` ALONE. A run's console carries the payload's own output, a
// program can start a line that way, and Go's runtime does, with a goroutine dump
// after it rather than a CPU number.
var bsdKernelPanicRE = regexp.MustCompile(`(?m)^panic: [^\r\n]*\r*\ncpuid = [0-9]+`)

var bsdFatalTrapRE = regexp.MustCompile(`(?m)^Fatal trap [0-9]+: [^\r\n]*`)

// bsdKernelPanic answers the panic a console carries, with the trap that caused it
// when the console shows one, or "".
func bsdKernelPanic(console string) string {
	loc := bsdKernelPanicRE.FindStringIndex(console)
	if loc == nil {
		return ""
	}
	line := console[loc[0]:loc[1]]
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	if traps := bsdFatalTrapRE.FindAllString(console[:loc[0]], -1); len(traps) > 0 {
		return line + ", after " + strings.TrimSpace(traps[len(traps)-1])
	}
	return line
}

// lastConsoleLine answers the last line a console printed that is not blank.
func lastConsoleLine(console string) string {
	lines := strings.FieldsFunc(console, func(r rune) bool { return r == '\r' || r == '\n' })
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
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
	g := &guest{cmd: cmd, in: in, gone: make(chan struct{})}
	go func() {
		// ⛔ THE END OF THE CONSOLE IS SAID, NOT ONLY SEEN. This loop used to return
		// at QEMU's exit and tell nobody, so every wait went on polling text that
		// would never change: a guest that panicked 65 seconds in cost its run the
		// remaining 23 minutes of a 25-minute budget. WSL-81.
		defer close(g.gone)
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

// wait pumps until a pattern matches anywhere, the guest is gone, or the context
// ends.
func (g *guest) wait(ctx context.Context, re *regexp.Regexp) error {
	return g.waitFrom(ctx, re, 0)
}

var bsdLoginRE = regexp.MustCompile(`login:`)

// waitBoot waits for a login prompt, and gives up at once on a console line
// that says there will never be one. It answers that line when it gave up.
func (g *guest) waitBoot(ctx context.Context) (string, bool) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		text := g.seen()
		if bsdLoginRE.MatchString(text) {
			return "", true
		}
		if failure := bsdBootFailure(text); failure != "" {
			return failure, false
		}
		select {
		case <-ctx.Done():
			return "", false
		case <-g.gone:
			text = g.seen()
			if bsdLoginRE.MatchString(text) {
				return "", true
			}
			if failure := bsdBootFailure(text); failure != "" {
				return failure, false
			}
			return "the guest's console closed before a login prompt, after: " + lastConsoleLine(text), false
		case <-tick.C:
		}
	}
}

// waitFrom waits for a match at or after a position in the stream. It answers nil
// on a match, a *guestGoneError when the guest can no longer answer, and the
// context's error when the budget ends.
//
// ⛔ A POSITION, NOT A COUNT OF PROMPTS. Waiting for the prompt pattern anywhere
// matches the prompt the command was TYPED at and returns immediately, so a
// command that is still running reads as finished and its output is read as
// somebody else's.
//
// ⛔ A PANICKED KERNEL ENDS THE WAIT WHEN IT PRINTS, NOT WHEN QEMU EXITS. FreeBSD
// dumps its memory and waits 15 seconds before the reboot that stops QEMU, and
// nothing typed afterwards reaches a shell. WSL-81.
func (g *guest) waitFrom(ctx context.Context, re *regexp.Regexp, from int) error {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		tail := g.after(from)
		if re.MatchString(tail) {
			return nil
		}
		if panicked := bsdKernelPanic(tail); panicked != "" {
			return &guestGoneError{reason: "the guest's kernel panicked: " + panicked}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-g.gone:
			// ⚠ One more look. What the pattern waits for can arrive in the same
			// read that ended the console.
			tail = g.after(from)
			if re.MatchString(tail) {
				return nil
			}
			if panicked := bsdKernelPanic(tail); panicked != "" {
				return &guestGoneError{reason: "the guest's kernel panicked: " + panicked}
			}
			return &guestGoneError{reason: "the guest's console closed, after: " + lastConsoleLine(g.seen())}
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
	// ⛔ A TYPED LINE CARRIES NO NEWLINE, AND ONE THAT DOES IS REFUSED. Joining the
	// lines of a script with `; ` is what let a comment swallow the commands after
	// it while the run exited 0. A caller's script travels on its own disk; what is
	// typed here is a line this tool wrote. WSL-79.
	if strings.ContainsAny(payload, "\r\n") {
		return 0, "", errors.New("a line typed at the guest console cannot carry a newline")
	}
	single := payload
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
	if err := g.waitFrom(ctx, doneRE, before); err != nil {
		var gone *guestGoneError
		if errors.As(err, &gone) {
			// ⚠ The advice is measured: a panic in the middle of `pkg install` left a
			// root filesystem the next boot would not mount without a manual check.
			return 0, cleanConsole(g.after(before)), fmt.Errorf("%s, before the command finished. "+
				"A panic while the guest writes can leave its filesystem needing a check this tool cannot run; "+
				"if the next run stops at a single-user shell, `wsl-toolkit bsd fetch --force` restores the published image", gone.reason)
		}
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

// stepError is what a guest step that did not succeed ends its run with: why the
// step could not finish when it did not, and otherwise its exit code and the first
// line it printed.
//
// ⛔ NO EXIT CODE FOR A STEP THAT NEVER FINISHED. It has none, and the zero it was
// left holding printed `(exit 0)` beside the panic that ended it, which reads as a
// step that succeeded. WSL-81.
func stepError(step string, code int, out string, err error) string {
	if err != nil {
		return step + ": " + err.Error()
	}
	detail := fmt.Sprintf("%s (exit %d)", step, code)
	if line := firstLine(out); line != "" {
		detail += ": " + line
	}
	return detail
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
			_ = g.waitFrom(ctx, bsdPoweredOffRE, 0)
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

// stopAndReadPanic stops the guest and answers a kernel panic it printed on the
// way down, or "".
func (g *guest) stopAndReadPanic() string {
	from := len(g.seen())
	g.stop()
	// ⚠ QEMU having exited is not the reader having read its last bytes.
	select {
	case <-g.gone:
	case <-time.After(2 * time.Second):
	}
	return bsdKernelPanic(g.after(from))
}

func bsdToken() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "TK" + strings.ToUpper(hex.EncodeToString(b[:])), nil
}
