// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// bsdJoinShapes is issue 33's comment: a comment line, a blank line and a compound
// command split across lines, each of which the typed join broke, ending in an exit
// code that must be the script's own.
const bsdJoinShapes = "echo a\n# note\n\nif true; then\necho b\nfi\nexit 3\n"

// bsdPanicConsole is WSL-72's prove on 2026-09-14, from its last line of output to
// the reboot notice, with the lines between cut. The serial console sends CRLF.
const bsdPanicConsole = "bootstrap:   installing 6\r\n" +
	"Fatal trap 12: page fault while in kernel mode\r\n" +
	"cpuid = 1; apic id = 01\r\n" +
	"fault virtual address\t= 0x18\r\n" +
	"current process\t\t= 7 (dom0)\r\n" +
	"trap number\t\t= 12\r\n" +
	"panic: page fault\r\n" +
	"cpuid = 1\r\n" +
	"time = 1789360766\r\n" +
	"KDB: stack backtrace:\r\n" +
	"#5 0xffffffff81088f06 at pmap_ts_referenced+0x5a6\r\n" +
	"#6 0xffffffff80f46778 at vm_pageout_worker+0xb18\r\n" +
	"Uptime: 1m5s\r\n" +
	"Automatic reboot in 15 seconds - press a key on the console to abort\r\n"

// bsdNeverRE is a marker no child prints, so a wait on it ends only some other way.
var bsdNeverRE = regexp.MustCompile(`TKNEVER [0-9]+`)

// TestBsdGuestChild is the QEMU the guest cases start. It prints what a guest's
// console would, then does what the case asks.
func TestBsdGuestChild(t *testing.T) {
	switch os.Getenv("TOOLKIT_BSD_CHILD") {
	case "panic":
		// ⚠ After the typed line arrives, as a panic during a command does. Printed
		// sooner, it would sit before the position the command's wait reads from.
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		_, _ = os.Stdout.WriteString(bsdPanicConsole)
		time.Sleep(60 * time.Second)
	case "exit":
		_, _ = os.Stdout.WriteString("Starting devd.\r\n")
		os.Exit(3)
	case "poweroff-panic":
		in := bufio.NewReader(os.Stdin)
		for {
			line, err := in.ReadString('\n')
			if strings.TrimSpace(line) == "poweroff" {
				_, _ = os.Stdout.WriteString(bsdPoweroffPanicConsole)
				os.Exit(0)
			}
			if err != nil {
				os.Exit(4)
			}
		}
	}
}

// bsdPoweroffPanicConsole is a run of 2026-09-14 that exited 0, from the shutdown's
// last line to the panic, with the lines between cut.
const bsdPoweroffPanicConsole = "Syncing disks, vnodes remaining... 0 done\r\n" +
	"All buffers synced.\r\n\r\n\r\n" +
	"Fatal trap 12: page fault while in kernel mode\r\n" +
	"cpuid = 0; apic id = 00\r\n" +
	"fault virtual address\t= 0x1b8\r\n" +
	"panic: page fault\r\n" +
	"cpuid = 0\r\n" +
	"KDB: stack backtrace:\r\n" +
	"#5 0xffffffff80c6b1bc at VOP_RECLAIM_APV+0x1c\r\n" +
	"Uptime: 3m21s\r\n"

// TestAPanicWhilePoweringOffIsCarriedOnTheResult is WSL-81's second shape: a payload
// that answered, then a kernel that panicked on the way down, which the run's exit
// cannot show.
func TestAPanicWhilePoweringOffIsCarriedOnTheResult(t *testing.T) {
	g := startBsdGuestChild(t, "poweroff-panic")
	g.graceful = true
	if got := g.stopAndReadPanic(); got != "panic: page fault, after Fatal trap 12: page fault while in kernel mode" {
		t.Fatalf("a panic while powering off was answered %q", got)
	}
}

// startBsdGuestChild runs this test binary as the guest's QEMU.
func startBsdGuestChild(t *testing.T, mode string) *guest {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOOLKIT_BSD_CHILD", mode)
	g, err := startGuest(context.Background(), self, []string{"-test.run=^TestBsdGuestChild$"}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.stop)
	return g
}

