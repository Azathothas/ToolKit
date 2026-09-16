// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	if v, err := exec.Command(bin, "--version").Output(); err == nil {
		for _, ln := range strings.Split(string(v), "\n") {
			if strings.HasPrefix(ln, "version:") {
				r.Extra["version"] = strings.TrimSpace(strings.TrimPrefix(ln, "version:"))
			}
		}
	}
	n := 0
	for _, f := range WithExt(t.Ours(), ".sh") {
		n++
		cmd := exec.Command(bin, "-s", "sh", f)
		cmd.Dir = t.Root
		if out, err := cmd.CombinedOutput(); err != nil {
			r.bad("%s: %s", f, firstFinding(string(out)))
		}
	}
	r.Extra["scripts"] = n
	return r
}

// PowerShell parses every tracked .ps1 and runs PSScriptAnalyzer over scripts/.
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
	parsed := "0"
	r.Extra["scripts"] = len(scripts)

	// One session for every file: starting pwsh per script is most of the cost
	// of the check it replaced.
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
  foreach ($r in @(Invoke-ScriptAnalyzer -Path 'scripts' -Recurse -Severity Error,Warning)) {
    Write-Output ("LINT|{0}:{1}|{2}: {3}" -f $r.ScriptName, $r.Line, $r.RuleName, $r.Message)
  }
} else { Write-Output "NOANALYZER" }
`
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-Command", parseAll)
	cmd.Dir = t.Root
	cmd.Env = append(os.Environ(), "CHECK_PS1_LIST="+strings.Join(scripts, "\n"))
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		r.bad("PowerShell could not be driven: %v", err)
		return r
	}
	for _, ln := range strings.Split(string(out), "\n") {
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
			r.bad("%s does not parse: %s", f[0], f[len(f)-1])
		case strings.HasPrefix(ln, "LINT|"):
			f := strings.SplitN(strings.TrimPrefix(ln, "LINT|"), "|", 2)
			r.bad("%s %s", f[0], f[len(f)-1])
		case strings.HasPrefix(ln, "COUNT|"):
			parsed = strings.TrimPrefix(ln, "COUNT|")
		case ln == "NOANALYZER":
			r.Extra["analyzer"] = "not installed"
		}
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

// GoModules runs gofmt, vet, build and test over every Go module in the tree.
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
	for _, m := range mods {
		dir := filepath.Join(t.Root, filepath.FromSlash(m))
		if out, err := runIn(dir, goBin, "build", "./..."); err != nil {
			r.bad("%s: go build: %s", m, firstFinding(out))
			continue
		}
		if out, err := runIn(dir, goBin, "vet", "./..."); err != nil {
			r.bad("%s: go vet: %s", m, firstFinding(out))
		}
		if out, err := runIn(dir, goBin, "test", "./..."); err != nil {
			r.bad("%s: go test: %s", m, firstFinding(out))
		}
		// ⚠ gofmt reports by PRINTING, not by exiting non-zero. Reading its
		// exit code would report a tree that needs formatting as clean.
		if out, err := runIn(dir, goBin, "fmt", "-n", "./..."); err == nil && strings.TrimSpace(out) != "" {
			if unformatted, err := runIn(dir, "gofmt", "-l", "."); err == nil && strings.TrimSpace(unformatted) != "" {
				r.bad("%s: gofmt would rewrite: %s", m, strings.Join(strings.Fields(unformatted), " "))
			}
		}
	}
	return r
}

func runIn(dir, bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
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
