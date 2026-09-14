// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// EventLogSchema is the record a relayed command writes, one JSON object per
// line. Its field names are the ones the stream log has always written, so a log
// recorded before this executable carried the relay still replays and compares.
const EventLogSchema = "wsl-toolkit-event/1"

// StreamFlushAfter is how long an unterminated line may wait before it is shown
// early, marked as unterminated. It is longer than a progress meter's redraw
// interval, so an ordinary carriage-return meter is never split by it.
const StreamFlushAfter = 2 * time.Second

// relayPoll is how often the relay looks for a line to flush or a silence to
// report. Nothing measured is finer than it needs to be.
const relayPoll = 250 * time.Millisecond

// Event is one record. A field that could not be measured is absent, never zero.
type Event struct {
	Schema          string   `json:"schema"`
	Seq             int64    `json:"seq"`
	TRel            float64  `json:"t_rel"`
	TWall           string   `json:"t_wall"`
	Kind            string   `json:"kind"`
	Prov            string   `json:"prov"`
	Distro          string   `json:"distro"`
	Stream          string   `json:"stream,omitempty"`
	Text            *string  `json:"text,omitempty"`
	Partial         *bool    `json:"partial,omitempty"`
	ExitCode        *int     `json:"exit_code,omitempty"`
	TimedOut        *bool    `json:"timed_out,omitempty"`
	SilenceS        *float64 `json:"silence_s,omitempty"`
	DistroState     string   `json:"distro_state,omitempty"`
	OutLines        *int     `json:"out_lines,omitempty"`
	ErrLines        *int     `json:"err_lines,omitempty"`
	DiskBytes       *int64   `json:"disk_bytes,omitempty"`
	DiskGrewBytes   *int64   `json:"disk_grew_bytes,omitempty"`
	ProgressPercent *float64 `json:"progress_percent,omitempty"`
	ProgressLabel   string   `json:"progress_label,omitempty"`
	ProgressAgeS    *float64 `json:"progress_age_s,omitempty"`
}

// TickFacts is what the host can read about a distribution while its command is
// quiet: WSL's word for it, and the size of its disk.
//
// ⚠ GROWTH IS EVIDENCE AND FLATNESS IS NOT. The disk file grows in large steps
// and has been measured unchanged six seconds after a guest wrote 120 MiB, so a
// disk that grew means something allocated and one that did not rules nothing out.
type TickFacts struct {
	State     string
	DiskBytes *int64
}

// RunOutcome is how the relayed command ended, as the relay records it.
type RunOutcome struct {
	Exit      int
	TimedOut  bool
	Cancelled bool
	Timeout   time.Duration
	// StartError is set when the command could not be started at all, which is a
	// different fact from a command that exited nonzero.
	StartError string
}

type relayStream struct {
	tag     string
	pending []byte
	since   time.Duration
	lines   int
	bytes   int64
}

type progressReading struct {
	percent float64
	label   string
	at      time.Duration
}

// RunLog relays one command's two streams through the settings a caller chose.
//
// ⛔ ONE PATH TO EVERY SINK. The live streams, the text copy and the event record
// are all written by emit, after redaction and truncation, so no sink can be
// reached by a line that skipped either.
type RunLog struct {
	mu          sync.Mutex
	s           LogSettings
	distro      string
	liveOut     io.Writer
	liveErr     io.Writer
	text        *os.File
	events      *os.File
	created     []string
	createdDirs []string
	facts       func() TickFacts
	now         func() time.Time
	start       time.Time
	begun       bool
	seq         int64
	out, err    relayStream
	lastStamp   time.Duration
	lastLine    time.Duration
	lastTick    time.Duration
	quiet       bool
	fired       map[time.Duration]bool
	lastDisk    *int64
	progress    *progressReading
	// stdoutErr, stderrErr, textErr and eventErr are the first failure writing
	// each place a line goes.
	//
	// ⛔ KEPT APART. A caller that stopped reading this process's output has not
	// made the event log unwritable, and the record of a run matters most when
	// nobody was watching it live: folded into one error, the first closed pipe
	// stopped the event log before its EXIT record.
	stdoutErr error
	stderrErr error
	textErr   error
	eventErr  error
	closed    bool
	stop      chan struct{}
	done      chan struct{}
	// manual leaves check to its caller rather than a timer, so a case can drive
	// the flush and the heartbeat against a clock it controls.
	manual bool
}

