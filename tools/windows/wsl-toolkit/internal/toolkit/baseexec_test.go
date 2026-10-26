// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// -- WSL-124: a directory the command cannot enter ------------------------------

// TestADirectoryTheCommandCannotEnterIsRefused is the defect: wsl.exe answers a
// --cd it cannot follow by starting in / and exiting 0. The command enters its
// directory itself now, and a refusal is the tool's, not a guest exit.
func TestADirectoryTheCommandCannotEnterIsRefused(t *testing.T) {
	g := newExecFakeGuest(t)
	t.Setenv(execFakeMissingEnv, "/srv/nope")
	var stderr bytes.Buffer
	res, err := RunBaseExec(context.Background(), g.wsl, BaseExec{
		Distro: "wsl-toolkit", User: "toolkit", Dir: "/srv/nope", Script: []byte("pwd\n"), Stderr: &stderr,
	})
	if !errors.Is(err, ErrDirNotEntered) || !strings.Contains(err.Error(), "--dir /srv/nope could not be entered in wsl-toolkit as toolkit") {
		t.Fatalf("a directory the guest lacks answered exit %d and %v", res.Exit, err)
	}
	if !strings.Contains(stderr.String(), "No such file or directory") || strings.Contains(stderr.String(), "wtk-exec-") {
		t.Errorf("the guest's own reason did not reach stderr whole and alone: %q", stderr.String())
	}
	calls := g.calls(t)
	if len(calls) != 1 || calls[0].Cd != "~" || !strings.Contains(calls[0].Stdin, "cd '/srv/nope' || {") {
		t.Fatalf("the command did not enter its directory itself from the home: %+v", calls)
	}
}

// TestACommandThatEntersItsDirectoryAnswersWithItsOwnExit holds the other half:
// the word the gate takes out never reaches the caller, and the exit is the
// command's.
func TestACommandThatEntersItsDirectoryAnswersWithItsOwnExit(t *testing.T) {
	g := newExecFakeGuest(t)
	t.Setenv(execFakeExitEnv, "7")
	var stderr bytes.Buffer
	res, err := RunBaseExec(context.Background(), g.wsl, BaseExec{
		Distro: "wsl-toolkit", User: "toolkit", Dir: "/srv/there", Script: []byte("pwd\n"), Stderr: &stderr,
	})
	if errors.Is(err, ErrDirNotEntered) || res.Exit != 7 {
		t.Fatalf("a command that ran answered exit %d and %v, want its own 7", res.Exit, err)
	}
	if stderr.String() != "the command's own stderr\n" {
		t.Errorf("stderr is %q, want the command's own and nothing of the gate", stderr.String())
	}
	// ⭐ THE HOME IS wsl.exe's OWN DEFAULT, and nothing is added in front of it.
	g2 := newExecFakeGuest(t)
	if _, err := RunBaseExec(context.Background(), g2.wsl, BaseExec{Distro: "wsl-toolkit", User: "toolkit", Dir: "~", Script: []byte("pwd\n")}); err != nil {
		t.Fatal(err)
	}
	if c := g2.calls(t); len(c) != 1 || strings.Contains(c[0].Stdin, "wtk-exec-") || c[0].Cd != "~" {
		t.Errorf("a command in the home was sent a gate: %+v", c)
	}
}

