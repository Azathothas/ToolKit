// SPDX-License-Identifier: 0BSD

package compat

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The stream-log runtime: sinks, state, the line emitter and the heartbeat.
// Ported from the script's stream-log.ps1, events.ps1 and redact.ps1.

// redaction is one compiled -Redact pattern.
type redaction struct{ re *regexp.Regexp }

// newRedactionSet compiles -Redact into regexes once, ahead of the run.
//
// COMPILING HERE RATHER THAN PER LINE is not an optimisation, it is where a
// bad pattern is REPORTED: a regex that does not compile fails on the first
// line of guest output otherwise, halfway into a run that has already created
// a distro.
func newRedactionSet(patterns []string) ([]*redaction, error) {
	var out []*redaction
	for _, p := range patterns {
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("-Redact '%s' is not a regular expression this host can compile: %v", p, err)
		}
		out = append(out, &redaction{re: re})
	}
	return out, nil
}

// applyRedaction replaces every match with three asterisks, before any sink
// sees the text.
//
// ⭐ IT RUNS ONCE, ON THE WAY IN. The rendered line, the file copy and the JSON
// record are all built from the result, so there is no sink that could be
// reached by a path that skipped this.
func applyRedaction(set []*redaction, text string) string {
	if len(set) == 0 {
		return text
	}
	t := text
	for _, r := range set {
		// ⛔ A REPLACEMENT WITH NO EXPANSIONS, AND THE MARKER ITSELF IS
		// EXPANSION-FREE. '$&' in a replacement string would paste the match
		// back in; three asterisks cannot, and ReplaceAllString is called with
		// a literal built so it stays that way.
		t = r.re.ReplaceAllString(t, literalMarker)
	}
	return t
}

const literalMarker = "***"

// limitLineBytes cuts the DETAIL at -MaxLineBytes and says how much went, or
// returns it unchanged when the bound is off or not reached.
//
// ⛔ IT COUNTS BYTES AND CUTS CHARACTERS. A UTF-8 character is up to four
// bytes, so cutting a byte array at an arbitrary index splits one and
// produces a replacement character that was never in the guest's output. The
// loop adds characters until the next one would cross the bound.
func limitLineBytes(text string, maxBytes int) string {
	if maxBytes <= 0 {
		return text
	}
	total := len(text) // UTF-8 bytes; Go strings are byte-addressed
	if total <= maxBytes {
		return text
	}
	kept := 0
	take := 0
	for _, r := range text {
		w := runeLen(r)
		if kept+w > maxBytes {
			break
		}
		kept += w
		take++
	}
	return string([]rune(text)[:take]) + "...(+" + strconv.Itoa(total-kept) + " bytes cut)"
}

func runeLen(r rune) int {
	switch {
	case r < 0x80:
		return 1
	case r < 0x800:
		return 2
	case r < 0x10000:
		return 3
	default:
		return 4
	}
}

// streamState is everything the log line and the heartbeat need, in one
// object.
//
// ELAPSED IS A MONOTONIC CLOCK AND NOT A DIFFERENCE OF TWO WALL READINGS, so
// a host that steps its clock mid-run cannot produce a relative stamp that
// goes backwards.
type streamState struct {
	distro   string
	settings *logSettings
	started  func() dur

	lastStamp dur
	lastLine  dur
	lastTick  dur
	counts    map[string]*streamCount
	out       io.Writer
	err       io.Writer
	text      io.WriteCloser
	events    *eventSink
	fired     []int
	quiet     bool
	lastDisk  *int64
	progress  *progressReading
}

type streamCount struct {
	Lines int64
	Bytes int64
}

func newStreamState(distro string, settings *logSettings, started func() dur, out, errw io.Writer) *streamState {
	return &streamState{
		distro:   distro,
		settings: settings,
		started:  started,
		counts:   map[string]*streamCount{"out": {}, "err": {}},
		out:      out,
		err:      errw,
	}
}

