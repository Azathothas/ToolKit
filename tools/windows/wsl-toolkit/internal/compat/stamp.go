// SPDX-License-Identifier: 0BSD

package compat

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The stream log: a timestamp on every line, and a heartbeat when there are
// none. Ported from the script's stamp.ps1 and stream-log.ps1.
//
// THE FAILURE IT EXISTS FOR: a command prints nothing for twenty minutes and
// a caller reading a pipe cannot tell that from a command that has died.
// Silence has to be a line, or it says nothing at all.

// logSettings is every decision about how the log RENDERS, resolved once,
// before anything runs.
//
// ⭐ ONE PLACE, AND IT IS A PURE FUNCTION except for the colour probe, which
// is passed in. The renderer, the file copy and the event record all read
// this object.
type logSettings struct {
	Columns       []string
	Format        string
	Separator     string
	PrefixOnly    bool
	Color         bool
	TextPath      string
	TextOverwrite bool
	EventPath     string
	Redact        []*redaction
	MaxBytes      int
	TickSeconds   int
	Escalate      []int
}

// splitDelimitedArgument splits one list parameter, however the caller was
// able to spell it.
//
// ⛔ THE DEFECT THIS EXISTS FOR IS MEASURED AND SILENT. A .ps1 run through
// `-File` cannot be given a real array and cannot have a parameter repeated:
// `-X 5,9` arrived as ONE string, "5,9", and PowerShell then converted it to
// the single value 59 through a culture where the comma is the THOUSANDS
// separator. A list parameter splits its own value.
//
// ⚠ IT IS NOT SAFE FOR EVERY LIST. It is correct where a comma cannot occur
// inside a value (a number, a column name) and where a documented escape
// exists (a regex writes a literal comma as the class [,]). It is WRONG for
// arbitrary text, which is why -ScriptArg takes a FILE instead.
func splitDelimitedArgument(values []string) []string {
	var out []string
	for _, v := range values {
		for _, piece := range strings.Split(v, ",") {
			t := strings.TrimSpace(piece)
			if t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

// resolveStreamLogSettings resolves every rendering decision. explicit is the
// set of parameters the caller actually passed, because a profile is a
// starting point, not a mode: anything passed beside one wins over it.
func resolveStreamLogSettings(o Options, colorAllowed bool) (*logSettings, error) {
	mode := o.TimestampMode
	columns := append([]string{}, o.TimestampColumns...)
	format := o.TimestampFormat
	separator := o.TimestampSeparator
	colorChoice := o.Color

	switch o.TimestampProfile {
	case "ci":
		if !o.wasPassed("TimestampColumns") {
			columns = []string{"rel", "delta"}
		}
		if !o.wasPassed("Color") {
			colorChoice = "never"
		}
	case "forensic":
		if !o.wasPassed("TimestampColumns") {
			columns = []string{"wall", "rel", "delta"}
		}
		if !o.wasPassed("TimestampFormat") {
			format = "%Y-%m-%d %H:%M:%S.%6f"
		}
		if !o.wasPassed("Color") {
			colorChoice = "never"
		}
	case "wall":
		// tss's own defaults, so a caller who already timestamps a pipeline
		// with that tool gets the same bytes from this one.
		if !o.wasPassed("TimestampColumns") {
			columns = []string{"wall"}
		}
		if !o.wasPassed("TimestampFormat") {
			format = "%Y-%m-%d %H:%M:%S"
		}
	default:
		// 'human' is the built-in default; 'raw' is handled in Run.
	}

	modeWasPassed := o.wasPassed("TimestampMode") && o.TimestampProfile == ""
	cols, err := resolveStampColumns(columns, mode, modeWasPassed)
	if err != nil {
		return nil, err
	}

	if format != "" {
		takes := false
		for _, c := range cols {
			if columnTakesFormat(c) {
				takes = true
				break
			}
		}
		if !takes {
			return nil, fmt.Errorf("-TimestampFormat renders a date or an elapsed time, and none of the column(s) you "+
				"asked for takes one: %s. It applies to 'rel' and 'wall'.", strings.Join(cols, ", "))
		}
	}

	// ⛔ 'auto' IS DECIDED ONCE, HERE, and not per line.
	useColor := false
	switch colorChoice {
	case "always":
		useColor = true
	case "never":
		useColor = false
	default:
		useColor = colorAllowed && envValue("NO_COLOR") == ""
	}

	// ⛔ SPLIT AND PARSED HERE, NEVER BY A NUMBERED PARAMETER, for the
	// thousand-separator reason splitDelimitedArgument records.
	var esc []int
	for _, e := range splitDelimitedArgument(o.TickEscalateSeconds) {
		n, err := strconv.Atoi(e)
		if err != nil {
			return nil, fmt.Errorf("-TickEscalateSeconds '%s' is not a whole number of seconds. Give them comma-separated, as 120,300,900.", e)
		}
		if n > 0 {
			esc = append(esc, n)
		}
	}
	sort.Ints(esc)
	seen := map[int]bool{}
	uniq := esc[:0]
	for _, n := range esc {
		if !seen[n] {
			seen[n] = true
			uniq = append(uniq, n)
		}
	}

	redactions, err := newRedactionSet(splitDelimitedArgument(o.Redact))
	if err != nil {
		return nil, err
	}

	return &logSettings{
		Columns:       cols,
		Format:        format,
		Separator:     separator,
		PrefixOnly:    o.PrefixOnly,
		Color:         useColor,
		TextPath:      o.StreamLogPath,
		TextOverwrite: o.StreamLogOverwrite,
		EventPath:     o.EventLog,
		Redact:        redactions,
		MaxBytes:      o.MaxLineBytes,
		TickSeconds:   o.TickSeconds,
		Escalate:      uniq,
	}, nil
}

// resolveStampColumns turns -TimestampMode or -TimestampColumns into the
// ordered list the renderer walks.
//
// ⛔ PASSING BOTH IS REFUSED. They are two spellings of one decision, and a
// precedence between them would be a rule a caller has to remember.
func resolveStampColumns(columns []string, mode string, modeWasPassed bool) ([]string, error) {
	known := map[string]bool{"rel": true, "delta": true, "wall": true, "iso": true, "epoch": true}
	if len(columns) > 0 {
		if modeWasPassed {
			return nil, fmt.Errorf("-TimestampColumns and -TimestampMode say the same thing two ways. Pass one. " +
				"-TimestampColumns is the one that can carry more than a single value.")
		}
		var out []string
		for _, raw := range columns {
			for _, c := range strings.Split(raw, ",") {
				t := strings.ToLower(strings.TrimSpace(c))
				if t == "" {
					continue
				}
				if !known[t] {
					return nil, fmt.Errorf("-TimestampColumns carries '%s'. The set is: rel, delta, wall, iso, epoch.", t)
				}
				for _, have := range out {
					if have == t {
						return nil, fmt.Errorf("-TimestampColumns names '%s' twice.", t)
					}
				}
				out = append(out, t)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("-TimestampColumns was passed with nothing in it.")
		}
		return out, nil
	}
	switch mode {
	case "Delta":
		return []string{"delta"}, nil
	case "Wall":
		return []string{"wall"}, nil
	case "Iso":
		return []string{"iso"}, nil
	case "Epoch":
		return []string{"epoch"}, nil
	default:
		return []string{"rel"}, nil
	}
}

func columnTakesFormat(column string) bool {
	return column == "rel" || column == "wall"
}

// stampDefaultFormat is tss's own default for a wall clock, and milliseconds
// for a relative one: a distro's lifecycle is interesting at that resolution,
// and a date repeated on every line of one run is a column nobody reads.
func stampDefaultFormat(mode string) string {
	if mode == "Wall" {
		return "%Y-%m-%d %H:%M:%S"
	}
	return "%H:%M:%S.%3f"
}

// formatStrftimeStamp renders a strftime format string. It is the surface
// `tss` has, because a caller who already timestamps a pipeline with that tool
// should not have to learn a second spelling.
//
// ⛔ IT DOES NOT BUILD A C LIBRARY FORMAT AND HAND IT TO A LOCALE. Every value
// here is formatted with the invariant culture and substituted directly, so a
// literal character in the format stays that character on every host.
//
// AN UNKNOWN SPECIFIER IS REFUSED. A format that silently renders '%q' as 'q'
// is a caller believing they asked for something. A SPECIFIER WITH NO MEANING
// IN THIS MODE IS REFUSED TOO: relative mode measures a duration, so '%Y' has
// no value to render, and answering 1970 would put an invented number on a
// log line.
//
// %9f CANNOT BE MEASURED HERE AND IT PADS. The tick is 100ns, so the ninth
// digit is a zero this function wrote. The page says so, rather than leaving
// nine digits to be read as nine digits of measurement.
func formatStrftimeStamp(format string, wall wallReading, elapsed dur, mode string) (string, error) {
	values := map[string]string{}
	var sub int64
	if mode == "Wall" {
		sub = wall.SubsecondTicks()
		values["Y"] = pad4(int64(wall.Year))
		values["m"] = pad2(int64(wall.Month))
		values["d"] = pad2(int64(wall.Day))
		values["H"] = pad2(int64(wall.Hour))
		values["M"] = pad2(int64(wall.Minute))
		values["S"] = pad2(int64(wall.Second))
		sign, offHours, offMinutes := wall.OffsetParts()
		values["z"] = sign + pad2(int64(offHours)) + ":" + pad2(int64(offMinutes))
		values["Z"] = wall.Zone
	} else {
		sub = elapsedTicks(elapsed) % 10_000_000
		// TOTAL hours, not hours-within-a-day. A run that passes 24 hours
		// reads 24:00:01 rather than starting again at zero: a relative stamp
		// that wraps is a stamp that lies about a long build.
		values["H"] = leftPad(int64(elapsed.Hours()), 2)
		values["M"] = pad2(int64(elapsed.Minutes()) % 60)
		values["S"] = pad2(int64(elapsed.Seconds()) % 60)
	}
	values["3f"] = leftPad(sub/10_000, 3)
	values["6f"] = leftPad(sub/10, 6)
	values["9f"] = leftPad(sub*100, 9)
	values["%"] = "%"

	var out strings.Builder
	i := 0
	for i < len(format) {
		c := format[i]
		if c != '%' {
			out.WriteByte(c)
			i++
			continue
		}
		if i+1 >= len(format) {
			return "", fmt.Errorf("-TimestampFormat ends with a bare '%%'.")
		}
		key := string(format[i+1])
		width := 2
		if (key == "3" || key == "6" || key == "9") && i+2 < len(format) && format[i+2] == 'f' {
			key += "f"
			width = 3
		}
		value, ok := values[key]
		if !ok {
			return "", fmt.Errorf("-TimestampFormat carries '%%%s', which is not a specifier this tool renders "+
				"in %s mode. The set here is: %%3f %%6f %%9f %%%% %s, each written with a leading percent sign.",
				key, mode, knownSpecifierNames(mode))
		}
		out.WriteString(value)
		i += width
	}
	return out.String(), nil
}

func knownSpecifierNames(mode string) string {
	if mode == "Wall" {
		return "H M S Y m d z Z"
	}
	return "H M S"
}

// wallReading is one wall-clock reading, with the pieces the stamp columns
// render. It exists so a Replay can render a record's OWN reading rather than
// this machine's clock.
type wallReading struct {
	Year   int
	Month  int
	Day    int
	Hour   int
	Minute int
	Second int
	Ticks  int64 // 100ns units within the second
	Offset int   // seconds east of UTC
	Zone   string

	// EpochSeconds is the reading as Unix seconds, for the epoch column.
	EpochSeconds int64
}

// SubsecondTicks is the reading's own fraction of a second, in 100ns ticks.
func (w wallReading) SubsecondTicks() int64 { return w.Ticks }

// OffsetParts spells the UTC offset the way %z renders it.
func (w wallReading) OffsetParts() (string, int, int) {
	off := w.Offset
	sign := "+"
	if off < 0 {
		sign = "-"
		off = -off
	}
	return sign, off / 3600, (off % 3600) / 60
}

func elapsedTicks(d dur) int64 {
	// A tick is 100ns, ten million to the second.
	return int64(d) / 100
}

func pad2(n int64) string { return leftPad(n, 2) }

func pad4(n int64) string { return leftPad(n, 4) }

func leftPad(n int64, width int) string {
	s := strconv.FormatInt(n, 10)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// formatStampColumn is one column's text. Every column is a pure function of
// the clock readings it is given, so a line's whole prefix can be rebuilt
// from a record without re-running anything.
func formatStampColumn(column, format string, wall wallReading, elapsed, delta dur) (string, error) {
	switch column {
	case "delta":
		// ⛔ The spelling here is the one -TimestampMode Delta already
		// produced, character for character.
		sub := elapsedTicks(delta) % 10_000_000
		return "+" + strconv.FormatInt(int64(delta.Seconds()), 10) + "." + leftPad(sub/10_000, 3), nil
	case "epoch":
		return strconv.FormatInt(wall.EpochSeconds, 10), nil
	case "iso":
		return formatStrftimeStamp("%Y-%m-%dT%H:%M:%S.%3f%z", wall, elapsed, "Wall")
	case "wall":
		f := format
		if f == "" {
			f = stampDefaultFormat("Wall")
		}
		return formatStrftimeStamp(f, wall, elapsed, "Wall")
	default:
		f := format
		if f == "" {
			f = stampDefaultFormat("Relative")
		}
		return formatStrftimeStamp(f, wall, elapsed, "Relative")
	}
}
