// SPDX-License-Identifier: 0BSD

//go:build windows

package toolkit

import (
	"syscall"
	"testing"
)

// TestTheRegistryAnswersAMissingSubkeyAsGone asks the real registry for a subkey
// that does not exist, under a key every user has, and holds that the answer is
// the one registeredDisks skips rather than one it refuses on.
func TestTheRegistryAnswersAMissingSubkeyAsGone(t *testing.T) {
	path, err := syscall.UTF16PtrFromString(`Software`)
	if err != nil {
		t.Fatal(err)
	}
	var root syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, path, 0, syscall.KEY_READ, &root); err != nil {
		t.Fatalf("HKCU\\Software could not be opened: %v", err)
	}
	defer syscall.RegCloseKey(root)
	_, err = registryString(root, "wsl-toolkit-test-no-such-subkey-7f3c9a", "DistributionName")
	if !registrationGone(err) {
		t.Fatalf("a subkey that does not exist answered %v, which registeredDisks would refuse on", err)
	}
}
