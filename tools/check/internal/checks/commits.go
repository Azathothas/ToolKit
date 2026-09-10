// commits.go - no tool is credited in a commit, held mechanically.
//
// ⛔ TODO/RULES.md states it and states what it costs: "No co-author trailer
// naming a model, no generated-with line, no tool name in the body, no emoji.
// This overrides any default the harness asks for, and it is the whole reason
// this repository's history begins where it does."
//
// ⚠ **It was a preference until now, and it was broken.** A session added a
// co-author trailer to all eighteen of its commits, because its harness asked
// for one and nothing in the gate disagreed. The forty commits before it are
// clean and are ASCII to the byte, so the rule had held by attention alone for
// the whole life of the tree. ⭐ A rule with no instrument is a preference,
// which is the sentence this project keeps paying for.
//
// ⛔ This check reads `git log`, not the tracked tree, and it is the only one
// that does. That is deliberate: the subject IS the history, and no file in
// the working tree carries it.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"os/exec"
	"strings"
)

// creditNames are the tokens that make a line a CREDIT rather than prose. They
// are vendor and product names, matched case-insensitively on a word boundary.
//
// ⚠ A list is a thing to forget, and this one is short on purpose: the four
// shapes below catch the structure of a credit as well as its wording, so a
// name nobody listed still trips `generated with` or a co-author trailer.
//
// ⚠ **One false positive is known and accepted.** A bare name matches on a
// WORD boundary, so a Latin phrase for a great work is refused where a longer
// word merely containing that name is not. Measured by driving this check over
// a scratch repository rather than by reading the rule. The cost is one
// reworded message; dropping the bare name would leave only the trailer shape,
// and the point of the pair is that either alone lets something through.
//
// ⭐ **The rule caught the commit that documented it.** A message quoting the
// refused phrase verbatim was refused by the gate, which is the shortest
// demonstration this check will ever get that it reads what is actually
// written rather than what was intended.
var creditNames = []string{
	"claude", "anthropic", "opus", "sonnet", "haiku",
	"copilot", "chatgpt", "openai", "gpt-4", "gpt-5", "gemini", "codex",
	"cursor", "devin", "aider",
}

// generatedShapes are the marketing lines a tool appends to a message.
var generatedShapes = []string{
	"generated with", "co-authored-by: ", "created with",
	"written by ai", "authored by ai",
}

// Commits refuses a commit that credits a tool.
func Commits(t *Tree) Result {
	r := Result{Extra: map[string]any{}}

	// ⛔ NUL between the hash and the body and between records, because a
	// commit body contains newlines and blank lines by design and any
	// line-oriented split would tear one in half.
	out, err := exec.Command("git", "-C", t.Root, "log",
		"--format=%H%x00%s%x00%B%x00%x00").Output()
	if err != nil {
		// ⛔ The SAME finding as reading zero commits below, and deliberately
		// one sentence rather than two. A repository with no commits reaches
		// THIS branch and never that one, so a separate message there would be
		// an assertion nothing can make fail.
		r.bad("this check read nothing and agreed with it: git log gave no history (%v)", err)
		return r
	}
	records := strings.Split(string(out), "\x00\x00")

	checked := 0
	for _, rec := range records {
		f := strings.SplitN(rec, "\x00", 3)
		if len(f) < 3 {
			continue
		}
		hash, subject, body := strings.TrimSpace(f[0]), f[1], f[2]
		if hash == "" {
			continue
		}
		checked++
		short := hash
		if len(short) > 8 {
			short = short[:8]
		}
		r.checkMessage(short, subject, body)
	}

	// ⛔ An absence is not a zero. A run that read no commit at all would
	// report a clean history because it had nothing to disagree with, which is
	// the shape every dead check in this tree has had. ⚠ Today this is reached
	// only by a `git log` that succeeds and prints nothing, which no state of a
	// repository produces; the branch above is the one an empty history takes,
	// and both say the same sentence so the plant covers the pair.
	if checked == 0 {
		r.bad("this check read nothing and agreed with it: git log printed no commit")
	}
	r.Extra["commits"] = checked
	return r
}