// assertSinkPathIsUsable refuses a sink path that cannot mean what the caller
// thinks it means.
//
// ⛔ A WINDOWS RESERVED DEVICE NAME IS REFUSED BY NAME. 'nul' is not a file:
// every byte written to it is discarded and every write reports success, so a
// run ends with the caller holding a log they never got. 'con' is the console.
// The set is matched with the EXTENSION STRIPPED and case-insensitively,
// because 'NUL.txt' and 'nul' are the same device.
//
// ⛔ IT TOUCHES NOTHING, which is why it is called from Run, before anything is
// created: a dry run that reports a plan the real run would refuse is worse
// than no dry run.
func assertSinkPathIsUsable(path, parameter string) error {
	if path == "" {
		return nil
	}
	// ⛔ THE SEPARATORS ARE SPLIT HERE, NOT BY THE PATH PACKAGE. That package
	// uses the HOST's separators, so on Linux a backslash is an ordinary
	// character and 'logs\CON.jsonl' has a file name of 'logs\CON', which is
	// not on the list. The rule is about Windows semantics whatever host is
	// asking.
	leaf := leafOfPath(path)
	if dot := strings.IndexByte(leaf, '.'); dot >= 0 {
		leaf = leaf[:dot]
	}
	for _, r := range reservedDeviceNames {
		if strings.EqualFold(leaf, r) {
			return fmt.Errorf("%s '%s' names the Windows reserved device '%s'. Writing to it "+
				"discards everything and reports success, so the run would end with no log and "+
				"no error. Pick a real path.", parameter, path, strings.ToUpper(r))
		}
	}
	return nil
}

var reservedDeviceNames = []string{
	"CON", "PRN", "AUX", "NUL",
	"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
	"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
}

// newTextSink is a UTF-8 file the rendered log is copied into, appended by
// default.
//
// ⛔ NO BYTE ORDER MARK AND NO COLOUR REACHES IT. A log with escape sequences
// in it is a log a later grep answers wrongly about.
func newTextSink(path string, overwrite bool) (io.WriteCloser, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_APPEND
	if overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	return os.OpenFile(path, flags, 0o644)
}

// formatStreamLogPrefix is the columns and the tag, as one string, with
// nothing of the guest's in it.
//
// THE TAG IS A FIXED FOUR-CHARACTER FIELD after the separator, which is the
// property a downstream awk or grep depends on. A trailing '~' inside those
// four says the line had not ended when it was printed.
//
// ⭐ wall CARRIES THE READING THE CALLER HAS. A live run reads the clock; a
// Replay passes the reading the record already carries, and rendering that
// against THIS machine's clock would stamp a run from last week with today's
// date.
func formatStreamLogPrefix(state *streamState, tag string, nowD, delta dur, partial bool, wall *wallReading) (string, error) {
	cfg := state.settings
	reading := wallNow()
	if wall != nil {
		reading = *wall
	}
	parts := make([]string, 0, len(cfg.Columns))
	for _, c := range cfg.Columns {
		text, err := formatStampColumn(c, cfg.Format, reading, nowD, delta)
		if err != nil {
			return "", err
		}
		parts = append(parts, text)
	}
	field := tag
	for len(field) < 3 {
		field += " "
	}
	if partial {
		return strings.Join(parts, " ") + cfg.Separator + field + "~", nil
	}
	if len(field) < 4 {
		field += " "
	}
	return strings.Join(parts, " ") + cfg.Separator + field, nil
}

// progressReading is the last progress the GUEST reported, and when. The age
// is the part that carries the warning: 40 percent reported twelve minutes ago
// is a different picture from 40 percent reported four seconds ago.
type progressReading struct {
	Percent float64
	Label   string
	At      dur
}

