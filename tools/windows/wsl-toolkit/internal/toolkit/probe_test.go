// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// -- the fake guest: answers a case stages for the verifier, binfmt.sh, uname --

// fakeGuestFile holds what the fake guest answers, one `script=answer` per line.
const fakeGuestFile = "guest"

// stageGuest tells the fake WSL how its guest answers: verify=ok|unpullable,
// binfmt=ok|refused.
func (h *fakeHost) stageGuest(t *testing.T, answers ...string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(h.state, fakeGuestFile), []byte(strings.Join(answers, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
}

// guestScripts is every script the fake guest was handed, oldest first.
func (h *fakeHost) guestScripts(t *testing.T) string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(h.state, "scripts"))
	return string(data)
}

// assigned reads one variable out of the assignments Exec puts before a script.
func assigned(script []byte, name string) string {
	m := regexp.MustCompile(`(?m)^` + name + `='([^']*)'; export`).FindSubmatch(script)
	if m == nil {
		return ""
	}
	return string(m[1])
}

// fakeGuestScript answers a guest script the case staged an answer for, and ok is
// false for any other, which the fake WSL then refuses.
//
// ⚠ NOTHING RUNS. The verifier's answer is the rows a working base prints, with
// the marker joined from the halves the verifier was sent; its pull failure is
// the exit code and the line verify.sh gives one.
func fakeGuestScript(state string, script []byte) (int, bool) {
	data, err := os.ReadFile(filepath.Join(state, fakeGuestFile))
	if err != nil {
		return 0, false
	}
	answers := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			answers[k] = v
		}
	}
	if f, err := os.OpenFile(filepath.Join(state, "scripts"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		_, _ = f.Write(append(script, []byte("\n-- end of script --\n")...))
		_ = f.Close()
	}
	switch {
	case bytes.HasSuffix(script, []byte("uname -m\n")):
		fmt.Println("x86_64")
		return 0, true
	case bytes.Contains(script, []byte(`: "${TK_M1:?TK_M1 is required}"`)):
		switch answers["verify"] {
		case "ok":
			fmt.Printf("automount ro\nengine podman version 6.1.4\n%s%s\nbinfmt-handlers 8\nbinfmt-own 8\nbinfmt-missing\nbinfmt-watch none\n",
				assigned(script, "TK_M1"), assigned(script, "TK_M2"))
			return 0, true
		case "unpullable":
			fmt.Println("automount ro")
			fmt.Fprintln(os.Stderr, "verify-pull: Error: initializing source docker://alpine:latest: pinging container registry registry-1.docker.io: Get \"https://registry-1.docker.io/v2/\": dial tcp: lookup registry-1.docker.io: no such host")
			return verifyNotJudged, true
		}
	case bytes.Contains(script, []byte(`: "${TK_BINFMT_IMAGE:?TK_BINFMT_IMAGE is required}"`)):
		if arch := assigned(script, "TK_ARCH"); arch != "" {
			fmt.Println("binfmt-ready " + assigned(script, "TK_HANDLER"))
			return 0, true
		}
		switch answers["binfmt"] {
		case "ok":
			fmt.Println("binfmt-complete")
			return 0, true
		case "refused":
			fmt.Fprintln(os.Stderr, "Error: initializing source docker://tonistiigi/binfmt: toomanyrequests: You have reached your unauthenticated pull rate limit")
			fmt.Fprintln(os.Stderr, "binfmt: the QEMU binary-format installer did not complete")
			return 3, true
		}
	}
	return 0, false
}

// -- WSL-174: a probe image that cannot be pulled is not a broken base ---------

