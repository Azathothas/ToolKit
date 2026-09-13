// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
