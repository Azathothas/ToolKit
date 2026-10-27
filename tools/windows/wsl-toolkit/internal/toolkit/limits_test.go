// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// -- WSL-136: a limit is enforced, and the answer says whether it was ---------

func TestAJobsLimitsAreReadAsPodmanReadsThem(t *testing.T) {
	for in, want := range map[string]int64{
		"64m": 64 << 20, "64M": 64 << 20, "512MiB": 512 << 20, "1.5g": 3 << 29, "2G": 2 << 30, "7340032": 7340032,
	} {
		l, err := ParseResourceLimits(in, "")
		if err != nil || l.MemoryBytes != want {
			t.Errorf("--memory %s read as %d, %v; want %d", in, l.MemoryBytes, err, want)
		}
	}
	for _, in := range []string{"1m", "64x", "m", "-64m", "64 m"} {
		if _, err := ParseResourceLimits(in, ""); err == nil || !strings.Contains(err.Error(), "--memory") {
			t.Errorf("--memory %q was accepted, or its refusal does not name the flag: %v", in, err)
		}
	}
	if l, err := ParseResourceLimits("", "1.5"); err != nil || l.CPUs != 1.5 || l.MemoryBytes != 0 {
		t.Errorf("--cpus 1.5 read as %+v, %v", l, err)
	}
	for _, in := range []string{"0", "0.001", "x", "NaN", "-1"} {
		if _, err := ParseResourceLimits("", in); err == nil || !strings.Contains(err.Error(), "--cpus") {
			t.Errorf("--cpus %q was accepted, or its refusal does not name the flag: %v", in, err)
		}
	}
	if l, err := ParseResourceLimits("", ""); err != nil || l.Any() {
		t.Errorf("no limit read as %+v, %v", l, err)
	}
	if _, err := parseLimits("jobs.memory", "nope", "jobs.cpus", ""); err == nil || !strings.Contains(err.Error(), "jobs.memory") {
		t.Errorf("a configuration's limit is refused without naming its key: %v", err)
	}
}

// TestAJobsLimitsReachPodmanAndTheWrapperReadsThem: the limits are flags of
// the engine, and the wrapper asks the container's own cgroup about each one
// that was asked for. A job with no limit runs the wrapper it always had.
func TestAJobsLimitsReachPodmanAndTheWrapperReadsThem(t *testing.T) {
	runner := &Runner{}
	spec := JobSpec{Image: "docker.io/library/alpine:latest", ContainerLifecycle: ContainerEphemeral,
		Resources: ResourceLimits{MemoryBytes: 64 << 20, CPUs: 1.5}}
	script := string(runner.containerScript(spec, "wtk-one", jobLayout("/home/toolkit", "one"), "marker"))
	for _, want := range []string{
		"'--memory' '67108864'", "'--cpus' '1.5'",
		"read -r m < /sys/fs/cgroup/memory.max", "read -r c _ < /sys/fs/cgroup/cpu.max",
		`marker-limits-$v`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("a job with limits does not carry %s:\n%s", want, script)
		}
	}
	// ⛔ A limit file reading max, or none at all, is a measured no. Read from the
	// wrapper itself, because the podman line quotes its empty pattern.
	check := limitsCheck(spec.Resources, "marker")
	for _, want := range []string{`case "$m" in ''|max) v=no ;; esac`, `case "$c" in ''|max) v=no ;; esac`} {
		if !strings.Contains(check, want) {
			t.Errorf("the wrapper does not carry %s:\n%s", want, check)
		}
	}
	spec.Resources = ResourceLimits{CPUs: 2}
	script = string(runner.containerScript(spec, "wtk-one", jobLayout("/home/toolkit", "one"), "marker"))
	if strings.Contains(script, "memory.max") || strings.Contains(script, "'--memory'") {
		t.Errorf("a job that asked for processors only is asked about memory:\n%s", script)
	}
	spec.Resources = ResourceLimits{}
	plain := string(runner.containerScript(spec, "wtk-two", jobLayout("/home/toolkit", "two"), "marker"))
	if !strings.Contains(plain, shellQuote("printf '\\n%s\\n' 'marker' >&2\nexec /bin/sh /job.sh\n")) {
		t.Errorf("a job with no limit no longer runs the wrapper it always had:\n%s", plain)
	}
	for _, absent := range []string{"--memory", "--cpus", "limits-"} {
		if strings.Contains(plain, absent) {
			t.Errorf("a job with no limit carries %s:\n%s", absent, plain)
		}
	}
}

