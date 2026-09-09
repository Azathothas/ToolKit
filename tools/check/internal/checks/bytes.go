// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ControlBytes refuses a literal control byte in a tracked text file. The
// runtime value is identical whether the byte is written as itself or as an
// escape, so only reviewability is ever at stake, which is exactly why it
// survives so long unnoticed: grep calls the file binary and skips it, and
// git diff prints "Binary files differ" so a review of the file shows no diff.
func ControlBytes(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	n := 0
	for _, f := range t.Ours() {
		if !TextFile(f) {
			continue
		}
		b := t.Read(f)
		if len(b) == 0 {
			continue
		}
		n++
		line := 1
		for i, c := range b {
			if c == '\n' {
				line++
				continue
			}
			if c == '\t' || c == '\r' || c >= 0x20 {
				continue
			}
			r.bad("%s:%d: literal control byte 0x%02x at offset %d; write it as an escape", f, line, c, i)
			break
		}
	}
	r.Extra["files"] = n
	return r
}

var sentenceRe = regexp.MustCompile(`[^.!?\n]{40,}`)

// OneHome refuses a sentence of twelve words or more that appears twice: in
// two documents, or twice inside one. A value in two places with no check
// between them drifts, and the copy a reader trusts is the wrong one.
//
// The within-a-page half was added after a 1,533-character section turned out
// to be present verbatim twice in the same file. Comparing only across files
// could not see it, and neither could a reader: the two copies were ninety
// lines apart, and each read correctly where it stood.
//
// Two exemptions, both narrow. The entry-point routers are exempt from each
// other, because each states the absolutes in full for a session that may be
// handed exactly one of them. And docs/history/ is exempt entirely, because a
// superseded page states things the live pages now state differently, which is
// the point of it.
func OneHome(t *Tree) Result {
	const minWords = 12
	r := Result{Extra: map[string]any{"min_words": minWords}}

	// The routers state the absolutes in full, because a session may be handed
	// exactly one of them. A sentence in two routers and nowhere else is
	// therefore not a second home; the same sentence anywhere else is.
	routers := map[string]bool{"docs/AGENTS.md": true}

	where := map[string][]string{}
	n := 0
	for _, f := range WithExt(t.Ours(), ".md") {
		if History(f) {
			continue
		}
		n++
		// ⛔ THE WHOLE FILE IS ONE BUFFER, not a line at a time. A sentence
		// wrapped across two lines is one sentence, and a per-line reading
		// finds neither half in the other document.
		var buf strings.Builder
		fence := false
		for _, ln := range Lines(t.Read(f)) {
			text := ln.Text
			trimmed := strings.TrimLeft(text, " 	")
			if strings.HasPrefix(trimmed, "```") {
				fence = !fence
				continue
			}
			if fence {
				continue
			}
			// A table row is not a sentence, and neither is a heading.
			if strings.HasPrefix(trimmed, "|") || strings.HasPrefix(trimmed, "#") {
				continue
			}
			buf.WriteString(" ")
			buf.WriteString(stripInline(text))
		}
		// One entry per file per sentence: a section repeated inside one
		// document is a different finding from a fact with two homes, and this
		// check is the second one.
		seen := map[string]bool{}
		for _, raw := range sentenceSplit.Split(buf.String(), -1) {
			s := normalise(raw)
			if len(strings.Fields(s)) < minWords || seen[s] {
				continue
			}
			seen[s] = true
			where[s] = append(where[s], f)
		}
	}

	var dupes []string
	for s, files := range where {
		if len(files) < 2 {
			continue
		}
		all := true
		for _, f := range files {
			if !routers[f] {
				all = false
				break
			}
		}
		if all {
			continue
		}
		sort.Strings(files)
		dupes = append(dupes, sprintf("%s: the same sentence appears in %s (%.60s...)", files[0], strings.Join(files[1:], ", "), s))
	}
	sort.Strings(dupes)
	for _, d := range dupes {
		r.bad("%s", d)
	}
	r.Extra["files"] = n
	return r
}

// sentenceSplit ends a sentence at a full stop, colon, exclamation or question
// mark followed by space. ⚠ The trailing space matters: without it every
// version number and every `foo.bar` splits a sentence in two.
var sentenceSplit = regexp.MustCompile(`[.:!?]+[ 	]+`)