// OpenRunLog opens every sink before anything is created, so an unwritable log
// path is refused while nothing has happened yet.
//
// ⚠ NOTHING IS TRUNCATED HERE. A replaced text log is truncated when the command
// starts, so a run that fails before its command leaves the previous log intact,
// and a file this call created is removed again if the command never starts.
func OpenRunLog(s LogSettings, liveOut, liveErr io.Writer) (*RunLog, error) {
	r := &RunLog{s: s, liveOut: liveOut, liveErr: liveErr, now: time.Now, fired: map[time.Duration]bool{}}
	r.out.tag, r.err.tag = "out", "err"
	if r.liveOut == nil {
		r.liveOut = io.Discard
	}
	if r.liveErr == nil {
		r.liveErr = io.Discard
	}
	var err error
	if s.TextPath != "" {
		if r.text, err = r.openSink(s.TextPath); err != nil {
			r.Abort()
			return nil, fmt.Errorf("--stream-log: %w", err)
		}
	}
	if s.EventPath != "" {
		if r.events, err = r.openSink(s.EventPath); err != nil {
			r.Abort()
			return nil, fmt.Errorf("--event-log: %w", err)
		}
	}
	return r, nil
}

func (r *RunLog) openSink(path string) (*os.File, error) {
	if err := r.ensureSinkDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	if _, statErr := os.Lstat(path); errors.Is(statErr, os.ErrNotExist) {
		r.created = append(r.created, path)
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

// ensureSinkDir makes only the missing directories and records only the ones
// this call made, so an aborted command can remove its own empty scaffolding.
func (r *RunLog) ensureSinkDir(dir string) error {
	var missing []string
	for {
		info, err := os.Stat(dir)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", dir)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		missing = append(missing, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			return fmt.Errorf("no existing parent directory contains %s", dir)
		}
		dir = parent
	}
	for i := len(missing) - 1; i >= 0; i-- {
		path := missing[i]
		if err := os.Mkdir(path, 0o755); err != nil {
			if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
				continue
			}
			return err
		}
		r.createdDirs = append(r.createdDirs, path)
	}
	return nil
}

// Begin starts the clock for one command in one distribution.
func (r *RunLog) Begin(distro string, facts func() TickFacts) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.begun || r.closed {
		return
	}
	r.begun, r.distro, r.facts, r.start = true, distro, facts, r.now()
	r.created = nil
	r.createdDirs = nil
	if r.text != nil && r.s.TextOverwrite {
		if err := r.text.Truncate(0); err != nil && r.textErr == nil {
			r.textErr = err
		}
	}
	if !r.manual && r.s.Active() {
		r.stop, r.done = make(chan struct{}), make(chan struct{})
		go r.poll(r.stop, r.done)
	}
}

// Abort closes a log whose command never started, and removes a file it created.
func (r *RunLog) Abort() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.begun {
		return
	}
	r.closed = true
	for _, f := range []*os.File{r.text, r.events} {
		if f != nil {
			_ = f.Close()
		}
	}
	for _, p := range r.created {
		_ = os.Remove(p)
	}
	for i := len(r.createdDirs) - 1; i >= 0; i-- {
		_ = os.Remove(r.createdDirs[i])
	}
}

// Stdout and Stderr are the writers a command's two streams are copied into.
func (r *RunLog) Stdout() io.Writer { return relayWriter{r, "out"} }
func (r *RunLog) Stderr() io.Writer { return relayWriter{r, "err"} }

type relayWriter struct {
	r   *RunLog
	tag string
}

func (w relayWriter) Write(p []byte) (int, error) {
	w.r.receive(w.tag, p)
	return len(p), nil
}

