// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// -- WSL-200: gc cleans this machine's half when the base is gone -------------

// noBaseHome is a state directory whose base was removed: an incoming directory
// and its open record, a job's host directory and its open record, a session of
// the removed base, and that base's open build record. Nothing is live, and the
// fake WSL lists no distribution.
type noBaseHome struct {
	h        *fakeHost
	r        *Runner
	home     string
	incoming string
	job      string
	session  string
}

const (
	noBaseJob      = "aaaaaaaaaaaaaaaa"
	noBaseSession  = "bbbbbbbbbbbbbbbb"
	noBaseIncoming = "cccccccccccccccc"
)

func newNoBaseHome(t *testing.T) *noBaseHome {
	t.Helper()
	h := newFakeHost(t)
	h.say(t, "registered", "")
	home := t.TempDir()
	cfg := DefaultConfig()
	cfg.Base.Name = fakeBaseName
	b := &Base{cfg: cfg, home: home, wsl: &Wsl{Path: h.wsl}, log: func(string) {}, disks: h.registry}
	n := &noBaseHome{h: h, home: home,
		r:        &Runner{cfg: cfg, base: b, wsl: b.wsl, home: home, ledger: ledgerAt(home), log: func(string) {}},
		incoming: filepath.Join(home, "incoming", noBaseIncoming),
		job:      filepath.Join(home, "jobs", noBaseJob),
		session:  filepath.Join(home, "jobs", noBaseSession),
	}
	for _, dir := range []string{n.incoming, n.job, filepath.Join(home, "base")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(n.incoming, "stdout.log"), []byte("out\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour).UTC()
	if err := writeSessionRecord(home, SessionRecord{Schema: SessionSchema, ID: noBaseSession, Distro: fakeBaseName, User: "toolkit",
		GuestDir: "/home/toolkit/" + GuestRoot + "/sessions/" + noBaseSession, Started: old}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []LedgerEntry{
		{Event: "open", Kind: "incoming", ID: noBaseIncoming, HostDir: n.incoming, At: old},
		{Event: "open", Kind: "job", ID: noBaseJob, Distro: fakeBaseName, GuestDir: "/home/toolkit/" + GuestRoot + "/jobs/" + noBaseJob, At: old},
		{Event: "open", Kind: baseBuildKind, ID: fakeBaseName, Distro: fakeBaseName, HostDir: filepath.Join(home, "base"), At: old},
	} {
		if err := n.r.ledger.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	return n
}

// openRecords is how many records the ledger holds open.
func (n *noBaseHome) openRecords(t *testing.T) int {
	t.Helper()
	open, err := n.r.ledger.Open()
	if err != nil {
		t.Fatal(err)
	}
	return len(open)
}

// ⛔ WITH NO BASE, `gc` PLANS AND `gc --apply` REMOVES THIS MACHINE'S HALF. It
// returned first, so resources listed what gc --apply never cleared. The dry run
// removes nothing, the apply removes what it planned and closes each stale
// record, and resources then lists none of it. WSL-200.
func TestGCWithNoBaseCleansThisMachinesHalf(t *testing.T) {
	n := newNoBaseHome(t)
	ctx := context.Background()
	before := n.r.Resources(ctx)
	if before.Owned.BaseRegistered || len(before.Owned.HostStaging) != 1 || len(before.Owned.Sessions) != 1 || len(before.Owned.OpenRecords) != 3 {
		t.Fatalf("resources before the cleanup: registered %v, staging %d, sessions %d, open records %d; want false, 1, 1 and 3",
			before.Owned.BaseRegistered, len(before.Owned.HostStaging), len(before.Owned.Sessions), len(before.Owned.OpenRecords))
	}
	if before.Owned.Sessions[0].State != "gone" {
		t.Fatalf("the removed base's session reads %q, want gone", before.Owned.Sessions[0].State)
	}

	plan, err := n.r.CleanupHost(ctx, false, CleanupPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.DryRun || len(plan.HostDirs) != 2 || len(plan.Sessions) != 1 || len(plan.Entries) != 3 {
		t.Fatalf("the dry run planned %+v; want the job's and the incoming directory, and the session", plan)
	}
	for _, e := range plan.Entries {
		if e.Action != CleanupRemove || e.Result != CleanupPlanned {
			t.Errorf("a dry run's entry reads %+v", e)
		}
	}
	for _, p := range []string{n.incoming, n.job, n.session} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("the dry run removed %s: %v", p, err)
		}
	}
	if got := n.openRecords(t); got != 3 {
		t.Errorf("the dry run closed records: %d open, want 3", got)
	}

	plan, err = n.r.CleanupHost(ctx, true, CleanupPolicy{})
	if err != nil || len(plan.Failed) != 0 {
		t.Fatalf("the apply answered %v, failed %v", err, plan.Failed)
	}
	for _, e := range plan.Entries {
		if e.Result != CleanupRemoved {
			t.Errorf("an applied entry reads %+v", e)
		}
	}
	for _, p := range []string{n.incoming, n.job, n.session} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the apply left %s: %v", p, err)
		}
	}
	if got := n.openRecords(t); got != 0 {
		t.Errorf("the apply left %d record(s) open, want none", got)
	}
	after := n.r.Resources(ctx)
	if len(after.Owned.HostStaging) != 0 || len(after.Owned.Sessions) != 0 || len(after.Owned.OpenRecords) != 0 {
		t.Errorf("resources after the cleanup: staging %v, sessions %v, records %v; want none of each",
			after.Owned.HostStaging, after.Owned.Sessions, after.Owned.OpenRecords)
	}
}

// ⛔ `gc` CHOOSES THIS MACHINE'S HALF WHERE THE BASE IS GONE, FOR BOTH ROUTES. The
// command chose for itself, so its choice was proved only where wsl.exe runs. GC
// chooses for the command and the helper, and a registered base is never
// cleaned as a gone one. WSL-200.
func TestGCChoosesThisMachinesHalfWhereTheBaseIsGone(t *testing.T) {
	n := newNoBaseHome(t)
	ctx := context.Background()
	var said []string
	n.r.log = func(s string) { said = append(said, s) }
	plan, err := n.r.GC(ctx, false, CleanupPolicy{}, false)
	if err != nil || !plan.DryRun || len(plan.HostDirs) != 2 || len(plan.Sessions) != 1 {
		t.Fatalf("gc with no base planned %+v, %v; want the two host directories and the session", plan, err)
	}
	for _, p := range []string{n.incoming, n.job, n.session} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("the dry run removed %s: %v", p, err)
		}
	}
	plan, err = n.r.GC(ctx, true, CleanupPolicy{}, false)
	if err != nil || plan.DryRun || len(plan.Entries) != 3 {
		t.Fatalf("gc --apply with no base answered %+v, %v", plan, err)
	}
	for _, p := range []string{n.incoming, n.job, n.session} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("gc --apply with no base left %s: %v", p, err)
		}
	}
	if got := n.openRecords(t); got != 0 {
		t.Errorf("gc --apply with no base left %d record(s) open, want none", got)
	}

	// ⛔ A REGISTERED BASE IS CLEANED WITH ITS GUEST. The fake refuses the guest's
	// scripts, so that cleanup fails, and it never reads the base as gone.
	n.h.say(t, "registered", fakeBaseName)
	said = nil
	_, _ = n.r.GC(ctx, false, CleanupPolicy{}, false)
	if s := strings.Join(said, "\n"); strings.Contains(s, "is not registered") {
		t.Errorf("a registered base was cleaned as a gone one:\n%s", s)
	}
}