func TestTheStartGateTakesItsWordOutAndHoldsNothingAfter(t *testing.T) {
	var out bytes.Buffer
	g := &startGate{dst: &out, started: []byte("wtk-exec-started-ab"), refused: []byte("wtk-exec-no-dir-ab")}
	for _, part := range []string{"<3>WSL a warning\n", "wtk-exec-sta", "rted-ab", "own line\n", "second\n"} {
		if _, err := g.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if out.String() != "<3>WSL a warning\nown line\nsecond\n" || g.Refused() {
		t.Fatalf("a started command's stream read %q, refused %v", out.String(), g.Refused())
	}
	out.Reset()
	r := &startGate{dst: &out, started: []byte("wtk-exec-started-ab"), refused: []byte("wtk-exec-no-dir-ab")}
	_, _ = r.Write([]byte("sh: cd: /x: No such file or directory\nwtk-exec-no-dir-ab"))
	if !r.Refused() || out.String() != "sh: cd: /x: No such file or directory\n" {
		t.Fatalf("a refused command read %q, refused %v", out.String(), r.Refused())
	}
	out.Reset()
	silent := &startGate{dst: &out, started: []byte("wtk-exec-started-ab"), refused: []byte("wtk-exec-no-dir-ab")}
	_, _ = silent.Write([]byte("wsl.exe could not start the shell\n"))
	if out.Len() != 0 {
		t.Fatal("a gate passed bytes on before it heard either word")
	}
	if err := silent.Flush(); err != nil || out.String() != "wsl.exe could not start the shell\n" || silent.Refused() {
		t.Fatalf("a gate that heard no word released %q, refused %v", out.String(), silent.Refused())
	}
	out.Reset()
	flood := &startGate{dst: &out, started: []byte("wtk-exec-started-ab"), refused: []byte("wtk-exec-no-dir-ab")}
	_, _ = flood.Write(bytes.Repeat([]byte("x"), maxGateHold+1))
	if out.Len() != maxGateHold+1 {
		t.Fatalf("a gate held %d bytes past its bound", maxGateHold+1-out.Len())
	}
}

// -- WSL-124: inputs, artifacts and stdin -----------------------------------------

// TestInputsAndArtifactsGetADirectoryOfTheCommandsOwn drives the whole attached
// path: the call's directory recorded, the inputs placed under it, both paths
// exported to the command, the output fetched, and the directory removed.
func TestInputsAndArtifactsGetADirectoryOfTheCommandsOwn(t *testing.T) {
	g := newExecFakeGuest(t)
	host := t.TempDir()
	in := filepath.Join(host, "app.toml")
	if err := os.WriteFile(in, []byte("name = 'x'\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(host, "out")
	if err := os.MkdirAll(filepath.Join(out, "d"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "d", "r.txt"), []byte("result"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(execFakeOutEnv, out)
	dest := filepath.Join(host, "delivered")
	res, err := RunBaseExec(context.Background(), g.wsl, BaseExec{
		Distro: "wsl-toolkit", User: "toolkit", Dir: "~", Script: []byte("true\n"),
		Inputs: []JobInput{
			{Name: "conf/app.toml", Path: in},
		},
		ArtifactDir: dest,
	})
	if err != nil || res.ArtifactError != "" {
		t.Fatalf("the call answered %v and %q", err, res.ArtifactError)
	}
	dir := execFakeHome + "/" + GuestRoot + "/jobs/" + res.ID
	var sent, framed, removed bool
	for _, c := range g.calls(t) {
		if body, ok := c.Files["in/conf/app.toml"]; ok && body == "name = 'x'\r\n" {
			sent = true
		}
		if strings.Contains(c.Stdin, ExecInEnv+"='"+dir+"/in'; export "+ExecInEnv) && strings.Contains(c.Stdin, ExecOutEnv+"='"+dir+"/out'; export "+ExecOutEnv) {
			framed = true
		}
		if strings.Contains(c.Stdin, "tk_dir='"+dir+"'") {
			removed = true
		}
	}
	if !sent || !framed || !removed {
		t.Fatalf("input placed %v, both paths exported %v, directory removed %v", sent, framed, removed)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "d", "r.txt")); err != nil || string(b) != "result" {
		t.Fatalf("the output did not arrive: %q %v", b, err)
	}
	if res.Artifacts.Delivered != 2 || len(res.Artifacts.List) != 2 {
		t.Errorf("the transfer counted %d and listed %+v", res.Artifacts.Delivered, res.Artifacts.List)
	}
	ledger, err := OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	open, err := ledger.Open()
	if err != nil {
		t.Fatal(err)
	}
	all, err := ledger.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 || len(all) != 2 || all[0].GuestDir != dir || all[0].Kind != "job" {
		t.Errorf("the ledger holds %+v, want the call's directory opened and closed", all)
	}
}

// TestAFailedTransferKeepsTheCallsDirectoryForARetry is a job's rule on the base
// exec path: the guest copy is the only one left.
func TestAFailedTransferKeepsTheCallsDirectoryForARetry(t *testing.T) {
	g := newExecFakeGuest(t)
	host := t.TempDir()
	out := filepath.Join(host, "out")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "r.txt"), []byte("result"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(execFakeOutEnv, out)
	blocker := filepath.Join(host, "blocker")
	if err := os.WriteFile(blocker, []byte("a file"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := RunBaseExec(context.Background(), g.wsl, BaseExec{
		Distro: "wsl-toolkit", User: "toolkit", Dir: "~", Script: []byte("true\n"), ArtifactDir: filepath.Join(blocker, "sub"),
	})
	if err != nil || res.ArtifactError == "" || res.Retained != execFakeHome+"/"+GuestRoot+"/jobs/"+res.ID {
		t.Fatalf("a failed transfer answered %v, %q, retained %q", err, res.ArtifactError, res.Retained)
	}
	for _, c := range g.calls(t) {
		if strings.Contains(c.Stdin, "tk_dir=") {
			t.Fatal("the directory holding the only copy of the output was removed")
		}
	}
	r := &Runner{ledger: mustLedger(t)}
	if dir, why := r.RetainedGuestDir(res.ID); dir != res.Retained {
		t.Errorf("artifacts retry would not find it: %q, %s", dir, why)
	}
}

func mustLedger(t *testing.T) *Ledger {
	t.Helper()
	l, err := OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// TestStdinReachesACommandThatTravelsAsAFile is --stdin: the command is placed as
// a file, so stdin is free to carry the caller's bytes.
func TestStdinReachesACommandThatTravelsAsAFile(t *testing.T) {
	g := newExecFakeGuest(t)
	var stdout bytes.Buffer
	res, err := RunBaseExec(context.Background(), g.wsl, BaseExec{
		Distro: "wsl-toolkit", User: "toolkit", Dir: "/srv/there", Script: []byte("cat\n"),
		Env: map[string]string{"TK_X": "1"}, Stdin: strings.NewReader("piped\x00bytes"), Stdout: &stdout,
	})
	if err != nil || res.Exit != 0 {
		t.Fatalf("the call answered %d, %v", res.Exit, err)
	}
	if stdout.String() != "piped\x00bytes" {
		t.Fatalf("the command read %q from stdin", stdout.String())
	}
	file := execFakeHome + "/" + GuestRoot + "/jobs/" + res.ID + "/payload.sh"
	var placed, ranIt bool
	for _, c := range g.calls(t) {
		if body, ok := c.Files["payload.sh"]; ok && strings.HasPrefix(body, "TK_X='1'; export TK_X\n") && strings.Contains(body, "cd '/srv/there' || {") {
			placed = true
		}
		if len(c.Argv) == 2 && c.Argv[0] == "/bin/sh" && c.Argv[1] == file && c.Cd == "~" {
			ranIt = true
		}
	}
	if !placed || !ranIt {
		t.Fatalf("the command was placed %v and run from its file %v", placed, ranIt)
	}
}

// TestAnAttachedCommandTravelsFramedOnEveryPath is the 7.0 channel guard on the
// attached `base exec`: a command that reads stdin must not eat the lines of its
// own script. The home, a --dir the command enters, and --input each send the
// command framed, and the frame closes on its own delimiter. WSL-124.
func TestAnAttachedCommandTravelsFramedOnEveryPath(t *testing.T) {
	const marker = "echo tk-framed-marker\n"
	for _, c := range []struct {
		name string
		e    BaseExec
	}{
		{"the home", BaseExec{Dir: "~"}},
		{"a --dir", BaseExec{Dir: "/srv/there"}},
		{"an --input", BaseExec{Dir: "/srv/there", Inputs: []JobInput{
			{Name: "a.txt", Bytes: []byte("a")},
		}}},
	} {
		g := newExecFakeGuest(t)
		e := c.e
		e.Distro, e.User, e.Script = "wsl-toolkit", "toolkit", []byte(marker)
		if _, err := RunBaseExec(context.Background(), g.wsl, e); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		found := false
		for _, call := range g.calls(t) {
			if !strings.Contains(call.Stdin, marker) {
				continue
			}
			found = true
			_, rest, ok := strings.Cut(call.Stdin, "{ . /dev/fd/9; } 9<<'")
			delim, _, ok2 := strings.Cut(rest, "'")
			if !ok || !ok2 || delim == "" || !strings.HasSuffix(call.Stdin, marker+delim+"\n") {
				t.Errorf("%s: the command went unframed:\n%s", c.name, call.Stdin)
			}
		}
		if !found {
			t.Errorf("%s: no call carried the command", c.name)
		}
	}
}

// -- WSL-124: run --artifacts makes no empty directory, and lists what arrived ----

func TestAnEmptyOutputDirectoryMakesNoHostDirectory(t *testing.T) {
	g := newExecFakeGuest(t)
	dest := filepath.Join(t.TempDir(), "artifacts")
	got, err := g.wsl.FetchArtifacts(context.Background(), "wsl-toolkit", "toolkit", "/home/user/out", dest, DefaultWorkspaceLimits(), nil)
	if err != nil || got.Delivered != 0 {
		t.Fatalf("an empty set answered %+v, %v", got, err)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an empty /out made %s: %v", dest, err)
	}
	if line := ArtifactsLine(got, dest); line != "artifacts: 0 entries, 0 B written to "+dest {
		t.Errorf("the line for an empty set reads %q", line)
	}
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "a.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(execFakeOutEnv, out)
	got, err = g.wsl.FetchArtifacts(context.Background(), "wsl-toolkit", "toolkit", "/home/user/out", dest, DefaultWorkspaceLimits(), nil)
	if err != nil || got.Delivered != 1 || len(got.List) != 1 || got.List[0] != (ArtifactEntry{Path: "a.txt", Kind: "file", Bytes: 3}) {
		t.Fatalf("a set of one file answered %+v, %v", got, err)
	}
}

// TestARunListsItsArtifactsAndAnEmptyOutMakesNoDirectory drives Runner.Run, the
// job path `run` takes, through the fake wsl.exe.
func TestARunListsItsArtifactsAndAnEmptyOutMakesNoDirectory(t *testing.T) {
	g := newExecFakeGuest(t)
	home, err := EnsureHome()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: DefaultConfig(), wsl: g.wsl, home: home, ledger: mustLedger(t), log: func(string) {}}
	dest := filepath.Join(t.TempDir(), "artifacts")
	spec := JobSpec{Image: "docker.io/library/alpine:latest", Script: []byte("true\n"), ArtifactDir: dest,
		ContainerLifecycle: ContainerEphemeral, Platform: "linux/amd64", Limits: DefaultWorkspaceLimits()}
	res := r.Run(context.Background(), spec)
	if res.Unreached || res.Exit != 0 || res.Artifacts != 0 || res.ArtifactError != "" {
		t.Fatalf("a job that left /out empty answered %+v", res)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a job that left /out empty made %s: %v", dest, err)
	}
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "app"), []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(execFakeOutEnv, out)
	res = r.Run(context.Background(), spec)
	if res.Artifacts != 1 || len(res.ArtifactList) != 1 || res.ArtifactList[0] != (ArtifactEntry{Path: "app", Kind: "file", Bytes: 6}) || res.ArtifactsUnlisted != 0 {
		t.Fatalf("a job's answer lists %+v for %d artifacts", res.ArtifactList, res.Artifacts)
	}
}

// TestTheHelperRouteMakesNoDirectoryForAnEmptySet is the same rule through the
// helper: the client unpacks the set, and makes the directory only for an entry.
func TestTheHelperRouteMakesNoDirectoryForAnEmptySet(t *testing.T) {
	body := []byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	var empty bytes.Buffer
	tw := tar.NewWriter(&empty)
	_ = tw.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755})
	_ = tw.Close()
	body = empty.Bytes()
	c := &HelperClient{endpoint: HelperEndpoint{Address: strings.TrimPrefix(srv.URL, "http://"), Token: "t"}, http: srv.Client()}
	dest := filepath.Join(t.TempDir(), "artifacts")
	got, err := c.DownloadArtifacts(context.Background(), "0123456789abcdef", dest, DefaultWorkspaceLimits())
	if err != nil || got.Delivered != 0 {
		t.Fatalf("an empty set answered %+v, %v", got, err)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an empty set made %s through the helper: %v", dest, err)
	}
	body = listedTar(t, map[string]string{"dir/x": "xy"}, "l", "dir/x")
	got, err = c.DownloadArtifacts(context.Background(), "0123456789abcdef", dest, DefaultWorkspaceLimits())
	if err != nil || got.Delivered != 2 || len(got.List) != 2 {
		t.Fatalf("a set of a file and a link answered %+v, %v", got, err)
	}
}

// TestAnEmptySetThroughARealHelperIsAnsweredAndReleased runs a job that leaves
// /out empty through the helper's own set directory and its own artifacts
// handler. ⛔ The helper answered 404 for that set once the fetch stopped making
// the directory, so the client released nothing and the set stayed open. WSL-124.
func TestAnEmptySetThroughARealHelperIsAnsweredAndReleased(t *testing.T) {
	g := newExecFakeGuest(t)
	home, err := EnsureHome()
	if err != nil {
		t.Fatal(err)
	}
	r := &Runner{cfg: DefaultConfig(), wsl: g.wsl, home: home, ledger: mustLedger(t), log: func(string) {}}
	h := &HelperServer{runner: r, log: func(string) {}}
	id, err := newJobID()
	if err != nil {
		t.Fatal(err)
	}
	res := r.Run(context.Background(), JobSpec{Image: "docker.io/library/alpine:latest", Script: []byte("true\n"),
		ArtifactDir: h.artifactDir(id, true), ContainerLifecycle: ContainerEphemeral, Platform: "linux/amd64",
		Limits: DefaultWorkspaceLimits()})
	if res.Unreached || res.Exit != 0 || res.ArtifactError != "" {
		t.Fatalf("a job that left /out empty answered %+v", res)
	}
	var answered []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rec := httptest.NewRecorder()
		h.handleArtifacts(rec, req)
		answered = append(answered, fmt.Sprintf("%s %d", req.Method, rec.Code))
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	}))
	defer srv.Close()
	c := &HelperClient{endpoint: HelperEndpoint{Address: strings.TrimPrefix(srv.URL, "http://"), Token: "t"}, http: srv.Client()}
	dest := filepath.Join(t.TempDir(), "artifacts")
	got, err := c.DownloadArtifacts(context.Background(), id, dest, DefaultWorkspaceLimits())
	if err != nil || got.Delivered != 0 {
		t.Fatalf("an empty set through the helper answered %+v, %v", got, err)
	}
	if strings.Join(answered, ", ") != "GET 200, DELETE 200" {
		t.Fatalf("the helper answered %q for an empty set; it is fetched and then released", answered)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an empty set made %s through the helper: %v", dest, err)
	}
	if _, err := os.Stat(filepath.Join(home, "artifacts", id)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the helper kept the released set: %v", err)
	}
}

