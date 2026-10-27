// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// -- WSL-178: a restart of the distribution alone keeps the boot id ------------

// startMark is the run state's mark beside its lock.
const startMark = ".wsl-toolkit-run-state.start"

// withProc points repair.sh's scan for a live conmon at a directory the case
// stages, so no container on the host running the suite decides it.
func withProc(t *testing.T, payload []byte, proc string) []byte {
	t.Helper()
	const scan = "for acr_p in /proc/[0-9]*; do"
	if n := bytes.Count(payload, []byte(scan)); n != 1 {
		t.Fatalf("repair.sh's scan for a live conmon occurs %d times, want once", n)
	}
	return bytes.Replace(payload, []byte(scan), []byte("for acr_p in "+filepath.ToSlash(proc)+"/[0-9]*; do"), 1)
}

// stagedConmon is one process of a staged /proc, named conmon and owned by uid.
func stagedConmon(t *testing.T, proc string, uid int) {
	t.Helper()
	p := filepath.Join(proc, "4242")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	u := strconv.Itoa(uid)
	for name, body := range map[string]string{
		"comm":   "conmon\n",
		"status": "Name:\tconmon\nState:\tS (sleeping)\nUid:\t" + u + "\t" + u + "\t" + u + "\t" + u + "\n",
	} {
		if err := os.WriteFile(filepath.Join(p, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestTheStaleCheckClearsAStateAnEarlierStartLeft runs the check a job and each
// start of the base send, in a real shell, over a record that names this boot.
// ⛔ `wsl --terminate` keeps the boot id, so the boot id alone read a dead
// start's state as current. A state held to another start of the base is cleared
// and held to this one. One held to this start, or to none, is current and held
// to it. ⛔ A container of this start keeps the state it runs on. WSL-178.
func TestTheStaleCheckClearsAStateAnEarlierStartLeft(t *testing.T) {
	posixShell(t)
	if _, err := os.Stat("/proc/sys/kernel/random/boot_id"); err != nil {
		t.Skip("this host has no boot id to compare against")
	}
	boot := readTrim(t, "/proc/sys/kernel/random/boot_id")
	initStart := procStart(t, 1)
	n, err := strconv.ParseUint(initStart, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	now := boot + " " + initStart
	another := boot + " " + strconv.FormatUint(n+7, 10)
	emptyProc := t.TempDir()
	checks := withProc(t, runStateChecks(), emptyProc)
	stage := func(mark string) staleHarness {
		h := newStaleHarness(t, boot+"\n", "")
		if mark != "" {
			if err := os.WriteFile(filepath.Join(h.root, startMark), []byte(mark+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return h
	}
	run := func(h staleHarness, payload []byte) (string, string, int) {
		var stdout, stderr bytes.Buffer
		code := exitOf(t, h.command(payload, &stdout, &stderr).Run())
		return stdout.String(), stderr.String(), code
	}
	mark := func(h staleHarness) string {
		b, _ := os.ReadFile(filepath.Join(h.root, startMark))
		return strings.TrimSpace(string(b))
	}

	earlier := stage(another)
	out, stderr, code := run(earlier, checks)
	if code != 0 || !strings.Contains(out, "run-state stale: the engine recorded an earlier start of this base\n") ||
		!strings.Contains(out, "repaired run-state") {
		t.Fatalf("a record of this boot held to another start: exit %d, out %q, stderr %q", code, out, stderr)
	}
	if earlier.present("containers") || earlier.present("libpod/tmp") {
		t.Error("an earlier start's state was answered as cleared and is still there")
	}
	if got := mark(earlier); got != now {
		t.Errorf("the cleared state is held to %q, and this start is %q", got, now)
	}

	held := stage(now)
	if out, _, code = run(held, checks); code != 0 || out != "run-state current\n" || !held.present("containers") || !held.present("libpod/tmp/alive") {
		t.Fatalf("a state held to this start: exit %d, out %q", code, out)
	}

	unmarked := stage("")
	if out, _, code = run(unmarked, checks); code != 0 || out != "run-state current\n" || !unmarked.present("containers") {
		t.Fatalf("a state no check has held: exit %d, out %q", code, out)
	}
	if got := mark(unmarked); got != now {
		t.Errorf("a current state is held to %q, and this start is %q", got, now)
	}

	// ⛔ A conmon of this account is a container of this start, on that state.
	busy := stage(another)
	ownProc := t.TempDir()
	stagedConmon(t, ownProc, os.Getuid())
	out, stderr, code = run(busy, withProc(t, runStateChecks(), ownProc))
	if code != 0 || !strings.Contains(out, "run-state kept: a container of this start of the base runs on it\n") || strings.Contains(out, "removed") {
		t.Fatalf("a state a container of this start runs on: exit %d, out %q, stderr %q", code, out, stderr)
	}
	if !busy.present("containers") || !busy.present("libpod/tmp/alive") || mark(busy) != another {
		t.Error("the state a container of this start runs on was removed or moved to this start")
	}
	// ⚠ Another account's conmon is not on this account's state.
	elsewhere := stage(another)
	otherProc := t.TempDir()
	stagedConmon(t, otherProc, os.Getuid()+1)
	if out, stderr, code = run(elsewhere, withProc(t, runStateChecks(), otherProc)); code != 0 || !strings.Contains(out, "repaired run-state") {
		t.Fatalf("another account's container kept this account's state: exit %d, out %q, stderr %q", code, out, stderr)
	}

	// ⛔ base ensure --repair clears it too, where it read a record of this boot as
	// another caller's clear.
	repair := stage(another)
	out, stderr, code = run(repair, withProc(t, repairPayload(), emptyProc))
	if code != 0 || !strings.Contains(out, "repaired run-state") || repair.present("containers") || mark(repair) != now {
		t.Fatalf("base ensure --repair over an earlier start's state: exit %d, out %q, stderr %q, mark %q", code, out, stderr, mark(repair))
	}
}

// TestBaseEnsureRepairSendsBootmark: `base ensure --repair` sends repair.sh after
// bootmark, so it holds what it cleared to this start. WSL-178.
func TestBaseEnsureRepairSendsBootmark(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	// ⚠ A staged guest is one that records what it was handed.
	h.stageGuest(t, "verify=ok")
	_, _ = b.repairRunState(context.Background())
	if !strings.Contains(h.guestScripts(t), string(guestRuntimePrologue())+sessionProcStart+string(repairScript)) {
		t.Fatal("base ensure --repair did not send the prologue, bootmark and repair.sh, in that order")
	}
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
	// ⭐ ONE TEXT: provisioning writes boot.sh's, which it is handed, as every
	// ensure hands engines.sh. WSL-178.
	if !strings.Contains(script, `  printf '%s' "$TK_BOOT_B64" | base64 -d`+"\n") {
		t.Error("provisioning does not write the boot script from the text it is handed")
	}
	b := &Base{cfg: DefaultConfig()}
	for name, env := range map[string]map[string]string{
		"provisioning": b.provisionRequest(AutomountOff, BaseInteropOff, BaseToolsetDeveloper, SharedTmpfsOn, io.Discard).Env,
		"engines.sh":   b.enginesRequest(io.Discard).Env,
	} {
		if got, err := base64.StdEncoding.DecodeString(env["TK_BOOT_B64"]); err != nil || !bytes.Equal(got, bootScript) {
			t.Errorf("%s is not handed boot.sh's text (%v)", name, err)
		}
	}
	// ⛔ THE OLD NAME GOES AFTER THE wsl.conf THAT NAMES boot.sh IS IN PLACE. A
	// provisioning that died between the two left a sealed base naming a script
	// that was gone. WSL-178.
	installed := strings.Index(script, "wsl_conf=$(install_changed /etc/wsl.conf.wsl-toolkit.new /etc/wsl.conf)")
	removed := strings.Index(script, "rm -f /usr/local/lib/wsl-toolkit/seal-boot.sh")
	if installed < 0 || removed < 0 || removed < installed {
		t.Errorf("provisioning removes seal-boot.sh at %d and installs wsl.conf at %d", removed, installed)
	}
}

// TestProvisioningWritesTheBootScriptItIsHanded runs provision.sh's install of
// the boot script in a real shell: the base's two lines, then boot.sh's text
// whole. ⛔ A text cut short is refused, and the script there is kept. WSL-178.
func TestProvisioningWritesTheBootScriptItIsHanded(t *testing.T) {
	posixShell(t)
	d := t.TempDir()
	section := strings.ReplaceAll(markedSection(t, provisionScript, "the boot script's install"), "/usr/local/lib/", filepath.ToSlash(d)+"/usr/local/lib/")
	boot := filepath.Join(d, "usr", "local", "lib", "wsl-toolkit", "boot.sh")
	run := func(text []byte) (string, error) {
		cmd := exec.Command("/bin/sh")
		cmd.Stdin = strings.NewReader("set -eu\nsay() { printf '%s\\n' \"$*\"; }\ndie() { printf 'provision: %s\\n' \"$*\" >&2; exit 3; }\n" + section)
		cmd.Env = append(os.Environ(), "TK_SHARED_TMPFS=off", "TK_USER=agent", "TK_BOOT_B64="+base64.StdEncoding.EncodeToString(text))
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run(bootScript); err != nil {
		t.Fatalf("the boot script's install failed: %v\n%s", err, out)
	}
	want := "#!/bin/sh\nseal=off\naccount=agent\n" + string(bootScript)
	got, err := os.ReadFile(boot)
	if err != nil || string(got) != want {
		t.Fatalf("provisioning wrote the boot script as %q (%v)", got, err)
	}
	if info, err := os.Stat(boot); err != nil || info.Mode().Perm() != 0o755 {
		t.Errorf("the boot script's mode is %v (%v)", info.Mode().Perm(), err)
	}
	out, err := run(bootScript[:len(bootScript)/2])
	if err == nil || !strings.Contains(out, "cut short") {
		t.Fatalf("a boot script cut short was installed: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(boot); string(got) != want {
		t.Error("a boot script cut short replaced the one there")
	}
}

// bootScriptHost runs boot.sh's text, as provisioning and engines.sh write it,
// with every absolute path under a temporary directory, runuser, su and umount
// stood in for, and the run-state check a stand-in that records who ran it.
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
	s := "seal=" + seal + "\naccount=agent\n" + markedSection(h.t, bootScript, "the boot script")
	for _, r := range [][2]string{
		// ⚠ The grants' lock and WSL's mount helper are the case's own. WSL-198.
		{"gb_helper=/sbin/mount.drvfs\n", "gb_helper=" + d + "/sbin/mount.drvfs\n"},
		{"gb_link=/usr/local/bin/mount.drvfs\n", "gb_link=" + d + "/usr/local/bin/mount.drvfs\n"},
		{"gb_mount_bin=/bin/mount\n", "gb_mount_bin=" + d + "/bin/mount\n"},
		{"/var/lib/", d + "/var/lib/"},
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
// Without systemd the unit goes, and the boot script runs the check. WSL-178.
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
	if strings.Contains(out, "clears another boot's record at every start") {
		t.Errorf("a base that no longer runs systemd was told a unit clears its record:\n%s", out)
	}
}

// renderedWslConf is /etc/wsl.conf as provision.sh's own template renders it,
// with bootLine where 8.0.0 puts the boot line and 7.x put the seal's, and
// resolv as the sealed resolver block or nothing.
func renderedWslConf(t *testing.T, user, bootLine, resolv string) string {
	t.Helper()
	s := string(provisionScript)
	const open = "cat > /etc/wsl.conf.wsl-toolkit.new <<CONF\n"
	begin := strings.Index(s, open)
	if begin < 0 {
		t.Fatal("provision.sh no longer writes wsl.conf from its template")
	}
	body := s[begin+len(open):]
	end := strings.Index(body, "\nCONF\n")
	if end < 0 {
		t.Fatal("provision.sh's wsl.conf template has no end")
	}
	cmd := exec.Command("/bin/sh")
	cmd.Stdin = strings.NewReader("cat <<CONF\n" + body[:end] + "\nCONF\n")
	cmd.Env = append(os.Environ(), "TK_USER="+user, "SYSTEMD_ENABLED=false", "BOOT_LINE="+bootLine,
		"AUTOMOUNT_BLOCK=enabled=false", "INTEROP_ENABLED=false", "RESOLV_BLOCK="+resolv)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TestEnginesShGivesAnEarlierBaseTheBootScript: every owner's ensure writes the
// boot script and the wsl.conf line that names it, where provisioning puts it, so
// a base an earlier release built gets them with no rebuild and no restart.
// ⛔ The next provisioning then finds wsl.conf unchanged. ⛔ The seal is the
// base's own, and the old script goes only once nothing names it. WSL-178.
func TestEnginesShGivesAnEarlierBaseTheBootScript(t *testing.T) {
	h := newEnginesHost(t)
	d := filepath.ToSlash(h.dir)
	const (
		conf    = "etc/wsl.conf"
		boot    = "usr/local/lib/wsl-toolkit/boot.sh"
		oldBoot = "usr/local/lib/wsl-toolkit/seal-boot.sh"
	)
	line := "command=" + d + "/" + boot
	oldLine := "command=" + d + "/" + oldBoot
	const sealedResolv = "\n[network]\ngenerateResolvConf=false"
	script := func(seal string) string {
		return "#!/bin/sh\nseal=" + seal + "\naccount=" + h.user + "\n" + string(bootScript)
	}

	// An unsealed base 7.x built: an empty line where the boot line goes.
	h.write(h.path(conf), renderedWslConf(t, h.user, "", ""), 0o644)
	out, _ := h.run(false, false, false)
	if got, want := h.read(conf), renderedWslConf(t, h.user, line, ""); got != want {
		t.Errorf("an unsealed earlier base's wsl.conf is not what provisioning writes:\n%q\nwant\n%q", got, want)
	}
	if h.read(boot) != script("on") {
		t.Errorf("an unsealed earlier base got the boot script\n%q", h.read(boot))
	}
	if !strings.Contains(out, "names "+d+"/"+boot+" from the next start") {
		t.Errorf("the boot line was not said:\n%s", out)
	}
	if out, _ = h.run(false, false, false); !strings.Contains(out, "names "+d+"/"+boot+", which runs at every start") {
		t.Errorf("a base that has the boot line was not told so:\n%s", out)
	}

	// A sealed base 7.x built: its line names seal-boot.sh, and the seal stays.
	h.write(h.path(conf), renderedWslConf(t, h.user, oldLine, sealedResolv), 0o644)
	h.write(h.path(oldBoot), "#!/bin/sh\n", 0o755)
	if err := os.Remove(h.path(boot)); err != nil {
		t.Fatal(err)
	}
	h.run(false, false, false)
	if got, want := h.read(conf), renderedWslConf(t, h.user, line, sealedResolv); got != want {
		t.Errorf("a sealed earlier base's wsl.conf is not what provisioning writes:\n%q\nwant\n%q", got, want)
	}
	if h.read(boot) != script("off") {
		t.Errorf("a sealed earlier base lost its seal:\n%q", h.read(boot))
	}
	if _, err := os.Stat(h.path(oldBoot)); err == nil {
		t.Error("seal-boot.sh outlived the line that named it")
	}

	// ⛔ The seal the boot script names is kept, whatever wsl.conf says.
	h.write(h.path(conf), renderedWslConf(t, h.user, "", sealedResolv), 0o644)
	h.run(false, false, false)
	if h.read(boot) != script("off") || !strings.Contains(h.read(conf), "\n"+line+"\n") {
		t.Errorf("a base whose boot script names its seal: wsl.conf %q, script %q", h.read(conf), h.read(boot))
	}

	// ⛔ A file this tool did not write is left, and so is the script it names.
	foreign := "[boot]\n" + oldLine + "\n"
	h.write(h.path(conf), foreign, 0o644)
	h.write(h.path(oldBoot), "#!/bin/sh\n", 0o755)
	if out, _ = h.run(false, false, false); h.read(conf) != foreign || !strings.Contains(out, "is not this tool's") {
		t.Errorf("a wsl.conf this tool did not write: %q\n%s", h.read(conf), out)
	}
	if _, err := os.Stat(h.path(oldBoot)); err != nil {
		t.Error("seal-boot.sh was removed while wsl.conf still named it")
	}
	other := "# Written by wsl-toolkit.\n[boot]\nsystemd=false\ncommand=/opt/elsewhere.sh\n"
	h.write(h.path(conf), other, 0o644)
	if out, _ = h.run(false, false, false); h.read(conf) != other || !strings.Contains(out, "names another boot command") {
		t.Errorf("a wsl.conf naming another boot command: %q\n%s", h.read(conf), out)
	}
}
