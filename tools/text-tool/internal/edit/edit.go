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

// Name is what this program calls itself in a message it writes, and it has one
// home so a rename cannot leave half the messages behind.
const Name = "text-tool"

// Schema versions the structured answer.
const Schema = "text-edit/2"

// FileReport is what happened to ONE named file.
type FileReport struct {
	Path string `json:"path"`
	// Matches is how many places the operation found in this file. ⚠ For
	// `write` it is 1, because the whole file is the one place.
	Matches int    `json:"matches"`
	Before  int    `json:"bytes_before"`
	After   int    `json:"bytes_after"`
	EOL     string `json:"eol"`
	Changed bool   `json:"changed"`
	// BOM says the file began with a UTF-8 byte order mark BEFORE this ran.
	BOM bool `json:"bom,omitempty"`
	// Lines are the 1-based line numbers the operation touched, at most
	// maxReportedLines of them, so a caller can see WHERE without reading the
	// file back.
	Lines []int `json:"lines,omitempty"`
	// LinesTruncated says Lines is shorter than Matches because of that cap,
	// so a caller comparing the two is not left deciding which one lied.
	LinesTruncated bool `json:"lines_truncated,omitempty"`
}

// Report is what one call did, or would have done, across every file it named.
//
// ⛔ ONE SHAPE, WHETHER ONE FILE WAS NAMED OR TWENTY. The first version emitted
// a flat object for a single file, and adding a second file would have given a
// caller two shapes to parse from one command. `files` always has an entry per
// path, in the order they were given.
type Report struct {
	Schema string `json:"schema"`
	Mode   string `json:"mode"`
	// Matches is the TOTAL across every file, which is what --expect checks.
	Matches int `json:"matches"`
	// Changed is true when ANY file's bytes moved.
	Changed bool         `json:"changed"`
	DryRun  bool         `json:"dry_run,omitempty"`
	Files   []FileReport `json:"files"`
}

type options struct {
	mode string
	// paths are every file named, in the order given. ⛔ ALL OF THEM ARE
	// CHANGED OR NONE ARE: see Run.
	paths []string

	// the payload, and which channel gave it
	payload    []byte
	haveLoad   bool
	loadSource string

	find              string
	haveFind          bool
	useRegex          bool
	line              int
	haveLine          bool
	insertAfter       int
	haveAfter         bool
	insertBefore      int
	haveBefore        bool
	deleteFrom        int
	deleteTo          int
	haveDelete        bool
	insertFind        string
	haveInsertFind    bool
	insertBeforeMatch bool
	betweenA          string
	betweenB          string
	haveBetween       bool

	expect         int
	haveExpect     bool
	allowUnmatched bool
	count          bool
	dryRun         bool
	asJSON         bool
	eol            string
	bom            string
}

