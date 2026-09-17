// skills_test.go - a skill stands alone, and never names a command that does
// not exist.
//
// ⛔ THE SECOND RULE IS THE ONE A WEAK AGENT DEPENDS ON. A capable agent reads a
// refusal and recovers. A weak one runs what the page said, gets exit 2, and
// reports the task as impossible - so a skill naming a command the binary does
// not have is worse than a skill with no example at all.
//
// ⚠ BOTH RULES WERE DRIVEN AGAINST THE REAL FILE before these cases existed: a
// `wsl-toolkit teleport --now` planted in a fenced block and a relative link out
// of skills/ each produced their finding, and the file restored byte-identical.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"strings"
	"testing"
)

// steManual is the shape this check reads out of the generated manual: a
// top-level command is a .SS line and a form is a .B line.
const steManual = ".SH COMMAND REFERENCE\n.SS base\n.TP\n.B wsl-toolkit base status\n.SS man\n.TP\n.B wsl-toolkit man\n"

func skillRun(t *testing.T, path, body string) Result {
	t.Helper()
	r := Result{Extra: map[string]any{}}
	skillFile(&r, path, []byte(body), steManual, nil)
	return r
}

const skillHead = "---\nname: wsl-toolkit\ndescription: \"A description long enough to be a real one, which a harness reads to decide.\"\n---\n\n# A skill\n\n"

func TestASkillNamesOnlyCommandsThatExist(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
		says string
	}{
		{
			name: "a command and a form that both exist",
			body: skillHead + "```powershell\nwsl-toolkit base status --probe\n```\n",
			want: 0,
		},
		{
			name: "a global flag before the command is skipped",
			body: skillHead + "```powershell\nwsl-toolkit --instance base base status\n```\n",
			want: 0,
		},
		{
			// ⛔ THE PLANT THAT WAS DRIVEN AGAINST THE REAL FILE.
			name: "a command that does not exist",
			body: skillHead + "```powershell\nwsl-toolkit teleport --now\n```\n",
			want: 1,
			says: "does not document",
		},
		{
			name: "a subcommand that does not exist",
			body: skillHead + "```powershell\nwsl-toolkit base evaporate\n```\n",
			want: 1,
			says: "base evaporate",
		},
		{
			// ⚠ PROSE NAMES NO COMMAND. "across the wsl-toolkit bridge" is a
			// sentence, and a scan that read it took `bridge` for a subcommand.
			name: "the same words in prose are not a command",
			body: skillHead + "It works across the wsl-toolkit bridge, for any wsl-toolkit instance.\n",
			want: 0,
		},
		{
			name: "a link out of the skill directory",
			body: skillHead + "See [the manual](../../tools/windows/wsl-toolkit/wsl-toolkit.md).\n",
			want: 1,
			says: "outside skills/",
		},
		{
			name: "an anchor and a URL are not links out",
			body: skillHead + "See [below](#later) and [herdr](https://example.invalid/x).\n",
			want: 0,
		},
		{
			name: "front matter whose name disagrees with the directory",
			body: "---\nname: something-else\ndescription: \"A description long enough to be a real one, which a harness reads.\"\n---\n\n# A skill\n",
			want: 1,
			says: "the directory is",
		},
		{
			name: "a description too short to route on",
			body: "---\nname: wsl-toolkit\ndescription: \"too short\"\n---\n\n# A skill\n",
			want: 1,
			says: "decides whether to load",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := skillRun(t, "skills/wsl-toolkit/SKILL.md", c.body)
			if r.Problems != c.want {
				t.Fatalf("problems = %d, want %d: %s", r.Problems, c.want, strings.Join(r.Detail, " | "))
			}
			if c.says != "" && !strings.Contains(strings.Join(r.Detail, " | "), c.says) {
				t.Fatalf("no finding said %q: %s", c.says, strings.Join(r.Detail, " | "))
			}
		})
	}
}

