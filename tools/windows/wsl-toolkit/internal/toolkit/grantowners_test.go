// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// grantsSeed writes the fstab, the live mounts and the grant ledger a drive of
// grants.sh starts from, and answers a reader of dir's files.
func grantsSeed(t *testing.T, dir, fstab, mounts, owners string) func(string) string {
	t.Helper()
	for name, body := range map[string]string{"fstab": fstab, "mounts": mounts, "owners": owners} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return func(name string) string {
		t.Helper()
		body, _ := os.ReadFile(filepath.Join(dir, name))
		return string(body)
	}
}

// liveLine is a /proc/mounts row for a DrvFS mount of source at target.
func liveLine(source, target, mode string) string {
	return fstabEscape(filepath.ToSlash(source)) + " " + target + " drvfs " + mode + ",noatime 0 0\n"
}

// TestALiveGrantRefusesATargetLiveFromAnotherDirectory drives a grant whose target
// another configuration has mounted from a different Windows directory. ⛔ The mode
// alone was compared: in the same mode the grant said "unchanged" and its agent
// wrote into the other checkout, and in another mode it unmounted that checkout
// and mounted its own. WSL-179.
func TestALiveGrantRefusesATargetLiveFromAnotherDirectory(t *testing.T) {
	posixShell(t)
	src, other := t.TempDir(), t.TempDir()
	for _, mode := range []string{"rw", "ro"} {
		dir := t.TempDir()
		theirs := liveLine(other, "/workspaces/shared", "rw")
		read := grantsSeed(t, dir, "", theirs, "")
		shared := BaseMount{Source: src, Target: "/workspaces/shared", Mode: mode}
		out, err := grantsDrive(t, dir, []BaseMount{shared}, b64(shared.Target))
		if err == nil || !strings.Contains(out, "is already mounted from") {
			t.Fatalf("a %s grant over a mount from another directory answered %v:\n%s", mode, err, out)
		}
		if read("log") != "" || read("mounts") != theirs || read("fstab") != "" {
			t.Fatalf("a refused %s grant changed the base:\nlog:\n%s\nmounts:\n%s\nfstab:\n%s", mode, read("log"), read("mounts"), read("fstab"))
		}
	}
}

// TestALiveGrantRefusesToRemountATargetItDidNotMount drives a grant whose target is
// live from the same directory in the other mode. ⛔ It was unmounted and mounted
// again in the grant's mode, whoever had mounted it. Only a mount the ledger gives
// this configuration is switched. WSL-179.
func TestALiveGrantRefusesToRemountATargetItDidNotMount(t *testing.T) {
	posixShell(t)
	src := t.TempDir()
	shared := BaseMount{Source: src, Target: "/workspaces/shared", Mode: "rw"}
	dir := t.TempDir()
	live := liveLine(src, shared.Target, "ro")
	read := grantsSeed(t, dir, "", live, "")
	out, err := grantsDrive(t, dir, []BaseMount{shared}, b64(shared.Target))
	if err == nil || !strings.Contains(out, "this configuration did not mount it") {
		t.Fatalf("a grant over a mount in another mode answered %v:\n%s", err, out)
	}
	if read("log") != "" || read("mounts") != live {
		t.Fatalf("a refused grant remounted a mount it did not make:\nlog:\n%s\nmounts:\n%s", read("log"), read("mounts"))
	}

	dir = t.TempDir()
	read = grantsSeed(t, dir, "", live, shared.Target+" "+b64(grantsMine)+" "+b64(grantsMine)+"\n")
	out, err = grantsDrive(t, dir, []BaseMount{shared}, b64(shared.Target))
	if err != nil || !strings.Contains(read("log"), "umount /workspaces/shared\nmount /workspaces/shared\n") {
		t.Fatalf("this configuration's own mount was not switched: %v\n%s\nlog:\n%s", err, out, read("log"))
	}
	if !strings.Contains(read("mounts"), " /workspaces/shared drvfs rw,") {
		t.Fatalf("the switched mount is not rw:\n%s", read("mounts"))
	}
}

