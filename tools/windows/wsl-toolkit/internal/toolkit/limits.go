// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// A job's memory and processor limits, and whether its container was held to
// them. WSL-136.
//
// ⛔ AN ENGINE THAT ACCEPTS A LIMIT HAS NOT ENFORCED IT. WSL-60 measured a base
// that took `--memory 64m`, recorded it, and let the container allocate 300 MB.
// So the answer is read INSIDE the container, from its own cgroup, before the
// payload starts, and is never concluded from the flag having been passed.

// ResourceLimits is what a job asks its container to be held to. Zero means
// none.
type ResourceLimits struct {
	MemoryBytes int64
	CPUs        float64
}

// Any says whether a limit was asked for at all.
func (l ResourceLimits) Any() bool { return l.MemoryBytes > 0 || l.CPUs > 0 }

// LimitsReport is a job's answer about its limits.
type LimitsReport struct {
	MemoryBytes int64   `json:"memory_bytes,omitempty"`
	CPUs        float64 `json:"cpus,omitempty"`
	// Enforced is yes where the container's own cgroup carried every limit asked
	// for, no where it carried none of one, and unknown where the container did
	// not report, or had no cgroup v2 tree to read.
	Enforced string `json:"enforced"`
}

// The three answers LimitsReport.Enforced gives.
const (
	LimitsEnforced   = "yes"
	LimitsUnenforced = "no"
	LimitsUnknown    = "unknown"
)

// MinMemoryBytes is the smallest memory limit podman accepts.
const MinMemoryBytes = 6 << 20

// memorySize is the grammar podman itself reads for --memory: a number, then an
// optional unit in binary multiples.
var memorySize = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)([kKmMgGtT]?)(?:[iI]?[bB])?$`)

// ParseResourceLimits reads --memory and --cpus, either of which may be empty.
//
// ⭐ THE SIZE IS PASSED TO podman AS BYTES, so this grammar and podman's cannot
// disagree about what was asked for.
func ParseResourceLimits(memory, cpus string) (ResourceLimits, error) {
	return parseLimits("--memory", memory, "--cpus", cpus)
}

// parseLimits reads a memory size and a processor count, each named in a
// refusal as the caller spelled it: a flag, or a configuration key.
func parseLimits(memoryName, memory, cpusName, cpus string) (ResourceLimits, error) {
	var l ResourceLimits
	if s := strings.TrimSpace(memory); s != "" {
		n, err := parseMemorySize(memoryName, s)
		if err != nil {
			return l, err
		}
		l.MemoryBytes = n
	}
	if s := strings.TrimSpace(cpus); s != "" {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return l, fmt.Errorf("%s %q is not a number of processors. Pass one such as 1 or 1.5", cpusName, cpus)
		}
		// ⚠ podman turns the count into a quota of 1000 microseconds a period or
		// more, so a hundredth is the smallest count it can carry.
		if f < 0.01 {
			return l, fmt.Errorf("%s %s is less than 0.01, which podman cannot carry. Pass 0.01 or more", cpusName, s)
		}
		l.CPUs = f
	}
	return l, nil
}

// parseMemorySize reads a size such as 512m, 2g, 1.5GiB or a count of bytes.
func parseMemorySize(name, s string) (int64, error) {
	m := memorySize.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("%s %q is not a size. Pass a number and an optional unit, such as 512m or 2g", name, s)
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, fmt.Errorf("%s %q is not a size: %w", name, s, err)
	}
	// ⚠ NO UNIT IS BYTES. strings.Index of an empty unit is 0, which read a
	// bare count as kibibytes until a case caught it.
	shift := 0
	if m[2] != "" {
		shift = strings.Index("kmgt", strings.ToLower(m[2])) + 1
	}
	bytes := n * math.Pow(1024, float64(shift))
	if bytes > float64(math.MaxInt64/2) {
		return 0, fmt.Errorf("%s %q is larger than any machine this runs on", name, s)
	}
	if bytes < MinMemoryBytes {
		return 0, fmt.Errorf("%s %s is less than %s, the smallest limit podman accepts", name, s, HumanBytes(MinMemoryBytes))
	}
	return int64(bytes), nil
}

// FormatCPUs spells a processor count as podman reads it, and as nothing where
// there is none.
func FormatCPUs(f float64) string {
	if f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// limitsArgs are the engine flags a job's limits become.
func limitsArgs(l ResourceLimits) []string {
	var args []string
	if l.MemoryBytes > 0 {
		args = append(args, "--memory", strconv.FormatInt(l.MemoryBytes, 10))
	}
	if l.CPUs > 0 {
		args = append(args, "--cpus", FormatCPUs(l.CPUs))
	}
	return args
}

// limitsCheck is the wrapper's half: it reads the container's own cgroup before
// the payload runs, and writes one of three marker lines on stderr. Empty where
// no limit was asked for, so such a job's wrapper is what it always was.
//
// ⛔ A LIMIT FILE THAT IS ABSENT IS A MEASURED NO, NOT AN UNKNOWN. A container
// in a cgroup that carries no limit has no memory.max, which is what WSL-60's
// base showed; unknown is kept for a container with no cgroup v2 tree at all.
func limitsCheck(l ResourceLimits, token string) string {
	if !l.Any() {
		return ""
	}
	var b strings.Builder
	b.WriteString("v=unknown\nif [ -f /sys/fs/cgroup/cgroup.controllers ]; then\n  v=yes\n")
	if l.MemoryBytes > 0 {
		b.WriteString("  m=; if [ -r /sys/fs/cgroup/memory.max ]; then read -r m < /sys/fs/cgroup/memory.max; fi\n")
		b.WriteString("  case \"$m\" in ''|max) v=no ;; esac\n")
	}
	if l.CPUs > 0 {
		b.WriteString("  c=; if [ -r /sys/fs/cgroup/cpu.max ]; then read -r c _ < /sys/fs/cgroup/cpu.max; fi\n")
		b.WriteString("  case \"$c\" in ''|max) v=no ;; esac\n")
	}
	b.WriteString("fi\n")
	b.WriteString("printf '\\n%s\\n' \"" + token + "-limits-$v\" >&2\n")
	return b.String()
}

// limitsMarkers strips the wrapper's limits line from the stream and remembers
// which of the three it was. It is three exact tokens, so a payload cannot make
// one by writing a line that merely looks like it.
type limitsMarkers struct {
	yes, no, unknown *markerStripper
}

// newLimitsMarkers puts the three strippers in front of dst and returns the
// writer to send the stream through.
func newLimitsMarkers(dst io.Writer, token string) (io.Writer, *limitsMarkers) {
	unknown := newMarkerStripper(dst, token+"-limits-"+LimitsUnknown)
	no := newMarkerStripper(unknown, token+"-limits-"+LimitsUnenforced)
	yes := newMarkerStripper(no, token+"-limits-"+LimitsEnforced)
	return yes, &limitsMarkers{yes: yes, no: no, unknown: unknown}
}

// Flush releases what each stripper held back, in the order the stream passes
// through them.
func (l *limitsMarkers) Flush() error {
	return errors.Join(l.yes.Flush(), l.no.Flush(), l.unknown.Flush())
}

// Verdict is the line the container wrote, or unknown where it wrote none.
func (l *limitsMarkers) Verdict() string {
	switch {
	case l.yes.Seen():
		return LimitsEnforced
	case l.no.Seen():
		return LimitsUnenforced
	}
	return LimitsUnknown
}
