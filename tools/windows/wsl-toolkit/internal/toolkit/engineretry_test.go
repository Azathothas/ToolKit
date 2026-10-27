// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
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

// stoppingTimeout stands in for timeout. It records its arguments, answers as a
// timeout that stopped its command for the first TK_FAILS calls, with TK_CODE,
// and runs the command after that.
const stoppingTimeout = logStandIn + `
n=0
if [ -f "$TK_COUNT" ]; then read -r n < "$TK_COUNT"; fi
n=$((n + 1))
printf '%s\n' "$n" > "$TK_COUNT"
if [ "$n" -le "$TK_FAILS" ]; then exit "$TK_CODE"; fi
shift
exec "$@"`

// runEngineSection runs the engine section for one family over a stand-in PATH,
// and answers what it said, the calls the stand-ins recorded and its exit.
func runEngineSection(t *testing.T, shell, section, family, dir string, env ...string) (string, []string, int) {
	t.Helper()
	log := filepath.Join(dir, "calls.txt")
	harness := "set -eu\n" +
		"say() { printf 'say: %s\\n' \"$*\" >&2; }\n" +
		"die() { printf 'die: %s\\n' \"$*\" >&2; exit 3; }\n" +
		"FAMILY=" + family + "\n" + section
	cmd := exec.Command(shell, "-c", harness)
	cmd.Env = append([]string{"PATH=" + dir, "TK_LOG=" + log, "TK_COUNT=" + filepath.Join(dir, "count")}, env...)
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

// engineManagers maps each family to the program its transaction runs first.
var engineManagers = map[string]string{
	"apk": "apk", "pacman": "pacman", "apt": "apt-get", "dnf": "dnf", "tdnf": "tdnf", "xbps": "xbps-install",
}

// TestAManagerThatHangsIsStoppedAndNamed runs the engine section with a timeout
// that stops the manager. ⛔ A first build of the alpine preset hung twice in
// `apk update`, and nothing bounded it. GNU timeout answers 124, and BusyBox's
// leaves the TERM, 143. WSL-203.
func TestAManagerThatHangsIsStoppedAndNamed(t *testing.T) {
	shell := posixShell(t)
	section := provisionerSection(t, "engine packages")
	for family, manager := range engineManagers {
		for _, stopped := range []string{"124", "143"} {
			dir := t.TempDir()
			for _, m := range engineManagers {
				writeStandIn(t, dir, m, logStandIn)
			}
			writeStandIn(t, dir, "sleep", logStandIn)
			writeStandIn(t, dir, "timeout", stoppingTimeout)
			said, calls, code := runEngineSection(t, shell, section, family, dir, "TK_FAILS=1", "TK_CODE="+stopped)
			if code != 0 {
				t.Errorf("%s, %s: one stopped call ended the build, exit %d: %s", family, stopped, code, said)
				continue
			}
			named := "attempt 1 of 3: " + manager + " ran past 900s and was stopped. Trying again in 15s"
			if !strings.Contains(said, named) {
				t.Errorf("%s, %s: the stop was not named: %s", family, stopped, said)
			}
			// ⛔ Every manager call, passt's included, came through the bound: the
			// line before each is timeout's own record of the same call.
			seen := 0
			for i, call := range calls {
				if !strings.HasPrefix(call, manager+" ") {
					continue
				}
				seen++
				if i == 0 || calls[i-1] != "timeout 900 "+call {
					t.Errorf("%s: %q ran without the bound: %v", family, call, calls)
				}
			}
			if seen < 2 {
				t.Errorf("%s: the manager ran %d times, want the transaction and passt: %v", family, seen, calls)
			}
		}
		dir := t.TempDir()
		for _, m := range engineManagers {
			writeStandIn(t, dir, m, logStandIn)
		}
		writeStandIn(t, dir, "sleep", logStandIn)
		writeStandIn(t, dir, "timeout", stoppingTimeout)
		said, _, code := runEngineSection(t, shell, section, family, dir, "TK_FAILS=99", "TK_CODE=124")
		if code != 3 || !strings.Contains(said, "in 3 attempts. The last one ended: "+manager+" ran past 900s and was stopped") {
			t.Errorf("%s: a manager stopped every time: exit %d, %s", family, code, said)
		}
	}
}

// TestTheRealTimeoutStopsAHungManager hands the section this host's own timeout
// and a manager that never answers, with the bound at one second, so the stop is
// a real one and not a stand-in's answer.
func TestTheRealTimeoutStopsAHungManager(t *testing.T) {
	shell := posixShell(t)
	realTimeout, err := exec.LookPath("timeout")
	if err != nil {
		t.Skip("this host carries no timeout")
	}
	realSleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("this host carries no sleep")
	}
	section := provisionerSection(t, "engine packages")
	short := strings.Replace(section, "ENGINE_STEP_SECONDS=900\n", "ENGINE_STEP_SECONDS=1\n", 1)
	if short == section {
		t.Fatal("the engine section sets no ENGINE_STEP_SECONDS=900")
	}
	dir := t.TempDir()
	writeStandIn(t, dir, "apk", logStandIn+"\nexec \""+realSleep+"\" 60")
	writeStandIn(t, dir, "sleep", logStandIn)
	writeStandIn(t, dir, "timeout", "exec \""+realTimeout+"\" \"$@\"")
	said, calls, code := runEngineSection(t, shell, short, "apk", dir)
	if code != 3 || !strings.Contains(said, "in 3 attempts. The last one ended: apk ran past 1s and was stopped") {
		t.Errorf("a hung apk: exit %d, %s", code, said)
	}
	if !standInListHas(calls, "sleep 30") {
		t.Errorf("the stopped attempts were not asked again: %v", calls)
	}
}

