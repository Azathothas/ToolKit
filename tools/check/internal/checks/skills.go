// SPDX-License-Identifier: 0BSD

package checks

import (
	"regexp"
	"strings"
)

// SkillsDir is where a standalone skill lives.
const SkillsDir = "skills/"

// Skills asserts that each skill is self-contained and that every command it
// shows a reader is a command this tool really has.
//
// ⛔ A SKILL IS HANDED TO AN AGENT WITH NOTHING ELSE. skills/README.md states
// the contract: "It is handed to an agent with no other context, so it repeats
// what it needs rather than pointing into this tree." A relative link out of
// skills/ breaks that for a reader who has only the one file, and there is no
// way for them to find out that it did.
//
// ⛔ AND A COMMAND THAT DOES NOT EXIST IS WORSE THAN NO PAGE. A capable agent
// reads a refusal and recovers. A weak one runs what the page said, gets exit 2,
// and reports the task as impossible. The generated manual is the authority
// here, and TestGeneratedManPageIsCurrent holds the manual to the binary's own
// registry, so this closes the loop from the skill to the code.
//
// ⚠ ONLY FENCED BLOCKS ARE READ. "across the wsl-toolkit bridge" is prose and
// names no command; reading prose would refuse the sentence rather than the
// command. A first version of this scan, run by hand, took `bridge` and
// `instance` for subcommands from exactly those two sentences.
func Skills(t *Tree) Result {
	r := Result{Extra: map[string]any{}}

	manual := string(t.Read(ManualPath))
	if strings.TrimSpace(manual) == "" {
		r.bad("%s", sprintf("%s is missing, and it is what says which commands exist", ManualPath))
		return r
	}

	// ⚠ READ ONCE, not per skill: it is the same file every time.
	textTool := textToolVocabulary(t)

	found := 0
	for _, f := range t.Files {
		if !strings.HasPrefix(f, SkillsDir) || !strings.HasSuffix(f, "/SKILL.md") {
			continue
		}
		found++
		skillFile(&r, f, t.Read(f), manual, textTool)
	}
	if found == 0 {
		r.bad("%s", sprintf("no %s*/SKILL.md is tracked, so this check asserted nothing", SkillsDir))
	}
	r.Extra["skills"] = found
	return r
}

// ManualPath is the generated manual, which is the authority for what exists.
const ManualPath = "tools/windows/wsl-toolkit/wsl-toolkit.1"

func skillFile(r *Result, path string, raw []byte, manual string, textTool map[string]bool) {
	body := string(raw)
	lines := Lines(raw)

	// The directory names the skill, and the front matter must agree with it.
	dir := strings.TrimSuffix(strings.TrimPrefix(path, SkillsDir), "/SKILL.md")
	if name := frontMatterValue(body, "name"); name == "" {
		r.bad("%s", sprintf("%s: the front matter declares no name, and a harness selects a skill by it", path))
	} else if name != dir {
		r.bad("%s", sprintf("%s: the front matter says name %q and the directory is %q. A harness that resolves one to the other finds nothing", path, name, dir))
	}
	if d := frontMatterValue(body, "description"); len(d) < 40 {
		r.bad("%s", sprintf("%s: the description is %d characters, and it is the only thing a harness reads when it decides whether to load this skill", path, len(d)))
	}

	inFence := false
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln.Text)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			// ⛔ A LINK OUT OF skills/ BREAKS THE ONE PROPERTY A SKILL HAS.
			for _, m := range skillLink.FindAllStringSubmatch(ln.Text, -1) {
				target := m[1]
				if strings.HasPrefix(target, "#") || strings.Contains(target, "://") {
					continue
				}
				if strings.HasPrefix(target, "../") || strings.HasPrefix(target, "/") {
					r.bad("%s", sprintf("%s:%d: links to %q, which is outside %s. A skill is read with nothing else present, so it repeats what it needs instead", path, ln.N, target, SkillsDir))
				}
			}
			continue
		}
		for _, cmd := range skillCommands(ln.Text) {
			if !manualHas(manual, cmd) {
				r.bad("%s", sprintf("%s:%d: names `wsl-toolkit %s`, which %s does not document. A weak agent runs what the page says and reports the task impossible", path, ln.N, cmd, ManualPath))
			}
		}
		for _, fl := range skillTextToolFlags(ln.Text) {
			if textTool != nil && !textTool[fl] {
				r.bad("%s", sprintf("%s:%d: passes text-tool %s, which its own usage text in %s does not document", path, ln.N, fl, TextToolMain))
			}
		}
	}
}

