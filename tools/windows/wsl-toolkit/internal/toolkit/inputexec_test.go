// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAnExecutableInputArrivesRepairedAndRunnable is WSL-128: --input-exec
// arrives with LF endings, no byte order mark and mode 0755, so the job runs it
// by its shebang, while --input of the same file arrives byte for byte at 0644.
func TestAnExecutableInputArrivesRepairedAndRunnable(t *testing.T) {
	raw := []byte("\xEF\xBB\xBF#!/bin/sh\r\necho hi\r\n")
	f := filepath.Join(t.TempDir(), "run.sh")
	if err := os.WriteFile(f, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	inputs := []JobInput{
		{Name: "run.sh", Path: f, Exec: true},
		{Name: "raw.sh", Path: f},
		{Name: "wire.sh", Bytes: raw, Exec: true},
	}
	if err := CheckInputs(inputs, DefaultWorkspaceLimits()); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := writeInputsTar(&buf, inputs); err != nil {
		t.Fatal(err)
	}
	type member struct {
		mode int64
		body string
	}
	got := map[string]member{}
	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		got[hdr.Name] = member{hdr.Mode, string(b)}
	}
	repaired := "#!/bin/sh\necho hi\n"
	for _, name := range []string{"run.sh", "wire.sh"} {
		if m := got[name]; m.mode != 0o755 || m.body != repaired {
			t.Errorf("%s arrived as mode %o, %q; want 0755 and %q", name, m.mode, m.body, repaired)
		}
	}
	if m := got["raw.sh"]; m.mode != 0o644 || m.body != string(raw) {
		t.Errorf("--input arrived as mode %o, %q; want 0644 and its bytes unchanged", m.mode, m.body)
	}
}

// TestAnExecutableInputThatCannotRunByItsShebangIsRefused: a file with no #!,
// and a UTF-16 one, are refused before the job is claimed, on both routes.
func TestAnExecutableInputThatCannotRunByItsShebangIsRefused(t *testing.T) {
	for name, body := range map[string][]byte{
		"data":   []byte("plain\r\ndata\r\n"),
		"wide":   {0xFF, 0xFE, '#', 0, '!', 0},
		"binary": append([]byte{0x7F, 'E', 'L', 'F'}, "\r\n"...),
	} {
		exec := []JobInput{
			{Name: name, Bytes: body, Exec: true},
		}
		err := CheckInputs(exec, DefaultWorkspaceLimits())
		if err == nil || !strings.Contains(err.Error(), "--input-exec "+name) {
			t.Errorf("--input-exec %s was accepted or not named: %v", name, err)
		}
		plain := []JobInput{
			{Name: name, Bytes: body},
		}
		if err := CheckInputs(plain, DefaultWorkspaceLimits()); err != nil {
			t.Errorf("--input %s was refused, and it takes any bytes: %v", name, err)
		}
	}
}

