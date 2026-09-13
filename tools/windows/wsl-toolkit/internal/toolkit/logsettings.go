// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Every decision about how a relayed command's stream renders, and where copies
// of it go, is resolved here once, before anything runs. The renderer, the file
// copy, the event record and a replay all read the result, so no sink can be
// reached by a path that skipped a rule, and a typo in a format is refused before
// a distribution is imported rather than on the first line of output.

// LogRequest is what the caller typed. A Set flag records that a value was passed
// explicitly, which is what lets a profile be a starting point rather than a
// mode: anything passed beside a profile wins over it.
type LogRequest struct {
	Profile        string
	Mode           string
	Columns        []string
	Format         string
	Separator      string
	SeparatorSet   bool
	PrefixOnly     bool
	Color          string
	ColorSet       bool
	TextPath       string
	TextOverwrite  bool
	EventPath      string
	Redact         []string
	MaxLineBytes   int
	ProgressPrefix string
	Tick           time.Duration
	TickSet        bool
	Escalate       string
	EscalateSet    bool
	// Terminal is whether both of this process's output streams are consoles,
	// decided once so a colour decision cannot change halfway through a run.
	Terminal bool
}

// LogSettings is a LogRequest resolved against its profile and validated.
type LogSettings struct {
	Profile        string            `json:"profile"`
	Columns        []string          `json:"columns,omitempty"`
	Formats        map[string]string `json:"formats,omitempty"`
	Separator      string            `json:"separator"`
	PrefixOnly     bool              `json:"prefix_only,omitempty"`
	Color          bool              `json:"color"`
	TextPath       string            `json:"text_path,omitempty"`
	TextOverwrite  bool              `json:"text_overwrite,omitempty"`
	EventPath      string            `json:"event_path,omitempty"`
	RedactCount    int               `json:"redact_patterns,omitempty"`
	MaxLineBytes   int               `json:"max_line_bytes,omitempty"`
	ProgressPrefix string            `json:"progress_prefix,omitempty"`
	Tick           time.Duration     `json:"-"`
	Escalate       []time.Duration   `json:"-"`
	redact         []*regexp.Regexp
}

// MaxLineBytesLimit bounds --max-line-bytes, and is the most a pending
// unterminated line is held before it is written out as a partial one.
const MaxLineBytesLimit = 1 << 20

// ProfileTick is the heartbeat a rendering profile turns on when --tick was not
// passed: thirty seconds of silence, the interval the stream log was built with.
const ProfileTick = 30 * time.Second

// DefaultEscalation is where a heartbeat says more as a silence grows.
var DefaultEscalation = []time.Duration{2 * time.Minute, 5 * time.Minute, 15 * time.Minute}

var columnNames = []string{"rel", "delta", "wall", "iso", "epoch"}

// Active reports whether the command needs the relay at all. With nothing active
// the command's streams are forwarded unchanged and nothing is recorded.
func (s LogSettings) Active() bool {
	return len(s.Columns) > 0 || s.TextPath != "" || s.EventPath != "" || len(s.redact) > 0 ||
		s.MaxLineBytes > 0 || s.ProgressPrefix != "" || s.Tick > 0
}

// Renders reports whether the live streams are rewritten line by line rather
// than passed through byte for byte.
//
// ⚠ A RELAYED LINE IS NOT THE GUEST'S BYTES. It is re-terminated with a newline,
// a carriage-return redraw becomes a line of its own, and an application that
// block-buffers off a terminal shows its lines late. So only the options that
// have to change what is shown do it: a prefix, a redaction, a truncation, or a
// consumed progress line. A sink or a heartbeat alone leaves the live bytes exact.
func (s LogSettings) Renders() bool {
	return len(s.Columns) > 0 || len(s.redact) > 0 || s.MaxLineBytes > 0 || s.ProgressPrefix != ""
}

