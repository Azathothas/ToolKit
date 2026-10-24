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
		case "brokenpull":
			fmt.Println("automount ro")
			fmt.Fprintln(os.Stderr, "verify-pull: "+insufficientIDs)
			return verifyNotJudged, true
		case "eofpull":
			fmt.Println("automount ro")
			fmt.Fprintln(os.Stderr, "verify-pull: "+unexpectedEOF)
			return verifyNotJudged, true
		}
	case bytes.Contains(script, []byte(`: "${TK_ROOT_ENGINE:?TK_ROOT_ENGINE is required}"`)):
		// ⭐ engines.sh completes unless a case stages its failure. WSL-130.
		if answers["engines"] == "refused" {
			fmt.Fprintln(os.Stderr, "engines: base.kvm keeps /dev/kvm usable through a systemd unit, and this base does not run systemd")
			return 3, true
		}
		fmt.Println("engines-complete")
		return 0, true
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

// insufficientIDs is what rootless podman answers a pull when the account's
// /etc/subuid range is gone: the base's own defect, which provision.sh repairs.
const insufficientIDs = `Error: copying system image from manifest list: writing blob: adding layer with blob "sha256:0123": processing tar file(potentially insufficient UIDs or GIDs available in user namespace (requested 0:42 for /etc/shadow): Check /etc/subuid and /etc/subgid if configured locally and run "podman system migrate": lchown /etc/shadow: invalid argument): exit status 1`

// ⛔ WSL-174, THE REPAIR. Every failed pull read as "registry unreachable", so a
// base that broke its own pull was refused the re-provisioning that repairs it,
// on every retry. A pull the base broke still re-provisions. The engine is down,
// so the provisioning that the fake WSL refuses ends in the kept-base refusal.
func TestAPullTheBaseBrokeStillReProvisions(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, false)
	h.stageGuest(t, "verify=brokenpull")
	_, err := b.EnsureWith(context.Background(), false, false)
	var nj *ProbeNotJudgedError
	if errors.As(err, &nj) || (err != nil && strings.Contains(err.Error(), "not judged")) {
		t.Fatalf("a pull the base broke read as not judged: %v", err)
	}
	if !s.said("the cause is in the base, not the registry") || !s.said("insufficient UIDs or GIDs") {
		t.Fatalf("the probe's pull failure was not named as the base's: %v", s.lines)
	}
	if !s.said("re-provisioning in place") {
		t.Fatalf("a pull the base broke did not re-provision: %v", s.lines)
	}
	assertKept(t, h, b, disk)
}

// unexpectedEOF is a blob fetch the registry's CDN cut off: no mark of the
// network or the registry is in it.
const unexpectedEOF = `Error: copying system image from manifest list: reading blob sha256:0123: Get "https://production.cloudflare.docker.com/registry-v2/docker/registry/v2/blobs/sha256/01/0123/data": EOF`

// ⛔ WSL-174, THE SECOND REPAIR. A reason that named neither the base nor a known
// registry word was judged against the base, so a blob fetch the CDN cut off
// re-provisioned the base, and a provisioning that failed too removed it. An
// unknown reason now refuses and keeps the base.
func TestAnUnknownPullFailureKeepsTheBase(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	h.stageGuest(t, "verify=eofpull")
	_, err := b.EnsureWith(context.Background(), false, false)
	var nj *ProbeNotJudgedError
	if !errors.As(err, &nj) || !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("a pull the CDN cut off answered %v; want a ProbeNotJudgedError carrying its reason", err)
	}
	if s.said("re-provisioning") {
		t.Fatalf("an unknown pull failure led to re-provisioning: %v", s.lines)
	}
	if i := callAt(h.calls(t), "wsl --terminate"); i >= 0 {
		t.Fatalf("an unknown pull failure restarted the base: %v", h.calls(t))
	}
	assertKept(t, h, b, disk)
}

