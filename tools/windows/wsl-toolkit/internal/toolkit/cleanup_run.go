// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cleanup removes what this executable owns and nothing else.
//
// ⛔ DRY RUN IS THE DEFAULT and the caller opts into acting. A tool that removed
// things by default the first time somebody ran it to see what it would do is a
// tool nobody runs a second time.
//
// ⭐ It finishes every loop before it reports. One item it cannot remove does not
// stop it removing the rest; stopping at the first failure would hide the state
// of everything after it.
func (r *Runner) Cleanup(ctx context.Context, apply bool, policy CleanupPolicy, includeImages bool) (CleanupPlan, error) {
	plan := CleanupPlan{Schema: CleanupSchema, DryRun: !apply}
	guestHome, err := r.guestHome(ctx)
	if err != nil {
		return plan, err
	}
	root := guestHome + "/" + GuestRoot

	targets, images, err := r.cleanupTargets(ctx)
	if err != nil {
		return plan, err
	}
	if includeImages {
		for _, i := range images {
			plan.Images = append(plan.Images, i.Name)
		}
	}

	// ⭐ ONE SET, FILTERED ONCE. The plan below and the apply underneath it read
	// the same slice, so a dry run cannot list something the apply spares or
	// spare something the apply removes.
	remove, spared := policy.Select(targets, time.Now())
	for _, sp := range spared {
		plan.Kept = append(plan.Kept, sp.Target.Name+": "+sp.Reason)
	}
	plan.Containers = Names(Of(remove, "container"))
	plan.GuestDirs = Names(Of(remove, "guest-dir"))
	plan.HostDirs = Names(Of(remove, "host-dir"))

	if !apply {
		return plan, nil
	}

	if err := r.applyGuestCleanup(ctx, &plan, root, remove, includeImages); err != nil {
		return plan, err
	}
	for _, t := range Of(remove, "host-dir") {
		if err := RemoveInside(r.home, t.Name); err != nil {
			plan.Failed = append(plan.Failed, "host directory "+t.Name+": "+err.Error())
			continue
		}
		plan.Removed = append(plan.Removed, "host directory "+t.Name)
	}

	r.closeStaleRecords(ctx, &plan)
	if n, err := r.ledger.Compact(); err == nil {
		plan.Removed = append(plan.Removed, fmt.Sprintf("ledger compacted to %d open record(s)", n))
	}
	if len(plan.Failed) > 0 {
		return plan, fmt.Errorf("%d item(s) could not be removed", len(plan.Failed))
	}
	return plan, nil
}