// clockStandIn stands in for date, answering TK_NOW, so the run's deadline is
// read against a clock that does not move.
const clockStandIn = `echo "$TK_NOW"`

// TestNoManagerCallOutlivesTheProvisioningRun runs the engine section with the
// run's deadline close. ⛔ The executable stops provisioning after 30 minutes,
// and three bounded tries of 900 seconds outlast that. A stalled alpine mirror's
// second try was cut there with no name, and the base was rolled back. WSL-204.
func TestNoManagerCallOutlivesTheProvisioningRun(t *testing.T) {
	shell := posixShell(t)
	section := provisionerSection(t, "engine packages")
	run := func(withTimeout bool, deadline, fails string) (string, []string, int) {
		t.Helper()
		dir := t.TempDir()
		for _, m := range engineManagers {
			writeStandIn(t, dir, m, logStandIn)
		}
		writeStandIn(t, dir, "sleep", logStandIn)
		writeStandIn(t, dir, "date", clockStandIn)
		if withTimeout {
			writeStandIn(t, dir, "timeout", stoppingTimeout)
		}
		return runEngineSection(t, shell, section, "apk", dir, "TK_FAILS="+fails, "TK_CODE=124", "TK_NOW=1000", "provision_deadline="+deadline)
	}
	ran := func(calls []string) bool {
		for _, call := range calls {
			if strings.HasPrefix(call, "apk ") || strings.HasPrefix(call, "timeout ") {
				return true
			}
		}
		return false
	}

	if said, calls, code := run(true, "1300", "0"); code != 0 || !standInListHas(calls, "timeout 300 apk update") {
		t.Errorf("with 300s left, the bound was not cut to them: exit %d, %v: %s", code, calls, said)
	}
	if said, calls, code := run(true, "5000", "0"); code != 0 || !standInListHas(calls, "timeout 900 apk update") {
		t.Errorf("with 4000s left, the bound was not its own 900s: exit %d, %v: %s", code, calls, said)
	}
	// ⛔ A stopped try with no time left for the pause is named, and no other try
	// is waited for.
	said, calls, code := run(true, "1010", "99")
	if code != 3 || !strings.Contains(said, "no time left for attempt 2. The last one ended: apk ran past 10s and was stopped") {
		t.Errorf("a try cut at the deadline: exit %d, %s", code, said)
	}
	if standInListHas(calls, "sleep 15") {
		t.Errorf("a try the run had no time for was waited for: %v", calls)
	}
	// ⛔ A call the run has no time left for is not started, with timeout or without it.
	for _, withTimeout := range []bool{true, false} {
		said, calls, code := run(withTimeout, "1000", "0")
		if code != 3 || !strings.Contains(said, "The last one ended: apk was not started: provisioning has no time left") || ran(calls) {
			t.Errorf("timeout %v, no time left: exit %d, %v: %s", withTimeout, code, calls, said)
		}
	}
}

