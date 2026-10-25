// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-178: the engine's run state is per boot, and it lives on the disk ------

// markedSection is one `# >>> NAME: begin` to `# <<< NAME: end` section of an
// embedded script, so a case runs the real text.
func markedSection(t *testing.T, script []byte, name string) string {
	t.Helper()
	s := string(script)
	begin := strings.Index(s, "# >>> "+name+": begin\n")
	end := strings.Index(s, "# <<< "+name+": end\n")
	if begin < 0 || end < begin {
		t.Fatalf("the %s section is not marked: begin at %d, end at %d", name, begin, end)
	}
	return s[begin:end]
}

// TestTheRunRootIsNamedOnce: every guest text that names the engine's run
// directory takes it from guestRunRootPrefix, and no shell file types it. A copy
// that moved alone splits the engine's state in two. WSL-178.
func TestTheRunRootIsNamedOnce(t *testing.T) {
	if !strings.Contains(string(guestRuntimePrologue()), "_tk_run="+guestRunRootPrefix+"$_tk_uid\n") {
		t.Errorf("the jobs' prologue does not name the run directory from the constant:\n%s", guestRuntimePrologue())
	}
	if !strings.Contains(userEnvironmentPrelude, "_wtk_run="+guestRunRootPrefix+"$_wtk_uid\n") {
		t.Error("distro run --user-env's prologue does not name the run directory from the constant")
	}
	for name, script := range map[string][]byte{
		"engines.sh": enginesScript, "provision.sh": provisionScript, "verify.sh": verifyScript,
		"stale.sh": staleScript, "repair.sh": repairScript, "binfmt.sh": binfmtScript, "grants.sh": grantsScript,
	} {
		for i, line := range strings.Split(string(script), "\n") {
			if strings.Contains(line, guestRunRootPrefix) && !strings.HasPrefix(strings.TrimSpace(line), "#") {
				t.Errorf("%s:%d types the run directory itself: %s", name, i+1, line)
			}
		}
	}
	b := &Base{cfg: DefaultConfig()}
	env := b.enginesRequest(io.Discard).Env
	if env["TK_RUN_PREFIX"] != guestRunRootPrefix {
		t.Errorf("engines.sh is handed the run directory %q, want %q", env["TK_RUN_PREFIX"], guestRunRootPrefix)
	}
}

// TestTheStartOfABootRunsTheJobsOwnCheck: what engines.sh installs for a start
// is the text `run` sends before its container, byte for byte. WSL-178.
func TestTheStartOfABootRunsTheJobsOwnCheck(t *testing.T) {
	b := &Base{cfg: DefaultConfig()}
	got, err := base64.StdEncoding.DecodeString(b.enginesRequest(io.Discard).Env["TK_RUN_STATE_B64"])
	if err != nil {
		t.Fatalf("TK_RUN_STATE_B64 does not decode: %v", err)
	}
	if !bytes.Equal(got, staleRunStatePayload()) {
		t.Fatalf("the check a start runs is not the one a job runs:\n%s", got)
	}
}

// TestAProbeFailureOverAnotherBootsRecordIsStaleWhateverPodmanSaid: verify.sh's
// run-state row, not podman's wording, makes a failure the stale state that
// `base ensure --repair` clears, through ensure, status and `ready`. WSL-178.
func TestAProbeFailureOverAnotherBootsRecordIsStaleWhateverPodmanSaid(t *testing.T) {
	const words = "Error: this engine's state belongs to an earlier start; remove it and run again"
	stale := verifyError("agent", 125, "engine podman version 9.0.0\nrun-state stale\nuser agent uid 1000\n", words+"\n")
	if stale == nil || !strings.HasPrefix(stale.Error(), "a container did not run as agent (exit 125): "+words) {
		t.Fatalf("a failure over another boot's record read %v", stale)
	}
	r, ok := staleRunStateRemediation(stale.Error())
	if !ok {
		t.Fatalf("a failure over another boot's record, in words podman may use, is not the stale state: %v", stale)
	}
	if _, ok := StaleRunStateRemediation(StaleRefusalError(r, stale).Error()); !ok {
		t.Error("ensure's refusal over it is not read as the stale state by ready")
	}
	current := verifyError("agent", 125, "run-state current\n", words+"\n")
	if _, ok := staleRunStateRemediation(current.Error()); ok {
		t.Errorf("a failure over this boot's record was read as stale: %v", current)
	}
}

// TestTheVerifierReadsTheRunStatesBootBesideThisOne runs verify.sh's row in a
// real shell. ⚠ An absent record, an empty one and this boot's are current, as
// stale.sh reads them. WSL-178.
func TestTheVerifierReadsTheRunStatesBootBesideThisOne(t *testing.T) {
	sh := posixShell(t)
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		t.Skip("this host has no boot id to compare against")
	}
	section := "set -eu\n" + markedSection(t, verifyScript, "the run state's boot")
	for _, c := range []struct {
		name, alive string
		write       bool
		want        string
	}{
		{"another boot's record", "00000000-0000-0000-0000-000000000000\n", true, "run-state stale\n"},
		{"this boot's record", string(raw), true, "run-state current\n"},
		{"no record", "", false, "run-state current\n"},
		{"an empty record", "", true, "run-state current\n"},
	} {
		dir := t.TempDir()
		if c.write {
			if err := os.MkdirAll(filepath.Join(dir, "libpod", "tmp"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "libpod", "tmp", "alive"), []byte(c.alive), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command(sh)
		cmd.Stdin = strings.NewReader(section)
		cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+dir)
		out, err := cmd.Output()
		if err != nil || string(out) != c.want {
			t.Errorf("%s: the row read %q (%v), want %q", c.name, out, err, c.want)
		}
	}
}

