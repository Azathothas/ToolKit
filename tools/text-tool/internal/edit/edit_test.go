// edit_test.go - the payloads and the files that break the alternatives.
//
// ⛔ EVERY FIXTURE HERE IS A REAL FAILURE. The hostile payload is the one
// docs/conventions/shell.md section 1 measured executing inside a QUOTED heredoc.
// The CRLF file, the file with no trailing newline and the file that is not valid
// UTF-8 are the three shapes a naive tool silently rewrites.
//
// SPDX-License-Identifier: 0BSD

package edit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hostile is the payload that a quoted heredoc executed: backticks, a dollar, a
// double quote, an apostrophe and a backslash escape.
const hostile = "a line with `backticks`, $DOLLARS, \"quotes\", 'apostrophes' and D:\\tools\\x\\d+"

func run(t *testing.T, args ...string) (int, string, error) {
	t.Helper()
	var out bytes.Buffer
	code, err := Run(args, &out, &out, nil)
	return code, out.String(), err
}

func write(t *testing.T, dir, name string, body []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAHostilePayloadRoundTripsExactly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hard.txt")
	code, _, err := run(t, "write", p, "--text", hostile)
	if code != 0 || err != nil {
		t.Fatalf("write exited %d: %v", code, err)
	}
	if got := string(read(t, p)); got != hostile {
		t.Fatalf("the payload did not survive:\n want %q\n  got %q", hostile, got)
	}
}

// TestASubstitutionThatMatchesADifferentNumberOfTimesIsRefused is the whole
// point of the tool.
//
// ⛔ A SILENT NO-OP THAT REPORTS SUCCESS is the failure this removes. It has
// happened twice in one session to the author of this file, using a helper whose
// replace returned the original string unchanged when nothing matched.
func TestASubstitutionThatMatchesADifferentNumberOfTimesIsRefused(t *testing.T) {
	dir := t.TempDir()
	body := []byte("one two one\n")

	for _, c := range []struct {
		name   string
		find   string
		expect string
		code   int
		says   string
	}{
		{"it matches none and one was expected", "three", "1", 1, "matches 0 times"},
		{"it matches two and one was expected", "one", "1", 1, "matches 2 times"},
		{"it matches two and two were expected", "one", "2", 0, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := write(t, dir, strings.ReplaceAll(c.name, " ", "-")+".txt", body)
			code, _, err := run(t, "edit", p, "--replace", c.find, "--text", "X", "--expect", c.expect)
			if code != c.code {
				t.Fatalf("exited %d, want %d: %v", code, c.code, err)
			}
			if c.code != 0 {
				if err == nil || !strings.Contains(err.Error(), c.says) {
					t.Fatalf("the refusal does not say %q: %v", c.says, err)
				}
				// ⛔ AND THE FILE IS UNTOUCHED. A refusal that had already
				// written would be worse than no refusal.
				if got := read(t, p); !bytes.Equal(got, body) {
					t.Fatalf("a refused edit changed the file: %q", got)
				}
			}
		})
	}
}

func TestASubstitutionWithoutExpectIsRefused(t *testing.T) {
	p := write(t, t.TempDir(), "x.txt", []byte("hello\n"))
	code, _, err := run(t, "edit", p, "--replace", "hello", "--text", "bye")
	if code != 2 || err == nil || !strings.Contains(err.Error(), "--expect") {
		t.Fatalf("exited %d with %v, and a search with no expected count is what this tool refuses", code, err)
	}
}

