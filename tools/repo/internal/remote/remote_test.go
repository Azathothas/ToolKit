// SPDX-License-Identifier: 0BSD

package remote

import (
	"strings"
	"testing"
)

func TestPinsAdded(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/.github/workflows/ci.yml b/.github/workflows/ci.yml",
		"--- a/.github/workflows/ci.yml",
		"+++ b/.github/workflows/ci.yml",
		"@@ -1,4 +1,4 @@",
		"-      uses: actions/checkout@1111111111111111111111111111111111111111 # v4",
		"+      uses: actions/checkout@2222222222222222222222222222222222222222 # v5",
		"       uses: actions/setup-go@3333333333333333333333333333333333333333 # v5",
		"+      uses: astral-sh/setup-uv@4444444444444444444444444444444444444444",
	}, "\n")

	shaV4, shaV5 := strings.Repeat("1", 40), strings.Repeat("2", 40)
	pins := PinsAdded(diff)
	if len(pins) != 2 {
		t.Fatalf("got %d pin(s), want 2: %+v", len(pins), pins)
	}
	if pins[0].Action != "actions/checkout" || pins[0].Tag != "v5" {
		t.Errorf("first pin is %+v", pins[0])
	}
	if pins[0].SHA != shaV5 {
		t.Errorf("first pin's sha is %q", pins[0].SHA)
	}
	// ⚠ A pin with no trailing comment is still a pin, and its missing label is
	// the finding rather than a reason to skip it.
	if pins[1].Action != "astral-sh/setup-uv" || pins[1].Tag != "" {
		t.Errorf("second pin is %+v", pins[1])
	}

	// ⛔ THE REMOVED LINE IS NOT CHECKED. A pin the diff takes out is not a
	// claim this change is making, and reporting it would raise a defect the
	// change is fixing.
	for _, p := range pins {
		if p.SHA == shaV4 {
			t.Error("a removed pin was checked")
		}
	}
	// And an untouched context line is not an addition either.
	for _, p := range pins {
		if p.Action == "actions/setup-go" {
			t.Error("a context line was read as an addition")
		}
	}
}

func TestPinsAddedIgnoresTheFileHeader(t *testing.T) {
	// `+++ b/path` starts with a plus and is not a content line. A parser that
	// missed that would try to read a pin out of a filename.
	diff := "+++ b/uses: owner/name@0000000000000000000000000000000000000000\n"
	if got := PinsAdded(diff); len(got) != 0 {
		t.Fatalf("the file header was read as content: %+v", got)
	}
}

// TestDeclaredRuntimeReadsAQuotedValue is the case for the defect found by
// running this check against a real third-party pull request.
//
// ⛔ `using: "node24"` is valid YAML and real actions write it that way. The
// shell version's capture kept the quotes, so a quoted "node20" matched no arm,
// fell through to the catch-all, and was reported as "unrecognised; check it"
// instead of the refusal this whole check exists to raise. A deprecated runtime
// evaded the one rule written for it by being spelled the other legal way.
func TestDeclaredRuntimeReadsAQuotedValue(t *testing.T) {
	cases := map[string]string{
		"runs:\n  using: node20\n":       "node20",
		"runs:\n  using: \"node20\"\n":   "node20",
		"runs:\n  using: 'node20'\n":     "node20",
		"runs:\n  using:   node24  \n":   "node24",
		"runs:\n  using: docker\n":       "docker",
		"runs:\n  using: composite\n":    "composite",
		"runs:\r\n  using: \"node24\"\r": "node24",
	}
	for manifest, want := range cases {
		if got := DeclaredRuntime(manifest); got != want {
			t.Errorf("DeclaredRuntime(%q) = %q, want %q", manifest, got, want)
		}
	}
}

// TestDeclaredRuntimeStaysInsideTheRunsBlock stops a `using:` belonging to
// something else being read as the action's runtime.
func TestDeclaredRuntimeStaysInsideTheRunsBlock(t *testing.T) {
	manifest := strings.Join([]string{
		"name: an action",
		"inputs:",
		"  using:",
		"    description: not the runtime",
		"    default: node16",
		"runs:",
		"  using: node24",
		"  main: dist/index.js",
		"branding:",
		"  using: also-not-the-runtime",
	}, "\n")
	if got := DeclaredRuntime(manifest); got != "node24" {
		t.Fatalf("DeclaredRuntime = %q, want node24", got)
	}

	// A manifest with no runs block at all has no runtime to report, and
	// reporting one from elsewhere would be worse than reporting none.
	if got := DeclaredRuntime("name: x\ninputs:\n  using: node16\n"); got != "" {
		t.Errorf("a using outside runs was read as the runtime: %q", got)
	}
	if got := DeclaredRuntime(""); got != "" {
		t.Errorf("an empty manifest returned %q", got)
	}
}

// TestVerdictDoesNotFailOnAnUnreadItem is the rule the shell version got wrong
// in one of its two modes.
func TestVerdictDoesNotFailOnAnUnreadItem(t *testing.T) {
	if got := (Report{NeedsHuman: 9}).Verdict(); got != 0 {
		t.Errorf("nine items needing a reading exited %d. Any repository with an open issue would be permanently red", got)
	}
	if got := (Report{Problems: 1}).Verdict(); got != 1 {
		t.Errorf("a claim that did not hold exited %d, want 1", got)
	}
	if got := (Report{Problems: 1, NeedsHuman: 9}).Verdict(); got != 1 {
		t.Errorf("a failed claim beside unread items exited %d, want 1", got)
	}
	if got := (Report{}).Verdict(); got != 0 {
		t.Errorf("a clean repository exited %d", got)
	}
}

func TestShort(t *testing.T) {
	if got := short(strings.Repeat("2", 40)); got != "222222222222" {
		t.Errorf("short = %q", got)
	}
	if got := short("abc"); got != "abc" {
		t.Errorf("a short value was truncated: %q", got)
	}
}
