// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// -- WSL-156: the paravirt device attempts, run rather than read --------------

// doorsDeviceSection is doors.sh's paravirt device section, from its banner to
// the removal of its work directory.
func doorsDeviceSection(t *testing.T) string {
	t.Helper()
	s := strings.ReplaceAll(string(doorsScript), "\r\n", "\n")
	begin := strings.Index(s, "# ----------------------------------------------------- the paravirt devices --\n")
	if begin < 0 {
		t.Fatal("doors.sh has no paravirt device section")
	}
	const last = "rm -rf \"$dev_dir\" 2>/dev/null\n"
	end := strings.Index(s[begin:], last)
	if end < 0 {
		t.Fatal("doors.sh's paravirt device section does not remove its work directory")
	}
	return s[begin : begin+end+len(last)]
}

// doorsDeviceProbeSource is the C program the section compiles in the guest.
func doorsDeviceProbeSource(t *testing.T) string {
	t.Helper()
	s := strings.ReplaceAll(string(doorsScript), "\r\n", "\n")
	const open = "<<'DEVC'\n"
	begin := strings.Index(s, open)
	if begin < 0 {
		t.Fatal("doors.sh carries no C probe")
	}
	body := s[begin+len(open):]
	end := strings.Index(body, "\nDEVC\n")
	if end < 0 {
		t.Fatal("doors.sh's C probe has no end")
	}
	return body[:end+1]
}

// deviceRow is one verdict row of the device section.
type deviceRow struct{ verdict, detail string }

// runDoorsDeviceSection runs the section under sh with a PATH of stand-ins: the
// tools it uses, and cc only when ccBody is not empty.
func runDoorsDeviceSection(t *testing.T, ccBody string, limit time.Duration) map[string]deviceRow {
	t.Helper()
	sh := posixShell(t)
	dir := t.TempDir()
	for _, tool := range []string{"mkdir", "cat", "grep", "sed", "cut", "rm", "timeout"} {
		real, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("this host has no %s to stand in for", tool)
		}
		writeStandIn(t, dir, tool, "exec "+real+" \"$@\"")
	}
	if ccBody != "" {
		writeStandIn(t, dir, "cc", ccBody)
	}
	harness := "set -u\n" +
		"say() { printf 'DOOR|%s|%s|%s\\n' \"$1\" \"$2\" \"$3\"; }\n" +
		fmt.Sprintf("TK_DOORS_MARK=%x\n", time.Now().UnixNano()) +
		doorsDeviceSection(t)
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, sh, "-c", harness)
	cmd.Env = []string{"PATH=" + dir}
	// ⚠ A probe that outlives its shell holds the pipes. The case does not wait
	// for it past this.
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("the device section did not finish within %s: %v; stdout %q stderr %q", limit, err, stdout.String(), stderr.String())
	}
	rows := map[string]deviceRow{}
	for _, line := range strings.Split(stdout.String(), "\n") {
		f := strings.SplitN(line, "|", 4)
		if len(f) == 4 && f[0] == "DOOR" {
			rows[f[1]] = deviceRow{f[2], f[3]}
		}
	}
	for _, id := range []string{"plumbing.kvm-create-vm", "plumbing.vsock-connect-host", "plumbing.dxg-ioctl"} {
		if _, ok := rows[id]; !ok {
			t.Fatalf("the device section emitted no %s row: %q", id, stdout.String())
		}
	}
	return rows
}

// TestAGuestWithNoCompilerReportsTheDevicesUnknown runs the fallback a guest with
// no compiler takes. ⛔ Nothing was tried there, so no row may read `closed`.
func TestAGuestWithNoCompilerReportsTheDevicesUnknown(t *testing.T) {
	for _, c := range []struct{ name, cc, why string }{
		{"no compiler", "", "no cc or gcc in this guest"},
		{"a compiler that cannot build", "echo 'cc: this build fails' >&2; exit 1", "cc could not build or run the probe: cc: this build fails"},
	} {
		rows := runDoorsDeviceSection(t, c.cc, 30*time.Second)
		for id, node := range map[string]string{
			"plumbing.kvm-create-vm": "/dev/kvm", "plumbing.vsock-connect-host": "", "plumbing.dxg-ioctl": "/dev/dxg",
		} {
			want := DoorUnknown
			if node != "" {
				if _, err := os.Stat(node); err != nil {
					want = DoorAbsent
				}
			}
			r := rows[id]
			if r.verdict != want {
				t.Errorf("%s: %s read %s, want %s: %s", c.name, id, r.verdict, want, r.detail)
			}
			if want == DoorUnknown && !strings.Contains(r.detail, c.why) {
				t.Errorf("%s: %s said %q, want the reason %q", c.name, id, r.detail, c.why)
			}
		}
	}
}