// readProgressLine reports whether one relayed line is a progress report, and
// what it says.
//
// ⭐ WHY A STDOUT PREFIX IS THE CHANNEL. The three host-side signals this tool
// measures say whether something is happening and never how much is left,
// because nothing inside the guest can tell the host anything. A prefix needs
// no injection, no mount, no named pipe and no agent in the image.
//
// ⛔ THE TOKEN IS THE CALLER'S AND THERE IS NO DEFAULT. A tool that silently
// swallowed every line beginning with some chosen string would be a tool that
// eats somebody's output.
//
// ⛔ A MALFORMED PREFIXED LINE IS RELAYED, NEVER SWALLOWED. nil here is what
// puts it back in the stream.
//
// The shape is the token, whitespace, a percentage, and an optional label.
func readProgressLine(text, token string) *progressReading {
	if token == "" {
		return nil
	}
	if !strings.HasPrefix(text, token) {
		return nil
	}
	rest := text[len(token):]
	// ⚠ THE TOKEN HAS TO END AT A BOUNDARY. Without this, a token of `P`
	// would consume every line beginning with the letter P.
	if rest != "" && !isSpaceByte(rest[0]) {
		return nil
	}
	rest = strings.TrimSpace(rest)
	m := progressShape.FindStringSubmatch(rest)
	if m == nil {
		return nil
	}
	pct, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	// A number outside the range is not a percentage, so the line is
	// somebody else's and goes back to the stream.
	if pct < 0 || pct > 100 {
		return nil
	}
	label := ""
	if len(m) > 2 {
		label = strings.TrimSpace(m[2])
	}
	return &progressReading{Percent: pct, Label: label}
}

var progressShape = regexp.MustCompile(`^([0-9]+(?:\.[0-9])?)\s*%?(?:\s+(.*))?$`)

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\v' || b == '\f' || b == '\r' || b == '\n'
}

// writeStreamLogLine writes one line out, with the stamp and the tag ahead of
// it.
//
// THE GUEST'S BYTES ARE NEVER TOUCHED unless the caller asked. Everything
// added is to the LEFT of the text.
//
// FOUR TAGS, TWO STREAMS. Guest stdout is written to stdout and guest stderr
// to stderr, because merging them would destroy the fact the tag reports. The
// tick and the note are the WATCHER'S lines rather than the command's, so
// they go to stderr.
//
// COLOUR NEVER REACHES A FILE OR A RECORD. The plain line is built once and
// the escape sequences are added only on the way to a console.
func (st *streamState) writeLine(tag, text string, partial bool, provenance string) {
	cfg := st.settings
	nowD := st.started()
	delta := nowD - st.lastStamp

	// ⛔ A TICK DOES NOT ADVANCE THE DELTA CLOCK. Delta means "since the
	// previous line", and a tick is the ABSENCE of a line rather than one.
	if tag != "tick" && tag != "note" {
		st.lastStamp = nowD
	}

	body := applyRedaction(cfg.Redact, text)
	body = limitLineBytes(body, cfg.MaxBytes)

	prefix, err := formatStreamLogPrefix(st, tag, nowD, delta, partial, nil)
	if err != nil {
		// A prefix that cannot render is said on the watcher's stream, once,
		// and the body still arrives.
		prefix = "??"
	}
	line := prefix + " " + body
	if cfg.PrefixOnly {
		line = prefix
	}

	writer := st.err
	if tag == "out" {
		writer = st.out
	}
	if writer != nil {
		if cfg.Color {
			const e = "\x1b["
			dim := e + "90m"
			off := e + "0m"
			tagColor := ""
			switch tag {
			case "err":
				tagColor = e + "31m"
			case "tick", "note":
				tagColor = e + "36m"
			}
			cut := len(prefix) - 4
			if cut < 0 {
				cut = 0
			}
			coloured := dim + prefix[:cut] + off + tagColor + prefix[cut:] + off
			if cfg.PrefixOnly {
				fmt.Fprintln(writer, coloured)
			} else {
				fmt.Fprintln(writer, coloured+" "+body)
			}
		} else {
			fmt.Fprintln(writer, line)
		}
	}

	if st.text != nil {
		fmt.Fprintln(st.text, line)
	}
	if st.events != nil {
		stream := "watcher"
		kind := "LOG"
		switch tag {
		case "out":
			stream = "stdout"
		case "err":
			stream = "stderr"
		case "tick":
			kind = "TICK"
		case "note":
			kind = "NOTE"
		}
		st.events.writeRecord(kind, nowD, provenance, stream, body, partial, nil)
	}
}