func (r *RunLog) elapsed() time.Duration { return r.now().Sub(r.start) }

func (r *RunLog) stream(tag string) *relayStream {
	if tag == "err" {
		return &r.err
	}
	return &r.out
}

func (r *RunLog) receive(tag string, p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.stream(tag)
	live := r.liveOut
	if tag == "err" {
		live = r.liveErr
	}
	if r.closed {
		// The relay has finished, so the bytes are forwarded and nothing records them.
		_, _ = live.Write(p)
		return
	}
	now := r.elapsed()
	st.bytes += int64(len(p))
	if r.quiet {
		r.emit("note", "obs", "output resumed after "+FormatSpan(now-r.lastLine)+" of silence", false, now)
		r.quiet = false
		r.fired = map[time.Duration]bool{}
	}
	if !r.s.Renders() {
		r.writeLive(tag, p)
	}
	had := len(st.pending) > 0
	buf := append(st.pending, p...)
	start, emitted := 0, false
	for i := 0; i < len(buf); i++ {
		switch buf[i] {
		case '\n':
			r.line(st, string(buf[start:i]), false, now)
			start, emitted = i+1, true
		case '\r':
			if i+1 >= len(buf) {
				// ⚠ HELD, NEVER EMITTED: it may be the first half of a CRLF split
				// across two reads. The next read, or the flush, resolves it.
				i = len(buf)
				continue
			}
			if buf[i+1] == '\n' {
				r.line(st, string(buf[start:i]), false, now)
				i++
			} else {
				// A carriage return ends a line that is being redrawn, which is
				// what makes a progress meter visible at all.
				r.line(st, string(buf[start:i]), true, now)
			}
			start, emitted = i+1, true
		}
	}
	st.pending = append([]byte(nil), buf[start:]...)
	for len(st.pending) > MaxLineBytesLimit {
		cut := utf8Cut(st.pending, MaxLineBytesLimit)
		r.line(st, string(st.pending[:cut]), true, now)
		st.pending = append([]byte(nil), st.pending[cut:]...)
		emitted = true
	}
	if len(st.pending) > 0 && (!had || emitted) {
		st.since = now
	}
	r.lastLine, r.lastTick = now, now
}

func utf8Cut(b []byte, max int) int {
	cut := max
	for cut > 0 && !utf8.RuneStart(b[cut]) {
		cut--
	}
	if cut == 0 {
		return max
	}
	return cut
}

// line is one line a command wrote: consumed as progress, or emitted.
func (r *RunLog) line(st *relayStream, text string, partial bool, now time.Duration) {
	// ⛔ A PARTIAL LINE IS NEVER A PROGRESS REPORT: its label may still be arriving.
	if !partial {
		if pct, label, ok := ParseProgress(r.s.ProgressPrefix, text); ok {
			r.progress = &progressReading{percent: pct, label: label, at: now}
			stream := "stdout"
			if st.tag == "err" {
				stream = "stderr"
			}
			r.record(Event{Kind: "PROGRESS", Prov: "obs", Stream: stream, ProgressPercent: &pct, ProgressLabel: label}, now)
			return
		}
	}
	st.lines++
	r.emit(st.tag, "obs", text, partial, now)
}

// emit renders one line and writes it to every sink.
//
// ⛔ A TICK AND A NOTE DO NOT ADVANCE THE DELTA CLOCK. Delta is the time since
// the previous line of output, and a heartbeat is the absence of one: with ticks
// advancing it, a five-second gap rendered as +0.619 against a tick written to the
// other stream.
func (r *RunLog) emit(tag, prov, text string, partial bool, now time.Duration) {
	body := LimitLineBytes(r.s.Redact(text), r.s.MaxLineBytes)
	delta := now - r.lastStamp
	if tag == "out" || tag == "err" {
		r.lastStamp = now
	}
	wall := r.start.Add(now)
	prefix := renderPrefix(r.s, tag, partial, wall, now, delta)
	plain := joinLine(prefix, body, r.s.PrefixOnly)
	watcher := tag == "tick" || tag == "note"
	if watcher || r.s.Renders() {
		shown := plain
		if r.s.Color && prefix != "" {
			shown = joinLine(colorPrefix(prefix, tag), body, r.s.PrefixOnly)
		}
		r.writeLive(tag, []byte(shown+"\n"))
	}
	if r.text != nil && r.textErr == nil {
		if _, err := fmt.Fprintln(r.text, plain); err != nil {
			r.textErr = err
		}
	}
	kind, stream := "LOG", "stdout"
	switch tag {
	case "err":
		stream = "stderr"
	case "tick":
		kind, stream = "TICK", "watcher"
	case "note":
		kind, stream = "NOTE", "watcher"
	}
	t, p := body, partial
	r.record(Event{Kind: kind, Prov: prov, Stream: stream, Text: &t, Partial: &p}, now)
}