// cleanupTargets enumerates everything cleanup could act on, with each thing's
// age and whether something is using it.
//
// ⛔ A CONTAINER'S AGE IS ITS JOB DIRECTORY'S. Both are named for the same job
// and made in the same second, and reading it this way needs no timestamp parsed
// out of the engine, whose rendering differs between versions. A container whose
// directory has already gone has no age at all, which Select treats as abandoned
// rather than as new.
func (r *Runner) cleanupTargets(ctx context.Context) ([]CleanupTarget, []OwnedThing, error) {
	containers, images, _, err := r.engineHolding(ctx)
	if err != nil {
		return nil, nil, err
	}
	running, err := r.runningContainers(ctx)
	if err != nil {
		return nil, nil, err
	}
	jobs, _, _, err := r.guestJobs(ctx)
	if err != nil {
		return nil, nil, err
	}

	dirTime := map[string]time.Time{}
	for _, j := range jobs {
		if id := jobIDFromPath(j.Path); id != "" {
			dirTime[id] = j.ModTime
		}
	}

	liveJob := map[string]bool{}
	var targets []CleanupTarget
	for _, c := range containers {
		id := jobIDFromContainer(c.Name)
		live := running[c.Name]
		if live && id != "" {
			liveJob[id] = true
		}
		targets = append(targets, CleanupTarget{
			Kind: "container", Name: c.Name + " (" + c.ID + ")", JobID: id,
			Live: live, ModTime: dirTime[id],
		})
	}

	// ⚠ A JOB IS ALSO LIVE BETWEEN ITS CONTAINER EXITING AND ITS ARTIFACTS BEING
	// FETCHED. The container is gone by then, because it runs with --rm, so the
	// engine cannot answer this and the ledger has to: an open record with no
	// close is work still in progress.
	openIDs, ledgerErr := r.openJobIDs()
	for id := range openIDs {
		liveJob[id] = true
	}
	for _, j := range jobs {
		id := jobIDFromPath(j.Path)
		targets = append(targets, CleanupTarget{
			Kind: "guest-dir", Name: j.Path, JobID: id,
			Live: liveJob[id], ModTime: j.ModTime,
		})
	}

	hostJobs := filepath.Join(r.home, "jobs")
	if entries, err := os.ReadDir(hostJobs); err == nil {
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			targets = append(targets, CleanupTarget{
				Kind: "host-dir", Name: filepath.Join(hostJobs, e.Name()), JobID: e.Name(),
				Live: liveJob[e.Name()], ModTime: info.ModTime(),
			})
		}
	}
	// ⛔ AN UPLOAD AND AN ARTIFACT SET DO NOT CARRY THE JOB'S ID, so liveJob
	// cannot answer for them. The ledger can: the helper opens a record before it
	// writes either directory and closes it when the client is done, so an open
	// record is a transfer still in flight. Without this, `gc --apply` with no age
	// limit would delete an artifact set out from under the job producing it.
	openDirs := r.openHostDirs()
	for _, dir := range []string{"uploads", "artifacts", "incoming"} {
		targets = append(targets, r.helperStateTargets(dir, openDirs)...)
	}
	if ledgerErr != nil {
		// Reported rather than swallowed: a ledger that cannot be read means the
		// liveness answer above is weaker than it looks.
		r.log("the ledger could not be read, so a job between its container and its artifacts may not be seen as live: " + ledgerErr.Error())
	}
	return targets, images, nil
}

// helperStateTargets brings the helper's own two directories into the survey.
//
// ⛔ THEY WERE INVISIBLE. An upload lived in home/uploads and an artifact set in
// home/artifacts, cleanup scanned home/jobs alone, and a helper that stayed up
// accumulated every workspace any client had ever sent. `gc --apply` with no age
// limit reported nothing to do beside thirteen directories on disk.
func (r *Runner) helperStateTargets(name string, openDirs map[string]bool) []CleanupTarget {
	root := filepath.Join(r.home, name)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []CleanupTarget
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(root, e.Name())
		out = append(out, CleanupTarget{
			Kind: "host-dir", Name: path, JobID: e.Name(),
			Live: openDirs[path], ModTime: info.ModTime(),
		})
	}
	return out
}

// runningContainers is the set the engine reports as running right now.
//
// ⛔ `podman ps` WITHOUT -a, and that is the whole difference. The old apply
// listed with -a and force-removed every row it got, so a container that had
// been started seconds earlier was removed along with the abandoned ones.
func (r *Runner) runningContainers(ctx context.Context) (map[string]bool, error) {
	script := "podman ps --filter label=" + JobLabel + " --format '{{.Names}}' 2>/dev/null || :\n"
	out, stderr, code, err := r.baseCapture(ctx, []byte(script), 2*time.Minute)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("could not ask the engine what is running (exit %d): %s", code, firstLine(stderr+out))
	}
	running := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			running[name] = true
		}
	}
	return running, nil
}

// StaleAfter is how long an open record is still evidence that work is live.
//
// ⛔ AN OPEN RECORD IS NOT PROOF OF A RUNNING PROCESS. It is proof that
// something started and did not record finishing, which is exactly what a KILLED
// run leaves behind. Treating one as live forever would mean cleanup can never
// collect the leftovers of a crash, which is the case it exists for. A record
// past its own deadline, or past this window when it carries none, stops
// counting; a container the engine still reports as running counts whatever its
// record says, because that signal is exact.
const StaleAfter = 6 * time.Hour