// TestTheLimitsLineIsStrippedAndRead: the wrapper's two lines leave the stream,
// in whatever pieces they arrive, and the second says what the cgroup carried.
func TestTheLimitsLineIsStrippedAndRead(t *testing.T) {
	// ⚠ Not a real token's 32 hex characters, which the secrets check reads as an
	// identifier. The strippers match whatever token they are given.
	const token = "wtk-started-test-token"
	for _, verdict := range []string{LimitsEnforced, LimitsUnenforced, LimitsUnknown} {
		var got bytes.Buffer
		w, l := newLimitsMarkers(&got, token)
		marker := newMarkerStripper(w, token)
		stream := "\n" + token + "\n\n" + token + "-limits-" + verdict + "\nreal error\n"
		for i := 0; i < len(stream); i += 7 {
			end := min(i+7, len(stream))
			if _, err := marker.Write([]byte(stream[i:end])); err != nil {
				t.Fatal(err)
			}
		}
		if err := errors.Join(marker.Flush(), l.Flush()); err != nil {
			t.Fatal(err)
		}
		if got.String() != "real error\n" || l.Verdict() != verdict || !marker.Seen() {
			t.Errorf("%s: the stream came out %q and read %s", verdict, got.String(), l.Verdict())
		}
	}
	var got bytes.Buffer
	w, l := newLimitsMarkers(&got, token)
	if _, err := w.Write([]byte("no line at all\n")); err != nil {
		t.Fatal(err)
	}
	_ = l.Flush()
	if l.Verdict() != LimitsUnknown || got.String() != "no line at all\n" {
		t.Errorf("a container that said nothing read %s, and the stream came out %q", l.Verdict(), got.String())
	}
}

// TestTheWrapperReadsTheContainersOwnCgroupInARealShell runs the wrapper's
// check against a cgroup tree laid out in a temporary directory.
func TestTheWrapperReadsTheContainersOwnCgroupInARealShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the wrapper is POSIX shell, and this host has none to run it in")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	both := ResourceLimits{MemoryBytes: 64 << 20, CPUs: 1}
	for _, c := range []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"both carried", map[string]string{"cgroup.controllers": "cpu memory", "memory.max": "67108864", "cpu.max": "100000 100000"}, LimitsEnforced},
		{"memory unlimited", map[string]string{"cgroup.controllers": "cpu memory", "memory.max": "max", "cpu.max": "100000 100000"}, LimitsUnenforced},
		{"processors unlimited", map[string]string{"cgroup.controllers": "cpu memory", "memory.max": "67108864", "cpu.max": "max 100000"}, LimitsUnenforced},
		// ⛔ The root cgroup has no limit files at all, which is WSL-60's base.
		{"no limit files", map[string]string{"cgroup.controllers": "cpu memory"}, LimitsUnenforced},
		{"no cgroup v2 tree", map[string]string{}, LimitsUnknown},
	} {
		root := t.TempDir()
		for name, body := range c.files {
			if err := os.WriteFile(filepath.Join(root, name), []byte(body+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		script := strings.ReplaceAll(limitsCheck(both, "tok"), "/sys/fs/cgroup", root)
		cmd := exec.Command(sh, "-c", script)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if want := "\ntok-limits-" + c.want + "\n"; stderr.String() != want {
			t.Errorf("%s: the wrapper wrote %q, want %q", c.name, stderr.String(), want)
		}
	}
}

// TestAProjectsLimitsLoadAndAreHeldToTheGrammar: jobs.memory and jobs.cpus are
// copied by the loader, and a value podman cannot read is refused by its key.
func TestAProjectsLimitsLoadAndAreHeldToTheGrammar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", filepath.Join(dir, "state"))
	prev := ExplicitConfigPath
	t.Cleanup(func() { ExplicitConfigPath = prev })
	write := func(jobs map[string]any) string {
		body, err := json.Marshal(map[string]any{"schema": ConfigSchema, "base": map[string]any{"name": DefaultBaseName}, "jobs": jobs})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "wsl-toolkit.json")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	ExplicitConfigPath = write(map[string]any{"container_lifecycle": "persistent", "memory": "512m", "cpus": 1.5})
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Jobs.Memory != "512m" || cfg.Jobs.CPUs != 1.5 {
		t.Fatalf("the loader kept jobs as %+v", cfg.Jobs)
	}
	ExplicitConfigPath = write(map[string]any{"container_lifecycle": "persistent", "memory": "lots"})
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "jobs.memory") {
		t.Fatalf("an unreadable jobs.memory was not refused by its key: %v", err)
	}
	ExplicitConfigPath = write(map[string]any{"container_lifecycle": "persistent", "cpus": -2})
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "jobs.cpus") {
		t.Fatalf("a negative jobs.cpus was not refused by its key: %v", err)
	}
}

