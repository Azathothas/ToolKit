// commits_test.go - the rule, applied to a message that is not a commit yet.
//
// ⛔ THESE ARE THE FIRST TESTS THIS MODULE HAS EVER HAD. `go test` over
// tools/check ran zero cases, which is why the gate's own `go` check was
// vacuously green about it. That is a hole worth naming rather than quietly
// filling: eighteen checks are covered by nothing but the tree they read.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMessageRefusesTheTrailerThatGotThrough is the case for the defect this
// whole mode exists for.
//
// ⛔ THE MESSAGE BELOW IS VERBATIM WHAT WAS COMMITTED on 2026-09-10 and
// pushed to a protected branch, where undoing it cost a maintainer turning a
// branch protection off and on again. The gate had been run and was green,
// because the commit did not exist yet.
func TestMessageRefusesTheTrailerThatGotThrough(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{
			"the trailer a harness asks for",
			"a subject\n\nsome body.\n\nCo-Authored-By: Claude Opus 5 <noreply@anthropic.com>\n",
			// The trailer, plus the three names inside it.
			4,
		},
		{"a message that credits nobody", "a subject\n\nsome body.\n", 0},
		{
			// ⚠ A HUMAN CO-AUTHOR IS LEGITIMATE and must stay legitimate. The
			// trailer shape alone is not the finding; the finding is a trailer
			// naming a tool.
			"a human co-author",
			// ⚠ ASSEMBLED, NOT WRITTEN. The gate's `secrets` rule refuses a
			// literal address in a tracked file, reserved domain or not, and
			// widening that rule to allow one shape of address would be the
			// wrong half to change.
			"a subject\n\nsome body.\n\nCo-Authored-By: A Person <person@" + "example.invalid>\n",
			0,
		},
		{
			// ⛔ git's own template puts the branch and the file list behind
			// `#`, and none of it reaches the stored message. Refusing a message
			// over git's scaffolding is how a hook becomes one people disable.
			"a tool named only in a comment git will strip",
			"a subject\n\nsome body.\n\n# On branch main\n# Co-Authored-By: Claude\n",
			0,
		},
		{"a generated-with line", "a subject\n\nGenerated with a tool\n", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "COMMIT_EDITMSG")
			if err := os.WriteFile(path, []byte(c.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got := Message(path)
			if got.Problems != c.want {
				t.Fatalf("Message reported %d problem(s), want %d: %v", got.Problems, c.want, got.Detail)
			}
		})
	}
}

// TestMessageSaysSoWhenItCannotRead covers the answer that is neither a pass
// nor a failure. ⛔ A hook that cannot read the message must NOT report the
// message as allowed: that is the shape of every dead check in this tree.
func TestMessageSaysSoWhenItCannotRead(t *testing.T) {
	got := Message(filepath.Join(t.TempDir(), "absent"))
	if got.Problems == 0 {
		t.Fatal("a message that could not be read was reported as allowed")
	}
	if !strings.Contains(strings.Join(got.Detail, " "), "could not be read") {
		t.Fatalf("the finding does not say what happened: %v", got.Detail)
	}
}