// writeLive writes to this process's stdout for the command's stdout, and to its
// stderr for everything else. A stream that failed once is not written again, so
// a closed pipe is one error rather than one per line.
func (r *RunLog) writeLive(tag string, p []byte) {
	w, failed := r.liveErr, &r.stderrErr
	if tag == "out" {
		w, failed = r.liveOut, &r.stdoutErr
	}
	if *failed != nil {
		return
	}
	if _, err := w.Write(p); err != nil {
		*failed = err
	}
}

// relayError names every place a line could not be written, or nil.
func (r *RunLog) relayError() error {
	var failed []string
	for _, f := range []struct {
		where string
		err   error
	}{{"this process's stdout", r.stdoutErr}, {"this process's stderr", r.stderrErr}, {"--stream-log", r.textErr}, {"--event-log", r.eventErr}} {
		if f.err != nil {
			failed = append(failed, f.where+": "+f.err.Error())
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return errors.New(strings.Join(failed, "; "))
}

// pendingText is the text of a line that was held and never ended.
//
// ⚠ A CARRIAGE RETURN AT ITS END WAS HELD ONLY IN CASE A NEWLINE FOLLOWED IT,
// and none did. It ended a redrawn line, so it is not part of what the line says.
func pendingText(p []byte) string { return strings.TrimSuffix(string(p), "\r") }

func joinLine(prefix, body string, prefixOnly bool) string {
	switch {
	case prefix == "":
		return body
	case prefixOnly:
		return prefix
	}
	return prefix + " " + body
}

// renderPrefix is the columns, the separator and a fixed four-character tag.
//
// ⭐ THE TAG IS A FIXED FIELD, which is what an awk or a grep over a long log
// depends on. A trailing ~ inside those four says the line had not ended when it
// was written: a carriage return redrew it, or it waited past the flush bound.
func renderPrefix(s LogSettings, tag string, partial bool, wall time.Time, elapsed, delta time.Duration) string {
	if len(s.Columns) == 0 {
		return ""
	}
	cols := make([]string, 0, len(s.Columns))
	for _, c := range s.Columns {
		v, err := renderColumn(c, s.formatFor(c), wall, elapsed, delta)
		if err != nil {
			v = "?"
		}
		cols = append(cols, v)
	}
	field := fmt.Sprintf("%-4s", tag)
	if partial {
		field = fmt.Sprintf("%-3s~", tag)
	}
	return strings.Join(cols, " ") + s.Separator + field
}

func colorPrefix(prefix, tag string) string {
	const esc = "\x1b["
	color := ""
	switch tag {
	case "err":
		color = esc + "31m"
	case "tick", "note":
		color = esc + "36m"
	}
	cut := len(prefix) - 4
	return esc + "90m" + prefix[:cut] + esc + "0m" + color + prefix[cut:] + esc + "0m"
}

func (r *RunLog) record(e Event, now time.Duration) {
	if r.events == nil || r.eventErr != nil {
		return
	}
	r.seq++
	e.Schema, e.Seq, e.Distro = EventLogSchema, r.seq, r.distro
	e.TRel = math.Round(now.Seconds()*1000) / 1000
	e.TWall = r.start.Add(now).Format(time.RFC3339Nano)
	data, err := json.Marshal(e)
	if err == nil {
		_, err = r.events.Write(append(data, '\n'))
	}
	if err != nil {
		r.eventErr = err
	}
}

// poll flushes and ticks until stop is closed.
//
// ⛔ THE CHANNELS ARE ITS ARGUMENTS AND ARE NEVER READ FROM THE STRUCT AGAIN. It
// used to select on r.stop on every pass, and Finish clears that field before it
// closes the channel it took. A pass still inside check, asking wsl.exe about the
// distribution, came back to a select on a nil channel, never saw the close, and
// Finish waited for it forever over a command that had already ended. WSL-80.
func (r *RunLog) poll(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	t := time.NewTicker(relayPoll)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			r.check()
		}
	}
}

