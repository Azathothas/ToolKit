// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// grantsRestart is a restart of the base a drive of grants.sh left: every live
// mount ends, and each drvfs line of dir's fstab is mounted again in its mode, as
// WSL mounts /etc/fstab when the distribution starts. WSL-190.
func grantsRestart(t *testing.T, dir string) {
	t.Helper()
	fstab, err := os.ReadFile(filepath.Join(dir, "fstab"))
	if err != nil {
		t.Fatal(err)
	}
	var mounts strings.Builder
	for _, line := range strings.Split(string(fstab), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || strings.HasPrefix(f[0], "#") || f[2] != "drvfs" {
			continue
		}
		mode := ""
		for _, o := range strings.Split(f[3], ",") {
			if o == "rw" || o == "ro" {
				mode = o
			}
		}
		fmt.Fprintf(&mounts, "%s %s drvfs %s,noatime 0 0\n", f[0], f[1], mode)
	}
	if err := os.WriteFile(filepath.Join(dir, "mounts"), []byte(mounts.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// blockLinesAt are the lines of dir's fstab that mount at target.
func blockLinesAt(t *testing.T, dir, target string) []string {
	t.Helper()
	fstab, _ := os.ReadFile(filepath.Join(dir, "fstab"))
	var at []string
	for _, line := range strings.Split(string(fstab), "\n") {
		if f := strings.Fields(line); len(f) > 1 && !strings.HasPrefix(f[0], "#") && f[1] == target {
			at = append(at, line)
		}
	}
	return at
}

// blockLine is the fstab line renderBaseMounts writes for one grant.
func blockLine(t *testing.T, m BaseMount) string {
	t.Helper()
	fstab64, _, err := renderBaseMounts([]BaseMount{m})
	if err != nil {
		t.Fatal(err)
	}
	line, err := base64.StdEncoding.DecodeString(fstab64)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(string(line), "\n")
}

// ownedBy is a grant ledger row giving target to file.
func ownedBy(target, file string) string { return target + " " + b64(file) + " " + b64(file) + "\n" }

// blockOf is an fstab holding the tool's block with these lines.
func blockOf(lines ...string) string {
	return "# wsl-toolkit mounts begin\n" + strings.Join(append(lines, ""), "\n") + "# wsl-toolkit mounts end\n"
}

// grantsTheirs is the other configuration file the drives below grant from.
const grantsTheirs = "/cfg/theirs.json"

// TestARestartMountsEveryFilesGrants drives a grant from each of two
// configuration files and then a restart. ⛔ The fstab block held the last
// caller's grants alone, so the restart mounted the second file's grant and
// dropped the first, and with automount off the first file's ensure then refused,
// or with --repair restarted over the second. Both come back. WSL-190.
func TestARestartMountsEveryFilesGrants(t *testing.T) {
	grantsShell(t)
	dir := t.TempDir()
	read := grantsSeed(t, dir, "", "", "")
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"}
	theirs := BaseMount{Source: t.TempDir(), Target: "/workspaces/theirs", Mode: "rw"}
	if out, err := grantsDriveAs(t, dir, GrantsLive, []BaseMount{mine}, b64(mine.Target), grantsMine); err != nil {
		t.Fatalf("this file's grant failed: %v\n%s", err, out)
	}
	if out, err := grantsDriveAs(t, dir, GrantsLive, []BaseMount{theirs}, b64(theirs.Target), grantsTheirs); err != nil {
		t.Fatalf("the other file's grant failed: %v\n%s", err, out)
	}
	grantsRestart(t, dir)
	mounts := read("mounts")
	if !strings.Contains(mounts, liveLine(mine.Source, mine.Target, "rw")) || !strings.Contains(mounts, liveLine(theirs.Source, theirs.Target, "rw")) {
		t.Fatalf("after a restart the mounts are:\n%s\nfrom the fstab:\n%s", mounts, read("fstab"))
	}
}

// TestAnEnsureLeavesABlockTwoFilesShareAsItWas drives each file's provisioning
// write over a block two files' grants made. ⛔ Each says the block is unchanged,
// so neither restarts the base: the block keeps its order and names each target
// once. A block written kept-first would read as changed on every other file's
// ensure. WSL-175, WSL-190.
func TestAnEnsureLeavesABlockTwoFilesShareAsItWas(t *testing.T) {
	grantsShell(t)
	dir := t.TempDir()
	read := grantsSeed(t, dir, "LABEL=root / ext4 defaults 0 1\n", "", "")
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"}
	theirs := BaseMount{Source: t.TempDir(), Target: "/workspaces/theirs", Mode: "rw"}
	if out, err := grantsDriveAs(t, dir, GrantsLive, []BaseMount{mine}, b64(mine.Target), grantsMine); err != nil {
		t.Fatalf("this file's grant failed: %v\n%s", err, out)
	}
	if out, err := grantsDriveAs(t, dir, GrantsLive, []BaseMount{theirs}, b64(theirs.Target), grantsTheirs); err != nil {
		t.Fatalf("the other file's grant failed: %v\n%s", err, out)
	}
	shared := read("fstab")
	for _, w := range []struct {
		owner string
		mount BaseMount
	}{{grantsMine, mine}, {grantsTheirs, theirs}} {
		out, err := grantsDriveAs(t, dir, GrantsWrite, []BaseMount{w.mount}, "", w.owner)
		if err != nil || !hasLine(out, "fstab unchanged") {
			t.Fatalf("%s's write over the shared block answered %v:\n%s\nfstab:\n%s", w.owner, err, out, read("fstab"))
		}
		if got := read("fstab"); got != shared {
			t.Fatalf("%s's write changed the shared block:\n%s\nwant:\n%s", w.owner, got, shared)
		}
	}
	want := "LABEL=root / ext4 defaults 0 1\n" + blockOf(blockLine(t, mine), blockLine(t, theirs))
	if shared != want {
		t.Fatalf("the shared block is:\n%s\nwant:\n%s", shared, want)
	}
}

// TestAWriteTakesTheTargetsItsConfigurationNames drives provisioning over a block
// whose line at a target this configuration names the ledger gives another file.
// ⛔ The block names the target once, as this file grants it, and the ledger row
// moves with it. Two lines would mount two directories there. WSL-190.
func TestAWriteTakesTheTargetsItsConfigurationNames(t *testing.T) {
	grantsShell(t)
	dir := t.TempDir()
	theirs := BaseMount{Source: t.TempDir(), Target: "/workspaces/s", Mode: "ro"}
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/s", Mode: "rw"}
	read := grantsSeed(t, dir, blockOf(blockLine(t, theirs)), "", ownedBy(theirs.Target, grantsTheirs))
	if out, err := grantsDriveAs(t, dir, GrantsWrite, []BaseMount{mine}, "", grantsMine); err != nil {
		t.Fatalf("the write failed: %v\n%s", err, out)
	}
	if got := blockLinesAt(t, dir, mine.Target); len(got) != 1 || got[0] != blockLine(t, mine) {
		t.Fatalf("after the write the block mounts %s with %q, want this file's line alone:\n%s", mine.Target, got, read("fstab"))
	}
	if got := read("owners"); got != ownedBy(mine.Target, grantsMine) {
		t.Fatalf("after the write the ledger is:\n%s", got)
	}
}

// TestALiveGrantTakesItsTargetFromAnotherFilesLine drives a grant of a target
// whose block line the ledger gives another file that does not have it mounted.
// ⛔ The grant's own line replaces it, so the mount comes from this file's
// directory, now and after a restart. WSL-190.
func TestALiveGrantTakesItsTargetFromAnotherFilesLine(t *testing.T) {
	grantsShell(t)
	dir := t.TempDir()
	theirs := BaseMount{Source: t.TempDir(), Target: "/workspaces/s", Mode: "rw"}
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/s", Mode: "rw"}
	read := grantsSeed(t, dir, blockOf(blockLine(t, theirs)), "", ownedBy(theirs.Target, grantsTheirs))
	if out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target)); err != nil {
		t.Fatalf("the grant failed: %v\n%s", err, out)
	}
	if got := blockLinesAt(t, dir, mine.Target); len(got) != 1 || got[0] != blockLine(t, mine) {
		t.Fatalf("after the grant the block mounts %s with %q:\n%s", mine.Target, got, read("fstab"))
	}
	if got := read("mounts"); got != liveLine(mine.Source, mine.Target, "rw") {
		t.Fatalf("the grant mounted:\n%s", got)
	}
	grantsRestart(t, dir)
	if got := read("mounts"); got != liveLine(mine.Source, mine.Target, "rw") {
		t.Fatalf("after a restart the mounts are:\n%s", got)
	}
}

// TestARevokeTakesItsTargetsLineWithItsRow drives a revoke of a target this file's
// configuration names while the ledger gives it to another file that does not
// have it mounted. ⛔ The revoke's ledger step drops that row, so the line goes
// with it: kept, it would mount at the next restart a grant no file holds. The
// other file's line at another target stays. WSL-190.
func TestARevokeTakesItsTargetsLineWithItsRow(t *testing.T) {
	grantsShell(t)
	dir := t.TempDir()
	theirs := BaseMount{Source: t.TempDir(), Target: "/workspaces/s", Mode: "rw"}
	other := BaseMount{Source: t.TempDir(), Target: "/workspaces/o", Mode: "rw"}
	mine := BaseMount{Source: t.TempDir(), Target: theirs.Target, Mode: "rw"}
	read := grantsSeed(t, dir, blockOf(blockLine(t, theirs), blockLine(t, other)), "",
		ownedBy(theirs.Target, grantsTheirs)+ownedBy(other.Target, grantsTheirs))
	out, err := grantsDriveFrom(t, dir, GrantsLive, nil, b64(mine.Target), grantsMine, fstabEscape(filepath.ToSlash(mine.Source)))
	if err != nil {
		t.Fatalf("the revoke failed: %v\n%s", err, out)
	}
	if got := read("fstab"); got != blockOf(blockLine(t, other)) {
		t.Fatalf("after the revoke the block is:\n%s\nand the ledger:\n%s", got, read("owners"))
	}
	if got := read("owners"); got != ownedBy(other.Target, grantsTheirs) {
		t.Fatalf("after the revoke the ledger is:\n%s", got)
	}
	if read("log") != "" {
		t.Fatalf("a revoke of a target nothing had mounted mounted or unmounted:\n%s", read("log"))
	}
}

// TestAKeptLineWinsOverThisFilesOwnForItsTarget drives a grant from a file whose
// configuration also names a target another file holds live. ⛔ That file's line
// stays the block's one line there, so a restart mounts its checkout and not this
// file's directory over it. WSL-190.
func TestAKeptLineWinsOverThisFilesOwnForItsTarget(t *testing.T) {
	grantsShell(t)
	dir := t.TempDir()
	theirs := BaseMount{Source: t.TempDir(), Target: "/workspaces/q", Mode: "rw"}
	alsoMine := BaseMount{Source: t.TempDir(), Target: "/workspaces/q", Mode: "rw"}
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/p", Mode: "rw"}
	read := grantsSeed(t, dir, blockOf(blockLine(t, theirs)), liveLine(theirs.Source, theirs.Target, "rw"), ownedBy(theirs.Target, grantsTheirs))
	if out, err := grantsDrive(t, dir, []BaseMount{mine, alsoMine}, b64(mine.Target)); err != nil {
		t.Fatalf("the grant failed: %v\n%s", err, out)
	}
	if got := blockLinesAt(t, dir, theirs.Target); len(got) != 1 || got[0] != blockLine(t, theirs) {
		t.Fatalf("the block mounts %s with %q, want the other file's line alone:\n%s", theirs.Target, got, read("fstab"))
	}
	grantsRestart(t, dir)
	if got := read("mounts"); !strings.Contains(got, liveLine(theirs.Source, theirs.Target, "rw")) || !strings.Contains(got, liveLine(mine.Source, mine.Target, "rw")) {
		t.Fatalf("after a restart the mounts are:\n%s", got)
	}
}

// TestALineTheLedgerGivesNoOtherFileIsReplaced drives the lines a caller still
// replaces. A line no ledger row names, as every 7.0.1 grant's, goes, since every
// verifier would refuse its mount after a restart. ⛔ So does this file's own line
// once its configuration no longer names it: an ensure after a hand edit to the
// file, and a revoke, leave no line behind. WSL-190.
func TestALineTheLedgerGivesNoOtherFileIsReplaced(t *testing.T) {
	grantsShell(t)
	dir := t.TempDir()
	old := BaseMount{Source: t.TempDir(), Target: "/workspaces/old", Mode: "rw"}
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"}
	read := grantsSeed(t, dir, blockOf(blockLine(t, old)), "", "")
	if out, err := grantsDriveIn(t, dir, GrantsWrite, []BaseMount{mine}, ""); err != nil {
		t.Fatalf("the write failed: %v\n%s", err, out)
	}
	if got := read("fstab"); got != blockOf(blockLine(t, mine)) {
		t.Fatalf("a line no ledger row names was kept:\n%s", got)
	}

	dir = t.TempDir()
	read = grantsSeed(t, dir, blockOf(blockLine(t, old)), "", ownedBy(old.Target, grantsMine))
	if out, err := grantsDriveIn(t, dir, GrantsWrite, []BaseMount{mine}, ""); err != nil {
		t.Fatalf("the write failed: %v\n%s", err, out)
	}
	if got := read("fstab"); got != blockOf(blockLine(t, mine)) {
		t.Fatalf("this file's own line outlived the grant its configuration dropped:\n%s", got)
	}
	if got := read("owners"); got != ownedBy(mine.Target, grantsMine) {
		t.Fatalf("after the write the ledger is:\n%s", got)
	}

	dir = t.TempDir()
	read = grantsSeed(t, dir, "", "", "")
	if out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target)); err != nil {
		t.Fatalf("the grant failed: %v\n%s", err, out)
	}
	if out, err := grantsDrive(t, dir, nil, b64(mine.Target)); err != nil {
		t.Fatalf("the revoke failed: %v\n%s", err, out)
	}
	if got := read("fstab"); got != blockOf() {
		t.Fatalf("a revoke left its own line in the block:\n%s", got)
	}
}

