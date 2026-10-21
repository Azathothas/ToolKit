// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// -- WSL-114: gc cleans up one project's jobs --------------------------------

// ⭐ One project's cleanup takes its own jobs, all three parts of each, and
// lists nothing of another project's as kept: those were never candidates.
func TestGCWorkspaceSelectsOneOfTwoProjects(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	mine, theirs := filepath.Join("C:", "src", "podssh"), filepath.Join("C:", "src", "podbox")
	targets := []CleanupTarget{
		{Kind: "container", Name: "wtk-aaaaaaaaaaaaaaaa (1)", JobID: "aaaaaaaaaaaaaaaa", ModTime: old, Workspace: mine},
		{Kind: "guest-dir", Name: "/home/toolkit/.wsl-toolkit/jobs/aaaaaaaaaaaaaaaa", JobID: "aaaaaaaaaaaaaaaa", ModTime: old, Workspace: mine},
		{Kind: "host-dir", Name: "jobs/aaaaaaaaaaaaaaaa", JobID: "aaaaaaaaaaaaaaaa", ModTime: old, Workspace: mine},
		{Kind: "container", Name: "wtk-bbbbbbbbbbbbbbbb (2)", JobID: "bbbbbbbbbbbbbbbb", ModTime: old, Workspace: theirs},
		{Kind: "host-dir", Name: "jobs/bbbbbbbbbbbbbbbb", JobID: "bbbbbbbbbbbbbbbb", ModTime: old, Workspace: theirs},
		{Kind: "host-dir", Name: "jobs/cccccccccccccccc", JobID: "cccccccccccccccc", ModTime: old},
		{Kind: "session", Name: "/home/toolkit/.wsl-toolkit/sessions/dddddddddddddddd", JobID: "dddddddddddddddd", ModTime: old},
		{Kind: "host-dir", Name: "uploads/eeeeeeeeeeeeeeee", JobID: "eeeeeeeeeeeeeeee", ModTime: old},
	}
	remove, spared := CleanupPolicy{Workspace: mine}.Select(targets, now)
	got := strings.Join(Names(remove), " ")
	if len(remove) != 3 || !strings.Contains(got, "aaaaaaaaaaaaaaaa") || strings.Contains(got, "bbbb") {
		t.Fatalf("one project's cleanup chose %s; want the three parts of its own job", got)
	}
	if len(spared) != 0 {
		t.Errorf("another project's work was listed as kept, which reads as a refusal: %v", spared)
	}
	if remove, _ := (CleanupPolicy{}).Select(targets, now); len(remove) != len(targets) {
		t.Errorf("with no project named, %d of %d were chosen", len(remove), len(targets))
	}
	if runtime.GOOS == "windows" {
		if remove, _ := (CleanupPolicy{Workspace: strings.ToUpper(mine)}).Select(targets, now); len(remove) != 3 {
			t.Errorf("Windows compares paths without case, and another spelling chose %d", len(remove))
		}
	}
}

// ⛔ A live job of the named project is spared as it is in a whole cleanup:
// naming a project is not a way of saying --include-live.
func TestGCWorkspaceStillSparesLiveWork(t *testing.T) {
	ws := filepath.Join("C:", "src", "podssh")
	targets := []CleanupTarget{
		{Kind: "container", Name: "wtk-live", JobID: "aaaaaaaaaaaaaaaa", Live: true, Workspace: ws},
	}
	remove, spared := CleanupPolicy{Workspace: ws}.Select(targets, time.Now())
	if len(remove) != 0 || len(spared) != 1 {
		t.Fatalf("a running job of the named project: removed %d, spared %d", len(remove), len(spared))
	}
}

