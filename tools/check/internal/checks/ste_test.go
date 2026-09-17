// ste_test.go - the countable half of ASD-STE100, and the two defects the rule
// itself had before it was ever run over the tree.
//
// ⛔ THE FIRST VERSION REPORTED 40 PROBLEMS AND HALF OF THEM WERE ITS OWN. It
// summed every item of an ordered list into one unit, so
// docs/methodology/sessions.md's seven-step start-of-session list read as a
// 16-sentence paragraph. docs/conventions/prose.md says a guard that refuses
// legitimate writing is worse than no guard, and that is what it was doing.
//
// ⛔ AND IT MASKED CODE LINE BY LINE, so an inline span that wraps across a
// newline - `gh release list --repo` does, in README.md - read as prose after
// the break and produced a finding about a command name.
//
// Both have their own case below, because both were found by running it rather
// than by reading it.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// steRun applies the rule to one document's bytes, the way the check does.
func steRun(t *testing.T, body string) Result {
	t.Helper()
	r := Result{Extra: map[string]any{}}
	steFile(&r, "doc.md", []byte(body))
	return r
}

func TestSTECountsWhatItCanAndRefusesWhatItCannot(t *testing.T) {
	long := "This sentence exists only to be too long, and it goes on past the ceiling " +
		"with more words and yet more words until it is clearly over the limit set here."

	cases := []struct {
		name string
		body string
		want int
		says string
	}{
		{
			name: "a sentence inside the ceiling",
			body: "The tool builds one distribution. It owns nothing else.\n",
			want: 0,
		},
		{
			name: "a sentence over 25 words",
			body: long + "\n",
			want: 1,
			says: "STE rule 4.1",
		},
		{
			// ⭐ A STEP IS HELD TIGHTER, which is STE's own split: somebody is
			// doing what the line says while they read it.
			name: "a step of 21 words, which is inside 25 and over 20",
			body: "1. Run the command and then read the answer that it gives to you before you go on to the next thing.\n",
			want: 1,
			says: "this step",
		},
		{
			name: "a paragraph of seven sentences",
			body: "One. Two. Three. Four. Five. Six. Seven.\n",
			want: 1,
			says: "STE rule 6.1",
		},
		{
			// ⛔ THE DEFECT THE RULE HAD. Seven list items are seven units, not
			// one seven-sentence paragraph.
			name: "seven list items are not one paragraph",
			body: "1. One.\n2. Two.\n3. Three.\n4. Four.\n5. Five.\n6. Six.\n7. Seven.\n",
			want: 0,
		},
		{
			name: "a bulleted list is not one paragraph either",
			body: "- One.\n- Two.\n- Three.\n- Four.\n- Five.\n- Six.\n- Seven.\n",
			want: 0,
		},
		{
			name: "a word with an approved replacement",
			body: "The base will terminate the job.\n",
			want: 1,
			says: "STE rule 1.1",
		},
		{
			name: "one concept written two ways",
			body: "The distro is registered.\n",
			want: 1,
			says: "STE rules 1.2 and 1.3",
		},
		{
			// ⭐ STE RULE 1.4: inside backticks it is a Technical Name, and this
			// tool really does have a command called `distro`.
			name: "the same word inside a code span is a technical name",
			body: "Run `wsl-toolkit distro list` to see them.\n",
			want: 0,
		},
		{
			// ⛔ THE OTHER DEFECT. The span opens on one line and closes on the
			// next, and a line-by-line mask read the tail as prose.
			name: "a code span that wraps across a line break",
			body: "Read it with `wsl-toolkit distro list\n--json` and then stop.\n",
			want: 0,
		},
		{
			name: "a fenced block is not prose",
			body: "```sh\nterminate the distro prior to this\n```\n",
			want: 0,
		},
		{
			name: "a table row is not prose",
			body: "| distro | terminate |\n| --- | --- |\n",
			want: 0,
		},
		{
			name: "a heading is not a paragraph",
			body: "# One. Two. Three. Four. Five. Six. Seven.\n",
			want: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := steRun(t, c.body)
			if r.Problems != c.want {
				t.Fatalf("problems = %d, want %d: %s", r.Problems, c.want, strings.Join(r.Detail, " | "))
			}
			if c.says != "" && !strings.Contains(strings.Join(r.Detail, " | "), c.says) {
				t.Fatalf("no finding cited %q: %s", c.says, strings.Join(r.Detail, " | "))
			}
		})
	}
}

