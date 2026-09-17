// SPDX-License-Identifier: 0BSD

package checks

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ⛔ WHAT WAS WRONG, AND IT COULD ONLY EVER BE FOUND AFTER A RELEASE.
// consumer.ps1 is the only thing in this repository that drives the tool the
// way somebody outside it does, and it ran ONLY against published binaries. So
// a refusal, a flag rename or a changed default added to the tool was
// undetectable by it until a tag had been cut and the artefacts were public.
//
// It fired for real on 2026-09-17: `wsl-toolkit-v3.0.0` published green and its
// release-smoke job then failed, because consumer.ps1 wrote a configuration
// shape `WSL-74` had made a refusal months of commits earlier. Nothing between
// that commit and that tag could have said so. TODO/PROGRESS.md finding 70,
// WSL-91.
//
// ⭐ THIS CHECK IS THE OTHER HALF. It builds the working tree and drives the
// same file over it with -Exe, so the cases that need no release and no
// distribution answer on every commit. The ones that DO need one skip, each
// carrying its reason, and consumer.ps1's own completeness assertion is what
// stops a local drive quietly becoming a smaller suite.

// consumerRunner is the file this check drives, relative to the tree root.
const consumerRunner = "tools/windows/wsl-toolkit/consumer.ps1"

// consumerModule is the module whose binary it drives.
const consumerModule = "tools/windows/wsl-toolkit"

// consumerReport is the document consumer.ps1 writes under -Json. Only the
// fields this check reads are named: a field it does not read is a field it
// cannot be wrong about.
type consumerReport struct {
	Schema string `json:"schema"`
	OK     bool   `json:"ok"`
	Exe    string `json:"exe"`
	// Complete says every case consumer.ps1 DECLARES was reached. ⛔ It is the
	// field that makes a green run mean something: a table that stopped early,
	// or a case dropped from the local path, exits with ok false AND complete
	// false, and reading only ok would miss a suite that shrank.
	Complete bool   `json:"complete"`
	Cases    int    `json:"cases"`
	Failed   int    `json:"failed"`
	Skipped  int    `json:"skipped"`
	Version  string `json:"version"`
	Detail   []struct {
		Name    string `json:"name"`
		Pass    bool   `json:"pass"`
		Skipped bool   `json:"skipped"`
		Why     string `json:"why"`
		Actual  string `json:"actual"`
	} `json:"detail"`
}

// Consumer builds the tool and drives consumer.ps1 over it.
func Consumer(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	runner := filepath.Join(t.Root, filepath.FromSlash(consumerRunner))
	if _, err := os.Stat(runner); err != nil {
		r.bad("%s is not there, and it is the only thing that drives this tool from outside", consumerRunner)
		return r
	}
	// ⛔ POWERSHELL 7, NOT WINDOWS POWERSHELL. consumer.ps1 refuses 5.1 itself,
	// because it passes arguments as a list, so pointing this at 5.1 would buy
	// an exit 2 that reads as a broken check rather than a missing tool.
	pwsh := findPwsh()
	if pwsh == "" {
		r.Extra["skipped"] = "no PowerShell 7 on PATH"
		return r
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		r.Extra["skipped"] = "no Go toolchain on PATH"
		return r
	}

	// The binary goes under .tmp, where check.ps1 already builds the gate
	// itself, so nothing this check makes lands outside the one directory this
	// repository treats as scratch.
	out := filepath.Join(t.Root, ".tmp", "consumer-wsl-toolkit")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		r.bad("could not make .tmp for the binary to drive: %v", err)
		return r
	}
	build := exec.Command(goBin, "build", "-o", out, ".")
	build.Dir = filepath.Join(t.Root, filepath.FromSlash(consumerModule))
	if msg, err := build.CombinedOutput(); err != nil {
		r.bad("the tool did not build, so nothing could be driven: %v: %s", err, firstLineOf(string(msg)))
		return r
	}

	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", runner, "-Exe", out, "-Json")
	cmd.Dir = t.Root
	stdout, runErr := cmd.Output()

	// ⛔ THE DOCUMENT IS READ BEFORE THE EXIT CODE IS JUDGED. consumer.ps1 exits
	// 1 when a case fails and 0 when none does, and the interesting half is
	// WHICH case: reporting "it exited 1" would hand a reader the one fact they
	// could already see.
	var rep consumerReport
	if err := json.Unmarshal(trimToJSON(stdout), &rep); err != nil {
		r.bad("%s put something on stdout that is not its report: %v", consumerRunner, err)
		if runErr != nil {
			r.bad("%s exited with %v", consumerRunner, runErr)
		}
		return r
	}
	r.Extra["cases"] = rep.Cases
	r.Extra["skipped_cases"] = rep.Skipped
	r.Extra["version"] = rep.Version
	for _, f := range consumerFindings(rep, out) {
		r.bad("%s", f)
	}
	// ⚠ THE EXIT CODE IS THE LAST THING READ, not the first. consumer.ps1 exits
	// 1 for a failing case and the document already named which one, so this only
	// fires where the two disagree - a non-zero exit over a report with nothing
	// wrong in it, which means the file died somewhere the report cannot see.
	if runErr != nil && len(r.Detail) == 0 {
		r.bad("%s exited non-zero over a report with no failing case: %v", consumerRunner, runErr)
	}
	return r
}

