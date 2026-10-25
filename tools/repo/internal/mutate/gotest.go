// gotest.go - what one `go test -json` run said, read event by event.
//
// ⛔ THE EXIT CODE IS NOT THE VERDICT. `go test` exits 1 when ANY package it was
// handed fails. Under `./...` that includes a sibling package the mutation
// stopped compiling, while every case the row names passed. The harness read
// that exit as "went red" and certified a guard nobody had tested. The events
// say which package failed and which case, so the verdict is read from them.
//
// ⛔ A BINARY THAT DIED WHILE A CASE RAN DID NOT NECESSARILY DIE IN IT. A panic
// on another goroutine, an os.Exit and the -timeout panic all kill the binary.
// None of them sends a `fail` event for the case. This file used to count every
// case still open as failed. A goroutine that init started could panic while a
// case slept, and the row was certified for a guard the case never reached. So
// an open case counts as failed only when the death names it. Either the dying
// goroutine, or one that started it, ran in testing.tRunner, or the -timeout
// panic lists the case as running. Any other death leaves the case untied, and
// the row is BROKEN with the death's own line.
//
// ⚠ THE ANCESTRY COMES FROM GODEBUG=tracebackancestors, which pool.go sets for
// every test process. Without it a goroutine the case started through the code
// under test shows only its creator, and a real red reads as untied.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// testEvent is one line of `go test -json`. ⚠ Build output arrives on the same
// stream since Go 1.24, as build-output and build-fail actions with ImportPath
// set rather than Package.
type testEvent struct {
	Action  string
	Package string
	Test    string
	Output  string
}

// testRun is what one run reported.
type testRun struct {
	// cases counts every case and subtest that started, and skipped those
	// that skipped themselves. ⚠ Both, for the reason Verdict holds both.
	cases, skipped int
	// failed counts named cases that reported fail, or were open when their
	// binary died and the death is tied to them.
	failed int
	// untied are the named cases open when their binary died with nothing
	// tying the death to them, and death is the line it died with.
	untied []string
	death  string
	// pkgs are the packages a named case ran in, sorted.
	pkgs []string
	// buildFailed are the packages whose test binary did not build.
	buildFailed []string
	// pkgFailed are the packages that failed for any reason.
	pkgFailed []string
	// text is what a reader needs: build output, then anything that was not
	// an event, then the output of the packages that failed.
	text string
}

// readTestRun parses stdout as events and keeps stderr as text.
//
// ⚠ A LINE THAT IS NOT AN EVENT IS KEPT AS TEXT, not dropped. An older
// toolchain, or GODEBUG=gotestjsonbuildtext=1, prints the compiler's errors as
// plain lines, and those lines are the reason a BROKEN row has to carry.
func readTestRun(stdout, stderr string) testRun {
	var r testRun
	var build, raw strings.Builder
	output := map[string]*strings.Builder{}
	open := map[string]map[string]bool{}
	ran := map[string]bool{}
	broke := map[string]bool{}
	failedPkg := map[string]bool{}
	for _, ln := range strings.Split(stdout, "\n") {
		ln = strings.TrimRight(ln, "\r")
		if strings.TrimSpace(ln) == "" {
			continue
		}
		var e testEvent
		if !strings.HasPrefix(ln, "{") || json.Unmarshal([]byte(ln), &e) != nil || e.Action == "" {
			raw.WriteString(ln + "\n")
			continue
		}
		switch e.Action {
		case "build-output":
			build.WriteString(e.Output)
			continue
		case "build-fail":
			continue
		}
		if output[e.Package] == nil {
			output[e.Package] = &strings.Builder{}
		}
		if open[e.Package] == nil {
			open[e.Package] = map[string]bool{}
		}
		named := strings.HasPrefix(e.Test, "Test")
		switch e.Action {
		case "output":
			output[e.Package].WriteString(e.Output)
			// ⭐ THE PACKAGE'S OWN FAIL LINE SAYS IT DID NOT BUILD, on every
			// toolchain. The FailedBuild field says so too, but only since Go
			// 1.24, and two ways to learn one fact is one nobody can mutate.
			if e.Test == "" && (strings.Contains(e.Output, "[build failed]") || strings.Contains(e.Output, "[setup failed]")) {
				broke[e.Package] = true
			}
		case "run":
			if named {
				r.cases++
				ran[e.Package] = true
				open[e.Package][e.Test] = true
			}
		case "skip", "pass", "fail":
			if e.Test == "" {
				if e.Action == "fail" {
					failedPkg[e.Package] = true
				}
				continue
			}
			delete(open[e.Package], e.Test)
			if !named {
				continue
			}
			switch e.Action {
			case "skip":
				r.skipped++
			case "fail":
				r.failed++
			}
		}
	}
	for _, pkg := range sortedKeys(failedPkg) {
		still := sortedKeys(open[pkg])
		if len(still) == 0 || output[pkg] == nil {
			continue
		}
		tied, line := diedIn(output[pkg].String(), still)
		r.failed += len(tied)
		if len(tied) == 0 {
			r.untied = append(r.untied, still...)
			r.death = line
		}
	}
	r.pkgs = sortedKeys(ran)
	r.buildFailed = sortedKeys(broke)
	r.pkgFailed = sortedKeys(failedPkg)

	var text strings.Builder
	text.WriteString(build.String())
	text.WriteString(raw.String())
	text.WriteString(stderr)
	for _, pkg := range r.pkgFailed {
		if b := output[pkg]; b != nil {
			text.WriteString(b.String())
		}
	}
	r.text = text.String()
	return r
}

