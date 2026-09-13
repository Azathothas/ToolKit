// SPDX-License-Identifier: 0BSD

package compat

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
)

// getenv is os.Getenv, named here so the options file reads like the
// parameter block it ports.
func getenv(name string) string { return envValue(name) }

func parseIntStrict(v string) (int, error) {
	t := strings.TrimSpace(v)
	if t == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, r := range t {
		if !unicode.IsDigit(r) && !(r == '-' && t[0] == '-' && len(t) > 1) {
			return 0, fmt.Errorf("%q is not a whole number", v)
		}
	}
	n, err := strconv.Atoi(t)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// console is the port of the script's output helpers.
//
// ⭐ THE CHANNELS ARE THE CONTRACT. Reports go to the report writer, which is
// where the PowerShell host put Write-Host for a child process. Notes go to
// the note writer, which is stderr, because -Action HostAddress makes the one
// value a caller assigns, and a note merged into it would be a corrupted
// answer rather than a chatty one.
type console struct {
	report io.Writer
	note   io.Writer
	color  bool
}

func (c *console) step(message string)  { c.fprint(c.report, "==> ", message, "36") }
func (c *console) ok(message string)    { c.fprint(c.report, "  * ", message, "32") }
func (c *console) warn(message string)  { c.fprint(c.report, "  ! ", message, "33") }
func (c *console) plain(message string) { c.fprint(c.report, "", message, "") }

// dim writes the quiet explanatory line, the port of the script's
// -ForegroundColor DarkGray.
func (c *console) dim(message string) { c.fprint(c.report, "", message, "90") }

func (c *console) fprint(w io.Writer, prefix, message, code string) {
	if w == nil {
		return
	}
	if c.color && code != "" {
		fmt.Fprintf(w, "\x1b[%sm%s%s\x1b[0m\n", code, prefix, message)
		return
	}
	fmt.Fprintf(w, "%s%s\n", prefix, message)
}

// noteLine writes to stderr. ⛔ IT IS NOT THE REPORT. See the channel contract
// above: one value on the report stream is the whole point of this split.
func (c *console) noteLine(message string) {
	if c.note == nil {
		return
	}
	fmt.Fprintln(c.note, message)
}

// confirmDestructive is the port of Confirm-Destructive. Destructive actions
// need -Force when nothing is interactive, and a refusal says how to proceed.
func (s *session) confirmDestructive(target, operation string) bool {
	if s.opts.Force {
		return true
	}
	if !s.interactive() {
		s.log.warn(fmt.Sprintf("%s on '%s' needs confirmation, but this session is non-interactive.", operation, target))
		s.log.warn("Re-run with -Force to proceed.")
		return false
	}
	fmt.Fprintf(s.out, "%s on '%s'? [y/N] ", operation, target)
	answer, err := readAnswer(s.in)
	if err != nil {
		return false
	}
	a := strings.ToLower(strings.TrimSpace(answer))
	return a == "y" || a == "yes"
}

func readAnswer(in io.Reader) (string, error) {
	if in == nil {
		return "", io.EOF
	}
	var b [256]byte
	n := 0
	for n < len(b) {
		one := make([]byte, 1)
		m, err := in.Read(one)
		if m > 0 {
			if one[0] == '\n' {
				return string(b[:n]), nil
			}
			b[n] = one[0]
			n++
			continue
		}
		if err != nil {
			return string(b[:n]), err
		}
	}
	return string(b[:n]), nil
}

// formatByteCount renders a size the way the script's Format-ByteCount does.
func formatByteCount(bytes int64) string {
	if bytes < 1024 {
		return strconv.FormatInt(bytes, 10) + " B"
	}
	if bytes < 1048576 {
		return strconv.FormatFloat(float64(bytes)/1024.0, 'f', 1, 64) + " KiB"
	}
	return strconv.FormatFloat(float64(bytes)/1048576.0, 'f', 1, 64) + " MiB"
}

// formatDuration renders a span the way the script's Format-Duration does:
// 59s, then 4m05s, then 1h04m, with total hours never wrapping at a day.
func formatDuration(d dur) string {
	total := int64(d.Seconds())
	switch {
	case total < 60:
		return strconv.FormatInt(total, 10) + "s"
	case total < 3600:
		return strconv.FormatInt(total/60, 10) + "m" + zeroPad2(total%60) + "s"
	default:
		return strconv.FormatInt(total/3600, 10) + "h" + zeroPad2((total%3600)/60) + "m"
	}
}

func zeroPad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}