func TestTheArtifactListIsBoundedAndTheCountStaysExact(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for i := 0; i < MaxListedArtifacts+3; i++ {
		_ = tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("f%04d", i), Typeflag: tar.TypeReg, Size: 1, Mode: 0o644})
		_, _ = tw.Write([]byte("x"))
	}
	_ = tw.WriteHeader(&tar.Header{Name: "dir/", Typeflag: tar.TypeDir, Mode: 0o755})
	_ = tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "f0000"})
	_ = tw.Close()
	got, err := extractInto(&buf, t.TempDir(), DefaultWorkspaceLimits())
	if err != nil {
		t.Fatal(err)
	}
	if got.Delivered != MaxListedArtifacts+5 || len(got.List) != MaxListedArtifacts || got.Unlisted != 5 {
		t.Fatalf("delivered %d, listed %d, unlisted %d", got.Delivered, len(got.List), got.Unlisted)
	}
	small, err := extractInto(bytes.NewReader(listedTar(t, map[string]string{"dir/": "", "dir/x": "xy"}, "dir/l", "x")), t.TempDir(), DefaultWorkspaceLimits())
	if err != nil {
		t.Fatal(err)
	}
	want := []ArtifactEntry{{Path: "dir", Kind: "dir"}, {Path: "dir/x", Kind: "file", Bytes: 2}}
	if len(small.List) != 3 || small.List[0] != want[0] || small.List[1] != want[1] || small.List[2].Path != "dir/l.link.txt" || small.List[2].Kind != "link" {
		t.Fatalf("the list reads %+v", small.List)
	}
}

