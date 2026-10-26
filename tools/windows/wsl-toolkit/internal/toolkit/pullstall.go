// pullstall.go - how long a pull may be silent, sized from what it pulls.
//
// ⛔ WHAT WAS WRONG. A pull was given up after four minutes with no output, and
// the comment beside the limit said a working pull printed a line every few
// seconds. It does not. podman on Windows is a remote client with no terminal,
// and it prints a line as a layer starts and another as it ends, with nothing
// between. So the stall limit was a minimum bandwidth: the default base's one
// 193.6 MB layer failed below about 6.5 Mbit/s, and the refusal said it was not
// a slow link. Every retry started from zero. WSL-176.
//
// ⭐ THE LIMIT IS THE TIME THE LARGEST LAYER TAKES AT A FLOOR RATE, read from
// the image's manifest before the pull, and never under four minutes. The
// export keeps four minutes, because it writes the archive as it moves.
// WSL_TOOLKIT_PULL_STALL names a pull's limit outright, for a slower link.
//
// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// PullFloorRate is the slowest link a pull's limits are sized for, in bytes a
// second. ⚠ About 2 Mbit/s, a quarter of the slowest link the old limit let the
// default base through, so a working link is never the one given up.
const PullFloorRate = 256 << 10

// PullStallEnv names a pull's stall limit outright, as a duration such as 45m.
const PullStallEnv = "WSL_TOOLKIT_PULL_STALL"

// pullStallFloor is the shortest a pull's stall limit is sized to, and a case's
// stand-in for it.
var pullStallFloor = EngineStallTimeout

// pullLimit is how long one pull may be silent, how long it may run, and the
// sentence that says where the first came from.
type pullLimit struct {
	Stall time.Duration
	Total time.Duration
	Why   string
	// Sized says the stall came from the image's own layers.
	Sized bool
}

// pullLimits answers one pull's limits: the variable where it is set, else the
// largest layer's time at PullFloorRate, else the floor.
//
// ⛔ A LAYER LIST THAT CANNOT BE READ IS NO REFUSAL. The pull still runs, with
// the four minutes it always had, and the line says why and names the variable.
func (e *Engine) pullLimits(ctx context.Context, ref string) (pullLimit, error) {
	if v := strings.TrimSpace(os.Getenv(PullStallEnv)); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return pullLimit{}, fmt.Errorf("%s=%q is not a positive duration such as 45m, so nothing was pulled", PullStallEnv, v)
		}
		return pullLimit{Stall: d, Total: max(EngineTransferTimeout, d), Why: "as " + PullStallEnv + " says"}, nil
	}
	largest, total, err := layerBytes(ctx, e.readManifest, ref, e.Arch)
	if err != nil {
		return pullLimit{
			Stall: pullStallFloor, Total: EngineTransferTimeout,
			Why: fmt.Sprintf("its layers could not be read, %v. On a link slower than %s/s a layer can take longer, so set %s",
				err, HumanBytes(PullFloorRate), PullStallEnv),
		}, nil
	}
	return sizedPullLimit(largest, total), nil
}

// sizedPullLimit is the rule pullLimits applies to a layer list, with no
// process in it.
func sizedPullLimit(largest, total int64) pullLimit {
	rate := HumanBytes(PullFloorRate) + "/s"
	lim := pullLimit{Stall: atFloorRate(largest), Sized: true}
	lim.Why = fmt.Sprintf("the time its largest layer, %s, takes at %s", HumanBytes(largest), rate)
	if lim.Stall < pullStallFloor {
		lim.Stall = pullStallFloor
		lim.Why = fmt.Sprintf("its largest layer, %s, takes less at %s", HumanBytes(largest), rate)
	}
	// ⚠ THE CEILING GROWS WITH THE STALL, or a long stall could never fire
	// before the thirty minutes every transfer has.
	lim.Total = max(EngineTransferTimeout, atFloorRate(total), lim.Stall)
	return lim
}

