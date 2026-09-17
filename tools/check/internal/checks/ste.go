// SPDX-License-Identifier: 0BSD

package checks

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// STE asserts the countable half of ASD-STE100, Simplified Technical English,
// over the documents a reader follows.
//
// ⛔ ONLY THE COUNTABLE HALF, AND THAT IS THE WHOLE DESIGN.
// docs/conventions/prose.md states the reason in its own words: a guard that
// tried to verify prose "would either pass vacuously or refuse legitimate
// writing, and both are worse than an honest scope". STE's voice rules - no
// passive, no gerund as a noun - cannot be decided by a regular expression
// without refusing correct sentences, so they stay with the review pass. What
// is here is arithmetic and an explicit table, and neither is a judgement.
//
// ⭐ THE CORPUS WAS MEASURED BEFORE THIS WAS WRITTEN, on 2026-09-17. The live
// documentation already held 4,836 sentences with ZERO over 25 words and a
// longest of 19, because prose.md has asked for short sentences all along. So
// this check is not a new burden placed on a corpus that fails it; it is the
// number that makes an existing rule enforceable, plus the three rules nobody
// was measuring.
//
// The four rules:
//
//  1. a sentence is at most 25 words, or 20 when it is a step in an ordered
//     list. STE writing rule 4.1, with procedures held tighter than description;
//  2. a paragraph is at most 6 sentences. STE writing rule 6.1;
//  3. a word with an approved replacement is replaced. STE rules 1.1 and 1.5,
//     applied through the explicit table below rather than through STE's full
//     dictionary, which has no entry for the technical names this tree needs;
//  4. one concept is written one way. STE rules 1.2 and 1.3. `distro` and
//     `distribution` are the same thing and this tree used both.
func STE(t *Tree) Result {
	r := Result{Extra: map[string]any{}}

	sentences, longest, files := 0, 0, 0
	for _, f := range t.Files {
		if !strings.HasSuffix(f, ".md") {
			continue
		}
		if why := steExemptReason(f); why != "" {
			continue
		}
		files++
		read, max := steFile(&r, f, t.Read(f))
		sentences += read
		if max > longest {
			longest = max
		}
	}
	r.Extra["files"] = files
	r.Extra["sentences"] = sentences
	r.Extra["longest_sentence"] = longest
	return r
}

// steExemptReason says why a document is outside this rule, or "" when it is
// inside it.
//
// ⛔ EVERY EXEMPTION IS A DECISION WITH A REASON, not a default. A rule with a
// silent scope is a rule nobody can argue with, and the set it skips grows.
//
// ⚠ THE RECORD IS EXEMPT AND IT IS NOT LAZINESS. TODO/ENTRY.md forbids
// rewriting an entry's title or premise after the fact, and the record is
// evidence of what was believed on a date rather than a document a reader
// follows. Rewriting it into STE would destroy the thing it exists to preserve.
// Measured on 2026-09-17: the record holds 18,858 sentences and FOUR of them
// are over 25 words, so the exemption costs almost nothing either way.
func steExemptReason(f string) string {
	switch {
	case strings.HasPrefix(f, "TODO/"):
		return "the work record: evidence of what was believed on a date, and ENTRY.md forbids rewriting a premise"
	case f == "CHANGELOG.md":
		return "the shipping log: its own rules forbid deleting or tidying an entry"
	case strings.HasPrefix(f, "docs/HISTORY/"):
		return "superseded wording, kept verbatim so a reader can see what a rule used to say"
	case strings.HasPrefix(f, "docs/reference-sweeps/"):
		return "readings taken from other projects, quoted rather than written here"
	case strings.HasPrefix(f, ".github/"):
		return "issue and pull request templates, whose wording GitHub renders"
	}
	return ""
}

