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
)

// stallingManager is a package manager stand-in that fails its first TK_FAILS
// calls the way a stalled mirror does, and installs nothing after that. passt is
// optional and never fails.
const stallingManager = logStandIn + `
case "$*" in *passt*) exit 0 ;; esac
n=0
if [ -f "$TK_COUNT" ]; then read -r n < "$TK_COUNT"; fi
n=$((n + 1))
printf '%s\n' "$n" > "$TK_COUNT"
if [ "$n" -le "$TK_FAILS" ]; then
  printf 'warning: a line the manager wrote first\n' >&2
  printf "error: failed retrieving file 'core.db' from geo.mirror.pkgbuild.com : Operation too slow\n" >&2
  exit 1
fi
exit 0`

// TestTheEnginePackagesAreAskedAgainAndTheStallNamed runs provision.sh's engine
// section for every family with a manager that stalls. ⛔ Finding 10: two builds
// failed in pacman on `Operation too slow` and rolled back, and the next attempt
// passed, because nothing retried. WSL-140.
func TestTheEnginePackagesAreAskedAgainAndTheStallNamed(t *testing.T) {
	shell := posixShell(t)
	section := provisionerSection(t, "engine packages")
	stall := "error: failed retrieving file 'core.db' from geo.mirror.pkgbuild.com : Operation too slow"
	run := func(family string, fails string) (string, []string, int) {
		t.Helper()
		dir := t.TempDir()
		for _, manager := range []string{"apk", "pacman", "apt-get", "dnf", "tdnf", "xbps-install"} {
			writeStandIn(t, dir, manager, stallingManager)
		}
		writeStandIn(t, dir, "sleep", logStandIn)
		log := filepath.Join(dir, "calls.txt")
		harness := "set -eu\n" +
			"say() { printf 'say: %s\\n' \"$*\" >&2; }\n" +
			"die() { printf 'die: %s\\n' \"$*\" >&2; exit 3; }\n" +
			"FAMILY=" + family + "\n" + section
		cmd := exec.Command(shell, "-c", harness)
		cmd.Env = []string{"PATH=" + dir, "TK_LOG=" + log, "TK_COUNT=" + filepath.Join(dir, "count"), "TK_FAILS=" + fails}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var exited *exec.ExitError
			if !errors.As(err, &exited) {
				t.Fatalf("%s: %v", family, err)
			}
			code = exited.ExitCode()
		}
		b, _ := os.ReadFile(log)
		return stderr.String(), strings.Split(strings.TrimSpace(string(b)), "\n"), code
	}
	for _, family := range []string{"apk", "pacman", "apt", "dnf", "tdnf", "xbps"} {
		said, calls, code := run(family, "1")
		if code != 0 {
			t.Errorf("%s: one stall ended the build, exit %d: %s", family, code, said)
			continue
		}
		if !strings.Contains(said, "attempt 1 of 3: "+stall+". Trying again in 15s") {
			t.Errorf("%s: the stall was not named: %s", family, said)
		}
		if !standInListHas(calls, "sleep 15") {
			t.Errorf("%s: the second attempt did not wait first: %v", family, calls)
		}
		said, calls, code = run(family, "99")
		if code != 3 || !strings.Contains(said, "in 3 attempts. The last one ended: "+stall) {
			t.Errorf("%s: a manager that never answered: exit %d, %s", family, code, said)
		}
		if !standInListHas(calls, "sleep 30") {
			t.Errorf("%s: the third attempt did not wait longer: %v", family, calls)
		}
	}
}