// Run parses one invocation and carries it out.
//
// ⛔ EVERY FILE IS READ AND CHANGED IN MEMORY BEFORE ANY OF THEM IS WRITTEN.
// A loop that wrote as it went would leave the first three files changed and the
// fourth refused, which is a half-applied edit nobody asked for and no single
// command can undo. The count check, the unmatched check and every operation's
// own refusal all run over the whole set first.
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

	rep := Report{Schema: Schema, Mode: o.mode, DryRun: o.dryRun || o.count}
	staged := make([][]byte, len(o.paths))
	var unmatched []string

	for i, p := range o.paths {
		before, err := readFile(p)
		if err != nil {
			return 2, err
		}
		eol := detectEOL(before, o.eol)
		after, matches, lines, err := o.apply(before, eol)
		if err != nil {
			return 1, fmt.Errorf("%s: %w", p, err)
		}
		staged[i] = after
		changed := !bytes.Equal(before, after)
		rep.Matches += matches
		rep.Changed = rep.Changed || changed
		rep.Files = append(rep.Files, FileReport{
			Path: p, Matches: matches, Before: len(before), After: len(after),
			EOL: eol, Changed: changed, BOM: bytes.HasPrefix(before, utf8BOM),
			Lines: lines, LinesTruncated: len(lines) > 0 && len(lines) < matches,
		})
		if matches == 0 && (o.haveFind || o.haveInsertFind) {
			unmatched = append(unmatched, p)
		}
	}

	// ⛔ THE COUNT IS CHECKED BEFORE ANYTHING IS WRITTEN. A substitution that
	// matched a different number of times than the caller believed is a
	// different edit from the one they asked for, and the files are left alone.
	// ⚠ ACROSS THE WHOLE SET, because that is the number a caller can state
	// without opening each file; the per-file counts are in the report.
	if o.haveExpect && rep.Matches != o.expect {
		emit(out, o.asJSON, "refused", rep)
		return 1, fmt.Errorf("--expect %d and this matches %d times across %d file(s). Nothing was written",
			o.expect, rep.Matches, len(o.paths))
	}

	// ⛔ A NAMED FILE THAT MATCHED NOTHING IS A REFUSAL, not a quiet zero. Give
	// five paths to one substitution and the one that has drifted, or whose name
	// has a typo, is the whole reason to read the report - and nobody reads a
	// report that says success. --allow-unmatched is for the caller who means it.
	//
	// ⚠ IT COMES AFTER --expect, so a caller who stated a number gets told about
	// the number. Both fire on one file that matched nothing, and "--expect 1 and
	// this matches 0 times" is the more precise of the two answers.
	if len(unmatched) > 0 && !o.allowUnmatched && !o.count {
		emit(out, o.asJSON, "refused", rep)
		return 1, fmt.Errorf("%d of %d file(s) matched nothing: %s. Nothing was written. "+
			"Pass --allow-unmatched if that is what you meant",
			len(unmatched), len(o.paths), strings.Join(unmatched, ", "))
	}
	if o.count || o.dryRun {
		// ⛔ "changed" IS WHAT HAPPENED, NOT WHAT WOULD HAVE. A dry run that
		// reports changed:true is telling a caller the file moved when nothing
		// did, and a caller reading the field rather than the prose acts on it.
		// What WOULD change is already in matches and in the byte counts.
		rep.Changed = false
		for i := range rep.Files {
			rep.Files[i].Changed = false
		}
		emit(out, o.asJSON, "would change", rep)
		return 0, nil
	}
	for i, p := range o.paths {
		if err := writeAtomic(p, staged[i]); err != nil {
			return 2, err
		}
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
//
// ⚠ ONE LINE PER FILE, AND A TOTAL ONLY WHEN THERE IS MORE THAN ONE. A total
// over a single file is the same number twice, which reads as two findings.
func emit(out io.Writer, asJSON bool, verb string, r Report) {
	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(r)
		return
	}
	for _, f := range r.Files {
		// A TRUNCATED LIST SAYS SO IN THE PROSE TOO, and names where it starts,
		// because the caller who reads the human line is the one who cannot see
		// lines_truncated.
		where := ""
		if f.LinesTruncated {
			where = fmt.Sprintf(", first %d at line %d", len(f.Lines), f.Lines[0])
		}
		fmt.Fprintf(out, "%s %s: %d match(es)%s, %d -> %d bytes, %s endings\n",
			verb, f.Path, f.Matches, where, f.Before, f.After, f.EOL)
	}
	if len(r.Files) > 1 {
		fmt.Fprintf(out, "%s %d file(s): %d match(es) in total\n", verb, len(r.Files), r.Matches)
	}
}