// TestACallerWithNoFileWritesItsOwnGrantsAlone drives a write from a
// configuration with no file, as the helper route sent before it sent its
// caller's. ⚠ It keeps nothing for another file, as before: its verifier refuses
// every mount it does not name, so a kept line would fail its own ensure after the
// restart. WSL-190.
func TestACallerWithNoFileWritesItsOwnGrantsAlone(t *testing.T) {
	grantsShell(t)
	dir := t.TempDir()
	theirs := BaseMount{Source: t.TempDir(), Target: "/workspaces/theirs", Mode: "rw"}
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"}
	read := grantsSeed(t, dir, blockOf(blockLine(t, theirs)), "", ownedBy(theirs.Target, grantsTheirs))
	if out, err := grantsDriveAs(t, dir, GrantsWrite, []BaseMount{mine}, "", ""); err != nil {
		t.Fatalf("the write failed: %v\n%s", err, out)
	}
	if got := read("fstab"); got != blockOf(blockLine(t, mine)) {
		t.Fatalf("a caller with no file kept another file's line:\n%s", got)
	}
}

// TestAGrantWhoseReadBackFailsTakesItsMountAway drives a grant whose read-back
// fails after it mounted. ⛔ The failure put the old block back and left the new
// mount, with no ledger row and in no configuration, so the next ensure read it as
// unconfigured. It is unmounted, and a mode switch gets its old mount back alone.
// WSL-189.
func TestAGrantWhoseReadBackFailsTakesItsMountAway(t *testing.T) {
	grantsShell(t)
	src := t.TempDir()
	mine := BaseMount{Source: src, Target: "/workspaces/mine", Mode: "rw"}
	dir := t.TempDir()
	read := grantsSeed(t, dir, "kept\n", "", "")
	if err := os.WriteFile(filepath.Join(dir, "runuser.fail"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target))
	if err == nil || !strings.Contains(out, "agent cannot write in /workspaces/mine") {
		t.Fatalf("a grant whose read-back failed answered %v:\n%s", err, out)
	}
	if read("mounts") != "" || read("log") != "mount /workspaces/mine\numount /workspaces/mine\n" || read("fstab") != "kept\n" || read("owners") != "" {
		t.Fatalf("a failed grant left the base changed:\nlog:\n%s\nmounts:\n%s\nfstab:\n%s\nowners:\n%s", read("log"), read("mounts"), read("fstab"), read("owners"))
	}

	old := BaseMount{Source: src, Target: mine.Target, Mode: "ro"}
	fstab := blockOf(blockLine(t, old))
	dir = t.TempDir()
	read = grantsSeed(t, dir, fstab, liveLine(src, mine.Target, "ro"), ownedBy(mine.Target, grantsMine))
	if err := os.WriteFile(filepath.Join(dir, "runuser.fail"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target)); err == nil {
		t.Fatalf("a switch whose read-back failed answered %v:\n%s", err, out)
	}
	if got := read("mounts"); got != liveLine(src, mine.Target, "ro") || read("fstab") != fstab {
		t.Fatalf("a failed switch did not leave the old mount alone:\nlog:\n%s\nmounts:\n%s\nfstab:\n%s", read("log"), got, read("fstab"))
	}
}

