// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// bsdJoinShapes is issue 33's comment: a comment line, a blank line and a compound
// command split across lines, each of which the typed join broke, ending in an exit
// code that must be the script's own.
const bsdJoinShapes = "echo a\n# note\n\nif true; then\necho b\nfi\nexit 3\n"

// TestTheGuestBootsWithoutTheHyperVWait holds the three things WSL-79 measured into
// the command line: the hypervisor bit hidden, which is what keeps FreeBSD's VMBus
// driver from holding root mount for about 105 seconds; no default devices; and the
// payload disk after the root disk, which is what makes it the guest's vtbd1.
func TestTheGuestBootsWithoutTheHyperVWait(t *testing.T) {
	args := bsdQemuArgs(BsdRunSpec{VCpus: 2, MemMiB: 2048}, "tk-payload-x.tar")
	value := func(flag string) string {
		for i := 0; i+1 < len(args); i++ {
			if args[i] == flag {
				return args[i+1]
			}
		}
		return ""
	}
	cpu := value("-cpu")
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

// TestTheBsdGuestDiskGrowsAndNeverShrinks holds the three answers a shared image
// can get. WSL-72: the guest disk grows to what a run asks for, an equal request
// is nothing to do, and a smaller one is refused with the file untouched,
// because a shorter file cuts off the filesystem inside it.
func TestTheBsdGuestDiskGrowsAndNeverShrinks(t *testing.T) {
	img := filepath.Join(t.TempDir(), "guest.raw")
	if err := os.WriteFile(img, make([]byte, 4096), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := growBsdImage(img, 8192)
	if err != nil || got != 8192 {
		t.Fatalf("growing to 8192 bytes answered (%d, %v)", got, err)
	}
	if info, _ := os.Stat(img); info.Size() != 8192 {
		t.Fatalf("the file is %d bytes after growing to 8192", info.Size())
	}

	if got, err := growBsdImage(img, 8192); err != nil || got != 8192 {
		t.Fatalf("an equal request answered (%d, %v); it is nothing to do", got, err)
	}

	got, err = growBsdImage(img, 4096)
	if err == nil {
		t.Fatal("a smaller disk was accepted, which cuts off the filesystem inside the image")
	}
	if !strings.Contains(err.Error(), "never shrinks") {
		t.Fatalf("the refusal does not say why: %v", err)
	}
	if info, _ := os.Stat(img); info.Size() != 8192 || got != 8192 {
		t.Fatalf("a refused shrink changed the file to %d bytes (answered %d)", info.Size(), got)
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
