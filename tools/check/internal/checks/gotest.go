// SPDX-License-Identifier: 0BSD

package checks

import (
	"encoding/json"
	"go/token"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"
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
// against the list. A case no pattern or two patterns match is a finding. Each
// process runs with -json. A listed case that did not start exactly once across
// them is a finding too. So is a case that started and was not listed.

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
		if rel, ok := relUnder(root, at); ok && split[rel] {
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

// relUnder is at's slash path under root, each spelled as the filesystem
// spells it.
//
// ⛔ BOTH ARE RESOLVED FIRST. A root spelled short, as DOWNLO~1, or through a
// link, would otherwise lie outside every directory go list names. The split
// would then be off, and only the time would say so.
func relUnder(root, at string) (string, bool) {
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if a, err := filepath.EvalSymlinks(at); err == nil {
		at = a
	}
	rel, err := filepath.Rel(root, at)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// goTestSplit runs one package's cases in k processes at once.
//
// ⚠ THE LIST BUILDS THE BINARY FIRST, so the k runs find its compile in the
// cache rather than make k of their own.
//
// ⭐ EACH PROCESS RUNS WITH -count=1, AND THE TEST CACHE IS NOT ASKED. Four
// processes of internal/toolkit's cases that go test could cache took 191 s and
// 317 s on the operator's host. With -count=1 the same cases took 90 s and
// 100 s. A cached result needs an unchanged package, which a gate after a
// change to it never has. CI's GOFLAGS carries -count=1 already. TOOL-40.
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
	counts := make([]map[string]int, len(patterns))
	each(len(patterns), func(i int) {
		cmd := exec.Command(goBin, "test", "-json", "-count=1", "-run", patterns[i], pkg)
		cmd.Dir = dir
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := output(cmd)
		ran, text := readTestEvents(string(out))
		counts[i] = ran
		if err != nil {
			runs[i] = sprintf("%s: go test: %s", m, testFailure(text+stderr.String()))
		}
	})
	var found []string
	for _, f := range runs {
		if f != "" {
			found = append(found, f)
		}
	}
	// ⛔ AND EACH LISTED CASE RAN ONCE, read from what the processes ran. A case
	// added after the list, or a TestMain that runs nothing, passes every
	// pattern's check and every process's exit code.
	if len(found) == 0 {
		if bad := unevenlyRun(names, counts); bad != "" {
			found = append(found, sprintf("%s: go test: %s's cases did not run once each across its processes: %s", m, pkg, bad))
		}
	}
	return found
}

// testEvent is the part of one `go test -json` event this check reads.
type testEvent struct {
	Action string
	Test   string
	Output string
}

// readTestEvents reads `go test -json`: how many times each top-level case
// started, and the output as the plain run would have printed it.
//
// ⚠ A CASE'S LINES COME AFTER ITS RESULT, as a plain run prints them. -json
// gives them first, and testFailure reads the lines under `--- FAIL`.
func readTestEvents(out string) (map[string]int, string) {
	ran := map[string]int{}
	held := map[string][]string{}
	var text strings.Builder
	for _, ln := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		var e testEvent
		if !strings.HasPrefix(ln, "{") || json.Unmarshal([]byte(ln), &e) != nil || e.Action == "" {
			text.WriteString(ln + "\n")
			continue
		}
		switch {
		case e.Test == "":
			text.WriteString(e.Output)
		case e.Action == "run":
			if !strings.Contains(e.Test, "/") {
				ran[e.Test]++
			}
		case e.Action == "output":
			if line := strings.TrimSpace(e.Output); strings.HasPrefix(line, "--- ") {
				held[e.Test] = append([]string{e.Output}, held[e.Test]...)
			} else if !strings.HasPrefix(line, "=== ") {
				held[e.Test] = append(held[e.Test], e.Output)
			}
		case e.Action == "pass" || e.Action == "fail" || e.Action == "skip":
			for _, o := range held[e.Test] {
				text.WriteString(o)
			}
			delete(held, e.Test)
		}
	}
	return ran, text.String()
}

// unevenlyRun names the first listed case that did not start exactly once
// across the processes, or a case that started and was not listed.
//
// ⚠ A BENCHMARK IS LISTED AND NEVER RUN, because -run starts no benchmark.
func unevenlyRun(names []string, counts []map[string]int) string {
	total := map[string]int{}
	for _, c := range counts {
		for name, n := range c {
			total[name] += n
		}
	}
	listed := map[string]bool{}
	for _, n := range names {
		listed[n] = true
		if strings.HasPrefix(n, "Benchmark") {
			continue
		}
		if total[n] != 1 {
			return sprintf("%s started %d time(s)", n, total[n])
		}
	}
	var extra []string
	for name := range total {
		if !listed[name] {
			extra = append(extra, name)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		return sprintf("%s started and was not listed", extra[0])
	}
	return ""
}

// caseKinds are the prefixes `go test -list` names a case by.
var caseKinds = []string{"Test", "Benchmark", "Fuzz", "Example"}

// listedCases reads `go test -list .`: one name a line, then the package's line.
//
// ⛔ ANY NAME GO ACCEPTS, NOT ASCII ALONE. A case named with another letter was
// left out of the list, so no pattern was checked against it.
func listedCases(out string) []string {
	var names []string
	for _, ln := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		ln = strings.TrimSpace(ln)
		if !token.IsIdentifier(ln) {
			continue
		}
		for _, kind := range caseKinds {
			if strings.HasPrefix(ln, kind) {
				names = append(names, ln)
				break
			}
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
				// ⚠ A WHOLE LETTER MORE, never part of one. Half a letter is
				// not UTF-8, and its pattern would not compile.
				_, size := utf8.DecodeRuneInString(name[need:])
				alt = regexp.QuoteMeta(name[:need+size])
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
