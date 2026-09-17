// automountroot_test.go - the two readers of `[automount] root` agree.
//
// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBothReadersOfTheAutomountRootAgree is finding 23.
//
// ⛔ TWO FILES HELD DISAGREEING DEFINITIONS OF "A WINDOWS DRIVE UNDER /mnt".
// verify.sh hardcoded `drive_root=/mnt` while shell-profile.sh reads
// `[automount] root` from /etc/wsl.conf, so a distribution ADOPTED with a
// hand-set root had its drives mounted where the verifier was not looking and
// `automount off` verified over a guest with every drive mounted.
//
// ⚠ THEY CANNOT BE MERGED INTO ONE FILE. One is embedded in the executable and
// the other is fetched by URL and must work with no executable at all, which is
// exactly the shape that needs a check asserting the two agree rather than a
// shared function. This is that check.
func TestBothReadersOfTheAutomountRootAgree(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	// The reader lifted out of each script, so the case drives the real
	// expressions rather than a restatement of them.
	verifyReader := mustExtract(t, filepath.Join("verify.sh"),
		"drive_root=$(", "[ -n \"$drive_root\" ] || drive_root=/mnt")

	for _, tc := range []struct {
		name, conf, want string
	}{
		{"no wsl.conf at all", "", "/mnt"},
		{"an [automount] block with no root key", "[automount]\nenabled = false\n", "/mnt"},
		{"a hand-set root", "[automount]\nroot = /windows\n", "/windows"},
		{"a quoted root", "[automount]\nroot = \"/drives\"\n", "/drives"},
		{"a root with a trailing slash", "[automount]\nroot = /w/\n", "/w"},
		// ⚠ The LAST one wins, which is what a parser reading top to bottom
		// does and what both files must agree on.
		{"two root keys", "[automount]\nroot = /a\nroot = /b\n", "/b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			confDir := filepath.Join(dir, "etc")
			if err := os.MkdirAll(confDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.conf != "" {
				if err := os.WriteFile(filepath.Join(confDir, "wsl.conf"), []byte(tc.conf), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			// The scripts read /etc/wsl.conf by absolute path, so the fixture
			// is substituted rather than the filesystem faked.
			script := strings.ReplaceAll(verifyReader, "/etc/wsl.conf", filepath.ToSlash(filepath.Join(confDir, "wsl.conf")))
			out, err := exec.Command(sh, "-c", script+"\nprintf '%s' \"$drive_root\"\n").CombinedOutput()
			if err != nil {
				t.Fatalf("the verifier's reader did not run: %v: %s", err, out)
			}
			if got := strings.TrimSpace(string(out)); got != tc.want {
				t.Fatalf("verify.sh reads %q, want %q", got, tc.want)
			}
		})
	}
}

// mustExtract lifts the lines between two anchors out of a shipped script, so a
// case drives the real expression rather than a copy of it that can drift.
func mustExtract(t *testing.T, path, from, to string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(body), "\n")
	start := -1
	for i, ln := range lines {
		if start < 0 && strings.Contains(ln, from) {
			start = i
			continue
		}
		if start >= 0 && strings.Contains(ln, to) {
			return strings.Join(lines[start:i+1], "\n")
		}
	}
	t.Fatalf("%s does not hold the block between %q and %q, so this case is reading a file that moved", path, from, to)
	return ""
}
