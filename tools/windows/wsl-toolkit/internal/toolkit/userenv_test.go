// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestUserEnvironmentKeepsThePathOrderContract(t *testing.T) {
	got := userEnvironmentPrelude
	want := `for _wtk_dir in "$HOME/.local/bin" "$HOME/bin" "$HOME/.cargo/bin" "$HOME/go/bin" "$HOME/.bun/bin" "$HOME/.deno/bin" "$HOME/.nix-profile/bin" /nix/var/nix/profiles/default/bin /usr/local/go/bin /usr/local/cargo/bin /usr/local/bin /usr/bin /bin /usr/local/sbin /usr/sbin /sbin; do`
	if !strings.Contains(got, want) {
		t.Fatalf("ordered path contract is absent:\n%s", got)
	}
}

// TestTheUserEnvironmentIsTheFrontOfTheComposedCommand reads the one production
// path to the prologue: --user-env reaches a guest through ComposePayload and
// nothing else.
func TestTheUserEnvironmentIsTheFrontOfTheComposedCommand(t *testing.T) {
	payload := []byte("printf '$HOME @hostaddress'\n")
	got := ComposePayload(payload, nil, true)
	if !bytes.HasPrefix(got, []byte(userEnvironmentPrelude)) || !bytes.HasSuffix(got, payload) {
		t.Fatalf("the prologue is not the front and the command the unchanged end: %q", got)
	}
	if plain := ComposePayload(payload, nil, false); bytes.Contains(plain, []byte("XDG_RUNTIME_DIR")) {
		t.Fatal("the prologue was composed into a command that did not ask for --user-env")
	}
}

// TestTheUserEnvironmentRefusesASymlinkedRunDirectoryAndDeduplicatesThePath runs
// the prologue through a real POSIX shell, with its runtime root moved into a
// temporary directory.
//
// ⛔ THE SYMLINK REFUSAL IS WHY THIS RUNS A SHELL. The runtime directory sits in a
// shared /tmp, so anyone able to write there can plant a link to a directory they
// read before the command starts, and the command's private files land in it.
func TestTheUserEnvironmentRefusesASymlinkedRunDirectoryAndDeduplicatesThePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the prologue is read by a POSIX shell, and this host has none to hand it to")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh on this host")
	}
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	extra := filepath.Join(tmp, "extra")
	for _, d := range []string{filepath.Join(home, ".local", "bin"), extra, filepath.Join(tmp, "elsewhere")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	prologue := strings.ReplaceAll(userEnvironmentPrelude, "/tmp/wsl-toolkit-run-", tmp+"/wsl-toolkit-run-")
	run := func(script string) (string, int) {
		cmd := exec.Command("/bin/sh", "-c", prologue+script)
		cmd.Env = []string{"HOME=" + home, "PATH=" + extra + ":/usr/bin:/usr/bin:/bin"}
		out, err := cmd.CombinedOutput()
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			return string(out), exited.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		return string(out), 0
	}
	runDir := tmp + "/wsl-toolkit-run-" + strconv.Itoa(os.Getuid())
	out, code := run(`printf '%s|%s|%s\n' "$XDG_RUNTIME_DIR" "$TMPDIR" "$PATH"`)
	fields := strings.Split(strings.TrimSpace(out), "|")
	if code != 0 || len(fields) != 3 || fields[0] != runDir || fields[1] != runDir+"/tmp" {
		t.Fatalf("exit %d, output %q, want the private runtime directory and its tmp", code, out)
	}
	path := strings.Split(fields[2], ":")
	seen := map[string]bool{}
	for _, p := range path {
		if seen[p] {
			t.Errorf("PATH carries %s twice: %s", p, fields[2])
		}
		seen[p] = true
	}
	if path[0] != filepath.Join(home, ".local", "bin") || path[len(path)-1] != extra {
		t.Errorf("PATH is not the user's tools first and the inherited entries after: %s", fields[2])
	}
	if info, err := os.Stat(runDir); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the runtime directory is not private: %v %v", info, err)
	}

	if err := os.RemoveAll(runDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(tmp, "elsewhere"), runDir); err != nil {
		t.Fatal(err)
	}
	out, code = run(`echo SHOULD-NOT-RUN`)
	if code != 2 || !strings.Contains(out, "is a symlink") || strings.Contains(out, "SHOULD-NOT-RUN") {
		t.Fatalf("a symlinked runtime directory answered exit %d and %q, want a refusal before the command", code, out)
	}

	if os.Geteuid() != 0 {
		return
	}
	// As root the directory can belong to somebody else, which is the other half
	// of the same attack.
	if err := os.Remove(runDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(runDir, 65534, 65534); err != nil {
		t.Fatal(err)
	}
	out, code = run(`echo SHOULD-NOT-RUN`)
	if code != 2 || !strings.Contains(out, "belongs to uid 65534") || strings.Contains(out, "SHOULD-NOT-RUN") {
		t.Fatalf("a runtime directory owned by another account answered exit %d and %q", code, out)
	}
}
