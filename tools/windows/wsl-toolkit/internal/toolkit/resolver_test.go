// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
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
	at := filepath.ToSlash(d)
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

// ⛔ AND SO DOES EVERY START OF A SEALED BASE, before it unmounts the directory.
func TestTheBootScriptWaitsForTheSharedResolver(t *testing.T) {
	h := newBootScriptHost(t)
	shared := filepath.ToSlash(filepath.Join(h.dir, "mnt", "wsl", "resolv.conf"))
	writeStandIn(t, h.bin, "sleep", "printf 'nameserver 192.0.2.7\\n' > '"+shared+"'")
	code, _, bootLog := h.run("off", h.bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if code != 0 || !strings.Contains(bootLog, "resolver refreshed") {
		t.Fatalf("a start before WSL wrote the shared resolver: exit %d, log %q", code, bootLog)
	}
	if got, _ := os.ReadFile(filepath.Join(h.dir, "etc", "resolv.conf")); string(got) != "nameserver 192.0.2.7\n" {
		t.Fatalf("the base's resolver is %q, want the one WSL wrote", got)
	}
}

// ⛔ A NAME THE SEALED BASE CANNOT RESOLVE IS NOT A VERDICT ON THE BASE. It exited 3,
// and ensure re-provisioned, restarted and could rebuild a base whose host had no
// network. It exits 4 with a `verify-pull:` line, which ensure reads as not judged
// and keeps the base. The name asked is the probe image's registry, not github.com.
func TestASealedBaseThatCannotResolveIsNotJudged(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	d := t.TempDir()
	at := filepath.ToSlash(d)
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
	if nj, ok := probeNotJudged(VerifyImage, code, stderr); !ok || nj.Registry != imageRegistry(VerifyImage) {
		t.Fatalf("ensure reads the failure as %v, %v; want not judged, so the base is kept", nj, ok)
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
