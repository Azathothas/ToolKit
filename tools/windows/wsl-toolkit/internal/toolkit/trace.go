// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// ⛔ WHY A TRACE EXISTS AT ALL. A job with a 2s deadline returned to its caller
// after 15.2 seconds and reported that it had taken 3.9. WSL-45 named four
// candidates for the missing eleven seconds - the deadline firing late, wsl.exe
// not returning, the stream relay outliving the killed process, or the
// container cleanup - and NONE of them looked different from the others from
// outside. A number nobody measured is worse than a blank, so this is the
// instrument that turns the guess into a reading.
//
// ⚠ It is not a profiler and must not become one. It records when named phases
// of one job ended, and that is all.

// jobTrace records when each phase of a job finished.
//
// ⛔ THE ZERO VALUE IS USABLE AND SILENT. Every method tolerates a nil receiver,
// so a caller that does not want a trace passes nil rather than every call site
// growing a condition.
type jobTrace struct {
	mu      sync.Mutex
	started time.Time
	marks   []traceMark
}

type traceMark struct {
	name string
	at   time.Time
}

func newJobTrace() *jobTrace { return &jobTrace{started: time.Now()} }

// Mark records that a named phase has just ended.
func (t *jobTrace) Mark(name string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.marks = append(t.marks, traceMark{name: name, at: time.Now()})
}

// Render is the intervals BETWEEN the marks, which is the question being asked.
// Absolute instants would make a reader do the subtraction, and the subtraction
// is the whole answer.
func (t *jobTrace) Render() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.marks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(t.marks))
	prev := t.started
	for _, m := range t.marks {
		parts = append(parts, fmt.Sprintf("%s %s", m.name, m.at.Sub(prev).Round(time.Millisecond)))
		prev = m.at
	}
	parts = append(parts, "total "+prev.Sub(t.started).Round(time.Millisecond).String())
	return strings.Join(parts, ", ")
}

// TraceEnabled says whether a caller asked for the phase timings on every job.
//
// ⚠ The trace is written WITHOUT this whenever a job passes its deadline,
// because that is the case it was built for and a reader who has to reproduce
// the failure with a flag set has already lost the run they wanted to look at.
func TraceEnabled() bool {
	return strings.TrimSpace(os.Getenv("WSL_TOOLKIT_TRACE")) != ""
}
