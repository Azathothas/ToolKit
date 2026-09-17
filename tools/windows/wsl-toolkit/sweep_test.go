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
	"errors"
	"flag"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestAskingTheBaseGroupForHelpIsNotAFailure holds the contract a weak agent
// depends on.
//
// ⛔ `base --help` EXITED 2 AND SAID `"--help" is not a base subcommand`.
// The top level answers --help with 0 and so does every subcommand in this
// group, so the one spelling a reader reaches for first was the one that
// looked like a failure. `skills/wsl-toolkit` tells an agent to run
// `wsl-toolkit base --help` AND to read the exit code from the process, which
// together told it its correct command had failed.
//
// ⚠ `base` WITH NO ARGUMENTS STAYS 2, and that is not the same question:
// nothing was asked, so "could not run" is the honest answer. The case asserts
// both so a fix to one does not quietly change the other.
func TestAskingTheBaseGroupForHelpIsNotAFailure(t *testing.T) {
	for _, spelling := range []string{"--help", "-h", "help"} {
		code, err := cmdBase(context.Background(), []string{spelling})
		if code != exitOK {
			t.Errorf("base %s exited %d, and asking for help is not a failure", spelling, code)
		}
		if !errors.Is(err, flag.ErrHelp) {
			t.Errorf("base %s returned %v, and the dispatcher reads flag.ErrHelp to mean help was asked for", spelling, err)
		}
	}
	if code, _ := cmdBase(context.Background(), nil); code != exitCannot {
		t.Errorf("base with no arguments exited %d, and nothing was asked, so it could not run", code)
	}
}

// TestTheBaseUsageAndTheManualNameTheSameSubcommands closes the gap finding 39
// named and left open.
//
// ⛔ TWO LISTS OF THE SAME SUBCOMMANDS, WRITTEN FROM MEMORY AT DIFFERENT
// TIMES, AND NOTHING COMPARED THEM. `baseUsage` in cmd_base.go is what a reader
// of `wsl-toolkit base` sees; the `base` command spec's HelpForms is what
// reaches wsl-toolkit.1 and `man`. They disagreed in BOTH directions: `herdr`
// and `bootstrap` were in the usage and in no manual, so neither had a flag
// documented anywhere, and `agent` was in the manual and in no usage, so a
// reader of the group never learned it exists.
//
// ⚠ IT READS THE ANGLE-BRACKET LIST, which is the line a reader actually
// sees, rather than the descriptions under it. A subcommand described below the
// line but missing from it is still missing from the sentence that lists them.
func TestTheBaseUsageAndTheManualNameTheSameSubcommands(t *testing.T) {
	m := regexp.MustCompile(`^wsl-toolkit base <([^>]+)>`).FindStringSubmatch(baseUsage)
	if m == nil {
		t.Fatal("baseUsage does not open with `wsl-toolkit base <a|b|c>`, so nothing can be read from it")
	}
	inUsage := map[string]bool{}
	for _, name := range strings.Split(m[1], "|") {
		inUsage[strings.TrimSpace(name)] = true
	}

	inManual := map[string]bool{}
	for _, spec := range registeredCommandSpecs() {
		if spec.Name != "base" {
			continue
		}
		for _, form := range spec.HelpForms {
			inManual[strings.TrimPrefix(form, "base ")] = true
		}
	}
	if len(inManual) == 0 {
		t.Fatal("the base command spec registers no help form, so this case is asserting nothing")
	}

	// ⭐ BOTH DIRECTIONS. Each one was the real defect once.
	var missingFromManual, missingFromUsage []string
	for name := range inUsage {
		if !inManual[name] {
			missingFromManual = append(missingFromManual, name)
		}
	}
	for name := range inManual {
		if !inUsage[name] {
			missingFromUsage = append(missingFromUsage, name)
		}
	}
	sort.Strings(missingFromManual)
	sort.Strings(missingFromUsage)
	if len(missingFromManual) > 0 {
		t.Errorf("baseUsage names %v, which the base HelpForms do not, so they reach no manual and no flag of theirs is documented", missingFromManual)
	}
	if len(missingFromUsage) > 0 {
		t.Errorf("the base HelpForms name %v, which baseUsage does not, so a reader of `wsl-toolkit base` never learns they exist", missingFromUsage)
	}
}

