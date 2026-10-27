// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-188: the engine's settings are drop-ins -------------------------------

// engineSettingsRun runs provision.sh's engine settings section with /etc and the
// account's home under dir, and answers what it printed. ⚠ mirrors.sh's functions
// come first, as the provisioning payload carries them, and registry is the
// configuration's base.registry, nil for none. WSL-196.
func engineSettingsRun(t *testing.T, dir string, registry ...*RegistryConfig) string {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	at := filepath.ToSlash(dir)
	section := strings.ReplaceAll(string(mirrorsScript)+"\n"+provisionerSection(t, "the engine's settings"), "/etc/containers/", at+"/etc/containers/")
	var r *RegistryConfig
	if len(registry) > 0 {
		r = registry[0]
	}
	script := "set -eu\nTK_USER=agent\nTK_HOME=" + at + "/home\n" + string(shellAssignments(registryEnv(r))) +
		"say() { printf '%s\\n' \"$*\"; }\ndie() { printf 'die: %s\\n' \"$*\"; exit 3; }\nchown() { :; }\npasta() { :; }\n" + section
	out, err := exec.Command(sh, "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("the engine settings section failed: %v\n%s", err, out)
	}
	return string(out)
}

// seedFile writes body at rel under dir.
func seedFile(t *testing.T, dir, rel, body string) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ⛔ A RE-PROVISIONING LEAVES THE OPERATOR'S MAIN FILES ALONE. It wrote the
// account's containers.conf and /etc/containers/registries.conf whole, so a
// mirror or a proxy set in either was gone after the next ensure. Its settings
// are drop-ins, and the account's sorts before engines.sh's systemd switch.
func TestAReProvisioningKeepsTheOperatorsEngineSettings(t *testing.T) {
	parallelCase(t)
	dir := t.TempDir()
	const (
		mirror = "[[registry]]\nlocation = \"docker.io\"\n[[registry.mirror]]\nlocation = \"mirror.example\"\n"
		proxy  = "[engine]\nenv = [\"HTTPS_PROXY=http://proxy.example:3128\"]\n"
	)
	registries := seedFile(t, dir, "etc/containers/registries.conf", mirror)
	containers := seedFile(t, dir, "home/.config/containers/containers.conf", proxy)
	engineSettingsRun(t, dir)
	if got, _ := os.ReadFile(registries); string(got) != mirror {
		t.Errorf("the operator's registries.conf was rewritten:\n%s", got)
	}
	if got, _ := os.ReadFile(containers); string(got) != proxy {
		t.Errorf("the operator's containers.conf was rewritten:\n%s", got)
	}
	reg, _ := os.ReadFile(filepath.Join(dir, "etc", "containers", "registries.conf.d", "99-wsl-toolkit.conf"))
	if !strings.Contains(string(reg), `short-name-mode = "enforcing"`) {
		t.Errorf("the registries drop-in does not enforce short names:\n%s", reg)
	}
	const dropIn = "10-wsl-toolkit.conf"
	conf, _ := os.ReadFile(filepath.Join(dir, "home", ".config", "containers", "containers.conf.d", dropIn))
	if !strings.Contains(string(conf), `cgroup_manager = "cgroupfs"`) || !strings.Contains(string(conf), `events_logger = "file"`) {
		t.Errorf("the account's drop-in does not carry cgroupfs and the file logger:\n%s", conf)
	}
	if dropIn >= "50-wsl-toolkit-cgroup.conf" {
		t.Error("the account's drop-in sorts after engines.sh's switch to the systemd manager, and would undo it")
	}
}

// ⚠ THE MAIN FILE AN EARLIER RELEASE WROTE IS THIS TOOL'S, AND IT GOES, so its
// slirp4netns pin cannot outlast a pasta installed since. A main registries.conf
// in the v1 format takes no drop-in, so it is written whole, as before.
func TestAnEarlierReleasesEngineSettingsMoveToTheDropIns(t *testing.T) {
	parallelCase(t)
	dir := t.TempDir()
	containers := seedFile(t, dir, "home/.config/containers/containers.conf",
		"# Written by wsl-toolkit. cgroupfs works with either configured init.\n[engine]\ncgroup_manager = \"cgroupfs\"\n[network]\ndefault_rootless_network_cmd = \"slirp4netns\"\n")
	registries := seedFile(t, dir, "etc/containers/registries.conf", "[registries.search]\nregistries = ['docker.io']\n")
	out := engineSettingsRun(t, dir)
	if _, err := os.Stat(containers); !os.IsNotExist(err) {
		t.Errorf("the containers.conf an earlier release wrote is still there: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(registries); !strings.Contains(string(got), `short-name-mode = "enforcing"`) || strings.Contains(string(got), "[registries.search]") {
		t.Errorf("a v1 registries.conf was not written whole:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "etc", "containers", "registries.conf.d", "99-wsl-toolkit.conf")); err == nil {
		t.Error("a drop-in was written beside a v1 registries.conf, which the engine refuses to mix")
	}
}
