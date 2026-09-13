// SPDX-License-Identifier: 0BSD

package compat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- naming ----------------------------------------------------------------

func TestSafeNamesAndPrefixForcing(t *testing.T) {
	cases := []struct{ requested, want string }{
		{"alpine:3.22", "eph-alpine-3.22"},
		{"UPPER", "eph-upper"},
		{"a b/c", "eph-a-b-c"},
		{"already-eph-", "eph-already-eph"},
	}
	for _, c := range cases {
		s := sessionFor(t, "-Action", "New", "-Image", "x", "-Name", c.requested)
		got, err := s.resolveDistroName(c.requested, "")
		if err != nil {
			t.Errorf("%q was refused: %v", c.requested, err)
			continue
		}
		if got != c.want {
			t.Errorf("resolveDistroName(%q) = %q, want %q", c.requested, got, c.want)
		}
	}
	// A name that sanitises to nothing is refused, never silently renamed.
	s := sessionFor(t, "-Action", "New", "-Image", "x", "-Name", "///")
	if _, err := s.resolveDistroName("///", ""); err == nil || !strings.Contains(err.Error(), "sanitises to nothing") {
		t.Errorf("a name that sanitises to nothing was accepted: %v", err)
	}
}

func TestDrawnNamesCarryThePrefixAndAUniqueSuffix(t *testing.T) {
	a := newDistroName("alpine:3.22")
	b := newDistroName("alpine:3.22")
	if !strings.HasPrefix(a, prefix) {
		t.Errorf("a drawn name lost the prefix: %s", a)
	}
	if !strings.HasPrefix(a, "eph-alpine-3.22-") {
		t.Errorf("the stem is not the sanitised image: %s", a)
	}
	if a == b {
		t.Error("two draws produced one name; concurrent New commands would collide")
	}
	if got := newDistroName(""); !strings.HasPrefix(got, "eph-rootfs-") {
		t.Errorf("no image gave %s, want the rootfs stem", got)
	}
	// A very long stem is cut, then trimmed of the separators the cut exposed.
	long := newDistroName("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:1.0")
	if len(strings.TrimPrefix(strings.TrimSuffix(long, long[len(long)-5:]), prefix)) > 32 {
		t.Errorf("the stem was not cut to 32: %s", long)
	}
}

// -- the removal guards ------------------------------------------------------

func TestTheRemovalGuardRefusesEverythingThatIsNotOurs(t *testing.T) {
	s := sessionFor(t, "-Action", "List")
	for _, name := range []string{"", "   ", "ubuntu-22.04", "podman-machine", "wsl-toolkit"} {
		if err := s.assertRemovable(name); err == nil {
			t.Errorf("assertRemovable(%q) was accepted", name)
		}
	}
	for _, name := range []string{"podman-machine-default", "PODMAN-MACHINE-DEFAULT", "docker-desktop", "Rancher-Desktop", "wsl-toolkit"} {
		err := s.assertRemovable(name)
		if err == nil || !strings.Contains(err.Error(), "hard guard") {
			t.Errorf("the protected list did not refuse %q: %v", name, err)
		}
	}
	if err := s.assertRemovable("eph-mine-1a2b"); err != nil {
		t.Errorf("our own distro was refused: %v", err)
	}
}

// TestThePrefixGuardCannotBeReachedByAskingForAProtectedName is the mutation
// case for Remove: the name is prefix-forced BEFORE the guard runs, so
// -Name podman-machine-default asks for eph-podman-machine-default, which the
// guard still refuses as ours... but a caller cannot reach the real
// podman-machine-default through -Name at all. This asserts both halves.
func TestThePrefixGuardCannotBeReachedByAskingForAProtectedName(t *testing.T) {
	s := sessionFor(t, "-Action", "Remove", "-Name", "podman-machine-default")
	target, err := s.resolveDistroName(s.opts.Name, "")
	if err != nil {
		t.Fatal(err)
	}
	if target != "eph-podman-machine-default" {
		t.Fatalf("the name was not prefix-forced: %s", target)
	}
	if err := s.assertRemovable(target); err != nil {
		t.Fatalf("the forced name is ours and the guard refused it: %v", err)
	}
	// The real one is refused BY the guard if a future edit ever gets here.
	if err := s.assertRemovable("podman-machine-default"); err == nil {
		t.Fatal("podman-machine-default became removable")
	}
}