func TestTheFilesShapeSurvives(t *testing.T) {
	dir := t.TempDir()

	t.Run("CRLF endings are kept, and a new line takes them", func(t *testing.T) {
		p := write(t, dir, "crlf.txt", []byte("alpha\r\nbeta\r\ngamma\r\n"))
		if code, _, err := run(t, "edit", p, "--line", "2", "--text", "BETA"); code != 0 {
			t.Fatalf("exited %d: %v", code, err)
		}
		if got := string(read(t, p)); got != "alpha\r\nBETA\r\ngamma\r\n" {
			t.Fatalf("the endings moved: %q", got)
		}
	})

	t.Run("a file with no trailing newline gains none", func(t *testing.T) {
		p := write(t, dir, "nonl.txt", []byte("no trailing newline"))
		if code, _, err := run(t, "edit", p, "--replace", "newline", "--text", "NEWLINE", "--expect", "1"); code != 0 {
			t.Fatalf("exited %d: %v", code, err)
		}
		if got := read(t, p); got[len(got)-1] == '\n' {
			t.Fatalf("a newline was added: %q", got)
		}
	})

	t.Run("bytes that are not UTF-8 survive", func(t *testing.T) {
		body := []byte("valid \xff\xfe invalid \x00 nul\n")
		p := write(t, dir, "bin.txt", body)
		if code, _, err := run(t, "edit", p, "--replace", "invalid", "--text", "INVALID", "--expect", "1"); code != 0 {
			t.Fatalf("exited %d: %v", code, err)
		}
		got := read(t, p)
		for _, b := range []byte{0xff, 0xfe, 0x00} {
			if !bytes.Contains(got, []byte{b}) {
				t.Errorf("the byte %#x did not survive: %q", b, got)
			}
		}
	})
}

func TestTheLineOperations(t *testing.T) {
	dir := t.TempDir()
	const body = "one\ntwo\nthree\n"

	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"replace a line", []string{"--line", "2", "--text", "TWO"}, "one\nTWO\nthree\n"},
		{"insert after a line", []string{"--insert-after", "1", "--text", "NEW"}, "one\nNEW\ntwo\nthree\n"},
		{"insert before the first line", []string{"--insert-after", "0", "--text", "NEW"}, "NEW\none\ntwo\nthree\n"},
		{"insert before a line", []string{"--insert-before", "3", "--text", "NEW"}, "one\ntwo\nNEW\nthree\n"},
		{"delete a line", []string{"--delete", "2"}, "one\nthree\n"},
		{"delete a range", []string{"--delete", "1,2"}, "three\n"},
		{"replace between two markers", []string{"--between", "one", "three", "--text", "ALL", "--expect", "1"}, "ALL\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := write(t, dir, strings.ReplaceAll(c.name, " ", "-")+".txt", []byte(body))
			args := append([]string{"edit", p}, c.args...)
			if code, _, err := run(t, args...); code != 0 {
				t.Fatalf("exited %d: %v", code, err)
			}
			if got := string(read(t, p)); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestALineNumberOutsideTheFileIsRefused holds the other half of every line
// operation: a number nobody checked is an edit somewhere nobody looked.
func TestALineNumberOutsideTheFileIsRefused(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"--line", "9", "--text", "X"},
		{"--line", "0", "--text", "X"},
		{"--insert-after", "9", "--text", "X"},
		{"--delete", "9"},
		{"--delete", "1,9"},
	} {
		p := write(t, dir, "f"+args[1]+args[0][2:3]+".txt", []byte("one\ntwo\n"))
		code, _, err := run(t, append([]string{"edit", p}, args...)...)
		if code != 1 || err == nil {
			t.Errorf("%v exited %d with %v, and a line outside the file is a refusal", args, code, err)
		}
	}
}

func TestACountChangesNothingAndSaysSo(t *testing.T) {
	body := []byte("a a a\n")
	p := write(t, t.TempDir(), "c.txt", body)
	code, out, err := run(t, "edit", p, "--replace", "a", "--count", "--json")
	if code != 0 || err != nil {
		t.Fatalf("exited %d: %v", code, err)
	}
	if !strings.Contains(out, `"matches": 3`) {
		t.Errorf("the count is not 3: %s", out)
	}
	// ⛔ "changed" IS WHAT HAPPENED. A dry run reporting changed:true tells a
	// caller reading the field rather than the prose that the file moved.
	if !strings.Contains(out, `"changed": false`) {
		t.Errorf("a count claimed the file changed: %s", out)
	}
	if got := read(t, p); !bytes.Equal(got, body) {
		t.Errorf("a count wrote to the file: %q", got)
	}
}

func TestAWriteIsAtomicAndKeepsTheMode(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "m.txt", []byte("old\n"))
	if err := os.Chmod(p, 0o600); err != nil {
		t.Skipf("this host does not carry file modes: %v", err)
	}
	before, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if code, _, err := run(t, "write", p, "--text", "new\n"); code != 0 {
		t.Fatalf("exited %d: %v", code, err)
	}
	after, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode().Perm() != after.Mode().Perm() {
		t.Errorf("the mode moved from %v to %v", before.Mode().Perm(), after.Mode().Perm())
	}
	// ⛔ AND NO TEMPORARY FILE IS LEFT. A killed process may leave one; a
	// successful call never does.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".text-") {
			t.Errorf("a temporary file was left behind: %s", e.Name())
		}
	}
}