// steFile applies the four rules to one document and returns how many sentences
// it read and the longest it found.
func steFile(r *Result, path string, raw []byte) (int, int) {
	masked := steMask(string(raw))
	lines := strings.Split(masked, "\n")

	total, longest := 0, 0
	unit := []string{}
	unitLine := 0
	unitIsItem := false

	// ⛔ A LIST ITEM IS NOT A PARAGRAPH, and the first version of this rule
	// said it was. It summed every item of an ordered list into one unit and
	// reported docs/methodology/sessions.md's seven-step start-of-session list as
	// a 16-sentence paragraph. That is a guard refusing correct writing, which
	// docs/conventions/prose.md names as worse than no guard at all. Each item is
	// its own unit, ended by the next marker or by a blank line.
	flush := func() {
		if len(unit) == 0 {
			return
		}
		n := len(steSentences(strings.Join(unit, " ")))
		if n > SentencesPerParagraph {
			what := "this paragraph"
			if unitIsItem {
				what = "this list item"
			}
			r.bad("%s", sprintf("%s:%d: %s is %d sentences and the ceiling is %d. STE rule 6.1: one topic, and a reader holds one topic",
				path, unitLine, what, n, SentencesPerParagraph))
		}
		unit = nil
		unitIsItem = false
	}

	for i, line := range lines {
		n := i + 1
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		if steItem.MatchString(line) {
			flush()
			unitIsItem = true
		}
		if len(unit) == 0 {
			unitLine = n
		}
		// ⚠ THE MARKER IS NOT A SENTENCE. "1." ends with a stop and is followed
		// by a capital, so the splitter read it as one, and every list item counted
		// one sentence more than it has. That is an off-by-one against the ceiling
		// in rule 6.1, in the direction that refuses correct writing.
		unit = append(unit, steItem.ReplaceAllString(line, ""))

		ceiling := WordsPerSentence
		kind := "a sentence"
		if steStep.MatchString(line) {
			// ⭐ A STEP IS HELD TIGHTER THAN DESCRIPTION, which is STE's own
			// split: somebody is doing what the line says while they read it.
			ceiling = WordsPerStep
			kind = "this step"
		}
		for _, s := range steSentences(steItem.ReplaceAllString(line, "")) {
			w := len(strings.Fields(s))
			total++
			if w > longest {
				longest = w
			}
			if w > ceiling {
				r.bad("%s", sprintf("%s:%d: %s is %d words and the ceiling is %d. STE rule 4.1: %s",
					path, n, kind, w, ceiling, steShorten(s)))
			}
		}

		lower := strings.ToLower(line)
		for _, sub := range steWords {
			if idx := steWordAt(lower, sub.bad); idx >= 0 {
				r.bad("%s", sprintf("%s:%d: %q has an approved replacement, %q. STE rule 1.1",
					path, n, sub.bad, sub.good))
			}
		}
		for _, sub := range steShortForms {
			if idx := steWordAt(lower, sub.bad); idx >= 0 {
				r.bad("%s", sprintf("%s:%d: %q and %q are one concept written two ways. STE rules 1.2 and 1.3: write %q in prose, and keep %q for the command or field that is named that",
					path, n, sub.bad, sub.good, sub.good, sub.bad))
			}
		}
	}
	flush()
	return total, longest
}

// WordsPerSentence and WordsPerStep are STE writing rule 4.1's two ceilings.
// SentencesPerParagraph is rule 6.1's.
const (
	WordsPerSentence      = 25
	WordsPerStep          = 20
	SentencesPerParagraph = 6
)

type steSub struct{ bad, good string }

// steWords is STE's dictionary applied where it can be applied here: a word
// that is not approved, beside the word that is.
//
// ⛔ IT IS A TABLE AND NOT THE DICTIONARY. STE's Part 2 approves about 900
// general words and has no entry for `distribution`, `namespace`, `digest` or
// `prerelease`, which is what STE's own Technical Name rule (1.4) exists for.
// Applying the whole dictionary here would refuse every technical noun this
// tree is about. Each row below is a general word with a general replacement.
var steWords = []steSub{
	{"utilize", "use"}, {"utilise", "use"}, {"utilizes", "uses"}, {"utilized", "used"},
	{"perform", "do"}, {"performs", "does"}, {"performed", "did"},
	{"prior to", "before"}, {"in order to", "to"}, {"due to the fact that", "because"},
	{"commence", "start"}, {"commences", "starts"}, {"commenced", "started"},
	{"terminate", "stop"}, {"terminates", "stops"}, {"terminated", "stopped"},
	{"obtain", "get"}, {"obtains", "gets"}, {"obtained", "got"},
	{"additional", "more"}, {"approximately", "about"}, {"sufficient", "enough"},
	{"subsequent", "next"}, {"subsequently", "then"}, {"numerous", "many"},
	{"attempt", "try"}, {"attempts", "tries"}, {"attempted", "tried"},
	{"assist", "help"}, {"assists", "helps"}, {"initiate", "start"},
	{"facilitate", "help"}, {"leverage", "use"}, {"endeavour", "try"},
	{"whilst", "while"}, {"amongst", "among"},
}

// steShortForms is one concept written two ways.
//
// ⚠ EACH SHORT FORM IS ALSO A NAME IN THIS TOOL - `wsl-toolkit distro list`,
// `base exec --dir`, the `config` command, `repo release`. That is exactly why
// the mask below removes every code span before this runs: inside backticks the
// short form is a Technical Name and STE rule 1.4 allows it, and outside them it
// is prose and rule 1.3 does not.
var steShortForms = []steSub{
	{"distro", "distribution"}, {"distros", "distributions"},
	{"repo", "repository"}, {"repos", "repositories"},
	{"config", "configuration"}, {"configs", "configurations"},
	{"dir", "directory"}, {"dirs", "directories"},
}

var (
	// A step in an ordered list, which STE holds to the tighter ceiling.
	steStep = regexp.MustCompile(`^\s*[0-9]+\.\s`)
	// Any list item, ordered or not. It ends the unit before it.
	steItem = regexp.MustCompile(`^\s*(?:[0-9]+\.|[-*+])\s`)
	// A sentence ends at a stop followed by something that starts one. A marker
	// starts a sentence here as surely as a capital does.
	steBreak = regexp.MustCompile(`(?U)[.!?]\s+`)
)

