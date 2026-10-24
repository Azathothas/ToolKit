// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"regexp"
	"strings"
	"testing"
)

// -- WSL-105, issue 37 item 1: the base's own handlers, not the kernel's total -

// ⛔ `ready` was `handlers > 0`. With the Podman machine running, 23 of its own
// handlers kept a base whose every handler was gone reading ready.
func TestReadinessIsTheBasesOwnSetAndNotTheKernelsTotal(t *testing.T) {
	gone := parseBinfmt("binfmt-handlers 23\nbinfmt-own 8\nbinfmt-missing qemu-aarch64 qemu-arm\nbinfmt-watch active\n")
	if gone == nil || gone.Ready || !gone.Recorded || gone.Own != 8 || len(gone.Missing) != 2 || gone.Watch != "active" {
		t.Fatalf("a base missing two of its own handlers beside 23 of another's read as %+v", gone)
	}
	held := parseBinfmt("binfmt-handlers 31\nbinfmt-own 8\nbinfmt-missing\nbinfmt-watch active\n")
	if held == nil || !held.Ready || held.Handlers != 31 || len(held.Missing) != 0 {
		t.Fatalf("a base holding all of its own handlers read as %+v", held)
	}
	emptied := parseBinfmt("binfmt-handlers 0\nbinfmt-own 8\nbinfmt-missing qemu-aarch64 qemu-arm qemu-loongarch64 qemu-mips64 qemu-mips64el qemu-ppc64le qemu-riscv64 qemu-s390x\nbinfmt-watch none\n")
	if emptied == nil || emptied.Ready || len(emptied.Missing) != 8 {
		t.Fatalf("an emptied table read as %+v", emptied)
	}
	unrecorded := parseBinfmt("binfmt-handlers 9\nbinfmt-own unrecorded\nbinfmt-watch none\n")
	if unrecorded == nil || unrecorded.Recorded || !unrecorded.Ready {
		t.Fatalf("a base no ensure has recorded read as %+v; with no record, readiness can only be the kernel's count", unrecorded)
	}
}

func TestAGoneHandlerIsARemediationNamingBaseEnsure(t *testing.T) {
	r, ok := binfmtRemediation(parseBinfmt("binfmt-handlers 0\nbinfmt-own 8\nbinfmt-missing qemu-aarch64\n"))
	if !ok || r.ID != "binfmt-handlers-gone" || !strings.HasSuffix(r.Command, "base ensure") || !strings.Contains(r.What, "qemu-aarch64") || !r.Advisory {
		t.Fatalf("a gone handler earned %+v, %v", r, ok)
	}
	r, ok = binfmtRemediation(parseBinfmt("binfmt-handlers 9\nbinfmt-own unrecorded\n"))
	if !ok || r.ID != "binfmt-handlers-unrecorded" || !r.Advisory {
		t.Fatalf("an unrecorded base earned %+v, %v", r, ok)
	}
	if _, ok := binfmtRemediation(parseBinfmt("binfmt-handlers 8\nbinfmt-own 8\nbinfmt-missing\n")); ok {
		t.Fatal("a base holding every handler of its own earned a remediation")
	}
}

// TestTheHandlersAreRegisteredByEveryEnsureAndWatched holds the shape of the
// rule: one script registers and records, the provisioner no longer registers
// once, the verifier reads the record, and the watcher registers again what is
// gone.
func TestTheHandlersAreRegisteredByEveryEnsureAndWatched(t *testing.T) {
	script := string(binfmtScript)
	for _, want := range []string{
		`for f in "$mnt"/usr/bin/qemu-*; do`,                     // the set is what the image carries
		`[ -e "$DIR/$name" ] && printf '%s\n' "$name" >> "$tmp"`, // that the kernel holds
		"--install all",
		"ExecStart=$SELF watch",
		"if [ \"${1:-}\" = watch ]; then",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("binfmt.sh does not carry %q", want)
		}
	}
	watch := script[strings.Index(script, "if [ \"${1:-}\" = watch ]; then"):]
	watch = watch[:strings.Index(watch, "\nfi\n")]
	if !strings.Contains(watch, "gone=$(missing") || !strings.Contains(watch, "if register &&") || !strings.Contains(watch, "sleep 30") {
		t.Errorf("the watcher does not register again what is gone, every 30 seconds:\n%s", watch)
	}
	if strings.Contains(string(provisionScript), "--install all") {
		t.Error("the provisioner still registers the handlers once, which binfmt.sh now does in every ensure")
	}
	if !strings.Contains(string(verifyScript), "/var/lib/wsl-toolkit/binfmt-handlers") {
		t.Error("the verifier does not read the base's own set")
	}
}

// -- WSL-168: root's podman in a base needs no D-Bus ---------------------------

// ⛔ ROOT'S PODMAN PICKED THE SYSTEMD CGROUP MANAGER on a Debian base with systemd
// and no D-Bus, crun answered `cannot open sd-bus`, and every `base ensure` exited
// 2 at the QEMU handlers. Every podman this tool runs as root in a base takes
// cgroupfs and the file events backend as global options, before its subcommand.
func TestRootsPodmanInABaseNeedsNoDBus(t *testing.T) {
	const global = "podman --cgroup-manager=cgroupfs --events-backend=file "
	// ⭐ ONE SCRIPT RUNS ROOT'S PODMAN. EnsurePlatform runs binfmt.sh with TK_ARCH,
	// so a job's platform and the ensure share one root podman. WSL-181.
	calls := podmanCalls(string(binfmtScript))
	if len(calls) == 0 {
		t.Error("binfmt.sh runs no podman, so this case read nothing")
	}
	for _, call := range calls {
		if !strings.HasPrefix(call, global) {
			t.Errorf("binfmt.sh runs root's podman without cgroupfs and the file events backend as global options: %s", call)
		}
	}
	// ⚠ binfmt.sh's podman is ONE function, so the loop above reads its body once.
	// Each command of the script that reaches the engine goes through it.
	for _, want := range []string{
		`root_podman run --rm --privileged --pull=never "$TK_BINFMT_IMAGE" --install "$TK_ARCH"`,
		`root_podman run --rm --privileged --pull=never "$TK_BINFMT_IMAGE" --install all`,
		`root_podman image exists "$TK_BINFMT_IMAGE"`,
		`mnt=$(root_podman image mount "$TK_BINFMT_IMAGE")`,
		`root_podman image unmount "$TK_BINFMT_IMAGE"`,
	} {
		if !strings.Contains(string(binfmtScript), want) {
			t.Errorf("binfmt.sh does not carry %q", want)
		}
	}
}

// podmanWord is the word podman at a command's start: `root_podman` holds the
// word and is a name, not a call of the engine.
var podmanWord = regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])(podman)[ \t]`)

// podmanCalls is each podman command in a script's code, from the word podman to
// the end of its line. A comment line is not code.
func podmanCalls(script string) []string {
	var calls []string
	for _, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		for _, m := range podmanWord.FindAllStringSubmatchIndex(line, -1) {
			calls = append(calls, line[m[2]:])
		}
	}
	return calls
}
