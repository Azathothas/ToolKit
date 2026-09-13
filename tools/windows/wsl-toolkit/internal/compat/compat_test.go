// SPDX-License-Identifier: 0BSD

package compat

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// e2e runs the interface exactly as a caller does: one argument list, one
// exit code, the report stream and the notes stream captured apart.
type e2e struct {
	code   int
	report string
	notes  string
}

func runCompat(t *testing.T, args ...string) e2e {
	t.Helper()
	// ⛔ THE STATE DIRECTORY IS ISOLATED: LOCALAPPDATA is what resolves it when
	// no -StateDir was passed, so every case here pins it to a directory the
	// test owns. A case that needs state in place first pins the SAME
	// directory through runCompatIn.
	return runCompatIn(t, t.TempDir(), args...)
}

func runCompatIn(t *testing.T, stateDir string, args ...string) e2e {
	t.Helper()
	t.Setenv("LOCALAPPDATA", stateDir)
	t.Setenv("WSL_TOOLKIT_STATE_DIR", "")
	t.Cleanup(func() {
		resolveWsl = findWsl
		lookupWSLAdapter = scanWSLAdapter
	})
	var report, notes bytes.Buffer
	code := Run(context.Background(), args, &report, &notes, strings.NewReader(""))
	return e2e{code: code, report: report.String(), notes: notes.String()}
}

func TestARefusalIsOneErrorLineAndExitOne(t *testing.T) {
	got := runCompat(t, "-Action", "List", "-Image", "alpine:3.22")
	if got.code != 1 {
		t.Fatalf("exit %d, want the tool's own 1", got.code)
	}
	if !strings.Contains(got.notes, "-Image is read by -Action New and not by -Action List") {
		t.Errorf("the refusal does not name the parameter and who reads it: %q", got.notes)
	}
	if got.report != "" {
		t.Errorf("a refusal put something on the report stream: %q", got.report)
	}
}

func TestNoStateDirectoryIsRefusedBeforeAnythingRuns(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("WSL_TOOLKIT_STATE_DIR", "")
	var report, notes bytes.Buffer
	code := Run(context.Background(), []string{"-Action", "Doctor"}, &report, &notes, nil)
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(notes.String(), "Pass -StateDir or set LOCALAPPDATA") {
		t.Errorf("the refusal does not say how to proceed: %q", notes.String())
	}
}

// TestADeadParameterUnderNoTimestampsIsRefused holds the refusal that keeps a
// rendering flag from silently doing nothing: the stream log is OFF, so
// -TickSeconds would be a flag the caller typed that nothing read.
func TestADeadParameterUnderNoTimestampsIsRefused(t *testing.T) {
	got := runCompat(t, "-Action", "Run", "-Name", "eph-x-1a2b", "-Command", "true",
		"-NoTimestamps", "-TickSeconds", "5")
	if got.code != 1 {
		t.Fatalf("exit %d, want 1", got.code)
	}
	if !strings.Contains(got.notes, "-NoTimestamps turns the stream log off, so -TickSeconds would do nothing") {
		t.Errorf("the refusal does not name both halves: %q", got.notes)
	}
	// And under the other spelling of the same decision.
	got = runCompat(t, "-Action", "Run", "-Name", "eph-x-1a2b", "-Command", "true",
		"-TimestampProfile", "raw", "-EventLog", "x.jsonl")
	if got.code != 1 || !strings.Contains(got.notes, "-TimestampProfile raw turns the stream log off") {
		t.Fatalf("raw profile dead flags: exit %d, %q", got.code, got.notes)
	}
}