// skillCommands pulls the command and any subcommand out of one fenced line.
//
// ⚠ GLOBAL FLAGS ARE SKIPPED, NOT PARSED. `--instance base` sits between the
// program and its command, and a reader of this line wants the command.
func skillCommands(line string) []string {
	fields := strings.Fields(line)
	out := []string{}
	for i := 0; i < len(fields); i++ {
		if fields[i] != "wsl-toolkit" && !strings.HasSuffix(fields[i], "\\wsl-toolkit.exe") {
			continue
		}
		rest := fields[i+1:]
		// Skip global flags and their values.
		for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
			flag := rest[0]
			rest = rest[1:]
			if skillFlagTakesValue[flag] && len(rest) > 0 {
				rest = rest[1:]
			}
		}
		if len(rest) == 0 {
			break
		}
		cmd := rest[0]
		if !skillWord.MatchString(cmd) {
			break
		}
		// A second bare word is a subcommand. A flag ends the name.
		if len(rest) > 1 && skillWord.MatchString(rest[1]) {
			out = append(out, cmd+" "+rest[1])
		} else {
			out = append(out, cmd)
		}
		break
	}
	return out
}

var skillFlagTakesValue = map[string]bool{"--instance": true, "--home": true, "--config": true}

// manualHas asks the generated manual whether a command or a form exists.
func manualHas(manual, cmd string) bool {
	if strings.Contains(cmd, " ") {
		return strings.Contains(manual, ".B wsl\\-toolkit "+strings.ReplaceAll(cmd, "-", "\\-")) ||
			strings.Contains(manual, ".B wsl-toolkit "+cmd)
	}
	for _, line := range strings.Split(manual, "\n") {
		if strings.TrimSpace(line) == ".SS "+cmd {
			return true
		}
	}
	return false
}

// frontMatterValue reads one key out of the YAML front matter, which is the
// only YAML this repository has and is three keys deep at most.
func frontMatterValue(body, key string) string {
	if !strings.HasPrefix(body, "---\n") {
		return ""
	}
	end := strings.Index(body[4:], "\n---")
	if end < 0 {
		return ""
	}
	for _, line := range strings.Split(body[4:4+end], "\n") {
		if !strings.HasPrefix(line, key+":") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(line, key+":"))
		return strings.Trim(v, `"'`)
	}
	return ""
}

var (
	skillLink = regexp.MustCompile(`\]\(([^)]+)\)`)
	skillWord = regexp.MustCompile(`^[a-z][a-z-]*$`)
)

// TextToolMain is where text-tool's usage text lives, and the usage text is the
// authority for what flags it has.
const TextToolMain = "tools/text-tool/main.go"

// textToolVocabulary reads the modes and flags text-tool documents about itself.
//
// ⛔ THE SAME LOOP THE wsl-toolkit MANUAL CLOSES, for the other product. A skill
// naming a flag the program does not have sends a weak agent to exit 2, and a
// weak agent reports the task as impossible rather than reading the refusal.
// text-tool has no generated manual, so its own --help text is the authority:
// it is the string the program actually prints, so it cannot describe a
// different build.
//
// ⚠ IT RETURNS nil WHEN IT CANNOT FIND THE TEXT, and the caller then asserts
// nothing rather than refusing every skill. A check that fails closed on a file
// it could not parse is a check that blocks the tree for the wrong reason.
func textToolVocabulary(t *Tree) map[string]bool {
	src := string(t.Read(TextToolMain))
	const marker = "const usage = `"
	i := strings.Index(src, marker)
	if i < 0 {
		return nil
	}
	rest := src[i+len(marker):]
	j := strings.Index(rest, "`")
	if j < 0 {
		return nil
	}
	known := map[string]bool{}
	for _, tok := range textToolToken.FindAllString(rest[:j], -1) {
		known[tok] = true
	}
	if len(known) == 0 {
		return nil
	}
	return known
}

// textToolToken picks a long flag out of the usage text.
var textToolToken = regexp.MustCompile(`--[a-z0-9][a-z0-9-]*`)

// skillTextToolFlags returns the long flags a line passes to text-tool.
//
// ⛔ THE INVOCATION MUST BE THE FIRST WORD ON THE LINE, and that narrowing was
// forced by a case. Without it, prose reading "run text-tool when --teleport is
// needed" reported --teleport as a flag, because the scan took every dashed word
// after the name. A fenced block also holds the tool's OWN OUTPUT, which quotes
// flags back, and that is the same defect from the other side.
//
// ⚠ WHAT THIS DELIBERATELY DOES NOT SEE: a call nested inside another command,
// such as `pwsh -c "text-tool ..."`. Reporting those needs a shell parser, and a
// rule that is narrow and true beats one that is wide and guesses.
func skillTextToolFlags(line string) []string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}
	first := fields[0]
	if first != "text-tool" && first != "text-tool.exe" &&
		!strings.HasSuffix(first, "/text-tool") && !strings.HasSuffix(first, "\text-tool.exe") {
		return nil
	}
	var out []string
	for _, f := range fields[1:] {
		if strings.HasPrefix(f, "--") {
			out = append(out, strings.TrimSuffix(f, ","))
		}
	}
	return out
}