// ResolveLogSettings applies the profile, then every explicit value over it, and
// refuses every combination in which a flag the caller typed would do nothing.
func ResolveLogSettings(r LogRequest) (LogSettings, error) {
	s := LogSettings{Profile: strings.ToLower(strings.TrimSpace(r.Profile)), Separator: " ", Formats: map[string]string{}}
	if r.SeparatorSet {
		s.Separator = r.Separator
	}
	color := strings.ToLower(strings.TrimSpace(r.Color))
	if color == "" {
		color = "auto"
	}
	switch color {
	case "auto", "always", "never":
	default:
		return s, fmt.Errorf("--color %q is not auto, always or never", r.Color)
	}

	mode := strings.ToLower(strings.TrimSpace(r.Mode))
	var explicitCols []string
	for _, raw := range r.Columns {
		for _, c := range strings.Split(raw, ",") {
			if c = strings.ToLower(strings.TrimSpace(c)); c != "" {
				explicitCols = append(explicitCols, c)
			}
		}
	}
	if len(r.Columns) > 0 && len(explicitCols) == 0 {
		return s, errors.New("--timestamp-column was passed with nothing in it")
	}
	// ⛔ TWO SPELLINGS OF ONE DECISION ARE REFUSED TOGETHER. A precedence between
	// them is a rule a caller would have to remember to predict their own output.
	if mode != "" && len(explicitCols) > 0 {
		return s, errors.New("--timestamp-mode and --timestamp-column say the same thing two ways. Pass one; --timestamp-column is the one that can carry more than a single column")
	}
	if mode != "" {
		explicitCols = []string{mode}
	}
	for i, c := range explicitCols {
		if c == "relative" {
			explicitCols[i] = "rel"
		}
	}

	profileTick := time.Duration(0)
	switch s.Profile {
	case "":
	case "raw":
		// ⛔ RAW RENDERS NO PREFIX, so a renderer flag beside it would do nothing.
		// The sinks, redaction and the heartbeat still apply: they are not rendering.
		var dead []string
		if len(explicitCols) > 0 {
			dead = append(dead, "--timestamp-mode/--timestamp-column")
		}
		if r.Format != "" {
			dead = append(dead, "--timestamp-format")
		}
		if r.SeparatorSet {
			dead = append(dead, "--timestamp-separator")
		}
		if r.PrefixOnly {
			dead = append(dead, "--prefix-only")
		}
		if r.ColorSet {
			dead = append(dead, "--color")
		}
		if len(dead) > 0 {
			return s, fmt.Errorf("--log-profile raw renders no prefix, so %s would do nothing. Drop raw to use them, or drop them", strings.Join(dead, " and "))
		}
	case "human":
		s.Columns = []string{"rel"}
		profileTick = ProfileTick
	case "ci":
		s.Columns = []string{"rel", "delta"}
		profileTick = ProfileTick
		if !r.ColorSet {
			color = "never"
		}
	case "forensic":
		s.Columns = []string{"wall", "rel", "delta"}
		s.Formats["wall"] = "%Y-%m-%d %H:%M:%S.%6f"
		s.Formats["rel"] = "%H:%M:%S.%6f"
		profileTick = ProfileTick
		if !r.ColorSet {
			color = "never"
		}
	case "wall":
		s.Columns = []string{"wall"}
		profileTick = ProfileTick
	default:
		return s, fmt.Errorf("--log-profile %q is not raw, human, ci, forensic or wall", r.Profile)
	}
	if len(explicitCols) > 0 {
		s.Columns = explicitCols
	}
	seen := map[string]bool{}
	for _, c := range s.Columns {
		if !containsString(columnNames, c) {
			return s, fmt.Errorf("timestamp column %q is not one of %s", c, strings.Join(columnNames, ", "))
		}
		if seen[c] {
			return s, fmt.Errorf("timestamp column %q is named twice", c)
		}
		seen[c] = true
	}
	if r.Format != "" {
		if !seen["rel"] && !seen["wall"] {
			return s, fmt.Errorf("--timestamp-format renders a date or an elapsed time, and none of the column(s) selected takes one: %s. It applies to rel and wall", strings.Join(s.Columns, ", "))
		}
		s.Formats = map[string]string{"rel": r.Format, "wall": r.Format}
	}
	// ⛔ EVERY SELECTED COLUMN IS RENDERED ONCE HERE, so an unknown specifier, or
	// a date specifier on an elapsed time, is refused before anything runs.
	for _, c := range s.Columns {
		if _, err := renderColumn(c, s.formatFor(c), time.Now(), 0, 0); err != nil {
			return s, err
		}
	}
	if r.PrefixOnly && len(s.Columns) == 0 {
		return s, errors.New("--prefix-only writes the timestamp prefix without the line, and no timestamp column is selected")
	}
	s.PrefixOnly = r.PrefixOnly
	switch color {
	case "always":
		s.Color = true
	case "auto":
		s.Color = r.Terminal && os.Getenv("NO_COLOR") == ""
	}

	for label, path := range map[string]string{"--stream-log": r.TextPath, "--event-log": r.EventPath} {
		if err := AssertSinkPath(label, path); err != nil {
			return s, err
		}
	}
	if r.TextPath != "" && r.EventPath != "" && strings.EqualFold(r.TextPath, r.EventPath) {
		return s, errors.New("--stream-log and --event-log name the same file, and two writers interleaving into one file produce neither")
	}
	if r.TextOverwrite && r.TextPath == "" {
		return s, errors.New("--stream-log-overwrite replaces the file --stream-log names, and no --stream-log was passed")
	}
	s.TextPath, s.TextOverwrite, s.EventPath = r.TextPath, r.TextOverwrite, r.EventPath

	for _, raw := range r.Redact {
		// ⚠ SPLIT ON COMMAS, so one flag can carry several patterns. A pattern
		// that needs a literal comma writes it as the class [,].
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p == "" {
				continue
			}
			re, err := regexp.Compile(p)
			if err != nil {
				return s, fmt.Errorf("--redact %q is not a regular expression: %w", p, err)
			}
			s.redact = append(s.redact, re)
		}
	}
	s.RedactCount = len(s.redact)
	if r.MaxLineBytes < 0 || r.MaxLineBytes > MaxLineBytesLimit {
		return s, fmt.Errorf("--max-line-bytes %d is outside 0 to %d. 0 never truncates", r.MaxLineBytes, MaxLineBytesLimit)
	}
	s.MaxLineBytes = r.MaxLineBytes
	token := r.ProgressPrefix
	if strings.ContainsAny(token, " \t\r\n") {
		return s, fmt.Errorf("--progress-prefix %q carries whitespace. A progress line is the token, whitespace, then a percentage, so the token itself cannot contain any", r.ProgressPrefix)
	}
	s.ProgressPrefix = token

	s.Tick = profileTick
	if r.TickSet {
		s.Tick = r.Tick
	}
	if s.Tick < 0 {
		return s, fmt.Errorf("--tick %s is negative. Pass 0 for no heartbeat, or a positive duration", s.Tick)
	}
	if s.Tick > 0 && s.Tick < MinTickInterval {
		s.Tick = MinTickInterval
	}
	if s.Tick > 0 {
		s.Escalate = append([]time.Duration(nil), DefaultEscalation...)
	}
	if r.EscalateSet {
		steps, err := ParseEscalation(r.Escalate)
		if err != nil {
			return s, err
		}
		if len(steps) > 0 && s.Tick == 0 {
			return s, errors.New("--tick-escalate says more at a silence threshold, and there is no heartbeat to say it. Pass --tick or a rendering --log-profile")
		}
		s.Escalate = steps
	}
	return s, nil
}

