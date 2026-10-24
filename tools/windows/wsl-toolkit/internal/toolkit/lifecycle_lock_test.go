// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// -- the base's lifecycle, the audit's second pass: WSL-169, WSL-172, WSL-175, WSL-177

// waitUntilSaid waits for a base to say sub, or fails the case.
func waitUntilSaid(t *testing.T, s *sayings, sub string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !s.said(sub) {
		if time.Now().After(deadline) {
			t.Fatalf("never said %q: %v", sub, s.lines)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ⛔ WSL-172. An ensure that waited behind `base revoke` went on with the grants
// it had read before the revoke, so the provisioner wrote the revoked grant back
// into the fstab block and the next start mounted it again.
func TestEnsureReadsTheGrantsAgainUnderTheLock(t *testing.T) {
	h := newFakeHost(t)
	b, s, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	quickLockPoll(t)
	was := SelectedInstance
	SelectedInstance = Instance{Name: "wsl169", Distro: fakeBaseName}
	t.Cleanup(func() { SelectedInstance = was })

	// The file as the revoke left it: no grant.
	path := filepath.Join(t.TempDir(), "config.json")
	stored := b.cfg
	stored.Base.Automount = AutomountOff
	stored.Base.Interop = BaseInteropOff
	if err := stored.WriteTo(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfigFile(path, "the case")
	if err != nil {
		t.Fatal(err)
	}
	// What the ensure read before the revoke: the grant still there.
	const target = "/workspaces/revoked"
	grant := BaseMount{Source: t.TempDir(), Target: target, Mode: "rw"}
	loaded.Base.Mounts = []BaseMount{grant}
	b.cfg = loaded
	h.say(t, "provision.out", "wsl-conf unchanged\nprovision-complete\n")
	h.say(t, "grants.out", "fstab changed\ngrants-complete\n")

	release, err := lockBaseAt(context.Background(), b.home, "base revoke", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := b.EnsureWith(context.Background(), false, false)
		done <- err
	}()
	waitUntilSaid(t, s, "waiting: the base is being changed by")
	release()
	<-done

	script, err := os.ReadFile(filepath.Join(h.state, "grants.out.script"))
	if err != nil {
		t.Fatalf("the case did not reach the grants step it is about: %v, %v", err, s.lines)
	}
	if strings.Contains(string(script), base64.StdEncoding.EncodeToString([]byte(target))) {
		t.Fatalf("the revoked grant was handed to the fstab block again:\n%s", script)
	}
	if !s.said("the grants changed while this waited") {
		t.Fatalf("the change was not said: %v", s.lines)
	}
}

// ⛔ WSL-169, WSL-172. A `--preset` switch was decided before the lock, so a
// second switch that waited for the first removed and rebuilt the base the first
// had just built from the same image.
func TestAPresetSwitchThatWaitedKeepsTheBaseTheFirstBuilt(t *testing.T) {
	const next = "docker.io/library/debian:trixie"
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	quickLockPoll(t)
	release, err := lockBaseAt(context.Background(), b.home, "base ensure --preset", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	b.cfg.Base.Image = next
	done := make(chan error, 1)
	go func() {
		_, err := b.EnsureSwitch(context.Background(), false)
		done <- err
	}()
	waitUntilSaid(t, s, "waiting: the base is being changed by")
	// The first switch finishes: its base is built from the preset, and its
	// record and marker say so.
	if err := b.writeRecord(); err != nil {
		t.Fatal(err)
	}
	marker, err := json.Marshal(Identity{Schema: IdentitySchema, Name: fakeBaseName, Image: next, User: b.cfg.Base.User, Built: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	h.say(t, "identity.json", string(marker))
	release()
	<-done
	assertKept(t, h, b, disk)
	if s.said("rebuilding from nothing") {
		t.Fatalf("the base the first switch built was rebuilt: %v", s.lines)
	}
	if i := callAt(h.calls(t), "podman"); i >= 0 {
		t.Fatalf("a rebuild was prepared for a base already built from the preset: %v", h.calls(t))
	}
}

// ⭐ The other half: a switch that finds the base still built from another image
// rebuilds it, as before.
func TestAPresetSwitchStillRebuildsABaseFromAnotherImage(t *testing.T) {
	h := newFakeHost(t)
	b, s, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	b.cfg.Base.Image = "docker.io/library/debian:trixie"
	if _, err := b.EnsureSwitch(context.Background(), false); err == nil {
		t.Fatal("a build on a fake WSL that runs no guest script reported success")
	}
	if !s.said("rebuilding from nothing") || callAt(h.calls(t), "wsl --unregister "+fakeBaseName) < 0 {
		t.Fatalf("a switch to another image did not rebuild: %v, %v", s.lines, h.calls(t))
	}
}

// ⛔ WSL-172. An import killed after the service registered it left the
// distribution stranded: the build did not count it as its own, so nothing
// unregistered it, and the next ensure and `base remove` both refused it as
// unmarked.
func TestAnImportThatFailedAfterRegisteringIsRolledBack(t *testing.T) {
	h := newFakeHost(t)
	b := fakeUnbuiltBase(t, h)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	h.say(t, "import", "killed")
	_, err := b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), "wsl --import failed") {
		t.Fatalf("a killed import answered %v", err)
	}
	if callAt(h.calls(t), "wsl --unregister "+fakeBaseName) < 0 || h.registered(t) {
		t.Fatalf("the distribution this process's import registered was left: %v", h.calls(t))
	}
	if _, err := os.Stat(b.Dir()); err == nil {
		t.Fatal("the disk directory of the rolled-back import is still there")
	}
}

// ⛔ WSL-177. A WSL that prints UTF-16 left a NUL between every letter, so no
// name matched and a registered base read as absent.
func TestAUTF16ListingIsRefusedNotReadAsEmpty(t *testing.T) {
	h := newFakeHost(t)
	h.say(t, "registered", fakeBaseName)
	h.say(t, "listing", "utf16")
	w := &Wsl{Path: h.wsl}
	exists, err := w.Exists(context.Background(), fakeBaseName)
	if err == nil || !strings.Contains(err.Error(), "UTF-16") {
		t.Fatalf("a UTF-16 listing answered %v, %v; want a refusal", exists, err)
	}
}

// leftoverWithDisk is an instance whose base directory holds a disk WSL does
// not list.
func leftoverWithDisk(t *testing.T, h *fakeHost) (*Base, *sayings, string) {
	t.Helper()
	b := fakeUnbuiltBase(t, h)
	s := &sayings{}
	b.log = s.log
	if err := os.MkdirAll(b.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	disk := filepath.Join(b.Dir(), "ext4.vhdx")
	if err := os.WriteFile(disk, []byte("the disk of a base whose registration was lost"), 0o600); err != nil {
		t.Fatal(err)
	}
	return b, s, disk
}

// ⛔ WSL-177. A record naming another distribution says whose disk the
// directory holds, and a build or a removal deleted it as a leftover anyway.
func TestARecordNamingAnotherDistributionKeepsItsDisk(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := leftoverWithDisk(t, h)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	if err := os.WriteFile(b.recordPath(), []byte(`{"schema":"wsl-toolkit-base/1","name":"wsl-toolkit-renamed","image":"`+DefaultBaseImage+`","user":"agent"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), "the disk of wsl-toolkit-renamed") {
		t.Fatalf("a build over a disk the record names as another's answered %v", err)
	}
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("the disk the record names is gone: %v", err)
	}
	if err := b.Remove(context.Background()); err == nil || !strings.Contains(err.Error(), "the disk of wsl-toolkit-renamed") {
		t.Fatalf("a removal over a disk the record names as another's answered %v", err)
	}
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("the disk the record names is gone: %v", err)
	}
}

// ⛔ WSL-177. A disk no registered distribution keeps can still be a base whose
// registration was lost, and a build deleted it to make way for its import.
func TestABuildMovesAnOrphanedDiskAside(t *testing.T) {
	h := newFakeHost(t)
	b, s, _ := leftoverWithDisk(t, h)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	_, _ = b.EnsureWith(context.Background(), false, false)
	asides, err := filepath.Glob(filepath.Join(b.home, "base.orphaned-*", "ext4.vhdx"))
	if err != nil || len(asides) != 1 {
		t.Fatalf("the orphaned disk was not moved aside: %v, %v, %v", asides, err, s.lines)
	}
	if got, err := os.ReadFile(asides[0]); err != nil || !strings.Contains(string(got), "registration was lost") {
		t.Fatalf("the disk moved aside is not the orphan: %q, %v", got, err)
	}
	if !s.said("moved a leftover disk aside to") {
		t.Fatalf("the move was not said: %v", s.lines)
	}
}

// ⛔ WSL-172. `--instance auto` only looked for a free name, so two callers at
// once both chose the same one and shared one base. Creating the directory is
// the claim.
func TestTwoAutoInstancesClaimTwoNames(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	first, err := claimInstance(map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := claimInstance(map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if first != "1" || second != "2" {
		t.Fatalf("two claims answered %q and %q; want 1 and 2", first, second)
	}
	// ⭐ A REGISTERED NAME IS STILL PASSED OVER.
	third, err := claimInstance(map[string]bool{strings.ToLower(InstanceDistro("3")): true})
	if err != nil || third != "4" {
		t.Fatalf("a claim beside a registered 3 answered %q, %v; want 4", third, err)
	}
}

// ⛔ WSL-175. provision.sh calls install_changed inside `$(...) ||`, where bash,
// the default base's /bin/sh, suspends errexit, so a mv that failed went on to say
// `changed` over a wsl.conf it never wrote. Run under every POSIX shell here.
func TestInstallChangedFailsWhenTheFileCannotBeWritten(t *testing.T) {
	shells := []string{posixShell(t)}
	if runtime.GOOS != "windows" {
		if bash, err := exec.LookPath("bash"); err == nil {
			shells = append(shells, bash)
		}
	}
	section := provisionerSection(t, "install_changed")
	for _, sh := range shells {
		t.Run(filepath.Base(sh), func(t *testing.T) {
			dir := t.TempDir()
			next := filepath.Join(dir, "wsl.conf.new")
			if err := os.WriteFile(next, []byte("a\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(dir, "missing", "wsl.conf")
			script := "set -e\n" + section + "\nr=$(install_changed \"$1\" \"$2\") || { echo DIED; exit 3; }\necho \"result=$r\"\n"
			out, err := exec.Command(sh, "-c", script, "sh", next, dest).CombinedOutput()
			if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 3 || !strings.Contains(string(out), "DIED") {
				t.Fatalf("a mv that failed answered %v:\n%s", err, out)
			}
		})
	}
}
