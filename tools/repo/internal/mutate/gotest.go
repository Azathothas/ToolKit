// gotest.go - what one `go test -json` run said, read event by event.
//
// ⛔ THE EXIT CODE IS NOT THE VERDICT. `go test` exits 1 when ANY package it was
// handed fails. Under `./...` that includes a sibling package the mutation
// stopped compiling, while every case the row names passed. The harness read
// that exit as "went red" and certified a guard nobody had tested. The events
// say which package failed and which case, so the verdict is read from them.
//
// ⚠ A CASE THAT NEVER FINISHED IS A CASE THAT FAILED, when its package failed.
// A panic on another goroutine, an os.Exit in the code under test and the
// -timeout panic all kill the binary. None of them sends a `fail` event for the
// case. Only cases the -run pattern selected were running, so a binary that
// died mid-case died in one of them.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"encoding/json"
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
	// failed counts named cases that reported fail, or started and never
	// finished in a package whose binary failed.
	failed int
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
	for pkg := range failedPkg {
		r.failed += len(open[pkg])
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

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
