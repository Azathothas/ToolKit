// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// -- the base's lifecycle: WSL-169, WSL-171, WSL-172, WSL-175 and WSL-177 -------

// roomyVolume makes the volume's free space a number the case chose, so a space
// check answers the same on every host.
func roomyVolume(t *testing.T, free int64) {
	t.Helper()
	was := freeSpace
	freeSpace = func(string) (int64, bool, error) { return free, true, nil }
	t.Cleanup(func() { freeSpace = was })
}

// quickLockPoll makes a waiting caller ask again at once.
func quickLockPoll(t *testing.T) {
	t.Helper()
	was := baseLockPoll
	baseLockPoll = 10 * time.Millisecond
	t.Cleanup(func() { baseLockPoll = was })
}

// noCalls fails a case whose fakes were reached at all.
func noCalls(t *testing.T, h *fakeHost) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(h.state, "calls")); err == nil {
		t.Fatalf("WSL or an engine was reached: %v", h.calls(t))
	}
}

// ⛔ WSL-171. A registered, marked base whose re-provisioning failed was removed
// and rebuilt: a stalled mirror or a full disk cost its home, credentials and
// images. With an engine that answers, nothing stopped the removal at all.
func TestAFailedReprovisionKeepsTheBaseAndNamesTheRebuild(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	_, err := b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), fakeBaseName+" failed, and it was NOT removed") ||
		!strings.Contains(err.Error(), "base recreate") || !strings.Contains(err.Error(), "provisioning exited") {
		t.Fatalf("a failed re-provisioning answered %v; want its own error, the base kept and base recreate named", err)
	}
	if !s.said("re-provisioning in place") {
		t.Fatalf("the case did not reach the re-provisioning it is about: %v", s.lines)
	}
	assertKept(t, h, b, disk)
	if i := callAt(h.calls(t), "podman"); i >= 0 {
		t.Fatalf("a failed re-provisioning asked an engine for a rebuild: %v", h.calls(t))
	}
}

// ⛔ WSL-169, widened by the audit. An engine that answered was asked first, and
// the base was then removed before the image was pulled, so a registry that
// refused the pull left no base at all.
func TestAForcedRebuildWithAnUnreachableImageKeepsTheBase(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	// ⚠ THE VOLUME IS THE CASE'S, or a host with little free would refuse at the
	// floor WSL-180 checks first, and never reach the pull this case drives.
	roomyVolume(t, 1<<40)
	h.say(t, "pull", "refused")
	_, err := b.EnsureWith(context.Background(), true, false)
	if err == nil || !strings.Contains(err.Error(), fakeBaseName+" was NOT removed") || !strings.Contains(err.Error(), "manifest unknown") {
		t.Fatalf("a forced rebuild whose pull was refused answered %v; want the refusal naming the base and the registry's reason", err)
	}
	assertKept(t, h, b, disk)
	assertNoArchive(t, b.home)
}

// ⛔ WSL-169, widened by the audit. The space check needs the archive, which
// came after the removal, so a volume too small for the import left no base.
func TestAForcedRebuildTheVolumeCannotHoldKeepsTheBase(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	// ⚠ EXACTLY THE FLOOR, with the old disk counted, so the floor WSL-180 checks
	// before the engine passes and the import's own check is the one that
	// refuses: it wants the floor and twice the archive.
	roomyVolume(t, BaseSpaceFloor-b.diskBytes())
	_, err := b.EnsureWith(context.Background(), true, false)
	if err == nil || !strings.Contains(err.Error(), fakeBaseName+" was NOT removed") || !strings.Contains(err.Error(), "NOT ENOUGH DISK SPACE") {
		t.Fatalf("a forced rebuild the volume cannot hold answered %v; want the refusal naming the base and the space", err)
	}
	if callAt(h.calls(t), "podman export") < 0 {
		t.Fatalf("the refusal came before the export, so the import's own check was not the one tested: %v", h.calls(t))
	}
	assertKept(t, h, b, disk)
	assertNoArchive(t, b.home)
}

// ⚠ The removal frees the old disk before the import, so a rebuild is not refused
// for the space the base it replaces holds.
func TestTheOldDiskCountsAsFreeForARebuild(t *testing.T) {
	dir := t.TempDir()
	rootfs := filepath.Join(dir, "rootfs.tar")
	if err := os.WriteFile(rootfs, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	need := int64(100) + 2*10
	roomyVolume(t, need-5)
	quiet := func(string) {}
	if err := assertImportSpaceFreeing(dir, rootfs, 100, 5, quiet); err != nil {
		t.Fatalf("an import that fits once the old disk is removed was refused: %v", err)
	}
	if err := assertImportSpaceFreeing(dir, rootfs, 100, 4, quiet); err == nil || !strings.Contains(err.Error(), "once the old disk is removed") {
		t.Fatalf("an import that does not fit even then answered %v", err)
	}
	if err := assertImportSpace(dir, rootfs, 100, quiet); err == nil || !strings.Contains(err.Error(), "nothing is registered") {
		t.Fatalf("a fresh import with no disk to free answered %v", err)
	}
}

// assertNoArchive fails a case that left a rootfs archive of its own behind.
func assertNoArchive(t *testing.T, home string) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(home, "base-rootfs-*.tar"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) > 0 {
		t.Fatalf("the rootfs archive was left behind: %v", left)
	}
}

