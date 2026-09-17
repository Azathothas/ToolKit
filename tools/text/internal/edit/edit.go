// Package edit is the whole of `text`: it reads a file as bytes, changes one
// thing about it, and writes it back atomically.
//
// ⛔ BYTES, NOT STRINGS, END TO END. A file this tool is asked to change may not
// be valid UTF-8, and re-encoding it would be a silent rewrite of every byte the
// decoder could not read. Nothing here decodes.
//
// SPDX-License-Identifier: 0BSD
package edit

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ErrUsage asks the caller to print the usage text beside the message.
var ErrUsage = errors.New("usage")

// Schema versions the structured answer.
const Schema = "text-edit/1"

// Report is what one call did, or would have done.
type Report struct {
	Schema string `json:"schema"`
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	// Matches is how many places the operation found. ⚠ For `write` it is 1,
	// because the whole file is the one place.
	Matches int    `json:"matches"`
	Before  int    `json:"bytes_before"`
	After   int    `json:"bytes_after"`
	EOL     string `json:"eol"`
	Changed bool   `json:"changed"`
	DryRun  bool   `json:"dry_run,omitempty"`
	// Lines are the 1-based line numbers the operation touched, at most
	// maxReportedLines of them, so a caller can see WHERE without reading the
	// file back.
	Lines []int `json:"lines,omitempty"`
	// LinesTruncated says Lines is shorter than Matches because of that cap,
	// so a caller comparing the two is not left deciding which one lied.
	LinesTruncated bool `json:"lines_truncated,omitempty"`
}

type options struct {
	mode string
	path string

	// the payload, and which channel gave it
	payload    []byte
	haveLoad   bool
	loadSource string

	find         string
	haveFind     bool
	useRegex     bool
	line         int
	haveLine     bool
	insertAfter  int
	haveAfter    bool
	insertBefore int
	haveBefore   bool
	deleteFrom   int
	deleteTo     int
	haveDelete   bool
	betweenA     string
	betweenB     string
	haveBetween  bool

	expect     int
	haveExpect bool
	count      bool
	dryRun     bool
	asJSON     bool
	eol        string
}

// Run parses one invocation and carries it out.
func Run(args []string, out, errOut io.Writer, stdin io.Reader) (int, error) {
	o, err := parse(args)
	if err != nil {
		return 2, err
	}
	if err := o.loadPayload(stdin); err != nil {
		return 2, err
	}
	if err := o.validate(); err != nil {
		return 2, err
	}
	o.noteLiteralEscapes(errOut)

	before, err := readFile(o.path)
	if err != nil {
		return 2, err
	}
	eol := detectEOL(before, o.eol)

	after, matches, lines, err := o.apply(before, eol)
	if err != nil {
		return 1, err
	}

	rep := Report{
		Schema: Schema, Path: o.path, Mode: o.mode, Matches: matches,
		Before: len(before), After: len(after), EOL: eol,
		Changed: !bytes.Equal(before, after), DryRun: o.dryRun || o.count,
		Lines: lines, LinesTruncated: len(lines) > 0 && len(lines) < matches,
	}

	// ⛔ THE COUNT IS CHECKED BEFORE ANYTHING IS WRITTEN. A substitution that
	// matched a different number of times than the caller believed is a
	// different edit from the one they asked for, and the file is left alone.
	if o.haveExpect && matches != o.expect {
		emit(out, o.asJSON, "refused", rep)
		return 1, fmt.Errorf("%s: --expect %d and this matches %d times. Nothing was written",
			o.path, o.expect, matches)
	}
	if o.count || o.dryRun {
		// ⛔ "changed" IS WHAT HAPPENED, NOT WHAT WOULD HAVE. A dry run that
		// reports changed:true is telling a caller the file moved when nothing
		// did, and a caller reading the field rather than the prose acts on it.
		// What WOULD change is already in matches and in the byte counts.
		rep.Changed = false
		emit(out, o.asJSON, "would change", rep)
		return 0, nil
	}
	if err := writeAtomic(o.path, after); err != nil {
		return 2, err
	}
	verb := "wrote"
	if !rep.Changed {
		verb = "left unchanged"
	}
	emit(out, o.asJSON, verb, rep)
	return 0, nil
}