// ⛔ A REFUSAL OVER TRUST NAMES WHERE TO LOOK IN THE BASE, and no removal. A
// broken CA bundle or a stale auth.json is the base's, and waiting on the
// registry never ends. WSL-174.
func TestATrustRefusalNamesWhereToLookInTheBase(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	for _, reason := range []string{
		`Error: pinging container registry registry-1.docker.io: Get "https://registry-1.docker.io/v2/": tls: failed to verify certificate: x509: certificate signed by unknown authority`,
		"Error: reading manifest latest in docker.io/library/alpine: unauthorized: authentication required",
	} {
		msg := b.notJudgedRefusal(&ProbeNotJudgedError{Image: VerifyImage, Registry: "docker.io", Reason: reason}).Error()
		for _, want := range []string{"/etc/ssl/certs", "containers/auth.json", ToolCommand("base shell --root"), "Neither needs a rebuild"} {
			if !strings.Contains(msg, want) {
				t.Errorf("the refusal over %q does not say %q: %s", reason, want, msg)
			}
		}
		if strings.Contains(msg, "recreate") || strings.Contains(msg, "--force") {
			t.Errorf("the refusal over %q names a removal: %s", reason, msg)
		}
	}
	plain := b.notJudgedRefusal(&ProbeNotJudgedError{Image: VerifyImage, Registry: "docker.io", Reason: "no such host"}).Error()
	if strings.Contains(plain, "auth.json") {
		t.Errorf("a refusal over the name sends the operator to the base's files: %s", plain)
	}
}