// ⛔ WSL-172. Every build exported into one fixed `base-rootfs.tar`, truncating it,
// so a second build destroyed the first one's archive under its import and then
// deleted it.
func TestTheRootfsArchiveIsThisProcesssOwn(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	other := filepath.Join(b.home, "base-rootfs.tar")
	const theirs = "another build's archive, mid-import"
	if err := os.WriteFile(other, []byte(theirs), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := b.EnsureWith(context.Background(), true, false); err == nil {
		t.Fatal("a build on a fake WSL that runs no guest script reported success")
	}
	if callAt(h.calls(t), "podman export") < 0 {
		t.Fatalf("the case did not reach the export it is about: %v", h.calls(t))
	}
	got, err := os.ReadFile(other)
	if err != nil || string(got) != theirs {
		t.Fatalf("another build's archive was touched: %q, %v", got, err)
	}
	assertNoArchive(t, b.home)
}

// fakeUnbuiltBase is an instance with no base: nothing registered, no disk and no
// record.
func fakeUnbuiltBase(t *testing.T, h *fakeHost) *Base {
	t.Helper()
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	t.Setenv(SSHDirEnv, t.TempDir())
	t.Setenv(BinDirEnv, t.TempDir())
	cfg := DefaultConfig()
	cfg.Base.Name = fakeBaseName
	h.say(t, "registered", "")
	h.say(t, "disks", "")
	return &Base{cfg: cfg, home: home, wsl: &Wsl{Path: h.wsl}, log: (&sayings{}).log, disks: h.registry}
}

// ⛔ WSL-172. The build said it owned the distribution before it imported one, so
// an import that failed because another process had just registered the name
// rolled back by unregistering THAT process's base, with no marker demanded.
func TestARollbackLeavesADistributionThisProcessDidNotImport(t *testing.T) {
	h := newFakeHost(t)
	b := fakeUnbuiltBase(t, h)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	h.say(t, "import", "race")
	// ⚠ THE WINNER STAMPED ITS BASE, so a removal that demanded the marker would
	// find one: only this process's own import may decide.
	marker, err := json.Marshal(Identity{Schema: IdentitySchema, Name: fakeBaseName, Image: DefaultBaseImage, User: b.cfg.Base.User, Built: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	h.say(t, "identity.json", string(marker))
	_, err = b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), "wsl --import failed") {
		t.Fatalf("an import that lost the race answered %v", err)
	}
	if i := callAt(h.calls(t), "wsl --unregister"); i >= 0 {
		t.Fatalf("the other process's base was unregistered (call %d of %v)", i, h.calls(t))
	}
	if !h.registered(t) {
		t.Fatal("the other process's base is no longer registered")
	}
}

// ⛔ WSL-172. The rollback's own guard, alone: with the owner check satisfied (the
// registry puts the disk in this home's base folder), a build that imported
// nothing still removes nothing. The race above cannot show it, because a racing
// importer runs in another state directory and the owner check refuses first.
func TestARollbackOfNoImportRemovesNothingEvenFromTheOwner(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	previous := readRegistrations
	t.Cleanup(func() { readRegistrations = previous })
	readRegistrations = h.registry
	h.say(t, "disks", fakeBaseName+"\t"+b.Dir())
	h.say(t, "calls", "")
	cause := errors.New("the build failed before its import")
	if err := b.rollbackImported(cause); !errors.Is(err, cause) {
		t.Fatalf("a rollback of no import answered %v; want the build's own cause", err)
	}
	assertKept(t, h, b, disk)
}

// ⛔ WSL-172. Nothing serialized a change to the base. A second caller now waits
// for the lock, reads nothing before it holds it, and names whom it waits for.
func TestEnsureWaitsForTheBasesLockAndNamesItsHolder(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	quickLockPoll(t)
	release, err := lockBaseAt(context.Background(), b.home, "base grant", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err = b.EnsureWith(ctx, true, false)
	holder := "process " + strconv.Itoa(os.Getpid()) + " running `base grant`"
	if err == nil || !strings.Contains(err.Error(), holder) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a rebuild beside a held lock answered %v; want the wait naming %s", err, holder)
	}
	if !s.said("waiting: the base is being changed by " + holder) {
		t.Fatalf("the wait was not said: %v", s.lines)
	}
	noCalls(t, h)
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("the disk is gone: %v", err)
	}
}