// goroutineHeader opens one goroutine's block in a trace.
//
// ⚠ A RUNTIME ERROR ADDS gp=, m= AND mp= TO THE HEADER, as GOTRACEBACK=system
// does. A stack overflow is one, and it is what deleting a recursion's base
// case gives. A header read without them missed the case's own goroutine, and
// a real red read as untied.
var goroutineHeader = regexp.MustCompile(`^goroutine \d+ (?:gp=\S+ m=\S+ (?:mp=\S+ )?)?\[`)

// diedIn reads a dead binary's output. It returns the open cases the death is
// tied to, and the line the binary died with.
//
// ⚠ THE LAST DEATH LINE AT THE LEFT MARGIN. A case can log text that reads like
// a panic, and t.Log indents it. A nested panic indents its inner lines.
func diedIn(out string, open []string) (tied []string, death string) {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	at := -1
	for i, ln := range lines {
		if strings.HasPrefix(ln, "panic: ") || strings.HasPrefix(ln, "fatal error: ") {
			at = i
		}
	}
	if at < 0 {
		// ⛔ NO TRACE AT ALL is an os.Exit, or a kill from outside the binary.
		// Nothing in the output says where it came from.
		return nil, ""
	}
	death = lines[at]
	if strings.HasPrefix(death, "panic: test timed out after ") {
		return timedOut(lines[at+1:], open), death
	}
	// ⛔ ONLY THE FIRST GOROUTINE AFTER THE DEATH LINE IS READ, because it is the
	// one that died. Some deaths print every goroutine, and the main goroutine
	// sits in testing.tRunner for as long as any case runs.
	for i := at + 1; i < len(lines); i++ {
		if !goroutineHeader.MatchString(lines[i]) {
			continue
		}
		for _, ln := range lines[i+1:] {
			if strings.TrimSpace(ln) == "" {
				break
			}
			if strings.HasPrefix(ln, "testing.tRunner(") {
				return open, death
			}
		}
		break
	}
	return nil, death
}

// timedOut reads the -timeout panic's list of running tests and keeps the open
// cases it names.
func timedOut(after, open []string) []string {
	running := map[string]bool{}
	listing := false
	for _, ln := range after {
		t := strings.TrimSpace(ln)
		if !listing {
			listing = t == "running tests:"
			continue
		}
		if t == "" {
			break
		}
		name, _, _ := strings.Cut(t, " (")
		running[name] = true
	}
	var tied []string
	for _, c := range open {
		if running[c] {
			tied = append(tied, c)
		}
	}
	return tied
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