// TestAFailedModeSwitchMountsTheOldOneBack drives a mode switch whose new mount
// fails. ⛔ The unmount was not recorded, so the failure put the old block back and
// left the target with nothing mounted. WSL-179.
func TestAFailedModeSwitchMountsTheOldOneBack(t *testing.T) {
	posixShell(t)
	src := t.TempDir()
	shared := BaseMount{Source: src, Target: "/workspaces/shared", Mode: "rw"}
	old := BaseMount{Source: src, Target: shared.Target, Mode: "ro"}
	block64, _, err := renderBaseMounts([]BaseMount{old})
	if err != nil {
		t.Fatal(err)
	}
	block, _ := base64.StdEncoding.DecodeString(block64)
	dir := t.TempDir()
	fstab := "# wsl-toolkit mounts begin\n" + string(block) + "# wsl-toolkit mounts end\n"
	read := grantsSeed(t, dir, fstab, liveLine(src, shared.Target, "ro"), shared.Target+" "+b64(grantsMine)+" "+b64(grantsMine)+"\n")
	if err := os.WriteFile(filepath.Join(dir, "mount.fail"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := grantsDrive(t, dir, []BaseMount{shared}, b64(shared.Target))
	if err == nil || !strings.Contains(out, "could not be mounted") {
		t.Fatalf("a failed mount answered %v:\n%s", err, out)
	}
	if !strings.Contains(read("mounts"), " /workspaces/shared drvfs ro,") || read("fstab") != fstab {
		t.Fatalf("a failed switch did not put the old mount back:\nlog:\n%s\nmounts:\n%s\nfstab:\n%s", read("log"), read("mounts"), read("fstab"))
	}
}

// TestALiveChangeRefusesATargetTheLedgerGivesAnotherFile drives a grant and a
// revoke of a target another configuration file granted. ⛔ Neither may unmount or
// claim it: the refusal names that file and its revoke. WSL-179.
func TestALiveChangeRefusesATargetTheLedgerGivesAnotherFile(t *testing.T) {
	posixShell(t)
	src := t.TempDir()
	shared := BaseMount{Source: src, Target: "/workspaces/shared", Mode: "rw"}
	theirs := "/cfg/theirs.json"
	live := liveLine(src, shared.Target, "rw")
	owners := shared.Target + " " + b64(theirs) + " " + b64(theirs) + "\n"
	for label, mounts := range map[string][]BaseMount{"grant": {shared}, "revoke": nil} {
		dir := t.TempDir()
		read := grantsSeed(t, dir, "kept\n", live, owners)
		out, err := grantsDrive(t, dir, mounts, b64(shared.Target))
		if err == nil || !strings.Contains(out, "is mounted for "+theirs) || !strings.Contains(out, "base revoke --target /workspaces/shared") {
			t.Fatalf("a %s of a target another file holds answered %v:\n%s", label, err, out)
		}
		if read("log") != "" || read("mounts") != live || read("fstab") != "kept\n" || read("owners") != owners {
			t.Fatalf("a refused %s changed the base:\nlog:\n%s\nmounts:\n%s\nowners:\n%s", label, read("log"), read("mounts"), read("owners"))
		}
	}
}

// TestTheLedgerSaysWhichFileMadeEachGrant drives a grant, a revoke and a
// provisioning write. A live change records or removes its own target alone, and
// a write records the configuration's grants, which the restart after it mounts
// alone. WSL-179.
func TestTheLedgerSaysWhichFileMadeEachGrant(t *testing.T) {
	posixShell(t)
	src := t.TempDir()
	mine := BaseMount{Source: src, Target: "/workspaces/mine", Mode: "rw"}
	named := BaseMount{Source: src, Target: "/workspaces/named", Mode: "ro"}
	theirs := "/workspaces/theirs " + b64("/cfg/theirs.json") + " " + b64("/cfg/theirs.json") + "\n"
	own := func(target string) string { return target + " " + b64(grantsMine) + " " + b64(grantsMine) + "\n" }
	dir := t.TempDir()
	read := grantsSeed(t, dir, "", "", theirs)

	if out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target)); err != nil {
		t.Fatalf("the grant failed: %v\n%s", err, out)
	}
	if got := read("owners"); got != theirs+own(mine.Target) {
		t.Fatalf("after a grant the ledger is:\n%s", got)
	}
	if out, err := grantsDrive(t, dir, nil, b64(mine.Target)); err != nil {
		t.Fatalf("the revoke failed: %v\n%s", err, out)
	}
	if got := read("owners"); got != theirs {
		t.Fatalf("after a revoke the ledger is:\n%s", got)
	}
	if out, err := grantsDriveIn(t, dir, GrantsWrite, []BaseMount{mine, named}, ""); err != nil {
		t.Fatalf("the write failed: %v\n%s", err, out)
	}
	if got := read("owners"); got != own(mine.Target)+own(named.Target) {
		t.Fatalf("after a write the ledger is:\n%s", got)
	}
}

