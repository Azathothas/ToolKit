package toolkit

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// TestTheProvisioningRunIsTheSharedTableThenTheProvisioner holds the one payload
// provisioning sends. WSL-70: the provisioner kept a second map of package names
// that knew six managers where the shared table knew twelve.
func TestTheProvisioningRunIsTheSharedTableThenTheProvisioner(t *testing.T) {
	b := &Base{cfg: Config{Base: BaseConfig{Name: "wsl-toolkit-tbl", User: "agent"}}}
	req := b.provisionRequest(AutomountOff, BaseInteropOff, BaseToolsetDeveloper, io.Discard)
	want := append(append(append([]byte{}, packagesScript...), '\n'), provisionScript...)
	if !bytes.Equal(req.Script, want) {
		t.Fatalf("the provisioning run sends %d bytes that are not the shared table and then the provisioner", len(req.Script))
	}
	if req.User != "root" || req.Env["TK_TOOLSET"] != BaseToolsetDeveloper {
		t.Errorf("the provisioning run is as %q with toolset %q", req.User, req.Env["TK_TOOLSET"])
	}
	if !bytes.Contains(packagesScript, []byte("\n# >>> shared package table: begin\n")) {
		t.Error("the embedded table does not carry its begin marker")
	}
	provision := string(provisionScript)
	for _, own := range []string{"package_table() {", "package_for() {", "detect_provider() {", "detect_os_id() {"} {
		if strings.Contains(provision, own) {
			t.Errorf("provision.sh defines %s itself, where the shared table is its one home", own)
		}
	}
	for _, call := range []string{`PROVIDER=$(detect_provider)`, `OS_ID=$(detect_os_id)`, `developer_resolved=$(package_for "$developer_name")`} {
		if !strings.Contains(provision, call) {
			t.Errorf("provision.sh does not carry %q", call)
		}
	}
}

