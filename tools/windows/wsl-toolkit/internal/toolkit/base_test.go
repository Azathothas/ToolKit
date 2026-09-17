// base_test.go - what a guest command's failure is named by.
//
// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"strings"
	"testing"
)

// TestAGuestFailureIsNamedByTheGuestsOwnLine is findings 12, 13 and 15.
//
// ⛔ EVERY FIXTURE HERE IS A REAL CAPTURE. A failed guest command was named by
// the FIRST line of its output in twenty-four places, and the first line is
// regularly a wrapper's: wsl.exe prints its own before the guest says anything,
// and podman prints a warning before its error.
func TestAGuestFailureIsNamedByTheGuestsOwnLine(t *testing.T) {
	for _, tc := range []struct {
		name    string
		streams []string
		want    string
	}{
		{
			// ⛔ MEASURED: a base built with automount off printed 93 of these
			// between provisioning and its restart. The guest's own error was
			// behind all of them.
			name: "wsl.exe writes its own lines first",
			streams: []string{
				"wsl: Failed to translate " + `D:\proj\x` + "\nwsl: Failed to translate " + `D:\` + "\n" +
					"mount: /mnt/e: permission denied\n",
			},
			want: "mount: /mnt/e: permission denied",
		},
		{
			// ⛔ MEASURED on a fedora base: this needed a diagnostic build to
			// read, because the warning was the first line.
			name: "podman warns before it fails",
			streams: []string{
				"WARN[0000] Using cgroups-v1 which is deprecated\n" +
					"newuidmap: write to uid_map failed: Operation not permitted\n",
			},
			want: "newuidmap: write to uid_map failed: Operation not permitted",
		},
		{
			// A shell prints its context first and its reason last.
			name:    "the last guest line is the reason",
			streams: []string{"+ set -e\n+ podman run alpine\nError: short-name resolution enforced\n"},
			want:    "Error: short-name resolution enforced",
		},
		{
			name:    "progress lines are not a failure",
			streams: []string{"Trying to pull docker.io/library/alpine:latest...\nCopying blob sha256:abc\nError: initializing source: pinging container registry\n"},
			want:    "Error: initializing source: pinging container registry",
		},
		{
			// ⛔ NOTHING BUT NOISE STILL SAYS SOMETHING. An empty message is
			// worse than one naming a wrapper's line.
			name:    "only a wrapper spoke",
			streams: []string{"wsl: Failed to translate " + `D:\y` + "\n"},
			want:    "wsl: Failed to translate " + `D:\y`,
		},
		{
			name:    "two streams, and the guest is in the second",
			streams: []string{"WARN[0000] noise\n", "modprobe: FATAL: Module nft_compat not found\n"},
			want:    "modprobe: FATAL: Module nft_compat not found",
		},
		{
			name:    "nothing at all",
			streams: []string{"", ""},
			want:    "the guest said nothing",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := guestFailure(tc.streams...); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			// ⛔ AND THE OLD BEHAVIOUR IS WHAT THIS REPLACES. Where the two
			// agree the fixture proves nothing, so the ones that matter are
			// asserted to DIFFER from the first line.
			if tc.want != "the guest said nothing" && len(tc.streams) > 0 {
				old := firstLine(tc.streams[0])
				if old == tc.want && strings.Contains(tc.name, "writes its own") {
					t.Fatalf("this fixture does not exercise the defect: the first line is already %q", old)
				}
			}
		})
	}
}

// TestAHostWideRepairNamesTheOtherInstances is finding 67.
//
// ⛔ WHAT IT COST. `wsl --shutdown` invalidates the cached boot id in EVERY
// distribution holding an engine. A session met it on one base, repaired that
// one, recorded exactly that, and left the default base - which every `run` and
// `matrix` uses - carrying the same damage into the next session, where the
// first container check exited 2 before anything could run.
func TestAHostWideRepairNamesTheOtherInstances(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current string
		managed []string
		want    []string
		quiet   bool
	}{
		{
			name: "the shape that cost a session", current: "base",
			managed: []string{"base", "acc", "podbox"},
			want:    []string{"HOST-WIDE", "acc", "podbox", "--instance acc base ensure --repair"},
		},
		{
			// ⛔ The DEFAULT instance has an empty name, and the sibling it
			// forgot was reached without --instance at all. A note that
			// skipped it would miss exactly the case that happened.
			name: "the default instance is one of the siblings", current: "base",
			managed: []string{"base", "muse"},
			want:    []string{"muse"},
		},
		{"only the one that failed", "base", []string{"base"}, nil, true},
		{"no instances at all", "", nil, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := siblingRepairNote(tc.current, tc.managed)
			if tc.quiet {
				if got != "" {
					t.Fatalf("a run with no sibling still warned: %q", got)
				}
				return
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Fatalf("the note does not say %q: %q", w, got)
				}
			}
			// ⛔ IT MUST NOT CLAIM THE OTHERS ARE BROKEN. Nothing probed them,
			// and asserting damage it never measured is the fabricated-number
			// row in forbidden-patterns.md.
			if !strings.Contains(got, "LIKELY") || !strings.Contains(got, "unprobed") {
				t.Fatalf("the note states damage it did not measure: %q", got)
			}
			// And the instance that DID fail is never told to repair itself
			// twice: the remediation's own Command already covers it.
			if strings.Contains(got, "--instance "+tc.current+" ") {
				t.Fatalf("the note repeats the instance that already failed: %q", got)
			}
		})
	}
}

// TestTheStaleRunStateRemediationStillFiresOnTheEnginesOwnWords. The sibling
// note is an addition, and an addition that changed what the remediation
// matches would be a worse defect than the one it fixes.
func TestTheStaleRunStateRemediationStillFiresOnTheEnginesOwnWords(t *testing.T) {
	r, ok := staleRunStateRemediation("Error: current system boot ID differs from cached boot ID; an unhandled reboot has occurred")
	if !ok {
		t.Fatal("the engine's own message no longer matches")
	}
	if r.ID != "stale-run-state" || !r.Repairable || r.Command == "" {
		t.Fatalf("the remediation lost a field: %+v", r)
	}
	if _, ok := staleRunStateRemediation("Error: something else entirely"); ok {
		t.Fatal("an unrelated failure was classified as a stale boot id")
	}
}