// ⛔ The cause decides. The base's own causes are an ordinary verify failure that
// names the pull; every other reason is not judged. WSL-174.
func TestAPullFailureIsJudgedByItsCause(t *testing.T) {
	registry := []string{
		unexpectedEOF,
		"Error: copying system image from manifest list: reading blob sha256:0123: unexpected EOF",
		`Error: copying system image from manifest list: reading blob sha256:0123: Get "https://production.cloudflare.docker.com/blob": http2: server sent GOAWAY and closed the connection`,
		"Error: copying system image from manifest list: reading blob sha256:0123: context deadline exceeded",
		"Error: copying system image from manifest list: reading blob sha256:0123: context canceled",
		"Error: reading blob sha256:0123: stream error: stream ID 3; INTERNAL_ERROR",
		"Error: reading blob sha256:0123: read tcp 10.0.0.2:41234->104.16.1.1:443: read: connection timed out",
		`Error: Get "https://registry-1.docker.io/v2/": dial tcp 1.2.3.4:443: connect: permission denied`,
		"podman pull exited 143",
		`Error: initializing source docker://alpine:latest: pinging container registry registry-1.docker.io: Get "https://registry-1.docker.io/v2/": dial tcp: lookup registry-1.docker.io: no such host`,
		"the pull did not finish within 120 seconds",
		"Error: initializing source docker://alpine:latest: reading manifest latest in docker.io/library/alpine: toomanyrequests: You have reached your unauthenticated pull rate limit",
		"Error: copying system image from manifest list: reading blob sha256:0123: fetching blob: received unexpected HTTP status: 503 Service Unavailable",
		`Error: initializing source docker://alpine:latest: pinging container registry registry-1.docker.io: Get "https://registry-1.docker.io/v2/": tls: failed to verify certificate: x509: certificate signed by unknown authority`,
		"Error: reading manifest latest in docker.io/library/alpine: unauthorized: authentication required",
		`Error: Get "https://registry-1.docker.io/v2/": dial tcp 1.2.3.4:443: i/o timeout`,
	}
	local := []string{
		insufficientIDs,
		`Error: copying system image from manifest list: writing blob: storing blob to file "/var/tmp/storage123/1": write /var/tmp/storage123/1: no space left on device`,
		"Error: open /etc/containers/policy.json: no such file or directory",
		`Error: creating overlay mount to /home/toolkit/.local/share/containers/storage/overlay/0123/merged: permission denied`,
		`Error: writing blob: mkdir /home/toolkit/.local/share/containers/storage/overlay/0123: read-only file system`,
	}
	for _, reason := range registry {
		stderr := "verify-pull: " + reason + "\n"
		if _, ok := probeNotJudged(VerifyImage, verifyNotJudged, stderr); !ok {
			t.Errorf("the registry's failure was judged against the base: %s", reason)
		}
	}
	for _, reason := range local {
		stderr := "verify-pull: " + reason + "\n"
		if nj, ok := probeNotJudged(VerifyImage, verifyNotJudged, stderr); ok {
			t.Errorf("the base's own pull failure read as not judged: %v", nj)
		}
		err, ok := probePullFailure(VerifyImage, verifyNotJudged, stderr)
		if !ok || !strings.Contains(err.Error(), reason) || !strings.Contains(err.Error(), "the cause is in the base") {
			t.Errorf("the base's own pull failure %q answered %v, %v", reason, err, ok)
		}
	}
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
	// ⚠ BusyBox `timeout` signals podman in its own place, so a bound that fires
	// there ends in 143, or 137 for a KILL, and never in GNU's 124. WSL-174.
	const busyboxBound = `echo 'Error: copying system image: context canceled' >&2; exit 143`
	cases := []struct {
		name, exists, pull, timeout string
		exit                        int
		reason                      string
		pulled                      bool
		clock                       bool
	}{
		{"present: no pull", "exit 0", "exit 0", "", 0, "", false, false},
		{"absent and pulled", "exit 1", "exit 0", "", 0, "", true, false},
		{"absent and the registry refuses, the measured defect", "exit 1", refused, "", verifyNotJudged, "lookup registry-1.docker.io: no such host", true, false},
		{"absent and the pull outlives its bound", "exit 1", "exit 0", "exit 124", verifyNotJudged, "the pull did not finish within 120 seconds", false, false},
		{"absent and BusyBox's bound fires: 143", "exit 1", "exit 0", busyboxBound, verifyNotJudged, "the pull did not finish within 120 seconds", false, false},
		{"absent and BusyBox's bound fires: 137", "exit 1", "exit 0", "exit 137", verifyNotJudged, "the pull did not finish within 120 seconds", false, false},
		{"absent and podman answers the signal with its own code after the whole bound", "exit 1", "echo 'Error: context canceled' >&2; exit 125", "", verifyNotJudged, "the pull did not finish within 120 seconds", true, true},
		{"absent and refused at once: the clock is not the bound", "exit 1", refused, "", verifyNotJudged, "no such host", true, false},
		{"the engine fails: left to the run", "echo 'Error: current system boot ID differs from cached boot ID' >&2; exit 125", refused, "", 0, "", false, false},
	}
	for _, c := range cases {
		dir := t.TempDir()
		log := filepath.Join(dir, "calls")
		// ⚠ The clock moves 121 seconds a call where the case says the pull took
		// the whole bound, and 1 second otherwise.
		step := "1"
		if c.clock {
			step = "121"
		}
		writeStandIn(t, dir, "date", `n=$(cat "$TK_CLOCK" 2>/dev/null) || n=1000; echo "$n"; echo $((n + `+step+`)) > "$TK_CLOCK"`)
		for _, tool := range []string{"cat"} {
			if p, err := exec.LookPath(tool); err == nil {
				writeStandIn(t, dir, tool, "exec "+p+" \"$@\"")
			}
		}
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
		cmd.Env = []string{"PATH=" + dir, "TK_IMAGE=" + image, "TK_LOG=" + log, "TK_CLOCK=" + filepath.Join(dir, "clock")}
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
// 2 over a base whose engine had just run a container. ⛔ AND THE FIRST REPAIR
// MADE IT A PROBLEM, which still exited 1 and read not ready. It is an advisory
// finding now, the base reads usable, and the answer has no problem.
func TestAFailedQEMUStepIsAnAdvisoryAndNotAFailedEnsure(t *testing.T) {
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
	if len(st.Problems) > 0 {
		t.Fatalf("a refused QEMU installer is a problem, which exits 1 and reads not ready: %q", st.Problems)
	}
	var step *Remediation
	for i := range st.Remediations {
		if st.Remediations[i].ID == "binfmt-step-failed" {
			step = &st.Remediations[i]
		}
	}
	if step == nil || !step.Advisory || !strings.HasSuffix(step.Command, "base ensure") ||
		!strings.Contains(step.What, "the QEMU handlers: binfmt.sh exited 3: the QEMU binary-format installer did not complete") {
		t.Fatalf("the QEMU step's failure is not an advisory finding on the answer: %+v", st.Remediations)
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

// shBody is the body of one shell function in a script, from its opening line
// to the closing brace at the start of a line.
func shBody(t *testing.T, script, name string) string {
	t.Helper()
	begin := strings.Index(script, "\n"+name+"() {\n")
	if begin < 0 {
		t.Fatalf("the script has no function %s", name)
	}
	body := script[begin+1:]
	return body[:strings.Index(body, "\n}\n")]
}

// ⛔ WSL-182, THE REPAIR. The installer's pull ran inside its privileged run,
// bounded only by the ensure's 20 minutes, and the digest pin sent every
// upgraded base back through it. The pull is its own bounded step, only for an
// absent image, and no run pulls.
func TestTheQEMUInstallerIsPulledBoundedAndOnlyWhenAbsent(t *testing.T) {
	script := string(binfmtScript)
	fetch := shBody(t, script, "fetch")
	for _, want := range []string{
		`root_podman image exists "$TK_BINFMT_IMAGE" >/dev/null 2>&1 && return 0`,
		`timeout 120 podman --cgroup-manager=cgroupfs --events-backend=file pull -q "$TK_BINFMT_IMAGE"`,
	} {
		if !strings.Contains(fetch, want) {
			t.Errorf("fetch does not carry %q:\n%s", want, fetch)
		}
	}
	register := shBody(t, script, "register")
	if i, j := strings.Index(register, "fetch || return"), strings.Index(register, "--pull=never"); i < 0 || j < i {
		t.Errorf("register runs the installer without fetching it first:\n%s", register)
	}
	// ⚠ EACH LINE OF CODE, NOT podmanCalls: those skip `root_podman`, which is
	// the name every run here goes through.
	runs := 0
	for _, line := range strings.Split(script, "\n") {
		code := strings.TrimSpace(line)
		if strings.HasPrefix(code, "#") || !strings.Contains(code, "podman run ") {
			continue
		}
		runs++
		if !strings.Contains(code, "--pull=never") {
			t.Errorf("binfmt.sh runs the installer with a pull of its own: %s", code)
		}
	}
	if runs == 0 {
		t.Error("binfmt.sh runs no installer, so this case read nothing")
	}
}

// TestAJobsPlatformPullIsBounded runs binfmt.sh's one-platform path through a
// real POSIX shell, with podman and timeout stood in for. WSL-182.
func TestAJobsPlatformPullIsBounded(t *testing.T) {
	sh := posixShell(t)
	const image = "docker.io/tonistiigi/binfmt:test@sha256:0123"
	const global = "--cgroup-manager=cgroupfs --events-backend=file "
	cases := []struct {
		name, exists, pull, timeout string
		pulled, ran                 bool
		said                        string
		slow                        bool
	}{
		{"present: no pull", "exit 0", "exit 0", "", false, true, "was not registered", false},
		{"absent and pulled", "exit 1", "exit 0", "", true, true, "was not registered", false},
		{"absent and the pull outlives its bound", "exit 1", "exit 0", "exit 124", false, false, "did not finish within 120 seconds", false},
		{"absent and BusyBox's bound fires: 143", "exit 1", "exit 0", "echo 'Error: context canceled' >&2; exit 143", false, false, "did not finish within 120 seconds", false},
		{"absent and the registry refuses", "exit 1", "echo 'Error: toomanyrequests: rate limit' >&2; exit 125", "", true, false, "toomanyrequests", false},
		{"absent and podman answers the signal with its own code after the whole bound", "exit 1", "echo 'Error: context canceled' >&2; exit 125", "", true, false, "did not finish within 120 seconds", true},
	}
	for _, c := range cases {
		dir := t.TempDir()
		log := filepath.Join(dir, "calls")
		writeStandIn(t, dir, "podman", logStandIn+"\ncase \"$*\" in\n  *' image exists '*) "+c.exists+" ;;\n  *' pull -q '*) "+c.pull+" ;;\nesac\nexit 0")
		// ⚠ The clock moves 121 seconds a call where the pull took the whole bound.
		step := "1"
		if c.slow {
			step = "121"
		}
		writeStandIn(t, dir, "date", `n=$(cat "$TK_CLOCK" 2>/dev/null) || n=1000; echo "$n"; echo $((n + `+step+`)) > "$TK_CLOCK"`)
		for _, tool := range []string{"sed", "tail", "cat"} {
			if p, err := exec.LookPath(tool); err == nil {
				writeStandIn(t, dir, tool, "exec "+p+" \"$@\"")
			}
		}
		timeout := `shift; exec "$@"`
		if c.timeout != "" {
			timeout = c.timeout
		}
		writeStandIn(t, dir, "timeout", logStandIn+"\n"+timeout)
		cmd := exec.Command(sh, "-c", string(binfmtScript))
		cmd.Env = []string{"PATH=" + dir, "TK_LOG=" + log, "TK_BINFMT_IMAGE=" + image, "TK_CLOCK=" + filepath.Join(dir, "clock"),
			"TK_ARCH=s390x", "TK_HANDLER=qemu-wsl-toolkit-absent"}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		_ = cmd.Run()
		calls, _ := os.ReadFile(log)
		pulled, bounded, ran, unbounded := false, false, false, false
		for _, line := range strings.Split(string(calls), "\n") {
			pulled = pulled || line == "podman "+global+"pull -q "+image
			bounded = bounded || line == "timeout 120 podman "+global+"pull -q "+image
			ran = ran || strings.HasPrefix(line, "podman "+global+"run ")
			unbounded = unbounded || (strings.Contains(line, " run ") && !strings.Contains(line, "--pull=never"))
		}
		if pulled != c.pulled || (pulled && !bounded) {
			t.Errorf("%s: pulled %v bounded %v, want pulled %v and bounded; calls %q", c.name, pulled, bounded, c.pulled, calls)
		}
		if ran != c.ran || unbounded {
			t.Errorf("%s: ran %v (a run with its own pull: %v), want ran %v; calls %q", c.name, ran, unbounded, c.ran, calls)
		}
		if !strings.Contains(stderr.String(), c.said) {
			t.Errorf("%s: stderr %q does not say %q", c.name, stderr.String(), c.said)
		}
	}
}

// -- WSL-181: the watcher's state is root's measure ----------------------------

// ⛔ THE VERIFIER RUNS AS THE ACCOUNT, and on a systemd base with no D-Bus its
// systemctl reached no manager, so a watcher root started read `none`. binfmt.sh
// records the state root measured, in its own file, and the verifier reads it.
func TestTheWatcherStateIsRecordedByRoot(t *testing.T) {
	const record = "/var/lib/wsl-toolkit/binfmt-watch"
	script := string(binfmtScript)
	for _, want := range []string{
		"WATCH_RECORD=" + record + "\n",
		"watch_state=$(systemctl is-active \"$UNIT\" 2>/dev/null) || :\nnote_watch \"${watch_state:-unknown}\"\n",
		"  note_watch none\n",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("binfmt.sh does not carry %q", want)
		}
	}
	if note := shBody(t, script, "note_watch"); !strings.Contains(note, `mv "$watch_tmp" "$WATCH_RECORD"`) {
		t.Errorf("note_watch does not write its record whole:\n%s", note)
	}
	if !strings.Contains(watcherStateSection(t), record) {
		t.Error("the verifier does not read the record binfmt.sh writes")
	}
}

// watcherStateSection is the verifier's own watcher state section.
func watcherStateSection(t *testing.T) string {
	t.Helper()
	script := string(verifyScript)
	begin := strings.Index(script, "# >>> the watcher state: begin\n")
	end := strings.Index(script, "# <<< the watcher state: end\n")
	if begin < 0 || end < begin {
		t.Fatalf("the verifier's watcher state section is not marked: begin at %d, end at %d", begin, end)
	}
	return script[begin:end]
}

// TestTheVerifierReadsRootsWatcherStateWhenSystemdIsSilent runs the verifier's
// watcher state section through a real POSIX shell. WSL-181.
func TestTheVerifierReadsRootsWatcherStateWhenSystemdIsSilent(t *testing.T) {
	sh := posixShell(t)
	cases := []struct {
		name, systemctl, record, want string
	}{
		{"no D-Bus, root recorded active: the measured defect", "echo 'Failed to connect to bus: No such file or directory' >&2; exit 1", "active", "active"},
		{"no D-Bus and no record", "exit 1", "", "none"},
		{"systemd answers: its answer wins", "echo inactive; exit 3", "active", "inactive"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		record := filepath.Join(dir, "binfmt-watch")
		if c.record != "" {
			if err := os.WriteFile(record, []byte(c.record+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		writeStandIn(t, dir, "systemctl", c.systemctl)
		if p, err := exec.LookPath("head"); err == nil {
			writeStandIn(t, dir, "head", "exec "+p+" \"$@\"")
		}
		section := strings.ReplaceAll(watcherStateSection(t), "/var/lib/wsl-toolkit/binfmt-watch", record)
		cmd := exec.Command(sh, "-c", "set -eu\n"+section)
		cmd.Env = []string{"PATH=" + dir}
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := strings.TrimSpace(string(out)); got != "binfmt-watch "+c.want {
			t.Errorf("%s: the verifier said %q, want %q", c.name, got, "binfmt-watch "+c.want)
		}
	}
}