// ⛔ AN INSTANCE WITH NO BASE STILL REFUSES AN ID NOTHING IN IT BELONGS TO, and
// still narrows to one it holds. The command refused it before it chose, and GC
// refuses it from the survey it cleans. WSL-115, WSL-200.
func TestGCWithNoBaseStillRefusesAnUnknownJob(t *testing.T) {
	n := newNoBaseHome(t)
	ctx := context.Background()
	if _, err := n.r.GC(ctx, true, CleanupPolicy{Job: "dddddddddddddddd"}, false); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("an id nothing here belongs to answered %v; want the unknown-job refusal", err)
	}
	for _, p := range []string{n.incoming, n.job, n.session} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("a refused cleanup removed %s: %v", p, err)
		}
	}
	plan, err := n.r.GC(ctx, false, CleanupPolicy{Job: noBaseJob}, false)
	if err != nil || len(plan.HostDirs) != 1 || plan.HostDirs[0] != n.job || len(plan.Sessions) != 0 {
		t.Fatalf("an id this instance holds answered %+v, %v; want its host directory alone", plan, err)
	}
}

// ⚠ A WSL THAT CANNOT BE ASKED CLEANS UP NEITHER HALF. Read as no base, a
// refused listing would remove this machine's half of a base that is still
// registered. The command exits 2 over it, as it did when it asked. WSL-200.
func TestGCThatCannotAskWSLCleansNothing(t *testing.T) {
	n := newNoBaseHome(t)
	n.h.say(t, "listing", "denied")
	plan, err := n.r.GC(context.Background(), true, CleanupPolicy{}, false)
	if !errors.Is(err, ErrRegistrationUnread) || !errors.Is(err, ErrWslDenied) || plan.Schema != CleanupSchema || len(plan.Entries) != 0 {
		t.Fatalf("gc that could not ask WSL answered %+v, %v; want the unread refusal and an empty plan", plan, err)
	}
	for _, p := range []string{n.incoming, n.job, n.session} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("gc that could not ask WSL removed %s: %v", p, err)
		}
	}
	if got := n.openRecords(t); got != 3 {
		t.Errorf("gc that could not ask WSL closed records: %d open, want 3", got)
	}
}

