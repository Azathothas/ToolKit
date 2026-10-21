// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-120: a job id is easy to use ----------------------------------------

// claimEnded makes a job that ran and ended under home.
func claimEnded(t *testing.T, home, id string) {
	t.Helper()
	owner, err := ClaimJob(home, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Finish(JobResult{ID: id}); err != nil {
		t.Fatal(err)
	}
}

func selectInstance(t *testing.T, name string) {
	t.Helper()
	previous := SelectedInstance
	t.Cleanup(func() { SelectedInstance = previous })
	SelectedInstance = Instance{Name: name, Distro: InstanceDistro(name)}
}

func again(instance, id string) string { return ToolCommandFor(instance, "logs "+id) }

// ⭐ The id, its container's name and a unique prefix of either name one job,
// and a full id passes through whether or not anything here holds it.
func TestAJobIsFoundByItsIDItsContainerAndAPrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	selectInstance(t, DefaultInstance)
	claimEnded(t, home, testJobID)
	claimEnded(t, home, "fedcba9876543210")
	for _, raw := range []string{testJobID, "wtk-" + testJobID, "0123", "01234567", "wtk-0123"} {
		got, err := ResolveJobID(home, raw, again)
		if err != nil || got != testJobID {
			t.Errorf("%q resolved to %q, %v; want %s", raw, got, err, testJobID)
		}
	}
	// ⛔ A full id nothing here holds passes through, so its own message stays.
	if got, err := ResolveJobID(home, "aaaaaaaaaaaaaaaa", again); err != nil || got != "aaaaaaaaaaaaaaaa" {
		t.Errorf("an unknown full id answered %q, %v", got, err)
	}
	// ⛔ A word that is no form of an id passes through, and so does a prefix
	// shorter than four characters.
	for _, raw := range []string{"latest", "012", "wtk-012", "0123456789ABCDEF", "../x"} {
		if got, err := ResolveJobID(home, raw, again); err != nil || got != raw {
			t.Errorf("%q was read as %q, %v; it is no form of an id and passes through", raw, got, err)
		}
	}
}

// ⛔ A prefix that names no job, or more than one, is refused and says which.
func TestAPrefixThatNamesNoSingleJobIsRefused(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	selectInstance(t, DefaultInstance)
	claimEnded(t, home, "abcd000000000001")
	claimEnded(t, home, "abcd000000000002")
	_, err := ResolveJobID(home, "abcd", again)
	if !errors.Is(err, ErrJobPrefix) || !strings.Contains(err.Error(), "abcd000000000001") || !strings.Contains(err.Error(), "abcd000000000002") {
		t.Errorf("a prefix of two jobs answered %v; want a refusal naming both", err)
	}
	_, err = ResolveJobID(home, "beef", again)
	if !errors.Is(err, ErrJobPrefix) || !strings.Contains(err.Error(), "logs") || !strings.Contains(err.Error(), "starts with it") {
		t.Errorf("a prefix of no job answered %v; want a refusal naming logs", err)
	}
}

// ⛔ A job another instance ran is refused here, with the command that reaches
// it there: its own record says whose it is, and a job under another
// instance's state is found and named too.
func TestAJobOfAnotherInstanceIsRefusedWithItsCommand(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", root)
	t.Setenv(InstanceRootEnv, root)
	selectInstance(t, "podbox")
	claimEnded(t, root, testJobID)
	selectInstance(t, DefaultInstance)
	_, err := ResolveJobID(root, "0123", again)
	if !errors.Is(err, ErrJobOfAnotherInstance) || !strings.Contains(err.Error(), "wsl-toolkit --instance podbox logs "+testJobID) {
		t.Errorf("a job instance podbox ran answered %v; want its command under --instance podbox", err)
	}

	other := filepath.Join(root, "instances", "muse")
	claimEnded(t, other, "feed000000000001")
	_, err = ResolveJobID(root, "feed", again)
	if !errors.Is(err, ErrJobOfAnotherInstance) || !strings.Contains(err.Error(), "wsl-toolkit --instance muse logs feed000000000001") {
		t.Errorf("a prefix of instance muse's job answered %v; want its command under --instance muse", err)
	}
}

// ⭐ The record is written by the first claim and kept by a later one.
func TestAJobRecordsTheInstanceThatClaimedItFirst(t *testing.T) {
	home := t.TempDir()
	selectInstance(t, "podbox")
	claimEnded(t, home, testJobID)
	selectInstance(t, DefaultInstance)
	claimEnded(t, home, testJobID)
	if name, known := RecordedInstance(home, testJobID); !known || name != "podbox" {
		t.Errorf("the record reads %q, %v; want podbox", name, known)
	}
	if err := os.Remove(filepath.Join(home, "jobs", testJobID, instanceRecordName)); err != nil {
		t.Fatal(err)
	}
	if _, known := RecordedInstance(home, testJobID); known {
		t.Error("a job with no record was read as knowing its instance")
	}
}