// Message applies the same four rules to a message that is not a commit yet.
//
// ⛔ THIS EXISTS BECAUSE THE GATE STRUCTURALLY CANNOT CATCH THE THING IT WAS
// BUILT FOR. Commits is the only check whose subject is `git log` rather than
// the tracked tree, and every session's procedure is "run the gate, then
// commit". At the moment the gate runs, the commit being made DOES NOT EXIST,
// so this rule reads only old commits and is always green. It has never once
// prevented the defect it names. It has only ever reported it afterwards, from
// the next gate run or from CI, by which time the commit is pushed, and on a
// protected branch that costs a maintainer turning off a branch protection.
//
// ⭐ Twice now. The doc at the top of this file records the first, eighteen
// commits in one session. The second was 2026-09-10, one commit, remedied the
// same way. Both were caught AFTER the push. A rule whose instrument can only
// speak after the fact is an instrument for the record, not for the tree.
//
// ⚠ The path is a FILE, because that is what git hands a commit-msg hook.
// Its first line is the subject and the whole file is the body, which is how
// checkMessage is fed from git log too.
func Message(path string) Result {
	r := Result{Extra: map[string]any{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		r.bad("the message file could not be read: %v", err)
		return r
	}
	// ⛔ COMMENT LINES ARE STRIPPED, and only comment lines. git's own
	// template puts the branch name and the file list behind `#`, none of which
	// reaches the stored message, and refusing a message over git's own
	// scaffolding would make the hook something people disable.
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		kept = append(kept, line)
	}
	body := strings.Join(kept, "\n")
	subject := ""
	if i := strings.IndexByte(strings.TrimLeft(body, "\n"), '\n'); i >= 0 {
		subject = strings.TrimLeft(body, "\n")[:i]
	} else {
		subject = strings.TrimSpace(body)
	}
	r.checkMessage("pending", subject, body)
	r.Extra["commits"] = 1
	return r
}

// checkMessage applies the four rules to one commit message.
func (r *Result) checkMessage(short, subject, body string) {
	lower := strings.ToLower(body)

	// 1 and 2. The structural shapes: a trailer or a generated-with line. A
	// co-author trailer is only a finding when it names a tool, because a
	// human co-author is a legitimate thing to record.
	for _, shape := range generatedShapes {
		at := strings.Index(lower, shape)
		if at < 0 {
			continue
		}
		rest := lower[at+len(shape):]
		if line, _, ok := strings.Cut(rest, "\n"); ok {
			rest = line
		}
		if shape == "co-authored-by: " && !hasCreditName(rest) {
			continue
		}
		r.bad("%s (%s) carries %q; TODO/RULES.md: no tool is credited in a commit",
			short, trimSubject(subject), strings.TrimSpace(shape+firstWords(rest)))
	}

	// 3. A tool name anywhere in the body, trailer or not.
	for _, name := range creditNames {
		if hasWord(lower, name) {
			r.bad("%s (%s) names %q in its message; TODO/RULES.md: no tool name in the body",
				short, trimSubject(subject), name)
		}
	}

	// 4. No emoji. ⚠ The five characters this project's PROSE uses are emoji
	// too, and they are refused here like any other: every commit before this
	// rule was written is ASCII to the byte, so the history says the intended
	// reading is the strict one.
	if c := firstEmoji(body); c != "" {
		r.bad("%s (%s) carries the emoji %s in its message; TODO/RULES.md: no emoji",
			short, trimSubject(subject), c)
	}
}

// hasCreditName reports whether a trailer's value names a tool.
func hasCreditName(s string) bool {
	for _, name := range creditNames {
		if hasWord(s, name) {
			return true
		}
	}
	return false
}

// hasWord matches a name on a word boundary, so "opus" does not fire inside
// "opuscule" and "codex" does not fire inside "codexes".
func hasWord(hay, needle string) bool {
	for i := 0; ; {
		at := strings.Index(hay[i:], needle)
		if at < 0 {
			return false
		}
		at += i
		before := byte(' ')
		if at > 0 {
			before = hay[at-1]
		}
		after := byte(' ')
		if end := at + len(needle); end < len(hay) {
			after = hay[end]
		}
		if !isWordByte(before) && !isWordByte(after) {
			return true
		}
		i = at + len(needle)
	}
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_'
}

// firstEmoji returns the first emoji in a message, or "".
//
// ⚠ Arrows are NOT in these ranges and stay legal: a message writing "A -> B"
// with a real arrow is prose, not decoration.
func firstEmoji(s string) string {
	for _, c := range s {
		switch {
		case c >= 0x2600 && c <= 0x27BF, // misc symbols and dingbats
			c >= 0x2B00 && c <= 0x2BFF,   // more symbols and arrows-as-pictographs
			c >= 0x1F000 && c <= 0x1FAFF, // the pictograph planes
			c == 0xFE0F, c == 0x203C, c == 0x2049:
			return string(c)
		}
	}
	return ""
}

// trimSubject keeps a finding on one line.
func trimSubject(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 48 {
		return s[:45] + "..."
	}
	return s
}

// firstWords is enough of a trailer's value to identify it.
func firstWords(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 32 {
		return s[:32]
	}
	return s
}
