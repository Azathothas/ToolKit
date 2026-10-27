// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// -- WSL-130: the engines step is the owner's, as the adapters are -------------

// ⛔ A GUEST'S ENSURE LEAVES THE ENGINES AS THE OWNER SET THEM. engines.sh takes
// away the file of every opt-in setting that is off, and a guest's configuration
// names none of them, so a guest's ensure undid what the owner turned on. The
// owner's ensure runs it, so the case is not passed by a step that never runs.
func TestOnlyTheOwnersEnsureAppliesTheEngines(t *testing.T) {
	for _, guest := range []bool{false, true} {
		h := newFakeHost(t)
		b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
		h.stageGuest(t, "verify=ok", "binfmt=ok")
		b.guest = guest
		if _, err := b.EnsureWith(context.Background(), false, false); err != nil {
			t.Fatalf("guest %v: the ensure failed: %v", guest, err)
		}
		ran := strings.Contains(h.guestScripts(t), `: "${TK_ROOT_ENGINE:?TK_ROOT_ENGINE is required}"`)
		if ran == guest {
			t.Fatalf("guest %v: engines.sh ran %v; only the owner's ensure runs it", guest, ran)
		}
		if guest && !s.said("the engines are left as they are") {
			t.Fatalf("a guest's ensure skipped the engines without saying so: %v", s.lines)
		}
		assertKept(t, h, b, disk)
	}
}

// ⛔ A FAILED ENGINES STEP FAILS THE ENSURE, AS A FAILED ADAPTER DOES, AND THE
// QEMU STEP STILL RUNS. Only an opt-in setting fails it: the base was asked for
// something it could not be given. The state comes back with the error. WSL-130.
func TestAFailedEnginesStepFailsTheEnsureWithTheState(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.stageGuest(t, "verify=ok", "binfmt=ok", "engines=refused")
	st, err := b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), "the engines: engines.sh exited 3: base.kvm keeps /dev/kvm usable") {
		t.Fatalf("a refused engines step answered %v; want its reason", err)
	}
	if errors.Is(err, ErrNotOwner) || !st.Healthy {
		t.Fatalf("a refused engines step came back without the base's state: %+v", st)
	}
	if !strings.Contains(h.guestScripts(t), "TK_BINFMT_IMAGE='"+BinfmtImage+"'") {
		t.Fatal("a refused engines step kept the QEMU step from running")
	}
	assertKept(t, h, b, disk)
}

// -- WSL-130 and WSL-136: what engines.sh writes, in a real shell -------------

// enginesHost is one run of engines.sh as this account, with every root path
// under a temporary directory and the system's tools stood in for.
type enginesHost struct {
	t    *testing.T
	dir  string
	uid  string
	user string
	log  string
	// registry is the configuration's base.registry, nil for none. WSL-196.
	registry *RegistryConfig
}

// newEnginesHost skips where there is no POSIX shell to run the script in.
//
// ⚠ THE SCRIPT RUNS AS ROOT IN A BASE, so here every /etc, /run, /sys, /dev,
// /tmp and linger path, and the account's home, is rewritten into the case's
// own directory, and systemctl, loginctl, podman, runuser and the group tools
// are stubs that log what they were asked.
func newEnginesHost(t *testing.T) *enginesHost {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("engines.sh is POSIX shell, and this host has none to run it in")
	}
	for _, tool := range []string{"sh", "sha256sum", "mktemp", "getent", "base64"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool + " on this host")
		}
	}
	me, err := user.Current()
	if err != nil {
		t.Skip("this account has no name: " + err.Error())
	}
	h := &enginesHost{t: t, dir: t.TempDir(), uid: me.Uid, user: me.Username}
	h.log = filepath.Join(h.dir, "calls")
	bin := filepath.Join(h.dir, "bin")
	for name, body := range map[string]string{
		"systemctl": `echo "systemctl $*" >> "$FAKE_LOG"; exit 0`,
		"loginctl":  `echo "loginctl $*" >> "$FAKE_LOG"; exit 1`,
		"podman":    `echo "podman $*" >> "$FAKE_LOG"; echo "cgroupfs file"`,
		"runuser":   `shift 3; exec "$@"`,
		"groupadd":  `echo "groupadd $*" >> "$FAKE_LOG"`,
		"usermod":   `echo "usermod $*" >> "$FAKE_LOG"`,
	} {
		h.write(filepath.Join(bin, name), "#!/bin/sh\n"+body+"\n", 0o755)
	}
	h.mkdir("run/systemd/system")
	return h
}

