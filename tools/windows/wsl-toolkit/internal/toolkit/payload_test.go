// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestFramePayloadRefusesANulByte(t *testing.T) {
	if _, err := FramePayload([]byte("echo a\x00b\n")); !errors.Is(err, ErrPayloadNUL) {
		t.Fatalf("a payload carrying NUL was framed: %v", err)
	}
}

// TestTheFrameShapeGivesTheCommandDevNullAndEndsOnItsDelimiter reads the frame
// as text. The behaviour it exists for is proved by running it, below, on a host
// that has a shell.
func TestTheFrameShapeGivesTheCommandDevNullAndEndsOnItsDelimiter(t *testing.T) {
	framed, err := FramePayload([]byte("echo no-final-newline"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(framed), "\n"), "\n")
	open := lines[1]
	if !strings.HasPrefix(open, "{ . /dev/fd/9; } 9<<'"+payloadDelimiterPrefix) || !strings.HasSuffix(open, "' </dev/null") {
		t.Fatalf("the opening line is not the compound command with stdin from /dev/null: %q", open)
	}
	delim := strings.TrimSuffix(strings.TrimPrefix(open, "{ . /dev/fd/9; } 9<<'"), "' </dev/null")
	if lines[len(lines)-1] != delim {
		t.Fatalf("the frame does not end on its delimiter line: %q", lines[len(lines)-1])
	}
	if lines[len(lines)-2] != "echo no-final-newline" {
		t.Fatalf("a payload with no final newline did not become one whole line: %q", lines[len(lines)-2])
	}
}

type fixedBytes struct{ chunks [][]byte }

func (f *fixedBytes) Read(p []byte) (int, error) {
	if len(f.chunks) == 0 {
		return 0, errors.New("exhausted")
	}
	n := copy(p, f.chunks[0])
	f.chunks = f.chunks[1:]
	return n, nil
}

