// acceptance_isolation_test.go - the acceptance suite's cases stand apart from
// the instance they run on. WSL-132.
//
// ⛔ THE MARKER CASE WROTE AN INSTANCE'S OWN CONFIGURATION WHOLE, with three keys,
// so the `base ensure` it ran read an instance that asked for systemd as asking for
// none. Four more cases failed on such an instance: two read its own file where
// they meant the working directory's, and two gave a helper a home that asked for
// no systemd. The suite runs only against a real machine, so these read the cases
// themselves and run the marker case's one helper wherever PowerShell is.
//
// SPDX-License-Identifier: 0BSD

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// TestTheMarkerCaseGivesTheConfigurationBackByteForByte holds the case to its
// restore: the bytes it read, written back in a finally, so a case that fails or
// throws restores too, and read back as part of its verdict.
//
// ⚠ ReadAllText and WriteAllText were the restore before. They decode and encode,
// so a byte order mark or a byte that is not UTF-8 did not come back.
func TestTheMarkerCaseGivesTheConfigurationBackByteForByte(t *testing.T) {
	body := acceptanceCaseBody(t, "the guest own marker decides what the base was built from")
	const keep, put, back = "$saved = [IO.File]::ReadAllBytes($cfgPath)",
		"[IO.File]::WriteAllBytes($cfgPath, $saved)",
		"$now = [IO.File]::ReadAllBytes($cfgPath)"
	const verdict = `if (-not $restored) { return "the configuration at $cfgPath was not restored byte for byte" }`
	for _, want := range []string{keep, put, back, verdict, "Edit-ConfigKey -Text $text -Section 'base' -Key 'image'"} {
		if !strings.Contains(body, want) {
			t.Errorf("the marker case no longer says %q", want)
		}
	}
	finally := strings.Index(body, "finally {")
	if at := strings.Index(body, put); finally < 0 || at < finally {
		t.Errorf("the marker case writes its saved bytes back outside a finally, so a case that throws leaves the file changed")
	}
	if at := strings.Index(body, back); at < strings.Index(body, put) {
		t.Errorf("the marker case does not read the file back after it restores it")
	}
	// ⛔ A CONFIGURATION WRITTEN WHOLE IS THE DEFECT, whatever keys it names.
	if strings.Contains(body, "ConvertTo-Json") {
		t.Errorf("the marker case writes a configuration of its own again, and every key it leaves out reads as its default")
	}
}

// TestEditConfigKeyChangesOnlyItsKey runs the suite's own Edit-ConfigKey and
// compares what it wrote with what it read: the one key differs, and nothing else
// does, systemd and passwordless sudo included.
func TestEditConfigKeyChangesOnlyItsKey(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("Edit-ConfigKey is PowerShell and pwsh is not on PATH. The Windows host runs this case")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "edit.ps1")
	driver := acceptanceFunction(t, "Edit-ConfigKey") + `
$ErrorActionPreference = 'Stop'
$text = [Text.UTF8Encoding]::new($false).GetString([IO.File]::ReadAllBytes($args[0]))
$edited = Edit-ConfigKey -Text $text -Section 'base' -Key 'image' -Value $args[2]
[IO.File]::WriteAllText($args[1], $edited, [Text.UTF8Encoding]::new($false))
`
	if err := os.WriteFile(script, []byte(driver), 0o600); err != nil {
		t.Fatal(err)
	}
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
		{"an instance that asks for systemd", full, func() map[string]any {
			m := decodeConfig(t, []byte(strings.TrimPrefix(full, "\ufeff")))
			m["base"].(map[string]any)["image"] = lie
			return m
		}()},
		{"no file", "", map[string]any{
			"schema": "wsl-toolkit-config/1",
			"base":   map[string]any{"image": lie},
		}},
	} {
		in, out := filepath.Join(dir, "in.json"), filepath.Join(dir, "out.json")
		if err := os.WriteFile(in, []byte(tc.in), 0o600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", script, in, out, lie)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: Edit-ConfigKey failed: %v\n%s", tc.name, err, b)
		}
		got, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if have := decodeConfig(t, got); !reflect.DeepEqual(have, tc.want) {
			t.Errorf("%s: Edit-ConfigKey wrote\n%s\nand only base.image should differ from\n%s", tc.name, got, tc.in)
		}
	}
}

// TestNoCaseReadsTheInstancesOwnConfigurationForAnotherFile holds four cases to
// the configuration they mean, on an instance that has a file of its own.
//
// ⛔ A NAMED INSTANCE'S OWN FILE OUTRANKS THE WORKING DIRECTORY, so a case that
// asks which project file wins asks from an empty home. ⛔ AND A HOME WITH NO FILE
// ASKS FOR NO SYSTEMD, so a case that gives a helper a home of its own writes the
// suite's base into it. Measured on an instance that asked for systemd: all four
// failed, and the rest of the suite passed.
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
	// ⚠ The base it copies leaves the name out, so a scratch home names its base
	// as it did with no file.
	if fn := acceptanceFunction(t, "Get-SuiteBase"); !strings.Contains(fn, "if ($p.Name -ne 'name')") {
		t.Errorf("Get-SuiteBase copies the base's name, which a scratch home did not name before:\n%s", fn)
	}
}

func decodeConfig(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
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