// splitStreamChunk cuts a stream's pending text into the lines that are ready
// to print, and hands back what is not.
//
// A CARRIAGE RETURN TERMINATES A LINE HERE, and that is the difference
// between this and every line-oriented timestamper. curl, apt and every
// layer-progress bar redraw one line with a carriage return and emit no
// newline for minutes, so a reader that waits for a newline shows NOTHING
// while a 200 MB download is visibly working.
//
// A TRAILING CARRIAGE RETURN IS HELD, NEVER EMITTED. It may be the first half
// of a CRLF split across two reads.
func splitStreamChunk(pending string) (ready []readyLine, remainder string) {
	var lines []readyLine
	start := 0
	i := 0
	for i < len(pending) {
		c := pending[i]
		if c == '\n' {
			lines = append(lines, readyLine{Text: pending[start:i], Partial: false})
			i++
			start = i
			continue
		}
		if c == '\r' {
			if i+1 >= len(pending) {
				break
			}
			if pending[i+1] == '\n' {
				lines = append(lines, readyLine{Text: pending[start:i], Partial: false})
				i += 2
				start = i
				continue
			}
			lines = append(lines, readyLine{Text: pending[start:i], Partial: true})
			i++
			start = i
			continue
		}
		i++
	}
	return lines, pending[start:]
}

type readyLine struct {
	Text    string
	Partial bool
}

// getExitCodeDiagnosis is what a non-zero exit code could mean, when the
// number alone is ambiguous.
//
// ⭐ 137 IS THE CODE FROM THE INCIDENT THAT PRODUCED THIS WHOLE LAYER, and it
// means "killed by signal 9" and nothing more. Naming the things that produce
// it, and which of them the readings here can speak to, is a thing a watcher
// can do that a grep cannot.
//
// ⛔ IT NEVER CLAIMS TO KNOW WHICH. Where the evidence does not separate them,
// it says so and lists them.
func getExitCodeDiagnosis(exitCode int, distroState string) string {
	if exitCode == 0 {
		return ""
	}
	if exitCode == 124 {
		return "exit 124 is this tool's own -CommandTimeoutSeconds. The distro was terminated by it."
	}
	if exitCode > 128 && exitCode < 160 {
		sig := exitCode - 128
		named := fmt.Sprintf("signal %d", sig)
		switch sig {
		case 9:
			named = "SIGKILL"
		case 15:
			named = "SIGTERM"
		case 2:
			named = "SIGINT"
		case 11:
			named = "SIGSEGV"
		}
		causes := []string{
			"the kernel out-of-memory killer inside the utility VM, which every WSL distro shares",
			"something outside this run sending a signal",
			"wsl --shutdown, or the utility VM going away, which takes every distro at once",
		}
		ruled := fmt.Sprintf("wsl reports the distro as '%s', which is consistent with the VM having gone", distroState)
		if distroState == "Running" {
			ruled = "the distro is still Running, so the whole utility VM did not go away"
		}
		return fmt.Sprintf("exit %d is 128+%d, which is %s and nothing more. It is produced by: %s. What can be ruled on here: %s. "+
			"This tool did not send it: its own timeout reports 124.",
			exitCode, sig, named, strings.Join(causes, "; "), ruled)
	}
	return fmt.Sprintf("exit %d is the command's own, passed through unchanged.", exitCode)
}

// -- the heartbeat -----------------------------------------------------------

// distroRunState is what WSL says about one distro, for the heartbeat line.
//
// IT NEVER FAILS THE RUN. A heartbeat that can fail is one that stops beating
// at the moment it matters, so an unreadable answer is the word 'unknown' and
// the tick still prints.
func (s *session) distroRunState(distro string) string {
	wsl, err := resolveWsl()
	if err != nil {
		return "unknown"
	}
	res := s.boundedCapture(wsl, []string{"--list", "--verbose"}, 10*time.Second)
	if res.TimedOut || res.Err != nil {
		return "unknown (wsl --list did not answer)"
	}
	for _, raw := range splitLines(res.Text) {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "*") {
			line = strings.TrimSpace(line[1:])
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] != distro {
			continue
		}
		return fields[1]
	}
	return "not registered"
}