// TestASkillNamesOnlyTextToolFlagsThatExist closes the same loop for the other
// product.
//
// ⛔ text-tool HAS NO GENERATED MANUAL, so its own usage text is the authority:
// it is the string the program prints, so it cannot describe a different build.
// The rule caught a real gap on its first run - the skill told a reader to run
// `text-tool --help`, which the program accepts and its usage text never
// mentioned - and the fix was to the usage text, not to the page.
//
// ⚠ THE VOCABULARY IS ALLOWED TO BE ABSENT. A check that refused every skill
// because it could not parse one Go file would block the tree for the wrong
// reason, so a nil vocabulary asserts nothing.
func TestASkillNamesOnlyTextToolFlagsThatExist(t *testing.T) {
	vocab := map[string]bool{"--text": true, "--expect": true, "--replace": true}
	cases := []struct {
		name  string
		line  string
		vocab map[string]bool
		want  int
	}{
		{"a flag it has", "text-tool edit x --replace a --text b --expect 1", vocab, 0},
		{"a flag it does not have", "text-tool edit x --teleport", vocab, 1},
		{"two of them", "text-tool edit x --teleport --vanish", vocab, 2},
		{"prose naming the tool passes no flags", "run text-tool when --teleport is needed", vocab, 0},
		{"a full path still counts", "/usr/local/bin/text-tool write x --teleport", vocab, 1},
		{"the Windows name still counts", "text-tool.exe write x --teleport", vocab, 1},
		// ⚠ A LINE THAT NAMES NO INVOCATION AT ALL, such as the tool's own
		// output quoted back inside a fenced block, passes no flags.
		{"output quoted back is not an invocation", "refused x: --expect 1 and this matches 0", vocab, 0},
		{"an unparseable usage asserts nothing", "text-tool edit x --teleport", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Result{Extra: map[string]any{}}
			body := skillHead + "```bash\n" + c.line + "\n```\n"
			skillFile(&r, "skills/text-tool/SKILL.md", []byte(body), steManual, c.vocab)
			got := 0
			for _, d := range r.Detail {
				if strings.Contains(d, "passes text-tool") {
					got++
				}
			}
			if got != c.want {
				t.Fatalf("%d finding(s), want %d: %s", got, c.want, strings.Join(r.Detail, " | "))
			}
		})
	}
}

// TestTheTextToolVocabularyComesFromTheRealUsageText holds the extraction to the
// file it reads, so a rename of the const cannot leave the rule asserting over
// an empty set and reporting green.
func TestTheTextToolVocabularyComesFromTheRealUsageText(t *testing.T) {
	src := "package main\n\nconst usage = `text-tool <write|edit>\n  --expect N  a count\n  --json      structured\n`\n"
	tree := treeWithFile(t, TextToolMain, src)
	got := textToolVocabulary(tree)
	for _, want := range []string{"--expect", "--json"} {
		if !got[want] {
			t.Fatalf("%s was not read out of the usage text: %v", want, got)
		}
	}
	if got["--absent"] {
		t.Fatal("it invented a flag the usage text does not hold")
	}
	// ⛔ AND A USAGE CONST THAT HOLDS NO FLAG AT ALL ANSWERS nil TOO. Without
	// this the rule would compare every skill against an EMPTY set and refuse
	// every flag in the tree, which is the widest possible false report, and
	// repo mutate called the row for it THEATRE until this case existed.
	empty := "package main\n\nconst usage = `text-tool does things\n`\n"
	if v := textToolVocabulary(treeWithFile(t, TextToolMain, empty)); v != nil {
		t.Fatalf("a usage text with no flags gave a vocabulary: %v", v)
	}
	// ⛔ A FILE WITH NO USAGE CONST ANSWERS nil, which makes the rule assert
	// nothing rather than refuse everything.
	if v := textToolVocabulary(treeWithFile(t, TextToolMain, "package main\n")); v != nil {
		t.Fatalf("a file with no usage text gave a vocabulary: %v", v)
	}
}