func TestTwoOperationsAtOnceAreRefused(t *testing.T) {
	p := write(t, t.TempDir(), "two.txt", []byte("one\n"))
	code, _, err := run(t, "edit", p, "--line", "1", "--delete", "1", "--text", "X")
	if code != 2 || err == nil || !strings.Contains(err.Error(), "one operation") {
		t.Fatalf("exited %d with %v, and two operations in one call is ambiguous", code, err)
	}
}

func TestBase64WrappedByAShellIsStillRead(t *testing.T) {
	p := filepath.Join(t.TempDir(), "w.txt")
	// ⚠ A LONG BASE64 VALUE ARRIVES WRAPPED. The strict decoder rejects the
	// whole value over one newline, which reads as "your data is corrupt".
	code, _, err := run(t, "write", p, "--b64", "aGVsbG8g d29ybGQ=")
	if code != 0 || err != nil {
		t.Fatalf("exited %d: %v", code, err)
	}
	if got := string(read(t, p)); got != "hello world" {
		t.Fatalf("got %q", got)
	}
}

// TestALineListShorterThanTheCountSaysWhy holds the report honest when the cap
// bites.
//
// ⛔ THE TWO HALVES USED TO DISAGREE WITH NOTHING SAYING WHY. Deleting 21
// lines answered `matches: 21` beside a list of 20 line numbers, and the reader
// had to guess which number was wrong. It was neither: the list is capped. This
// was found driving the tool on a real edit the day after it shipped, and it is
// the same class as the refused edit that printed "wrote" - a report whose shape
// misleads is a defect even when every number in it is correct.
func TestALineListShorterThanTheCountSaysWhy(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 1; i <= 40; i++ {
		b.WriteString("line\n")
	}

	// 21 lines deleted, and the list holds maxReportedLines of them.
	p := write(t, dir, "many.txt", []byte(b.String()))
	code, out, err := run(t, "edit", p, "--delete", "5,25", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %v", code, err)
	}
	if !strings.Contains(out, "\"lines_truncated\": true") {
		t.Fatalf("21 matches and a capped list, and the report does not say so: %s", out)
	}

	// ⭐ AND IT DOES NOT CLAIM TRUNCATION WHEN THE LIST IS WHOLE. A field that
	// is always true is one nobody can read anything from.
	q := write(t, dir, "few.txt", []byte(b.String()))
	code, out, err = run(t, "edit", q, "--delete", "5,9", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %v", code, err)
	}
	if strings.Contains(out, "\"lines_truncated\"") {
		t.Fatalf("5 matches, 5 listed, and it still reports a truncation: %s", out)
	}

	// ⚠ A write NAMES NO LINES AT ALL, and 0 listed against 1 match must not
	// read as a cap. The guard for that is the one this field is derived through.
	w := write(t, dir, "w.txt", []byte("x\n"))
	code, out, err = run(t, "write", w, "--text", "y", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %v", code, err)
	}
	if strings.Contains(out, "\"lines_truncated\"") {
		t.Fatalf("a write lists no lines and that is not a truncation: %s", out)
	}
}