func (h *enginesHost) path(rel string) string { return filepath.Join(h.dir, filepath.FromSlash(rel)) }

func (h *enginesHost) mkdir(rel string) {
	if err := os.MkdirAll(h.path(rel), 0o755); err != nil {
		h.t.Fatal(err)
	}
}

func (h *enginesHost) write(path, body string, mode os.FileMode) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		h.t.Fatal(err)
	}
}

// socket makes a listening socket at rel, which is what [ -S ] asks for.
func (h *enginesHost) socket(rel string) {
	h.mkdir(filepath.Dir(rel))
	l, err := net.Listen("unix", h.path(rel))
	if err != nil {
		h.t.Skip("no unix socket here: " + err.Error())
	}
	h.t.Cleanup(func() { _ = l.Close() })
}

func (h *enginesHost) read(rel string) string {
	data, err := os.ReadFile(h.path(rel))
	if err != nil {
		return ""
	}
	return string(data)
}

func (h *enginesHost) script() string {
	d := filepath.ToSlash(h.dir)
	s := string(enginesPayload())
	for _, r := range [][2]string{
		{`home=$(getent passwd "$TK_USER" 2>/dev/null | cut -d: -f6) || home=`, "home=" + d + "/home"},
		{"/usr/lib/systemd/user/", d + "/usr/lib/systemd/user/"},
		{" /lib/systemd/user/", " " + d + "/lib/systemd/user/"},
		{"[ -c /dev/kvm ]", "[ -e /dev/kvm ]"},
		{"/dev/kvm", d + "/dev/kvm"},
		{"/var/lib/systemd/linger", d + "/linger"},
		{"/sys/fs/cgroup", d + "/sys"},
		{"/tmp/wsl-toolkit-run-", d + "/tmp/wsl-toolkit-run-"},
		{"/usr/local/lib/", d + "/usr/local/lib/"},
		{"/etc/", d + "/etc/"},
		{"/run/", d + "/run/"},
	} {
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	return s
}

// run runs engines.sh with the three opt-in settings, and answers its output
// and how long it took.
func (h *enginesHost) run(rootEngine, runtimeDir, kvm bool) (string, time.Duration) {
	h.t.Helper()
	b := func(v bool) string {
		if v {
			return "true"
		}
		return "false"
	}
	cmd := exec.Command("sh")
	cmd.Stdin = strings.NewReader(h.script())
	cmd.Env = append(os.Environ(),
		"PATH="+h.path("bin")+":"+os.Getenv("PATH"), "FAKE_LOG="+h.log,
		"TK_USER="+h.user, "TK_SYSTEMD=true",
		"TK_ROOT_ENGINE="+b(rootEngine), "TK_RUNTIME_DIR="+b(runtimeDir), "TK_KVM="+b(kvm),
		"TK_RUN_PREFIX="+filepath.ToSlash(h.dir)+"/tmp/wsl-toolkit-run-",
		"TK_RUN_STATE_B64="+base64.StdEncoding.EncodeToString(staleRunStatePayload()),
		"TK_BOOT_B64="+base64.StdEncoding.EncodeToString(bootScript),
		// ⚠ The mount helper's paths are the case's own, so nothing is written on
		// the host running it. WSL-198.
		"TK_MOUNT_HELPER_B64="+base64.StdEncoding.EncodeToString([]byte(mountHelperIn(h.t, h.dir))))
	for k, v := range registryEnv(h.registry) {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	start := time.Now()
	if err := cmd.Run(); err != nil {
		h.t.Fatalf("engines.sh failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "engines-complete") {
		h.t.Fatalf("engines.sh did not reach its last line:\n%s", out.String())
	}
	return out.String(), time.Since(start)
}

// TestEnginesShWritesEachSettingsFileInARealShell: each setting's file says
// what it is for, a setting turned off takes its file away, and the delegation
// moves the account's engine only behind a session bus. WSL-130, WSL-136.
func TestEnginesShWritesEachSettingsFileInARealShell(t *testing.T) {
	h := newEnginesHost(t)
	const (
		rootConf = "etc/containers/containers.conf.d/50-wsl-toolkit-root.conf"
		profile  = "etc/profile.d/wsl-toolkit-runtime-dir.sh"
		unit     = "etc/systemd/system/wsl-toolkit-kvm.service"
		delegate = "etc/systemd/system/user@.service.d/50-wsl-toolkit-delegate.conf"
		manager  = "home/.config/containers/containers.conf.d/50-wsl-toolkit-cgroup.conf"
	)
	h.write(h.path("dev/kvm"), "", 0o600)
	d := filepath.ToSlash(h.dir)

	// ⛔ No user bus can appear, so there is no wait for one, and the account's
	// engine keeps cgroupfs.
	out, took := h.run(true, true, true)
	if took > 5*time.Second {
		t.Errorf("engines.sh waited %s for a session bus no dbus.socket can start", took)
	}
	if !strings.Contains(h.read(delegate), "Delegate=cpu cpuset io memory pids") {
		t.Errorf("the delegation drop-in reads %q", h.read(delegate))
	}
	if _, err := os.Stat(h.path("linger/" + h.user)); err != nil {
		t.Errorf("the account is not left lingering: %v", err)
	}
	if h.read(manager) != "" || !strings.Contains(out, "keeps cgroupfs") {
		t.Errorf("with no session bus the account's engine was moved: %q\n%s", h.read(manager), out)
	}
	// ⛔ No system bus: root's engine gets cgroupfs, and always the file log.
	root := h.read(rootConf)
	if !strings.Contains(root, `cgroup_manager = "cgroupfs"`) || !strings.Contains(root, `events_logger = "file"`) {
		t.Errorf("root's engine on a base with no system bus reads %q", root)
	}
	prof := h.read(profile)
	if !strings.Contains(prof, "_wtk_run="+d+"/tmp/wsl-toolkit-run-"+h.uid) {
		t.Errorf("the runtime-dir profile reads %q", prof)
	}
	if err := os.MkdirAll(h.path("tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	sourced, err := exec.Command("sh", "-c", ". "+h.path(profile)+"; printf '%s' \"$XDG_RUNTIME_DIR\"").Output()
	if err != nil || string(sourced) != d+"/tmp/wsl-toolkit-run-"+h.uid {
		t.Errorf("a login shell that reads the profile names %q as its runtime directory, %v", sourced, err)
	}
	kvm := h.read(unit)
	for _, want := range []string{"ExecStart=chgrp kvm " + d + "/dev/kvm", "ExecStart=chmod g+rw " + d + "/dev/kvm", "WantedBy=multi-user.target"} {
		if !strings.Contains(kvm, want) {
			t.Errorf("the kvm unit does not carry %q:\n%s", want, kvm)
		}
	}
	calls, _ := os.ReadFile(h.log)
	if !strings.Contains(string(calls), "systemctl enable wsl-toolkit-kvm.service") || !strings.Contains(string(calls), "usermod -aG kvm "+h.user) {
		t.Errorf("the kvm unit was not enabled, or the account not added to kvm:\n%s", calls)
	}

	// ⛔ A system bus: root's engine keeps its own systemd manager. A user bus:
	// the account's engine moves to the systemd manager.
	h.socket("run/dbus/system_bus_socket")
	h.socket("run/user/" + h.uid + "/bus")
	h.run(true, false, false)
	if root := h.read(rootConf); strings.Contains(root, "cgroup_manager") || !strings.Contains(root, `events_logger = "file"`) {
		t.Errorf("root's engine on a base with a system bus reads %q", root)
	}
	if !strings.Contains(h.read(manager), `cgroup_manager = "systemd"`) {
		t.Errorf("with a session bus the account's engine reads %q", h.read(manager))
	}
	// ⭐ Each setting turned off takes its own file away.
	if h.read(profile) != "" || h.read(unit) != "" {
		t.Errorf("a setting turned off left its file: profile %q, unit %q", h.read(profile), h.read(unit))
	}
	h.run(false, false, false)
	if h.read(rootConf) != "" {
		t.Errorf("root_engine turned off left %q", h.read(rootConf))
	}
}
