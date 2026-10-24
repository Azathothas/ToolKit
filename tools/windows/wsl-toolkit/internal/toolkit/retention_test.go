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

// ⭐ An ended ephemeral job's record is removed and the answer says so, and one
// that holds what the answer does not is kept, with its own cause.
func TestRemoveRecordLeavesNothingUnlessTheRecordIsTheOnlyCopy(t *testing.T) {
	home := t.TempDir()
	res := endedEphemeral(t, home)
	RemoveJobRecord(home, res.ContainerLifecycle, &res)
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
		RemoveJobRecord(home, res.ContainerLifecycle, &res)
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

// ⛔ An ephemeral job whose teardown failed keeps its record and its owner's
// lock, so gc reads its owner as gone, and its answer claims no removal.
// WSL-121.
func TestAnEphemeralJobWhoseTeardownFailedKeepsItsRecord(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "teardown-fails"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(staleFakeWslEnv, dir)
	t.Setenv("WSL_TOOLKIT_HOME", home)
	cfg := Config{Base: BaseConfig{Name: "wsl-toolkit-stale", Image: "docker.io/library/alpine:latest", User: "agent"}}
	r := &Runner{cfg: cfg, wsl: &Wsl{Path: self}, home: home, ledger: &Ledger{path: filepath.Join(home, "ledger.jsonl")}, log: func(string) {}}
	res := r.Run(t.Context(), JobSpec{
		Image: "docker.io/library/alpine:latest", Script: []byte("true\n"),
		ContainerLifecycle: ContainerEphemeral,
	})
	if res.Unreached || res.TeardownError == "" {
		t.Fatalf("a failed teardown is not on the result: %+v", res)
	}
	RemoveJobRecord(home, ContainerEphemeral, &res)
	if res.RecordRemoved || !strings.Contains(res.RecordKept, "teardown failed") {
		t.Fatalf("a job whose teardown failed reads removed %v, kept %q", res.RecordRemoved, res.RecordKept)
	}
	if _, err := os.Stat(filepath.Join(home, "jobs", res.ID, ownerLockName)); err != nil {
		t.Fatalf("the owner's lock is gone: %v", err)
	}
	if !ownerGone(home, res.ID) {
		t.Error("gc does not read the owner of a job whose teardown failed as gone")
	}
}

// ⛔ A guest directory left behind is named as itself, never as artifacts that
// were not delivered: a job that asked for none could not have had that cause.
func TestAKeptRecordNamesItsOwnCause(t *testing.T) {
	home := t.TempDir()
	res := endedEphemeral(t, home)
	res.GuestDir = "/guest/jobs/" + testJobID
	RemoveJobRecord(home, ContainerEphemeral, &res)
	if strings.Contains(res.RecordKept, "artifacts") || !strings.Contains(res.RecordKept, res.GuestDir) {
		t.Errorf("a left guest directory keeps the record because %q", res.RecordKept)
	}
	// ⚠ A reason already given stands, and nothing is removed under it.
	home = t.TempDir()
	res = endedEphemeral(t, home)
	res.RecordKept = DetachedRecordKept
	RemoveJobRecord(home, ContainerEphemeral, &res)
	if res.RecordRemoved || res.RecordKept != DetachedRecordKept {
		t.Errorf("a detached job's record reads %+v", res)
	}
	if _, err := os.Stat(filepath.Join(home, "jobs", testJobID)); err != nil {
		t.Errorf("a detached job's record is gone: %v", err)
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