// TestTheEOLModeIsDos2unixAndUnix2dos holds the mode that replaces two programs.
//
// ⭐ THE COUNT IS ENDINGS CONVERTED, NOT LINES IN THE FILE, so a caller can tell
// "there was nothing to do" from "everything moved" without comparing byte
// totals, and --expect can name either.
//
// ⚠ A LONE CARRIAGE RETURN IS LEFT ALONE. Converting one would rewrite a CR that
// sits inside a quoted string in an otherwise LF file, which is a silent edit to
// data rather than to line structure.
func TestTheEOLModeIsDos2unixAndUnix2dos(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name  string
		body  []byte
		args  []string
		want  []byte
		count int
	}{
		{"unix2dos", []byte("a\nb\nc\n"), []string{"--crlf"}, []byte("a\r\nb\r\nc\r\n"), 3},
		{"dos2unix", []byte("a\r\nb\r\n"), []string{"--lf"}, []byte("a\nb\n"), 2},
		{"--to is the long spelling", []byte("a\nb\n"), []string{"--to", "crlf"}, []byte("a\r\nb\r\n"), 2},
		{"already converted counts nothing", []byte("a\nb\n"), []string{"--lf"}, []byte("a\nb\n"), 0},
		{"a mixed file is made whole", []byte("a\r\nb\nc\r\n"), []string{"--lf"}, []byte("a\nb\nc\n"), 2},
		{"the BOM is stripped", []byte("\xEF\xBB\xBFa\n"), []string{"--lf", "--bom", "strip"}, []byte("a\n"), 1},
		{"the BOM is added", []byte("a\n"), []string{"--lf", "--bom", "add"}, []byte("\xEF\xBB\xBFa\n"), 1},
		{"the BOM is kept by default", []byte("\xEF\xBB\xBFa\n"), []string{"--crlf"}, []byte("\xEF\xBB\xBFa\r\n"), 1},
		{"a lone CR is not a line ending", []byte("a\rb\nc\n"), []string{"--crlf"}, []byte("a\rb\r\nc\r\n"), 2},
		{"no trailing newline gains none", []byte("a\nb"), []string{"--crlf"}, []byte("a\r\nb"), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := write(t, dir, strings.ReplaceAll(c.name, " ", "-")+".txt", c.body)
			code, out, err := run(t, append([]string{"eol", p}, c.args...)...)
			if code != 0 {
				t.Fatalf("exit %d: %v", code, err)
			}
			if got := read(t, p); !bytes.Equal(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
			if want := fmt.Sprintf("%d match(es)", c.count); !strings.Contains(out, want) {
				t.Fatalf("the report does not say %q: %s", want, out)
			}
		})
	}
}

// TestTheEOLModeRefusesWhatItCannotDo keeps the mode from guessing.
func TestTheEOLModeRefusesWhatItCannotDo(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "x.txt", []byte("a\n"))
	cases := []struct {
		name string
		want string
		args []string
	}{
		{"no target at all", "needs --to", []string{"eol", p}},
		{"a payload", "takes no payload", []string{"eol", p, "--lf", "--text", "x"}},
		{"an operation", "takes no operation", []string{"eol", p, "--lf", "--line", "1"}},
		{"--bom outside eol", "--bom is for", []string{"write", p, "--text", "x", "--bom", "strip"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _, err := run(t, c.args...)
			if code == 0 {
				t.Fatal("it was accepted")
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("the refusal does not say %q: %v", c.want, err)
			}
			if got := read(t, p); !bytes.Equal(got, []byte("a\n")) {
				t.Fatalf("it was refused and the file moved: %q", got)
			}
		})
	}
}

// TestManyFilesChangeTogetherOrNotAtAll is the property that makes naming
// several files safe.
//
// ⛔ A LOOP THAT WROTE AS IT WENT would leave the first two files changed and the
// third refused, and no single command undoes that. Every file is read, applied
// and checked before any of them is written.
func TestManyFilesChangeTogetherOrNotAtAll(t *testing.T) {
	dir := t.TempDir()
	body := []byte("the quick brown fox\n")
	a := write(t, dir, "a.txt", body)
	b := write(t, dir, "b.txt", body)
	c := write(t, dir, "c.txt", body)

	// the total is what --expect names, and a wrong total writes nothing
	code, _, err := run(t, "edit", a, b, c, "--replace", "quick", "--text", "slow", "--expect", "2")
	if code != 1 {
		t.Fatalf("a wrong total was not refused: exit %d, %v", code, err)
	}
	for _, p := range []string{a, b, c} {
		if got := read(t, p); !bytes.Equal(got, body) {
			t.Fatalf("%s moved although the call was refused: %q", p, got)
		}
	}

	// the right total changes every one of them
	code, out, err := run(t, "edit", a, b, c, "--replace", "quick", "--text", "slow", "--expect", "3")
	if code != 0 {
		t.Fatalf("exit %d: %v", code, err)
	}
	for _, p := range []string{a, b, c} {
		if got := read(t, p); !bytes.Contains(got, []byte("slow")) {
			t.Fatalf("%s was not changed: %q", p, got)
		}
	}
	// ⚠ THE TOTAL LINE ONLY APPEARS FOR MORE THAN ONE FILE, because over a
	// single file it is the same number printed twice and reads as two findings.
	if !strings.Contains(out, "3 file(s): 3 match(es) in total") {
		t.Fatalf("no total line for three files: %s", out)
	}
	code, out, _ = run(t, "edit", a, "--replace", "slow", "--text", "quick", "--expect", "1")
	if code != 0 || strings.Contains(out, "in total") {
		t.Fatalf("one file printed a total line: %s", out)
	}
}