// parse turns one invocation into options.
//
// ⛔ EVERY LEADING ARGUMENT AFTER THE MODE IS A PATH, up to the first one that
// starts with a dash. That rule is what lets one command name twenty files
// without a separator to get wrong, and it is why no path may begin with a dash:
// a file called `-x` is indistinguishable from a flag, and guessing would be
// worse than refusing.
func parse(args []string) (*options, error) {
	o := &options{eol: "keep", bom: "keep", deleteTo: -1}
	if len(args) < 1 {
		return nil, fmt.Errorf("%w: a mode is needed", ErrUsage)
	}
	o.mode = args[0]
	switch o.mode {
	case "write", "append", "edit", "eol":
	default:
		return nil, fmt.Errorf("%w: %q is not a mode", ErrUsage, o.mode)
	}

	rest := args[1:]
	for len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		o.paths = append(o.paths, rest[0])
		rest = rest[1:]
	}
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
			"--line", "--insert-after", "--insert-before", "--delete",
			"--after", "--before",
			"--expect", "--eol", "--to", "--bom", "--files-from":
			if v, err = need(i, a); err != nil {
				return nil, err
			}
			i++
		}
		switch a {
		case "--files-from":
			// ⭐ A LIST OF PATHS IN A FILE, one per line, for the caller who has
			// more of them than a command line will hold. Blank lines and lines
			// beginning with # are skipped, so the output of a search can be
			// commented and handed straight back.
			named, readErr := os.ReadFile(v)
			if readErr != nil {
				return nil, readErr
			}
			for _, ln := range strings.Split(string(named), "\n") {
				ln = strings.TrimSpace(strings.TrimSuffix(ln, "\r"))
				if ln == "" || strings.HasPrefix(ln, "#") {
					continue
				}
				o.paths = append(o.paths, ln)
			}
		case "--after":
			o.insertFind, o.haveInsertFind, o.insertBeforeMatch = v, true, false
		case "--before":
			o.insertFind, o.haveInsertFind, o.insertBeforeMatch = v, true, true
		case "--allow-unmatched":
			o.allowUnmatched = true
		case "--to":
			switch v {
			case "lf", "crlf":
				o.eol = v
			default:
				return nil, fmt.Errorf("--to takes lf or crlf, not %q", v)
			}
		case "--lf":
			o.eol = "lf"
		case "--crlf":
			o.eol = "crlf"
		case "--bom":
			switch v {
			case "keep", "strip", "add":
				o.bom = v
			default:
				return nil, fmt.Errorf("--bom takes keep, strip or add, not %q", v)
			}
		case "--between":
			// ⛔ TWO ARGUMENTS, NOT ONE SEPARATED BY A COMMA. The comma form cut
			// at the FIRST comma, so an anchor holding one - which ordinary prose
			// and most Go declarations do - silently became two different anchors
			// and matched nothing. A separator that appears in the data is not a
			// separator.
			if i+2 >= len(rest) {
				return nil, errors.New("--between takes two patterns: --between START END")
			}
			o.betweenA, o.betweenB, o.haveBetween = rest[i+1], rest[i+2], true
			i += 2
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
// takesPayload says whether this invocation could use one at all.
//
// ⛔ AN OPERATION THAT TAKES NO PAYLOAD MUST NOT READ stdin. --delete, --count
// and the eol mode all refuse one, and reading anyway HUNG this tool: the
// character-device guard in loadPayload recognises a terminal, and a PIPE
// inherited from a parent that never writes and never closes is not a terminal,
// so io.ReadAll waits for ever. Measured 2026-09-17, driving `edit --delete`
// from an agent harness, where it hung until the harness timed it out.
func (o *options) takesPayload() bool {
	return o.mode != "eol" && !o.haveDelete && !o.count
}

func (o *options) loadPayload(stdin io.Reader) error {
	if o.haveLoad || stdin == nil || !o.takesPayload() {
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
		{"--after", o.haveInsertFind && !o.insertBeforeMatch},
		{"--before", o.haveInsertFind && o.insertBeforeMatch},
	} {
		if p.set {
			on = append(on, p.name)
		}
	}
	return on
}

func (o *options) validate() error {
	if len(o.paths) == 0 {
		return fmt.Errorf("%w: name at least one file", ErrUsage)
	}
	ops := o.operations()
	// ⛔ A LINE OR A RANGE MEANS ONE FILE. --line 12 over four files names four
	// different places that happen to share a number, and a caller who meant
	// that can say it four times. --replace and eol are the operations whose
	// meaning does not change with the file.
	if len(o.paths) > 1 {
		for _, f := range []struct {
			name string
			set  bool
		}{
			{"--line", o.haveLine}, {"--insert-after", o.haveAfter},
			{"--insert-before", o.haveBefore}, {"--delete", o.haveDelete},
		} {
			if f.set {
				return fmt.Errorf("%s names a place in ONE file and %d were given. "+
					"Run it once per file, or use --replace, which names the same text in each",
					f.name, len(o.paths))
			}
		}
	}
	switch o.mode {
	case "eol":
		// ⭐ THIS MODE IS dos2unix AND unix2dos, and it takes no operation and no
		// payload because converting a file's endings is not an edit to its text.
		if len(ops) > 0 {
			return fmt.Errorf("eol takes no operation, and %s was given. Use `edit` for that", ops[0])
		}
		if o.haveLoad {
			return errors.New("eol takes no payload: it converts the file that is there")
		}
		if o.eol == "keep" && o.bom == "keep" {
			return fmt.Errorf("%w: eol needs --to lf, --to crlf, --lf, --crlf or --bom", ErrUsage)
		}
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
		if (o.haveFind || o.haveInsertFind) && !o.haveExpect && !o.count {
			return errors.New("--replace, --after and --before need --expect N, the number of matches you believe are there. " +
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
	if o.bom != "keep" && o.mode != "eol" {
		return fmt.Errorf("--bom is for `eol`, and %s was given", o.mode)
	}
	if o.useRegex && !o.haveFind && !o.haveBetween && !o.haveInsertFind {
		return errors.New("--regex applies to --replace, --between, --after and --before")
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
	fmt.Fprintln(errOut, Name+": note: --text carries a literal backslash-n or backslash-t. "+
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
