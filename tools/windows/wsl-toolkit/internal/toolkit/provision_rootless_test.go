package toolkit

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// provisionerSection is one marked section of provision.sh, between its begin and
// end comments. The markers are what let a case run the real text rather than a
// paraphrase of it.
func provisionerSection(t *testing.T, name string) string {
	t.Helper()
	script := string(provisionScript)
	begin := strings.Index(script, "# >>> "+name+": begin\n")
	end := strings.Index(script, "# <<< "+name+": end\n")
	if begin < 0 || end < begin {
		t.Fatalf("provision.sh's %s section is not marked: begin at %d, end at %d", name, begin, end)
	}
	return script[begin:end]
}

// posixShell is /bin/sh, or a skip.
func posixShell(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the provisioner is read by a POSIX shell, and this host has none to hand it to")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh on this host")
	}
	return "/bin/sh"
}

// writeStandIn puts one executable on the stand-in PATH.
func writeStandIn(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// logStandIn records its own name and arguments, one line per call. ⚠ It uses the
// shell's own suffix removal rather than basename, because the stand-in PATH holds
// nothing but these files.
const logStandIn = "printf '%s %s\\n' \"${0##*/}\" \"$*\" >> \"$TK_LOG\""

// capToolOutsidePath is the first absolute candidate provision.sh's cap_tool names
// that this host actually carries, or an empty string. A case that stages "no
// capability tool anywhere" cannot be staged on a host that has one at a path the
// section reaches for without consulting PATH.
func capToolOutsidePath() string {
	for _, candidate := range []string{"/usr/sbin/getcap", "/sbin/getcap", "/usr/bin/getcap", "/bin/getcap"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

// engineCase is one family and the commands its engine section is expected to run.
type engineCase struct {
	family string
	want   []string
}

// TestTheProvisionerInstallsWhatEachFamilysEngineNeeds runs provision.sh's own
// engine package section through a POSIX shell with every manager stood in for, and
// holds what each family installs.
//
// ⛔ WSL-86. The apt arm installed no firewall package at all, where the apk arm
// installs iptables and the pacman arm iptables-nft. podman's netavark shells out to
// nft by that name, so a Debian base refused the QEMU binary-format installer's
// rootful run with "unable to execute nft: No such file or directory" and rolled the
// distribution back. Measured on this host on 2026-09-15.
func TestTheProvisionerInstallsWhatEachFamilysEngineNeeds(t *testing.T) {
	shell := posixShell(t)
	section := provisionerSection(t, "engine packages")
	managers := []string{"apk", "pacman", "apt-get", "dnf", "tdnf", "xbps-install"}
	apk := engineCase{
		family: "apk",
		want: []string{
			"apk update",
			"apk add --no-cache podman crun fuse-overlayfs slirp4netns shadow shadow-uidmap iptables ip6tables ca-certificates tar iproute2",
			"apk add --no-cache passt",
		},
	}
	pacman := engineCase{
		family: "pacman",
		want: []string{
			"pacman -Syu --noconfirm --needed podman crun fuse-overlayfs slirp4netns shadow iptables-nft ca-certificates tar iproute2",
			"pacman -S --noconfirm --needed passt",
		},
	}
	apt := engineCase{
		family: "apt",
		want: []string{
			"apt-get update -qq",
			"apt-get install -y -qq --no-install-recommends podman uidmap fuse-overlayfs slirp4netns ca-certificates iproute2 nftables",
			"apt-get install -y -qq --no-install-recommends passt",
		},
	}
	dnf := engineCase{
		family: "dnf",
		want: []string{
			"dnf -y --setopt=install_weak_deps=False install podman fuse-overlayfs slirp4netns shadow-utils iproute tar",
			"dnf -y --setopt=install_weak_deps=False install passt",
		},
	}
	tdnf := engineCase{
		family: "tdnf",
		want: []string{
			"tdnf install -y podman shadow-utils tar iproute2",
			"tdnf install -y passt",
		},
	}
	xbps := engineCase{
		family: "xbps",
		want: []string{
			"xbps-install -Sy podman crun fuse-overlayfs slirp4netns shadow iproute2 ca-certificates tar",
			"xbps-install -Sy passt",
		},
	}
	for _, c := range []engineCase{apk, pacman, apt, dnf, tdnf, xbps} {
		dir := t.TempDir()
		for _, manager := range managers {
			writeStandIn(t, dir, manager, logStandIn)
		}
		log := filepath.Join(dir, "calls.txt")
		cmd := exec.Command(shell, "-c", "set -eu\nFAMILY="+c.family+"\n"+section)
		// ⛔ PATH IS THE STAND-IN DIRECTORY ALONE, or this host's own apt-get runs.
		cmd.Env = []string{"PATH=" + dir, "TK_LOG=" + log}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v; stderr %q", c.family, err, stderr.String())
		}
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatalf("%s installed nothing: %v", c.family, err)
		}
		got := strings.Split(strings.TrimSpace(string(b)), "\n")
		if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
			t.Errorf("%s ran\n%s\nwant\n%s", c.family, strings.Join(got, "\n"), strings.Join(c.want, "\n"))
		}
	}
}

