// SPDX-License-Identifier: 0BSD

package toolkit

import (
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
