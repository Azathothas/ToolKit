// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// tarModes reads back what an upload actually put on the wire, because the
// claim under test is about the bytes a guest receives and not about a counter.
func tarModes(t *testing.T, blob []byte) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	r := tar.NewReader(bytes.NewReader(blob))
	for {
		h, err := r.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("reading the archive: %v", err)
		}
		out[h.Name] = h.Mode
		_, _ = io.Copy(io.Discard, r)
	}
}

// TestTheExecutableBitSurvivesAFilesystemThatCannotHoldIt is the case for the
// workaround issue 29 exists to remove.
//
// ⛔ NTFS HOLDS NO POSIX MODE, so every script in a Windows checkout arrived at
// 0644 and the first one to run failed with `Permission denied` naming the
// script rather than the transfer. A consumer measured 396 of 396 needing
// repair and shipped a `restore-modes.sh` plus a wrapper to call it.
func TestTheExecutableBitSurvivesAFilesystemThatCannotHoldIt(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH, and the git index is what this reads")
	}
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("run.sh", "#!/bin/sh\necho hello\n")
	write("notes.md", "# not a script\n")
	write("data.json", "{}\n")

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", "run.sh", "notes.md", "data.json")
	// The mode the INDEX carries is the claim. On this host the working tree
	// cannot carry it, which is the whole problem.
	git("update-index", "--chmod=+x", "run.sh")

	var buf bytes.Buffer
	if _, err := writeWorkspaceTar(&buf, root, DefaultWorkspaceLimits(), []string{".git"}); err != nil {
		t.Fatalf("the upload failed: %v", err)
	}
	modes := tarModes(t, buf.Bytes())
	if got := modes["run.sh"]; got&0o111 == 0 {
		t.Errorf("run.sh arrived %#o; the git index marks it executable", got)
	}
	if got := modes["notes.md"]; got&0o111 != 0 {
		t.Errorf("notes.md arrived %#o; nothing marks it executable and marking data executable is the failure mode to avoid", got)
	}
	if got := modes["data.json"]; got&0o111 != 0 {
		t.Errorf("data.json arrived %#o; nothing marks it executable", got)
	}
}

// TestAnUnstagedScriptStillArrivesRunnable covers the gap the index alone
// leaves.
//
// ⚠ MEASURED BY A CONSUMER ON 2026-09-12: a new script written and not yet
// staged is in no index, so a repair that reads the index reported 396 of 396
// files fixed in the same run that failed `Permission denied`, rc 126. The file
// declares itself with `#!` and that is what is read.
func TestAnUnstagedScriptStillArrivesRunnable(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "new.sh"), []byte("#!/usr/bin/env bash\ntrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plain.txt"), []byte("#not a shebang\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	up, err := writeWorkspaceTar(&buf, root, DefaultWorkspaceLimits(), nil)
	if err != nil {
		t.Fatal(err)
	}
	modes := tarModes(t, buf.Bytes())
	if got := modes["new.sh"]; got&0o111 == 0 {
		t.Errorf("new.sh arrived %#o; it carries a shebang", got)
	}
	if got := modes["plain.txt"]; got&0o111 != 0 {
		t.Errorf("plain.txt arrived %#o; `#` alone is not a shebang", got)
	}
	// ⛔ A mode this tool supplied is ANNOUNCED. The alternative is the blind
	// `chmod -R +x` that reports nothing.
	if !strings.Contains(up.ExecRestored, "shebang") {
		t.Errorf("the upload did not report the restored bit: %q", up.ExecRestored)
	}
}

