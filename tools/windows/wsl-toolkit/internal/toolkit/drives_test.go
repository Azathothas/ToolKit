// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
)

// indexOf is where a call first appears in a fake's sequence, or -1.
func indexOf(calls []string, want string) int {
	for i, c := range calls {
		if c == want {
			return i
		}
	}
	return -1
}

// TestDriveMountPointsAreSweptAfterTheRestart builds a base with automount off
// through the fake WSL and holds the order. ⛔ The sweep ran inside provision.sh,
// before the restart: a drive mounted after it was left behind (finding 18,
// WSL-155), and every session before the restart relayed one `wsl: Failed to
// translate` line per Windows PATH entry (finding 13, WSL-153). After the restart
// nothing is mounted under the sweep and nothing is translated.
func TestDriveMountPointsAreSweptAfterTheRestart(t *testing.T) {
	f := newBuildFakeWsl(t)
	b := f.base(buildFakeConfig("wsl-toolkit-sweep", AutomountOff), nil)
	b.archive = writeTinyArchive
	if _, err := b.EnsureWith(t.Context(), false, false); err != nil {
		t.Fatalf("the build failed: %v; calls %v", err, f.state().scripts())
	}
	calls := f.state().scripts()
	grants, restart, sweep, verify := indexOf(calls, "grants@root"), indexOf(calls, "--terminate"),
		indexOf(calls, "drives@root"), indexOf(calls, "verify@agent")
	if grants < 0 || restart < grants || sweep < restart || verify < sweep {
		t.Fatalf("grants at %d, the restart at %d, the sweep at %d and verify at %d; calls %v", grants, restart, sweep, verify, calls)
	}
	if strings.Contains(string(provisionScript), "for drive_dir in /mnt/?") {
		t.Error("provision.sh still sweeps the drive mount points before the restart")
	}

	// ⚠ A base whose drives are mounted has no mount points to sweep.
	g := newBuildFakeWsl(t)
	ro := g.base(buildFakeConfig("wsl-toolkit-sweep", AutomountReadOnly), nil)
	ro.archive = writeTinyArchive
	if _, err := ro.EnsureWith(t.Context(), false, false); err != nil {
		t.Fatal(err)
	}
	if indexOf(g.state().scripts(), "drives@root") >= 0 {
		t.Error("a base with its drives mounted read only had its drive mount points swept")
	}
}

// TestAReprovisionThatOwesNoRestartStillSweeps re-provisions a registered base
// whose start settings did not change, so WSL-175 skips its restart. ⚠ The sweep
// ran inside provision.sh on every run before it moved to drives.sh, so it still
// runs on that path, before the base is verified again. WSL-153.
func TestAReprovisionThatOwesNoRestartStillSweeps(t *testing.T) {
	f := newBuildFakeWsl(t)
	b := f.base(buildFakeConfig("wsl-toolkit-keep", AutomountOff), nil)
	f.update(func(s *buildFakeState) {
		s.Distros["wsl-toolkit-keep"] = &buildFakeDistro{Name: "wsl-toolkit-keep", Dir: b.Dir()}
		s.Unchanged = true
		s.VerifyRefusals = 1
	})
	if err := f.w.WriteIdentity(t.Context(), "wsl-toolkit-keep", Identity{Image: b.cfg.Base.Image, User: "agent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.EnsureWith(t.Context(), false, false); err != nil {
		t.Fatalf("the re-provision failed: %v; calls %v", err, f.state().scripts())
	}
	calls := f.state().scripts()
	if indexOf(calls, "--terminate") >= 0 {
		t.Fatalf("a re-provision that changed no start setting restarted the base; calls %v", calls)
	}
	grants, sweep := indexOf(calls, "grants@root"), indexOf(calls, "drives@root")
	last := -1
	for i, c := range calls {
		if c == "verify@agent" {
			last = i
		}
	}
	if grants < 0 || sweep < grants || last < sweep {
		t.Fatalf("grants at %d, the sweep at %d and the last verify at %d; calls %v", grants, sweep, last, calls)
	}
}

// TestTheSweepRemovesOnlyEmptyDriveLetters runs drives.sh in a real shell over a
// staged /mnt and mounts table.
func TestTheSweepRemovesOnlyEmptyDriveLetters(t *testing.T) {
	posixShell(t)
	run := func(mnt string) (string, string, int) {
		t.Helper()
		mounts := filepath.Join(t.TempDir(), "mounts")
		if err := forksafe.WriteFile(mounts, []byte("none / ext4 rw 0 0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		script := string(drivesScript)
		for from, to := range map[string]string{"for drive_dir in /mnt/?;": "for drive_dir in " + mnt + "/?;", "done < /proc/mounts": "done < " + mounts} {
			if strings.Count(script, from) != 1 {
				t.Fatalf("drives.sh names %q %d times, want once", from, strings.Count(script, from))
			}
			script = strings.Replace(script, from, to, 1)
		}
		cmd := exec.Command("/bin/sh", "-c", script)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var exited *exec.ExitError
			if !errors.As(err, &exited) {
				t.Fatal(err)
			}
			code = exited.ExitCode()
		}
		return stdout.String(), stderr.String(), code
	}
	mnt := t.TempDir()
	for _, d := range []string{"c", "e", "wsl"} {
		if err := os.Mkdir(filepath.Join(mnt, d), 0o777); err != nil {
			t.Fatal(err)
		}
	}
	out, stderr, code := run(mnt)
	if code != 0 || !strings.Contains(out, "drive mount points removed: 2") || !strings.Contains(out, "drives-complete") {
		t.Fatalf("exit %d, out %q, stderr %q", code, out, stderr)
	}
	for _, gone := range []string{"c", "e"} {
		if _, err := os.Stat(filepath.Join(mnt, gone)); err == nil {
			t.Errorf("/mnt/%s is still there", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(mnt, "wsl")); err != nil {
		t.Error("a directory that is not a drive letter was removed")
	}
	full := t.TempDir()
	if err := os.MkdirAll(filepath.Join(full, "d", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := run(full); code != 3 || !strings.Contains(stderr, "is not an empty directory") {
		t.Fatalf("a drive letter holding files answered exit %d, %q", code, stderr)
	}
}