// emit reports what happened. ⛔ THE VERB IS PASSED IN RATHER THAN INFERRED.
// It was inferred, and a refused edit printed "wrote PATH: 2 match(es)" on the
// line above "Nothing was written", because the report knew the bytes WOULD have
// changed and not whether they DID. A tool that reports an action it did not take
// is the class this repository refuses, and it took one run to commit it.
func emit(out io.Writer, asJSON bool, verb string, r Report) {
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(r)
		return
	}
	// A TRUNCATED LIST SAYS SO IN THE PROSE TOO, and names where it starts,
	// because the caller who reads the human line is the one who cannot see
	// lines_truncated.
	where := ""
	if r.LinesTruncated {
		where = fmt.Sprintf(", first %d at line %d", len(r.Lines), r.Lines[0])
	}
	fmt.Fprintf(out, "%s %s: %d match(es)%s, %d -> %d bytes, %s endings\n",
		verb, r.Path, r.Matches, where, r.Before, r.After, r.EOL)
}

func parse(args []string) (*options, error) {
	o := &options{eol: "keep", deleteTo: -1}
	if len(args) < 2 {
		return nil, fmt.Errorf("%w: a mode and a path are both needed", ErrUsage)
	}
	o.mode, o.path = args[0], args[1]
	switch o.mode {
	case "write", "append", "edit":
	default:
		return nil, fmt.Errorf("%w: %q is not a mode", ErrUsage, o.mode)
	}

	rest := args[2:]
	need := func(i int, flag string) (string, error) {
		if i+1 >= len(rest) {
			return "", fmt.Errorf("%s needs a value", flag)
		}
		return rest[i+1], nil
	}
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		var v string
		var err error
		switch a {
		case "--text", "--b64", "--from", "--replace", "--replace-b64", "--replace-from",
			"--line", "--insert-after", "--insert-before", "--delete", "--between",
			"--expect", "--eol":
			if v, err = need(i, a); err != nil {
				return nil, err
			}
			i++
		}
		switch a {
		case "--text":
			o.payload, o.haveLoad, o.loadSource = []byte(v), true, "--text"
		case "--b64":
			// ⚠ WHITESPACE IS STRIPPED FIRST, AND NOT THE WHITESPACE YOU EXPECT.
			// Measured 2026-09-17: Go base64 IGNORES \r and \n on its own, so a
			// value a shell wrapped across LINES already decodes. What it refuses
			// is a SPACE or a TAB, with `illegal base64 data at input byte 8`.
			// ⛔ The first version of this comment blamed the newline, and repo
			// mutate called the row THEATRE: removing the strip left the newline
			// case passing, because the newline was never the problem.
			raw, decErr := decodeB64(v, "--b64")
			if decErr != nil {
				return nil, decErr
			}
			o.payload, o.haveLoad, o.loadSource = raw, true, "--b64"
		case "--from":
			raw, readErr := os.ReadFile(v)
			if readErr != nil {
				return nil, readErr
			}
			o.payload, o.haveLoad, o.loadSource = raw, true, "--from"
		case "--replace":
			o.find, o.haveFind = v, true
		// ⛔ THE SEARCH NEEDS THE SAME CHANNELS AS THE PAYLOAD, and the first
		// version of this tool gave it only the shell. Its author then tried to
		// search for a string holding a backslash-n, printf turned it into a real
		// newline, and the search matched nothing. The refusal said so, which is
		// the tool working; having no safe channel for the search was the gap.
		case "--replace-b64":
			raw, decErr := decodeB64(v, "--replace-b64")
			if decErr != nil {
				return nil, decErr
			}
			o.find, o.haveFind = string(raw), true
		case "--replace-from":
			raw, readErr := os.ReadFile(v)
			if readErr != nil {
				return nil, readErr
			}
			o.find, o.haveFind = string(raw), true
		case "--regex":
			o.useRegex = true
		case "--line":
			if o.line, err = atoi(v, a); err != nil {
				return nil, err
			}
			o.haveLine = true
		case "--insert-after":
			if o.insertAfter, err = atoi(v, a); err != nil {
				return nil, err
			}
			o.haveAfter = true
		case "--insert-before":
			if o.insertBefore, err = atoi(v, a); err != nil {
				return nil, err
			}
			o.haveBefore = true
		case "--delete":
			from, to, delErr := parseRange(v)
			if delErr != nil {
				return nil, delErr
			}
			o.deleteFrom, o.deleteTo, o.haveDelete = from, to, true
		case "--between":
			a2, b2, ok := strings.Cut(v, ",")
			if !ok || a2 == "" || b2 == "" {
				return nil, errors.New("--between takes two patterns separated by a comma")
			}
			o.betweenA, o.betweenB, o.haveBetween = a2, b2, true
		case "--expect":
			if o.expect, err = atoi(v, a); err != nil {
				return nil, err
			}
			o.haveExpect = true
		case "--eol":
			switch v {
			case "keep", "lf", "crlf":
				o.eol = v
			default:
				return nil, fmt.Errorf("--eol takes keep, lf or crlf, and %q was given", v)
			}
		case "--count":
			o.count = true
		case "--dry-run":
			o.dryRun = true
		case "--json":
			o.asJSON = true
		default:
			return nil, fmt.Errorf("%w: %q is not a flag this takes", ErrUsage, a)
		}
	}
	return o, nil
}