func TestADelimiterThatOccursInThePayloadIsDrawnAgain(t *testing.T) {
	first := bytes.Repeat([]byte{0xAB}, 16)
	second := bytes.Repeat([]byte{0xCD}, 16)
	collision := payloadDelimiterPrefix + strings.Repeat("ab", 16)
	got, err := payloadDelimiterFrom(&fixedBytes{chunks: [][]byte{first, second}}, []byte("echo "+collision+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got != payloadDelimiterPrefix+strings.Repeat("cd", 16) {
		t.Fatalf("the delimiter %q was used although the payload carries %q", got, collision)
	}
}

// TestAFramedCommandThatReadsStdinCannotEatTheRestOfIt runs the frame through a
// real POSIX shell reading it from a pipe, the way wsl.exe feeds /bin/sh.
//
// ⛔ THE DEFECT IT PROVES ABSENT. Unframed, `cat` consumed the lines after it and
// the run exited 0 over a command that never ran. The whole here-document being
// parsed first is what stops that. `</dev/null` is what hands the command
// /dev/null instead of the pipe the shell reads, and the stdin case below is the
// one that goes red without it.
func TestAFramedCommandThatReadsStdinCannotEatTheRestOfIt(t *testing.T) {
	sh := "/bin/sh"
	if runtime.GOOS == "windows" {
		t.Skip("the frame is read by a POSIX shell, and this host has none to hand it to")
	}
	if _, err := os.Stat(sh); err != nil {
		t.Skip("no /bin/sh on this host")
	}
	pad := strings.Repeat("# "+strings.Repeat("x", 97)+"\n", 200)
	cases := map[string]struct {
		payload string
		out     string
		code    int
	}{
		"a command reading stdin":         {"cat >/dev/null\n" + pad + "echo SECOND-LINE-RAN\nexit 7\n", "SECOND-LINE-RAN", 7},
		"the command's stdin":             {"if [ -c /dev/stdin ]; then echo devnull; else echo the-shells-pipe; fi\n", "devnull", 0},
		"descriptor 9 reused":             {"exec 9>/dev/null\n" + pad + "echo AFTER-FD9\nexit 5\n", "AFTER-FD9", 5},
		"return at top level":             {"echo before\nreturn 3\necho NOT-REACHED\n", "before", 3},
		"an environment assignment ahead": {"printf '%s' \"$WTK_PROBE\"\n", "it's $HOME", 0},
	}
	probe := EnvPair{Name: "WTK_PROBE", Value: "it's $HOME"}
	for label, c := range cases {
		payload := ComposePayload([]byte(c.payload), []EnvPair{probe}, false)
		framed, err := FramePayload(payload)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(sh)
		cmd.Stdin = bytes.NewReader(framed)
		out, err := cmd.Output()
		code := 0
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			code = exited.ExitCode()
		} else if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if strings.TrimSpace(string(out)) != c.out || code != c.code {
			t.Errorf("%s: printed %q and exited %d, want %q and %d", label, out, code, c.out, c.code)
		}
	}
}

func TestACallersCommandIsFramedOnTheWayToTheShellAndAToolQuestionIsNot(t *testing.T) {
	framed, err := ExecRequest{Script: []byte("cat\necho after\n"), Env: map[string]string{"A": "1"}, Payload: true}.stdin()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(framed, []byte("' </dev/null\nA='1'; export A\ncat\necho after\n")) {
		t.Fatalf("a caller's command reaches the shell unframed: %q", framed)
	}
	plain, err := ExecRequest{Script: []byte("true\n")}.stdin()
	if err != nil || string(plain) != "true\n" {
		t.Fatalf("a question this tool asks itself was changed: %q, %v", plain, err)
	}
	if _, err := (ExecRequest{Script: []byte("a\x00"), Payload: true}).stdin(); err == nil {
		t.Fatal("a command the frame cannot carry reached the shell")
	}
}

func TestComposePayloadPutsThePrologueFirstAndTheCommandLastAndUnchanged(t *testing.T) {
	command := []byte("printf '%s' \"$PATH\"\n")
	got := ComposePayload(command, []EnvPair{{Name: "PATH", Value: "/caller/bin"}, {Name: "TMPDIR", Value: "/caller/tmp"}}, true)
	if !bytes.HasSuffix(got, command) {
		t.Fatalf("the command is not the exact suffix: %q", got)
	}
	prologue := bytes.LastIndex(got, []byte("_wtk_run=/tmp/wsl-toolkit-run-"))
	callerPath := bytes.Index(got, []byte("PATH='/caller/bin'; export PATH"))
	callerTmp := bytes.Index(got, []byte("TMPDIR='/caller/tmp'; export TMPDIR"))
	// ⛔ A VALUE THE CALLER SET WINS OVER WHAT THE PROLOGUE PREPARES, so the
	// caller's assignments come after it.
	if prologue < 0 || callerPath < prologue || callerTmp < callerPath {
		t.Fatalf("the order is not prologue, then the caller's assignments in order: %d %d %d", prologue, callerPath, callerTmp)
	}
	if plain := ComposePayload(command, nil, false); !bytes.Equal(plain, command) {
		t.Fatalf("with nothing to add the payload changed: %q", plain)
	}
}

func TestAnEnvironmentValueIsAssignedAndExportedNeverSubstituted(t *testing.T) {
	for value, want := range map[string]string{
		"https://x/a&b":      "URL='https://x/a&b'; export URL",
		"it's":               `URL='it'\''s'; export URL`,
		"$X `id` \"q\" $(x)": "URL='$X `id` \"q\" $(x)'; export URL",
		"":                   "URL=''; export URL",
	} {
		pair := EnvPair{Name: "URL", Value: value}
		if got := strings.TrimSpace(string(ComposePayload(nil, []EnvPair{pair}, false))); got != want {
			t.Errorf("value %q became %q, want %q", value, got, want)
		}
	}
}

func TestAPairIsNameEqualsValueWithAShellName(t *testing.T) {
	for _, bad := range []string{"NOEQUALS", "=value", "2BAD=x", "BAD-NAME=x", "a b=c"} {
		if _, err := ParseEnvPair(bad); err == nil {
			t.Errorf("%q was accepted as a pair", bad)
		}
	}
	p, err := ParseEnvPair("A=b=c")
	if err != nil || p.Name != "A" || p.Value != "b=c" {
		t.Fatalf("A=b=c parsed as %+v, %v", p, err)
	}
}

func TestAnEnvFileIsRepairedSkipsCommentsAndRefusesABadLineByNumber(t *testing.T) {
	pairs, err := ParseEnvFile([]byte("\xEF\xBB\xBF# a comment\r\n\r\nA=1\r\nB=two words\r\n"), "pairs.env")
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 2 || pairs[0] != (EnvPair{"A", "1"}) || pairs[1] != (EnvPair{"B", "two words"}) {
		t.Fatalf("pairs = %+v", pairs)
	}
	_, err = ParseEnvFile([]byte("A=1\nNOTAPAIR\n"), "pairs.env")
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("a bad line was not refused by number: %v", err)
	}
	if _, err := ParseEnvFile([]byte{0xFF, 0xFE, 'A', 0, '=', 0}, "pairs.env"); err == nil {
		t.Fatal("a UTF-16 file was read")
	}
}

func TestTheRepairReportSaysWhatChangedInTheCopy(t *testing.T) {
	raw := []byte("\xEF\xBB\xBFa\r\nb\r\nc")
	out, rep, err := RepairGuestScriptReport(raw)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "a\nb\nc\n" || rep.CRLF != 2 || !rep.BOMRemoved || !rep.NewlineAdd || !rep.Changed() {
		t.Fatalf("out %q report %+v", out, rep)
	}
	if string(raw) != "\xEF\xBB\xBFa\r\nb\r\nc" {
		t.Fatal("the bytes that were read were modified")
	}
}
