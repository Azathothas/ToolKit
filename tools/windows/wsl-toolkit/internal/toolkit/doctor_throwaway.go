// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"time"
)

// ThrowawayFacts is what `doctor` answers about throwaway distributions before
// any is created: the failures that otherwise arrive halfway into an import.
//
// ⛔ EVERY FIELD IS READ, AND ONE THAT COULD NOT BE READ IS NULL WITH A REASON.
// A stopped engine arrives as a pull error, NAT networking as a guest that
// cannot reach a fixture on 127.0.0.1, and a full volume as a partial disk; each
// is knowable in seconds, and none is a number this report may invent.
type ThrowawayFacts struct {
	Dir              string       `json:"dir"`
	FreeBytes        *int64       `json:"free_bytes"`
	ImportFloorBytes int64        `json:"import_floor_bytes"`
	Owned            *int         `json:"owned"`
	Leftovers        *int         `json:"leftovers"`
	LeftoverBytes    *int64       `json:"leftover_bytes"`
	Snapshots        *int         `json:"snapshots"`
	SnapshotBytes    *int64       `json:"snapshot_bytes"`
	ListError        string       `json:"list_error,omitempty"`
	Network          *HostAddress `json:"network,omitempty"`
	NetworkError     string       `json:"network_error,omitempty"`
	Engine           string       `json:"engine,omitempty"`
	Platform         string       `json:"platform,omitempty"`
	EngineError      string       `json:"engine_error,omitempty"`
	// ClockResolutionNS is the smallest step this host's wall clock was seen to
	// take while sampling. ⚠ %9f in a timestamp format pads below it.
	ClockResolutionNS *int64 `json:"clock_resolution_ns"`
}

// ReadThrowawayFacts asks, and changes nothing. Under fast the host engine is not
// asked, because that question can take a minute on a stopped podman machine.
func ReadThrowawayFacts(ctx context.Context, fast bool) ThrowawayFacts {
	f := ThrowawayFacts{ImportFloorBytes: ThrowawaySpaceFloor}
	home, err := Home()
	if err != nil {
		f.ListError = err.Error()
		return f
	}
	f.Dir = ThrowawayDir(home)
	if free, ok, err := FreeSpace(f.Dir); err == nil && ok {
		f.FreeBytes = &free
	}
	if t, err := NewThrowaways(nil); err != nil {
		f.ListError = err.Error()
	} else if rep, err := t.List(ctx); err != nil {
		f.ListError = err.Error()
	} else {
		owned, leftovers, snapshots := len(rep.Owned), len(rep.Leftovers), len(rep.Snapshots)
		var lb, sb int64
		for _, l := range rep.Leftovers {
			lb += l.Bytes
		}
		for _, s := range rep.Snapshots {
			sb += s.Bytes
		}
		f.Owned, f.Leftovers, f.Snapshots, f.LeftoverBytes, f.SnapshotBytes = &owned, &leftovers, &snapshots, &lb, &sb
	}
	if ans, err := ResolveHostAddress(); err != nil {
		f.NetworkError = err.Error()
	} else {
		f.Network = &ans
	}
	if fast {
		f.EngineError = "not asked under --fast"
	} else if e, err := FindEngine(ctx); err != nil {
		f.EngineError = err.Error()
	} else {
		f.Engine, f.Platform = e.Name+" at "+e.Path, e.Platform()
	}
	f.ClockResolutionNS = sampleClockResolution(40 * time.Millisecond)
	return f
}

// sampleClockResolution reports the smallest nonzero step between wall-clock
// readings over a window, or nil when fewer than two distinct readings arrived.
func sampleClockResolution(window time.Duration) *int64 {
	start := time.Now()
	last := start.UnixNano()
	var best int64
	for spins := 0; time.Since(start) < window && spins < 2_000_000; spins++ {
		now := time.Now().UnixNano()
		if step := now - last; step > 0 && (best == 0 || step < best) {
			best = step
		}
		last = now
	}
	if best == 0 {
		return nil
	}
	return &best
}
