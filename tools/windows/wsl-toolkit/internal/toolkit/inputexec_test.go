// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
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
