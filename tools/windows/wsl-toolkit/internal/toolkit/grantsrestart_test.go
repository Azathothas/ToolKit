// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/base64"
	"fmt"
	"os"
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
	posixShell(t)
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
	posixShell(t)
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
	posixShell(t)
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
	posixShell(t)
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
	posixShell(t)
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
	posixShell(t)
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
	posixShell(t)
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
// configuration with no file, as the helper route sends. ⚠ It keeps nothing for
// another file, as before: its verifier refuses every mount it does not name, so a
// kept line would fail its own ensure after the restart. WSL-190.
func TestACallerWithNoFileWritesItsOwnGrantsAlone(t *testing.T) {
	posixShell(t)
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
	posixShell(t)
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
	posixShell(t)
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
