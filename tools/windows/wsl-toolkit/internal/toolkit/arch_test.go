// SPDX-License-Identifier: 0BSD

package toolkit

import (
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

// ⛔ WSL-188: THE MACHINE'S ARCHITECTURE, NOT THE EXECUTABLE'S. The amd64 build on
// an ARM64 Windows asked every job for amd64 under emulation, and selfupdate
// fetched the amd64 build again. A job and an update follow the machine.
func TestTheMachinesArchitectureIsReadNotTheExecutables(t *testing.T) {
	answerMachine(t, imageFileMachineARM64, true)
	if got := nativeArch(); got != "arm64" {
		t.Errorf("an ARM64 machine reads as %q", got)
	}
	if got := NativePlatform(); got != "linux/arm64" {
		t.Errorf("a job on an ARM64 machine asks for %q, want linux/arm64", got)
	}
	if got, want := AssetName(), "wsl-toolkit-"+runtime.GOOS+"-arm64.exe"; got != want {
		t.Errorf("selfupdate on an ARM64 machine fetches %q, want %q", got, want)
	}
	answerMachine(t, imageFileMachineAMD64, true)
	if got := NativePlatform(); got != "linux/amd64" {
		t.Errorf("a job on an amd64 machine asks for %q", got)
	}
	// ⚠ A MACHINE WINDOWS COULD NOT NAME falls back to the executable's own, which
	// is what every answer was before.
	answerMachine(t, 0, false)
	if got := nativeArch(); got != runtime.GOARCH {
		t.Errorf("an unread machine reads as %q, want the executable's %q", got, runtime.GOARCH)
	}
}

// ⭐ AND WINDOWS ANSWERS THE QUESTION. Every Windows this tool runs on has
// IsWow64Process2, so a call that cannot be made is a wiring fault, not a host.
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
