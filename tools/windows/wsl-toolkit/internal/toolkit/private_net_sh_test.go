// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestTheWrapperLeavesNoWorkDirectoryInARealShell runs private-net.sh in a real
// POSIX shell, with stand-ins for pasta and nft. ⛔ `exec pasta` replaces the
// wrapper's shell, so its trap never ran and every call left a directory in /tmp
// holding the payload. The payload removes it now. WSL-133.
func TestTheWrapperLeavesNoWorkDirectoryInARealShell(t *testing.T) {
	sh := posixShell(t)
	bin := t.TempDir()
	// The stand-in pasta runs what follows its "--", as pasta does inside the
	// namespace it makes. The stand-in nft accepts every rule.
	writeStandIn(t, bin, "pasta", `while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do shift; done; shift; exec "$@"`)
	writeStandIn(t, bin, "nft", "exit 0")
	tmp := t.TempDir()
	cmd := exec.Command(sh, "-c", string(privateNetScript))
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TMPDIR="+tmp,
		"TK_PRIVATE_NET_PAYLOAD_B64="+base64.StdEncoding.EncodeToString([]byte("printf 'ran\\n'\n")),
		"TK_HOSTADDR=",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil || stdout.String() != "ran\n" {
		t.Fatalf("the wrapper ran its payload as %q, %v: %s", stdout.String(), err, stderr.String())
	}
	left, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("the wrapper left %d entries in its temporary directory, first %s", len(left), left[0].Name())
	}
}

// TestAStaleShellLauncherIsSweptInARealShell runs the sweep `base shell
// --private-net` makes before it places a launcher: one older than an hour goes,
// a fresh one stays, and a missing directory is not a failure. WSL-133.
func TestAStaleShellLauncherIsSweptInARealShell(t *testing.T) {
	sh := posixShell(t)
	shells := filepath.Join(t.TempDir(), "shells")
	old, fresh := filepath.Join(shells, "old"), filepath.Join(shells, "fresh")
	for _, d := range []string{old, fresh} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	then := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, then, then); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(sh, "-c", string(staleShellSweep(shells))).CombinedOutput(); err != nil {
		t.Fatalf("the sweep failed: %v: %s", err, out)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("a launcher two hours old was kept: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("a fresh launcher was removed: %v", err)
	}
	if out, err := exec.Command(sh, "-c", string(staleShellSweep(filepath.Join(shells, "none")))).CombinedOutput(); err != nil {
		t.Errorf("a sweep of a missing directory failed: %v: %s", err, out)
	}
}
