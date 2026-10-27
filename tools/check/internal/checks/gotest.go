// SPDX-License-Identifier: 0BSD

package checks

import (
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// ⭐ ONE PACKAGE'S CASES RUN IN SEVERAL PROCESSES AT ONCE. internal/toolkit holds
// about 900 cases, and one process took 150 s of them on the operator's host.
// Almost all of it was cases that set a variable and so cannot run beside each
// other in one process. In separate processes they can. TOOL-40.
//
// ⛔ SOUND ONLY FOR A PACKAGE WHOSE CASES KEEP THEIR STATE TO THEIR OWN PROCESS.
// That is package variables, the environment, the working directory, and the
// temporary directories a case makes. A case that wrote a fixed path, or found
// processes by a fixed mark, would meet a case of another process. So a package
// is named here only after that was read, and splitTests names the reading.
//
// ⛔ EVERY CASE RUNS EXACTLY ONCE. The cases are listed from the package's own
// binary, dealt to the processes as -run patterns, and each pattern is checked
// against the list. A case no pattern or two patterns match is a finding.

// splitTests are the packages whose cases run in several processes, by their
// directory under the repository root.
//
// ⭐ internal/toolkit: its cases start fakes from temporary directories of their
// own, and set variables with t.Setenv, which one process alone sees. Two whole
// runs of the package already run at once in the twins job, and a0ec0f6 fixed
// the one fake that found processes host-wide.
var splitTests = map[string]bool{
	"tools/windows/wsl-toolkit/internal/toolkit": true,
}

// testProcesses is how many processes a split package's cases run in: a quarter
// of the CPUs, at least two and at most four.
func testProcesses() int { return min(4, max(2, runtime.NumCPU()/4)) }

// missingSplits names each package split names that holds no test file under
// root.
//
// ⛔ A PACKAGE THAT MOVED WOULD RUN IN ONE PROCESS AGAIN, and nothing would say
// so. Its cases still pass, so only this finding tells.
func missingSplits(root string, split map[string]bool) []string {
	var found []string
	for dir := range split {
		cases, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*_test.go"))
		if err != nil || len(cases) == 0 {
			found = append(found, sprintf("go test: splitTests names %s, which holds no test file, so nothing runs in several processes", dir))
		}
	}
	sort.Strings(found)
	return found
}

// goTestModule runs go test over one module and returns what failed. The
// packages split names run in k processes each.
func goTestModule(root, goBin, m, dir string, split map[string]bool, k int) []string {
	out, err := runIn(dir, goBin, "list", "-f", "{{.ImportPath}}\t{{.Dir}}", "./...")
	if err != nil {
		return []string{sprintf("%s: go list: %s", m, firstFinding(out))}
	}
	var rest, apart []string
	for _, ln := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		pkg, at, ok := strings.Cut(ln, "\t")
		if !ok {
			continue
		}
		rel, err := filepath.Rel(root, at)
		if err == nil && split[filepath.ToSlash(rel)] {
			apart = append(apart, pkg)
			continue
		}
		rest = append(rest, pkg)
	}
	if len(apart) == 0 {
		rest = []string{"./..."}
	}
	// ⚠ ONE JOB FOR THE REST OF THE MODULE AND ONE PER SPLIT PACKAGE, at once,
	// as `go test ./...` runs its packages at once.
	jobs := make([][]string, 1+len(apart))
	each(len(jobs), func(i int) {
		if i == 0 {
			if len(rest) == 0 {
				return
			}
			if out, err := runIn(dir, goBin, append([]string{"test"}, rest...)...); err != nil {
				jobs[i] = []string{sprintf("%s: go test: %s", m, testFailure(out))}
			}
			return
		}
		jobs[i] = goTestSplit(goBin, m, dir, apart[i-1], k)
	})
	var found []string
	for _, j := range jobs {
		found = append(found, j...)
	}
	return found
}

// goTestSplit runs one package's cases in k processes at once.
//
// ⚠ THE LIST BUILDS THE BINARY FIRST, so the k runs find its compile in the
// cache rather than make k of their own.
func goTestSplit(goBin, m, dir, pkg string, k int) []string {
	out, err := runIn(dir, goBin, "test", "-list", ".", pkg)
	if err != nil {
		return []string{sprintf("%s: go test: %s", m, testFailure(out))}
	}
	names := listedCases(out)
	if len(names) == 0 {
		if out, err := runIn(dir, goBin, "test", pkg); err != nil {
			return []string{sprintf("%s: go test: %s", m, testFailure(out))}
		}
		return nil
	}
	patterns := casePatterns(names, k)
	if bad := unevenlyDealt(names, patterns); bad != "" {
		return []string{sprintf("%s: go test: %s's cases were not dealt to its processes once each: %s", m, pkg, bad)}
	}
	runs := make([]string, len(patterns))
	each(len(patterns), func(i int) {
		if out, err := runIn(dir, goBin, "test", "-run", patterns[i], pkg); err != nil {
			runs[i] = sprintf("%s: go test: %s", m, testFailure(out))
		}
	})
	var found []string
	for _, f := range runs {
		if f != "" {
			found = append(found, f)
		}
	}
	return found
}

var caseName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// listedCases reads `go test -list .`: one name a line, then the package's line.
func listedCases(out string) []string {
	var names []string
	for _, ln := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if ln = strings.TrimSpace(ln); caseName.MatchString(ln) {
			names = append(names, ln)
		}
	}
	return names
}

// casePatterns deals names to at most k processes, as -run patterns: a run of
// names in sorted order to each, of about equal count.
//
// ⭐ EACH PATTERN IS A FEW PREFIXES, NOT A LIST OF NAMES. 900 names spelled out
// would pass the 32,767 characters Windows allows a command line. A name in the
// middle of a run shares little with the names outside it, so a short prefix
// picks it out. ⚠ A name that begins another process's name is matched whole.
func casePatterns(names []string, k int) []string {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	uniq := sorted[:0]
	for i, n := range sorted {
		if i == 0 || n != sorted[i-1] {
			uniq = append(uniq, n)
		}
	}
	k = max(1, min(k, len(uniq)))
	patterns := make([]string, 0, k)
	for s := 0; s < k; s++ {
		lo, hi := s*len(uniq)/k, (s+1)*len(uniq)/k
		var alts []string
		for i := lo; i < hi; i++ {
			name := uniq[i]
			need := 0
			if lo > 0 {
				need = max(need, sharedPrefix(name, uniq[lo-1]))
			}
			if hi < len(uniq) {
				need = max(need, sharedPrefix(name, uniq[hi]))
			}
			alt := regexp.QuoteMeta(name) + "$"
			if need < len(name) {
				alt = regexp.QuoteMeta(name[:need+1])
			}
			if len(alts) == 0 || alts[len(alts)-1] != alt {
				alts = append(alts, alt)
			}
		}
		patterns = append(patterns, "^(?:"+strings.Join(alts, "|")+")")
	}
	return patterns
}

// sharedPrefix is how many leading bytes a and b share.
func sharedPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// unevenlyDealt names the first case that no pattern, or more than one,
// matches, the way `go test -run` matches a case's name.
func unevenlyDealt(names, patterns []string) string {
	var res []*regexp.Regexp
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return sprintf("the pattern %q does not compile: %v", p, err)
		}
		res = append(res, re)
	}
	for _, n := range names {
		hits := 0
		for _, re := range res {
			if re.MatchString(n) {
				hits++
			}
		}
		if hits != 1 {
			return sprintf("%s is matched by %d pattern(s)", n, hits)
		}
	}
	return ""
}