// TestAProbeThatNeverFinishesIsBoundedFromOutside holds the `timeout` around the
// compiled probe. A driver call that never returns kept `base doors` waiting
// with nothing to end it but the caller's own budget.
func TestAProbeThatNeverFinishesIsBoundedFromOutside(t *testing.T) {
	// The stand-in compiler writes a "probe" that never finishes.
	cc := `while [ "$#" -gt 0 ]; do if [ "$1" = -o ]; then out=$2; fi; shift; done
printf '#!/bin/sh\nexec sleep 120\n' > "$out"
chmod 755 "$out"`
	posixShell(t)
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("this host has no sleep")
	}
	chmod, err := exec.LookPath("chmod")
	if err != nil {
		t.Skip("this host has no chmod")
	}
	cc = strings.ReplaceAll(cc, "exec sleep", "exec "+sleep)
	cc = strings.ReplaceAll(cc, "chmod 755", chmod+" 755")
	started := time.Now()
	rows := runDoorsDeviceSection(t, cc, 45*time.Second)
	if waited := time.Since(started); waited > 35*time.Second {
		t.Fatalf("the device section waited %s on a probe that never finished", waited)
	}
	if r := rows["plumbing.vsock-connect-host"]; r.verdict != DoorUnknown {
		t.Errorf("a probe that was ended read %s for vsock, want unknown: %s", r.verdict, r.detail)
	}
}

// TestTheWindowsAttemptsLeaveTheProbesStdinAlone runs doors.sh's own two
// Windows launches with a stand-in that reads stdin, and with the rest of a
// script waiting there.
//
// ⛔ THE PROBE ARRIVES ON STDIN. On a base with interop on, cmd.exe read the rest
// of it, and `base doors` exited 2 saying only `exit status 1`. Measured on a
// scratch base while driving WSL-156.
func TestTheWindowsAttemptsLeaveTheProbesStdinAlone(t *testing.T) {
	sh := posixShell(t)
	s := strings.ReplaceAll(string(doorsScript), "\r\n", "\n")
	var launches []string
	for _, line := range strings.Split(s, "\n") {
		switch {
		case strings.HasPrefix(line, `stub_out=$("$stub" `):
			launches = append(launches, line)
		case strings.HasPrefix(line, `elif out=$("$winexe" `):
			launches = append(launches, "if "+strings.TrimPrefix(line, "elif ")+" :; fi")
		}
	}
	if len(launches) != 2 {
		t.Fatalf("doors.sh has %d Windows launches this case knows, want 2: %q", len(launches), launches)
	}
	dir := t.TempDir()
	reader := filepath.Join(dir, "reader")
	writeStandIn(t, dir, "reader", "while read -r line; do :; done")
	for _, launch := range launches {
		cmd := exec.Command(sh, "-c", "stub="+reader+"\nwinexe="+reader+"\n"+launch+"\nread -r left || left=\nprintf 'left=%s\\n' \"$left\"\n")
		cmd.Stdin = strings.NewReader("the rest of the probe\n")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v", launch, err)
		}
		if got := strings.TrimSpace(string(out)); got != "left=the rest of the probe" {
			t.Errorf("%s took the probe's stdin: %q", launch, got)
		}
	}
}

