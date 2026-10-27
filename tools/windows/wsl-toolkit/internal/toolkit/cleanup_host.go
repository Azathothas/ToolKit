// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrRegistrationUnread is a `gc` that could not ask WSL whether the base is
// registered, so it cleaned up neither half. WSL-200.
var ErrRegistrationUnread = errors.New("WSL could not say whether the base is registered, so nothing was cleaned up")

// GC is the cleanup `gc` runs, directly or through the helper: the cleanup with a
// base where the base is registered, and this machine's half alone where it is
// not. WSL-200.
//
// ⛔ ONE CHOICE FOR BOTH ROUTES. The command and the helper each chose for
// themselves, and the command's choice could be proved only where wsl.exe runs.
// A host without it could not see the fix go.
func (r *Runner) GC(ctx context.Context, apply bool, policy CleanupPolicy, images bool) (CleanupPlan, error) {
	registered, err := r.wsl.Exists(ctx, r.cfg.Base.Name)
	if err != nil {
		return CleanupPlan{Schema: CleanupSchema, DryRun: !apply, Entries: []CleanupEntry{}}, fmt.Errorf("%w: %w", ErrRegistrationUnread, err)
	}
	if !registered {
		return r.CleanupHost(ctx, apply, policy)
	}
	return r.Cleanup(ctx, apply, policy, images)
}

// CleanupHost is a cleanup for an instance whose base is not registered: this
// machine's half alone, with the plan, the dry run, the entries and the verdict
// a cleanup with a base has. WSL-200.
//
// ⛔ IT RETURNED BEFORE IT LOOKED. With no base registered, `gc` and `gc --apply`
// said there was nothing inside it to clean up and exited 0. `resources` still
// listed the open records, the incoming directories and the removed base's
// sessions, and said `gc --apply` cleared them. Measured after `base remove`: 4
// open records, 4 incoming directories and 2 gone sessions, and none removed.
//
// ⭐ THE SAME SURVEY AND THE SAME POLICY. hostTargets is the half the cleanup
// with a base reads, and Select decides as it does there, so a live job, a
// transfer in flight and the age rule spare what they spare there.
//
// ⚠ A named cache and an image live in the base's engine, so with no base there
// is none of either to clean up.
func (r *Runner) CleanupHost(ctx context.Context, apply bool, policy CleanupPolicy) (CleanupPlan, error) {
	plan := CleanupPlan{Schema: CleanupSchema, DryRun: !apply, Entries: []CleanupEntry{}}
	if policy.Caches {
		r.log(r.cfg.Base.Name + " is not registered, so it holds no named cache to clean up")
		return plan, nil
	}
	r.log(r.cfg.Base.Name + " is not registered, so only what this machine holds for it is cleaned up")
	openIDs, ledgerErr := r.openJobIDs()
	liveJob := map[string]bool{}
	for id := range openIDs {
		liveJob[id] = true
	}
	targets := r.hostTargets(ctx, liveJob, r.workspaceReader())
	if ledgerErr != nil {
		r.log("the ledger could not be read, so a job that is still running may not be seen as live: " + ledgerErr.Error())
	}
	note, err := policy.Unmatched(targets)
	if err != nil {
		return plan, err
	}
	if note != "" {
		r.log(note)
	}
	remove, spared := policy.Select(targets, time.Now())
	for _, sp := range spared {
		plan.Kept = append(plan.Kept, sp.Target.Name+": "+sp.Reason)
	}
	plan.HostDirs = Names(Of(remove, "host-dir"))
	plan.Sessions = Names(Of(remove, "session"))
	plan.Entries = cleanupEntries(remove, spared, SelectedInstance.Name)
	if !apply {
		return plan, nil
	}

	sessionDone := r.removeSessions(ctx, &plan, Of(remove, "session"))
	hostDone := r.removeHostDirs(&plan, Of(remove, "host-dir"))
	for i, t := range remove {
		e := &plan.Entries[i]
		switch t.Kind {
		case "session":
			settleFrom(e, sessionDone, t.JobID)
		case "host-dir":
			settleFrom(e, hostDone, t.Name)
		}
	}
	r.closeHostRecords(ctx, &plan)
	// ⚠ ONLY A LEDGER THAT IS THERE IS COMPACTED, so a state directory with none
	// is not given one.
	if _, err := os.Stat(r.ledger.path); err == nil {
		if n, err := r.ledger.Compact(); err == nil {
			plan.Removed = append(plan.Removed, fmt.Sprintf("ledger compacted to %d open record(s)", n))
		}
	}
	if len(plan.Failed) > 0 {
		return plan, fmt.Errorf("%d item(s) could not be removed", len(plan.Failed))
	}
	return plan, nil
}

