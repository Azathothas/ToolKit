// SPDX-License-Identifier: 0BSD

package compat

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// One JSON object per line, one per event. Ported from the script's
// events.ps1.

// eventSink is the writer side of -EventLog.
//
// ⭐ THE RENDERED LOG IS A VIEW OVER THIS. Anything the terminal shows that
// this does not carry would be a renderer that knows something the record
// does not.
//
// ⛔ THE SCHEMA CARRIES A VERSION. A positional or unversioned record that
// changes shape mis-reads silently, and the reader that mis-reads it is a
// program rather than a person.
type eventSink struct {
	w       io.Writer
	seq     int64
	distro  string
	started time.Time
}

func newEventSink(path, distro string) (*eventSink, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &eventSink{w: f, distro: distro, started: now()}, nil
}

// writeRecord writes one event. Fields that are always present are always
// present, and a field that could not be measured is ABSENT rather than zero.
//
// ⭐ `seq` IS MONOTONIC AND GAPLESS. A gap in it means records were dropped,
// which is itself a finding rather than something for a reader to work out.
//
// ⭐ `prov` SAYS HOW THE FACT WAS OBTAINED: obs read from an interface, der
// computed from observations, inf a judgement that can be wrong. ⛔ An
// inference carries the reasoning on the line it appears in.
func (e *eventSink) writeRecord(kind string, rel dur, provenance, stream, text string, partial bool, data map[string]any) {
	if e == nil {
		return
	}
	e.seq++
	// The encoding/json package sorts map keys on write, so field order in
	// the file is the sorter's, not this call's. ⛔ THAT IS FINE, and the
	// reason is worth writing down: the schema version on every record is
	// what a reader keys on, and a positional reader of a versioned,
	// self-describing record is the reader this schema exists to prevent.
	rec := map[string]any{
		"schema": "wsl-toolkit-event/1",
		"seq":    e.seq,
		"t_rel":  roundHalfUp(rel.Seconds(), 3),
		"t_wall": now().Format(time.RFC3339Nano),
		"kind":   kind,
		"prov":   provenance,
		"distro": e.distro,
	}
	if stream != "" {
		rec["stream"] = stream
	}
	if text != "" || partial {
		rec["text"] = text
		rec["partial"] = partial
	}
	for k, v := range data {
		rec[k] = v
	}
	line, err := json.Marshal(rec)
	if err == nil {
		fmt.Fprintf(e.w, "%s\n", line)
	}
}

// Close flushes and closes the file behind the sink.
func (e *eventSink) Close() error {
	if e == nil {
		return nil
	}
	if c, ok := e.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// The json package sorts map keys, so the order list is advisory only; the
// schema version is what a reader actually keys on.

// -- the reader side: Replay and Compare -------------------------------------

// eventRecord is one record of a recorded run, with the shape checked before
// it is trusted.
type eventRecord struct {
	Seq     int64
	TRel    float64
	TWall   string
	Kind    string
	Prov    string
	Distro  string
	Stream  string
	Text    string
	Partial bool
	Data    map[string]any
}

// readEventLogFile reads one recorded run.
//
// ⛔ A GAP IN `seq` IS REPORTED, NEVER SMOOTHED OVER. The field is documented
// as monotonic and gapless, so a gap means records were dropped. That is a
// finding about the RECORDING rather than about the run, and a reader handed
// a quietly-renumbered log would draw conclusions about a run they were not
// shown. It is a refusal here and it names the two sequence numbers.
//
// ⛔ THE SCHEMA IS CHECKED, NOT ASSUMED.
func readEventLogFile(path string) ([]eventRecord, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("No event log at '%s'. -EventLog writes one; this reads it back.", path)
	}
	var records []eventRecord
	prev := int64(-1)
	for n, line := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("Line %d of '%s' is not JSON: %v", n+1, path, err)
		}
		schema, _ := raw["schema"].(string)
		if schema != "wsl-toolkit-event/1" {
			saw := schema
			if _, exists := raw["schema"]; !exists {
				saw = "(none)"
			}
			return nil, fmt.Errorf("Line %d of '%s' declares schema '%s' and this build reads 'wsl-toolkit-event/1'.", n+1, path, saw)
		}
		rec := eventRecord{
			Kind:    stringField(raw, "kind"),
			TWall:   stringField(raw, "t_wall"),
			Prov:    stringField(raw, "prov"),
			Distro:  stringField(raw, "distro"),
			Stream:  stringField(raw, "stream"),
			Text:    stringField(raw, "text"),
			Partial: boolField(raw, "partial"),
			Data:    raw,
		}
		rec.TRel = floatField(raw, "t_rel")
		if v, ok := raw["seq"].(float64); ok {
			rec.Seq = int64(v)
		}
		if prev >= 0 && rec.Seq != prev+1 {
			return nil, fmt.Errorf("Line %d of '%s': seq jumps from %d to %d. That field is "+
				"gapless by construction, so records were dropped and this log is not the "+
				"whole run. Nothing was rendered.", n+1, path, prev, rec.Seq)
		}
		prev = rec.Seq
		records = append(records, rec)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("'%s' holds no records.", path)
	}
	return records, nil
}

