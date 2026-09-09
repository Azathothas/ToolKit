// SPDX-License-Identifier: 0BSD

package deslop

import "testing"

// ⛔ THE ACCEPT LIST IS AS LOAD-BEARING AS THE REFUSE LIST. This tool deletes
// files when asked, and an unanchored match on "agent" would take `src/agents/`
// in a project that happens to build one. Every name below that is NOT matched
// is a deletion this guard prevents.
func TestIsAgentFacing(t *testing.T) {
	matched := []string{
		"AGENTS.md",
		"docs/AGENTS.md",
		"deep/nested/path/AGENTS.md",
		"CLAUDE.md",
		"GEMINI.md",
		".cursorrules",
		"sub/.windsurfrules",
		"ROUTE.md",
		"ADOPT.md",
		"MAINTAIN.md",
		".github/copilot-instructions.md",
		"bootstrap/init.sh",
		"docs/methodology/gate.md",
		"docs/templates/entry.md",
	}
	for _, p := range matched {
		if !isAgentFacing(p) {
			t.Errorf("isAgentFacing(%q) = false, and this tool is meant to inventory it", p)
		}
	}

	spared := []string{
		"src/agents/runner.go",
		"internal/agent.go",
		"AGENTS.md.bak",
		"docs/agents.md",
		"docs/methodologies/other.md",
		"docs/conventions/prose.md",
		"README.md",
		"bootstrapping/notes.md",
		"my-bootstrap/x",
		"ROUTES.md",
		"a/ROUTE.md",
		".github/workflows/ci.yml",
		"cursorrules",
	}
	for _, p := range spared {
		if isAgentFacing(p) {
			t.Errorf("isAgentFacing(%q) = true. --apply would DELETE it, and it is not one of the template's files", p)
		}
	}
}

// ⚠ `ROUTE.md` at the root is a router; `a/ROUTE.md` is somebody's document.
// The distinction is deliberate and the shell implementation made it the same
// way, so it is asserted rather than left to a reader of the switch.
func TestRootOnlyNamesAreNotMatchedDeeper(t *testing.T) {
	for _, p := range []string{"ROUTE.md", "ADOPT.md", "MAINTAIN.md"} {
		if !isAgentFacing(p) {
			t.Errorf("%q at the root should be matched", p)
		}
		if isAgentFacing("sub/" + p) {
			t.Errorf("sub/%s should not be matched: only the root name is the template's", p)
		}
	}
}