// TestEveryBaseGetsTheBootScript: the boot line is written outside the shared
// tmpfs branch, so a base that keeps /mnt/wsl gets the start's run state check
// too. WSL-178.
func TestEveryBaseGetsTheBootScript(t *testing.T) {
	script := string(provisionScript)
	if n := strings.Count(script, "BOOT_LINE="); n != 1 {
		t.Fatalf("the boot line is assigned %d times, want once and to the script", n)
	}
	open := strings.Index(script, `if [ "$TK_SHARED_TMPFS" = off ]; then`)
	boot := strings.Index(script, `BOOT_LINE="command=$BOOT_SCRIPT"`)
	if open < 0 || boot < 0 {
		t.Fatal("the provisioner no longer has the shared tmpfs branch or the boot line")
	}
	end := strings.Index(script[open:], "\nfi\n")
	if end < 0 || open+end > boot {
		t.Fatal("the boot line is written inside the shared tmpfs branch, so a base that keeps /mnt/wsl has no boot script")
	}
}

// bootScriptHost runs provision.sh's boot script with every absolute path under
// a temporary directory, runuser, su and umount stood in for, and the run-state
// check a stand-in that records who ran it.
type bootScriptHost struct {
	t                 *testing.T
	dir, bin, log, rs string
}

func newBootScriptHost(t *testing.T) bootScriptHost {
	t.Helper()
	posixShell(t)
	d := t.TempDir()
	h := bootScriptHost{t: t, dir: d, bin: filepath.Join(d, "bin"), log: filepath.Join(d, "calls"),
		rs: filepath.Join(d, "usr", "local", "lib", "wsl-toolkit", "run-state.sh")}
	for _, sub := range []string{"bin", "run", "etc", "proc", "mnt/wsl", "usr/local/lib/wsl-toolkit"} {
		if err := os.MkdirAll(filepath.Join(d, filepath.FromSlash(sub)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{
		"runuser": `echo "runuser $*" >> "$FAKE_LOG"; shift 3; exec "$@"`,
		"su":      `echo "su $*" >> "$FAKE_LOG"; c=; while [ $# -gt 0 ]; do if [ "$1" = -c ]; then shift; c=$1; fi; shift; done; exec /bin/sh -c "$c"`,
		"umount":  `echo "umount $*" >> "$FAKE_LOG"; exit 0`,
	} {
		writeStandIn(t, h.bin, name, body)
	}
	if err := os.WriteFile(filepath.Join(d, "proc", "mounts"), []byte("none "+filepath.ToSlash(d)+"/mnt/wsl tmpfs rw 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h bootScriptHost) script(seal string) string {
	d := filepath.ToSlash(h.dir)
	s := "seal=" + seal + "\naccount=agent\n" + markedSection(h.t, provisionScript, "the boot script")
	for _, r := range [][2]string{
		{"/run/", d + "/run/"},
		{"/usr/local/lib/", d + "/usr/local/lib/"},
		{"/mnt/wsl", d + "/mnt/wsl"},
		{"/proc/mounts", d + "/proc/mounts"},
		{"/etc/", d + "/etc/"},
	} {
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	return s
}

// run answers the boot script's exit, the calls the stand-ins logged, and its log.
func (h bootScriptHost) run(seal, path string) (int, string, string) {
	h.t.Helper()
	_ = os.Remove(h.log)
	cmd := exec.Command("/bin/sh")
	cmd.Stdin = strings.NewReader(h.script(seal))
	cmd.Env = append(os.Environ(), "PATH="+path, "FAKE_LOG="+h.log)
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			h.t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	calls, _ := os.ReadFile(h.log)
	bootLog, _ := os.ReadFile(filepath.Join(h.dir, "run", "wsl-toolkit-boot.log"))
	return code, string(calls), string(bootLog)
}

// TestTheBootScriptChecksTheRunStateAsTheAccount: at every start, sealed or
// not, the run-state check runs as the account through runuser, or su where
// there is no runuser. A failed check fails no start, and only a sealed base
// unmounts /mnt/wsl. WSL-178, WSL-68.
func TestTheBootScriptChecksTheRunStateAsTheAccount(t *testing.T) {
	h := newBootScriptHost(t)
	rs := filepath.ToSlash(h.rs)
	code, calls, bootLog := h.run("on", "")
	if code != 0 || !strings.Contains(bootLog, "is not written yet") {
		t.Fatalf("a start before base ensure wrote the check: exit %d, log %q", code, bootLog)
	}
	if err := os.WriteFile(h.rs, []byte("echo \"run-state ran\" >> \"$FAKE_LOG\"\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := h.bin + string(os.PathListSeparator) + os.Getenv("PATH")
	code, calls, bootLog = h.run("on", path)
	if code != 0 {
		t.Fatalf("a check that failed failed the start: exit %d", code)
	}
	if !strings.Contains(calls, "runuser -u agent -- /bin/sh "+rs+"\n") || !strings.Contains(calls, "run-state ran") {
		t.Fatalf("an unsealed base's start did not run the check as the account:\n%s", calls)
	}
	if !strings.Contains(bootLog, "run state check exited 3") {
		t.Errorf("the check's failure is not in the boot log: %q", bootLog)
	}
	if strings.Contains(calls, "umount") {
		t.Errorf("a base that keeps /mnt/wsl unmounted it at its start:\n%s", calls)
	}

	if err := os.WriteFile(filepath.Join(h.dir, "mnt", "wsl", "resolv.conf"), []byte("nameserver 192.0.2.53\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, calls, _ = h.run("off", path)
	if code != 0 || !strings.Contains(calls, "umount "+filepath.ToSlash(h.dir)+"/mnt/wsl") || !strings.Contains(calls, "run-state ran") {
		t.Fatalf("a sealed base's start: exit %d, calls:\n%s", code, calls)
	}

	// ⚠ No runuser, as on Alpine: su runs the check as the account.
	bare := filepath.Join(h.dir, "bare")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"su", "umount"} {
		if err := os.Symlink(filepath.Join(h.bin, tool), filepath.Join(bare, tool)); err != nil {
			t.Fatal(err)
		}
	}
	for _, tool := range []string{"date", "grep", "mktemp", "chmod", "mv", "rm"} {
		if p, err := exec.LookPath(tool); err == nil {
			if err := os.Symlink(p, filepath.Join(bare, tool)); err != nil {
				t.Fatal(err)
			}
		}
	}
	code, calls, _ = h.run("on", bare)
	if code != 0 || !strings.Contains(calls, "su -s /bin/sh -c /bin/sh "+rs+" agent\n") || !strings.Contains(calls, "run-state ran") {
		t.Fatalf("a base with no runuser: exit %d, calls:\n%s", code, calls)
	}
}

// TestEnginesShKeepsTheRunStatePerBootInARealShell: every ensure installs the
// job's own check, keeps systemd-tmpfiles' age off the run directory, and on a
// systemd base enables a unit that runs the check as the account at each start.
// Without systemd it says whether wsl.conf names the boot script. WSL-178.
func TestEnginesShKeepsTheRunStatePerBootInARealShell(t *testing.T) {
	h := newEnginesHost(t)
	d := filepath.ToSlash(h.dir)
	const (
		runState = "usr/local/lib/wsl-toolkit/run-state.sh"
		tmpfiles = "etc/tmpfiles.d/wsl-toolkit.conf"
		unit     = "etc/systemd/system/wsl-toolkit-run-state.service"
	)
	out, _ := h.run(false, false, false)
	if h.read(runState) != string(staleRunStatePayload()) {
		t.Errorf("the check a start runs is not the one a job runs:\n%s", h.read(runState))
	}
	if !strings.Contains(h.read(tmpfiles), "\nx "+d+"/tmp/wsl-toolkit-run-*\n") {
		t.Errorf("the tmpfiles rule reads %q", h.read(tmpfiles))
	}
	u := h.read(unit)
	for _, want := range []string{"Type=oneshot\n", "User=" + h.user + "\n", "ExecStart=-/bin/sh " + d + "/" + runState + "\n", "WantedBy=multi-user.target\n"} {
		if !strings.Contains(u, want) {
			t.Errorf("the run-state unit does not carry %q:\n%s", want, u)
		}
	}
	calls, _ := os.ReadFile(h.log)
	if !strings.Contains(string(calls), "systemctl enable wsl-toolkit-run-state.service") || !strings.Contains(out, "clears another boot's record at every start") {
		t.Errorf("the run-state unit was not enabled:\n%s\n%s", calls, out)
	}

	if err := os.RemoveAll(h.path("run/systemd/system")); err != nil {
		t.Fatal(err)
	}
	out, _ = h.run(false, false, false)
	calls, _ = os.ReadFile(h.log)
	if h.read(unit) != "" || !strings.Contains(string(calls), "systemctl disable wsl-toolkit-run-state.service") {
		t.Errorf("a base that no longer runs systemd kept the unit: %q\n%s", h.read(unit), calls)
	}
	if !strings.Contains(out, "names no boot script") {
		t.Errorf("a base whose wsl.conf names no boot script was not told so:\n%s", out)
	}
	h.write(h.path("etc/wsl.conf"), "[boot]\ncommand="+d+"/usr/local/lib/wsl-toolkit/boot.sh\n", 0o644)
	out, _ = h.run(false, false, false)
	if !strings.Contains(out, "the boot script clears another boot's record") {
		t.Errorf("a base whose wsl.conf names the boot script was not told so:\n%s", out)
	}
}