// distroDiskFact is the distro's own virtual disk: how big it is.
//
// ⭐ THE SIZE IS THE SIGNAL AND THE WRITE TIME IS NOT. Measured on a real
// machine: the VHDX write time advanced every 1-3s while the guest wrote AND
// while the guest sat in `sleep 14` doing nothing, so recency says nothing
// about the command. The LENGTH moved from 79,691,776 to 583,008,256 while
// the guest allocated, and was flat while it did not.
//
// ⚠ SO GROWTH IS EVIDENCE AND FLATNESS IS NOT. A disk that grew means
// something inside allocated. A disk that did not grow rules nothing out.
//
// IT NEVER FAILS THE RUN, for the same reason distroRunState does not.
func (s *session) distroDiskFact(distro string) *int64 {
	dir := filepath.Join(s.baseDir, distro)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var best *string
	var bestName string
	var bestSize int64 = -1
	for i := range entries {
		e := entries[i]
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".vhdx") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.Size() > bestSize {
			bestSize = info.Size()
			bestName = e.Name()
			best = &bestName
		}
	}
	if best == nil {
		return nil
	}
	return &bestSize
}

// writeStreamLogTick is the heartbeat, and the reason this layer exists at
// all.
//
// NOTHING IS INJECTED INTO THE GUEST TO PRODUCE IT. Every figure on the line
// is one this process already holds: its own monotonic clock, the counts it
// kept while relaying, one read-only question to wsl.exe, and the length of a
// file on this host's own disk. An image with no shell, no coreutils and no
// clock ticks exactly as well as a full userspace.
//
// IT FIRES ON SILENCE, NOT ON A TIMER. A command printing a line a second
// produces no ticks at all.
//
// ⭐ IT SAYS MORE AS THE SILENCE GROWS, rather than the same thing again. A
// line repeated forty times is one a reader stops reading.
func (st *streamState) writeTick(s *session) {
	nowD := st.started()
	silent := nowD - st.lastLine
	disk := s.distroDiskFact(st.distro)
	// ⭐ THE DELTA SINCE THE PREVIOUS TICK, not the absolute size. The size on
	// its own is the same number every tick; what moved between two readings
	// is the only part that is evidence.
	var grew *int64
	if disk != nil {
		if st.lastDisk != nil {
			g := *disk - *st.lastDisk
			grew = &g
		}
		st.lastDisk = disk
	}
	diskText := "disk unreadable"
	if disk != nil {
		switch {
		case grew == nil:
			diskText = "disk " + formatByteCount(*disk)
		case *grew > 0:
			diskText = "disk " + formatByteCount(*disk) + " (+" + formatByteCount(*grew) + " since last tick)"
		default:
			diskText = "disk " + formatByteCount(*disk) + " (unchanged)"
		}
	}
	runState := s.distroRunState(st.distro)
	text := formatDuration(silent) + " silent | elapsed " + formatDuration(nowD) +
		" | out " + strconv.FormatInt(st.counts["out"].Lines, 10) + " lines " + formatByteCount(st.counts["out"].Bytes) +
		" | err " + strconv.FormatInt(st.counts["err"].Lines, 10) + " lines " + formatByteCount(st.counts["err"].Bytes) +
		" | distro " + runState + " | " + diskText
	if st.progress != nil {
		age := nowD - st.progress.At
		shown := "progress " + formatPercent(st.progress.Percent)
		if st.progress.Label != "" {
			shown += " " + st.progress.Label
		}
		text += " | " + shown + " (" + formatDuration(age) + " ago)"
	}
	st.writeLine("tick", text, false, "obs")
	if st.events != nil {
		data := map[string]any{
			"silence_s":    roundHalfUp(silent.Seconds(), 1),
			"distro_state": runState,
			"out_lines":    st.counts["out"].Lines,
			"err_lines":    st.counts["err"].Lines,
		}
		if disk != nil {
			data["disk_bytes"] = *disk
		}
		if grew != nil {
			data["disk_grew_bytes"] = *grew
		}
		if st.progress != nil {
			data["progress_percent"] = st.progress.Percent
			data["progress_age_s"] = roundHalfUp((nowD - st.progress.At).Seconds(), 1)
			if st.progress.Label != "" {
				data["progress_label"] = st.progress.Label
			}
		}
		st.events.writeRecord("TICK_FACTS", nowD, "obs", "", "", false, data)
	}
	st.writeEscalation(s, silent, grew, runState)
	st.lastTick = nowD
}

