// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// HeldSession is one detached base session as `resources` reports it. WSL-95.
type HeldSession struct {
	ID       string `json:"id"`
	Distro   string `json:"distro"`
	User     string `json:"user"`
	GuestDir string `json:"guest_dir"`
	// State is running, ended, gone or unknown. Unknown carries its reason.
	State   string `json:"state"`
	Exit    *int   `json:"exit,omitempty"`
	Unknown string `json:"unknown,omitempty"`
}

// sessionSurvey is one recorded session and what the guest says about it.
type sessionSurvey struct {
	rec SessionRecord
	st  SessionState
	// unknown is why the state could not be read, or "".
	unknown string
	// id is the directory name where the record itself does not read.
	id string
}

// surveySessions reads every detached base session this state directory
// records. One guest call answers for every session of one account in one
// distribution, and `gc` and `resources` both read this one survey.
//
// ⚠ A SESSION THE GUEST CANNOT BE ASKED ABOUT IS UNKNOWN, never ended.
// Reading it as ended would remove the files of a payload that may still be
// writing them. A session whose distribution is no longer registered is not
// such a case: its guest half is gone with the distribution.
func (r *Runner) surveySessions(ctx context.Context) []sessionSurvey {
	entries, err := os.ReadDir(filepath.Join(r.home, "jobs"))
	if err != nil {
		return nil
	}
	type group struct{ distro, user string }
	groups := map[group][]SessionRecord{}
	var out []sessionSurvey
	for _, e := range entries {
		rec, ok, err := ReadSessionRecord(r.home, e.Name())
		if err != nil {
			// ⛔ A RECORD THAT DOES NOT READ IS NAMED, never skipped: skipping it
			// would hide a host directory that nothing ever removes.
			if ValidJobID(e.Name()) {
				out = append(out, sessionSurvey{id: e.Name(), unknown: err.Error()})
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
					out = append(out, sessionSurvey{rec: rec, id: rec.ID, unknown: "WSL could not say whether " + k.distro + " is registered: " + err.Error()})
				}
				continue
			}
			registered[k.distro] = reg
		}
		if !reg {
			for _, rec := range recs {
				out = append(out, sessionSurvey{rec: rec, id: rec.ID, st: SessionState{Gone: true}})
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
				out = append(out, sessionSurvey{rec: rec, id: rec.ID, unknown: err.Error()})
				continue
			}
			out = append(out, sessionSurvey{rec: rec, id: rec.ID, st: states[rec.GuestDir]})
		}
	}
	return out
}

// sessionTargets is the survey as cleanup targets.
func (r *Runner) sessionTargets(ctx context.Context) []CleanupTarget {
	var out []CleanupTarget
	for _, s := range r.surveySessions(ctx) {
		out = append(out, s.target())
	}
	return out
}

// heldSessions is the survey as `resources` reports it.
func (r *Runner) heldSessions(ctx context.Context) []HeldSession {
	var out []HeldSession
	for _, s := range r.surveySessions(ctx) {
		out = append(out, s.held())
	}
	return out
}

// target is one session as cleanup sees it. Its age is from its end where the
// guest recorded one, and from its start otherwise.
func (s sessionSurvey) target() CleanupTarget {
	if s.rec.ID == "" {
		return CleanupTarget{Kind: "session", Name: s.id, JobID: s.id, Unknown: s.unknown}
	}
	t := CleanupTarget{Kind: "session", Name: sessionTargetName(s.rec), JobID: s.rec.ID, ModTime: s.rec.Started, Unknown: s.unknown}
	switch {
	case s.unknown != "":
	case s.st.Alive:
		t.Live = true
	case s.st.Ended != "":
		if at, err := time.Parse(time.RFC3339, s.st.Ended); err == nil {
			t.ModTime = at
		}
	}
	return t
}

func (s sessionSurvey) held() HeldSession {
	h := HeldSession{ID: s.id, Distro: s.rec.Distro, User: s.rec.User, GuestDir: s.rec.GuestDir, Unknown: s.unknown}
	switch {
	case s.unknown != "":
		h.State = "unknown"
	case s.st.Gone:
		h.State = "gone"
	case s.st.Alive:
		h.State = "running"
	default:
		h.State = "ended"
		v := s.st.Verdict()
		h.Exit = &v
	}
	return h
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