func TestASinkPathIsRefusedBeforeADryRunReportsAPlan(t *testing.T) {
	// ⛔ A DRY RUN THAT DISAGREES WITH THE RUN IT DESCRIBES IS WORSE THAN NO
	// DRY RUN: the sink refusal moved ahead of the actions for exactly this.
	got := runCompat(t, "-Action", "Run", "-Name", "eph-x-1a2b", "-Command", "true",
		"-StreamLogPath", "nul", "-DryRun")
	if got.code != 1 || !strings.Contains(got.notes, "reserved device") {
		t.Fatalf("a reserved sink path survived into a dry run: exit %d, %q", got.code, got.notes)
	}
	if got.report != "" {
		t.Errorf("a plan was printed over a run that would refuse: %q", got.report)
	}
}

func TestAHostAddressIsTheOnlyValueOnTheReportStream(t *testing.T) {
	// ⭐ THE VALUE IS THE ONLY THING ON THE REPORT STREAM: every explanatory
	// line goes to the notes, so both of these assign one address:
	//
	//   $addr = wsl-toolkit script -Action HostAddress
	//   $addr = wsl-toolkit script -Action HostAddress 2>$null
	t.Setenv("USERPROFILE", filepath.Join(t.TempDir(), "absent"))
	lookupWSLAdapter = func() (string, string, bool) { return "172.23.96.1", "vEthernet (WSL)", true }
	got := runCompat(t, "-Action", "HostAddress")
	if got.code != 0 {
		t.Fatalf("exit %d: %q", got.code, got.notes)
	}
	if addr := strings.TrimSpace(got.report); addr != "172.23.96.1" {
		t.Errorf("the report stream = %q, want the address alone", got.report)
	}
	if !strings.Contains(got.notes, "NOT REACHABLE FROM THE DISTRO") {
		t.Errorf("the loopback warning is missing from the notes: %q", got.notes)
	}
}

func TestAMirroredHostAnswersLoopback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("USERPROFILE", dir)
	if err := writeFile(filepath.Join(dir, ".wslconfig"), "[wsl2]\nnetworkingMode=mirrored\n"); err != nil {
		t.Fatal(err)
	}
	got := runCompat(t, "-Action", "HostAddress")
	if got.code != 0 || strings.TrimSpace(got.report) != "127.0.0.1" {
		t.Fatalf("mirrored gave exit %d, report %q", got.code, got.report)
	}
	if !strings.Contains(got.notes, "share the loopback address") {
		t.Errorf("the mirrored explanation is missing: %q", got.notes)
	}
}

func TestABridgedHostRefusesRatherThanGuessing(t *testing.T) {
	// A mode with more than one right answer is refused with the candidates
	// named: an address invented here is one a caller binds a fixture to and
	// then debugs for an hour.
	dir := t.TempDir()
	t.Setenv("USERPROFILE", dir)
	if err := writeFile(filepath.Join(dir, ".wslconfig"), "[wsl2]\nnetworkingMode=bridged\n"); err != nil {
		t.Fatal(err)
	}
	got := runCompat(t, "-Action", "HostAddress")
	if got.code != 1 {
		t.Fatalf("exit %d, want 1", got.code)
	}
	if !strings.Contains(got.notes, "'bridged'") || !strings.Contains(got.notes, "/proc/net/route") {
		t.Errorf("the refusal does not name the mode or the way out: %q", got.notes)
	}
	if got.report != "" {
		t.Errorf("a refusal put an address on the report stream: %q", got.report)
	}
}

// TestListExitsTwoWhenWSLRefusesToAnswer is the exit-code half of the refusal
// contract: a partial inventory is never presented as complete, and it is
// never exit 0.
func TestListExitsTwoWhenWSLRefusesToAnswer(t *testing.T) {
	stubWsl(t, `echo "service unavailable" >&2; exit 1`)
	got := runCompat(t, "-Action", "List")
	if got.code != 2 {
		t.Fatalf("exit %d, want 2", got.code)
	}
	if !strings.Contains(got.notes, "could not list the distributions") {
		t.Errorf("the notes do not say what is missing: %q", got.notes)
	}
}

