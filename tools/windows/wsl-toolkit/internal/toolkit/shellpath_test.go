// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// shellPath is dir as a script's text can carry it unquoted: forward slashes,
// and on Windows its short name where the long one cannot be carried. A case
// skips, saying why, where neither will do.
//
// ⛔ A CASE PASTES ITS DIRECTORY INTO THE SCRIPT IT DRIVES, as the guest's own
// paths are written there. A temporary directory holding a space broke the
// script, measured with TMP set to one: ten of the eleven WSL-198 cases failed,
// and two older ones. A Windows profile such as `First Last` puts one there.
// WSL-198.
func shellPath(t *testing.T, dir string) string {
	t.Helper()
	if p := filepath.ToSlash(dir); shellSafe(p) {
		return p
	}
	if short, ok := shortPath(dir); ok {
		if p := filepath.ToSlash(short); shellSafe(p) {
			return p
		}
	}
	t.Skipf("the case's directory %q holds a character a script cannot carry unquoted, and it has no short name that does. Point TMP at a directory without one", dir)
	return ""
}

// shellSafe is whether p holds only letters, digits and `/:._~+-`, which a
// script's text carries unquoted and unchanged.
func shellSafe(p string) bool {
	if p == "" {
		return false
	}
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/', r == ':', r == '.', r == '_', r == '~', r == '+', r == '-':
		default:
			return false
		}
	}
	return true
}

// ⛔ A DIRECTORY WITH A SPACE IS HANDED ON BY ITS SHORT NAME, OR SKIPPED, and never
// pasted into a script as it is. WSL-198.
func TestACasesDirectoryWithASpaceIsNotPastedIntoAScript(t *testing.T) {
	for p, want := range map[string]bool{
		"D:/build/DOWNLO~1/ToolKit/x": true,
		"/tmp/TestX123/001":           true,
		"D:/build/First Last/x":       false,
		"/tmp/it's":                   false,
		`D:\build\x`:                  false,
		"/tmp/$HOME":                  false,
		"":                            false,
	} {
		if got := shellSafe(p); got != want {
			t.Errorf("shellSafe(%q) = %v, want %v", p, got, want)
		}
	}
	// ⚠ Only Windows names a directory twice. Elsewhere one with a space skips.
	if runtime.GOOS != "windows" {
		return
	}
	d := filepath.Join(t.TempDir(), "with space")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := shellPath(t, d); !shellSafe(got) {
		t.Fatalf("shellPath answered %q, which a script cannot carry", got)
	}
}
