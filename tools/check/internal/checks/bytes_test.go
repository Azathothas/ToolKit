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

// ⭐ Two skills may state one rule in the same words, because an agent is handed
// one skill and nothing else. The two skills' pipe rule was worded apart only so
// that this check would pass. The same sentence in a skill and any other page is
// still a fact with two homes. DOC-11.
func TestTwoSkillsMayShareASentenceAndNoOtherPageMay(t *testing.T) {
	const rule = "Piped into another program, such as `findstr`, the exit code is that program's code and not the tool's.\n"
	shared := treeOf(t, map[string]string{
		"skills/one/SKILL.md": "# one\n\n" + rule,
		"skills/two/SKILL.md": "# two\n\n" + rule,
	})
	if r := OneHome(shared); r.Problems != 0 {
		t.Fatalf("two skills that share one rule were refused:\n%s", strings.Join(r.Detail, "\n"))
	}
	for name, other := range map[string]string{
		"a page":                "docs/page.md",
		"the skills' own index": "skills/README.md",
	} {
		tree := treeOf(t, map[string]string{
			"skills/one/SKILL.md": "# one\n\n" + rule,
			"skills/two/SKILL.md": "# two\n\n" + rule,
			other:                 "# other\n\n" + rule,
		})
		r := OneHome(tree)
		detail := strings.Join(r.Detail, "\n")
		if r.Problems != 1 || !strings.Contains(detail, other) {
			t.Errorf("%s: want the sentence refused once, naming %s, got %d:\n%s", name, other, r.Problems, detail)
		}
	}
}