// TestAnAttachedBaseExecPlacesAnExecutableInputRepaired is base exec
// --input-exec, attached: the copy in the command's in directory has LF endings
// and mode 0755, and --input of the same file beside it keeps its bytes at 0644.
func TestAnAttachedBaseExecPlacesAnExecutableInputRepaired(t *testing.T) {
	g := newExecFakeGuest(t)
	raw := "#!/bin/sh\r\necho hi\r\n"
	f := filepath.Join(t.TempDir(), "run.sh")
	if err := os.WriteFile(f, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := RunBaseExec(context.Background(), g.wsl, BaseExec{
		Distro: "wsl-toolkit", User: "toolkit", Dir: "~", Script: []byte("\"$" + ExecInEnv + "/bin/run.sh\"\n"),
		Inputs: []JobInput{
			{Name: "bin/run.sh", Path: f, Exec: true},
			{Name: "raw.sh", Path: f},
		},
	})
	if err != nil || res.Exit != 0 {
		t.Fatalf("the call answered %d, %v", res.Exit, err)
	}
	placed := false
	for _, c := range g.calls(t) {
		body, ok := c.Files["in/bin/run.sh"]
		if !ok {
			continue
		}
		placed = true
		if body != "#!/bin/sh\necho hi\n" || c.Modes["in/bin/run.sh"] != 0o755 {
			t.Errorf("--input-exec arrived as mode %o, %q; want 0755 and LF endings", c.Modes["in/bin/run.sh"], body)
		}
		if c.Files["in/raw.sh"] != raw || c.Modes["in/raw.sh"] != 0o644 {
			t.Errorf("--input arrived as mode %o, %q; want 0644 and its bytes unchanged", c.Modes["in/raw.sh"], c.Files["in/raw.sh"])
		}
	}
	if !placed {
		t.Fatal("no archive placed the executable input in the command's in directory")
	}
}

// TestADetachedSessionPlacesAnExecutableInputRepaired is the same on the
// session's one archive.
func TestADetachedSessionPlacesAnExecutableInputRepaired(t *testing.T) {
	d := "/home/user/" + GuestRoot + "/sessions/0123456789abcdef"
	files := sessionFiles(d, SessionStart{
		Payload: []byte("\"$" + ExecInEnv + "/run.sh\"\n"),
		Inputs: []JobInput{
			{Name: "run.sh", Bytes: []byte("#!/bin/sh\r\ntrue\r\n"), Exec: true},
		},
	})
	var buf bytes.Buffer
	if _, err := writeInputsTar(&buf, files); err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Name != "in/run.sh" {
			continue
		}
		b, _ := io.ReadAll(tr)
		if hdr.Mode != 0o755 || string(b) != "#!/bin/sh\ntrue\n" {
			t.Fatalf("--input-exec arrived in the session as mode %o, %q; want 0755 and LF endings", hdr.Mode, b)
		}
		return
	}
	t.Fatalf("the session's archive holds no in/run.sh: %+v", files)
}

// TestAnExecutableInputCrossesTheHelper: the mark travels with the bytes, and
// the helper repairs them where it writes them.
func TestAnExecutableInputCrossesTheHelper(t *testing.T) {
	req := HelperRunRequest{Inputs: []HelperInput{
		{Name: "run.sh", B64: base64.StdEncoding.EncodeToString([]byte("#!/bin/sh\r\ntrue\r\n")), Exec: true},
	}}
	_, inputs, err := req.jobExtras()
	if err != nil || len(inputs) != 1 || !inputs[0].Exec {
		t.Fatalf("the helper read %+v, %v", inputs, err)
	}
	req.Inputs[0].B64 = base64.StdEncoding.EncodeToString([]byte("no shebang"))
	if _, _, err := req.jobExtras(); err == nil {
		t.Error("the helper took an executable input with no shebang")
	}
}

// TestAnInputIsNamedAsItWasTyped: every refusal names the flag that gave an
// input and the NAME as it was typed. A name refused under --input-exec read
// --input, one name given to both flags read as --input twice, and an input a
// base exec placed under in/ was named in/NAME and /in/in/NAME. WSL-128.
func TestAnInputIsNamedAsItWasTyped(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "run.sh")
	script := []byte("#!/bin/sh\ntrue\n")
	if err := os.WriteFile(f, script, 0o600); err != nil {
		t.Fatal(err)
	}
	refused := map[string][]JobInput{
		`--input-exec name "../x"`: {
			{Name: "../x", Bytes: script, Exec: true},
		},
		`--input and --input-exec both name "a"`: {
			{Name: "a", Path: f},
			{Name: "a", Path: f, Exec: true},
		},
		`--input-exec names "a" twice`: {
			{Name: "a", Bytes: script, Exec: true},
			{Name: "a", Bytes: script, Exec: true},
		},
		`--input-exec "a" is a file and --input "a/b" needs it to be a directory`: {
			{Name: "a", Bytes: script, Exec: true},
			{Name: "a/b", Bytes: script},
		},
	}
	for want, inputs := range refused {
		if err := CheckInputs(inputs, DefaultWorkspaceLimits()); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("CheckInputs answered %v; want it to say %s", err, want)
		}
	}
	// ⛔ A file that went after the check is named as typed, where a base exec's
	// command sees it, and a job's form stays /in/NAME.
	gone := filepath.Join(dir, "gone.sh")
	written := map[string]JobInput{
		"--input-exec bin/run.sh=":              JobInput{Name: "bin/run.sh", Path: gone, Exec: true}.under("in"),
		"stopped at $" + ExecInEnv + "/raw.sh,": JobInput{Name: "raw.sh", Path: gone}.under("in"),
		"stopped at /in/raw.sh,":                {Name: "raw.sh", Path: gone},
	}
	for want, in := range written {
		_, err := writeInputsTar(io.Discard, []JobInput{in})
		if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "in/in/") ||
			strings.Contains(err.Error(), "in/bin/run.sh") {
			t.Errorf("a vanished %s answered %v; want it named %q", in.flag(), err, want)
		}
	}
}