func TestAJobRecordsItsWorkspaceWithItsClaim(t *testing.T) {
	home := t.TempDir()
	id := "0123456789abcdef"
	owner, err := ClaimJob(home, id)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	if RecordedWorkspace(home, id) != "" {
		t.Fatal("a job that recorded nothing answered a workspace")
	}
	if err := owner.RecordWorkspace(""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "jobs", id, workspaceRecordName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an empty key wrote a record: %v", err)
	}
	key, err := WorkspaceKey(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.RecordWorkspace(key); err != nil {
		t.Fatal(err)
	}
	if got := RecordedWorkspace(home, id); got != key {
		t.Errorf("the record reads %q; want %q", got, key)
	}
	if RecordedWorkspace(home, "../"+id) != "" {
		t.Error("a path that is not an id was read as a job")
	}
}

// ⭐ Two spellings of one directory are one project, and a directory removed
// since still has a key, so its jobs can still be cleaned up by name.
func TestWorkspaceKeyNamesOneProjectHoweverItIsSpelled(t *testing.T) {
	dir := t.TempDir()
	a, err := WorkspaceKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := WorkspaceKey(filepath.Join(dir, "sub", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if !SameWorkspace(a, b) {
		t.Errorf("%q and %q are one directory and two keys", a, b)
	}
	gone, err := WorkspaceKey(filepath.Join(dir, "removed"))
	if err != nil || !SameWorkspace(gone, filepath.Join(a, "removed")) {
		t.Errorf("a directory that does not exist keyed as %q, %v", gone, err)
	}
}

func TestANarrowingNothingSatisfiesIsAnsweredAsItsKind(t *testing.T) {
	targets := []CleanupTarget{
		{Kind: "host-dir", Name: "jobs/aaaaaaaaaaaaaaaa", JobID: "aaaaaaaaaaaaaaaa", Workspace: "C:/src/a"},
	}
	if _, err := (CleanupPolicy{Job: "bbbbbbbbbbbbbbbb"}).Unmatched(targets); !errors.Is(err, ErrUnknownJob) || !strings.Contains(err.Error(), "logs") {
		t.Errorf("a job nothing here belongs to answered %v; want a refusal naming where the jobs are listed", err)
	}
	if note, err := (CleanupPolicy{Job: "aaaaaaaaaaaaaaaa"}).Unmatched(targets); err != nil || note != "" {
		t.Errorf("a job that is here answered %q, %v", note, err)
	}
	note, err := (CleanupPolicy{Workspace: "C:/src/b"}).Unmatched(targets)
	if err != nil || !strings.Contains(note, "C:/src/b") {
		t.Errorf("a project with nothing here answered %q, %v; want a note and no refusal", note, err)
	}
}

// ⛔ The helper's jobs record its uploads and an older helper drops the field,
// so the client refuses before it sends anything.
func TestTheHelperIsNotAskedForOneProjectsCleanup(t *testing.T) {
	plan, err := (&HelperClient{}).Cleanup(context.Background(), true, CleanupPolicy{Workspace: "C:/src/a"}, false)
	if !errors.Is(err, ErrWorkspaceViaHelper) {
		t.Fatalf("one project's cleanup through the helper answered %v", err)
	}
	if len(plan.Removed)+len(plan.Containers)+len(plan.HostDirs) != 0 {
		t.Errorf("a refused cleanup carried a plan: %+v", plan)
	}
}

// ⭐ An unknown job the helper names is the same refusal the direct route
// makes, so the command exits 2 either way, and its words are not doubled.
// WSL-115.
func TestTheHelpersUnknownJobIsTheDirectRoutesRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema":"x","plan":{"schema":"wsl-toolkit-cleanup/1","dry_run":true},"error":"no such job: 0123456789abcdef. Nothing in this instance belongs to it","unknown":true}`))
	}))
	defer srv.Close()
	c := &HelperClient{endpoint: HelperEndpoint{Address: strings.TrimPrefix(srv.URL, "http://"), Token: "t"}, http: srv.Client()}
	_, err := c.Cleanup(context.Background(), false, CleanupPolicy{Job: "0123456789abcdef"}, false)
	if !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("the helper's unknown job answered %v", err)
	}
	if strings.Count(err.Error(), ErrUnknownJob.Error()) != 1 {
		t.Errorf("the refusal repeats itself: %v", err)
	}
}