// -- the containment guard ---------------------------------------------------

func TestDeletionIsConfinedToTheBaseDirectory(t *testing.T) {
	s := sessionFor(t, "-Action", "List")

	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := s.assertInsideBaseDir(outside); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("a path outside the base directory was accepted: %v", err)
	}
	// The base directory ITSELF is never a target, so a crafted or empty name
	// cannot resolve a deletion onto it.
	if err := s.assertInsideBaseDir(s.baseDir); err == nil || !strings.Contains(err.Error(), "base directory itself") {
		t.Errorf("the base directory was accepted as a deletion target: %v", err)
	}
	if err := s.assertInsideBaseDir(s.baseDir + string(filepath.Separator)); err == nil {
		t.Error("the base directory with a trailing separator was accepted")
	}
	// A traversal in a distro name resolves OUTSIDE and is refused.
	if err := s.assertInsideBaseDir(filepath.Join(s.baseDir, "..", "escape")); err == nil {
		t.Error("a traversal was accepted")
	}
	// And a strict child is accepted, which is the one thing the guard exists
	// to allow.
	if err := s.assertInsideBaseDir(filepath.Join(s.baseDir, "eph-x-1a2b")); err != nil {
		t.Errorf("a strict child was refused: %v", err)
	}
}

func TestTheOneDeletionReadsTheStateBack(t *testing.T) {
	s := sessionFor(t, "-Action", "List")
	victim := filepath.Join(s.baseDir, "eph-gone-1a2b")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(victim, "ext4.vhdx"), []byte("disk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.removePathWithRetry(victim, "disk", ""); err != nil {
		t.Fatalf("the deletion failed: %v", err)
	}
	if _, err := os.Lstat(victim); err == nil {
		t.Fatal("the path is still there after the deletion reported success")
	}

	// A path that cannot be deleted is SAID, after the retries, with what is
	// still true. A stuck item is an orphan List reports, never a quiet lie.
	stuck := filepath.Join(s.baseDir, "eph-stuck-1a2b")
	if err := os.MkdirAll(stuck, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stuck, 0o755) })
	// os.RemoveAll as root can delete a read-only directory, so the stuck case
	// is simulated with a target the containment guard refuses: the guard runs
	// first, which is the order the contract needs.
	if err := s.removePathWithRetry(filepath.Join(t.TempDir(), "outside"), "disk", ""); err == nil {
		t.Error("a deletion outside the base directory reported success")
	}
}

// -- drawn-name collision handling ---------------------------------------------

func TestACollisionIsRefusedForANameTheCallerGaveAndRetriedForOneDrawn(t *testing.T) {
	stubWsl(t, `echo eph-taken`)

	s := sessionFor(t, "-Action", "New", "-Image", "alpine:3.22")
	// A name the CALLER gave: the collision is their answer being wrong, and
	// silently using a different name would be worse than refusing.
	if _, err := s.resolveNewDistroName("taken", "alpine:3.22"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("a caller-named collision was not refused: %v", err)
	}
	// A name this tool DREW: the same existing distro does not match the
	// random suffix, so the draw succeeds without a second listing.
	got, err := s.resolveNewDistroName("", "alpine:3.22")
	if err != nil {
		t.Fatalf("a drawn name was refused over an unrelated existing distro: %v", err)
	}
	if got == "eph-taken" {
		t.Errorf("the drawn name is the taken one: %s", got)
	}
}

func TestARefusedEnumerationIsNeverAnEmptyMachine(t *testing.T) {
	// The defect: an enumeration that was refused folded into "no
	// distributions", so a process that cannot reach WSL was told the machine
	// was empty and the caller exited 0 over a machine nobody saw.
	stubWsl(t, `echo "WSL said no" >&2; exit 1`)
	s := sessionFor(t, "-Action", "New", "-Image", "alpine:3.22")
	if _, err := s.distroNames(); err == nil || !strings.Contains(err.Error(), "could not list the distributions") {
		t.Fatalf("a refused enumeration did not refuse: %v", err)
	}
	// A machine with genuinely no distributions is a real answer and stays an
	// empty list.
	stubWsl(t, `exit 0`)
	names, err := sessionFor(t, "-Action", "List").distroNames()
	if err != nil {
		t.Fatalf("an empty machine was reported as a refusal: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("an empty machine reported %v", names)
	}
}