// TestTheRecordIsOutsideSTEAndSaysWhy holds the scope.
//
// ⛔ A RULE WITH A SILENT SCOPE IS ONE NOBODY CAN ARGUE WITH, and the set it
// skips grows. Every exemption is a path with a reason, and this asserts both
// that the record is out and that the documents a reader follows are in.
func TestTheRecordIsOutsideSTEAndSaysWhy(t *testing.T) {
	out := []string{"TODO/PROGRESS.md", "CHANGELOG.md", "docs/HISTORY/consumers.md", "docs/reference-sweeps/usable.md"}
	for _, f := range out {
		why := steExemptReason(f)
		if why == "" {
			t.Errorf("%s is inside the rule, and the record is evidence rather than a document a reader follows", f)
		}
		if len(why) < 20 {
			t.Errorf("%s is exempt with the reason %q, which does not say why", f, why)
		}
	}
	in := []string{"docs/AGENTS.md", "README.md", "skills/wsl-toolkit/SKILL.md", "tools/windows/wsl-toolkit/wsl-toolkit.md"}
	for _, f := range in {
		if why := steExemptReason(f); why != "" {
			t.Errorf("%s is exempt as %q, and it is a document a reader follows", f, why)
		}
	}
}

// TestAPowerShellInvocationInADocumentPassesNoProfile holds the last line of
// Azathothas/TEMPLATE issue 16: caution an agent to invoke PowerShell with
// -NoProfile.
//
// ⛔ A PROFILE RUNS BEFORE THE COMMAND AND CAN WRITE TO ITS OUTPUT. Somebody
// else's $PROFILE prints a banner or defines a colliding function, and the
// command a document told a reader to run answers differently on their machine.
//
// ⭐ MEASURED BEFORE THE RULE WAS WRITTEN: not one live document broke it. This
// is a check on a practice that already holds, so it cannot drift.
//
// ⚠ THE FOURTH CASE IS THE ONE THAT NARROWED THE PATTERN. TEMPLATE had to
// narrow its own version after a fixture showed it reporting a comment that
// merely mentioned the shell.
func TestAPowerShellInvocationInADocumentPassesNoProfile(t *testing.T) {
	cases := []struct {
		name string
		line string
		want int
	}{
		{"an invocation with no flag", "pwsh -File x.ps1", 1},
		{"the Windows host with no flag", "powershell -Command x", 1},
		{"the flagged form", "pwsh -NoProfile -File x.ps1", 0},
		{"the Windows host, flagged", "powershell -NoProfile -Command x", 0},
		{"prose that merely names it", "powershell 5.1 answered 131 cases", 0},
		{"a bare name with no flag at all", "run powershell now", 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := "# A page\n\n```bash\n" + c.line + "\n```\n"
			tree := treeWithFile(t, "docs/x.md", body)
			r := Docs(tree)
			got := 0
			for _, d := range r.Detail {
				if strings.Contains(d, "-NoProfile") {
					got++
				}
			}
			if got != c.want {
				t.Fatalf("%d finding(s) about -NoProfile, want %d: %s", got, c.want, strings.Join(r.Detail, " | "))
			}
		})
	}
}

// treeWithFile writes one tracked file and returns a Tree over it.
func treeWithFile(t *testing.T, name, body string) *Tree {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return &Tree{Root: root, Files: []string{name}, cache: map[string][]byte{}}
}