// TestAFileThatGrowsDoesNotKillTheCopy is the case for a defect measured
// against a live index daemon.
//
// ⛔ A TAR MEMBER'S SIZE GOES INTO ITS HEADER BEFORE ITS BYTES ARE READ, so an
// unbounded copy of a file that grew overran the declared length and the
// archiver refused with `archive/tar: write too long`. That names the archiver
// and not the file, and the whole job exited 2 in 475 ms. The consumer worked
// around it by excluding four sidecars by name.
//
// ⛔ NO WRITER RACES THE COPY HERE, AND THE FIRST VERSION OF THIS CASE DID.
// It appended from a goroutine and asserted that the file had grown by the time
// the copy finished. That passed on Windows and failed on Linux, where the walk
// completed in under a millisecond and the goroutine was never scheduled in
// between: the case asserted a truncation that had not happened, on a machine
// that was behaving correctly. ⚠ A test whose subject is a gap between two
// moments has to CONTROL both moments. Handing in the FileInfo from before the
// growth is exactly the gap, with no timing in it.
func TestAFileThatGrowsDoesNotKillTheCopy(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "daemon.log")
	if err := os.WriteFile(p, bytes.Repeat([]byte("a"), 64), 0o644); err != nil {
		t.Fatal(err)
	}
	// The stat the walker would have taken.
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// The daemon appends, after the header's size is settled and before the
	// bytes are read.
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(bytes.Repeat([]byte("b"), 4096)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	var up WorkspaceUpload
	if err := writeRegularMember(tw, &up, p, "daemon.log", info, 0o644); err != nil {
		t.Fatalf("a growing file killed the copy: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	// ⛔ NOT COUNTED AS AN OMISSION. The file travelled; a prefix of a file
	// something is still writing is a snapshot, not a missing input, and folding
	// the two together makes the serious number go up for the ordinary case.
	if up.Omitted != 0 {
		t.Errorf("omitted = %d, want 0: the file arrived", up.Omitted)
	}
	if up.Truncated != 1 {
		t.Errorf("truncated = %d, want 1", up.Truncated)
	}
	// The archive is whole, and it holds exactly what the header declared.
	r := tar.NewReader(bytes.NewReader(buf.Bytes()))
	h, err := r.Next()
	if err != nil {
		t.Fatalf("the archive is not readable: %v", err)
	}
	if h.Size != info.Size() {
		t.Errorf("the member declares %d bytes, want the %d that were stat'ed", h.Size, info.Size())
	}
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("the member is not readable: %v", err)
	}
	if int64(len(body)) != info.Size() {
		t.Errorf("the member carries %d bytes, want %d", len(body), info.Size())
	}
	if bytes.ContainsRune(body, 'b') {
		t.Error("the member carries bytes appended after its size was settled")
	}
}

// TestAutomountDefaultsToReadOnly holds the ruling that a job cannot destroy
// the checkout on the Windows host through /mnt.
func TestAutomountDefaultsToReadOnly(t *testing.T) {
	got, err := NormalizeAutomount("")
	if err != nil {
		t.Fatal(err)
	}
	if got != AutomountReadOnly {
		t.Errorf("the default automount is %q, want %q", got, AutomountReadOnly)
	}
	for _, ok := range []string{AutomountReadOnly, AutomountReadWrite, AutomountOff} {
		if _, err := NormalizeAutomount(ok); err != nil {
			t.Errorf("%q was refused: %v", ok, err)
		}
	}
	if _, err := NormalizeAutomount("read-only"); err == nil {
		t.Error("a spelling that is not one of the three was accepted")
	}
}

// TestTheProvisionerReadsTheAutomountSetting asserts the embedded script has a
// branch for each value, because a value the Go side accepts and the script
// does not is a base that fails at provisioning time.
func TestTheProvisionerReadsTheAutomountSetting(t *testing.T) {
	for _, want := range []string{"TK_AUTOMOUNT", "options=\"metadata,ro\"", "enabled=false"} {
		if !strings.Contains(string(provisionScript), want) {
			t.Errorf("the provisioner does not carry %q", want)
		}
	}
}

// TestPasswordlessSudoIsValidatedBeforeActivation holds the opt-in path to
// validation before activation and to a live non-interactive privilege check.
func TestPasswordlessSudoIsValidatedBeforeActivation(t *testing.T) {
	for _, want := range []string{
		"TK_PASSWORDLESS_SUDO",
		`visudo -cf "$sudoers_tmp"`,
		`rm -f "$sudoers_path"`,
	} {
		if !strings.Contains(string(provisionScript), want) {
			t.Errorf("the provisioner does not carry %q", want)
		}
	}
	if !strings.Contains(string(verifyScript), "sudo -n true") {
		t.Error("the verifier does not exercise passwordless sudo as the configured account")
	}
}

// verifierSudoSection is the verifier's own passwordless sudo section.
func verifierSudoSection(t *testing.T) string {
	t.Helper()
	script := string(verifyScript)
	begin := strings.Index(script, "# >>> passwordless sudo: begin\n")
	end := strings.Index(script, "# <<< passwordless sudo: end\n")
	if begin < 0 || end < begin {
		t.Fatalf("the verifier's passwordless sudo section is not marked: begin at %d, end at %d", begin, end)
	}
	return "set -eu\n" + script[begin:end]
}

// TestTheVerifierReadsTheSudoItPromises runs the verifier's sudo section through a
// real POSIX shell, with a sudo written here that answers as a rule would.
//
// ⛔ WSL-85. A base built with passwordless sudo and set to false kept the rule, and
// `base status --probe` answered healthy and printed `passwordless false`, because
// the verifier checked nothing for false. Measured on a throwaway arch base on
// 2026-09-15.
func TestTheVerifierReadsTheSudoItPromises(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the verifier is read by a POSIX shell, and this host has none to hand it to")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh on this host")
	}
	const (
		granted = "exit 0"
		refused = "echo 'sudo: a password is required' >&2; exit 1"
		absent  = ""
	)
	cases := []struct {
		name, setting, sudo string
		exit                int
		refusal             string
	}{
		{"a rule kept under false, the measured defect", "false", granted, 3,
			"the configured account can use sudo without a password, and passwordless sudo is off"},
		{"no rule under false", "false", refused, 0, ""},
		{"no sudo under false", "false", absent, 0, ""},
		{"a rule under true", "true", granted, 0, ""},
		{"no rule under true", "true", refused, 3, "the configured account cannot use sudo without a password"},
		{"no sudo under true", "true", absent, 3, "passwordless sudo was requested and sudo is absent"},
		{"a setting the script does not know", "yes", granted, 3, "unknown passwordless sudo setting yes"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		if c.sudo != absent {
			if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte("#!/bin/sh\n"+c.sudo+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command("/bin/sh", "-c", verifierSudoSection(t))
		// ⛔ PATH IS THIS DIRECTORY ALONE. A runner whose own account has passwordless
		// sudo would otherwise answer every case from its real sudo.
		cmd.Env = []string{"PATH=" + dir, "TK_PASSWORDLESS_SUDO=" + c.setting}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var exited *exec.ExitError
			if !errors.As(err, &exited) {
				t.Fatalf("%s: %v", c.name, err)
			}
			code = exited.ExitCode()
		}
		if code != c.exit {
			t.Errorf("%s: exit %d, want %d; stderr %q", c.name, code, c.exit, stderr.String())
		}
		if c.exit == 0 && stdout.String() != "passwordless-sudo "+c.setting+"\n" {
			t.Errorf("%s: stdout %q, want the setting's row", c.name, stdout.String())
		}
		if c.refusal == "" && stderr.Len() > 0 {
			t.Errorf("%s: wrote to stderr over sudo that agrees: %q", c.name, stderr.String())
		}
		if !strings.Contains(stderr.String(), c.refusal) {
			t.Errorf("%s: stderr %q does not name %q", c.name, stderr.String(), c.refusal)
		}
	}
}

// TestTheProvisionerScopesEveryRestrictiveUmask is the case for a mask that
// leaked. The sudoers candidate took `umask 077` at the top level of the script,
// so every later step inherited it: a fresh build left /workspaces at 0700 and
// /etc/fstab at 0600, and the configured account could not reach its own
// checkout. Replayed on Arch with GNU mkdir on 2026-09-13.
func TestTheProvisionerScopesEveryRestrictiveUmask(t *testing.T) {
	setsItsOwn := false
	for i, line := range strings.Split(string(provisionScript), "\n") {
		code := strings.TrimSpace(line)
		if strings.HasPrefix(code, "#") || !strings.Contains(code, "umask") {
			continue
		}
		if code == "umask 022" {
			setsItsOwn = true
			continue
		}
		if !strings.HasPrefix(code, "( umask ") {
			t.Errorf("provision.sh line %d changes the creation mask outside a subshell, so every later step inherits it: %s", i+1, code)
		}
	}
	if !setsItsOwn {
		t.Error("provision.sh does not set its own creation mask, so the modes it creates depend on whatever mask wsl.exe handed it")
	}
}

// TestTheVerifierSeparatesAnUnreachableMountFromAnAbsentOne holds the order of
// two checks. `[ -d ]` over a target whose parent this account cannot enter
// fails exactly as a missing mount does, so the traversal check has to come
// first or a present mount is still reported absent.
func TestTheVerifierSeparatesAnUnreachableMountFromAnAbsentOne(t *testing.T) {
	// ⚠ The printed messages, not the words: the comment above the check says
	// "cannot enter" too, and matching that would pass with the check deleted.
	script := string(verifyScript)
	enter := strings.Index(script, "printf 'verify: this account cannot enter %s")
	absent := strings.Index(script, "printf 'verify: explicit mount %s is absent")
	if enter < 0 {
		t.Fatal("the verifier does not name a directory above a mount that this account cannot enter")
	}
	if absent < 0 || enter > absent {
		t.Fatal("the traversal check does not run before the absence check, so an unreachable mount is still reported absent")
	}
}

// TestTheVerifierReadsTheEngineVersionFromStdoutAlone is the case for a report
// that published a warning as a version. After a restart podman writes a stale
// pause.pid notice to stderr, and the merged stream made `base status` answer
// "pause.pid file refers to PID 47 ..." in its engine field.
func TestTheVerifierReadsTheEngineVersionFromStdoutAlone(t *testing.T) {
	script := string(verifyScript)
	if strings.Contains(script, "podman --version 2>&1") {
		t.Fatal("the verifier merges podman's stderr into the engine version it reports")
	}
	if !strings.Contains(script, "printf 'engine %s\\n' \"$(podman --version 2>/dev/null") {
		t.Fatal("the verifier no longer reports the engine version from podman's stdout")
	}
}

// TestAutomountOffLeavesNoDriveMountPoints is the case for a drive that read as
// absent in the configuration and present in the guest. A fresh import's first
// start automounts every Windows drive before provisioning writes `automount
// off`, and the restart left nine empty 0777 directories: `ls /mnt/c` succeeded
// and listed nothing, which is not what the setting promises. Measured on a
// rebuilt Arch base on 2026-09-13.
func TestAutomountOffLeavesNoDriveMountPoints(t *testing.T) {
	provision := string(provisionScript)
	for _, want := range []string{
		"\ncd /\n",
		`for drive_dir in /mnt/?; do`,
		`umount "$drive_dir" 2>/dev/null || umount -l "$drive_dir"`,
		`rmdir "$drive_dir" 2>/dev/null || die`,
	} {
		if !strings.Contains(provision, want) {
			t.Errorf("the provisioner does not carry %q", want)
		}
	}
	if !strings.Contains(string(verifyScript), `if [ -e "$drive_dir" ]; then`) {
		t.Error("the verifier does not refuse a drive mount point that automount off should have left absent")
	}
}

// verifierDriveSection is the verifier's own drive section, reading a mounts table
// and a drive root under dir in place of /proc/mounts and /mnt.
//
// ⛔ EACH PATH IS REPLACED EXACTLY ONCE, OR THE CASE STOPS. A section that stopped
// naming them would otherwise read this host's own /proc/mounts and /mnt.
func verifierDriveSection(t *testing.T, mounts, root string) string {
	t.Helper()
	script := string(verifyScript)
	begin := strings.Index(script, "# >>> the windows drives: begin\n")
	end := strings.Index(script, "# <<< the windows drives: end\n")
	if begin < 0 || end < begin {
		t.Fatalf("the verifier's drive section is not marked: begin at %d, end at %d", begin, end)
	}
	section := script[begin:end]
	// ⚠ THE ROOT IS THE FALLBACK LINE, NOT AN ASSIGNMENT. verify.sh reads
	// `[automount] root` from /etc/wsl.conf and falls back to /mnt, so there is
	// no `drive_root=/mnt` line to substitute any more. Pinning the fallback is
	// what gives this case a root it can build a mounts table under, and the
	// count assertion below is what turns a moved line into a refusal rather
	// than a case that silently drives the real /mnt.
	for from, to := range map[string]string{
		"\ndrive_mounts=/proc/mounts\n":                 "\ndrive_mounts='" + mounts + "'\n",
		"\n[ -n \"$drive_root\" ] || drive_root=/mnt\n": "\ndrive_root='" + root + "'\n",
	} {
		if n := strings.Count(section, from); n != 1 {
			t.Fatalf("the drive section names %q %d times, want once", strings.TrimSpace(from), n)
		}
		section = strings.Replace(section, from, to, 1)
	}
	return "set -eu\n" + section
}

// TestTheVerifierReadsTheDrivesItPromises runs the verifier's drive section through
// a real POSIX shell over a mounts table written here.
//
// ⛔ WSL-84. A base changed from rw to ro kept /mnt/c mounted `9p rw`, and the
// verifier printed `automount ro` and passed, because it checked nothing for ro or
// rw. Measured on a throwaway arch base on 2026-09-14, with each direction below.
func TestTheVerifierReadsTheDrivesItPromises(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the verifier is read by a POSIX shell, and this host has none to hand it to")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh on this host")
	}
	const (
		readOnly = "ro,noatime,aname=drvfs;path=X:\\;uid=1000;gid=1000;metadata"
		writable = "rw,noatime,aname=drvfs;path=X:\\;uid=1000;gid=1000;metadata"
		tmpfs    = "rw,relatime,inode64"
	)
	type mount struct{ target, options string }
	var (
		cWritable = mount{"c", writable}
		dWritable = mount{"d", writable}
		cReadOnly = mount{"c", readOnly}
		dReadOnly = mount{"d", readOnly}
		wslOwn    = mount{"wsl", tmpfs}
		wslgOwn   = mount{"wslg/distro", tmpfs}
		belowC    = mount{"c/work", writable}
	)
	cases := []struct {
		name    string
		setting string
		mounts  []mount
		dirs    []string
		exit    int
		row     string
		refusal string
	}{
		{"writable drives under ro, the measured defect", AutomountReadOnly,
			[]mount{cWritable, dWritable}, nil, 3, AutomountReadWrite,
			"automount is ro, and below /mnt 0 mount(s) are read-only and 2 writable"},
		{"no drive under ro, as after off", AutomountReadOnly,
			nil, nil, 3, AutomountOff,
			"automount is ro, and below /mnt 0 mount(s) are read-only and 0 writable"},
		{"a read-only drive under off, as after ro", AutomountOff,
			[]mount{cReadOnly}, nil, 3, AutomountReadOnly,
			"a Windows drive is mounted below /mnt even though automount is off"},
		{"read-only drives under ro, beside WSL's own writable mounts", AutomountReadOnly,
			[]mount{cReadOnly, dReadOnly, wslOwn, wslgOwn}, nil, 0, AutomountReadOnly, ""},
		{"writable drives under rw", AutomountReadWrite,
			[]mount{cWritable}, nil, 0, AutomountReadWrite, ""},
		{"a read-only drive under rw", AutomountReadWrite,
			[]mount{cReadOnly}, nil, 3, AutomountReadOnly,
			"automount is rw, and below /mnt 1 mount(s) are read-only and 0 writable"},
		{"a writable mount below a read-only drive", AutomountReadOnly,
			[]mount{cReadOnly, belowC}, nil, 3, AutomountMixed,
			"automount is ro, and below /mnt 1 mount(s) are read-only and 1 writable"},
		{"an empty mount point under off", AutomountOff,
			nil, []string{"c"}, 3, AutomountOff, "c exists even though automount is off"},
		{"nothing under off", AutomountOff,
			nil, []string{"wsl"}, 0, AutomountOff, ""},
		{"a setting the script does not know", "read-only",
			nil, nil, 3, AutomountOff, "unknown automount setting read-only"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		if strings.ContainsAny(dir, " \t'") {
			t.Skipf("the temporary directory %q cannot be a field in a mounts table", dir)
		}
		root := filepath.Join(dir, "mnt")
		for _, d := range append([]string{""}, c.dirs...) {
			if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		var table strings.Builder
		fmt.Fprintf(&table, "none %s/wsl tmpfs rw,relatime 0 0\n", dir)
		for _, m := range c.mounts {
			fmt.Fprintf(&table, "X:\\134 %s/%s 9p %s 0 0\n", root, m.target, m.options)
		}
		mounts := filepath.Join(dir, "mounts")
		if err := os.WriteFile(mounts, []byte(table.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("/bin/sh", "-c", verifierDriveSection(t, mounts, root))
		cmd.Env = []string{"PATH=/usr/bin:/bin", "TK_AUTOMOUNT=" + c.setting}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var exited *exec.ExitError
			if !errors.As(err, &exited) {
				t.Fatalf("%s: %v", c.name, err)
			}
			code = exited.ExitCode()
		}
		if code != c.exit {
			t.Errorf("%s: exit %d, want %d; stderr %q", c.name, code, c.exit, stderr.String())
		}
		if got := parseAutomount(stdout.String()); got != c.row {
			t.Errorf("%s: the automount row read %q, want %q; stdout %q", c.name, got, c.row, stdout.String())
		}
		if c.refusal == "" && stderr.Len() > 0 {
			t.Errorf("%s: wrote to stderr over drives that agree: %q", c.name, stderr.String())
		}
		// ⚠ THE ROOT IS SUBSTITUTED INTO THE EXPECTATION. The refusals name the
		// root they measured rather than a hardcoded /mnt, because verify.sh
		// reads it from wsl.conf now: a distribution adopted with a hand-set
		// root had its drives mounted where the old check was not looking.
		want := strings.ReplaceAll(c.refusal, "/mnt", root)
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("%s: stderr %q does not name %q", c.name, stderr.String(), want)
		}
	}
}

// TestAVerifierRefusalNamesTheSettingAndNotAContainer is the message a drive refusal
// gives. It is made before a container runs, and read "a container did not run as
// toolkit (exit 3)" when WSL-84 was first driven, which sends a reader after the
// engine. An engine failure keeps the engine's words, which a remediation reads.
func TestAVerifierRefusalNamesTheSettingAndNotAContainer(t *testing.T) {
	const drives = "automount is ro, and below /mnt 0 mount(s) are read-only and 10 writable"
	got := verifyError("toolkit", verifyRefused, "automount rw\n", "verify: "+drives+"\n")
	if got == nil || got.Error() != "the base does not match its configuration, checked as toolkit: "+drives {
		t.Errorf("the drive refusal read %v", got)
	}
	engine := "Error: current system boot ID differs from cached boot ID; an unclean shutdown may have occurred. " +
		"Delete /tmp/wsl-toolkit-run-1000/containers and /tmp/wsl-toolkit-run-1000/libpod/tmp"
	stale := verifyError("toolkit", 125, "", engine+"\n")
	if stale == nil || !strings.HasPrefix(stale.Error(), "a container did not run as toolkit (exit 125): Error: current system boot ID") {
		t.Errorf("an engine failure read %v", stale)
	} else if _, ok := staleRunStateRemediation(stale.Error()); !ok {
		t.Errorf("an engine failure no longer carries the words its remediation reads: %v", stale)
	}
	if other := verifyError("toolkit", verifyRefused, "", "sh: podman: not found\n"); other == nil || !strings.HasPrefix(other.Error(), "a container did not run") {
		t.Errorf("an exit 3 with no verify line was read as a refusal: %v", other)
	}
	if timeout := verifyError("toolkit", 124, "automount ro\n", "verify: "+drives+"\n"); timeout == nil || !strings.HasPrefix(timeout.Error(), "a container did not run as toolkit (exit 124)") {
		t.Errorf("a verification ended by its deadline was read as a refusal: %v", timeout)
	}
}

// TestAVerifierRefusalIsReadPastWslsOwnLines is the refusal WSL-85 measured. After a
// restart wsl.exe wrote its own line ahead of the verifier's on the same stream, and
// `base ensure` over a sudo rule it could not remove said "a container did not run as
// agent (exit 3): wsl: Failed to start the systemd user session for 'agent'".
func TestAVerifierRefusalIsReadPastWslsOwnLines(t *testing.T) {
	const (
		session = "wsl: Failed to start the systemd user session for 'agent'. See journalctl for more details."
		sudo    = "the configured account can use sudo without a password, and passwordless sudo is off"
		engine  = "Error: current system boot ID differs from cached boot ID; an unclean shutdown may have occurred."
	)
	refused := verifyError("agent", verifyRefused, "automount off\n", session+"\nverify: "+sudo+"\n")
	if refused == nil || refused.Error() != "the base does not match its configuration, checked as agent: "+sudo {
		t.Errorf("a refusal after wsl.exe's own line read %v", refused)
	}
	failed := verifyError("agent", 125, "", session+"\r\n"+engine+"\n")
	if failed == nil || failed.Error() != "a container did not run as agent (exit 125): "+engine {
		t.Errorf("an engine failure after wsl.exe's own line read %v", failed)
	} else if _, ok := staleRunStateRemediation(failed.Error()); !ok {
		t.Errorf("an engine failure after wsl.exe's own line lost the words its remediation reads: %v", failed)
	}
	if only := verifyError("agent", 1, "", session+"\n"); only == nil || only.Error() != "a container did not run as agent (exit 1): "+session {
		t.Errorf("a failure with nothing but wsl.exe's line named nothing: %v", only)
	}
}

// TestTheWindowsDriveMountsAreReadFromTheMountTable holds the root shell's source.
// It read the one-letter directories under /mnt and called each drive writable, in a
// base whose drives were all mounted `9p ro`. The lines below are the shapes measured
// in the default base and a throwaway on 2026-09-14.
func TestTheWindowsDriveMountsAreReadFromTheMountTable(t *testing.T) {
	const options = "noatime,aname=drvfs;path=X:\\;uid=1000;gid=1000;metadata;symlinkroot=/mnt/,cache=0x5,access=client,msize=65536,trans=fd,rfd=5,wfd=5"
	table := strings.Join([]string{
		"none /mnt/wsl tmpfs rw,relatime,inode64 0 0",
		"/dev/sdf /mnt/wslg/distro ext4 ro,relatime,discard,errors=remount-ro,data=ordered 0 0",
		"C:\\134 /mnt/c 9p ro," + options + " 0 0",
		"D:\\134 /mnt/d 9p rw," + options + " 0 0",
		"none /mnt/c/work tmpfs rw,relatime 0 0",
		"none /workspaces/project 9p rw," + options + " 0 0",
		"short line",
	}, "\n")
	got := parseWindowsDriveMounts(table)
	want := []WindowsDriveMount{
		{Target: "/mnt/c", ReadOnly: true},
		{Target: "/mnt/d", ReadOnly: false},
		{Target: "/mnt/c/work", ReadOnly: false},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("read %+v, want %+v", got, want)
	}
}

// TestTheVerifiersAutomountRowIsReadByItsValue holds what the report takes from the
// verifier: a value the script prints, and never an empty or unknown one read as off.
func TestTheVerifiersAutomountRowIsReadByItsValue(t *testing.T) {
	for out, want := range map[string]string{
		"interop off\nautomount rw\nengine podman version 6.1.1\n": AutomountReadWrite,
		"automount mixed\n":           AutomountMixed,
		"automount off\n":             AutomountOff,
		"  automount ro\r\n":          AutomountReadOnly,
		"automount read-only\n":       "",
		"automount \n":                "",
		"engine podman version 6.1.1": "",
	} {
		if got := parseAutomount(out); got != want {
			t.Errorf("%q read %q, want %q", out, got, want)
		}
	}
	if _, err := NormalizeAutomount(AutomountMixed); err == nil {
		t.Error("mixed, a measurement of the drives, was accepted as a setting")
	}
}

func TestManagedAccountGetsPrivateXDGDirectories(t *testing.T) {
	for _, want := range []string{
		`"$TK_HOME/.config" "$TK_HOME/.cache" "$TK_HOME/.local" "$TK_HOME/.local/share" "$TK_HOME/.local/state"`,
		`chmod 0700 "$TK_HOME"`,
	} {
		if !strings.Contains(string(provisionScript), want) {
			t.Errorf("the provisioner does not carry %q", want)
		}
	}
	if !strings.Contains(string(verifyScript), "xdg-dirs ready") {
		t.Error("the verifier does not report the managed account's XDG directories")
	}
}

// TestANativeJobNamesItsPlatform is the case for a defect that only driving the
// real thing could have found.
//
// ⛔ AN EMPTY PLATFORM MEANT NO `--platform` ON THE PODMAN COMMAND LINE, so
// podman selected whatever variant of the image the local store already held.
// Measured on this host on 2026-09-12: after one `--platform linux/arm64` run of
// `alpine`, a later run that asked for NOTHING reported `aarch64` from `uname
// -m` on an x86_64 machine. The only sign was a warning on podman's stderr,
// which a caller reading the JSON answer never sees.
//
// ⚠ A green suite could not see this. Every unit test passed over it, because
// the defect is in what the local image store happens to contain and not in the
// code's own logic.
func TestANativeJobNamesItsPlatform(t *testing.T) {
	native := NativePlatform()
	if native == "" {
		t.Fatal("the native platform resolved to nothing, which is the defect this exists to hold closed")
	}
	got, err := NormalizePlatform(native)
	if err != nil {
		t.Fatalf("the native platform %q is not one this tool accepts: %v", native, err)
	}
	if got != native {
		t.Errorf("the native platform normalizes to %q, not to itself", got)
	}
}

// TestASubdirectoryWorkspaceReadsItsOwnPaths pins the assumption the git lookup
// rests on.
//
// ⚠ A WORKSPACE IS OFTEN NOT THE REPOSITORY ROOT. `git ls-files` reports paths
// relative to the directory it runs in, not to the top of the working tree, and
// the walker's own keys are relative to the workspace. The two agree, and this
// is the case that says so rather than a comment claiming it: `--full-name`
// exists precisely because the other behaviour is available, so a later edit
// reaching for it would silently stop matching every path.
func TestASubdirectoryWorkspaceReadsItsOwnPaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH, and the git index is what this reads")
	}
	root := t.TempDir()
	sub := filepath.Join(root, "tools", "inner")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deep.sh"), []byte("true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(root, "init", "-q")
	git(root, "add", "tools/inner/deep.sh")
	git(root, "update-index", "--chmod=+x", "tools/inner/deep.sh")

	// The workspace is the SUBDIRECTORY, not the repository root.
	var buf bytes.Buffer
	if _, err := writeWorkspaceTar(&buf, sub, DefaultWorkspaceLimits(), nil); err != nil {
		t.Fatal(err)
	}
	if got := tarModes(t, buf.Bytes())["deep.sh"]; got&0o111 == 0 {
		t.Errorf("deep.sh arrived %#o; the index marks it executable and the workspace is its own directory", got)
	}
}