// ⛔ WSL-172. `base remove` holds the same lock.
func TestRemoveWaitsForTheBasesLock(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	quickLockPoll(t)
	release, err := lockBaseAt(context.Background(), b.home, "base ensure", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := b.Remove(ctx); err == nil || !strings.Contains(err.Error(), "running `base ensure`") {
		t.Fatalf("a removal beside a held lock answered %v", err)
	}
	noCalls(t, h)
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("the disk is gone: %v", err)
	}
	release()
	// ⭐ AND THE LOCK IS LET GO: the same removal then runs.
	if err := b.Remove(context.Background()); err != nil {
		t.Fatalf("the removal after the lock was let go: %v", err)
	}
	if _, err := os.Stat(disk); err == nil {
		t.Fatal("the disk survived a removal that ran")
	}
}

// otherNameHoldsTheDisk is WSL-167's rename: WSL lists the base under another
// name, with its disk in this instance's directory, and the configured name is
// not listed.
func otherNameHoldsTheDisk(t *testing.T, h *fakeHost, b *Base) {
	t.Helper()
	h.say(t, "registered", "wsl-toolkit-renamed")
	h.say(t, "disks", "wsl-toolkit-renamed\t"+b.Dir())
}

// ⛔ WSL-177. A removal whose configured name was not listed deleted the
// instance's disk directory with no unregister in front of it, whoever's disk it
// was.
func TestARemovalLeavesADiskAnotherDistributionKeeps(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	otherNameHoldsTheDisk(t, h, b)
	err := b.Remove(context.Background())
	if err == nil || !strings.Contains(err.Error(), "was NOT deleted") || !strings.Contains(err.Error(), "wsl-toolkit-renamed registered with its disk there") {
		t.Fatalf("a removal over another distribution's disk answered %v", err)
	}
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("the other distribution's disk is gone: %v", err)
	}
}

// ⛔ WSL-177. A build treated the same directory as a leftover and deleted it.
func TestABuildLeavesADiskAnotherDistributionKeeps(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	otherNameHoldsTheDisk(t, h, b)
	h.engine(t, true)
	_, err := b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), "left over from an earlier build") || !strings.Contains(err.Error(), "wsl-toolkit-renamed") {
		t.Fatalf("a build over another distribution's disk answered %v", err)
	}
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("the other distribution's disk is gone: %v", err)
	}
}

// ⛔ WSL-177. A registry that cannot be read is a refusal, not an empty machine.
func TestARemovalRefusesWhileTheRegistryCannotBeRead(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.say(t, "registered", "")
	b.disks = func() (map[string]string, error) { return nil, errors.New("access is denied") }
	err := b.Remove(context.Background())
	if err == nil || !strings.Contains(err.Error(), "could not be read") || !strings.Contains(err.Error(), "access is denied") {
		t.Fatalf("a removal with an unreadable registry answered %v", err)
	}
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("the disk is gone: %v", err)
	}
}

// ⛔ WSL-175. Every re-provision ended in a restart, which stops every job and
// agent in the base. Only a change to wsl.conf or the fstab block restarts it.
func TestAReprovisionRestartsTheBaseOnlyWhenWhatWslReadsChanged(t *testing.T) {
	for _, tc := range []struct {
		name, provision, grants string
		restart                 bool
	}{
		{"nothing changed", "wsl-conf unchanged\n", "fstab unchanged\n", false},
		{"wsl.conf changed", "wsl-conf changed\n", "fstab unchanged\n", true},
		{"the fstab block changed", "wsl-conf unchanged\n", "fstab changed\n", true},
		// ⚠ A provisioner that does not say is an older one, and restarts as before.
		{"neither said", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFakeHost(t)
			b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
			h.say(t, "provision.out", tc.provision+"provision-complete\n")
			h.say(t, "grants.out", tc.grants+"grants-complete\n")
			restarted, err := b.provision(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			terminated := callAt(h.calls(t), "wsl --terminate "+fakeBaseName) >= 0
			if restarted != tc.restart || terminated != tc.restart {
				t.Fatalf("restarted %v, terminated %v; want %v: %v", restarted, terminated, tc.restart, h.calls(t))
			}
		})
	}
}