// TestARemoveDryRunForcesThePrefixBeforeItPlans holds the order that keeps a
// protected distro unreachable: the plan names the FORCED name, so a caller
// asking for podman-machine-default sees what would actually be removed.
func TestARemoveDryRunForcesThePrefixBeforeItPlans(t *testing.T) {
	got := runCompat(t, "-Action", "Remove", "-Name", "podman-machine-default", "-DryRun")
	if got.code != 0 {
		t.Fatalf("exit %d: %q", got.code, got.notes)
	}
	if !strings.Contains(got.report, "eph-podman-machine-default") {
		t.Errorf("the plan does not name the forced name: %q", got.report)
	}
	if !strings.Contains(got.report, "a protected distro cannot be reached by asking for it") {
		t.Errorf("the plan does not carry the guard's reason: %q", got.report)
	}
}

// TestPurgeDryRunNamesDistroDisksAndTarballsAndNeverSnapshots holds the one
// design question of Purge: it collects what was LEFT BEHIND, and the one
// durable thing the tool makes on purpose is never in the plan.
func TestPurgeDryRunNamesDistroDisksAndTarballsAndNeverSnapshots(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("WSL_TOOLKIT_STATE_DIR", "")
	base := filepath.Join(dir, "wsl-ephemeral")
	if err := osMkdirAll(filepath.Join(base, "snapshots")); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(base, "snapshots", "keep.tar"), "snapshot"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(base, "orphan.tar"), "rootfs"); err != nil {
		t.Fatal(err)
	}
	stubWsl(t, `echo eph-gone-1a2b; echo ubuntu-22.04`)
	got := runCompatIn(t, dir, "-Action", "Purge", "-DryRun")
	if got.code != 0 {
		t.Fatalf("exit %d: %q", got.code, got.notes)
	}
	if !strings.Contains(got.report, "unregister eph-gone-1a2b") || !strings.Contains(got.report, "delete     "+filepath.Join(base, "orphan.tar")) {
		t.Errorf("the plan does not name the owned state: %q", got.report)
	}
	if strings.Contains(got.report, "keep.tar") {
		t.Errorf("A SNAPSHOT IS IN THE PURGE PLAN: %q", got.report)
	}
}

func TestPurgeSaysWhatItKeeps(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("WSL_TOOLKIT_STATE_DIR", "")
	base := filepath.Join(dir, "wsl-ephemeral")
	if err := osMkdirAll(filepath.Join(base, "snapshots")); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(base, "snapshots", "keep.tar"), "snapshot"); err != nil {
		t.Fatal(err)
	}
	stubWsl(t, `exit 0`)
	got := runCompatIn(t, dir, "-Action", "Purge")
	if got.code != 0 {
		t.Fatalf("exit %d: %q", got.code, got.notes)
	}
	if !strings.Contains(got.notes, "KEPT") || !strings.Contains(got.notes, "never removes") {
		t.Errorf("the kept snapshots are not named: %q", got.notes)
	}
}

func TestDoctorCreatesNothingAndSaysHowEachRowWasTaken(t *testing.T) {
	stubWsl(t, `if [ "$1" = "--version" ]; then echo "WSL version: 2.6.1.0"; exit 0; fi
if [ "$1" = "--list" ]; then echo eph-x-1a2b; echo podman-machine-default; exit 0; fi
exit 0`)
	got := runCompat(t, "-Action", "Doctor")
	if got.code != 0 {
		t.Fatalf("exit %d: %q", got.code, got.notes)
	}
	for _, want := range []string{" obs  ", " der  ", "protected", "read-only, and it created nothing"} {
		if !strings.Contains(got.report, want) {
			t.Errorf("the doctor report is missing %q:\n%s", want, got.report)
		}
	}
	// The protected list is reported where a reader finds out the two tools
	// share a machine.
	if !strings.Contains(got.report, "podman-machine-default") {
		t.Errorf("the protected distro is not reported: %s", got.report)
	}
}