// TestAFileThatMatchedNothingRefusesTheWholeCall is the guard for the typo.
//
// ⛔ A PATH WITH A TYPO AND A FILE THAT HAS DRIFTED LOOK IDENTICAL from inside
// this tool, and both answer zero. Reporting success over them is how a
// multi-file substitution silently does less than it was asked to.
func TestAFileThatMatchedNothingRefusesTheWholeCall(t *testing.T) {
	dir := t.TempDir()
	hit := write(t, dir, "hit.txt", []byte("found here\n"))
	miss := write(t, dir, "miss.txt", []byte("nothing\n"))

	code, _, err := run(t, "edit", hit, miss, "--replace", "found", "--text", "seen", "--expect", "1")
	if code != 1 {
		t.Fatalf("the unmatched file was not refused: exit %d", code)
	}
	if err == nil || !strings.Contains(err.Error(), "matched nothing") {
		t.Fatalf("the refusal does not name it: %v", err)
	}
	if got := read(t, hit); !bytes.Equal(got, []byte("found here\n")) {
		t.Fatalf("the matching file moved anyway: %q", got)
	}

	// ⭐ AND THE CALLER WHO MEANS IT CAN SAY SO.
	code, _, err = run(t, "edit", hit, miss, "--replace", "found", "--text", "seen",
		"--expect", "1", "--allow-unmatched")
	if code != 0 {
		t.Fatalf("--allow-unmatched did not permit it: exit %d, %v", code, err)
	}
	if got := read(t, hit); !bytes.Contains(got, []byte("seen")) {
		t.Fatalf("it was allowed and nothing happened: %q", got)
	}
}

// TestAPlaceInOneFileIsRefusedForMany keeps a line number from meaning four
// different places that happen to share a number.
func TestAPlaceInOneFileIsRefusedForMany(t *testing.T) {
	dir := t.TempDir()
	a := write(t, dir, "a.txt", []byte("one\ntwo\n"))
	b := write(t, dir, "b.txt", []byte("one\ntwo\n"))
	for _, op := range [][]string{
		{"--line", "1"}, {"--insert-after", "1"}, {"--insert-before", "1"}, {"--delete", "1"},
	} {
		args := append([]string{"edit", a, b}, op...)
		if op[0] != "--delete" {
			args = append(args, "--text", "x")
		}
		code, _, err := run(t, args...)
		if code == 0 {
			t.Fatalf("%s over two files was accepted", op[0])
		}
		if err == nil || !strings.Contains(err.Error(), "ONE file") {
			t.Fatalf("%s: the refusal does not say why: %v", op[0], err)
		}
	}
}

// refusingReader records that it was read and then FAILS, rather than blocking.
//
// ⛔ THE FIRST VERSION OF THIS BLOCKED FOR EVER, which is what the real pipe
// does, and that made the mutation row HANG instead of going red. A guard whose
// failure mode is a hang is worse than one that fails: repo mutate waits on it,
// the run never finishes, and nobody learns anything. Recording the read and
// returning an error proves the same thing in microseconds.
type refusingReader struct{ read chan struct{} }

func (b *refusingReader) Read([]byte) (int, error) {
	select {
	case <-b.read:
	default:
		close(b.read)
	}
	return 0, errors.New("stdin was read and this operation takes no payload")
}

// TestAnOperationWithNoPayloadNeverReadsStdin is the hang, in a case.
//
// ⛔ MEASURED 2026-09-17: `edit --delete` HUNG when driven from an agent
// harness. loadPayload guards against a TERMINAL by looking for a character
// device, and a pipe from a parent that never writes and never closes is not
// one, so io.ReadAll waited for ever. The operations that refuse a payload must
// never reach stdin at all, which is a stronger rule than naming the shapes.
func TestAnOperationWithNoPayloadNeverReadsStdin(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		args []string
	}{
		{"delete", []string{"edit", "", "--delete", "2"}},
		{"count", []string{"edit", "", "--replace", "a", "--count"}},
		{"eol", []string{"eol", "", "--crlf"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := write(t, dir, c.name+".txt", []byte("a\nb\nc\n"))
			args := append([]string{}, c.args...)
			args[1] = p
			r := &refusingReader{read: make(chan struct{})}
			var out bytes.Buffer
			code, err := Run(args, &out, &out, r)
			if code != 0 {
				t.Fatalf("exit %d: %v", code, err)
			}
			select {
			case <-r.read:
				t.Fatal("it read stdin for an operation that takes no payload")
			default:
			}
		})
	}
}