// TestTheRunDeadlineIsTheRunsLimitLessItsMargin runs the deadline section with a
// clock that does not move, and without a limit. WSL-204.
func TestTheRunDeadlineIsTheRunsLimitLessItsMargin(t *testing.T) {
	shell := posixShell(t)
	section := provisionerSection(t, "run deadline")
	dir := t.TempDir()
	writeStandIn(t, dir, "date", clockStandIn)
	for _, c := range []struct{ limit, want string }{{"1800", "2680"}, {"", ""}} {
		cmd := exec.Command(shell, "-c", "set -eu\n"+section+"printf '%s' \"$provision_deadline\"\n")
		cmd.Env = []string{"PATH=" + dir, "TK_NOW=1000"}
		if c.limit != "" {
			cmd.Env = append(cmd.Env, "TK_PROVISION_SECONDS="+c.limit)
		}
		out, err := cmd.Output()
		if err != nil || string(out) != c.want {
			t.Errorf("a limit of %q: deadline %q, %v, want %q", c.limit, out, err, c.want)
		}
	}
}

// TestProvisioningHandsTheScriptItsOwnLimit holds the provisioning run's limit
// and the one the script cuts its bounds to as one value. WSL-204.
func TestProvisioningHandsTheScriptItsOwnLimit(t *testing.T) {
	b := &Base{cfg: Config{Base: BaseConfig{Name: "wsl-toolkit-tbl", User: "agent"}}}
	req := b.provisionRequest(AutomountOff, BaseInteropOff, BaseToolsetDeveloper, SharedTmpfsOn, io.Discard)
	if req.Timeout <= 0 || req.Env["TK_PROVISION_SECONDS"] != strconv.Itoa(int(req.Timeout/time.Second)) {
		t.Fatalf("the run stops after %v and hands the script TK_PROVISION_SECONDS=%q", req.Timeout, req.Env["TK_PROVISION_SECONDS"])
	}
}

// boundedManagerCall is a package manager's install or refresh, the calls that
// reach a mirror.
var boundedManagerCall = regexp.MustCompile(`\b(apk (add|update|upgrade)|pacman -S|apt-get (install|update|upgrade)|dnf -y|tdnf install|xbps-install)\b`)

// TestEveryPackageManagerCallInTheProvisionerIsBounded reads provision.sh for a
// manager call that reaches a mirror without bounded in front of it. WSL-203.
func TestEveryPackageManagerCallInTheProvisionerIsBounded(t *testing.T) {
	seen := 0
	for i, line := range strings.Split(string(provisionScript), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		at := boundedManagerCall.FindStringIndex(line)
		if at == nil {
			continue
		}
		seen++
		if !strings.HasSuffix(line[:at[0]], "bounded ") {
			t.Errorf("provision.sh:%d calls a package manager without bounded: %s", i+1, strings.TrimSpace(line))
		}
	}
	if seen < 30 {
		t.Errorf("found %d package manager calls in provision.sh; the pattern no longer reads them", seen)
	}
}
