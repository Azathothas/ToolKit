// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-121: what a job leaves, and a record whose owner is gone ------------

// openRecord appends an open job record to home's ledger.
func openRecord(t *testing.T, home, id string) *Ledger {
	t.Helper()
	led := &Ledger{path: filepath.Join(home, "ledger.jsonl")}
	if err := led.Append(LedgerEntry{Event: "open", Kind: "job", ID: id}); err != nil {
		t.Fatal(err)
	}
	return led
}

// ⭐ A record whose owner's lock is free and which recorded no end is closed,
// with the reason; one whose owner runs, or which recorded its end, is not.
func TestARecordWhoseOwnerIsLostIsClosedWhenRead(t *testing.T) {
	home := t.TempDir()
	lost, running, ended := testJobID, "fedcba9876543210", "aaaaaaaaaaaaaaaa"
	var led *Ledger
	for _, id := range []string{lost, running, ended} {
		led = openRecord(t, home, id)
	}
	killed, err := ClaimJob(home, lost)
	if err != nil {
		t.Fatal(err)
	}
	killed.Release()
	alive, err := ClaimJob(home, running)
	if err != nil {
		t.Fatal(err)
	}
	defer alive.Release()
	claimEnded(t, home, ended)

	closed, err := CloseLostOwners(home)
	if err != nil || len(closed) != 1 || closed[0] != lost {
		t.Fatalf("closed %v, %v; want the lost owner's record alone", closed, err)
	}
	all, err := led.All()
	if err != nil {
		t.Fatal(err)
	}
	last := all[len(all)-1]
	if last.Event != "close" || last.ID != lost || last.Note != OwnerLostNote || !strings.Contains(last.Note, "owner was lost") {
		t.Errorf("the close reads %+v", last)
	}
	if again, err := CloseLostOwners(home); err != nil || len(again) != 0 {
		t.Errorf("a second read closed %v, %v", again, err)
	}
	// ⚠ A reader on a machine with no ledger writes none.
	empty := t.TempDir()
	if _, err := CloseLostOwners(empty); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(empty, "ledger.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a reader created a ledger: %v", err)
	}
}

// endedEphemeral makes an ended ephemeral job and answers its result.
func endedEphemeral(t *testing.T, home string) JobResult {
	t.Helper()
	claimEnded(t, home, testJobID)
	return JobResult{ID: testJobID, ContainerLifecycle: ContainerEphemeral, Transcript: filepath.Join(home, "jobs", testJobID)}
}

// ⭐ --remove-record removes an ended ephemeral job's record and says so, and
// keeps one that holds what the answer does not.
func TestRemoveRecordLeavesNothingUnlessTheRecordIsTheOnlyCopy(t *testing.T) {
	home := t.TempDir()
	res := endedEphemeral(t, home)
	RemoveJobRecord(home, &res)
	if !res.RecordRemoved || res.Transcript != "" || res.RecordKept != "" {
		t.Fatalf("the answer after the removal is %+v", res)
	}
	if _, err := os.Stat(filepath.Join(home, "jobs", testJobID)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the host record is still there: %v", err)
	}

	for name, change := range map[string]func(*JobResult){
		"a cut copy of stdout":       func(r *JobResult) { r.StdoutTruncated = true },
		"artifacts not delivered":    func(r *JobResult) { r.ArtifactError = "a member was refused" },
		"a persistent container":     func(r *JobResult) { r.ContainerLifecycle = ContainerPersistent },
		"a retained guest directory": func(r *JobResult) { r.GuestDir = "/home/toolkit/.wsl-toolkit/jobs/" + testJobID },
	} {
		home := t.TempDir()
		res := endedEphemeral(t, home)
		change(&res)
		RemoveJobRecord(home, &res)
		if res.RecordRemoved {
			t.Errorf("%s: the record was removed", name)
		}
		if _, err := os.Stat(filepath.Join(home, "jobs", testJobID)); err != nil {
			t.Errorf("%s: the record is gone: %v", name, err)
		}
		if res.ContainerLifecycle == ContainerEphemeral && res.RecordKept == "" {
			t.Errorf("%s: the answer does not say why the record was kept", name)
		}
	}
}

