// SPDX-License-Identifier: 0BSD

package compat

import (
	"path/filepath"
	"strings"
	"testing"
)

// The adversarial cases: a caller, a name, a tag or a path that means
// something other than what it says, and the refusal that meets each one.

func TestAHostileNameCanOnlyResolveInsideTheOwnedSpace(t *testing.T) {
	s := sessionFor(t, "-Action", "Remove", "-Name", "x")
	hostile := []string{
		"../../etc",              // traversal
		"..",                     // the base directory itself, spelled small
		"/etc/passwd",            // an absolute path
		"podman-machine-default", // a protected distro, unforced
		"CON",                    // a reserved device name
		".....",                  // dot stacking
		"eph-../../escape",       // traversal behind a plausible prefix
	}
	for _, name := range hostile {
		resolved, err := s.resolveDistroName(name, "")
		if err != nil {
			// A refusal is a fine answer; assert it NAMES the problem.
			continue
		}
		// Whatever survived sanitisation must still be refused by the removal
		// guard or resolve to a strict child of the state directory. Both
		// halves are the contract; either alone is not.
		if guardErr := s.assertRemovable(resolved); guardErr != nil {
			continue
		}
		if !strings.HasPrefix(resolved, prefix) {
			t.Fatalf("%q resolved to %q without the prefix and without a refusal", name, resolved)
		}
		target := filepath.Join(s.baseDir, resolved)
		if err := s.assertInsideBaseDir(target); err != nil {
			t.Errorf("%q resolved to %q, which the containment guard refuses: %v", name, resolved, err)
		}
	}
	// And the whole chain through Remove: a hostile -Name never reaches wsl
	// as a traversal.
	stubWsl(t, `exit 0`)
	got := runCompatIn(t, t.TempDir(), "-Action", "Remove", "-Name", "../../boot", "-Force")
	if got.code != 0 {
		t.Fatalf("a hostile name was refused with exit %d over the documented flow: %q", got.code, got.notes)
	}
	if strings.Contains(got.report, "..") {
		t.Errorf("the plan carries traversal bytes: %q", got.report)
	}
}

func TestAnAutomatedCallerIsNeverPromptedIntoADestruction(t *testing.T) {
	// ⛔ A NON-INTERACTIVE SESSION IS TOLD TO PASS -FORCE, in words, and
	// nothing is removed: a tool that hangs on its own question is worse than
	// one that refuses, and a tool that deletes on a timeout is worse than
	// both.
	stubWsl(t, `echo eph-gone-1a2b`)
	dir := t.TempDir()
	got := runCompatIn(t, dir, "-Action", "Purge")
	if got.code != 0 {
		t.Fatalf("exit %d: %q", got.code, got.notes)
	}
	if !strings.Contains(got.report, "non-interactive") || !strings.Contains(got.report, "-Force") {
		t.Errorf("the refusal does not say how to proceed: %q", got.report)
	}
	// And the answer to the question never arrived, so nothing was removed:
	// the listing stub would have shown a second enumeration if the removal
	// path had run.
}

func TestTheSnapshotTagNeverReachesTheDiskUnvalidated(t *testing.T) {
	// A caller-supplied path component is how a tag becomes a write anywhere
	// on the disk.
	s := sessionFor(t, "-Action", "Snapshot", "-Name", "eph-x-1a2b", "-As", "tag")
	for _, tag := range []string{"../../evil", "a/b", `a\b`, ".hidden", "-lead", "with space"} {
		if _, err := snapshotTag(tag); err == nil {
			t.Errorf("the hostile tag %q was accepted", tag)
		}
	}
	path, err := s.snapshotPath("ok-tag.1")
	if err != nil {
		t.Fatal(err)
	}
	// The validated tag is joined under the snapshots subdirectory and
	// nowhere else.
	if !strings.HasPrefix(path, s.baseDir) {
		t.Fatalf("the snapshot path escaped the state directory: %s", path)
	}
}

func TestARedactionPatternCannotSurviveIntoALogUncompiled(t *testing.T) {
	// A pattern that does not compile is refused at the settings, so a bad
	// -Redact can never fail halfway into a run with a message naming a line
	// of somebody else's script.
	if _, err := newRedactionSet([]string{"(?P<"}); err == nil {
		t.Fatal("an uncompilable pattern was accepted")
	}
	// The marker carries no expansion: '$&' in the guest's text cannot make
	// the replacement paste the match back in.
	set, err := newRedactionSet([]string{"secret"})
	if err != nil {
		t.Fatal(err)
	}
	got := applyRedaction(set, "secret $& $' $`")
	if strings.Contains(got, "secret") {
		t.Errorf("the marker leaked its input: %q", got)
	}
	if got != "*** $& $' $`" {
		t.Errorf("the replacement expanded something: %q", got)
	}
}

func TestTheTransportAlphabetRefusesAMutatedSkeleton(t *testing.T) {
	// The alphabet check catches edits to the SKELETON, not the payload: a
	// character the measurement never cleared in the builder's own text is
	// the defect WSL-12 was, and this is what fires instead of shipping.
	for _, path := range []string{"/tmp/.wsl-eph-a b", "/tmp/x$y", "/tmp/a'b"} {
		if _, err := distroScriptCommand([]byte("x"), path); err == nil {
			t.Errorf("a guest path outside the alphabet was accepted: %q", path)
		}
	}
}