// TestAKernelPanicIsToldFromAProgramThatSaysPanic holds WSL-81's shape: FreeBSD's
// panic line followed by the CPU that took it, named with the trap before it, and
// not a program's own line that starts the same way.
func TestAKernelPanicIsToldFromAProgramThatSaysPanic(t *testing.T) {
	want := "panic: page fault, after Fatal trap 12: page fault while in kernel mode"
	if got := bsdKernelPanic(bsdPanicConsole); got != want {
		t.Fatalf("the prove's console was answered %q, want %q", got, want)
	}
	goRuntime := "panic: runtime error: index out of range [3] with length 3\n\ngoroutine 1 [running]:\nmain.main()\n\t/work/main.go:5 +0x1d\nexit status 2\n"
	if got := bsdKernelPanic(goRuntime); got != "" {
		t.Fatalf("a Go program's panic in a payload's output was read as the guest's kernel: %q", got)
	}
}

// TestAGuestWhoseKernelPanicsEndsTheCommandAtOnce is WSL-81: a guest whose console
// shows a panic, from a QEMU that has not exited yet, ends the command's wait with
// the panic and the way back, rather than at the end of the budget.
func TestAGuestWhoseKernelPanicsEndsTheCommandAtOnce(t *testing.T) {
	g := startBsdGuestChild(t, "panic")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	started := time.Now()
	_, _, err := g.run(ctx, "true")
	if err == nil || !strings.Contains(err.Error(), "kernel panicked: panic: page fault, after Fatal trap 12") ||
		!strings.Contains(err.Error(), "goes with this run's overlay") {
		t.Fatalf("a panicked guest answered %v after %s", err, time.Since(started))
	}
	if waited := time.Since(started); waited > 15*time.Second {
		t.Fatalf("the panic was named after %s, which is the budget and not the panic", waited)
	}
}

// TestAStepTheGuestNeverFinishedCarriesNoExitCode is WSL-81's claim audit: a grow a
// panic ended printed `(exit 0)` beside the panic, a code the guest never sent. A
// step that did finish keeps its code and the first line it printed.
func TestAStepTheGuestNeverFinishedCarriesNoExitCode(t *testing.T) {
	g := startBsdGuestChild(t, "panic")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	code, out, err := g.run(ctx, "true")
	got := stepError("the root filesystem did not grow to the 12.0 GiB disk", code, out, err)
	if !strings.Contains(got, "12.0 GiB disk: the guest's kernel panicked: panic: page fault") || strings.Contains(got, "(exit") {
		t.Fatalf("a step a panic ended was described as %q", got)
	}
	got = stepError("the script did not come off its disk in the guest", 3, "tar: Error opening archive\r\nmore", nil)
	if got != "the script did not come off its disk in the guest (exit 3): tar: Error opening archive" {
		t.Fatalf("a step that finished and failed was described as %q", got)
	}
}

// TestAGuestWhoseConsoleClosesIsNotWaitedOn is WSL-81: QEMU exiting ends the boot's
// wait and a command's, rather than leaving both polling text that will not change.
func TestAGuestWhoseConsoleClosesIsNotWaitedOn(t *testing.T) {
	g := startBsdGuestChild(t, "exit")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	started := time.Now()
	if failure, ok := g.waitBoot(ctx); ok || !strings.Contains(failure, "console closed before a login prompt, after: Starting devd.") {
		t.Fatalf("a boot whose console closed answered (%q, %v) after %s", failure, ok, time.Since(started))
	}
	err := g.waitFrom(ctx, bsdNeverRE, 0)
	var gone *guestGoneError
	if !errors.As(err, &gone) || !strings.Contains(gone.reason, "console closed, after: Starting devd.") {
		t.Fatalf("a command's wait on a closed console answered %v after %s", err, time.Since(started))
	}
	if waited := time.Since(started); waited > 15*time.Second {
		t.Fatalf("a closed console was noticed after %s, which is the budget", waited)
	}
}