// ⭐ A plan's entries carry what a program needs: the kind, the job, the
// instance, the project, the path, the action and the result.
func TestACleanupPlanCarriesOneEntryPerTarget(t *testing.T) {
	home := filepath.Join("C:", "state")
	remove := []CleanupTarget{
		{Kind: "container", Name: "wtk-" + testJobID + " (abc123)", JobID: testJobID, Path: "wtk-" + testJobID, Workspace: "C:/src/podbox"},
		{Kind: "host-dir", Name: filepath.Join(home, "jobs", testJobID), JobID: testJobID},
		{Kind: "host-dir", Name: filepath.Join(home, "uploads", "fedcba9876543210"), JobID: "fedcba9876543210"},
	}
	spared := []Spared{{Target: CleanupTarget{Kind: "guest-dir", Name: "/home/toolkit/.wsl-toolkit/jobs/aaaaaaaaaaaaaaaa", JobID: "aaaaaaaaaaaaaaaa", Live: true}, Reason: "in use right now"}}
	entries := cleanupEntries(remove, spared, "podbox")
	if len(entries) != 4 {
		t.Fatalf("%d entries for 4 targets: %+v", len(entries), entries)
	}
	c := entries[0]
	if c.Kind != "container" || c.Job != testJobID || c.Instance != "podbox" || c.Workspace != "C:/src/podbox" ||
		c.Path != "wtk-"+testJobID || c.Action != CleanupRemove || c.Result != CleanupPlanned {
		t.Errorf("the container's entry is %+v", c)
	}
	if entries[1].Job != testJobID || entries[2].Job != "" {
		t.Errorf("a job's host directory carries job %q and an upload carries %q; want the job's id and none", entries[1].Job, entries[2].Job)
	}
	if k := entries[3]; k.Action != CleanupKeep || k.Result != CleanupKept || k.Reason != "in use right now" {
		t.Errorf("the kept entry is %+v", k)
	}
}

// ⛔ The apply settles each entry from what the guest said, and one nothing
// reported is unknown rather than removed.
func TestACleanupEntryIsSettledFromWhatTheGuestReported(t *testing.T) {
	var plan CleanupPlan
	done := readGuestCleanup(&plan, "removed container wtk-a\nFAILED directory /g/jobs/b\nFAILED refused /etc\ncleanup-complete 0\n")
	if len(plan.Removed) != 1 || len(plan.Failed) != 2 {
		t.Fatalf("the plan's strings are %+v", plan)
	}
	var removed, failed, refused, missing CleanupEntry
	settleFrom(&removed, done, "container wtk-a")
	settleFrom(&failed, done, "directory /g/jobs/b")
	settleFrom(&refused, done, "directory /etc")
	settleFrom(&missing, done, "container wtk-c")
	if removed.Result != CleanupRemoved || failed.Result != CleanupFailed || refused.Result != CleanupFailed || missing.Result != CleanupUnknown {
		t.Errorf("settled as %q, %q, %q and %q", removed.Result, failed.Result, refused.Result, missing.Result)
	}
	if !strings.Contains(refused.Reason, "refused") || missing.Reason == "" {
		t.Errorf("the reasons are %q and %q", refused.Reason, missing.Reason)
	}
}

// ⭐ A fleet's row runs under the id its caller drew for it.
func TestARowRunsUnderTheIDItsFleetDrew(t *testing.T) {
	spec := MatrixSpec{RowIDs: []string{testJobID, "fedcba9876543210"}}
	img := Image{ID: "alpine", Ref: "docker.io/library/alpine:latest"}
	if got := rowAt(spec, 1, img, "", "", DefaultWorkspaceLimits()).ID; got != "fedcba9876543210" {
		t.Errorf("row 1 runs as %q", got)
	}
	if got := rowAt(MatrixSpec{}, 0, img, "", "", DefaultWorkspaceLimits()).ID; got != "" {
		t.Errorf("a row of a fleet that drew no ids runs as %q", got)
	}
}