// doorsFakeLibc stands in for the kernel under the compiled probe: every device
// opens, ioctl answers the errno the case names, and connect blocks in a read
// that only a signal ends, or is refused at once.
//
// ⚠ NO SYSTEM HEADER DECLARES THESE HERE, because glibc's own declarations of
// connect and ioctl differ from these in form and a build would refuse them.
const doorsFakeLibc = `#include <errno.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

extern int openat(int, const char *, int, ...);
struct sockaddr;

int open(const char *path, int flags, ...) { (void)path; (void)flags; return openat(-100, "/dev/null", 0); }
int socket(int d, int t, int p) { (void)d; (void)t; (void)p; return openat(-100, "/dev/null", 0); }
int ioctl(int fd, unsigned long req, ...) {
    const char *e = getenv("TK_FAKE_IOCTL_ERRNO");
    (void)fd; (void)req;
    if (e == NULL || atoi(e) == 0) return 0;
    errno = atoi(e);
    return -1;
}
int connect(int s, const struct sockaddr *a, unsigned int n) {
    int p[2];
    char c;
    (void)s; (void)a; (void)n;
    if (getenv("TK_FAKE_CONNECT_BLOCKS") == NULL) { errno = ECONNREFUSED; return -1; }
    if (pipe(p) != 0) return -1;
    if (read(p[0], &c, 1) < 0) return -1;
    return 0;
}
`

// buildDoorsDeviceProbe compiles the C probe doors.sh carries against the fake
// kernel above, or skips where this host has no compiler.
func buildDoorsDeviceProbe(t *testing.T) string {
	t.Helper()
	posixShell(t)
	cc, err := exec.LookPath("cc")
	if err != nil {
		if cc, err = exec.LookPath("gcc"); err != nil {
			t.Skip("this host has no cc or gcc to build the probe with")
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "probe.c"), []byte(doorsDeviceProbeSource(t)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fake.c"), []byte(doorsFakeLibc), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(dir, "probe")
	if out, err := exec.Command(cc, "-O0", "-w", "-o", probe, filepath.Join(dir, "probe.c"), filepath.Join(dir, "fake.c")).CombinedOutput(); err != nil {
		t.Fatalf("the probe doors.sh carries did not build: %v: %s", err, out)
	}
	return probe
}

// runDoorsDeviceProbe runs the built probe with the fake kernel's answers.
func runDoorsDeviceProbe(t *testing.T, probe string, env ...string) map[string]deviceRow {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, probe)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("the probe with %v did not finish: %v; %q", env, err, out)
	}
	rows := map[string]deviceRow{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.SplitN(line, " ", 3)
		if len(f) == 3 {
			rows[f[0]] = deviceRow{f[1], f[2]}
		}
	}
	return rows
}

// TestTheDxgVerdictIsWhatTheDriverAnswered is the overclaim the first WSL-156
// build carried. It sent a guessed ioctl, dxgkrnl refused it with ENOTTY, and the
// row still read `open`. Only an answered adapter query is `open` now.
func TestTheDxgVerdictIsWhatTheDriverAnswered(t *testing.T) {
	probe := buildDoorsDeviceProbe(t)
	for _, c := range []struct {
		errno, want, says string
	}{
		{"0", DoorOpen, "was told 0"},
		{"25", DoorUnknown, "what the driver serves is unmeasured"}, // ENOTTY
		{"13", DoorClosed, "refused the adapter query"},             // EACCES
	} {
		r := runDoorsDeviceProbe(t, probe, "TK_FAKE_IOCTL_ERRNO="+c.errno)["dxg"]
		if r.verdict != c.want || !strings.Contains(r.detail, c.says) {
			t.Errorf("an adapter query answered errno %s read %s %q, want %s and %q", c.errno, r.verdict, r.detail, c.want, c.says)
		}
	}
}

// TestTheVsockConnectIsEndedByItsAlarm holds the alarm around the vsock connect.
// ⛔ It was installed through signal(), which glibc gives SA_RESTART, so the
// connect resumed after the alarm and the six seconds bounded nothing.
func TestTheVsockConnectIsEndedByItsAlarm(t *testing.T) {
	probe := buildDoorsDeviceProbe(t)
	started := time.Now()
	r := runDoorsDeviceProbe(t, probe, "TK_FAKE_CONNECT_BLOCKS=1")["vsock"]
	if r.verdict != DoorClosed || !strings.Contains(r.detail, "no answer within 6 seconds") {
		t.Errorf("a connect nothing answered read %s %q, want closed after 6 seconds", r.verdict, r.detail)
	}
	if waited := time.Since(started); waited > 12*time.Second {
		t.Errorf("a connect nothing answered held the probe for %s", waited)
	}
}
