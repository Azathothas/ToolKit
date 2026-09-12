package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNativeInventoryCoversStandaloneProbe(t *testing.T) {
	b, err := os.ReadFile("../../../../../scripts/doctor/doctor.ps1")
	if err != nil {
		t.Fatal(err)
	}
	rows := regexp.MustCompile(`(?m)^\s*@\('([^']+)','([^']+)','([^']+)',@\(`).FindAllSubmatch(b, -1)
	want := map[string]string{}
	for _, row := range rows {
		want[string(row[1])] = string(row[2]) + "/" + string(row[3])
	}
	if len(want) < 80 {
		t.Fatalf("parsed only %d legacy inventory rows", len(want))
	}
	catalog := ToolCatalog()
	if len(catalog) != len(want) {
		t.Fatalf("native=%d legacy=%d", len(catalog), len(want))
	}
	for _, row := range catalog {
		if want[row.ID] != row.Group+"/"+row.Binary {
			t.Errorf("inventory mismatch for %s", row.ID)
		}
	}
}

func TestResolveExecutableReturnsCanonicalPath(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ResolveExecutable(self)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(self)
	if err != nil || got.Resolved != want {
		t.Fatalf("got %q, expected %q: %v", got.Resolved, want, err)
	}
}

func TestMissingExecutableDoesNotBecomePresent(t *testing.T) {
	_, err := ResolveExecutable(filepath.Join(t.TempDir(), "missing.exe"))
	if err == nil {
		t.Fatal("missing executable was accepted")
	}
}

func TestScoopDescriptorResolvesRealExecutable(t *testing.T) {
	if runtime.GOOS != "windows" {
		return
	}
	dir := t.TempDir()
	shim := filepath.Join(dir, "tool.exe")
	actual := filepath.Join(dir, "actual.exe")
	for _, file := range []string{shim, actual} {
		if err := os.WriteFile(file, []byte("fixture"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "tool.shim"), []byte("path = \""+actual+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// ⚠ COMPARED AS CANONICAL PATHS. A CI runner's temporary directory is
	// the 8.3 short form of its account name and the resolver answers the long
	// one, so a string comparison of the two failed over a tool that was right.
	wantResolved, err := RealPath(actual)
	if err != nil {
		wantResolved = actual
	}
	got, err := ResolveExecutable(shim)
	if err != nil || got.Kind != "scoop-target" {
		t.Fatalf("%+v: %v", got, err)
	}
	if !pathEqual(got.Resolved, wantResolved) {
		t.Fatalf("resolved %q, expected %q", got.Resolved, wantResolved)
	}
}

func TestToolProbeContinuesAfterAnUnusablePathEntry(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	name := "probe"
	badBody, goodBody := "#!/bin/sh\nexit 7\n", "#!/bin/sh\necho probe 1.2.3\n"
	if runtime.GOOS == "windows" {
		name = "probe.cmd"
		badBody, goodBody = "@exit /b 7\r\n", "@echo probe 1.2.3\r\n"
	}
	bad := filepath.Join(first, name)
	good := filepath.Join(second, name)
	if err := os.WriteFile(bad, []byte(badBody), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(good, []byte(goodBody), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", strings.Join([]string{first, second}, string(os.PathListSeparator)))

	got := probeTool(context.Background(), ToolSpec{ID: "probe", Group: "test", Binary: "probe", Args: []string{"--version"}}, false)
	if got.Status != "working" || got.Version != "1.2.3" {
		t.Fatalf("the second executable was not used: %+v", got)
	}
	if !pathEqual(got.Path, good) {
		t.Fatalf("path = %q, want %q", got.Path, good)
	}
	if !strings.Contains(strings.Join(got.Notes, "\n"), bad) {
		t.Fatalf("the failed candidate is absent from the notes: %v", got.Notes)
	}
}

func TestCaptureMarksOverflowInsteadOfGrowing(t *testing.T) {
	b := &boundedBuffer{max: 4}
	if n, err := b.Write([]byte("abcdefgh")); n != 8 || err != nil {
		t.Fatalf("write=%d %v", n, err)
	}
	if b.String() != "abcd" || !b.truncated {
		t.Fatal("capture limit not enforced")
	}
}

func TestChildProcess(t *testing.T) {
	if os.Getenv("TOOLKIT_TEST_CHILD") != "1" {
		return
	}
	switch os.Getenv("TOOLKIT_TEST_MODE") {
	case "exit":
		os.Exit(37)
	case "wait":
		time.Sleep(30 * time.Second)
	}
}

func TestChildExitAndDeadlineReachCaller(t *testing.T) {
	t.Setenv("TOOLKIT_TEST_CHILD", "1")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOOLKIT_TEST_MODE", "exit")
	_, _, err = Output(context.Background(), self, "-test.run=^TestChildProcess$")
	if ExitCode(err) != 37 {
		t.Fatalf("child exit=%d: %v", ExitCode(err), err)
	}
	t.Setenv("TOOLKIT_TEST_MODE", "wait")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, _, err = Output(ctx, self, "-test.run=^TestChildProcess$")
	if ExitCode(err) != 124 {
		t.Fatalf("deadline exit=%d: %v", ExitCode(err), err)
	}
}
