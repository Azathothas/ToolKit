// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"runtime"
	"testing"
)

// answerMachine makes nativeMachine answer m for one case.
func answerMachine(t *testing.T, m uint16, ok bool) {
	t.Helper()
	previous := nativeMachine
	t.Cleanup(func() { nativeMachine = previous })
	nativeMachine = func() (uint16, bool) { return m, ok }
}

// answerExecutable makes the executable's own architecture arch for one case.
//
// ⛔ ONE NO MACHINE HERE ANSWERS, so an answer that fell back to the executable's
// reads as that and not as the machine's, on an amd64 host and an arm64 one
// alike. With runtime.GOARCH in its place, a lost ARM64 row stayed green on
// arm64. WSL-188.
func answerExecutable(t *testing.T, arch string) {
	t.Helper()
	previous := executableArch
	t.Cleanup(func() { executableArch = previous })
	executableArch = arch
}

// ⛔ WSL-188: THE MACHINE'S ARCHITECTURE, NOT THE EXECUTABLE'S. The amd64 build on
// an ARM64 Windows asked every job for amd64 under emulation, and selfupdate
// fetched the amd64 build again. A job and an update follow the machine.
//
// ⚠ EACH MACHINE IS ASKED BOTH WAYS, ARM64 AND AMD64, so a row that falls back
// to the executable's architecture goes red on either host.
func TestTheMachinesArchitectureIsReadNotTheExecutables(t *testing.T) {
	answerExecutable(t, "riscv64")
	for _, c := range []struct {
		machine uint16
		arch    string
	}{
		{imageFileMachineARM64, "arm64"},
		{imageFileMachineAMD64, "amd64"},
	} {
		answerMachine(t, c.machine, true)
		if got := nativeArch(); got != c.arch {
			t.Errorf("an %s machine reads as %q", c.arch, got)
		}
		if got, want := NativePlatform(), "linux/"+c.arch; got != want {
			t.Errorf("a job on an %s machine asks for %q, want %q", c.arch, got, want)
		}
		if got, want := AssetName(), "wsl-toolkit-"+runtime.GOOS+"-"+c.arch+".exe"; got != want {
			t.Errorf("selfupdate on an %s machine fetches %q, want %q", c.arch, got, want)
		}
	}
	// ⚠ A MACHINE WINDOWS COULD NOT NAME falls back to the executable's own, which
	// is what every answer was before.
	answerMachine(t, 0, false)
	if got := nativeArch(); got != "riscv64" {
		t.Errorf("an unread machine reads as %q, want the executable's riscv64", got)
	}
}

// ⛔ WSL-188: AND DOCTOR REPORTS THE MACHINE'S. On ARM64 Windows the amd64 build
// reported x86_64 while its jobs ran linux/arm64.
func TestDoctorReportsTheMachinesArchitecture(t *testing.T) {
	_ = newFakeHost(t)
	answerExecutable(t, "riscv64")
	answerMachine(t, imageFileMachineARM64, true)
	report, err := Doctor(context.Background(), DoctorOptions{Fast: true, Group: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Host["arch"] != "aarch64" {
		t.Errorf("doctor on an ARM64 machine reports arch %v, want aarch64", report.Host["arch"])
	}
	answerMachine(t, imageFileMachineAMD64, true)
	if got := doctorArch(); got != "x86_64" {
		t.Errorf("doctor on an amd64 machine reports arch %q, want x86_64", got)
	}
	answerMachine(t, 0, false)
	if got := doctorArch(); got != "riscv64" {
		t.Errorf("doctor on a machine Windows could not name reports arch %q, want the executable's riscv64", got)
	}
}

// ⭐ AND WINDOWS ANSWERS THE QUESTION. Every Windows this tool runs on has
// IsWow64Process2, so a call that cannot be made is a wiring fault, not a host.
//
// ⚠ WINDOWS ONLY, AND ACCEPTED AS SUCH. IsWow64Process2 is a Windows call, and
// readNativeMachine elsewhere asks nothing, so its row is proved on Windows and
// SKIPPED on Linux. WSL-188.
func TestWindowsNamesItsOwnMachine(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows has IsWow64Process2 to ask")
	}
	m, ok := readNativeMachine()
	if !ok {
		t.Fatal("IsWow64Process2 did not answer")
	}
	switch m {
	case imageFileMachineAMD64, imageFileMachineARM64:
	default:
		t.Fatalf("IsWow64Process2 named machine %#x, which this tool is not built for", m)
	}
}
