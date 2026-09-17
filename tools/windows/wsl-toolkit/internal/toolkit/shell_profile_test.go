package toolkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ⛔ THE PROFILE'S OWN MECHANISMS HAD NO CASE IN THE GATE. They were driven across
// thirteen images with `matrix --images all`, which is the real proof and which nobody
// runs on a commit. A regression in the file would have reached a release with the gate
// green, so the guard mutation lens asked for this on 2026-09-17.
//
// ⭐ IT DRIVES THE SHIPPED COPY, the one the executable carries, so a file that stopped
// agreeing with its definition cannot pass here and fail in a base.
func profileUnderSh(t *testing.T, args ...string) (string, string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX sh on this machine to drive the profile with")
	}
	body, err := shippedTree.ReadFile("shipped/shell-profile.sh")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "shell-profile.sh")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	// ⛔ `sh -c` IS NOT INTERACTIVE, and every mechanism here runs only for a shell a
	// person is typing at. `-i` is what puts `i` in `$-`.
	cmd := exec.Command(sh, append([]string{"-i", "-c"}, args...)...)
	cmd.Env = append(os.Environ(),
		"HOME="+filepath.ToSlash(home),
		"WSL_TOOLKIT_PROFILE=",
		"TK_PROFILE="+filepath.ToSlash(path),
	)
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	_ = cmd.Run()
	return strings.TrimSpace(out.String()), errBuf.String()
}

const profileReadsPATH = `PATH="$TK_PATH"; unset WSL_TOOLKIT_PROFILE; . "$TK_PROFILE" 2>/dev/null; printf %s "$PATH"`

func profilePATH(t *testing.T, in string, off bool, interactive bool) string {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX sh on this machine to drive the profile with")
	}
	body, err := shippedTree.ReadFile("shipped/shell-profile.sh")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "shell-profile.sh")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	flags := []string{"-i", "-c"}
	if !interactive {
		flags = []string{"-c"}
	}
	cmd := exec.Command(sh, append(flags, profileReadsPATH)...)
	env := append(os.Environ(),
		"HOME="+filepath.ToSlash(home),
		"TK_PROFILE="+filepath.ToSlash(path),
		"TK_PATH="+in,
	)
	if off {
		env = append(env, "WSL_TOOLKIT_NO_PROFILE=1")
	}
	cmd.Env = env
	out, _ := cmd.Output()
	return strings.TrimSpace(string(out))
}

// ⭐ THE DE-DUPLICATION KEEPS THE FIRST OCCURRENCE IN ITS PLACE, so which binary wins
// does not change. ⛔ And an empty element is dropped, because `PATH=/bin:` searches the
// current directory for every command typed.
func TestTheProfileDeduplicatesAnInteractiveShellsPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/a:/b:/a:/c:/b", "/a:/b:/c"},
		{"/a::/b:", "/a:/b"},
		{"/a:/b", "/a:/b"},
		{"/only", "/only"},
	}
	for _, c := range cases {
		if got := profilePATH(t, c.in, false, true); got != c.want {
			t.Fatalf("PATH %q became %q, want %q", c.in, got, c.want)
		}
	}
}

// ⛔ A NON-INTERACTIVE SHELL KEEPS ITS PATH EXACTLY. `distro run -c` and `matrix -c`
// send every command to one, so a profile that rewrote it would change what every
// caller's command resolves, silently and after their own setup.
func TestTheProfileLeavesANonInteractiveShellAlone(t *testing.T) {
	const in = "/a:/b:/a"
	if got := profilePATH(t, in, false, false); got != in {
		t.Fatalf("a non-interactive shell's PATH became %q, want %q", got, in)
	}
}

// ⭐ ONE NAMED SWITCH TURNS ALL OF IT OFF.
func TestTheProfileSwitchTurnsEverythingOff(t *testing.T) {
	const in = "/a:/b:/a"
	if got := profilePATH(t, in, true, true); got != in {
		t.Fatalf("with WSL_TOOLKIT_NO_PROFILE set, PATH became %q, want %q", got, in)
	}
}

// ⛔ A PROFILE ERROR DOES NOT CHANGE A SHELL'S EXIT CODE: the shell starts anyway and
// the line that failed did nothing. So the assertion is on STDERR, which is where
// WSL-71 put it.
//
// ⛔ AND IT IS A DELTA, NOT A BYTE COUNT. The shell writes its own line on a machine
// with no controlling terminal - here, "cannot set terminal process group" - and
// Photon's own /etc/profile.d writes 66 bytes on every login shell. An absolute count
// reports the host's noise as this file's defect, which it did the first time this case
// ran.
func TestTheProfileAddsNothingToStderr(t *testing.T) {
	_, without := profileUnderSh(t, `exit 0`)
	_, with := profileUnderSh(t, `unset WSL_TOOLKIT_PROFILE; . "$TK_PROFILE"; exit 0`)
	if with != without {
		t.Fatalf("the profile ADDED to stderr. without: %q with: %q", without, with)
	}
}
