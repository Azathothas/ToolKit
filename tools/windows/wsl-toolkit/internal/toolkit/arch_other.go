// SPDX-License-Identifier: 0BSD

//go:build !windows

package toolkit

// readNativeMachine has nothing to ask off Windows, so the executable's own
// architecture stands in for the machine's. WSL-188.
func readNativeMachine() (uint16, bool) { return 0, false }