// TestTheHelperRouteAlsoNamesAPlatform closes the sibling door.
//
// ⛔ ONE ROUTE ENFORCING WHAT ITS SIBLING DOES NOT is the shape that produced
// the defect in the first place: `--script` repaired CRLF and `-c` did not. The
// direct route resolves an unnamed platform to the native one, and a request
// from an older client or from a caller writing the JSON by hand reaches the
// helper's own fallback instead.
func TestTheHelperRouteAlsoNamesAPlatform(t *testing.T) {
	var emptyConfig Config
	if got := effectiveJobPlatform(HelperRunRequest{}, emptyConfig); got != NativePlatform() {
		t.Errorf("a request naming no platform resolved to %q, want the native %q", got, NativePlatform())
	}
	// What the caller asked for is never overwritten, and neither is the
	// configured default.
	if got := effectiveJobPlatform(HelperRunRequest{Platform: "linux/arm64"}, emptyConfig); got != "linux/arm64" {
		t.Errorf("a request naming linux/arm64 resolved to %q", got)
	}
	cfg := Config{Jobs: JobConfig{Platform: "linux/s390x"}}
	if got := effectiveJobPlatform(HelperRunRequest{}, cfg); got != "linux/s390x" {
		t.Errorf("the configured platform was ignored: %q", got)
	}
}