// ParseEscalation reads comma-separated silence thresholds, or none.
//
// ⛔ EACH IS A DURATION PARSED HERE, so `2m,5m` is two thresholds and anything
// that is not a duration is a refusal rather than a number nobody typed.
func ParseEscalation(raw string) ([]time.Duration, error) {
	v := strings.TrimSpace(raw)
	if v == "" || strings.EqualFold(v, "none") {
		return nil, nil
	}
	set := map[time.Duration]bool{}
	for _, piece := range strings.Split(v, ",") {
		piece = strings.TrimSpace(piece)
		if piece == "" {
			continue
		}
		d, err := time.ParseDuration(piece)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("--tick-escalate %q is not a positive duration. Give them comma-separated, as 2m,5m,15m, or pass none", piece)
		}
		set[d] = true
	}
	out := make([]time.Duration, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (s LogSettings) formatFor(column string) string {
	if f := s.Formats[column]; f != "" {
		return f
	}
	switch column {
	case "wall":
		return "%Y-%m-%d %H:%M:%S"
	case "rel":
		return "%H:%M:%S.%3f"
	}
	return ""
}

// Redact replaces every match of every pattern before any sink sees the text.
//
// ⚠ A REPLACEMENT FUNCTION, NOT A REPLACEMENT STRING. A string would expand `$`
// sequences, so a marker carrying one would paste part of the match back in.
func (s LogSettings) Redact(text string) string {
	for _, re := range s.redact {
		text = re.ReplaceAllStringFunc(text, func(string) string { return "***" })
	}
	return text
}

// LimitLineBytes cuts the text at the bound and says how much went.
//
// ⛔ IT COUNTS BYTES AND CUTS WHOLE CHARACTERS. A cut through a multi-byte
// character produces a replacement character the guest never wrote.
func LimitLineBytes(text string, max int) string {
	if max <= 0 || len(text) <= max {
		return text
	}
	cut := 0
	for cut < len(text) {
		_, size := utf8.DecodeRuneInString(text[cut:])
		if cut+size > max {
			break
		}
		cut += size
	}
	return text[:cut] + "...(+" + strconv.Itoa(len(text)-cut) + " bytes cut)"
}

// renderColumn is one column's text, a pure function of its clock readings, so a
// prefix is rebuilt from a record without re-running anything.
func renderColumn(column, format string, wall time.Time, elapsed, delta time.Duration) (string, error) {
	switch column {
	case "delta":
		if delta < 0 {
			delta = 0
		}
		return fmt.Sprintf("+%d.%03d", int64(delta/time.Second), int64(delta%time.Second/time.Millisecond)), nil
	case "epoch":
		return strconv.FormatInt(wall.Unix(), 10), nil
	case "iso":
		return Strftime("%Y-%m-%dT%H:%M:%S.%3f%z", wall, 0, false)
	case "wall":
		return Strftime(format, wall, 0, false)
	case "rel":
		return Strftime(format, time.Time{}, elapsed, true)
	}
	return "", fmt.Errorf("timestamp column %q is not one of %s", column, strings.Join(columnNames, ", "))
}

// Strftime renders a format with the specifier set tss uses.
//
// ⛔ AN UNKNOWN SPECIFIER IS REFUSED, and so is one with no meaning in the mode
// asked for: an elapsed time has no year, and rendering 0001 would put an
// invented number on a log line. %9f pads below the clock's own resolution, which
// `wsl-toolkit doctor` measures rather than this function claiming it.
func Strftime(format string, wall time.Time, elapsed time.Duration, relative bool) (string, error) {
	values := map[string]string{"%": "%"}
	var sub int64
	if relative {
		if elapsed < 0 {
			elapsed = 0
		}
		sub = int64(elapsed % time.Second)
		// ⚠ TOTAL HOURS, so a run past a day reads 25:00:01 rather than wrapping.
		values["H"] = fmt.Sprintf("%02d", int64(elapsed/time.Hour))
		values["M"] = fmt.Sprintf("%02d", int64(elapsed/time.Minute)%60)
		values["S"] = fmt.Sprintf("%02d", int64(elapsed/time.Second)%60)
	} else {
		sub = int64(wall.Nanosecond())
		_, offset := wall.Zone()
		sign := "+"
		if offset < 0 {
			sign, offset = "-", -offset
		}
		zone, _ := wall.Zone()
		values["Y"] = fmt.Sprintf("%04d", wall.Year())
		values["m"] = fmt.Sprintf("%02d", int(wall.Month()))
		values["d"] = fmt.Sprintf("%02d", wall.Day())
		values["H"] = fmt.Sprintf("%02d", wall.Hour())
		values["M"] = fmt.Sprintf("%02d", wall.Minute())
		values["S"] = fmt.Sprintf("%02d", wall.Second())
		values["z"] = fmt.Sprintf("%s%02d:%02d", sign, offset/3600, offset%3600/60)
		values["Z"] = zone
	}
	values["3f"] = fmt.Sprintf("%03d", sub/int64(time.Millisecond))
	values["6f"] = fmt.Sprintf("%06d", sub/int64(time.Microsecond))
	values["9f"] = fmt.Sprintf("%09d", sub)

	var b strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		if i+1 >= len(format) {
			return "", errors.New("--timestamp-format ends with a bare %")
		}
		key := string(format[i+1])
		width := 2
		if (key == "3" || key == "6" || key == "9") && i+2 < len(format) && format[i+2] == 'f' {
			key += "f"
			width = 3
		}
		v, ok := values[key]
		if !ok {
			mode := "wall"
			if relative {
				mode = "rel"
			}
			keys := make([]string, 0, len(values))
			for k := range values {
				keys = append(keys, "%"+k)
			}
			sort.Strings(keys)
			return "", fmt.Errorf("--timestamp-format carries %%%s, which is not a specifier the %s column renders. The set there is %s", key, mode, strings.Join(keys, " "))
		}
		b.WriteString(v)
		i += width - 1
	}
	return b.String(), nil
}