func stringField(raw map[string]any, name string) string {
	if v, ok := raw[name].(string); ok {
		return v
	}
	return ""
}

func boolField(raw map[string]any, name string) bool {
	v, ok := raw[name]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func floatField(raw map[string]any, name string) float64 {
	v, ok := raw[name].(float64)
	if !ok {
		return 0
	}
	return v
}

// runSummary is the figures a comparison is made of, derived once so Replay
// and Compare cannot disagree about them.
//
// ⭐ THE LONGEST SILENCE IS THE FIGURE THAT EARNS THIS. A run whose result
// stayed green while its longest gap grew fifteen times has a regression no
// exit code reports, and nothing else in the record surfaces it.
type runSummary struct {
	Records      int
	Duration     float64
	OutLines     int64
	OutBytes     int64
	ErrLines     int64
	ErrBytes     int64
	FirstOutput  *float64
	LongestGap   float64
	LongestGapAt float64
	ExitCode     *int
	TimedOut     bool
}

// eventLogSummary derives the figures from the records.
//
// ⚠ THE KINDS AND STREAM NAMES ARE THE WRITER'S, read from it rather than
// imagined. writeRecord writes kind LOG with stream stdout, stderr or
// watcher.
func eventLogSummary(records []eventRecord) runSummary {
	var s runSummary
	s.Records = len(records)
	var firstOutput *float64
	longest, longestAt := 0.0, 0.0
	lastOutput := 0.0
	for _, r := range records {
		t := r.TRel
		if t > s.Duration {
			s.Duration = t
		}
		switch r.Kind {
		case "LOG":
			if r.Stream == "stderr" {
				s.ErrLines++
				s.ErrBytes += int64(len(r.Text))
			} else {
				s.OutLines++
				s.OutBytes += int64(len(r.Text))
			}
			tCopy := t
			if firstOutput == nil {
				firstOutput = &tCopy
			}
			if gap := t - lastOutput; gap > longest {
				longest, longestAt = gap, t
			}
			lastOutput = t
		case "EXIT":
			if v, ok := r.Data["exit_code"].(float64); ok {
				code := int(v)
				s.ExitCode = &code
			}
			if v, ok := r.Data["timed_out"].(bool); ok {
				s.TimedOut = v
			}
		}
	}
	// ⚠ THE TAIL COUNTS. A run whose last line arrived at four seconds and
	// which ended at four minutes was silent for the rest, and a summary that
	// measured only the gaps BETWEEN lines would report the quietest part of
	// the run as not having happened at all.
	if tail := s.Duration - lastOutput; tail > longest {
		longest, longestAt = tail, s.Duration
	}
	s.FirstOutput = firstOutput
	s.LongestGap = longest
	s.LongestGapAt = longestAt
	return s
}

// formatRunSummary is the one-line reading of a recorded run, shared by
// Replay and Compare.
func formatRunSummary(s runSummary) string {
	first := "no output"
	if s.FirstOutput != nil {
		first = formatDuration(durFromSeconds(*s.FirstOutput)) + " to first output"
	}
	code := "no exit recorded"
	if s.ExitCode != nil {
		code = "exit " + strconv.Itoa(*s.ExitCode)
	}
	return "elapsed " + formatDuration(durFromSeconds(s.Duration)) +
		" | " + first +
		" | longest silence " + formatDuration(durFromSeconds(s.LongestGap)) +
		" | out " + strconv.FormatInt(s.OutLines, 10) + " lines " + formatByteCount(s.OutBytes) +
		" | err " + strconv.FormatInt(s.ErrLines, 10) + " lines " + formatByteCount(s.ErrBytes) +
		" | " + code
}

func durFromSeconds(v float64) dur {
	return dur(int64(v * float64(time.Second)))
}

// wallNow converts this machine's clock into a wallReading.
func wallNow() wallReading {
	return wallFromTime(now())
}

func wallFromTime(t time.Time) wallReading {
	t = t.Local()
	zone, _ := t.Zone()
	_, offset := t.Zone()
	return wallReading{
		Year:         t.Year(),
		Month:        int(t.Month()),
		Day:          t.Day(),
		Hour:         t.Hour(),
		Minute:       t.Minute(),
		Second:       t.Second(),
		Ticks:        int64(t.Nanosecond()) / 100,
		Offset:       offset,
		Zone:         zone,
		EpochSeconds: t.Unix(),
	}
}

// parseWallReading parses the record's own wall reading. An unparseable one
// is nil, and the renderer then has no reading to reuse.
func parseWallReading(text string) *wallReading {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil
	}
	w := wallFromTime(t)
	return &w
}