// stillRunning says whether an open record is recent enough to mean live work.
func stillRunning(e LedgerEntry, now time.Time) bool {
	if !e.Deadline.IsZero() {
		// A job carries the deadline it was given, so a 12-hour fleet is live
		// for twelve hours and a 30-second job is not live for six.
		return now.Before(e.Deadline.Add(5 * time.Minute))
	}
	if e.At.IsZero() {
		return false
	}
	return now.Sub(e.At) < StaleAfter
}

// openJobIDs is every job the ledger says was started, not finished, and not yet
// stale.
func (r *Runner) openJobIDs() (map[string]bool, error) {
	open, err := r.ledger.Open()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	ids := map[string]bool{}
	for _, e := range open {
		if e.Kind == "job" && e.ID != "" && stillRunning(e, now) {
			ids[e.ID] = true
		}
	}
	return ids, nil
}

// openHostDirs is every host directory the ledger says is still in use.
//
// ⚠ A LEDGER THAT CANNOT BE READ RETURNS AN EMPTY SET, and an empty set
// means nothing is spared for this reason. That is the safe direction only
// because the age rule still applies: a fresh transfer inside --older-than is
// kept whatever the ledger says, and a caller who passed no age limit asked for
// everything.
func (r *Runner) openHostDirs() map[string]bool {
	open, err := r.ledger.Open()
	if err != nil {
		r.log("the ledger could not be read, so a transfer in flight may not be seen as live: " + err.Error())
		return map[string]bool{}
	}
	now := time.Now()
	dirs := map[string]bool{}
	for _, e := range open {
		if e.HostDir != "" && stillRunning(e, now) {
			dirs[e.HostDir] = true
		}
	}
	return dirs
}

// jobIDFromContainer recovers the job id a container name carries.
func jobIDFromContainer(name string) string { return strings.TrimPrefix(name, "wtk-") }

// jobIDFromPath recovers the job id a guest directory is named for.
func jobIDFromPath(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// containerNameOf strips the id a plan renders beside a container's name.
func containerNameOf(rendered string) string {
	if i := strings.Index(rendered, " ("); i >= 0 {
		return rendered[:i]
	}
	return rendered
}

// applyGuestCleanup removes exactly the named containers and directories.
//
// ⛔ IT IS GIVEN A LIST AND DOES NOT ENUMERATE. The script this replaced globbed
// the guest again at the moment of removal, so whatever the plan had decided was
// irrelevant by the time anything was deleted.
func (r *Runner) applyGuestCleanup(ctx context.Context, plan *CleanupPlan, root string, remove []CleanupTarget, pruneImages bool) error {
	var b strings.Builder
	if pruneImages {
		b.WriteString("podman image prune -a -f >/dev/null 2>&1 || :\n")
	}
	b.WriteString("root=" + shellQuote(root) + "\n")
	b.WriteString("removed=0\n")
	for _, t := range Of(remove, "container") {
		q := shellQuote(containerNameOf(t.Name))
		b.WriteString("if podman rm -f " + q + " >/dev/null 2>&1; then printf 'removed container %s\\n' " + q +
			"; else printf 'FAILED container %s\\n' " + q + "; fi\n")
	}
	for _, t := range Of(remove, "guest-dir") {
		b.WriteString("d=" + shellQuote(t.Name) + "\n")
		// ⭐ The containment test is made by the guest, over the exact path the
		// guest is about to act on. It is defence in depth rather than the only
		// guard: Select chose this path out of an enumeration this tool made.
		b.WriteString(`case "$d" in "$root"/jobs/*|"$root"/staging/*) : ;; *) printf 'FAILED refused %s\n' "$d"; d= ;; esac` + "\n")
		b.WriteString(`case "$d" in *..*) printf 'FAILED refused %s\n' "$d"; d= ;; esac` + "\n")
		b.WriteString(`if [ -n "$d" ]; then` + "\n")
		b.WriteString(`  rm -rf "$d" 2>/dev/null || :` + "\n")
		// A job that ran with --user had its mounts re-owned into this account's
		// subuid range, and only the user namespace can unlink what is inside.
		b.WriteString(`  if [ -e "$d" ]; then podman unshare rm -rf "$d" >/dev/null 2>&1 || :; fi` + "\n")
		b.WriteString(`  if [ -e "$d" ]; then printf 'FAILED directory %s\n' "$d"; else printf 'removed directory %s\n' "$d"; removed=$((removed+1)); fi` + "\n")
		b.WriteString("fi\n")
	}
	b.WriteString(`printf 'cleanup-complete %s\n' "$removed"` + "\n")

	out, stderr, code, err := r.baseCapture(ctx, []byte(b.String()), 20*time.Minute)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "removed "):
			plan.Removed = append(plan.Removed, strings.TrimPrefix(line, "removed "))
		case strings.HasPrefix(line, "FAILED "):
			plan.Failed = append(plan.Failed, strings.TrimPrefix(line, "FAILED "))
		}
	}
	if err != nil || code != 0 {
		return fmt.Errorf("cleanup in the guest exited %d: %s", code, firstLine(stderr+out))
	}
	if !strings.Contains(out, "cleanup-complete") {
		return fmt.Errorf("cleanup exited 0 without reaching its last line")
	}
	return nil
}