// AssertSinkPath refuses a log path that cannot mean what the caller thinks.
//
// ⛔ A WINDOWS RESERVED DEVICE NAME IS REFUSED BY NAME. `nul` discards every byte
// and reports success, and `con` prints the copy back into the stream the log
// exists to keep clean. The name is judged on its last segment, split on both
// separators whatever host is asking, with everything from its first dot removed.
func AssertSinkPath(label, path string) error {
	if path == "" {
		return nil
	}
	leaf := path
	if i := strings.LastIndexAny(leaf, `\/`); i >= 0 {
		leaf = leaf[i+1:]
	}
	if i := strings.IndexByte(leaf, '.'); i >= 0 {
		leaf = leaf[:i]
	}
	if windowsDeviceName(strings.ToUpper(strings.TrimSpace(leaf))) {
		return fmt.Errorf("%s %q names the Windows device %s. Writing to it discards everything and reports success, so the run would end with no log and no error. Pick a real path", label, path, strings.ToUpper(leaf))
	}
	return nil
}

// progressLine is ^TOKEN<whitespace>PERCENT[%][<whitespace>LABEL]$ after the
// token, with at most one decimal place.
var progressLine = regexp.MustCompile(`^([0-9]+(?:\.[0-9])?)\s*%?(?:\s+(.*))?$`)