// listedTar is an archive of the given members, in name order, and one link.
func listedTar(t *testing.T, members map[string]string, link, target string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, name := range []string{"dir/", "dir/x"} {
		body, ok := members[name]
		if !ok {
			continue
		}
		if strings.HasSuffix(name, "/") {
			_ = tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeDir, Mode: 0o755})
			continue
		}
		_ = tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(body))})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.WriteHeader(&tar.Header{Name: link, Typeflag: tar.TypeSymlink, Linkname: target})
	_ = tw.Close()
	return buf.Bytes()
}

// -- WSL-123: an account of the base -------------------------------------------

func TestAnAccountIsAskedForBeforeTheCommandRuns(t *testing.T) {
	g := newExecFakeGuest(t)
	t.Setenv(execFakeUsersEnv, "alice:1001,toor:0")
	if uid, ok, err := AccountUID(context.Background(), g.wsl, "wsl-toolkit", "toolkit", "alice"); err != nil || !ok || uid != 1001 {
		t.Errorf("alice answered %d, %v, %v", uid, ok, err)
	}
	if uid, ok, err := AccountUID(context.Background(), g.wsl, "wsl-toolkit", "toolkit", "toor"); err != nil || !ok || uid != 0 {
		t.Errorf("toor answered %d, %v, %v", uid, ok, err)
	}
	if _, ok, err := AccountUID(context.Background(), g.wsl, "wsl-toolkit", "toolkit", "bob"); err != nil || ok {
		t.Errorf("an account the base lacks answered %v, %v", ok, err)
	}
	before := len(g.calls(t))
	if _, _, err := AccountUID(context.Background(), g.wsl, "wsl-toolkit", "toolkit", "x'; rm -rf /"); err == nil {
		t.Error("a name that is not an account's was asked about")
	}
	if len(g.calls(t)) != before {
		t.Error("a refused name still reached the guest")
	}
}