// ⛔ THE DEFECT: verify.sh pulled the probe image inside its run, so Docker Hub
// refusing one address read as an engine that cannot run a container. Ensure then
// re-provisioned, which is a full upgrade and a restart of WSL's VM under every
// job, and a provisioning that also failed rebuilt the base. The fake engine
// answers here, so the unfixed path reaches the removal.
func TestAProbeImageThatCannotBePulledKeepsTheBase(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	h.stageGuest(t, "verify=unpullable")
	_, err := b.EnsureWith(context.Background(), false, false)
	var nj *ProbeNotJudgedError
	if !errors.As(err, &nj) || nj.Registry != "docker.io" || nj.Image != VerifyImage {
		t.Fatalf("an unpullable probe image answered %v; want a ProbeNotJudgedError naming docker.io", err)
	}
	for _, want := range []string{
		"not judged: registry docker.io unreachable",
		"no such host",
		fakeBaseName + " was NOT re-provisioned, restarted or rebuilt",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
	if s.said("re-provisioning") {
		t.Fatalf("a probe that was not judged led to re-provisioning: %v", s.lines)
	}
	if i := callAt(h.calls(t), "wsl --terminate"); i >= 0 {
		t.Fatalf("a probe that was not judged restarted the base: %v", h.calls(t))
	}
	assertKept(t, h, b, disk)
}

// ⭐ The status half: the problem names the registry, and is not the engine's.
func TestAStatusSaysAProbeThatCannotPullWasNotJudged(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	h.stageGuest(t, "verify=unpullable")
	st, err := b.Status(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(st.Problems, "\n")
	if !strings.Contains(joined, "not judged: registry docker.io unreachable") || strings.Contains(joined, "a container did not run") {
		t.Fatalf("a probe that could not pull reported %q", joined)
	}
	if st.Healthy {
		t.Fatal("a probe that was not judged reported the base usable")
	}
}

// probeImageSection is the verifier's own probe image section.
func probeImageSection(t *testing.T) string {
	t.Helper()
	script := string(verifyScript)
	begin := strings.Index(script, "# >>> the probe image: begin\n")
	end := strings.Index(script, "# <<< the probe image: end\n")
	if begin < 0 || end < begin {
		t.Fatalf("the verifier's probe image section is not marked: begin at %d, end at %d", begin, end)
	}
	return "set -eu\n" + script[begin:end] + "printf 'probe-image ready\\n'\n"
}

// TestTheProbeImagePullIsItsOwnBoundedStep runs the verifier's probe image section
// through a real POSIX shell, with podman and timeout stood in for.
//
// ⛔ WSL-174. A pull refused by the registry exits 4 with a `verify-pull:` line,
// and never reaches the run. An engine that fails `image exists` with anything
// but 1 is left to the run, which reports the engine's own words, stale run state
// included.
func TestTheProbeImagePullIsItsOwnBoundedStep(t *testing.T) {
	sh := posixShell(t)
	const image = "docker.io/library/alpine:latest"
	const refused = `echo 'Error: initializing source docker://alpine:latest: pinging container registry registry-1.docker.io: dial tcp: lookup registry-1.docker.io: no such host' >&2; exit 125`
	cases := []struct {
		name, exists, pull, timeout string
		exit                        int
		reason                      string
		pulled                      bool
	}{
		{"present: no pull", "exit 0", "exit 0", "", 0, "", false},
		{"absent and pulled", "exit 1", "exit 0", "", 0, "", true},
		{"absent and the registry refuses, the measured defect", "exit 1", refused, "", verifyNotJudged, "lookup registry-1.docker.io: no such host", true},
		{"absent and the pull outlives its bound", "exit 1", "exit 0", "exit 124", verifyNotJudged, "the pull did not finish within 120 seconds", false},
		{"the engine fails: left to the run", "echo 'Error: current system boot ID differs from cached boot ID' >&2; exit 125", refused, "", 0, "", false},
	}
	for _, c := range cases {
		dir := t.TempDir()
		log := filepath.Join(dir, "calls")
		podman := logStandIn + "\ncase \"$1 $2\" in\n  'image exists') " + c.exists + " ;;\n  'pull -q') " + c.pull + " ;;\nesac\nexit 0"
		writeStandIn(t, dir, "podman", podman)
		for _, tool := range []string{"sed", "tail"} {
			if p, err := exec.LookPath(tool); err == nil {
				writeStandIn(t, dir, tool, "exec "+p+" \"$@\"")
			}
		}
		timeout := `shift; exec "$@"`
		if c.timeout != "" {
			timeout = c.timeout
		}
		writeStandIn(t, dir, "timeout", logStandIn+"\n"+timeout)
		cmd := exec.Command(sh, "-c", probeImageSection(t))
		cmd.Env = []string{"PATH=" + dir, "TK_IMAGE=" + image, "TK_LOG=" + log}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var exited *exec.ExitError
			if !errors.As(err, &exited) {
				t.Fatalf("%s: %v", c.name, err)
			}
			code = exited.ExitCode()
		}
		calls, _ := os.ReadFile(log)
		if code != c.exit {
			t.Errorf("%s: exit %d, want %d; stderr %q; calls %q", c.name, code, c.exit, stderr.String(), calls)
		}
		if c.exit == 0 && !strings.Contains(stdout.String(), "probe-image ready") {
			t.Errorf("%s: the section stopped before its end: %q", c.name, stdout.String())
		}
		if reason, ok := lastPrefixed(stderr.String(), "verify-pull: "); c.exit != 0 && (!ok || !strings.Contains(reason, c.reason)) {
			t.Errorf("%s: stderr %q does not carry a `verify-pull:` line naming %q", c.name, stderr.String(), c.reason)
		}
		pulled, bounded := false, false
		for _, line := range strings.Split(string(calls), "\n") {
			pulled = pulled || line == "podman pull -q "+image
			bounded = bounded || line == "timeout 120 podman pull -q "+image
		}
		if pulled != c.pulled {
			t.Errorf("%s: pulled %v, want %v; calls %q", c.name, pulled, c.pulled, calls)
		}
		if pulled && !bounded {
			t.Errorf("%s: the pull ran without its bound: %q", c.name, calls)
		}
	}
}

// ⚠ The exit code and the line are read together: a code of 4 with no line is the
// engine's, and keeps the engine's words.
func TestANotJudgedProbeNeedsItsCodeAndItsLine(t *testing.T) {
	if _, ok := probeNotJudged(VerifyImage, verifyNotJudged, "Error: something else\n"); ok {
		t.Fatal("exit 4 with no verify-pull line read as not judged")
	}
	if _, ok := probeNotJudged(VerifyImage, 125, "verify-pull: no such host\n"); ok {
		t.Fatal("a verify-pull line under exit 125 read as not judged")
	}
	nj, ok := probeNotJudged("ghcr.io/example/probe:1", verifyNotJudged, "wsl: a warning\nverify-pull: no such host\n")
	if !ok || nj.Registry != "ghcr.io" || nj.Reason != "no such host" {
		t.Fatalf("a not judged probe read as %+v, %v", nj, ok)
	}
	if got := imageRegistry("alpine"); got != "docker.io" {
		t.Fatalf("an unqualified reference names registry %q", got)
	}
}

// ⛔ `gc --images` PRUNED THE PROBE IMAGE, so the next probe was a pull. The kept
// images are each held by a container through the prune, and the plan spares them.
func TestGcImagesKeepsTheProbeAndTheQEMUImages(t *testing.T) {
	script := keptImagesPrune()
	prune := strings.Index(script, "podman image prune -a -f")
	for i, ref := range []string{VerifyImage, BinfmtImage} {
		create := strings.Index(script, "podman create --name "+shellQuote(keptImagePin(i))+" "+shellQuote(ref))
		removed := strings.LastIndex(script, "podman rm -f "+shellQuote(keptImagePin(i)))
		if create < 0 || prune < 0 || create > prune || removed < prune {
			t.Errorf("%s is not held by a container through the prune:\n%s", ref, script)
		}
		name, _, _ := strings.Cut(ref, "@")
		if _, kept := keptImageReason(name); !kept {
			t.Errorf("the plan lists %s for removal", name)
		}
	}
	if _, kept := keptImageReason("docker.io/library/debian:bookworm"); kept {
		t.Error("the plan spares an image nothing here runs")
	}
}

// ⭐ The dry run reads as the apply does: the kept images are under kept.
func TestTheGcImagesPlanListsTheKeptImagesAsKept(t *testing.T) {
	binfmt, _, _ := strings.Cut(BinfmtImage, "@")
	remove, kept := planImages([]OwnedThing{
		{Name: VerifyImage}, {Name: "docker.io/library/debian:bookworm"}, {Name: binfmt},
	})
	if len(remove) != 1 || remove[0] != "docker.io/library/debian:bookworm" {
		t.Fatalf("the plan removes %v; want the one image nothing here runs", remove)
	}
	if len(kept) != 2 || !strings.Contains(kept[0], VerifyImage) || !strings.Contains(kept[1], binfmt) {
		t.Fatalf("the plan keeps %v; want the probe's and the QEMU installer's", kept)
	}
}

// -- WSL-181: the QEMU step fails no ensure ------------------------------------

// ⛔ THE DEFECT: a registry that refused the QEMU installer made every ensure exit
// 2 over a base whose engine had just run a container. It is a problem on the
// answer now, and the base reads usable.
func TestAFailedQEMUStepIsAProblemAndNotAFailedEnsure(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.stageGuest(t, "verify=ok", "binfmt=refused")
	st, err := b.EnsureWith(context.Background(), false, false)
	if err != nil {
		t.Fatalf("a refused QEMU installer failed the ensure: %v", err)
	}
	if !st.Healthy {
		t.Fatalf("a base whose container ran read unusable: %+v", st)
	}
	joined := strings.Join(st.Problems, "\n")
	if !strings.Contains(joined, "the QEMU handlers: binfmt.sh exited 3: the QEMU binary-format installer did not complete") {
		t.Fatalf("the QEMU step's failure is not on the answer: %q", joined)
	}
	if !strings.Contains(h.guestScripts(t), "TK_BINFMT_IMAGE='"+BinfmtImage+"'") {
		t.Fatal("the case never reached the QEMU step")
	}
	assertKept(t, h, b, disk)
}

// ⭐ A job's platform registers through binfmt.sh, the one home of root's podman.
func TestAJobsPlatformRegistersThroughBinfmtSh(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	h.stageGuest(t)
	if err := b.EnsurePlatform(context.Background(), "linux/s390x"); err != nil {
		t.Fatalf("a foreign platform was not prepared: %v", err)
	}
	scripts := h.guestScripts(t)
	if !strings.Contains(scripts, string(binfmtScript)) || !strings.Contains(scripts, "TK_ARCH='s390x'; export") {
		t.Fatalf("the platform was not registered through binfmt.sh:\n%s", scripts)
	}
}

// -- WSL-182: the privileged installer is pinned by digest ---------------------

// ⛔ THE INSTALLER RUNS PRIVILEGED FROM A TAG. A tag pushed again would register
// whatever it names in the handler table every distribution shares. Every
// privileged run in this module names the pinned image, and the pin is a digest.
func TestThePrivilegedQEMUInstallerIsPinnedByDigest(t *testing.T) {
	if !regexp.MustCompile(`^docker\.io/tonistiigi/binfmt:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}$`).MatchString(BinfmtImage) {
		t.Fatalf("the QEMU installer is %q, not a tag pinned by its sha256 digest", BinfmtImage)
	}
	if !strings.HasSuffix(BinfmtImage, "@"+binfmtPinnedDigest) {
		t.Fatalf("the QEMU installer %q does not carry binfmtPinnedDigest", BinfmtImage)
	}
	root := filepath.Join("..", "..")
	seen := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if strings.HasSuffix(path, "_test.go") || !(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".sh")) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for n, line := range strings.Split(string(data), "\n") {
			code := strings.TrimSpace(line)
			if !strings.Contains(code, "--privileged") || strings.HasPrefix(code, "#") || strings.HasPrefix(code, "//") {
				continue
			}
			seen++
			if !strings.Contains(code, `"$TK_BINFMT_IMAGE"`) {
				t.Errorf("%s:%d runs privileged from an image that is not the pinned one: %s", path, n+1, code)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Fatal("no privileged run was found, so this case read nothing")
	}
}