// ⛔ WSL-175. A run that wrote wsl.conf and failed before its restart left the
// file as the next run writes it, so the next run read "unchanged" over settings
// WSL never read. The restart it owed is still owed.
func TestARestartAFailedRunOwedIsStillOwed(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	h.say(t, "provision.out", "wsl-conf changed\n")
	if _, err := b.provision(context.Background()); err == nil {
		t.Fatal("a provisioner that did not reach its last line succeeded")
	}
	if callAt(h.calls(t), "wsl --terminate "+fakeBaseName) >= 0 {
		t.Fatalf("the case's failed run restarted the base: %v", h.calls(t))
	}
	h.say(t, "provision.out", "wsl-conf unchanged\nprovision-complete\n")
	h.say(t, "grants.out", "fstab unchanged\ngrants-complete\n")
	restarted, err := b.provision(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !restarted || callAt(h.calls(t), "wsl --terminate "+fakeBaseName) < 0 {
		t.Fatalf("the restart the failed run owed was not paid: restarted %v, %v", restarted, h.calls(t))
	}
	// ⭐ AND ONCE PAID IT IS NOT OWED AGAIN.
	if restarted, err := b.provision(context.Background()); err != nil || restarted {
		t.Fatalf("a third run that changed nothing restarted the base: %v, %v", restarted, err)
	}
}

// ⚠ WSL-175. A re-provision that changed nothing and still does not verify gets
// the restart every re-provision used to end in, so nothing that worked stops.
func TestAReprovisionThatChangedNothingStillRestartsWhenItDoesNotVerify(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.say(t, "provision.out", "wsl-conf unchanged\nprovision-complete\n")
	h.say(t, "grants.out", "fstab unchanged\ngrants-complete\n")
	_, err := b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), "re-provisioned and it still does not verify") {
		t.Fatalf("a base the fake cannot verify answered %v", err)
	}
	if callAt(h.calls(t), "wsl --terminate "+fakeBaseName) < 0 {
		t.Fatalf("a base that did not verify was not restarted: %v", h.calls(t))
	}
	assertKept(t, h, b, disk)
}

// TestInstallChangedSaysWhetherTheFileChanged runs provision.sh's own comparison.
// WSL-175.
func TestInstallChangedSaysWhetherTheFileChanged(t *testing.T) {
	sh := posixShell(t)
	section := provisionerSection(t, "install_changed")
	for _, tc := range []struct {
		name, before, next, want string
		exists                   bool
	}{
		{"the same bytes", "a\nb\n", "a\nb\n", "unchanged", true},
		{"a different line", "a\nb\n", "a\nc\n", "changed", true},
		{"only a trailing newline", "a\nb", "a\nb\n", "changed", true},
		{"no file yet", "", "a\n", "changed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dest, next := filepath.Join(dir, "wsl.conf"), filepath.Join(dir, "wsl.conf.new")
			if tc.exists {
				if err := os.WriteFile(dest, []byte(tc.before), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(next, []byte(tc.next), 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(sh, "-c", section+"\ninstall_changed \"$1\" \"$2\"", "sh", next, dest).CombinedOutput()
			if err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			if got := strings.TrimSpace(string(out)); got != tc.want {
				t.Fatalf("answered %q, want %q", got, tc.want)
			}
			if got, err := os.ReadFile(dest); err != nil || string(got) != tc.next {
				t.Fatalf("the file holds %q, %v; want %q", got, err, tc.next)
			}
			if _, err := os.Stat(next); err == nil {
				t.Fatal("the new file was left beside it")
			}
		})
	}
}

// TestTheGrantsBlockSaysWhetherItChanged runs grants.sh's write mode against a
// file of the case's own in place of /etc/fstab. WSL-175.
func TestTheGrantsBlockSaysWhetherItChanged(t *testing.T) {
	sh := posixShell(t)
	fstab := filepath.Join(t.TempDir(), "fstab")
	if err := os.WriteFile(fstab, []byte("LABEL=root / ext4 defaults 0 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := strings.ReplaceAll(string(grantsScript), "/etc/fstab", fstab)
	// ⚠ The guest's grant lock and owners ledger live under /var/lib, which a
	// case run as an ordinary account cannot write. WSL-179.
	script = strings.ReplaceAll(script, "/var/lib/wsl-toolkit", filepath.ToSlash(t.TempDir()))
	run := func() string {
		t.Helper()
		cmd := exec.Command(sh, "-s")
		cmd.Stdin = strings.NewReader(script)
		cmd.Env = append(os.Environ(), "TK_USER=agent", "TK_GRANTS_MODE=write", "TK_FSTAB_B64=", "TK_MOUNT_CHECKS=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return string(out)
	}
	if out := run(); !hasLine(out, "fstab changed") || !hasLine(out, "grants-complete") {
		t.Fatalf("the first write did not say it changed the block:\n%s", out)
	}
	if out := run(); !hasLine(out, "fstab unchanged") {
		t.Fatalf("the same block written again did not say it was unchanged:\n%s", out)
	}
	got, err := os.ReadFile(fstab)
	if err != nil || !strings.Contains(string(got), "LABEL=root / ext4") || !strings.Contains(string(got), "# wsl-toolkit mounts begin") {
		t.Fatalf("the block was not written beside the file's own lines: %q, %v", got, err)
	}
}