// check flushes a line that has waited too long and reports a silence.
//
// ⚠ THE FACTS ARE READ WITH THE LOCK RELEASED. They ask wsl.exe, which can take
// seconds, and a relay that held its lock through that would stall the command's
// own output behind its heartbeat.
func (r *RunLog) check() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	now := r.elapsed()
	for _, st := range []*relayStream{&r.out, &r.err} {
		if len(st.pending) > 0 && now-st.since >= StreamFlushAfter {
			// ⭐ A PROMPT WAITING ON INPUT IS EXACTLY THIS SHAPE, and it is the
			// one case where showing nothing means waiting forever.
			r.line(st, pendingText(st.pending), true, now)
			st.pending = nil
			r.lastLine, r.lastTick = now, now
		}
	}
	due := r.tickDue(now)
	facts := r.facts
	r.mu.Unlock()
	if !due {
		return
	}
	f := TickFacts{State: "unknown"}
	if facts != nil {
		f = facts()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if now = r.elapsed(); r.closed || !r.tickDue(now) {
		return
	}
	r.tick(now, f)
}

func (r *RunLog) tickDue(now time.Duration) bool {
	return r.s.Tick > 0 && now-r.lastLine >= r.s.Tick && now-r.lastTick >= r.s.Tick
}

// tick is the heartbeat: how long it has been quiet, what has come out, what WSL
// says about the distribution, and what its disk did since the last tick.
//
// ⛔ NOTHING IS INJECTED INTO THE GUEST to produce it, so an image with no shell
// utilities ticks as well as a full userland. It fires on silence rather than on a
// timer, because a heartbeat that beats through output is one people filter out.
func (r *RunLog) tick(now time.Duration, f TickFacts) {
	silent := now - r.lastLine
	var grew *int64
	diskText := "disk unreadable"
	if f.DiskBytes != nil {
		diskText = "disk " + HumanBytes(*f.DiskBytes)
		if r.lastDisk != nil {
			g := *f.DiskBytes - *r.lastDisk
			grew = &g
			if g > 0 {
				diskText += " (+" + HumanBytes(g) + " since the last tick)"
			} else {
				diskText += " (unchanged)"
			}
		}
		d := *f.DiskBytes
		r.lastDisk = &d
	}
	text := fmt.Sprintf("%s silent | elapsed %s | out %d lines %s | err %d lines %s | distro %s | %s",
		FormatSpan(silent), FormatSpan(now), r.out.lines, HumanBytes(r.out.bytes), r.err.lines, HumanBytes(r.err.bytes), f.State, diskText)
	var pct, age *float64
	label := ""
	if r.progress != nil {
		p, a := r.progress.percent, (now - r.progress.at).Seconds()
		pct, age, label = &p, &a, r.progress.label
		shown := "progress " + FormatPercent(p)
		if label != "" {
			shown += " " + label
		}
		// ⛔ THE LAST REPORT AND ITS AGE, NEVER AN ESTIMATE. A remaining time
		// drawn from one sample is a number nobody measured.
		text += " | " + shown + " (" + FormatSpan(now-r.progress.at) + " ago)"
	}
	r.emit("tick", "obs", text, false, now)
	s, ol, el := math.Round(silent.Seconds()*10)/10, r.out.lines, r.err.lines
	r.record(Event{Kind: "TICK_FACTS", Prov: "obs", SilenceS: &s, DistroState: f.State, OutLines: &ol, ErrLines: &el,
		DiskBytes: f.DiskBytes, DiskGrewBytes: grew, ProgressPercent: pct, ProgressLabel: label, ProgressAgeS: roundTenth(age)}, now)
	r.escalate(now, silent, grew, f.State)
	r.lastTick, r.quiet = now, true
}

