// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-188: a sealed base's resolver ----------------------------------------

// sharedResolverRun runs provision.sh's shared resolver section with /etc, /mnt/wsl
// and /proc/mounts under a directory of its own, the base's resolver naming
// 198.51.100.1, and mounts as the mount table. A sleep that is the moment WSL
// writes the shared file, naming 192.0.2.7, stands in for the wait. It answers
// what the section printed and the base's resolver after it.
func sharedResolverRun(t *testing.T, mounts string) (string, string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	d := t.TempDir()
	for _, sub := range []string{"mnt/wsl", "etc"} {
		if err := os.MkdirAll(filepath.Join(d, filepath.FromSlash(sub)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	etc := filepath.Join(d, "etc", "resolv.conf")
	if err := os.WriteFile(etc, []byte("nameserver 198.51.100.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "mounts"), []byte(mounts), 0o644); err != nil {
		t.Fatal(err)
	}
	at := shellPath(t, d)
	section := strings.NewReplacer("/mnt/wsl/resolv.conf", at+"/mnt/wsl/resolv.conf", "/etc/resolv.conf", at+"/etc/resolv.conf",
		"/proc/mounts", at+"/mounts").Replace(provisionerSection(t, "the shared resolver"))
	script := "set -eu\nsay() { printf '%s\\n' \"$*\"; }\ndie() { printf 'die: %s\\n' \"$*\"; exit 3; }\n" +
		"sleep() { printf 'nameserver 192.0.2.7\\n' > '" + at + "/mnt/wsl/resolv.conf'; }\n" + section
	out, err := exec.Command(sh, "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("the shared resolver section failed: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(etc)
	return string(out), string(got)
}

// ⛔ PROVISIONING WAITS FOR THE SHARED RESOLVER. WSL writes /mnt/wsl/resolv.conf
// as a distribution starts, and read once it was empty, so a sealed base kept the
// resolver it had.
func TestProvisioningWaitsForTheSharedResolver(t *testing.T) {
	out, got := sharedResolverRun(t, "none / ext4 rw 0 0\nnone /mnt/wsl tmpfs rw 0 0\n")
	if got != "nameserver 192.0.2.7\n" || !strings.Contains(out, "/etc/resolv.conf, 1 nameserver(s)") {
		t.Fatalf("a shared resolver WSL wrote a moment late was not taken: /etc/resolv.conf %q\n%s", got, out)
	}
}

// ⚠ AND ONLY WHILE /mnt/wsl IS MOUNTED. A sealed base that is provisioned again
// closed it at its last start, so nothing can write the file, and the wait cost
// every re-provisioning ten seconds. The base keeps the resolver it has. WSL-188.
func TestAReProvisionedSealedBaseDoesNotWaitForAClosedResolver(t *testing.T) {
	out, got := sharedResolverRun(t, "none / ext4 rw 0 0\n")
	// ⚠ The resolver's path in the line is the case's own, which stands in for it.
	if got != "nameserver 198.51.100.1\n" || !strings.Contains(out, "/etc/resolv.conf this base already had") {
		t.Fatalf("a base whose /mnt/wsl is closed waited for its resolver: /etc/resolv.conf %q\n%s", got, out)
	}
}

// sealedBootRun runs the boot script's text for a sealed base under the sh this
// host has, with /run, /etc, /mnt/wsl and /proc/mounts under a directory of its
// own, /mnt/wsl mounted and the base's resolver naming 198.51.100.1. sleep and
// umount are functions, and sleep is the moment WSL writes the shared file,
// naming 192.0.2.7. It answers the boot log and the base's resolver after it.
//
// ⚠ FUNCTIONS, NOT FILES ON PATH, so Git's sh on Windows drives the wait too. The
// first case put a stand-in sleep on PATH for /bin/sh, and skipped there. WSL-188.
func sealedBootRun(t *testing.T) (string, string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	d := t.TempDir()
	at := shellPath(t, d)
	for _, sub := range []string{"run", "mnt/wsl", "usr/local/lib/wsl-toolkit"} {
		if err := os.MkdirAll(filepath.Join(d, filepath.FromSlash(sub)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	etc := seedFile(t, d, "etc/resolv.conf", "nameserver 198.51.100.1\n")
	seedFile(t, d, "proc/mounts", "none "+at+"/mnt/wsl tmpfs rw 0 0\n")
	h := bootScriptHost{t: t, dir: d}
	script := "sleep() { printf 'nameserver 192.0.2.7\\n' > '" + at + "/mnt/wsl/resolv.conf'; }\numount() { :; }\n" + h.script("off")
	if out, err := exec.Command(sh, "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("the boot script failed: %v\n%s", err, out)
	}
	bootLog, _ := os.ReadFile(filepath.Join(d, "run", "wsl-toolkit-boot.log"))
	got, _ := os.ReadFile(etc)
	return string(bootLog), string(got)
}

// ⛔ AND SO DOES EVERY START OF A SEALED BASE, before it unmounts the directory.
func TestTheBootScriptWaitsForTheSharedResolver(t *testing.T) {
	bootLog, got := sealedBootRun(t)
	if !strings.Contains(bootLog, "resolver refreshed") || got != "nameserver 192.0.2.7\n" {
		t.Fatalf("a start before WSL wrote the shared resolver kept /etc/resolv.conf %q, log %q", got, bootLog)
	}
}

// ⛔ A SEALED BASE WHOSE RESOLVER DID NOT ANSWER IS RESTARTED, AND NOTHING MORE.
// Read as a pull the registry refused, ensure kept it from the restart too, and
// only a restart runs the boot script that refreshes /etc/resolv.conf. A base
// whose last start kept a stale resolver stayed broken, told to wait for
// docker.io. Re-provisioning it changes nothing, so it is not re-provisioned.
func TestASealedBaseWhoseResolverDidNotAnswerIsRestarted(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.stageGuest(t, "verify=unresolved-until-restart", "binfmt=ok")
	st, err := b.EnsureWith(context.Background(), false, false)
	if err != nil || !st.Healthy {
		t.Fatalf("a sealed base whose restart refreshed its resolver answered %v, healthy %v", err, st.Healthy)
	}
	if callAt(h.calls(t), "wsl --terminate "+fakeBaseName) < 0 {
		t.Fatalf("a sealed base whose resolver did not answer was not restarted: %v", h.calls(t))
	}
	if s.said("re-provisioning") || strings.Contains(h.guestScripts(t), "provision-complete") {
		t.Fatalf("a sealed base whose resolver did not answer was re-provisioned: %v", s.lines)
	}
	assertKept(t, h, b, disk)
}

// ⛔ AND ONE WHOSE RESOLVER STILL DOES NOT ANSWER IS REFUSED BY NAME, restarted
// once and kept. The refusal names the base's own resolver and the restart, and
// where the boot script says what it read, not the registry alone. WSL-188.
func TestASealedResolverThatStillDoesNotAnswerIsRefusedByName(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.stageGuest(t, "verify=unresolved")
	_, err := b.EnsureWith(context.Background(), false, false)
	var sr *SealedResolverError
	if !errors.As(err, &sr) || sr.Registry != registryName(VerifyImage) {
		t.Fatalf("a sealed resolver that never answered was refused with %v; want a SealedResolverError", err)
	}
	for _, want := range []string{
		"this sealed base's resolver, /etc/resolv.conf, did not answer for docker.io",
		fakeBaseName + " was restarted, which refreshes /etc/resolv.conf from WSL's shared file",
		"NOT re-provisioned or rebuilt",
		"/run/wsl-toolkit-boot.log",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
	restarts := 0
	for _, c := range h.calls(t) {
		if strings.HasPrefix(c, "wsl --terminate ") {
			restarts++
		}
	}
	if restarts != 1 {
		t.Errorf("the base was restarted %d times, want once: %v", restarts, h.calls(t))
	}
	if s.said("re-provisioning") {
		t.Fatalf("a sealed resolver that never answered led to re-provisioning: %v", s.lines)
	}
	assertKept(t, h, b, disk)
}

// ⛔ A NAME THE SEALED BASE CANNOT RESOLVE IS NOT A VERDICT ON THE BASE. It exited 3,
// and ensure re-provisioned, restarted and could rebuild a base whose host had no
// network. It exits 4 with its own `verify-resolver:` line, which ensure reads as
// the base's resolver: it restarts the base and keeps it. ⚠ Not a `verify-pull:`
// line, which kept the base from the restart too. The name asked is the probe
// image's registry, not github.com.
func TestASealedBaseThatCannotResolveIsNotJudged(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	d := t.TempDir()
	at := shellPath(t, d)
	if err := os.WriteFile(filepath.Join(d, "resolv.conf"), []byte("nameserver 192.0.2.53\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "mounts"), []byte("none / ext4 rw 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	section := strings.NewReplacer("/etc/resolv.conf", at+"/resolv.conf", "/proc/mounts", at+"/mounts").
		Replace(markedSection(t, verifyScript, "the shared tmpfs"))
	env := verifyEnvFor(t)
	run := func() (int, string) {
		cmd := exec.Command(sh, "-c", "set -eu\ngetent() { printf '%s\\n' \"$*\" >> '"+at+"/asked'; [ -e '"+at+"/getent.ok' ]; }\n"+section)
		cmd.Env = append(os.Environ(), "TK_SHARED_TMPFS=off", "TK_IMAGE_REGISTRY="+env["TK_IMAGE_REGISTRY"])
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		var ee *exec.ExitError
		if err != nil && !errors.As(err, &ee) {
			t.Fatal(err)
		}
		if ee != nil {
			return ee.ExitCode(), stderr.String()
		}
		return 0, stderr.String()
	}
	code, stderr := run()
	if code != verifyNotJudged || !strings.Contains(stderr, "cannot resolve a name") {
		t.Fatalf("a sealed base that cannot resolve answered exit %d: %s", code, stderr)
	}
	if sr, ok := sealedResolverFailure(code, stderr); !ok || sr.Registry != registryName(VerifyImage) {
		t.Fatalf("ensure reads the failure as %v, %v; want the base's resolver, so the base is restarted and kept", sr, ok)
	}
	if nj, ok := probeNotJudged(VerifyImage, code, stderr); ok {
		t.Fatalf("ensure reads the failure as a pull the registry refused, %v, which keeps the base from its restart", nj)
	}
	if asked, _ := os.ReadFile(filepath.Join(d, "asked")); string(asked) != "hosts "+registryName(VerifyImage)+"\n" {
		t.Errorf("the verifier resolved %q, want the probe's registry %s", asked, registryName(VerifyImage))
	}
	if err := os.WriteFile(filepath.Join(d, "getent.ok"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, stderr := run(); code != 0 {
		t.Fatalf("a sealed base that resolves answered exit %d: %s", code, stderr)
	}
}

// sealSectionRun runs the verifier's shared tmpfs section for a sealed base whose
// boot script may still be running: /mnt/wsl mounted, the boot log bootLog or
// absent where empty, a resolver naming 192.0.2.53 and a getent that answers.
// sleep is a function that is the boot script reaching its unmount. It answers
// the exit, what the section said on stderr, and how often it slept.
func sealSectionRun(t *testing.T, bootLog string) (int, string, int) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	d := t.TempDir()
	at := shellPath(t, d)
	seedFile(t, d, "resolv.conf", "nameserver 192.0.2.53\n")
	seedFile(t, d, "mounts", "none / ext4 rw 0 0\nnone /mnt/wsl tmpfs rw 0 0\n")
	if bootLog != "" {
		seedFile(t, d, "boot.log", bootLog)
	}
	section := strings.NewReplacer("/etc/resolv.conf", at+"/resolv.conf", "/proc/mounts", at+"/mounts",
		"/run/wsl-toolkit-boot.log", at+"/boot.log").Replace(markedSection(t, verifyScript, "the shared tmpfs"))
	script := "set -eu\ngetent() { :; }\n" +
		"sleep() { printf x >> '" + at + "/slept'; printf 'none / ext4 rw 0 0\\n' > '" + at + "/mounts'; " +
		"printf 'boot\\nshared tmpfs unmounted\\n' > '" + at + "/boot.log'; }\n" + section
	cmd := exec.Command(sh, "-c", script)
	cmd.Env = append(os.Environ(), "TK_SHARED_TMPFS=off", "TK_IMAGE_REGISTRY=docker.io")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	slept, _ := os.ReadFile(filepath.Join(d, "slept"))
	return code, stderr.String(), len(slept)
}

// ⛔ THE VERIFIER WAITS FOR THE BOOT SCRIPT'S UNMOUNT. WSL does not wait for a
// [boot] command: measured on WSL 3.0.3, a command run at once after a restart
// found /mnt/wsl mounted and no boot log. The boot script waits for the shared
// resolver first, so a verifier that read the mount table at once refused a base
// about to seal, and ensure re-provisioned it. WSL-188.
func TestTheVerifierWaitsForTheBootScriptsUnmount(t *testing.T) {
	if code, stderr, slept := sealSectionRun(t, ""); code != 0 || slept != 1 {
		t.Fatalf("a sealed base whose boot script unmounted a moment late answered exit %d after %d wait(s): %s", code, slept, stderr)
	}
	// ⚠ A LOG THAT SAYS THE UNMOUNT FAILED IS NOT WAITED ON, and is refused at once.
	code, stderr, slept := sealSectionRun(t, "boot\nshared tmpfs NOT unmounted\n")
	if code != 3 || slept != 0 || !strings.Contains(stderr, "is still mounted") {
		t.Fatalf("a boot script that failed to unmount answered exit %d after %d wait(s): %s", code, slept, stderr)
	}
}

// ⭐ THE REGISTRY IS RESOLVED BY ITS HOST NAME, with no port. WSL-188.
func TestTheVerifierIsHandedTheProbesRegistryName(t *testing.T) {
	for ref, want := range map[string]string{
		"docker.io/library/alpine:latest": "docker.io",
		"alpine":                          "docker.io",
		"localhost:5000/probe:1":          "localhost",
		"ghcr.io/pkgforge-dev/x:latest":   "ghcr.io",
	} {
		if got := registryName(ref); got != want {
			t.Errorf("registryName(%q) = %q, want %q", ref, got, want)
		}
	}
	if got := verifyEnvFor(t)["TK_IMAGE_REGISTRY"]; got != registryName(VerifyImage) {
		t.Errorf("the verifier is handed %q, want %q", got, registryName(VerifyImage))
	}
}

// verifyEnvFor is the verifier's environment for the default configuration.
func verifyEnvFor(t *testing.T) map[string]string {
	t.Helper()
	cfg := DefaultConfig()
	env, err := verifyEnv(cfg, "m1", "m2")
	if err != nil {
		t.Fatal(err)
	}
	return env
}