// TestTheProvisionerInstallsAnEngineOnlyThroughTheManagersItHasPackagesFor runs
// provision.sh's package manager section through a real POSIX shell, after the
// shared table, on a PATH holding one stand-in manager and a stand-in uname.
//
// ⛔ The table's detection knows twelve managers and the provisioner has engine
// packages for six, so a manager it finds and cannot install an engine through is
// refused by name rather than reaching an install that has no arm for it. WSL-70.
func TestTheProvisionerInstallsAnEngineOnlyThroughTheManagersItHasPackagesFor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the provisioner is read by a POSIX shell, and this host has none to hand it to")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh on this host")
	}
	script := string(provisionScript)
	begin := strings.Index(script, "# >>> package manager: begin\n")
	end := strings.Index(script, "# <<< package manager: end\n")
	if begin < 0 || end < begin {
		t.Fatalf("the provisioner's package manager section is not marked: begin at %d, end at %d", begin, end)
	}
	harness := string(packagesScript) + "\nset -eu\n" +
		"say() { printf 'say: %s\\n' \"$*\" >&2; }\n" +
		"die() { printf 'die: %s\\n' \"$*\" >&2; exit 3; }\n" + script[begin:end]
	cases := []struct {
		manager string
		exit    int
		said    string
	}{
		{"apk", 0, "say: package manager: apk on "},
		{"xbps-install", 0, "say: package manager: xbps on "},
		{"zypper", 3, "die: zypper is installed, and this script installs a container engine only through apk, apt, dnf, pacman, tdnf or xbps"},
		{"emerge", 3, "die: emerge is installed, and this script installs a container engine only through apk, apt, dnf, pacman, tdnf or xbps"},
		{"", 3, "die: no package manager this script knows: tried apk apt dnf emerge pacman pkg pkg_add pkgin tdnf xbps yum zypper"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		write := func(name, body string) {
			if err := os.WriteFile(dir+"/"+name, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		write("uname", "echo Linux")
		if c.manager != "" {
			write(c.manager, "exit 0")
		}
		cmd := exec.Command("/bin/sh", "-c", harness)
		// ⛔ PATH IS THIS DIRECTORY ALONE. The host's own apt-get would otherwise be
		// found ahead of the stand-in.
		cmd.Env = []string{"PATH=" + dir}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var exited *exec.ExitError
			if !errors.As(err, &exited) {
				t.Fatalf("%q: %v", c.manager, err)
			}
			code = exited.ExitCode()
		}
		if code != c.exit || !strings.HasPrefix(strings.TrimSpace(stderr.String()), c.said) {
			t.Errorf("%q: exit %d and %q, want exit %d and a line starting %q", c.manager, code, stderr.String(), c.exit, c.said)
		}
	}
}

// provisionerDeveloperSection is provision.sh's developer section, after the shared
// table, with every package manager and the two reporting functions stood in for.
func provisionerDeveloperSection(t *testing.T, family, osID string) string {
	t.Helper()
	script := string(provisionScript)
	begin := strings.Index(script, "    # >>> developer packages: begin\n")
	end := strings.Index(script, "    # <<< developer packages: end\n")
	if begin < 0 || end < begin {
		t.Fatalf("the provisioner's developer section is not marked: begin at %d, end at %d", begin, end)
	}
	var s strings.Builder
	s.Write(packagesScript)
	s.WriteString("\nset -eu\n")
	s.WriteString("say() { printf 'say: %s\\n' \"$*\" >&2; }\n")
	s.WriteString("die() { printf 'die: %s\\n' \"$*\" >&2; exit 3; }\n")
	for _, manager := range []string{"apk", "pacman", "apt-get", "dnf", "tdnf", "xbps-install"} {
		s.WriteString(strings.ReplaceAll(manager, "-", "_") + "_stub() { printf '" + manager + " %s\\n' \"$*\" >&2; }\n")
	}
	s.WriteString("FAMILY=" + family + "\nPROVIDER=" + family + "\nOS_ID=" + osID + "\n")
	section := script[begin:end]
	// ⛔ EACH MANAGER IS REPLACED EXACTLY ONCE, OR THE CASE STOPS. A section that
	// stopped calling one by that name would otherwise run this host's own.
	for _, manager := range []string{"apk", "pacman", "apt-get", "dnf", "tdnf", "xbps-install"} {
		call := " " + manager + " "
		if n := strings.Count(section, call); n != 1 {
			t.Fatalf("the developer section calls %q %d times, want once", manager, n)
		}
		section = strings.Replace(section, call, " "+strings.ReplaceAll(manager, "-", "_")+"_stub ", 1)
	}
	s.WriteString(section)
	return s.String()
}

// TestTheProvisionerResolvesItsDeveloperPackagesThroughTheSharedTable runs the
// provisioner's own developer section through a real POSIX shell, after the shared
// table, for each family the provisioner installs through. The first four are what
// the per-family lists installed before, byte for byte; Photon, Void and Chimera are
// the table's own measurements, which those lists had never been.
func TestTheProvisionerResolvesItsDeveloperPackagesThroughTheSharedTable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the provisioner is read by a POSIX shell, and this host has none to hand it to")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh on this host")
	}
	cases := []struct {
		family, osID, want string
		skipped            []string
	}{
		{"apk", "alpine", "apk add --no-cache bash build-base curl git jq nodejs npm openssh-client ripgrep tmux unzip", nil},
		{"pacman", "arch", "pacman -S --noconfirm --needed bash base-devel curl git jq nodejs npm openssh ripgrep tmux unzip", nil},
		{"apt", "debian", "apt-get install -y -qq --no-install-recommends bash build-essential curl git jq nodejs npm openssh-client ripgrep tmux unzip", nil},
		{"dnf", "fedora", "dnf -y --setopt=install_weak_deps=False install bash gcc gcc-c++ make curl git jq nodejs npm openssh-clients ripgrep tmux unzip", nil},
		{"tdnf", "photon", "tdnf install -y bash gcc make curl git jq nodejs openssh-clients tmux unzip", []string{"npm", "ripgrep"}},
		{"xbps", "void", "xbps-install -Sy bash base-devel curl git jq nodejs openssh ripgrep tmux unzip", []string{"npm"}},
		{"apk", "chimera", "apk add --no-cache bash base-devel curl git jq nodejs openssh tmux unzip", []string{"npm", "ripgrep"}},
	}
	for _, c := range cases {
		cmd := exec.Command("/bin/sh", "-c", provisionerDeveloperSection(t, c.family, c.osID))
		cmd.Env = []string{"PATH=/usr/bin:/bin"}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			var exited *exec.ExitError
			if !errors.As(err, &exited) {
				t.Fatalf("%s on %s: %v", c.family, c.osID, err)
			}
			t.Errorf("%s on %s: exit %d; stderr %q", c.family, c.osID, exited.ExitCode(), stderr.String())
			continue
		}
		var installs, skipped []string
		for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
			switch {
			case strings.HasPrefix(line, "say: "+c.osID+"'s "+c.family+" carries no package for "):
				skipped = append(skipped, strings.TrimPrefix(line, "say: "+c.osID+"'s "+c.family+" carries no package for "))
			case line != "":
				installs = append(installs, line)
			}
		}
		if len(installs) != 1 || installs[0] != c.want {
			t.Errorf("%s on %s installed %q, want %q", c.family, c.osID, installs, c.want)
		}
		if strings.Join(skipped, ",") != strings.Join(c.skipped, ",") {
			t.Errorf("%s on %s named %q as not carried, want %q", c.family, c.osID, skipped, c.skipped)
		}
	}
}