// TestTheGuestBootsWithoutTheHyperVWait holds the measured CPU profile: WSL-81's
// one-processor default; WSL-79's hidden hypervisor bit, which keeps FreeBSD's
// VMBus driver from holding root mount for about 105 seconds; no default devices;
// and the payload disk after the root disk, which makes it the guest's vtbd1.
func TestTheGuestBootsWithoutTheHyperVWait(t *testing.T) {
	args := bsdQemuArgs(BsdRunSpec{VCpus: BsdDefaultVCPUs, MemMiB: 2048}, "tk-overlay-x.qcow2", "tk-payload-x.tar")
	value := func(flag string) string {
		for i := 0; i+1 < len(args); i++ {
			if args[i] == flag {
				return args[i+1]
			}
		}
		return ""
	}
	cpu := value("-cpu")
	if BsdDefaultVCPUs != 1 || value("-smp") != "1" {
		t.Errorf("the panic-rate profile uses %d processors and emits -smp %q", BsdDefaultVCPUs, value("-smp"))
	}
	for _, feature := range []string{"-hypervisor", "-clflush", "-clflushopt"} {
		if !strings.Contains(","+cpu+",", ","+feature+",") {
			t.Errorf("the CPU model %q does not carry %s", cpu, feature)
		}
	}
	joined := " " + strings.Join(args, " ") + " "
	if !strings.Contains(joined, " -nodefaults ") {
		t.Errorf("the guest boots with QEMU's default devices: %s", joined)
	}
	root, payload := strings.Index(joined, "id=root0"), strings.Index(joined, "id=payload0")
	if root < 0 || payload < 0 || payload < root {
		t.Errorf("the payload disk is not after the root disk, so it is not vtbd1: %s", joined)
	}
	if value("-nic") != "none" {
		t.Errorf("a guest given no network carries no explicit -nic none: %s", joined)
	}
}

// TestABsdScriptTravelsAsBytesAndComesBackWhole is WSL-79: the payload disk holds
// the script byte for byte, as the one member of a tar padded to whole records.
func TestABsdScriptTravelsAsBytesAndComesBackWhole(t *testing.T) {
	archive, err := bsdPayloadArchive([]byte(bsdJoinShapes))
	if err != nil {
		t.Fatal(err)
	}
	if len(archive)%bsdPayloadRecord != 0 {
		t.Fatalf("the payload disk is %d bytes, not whole records of %d", len(archive), bsdPayloadRecord)
	}
	tr := tar.NewReader(bytes.NewReader(archive))
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Name != bsdPayloadMember || string(body) != bsdJoinShapes {
		t.Fatalf("the disk holds %q = %q, not the script", hdr.Name, body)
	}
	if _, err := tr.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("the disk holds more than the script: %v", err)
	}
}

// TestALineTypedAtTheGuestConsoleRefusesANewline holds the join shut: a line with
// a newline in it is refused before anything is typed, rather than joined with `; `.
func TestALineTypedAtTheGuestConsoleRefusesANewline(t *testing.T) {
	g := &guest{}
	if _, _, err := g.run(context.Background(), "echo a\necho b"); err == nil {
		t.Fatal("a typed line carrying a newline was accepted, which is the join that let a comment end a script")
	}
}

