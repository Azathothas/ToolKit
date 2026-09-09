// SPDX-License-Identifier: 0BSD

package checks

import (
	"os/exec"
	"path/filepath"
	"runtime"
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
	r.Extra["scripts"] = len(scripts)

	// One session for every file: starting pwsh per script is most of the cost
	// of the check it replaced.
	const parseAll = `
$bad = 0
foreach ($f in $args) {
  $e = $null; $tk = $null
  $null = [Management.Automation.Language.Parser]::ParseFile((Resolve-Path -LiteralPath $f), [ref]$tk, [ref]$e)
  if ($e -and $e.Count -gt 0) { $bad++; Write-Output ("PARSE|{0}|{1}" -f $f, $e[0].Message) }
}
if (Get-Module -ListAvailable PSScriptAnalyzer) {
  Import-Module PSScriptAnalyzer
  foreach ($r in @(Invoke-ScriptAnalyzer -Path 'scripts' -Recurse -Severity Error,Warning)) {
    Write-Output ("LINT|{0}:{1}|{2}: {3}" -f $r.ScriptName, $r.Line, $r.RuleName, $r.Message)
  }
} else { Write-Output "NOANALYZER" }
`
	args := append([]string{"-NoProfile", "-NonInteractive", "-Command", parseAll, "--"}, scripts...)
	cmd := exec.Command(pwsh, args...)
	cmd.Dir = t.Root
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		r.bad("PowerShell could not be driven: %v", err)
		return r
	}
	for _, ln := range strings.Split(string(out), "\n") {
		ln = strings.TrimRight(ln, "\r")
		switch {
		case strings.HasPrefix(ln, "PARSE\t"):
			f := strings.SplitN(strings.TrimPrefix(ln, "PARSE\t"), "\t", 2)
			r.bad("%s does not parse: %s", f[0], f[len(f)-1])
		case strings.HasPrefix(ln, "LINT\t"):
			f := strings.SplitN(strings.TrimPrefix(ln, "LINT\t"), "\t", 2)
			r.bad("%s %s", f[0], f[len(f)-1])
		case ln == "NOANALYZER":
			r.Extra["analyzer"] = "not installed"
		}
	}
	if _, ok := r.Extra["analyzer"]; !ok {
		r.Extra["analyzer"] = "ran"
	}
	return r
}

// Bundle rebuilds the PowerShell product from its parts and compares BOTH
// tracked copies against the result.
//
// ⛔ WITHOUT THIS, EITHER COPY COULD SILENTLY STOP BEING WHAT ANYBODY WROTE: a
// part edited and never rebuilt, a product edited by hand, or a rebuild that
// refreshed one copy and not the other. TODO/RULES.md section 4.
func Bundle(t *Tree) Result {
	return runPwshScript(t, "scripts/windows/wsl-toolkit/build.ps1", "bundle", "-Test")
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

func runPwshScript(t *Tree, script, label string, args ...string) Result {
	r := Result{Extra: map[string]any{}}
	pwsh := findPwsh()
	if pwsh == "" {
		r.Extra["skipped"] = "no PowerShell on PATH"
		return r
	}
	if len(t.Read(script)) == 0 {
		r.bad("%s is missing", script)
		return r
	}
	full := append([]string{"-NoProfile", "-NonInteractive", "-File", script}, args...)
	cmd := exec.Command(pwsh, full...)
	cmd.Dir = t.Root
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.bad("%s: %s", label, firstFinding(string(out)))
	}
	return r
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