// TestBetweenTakesTwoArgumentsBecauseAnAnchorMayHoldAComma is the defect the
// comma separator was.
//
// ⛔ `--between "a, b,END"` CUT AT THE FIRST COMMA, so the anchors became "a"
// and " b,END" and the range matched nothing. Ordinary prose and most Go
// declarations hold a comma, so the separator appeared in the data it
// separated. Found driving this tool on its own source.
func TestBetweenTakesTwoArgumentsBecauseAnAnchorMayHoldAComma(t *testing.T) {
	dir := t.TempDir()
	body := []byte("keep\n// Report is what one call did, or would have done.\nmiddle\n}\nkeep\n")
	p := write(t, dir, "x.go", body)
	code, _, err := run(t, "edit", p,
		"--between", "// Report is what one call did, or would have done.", "}",
		"--text", "REPLACED", "--expect", "1")
	if code != 0 {
		t.Fatalf("an anchor holding a comma was not matched: exit %d, %v", code, err)
	}
	want := []byte("keep\nREPLACED\nkeep\n")
	if got := read(t, p); !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestInsertingBesideAMatchKeepsTheMatch is the operation that removes the
// commonest way to damage a file with this tool.
//
// ⛔ THE DEFECT IT REPLACES IS REAL AND RECENT. Adding a block above an anchor
// with --replace means searching for the anchor and replacing it with the block
// PLUS the anchor, and forgetting the second half deletes the anchor. That
// happened three times in one session in this repository: twice it removed a Go
// function's declaration and left the body orphaned, once it removed a
// document's heading. --after and --before cannot express that mistake.
func TestInsertingBesideAMatchKeepsTheMatch(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		body string
		args []string
		want string
	}{
		{
			"before keeps the anchor",
			"intro\n## Heading\nbody\n",
			[]string{"--before", "## Heading", "--text", "added", "--expect", "1"},
			"intro\nadded\n## Heading\nbody\n",
		},
		{
			"after keeps the anchor",
			"intro\n## Heading\nbody\n",
			[]string{"--after", "## Heading", "--text", "added", "--expect", "1"},
			"intro\n## Heading\nadded\nbody\n",
		},
		{
			"every match is used and the count says how many",
			"x\nmark\ny\nmark\n",
			[]string{"--after", "mark", "--text", "added", "--expect", "2"},
			"x\nmark\nadded\ny\nmark\nadded\n",
		},
		{
			"a regular expression names the line",
			"alpha\nbeta\n",
			[]string{"--after", "^be", "--regex", "--text", "added", "--expect", "1"},
			"alpha\nbeta\nadded\n",
		},
		{
			// ⚠ A LAST LINE WITH NO NEWLINE would be joined onto the payload.
			"an anchor with no trailing newline gains one",
			"alpha\nbeta",
			[]string{"--after", "beta", "--text", "added", "--expect", "1"},
			"alpha\nbeta\nadded\n",
		},
		{
			"CRLF is kept and the inserted line takes it",
			"alpha\r\nbeta\r\n",
			[]string{"--before", "beta", "--text", "added", "--expect", "1"},
			"alpha\r\nadded\r\nbeta\r\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := write(t, dir, strings.ReplaceAll(c.name, " ", "-")+".txt", []byte(c.body))
			code, _, err := run(t, append([]string{"edit", p}, c.args...)...)
			if code != 0 {
				t.Fatalf("exit %d: %v", code, err)
			}
			if got := string(read(t, p)); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestInsertingBesideAMatchNeedsExpect holds the new operations to the same
// discipline as --replace: a pattern names no place of its own.
func TestInsertingBesideAMatchNeedsExpect(t *testing.T) {
	dir := t.TempDir()
	body := []byte("intro\n## Heading\n")
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"no --expect at all", []string{"--after", "## Heading", "--text", "x"}, "need --expect"},
		{"a pattern that matches nothing", []string{"--after", "absent", "--text", "x", "--expect", "1"}, "matches 0 times"},
		{"a pattern that matches more", []string{"--after", "n", "--text", "x", "--expect", "1"}, "matches 2 times"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := write(t, dir, strings.ReplaceAll(c.name, " ", "-")+".txt", body)
			code, _, err := run(t, append([]string{"edit", p}, c.args...)...)
			if code == 0 {
				t.Fatal("it was accepted")
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("the refusal does not say %q: %v", c.want, err)
			}
			if got := read(t, p); !bytes.Equal(got, body) {
				t.Fatalf("it was refused and the file moved: %q", got)
			}
		})
	}
}