func TestAnAccountNameIsAnAccountsName(t *testing.T) {
	for _, ok := range []string{"alice", "_svc", "build-bot", "a.b", "Admin2"} {
		if err := ValidateBaseAccountName(ok); err != nil {
			t.Errorf("%q was refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "1abc", "-x", "a b", "a'b", "a$", strings.Repeat("a", 33), ".hidden"} {
		if err := ValidateBaseAccountName(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

// -- WSL-124: the detached session ------------------------------------------------

func TestASessionsOneArchiveCarriesItsInputsAndItsStdin(t *testing.T) {
	d := "/home/user/" + GuestRoot + "/sessions/0123456789abcdef"
	files := sessionFiles(d, SessionStart{
		Payload: []byte("A='1'; export A\ncat\n"),
		Inputs: []JobInput{
			{Name: "x/app.toml", Bytes: []byte("t")},
		},
		ArtifactDir: `C:\out`, Stdin: []byte{},
	})
	if len(files) != 3 || files[0].member() != "payload.sh" || files[1].member() != "in/x/app.toml" || files[2].member() != "stdin" {
		t.Fatalf("the archive is %+v", files)
	}
	want := ExecInEnv + "='" + d + "/in'; export " + ExecInEnv + "\n" + ExecOutEnv + "='" + d + "/out'; export " + ExecOutEnv + "\nA='1'; export A\ncat\n"
	if string(files[0].Bytes) != want {
		t.Fatalf("the payload is\n%q\nwant\n%q", files[0].Bytes, want)
	}
	if plain := sessionFiles(d, SessionStart{Payload: []byte("true\n")}); len(plain) != 1 || string(plain[0].Bytes) != "true\n" {
		t.Errorf("a session that asked for none of it got %+v", plain)
	}
	launch := string(sessionLaunchScript(d, "~", 0, time.Second))
	if !strings.Contains(launch, `i=/dev/null; if [ -f "$d/stdin" ]; then i="$d/stdin"; fi`) || !strings.Contains(launch, `setsid /bin/sh "$d/payload.sh" <"$i" `) {
		t.Fatalf("the supervisor does not give the payload the session's stdin:\n%s", launch)
	}
}

func TestASessionsOutputIsDeliveredOnce(t *testing.T) {
	g := newExecFakeGuest(t)
	home, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "r.txt"), []byte("result"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(execFakeOutEnv, out)
	id := "0123456789abcdef"
	dest := filepath.Join(t.TempDir(), "dest")
	rec := SessionRecord{Schema: SessionSchema, ID: id, Distro: "wsl-toolkit", User: "toolkit",
		GuestDir: execFakeHome + "/" + GuestRoot + "/sessions/" + id, ArtifactDir: dest}
	if err := writeSessionRecord(home, rec); err != nil {
		t.Fatal(err)
	}
	zero := 0
	first := DeliverSessionArtifacts(context.Background(), g.wsl, home, rec, SessionState{Exit: &zero})
	if first == nil || first.Error != "" || first.Delivered != 1 || first.Earlier {
		t.Fatalf("the first delivery is %+v", first)
	}
	if err := os.Remove(filepath.Join(dest, "r.txt")); err != nil {
		t.Fatal(err)
	}
	second := DeliverSessionArtifacts(context.Background(), g.wsl, home, rec, SessionState{Exit: &zero})
	if second == nil || !second.Earlier || second.Delivered != 1 {
		t.Fatalf("the second delivery is %+v", second)
	}
	if _, err := os.Stat(filepath.Join(dest, "r.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a second wait wrote the output again over what the caller did with it")
	}
	if none := DeliverSessionArtifacts(context.Background(), g.wsl, home, SessionRecord{ID: id}, SessionState{}); none != nil {
		t.Errorf("a session that asked for nothing delivered %+v", none)
	}
	gone := rec
	gone.ID = "fedcba9876543210"
	if d := DeliverSessionArtifacts(context.Background(), g.wsl, home, gone, SessionState{Gone: true}); d == nil || d.Error == "" {
		t.Errorf("a session whose guest is gone delivered %+v", d)
	}
}

// -- WSL-133: the private shell's launcher ----------------------------------------

func TestThePrivateShellsLauncherRemovesItselfAndCarriesTheWrapper(t *testing.T) {
	g := newExecFakeGuest(t)
	path, err := PrivateShell(context.Background(), g.wsl, "wsl-toolkit", "toolkit", []byte("exec bash -l\n"), "172.23.96.1")
	if err != nil {
		t.Fatal(err)
	}
	dir := strings.TrimSuffix(path, "/"+privateShellName)
	if !strings.HasPrefix(dir, execFakeHome+"/"+GuestRoot+"/shells/") || dir == path {
		t.Fatalf("the launcher is at %q", path)
	}
	var body string
	swept, placed := -1, -1
	for i, c := range g.calls(t) {
		if b, ok := c.Files[privateShellName]; ok {
			body = b
		}
		if strings.Contains(c.Stdin, string(staleShellSweep(dir[:strings.LastIndex(dir, "/")]))) && swept < 0 {
			swept = i
		}
		if len(c.Argv) == 3 && c.Argv[0] == "/bin/mkdir" && c.Argv[2] == dir {
			placed = i
		}
	}
	// ⛔ A LAUNCHER NO SHELL RAN IS SWEPT BY THE NEXT CALL, before it places its own.
	if swept < 0 || placed < swept {
		t.Fatalf("the stale launchers under %s are swept at call %d and the launcher placed at call %d", dir[:strings.LastIndex(dir, "/")], swept, placed)
	}
	if !strings.HasPrefix(body, "rm -rf '"+dir+"'\n") {
		t.Fatalf("the launcher does not remove its own directory first:\n%.200s", body)
	}
	if !strings.Contains(body, "TK_HOSTADDR='172.23.96.1'; export TK_HOSTADDR") || !strings.Contains(body, "TK_PRIVATE_NET_PAYLOAD_B64=") ||
		!strings.Contains(body, "exec pasta --quiet --config-net") {
		t.Fatalf("the launcher does not carry the wrapper and its variables:\n%.400s", body)
	}
}

// TestTheWrappersPayloadRemovesTheWorkDirectory: `exec pasta` replaces the
// wrapper's shell, so its trap never ran and every call left a directory in /tmp
// holding the payload. The payload's first line removes it now.
func TestTheWrappersPayloadRemovesTheWorkDirectory(t *testing.T) {
	s := string(privateNetScript)
	first := strings.Index(s, `printf '%s\n' 'rm -rf -- "${0%/*}"'; cat "$WORK/payload.sh"; } > "$WORK/enter.sh.payload"`)
	if first < 0 || strings.Contains(s, `cp "$WORK/payload.sh" "$WORK/enter.sh.payload"`) {
		t.Fatal("the payload the namespace runs does not remove the wrapper's work directory first")
	}
	if strings.LastIndex(s, "exec pasta --quiet") < first {
		t.Fatal("the payload is written after pasta starts")
	}
}
