// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"fmt"
	"sort"
	"time"
)

// ⛔ WHY THIS FILE EXISTS. Cleanup built its report from one list and acted on
// another. The report came from an unfiltered enumeration; the action came from
// a shell loop that force-removed EVERY labelled container and only then looked
// at an age. So `gc --apply --older-than 24h`, which reads as "remove what has
// been abandoned for a day", killed a job that had been running for seconds: the
// container was gone, the job exited 137 with its work half done, and cleanup
// reported a success. A dry run that lists more than the apply removes is a dry
// run nobody can act on.

// CleanupPolicy is what the caller asked for.
type CleanupPolicy struct {
	// OlderThan spares anything touched more recently. Zero means no age rule.
	OlderThan time.Duration
	// IncludeLive removes work that is still running. ⛔ OFF BY DEFAULT and
	// there is no way to reach it by accident: killing another agent's build is
	// something a caller says out loud.
	IncludeLive bool
	// Job narrows everything to one job id. Empty means the whole store.
	//
	// ⛔ IT NARROWS AND IT DOES NOT WIDEN. A job id that is still running is
	// spared exactly as it would be without this, because `--include-live` is
	// the only way past that and naming one job is not a way of saying it.
	// WSL-52, and WSL-36 is the ruling it obeys.
	Job string
}

// ForJob narrows a policy to one job.
func (p CleanupPolicy) ForJob(id string) CleanupPolicy { p.Job = id; return p }

// CleanupTarget is one thing cleanup could remove, carrying the two facts that
// decide whether it may.
type CleanupTarget struct {
	Kind string `json:"kind"` // container, guest-dir or host-dir
	Name string `json:"name"` // a container name, or a path
	// JobID ties a container to its directory. Both are named for the same job,
	// so one age and one liveness answer covers the pair.
	JobID string `json:"job_id,omitempty"`
	// Live means something is using it right now: a container the engine reports
	// as running, or a directory belonging to one.
	Live bool `json:"live,omitempty"`
	// ModTime is when it was last touched. ⚠ A ZERO VALUE MEANS UNKNOWN, not
	// old. A container whose job directory has already been removed has no age
	// to read, which is a different thing from having an age of zero.
	ModTime time.Time `json:"mod_time,omitempty"`
}

// Spared is one target cleanup did not touch, and why.
type Spared struct {
	Target CleanupTarget `json:"target"`
	Reason string        `json:"reason"`
}

// Select splits the targets into what this policy removes and what it spares.
//
// ⭐ IT IS THE ONLY PLACE THE DECISION IS MADE, and it touches nothing. The plan
// prints what it returns and the apply consumes exactly the same slice, so the
// two cannot describe different sets. That was the defect: they were computed
// separately and only one of them applied the age.
func (p CleanupPolicy) Select(targets []CleanupTarget, now time.Time) (remove []CleanupTarget, spared []Spared) {
	for _, t := range targets {
		switch {
		case p.Job != "" && t.JobID != p.Job:
			// ⚠ NOT "spared", and the difference matters to a reader: a target
			// belonging to another job was never a candidate, and listing it as
			// something this run declined to remove would make one job's cleanup
			// report look like a refusal to clean the rest.
			continue
		case t.Live && !p.IncludeLive:
			spared = append(spared, Spared{Target: t, Reason: "in use right now. Pass --include-live to remove it anyway"})
		case p.OlderThan > 0 && !t.ModTime.IsZero() && now.Sub(t.ModTime) < p.OlderThan:
			age := now.Sub(t.ModTime).Round(time.Second)
			spared = append(spared, Spared{Target: t, Reason: fmt.Sprintf("last touched %s ago, which is inside the %s window", age, p.OlderThan)})
		default:
			// ⚠ An unknown age with nothing using it is REMOVED. That is a
			// container whose job directory is already gone and which the engine
			// does not report as running, which is what abandoned means. Sparing
			// it would mean the documented `--older-than 24h` cleanup could never
			// collect the one thing it exists for.
			remove = append(remove, t)
		}
	}
	sort.SliceStable(remove, func(i, j int) bool { return cleanupOrder(remove[i]) < cleanupOrder(remove[j]) })
	return remove, spared
}

// cleanupOrder puts containers first. ⛔ A directory a container still has open
// cannot be removed, and the error names the directory rather than the container
// holding it.
func cleanupOrder(t CleanupTarget) int {
	switch t.Kind {
	case "container":
		return 0
	case "guest-dir":
		return 1
	default:
		return 2
	}
}

// Names is the plan's rendering of a set of targets.
func Names(targets []CleanupTarget) []string {
	if len(targets) == 0 {
		return nil
	}
	out := make([]string, 0, len(targets))
	for _, t := range targets {
		out = append(out, t.Name)
	}
	return out
}

// Of returns the targets of one kind, in order.
func Of(targets []CleanupTarget, kind string) []CleanupTarget {
	var out []CleanupTarget
	for _, t := range targets {
		if t.Kind == kind {
			out = append(out, t)
		}
	}
	return out
}