// TestPathsCanComeFromAFile covers the channel for more paths than a command
// line holds.
//
// ⛔ THIS HAD NO CASE AT ALL UNTIL THE GUARD-MUTATION LENS ASKED FOR ONE. It was
// driven by hand, it worked, and "I ran it once" is not a guard. The comment
// skipping and the blank-line skipping are each a branch nothing exercised.
//
// ⭐ THE # SKIP IS WHAT MAKES A SEARCH'S OUTPUT USABLE DIRECTLY: a caller can
// comment out a path they decided against and hand the same file back.
func TestPathsCanComeFromAFile(t *testing.T) {
	dir := t.TempDir()
	body := []byte("target here\n")
	a := write(t, dir, "a.txt", body)
	b := write(t, dir, "b.txt", body)
	skipped := write(t, dir, "skipped.txt", body)

	list := write(t, dir, "list.txt", []byte(
		"# a comment naming a file that must NOT be read\n"+
			"#"+skipped+"\n"+
			"\n"+
			a+"\n"+
			"   \n"+
			b+"\n"))

	code, out, err := run(t, "edit", "--files-from", list,
		"--replace", "target", "--text", "hit", "--expect", "2")
	if code != 0 {
		t.Fatalf("exit %d: %v", code, err)
	}
	for _, p := range []string{a, b} {
		if got := read(t, p); !bytes.Contains(got, []byte("hit")) {
			t.Fatalf("%s was named in the list and not changed: %q", p, got)
		}
	}
	// ⛔ THE COMMENTED PATH MUST BE UNTOUCHED. A list format that read a
	// commented line would change a file the caller explicitly excluded.
	if got := read(t, skipped); !bytes.Equal(got, body) {
		t.Fatalf("a commented path was edited anyway: %q", got)
	}
	if strings.Contains(out, skipped) {
		t.Fatalf("a commented path appears in the report: %s", out)
	}

	// ⚠ A LIST AND NAMED PATHS TOGETHER: both are used, so --expect counts both.
	c := write(t, dir, "c.txt", body)
	code, _, err = run(t, "edit", c, "--files-from", list,
		"--replace", "hit", "--text", "again", "--expect", "2", "--allow-unmatched")
	if code != 0 {
		t.Fatalf("a list beside a named path: exit %d: %v", code, err)
	}

	// ⛔ A LIST THAT IS NOT THERE IS AN ERROR THAT NAMES THE LIST, never an empty
	// list silently edited. ⚠ THE MESSAGE IS THE ASSERTION, not the exit code:
	// with the read error swallowed the call still fails, because no path was
	// collected and `name at least one file` fires instead - which is a true
	// statement and a useless answer to "your list is missing". repo mutate
	// called the first version of this row THEATRE for exactly that.
	missing := filepath.Join(dir, "absent.txt")
	code, _, err = run(t, "edit", "--files-from", missing,
		"--replace", "x", "--text", "y", "--expect", "1")
	if code == 0 {
		t.Fatalf("a missing list was accepted: %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "absent.txt") {
		t.Fatalf("the refusal does not name the list it could not read: %v", err)
	}
}

// TestBetweenNeedsExpectLikeEveryOtherSearch is the guard --between did not have.
//
// ⛔ IT WAS THE ONLY SEARCH OPERATION WITHOUT ONE, and it is the widest of them:
// --replace changes one string, --after and --before add a line, and --between
// deletes a whole region. Measured on 2026-09-17 against the published 3.1.0
// behaviour: a file with TWO ranges wrote nothing and exited 0, and a file with
// NO range wrote nothing and exited 0. applyBetween's own comment claimed "a
// file with two ranges refuses rather than silently changing the first" - true
// only for a caller who passed a flag nothing required.
func TestBetweenNeedsExpectLikeEveryOtherSearch(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		body string
		a, b string
	}{
		{"two ranges", "A\nx\nB\nA\ny\nB\n", "A", "B"},
		{"no range at all", "A\nx\nB\n", "NOPE", "ALSO"},
		{"exactly one range", "A\nx\nB\n", "A", "B"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := write(t, dir, strings.ReplaceAll(tc.name, " ", "-")+".txt", []byte(tc.body))
			code, _, err := run(t, "edit", p, "--between", tc.a, tc.b, "--text", "GONE")
			if code != 2 {
				t.Fatalf("--between with no --expect exited %d, and a search with no stated count must be refused: %v", code, err)
			}
			// ⛔ AND THE FILE IS UNTOUCHED, which is the half that mattered: the
			// old behaviour also wrote nothing, and reported success for it.
			if got := read(t, p); !bytes.Equal(got, []byte(tc.body)) {
				t.Fatalf("a refused call wrote: %q", got)
			}
		})
	}
}

