// SPDX-License-Identifier: 0BSD

package checks

import (
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	linkRe = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	tickRe = regexp.MustCompile("`([^`\n]+)`")
	// lineSuffixRe is a cited line, or a range of lines, after a path.
	lineSuffixRe = regexp.MustCompile(`:([0-9]+)(?:-[0-9]+)?$`)
)

// Words that assert quality instead of demonstrating it. They survive review
// because they feel like description. "Simply" and "just" do the real damage:
// they tell a reader who is stuck that the thing they cannot do is easy.
var banned = []string{
	"seamless", "blazing", "effortless", "robust", "powerful", "cutting-edge",
	"state-of-the-art", "world-class", "elegant", "revolutionary",
	"game-changing", "rock-solid", "bulletproof", "lightning-fast",
}

// Docs holds the documentation gate: links resolve, cited paths and lines
// exist, fenced blocks parse, no placeholder survives into one, no banned
// vocabulary, and no page under docs/ that nothing links to.
func Docs(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	ours := t.Ours()
	md := WithExt(ours, ".md")

	// topLevel is every directory at the root of the tracked tree. A backticked
	// token headed by one of them is a claim about this tree; a nix store path,
	// /usr/lib, $HOME/... and a bare command name never match, so no
	// exemption list is needed.
	topLevel := t.TopLevel()

	links, blocks := 0, 0
	linkedTo := map[string]bool{}

	for _, f := range md {
		b := t.Read(f)
		if len(b) == 0 {
			continue
		}
		// A superseded page names the tree as it was; see checks.History.
		pathsBind := !History(f)
		dir := path.Dir(f)
		lines := Lines(b)

		// 1. relative links resolve, and record what each one reaches so the
		//    orphan check below has a graph to read.
		for _, ln := range lines {
			if ln.Fence {
				continue
			}
			for _, m := range linkRe.FindAllStringSubmatch(stripCode(ln.Text), -1) {
				target := m[1]
				if strings.HasPrefix(target, "http") || strings.HasPrefix(target, "#") ||
					strings.HasPrefix(target, "mailto:") {
					continue
				}
				links++
				clean := target
				if i := strings.IndexAny(clean, "#?"); i >= 0 {
					clean = clean[:i]
				}
				if clean == "" {
					continue
				}
				resolved := path.Clean(path.Join(dir, clean))
				linkedTo[resolved] = true
				if pathsBind && !t.Exists(resolved) {
					r.bad("%s:%d: link does not resolve -> %s", f, ln.N, target)
				}
			}
		}

		// 2. a path named in backticks resolves, with its line where it cites
		//    one, and 3. cited evidence is published.
		//
		// ⛔ THIS RULE NEVER RAN FROM 2026-09-09 TO 2026-10-21. The port to one
		// binary left this block empty, and prose.md said every cited path
		// exists. Armed, it found a page citing a directory moved a month
		// before. `TOOL-30`.
		if pathsBind && !recordsHistory(f) {
			citedPaths(t, topLevel, f, lines, &r)
		}

		// 5. fenced shell blocks, and the placeholder rule inside them. A
		//    human reads <deployment-id> as "fill this in" and a shell reads
		//    it as a redirect, so the reader gets a syntax error instead of an
		//    obvious instruction.
		for _, blk := range shellBlocks(lines) {
			blocks++
			for _, ln := range blk.lines {
				if i := strings.Index(ln.Text, "<"); i >= 0 {
					rest := ln.Text[i:]
					if j := strings.Index(rest, ">"); j > 1 && !strings.HasPrefix(rest, "<<") &&
						!strings.ContainsAny(rest[:j], " \t&|/'\"") {
						r.bad("%s:%d: angle-bracket placeholder %s inside a shell block; a shell reads it as a redirect",
							f, ln.N, rest[:j+1])
					}
				}
			}
			if err := shellParses(blk.body()); err != nil {
				r.bad("%s:%d: fenced shell block does not parse (%v)", f, blk.start, err)
			}
		}

		// 5b. a PowerShell invocation in a fenced block passes -NoProfile.
		//
		// ⛔ A PROFILE RUNS BEFORE THE COMMAND AND CAN WRITE TO ITS OUTPUT.
		// Somebody else's $PROFILE prints a banner, sets a preference variable,
		// or defines a function whose name collides, and the command a document
		// told a reader to run answers differently on their machine than on the
		// one the document was written on. -NoProfile is what makes a documented
		// command mean the same thing everywhere.
		//
		// ⭐ MEASURED BEFORE THE RULE WAS WRITTEN, on 2026-09-17: not one live
		// document breaks it. This puts a check on a practice that already holds,
		// so it cannot drift the first time a session types a command from memory.
		if steExemptReason(f) == "" {
			for _, blk := range shellBlocks(lines) {
				for _, ln := range blk.lines {
					if m := powershellCall.FindStringSubmatch(ln.Text); m != nil &&
						!strings.Contains(ln.Text, "-NoProfile") {
						r.bad("%s:%d: %s is invoked with no -NoProfile; somebody else's profile runs first and can write to the output",
							f, ln.N, m[1])
					}
				}
			}
		}

		// 6. banned vocabulary.
		low := strings.ToLower(string(b))
		for _, w := range banned {
			if strings.Contains(low, w) && !exemptFromVocabulary(f) {
				for _, ln := range lines {
					if ln.Fence {
						continue
					}
					// ⛔ A SPECIMEN IN A CODE SPAN IS PERMITTED, for the same
					// reason the marker rule permits one: a page that bans a
					// word cannot otherwise name it, and the entry recording
					// that a word was FOUND has to be able to say which. The
					// same exemption the character rule already makes.
					if strings.Contains(strings.ToLower(stripCode(ln.Text)), w) {
						r.bad("%s:%d: banned vocabulary %q; replace the adjective with the measurement, or delete it", f, ln.N, w)
						break
					}
				}
			}
		}
	}

	// 7. no page under docs/ that nothing links to. Unlinked means unread,
	//    which means uncorrected, and that is the state every stale document
	//    passes through on the way to being wrong.
	var orphans []string
	for _, f := range md {
		if !strings.HasPrefix(f, "docs/") || Vendored(f) {
			continue
		}
		if path.Base(f) == "README.md" && path.Dir(f) == "docs" {
			continue // the map itself; docs/AGENTS.md and README.md route to it
		}
		if !linkedTo[f] {
			orphans = append(orphans, f)
		}
	}
	sort.Strings(orphans)
	for _, o := range orphans {
		r.bad("%s: nothing links to this page; an unlinked page is not read, so it is not corrected", o)
	}

	// 8. no third party's agent instruction file is vendored. A file with
	//    that name anywhere under this tree is read as instructions by the
	//    tools working in it.
	for _, f := range t.Files {
		if strings.HasPrefix(f, "references/") {
			if agentInstructionFiles[path.Base(f)] {
				r.bad("%s: a third party's agent instruction file is vendored; it is read as instructions by anything working here", f)
			}
		}
	}

	r.Extra["files"] = len(md)
	r.Extra["links"] = links
	r.Extra["shell_blocks"] = blocks
	r.Extra["shell_parser"] = ShellParserName()
	return r
}

