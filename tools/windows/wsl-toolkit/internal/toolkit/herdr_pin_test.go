// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
)

// TestTheDefaultHerdrIsTheBuildThisRepositoryPublished runs the herdr adapter's install.sh
// against stand-ins for the base's commands, as far as the digest check, once for each
// way a release is chosen. WSL-141.
//
// ⭐ THE DEFAULT IS THIS REPOSITORY'S BUILD: the version the script pins, downloaded from
// the wsl-toolkit release that published it, under the name release.yml gives it, and
// checked against that architecture's pinned digest. herdr's own aarch64 asset is a
// different build, so a default that went back to herdr's release could never pass.
// ⛔ A CONFIGURED VERSION IS STILL herdr's OWN RELEASE, as it was in 7.0.0, and a nightly
// still comes from its tag.
//
// The stand-in for curl serves bytes that match no digest, so every run stops at the
// refusal, and the case also holds that nothing was installed.
func TestTheDefaultHerdrIsTheBuildThisRepositoryPublished(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh runs under a POSIX shell, which the Linux job has")
	}
	found := map[string]string{}
	for _, tool := range []string{"sh", "cat", "id", "uname", "cut", "mktemp", "sha256sum", "base64"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s is not on PATH", tool)
		}
		found[tool] = path
	}
	script, err := adapterScript("herdr", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	pinned := func(name string) string {
		t.Helper()
		m := regexp.MustCompile(`(?m)^` + name + `=(\S+)$`).FindSubmatch(script)
		if m == nil {
			t.Fatalf("install.sh assigns no %s at the start of a line", name)
		}
		return string(m[1])
	}
	version := pinned("HERDR_VERSION")
	digests := map[string]string{
		"x86_64":  pinned("HERDR_X86_64_PINNED_SHA256"),
		"aarch64": pinned("HERDR_AARCH64_PINNED_SHA256"),
	}

	root := t.TempDir()
	home, bin, dest := filepath.Join(root, "home"), filepath.Join(root, "bin"), filepath.Join(root, "usr-local-bin")
	for _, dir := range []string{home, bin, dest} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, body string) {
		t.Helper()
		if err := forksafe.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	asked := filepath.Join(root, "asked")
	for _, name := range []string{"pacman", "sshd", "ssh-keygen", "ss"} {
		write(name, "exit 0\n")
	}
	write("cat", "if [ \"$1\" = /proc/1/comm ]; then echo systemd; exit 0; fi\nexec "+found["cat"]+" \"$@\"\n")
	write("getent", "printf 'herdr:x:1000:1000::%s:/bin/bash\\n' \"$STUB_HOME\"\n")
	write("id", "if [ \"$1\" = -gn ]; then echo herdr; exit 0; fi\nexec "+found["id"]+" \"$@\"\n")
	write("systemctl", "if [ \"$1\" = is-active ]; then [ \"${STUB_ACTIVE:-}\" = yes ]; exit; fi\n"+
		"if [ \"$1\" = is-enabled ]; then echo masked; fi\nexit 0\n")
	// ⚠ install IS THE FIRST STEP AFTER THE BINARY, and this one stops the script there, so
	// a run whose download matches writes nothing outside the case's own directory.
	write("install", "echo 'install: stopped here by the case' >&2\nexit 7\n")
	write("uname", "if [ \"$1\" = -m ]; then echo \"$STUB_ARCH\"; exit 0; fi\nexec "+found["uname"]+" \"$@\"\n")
	// curl records the URL it was asked for and serves bytes no digest matches.
	write("curl", "out=\nfor a in \"$@\"; do\n  case $a in https://*) printf '%s\\n' \"$a\" > \"$STUB_ASKED\" ;; esac\ndone\n"+
		"while [ $# -gt 0 ]; do if [ \"$1\" = --output ]; then out=$2; shift; fi; shift; done\n"+
		"printf 'not herdr\\n' > \"$out\"\n")
	config := base64.StdEncoding.EncodeToString([]byte("onboarding = false\n"))

	run := func(arch string, extra ...string) (int, string, string) {
		t.Helper()
		_ = os.Remove(asked)
		cmd := exec.Command(found["sh"], "-s")
		cmd.Stdin = strings.NewReader(string(script))
		cmd.Env = append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base", "TK_SSH_CLIENT_KEY=ssh-ed25519 AAAA",
			"TK_FILE_CONFIG_TOML_B64="+config, "TK_HERDR_BIN="+filepath.Join(dest, "herdr"),
			"STUB_HOME="+home, "STUB_ARCH="+arch, "STUB_ASKED="+asked)
		cmd.Env = append(cmd.Env, extra...)
		out, err := cmd.CombinedOutput()
		var exited *exec.ExitError
		if err != nil && !errors.As(err, &exited) {
			t.Fatalf("install.sh did not start: %v", err)
		}
		url, _ := os.ReadFile(asked)
		return cmd.ProcessState.ExitCode(), string(out), strings.TrimSpace(string(url))
	}
	refused := func(name string, code int, out, want string) {
		t.Helper()
		if code != 3 || !strings.Contains(out, "does not match its pinned digest: expected "+want+",") {
			t.Fatalf("%s answered exit %d, and a download that matches no digest must stop the adapter naming %s:\n%s", name, code, want, out)
		}
		if left, _ := os.ReadDir(dest); len(left) != 0 {
			t.Fatalf("%s left %d file(s) where herdr is installed, and a refused download installs nothing", name, len(left))
		}
	}

	ours := regexp.MustCompile(`^https://github\.com/` + regexp.QuoteMeta(ReleaseRepo) + `/releases/download/wsl-toolkit-v[0-9]+\.[0-9]+\.[0-9]+/(.+)$`)
	for _, arch := range []string{"x86_64", "aarch64"} {
		code, out, url := run(arch)
		refused("the default on "+arch, code, out, digests[arch])
		m := ours.FindStringSubmatch(url)
		if m == nil {
			t.Fatalf("the default on %s downloads %q, and its digest is the one a wsl-toolkit release of %s publishes", arch, url, ReleaseRepo)
		}
		if want := "herdr-" + version + "-linux-" + arch; m[1] != want {
			t.Fatalf("the default on %s downloads %s, and release.yml names that build %s", arch, m[1], want)
		}
	}

	configured := strings.Repeat("c3", 32)
	code, out, url := run("x86_64", "TK_ADAPTER_VERSION=0.10.1", "TK_ADAPTER_SHA256_X86_64="+configured)
	refused("a configured version", code, out, configured)
	if want := "https://github.com/herdrdev/herdr/releases/download/v0.10.1/herdr-linux-x86_64"; url != want {
		t.Fatalf("a configured version downloads %q, and in 7.0.0 it downloaded herdr's own %s", url, want)
	}

	const tag = "herdr-nightly-20260915-052779c4159e"
	nightly := "https://github.com/" + ReleaseRepo + "/releases/download/" + tag
	code, out, url = run("aarch64", "TK_ADAPTER_VERSION="+tag, "TK_ADAPTER_URL_BASE="+nightly, "TK_ADAPTER_SHA256_AARCH64="+configured)
	refused("a nightly", code, out, configured)
	if want := nightly + "/herdr-linux-aarch64"; url != want {
		t.Fatalf("a nightly downloads %q, and the nightly publishes %s", url, want)
	}

	// ⛔ A RELEASE INSTALLED UNDER A RUNNING SERVER IS SAID TO SERVE NOTHING YET. The
	// server is never restarted by an ensure, so an existing base that moves to the new
	// pin keeps serving the herdr it started with, and the ensure has to say so. The
	// stand-in curl's bytes are the release here, so the download passes its digest.
	// WSL-141.
	const kept = "the running herdr server keeps the herdr it started with until it restarts"
	served := "TK_ADAPTER_SHA256_X86_64=" + sha256Hex([]byte("not herdr\n"))
	code, out, _ = run("x86_64", "TK_ADAPTER_VERSION=0.10.1", served)
	if code != 7 || strings.Contains(out, kept) {
		t.Fatalf("a first install with no server running answered exit %d, and it reaches the stop with nothing to say about a server:\n%s", code, out)
	}
	if err := os.Remove(filepath.Join(dest, "herdr")); err != nil {
		t.Fatal(err)
	}
	code, out, _ = run("x86_64", "TK_ADAPTER_VERSION=0.10.1", served, "STUB_ACTIVE=yes")
	if code != 7 || !strings.Contains(out, kept) || !strings.Contains(out, "base exec --root -c 'systemctl restart wsl-toolkit-herdr.service'") {
		t.Fatalf("a release installed under a running server answered exit %d, and it says the server keeps the old herdr and how to restart it:\n%s", code, out)
	}
	code, out, _ = run("x86_64", "TK_ADAPTER_VERSION=0.10.1", served, "STUB_ACTIVE=yes")
	if code != 7 || strings.Contains(out, kept) || !strings.Contains(out, "matches its pinned digest") {
		t.Fatalf("an ensure that installed nothing answered exit %d, and a server already running the installed herdr is not stale:\n%s", code, out)
	}
}
