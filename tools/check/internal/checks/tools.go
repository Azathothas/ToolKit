// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// The checks in this file drive a tool rather than read the tree. Each one is
// SKIPPED when its tool is absent, and a skip says so rather than passing.
//
// ⛔ A SKIP IS NEITHER A PASS NOR A FAILURE. A check that quietly runs nothing
// and reports success is the worst answer this repository can give, so every
// one of these publishes what it found and what it could not.

// Shellcheck runs shellcheck over every tracked shell script, as CI does.
//
// ⚠ THE VERSION MATTERS AND THIS CANNOT CONTROL IT. shellcheck 0.11.0 reports
// nothing for `cd "$D" && cmd || true` and the version on ubuntu-latest reports
// SC2015 and fails the job. docs/methodology/gate.md owns what that costs. The
// answer here is the local tool's; the CI result is the one that gates a merge.
func Shellcheck(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	bin, err := exec.LookPath("shellcheck")
	if err != nil {
		r.Extra["skipped"] = "shellcheck is not on PATH"
		return r
	}
	if v, err := output(exec.Command(bin, "--version")); err == nil {
		for _, ln := range strings.Split(string(v), "\n") {
			if strings.HasPrefix(ln, "version:") {
				r.Extra["version"] = strings.TrimSpace(strings.TrimPrefix(ln, "version:"))
			}
		}
	}
	scripts := WithExt(t.Ours(), ".sh")
	lint := func(files []string) (string, error) {
		cmd := exec.Command(bin, append([]string{"-s", "sh"}, files...)...)
		cmd.Dir = t.Root
		out, err := combined(cmd)
		return string(out), err
	}
	for _, f := range lintEach(scripts, lint) {
		r.bad("%s", f)
	}
	r.Extra["scripts"] = len(scripts)
	return r
}

// lintEach runs lint over each file alone, all at once. It returns one finding
// per failing file, in the order files lists them, with the text that file's
// own run gave.
//
// ⛔ ONE PROCESS PER SCRIPT, NEVER A BATCH. shellcheck follows a `.` or `source`
// only to a file that is also on its command line. Alone, a script that sources
// a sibling reports SC1091, and SC2154 for each name the sibling sets. Beside
// it in one batch, the same script passes. Batches were dealt by file size and
// CPU count, so one tree could pass on one host and fail on another.
// TestEachScriptIsLintedAlone holds it with a lint that acts as shellcheck
// does, and TestTheShellcheckCheckFailsAScriptThatSourcesASibling with the tool.
//
// ⚠ THE LONE RUNS COST LITTLE MORE THAN BATCHES DID. Most of a run is the
// analysis, not the process start, so they run side by side. scripts/README.md
// has the figures.
func lintEach(files []string, lint func([]string) (string, error)) []string {
	found := make([]string, len(files))
	each(len(files), func(i int) {
		if out, err := lint(files[i : i+1]); err != nil {
			found[i] = files[i] + ": " + firstFinding(out)
		}
	})
	var out []string
	for _, f := range found {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// weights is each file's size plus one, which is what balance deals by. The one
// keeps an empty file from weighing nothing. A tree of empty files is then
// dealt across every group, not into the first.
func weights(t *Tree, files []string) []int64 {
	w := make([]int64, len(files))
	for i, f := range files {
		w[i] = int64(len(t.Read(f))) + 1
	}
	return w
}

// balance deals the indices of weight into at most k groups, heaviest first.
// Each goes to the group that holds the least so far. Every group's indices
// come back in ascending order, and there is always at least one group.
//
// ⛔ EVERY INDEX LANDS IN EXACTLY ONE GROUP. A file dealt to no group is a file
// no tool reads and no finding names, which is a pass over something unread.
// TestBalanceDealsEveryFileOnce holds it.
func balance(weight []int64, k int) [][]int {
	k = max(1, min(k, len(weight)))
	order := make([]int, len(weight))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return weight[order[a]] > weight[order[b]] })
	groups := make([][]int, k)
	load := make([]int64, k)
	for _, i := range order {
		g := 0
		for j := 1; j < k; j++ {
			if load[j] < load[g] {
				g = j
			}
		}
		groups[g] = append(groups[g], i)
		load[g] += weight[i]
	}
	for _, g := range groups {
		sort.Ints(g)
	}
	return groups
}

