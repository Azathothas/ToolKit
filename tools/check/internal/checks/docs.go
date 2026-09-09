// SPDX-License-Identifier: 0BSD

package checks

import (
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"
)

var (
	linkRe = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	tickRe = regexp.MustCompile("`([^`\n]+)`")
	// A backticked token whose first segment is a real top-level directory of
	// this repository. A nix store path, /usr/lib, $HOME/... and a bare
	// command name never match, so no exemption list is needed, and a long
	// exemption list is a check that has been argued out of its job.
	numRe = regexp.MustCompile(`\b(experiments|poc)/([0-9]{2,3})-`)
)

// Words that assert quality instead of demonstrating it. They survive review
// because they feel like description. "Simply" and "just" do the real damage:
// they tell a reader who is stuck that the thing they cannot do is easy.
var banned = []string{
	"seamless", "blazing", "effortless", "robust", "powerful", "cutting-edge",
	"state-of-the-art", "world-class", "elegant", "revolutionary",
	"game-changing", "rock-solid", "bulletproof", "lightning-fast",
}

// Docs holds the documentation gate: links resolve, cited paths exist, fenced
// blocks parse, no placeholder survives into one, no banned vocabulary, and
// no page under docs/ that nothing links to.
func Docs(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	ours := t.Ours()
	md := WithExt(ours, ".md")

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

		// 2. repo paths named in backticks resolve, 3. cited evidence is
		//    published, and 4. an experiment or POC referenced by number
		//    exists. The three live in citedPaths because the set of files
		//    they read is wider than the set every other rule here reads.
		if pathsBind {
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

		// 6. banned vocabulary.
		low := strings.ToLower(string(b))
		for _, w := range banned {
			if strings.Contains(low, w) && !exemptFromVocabulary(f) {
				for _, ln := range lines {
					if ln.Fence {
						continue
					}
					if strings.Contains(strings.ToLower(ln.Text), w) {
						r.bad("%s:%d: banned vocabulary %q; replace the adjective with the measurement, or delete it", f, ln.N, w)
						break
					}
				}
			}
		}
	}

	// A vendored methodology page is not ours to rewrite, and every rule above
	// skips it for that reason: docs/methodology/ is outside Ours(), so nothing
	// read those files at all and a path cited in one was never checked. The
	// cited-path rule is the one that still binds there, because a backticked
	// path headed by one of THIS repository's directories is a claim about THIS
	// tree whoever typed the sentence.
	//
	// Measured before arming, over the ten pages at the recorded pin: zero
	// findings. If a re-fetch at a newer pin does fire one, the remedy is a line
	// in docs/methodology/PROVENANCE.md recording what the new page cites, not
	// an edit to a vendored file.
	for _, f := range WithExt(t.Files, ".md") {
		if !strings.HasPrefix(f, "docs/methodology/") || !Vendored(f) {
			continue
		}
		if b := t.Read(f); len(b) > 0 {
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

	// 7b. and the MAP routes to it. ⛔ Linked-from-somewhere and routed-to are
	//     different properties: docs/README.md says it answers which document
	//     answers which question, and a page missing from it is a question the
	//     map silently does not answer. ⚠ Two were missing when this was
	//     written, and one was a shipped component's own page.

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

	// 10. the vendored methodology's unresolved links agree with what
	//     PROVENANCE.md publishes.

	// 11. every commit hash cited under docs/history/ is accounted for.

	// 9. a count quoted in prose agrees with the tree. The defect is a
	//    sentence like "all 31 experiments" that was true when it was
	//    written: nothing edits it when the 32nd lands, and no other check
	//    can see prose disagreeing with a directory listing.

	r.Extra["files"] = len(md)
	r.Extra["links"] = links
	r.Extra["shell_blocks"] = blocks
	return r
}

// quantity is a phrase whose number is derivable from the tree.
var quantities = []struct {
	noun  string
	count func(*Tree) int
}{
	{"experiments", func(t *Tree) int { return countMatching(t, "experiments/", ".sh", true) }},
	{"POCs", func(t *Tree) int { return countDirs(t, "poc/") }},
	{"upstream trees", func(t *Tree) int { return countDirs(t, "references/") }},
}

var quotedRe = regexp.MustCompile(`\*\*(\d+)\*\*\s+(experiments|POCs|upstream trees)\b`)

func countDefects(t *Tree, r *Result) {
	for _, f := range WithExt(t.Ours(), ".md") {
		for _, ln := range Lines(t.Read(f)) {
			if ln.Fence {
				continue
			}
			for _, m := range quotedRe.FindAllStringSubmatch(ln.Text, -1) {
				for _, q := range quantities {
					if q.noun != m[2] {
						continue
					}
					if want := q.count(t); m[1] != itoa(want) {
						r.bad("%s:%d: prose says %s %s; the tree has %d", f, ln.N, m[1], m[2], want)
					}
				}
			}
		}
	}
}

// countMatching counts tracked files directly under prefix with the given
// extension. numbered restricts it to the NN- form, so a shared library like
// experiments/lib.sh is not counted as an experiment.
func countMatching(t *Tree, prefix, ext string, numbered bool) int {
	n := 0
	for _, f := range t.Files {
		if !strings.HasPrefix(f, prefix) || path.Ext(f) != ext {
			continue
		}
		if strings.Contains(strings.TrimPrefix(f, prefix), "/") {
			continue
		}
		if numbered {
			base := path.Base(f)
			if len(base) < 2 || base[0] < '0' || base[0] > '9' {
				continue
			}
		}
		n++
	}
	return n
}

func countDirs(t *Tree, prefix string) int {
	seen := map[string]bool{}
	for _, f := range t.Files {
		if !strings.HasPrefix(f, prefix) {
			continue
		}
		rest := strings.TrimPrefix(f, prefix)
		i := strings.Index(rest, "/")
		if i <= 0 {
			continue
		}
		seen[rest[:i]] = true
	}
	return len(seen)
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

// families are this project's four tools. The tree writes `binary/elfload.c` for
// tool/runtime/binary/elfload.c and `binary/nss.md` for docs/binary/nss.md: the family
// name is the whole address a reader needs and the prefix is noise. The
// shorthand is legitimate, so the check expands it rather than exempting it,
// and a shorthand naming nothing is still a defect.
var families = map[string]bool{"pga": true, "pgb": true, "pgc": true, "pgd": true}

// familyRoots are the two directories a family shorthand may live under. Both
// are tried and exactly one must answer: a token resolving under neither names
// nothing, and one resolving under both is ambiguous and the reader cannot
// tell which was meant.
var familyRoots = []string{"docs/", "tool/runtime/"}

// resolveFamily expands a family shorthand. It returns the paths that answered,
// so the caller can report "nothing" and "both" as the different defects they
// are.
func resolveFamily(t *Tree, tok string) []string {
	var hits []string
	for _, root := range familyRoots {
		if t.TrackedUnder(root + tok) {
			hits = append(hits, root+tok)
		}
	}
	return hits
}

// ownerRepoRe is a backticked two-segment token. That is the shape of an
// upstream owner/repo AND of most of this repository's own paths, which is why
// the shape alone cannot decide; namesUpstream asks who owns the head.
var ownerRepoRe = regexp.MustCompile("`([A-Za-z0-9_.-]+)/[A-Za-z0-9_.-]+`")

// namesUpstream reports whether a line is about somebody else's tree, in which
// case a path on it is theirs and this repository cannot be asked to hold it.
// Two shapes say so: a path into references/, and a backticked owner/repo whose
// head is neither a directory of this repository nor a family shorthand.
//
// The head test has to stay. Matching on the two-segment shape alone treats
// every line citing docs/limitations.md, cmd/check or internal/cfg as somebody
// else's, which is most of what these documents cite, and the cited-path rule
// below then cannot refuse a two-segment path that does not exist.
// docs/history/entries/migration.md carries what that cost.
func namesUpstream(line string, topLevel map[string]bool) bool {
	if strings.Contains(line, "references/") {
		return true
	}
	for _, m := range ownerRepoRe.FindAllStringSubmatch(line, -1) {
		if !topLevel[m[1]] && !families[m[1]] {
			return true
		}
	}
	return false
}

// citedPaths is rules 2, 3 and 4: a repository path named in backticks
// resolves, cited evidence is published rather than merely present on this
// machine, and an experiment or POC named by number exists.
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
			// A trailing slash names a directory. For a family that is the
			// family itself, which legitimately spans both roots, so the
			// ambiguity rule below does not apply to it.
			namesDir := strings.HasSuffix(tok, "/")
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
			case families[head]:
				if strings.HasSuffix(tok, "-") {
					continue
				}
				hits := resolveFamily(t, tok)
				switch {
				case len(hits) == 0:
					r.bad("%s:%d: cited path does not exist -> `%s` (a %s/ shorthand; looked under %s)",
						f, ln.N, tok, head, strings.Join(familyRoots, " and "))
				case len(hits) > 1 && !namesDir:
					r.bad("%s:%d: `%s` is ambiguous: it names %s", f, ln.N, tok, strings.Join(hits, " and "))
				}
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
		for _, m := range numRe.FindAllStringSubmatch(ln.Text, -1) {
			if !globExists(t, m[1]+"/"+m[2]+"-") {
				r.bad("%s:%d: %s/%s- is referenced but no such directory or script exists", f, ln.N, m[1], m[2])
			}
		}
	}
}

// docsMapPage is the map every other page under docs/ has to be reachable from.
const docsMapPage = "docs/README.md"

// citedCommitsFile records where the commit each history page cites actually
// lives.
const citedCommitsFile = "docs/history/CITED-COMMITS.txt"

// hashCiteRe is a backticked hex token of a length a commit id is abbreviated
// to. ⚠ Seven is the shortest this tree has used and twelve the longest; a
// wider class matches a nix store hash fragment and a sha256 prefix, which are
// not commits and have no business in this accounting.
var hashCiteRe = regexp.MustCompile("`([0-9a-f]{7,12})`")

// citedCommits binds the hashes on the history pages to the file that says
// where each one lives.
//
// ⛔ The rule this replaces was false, and it took measuring to see it.
// docs/history/README.md said a commit hash on any page there names a commit in
// the ARCHIVE repository. Thirteen do; six name another project's repository,
// two name a local history the squash replaced, and two are not hashes.
//
// ⛔ It matters more now than it did, because the archive remote is gone. The
// subject and date of an archive commit are in that file and nowhere else, so a
// row nothing cites is a row somebody deleted the last reference to, and a
// citation with no row is a hash nobody can resolve any more.
func citedCommits(t *Tree, r *Result) {
	listed := map[string]string{}
	body := t.Read(citedCommitsFile)
	if len(body) == 0 {
		r.bad("%s is missing; the commit hashes the history pages cite have nothing accounting for them", citedCommitsFile)
		return
	}
	for _, ln := range Lines(body) {
		if strings.HasPrefix(ln.Text, "#") || strings.TrimSpace(ln.Text) == "" {
			continue
		}
		f := strings.SplitN(ln.Text, "	", 3)
		if len(f) < 3 {
			r.bad("%s:%d: want a hash, a class and what it names, tab separated", citedCommitsFile, ln.N)
			continue
		}
		listed[f[0]] = f[1]
	}

	found := map[string]bool{}
	for _, f := range t.Files {
		if !strings.HasPrefix(f, HistoryDir) || path.Ext(f) != ".md" {
			continue
		}
		if f == citedCommitsFile {
			continue
		}
		for _, ln := range Lines(t.Read(f)) {
			for _, m := range hashCiteRe.FindAllStringSubmatch(ln.Text, -1) {
				found[m[1]] = true
				if _, ok := listed[m[1]]; !ok {
					r.bad("%s:%d: cites `%s` and %s does not say where that commit lives; the archive is retired, so an unaccounted hash resolves nowhere",
						f, ln.N, m[1], citedCommitsFile)
				}
			}
		}
	}
	// ⭐ The oracle rows are cited as a DIRECTORY name rather than in
	// backticks, and their tree is in this repository. They are looked for
	// where they actually appear.
	for _, f := range t.Files {
		if !strings.HasPrefix(f, HistoryDir+"oracle/") {
			continue
		}
		rest := strings.TrimPrefix(f, HistoryDir+"oracle/")
		if i := strings.Index(rest, "/"); i > 0 {
			found[rest[:i]] = true
		}
	}
	var stale []string
	for h := range listed {
		if !found[h] {
			stale = append(stale, h)
		}
	}
	sort.Strings(stale)
	for _, h := range stale {
		r.bad("%s lists %s and no page under %s cites it; re-derive the file", citedCommitsFile, h, HistoryDir)
	}
	r.Extra["cited_commits"] = len(listed)
}

// exemptFromVocabulary marks the two places a banned word is not a defect:
// the page that defines the list, which has to be able to name them, and
// material the operator supplied, which is an input to this project and is
// kept as it arrived rather than rewritten to match its rules.
func exemptFromVocabulary(f string) bool {
	return strings.Contains(f, "conventions/prose.md") ||
		strings.HasPrefix(f, "docs/supplied/")
}

// provenancePath records what was vendored under docs/methodology/, from where,
// and which of that material's own links do not resolve in this tree.
const provenancePath = "docs/methodology/PROVENANCE.md"

// methodologyLinks compares the unresolved-link list PROVENANCE.md publishes
// against the one derived from the vendored pages themselves, so that vendoring
// one more page is a one-line diff nobody can forget rather than a paragraph.
//
// The rule is here because the page claimed it for a whole rebuild and nothing
// implemented it. In that time the published block went from true to naming
// eight where the tree derived three: five of the named pages had since been
// written here, so their links resolved and no reader was told. PROVENANCE.md
// sat inside docs/methodology/, which Ours() drops, so no check read the claim
// either.
func methodologyLinks(t *Tree, r *Result) {
	derived := map[string]bool{}
	for _, f := range WithExt(t.Files, ".md") {
		if !strings.HasPrefix(f, "docs/methodology/") || f == provenancePath {
			continue
		}
		for _, ln := range Lines(t.Read(f)) {
			if ln.Fence {
				continue
			}
			for _, m := range linkRe.FindAllStringSubmatch(stripCode(ln.Text), -1) {
				target := m[1]
				if strings.HasPrefix(target, "http") || strings.HasPrefix(target, "#") ||
					strings.HasPrefix(target, "mailto:") {
					continue
				}
				clean := target
				if i := strings.IndexAny(clean, "#?"); i >= 0 {
					clean = clean[:i]
				}
				if clean == "" {
					continue
				}
				if !t.Exists(path.Join("docs/methodology", clean)) {
					derived[target] = true
				}
			}
		}
	}

	body := t.Read(provenancePath)
	if len(body) == 0 {
		r.bad("%s: the page recording what is vendored under docs/methodology/ is missing", provenancePath)
		return
	}
	published, found := publishedList(Lines(body))
	if !found {
		r.bad("%s: no indented block publishing the unresolved-link list; the page says this check compares one", provenancePath)
		return
	}
	for _, k := range sortedKeys(derived) {
		if !published[k] {
			r.bad("%s: a vendored methodology page links %s and it does not resolve here; the published list omits it", provenancePath, k)
		}
	}
	for _, k := range sortedKeys(published) {
		if !derived[k] {
			r.bad("%s: the published list names %s, which resolves now; the list is stale", provenancePath, k)
		}
	}
}

// publishedList reads the one indented block outside a fence. Four spaces is
// the whole marker: the derivation command above it is fenced, so the fence
// state separates the two without either needing a label.
func publishedList(lines []Line) (map[string]bool, bool) {
	out := map[string]bool{}
	found := false
	for _, ln := range lines {
		if ln.Fence || !strings.HasPrefix(ln.Text, "    ") {
			continue
		}
		for _, tok := range strings.Fields(ln.Text) {
			found = true
			out[tok] = true
		}
	}
	return out, found
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