// closeStaleRecords closes a record whose resource is gone.
//
// ⛔ WITHOUT THIS the ledger keeps an open record for something nothing can
// find, `resources` says a run was interrupted forever, and the liveness signal
// cleanup now depends on stops meaning anything.
//
// ⭐ It asks the guest what is still there rather than assuming this run removed
// it, so a record left open by an earlier cleanup, or by a directory somebody
// removed by hand, is closed by the same sweep.
func (r *Runner) closeStaleRecords(ctx context.Context, plan *CleanupPlan) {
	after, _, _, err := r.guestJobs(ctx)
	if err != nil {
		plan.Failed = append(plan.Failed, "could not re-read the guest, so no record was closed: "+err.Error())
		return
	}
	remaining := map[string]bool{}
	for _, j := range after {
		remaining[j.Path] = true
	}
	open, err := r.ledger.Open()
	if err != nil {
		plan.Failed = append(plan.Failed, "the ledger could not be read: "+err.Error())
		return
	}
	for _, e := range open {
		switch {
		case e.GuestDir != "" && !remaining[e.GuestDir]:
		case e.HostDir != "" && !dirExists(e.HostDir):
			// ⚠ A HELPER'S UPLOAD AND ARTIFACT RECORDS NAME A HOST PATH, not a
			// guest one, so a sweep that only looked at GuestDir left every one
			// of them open. An open record is what cleanup reads as live work,
			// so those directories would have been spared forever.
		default:
			continue
		}
		if err := r.ledger.Append(LedgerEntry{Event: "close", Kind: e.Kind, ID: e.ID, Note: "closed by gc: its directory is gone"}); err != nil {
			plan.Failed = append(plan.Failed, "could not close the record for "+e.ID+": "+err.Error())
			continue
		}
		plan.Removed = append(plan.Removed, "record "+e.Kind+"/"+e.ID)
	}
}

// hostStaging is what the helper is holding on this machine for its clients.
//
// ⭐ It reads the DIRECTORIES rather than the in-memory map, so a helper that
// was killed between an upload and its job still reports what it left behind.
func (r *Runner) hostStaging() []HostStage {
	var out []HostStage
	for _, kind := range []string{"uploads", "artifacts", "incoming"} {
		root := filepath.Join(r.home, kind)
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(root, e.Name())
			st := HostStage{Kind: strings.TrimSuffix(kind, "s"), Path: p}
			if info, err := e.Info(); err == nil {
				st.ModTime = info.ModTime()
			}
			if size, ok, err := dirSize(p); err == nil {
				st.Bytes, st.Known = size, ok
			}
			out = append(out, st)
		}
	}
	return out
}

// dirExists answers without distinguishing why not: a path this process cannot
// stat is one it cannot clean up either, and both mean the record can close.
func dirExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