// PowerShell parses every tracked .ps1 and runs PSScriptAnalyzer over each one.
// TOOL-35: this line said scripts/ alone after the analyzer was widened.
//
// ⛔ BOTH HALVES, and the analyzer is the one that catches what a parse cannot:
// a case-shadowed parameter, a state-changing verb with no ShouldProcess, a
// non-ASCII byte in a file Windows PowerShell 5.1 has to decode.
func PowerShell(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	pwsh := findPwsh()
	if pwsh == "" {
		r.Extra["skipped"] = "no PowerShell on PATH"
		return r
	}
	scripts := WithExt(t.Ours(), ".ps1")
	r.Extra["scripts"] = len(scripts)

	// One session for every file: starting pwsh per script is most of the cost
	// of the check it replaced.
	//
	// ⭐ AND A FEW SESSIONS AT ONCE, each over its own share of the files. The
	// analyzer's cost is per file and uneven. In one run on a loaded Windows
	// host, one session took 42 s over all 26 files. With cc_profile.ps1 in a
	// session of its own, the two sessions took 20 s. That is one run, not a
	// median. In golang:1.25 on four CPUs the split made no difference, at
	// about 2.5 s and 9 s in two sets. psSessions says how many share the work.
	//
	// ⛔ THE FILE LIST ARRIVES IN THE ENVIRONMENT, NEVER AS ARGUMENTS. `pwsh
	// -Command <text> -- a.ps1 b.ps1` does NOT bind those names to $args the way
	// -File does; it appends them to the command text, so $args was EMPTY and the
	// loop below ran zero times while the names were echoed as output. Measured on
	// 2026-09-16: the same invocation printed `ARGS_SEEN|0` and then `a.ps1` and
	// `b.ps1`. An environment variable also needs no quoting, which is what
	// conventions/shell.md asks for over an escaped payload.
	//
	// ⛔ AND THE COUNT IS RETURNED, because "parsed nothing" is the failure this
	// check spent its whole life in. The caller refuses a count that is not the
	// number of scripts it handed over.
	const parseAll = `
$list = $env:CHECK_PS1_LIST
# TWO RULES ARE EXCLUDED AND EACH EXCLUSION IS A DECISION, not a way to make a
# red list go green. PSUseShouldProcessForStateChangingFunctions wants -WhatIf
# on New-*/Set-* helpers INSIDE a test harness, which no caller ever passes;
# PSUseSingularNouns would rename Get-ReleaseAssets, which downloads all of
# them, to a name that is less true. Everything else the analyzer reports is a
# finding this repository fixes.
$exclude = @('PSUseShouldProcessForStateChangingFunctions', 'PSUseSingularNouns')
$seen = 0
if ($list) {
  foreach ($f in $list.Split([char]10)) {
    $f = $f.Trim()
    if (-not $f) { continue }
    $seen++
    $e = $null; $tk = $null
    $null = [Management.Automation.Language.Parser]::ParseFile((Resolve-Path -LiteralPath $f).Path, [ref]$tk, [ref]$e)
    if ($e -and $e.Count -gt 0) { Write-Output ("PARSE|{0}|{1}" -f $f, $e[0].Message) }
  }
}
Write-Output ("COUNT|{0}" -f $seen)
if (Get-Module -ListAvailable PSScriptAnalyzer) {
  Import-Module PSScriptAnalyzer
  # EVERY TRACKED SCRIPT, NOT ONE DIRECTORY. This read -Path 'scripts' -Recurse
  # until 2026-09-17, so the eight .ps1 files under tools/ were parsed and never
  # analysed - and the one real finding in the tree was in that half:
  # consumer.ps1 held a non-ASCII byte with no BOM, which is the exact rule
  # PSUseBOMForUnicodeEncodedFile exists for and which that file's own header
  # claimed it did not need. A guard on one of several paths into the same
  # thing is the commonest hole there is.
  #
  # -Path TAKES ONE PATH. Handing it the array refuses with "Cannot convert
  # System.Object[] to System.String", so the loop is per file rather than one
  # call, and $seen above is what proves the list arrived at all.
  #
  # FILE NAMES THE SCRIPT THE LINT LINES AFTER IT ARE ABOUT. A lint line carries
  # only the leaf name, and the caller merges several sessions.
  foreach ($f in $list.Split([char]10)) {
    $f = $f.Trim()
    if (-not $f) { continue }
    Write-Output ("FILE|{0}" -f $f)
    foreach ($r in @(Invoke-ScriptAnalyzer -Path $f -Severity Error,Warning -ExcludeRule $exclude)) {
      Write-Output ("LINT|{0}:{1}|{2}: {3}" -f $r.ScriptName, $r.Line, $r.RuleName, $r.Message)
    }
  }
} else { Write-Output "NOANALYZER" }
`
	w := weights(t, scripts)
	groups := balance(w, psSessions(w))
	outs := make([][]byte, len(groups))
	errs := make([]error, len(groups))
	each(len(groups), func(g int) {
		list := make([]string, len(groups[g]))
		for i, at := range groups[g] {
			list[i] = scripts[at]
		}
		cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-Command", parseAll)
		cmd.Dir = t.Root
		cmd.Env = append(os.Environ(), "CHECK_PS1_LIST="+strings.Join(list, "\n"))
		outs[g], errs[g] = combined(cmd)
	})
	for g := range groups {
		if errs[g] != nil && len(outs[g]) == 0 {
			r.bad("PowerShell could not be driven: %v", errs[g])
			return r
		}
	}
	found, parsed, noAnalyzer := psRead(scripts, linesOf(outs))
	for _, f := range found {
		r.bad("%s", f)
	}
	if noAnalyzer {
		r.Extra["analyzer"] = "not installed"
	}
	// ⛔ A CHECK THAT READ NOTHING IS NOT A CHECK THAT AGREED.
	r.Extra["parsed"] = parsed
	if want := strconv.Itoa(len(scripts)); parsed != want {
		r.bad("PowerShell parsed %s of the %s tracked scripts; the session read no file list", parsed, want)
	}
	if _, ok := r.Extra["analyzer"]; !ok {
		r.Extra["analyzer"] = "ran"
	}
	return r
}

