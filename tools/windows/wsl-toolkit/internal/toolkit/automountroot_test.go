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
	// ⛔ AND doors.sh's, which hardcoded /mnt. WSL-188.
	readers := map[string]string{
		"verify.sh": verifyReader,
		"doors.sh":  mustExtract(t, "doors.sh", "drive_root=$(", "[ -n \"$drive_root\" ] || drive_root=/mnt"),
	}

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
			for file, reader := range readers {
				script := strings.ReplaceAll(reader, "/etc/wsl.conf", filepath.ToSlash(filepath.Join(confDir, "wsl.conf")))
				out, err := exec.Command(sh, "-c", script+"\nprintf '%s' \"$drive_root\"\n").CombinedOutput()
				if err != nil {
					t.Fatalf("%s's reader did not run: %v: %s", file, err, out)
				}
				if got := strings.TrimSpace(string(out)); got != tc.want {
					t.Fatalf("%s reads %q, want %q", file, got, tc.want)
				}
			}
		})
	}
}

// ⛔ WSL-188: doors.sh LOOKS FOR THE DRIVES WHERE wsl.conf PUTS THEM. It hardcoded
// /mnt three times, so a distribution adopted with `[automount] root = /windows`
// reported its drives closed, reached no cmd.exe, and read a Windows directory on
// PATH as none. Each of the three is driven with a hand-set root.
func TestDoorsLooksForTheDrivesUnderTheAutomountRoot(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	dir := t.TempDir()
	root := filepath.ToSlash(filepath.Join(dir, "windows"))
	conf := filepath.Join(dir, "wsl.conf")
	if err := os.WriteFile(conf, []byte("[automount]\nroot = "+root+"/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mounts := filepath.Join(dir, "mounts")
	if err := os.WriteFile(mounts, []byte("C:\\134 "+root+"/c 9p rw 0 0\nnone /mnt/wsl tmpfs rw 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := filepath.Join(dir, "windows", "c", "Windows", "System32", "cmd.exe")
	if err := os.MkdirAll(filepath.Dir(cmd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cmd, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	reader := mustExtract(t, "doors.sh", "drive_root=$(", "say fs.windows-drives closed")
	finder := mustExtract(t, "doors.sh", "winexe=", "done")
	path := mustExtract(t, "doors.sh", `case ":$PATH:" in`, "esac")
	script := "say() { printf '%s|%s|%s\\n' \"$1\" \"$2\" \"$3\"; }\n" +
		strings.NewReplacer("/etc/wsl.conf", filepath.ToSlash(conf), "/proc/mounts", filepath.ToSlash(mounts)).Replace(reader) + "\nfi\n" +
		finder + "\nprintf 'winexe|%s\\n' \"$winexe\"\nPATH=/usr/bin:" + root + "/c/Windows\n" + path + "\n"
	out, err := exec.Command(sh, "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("doors.sh's drive rows did not run: %v: %s", err, out)
	}
	for _, want := range []string{
		"fs.windows-drives|open|mounted: " + root + "/c ",
		"winexe|" + filepath.ToSlash(cmd) + "\n",
		"interop.windows-path|open|",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("doors.sh does not say %q under the root %s:\n%s", want, root, out)
		}
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