// stripInline removes the markup that is not prose: code spans, and the link
// syntax around a label. What is left is what a reader reads.
func stripInline(s string) string {
	var b strings.Builder
	in := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '`':
			in = !in
			b.WriteByte(' ')
		case in:
		case s[i] == '[' || s[i] == ']':
			b.WriteByte(' ')
		case s[i] == '(' && i > 0 && s[i-1] == ']':
			b.WriteByte(' ')
		default:
			b.WriteByte(s[i])
		}
	}
	out := b.String()
	return linkTargetRe.ReplaceAllString(out, " ")
}

// linkTargetRe is the destination of a markdown link, which is a path rather
// than prose and would otherwise make two pages citing one file look alike.
var linkTargetRe = regexp.MustCompile(`\([^)]*\)`)

// normalise reduces a sentence to lowercase words, so two pages that differ
// only in punctuation or emphasis are still saying the same thing.
func normalise(s string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteRune(c)
		default:
			b.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// Placeholders refuses a template marker that survived into a real file.
//
// The pattern is assembled from pieces so that no line of this file contains
// any of the literals it looks for. The first version did, and reported
// itself: the same class as a page about escape sequences containing the byte
// it warns about.
// placeholderRe is a double-brace placeholder, and the two exclusions under it
// are what keep it from firing on correct files.
//
// EXCLUDED: `${{ ... }}` is GitHub Actions expression syntax, and `{{.Field}}`,
// `{{json .X}}`, `{{range .X}}` and `{{end}}` are Go templates -- every
// `podman info --format` and `docker inspect --format` string in this tree has
// that shape. A rule that fires on those fires on every correct workflow and
// every correct format string, and a rule that fires on correct files gets
// switched off within a week.
//
// The exclusion is "a dot or a lowercase letter after the braces", which cannot
// collide with a placeholder: every one this template ships begins with an
// UPPERCASE letter.
var placeholderRe = regexp.MustCompile(`\{\{[^}]*\}\}` + `|TODO` + `\(FILL` + `|XXX` + `-REPLACE`)

// placeholderExempt are the files that hold placeholders as their job.
//
// TODO/ENTRY.md is the shape an entry is written from, so holding them is its
// whole job; a check that failed on it would fail on a correct tree.
// The two shell implementations are exempt because each contains the patterns
// it looks for, and exempting only one is how the twins came to disagree.
//
// IT NAMES FILES, NOT DIRECTORIES. A directory-shaped exemption grants itself
// to whatever lands there next.
var placeholderExempt = map[string]bool{
	"TODO/ENTRY.md":                         true,
	"scripts/common/check-placeholders.sh":  true,
	"scripts/common/check-placeholders.ps1": true,
	"tools/check/internal/checks/bytes.go":  true,
}

// goTemplateOrExpression reports whether a match is one of the two shapes that
// are not placeholders.
func goTemplateOrExpression(line, match string, at int) bool {
	if at > 0 && line[at-1] == '$' {
		return true
	}
	inner := strings.TrimPrefix(match, "{{")
	inner = strings.TrimLeft(inner, " ")
	if inner == "" {
		return false
	}
	c := inner[0]
	return c == '.' || (c >= 'a' && c <= 'z')
}

func Placeholders(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	re := placeholderRe
	n := 0
	for _, f := range t.Ours() {
		if !TextFile(f) || placeholderExempt[f] {
			continue
		}
		n++
		for _, ln := range Lines(t.Read(f)) {
			if ln.Fence {
				continue
			}
			for _, loc := range re.FindAllStringIndex(ln.Text, -1) {
				m := ln.Text[loc[0]:loc[1]]
				if goTemplateOrExpression(ln.Text, m, loc[0]) {
					continue
				}
				r.bad("%s:%d: unfilled placeholder %s", f, ln.N, m)
			}
		}
	}
	r.Extra["files_scanned"] = n
	return r
}

// CRuntime asserts every carried C source at least parses. These files are
// embedded as strings and compiled by cc at build or bundle time, not by the
// Go toolchain, so `go build` succeeds on a C file that cannot compile. Worse,
// the bundler catches a compile failure and falls back to a shell AppRun,
// which runs under the host's interpreter and loads the host's libc: the exact
// thing the C exists to avoid, shipped as a warning in a build log.
//
// -c to a throwaway object, not a link: these have wildly different link
// requirements and parsing is the property that breaks.
func CRuntime(t *Tree) Result {
	r := Result{Extra: map[string]any{}}

	// ⛔ Run FIRST and unconditionally, because it needs no compiler. Compiling
	// the sources is only half the job: they are named from Go by a
	// family-relative string and handed to cc at build time, so a wrong name
	// builds green and fails on first use.
	//
	// It did. The move that grouped these by the tool that compiles them left
	// two call sites composing runtime-src/appimage/appimage/storefix.c from a sibling's
	// directory; cc was asked for a file that was not there, and every bundle
	// built afterwards shipped with no store-path interposer while the build
	// log said `cc exited 1` and read like a broken toolchain.
	r.Extra["references"] = runtimeRefs(t, &r)

	cc, args := cCompiler()
	if cc == "" {
		r.Extra["compiler"] = ""
		return r
	}
	r.Extra["compiler"] = cc
	tmp, err := os.MkdirTemp("", "cparse")
	if err != nil {
		r.bad("cannot create a scratch directory: %v", err)
		return r
	}
	defer os.RemoveAll(tmp)

	var srcs []string
	for _, f := range t.Ours() {
		if strings.HasPrefix(f, "tool/runtime/") && path.Ext(f) == ".c" {
			srcs = append(srcs, f)
		}
	}
	sort.Strings(srcs)
	for _, f := range srcs {
		obj := filepath.Join(tmp, strings.ReplaceAll(f, "/", "_")+".o")
		argv := append(append([]string{}, args...), "-c", "-o", obj,
			`-DPGT_APPIMAGE_APPRUN_DEFAULT="x"`, filepath.Join(t.Root, filepath.FromSlash(f)))
		cmd := exec.Command(cc, argv...)
		cmd.Dir = filepath.Join(t.Root, filepath.FromSlash(path.Dir(f)))
		if out, err := cmd.CombinedOutput(); err != nil {
			r.bad("%s does not parse: %s", f, firstError(string(out)))
		}
	}
	r.Extra["sources"] = len(srcs)
	return r
}

// cCompiler picks something that can parse Linux C on this host. A native cc
// is preferred; where there is none, zig ships a cross toolchain with glibc
// headers, which is what makes this check runnable on Windows at all.
func cCompiler() (string, []string) {
	if p, err := exec.LookPath("cc"); err == nil && !windowsOnly(p) {
		return p, nil
	}
	if p, err := exec.LookPath("zig"); err == nil {
		return p, []string{"cc", "-target", "x86_64-linux-gnu"}
	}
	if p, err := exec.LookPath("gcc"); err == nil && !windowsOnly(p) {
		return p, nil
	}
	return "", nil
}

// A mingw gcc on Windows has no glibc headers, so it reports every source as
// broken. Reporting that as a failure is worse than saying the check could
// not run.
func windowsOnly(p string) bool {
	return strings.Contains(strings.ToLower(p), "mingw") || strings.HasSuffix(strings.ToLower(p), ".exe") && os.Getenv("MSYSTEM") == ""
}

func firstError(out string) string {
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, "error:") || strings.Contains(ln, "fatal") {
			return strings.TrimSpace(ln)
		}
	}
	return firstLine(strings.TrimSpace(out))
}