func roundTenth(v *float64) *float64 {
	if v == nil {
		return nil
	}
	x := math.Round(*v*10) / 10
	return &x
}

// escalate says more at each silence threshold, once per silence.
//
// ⛔ NOTHING HERE IS STATED AS A FACT ABOUT THE COMMAND. The strongest line is what
// the readings are consistent with, marked as an inference and carrying the
// readings it was drawn from: proving a hang needs the program's intent, and a
// watcher that says "hung" will one day say it about a working build.
func (r *RunLog) escalate(now, silent time.Duration, grew *int64, state string) {
	for _, step := range r.s.Escalate {
		if silent < step || r.fired[step] {
			continue
		}
		r.fired[step] = true
		if len(r.fired) == 1 {
			r.emit("note", "obs", "the command writes into a pipe rather than a terminal, so a program that block-buffers "+
				"off a terminal holds its lines and they arrive late, carrying the time they were received. stdbuf -oL in "+
				"the command asks for line buffering where the image has it", false, now)
		}
		verdict, because := "nothing can be concluded from what is measurable here", ""
		switch {
		case state == "stopped" || state == "not registered":
			verdict = "the distribution is not running, so this is not a quiet command"
			because = "WSL reports the distribution as " + state
		case grew == nil:
			because = "the distribution's disk could not be read twice, so there is no second signal"
		case *grew > 0:
			verdict = "something inside allocated while it was quiet. Consistent with a download, an unpack or a build that reports nothing"
			because = "the disk grew " + HumanBytes(*grew) + " between the last two ticks"
		default:
			verdict = "NOTHING is ruled out. A lock, a network call inside a long connect timeout, a computation that writes no files, " +
				"and a guest writing hard whose disk has not been extended yet all read exactly like this"
			because = "the disk did not grow between the last two ticks, and that reading is coarse: it moves in large steps"
		}
		if state == "unknown" {
			because = "WSL could not be asked about the distribution; " + because
		}
		r.emit("note", "inf", "after "+FormatSpan(silent)+" of silence: "+verdict+". because: "+because, false, now)
		if len(r.fired) >= 2 {
			r.emit("note", "obs", "to bound a run like this, pass --timeout: the distribution is terminated and the answer is 124. "+
				"To see what is registered right now, from another shell: wsl-toolkit distro list", false, now)
		}
	}
}

// Finish flushes what is pending, records how the command ended, and closes
// every sink. It returns the first sink error, so a log that could not be written
// is reported rather than silently short.
func (r *RunLog) Finish(o RunOutcome, facts func() TickFacts) error {
	r.mu.Lock()
	begun, stop, done := r.begun, r.stop, r.done
	r.stop, r.done = nil, nil
	r.mu.Unlock()
	if !begun {
		// A command that never started has no run to record.
		r.Abort()
		return nil
	}
	if stop != nil {
		close(stop)
		<-done
	}
	// Read before the lock, and only when a diagnosis needs the distribution's state.
	f := TickFacts{State: "unknown"}
	if o.Exit != 0 && !o.TimedOut && !o.Cancelled && o.StartError == "" && facts != nil {
		f = facts()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.relayError()
	}
	now := r.elapsed()
	for _, st := range []*relayStream{&r.out, &r.err} {
		if len(st.pending) > 0 {
			r.line(st, pendingText(st.pending), true, now)
			st.pending = nil
		}
	}
	switch {
	case o.StartError != "":
		r.emit("note", "obs", "the command could not be started: "+o.StartError, false, now)
	case o.TimedOut:
		r.emit("tick", "obs", fmt.Sprintf("TIMED OUT after %s: --timeout %s was reached, so %s was terminated. The answer is 124",
			FormatSpan(now), o.Timeout, r.distro), false, now)
	case o.Cancelled:
		r.emit("note", "obs", "the run was cancelled, so "+r.distro+" was terminated. The answer is 130", false, now)
	default:
		if why := DiagnoseExit(o.Exit, f.State); why != "" {
			r.emit("note", "inf", why, false, now)
		}
	}
	code, timedOut := o.Exit, o.TimedOut
	r.record(Event{Kind: "EXIT", Prov: "obs", ExitCode: &code, TimedOut: &timedOut}, now)
	r.closed = true
	if r.text != nil {
		if err := r.text.Close(); err != nil && r.textErr == nil {
			r.textErr = err
		}
	}
	if r.events != nil {
		if err := r.events.Close(); err != nil && r.eventErr == nil {
			r.eventErr = err
		}
	}
	return r.relayError()
}