// steSentences splits masked prose into sentences.
func steSentences(s string) []string {
	var out []string
	rest := strings.TrimSpace(s)
	for len(rest) > 0 {
		loc := steBreak.FindStringIndex(rest)
		if loc == nil {
			break
		}
		head := strings.TrimSpace(rest[:loc[1]])
		tail := strings.TrimSpace(rest[loc[1]:])
		// ⚠ A STOP INSIDE A NUMBER OR AN ABBREVIATION IS NOT A SENTENCE END.
		// Only a capital or a marker starts the next one.
		if tail == "" || !steStarts(tail) {
			rest = rest[:loc[0]] + " " + rest[loc[1]:]
			continue
		}
		if head != "" {
			out = append(out, head)
		}
		rest = tail
	}
	if strings.TrimSpace(rest) != "" {
		out = append(out, strings.TrimSpace(rest))
	}
	return out
}

func steStarts(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
			return true
		case r == Stop || r == Star || r == Warn:
			return true
		case r == '*' || r == '_' || r == '"':
			continue
		default:
			return false
		}
	}
	return false
}

// steWordAt finds a whole word, and returns where. A substring inside a longer
// word is not the word.
func steWordAt(haystack, needle string) int {
	from := 0
	for {
		i := strings.Index(haystack[from:], needle)
		if i < 0 {
			return -1
		}
		i += from
		before := byte(' ')
		if i > 0 {
			before = haystack[i-1]
		}
		after := byte(' ')
		if end := i + len(needle); end < len(haystack) {
			after = haystack[end]
		}
		if !steWordByte(before) && !steWordByte(after) {
			return i
		}
		from = i + 1
		if from >= len(haystack) {
			return -1
		}
	}
}

func steWordByte(b byte) bool {
	return b == '-' || b == '_' || b == '/' || b == '.' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// steMask blanks everything a reader does not read as a sentence, and keeps
// every byte offset and every newline so a finding can still name its line.
//
// ⛔ THE MASK RUNS OVER THE WHOLE DOCUMENT, NOT LINE BY LINE. An inline code
// span wraps across a line break in this tree - `gh release list --repo` does,
// in README.md - and a line-by-line mask reads the half after the break as
// prose. That produced a false finding in the first measurement, and it is the
// one defect a scanner of this shape reliably has.
func steMask(s string) string {
	b := []byte(s)
	blank := func(from, to int) {
		for i := from; i < to && i < len(b); i++ {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
	}

	// Fenced blocks, front matter, tables and headings, by line.
	inFence, inFront := false, false
	pos := 0
	for i, line := range strings.Split(s, "\n") {
		start, end := pos, pos+len(line)
		pos = end + 1
		trimmed := strings.TrimSpace(line)
		switch {
		case i == 0 && trimmed == "---":
			inFront = true
			blank(start, end)
			continue
		case inFront:
			if trimmed == "---" {
				inFront = false
			}
			blank(start, end)
			continue
		case strings.HasPrefix(trimmed, "```"):
			inFence = !inFence
			blank(start, end)
			continue
		case inFence, strings.HasPrefix(trimmed, "|"), strings.HasPrefix(trimmed, "#"):
			blank(start, end)
			continue
		}
	}

	out := string(b)
	// Inline code, then link targets, then URLs, then paths. Order matters: a
	// path inside a code span is already gone by the time paths are masked.
	for _, re := range steSpans {
		out = re.ReplaceAllStringFunc(out, func(m string) string {
			keep := []byte(m)
			for i := range keep {
				if keep[i] != '\n' {
					keep[i] = ' '
				}
			}
			return string(keep)
		})
	}
	return out
}

var steSpans = []*regexp.Regexp{
	regexp.MustCompile("(?s)`[^`]*`"),
	regexp.MustCompile(`\]\([^)]*\)`),
	regexp.MustCompile(`https?://\S+`),
	regexp.MustCompile(`[A-Za-z0-9_.-]*[\\/][A-Za-z0-9_.\\/-]+`),
}

// steShorten keeps a finding readable without losing which sentence it names.
func steShorten(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= 72 {
		return s
	}
	return s[:69] + "..."
}

// SteExemptions is what the check skips and why, so a reader can see the scope
// without reading the code. It is sorted, because a set printed in map order is
// a set that looks different every run.
func SteExemptions() []string {
	seen := map[string]string{
		"TODO/":                  steExemptReason("TODO/x.md"),
		"CHANGELOG.md":           steExemptReason("CHANGELOG.md"),
		"docs/HISTORY/":          steExemptReason("docs/HISTORY/x.md"),
		"docs/reference-sweeps/": steExemptReason("docs/reference-sweeps/x.md"),
		".github/":               steExemptReason(".github/x.md"),
	}
	out := make([]string, 0, len(seen))
	for k, v := range seen {
		out = append(out, fmt.Sprintf("%s: %s", k, v))
	}
	sort.Strings(out)
	return out
}
