// automountroot_test.go - the two readers of `[automount] root` agree.
//
// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
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
	parallelCase(t)
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
		// ⛔ WSL-188: THE LONE SLASH IS A ROOT, which puts the drives at /c. Every
		// trailing slash was stripped, so it read as unset and /mnt was searched.
		{"a root of /", "[automount]\nroot = /\n", "/"},
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
				if err := forksafe.WriteFile(filepath.Join(confDir, "wsl.conf"), []byte(tc.conf), 0o644); err != nil {
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
	parallelCase(t)
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	dir := t.TempDir()
	root := filepath.ToSlash(filepath.Join(dir, "windows"))
	conf := filepath.Join(dir, "wsl.conf")
	if err := forksafe.WriteFile(conf, []byte("[automount]\nroot = "+root+"/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mounts := filepath.Join(dir, "mounts")
	if err := forksafe.WriteFile(mounts, []byte("C:\\134 "+root+"/c 9p rw 0 0\nnone /mnt/wsl tmpfs rw 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := filepath.Join(dir, "windows", "c", "Windows", "System32", "cmd.exe")
	if err := os.MkdirAll(filepath.Dir(cmd), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := forksafe.WriteFile(cmd, nil, 0o755); err != nil {
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

// ⛔ WSL-188: AND A ROOT OF / IS KEPT. `root = /` puts the drives at /c, and the
// reader stripped its lone slash, so doors looked under /mnt and reported the
// drives closed and no Windows directory on PATH. ⚠ A path on no drive is not
// read as one, which a root of / would make every path.
func TestDoorsReadsARootOfSlash(t *testing.T) {
	parallelCase(t)
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	dir := t.TempDir()
	conf := seedFile(t, dir, "wsl.conf", "[automount]\nroot = /\n")
	mounts := seedFile(t, dir, "mounts", "C:\\134 /c 9p rw 0 0\nnone /mnt/wsl tmpfs rw 0 0\n")
	reader := strings.NewReplacer("/etc/wsl.conf", filepath.ToSlash(conf), "/proc/mounts", filepath.ToSlash(mounts)).
		Replace(mustExtract(t, "doors.sh", "drive_root=$(", "say fs.windows-drives closed"))
	path := mustExtract(t, "doors.sh", `case ":$PATH:" in`, "esac")
	run := func(onPath string) string {
		t.Helper()
		script := "say() { printf '%s|%s|%s\\n' \"$1\" \"$2\" \"$3\"; }\n" + reader + "\nfi\nPATH=" + onPath + "\n" + path + "\n"
		out, err := exec.Command(sh, "-c", script).CombinedOutput()
		if err != nil {
			t.Fatalf("doors.sh's drive rows did not run: %v: %s", err, out)
		}
		return string(out)
	}
	out := run("/usr/bin:/c/Windows")
	for _, want := range []string{"fs.windows-drives|open|mounted: /c ", "interop.windows-path|open|"} {
		if !strings.Contains(out, want) {
			t.Errorf("doors.sh does not say %q under a root of /:\n%s", want, out)
		}
	}
	if out := run("/usr/bin:/bin"); !strings.Contains(out, "interop.windows-path|closed|") {
		t.Errorf("doors.sh read a PATH on no drive as a Windows directory under a root of /:\n%s", out)
	}
}

// ⛔ WSL-188: THE VERIFIER COUNTS THE DRIVES UNDER A ROOT OF / TOO. It searched
// /mnt, so a base adopted with `root = /` read its drives as off: automount off
// verified over a guest with a drive mounted, and rw refused one that kept it.
func TestTheVerifierCountsTheDrivesUnderARootOfSlash(t *testing.T) {
	parallelCase(t)
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	dir := t.TempDir()
	conf := seedFile(t, dir, "wsl.conf", "[automount]\nroot = /\n")
	mounts := seedFile(t, dir, "mounts", "C:\\134 /c 9p rw,noatime,aname=drvfs 0 0\nnone /mnt/wsl tmpfs rw 0 0\n")
	section := markedSection(t, verifyScript, "the windows drives")
	for from, to := range map[string]string{
		"/etc/wsl.conf":                 filepath.ToSlash(conf),
		"\ndrive_mounts=/proc/mounts\n": "\ndrive_mounts='" + filepath.ToSlash(mounts) + "'\n",
	} {
		if n := strings.Count(section, from); n != 1 {
			t.Fatalf("the drive section names %q %d times, want once", strings.TrimSpace(from), n)
		}
		section = strings.Replace(section, from, to, 1)
	}
	for _, c := range []struct {
		setting, said string
		code          int
	}{
		{AutomountReadWrite, "automount rw", 0},
		{AutomountOff, "a Windows drive is mounted below / even though automount is off", 3},
	} {
		cmd := exec.Command(sh, "-c", "set -eu\n"+section)
		cmd.Env = append(os.Environ(), "TK_AUTOMOUNT="+c.setting)
		out, err := cmd.CombinedOutput()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		if code != c.code || !strings.Contains(string(out), c.said) {
			t.Errorf("automount %s over a drive at /c answered exit %d, want %d saying %q:\n%s", c.setting, code, c.code, c.said, out)
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