// -- WSL-130 and WSL-136: what base ensure applies to a base's engines ---------

func TestTheEnginesRunAsRootWithTheConfigurationsSettings(t *testing.T) {
	cfg := adapterBase()
	cfg.Base.RootEngine, cfg.Base.Kvm = true, true
	b := &Base{cfg: cfg}
	req := b.enginesRequest(nil)
	if req.User != "root" || !bytes.Equal(req.Script, enginesPayload()) {
		t.Fatalf("engines.sh runs as %q, or is not the embedded script", req.User)
	}
	for k, want := range map[string]string{
		"TK_USER": "herdr", "TK_SYSTEMD": "true", "TK_ROOT_ENGINE": "true", "TK_RUNTIME_DIR": "false", "TK_KVM": "true",
	} {
		if req.Env[k] != want {
			t.Errorf("engines.sh is given %s=%q, want %q", k, req.Env[k], want)
		}
	}
}

// TestEachEnginesSettingOwnsItsFiles reads the script, which runs only as root
// in a base: each opt-in setting writes its file and removes it when it is off,
// and the account's engine moves to the systemd manager only behind a bus.
func TestEachEnginesSettingOwnsItsFiles(t *testing.T) {
	src := string(enginesScript)
	for _, want := range []string{
		"Delegate=cpu cpuset io memory pids",
		`install_file "$tmp" "$ROOT_CONF" 0644`, `rm -f "$ROOT_CONF"`,
		`install_file "$tmp" "$PROFILE" 0644`, `rm -f "$PROFILE"`,
		`install_file "$tmp" "$KVM_PATH" 0644`, `rm -f "$KVM_PATH"`,
		`ExecStart=chgrp kvm /dev/kvm`, `ExecStart=chmod g+rw /dev/kvm`,
		`[ "$systemd_up" = true ] || die "base.kvm keeps /dev/kvm usable through a systemd unit`,
		`if [ "$systemd_up" != true ] || [ ! -S /run/dbus/system_bus_socket ]; then`,
		`write_manager remove`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("engines.sh no longer carries %q", want)
		}
	}
	bus := strings.Index(src, `if [ -S "/run/user/$uid/bus" ] && systemctl is-active --quiet "user@$uid.service" && write_manager write; then`)
	if bus < 0 {
		t.Error("the account's engine is moved to the systemd manager without asking whether its session bus answers")
	}
	if !strings.Contains(src, "printf 'engines-complete\\n'") {
		t.Error("engines.sh no longer prints the line ensureEngines asserts")
	}
}

func TestKvmNeedsSystemdAndIsReadByOpeningIt(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Base.Kvm = true
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "base.systemd") {
		t.Fatalf("base.kvm without systemd was accepted: %v", err)
	}
	cfg.Base.Systemd = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("base.kvm with systemd was refused: %v", err)
	}
	for out, want := range map[string]string{"x\nkvm usable\n": KvmUsable, "kvm unusable\n": KvmUnusable, "kvm absent\n": KvmAbsent, "kvm maybe\n": "", "": ""} {
		if got := parseKvm(out); got != want {
			t.Errorf("%q read as %q, want %q", out, got, want)
		}
	}
	if p := kvmProblem(true, KvmUnusable, "toolkit"); !strings.Contains(p, ToolCommand("base ensure")) {
		t.Errorf("an unusable node asked for is %q", p)
	}
	if p := kvmProblem(true, KvmAbsent, "toolkit"); !strings.Contains(p, "nested virtualization") {
		t.Errorf("an absent node asked for is %q", p)
	}
	for _, p := range []string{kvmProblem(false, KvmUnusable, "toolkit"), kvmProblem(true, KvmUsable, "toolkit"), kvmProblem(true, "", "toolkit")} {
		if p != "" {
			t.Errorf("a node nobody asked for, a usable one, or one not read is a problem: %q", p)
		}
	}
	if !strings.Contains(string(verifyScript), "if (exec 3<>/dev/kvm) 2>/dev/null; then kvm=usable") {
		t.Error("the verifier no longer opens /dev/kvm to read it")
	}
}