// stubGuestWsl answers the listing AND the transport: it decodes the base64
// payload out of the last argument exactly the way /bin/sh would, so the
// behaviour below matches on the payload's own text.
func stubGuestWsl(t *testing.T, probeBehaviour string) {
	t.Helper()
	script := `case " $* " in
  *" --list --quiet "*) echo eph-x-1a2b; exit 0 ;;
  *" --import "*|*" --terminate "*|*" --unregister "*) exit 0 ;;
esac
for wtk_last in "$@"; do :; done
wtk_b64=$(printf '%s' "$wtk_last" | sed -n 's/.*echo \([A-Za-z0-9+/=]*\)|base64.*/\1/p')
wtk_text=
[ -n "$wtk_b64" ] && wtk_text=$(printf '%s' "$wtk_b64" | base64 -d 2>/dev/null)
case "$wtk_text" in
  *"__WSL_OK__"*)
` + probeBehaviour + `
    ;;
  *"make world"*)
    echo "guest ran the command" >&2
    exit 42
    ;;
  *)
    exit 3
    ;;
esac`
	stubWsl(t, script)
}

func TestTheRunnerExitsWithTheInnerCommand(t *testing.T) {
	stubGuestWsl(t, `echo __WSL_OK__
echo "Linux builder 6.6"
exit 0`)
	got := runCompat(t, "-Action", "Run", "-Name", "eph-x-1a2b", "-Command", "make world", "-NoTimestamps")
	if got.code != 42 {
		t.Fatalf("exit %d, want the inner command's 42 (notes: %q)", got.code, got.notes)
	}
	if !strings.Contains(got.notes, "guest ran the command") {
		t.Errorf("the guest's own stderr did not reach stderr: %q", got.notes)
	}
}

// TestNewSmokeProbeFailureNamesTheChannel runs the creation far enough to hit
// the probe, and the refusal names the transport instead of a mystery.
func TestNewSmokeProbeFailureNamesTheChannel(t *testing.T) {
	// The guest answers, but the marker never arrives: exactly what a rootfs
	// whose /bin/sh cannot carry the channel looks like from outside.
	stubGuestWsl(t, `echo "a shell that cannot carry a payload"`)
	stubEngine(t)
	// ⛔ THE VOLUME IS HELD STILL. Without this the preflight's answer depends
	// on how full the machine running the test happens to be, and on a small
	// volume the space refusal fires before the probe ever does.
	previousFree := volumeFree
	volumeFree = func(string) (int64, bool) { return 1 << 40, true }
	t.Cleanup(func() { volumeFree = previousFree })
	got := runCompat(t, "-Action", "New", "-Image", "alpine:3.22", "-Name", "probe", "-Force")
	if got.code != 1 {
		t.Fatalf("exit %d: %q", got.code, got.notes)
	}
	if !strings.Contains(got.notes, "/bin/sh did not run") {
		t.Errorf("the refusal does not name the channel: %q", got.notes)
	}
	// ⛔ THE ROLLBACK IS THE CONTRACT: a creation that failed halfway says so
	// on the report stream, and the ORIGINAL error is what reaches the caller
	// as the refusal, not the rollback's own noise.
	if !strings.Contains(got.report, "creation failed; rolling back") {
		t.Errorf("the rollback was not said: %q", got.report)
	}
}

func stubEngine(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	engine := filepath.Join(dir, "podman")
	script := `#!/bin/sh
case "$1 $2" in
  "info --format") echo x86_64; exit 0 ;;
  "pull --platform") exit 0 ;;
  "create --platform") echo c0ffee123456; exit 0 ;;
  "export -o") head -c 4096 /dev/zero > "$3"; exit 0 ;;
  "rm -f") exit 0 ;;
  "image inspect") echo '{"Env":["PATH=/usr/bin"],"WorkingDir":"/work"}'; exit 0 ;;
  *) exit 0 ;;
esac
`
	if err := writeFileExec(engine, script); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(filepath.ListSeparator)+currentPath(t))
}

func currentPath(t *testing.T) string {
	t.Helper()
	return os.Getenv("PATH")
}