// consumerFindings reads the report and says what is wrong with it.
//
// ⛔ IT HAS NO PROCESS IN IT, which is finding 42's lesson and finding 76's: a
// rule that binds to a host passes where it was written and goes red in the
// container, and a verdict that can only be reached by building a binary and
// starting PowerShell is a verdict with no case. Everything above this line
// runs things; everything in here is arithmetic over a document.
func consumerFindings(rep consumerReport, builtExe string) []string {
	var out []string
	if rep.Schema != "wsl-toolkit-consumer/1" {
		out = append(out, sprintf("%s answered schema %q", consumerRunner, rep.Schema))
	}
	// ⛔ A RUN THAT REACHED NO CASE IS NOT A RUN THAT AGREED. It is the shape
	// this whole file exists to refuse, and the one a check takes on its way to
	// reporting nothing.
	if rep.Cases == 0 {
		out = append(out, sprintf("%s reached no case at all", consumerRunner))
	}
	if !rep.Complete {
		out = append(out, sprintf("%s did not reach every case it declares, so this run covered less than the file says it does", consumerRunner))
	}
	// The report names what it actually drove. A run that downloaded a release
	// instead would be answering about a binary this commit did not build.
	if rep.Exe != builtExe {
		out = append(out, sprintf("%s drove %q and this check built %q", consumerRunner, rep.Exe, builtExe))
	}
	named := 0
	for _, c := range rep.Detail {
		if c.Skipped || c.Pass {
			continue
		}
		named++
		out = append(out, sprintf("the released-binary contract is broken by this tree: %s (%s)", c.Name, firstLineOf(c.Actual)))
	}
	// ⚠ A FAILURE COUNT THE DETAIL ROWS DO NOT ACCOUNT FOR IS STILL A FAILURE.
	// Believing the two always agree is how a report that lost its rows reads as
	// a report with nothing in them.
	if rep.Failed > named {
		out = append(out, sprintf("%s reported %d failure(s) and named %d of them", consumerRunner, rep.Failed, named))
	}
	return out
}

// trimToJSON takes the last line of a child's stdout, because a PowerShell host
// can put a banner ahead of the document a script wrote.
func trimToJSON(b []byte) []byte {
	lines := strings.Split(strings.TrimRight(string(b), "\r\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		ln := strings.TrimSpace(lines[i])
		if strings.HasPrefix(ln, "{") {
			return []byte(ln)
		}
	}
	return b
}

func firstLineOf(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return s[:i]
	}
	return s
}