// acceptancePath is the suite whose sweep this checks.
const acceptancePath = "acceptance.ps1"

// sweptElsewhere are `--json` surfaces the sweep deliberately does not call,
// each with the reason. ⛔ Name the surface, not a prefix: an exemption written
// as "base " would grant itself to whatever `base` subcommand lands there next.
var sweptElsewhere = map[string]string{
	"base recreate":   "it destroys and rebuilds the base, which every later case needs",
	"base remove":     "it unregisters the base. The cleanup case at the end owns that path",
	"base shell":      "it attaches an interactive shell and produces no document at all",
	"base grant":      "it mounts a Windows directory into the base and writes the configuration, which the sweep's shared base must not gain",
	"base revoke":     "it unmounts a grant and writes the configuration, and the sweep's shared base has none to take away",
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
	"bsd fetch":       "it downloads 635 MB from a release mirror. `bsd status` reports whether the image arrived and is swept",
	"bsd run":         "it boots a guest from the image `bsd fetch` downloads, which not every acceptance host holds",
	"distro new":      "it imports a whole distribution. Its own case covers the document it produces",
	"distro run":      "it runs a command in a distribution. Its own case covers the document, against one it made",
	"distro enter":    "its only document is the --dry-run plan of an interactive shell, and its own case covers that plan",
	"distro remove":   "it unregisters a distribution. Its own case covers the document, against one it made",
	"distro snapshot": "it exports a whole distribution. Its own case covers the document",
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

	// ⛔ ITS OWN REGISTRY, BECAUSE THE VERDICT USED TO DEPEND ON WHAT RAN
	// FIRST. flagSets is package level and this walk ADDED to whatever was
	// already in it. A manual case resets and repopulates it, so with one
	// running first every surface was registered and this answered correctly;
	// run alone, `go test . -run TestEveryJSONSurfaceReachesTheSweep` reported
	// four rows of sweptElsewhere as stale and sent a session looking for a
	// defect a full run does not have. Measured 2026-09-17, both ways.
	//
	// ⚠ A CASE THAT IS RIGHT ONLY IN COMPANY IS NOT RIGHT. CI and the gate run
	// the full package, so it never fired there, which is exactly what made it
	// cost a session rather than a commit.
	flagSets.Lock()
	previous := flagSets.byName
	flagSets.byName = map[string]*flag.FlagSet{}
	flagSets.Unlock()
	defer func() {
		flagSets.Lock()
		flagSets.byName = previous
		flagSets.Unlock()
	}()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	realErr := os.Stderr
	os.Stderr = devnull
	defer func() { os.Stderr = realErr }()

	for _, call := range commands {
		_, _ = call(ctx, []string{"-h"})
	}
	// ⛔ EVERY SUBCOMMAND sweptElsewhere NAMES IS BUILT HERE. It used to name
	// four that this walk never built - `base grant`, `base revoke`,
	// `bsd fetch` and `bsd run` - so they could never be seen to take --json,
	// and the case reported its own exemptions as stale. The completeness
	// assertion below is what turns a missing name into a refusal rather than
	// a silent gap.
	for _, sub := range [][]string{
		{"base", "ensure"}, {"base", "status"}, {"base", "recreate"}, {"base", "remove"},
		{"base", "shell"}, {"base", "presets"}, {"base", "grant"}, {"base", "revoke"},
		{"helper", "serve"}, {"helper", "status"}, {"helper", "stop"},
		{"images", "warm"}, {"images", "pull"},
		{"artifacts", "retry"}, {"config", "validate"},
		{"bsd", "status"}, {"bsd", "fetch"}, {"bsd", "run"},
		{"distro", "list"}, {"distro", "new"}, {"distro", "run"}, {"distro", "enter"},
		{"distro", "remove"}, {"distro", "purge"}, {"distro", "snapshot"},
		{"distro", "replay"}, {"distro", "compare"},
	} {
		args := append(append([]string{}, sub[1:]...), "-h")
		switch sub[0] {
		case "distro":
			_, _ = cmdDistro(ctx, args)
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
		case "bsd":
			_, _ = cmdBsd(ctx, args)
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