// TestTheVerifierMeasuresDelegationWhereTheEngineMakesCgroups is the reading
// that misled: on an Arch base with systemd the verifier's own cgroup could
// not be written, and every container was in the account's user manager.
func TestTheVerifierMeasuresDelegationWhereTheEngineMakesCgroups(t *testing.T) {
	src := string(verifyScript)
	user := strings.Index(src, `if [ -d "$cg_user" ] && mkdir "$cg_user/wsl-toolkit-delegation-probe.$$" 2>/dev/null; then`)
	own := strings.Index(src, `if mkdir "$cg_probe" 2>/dev/null; then`)
	if user < 0 || own < 0 || user > own {
		t.Fatal("the verifier does not ask the account's user manager before its own cgroup")
	}
	// ⛔ The user manager's answer is a row of its own, so the parser can weigh it
	// against the engine's manager.
	if block := src[user:own]; !strings.Contains(block, "printf 'cgroup-account-delegated yes\\n'") ||
		strings.Contains(block, "cgroup-delegated yes") {
		t.Fatalf("the user manager's probe does not answer on its own row:\n%s", block)
	}
	// Measured on wsl-toolkit-f5a once base ensure had delegated it.
	measured := "cgroup-version v2\ncgroup-controllers cpuset cpu io memory hugetlb pids misc\ncgroup-self 0::/non-systemd\n" +
		"cgroup-account-delegated yes\ncgroup-account-controllers cpuset cpu io memory pids\ncgroup-delegated no\ncgroup-under none\n" +
		"cgroup-manager systemd\nengine-rootless true\ncgroup-limit 67108864\n"
	cg := parseCapabilities(measured)
	if !cg.Delegated || cg.Mechanism != "systemd" || cg.Enforced != "yes" || cg.StatsUsable != "yes" || cg.Manager != "systemd" ||
		cg.AccountControllers != "cpuset cpu io memory pids" {
		t.Fatalf("the delegated base reads as %+v", cg)
	}
	first := strings.Index(engineInfoScript, `if [ "${cgmgr:-}" = systemd ] && mkdir "$cguser" 2>/dev/null; then`)
	then := strings.Index(engineInfoScript, `elif mkdir "$cgdir" 2>/dev/null; then`)
	if first < 0 || then < 0 || first > then {
		t.Error("inspect's delegation row does not ask the account's user manager, beside a systemd engine, before its own cgroup")
	}
	if !strings.Contains(engineInfoScript, "case $f in *CgroupManager*) cgmgr=$v ;; esac") {
		t.Error("inspect does not keep the engine's cgroup manager to weigh the user manager's probe")
	}
}

// TestAUserManagerTheEngineDoesNotUseIsNotDelegation is the base 7.0.0 built
// with systemd and no session bus: user@UID.service runs and the account can
// write it, and podman keeps cgroupfs, so a container's limit is missing.
// ⛔ Stats and limits stand or fall together, so delegated is false, and the
// finding names the engine's manager. WSL-136.
func TestAUserManagerTheEngineDoesNotUseIsNotDelegation(t *testing.T) {
	out := "cgroup-version v2\ncgroup-self 0::/non-systemd\n" +
		"cgroup-account-delegated yes\ncgroup-account-controllers cpu memory pids\ncgroup-delegated no\ncgroup-under none\n" +
		"cgroup-manager cgroupfs\nengine-rootless true\ncgroup-limit missing\n"
	cg := parseCapabilities(out)
	if cg.Delegated || cg.Mechanism != "none" || cg.StatsUsable != "no" || cg.Enforced != "no" {
		t.Fatalf("a user manager beside cgroupfs reads as %+v", cg)
	}
	r, ok := cgroupRemediation(cg, true)
	if !ok || !strings.Contains(r.What, "cgroupfs") || strings.Contains(r.What, "holds no delegated cgroup") {
		t.Fatalf("its finding is %+v", r)
	}
	// ⭐ The script's own cgroup still counts as it did in 7.0.x, whatever the
	// engine's manager.
	own := parseCapabilities("cgroup-version v2\ncgroup-account-delegated no\ncgroup-delegated yes\ncgroup-under none\n" +
		"cgroup-manager cgroupfs\nengine-rootless true\ncgroup-limit 67108864\n")
	if !own.Delegated || own.Mechanism != "delegated" || own.StatsUsable != "yes" {
		t.Fatalf("a delegated own cgroup reads as %+v", own)
	}
}

// -- WSL-130: base image load ---------------------------------------------------

func TestTheImagesALoadNamedAreReadFromItsOutput(t *testing.T) {
	for out, want := range map[string]string{
		"Loaded image: docker.io/library/busybox:latest\n":                                         "docker.io/library/busybox:latest",
		"Getting image source signatures\r\nLoaded image(s): docker.io/a/b:1, docker.io/c/d:2\r\n": "docker.io/a/b:1 docker.io/c/d:2",
		"Copying blob sha256:abc\n":                                                                "",
	} {
		if got := strings.Join(parseLoadedImages(out), " "); got != want {
			t.Errorf("%q read as %q, want %q", out, got, want)
		}
	}
}