// EventRun is one recorded run. An appended event log holds one per command.
type EventRun struct {
	Index  int
	Events []Event
}

// ReadEventRuns reads an event log and splits it into its runs.
//
// ⛔ A GAP IN seq IS REFUSED, NEVER SMOOTHED OVER. The field is gapless by
// construction, so a gap means records were dropped and the file is not the
// whole run. ⭐ A seq OF 1 STARTS A RUN, because --event-log appends and every
// command numbers from one: a file holding three commands is three runs, and
// reading them as one would measure a run that never happened.
func ReadEventRuns(path string) ([]EventRun, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var runs []EventRun
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			return nil, fmt.Errorf("%s line %d is not an event record: %w", path, lineNo, err)
		}
		if e.Schema != EventLogSchema {
			return nil, fmt.Errorf("%s line %d declares schema %q, and this build reads %q", path, lineNo, e.Schema, EventLogSchema)
		}
		if e.Seq == 1 {
			runs = append(runs, EventRun{Index: len(runs) + 1})
		} else if len(runs) == 0 {
			return nil, fmt.Errorf("%s line %d has seq %d before any seq 1, so the start of the run is missing", path, lineNo, e.Seq)
		} else if prev := runs[len(runs)-1].Events; e.Seq != prev[len(prev)-1].Seq+1 {
			return nil, fmt.Errorf("%s line %d: seq jumps from %d to %d. The field is gapless, so records were dropped and nothing is rendered",
				path, lineNo, prev[len(prev)-1].Seq, e.Seq)
		}
		runs[len(runs)-1].Events = append(runs[len(runs)-1].Events, e)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("%s holds no records", path)
	}
	return runs, nil
}

// SelectRun picks one run: a 1-based index, or 0 for the last.
func SelectRun(runs []EventRun, index int) (EventRun, error) {
	if index == 0 {
		return runs[len(runs)-1], nil
	}
	if index < 0 || index > len(runs) {
		return EventRun{}, fmt.Errorf("run %d does not exist: the log holds %d run(s)", index, len(runs))
	}
	return runs[index-1], nil
}

