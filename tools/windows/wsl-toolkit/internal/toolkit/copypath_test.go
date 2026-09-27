// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// WSL-98: every failure in the copy path names the member it stopped at, the
// guest's own words win where the guest stopped first, and a writer can never
// wait for ever on a guest that stopped reading.

func TestAMemberThatVanishedBeforeItsOpenIsNamed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gone.txt")
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
	var up WorkspaceUpload
	err = writeRegularMember(tar.NewWriter(io.Discard), &up, p, "sub/gone.txt", info, 0o644)
	if err == nil || !strings.Contains(err.Error(), "stopped at sub/gone.txt, opening it") {
		t.Fatalf("err = %v, want it to name sub/gone.txt and the open", err)
	}
}

// writeTree makes a workspace larger than one read of the guest, so its writer
// still has bytes to send when the guest stops.
func writeTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for i := 0; i < 8; i++ {
		body := bytes.Repeat([]byte{'x'}, 4096)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAGuestThatStopsReadingSpeaksFirstAndNothingWaitsForEver(t *testing.T) {
	root := writeTree(t)
	type outcome struct {
		up  WorkspaceUpload
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		up, err := pumpArchive(
			func(w io.Writer) (WorkspaceUpload, error) {
				return writeWorkspaceTar(w, root, DefaultWorkspaceLimits(), nil)
			},
			func(r io.Reader) (int, error) {
				// The guest reads one record and stops, as a tar that failed does.
				_, _ = io.ReadFull(r, make([]byte, 512))
				return 2, &ProcessError{Code: 2, Op: "process", Err: errors.New("exit status 2")}
			},
			func() string { return "tar: /home/toolkit/x: Cannot write: No space left on device" })
		done <- outcome{up, err}
	}()
	select {
	case o := <-done:
		msg := fmt.Sprint(o.err)
		for _, want := range []string{"exited 2", "No space left on device", "The copy had reached", "stopped at f"} {
			if !strings.Contains(msg, want) {
				t.Errorf("err = %q, want it to carry %q", msg, want)
			}
		}
	case <-time.After(20 * time.Second):
		t.Fatal("pumpArchive did not return: the writer waits on a pipe nobody reads")
	}
}

func TestAFailureOnThisSideIsNamedAheadOfTheCutArchiveItCauses(t *testing.T) {
	refused := fmt.Errorf("%w: the workspace passes 1.0 KiB at big.bin", ErrWorkspaceRefused)
	_, err := pumpArchive(
		func(w io.Writer) (WorkspaceUpload, error) { return WorkspaceUpload{}, refused },
		func(r io.Reader) (int, error) {
			_, _ = io.Copy(io.Discard, r)
			return 2, errors.New("exit status 2")
		},
		func() string { return "tar: Unexpected EOF in archive" })
	if !errors.Is(err, ErrWorkspaceRefused) || strings.Contains(fmt.Sprint(err), "Unexpected EOF") {
		t.Errorf("err = %v, want the refusal and not the guest's complaint about it", err)
	}
	_, err = pumpArchive(
		func(w io.Writer) (WorkspaceUpload, error) { return WorkspaceUpload{}, nil },
		func(r io.Reader) (int, error) {
			_, _ = io.Copy(io.Discard, r)
			return 2, errors.New("exit status 2")
		},
		func() string { return "tar: ./a: Cannot open: Permission denied" })
	if !strings.Contains(fmt.Sprint(err), "Cannot open: Permission denied") {
		t.Errorf("err = %v, want the guest's own words when only the guest failed", err)
	}
}

func TestAFetchNamesTheFailureThatCausedTheOthers(t *testing.T) {
	refused := fmt.Errorf("%w: %q names the Windows device \"nul\"", ErrWorkspaceRefused, "nul")
	cut := artifactError("b.txt", "writing its bytes", io.ErrUnexpectedEOF)
	cases := []struct {
		name       string
		code       int
		stderr     string
		extractErr error
		want, not  string
	}{
		{"a refusal beats the broken pipe it causes", 2, "tar: write error: Broken pipe", refused, "names the Windows device", "Broken pipe"},
		{"the guest's words beat the cut archive", 2, "tar: ./b.txt: Cannot open: Permission denied", cut, "Cannot open: Permission denied", ""},
		{"a failure here names its member", 0, "", cut, "stopped at b.txt, writing its bytes", ""},
		{"the guest failed alone", 2, "tar: ./c: Cannot stat: No such file", nil, "Cannot stat", ""},
	}
	for _, c := range cases {
		err := fetchFailure("/home/toolkit/.wsl-toolkit/jobs/x/out", c.code, nil, c.stderr, c.extractErr)
		msg := fmt.Sprint(err)
		if !strings.Contains(msg, c.want) || (c.not != "" && strings.Contains(msg, c.not)) {
			t.Errorf("%s: err = %q, want %q and not %q", c.name, msg, c.want, c.not)
		}
	}
	if err := fetchFailure("/x", 0, nil, "", nil); err != nil {
		t.Errorf("both halves succeeded and err = %v", err)
	}
}

func TestACutArtifactArchiveNamesWhereItBroke(t *testing.T) {
	var b bytes.Buffer
	tw := tar.NewWriter(&b)
	for _, m := range []struct {
		name string
		size int
	}{{"a.txt", 10}, {"b.txt", 2000}} {
		if err := tw.WriteHeader(&tar.Header{Name: m.name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(m.size)}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(bytes.Repeat([]byte{'y'}, m.size)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	whole := b.Bytes()
	// Cut inside b.txt's bytes: the member is named, with what was being done.
	_, err := extractInto(bytes.NewReader(whole[:512*3+700]), t.TempDir(), DefaultWorkspaceLimits())
	if !strings.Contains(fmt.Sprint(err), "stopped at b.txt, writing its bytes") {
		t.Errorf("a cut inside b.txt: err = %v", err)
	}
	// Cut inside b.txt's header: the member before it is named.
	_, err = extractInto(bytes.NewReader(whole[:512*2+100]), t.TempDir(), DefaultWorkspaceLimits())
	if !strings.Contains(fmt.Sprint(err), "broke after a.txt") {
		t.Errorf("a cut inside b.txt's header: err = %v", err)
	}
}

func TestAWindowsDeviceNameIsRecognisedInEveryShape(t *testing.T) {
	for _, name := range []string{"NUL", "nul", "NUL.txt", "con.log", "COM1", "lpt9.x", "Aux", "nul "} {
		if !isWindowsDeviceName(name) {
			t.Errorf("%q is a device name to Windows", name)
		}
	}
	for _, name := range []string{"null", "CONSOLE", "com10", "aux2", "nul_", "prn-x"} {
		if isWindowsDeviceName(name) {
			t.Errorf("%q is an ordinary name", name)
		}
	}
}

// TestAFileNamedLikeADeviceTravelsWithItsBytes carries a real file named NUL,
// which Windows opens as the device by its plain path.
func TestAFileNamedLikeADeviceTravelsWithItsBytes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the device mapping is a property of Windows")
	}
	root := t.TempDir()
	long := `\\?\` + filepath.Join(root, "NUL")
	body := []byte("two lines of a known_hosts fragment\nand the second\n")
	if err := os.WriteFile(long, body, 0o600); err != nil {
		t.Fatal(err)
	}
	// ⚠ Removed through the same prefix, or the directory cleanup meets the
	// device instead of the file and fails the case.
	t.Cleanup(func() { _ = os.Remove(long) })
	var archive bytes.Buffer
	if _, err := writeWorkspaceTar(&archive, root, DefaultWorkspaceLimits(), nil); err != nil {
		t.Fatalf("the copy refused a file it can read: %v", err)
	}
	tr := tar.NewReader(&archive)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			t.Fatal("NUL is not in the archive")
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Name != "NUL" {
			continue
		}
		got, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, body) {
			t.Fatalf("NUL arrived as %q, want its own %d bytes", got, len(body))
		}
		return
	}
}

// TestADeviceSafePathOnlyChangesADeviceName holds the rewrite to the names it
// exists for.
func TestADeviceSafePathOnlyChangesADeviceName(t *testing.T) {
	if runtime.GOOS != "windows" {
		if got := deviceSafePath("/work/NUL"); got != "/work/NUL" {
			t.Errorf("on %s the path changed to %q", runtime.GOOS, got)
		}
		return
	}
	cases := map[string]string{
		`C:\w\NUL`:           `\\?\C:\w\NUL`,
		`C:\w\sub\con.log`:   `\\?\C:\w\sub\con.log`,
		`C:\w\file.txt`:      `C:\w\file.txt`,
		`\\server\share\NUL`: `\\?\UNC\server\share\NUL`,
		`\\?\C:\w\NUL`:       `\\?\C:\w\NUL`,
	}
	for in, want := range cases {
		if got := deviceSafePath(in); got != want {
			t.Errorf("deviceSafePath(%q) = %q, want %q", in, got, want)
		}
	}
}