// atFloorRate is how long n bytes take at PullFloorRate, rounded up to a second.
func atFloorRate(n int64) time.Duration {
	if n <= 0 {
		return 0
	}
	return time.Duration((n+PullFloorRate-1)/PullFloorRate) * time.Second
}

// readManifest asks the engine for the manifest a reference names, from the
// registry where the engine holds no list by that name.
func (e *Engine) readManifest(ctx context.Context, ref string) ([]byte, error) {
	if e.Name == "wslc" {
		return nil, errors.New("wslc has no manifest inspect")
	}
	out, stderr, err := (engineCall{Timeout: time.Minute, What: e.Name + " manifest inspect " + ref}).
		run(ctx, e.Path, "manifest", "inspect", ref)
	if err != nil {
		return nil, fmt.Errorf("%s manifest inspect %s: %w: %s", e.Name, ref, err, firstLine(stderr))
	}
	return []byte(out), nil
}

// imageManifest is the part of an image manifest or an image index this reads.
type imageManifest struct {
	Layers []struct {
		Size int64 `json:"size"`
	} `json:"layers"`
	Manifests []struct {
		Digest   string `json:"digest"`
		Platform *struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
		} `json:"platform"`
	} `json:"manifests"`
}

// layerBytes answers the largest layer and the sum of every layer of the image
// ref names for linux/arch, reading at most two manifests: an index, then the
// image it names for this platform.
//
// ⚠ AN ENGINE THAT ANSWERS AN INDEX FOR THE IMAGE ITSELF NAMES NO LAYER, and a
// third read would answer the same, so that is an error and not a loop.
func layerBytes(ctx context.Context, read func(context.Context, string) ([]byte, error), ref, arch string) (int64, int64, error) {
	at := ref
	for range 2 {
		raw, err := read(ctx, at)
		if err != nil {
			return 0, 0, err
		}
		var doc imageManifest
		if err := json.Unmarshal(bytes.TrimSpace(raw), &doc); err != nil {
			return 0, 0, fmt.Errorf("the manifest of %s does not parse: %w", at, err)
		}
		if len(doc.Layers) > 0 {
			var largest, total int64
			for _, l := range doc.Layers {
				largest = max(largest, l.Size)
				total += l.Size
			}
			if largest <= 0 {
				return 0, 0, fmt.Errorf("the manifest of %s names no layer's size", at)
			}
			return largest, total, nil
		}
		digest := ""
		for _, m := range doc.Manifests {
			if m.Platform != nil && strings.EqualFold(m.Platform.OS, "linux") && strings.EqualFold(m.Platform.Architecture, arch) && m.Digest != "" {
				digest = m.Digest
				break
			}
		}
		if digest == "" {
			return 0, 0, fmt.Errorf("the manifest of %s names no layer and no linux/%s image", at, arch)
		}
		at = imageRepository(ref) + "@" + digest
	}
	return 0, 0, fmt.Errorf("the engine answered an index for %s itself, which names no layer", at)
}

// imageRepository is a reference with its tag and digest taken off.
//
// ⚠ A COLON BEFORE THE LAST SLASH IS A REGISTRY'S PORT, not a tag.
func imageRepository(ref string) string {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		ref = ref[:colon]
	}
	return ref
}

// pullStallHint is where to look when a pull stalls, for the engine that pulled.
func pullStallHint(engine string, lim pullLimit) string {
	sized := ""
	if lim.Sized {
		sized = fmt.Sprintf(" This limit was %s.", lim.Why)
	}
	return fmt.Sprintf("%s prints nothing while one layer downloads, so a link slower than %s/s reads as silence too.%s "+
		"On a slow link, set %s to a longer span, such as 45m, and retry. Otherwise the registry or the network stopped: "+
		"podman system connection list", engine, HumanBytes(PullFloorRate), sized, PullStallEnv)
}