// TestAMountAFailedGrantCannotTakeAwayIsNamed drives a failed grant whose own
// mount will not unmount. The refusal names it, so nobody reads the base as
// unchanged. WSL-189.
func TestAMountAFailedGrantCannotTakeAwayIsNamed(t *testing.T) {
	grantsShell(t)
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"}
	dir := t.TempDir()
	grantsSeed(t, dir, "", "", "")
	for _, name := range []string{"runuser.fail", "umount.fail"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target))
	if err == nil || !strings.Contains(out, "/workspaces/mine stays mounted, because it could not be unmounted: the stand-in umount failed") {
		t.Fatalf("a failed grant whose mount stayed answered %v:\n%s", err, out)
	}
}

// TestAFailedSwitchMountsNothingOverAMountThatStayed drives a switch from ro to rw
// whose read-back fails and whose new mount then will not unmount. ⛔ The old mount
// was mounted again on top of it, so the target held two mounts, and the refusal
// said only that the target stayed mounted. The new mount stays alone, the
// refusal says it is in the new mode, and the block and the ledger are as they
// were. WSL-189.
func TestAFailedSwitchMountsNothingOverAMountThatStayed(t *testing.T) {
	grantsShell(t)
	src := t.TempDir()
	mine := BaseMount{Source: src, Target: "/workspaces/mine", Mode: "rw"}
	fstab := blockOf(blockLine(t, BaseMount{Source: src, Target: mine.Target, Mode: "ro"}))
	dir := t.TempDir()
	read := grantsSeed(t, dir, fstab, liveLine(src, mine.Target, "ro"), ownedBy(mine.Target, grantsMine))
	for name, body := range map[string]string{"runuser.fail": "", "umount.failat": "2\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target))
	if err == nil || !strings.Contains(out, "/workspaces/mine stays mounted rw, the new mode, because it could not be unmounted: the stand-in umount failed. Its old mount was not put back over it") {
		t.Fatalf("a failed switch whose new mount stayed answered %v:\n%s", err, out)
	}
	if got := read("mounts"); got != liveLine(src, mine.Target, "rw") {
		t.Fatalf("a failed switch whose new mount stayed left the mounts:\n%s\nlog:\n%s", got, read("log"))
	}
	if read("fstab") != fstab || read("owners") != ownedBy(mine.Target, grantsMine) {
		t.Fatalf("a failed switch changed the block or the ledger:\n%s\n%s", read("fstab"), read("owners"))
	}
}

// TestAFailedUnmountOfAMountAlreadyGoneIsNotNamed drives a failed switch whose new
// mount another process took away first, so its unmount fails. ⚠ The refusal said
// the target stayed mounted when nothing was. Nothing is named as staying, and the
// old mount is put back. WSL-189.
func TestAFailedUnmountOfAMountAlreadyGoneIsNotNamed(t *testing.T) {
	grantsShell(t)
	src := t.TempDir()
	mine := BaseMount{Source: src, Target: "/workspaces/mine", Mode: "rw"}
	fstab := blockOf(blockLine(t, BaseMount{Source: src, Target: mine.Target, Mode: "ro"}))
	dir := t.TempDir()
	read := grantsSeed(t, dir, fstab, liveLine(src, mine.Target, "ro"), ownedBy(mine.Target, grantsMine))
	for name, body := range map[string]string{"runuser.fail": "", "umount.failat": "2\n", "umount.gone": ""} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target))
	if err == nil || !strings.Contains(out, "agent cannot write in /workspaces/mine") || strings.Contains(out, "stays mounted") {
		t.Fatalf("a failed switch whose new mount was already gone answered %v:\n%s", err, out)
	}
	if got := read("mounts"); got != liveLine(src, mine.Target, "ro") {
		t.Fatalf("a failed switch whose new mount was already gone left the mounts:\n%s\nlog:\n%s", got, read("log"))
	}
}

