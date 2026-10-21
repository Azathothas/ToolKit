// SPDX-License-Identifier: 0BSD

package checks

import (
	"strings"
	"testing"
)

// ⛔ A carriage return inside a line passed, because every `\r` did: in the muse
// probe, `tr -d` with that byte between its quotes showed every reader an empty
// pair of quotes. TOOL-28.
func TestTheControlByteRuleRefusesACarriageReturnInsideALine(t *testing.T) {
	tree := treeOf(t, map[string]string{
		"crlf.ps1":  "Write-Output 'a'\r\nWrite-Output 'b'\r\n",
		"lf.sh":     "#!/bin/sh\necho a\n",
		"inside.sh": "#!/bin/sh\nx=$(printf 'a' | tr -d '\r' | head -1)\n",
		"last.md":   "# title\r",
	})
	r := ControlBytes(tree)
	detail := strings.Join(r.Detail, "\n")
	if r.Problems != 2 {
		t.Fatalf("want exactly the two lone carriage returns refused, got %d:\n%s", r.Problems, detail)
	}
	for _, want := range []string{"inside.sh:2: a carriage return inside a line", "last.md:1: a carriage return inside a line"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the refusal does not name %q:\n%s", want, detail)
		}
	}
	if strings.Contains(detail, "crlf.ps1") || strings.Contains(detail, "lf.sh") {
		t.Errorf("a CRLF or an LF file was refused:\n%s", detail)
	}
}