// TestTheJoinShapesRunWholeThroughTheGuestSteps runs issue 33's three shapes through
// the same lines the guest is sent, in a real POSIX shell against the same archive.
//
// ⚠ SKIPPED WHERE THERE IS NO sh AND tar TO RUN THEM, and on Windows, whose shells
// would be reading a Windows path. The ubuntu CI job has both.
func TestTheJoinShapesRunWholeThroughTheGuestSteps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the guest steps are POSIX shell and this host's shells read Windows paths")
	}
	for _, tool := range []string{"sh", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not on PATH", tool)
		}
	}
	dir := t.TempDir()
	archive, err := bsdPayloadArchive([]byte(bsdJoinShapes))
	if err != nil {
		t.Fatal(err)
	}
	disk := filepath.Join(dir, "payload.tar")
	if err := os.WriteFile(disk, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	// A copy an ended run left behind, which the extract sweeps.
	stale := filepath.Join(dir, "tkffffffffffffffff.sh")
	if err := os.WriteFile(stale, []byte("exit 9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	name := "tk0123456789abcdef"
	extract, run := bsdPayloadSteps(disk, dir, name)
	if out, err := exec.Command("sh", "-c", extract).CombinedOutput(); err != nil {
		t.Fatalf("the script did not come off its disk: %v: %s", err, out)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a copy an ended run left was not swept: %v", err)
	}
	// ⚠ In a subshell, as the guest runs it, so the code read is the script's own.
	out, err := exec.Command("sh", "-c", "( "+run+" )").Output()
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() != 3 {
		t.Fatalf("the script's exit was %v, not its own 3", err)
	}
	if string(out) != "a\nb\n" {
		t.Fatalf("stdout = %q: a line after the comment, the blank line or the split compound did not run", out)
	}
	if _, err := os.Stat(filepath.Join(dir, name+".sh")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a script that exited 3 left its copy behind: %v", err)
	}
}

// TestTheBsdGuestDiskIsNeverSmallerThanTheImage holds the three answers a run's disk
// can get, and that none of them writes the image. WSL-72's disk is the size a run
// asks for, an equal one is that size, and a smaller one is refused because it cuts
// off the filesystem inside the image. WSL-83: the overlay carries the size, so the
// image keeps the size it has.
func TestTheBsdGuestDiskIsNeverSmallerThanTheImage(t *testing.T) {
	img := filepath.Join(t.TempDir(), "guest.raw")
	if err := os.WriteFile(img, make([]byte, 8192), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := bsdGuestDisk(img, 16384); err != nil || got != 16384 {
		t.Fatalf("a larger disk answered (%d, %v)", got, err)
	}
	if got, err := bsdGuestDisk(img, 8192); err != nil || got != 8192 {
		t.Fatalf("an equal disk answered (%d, %v)", got, err)
	}
	got, err := bsdGuestDisk(img, 4096)
	if err == nil || got != 8192 || !strings.Contains(err.Error(), "cuts off the filesystem") {
		t.Fatalf("a smaller disk answered (%d, %v), want a refusal that says why", got, err)
	}
	if info, _ := os.Stat(img); info.Size() != 8192 {
		t.Fatalf("the image is %d bytes after three answers, and no answer may write it", info.Size())
	}
}

// TestARunWritesToAnOverlayAndNeverTheImage is WSL-83's ruling: the guest's root
// disk is a qcow2 overlay whose backing file is the image, so QEMU opens the image
// read-only and a panic cannot change it. The image's name reaches QEMU only through
// the overlay.
func TestARunWritesToAnOverlayAndNeverTheImage(t *testing.T) {
	const overlay, payload = "tk-overlay-0123456789abcdef.qcow2", "tk-payload-0123456789abcdef.tar"
	joined := " " + strings.Join(bsdQemuArgs(BsdRunSpec{VCpus: 1, MemMiB: 2048}, overlay, payload), " ") + " "
	if !strings.Contains(joined, " if=none,file="+overlay+",format=qcow2,id=root0 ") {
		t.Errorf("the root disk is not the run's overlay: %s", joined)
	}
	if strings.Contains(joined, BsdImageName) {
		t.Errorf("QEMU is handed the image itself, which it would open for writing: %s", joined)
	}
	got := strings.Join(bsdOverlayArgs(BsdImageName, overlay, 12<<30), " ")
	want := "create -q -f qcow2 -b " + BsdImageName + " -F raw " + overlay + " 12884901888"
	if got != want {
		t.Errorf("qemu-img makes the overlay with %q, want %q", got, want)
	}
}

// TestTheFilesARunLeftAreSweptByTheirShape holds the sweep: a payload disk and an
// overlay an hour old go, one younger stays because another run may be about to
// open it, and a file of any other shape stays whatever its age.
func TestTheFilesARunLeftAreSweptByTheirShape(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-2 * time.Hour)
	files := map[string]bool{
		"tk-payload-0123456789abcdef.tar":   false,
		"tk-overlay-0123456789abcdef.qcow2": false,
		"tk-overlay-fedcba9876543210.qcow2": true,
		BsdImageName:                        true,
		BsdImageName + ".xz":                true,
		"tk-overlay-notes.txt":              true,
	}
	for name := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if name != "tk-overlay-fedcba9876543210.qcow2" {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	sweepBsdRunFiles(dir)
	for name, kept := range files {
		_, err := os.Stat(filepath.Join(dir, name))
		if kept && err != nil {
			t.Errorf("%s was swept: %v", name, err)
		}
		if !kept && !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s, a run's file two hours old, was left: %v", name, err)
		}
	}
}

// TestARootNotProperlyDismountedIsNamed is WSL-83's second run: its boot printed the
// line over a root the previous poweroff's panic left, and the run ran its payload on
// the unchecked filesystem saying nothing.
func TestARootNotProperlyDismountedIsNamed(t *testing.T) {
	boot := "Trying to mount root from ufs:/dev/gpt/rootfs [rw]...\r\n" +
		"WARNING: / was not properly dismounted\r\n" +
		"Starting file system checks:\r\n"
	if got := bsdRootNotDismounted(boot); got != "WARNING: / was not properly dismounted" {
		t.Errorf("the boot's line was answered %q", got)
	}
	if got := bsdRootNotDismounted("Starting file system checks:\r\n/dev/gpt/rootfs: FILE SYSTEM CLEAN; SKIPPING CHECKS\r\n"); got != "" {
		t.Errorf("a clean boot was answered %q", got)
	}
}

// TestQemuImgIsFoundBesideTheEmulator holds the order FindQemuImg looks in: the
// emulator's own directory before PATH, because the two ship together.
func TestQemuImgIsFoundBesideTheEmulator(t *testing.T) {
	dir := t.TempDir()
	name := "qemu-img"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	emulator := filepath.Join(dir, "qemu-system-x86_64")
	beside := filepath.Join(dir, name)
	for _, path := range []string{emulator, beside} {
		if err := os.WriteFile(path, []byte("x"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", t.TempDir())
	if got, err := FindQemuImg(emulator); err != nil || got != beside {
		t.Errorf("FindQemuImg answered (%q, %v), want the one beside the emulator, %q", got, err, beside)
	}
	if got, err := FindQemuImg(filepath.Join(t.TempDir(), "qemu-system-x86_64")); err == nil {
		t.Errorf("with none beside the emulator and none on PATH it answered %q", got)
	}
}

// TestABootThatCannotReachLoginIsNamed is the case for a damaged image that sat
// at its loader prompt for a whole ten-minute budget while the run waited for a
// `login:` that was never coming, then reported nothing about why. WSL-72.
func TestABootThatCannotReachLoginIsNamed(t *testing.T) {
	dead := "/Loading /boot/loader.conf.local\r\n-c\\Loading kernel...\r\n|/-\\|/-\\|/Failed to load kernel 'kernel'\r\n-can't load 'kernel'\r\nType '?' for a list of commands, 'help' for more detailed help.\r\nOK "
	if got := bsdBootFailure(dead); got != "-can't load 'kernel'" {
		t.Fatalf("a loader that cannot load the kernel was answered %q", got)
	}
	for _, console := range []string{
		"mountroot> ",
		"Enter full pathname of shell or RETURN for /bin/sh: ",
		"panic: ffs_valloc: dup alloc\n",
	} {
		if bsdBootFailure(console) == "" {
			t.Errorf("a boot that stopped at %q was not recognised as over", console)
		}
	}
	healthy := "Timecounters tick every 10.000 msec\r\nStarting devd.\r\nFreeBSD/amd64 (freebsd) (ttyu0)\r\n\r\nlogin: "
	if got := bsdBootFailure(healthy); got != "" {
		t.Fatalf("a healthy boot was reported as over at %q", got)
	}
	// ⛔ The line that stopped a healthy boot, measured on 2026-09-13: rc reporting
	// the PREVIOUS boot's panic while this one carries on towards a login prompt.
	recovering := "Starting syslogd.\r\nsavecore 880 - - reboot after panic: page fault\r\nSep 13 04:00:21 freebsd savecore[880]: reboot after panic: page fault\r\nsavecore 880 - - writing core to /var/crash/vmcore.0\r\n"
	if got := bsdBootFailure(recovering); got != "" {
		t.Fatalf("a boot reporting an earlier panic was reported as over at %q", got)
	}
}