// psSessions is how many sessions share the scripts. It is enough that no share
// outweighs the heaviest file, which sets the floor whatever the split, and it
// is never more than the slots or the scripts. ⚠ Each session pays pwsh's start
// and the analyzer's import, about 2.5 s on a Windows development host, so more
// would buy nothing.
func psSessions(weight []int64) int {
	var total, heaviest int64
	for _, w := range weight {
		total += w
		heaviest = max(heaviest, w)
	}
	n := 1
	if heaviest > 0 {
		n = int((total + heaviest - 1) / heaviest)
	}
	return max(1, min(n, len(weight), cap(slots)))
}

// linesOf is what every session wrote as one list of lines, sessions in order.
func linesOf(outs [][]byte) []string {
	var lines []string
	for _, out := range outs {
		lines = append(lines, strings.Split(string(out), "\n")...)
	}
	return lines
}

// psRead reads the lines every session wrote, sessions in order. It returns the
// findings in the order one session over every script gave them: each parse
// failure by script, then each analyzer finding by script. parsed is the sum of
// every session's count. It is a string because the JSON has always said it as
// one.
//
// ⛔ EVERY SESSION'S LINES ARE READ AND EVERY COUNT IS ADDED. A merge that read
// one session would call the others' scripts clean without reading what they
// said. Its count would still look like a number.
func psRead(scripts []string, lines []string) (found []string, parsed string, noAnalyzer bool) {
	at := make(map[string]int, len(scripts))
	for i, s := range scripts {
		at[s] = i
	}
	// One bucket per script, and a last one for a line no script claims.
	parse := make([][]string, len(scripts)+1)
	lint := make([][]string, len(scripts)+1)
	cur, seen := len(scripts), 0
	for _, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		switch {
		// ⛔ THE SEPARATOR IS THE ONE parseAll ABOVE ACTUALLY WRITES, a pipe.
		// This read for "PARSE\t" and "LINT\t" until 2026-09-16, so no line it
		// produced could ever match and the check reported ok over every broken
		// script there was. It is the shape reviews.md calls theatre: green,
		// trusted, and incapable of refusing. A tab is not usable here anyway,
		// because PSScriptAnalyzer messages contain them.
		case strings.HasPrefix(ln, "PARSE|"):
			f := strings.SplitN(strings.TrimPrefix(ln, "PARSE|"), "|", 2)
			i, ok := at[f[0]]
			if !ok {
				i = len(scripts)
			}
			parse[i] = append(parse[i], sprintf("%s does not parse: %s", f[0], f[len(f)-1]))
		case strings.HasPrefix(ln, "FILE|"):
			cur = len(scripts)
			if i, ok := at[strings.TrimPrefix(ln, "FILE|")]; ok {
				cur = i
			}
		case strings.HasPrefix(ln, "LINT|"):
			f := strings.SplitN(strings.TrimPrefix(ln, "LINT|"), "|", 2)
			lint[cur] = append(lint[cur], sprintf("%s %s", f[0], f[len(f)-1]))
		case strings.HasPrefix(ln, "COUNT|"):
			n, _ := strconv.Atoi(strings.TrimPrefix(ln, "COUNT|"))
			seen += n
		case ln == "NOANALYZER":
			noAnalyzer = true
		}
	}
	for _, p := range parse {
		found = append(found, p...)
	}
	for _, l := range lint {
		found = append(found, l...)
	}
	return found, strconv.Itoa(seen), noAnalyzer
}