// rootlessCase is one arrangement of newuidmap, newgidmap and the capability tools.
type rootlessCase struct {
	name       string
	family     string
	caps       map[string]string // what the stand-in getcap answers, by file name
	setuid     []string          // which of the two carry the setuid bit
	missing    []string          // which of the two are not on PATH at all
	noCapTools bool              // neither getcap nor setcap exists, and no package supplies them
	setcapDead bool              // setcap exits 0 and changes nothing
	exit       int
	says       string
	setcapCall []string
}

// TestTheProvisionerRestoresTheCapabilityRootlessIdMappingNeeds runs provision.sh's
// own rootless id mapping section with newuidmap, newgidmap, getcap and setcap stood
// in for.
//
// ⛔ WSL-86. The check asked only whether the binary was on PATH. Fedora's imported
// newuidmap carried neither the setuid bit nor cap_setuid, because a tar unpacked
// without extended attributes drops a file capability, so the base built and
// verification then answered "newuidmap: write to uid_map failed: Operation not
// permitted". Measured on this host on 2026-09-15.
func TestTheProvisionerRestoresTheCapabilityRootlessIdMappingNeeds(t *testing.T) {
	shell := posixShell(t)
	section := provisionerSection(t, "rootless id mapping")
	carried := map[string]string{
		"newuidmap": "cap_setuid=ep",
		"newgidmap": "cap_setgid=ep",
	}
	both := rootlessCase{
		name:   "both carry their capability",
		family: "dnf",
		caps:   carried,
	}
	setuid := rootlessCase{
		name:   "both are setuid, as a distribution that ships them that way",
		family: "apk",
		setuid: []string{"newuidmap", "newgidmap"},
		says:   "is setuid",
	}
	restored := rootlessCase{
		name:       "an imported rootfs dropped both, and setcap gives them back",
		family:     "dnf",
		says:       "arrived without cap_setuid and now carries it",
		setcapCall: []string{"cap_setuid+ep newuidmap", "cap_setgid+ep newgidmap"},
	}
	deadSetcap := rootlessCase{
		name:       "setcap reports success and changes nothing",
		family:     "dnf",
		setcapDead: true,
		exit:       3,
		says:       "carries neither the setuid bit nor cap_setuid",
		setcapCall: []string{"cap_setuid+ep newuidmap"},
	}
	noTools := rootlessCase{
		name:       "no package supplied the capability tools",
		family:     "apt",
		noCapTools: true,
		exit:       3,
		says:       "libcap2-bin supplied no getcap",
	}
	absent := rootlessCase{
		name:    "newuidmap is not there at all",
		family:  "dnf",
		missing: []string{"newuidmap"},
		exit:    3,
		says:    "newuidmap is missing, so a rootless engine cannot map more than one id",
	}
	outside := capToolOutsidePath()
	for _, c := range []rootlessCase{both, setuid, restored, deadSetcap, noTools, absent} {
		if c.noCapTools && outside != "" {
			t.Logf("%s: not staged, because this host carries %s and the section reaches for it by path", c.name, outside)
			continue
		}
		dir := t.TempDir()
		capsDir := filepath.Join(dir, "caps")
		if err := os.Mkdir(capsDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, value := range c.caps {
			if err := os.WriteFile(filepath.Join(capsDir, name), []byte(name+" "+value+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for _, tool := range []string{"newuidmap", "newgidmap"} {
			if standInListHas(c.missing, tool) {
				continue
			}
			writeStandIn(t, dir, tool, "exit 0")
			if standInListHas(c.setuid, tool) {
				// ⚠ fs.ModeSetuid, NOT the octal 0o4000. os.Chmod takes a
				// fs.FileMode, whose setuid flag is a high bit; 0o4755 passed
				// here sets 0755 and silently drops the bit the case is about.
				if err := os.Chmod(filepath.Join(dir, tool), 0o755|fs.ModeSetuid); err != nil {
					t.Fatal(err)
				}
			}
		}
		log := filepath.Join(dir, "calls.txt")
		if !c.noCapTools {
			writeStandIn(t, dir, "getcap", "[ -f \"$TK_CAPS/${1##*/}\" ] || exit 1\nread -r gc_line < \"$TK_CAPS/${1##*/}\"\nprintf '%s\\n' \"$gc_line\"")
			setcapBody := "printf '%s %s\\n' \"$1\" \"${2##*/}\" >> \"$TK_LOG\""
			if !c.setcapDead {
				setcapBody += "\nprintf '%s %s\\n' \"${2##*/}\" \"${1%+ep}=ep\" > \"$TK_CAPS/${2##*/}\""
			}
			writeStandIn(t, dir, "setcap", setcapBody)
		}
		// Every package manager exists and installs nothing, so a run that reaches
		// for the capability package still finds no getcap afterwards.
		for _, manager := range []string{"apk", "pacman", "apt-get", "dnf", "tdnf", "xbps-install"} {
			writeStandIn(t, dir, manager, "exit 0")
		}
		harness := "set -eu\n" +
			"say() { printf 'say: %s\\n' \"$*\" >&2; }\n" +
			"die() { printf 'die: %s\\n' \"$*\" >&2; exit 3; }\n" +
			"FAMILY=" + c.family + "\n" + section
		cmd := exec.Command(shell, "-c", harness)
		// ⛔ PATH IS THE STAND-IN DIRECTORY ALONE, or this host's own newuidmap
		// answers the case that stages one that is not installed.
		cmd.Env = []string{"PATH=" + dir, "TK_LOG=" + log, "TK_CAPS=" + capsDir}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var exited *exec.ExitError
			if !errors.As(err, &exited) {
				t.Fatalf("%s: %v; stderr %q", c.name, err, stderr.String())
			}
			code = exited.ExitCode()
		}
		if code != c.exit {
			t.Errorf("%s: exit %d, want %d; stderr %q", c.name, code, c.exit, stderr.String())
		}
		if c.says != "" && !strings.Contains(stderr.String(), c.says) {
			t.Errorf("%s: said %q, want a line carrying %q", c.name, stderr.String(), c.says)
		}
		var calls []string
		if b, err := os.ReadFile(log); err == nil {
			calls = strings.Split(strings.TrimSpace(string(b)), "\n")
		}
		if strings.Join(calls, ",") != strings.Join(c.setcapCall, ",") {
			t.Errorf("%s: setcap ran %q, want %q", c.name, calls, c.setcapCall)
		}
	}
}

// standInListHas is here rather than the standard library's generic helper because
// the shape is three lines and this package holds its own.
func standInListHas(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