// TestTheVerifierLeavesAMountTheLedgerGivesAnotherFile drives verify.sh's
// unconfigured-mount check. ⛔ A mount another configuration file granted was
// refused, and the ensure that followed restarted the base and dropped it. It is
// named and left, and a mount nobody's ledger names, or this file's own, is still
// refused. WSL-179.
func TestTheVerifierLeavesAMountTheLedgerGivesAnotherFile(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	dir := t.TempDir()
	mounts := filepath.Join(dir, "mounts")
	owners := filepath.Join(dir, "owners")
	theirs := "/cfg/theirs.json"
	if err := os.WriteFile(mounts, []byte("drvfs /workspaces/theirs drvfs rw 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(owners, []byte("/workspaces/theirs "+b64(theirs)+" "+b64(theirs)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	block := mustExtract(t, "verify.sh", "mount_count=0", "done < /proc/mounts") + "\nfi\n"
	script := strings.ReplaceAll(block, "/proc/mounts", filepath.ToSlash(mounts))
	script = strings.ReplaceAll(script, "/var/lib/wsl-toolkit/grant-owners", filepath.ToSlash(owners))
	run := func(owner string) (string, error) {
		cmd := exec.Command(sh, "-c", script)
		cmd.Env = append(os.Environ(), "TK_AUTOMOUNT=off", "TK_MOUNT_CHECKS=", "TK_MOUNT_GONE=",
			"TK_CONFIG_OWNER="+owner, "TMPDIR="+filepath.ToSlash(dir))
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := run(b64(grantsMine))
	if err != nil {
		t.Fatalf("a mount another file granted was refused: %v: %s", err, out)
	}
	if got := parseHeldMounts(out); len(got) != 1 || got[0] != "/workspaces/theirs, granted by "+theirs {
		t.Fatalf("the held mount reads %q from:\n%s", got, out)
	}
	for label, owner := range map[string]string{"its own file": b64(theirs), "no file": ""} {
		if out, err := run(owner); err == nil || !strings.Contains(out, "unconfigured Windows directory is mounted at /workspaces/theirs") {
			t.Fatalf("a mount the ledger gives %s answered %v: %s", label, err, out)
		}
	}
}

// TestTheGrantAndVerifyStepsAreToldTheirConfigurationFile holds what names the
// caller in the ledger: grants.sh and verify.sh are both handed the file, as one
// key Windows compares without case. WSL-179.
func TestTheGrantAndVerifyStepsAreToldTheirConfigurationFile(t *testing.T) {
	cfg := grantBase(t)
	cfg.Base.Mounts = []BaseMount{
		{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"},
	}
	key := filepath.Clean(cfg.Path())
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	genv, err := grantsEnv(cfg, GrantsLive, "/workspaces/mine")
	if err != nil {
		t.Fatal(err)
	}
	if genv["TK_GRANT_OWNER"] != b64(key) || genv["TK_GRANT_OWNER_NAME"] != b64(filepath.Clean(cfg.Path())) {
		t.Fatalf("grants.sh is handed owner %q named %q, want the key of %s", genv["TK_GRANT_OWNER"], genv["TK_GRANT_OWNER_NAME"], cfg.Path())
	}
	venv, err := verifyEnv(cfg, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if venv["TK_CONFIG_OWNER"] != genv["TK_GRANT_OWNER"] {
		t.Fatalf("verify.sh is handed owner %q, grants.sh %q", venv["TK_CONFIG_OWNER"], genv["TK_GRANT_OWNER"])
	}
}

// TestASecondGrantWaitsForTheFirst holds the instance's grant lock. ⛔ Two grants at
// once each wrote the file without the other's grant. The second waits, and
// refuses rather than changing anything when the first keeps it. WSL-179.
func TestASecondGrantWaitsForTheFirst(t *testing.T) {
	t.Setenv(InstanceRootEnv, t.TempDir())
	previous := SelectedInstance
	t.Cleanup(func() { SelectedInstance = previous })
	SelectedInstance = Instance{Name: "g"}
	first, err := LockGrants(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if release, err := LockGrants(ctx, nil); err == nil {
		release()
		t.Fatal("a second grant took the lock the first holds")
	} else if !strings.Contains(err.Error(), "waiting for another grant or revoke on instance g") {
		t.Fatalf("a second grant answered %v", err)
	}
	wait := grantLockWait
	t.Cleanup(func() { grantLockWait = wait })
	grantLockWait = 0
	if _, err := LockGrants(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("a grant that waited out the lock answered %v", err)
	}
	grantLockWait = wait
	first()
	second, err := LockGrants(context.Background(), nil)
	if err != nil {
		t.Fatalf("the lock was not free once the first let go: %v", err)
	}
	second()
}