func roundHalfUp(v float64, digits int) float64 {
	p := 1.0
	for i := 0; i < digits; i++ {
		p *= 10
	}
	return float64(int64(v*p+0.5)) / p
}

// writeEscalation is what the tick adds once the silence passes a threshold,
// and each threshold fires ONCE per silence rather than on every tick after
// it.
//
// ⛔ NOTHING HERE IS STATED AS A FACT ABOUT THE COMMAND. The strongest thing
// it says is what the readings are consistent with, marked as an inference,
// with the readings it was drawn from named on the same line. Proving a hang
// needs the program's intent, which a watcher does not have, and a watcher
// that says "hung" will one day say it about a working build.
func (st *streamState) writeEscalation(s *session, silent dur, diskGrew *int64, distroState string) {
	for _, step := range st.settings.Escalate {
		if silent.Seconds() < float64(step) {
			continue
		}
		firedAlready := false
		for _, f := range st.fired {
			if f == step {
				firedAlready = true
				break
			}
		}
		if firedAlready {
			continue
		}
		st.fired = append(st.fired, step)

		if len(st.fired) == 1 {
			st.writeLine("note",
				"this tool relays the guest through a pipe rather than a terminal, so an "+
					"application that block-buffers off a tty is buffering, and a line will arrive "+
					"late carrying the time it was RECEIVED. -NoTimestamps hands the handles through "+
					"untouched and has none of that.", false, "obs")
		}

		var because []string
		verdict := "nothing can be concluded from what is measurable here"
		switch {
		case distroState != "Running":
			because = append(because, fmt.Sprintf("wsl says the distro is '%s'", distroState))
			verdict = "the distro is not running, so this is not a quiet command"
		case diskGrew == nil:
			because = append(because, "the distro disk could not be read, so there is no second signal")
		case *diskGrew > 0:
			because = append(because, fmt.Sprintf("the distro disk grew %s between the last two ticks", formatByteCount(*diskGrew)))
			verdict = "something inside allocated while it was quiet. Consistent with a download, an " +
				"unpack or a build that reports nothing"
		default:
			// ⛔ THIS BRANCH RULES NOTHING OUT AND SAYS SO. A flat disk is not
			// evidence of a stall: a computation that writes no files produces
			// exactly this reading, and so does a deadlock.
			because = append(because, "the distro disk did not grow between the last two ticks, and that reading "+
				"is coarse: it moves in large steps and has been measured not to change "+
				"six seconds after a guest wrote 120 MiB")
			verdict = "NOTHING is ruled out. A prompt waiting on stdin that was never attached, a " +
				"lock, a network call inside a long connect timeout, a computation that writes " +
				"no files, and a guest writing hard whose disk has not been extended yet all " +
				"read exactly like this"
		}
		st.writeLine("note",
			fmt.Sprintf("after %s of silence: %s. because: %s", formatDuration(silent), verdict, strings.Join(because, "; ")),
			false, "inf")

		if len(st.fired) >= 2 {
			st.writeLine("note",
				"to bound a run like this, pass -CommandTimeoutSeconds N: the distro is terminated and "+
					"the exit code is 124. To see what is registered right now, from another shell: "+
					"wsl-toolkit script -Action List", false, "obs")
		}
	}
}

// writeSilenceEnd is what the tick says when output came back.
//
// ⭐ IT IS A LINE BECAUSE THE ABSENCE OF ONE IS AMBIGUOUS. A run that
// recovered at four minutes and a run that never recovered look identical in
// a log that reports only the alarm.
func (st *streamState) writeSilenceEnd(silent dur) {
	st.writeLine("note", "output resumed after "+formatDuration(silent)+" of silence", false, "obs")
	st.fired = nil
}

// formatPercent renders a percentage the same way everywhere it is shown.
//
// ⚠ INVARIANT RENDERING, always, because a machine whose decimal separator is
// a comma would otherwise render 42.5 as '42,5' in a log a script parses.
func formatPercent(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10) + "%"
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + "%"
}
