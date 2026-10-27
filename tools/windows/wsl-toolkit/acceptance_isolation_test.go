// acceptance_isolation_test.go - the acceptance suite's cases stand apart from
// the instance they run on. WSL-132.
//
// ⛔ THE MARKER CASE WROTE AN INSTANCE'S OWN CONFIGURATION WHOLE, with three keys,
// so the `base ensure` it ran read an instance that asked for systemd as asking for
// none. Four more cases failed on such an instance: two read its own file where
// they meant the working directory's, and two gave a helper a home that asked for
// no systemd. The suite runs only against a real machine, so these run its own
// functions and the marker case itself under pwsh, against a tool that is a stub.
//
// ⛔ THEY RUN WHERE pwsh RUNS, BY RULING AND NOT BY CHANCE. The suite is PowerShell,
// so nothing else can run what it does. On Windows pwsh is the suite's own host,
// and a missing one fails the case. Elsewhere a missing one skips the case and
// says so: CI's ubuntu runner carries pwsh and runs them, and the golang image
// the Linux proof runs in carries none, so its rows read SKIPPED there.
//
// SPDX-License-Identifier: 0BSD

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// suitePwsh is the pwsh that runs the suite's own PowerShell, under the ruling
// the file's header states.
func suitePwsh(t *testing.T) string {
	t.Helper()
	pwsh, err := exec.LookPath("pwsh")
	if err == nil {
		return pwsh
	}
	if runtime.GOOS == "windows" {
		t.Fatalf("pwsh is not on PATH, and on Windows it is the acceptance suite's own host: %v", err)
	}
	t.Skip("WSL-132 ruling: this case runs the acceptance suite's PowerShell, and this host has no pwsh. Windows and CI's ubuntu runner run it")
	return ""
}

// runSuiteScript writes script and runs it under pwsh with args, and fails the
// case when pwsh does.
func runSuiteScript(t *testing.T, pwsh, script string, args ...string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "driver.ps1")
	if err := forksafe.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(pwsh, append([]string{"-NoProfile", "-NonInteractive", "-File", path}, args...)...)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the driver failed: %v\n%s", err, b)
	}
}

// suiteFunctions is each named top-level function of the suite, whole.
func suiteFunctions(t *testing.T, names ...string) string {
	t.Helper()
	var b strings.Builder
	for _, name := range names {
		b.WriteString(acceptanceFunction(t, name))
		b.WriteString("\n")
	}
	return b.String()
}

// markerStub is the tool the marker case meets: a base built from Arch, a
// configuration at $script:StubConfig, and an ensure that copies what it read to
// $script:StubSnapshot and answers $script:StubEnsure. ⚠ The names are unlike any
// the case uses, because PowerShell looks a name up through its caller's scopes.
const markerStub = `
function Invoke-Tool {
    param([Parameter(Mandatory = $true)][string[]]$ToolArgs, [string]$WorkingDirectory = '')
    $asked = $ToolArgs -join ' '
    if ($asked -eq 'base status --probe --json') {
        return [pscustomobject]@{ Code = 0; Err = ''
            Out = '{"identity":{"image":"ghcr.io/pkgforge-dev/archlinux:latest"},"built_from":"ghcr.io/pkgforge-dev/archlinux:latest"}' }
    }
    if ($asked -eq 'config') { return [pscustomobject]@{ Code = 0; Err = ''; Out = $script:StubConfig + "` + "`n" + `" } }
    if ($asked -eq 'base ensure') {
        if (Test-Path -LiteralPath $script:StubConfig) { Copy-Item -LiteralPath $script:StubConfig -Destination $script:StubSnapshot }
        if ($script:StubEnsure -eq 'throw') { throw 'the stub base ensure threw' }
        return [pscustomobject]@{ Code = [int]$script:StubEnsure; Out = ''
            Err = 'the base was built from ghcr.io/pkgforge-dev/archlinux:latest. Run base recreate to rebuild it' }
    }
    throw "the stub answers no $asked"
}
# A restore that restores nothing, for the case that has to notice.
if ($script:StubBroken) { function Restore-ConfigBytes { param($Path, $Saved) } }
`

