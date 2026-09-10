// sweep_test.go - every `--json` surface reaches the sweep that checks them.
//
// ⛔ THE GUARD THAT WAS ITSELF A HAND-WRITTEN LIST. `TOOL-17` built a sweep in
// acceptance.ps1 asserting that every surface advertising `--json` puts exactly
// one parsable document on stdout, because commands were being added with that
// defect faster than cases were being written for them. The sweep's own list of
// surfaces is typed into a PowerShell array, so the very next `--json` surface
// this repository added - `inspect` - was not in it, and nothing said so.
//
// ⭐ THE LIST IS NOW A DECISION RATHER THAN A MEMORY. This walks the binary's
// real flag sets, and a surface that defines `--json` must either appear in the
// sweep or be exempted BELOW, by name, with the reason. An omission is a line
// somebody wrote rather than a line nobody noticed.
//
// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"flag"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// acceptancePath is the suite whose sweep this checks.
const acceptancePath = "acceptance.ps1"

// sweptElsewhere are `--json` surfaces the sweep deliberately does not call,
// each with the reason. ⛔ Name the surface, not a prefix: an exemption written
// as "base " would grant itself to whatever `base` subcommand lands there next.
var sweptElsewhere = map[string]string{
	"base recreate":   "it destroys and rebuilds the base, which every later case needs",
	"base remove":     "it unregisters the base. The cleanup case at the end owns that path",
	"base shell":      "it attaches an interactive shell and produces no document at all",
	"helper serve":    "it starts a process. `helper status` is the readable surface and is swept",
	"helper stop":     "the same, from the other end",
	"selfupdate":      "it reaches the network and can replace this executable",
	"matrix":          "it runs a fleet. Its own cases cover the document it produces",
	"images pull":     "it reaches a registry. Its own case covers the unreachable answer",
	"images warm":     "its own case covers it, and asserts nothing was pulled",
	"artifacts retry": "its own case covers it, against a job whose transfer really failed",
	"config validate": "its own case covers it, including the refusal",
	"ready":           "its own case covers it, and it is the longest-running survey here",
	"examples":        "its own case covers it, and asserts every example parses as one command",
}

var sweepName = regexp.MustCompile(`(?m)^\s*@\{\s*n\s*=\s*'([^']+)'`)

func TestEveryJSONSurfaceReachesTheSweep(t *testing.T) {
	body, err := os.ReadFile(acceptancePath)
	if err != nil {
		t.Fatalf("the acceptance suite could not be read: %v", err)
	}
	swept := map[string]bool{}
	for _, m := range sweepName.FindAllStringSubmatch(string(body), -1) {
		swept[m[1]] = true
	}
	if len(swept) < 5 {
		t.Fatalf("only %d sweep row(s) were found in %s, so this case is checking almost nothing", len(swept), acceptancePath)
	}

	for _, name := range jsonSurfaces(t) {
		if swept[name] || sweptElsewhere[name] != "" {
			continue
		}
		t.Errorf("%s takes --json and the sweep in %s does not call it. Add a row, or a reason to sweptElsewhere", name, acceptancePath)
	}

	// ⛔ AND AN EXEMPTION FOR A SURFACE THAT NO LONGER EXISTS IS DELETED, not
	// left. An exemption list nobody prunes grants itself to whatever takes the
	// name next, which is the class DOC-04 closed for directory prefixes.
	have := map[string]bool{}
	for _, n := range jsonSurfaces(t) {
		have[n] = true
	}
	var stale []string
	for name := range sweptElsewhere {
		if !have[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("sweptElsewhere names %s, which take no --json flag. Delete the row rather than keeping it", strings.Join(stale, ", "))
	}
}

// jsonSurfaces builds every command's flag set and returns the names of those
// that define `--json`.
//
// ⚠ SUBCOMMANDS ARE NAMED BY HAND, for the reason manual_test.go carries: their
// flag sets are built inside their parent's dispatch and there is no table to
// walk. A subcommand missing from the list below is invisible to this case, and
// that is the residue of the same hole rather than a closed one.
func jsonSurfaces(t *testing.T) []string {
	t.Helper()
	ctx := context.Background()
	quiet = true
	defer func() { quiet = false }()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	realErr := os.Stderr
	os.Stderr = devnull
	defer func() { os.Stderr = realErr }()

	for name, call := range commands {
		// `script` forwards its arguments to the embedded PowerShell verbatim,
		// so -h there would start a process. It registers no flag set.
		if name == "script" {
			continue
		}
		_, _ = call(ctx, []string{"-h"})
	}
	for _, sub := range [][]string{
		{"base", "ensure"}, {"base", "status"}, {"base", "recreate"}, {"base", "remove"},
		{"base", "shell"}, {"base", "presets"},
		{"helper", "serve"}, {"helper", "status"}, {"helper", "stop"},
		{"images", "warm"}, {"images", "pull"},
		{"artifacts", "retry"}, {"config", "validate"},
	} {
		args := append(append([]string{}, sub[1:]...), "-h")
		switch sub[0] {
		case "base":
			_, _ = cmdBase(ctx, args)
		case "helper":
			_, _ = cmdHelper(ctx, args)
		case "images":
			_, _ = cmdImages(ctx, args)
		case "artifacts":
			_, _ = cmdArtifacts(ctx, args)
		case "config":
			_, _ = cmdConfig(args)
		}
	}
	os.Stderr = realErr

	flagSets.Lock()
	defer flagSets.Unlock()
	var out []string
	for name, fs := range flagSets.byName {
		// ⚠ A name ending in "-h" is this walk's own artefact: passing -h as a
		// SUBCOMMAND makes the parent build a flag set called "base -h". It is
		// not a surface anybody can call.
		if strings.HasSuffix(name, " -h") {
			continue
		}
		fs.VisitAll(func(f *flag.Flag) {
			if f.Name == "json" {
				out = append(out, name)
			}
		})
	}
	sort.Strings(out)
	return out
}
