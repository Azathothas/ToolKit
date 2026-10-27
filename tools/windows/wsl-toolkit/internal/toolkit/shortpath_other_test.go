// SPDX-License-Identifier: 0BSD

//go:build !windows

package toolkit

// shortPath is false off Windows, where a path has one name. WSL-198.
func shortPath(string) (string, bool) { return "", false }