// ⛔ THE POLICY IS THE ONE A CLEANUP WITH A BASE HAS. A transfer still in flight
// is kept, a job nothing here belongs to is refused, and a build that holds the
// base's lock keeps its record. WSL-200.
func TestGCWithNoBaseKeepsWhatACleanupWithABaseKeeps(t *testing.T) {
	n := newNoBaseHome(t)
	ctx := context.Background()
	if _, err := n.r.CleanupHost(ctx, false, CleanupPolicy{Job: "dddddddddddddddd"}); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("a job nothing here belongs to answered %v; want the unknown-job refusal", err)
	}
	if err := n.r.ledger.Append(LedgerEntry{Event: "open", Kind: "incoming", ID: noBaseIncoming, HostDir: n.incoming}); err != nil {
		t.Fatal(err)
	}
	release, err := lockExclusive(filepath.Join(n.home, baseLockName))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	plan, err := n.r.CleanupHost(ctx, true, CleanupPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(n.incoming); err != nil {
		t.Errorf("a transfer still in flight was removed: %v", err)
	}
	kept := strings.Join(plan.Kept, "\n")
	if !strings.Contains(kept, n.incoming) || !strings.Contains(kept, "record "+baseBuildKind+"/"+fakeBaseName) {
		t.Errorf("the plan keeps %q; want the transfer and the build's record", kept)
	}
	open, err := n.r.ledger.Open()
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, e := range open {
		kinds = append(kinds, e.Kind)
	}
	if strings.Join(kinds, " ") != "base incoming" && strings.Join(kinds, " ") != "incoming base" {
		t.Errorf("the open records are %v; want the build's and the transfer's", kinds)
	}
}

// ⛔ A BUILD'S RECORD IS READ AGAIN UNDER THE LOCK. A build that registered its
// distribution after the records were read is one a later process must still
// find, so its record stays open. WSL-150, WSL-200.
func TestABuildThatRegisteredItsBaseKeepsItsRecord(t *testing.T) {
	n := newNoBaseHome(t)
	n.h.say(t, "registered", fakeBaseName)
	plan := CleanupPlan{}
	build := LedgerEntry{Kind: baseBuildKind, ID: fakeBaseName, Distro: fakeBaseName}
	n.r.closeBuildRecords(context.Background(), &plan, []LedgerEntry{build})
	if len(plan.Removed) != 0 || len(plan.Failed) != 0 {
		t.Fatalf("a build whose base is registered answered removed %v, failed %v", plan.Removed, plan.Failed)
	}
	open, err := n.r.ledger.Open()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range open {
		if e.Kind == baseBuildKind {
			return
		}
	}
	t.Fatalf("the record of a build whose base is registered was closed: %+v", open)
}

// ⭐ A RECORD WHOSE OWNER IS GONE IS CLOSED, as a cleanup with a base closes it,
// even where the age rule keeps its directory. WSL-121, WSL-200.
func TestGCWithNoBaseClosesARecordWhoseOwnerIsGone(t *testing.T) {
	n := newNoBaseHome(t)
	const lost = "eeeeeeeeeeeeeeee"
	owner, err := ClaimJob(n.home, lost)
	if err != nil {
		t.Fatal(err)
	}
	owner.Release()
	if err := n.r.ledger.Append(LedgerEntry{Event: "open", Kind: "job", ID: lost}); err != nil {
		t.Fatal(err)
	}
	plan, err := n.r.CleanupHost(context.Background(), true, CleanupPolicy{OlderThan: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(n.home, "jobs", lost)); err != nil {
		t.Fatalf("the age rule did not keep the young job's directory: %v", err)
	}
	if !strings.Contains(strings.Join(plan.Removed, "\n"), "record job/"+lost+": its owner was lost") {
		t.Errorf("the record whose owner is gone was not closed: %v", plan.Removed)
	}
}

// ⛔ THROUGH THE HELPER TOO. Its cleanup asked the guest for its home first, and
// failed over a base that is not registered. WSL-200.
func TestTheHelpersGCWithNoBaseCleansThisMachinesHalf(t *testing.T) {
	n := newNoBaseHome(t)
	h := &HelperServer{runner: n.r, log: func(string) {}}
	rec := httptest.NewRecorder()
	h.handleGC(rec, httptest.NewRequest(http.MethodPost, "/v1/gc", strings.NewReader(`{"apply":true}`)))
	var got struct {
		Plan  CleanupPlan `json:"plan"`
		Error string      `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("the helper answered %q: %v", rec.Body.String(), err)
	}
	if got.Error != "" || got.Plan.DryRun || len(got.Plan.HostDirs) != 2 || len(got.Plan.Sessions) != 1 {
		t.Fatalf("the helper's cleanup with no base answered %+v", got)
	}
	if _, err := os.Stat(n.incoming); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the helper's apply left %s: %v", n.incoming, err)
	}
}