// GoModules runs gofmt, vet, build and test over every Go module in the tree.
//
// ⭐ THE MODULES ARE CHECKED AT ONCE, and each module's own steps still run in
// order. A module that does not build is not vetted or tested, as before.
func GoModules(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	goBin, err := exec.LookPath("go")
	if err != nil {
		r.Extra["skipped"] = "no Go toolchain on PATH"
		return r
	}
	var mods []string
	for _, f := range t.Files {
		if filepath.Base(f) == "go.mod" {
			mods = append(mods, filepath.Dir(f))
		}
	}
	r.Extra["modules"] = len(mods)
	for _, f := range goModules(mods, func(m string) []string { return goModule(t.Root, goBin, m) }) {
		r.bad("%s", f)
	}
	return r
}

// goModules runs check over every module at once and returns what each found,
// modules in the order mods lists them.
//
// ⚠ ON FOUR CPUS FROM A COLD BUILD CACHE, AS CI HAS, THIS IS SLOWER. The
// slowest module sets the floor, and the others compete with it for the CPUs.
// Pinned to four CPUs in golang:1.25, the go check's median was 107 s against
// 103 s one module after another, and 171 s against 139 s in a second set.
// Building the shared standard packages first did not shorten it either.
// Whether it helps a host with spare CPUs is not measured.
func goModules(mods []string, check func(m string) []string) []string {
	found := make([][]string, len(mods))
	each(len(mods), func(i int) { found[i] = check(mods[i]) })
	var out []string
	for _, f := range found {
		out = append(out, f...)
	}
	return out
}

// goModule is build, vet, test and gofmt over one module, in that order.
func goModule(root, goBin, m string) []string {
	var found []string
	dir := filepath.Join(root, filepath.FromSlash(m))
	if out, err := runIn(dir, goBin, "build", "./..."); err != nil {
		return []string{sprintf("%s: go build: %s", m, firstFinding(out))}
	}
	if out, err := runIn(dir, goBin, "vet", "./..."); err != nil {
		found = append(found, sprintf("%s: go vet: %s", m, firstFinding(out)))
	}
	if out, err := runIn(dir, goBin, "test", "./..."); err != nil {
		found = append(found, sprintf("%s: go test: %s", m, testFailure(out)))
	}
	// ⚠ gofmt reports by PRINTING, not by exiting non-zero. Reading its
	// exit code would report a tree that needs formatting as clean.
	if out, err := runIn(dir, goBin, "fmt", "-n", "./..."); err == nil && strings.TrimSpace(out) != "" {
		if unformatted, err := runIn(dir, "gofmt", "-l", "."); err == nil && strings.TrimSpace(unformatted) != "" {
			found = append(found, sprintf("%s: gofmt would rewrite: %s", m, strings.Join(strings.Fields(unformatted), " ")))
		}
	}
	return found
}

func runIn(dir, bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := combined(cmd)
	return string(out), err
}

func findPwsh() string {
	for _, name := range []string{"pwsh", "powershell"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	if runtime.GOOS == "windows" {
		return ""
	}
	return ""
}

// firstFinding is the first line that reads like a problem, so a wall of
// output does not arrive in a summary table.
// testFailure is firstFinding with the failed case's own lines after it.
//
// ⛔ THE CASE'S MESSAGE IS THE FINDING. `--- FAIL: TestX` alone names the case
// and not what it saw, so a failure only a CI runner meets could not be read
// from the log and had to be guessed at.
func testFailure(out string) string {
	lines := strings.Split(out, "\n")
	// ⛔ A TIMEOUT PRINTS NO `--- FAIL`, only a panic and the cases still running,
	// so a hang on a runner was reported as the package's FAIL line alone.
	for i, ln := range lines {
		if !strings.HasPrefix(strings.TrimSpace(ln), "panic: test timed out") {
			continue
		}
		said := []string{strings.TrimSpace(ln)}
		for _, next := range lines[i+1:] {
			next = strings.TrimSpace(strings.TrimRight(next, "\r"))
			if len(said) > 8 || next == "" {
				break
			}
			said = append(said, next)
		}
		return strings.Join(said, " | ")
	}
	for i, ln := range lines {
		if !strings.HasPrefix(strings.TrimSpace(ln), "--- FAIL") {
			continue
		}
		said := []string{strings.TrimSpace(ln)}
		for _, next := range lines[i+1:] {
			next = strings.TrimRight(next, "\r")
			if len(said) > 8 || !strings.HasPrefix(next, " ") || strings.TrimSpace(next) == "" {
				break
			}
			said = append(said, strings.TrimSpace(next))
		}
		return strings.Join(said, " | ")
	}
	return firstFinding(out)
}

func firstFinding(out string) string {
	var first string
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(strings.TrimRight(ln, "\r"))
		if ln == "" {
			continue
		}
		if first == "" {
			first = ln
		}
		low := strings.ToLower(ln)
		if strings.Contains(low, "fail") || strings.Contains(low, "error") || strings.HasPrefix(ln, "!") {
			return ln
		}
	}
	return first
}