// verifyHeldRun runs verify.sh's sweep of the live Windows mounts under
// grantsShell, over dir's mounts and owners files, handed env as verifyEnv makes
// it. The targets env's TK_MOUNT_CHECKS names are the configured ones, as the
// script's own check of each one leaves them.
func verifyHeldRun(t *testing.T, dir string, env map[string]string) (string, error) {
	t.Helper()
	sh, bin := grantsShell(t)
	at := filepath.ToSlash(dir)
	var allow strings.Builder
	fields := strings.Fields(env["TK_MOUNT_CHECKS"])
	for i := 0; i+1 < len(fields); i += 2 {
		target, err := base64.StdEncoding.DecodeString(fields[i])
		if err != nil {
			t.Fatal(err)
		}
		allow.WriteString(string(target) + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "allow"), []byte(allow.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	// ⚠ FROM THE EMBEDDED SCRIPT, since a case may run from a directory of its own.
	verify := string(verifyScript)
	from := strings.Index(verify, "if [ \"$TK_AUTOMOUNT\" = off ]; then\n")
	to := -1
	if from >= 0 {
		to = strings.Index(verify[from:], "done < /proc/mounts\n")
	}
	if to < 0 {
		t.Fatal("verify.sh holds no sweep of the live Windows mounts, so this case is reading a script that moved")
	}
	script := "mount_allowlist=" + at + "/allow\n" + verify[from:from+to] + "done < /proc/mounts\nfi\n"
	script = strings.ReplaceAll(script, "/proc/mounts", at+"/mounts")
	script = strings.ReplaceAll(script, "/var/lib/wsl-toolkit/grant-owners", at+"/owners")
	run := filepath.Join(dir, "verify-run.sh")
	if err := os.WriteFile(run, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, filepath.ToSlash(run))
	cmd.Env = append(shellEnv(bin), "TK_AUTOMOUNT="+env["TK_AUTOMOUNT"], "TK_CONFIG_OWNER="+env["TK_CONFIG_OWNER"])
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestTheHelperRouteActsForTheCallersFile drives an ensure and then a probe through
// the helper, for a file whose base holds another file's grant too, across a
// restart. ⛔ The helper was sent no file. Its write kept no other file's line, so
// the restart dropped that grant, and its verifier refused each kept one as
// unconfigured after any restart, so its ensure then refused. Each now names the
// caller as the CLI does, keeps the other file's line, and passes. WSL-190.
func TestTheHelperRouteActsForTheCallersFile(t *testing.T) {
	grantsShell(t)
	_ = newFakeHost(t)
	_ = newDefaultBaseCase(t)
	cfg := grantBase(t)
	cfg.Base.Name = "wsl-toolkit-g190"
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"}
	theirs := BaseMount{Source: t.TempDir(), Target: "/workspaces/theirs", Mode: "rw"}
	cfg.Base.Mounts = []BaseMount{mine}

	var ensure helperEnsureRequest
	status := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(r.Body).Decode(&ensure)
		} else {
			status = r.URL.RequestURI()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema":"` + HelperSchema + `","state":{}}`))
	}))
	defer srv.Close()
	client := &HelperClient{endpoint: HelperEndpoint{Address: strings.TrimPrefix(srv.URL, "http://"), Token: "t"}, http: srv.Client(), cfg: cfg}
	_, _ = client.BaseEnsureWith(context.Background(), false, true)
	_, _ = client.BaseStatus(context.Background(), true)

	// ⚠ THE RUNNER ITSELF NEEDS wsl.exe, which a Linux host has not. The
	// configuration it acts on is read before the runner is made.
	h := &HelperServer{log: func(string) {}, runners: map[string]*Runner{}}
	_, ensured, _ := h.runnerForEnsure(ensure)
	_, probed, _ := h.runnerForQuery(httptest.NewRequest(http.MethodGet, status, nil))
	key, name := grantOwner(cfg)
	for label, got := range map[string]Config{"ensure": ensured, "probe": probed} {
		if k, _ := grantOwner(got); k != key {
			t.Fatalf("the helper's %s names its caller %q in the grant ledger, want %q, the CLI's", label, k, key)
		}
	}

	dir := t.TempDir()
	read := grantsSeed(t, dir, blockOf(blockLine(t, mine), blockLine(t, theirs)),
		liveLine(mine.Source, mine.Target, "rw")+liveLine(theirs.Source, theirs.Target, "rw"),
		mine.Target+" "+key+" "+name+"\n"+ownedBy(theirs.Target, grantsTheirs))
	env, err := grantsEnv(ensured, GrantsWrite, BaseMount{})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := grantsRun(t, dir, env); err != nil {
		t.Fatalf("the helper's provisioning write failed: %v\n%s", err, out)
	}
	if got := blockLinesAt(t, dir, theirs.Target); len(got) != 1 || got[0] != blockLine(t, theirs) {
		t.Fatalf("the helper's write left the other file's line as %q:\n%s", got, read("fstab"))
	}
	grantsRestart(t, dir)
	venv, err := verifyEnv(probed, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	out, err := verifyHeldRun(t, dir, venv)
	if err != nil {
		t.Fatalf("the helper's probe after a restart refused: %v\n%s\nmounts:\n%s", err, out, read("mounts"))
	}
	if got := parseHeldMounts(out); len(got) != 1 || got[0] != theirs.Target+", granted by "+grantsTheirs {
		t.Fatalf("the helper's probe names the held mounts %q:\n%s", got, out)
	}
}

// TestAHelpersRunnerIsKeptForItsCallersFile holds the helper's runner cache and the
// file it is sent. ⛔ Two files with the same settings share a fingerprint, so a
// runner cached for one served the other and named the wrong file in the grant
// ledger. ⚠ A file named relative to nothing is refused. WSL-190.
func TestAHelpersRunnerIsKeptForItsCallersFile(t *testing.T) {
	_ = newFakeHost(t)
	_ = newDefaultBaseCase(t)
	cfg := grantBase(t)
	cfg.Base.Name = "wsl-toolkit-g190"
	other := cfg
	other.path = filepath.Join(t.TempDir(), "other.json")
	h := &HelperServer{log: func(string) {}, runners: map[string]*Runner{}}
	mine := &Runner{base: &Base{cfg: cfg}}
	h.runners[helperRunnerKey(cfg, false)] = mine
	if r, _, _ := h.runnerFor(&cfg, "", false); r != mine {
		t.Fatal("the runner cached for this file was not found again")
	}
	if r, _, _ := h.runnerFor(&other, "", false); r == mine {
		t.Fatal("a runner cached for one file served another with the same settings")
	}
	sent := DefaultConfig()
	if err := callerFile(&sent, "config.json"); err == nil || sent.Path() != "" {
		t.Fatalf("a relative caller file answered %v and set %q", err, sent.Path())
	}
}

// TestTheHelpersEnsureReadsItsCallersGrantsAsUnchanged holds the helper's ensure,
// now that it is sent its caller's file, reading that file's grants again under the
// base's lock. ⚠ It is handed each source resolved and reads the file as written,
// so a relative source read as a change on every ensure, and the log said the
// grants changed while it waited. A real change is still taken. WSL-172, WSL-190.
func TestTheHelpersEnsureReadsItsCallersGrantsAsUnchanged(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := grantBase(t)
	cfg.path = filepath.Join(dir, "config.json")
	cfg.Base.Mounts = []BaseMount{{Source: filepath.Join(dir, "src"), Target: "/workspaces/src", Mode: "rw"}}
	if err := cfg.WriteTo(cfg.path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(cfg.path)
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	mounts := file["base"].(map[string]any)["mounts"].([]any)
	mounts[0].(map[string]any)["source"] = "src"
	if raw, err = json.Marshal(file); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfigFile(cfg.path, "the case")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Base.Mounts[0].Source != "src" {
		t.Fatalf("the file's grant reads %q, want the relative src", loaded.Base.Mounts[0].Source)
	}
	s := &sayings{}
	b := &Base{cfg: loaded.Portable(), log: s.log}
	if changed, err := b.adoptStoredGrants(); err != nil || changed || s.said("the grants changed") {
		t.Fatalf("the same grants handed resolved read as changed %v, %v: %v", changed, err, s.lines)
	}
	b.cfg.Base.Mounts[0].Mode = "ro"
	if changed, err := b.adoptStoredGrants(); err != nil || !changed || b.cfg.Base.Mounts[0].Mode != "rw" {
		t.Fatalf("a grant the file changed was not taken: %v, %v, %+v", changed, err, b.cfg.Base.Mounts)
	}
}