// TestTheMarkerCaseGivesTheConfigurationBackByteForByte runs the marker case
// itself against a stubbed tool. The ensure reads the file with one key changed
// and every other kept, and the file comes back byte for byte whether the case
// passes, fails or throws. A restore that missed is the case's own failure.
//
// ⚠ THE FILE CARRIES A BYTE ORDER MARK, CRLF AND A BYTE THAT IS NOT UTF-8. Text
// read and written back loses each of them, which a JSON comparison cannot see.
func TestTheMarkerCaseGivesTheConfigurationBackByteForByte(t *testing.T) {
	pwsh := suitePwsh(t)
	const name = "the guest own marker decides what the base was built from"
	body := acceptanceCaseBody(t, name)
	open := strings.Index(body, "{\n")
	if open < 0 {
		t.Fatalf("the marker case opens no block:\n%s", body)
	}
	driver := `param([string]$StubConfig, [string]$StubSnapshot, [string]$StubEnsure, [string]$StubAnswer, [string]$StubBroken)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
` + suiteFunctions(t, "Show-Bytes", "Read-ToolJson", "Edit-ConfigKey", "Restore-ConfigBytes", "Test-ConfigBytes") + markerStub + `
$case = {` + body[open+1:] + `
}
$answer = try { [string](& $case) } catch { "THREW: $($_.Exception.Message)" }
[IO.File]::WriteAllText($StubAnswer, $answer)
`
	const lie = "docker.io/library/alpine:latest"
	asksForSystemd := append([]byte("\ufeff"), []byte(strings.ReplaceAll(`{
  "schema": "wsl-toolkit-config/1",
  "base": {
    "name": "wsl-toolkit-s132",
    "image": "ghcr.io/pkgforge-dev/archlinux:latest",
    "user": "toolkit",
    "systemd": true,
    "passwordless_sudo": true,
    "kvm": true
  },
  "jobs": {"timeout": "1h", "env": {"STAMP": "2026-10-21T00:00:00Z", "RAW": "a`, "\n", "\r\n"))...)
	asksForSystemd = append(asksForSystemd, 0xff)
	asksForSystemd = append(asksForSystemd, []byte("b\"}}\r\n}\r\n")...)
	for _, tc := range []struct {
		name           string
		file           []byte // nil is no file
		ensure, broken string
		answer         string
		// read is what the ensure read, with base.image the lie. nil skips it.
		read map[string]any
	}{
		{"an instance that asks for systemd", asksForSystemd, "1", "", "True", withImage(t, asksForSystemd, lie)},
		{"an ensure that agrees with the drift", asksForSystemd, "0", "", "base ensure exited 0 rather than 1 over a drifted base", nil},
		{"an ensure that throws", asksForSystemd, "throw", "", "THREW: the stub base ensure threw", nil},
		{"no file", nil, "1", "", "True", map[string]any{"schema": "wsl-toolkit-config/1", "base": map[string]any{"image": lie}}},
		{"an empty file", []byte{}, "1", "", "True", map[string]any{"schema": "wsl-toolkit-config/1", "base": map[string]any{"image": lie}}},
		{"a restore that restores nothing", asksForSystemd, "1", "broken", "the configuration at CONFIG was not restored byte for byte", nil},
	} {
		dir := t.TempDir()
		cfg, snapshot, answer := filepath.Join(dir, "config.json"), filepath.Join(dir, "read.json"), filepath.Join(dir, "answer.txt")
		if tc.file != nil {
			if err := forksafe.WriteFile(cfg, tc.file, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		args := []string{"-StubConfig", cfg, "-StubSnapshot", snapshot, "-StubEnsure", tc.ensure, "-StubAnswer", answer}
		if tc.broken != "" {
			args = append(args, "-StubBroken", tc.broken)
		}
		runSuiteScript(t, pwsh, driver, args...)
		got, err := os.ReadFile(answer)
		if err != nil {
			t.Fatal(err)
		}
		if want := strings.ReplaceAll(tc.answer, "CONFIG", cfg); string(got) != want {
			t.Errorf("%s: the case answered %q, want %q", tc.name, got, want)
		}
		if tc.read != nil {
			read, err := os.ReadFile(snapshot)
			if err != nil {
				t.Fatalf("%s: the ensure read no configuration: %v", tc.name, err)
			}
			if have := decodeConfig(t, read); !reflect.DeepEqual(have, tc.read) {
				t.Errorf("%s: the ensure read\n%s\nand only base.image should differ from what the file held", tc.name, read)
			}
		}
		if tc.broken != "" {
			continue
		}
		after, err := os.ReadFile(cfg)
		switch {
		case tc.file == nil && err == nil:
			t.Errorf("%s: the case left a configuration where there was none:\n%s", tc.name, after)
		case tc.file != nil && err != nil:
			t.Errorf("%s: the case removed the configuration: %v", tc.name, err)
		case tc.file != nil && !bytes.Equal(after, tc.file):
			t.Errorf("%s: the configuration came back as\n%q\nnot as\n%q", tc.name, after, tc.file)
		}
	}
}

// withImage is the configuration b decodes to, with base.image set to image.
func withImage(t *testing.T, b []byte, image string) map[string]any {
	t.Helper()
	m := decodeConfig(t, b)
	m["base"].(map[string]any)["image"] = image
	return m
}

// TestEditConfigKeyChangesOnlyItsKey runs the suite's own Edit-ConfigKey and
// compares what it wrote with what it read: the one key differs, and nothing else
// does, systemd and passwordless sudo included.
func TestEditConfigKeyChangesOnlyItsKey(t *testing.T) {
	pwsh := suitePwsh(t)
	dir := t.TempDir()
	driver := acceptanceFunction(t, "Edit-ConfigKey") + `
$ErrorActionPreference = 'Stop'
$text = [Text.UTF8Encoding]::new($false).GetString([IO.File]::ReadAllBytes($args[0]))
$edited = Edit-ConfigKey -Text $text -Section 'base' -Key 'image' -Value $args[2]
[IO.File]::WriteAllText($args[1], $edited, [Text.UTF8Encoding]::new($false))
`
	const lie = "docker.io/library/alpine:latest"
	full := "\ufeff" + strings.ReplaceAll(`{
  "schema": "wsl-toolkit-config/1",
  "base": {
    "name": "wsl-toolkit-s132",
    "image": "ghcr.io/pkgforge-dev/archlinux:latest",
    "user": "toolkit",
    "systemd": true,
    "passwordless_sudo": true,
    "toolset": "standard",
    "mounts": [{"source": "C:\\work\\one", "target": "/mnt/one", "mode": "rw"}],
    "root_engine": true,
    "kvm": true
  },
  "jobs": {
    "container_lifecycle": "persistent",
    "timeout": "1h",
    "env": {"STAMP": "2026-10-21T00:00:00Z"},
    "cpus": 1.5,
    "max_bytes": 1073741824
  },
  "matrix": ["alpine"],
  "requires": "8.0.0"
}`, "\n", "\r\n")
	for _, tc := range []struct {
		name, in string
		want     map[string]any
	}{
		{"an instance that asks for systemd", full, withImage(t, []byte(full), lie)},
		{"no file", "", map[string]any{
			"schema": "wsl-toolkit-config/1",
			"base":   map[string]any{"image": lie},
		}},
	} {
		in, out := filepath.Join(dir, "in.json"), filepath.Join(dir, "out.json")
		if err := forksafe.WriteFile(in, []byte(tc.in), 0o600); err != nil {
			t.Fatal(err)
		}
		runSuiteScript(t, pwsh, driver, in, out, lie)
		got, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if have := decodeConfig(t, got); !reflect.DeepEqual(have, tc.want) {
			t.Errorf("%s: Edit-ConfigKey wrote\n%s\nand only base.image should differ from\n%s", tc.name, got, tc.in)
		}
	}
}

// TestGetSuiteBaseAsksAScratchHomeForTheSuitesBase runs Get-SuiteBase on what
// `config --json` answers for an instance with a file of its own, writes it as a
// scratch home's file the way the cases do, and loads both files. A helper is
// sent the same base from either, all but the name.
//
// ⛔ A RELATIVE GRANT SOURCE IS RESOLVED AT THE FILE THAT NAMES IT, so one copied
// as written named a directory beside the scratch home. ⚠ AND THE NAME IS LEFT
// OUT, so a scratch home names its base as it did with no file.
func TestGetSuiteBaseAsksAScratchHomeForTheSuitesBase(t *testing.T) {
	pwsh := suitePwsh(t)
	quietCLI(t)
	selectInstance(t, "s132")
	was := toolkit.ExplicitConfigPath
	t.Cleanup(func() { toolkit.ExplicitConfigPath = was })
	root := t.TempDir()
	suiteDir, granted := filepath.Join(root, "suite"), filepath.Join(root, "granted")
	for _, d := range []string{filepath.Join(suiteDir, "rel", "one"), granted} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	suite := filepath.Join(suiteDir, "config.json")
	doc, err := json.Marshal(map[string]any{
		"schema": "wsl-toolkit-config/1",
		"base": map[string]any{
			"name": "wsl-toolkit-s132", "image": "ghcr.io/pkgforge-dev/archlinux:latest", "user": "toolkit",
			"automount": "off", "interop": "off", "systemd": true, "passwordless_sudo": true, "toolset": "developer",
			"mounts": []map[string]string{
				{"source": filepath.Join("rel", "one"), "target": "/workspaces/one", "mode": "rw"},
				{"source": granted, "target": "/workspaces/two", "mode": "ro"},
			},
			"root_engine": true, "runtime_dir": true, "kvm": true,
		},
		"jobs": map[string]any{"timeout": "1h"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := forksafe.WriteFile(suite, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	var code int
	report := captureStdout(t, func() { code = run(context.Background(), []string{"--config", suite, "config", "--json"}) })
	if code != exitOK {
		t.Fatalf("config --json answered %d: %s", code, report)
	}
	answered := filepath.Join(root, "report.json")
	if err := forksafe.WriteFile(answered, []byte(report), 0o600); err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(root, "scratch")
	driver := `param([string]$StubReport, [string]$StubHome)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$script:StubReport = $StubReport
` + suiteFunctions(t, "Show-Bytes", "Read-ToolJson", "Get-StateDir", "Set-StateConfig", "Get-SuiteBase") + `
function Invoke-Tool {
    param([Parameter(Mandatory = $true)][string[]]$ToolArgs, [string]$WorkingDirectory = '')
    if (($ToolArgs -join ' ') -ne 'config --json') { throw "the stub answers no $($ToolArgs -join ' ')" }
    return [pscustomobject]@{ Code = 0; Err = ''; Out = [IO.File]::ReadAllText($script:StubReport) }
}
$script:Instance = ''
$null = Set-StateConfig -StateHome $StubHome -Config @{ base = (Get-SuiteBase) }
`
	runSuiteScript(t, pwsh, driver, "-StubReport", answered, "-StubHome", scratch)
	written := filepath.Join(scratch, "config.json")
	b, err := os.ReadFile(written)
	if err != nil {
		t.Fatalf("no scratch configuration was written: %v", err)
	}
	if base, _ := decodeConfig(t, b)["base"].(map[string]any); base == nil {
		t.Fatalf("the scratch configuration carries no base:\n%s", b)
	} else if _, named := base["name"]; named {
		t.Errorf("the scratch configuration names its base, which a scratch home did not before:\n%s", b)
	}
	load := func(path string) toolkit.BaseConfig {
		t.Helper()
		toolkit.ExplicitConfigPath = path
		cfg, err := toolkit.LoadConfig()
		if err != nil {
			t.Fatalf("%s could not be loaded: %v", path, err)
		}
		base := cfg.Portable().Base
		base.Name = ""
		return base
	}
	if want, got := load(suite), load(written); !reflect.DeepEqual(got, want) {
		t.Errorf("a helper is sent\n%+v\nfrom the scratch home, and the suite's base is\n%+v\nwritten there as\n%s", got, want, b)
	}
}

// TestNoCaseReadsTheInstancesOwnConfigurationForAnotherFile holds four cases to
// the configuration they mean, on an instance that has a file of its own.
//
// ⛔ A NAMED INSTANCE'S OWN FILE OUTRANKS THE WORKING DIRECTORY, so a case that
// asks which project file wins asks from an empty home. ⛔ AND A HOME WITH NO FILE
// ASKS FOR NO SYSTEMD, so a case that gives a helper a home of its own writes the
// suite's base into it, which TestGetSuiteBaseAsksAScratchHomeForTheSuitesBase
// holds. Measured on an instance that asked for systemd: all four failed, and the
// rest of the suite passed.
func TestNoCaseReadsTheInstancesOwnConfigurationForAnotherFile(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"the nearer configuration wins whole and config names the order",
			"foreach ($a in @('--home', $empty, 'config', '--json'))"},
		{"a configuration this tool does not read is refused by name",
			"foreach ($a in @('--home', $bare, 'config', '--json'))"},
		{"a helper job leaves no uploaded workspace or artifact set behind",
			"Set-StateConfig -StateHome $home3 -Config @{ base = (Get-SuiteBase) }"},
		{"a helper resolves a catalog id against the config as it is now",
			"base   = (Get-SuiteBase)\n                images = @("},
	} {
		if body := acceptanceCaseBody(t, tc.name); !strings.Contains(body, tc.want) {
			t.Errorf("%q no longer says %q", tc.name, tc.want)
		}
	}
}

// TestTheSuiteRefusesAPowerShellItsJSONCannotRunOn runs the suite's own
// Get-RunnerRefusal. WSL-132: Edit-ConfigKey reads JSON with
// System.Text.Json.Nodes, which .NET 6 first shipped, so PowerShell 7.0 and 7.1
// threw inside the marker case. The suite refuses them before any case runs.
func TestTheSuiteRefusesAPowerShellItsJSONCannotRunOn(t *testing.T) {
	pwsh := suitePwsh(t)
	out := filepath.Join(t.TempDir(), "answers.txt")
	driver := `param([string]$StubAnswers)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
` + suiteFunctions(t, "Get-RunnerRefusal") + `
$answers = foreach ($c in @('Desktop 5.1.22621.4391', 'Core 7.0.3', 'Core 7.1.7', 'Core 7.2.0', 'Core 7.6.8')) {
    $edition, $version = $c -split ' '
    $v = if ($edition -eq 'Core') { [System.Management.Automation.SemanticVersion]::Parse($version) } else { [version]$version }
    "$c=" + (Get-RunnerRefusal -Edition $edition -Version $v)
}
[IO.File]::WriteAllLines($StubAnswers, [string[]]$answers)
`
	runSuiteScript(t, pwsh, driver, "-StubAnswers", out)
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	answers := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(strings.ReplaceAll(string(b), "\r\n", "\n")), "\n") {
		k, v, _ := strings.Cut(line, "=")
		answers[k] = v
	}
	for runner, want := range map[string]string{
		"Desktop 5.1.22621.4391": "PowerShell 7.2 or later",
		"Core 7.0.3":             "System.Text.Json.Nodes",
		"Core 7.1.7":             "System.Text.Json.Nodes",
		"Core 7.2.0":             "",
		"Core 7.6.8":             "",
	} {
		got, ok := answers[runner]
		switch {
		case !ok:
			t.Errorf("%s was not asked about: %q", runner, b)
		case want == "" && got != "":
			t.Errorf("%s is refused, and its .NET carries System.Text.Json.Nodes: %s", runner, got)
		case want != "" && !strings.Contains(got, want):
			t.Errorf("%s is answered %q, which does not say %q", runner, got, want)
		}
	}
	// ⚠ AND SETUP ASKS IT, before the first case.
	text := acceptanceText(t)
	ask := "$refusal = Get-RunnerRefusal -Edition $PSVersionTable.PSEdition -Version $PSVersionTable.PSVersion\nif ($refusal) { Exit-Cannot $refusal }"
	if at := strings.Index(text, ask); at < 0 || at > strings.Index(text, "\ntry {\n") {
		t.Errorf("setup no longer refuses a PowerShell Get-RunnerRefusal names, before the first case")
	}
}

func decodeConfig(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimPrefix(b, []byte("\ufeff")), &m); err != nil {
		t.Fatalf("not a JSON object: %v\n%s", err, b)
	}
	return m
}

// acceptanceText is the suite with its line endings read as LF.
func acceptanceText(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(acceptancePath)
	if err != nil {
		t.Fatalf("the acceptance suite could not be read: %v", err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// acceptanceCaseBody is one case, from its Test-Case line to the brace that
// closes it.
func acceptanceCaseBody(t *testing.T, name string) string {
	t.Helper()
	text := acceptanceText(t)
	start := strings.Index(text, "Test-Case '"+name+"'")
	if start < 0 {
		t.Fatalf("the acceptance suite has no case named %q", name)
	}
	end := strings.Index(text[start:], "\n    }\n")
	if end < 0 {
		t.Fatalf("the case %q has no closing brace at its own indentation", name)
	}
	return text[start : start+end]
}

// acceptanceFunction is one top-level function of the suite, whole.
func acceptanceFunction(t *testing.T, name string) string {
	t.Helper()
	fn := regexp.MustCompile(`(?ms)^function ` + regexp.QuoteMeta(name) + ` \{\n.*?^\}\n`).FindString(acceptanceText(t))
	if fn == "" {
		t.Fatalf("the acceptance suite has no function %s", name)
	}
	return fn
}
