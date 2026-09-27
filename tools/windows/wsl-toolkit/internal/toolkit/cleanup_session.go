// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// sessionTargets surveys every detached base session this state directory
// records. One guest call answers for every session of one account in one
// distribution. WSL-95.
//
// ⚠ A SESSION THE GUEST CANNOT BE ASKED ABOUT IS KEPT, and the plan says why.
// Reading it as ended would remove the files of a payload that may still be
// writing them. A session whose distribution is no longer registered is not
// such a case: its guest half is gone with the distribution.
func (r *Runner) sessionTargets(ctx context.Context) []CleanupTarget {
	entries, err := os.ReadDir(filepath.Join(r.home, "jobs"))
	if err != nil {
		return nil
	}
	type group struct{ distro, user string }
	groups := map[group][]SessionRecord{}
	var out []CleanupTarget
	for _, e := range entries {
		rec, ok, err := ReadSessionRecord(r.home, e.Name())
		if err != nil {
			// ⛔ A RECORD THAT DOES NOT READ IS NAMED, never skipped: skipping it
			// would hide a host directory that nothing ever removes.
			if ValidJobID(e.Name()) {
				out = append(out, CleanupTarget{Kind: "session", Name: e.Name(), JobID: e.Name(), Unknown: err.Error()})
			}
			continue
		}
		if ok {
			k := group{rec.Distro, rec.User}
			groups[k] = append(groups[k], rec)
		}
	}
	keys := make([]group, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].distro != keys[j].distro {
			return keys[i].distro < keys[j].distro
		}
		return keys[i].user < keys[j].user
	})
	registered := map[string]bool{}
	for _, k := range keys {
		recs := groups[k]
		reg, known := registered[k.distro]
		if !known {
			var err error
			if reg, err = r.wsl.Exists(ctx, k.distro); err != nil {
				for _, rec := range recs {
					out = append(out, sessionTarget(rec, SessionState{}, "WSL could not say whether "+k.distro+" is registered: "+err.Error()))
				}
				continue
			}
			registered[k.distro] = reg
		}
		if !reg {
			for _, rec := range recs {
				out = append(out, sessionTarget(rec, SessionState{Gone: true}, ""))
			}
			continue
		}
		dirs := make([]string, len(recs))
		for i, rec := range recs {
			dirs[i] = rec.GuestDir
		}
		states, err := readSessionStates(ctx, r.wsl, k.distro, k.user, dirs)
		for _, rec := range recs {
			if err != nil {
				out = append(out, sessionTarget(rec, SessionState{}, err.Error()))
				continue
			}
			out = append(out, sessionTarget(rec, states[rec.GuestDir], ""))
		}
	}
	return out
}

// sessionTarget is one session as cleanup sees it. Its age is from its end
// where the guest recorded one, and from its start otherwise.
func sessionTarget(rec SessionRecord, st SessionState, unknown string) CleanupTarget {
	t := CleanupTarget{Kind: "session", Name: sessionTargetName(rec), JobID: rec.ID, ModTime: rec.Started, Unknown: unknown}
	switch {
	case unknown != "":
	case st.Alive:
		t.Live = true
	case st.Ended != "":
		if at, err := time.Parse(time.RFC3339, st.Ended); err == nil {
			t.ModTime = at
		}
	}
	return t
}

func sessionTargetName(rec SessionRecord) string {
	return rec.ID + " (" + rec.User + " in " + rec.Distro + ", " + rec.GuestDir + ")"
}

// removeSessions removes each selected session's guest and host directories.
// A session that is still running is reached only through --include-live, and
// it is stopped before its files are removed.
func (r *Runner) removeSessions(ctx context.Context, plan *CleanupPlan, targets []CleanupTarget) {
	for _, t := range targets {
		rec, ok, err := ReadSessionRecord(r.home, t.JobID)
		if err != nil || !ok {
			plan.Failed = append(plan.Failed, "session "+t.JobID+": its record could not be read")
			continue
		}
		if t.Live {
			if _, err := StopSession(ctx, r.wsl, rec, SessionGrace); err != nil {
				plan.Failed = append(plan.Failed, "session "+t.Name+": "+err.Error())
				continue
			}
		}
		if err := RemoveSession(ctx, r.wsl, r.home, rec); err != nil {
			plan.Failed = append(plan.Failed, "session "+t.Name+": "+err.Error())
			continue
		}
		plan.Removed = append(plan.Removed, "session "+t.Name)
	}
}