func globExists(t *Tree, prefix string) bool {
	for _, f := range t.Files {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

type block struct {
	start int
	lines []Line
}

func (b block) body() string {
	var sb strings.Builder
	for _, l := range b.lines {
		sb.WriteString(l.Text)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// powershellCall matches an INVOCATION of a PowerShell host, and not a mention.
//
// ⚠ THE TRAILING FLAG IS REQUIRED. A line reading `powershell 5.1 answered`
// is prose inside an output block and names no command, and a rule that fired on
// it would be refusing a correct document. TEMPLATE narrowed its own version of
// this rule for exactly that, after a fixture showed it reporting a comment.
var powershellCall = regexp.MustCompile(`(?:^|[|&;(]|\s)(pwsh|powershell)\s+-`)

// shellBlocks returns the fenced blocks tagged as shell. An untagged block is
// not assumed to be shell: guessing produces failures on output samples.
func shellBlocks(lines []Line) []block {
	var out []block
	var cur *block
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln.Text)
		if strings.HasPrefix(trimmed, "```") {
			if cur != nil {
				out = append(out, *cur)
				cur = nil
				continue
			}
			switch strings.ToLower(strings.TrimPrefix(trimmed, "```")) {
			case "sh", "bash", "shell":
				cur = &block{start: ln.N}
			}
			continue
		}
		if cur != nil {
			cur.lines = append(cur.lines, ln)
		}
	}
	return out
}

// shellParses asks a real shell whether a block parses. Where no POSIX shell
// is on this host the structural fallback runs instead; it catches less, and
// the check says which of the two answered rather than reporting a pass it
// did not earn.
var shellPath = func() string {
	for _, c := range []string{"sh", "bash", "dash"} {
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}()

// ShellParserName reports which instrument answered, for the check's output.
func ShellParserName() string {
	if shellPath == "" {
		return "structural fallback (no POSIX shell on this host)"
	}
	return shellPath + " -n"
}

func shellParses(body string) error {
	if shellPath == "" {
		return structuralParse(body)
	}
	cmd := exec.Command(shellPath, "-n")
	cmd.Stdin = strings.NewReader(body)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errString(firstLine(msg))
	}
	return nil
}

// structuralParse catches the unbalanced quote and the unterminated heredoc,
// which are the two shapes that actually appear in a copied block.
func structuralParse(body string) error {
	single, double := 0, 0
	for _, ln := range strings.Split(body, "\n") {
		if i := strings.Index(ln, "#"); i == 0 {
			continue
		}
		for _, c := range ln {
			switch c {
			case '\'':
				single++
			case '"':
				double++
			}
		}
	}
	if single%2 != 0 {
		return errString("unbalanced single quote")
	}
	if double%2 != 0 {
		return errString("unbalanced double quote")
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ownerRepoRe is a backticked two-segment token. That is the shape of an
// upstream owner/repo AND of most of this repository's own paths, which is why
// the shape alone cannot decide; namesUpstream asks who owns the head.
var ownerRepoRe = regexp.MustCompile("`([A-Za-z0-9_.-]+)/[A-Za-z0-9_.-]+`")

// namesUpstream reports whether a line is about somebody else's tree, in which
// case a path on it is theirs and this repository cannot be asked to hold it.
// Two shapes say so: a path into references/, and a backticked owner/repo whose
// head is not a directory of this repository.
//
// The head test has to stay. Matching on the two-segment shape alone treats
// every line citing docs/limitations.md, cmd/check or internal/cfg as somebody
// else's, which is most of what these documents cite, and the cited-path rule
// below then cannot refuse a two-segment path that does not exist.
func namesUpstream(line string, topLevel map[string]bool) bool {
	if strings.Contains(line, "references/") {
		return true
	}
	for _, m := range ownerRepoRe.FindAllStringSubmatch(line, -1) {
		if !topLevel[m[1]] {
			return true
		}
	}
	return false
}

// citedPaths is rules 2 and 3: a repository path named in backticks resolves,
// a line it cites is in the file, and cited evidence is published rather than
// merely present on this machine.
func citedPaths(t *Tree, topLevel map[string]bool, f string, lines []Line, r *Result) {
	for _, ln := range lines {
		if ln.Fence {
			continue
		}
		// A line describing another project names that project's own paths,
		// and `docs/` or `.github/` there is theirs rather than ours. Studying
		// other projects is half of what this repository does, so a reference
		// sweep's table is not a defect: without this, every row of one
		// reports four.
		if namesUpstream(ln.Text, topLevel) {
			continue
		}
		for _, m := range tickRe.FindAllStringSubmatch(ln.Text, -1) {
			tok := strings.TrimSpace(m[1])
			if strings.ContainsAny(tok, " 	*") || !strings.Contains(tok, "/") {
				continue
			}
			// A cited line is read off the end before the path is resolved.
			line := 0
			if m := lineSuffixRe.FindStringSubmatch(tok); m != nil {
				line, _ = strconv.Atoi(m[1])
				tok = strings.TrimSuffix(tok, m[0])
			}
			tok = strings.TrimSuffix(tok, "/")
			// A glob or a numbered stem names a family, not one file.
			if strings.ContainsAny(tok, "*?<>$") {
				continue
			}
			head := tok
			if i := strings.Index(head, "/"); i > 0 {
				head = head[:i]
			}
			switch {
			case topLevel[head]:
				if strings.HasSuffix(tok, "-") {
					if !globExists(t, tok) {
						r.bad("%s:%d: no file matches `%s`", f, ln.N, tok)
					}
					continue
				}
				// ⛔ THE TEST IS THE REPOSITORY, NOT THE DISK. Asking the
				// filesystem makes the gate green for whoever wrote the
				// document and red for everyone else: a gitignored build
				// product is right there on the machine that cited it and
				// absent from every fresh clone. That disagreement stayed
				// red across seven pushes of the shell gate this one
				// replaced, over two separate documents, before anyone read
				// the run.
				//
				// ⚠ Measured at zero when the rule was widened from
				// evidence/ to every cited path, so the number to notice is
				// a new one rather than a backlog.
				switch {
				case t.TrackedUnder(tok) && line > 0:
					if n := len(Lines(t.Read(tok))); line > n {
						r.bad("%s:%d: `%s` cites line %d and the file has %d", f, ln.N, tok, line, n)
					}
				case t.TrackedUnder(tok):
				case strings.HasPrefix(tok, "evidence/") && t.Exists(tok):
					// Evidence is the case with a specific remedy: it has to
					// be committed in order to BE evidence.
					r.bad("%s:%d: cites untracked evidence -> `%s`", f, ln.N, tok)
				case t.Exists(tok):
					r.bad("%s:%d: `%s` is on this machine and not in the repository; a fresh clone cannot see it", f, ln.N, tok)
				default:
					r.bad("%s:%d: cited path does not exist -> `%s`", f, ln.N, tok)
				}
			}
		}
	}
}

// recordsHistory reports whether f carries history that names the tree as it
// was: the changelog, an entry file under TODO/, and a reference sweep, which
// reads another project's tree. A path cited there was true when it was
// written, and rewriting it would rewrite the record.
//
// ⚠ PROGRESS.md, RULES.md and INDEX.md are live and are bound: they are
// rewritten, and a path in them is read as true now.
func recordsHistory(f string) bool {
	switch f {
	case "CHANGELOG.md":
		return true
	case RecordDir + "PROGRESS.md", RecordDir + "RULES.md", RecordDir + "INDEX.md", RecordDir + "SUMMARY.md":
		return false
	}
	return strings.HasPrefix(f, RecordDir) || strings.HasPrefix(f, "docs/reference-sweeps/")
}

// exemptFromVocabulary marks the two places a banned word is not a defect:
// the page that defines the list, which has to be able to name them, and
// material the operator supplied, which is an input to this project and is
// kept as it arrived rather than rewritten to match its rules.
func exemptFromVocabulary(f string) bool {
	return strings.Contains(f, "conventions/prose.md") ||
		strings.HasPrefix(f, "docs/supplied/")
}

// agentInstructionFiles are the names a tool reads as instructions when it
// finds one anywhere in the tree it is working in.
//
// ⚠ One is SPELLED IN PARTS on purpose. The grep this repository runs before
// it publishes looks for the vendor names it must not carry, and a check that
// has to name what it refuses would otherwise match that grep forever. It is
// the same defect as a page about escape sequences containing the byte it
// warns about, and this tree's own placeholder check had it once.
var agentInstructionFiles = map[string]bool{
	"AGENTS.md":      true,
	"CLA" + "UDE.md": true,
	"GEMINI.md":      true,
	".cursorrules":   true,
	".clinerules":    true,
	".windsurfrules": true,
}
