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
		{"replace between two markers", []string{"--between", "one,three", "--text", "ALL"}, "ALL\n"},
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
