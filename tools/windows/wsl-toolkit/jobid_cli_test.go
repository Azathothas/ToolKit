// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-120: ids and answers --------------------------------------------------

func selectInstance(t *testing.T, name string) {
	t.Helper()
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	toolkit.SelectedInstance = toolkit.Instance{Name: name, Distro: toolkit.InstanceDistro(name)}
}

// endedJob makes a job that ended with exit, and its stdout transcript.
func endedJob(t *testing.T, home, id string, exit int) {
	t.Helper()
	owner, err := toolkit.ClaimJob(home, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "jobs", id, "stdout.log"), []byte("from the job\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res := toolkit.JobResult{ID: id, Exit: exit}
	res.Seal()
	if err := owner.Finish(res); err != nil {
		t.Fatal(err)
	}
}

// ⭐ Every command that takes a job reads a prefix through the one resolver,
// before it reads anything else, so a prefix that names nothing is refused by
// each of them in the resolver's own words.
//
// ⚠ inspect, gc and resources pass --via-helper, and no helper answers in an
// empty home: a refusal that went missing fails at the absent helper in
// milliseconds rather than reaching a real base.
func TestEveryJobCommandReadsAPrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	selectInstance(t, toolkit.DefaultInstance)
	ctx := context.Background()
	for _, c := range []struct {
		name string
		run  func() (int, error)
	}{
		{"logs", func() (int, error) { return cmdLogs(ctx, []string{"beef1234"}) }},
		{"logs --follow", func() (int, error) { return cmdLogs(ctx, []string{"beef1234", "--follow"}) }},
		{"wait", func() (int, error) { return cmdWait(ctx, []string{"beef1234"}) }},
		{"stop", func() (int, error) { return cmdStop(ctx, []string{"wtk-beef1234"}) }},
		{"inspect", func() (int, error) { return cmdInspect(ctx, []string{"beef1234", "--via-helper"}) }},
		{"artifacts retry", func() (int, error) {
			return cmdArtifacts(ctx, []string{"retry", "beef1234", "--to", filepath.Join(home, "out")})
		}},
		{"gc --job", func() (int, error) { return cmdGC(ctx, []string{"--job", "beef1234", "--via-helper"}) }},
	} {
		code, err := c.run()
		if code != exitCannot || err == nil || !strings.Contains(err.Error(), "starts with it") || !strings.Contains(err.Error(), "logs") {
			t.Errorf("%s with a prefix of no job answered %d, %v; want exit 2 and the resolver's refusal", c.name, code, err)
		}
	}
	// ⛔ resources --job narrowed by any text and exited 0 before, so a prefix
	// that names no job here narrows as it did: here it reaches the absent
	// helper, and is not refused by the resolver.
	code, err := cmdResources(ctx, []string{"--job", "beef1234", "--via-helper"})
	if err == nil || strings.Contains(err.Error(), "starts with it") {
		t.Errorf("resources --job with a prefix of no job answered %d, %v; want it to narrow as before", code, err)
	}
}

// ⭐ A prefix and a container's name reach the job they name.
func TestAPrefixReachesTheJobItNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	selectInstance(t, toolkit.DefaultInstance)
	endedJob(t, home, cliJobID, 3)
	code, err := cmdWait(context.Background(), []string{"0123"})
	if code != 3 || err != nil {
		t.Fatalf("wait 0123 answered %d, %v; want the job's own 3", code, err)
	}
	var got int
	out := captureStdout(t, func() { got, err = cmdLogs(context.Background(), []string{"wtk-" + cliJobID[:6]}) })
	if got != exitOK || err != nil || out != "from the job\n" {
		t.Fatalf("logs wtk-%s answered %d, %v and wrote %q", cliJobID[:6], got, err, out)
	}
}

// ⛔ A job another instance ran is refused with the exact command that reaches
// it, the caller's own flags and the job's full id included.
func TestAJobOfAnotherInstanceIsAnsweredWithTheCommandThatReachesIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	selectInstance(t, "podbox")
	endedJob(t, home, cliJobID, 0)
	selectInstance(t, toolkit.DefaultInstance)
	code, err := cmdStop(context.Background(), []string{"0123", "--grace", "5s"})
	want := "wsl-toolkit --instance podbox stop " + cliJobID + " --grace 5s"
	if code != exitCannot || err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("stop answered %d, %v; want %q", code, err, want)
	}
}

func TestACommandIsPrintedAgainWithTheFullID(t *testing.T) {
	got := commandWords(withJobID([]string{"--job=0123", "--to", `C:\out dir`, "0123"}, "0123", cliJobID))
	if want := "--job=" + cliJobID + ` --to 'C:\out dir' ` + cliJobID; got != want {
		t.Errorf("the command reads %q; want %q", got, want)
	}
}

// ⭐ --json-compact and WSL_TOOLKIT_JSON=compact put an answer on one line, and
// the default stays indented.
func TestAnAnswerIsOneLineWhenAskedFor(t *testing.T) {
	t.Cleanup(func() { jsonCompact = false })
	lines := func(args ...string) (string, int) {
		jsonCompact = false
		var code int
		out := captureStdout(t, func() { code = run(context.Background(), args) })
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("%v wrote %q, which is not one document", args, out)
		}
		return out, code
	}
	if out, _ := lines("--json-compact", "version", "--json"); strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "}\n") {
		t.Errorf("--json-compact wrote %q", out)
	}
	if out, _ := lines("version", "--json"); strings.Count(out, "\n") < 3 {
		t.Errorf("the default answer is not indented: %q", out)
	}
	t.Setenv(jsonEnv, "compact")
	if out, _ := lines("version", "--json"); strings.Count(out, "\n") != 1 {
		t.Errorf("%s=compact wrote %q", jsonEnv, out)
	}
	// ⛔ ANY OTHER VALUE IS SAID AND NOT REFUSED. The variable was read by nothing
	// before, so a refusal made every command exit 2 in a shell that set it.
	t.Setenv(jsonEnv, "pretty")
	if out, code := lines("version", "--json"); code != exitOK || strings.Count(out, "\n") < 3 {
		t.Errorf("%s=pretty answered %d and wrote %q; want exit 0 and the indented default", jsonEnv, code, out)
	}
	if err := readJSONEnv(); err == nil || !strings.Contains(err.Error(), "stays indented") {
		t.Errorf("%s=pretty is said as %v", jsonEnv, err)
	}
}