// TestBetweenSaysWhichLinesItTook is the other half of the same defect, and no
// count could have caught it.
//
// ⛔ A RANGE CAN BE THE WRONG ONE AND STILL BE EXACTLY ONE MATCH. An anchor that
// also appears earlier in the file pairs the FIRST occurrence with the closing
// anchor, so `--expect 1` is satisfied and a much larger region is replaced.
// Measured on 2026-09-17 on this repository's own consumer.ps1: 745 lines and
// 36 KB went, reported as `1 match(es)` with exit 0. The tool computed the range
// and did not print it.
func TestBetweenSaysWhichLinesItTook(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "x.txt", []byte("keep1\nif (X) {\n  early\n}\nMIDDLE\nif (X) {\n  late\n}\nEND\n"))
	code, out, err := run(t, "edit", p, "--between", "if (X) {", "END", "--text", "REPLACED", "--expect", "1")
	if code != 0 {
		t.Fatalf("exit %d: %v", code, err)
	}
	// The range it actually took, in the human line, where the caller reads it.
	if !strings.Contains(out, "lines 2-9 (8 line(s))") {
		t.Fatalf("the report does not say which lines went: %q", out)
	}
	// ⚠ AND THE SPAN SURVIVES INTO THE DOCUMENT, because the human line is for
	// the caller watching and the JSON is for whatever reads the run afterwards.
	// TestTheReportNamesTheOperationThatRan is what holds `op` itself.
	code, out, err = run(t, "edit", p, "--json", "--count", "--between", "keep1", "REPLACED")
	if code != 0 {
		t.Fatalf("--count exit %d: %v", code, err)
	}
	if !strings.Contains(out, `"op": "between"`) {
		t.Fatalf("the document does not name the operation: %q", out)
	}
}

// TestTheReportNamesTheOperationThatRan holds every operation to saying what it
// was, so the JSON document is self-describing rather than positional.
func TestTheReportNamesTheOperationThatRan(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		want string
		args []string
	}{
		{"replace", []string{"--replace", "b", "--text", "X", "--expect", "1"}},
		{"after", []string{"--after", "b", "--text", "X", "--expect", "1"}},
		{"before", []string{"--before", "b", "--text", "X", "--expect", "1"}},
		{"between", []string{"--between", "a", "c", "--text", "X", "--expect", "1"}},
		{"line", []string{"--line", "2", "--text", "X"}},
		{"delete", []string{"--delete", "2"}},
		{"insert-after", []string{"--insert-after", "1", "--text", "X"}},
		{"insert-before", []string{"--insert-before", "1", "--text", "X"}},
	} {
		t.Run(tc.want, func(t *testing.T) {
			p := write(t, dir, tc.want+".txt", []byte("a\nb\nc\n"))
			args := append([]string{"edit", p, "--json"}, tc.args...)
			code, out, err := run(t, args...)
			if code != 0 {
				t.Fatalf("exit %d: %v", code, err)
			}
			if !strings.Contains(out, `"op": "`+tc.want+`"`) {
				t.Fatalf("the document does not name the operation %q: %s", tc.want, out)
			}
		})
	}
}