func atoi(v, flag string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s takes a number and %q was given", flag, v)
	}
	return n, nil
}

func parseRange(v string) (int, int, error) {
	from, to, ok := strings.Cut(v, ",")
	a, err := strconv.Atoi(strings.TrimSpace(from))
	if err != nil {
		return 0, 0, fmt.Errorf("--delete takes N or N,M and %q was given", v)
	}
	if !ok {
		return a, a, nil
	}
	b, err := strconv.Atoi(strings.TrimSpace(to))
	if err != nil {
		return 0, 0, fmt.Errorf("--delete takes N or N,M and %q was given", v)
	}
	if b < a {
		return 0, 0, fmt.Errorf("--delete %s ends before it starts", v)
	}
	return a, b, nil
}

// loadPayload reads stdin when no channel was named and stdin is not a terminal.
func (o *options) loadPayload(stdin io.Reader) error {
	if o.haveLoad || stdin == nil {
		return nil
	}
	if f, ok := stdin.(*os.File); ok {
		st, err := f.Stat()
		if err != nil || st.Mode()&os.ModeCharDevice != 0 {
			// A terminal. Reading it would hang waiting for somebody to type.
			return nil
		}
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	if len(raw) > 0 {
		o.payload, o.haveLoad, o.loadSource = raw, true, "stdin"
	}
	return nil
}

func (o *options) operations() []string {
	var on []string
	for _, p := range []struct {
		name string
		set  bool
	}{
		{"--replace", o.haveFind}, {"--line", o.haveLine},
		{"--insert-after", o.haveAfter}, {"--insert-before", o.haveBefore},
		{"--delete", o.haveDelete}, {"--between", o.haveBetween},
	} {
		if p.set {
			on = append(on, p.name)
		}
	}
	return on
}

func (o *options) validate() error {
	ops := o.operations()
	switch o.mode {
	case "write", "append":
		if len(ops) > 0 {
			return fmt.Errorf("%s takes no operation, and %s was given. Use `edit` for that", o.mode, ops[0])
		}
		if !o.haveLoad {
			return fmt.Errorf("%s needs a payload: --text, --b64, --from, or stdin", o.mode)
		}
	case "edit":
		if len(ops) == 0 {
			return fmt.Errorf("%w: edit needs one operation", ErrUsage)
		}
		if len(ops) > 1 {
			return fmt.Errorf("edit takes one operation and %d were given: %s", len(ops), strings.Join(ops, ", "))
		}
		// ⛔ A SUBSTITUTION WITHOUT --expect IS THE DEFECT THIS TOOL REMOVES.
		// Every other operation names its own place; a search does not, and a
		// search that silently matched nothing is what a caller never notices.
		if o.haveFind && !o.haveExpect && !o.count {
			return errors.New("--replace needs --expect N, the number of matches you believe are there. " +
				"Use --count first if you do not know. A substitution that matches a different number of " +
				"times is a different edit from the one you asked for")
		}
		if o.haveDelete && o.haveLoad {
			return errors.New("--delete takes no payload")
		}
		if !o.haveDelete && !o.haveLoad && !o.count {
			return errors.New("this operation needs a payload: --text, --b64, --from, or stdin")
		}
	}
	if o.useRegex && !o.haveFind && !o.haveBetween {
		return errors.New("--regex applies to --replace and --between")
	}
	return nil
}

func readFile(p string) ([]byte, error) {
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return raw, err
}

// detectEOL answers what this file's line ending is, or what was asked for.
//
// ⚠ A FILE WITH NO NEWLINE AT ALL, OR A NEW ONE, TAKES LF. Guessing CRLF for an
// empty file would put a Windows ending in a repository whose .gitattributes
// decides that, and this tool does not second-guess git.
func detectEOL(body []byte, want string) string {
	switch want {
	case "lf", "crlf":
		return want
	}
	if bytes.Contains(body, []byte("\r\n")) {
		return "crlf"
	}
	return "lf"
}

func toEOL(body []byte, eol string) []byte {
	flat := bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	if eol == "crlf" {
		return bytes.ReplaceAll(flat, []byte("\n"), []byte("\r\n"))
	}
	return flat
}

// splitLines keeps each line's own terminator, so a file whose last line has no
// newline round-trips unchanged.
func splitLines(body []byte) [][]byte {
	if len(body) == 0 {
		return nil
	}
	var out [][]byte
	start := 0
	for i := 0; i < len(body); i++ {
		if body[i] == '\n' {
			out = append(out, body[start:i+1])
			start = i + 1
		}
	}
	if start < len(body) {
		out = append(out, body[start:])
	}
	return out
}

func writeAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// ⭐ THE TEMPORARY FILE IS IN THE SAME DIRECTORY. A rename across volumes is
	// a copy, which loses the property that a killed process leaves the old file
	// intact rather than a truncated one.
	tmp, err := os.CreateTemp(dir, ".text-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if info, err := os.Stat(path); err == nil {
		_ = os.Chmod(name, info.Mode().Perm())
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func regexpFor(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%q is not a regular expression: %w", pattern, err)
	}
	return re, nil
}

// noteLiteralEscapes says when a --text payload carries a backslash-n or a
// backslash-t as two characters.
//
// ⚠ A NOTE, NOT A REFUSAL, AND THE DIFFERENCE MATTERS. Writing Go, JSON or a
// regular expression legitimately puts a literal backslash-n in a file, so
// refusing it would refuse correct work. But an agent reaching for --text with
// an escape in it almost always meant a newline, and this tool deliberately does
// NOT interpret escapes: interpreting them is the mangling it exists to avoid.
//
// ⛔ MEASURED ON ITS OWN AUTHOR. The first edit made with this tool used
// backslash-n in --text expecting newlines, got two characters, and matched
// nothing. The refusal said so, and this says WHY.
func (o *options) noteLiteralEscapes(errOut io.Writer) {
	if o.loadSource != "--text" || errOut == nil {
		return
	}
	if !bytes.Contains(o.payload, []byte(`\n`)) && !bytes.Contains(o.payload, []byte(`\t`)) {
		return
	}
	fmt.Fprintln(errOut, "text: note: --text carries a literal backslash-n or backslash-t. "+
		"This tool writes bytes as given and interprets no escape, because interpreting one is the "+
		"mangling it exists to avoid. For a real newline use --b64 or --from.")
}

// decodeB64 reads a base64 value, tolerating the whitespace a shell adds.
//
// ⚠ IT IS THE SPACE AND THE TAB, NOT THE NEWLINE. Measured 2026-09-17: Go
// base64 ignores \r and \n on its own, so a value wrapped across LINES already
// decodes. What it refuses is a space or a tab, with `illegal base64 data at
// input byte 8`, and a space is how a shell wraps a long argument.
//
// ⛔ ONE HOME, because the payload and the search both need it. Two copies
// made a mutation row match twice and report BROKEN, which finding 41 says is
// neither red nor green.
func decodeB64(v, flag string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(v), ""))
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64, which usually means a shell mangled it: %w", flag, err)
	}
	return raw, nil
}