// ParseProgress reads one complete line as a progress report, or reports that it
// is ordinary output.
//
// ⛔ THE TOKEN IS THE CALLER'S, it must end at whitespace, and a malformed or
// out-of-range line is RELAYED rather than swallowed: consuming a line nobody
// can see the reason for is the defect this parse exists to avoid.
func ParseProgress(token, line string) (percent float64, label string, ok bool) {
	if token == "" || !strings.HasPrefix(line, token) {
		return 0, "", false
	}
	rest := line[len(token):]
	if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
		return 0, "", false
	}
	m := progressLine.FindStringSubmatch(strings.TrimSpace(rest))
	if m == nil {
		return 0, "", false
	}
	// The pattern admits only digits and one decimal place, so no sign, no
	// exponent and no NaN reaches the parse, and a value past 100 is the one left.
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil || v > 100 {
		return 0, "", false
	}
	return v, strings.TrimSpace(m[2]), true
}

// FormatPercent renders a percentage the same way everywhere: whole numbers
// without a point, one place otherwise.
func FormatPercent(v float64) string {
	if v == math.Floor(v) {
		return strconv.FormatFloat(v, 'f', 0, 64) + "%"
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + "%"
}

// FormatSpan renders a duration for a person: 59s, 1m00s, 1h00m.
func FormatSpan(d time.Duration) string {
	total := int64(d / time.Second)
	if total < 0 {
		total = 0
	}
	switch {
	case total < 60:
		return fmt.Sprintf("%ds", total)
	case total < 3600:
		return fmt.Sprintf("%dm%02ds", total/60, total%60)
	}
	return fmt.Sprintf("%dh%02dm", total/3600, total%3600/60)
}

// DiagnoseExit says what a nonzero exit could mean, where the number alone is
// ambiguous, and never claims to know which. It reads a command whose deadline did
// not fire and that was not cancelled; those two are said by name elsewhere.
func DiagnoseExit(code int, distroState string) string {
	switch {
	case code == 0:
		return ""
	case code == ExitTimeout:
		return "exit 124 is the command's own: this tool's --timeout did not fire, so a timeout inside the command is the likelier reading"
	case code > 128 && code < 160:
		sig := code - 128
		name := map[int]string{2: "SIGINT", 9: "SIGKILL", 11: "SIGSEGV", 15: "SIGTERM"}[sig]
		if name == "" {
			name = "signal " + strconv.Itoa(sig)
		}
		ruled := "WSL reports the distribution as " + distroState + ", which is consistent with the virtual machine having gone away"
		if distroState == "running" {
			ruled = "the distribution is still running, so the whole WSL virtual machine did not go away"
		}
		return fmt.Sprintf("exit %d is 128+%d, which is %s and nothing more. It is produced by the out-of-memory killer inside the WSL virtual machine every distribution shares, "+
			"by something outside this run sending a signal, or by wsl --shutdown. What can be ruled on here: %s. This tool did not send it: its own deadline answers 124", code, sig, name, ruled)
	}
	return fmt.Sprintf("exit %d is the command's own, passed through unchanged", code)
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