// TestAnInputMemberIsWrittenUnderItsArchivePathAndNamedWhereItIsSeen: the
// member writer puts a file at its archive path and names a failure by the
// name it was handed for that, so an input under in/ never reads in/NAME.
// WSL-128.
func TestAnInputMemberIsWrittenUnderItsArchivePathAndNamedWhereItIsSeen(t *testing.T) {
	p := filepath.Join(t.TempDir(), "raw.sh")
	if err := os.WriteFile(p, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	var up WorkspaceUpload
	err = writeRegularMemberAs(tar.NewWriter(&buf), &up, p, "in/raw.sh", "$"+ExecInEnv+"/raw.sh", info, 0o644)
	if err == nil || !strings.Contains(err.Error(), "stopped at $"+ExecInEnv+"/raw.sh, opening it") {
		t.Errorf("err = %v, want it to name $%s/raw.sh and the open", err, ExecInEnv)
	}
	if hdr, err := tar.NewReader(&buf).Next(); err != nil || hdr.Name != "in/raw.sh" {
		t.Errorf("the member was written as %+v, %v; want in/raw.sh", hdr, err)
	}
}

// TestAnInputThatFailsInsideItsCopyIsNamedWhereItIsSeen: a failure inside the
// member writer names a base exec's input under WSL_TOOLKIT_IN, not in/NAME.
// A directory past the check is the input whose read fails. WSL-128.
func TestAnInputThatFailsInsideItsCopyIsNamedWhereItIsSeen(t *testing.T) {
	dir := t.TempDir()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	// ⚠ A host that sizes a directory at 0 reads nothing from it, so the copy
	// cannot fail there, and this case has nothing to assert.
	if info.Size() == 0 {
		t.Skip("this host sizes a directory at 0, so its copy reads nothing and cannot fail")
	}
	_, err = writeInputsTar(io.Discard, []JobInput{JobInput{Name: "d", Path: dir}.under("in")})
	want := "stopped at $" + ExecInEnv + "/d, copying its bytes"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("a copy that failed inside its member answered %v; want %q", err, want)
	}
}

// TestAnExecutableInputThatIsNotAFileIsRefusedAsAnInputIs: a directory given as
// --input-exec is refused before it is read, in --input's words, at the check
// and where the archive is written. WSL-128.
func TestAnExecutableInputThatIsNotAFileIsRefusedAsAnInputIs(t *testing.T) {
	dir := t.TempDir()
	in := JobInput{Name: "run.sh", Path: dir, Exec: true}
	want := "--input-exec run.sh=" + dir + " is not a regular file"
	if err := CheckInputs([]JobInput{in}, DefaultWorkspaceLimits()); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("CheckInputs answered %v; want %q", err, want)
	}
	if _, err := writeInputsTar(io.Discard, []JobInput{in.under("in")}); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("writeInputsTar answered %v; want %q", err, want)
	}
}
