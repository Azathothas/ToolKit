// SPDX-License-Identifier: 0BSD

package checks

import (
	"strings"
	"testing"
)

// home assembles a Windows home path at run time. ⚠ Written literally, each
// fixture below would be a finding in this very file, because the rule reads
// every file in the tree and exempts only the one that holds its patterns.
func home(sep, name, rest string) string {
	return strings.Join([]string{"C:", "Users", name, rest}, sep)
}

// TestAWindowsHomePathIsFoundWithEitherSeparator is the case for a rule that
// could only ever see a forward slash. `[\/]` in a Go character class is an
// escaped `/`, so a path written the way Windows writes it passed the gate. The
// published tree carried a real username that way. TOOL-25.
func TestAWindowsHomePathIsFoundWithEitherSeparator(t *testing.T) {
	for _, s := range []string{
		home(`\`, "alice", "Downloads"),
		home("/", "alice", "Downloads"),
		`"disk_path": "` + home(`\\`, "alice", "AppData") + `"`,
	} {
		m := homePathRe.FindString(s)
		if m == "" || genericHome(m) {
			t.Errorf("%s was not reported as a private home path (match %q)", s, m)
		}
	}
}

// TestAGenericWindowsHomeIsNotAFingerprint holds the other direction, which is
// the one that decides whether anybody keeps the rule switched on.
func TestAGenericWindowsHomeIsNotAFingerprint(t *testing.T) {
	for _, s := range []string{
		home(`\`, "RUNNER~1", "AppData"),
		home(`\`, "runneradmin", "AppData"),
		home(`\`, "USER", "Downloads"),
		home(`\`, "...", "snapshots"),
		"/home/toolkit/.config",
	} {
		if m := homePathRe.FindString(s); m != "" && !genericHome(m) {
			t.Errorf("%s was reported as a private home path (match %q)", s, m)
		}
	}
}
