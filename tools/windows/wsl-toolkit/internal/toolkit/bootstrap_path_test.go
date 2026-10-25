// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// shippedBootstrap is the bootstrap.sh this executable carries, which is what
// `base bootstrap` runs. ⚠ It is read from the embedded copy rather than from
// scripts/common/, so a mutation row on the module's own file holds the cases.
func shippedBootstrap(t *testing.T) string {
	t.Helper()
	b, err := shippedTree.ReadFile("shipped/bootstrap.sh")
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// TestBootstrapPutsItsPrefixAfterTheSystemPath runs bootstrap.sh's own
// install_path_line in a real shell over a staged home. ⛔ Finding 62: the line
// put the prefix FIRST, so after `base bootstrap` a login shell found each agent's
// own launcher ahead of this tool's wrapper. A login file holding that line has it
// rewritten, and nothing else in the file moves. WSL-147.
func TestBootstrapPutsItsPrefixAfterTheSystemPath(t *testing.T) {
	shell := posixShell(t)
	src := shippedBootstrap(t)
	var funcs []string
	for _, name := range []string{"append_once", "append_to_login_files", "replace_once", "install_path_line"} {
		funcs = append(funcs, bootstrapFunction(t, src, name))
	}
	home := t.TempDir()
	prefix := filepath.ToSlash(filepath.Join(home, ".local"))
	before := "export PATH=\"" + prefix + "/bin:$PATH\""
	after := "export PATH=\"$PATH:" + prefix + "/bin\""
	profile := "# the account's own\numask 022\n\n# Added by bootstrap.\n" + before + "\nalias ll='ls -l'\n"
	if err := os.WriteFile(filepath.Join(home, ".profile"), []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".bash_profile"), []byte(before+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	harness := "set -eu\nSELF=bootstrap\nDRY_RUN=0\nPREFIX='" + prefix + "'\n" +
		"step() { printf 'step: %s\\n' \"$*\" >&2; }\n" + strings.Join(funcs, "\n") + "\ninstall_path_line\n"
	for run := 1; run <= 2; run++ {
		cmd := exec.Command(shell, "-c", harness)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + home}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("run %d: %v; stderr %q", run, err, stderr.String())
		}
		if run == 1 && !strings.Contains(stderr.String(), "moved "+prefix+"/bin after the system path in "+home+"/.profile") {
			t.Errorf("the rewrite was not said: %q", stderr.String())
		}
	}
	got, err := os.ReadFile(filepath.Join(home, ".profile"))
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Replace(profile, before, after, 1); string(got) != want {
		t.Errorf("~/.profile after two runs reads\n%s\nwant\n%s", got, want)
	}
	bash, err := os.ReadFile(filepath.Join(home, ".bash_profile"))
	if err != nil {
		t.Fatal(err)
	}
	if string(bash) != after+"\n" {
		t.Errorf("~/.bash_profile reads %q, want the line rewritten once", bash)
	}

	// ⭐ A home with no line gets the one that keeps the system path first.
	fresh := t.TempDir()
	cmd := exec.Command(shell, "-c", strings.Replace(harness, "PREFIX='"+prefix+"'", "PREFIX='/opt/p'", 1))
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + fresh}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("a fresh home: %v; %s", err, out)
	}
	if got, _ := os.ReadFile(filepath.Join(fresh, ".profile")); !strings.Contains(string(got), "\nexport PATH=\"$PATH:/opt/p/bin\"\n") {
		t.Errorf("a fresh ~/.profile reads %q", got)
	}
}

// TestEveryAgentProbeNamesTheRemedyForAShadowedWrapper holds the problem an
// agent probe raises over a login shell that finds the vendor's launcher first:
// it names the command that moves the line. WSL-147.
func TestEveryAgentProbeNamesTheRemedyForAShadowedWrapper(t *testing.T) {
	for _, name := range []string{"muse", "omp", "pi"} {
		probe, err := adapterScript(name, "probe.sh")
		if err != nil {
			t.Fatal(err)
		}
		var line string
		for _, l := range strings.Split(string(probe), "\n") {
			if strings.Contains(l, "problem ") && strings.Contains(l, "which is not this tool's wrapper") {
				line = l
			}
		}
		if line == "" {
			t.Fatalf("the %s probe raises no problem over a shadowed wrapper", name)
		}
		if !strings.Contains(line, "Run: base bootstrap, which moves its own line after the system path") {
			t.Errorf("the %s probe's problem names no remedy: %s", name, strings.TrimSpace(line))
		}
	}
}