// runtimeRefs checks every Go string literal naming a carried runtime source
// against what tool/runtime/ actually holds, and returns how many it read.
//
// ⚠ Comment lines are skipped, so this stays a rule about CALL SITES. A
// comment naming a source is prose and the prose rules bind it; a compile
// command naming one is a claim the build makes.
func runtimeRefs(t *Tree, r *Result) int {
	const root = "tool/runtime/"
	carried := map[string]bool{}
	families := map[string]bool{}
	for _, f := range t.Files {
		if !strings.HasPrefix(f, root) {
			continue
		}
		rel := strings.TrimPrefix(f, root)
		carried[rel] = true
		if i := strings.Index(rel, "/"); i > 0 {
			families[rel[:i]] = true
		}
	}
	if len(families) == 0 {
		r.bad("%s holds no family directories; the reference rule would match nothing", root)
		return 0
	}
	// The families are derived from the tree, so adding one needs no change
	// here and a rename cannot leave the pattern behind.
	var names []string
	for f := range families {
		names = append(names, regexp.QuoteMeta(f))
	}
	sort.Strings(names)
	re := regexp.MustCompile(`"((?:` + strings.Join(names, "|") + `)/[A-Za-z0-9_.\-]+\.[ch])"`)

	seen := 0
	for _, f := range WithExt(t.Ours(), ".go") {
		for _, ln := range Lines(t.Read(f)) {
			if isComment(ln.Text) {
				continue
			}
			for _, m := range re.FindAllStringSubmatch(ln.Text, -1) {
				seen++
				if !carried[m[1]] {
					r.bad("%s:%d: names the carried runtime source %q, and %s%s is not in the tree",
						f, ln.N, m[1], root, m[1])
				}
			}
		}
	}
	if seen == 0 {
		r.bad("no Go source names a carried runtime source; the reference rule read nothing")
	}
	return seen
}