// Replay renders recorded runs through the same renderer a live relay uses.
//
// ⚠ THE RECORD'S OWN WALL READING, never this machine's clock: a replay of last
// week's run stamped with today's date says something false about when it
// happened. It runs nothing and creates nothing.
func Replay(runs []EventRun, s LogSettings, out, errw io.Writer) (int, error) {
	rendered := 0
	for _, run := range runs {
		if len(runs) > 1 {
			first := run.Events[0]
			if _, err := fmt.Fprintf(errw, "==> run %d of %d: %s, started %s\n", run.Index, len(runs), first.Distro, first.TWall); err != nil {
				return rendered, err
			}
		}
		var lastLog time.Duration
		for _, e := range run.Events {
			if e.Text == nil || (e.Kind != "LOG" && e.Kind != "TICK" && e.Kind != "NOTE") {
				continue
			}
			tag := "out"
			switch {
			case e.Kind == "TICK":
				tag = "tick"
			case e.Kind == "NOTE":
				tag = "note"
			case e.Stream == "stderr":
				tag = "err"
			}
			at := time.Duration(e.TRel * float64(time.Second))
			wall, err := time.Parse(time.RFC3339Nano, e.TWall)
			if err != nil && needsWall(s.Columns) {
				return rendered, fmt.Errorf("record %d of run %d has no readable t_wall, so a wall column cannot be rendered from it", e.Seq, run.Index)
			}
			delta := at - lastLog
			if tag == "out" || tag == "err" {
				lastLog = at
			}
			partial := e.Partial != nil && *e.Partial
			body := LimitLineBytes(s.Redact(*e.Text), s.MaxLineBytes)
			prefix := renderPrefix(s, tag, partial, wall, at, delta)
			shown := joinLine(prefix, body, s.PrefixOnly)
			if s.Color && prefix != "" {
				shown = joinLine(colorPrefix(prefix, tag), body, s.PrefixOnly)
			}
			w := errw
			if tag == "out" {
				w = out
			}
			if _, err := fmt.Fprintln(w, shown); err != nil {
				return rendered, err
			}
			rendered++
		}
	}
	return rendered, nil
}

func needsWall(cols []string) bool {
	for _, c := range cols {
		if c == "wall" || c == "iso" || c == "epoch" {
			return true
		}
	}
	return false
}

// EventSummary is one run's measured figures.
type EventSummary struct {
	Schema             string   `json:"schema"`
	Path               string   `json:"path"`
	Run                int      `json:"run"`
	Runs               int      `json:"runs"`
	Distro             string   `json:"distro,omitempty"`
	Started            string   `json:"started,omitempty"`
	Records            int      `json:"records"`
	Duration           float64  `json:"duration_seconds"`
	FirstOutput        *float64 `json:"first_output_seconds"`
	LongestSilence     float64  `json:"longest_silence_seconds"`
	LongestSilenceEnds float64  `json:"longest_silence_ends_seconds"`
	StdoutLines        int      `json:"stdout_lines"`
	StdoutBytes        int64    `json:"stdout_bytes"`
	StderrLines        int      `json:"stderr_lines"`
	StderrBytes        int64    `json:"stderr_bytes"`
	ExitCode           *int     `json:"exit_code"`
	TimedOut           *bool    `json:"timed_out"`
}

// EventSummarySchema versions EventSummary.
const EventSummarySchema = "wsl-toolkit-event-summary/1"

// SummarizeRun derives the figures a comparison is made of, once, so a replay and
// a comparison cannot disagree about them.
//
// ⭐ THE LONGEST SILENCE IS THE FIGURE THAT EARNS THIS, and the silence after the
// last line counts: a run whose last line arrived at four seconds and which ended
// at four minutes was quiet for the rest.
func SummarizeRun(path string, run EventRun, runs int) EventSummary {
	s := EventSummary{Schema: EventSummarySchema, Path: path, Run: run.Index, Runs: runs, Records: len(run.Events)}
	if len(run.Events) > 0 {
		s.Distro, s.Started = run.Events[0].Distro, run.Events[0].TWall
	}
	last := 0.0
	for _, e := range run.Events {
		if e.TRel > s.Duration {
			s.Duration = e.TRel
		}
		switch e.Kind {
		case "LOG":
			if s.FirstOutput == nil {
				v := e.TRel
				s.FirstOutput = &v
			}
			if gap := e.TRel - last; gap > s.LongestSilence {
				s.LongestSilence, s.LongestSilenceEnds = gap, e.TRel
			}
			last = e.TRel
			n := 0
			if e.Text != nil {
				n = len(*e.Text)
			}
			if e.Stream == "stderr" {
				s.StderrLines++
				s.StderrBytes += int64(n)
			} else {
				s.StdoutLines++
				s.StdoutBytes += int64(n)
			}
		case "EXIT":
			s.ExitCode, s.TimedOut = e.ExitCode, e.TimedOut
		}
	}
	if tail := s.Duration - last; tail > s.LongestSilence {
		s.LongestSilence, s.LongestSilenceEnds = tail, s.Duration
	}
	return s
}
