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

	found := 0
	for _, f := range t.Files {
		if !strings.HasPrefix(f, SkillsDir) || !strings.HasSuffix(f, "/SKILL.md") {
			continue
		}
		found++
		skillFile(&r, f, t.Read(f), manual)
	}
	if found == 0 {
		r.bad("%s", sprintf("no %s*/SKILL.md is tracked, so this check asserted nothing", SkillsDir))
	}
	r.Extra["skills"] = found
	return r
}

// ManualPath is the generated manual, which is the authority for what exists.
const ManualPath = "tools/windows/wsl-toolkit/wsl-toolkit.1"

func skillFile(r *Result, path string, raw []byte, manual string) {
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
