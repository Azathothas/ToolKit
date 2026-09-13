// SPDX-License-Identifier: 0BSD

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/check/internal/checks"
)

// TestAGateSkipIsReportedAsASkip is the case for two outputs that could not tell
// a check that ran from one that could not. A host with no shellcheck and no Go
// toolchain printed `ok shellcheck` and `ok go`, and the JSON said 0 for both.
// TOOL-24.
func TestAGateSkipIsReportedAsASkip(t *testing.T) {
	rows := []gateRow{
		{"docs", checks.Result{}},
		{"shellcheck", checks.Result{Extra: map[string]any{"skipped": "shellcheck is not on PATH"}}},
	}

	var text bytes.Buffer
	if code := renderGate(&text, rows, false); code != 0 {
		t.Fatalf("a skip made the gate exit %d; a host that cannot run a check is not a defect in the tree", code)
	}
	out := text.String()
	if strings.Contains(out, "ok     shellcheck") {
		t.Fatalf("a skipped check was printed as a pass:\n%s", out)
	}
	if !strings.Contains(out, "skip   shellcheck") || !strings.Contains(out, "shellcheck is not on PATH") {
		t.Fatalf("the skip and its reason are not in the text verdict:\n%s", out)
	}
	if !strings.Contains(out, "on 1 of 2 checks") {
		t.Fatalf("the verdict does not say how much of the gate ran:\n%s", out)
	}

	var raw bytes.Buffer
	if code := renderGate(&raw, rows, true); code != 0 {
		t.Fatalf("a skip made the JSON gate exit %d", code)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw.Bytes(), &doc); err != nil {
		t.Fatalf("the JSON verdict does not parse: %v\n%s", err, raw.String())
	}
	skipped, _ := doc["skipped"].(map[string]any)
	if skipped["shellcheck"] != "shellcheck is not on PATH" {
		t.Fatalf("the JSON verdict does not name the skipped check: %s", raw.String())
	}
	if _, listed := skipped["docs"]; listed {
		t.Fatalf("a check that ran was listed as skipped: %s", raw.String())
	}
}

// TestASingleCheckSkipIsReportedAsASkip is the same rule for `check NAME`, which
// prints through its own function and printed `ok` over a skip the same way.
func TestASingleCheckSkipIsReportedAsASkip(t *testing.T) {
	var out bytes.Buffer
	report(&out, "go", checks.Result{Extra: map[string]any{"skipped": "no Go toolchain on PATH"}})
	if got := out.String(); strings.Contains(got, "  ok  ") || !strings.Contains(got, "skip   go: no Go toolchain on PATH") {
		t.Fatalf("a skipped single check printed %q", got)
	}
}
