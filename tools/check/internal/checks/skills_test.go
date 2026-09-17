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
	skillFile(&r, path, []byte(body), steManual)
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