// closeHostRecords closes each open record whose resource is gone, for an
// instance whose base is not registered. WSL-200.
//
// ⛔ A GUEST DIRECTORY IS GONE WITH ITS DISTRIBUTION. A record naming one in a
// distribution that is not registered is closed. One in a distribution that is
// registered is left alone, because no guest is asked here.
//
// ⛔ A BUILD'S RECORD IS CLOSED ONLY UNDER THE BASE'S LOCK, and only where its
// distribution is still not registered there. A build holds that lock from
// before its record until after its stamp, so a build in progress keeps its
// record, and a build killed after its import is still found. WSL-150.
func (r *Runner) closeHostRecords(ctx context.Context, plan *CleanupPlan) {
	open, err := r.ledger.Open()
	if err != nil {
		plan.Failed = append(plan.Failed, "the ledger could not be read: "+err.Error())
		return
	}
	registered, listErr := r.registeredNames(ctx)
	if listErr != nil {
		plan.Failed = append(plan.Failed, "WSL could not say what is registered, so no record naming a distribution was closed: "+listErr.Error())
	}
	gone := func(e LedgerEntry) bool {
		return listErr == nil && !registered[strings.ToLower(r.recordDistro(e))]
	}
	var builds []LedgerEntry
	for _, e := range open {
		note := ""
		switch {
		case e.Kind == baseBuildKind && gone(e):
			builds = append(builds, e)
			continue
		case e.GuestDir != "" && gone(e):
			note = "closed by gc: " + r.recordDistro(e) + " is not registered, so its directory is gone"
		case e.HostDir != "" && !dirExists(e.HostDir):
			note = "closed by gc: its directory is gone"
		default:
			continue
		}
		r.closeRecord(plan, e, note)
	}
	if len(builds) > 0 {
		r.closeBuildRecords(ctx, plan, builds)
	}
	// ⭐ AND A RECORD WHOSE OWNER IS GONE, as a cleanup with a base closes it.
	// WSL-121.
	lost, err := CloseLostOwners(r.home)
	if err != nil {
		plan.Failed = append(plan.Failed, "the records whose owner was lost could not be closed: "+err.Error())
	}
	for _, id := range lost {
		plan.Removed = append(plan.Removed, "record job/"+id+": its owner was lost")
	}
}

// closeBuildRecords closes the open build records of distributions that are not
// registered, holding the base's lock. A build that holds it keeps its record.
func (r *Runner) closeBuildRecords(ctx context.Context, plan *CleanupPlan, builds []LedgerEntry) {
	release, err := lockExclusive(filepath.Join(r.home, baseLockName))
	if errors.Is(err, errLockHeld) {
		for _, e := range builds {
			plan.Kept = append(plan.Kept, "record "+e.Kind+"/"+e.ID+": a build holds the base's lock, so its record is kept")
		}
		return
	}
	if err != nil {
		plan.Failed = append(plan.Failed, "the base's lock could not be taken, so no build's record was closed: "+err.Error())
		return
	}
	defer release()
	// ⛔ ASKED AGAIN UNDER THE LOCK. A build that registered its distribution
	// before this took the lock is one a later process must still find.
	registered, err := r.registeredNames(ctx)
	if err != nil {
		plan.Failed = append(plan.Failed, "WSL could not say what is registered, so no build's record was closed: "+err.Error())
		return
	}
	for _, e := range builds {
		if registered[strings.ToLower(r.recordDistro(e))] {
			continue
		}
		r.closeRecord(plan, e, "closed by gc: "+r.recordDistro(e)+" is not registered and no build holds the base's lock")
	}
}

// closeRecord appends the close of one open record, and says so in the plan.
func (r *Runner) closeRecord(plan *CleanupPlan, e LedgerEntry, note string) {
	if err := r.ledger.Append(LedgerEntry{Event: "close", Kind: e.Kind, ID: e.ID, Note: note}); err != nil {
		plan.Failed = append(plan.Failed, "could not close the record for "+e.ID+": "+err.Error())
		return
	}
	plan.Removed = append(plan.Removed, "record "+e.Kind+"/"+e.ID)
}

// recordDistro is the distribution a record names, and the configured base's
// where it names none.
func (r *Runner) recordDistro(e LedgerEntry) string {
	if e.Distro != "" {
		return e.Distro
	}
	return r.cfg.Base.Name
}

// registeredNames is every distribution WSL lists, in lower case.
func (r *Runner) registeredNames(ctx context.Context) (map[string]bool, error) {
	names, err := r.wsl.listNames(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, n := range names {
		out[strings.ToLower(n)] = true
	}
	return out, nil
}
