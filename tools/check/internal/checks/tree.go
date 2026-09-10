// Package checks holds the repository gates: one tree walk, one binary, no
// POSIX layer. Each check is the sole authority on its own subject.
//
// SPDX-License-Identifier: 0BSD
package checks

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Result is what every check returns. Problems is the number of findings;
// Detail carries one line each. Extra holds the counters a check publishes in
// its JSON, keyed by the field name that JSON uses.
type Result struct {
	Problems int
	Detail   []string
	Extra    map[string]any
}

func (r *Result) bad(format string, a ...any) {
	r.Problems++
	r.Detail = append(r.Detail, sprintf(format, a...))
}

// Tree is the tracked file set, read once and shared by every check.
//
// ⛔ Reading git's own answer rather than walking the filesystem is the whole
// design, and it was paid for. A check that asks the DISK agrees with whoever
// ran it last and disagrees with a fresh clone, so it goes green for the
// author of a change and red for everybody else - which is the worst place a
// disagreement can sit, because the person who can fix it is the one being
// told nothing is wrong.
type Tree struct {
	Root  string
	Files []string

	mu    sync.Mutex
	cache map[string][]byte
}

// notOurs are trees this repository carries but does not write. A gate that
// reports their internal state teaches a reader to skip its output.
//
// LICENSES/ holds the SPDX texts verbatim. They carry curly quotes and a
// copyright sign, and editing one to satisfy a character rule would make it a
// licence text nobody published.
//
// ⚠ ONE ENTRY, AND ADDING A SECOND IS A DECISION. An exemption list is the
// cheapest place for a real finding to hide, so a tree goes here only when
// editing it would destroy the property that makes it worth carrying.
var notOurs = []string{"LICENSES/"}

// Vendored reports whether p belongs to a tree this repository does not write.
func Vendored(p string) bool {
	for _, v := range notOurs {
		if strings.HasPrefix(p, v) {
			return true
		}
	}
	return false
}

// HistoryDir holds superseded wording, kept in its original words.
const HistoryDir = "docs/HISTORY/"

// History reports whether p is a superseded page. Such a page is ours and the
// character rules apply to it like anywhere else, but three checks do not:
//
//   - one fact one home, because it states on purpose what the live pages now
//     state differently;
//   - cited paths and links, because it describes the tree as it was, and a
//     file named there may since have been renamed or retired.
//
// Editing those citations to match today would make the page a new document
// about an old one, which is the thing the directory exists to prevent.
func History(p string) bool { return strings.HasPrefix(p, HistoryDir) }

// Ours is the tracked set minus the vendored trees.
func (t *Tree) Ours() []string {
	out := make([]string, 0, len(t.Files))
	for _, f := range t.Files {
		if !Vendored(f) {
			out = append(out, f)
		}
	}
	return out
}

// WithExt filters a list to the given extensions, which carry their dot.
func WithExt(files []string, ext ...string) []string {
	var out []string
	for _, f := range files {
		e := strings.ToLower(path.Ext(f))
		for _, want := range ext {
			if e == want {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// Load reads the tracked file list from git. It returns an error when this is
// not a git tree, which the caller reports as exit 2: a gate that cannot see
// the tree has not run, and that is neither a pass nor a failure.
func Load(root string) (*Tree, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	// The repository root, not the directory this was invoked from. Without it
	// a run from a subdirectory silently scopes itself to that subtree and
	// reports clean over everything else: the scope of a guard must not depend
	// on who called it.
	top, err := exec.Command("git", "-C", abs, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, err
	}
	abs = strings.TrimSpace(string(top))

	t := &Tree{Root: abs, cache: map[string][]byte{}}
	seen := map[string]bool{}
	// ⛔ TRACKED **PLUS UNTRACKED-BUT-NOT-IGNORED**, and the second list is not
	// optional. `git ls-files` alone cannot see a file that has never been
	// staged, which is exactly when a new file is most likely to carry a
	// credential and exactly what the next `git add -A` would take. Ignored
	// files stay out: they are ignored on purpose.
	for _, args := range [][]string{
		{"-C", abs, "ls-files", "-z"},
		{"-C", abs, "ls-files", "--others", "--exclude-standard", "-z"},
	} {
		out, err := exec.Command("git", args...).Output()
		if err != nil {
			return nil, err
		}
		for _, b := range bytes.Split(out, []byte{0}) {
			if len(b) == 0 {
				continue
			}
			p := string(b)
			if !seen[p] {
				seen[p] = true
				t.Files = append(t.Files, p)
			}
		}
	}
	sort.Strings(t.Files)
	return t, nil
}

// Read returns a tracked file's bytes from the working tree, memoised. A file
// git lists but the working tree lacks reads as empty rather than as an error,
// so one deleted-but-staged path cannot take a whole gate down.
func (t *Tree) Read(p string) []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	if b, ok := t.cache[p]; ok {
		return b
	}
	b, err := os.ReadFile(filepath.Join(t.Root, filepath.FromSlash(p)))
	if err != nil {
		b = nil
	}
	t.cache[p] = b
	return b
}

// TopLevel is the set of first path segments the tracked tree has. It decides
// whether a token that looks like a path is a claim about THIS repository:
// a nix store path, /usr/lib, a module import path and a bare command name all
// have a head this set does not hold, so no exemption list is needed.
//
// ⛔ One definition, because two checks ask the question. check docs asks it of
// a document and check source asks it of a comment, and a token that is a
// citation in one file cannot be something else in the other.
func (t *Tree) TopLevel() map[string]bool {
	top := map[string]bool{}
	for _, f := range t.Files {
		if i := strings.Index(f, "/"); i > 0 {
			top[f[:i]] = true
		}
	}
	return top
}

// Exists reports whether a repository-relative path is on disk.
func (t *Tree) Exists(p string) bool {
	_, err := os.Stat(filepath.Join(t.Root, filepath.FromSlash(p)))
	return err == nil
}

// Tracked reports whether a repository-relative path is in the index. A file
// merely present on this machine is not published, so a document citing one
// is citing something a reader cannot fetch.
func (t *Tree) Tracked(p string) bool {
	for _, f := range t.Files {
		if f == p {
			return true
		}
	}
	return false
}

// Line is one line of a file with its 1-based number and whether it sits
// inside a fenced block. Almost every prose check needs the fence state: a
// sed script in an example is not a link, and a page that bans a character
// has to be able to show a reader which character it means.
type Line struct {
	N     int
	Text  string
	Fence bool
}

// Lines splits a file into numbered lines with fence state resolved.
func Lines(b []byte) []Line {
	var out []Line
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
	fence := false
	n := 0
	for sc.Scan() {
		n++
		s := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(s), "```") {
			out = append(out, Line{N: n, Text: s, Fence: true})
			fence = !fence
			continue
		}
		out = append(out, Line{N: n, Text: s, Fence: fence})
	}
	return out
}

// TextFile reports whether a path is one the character and byte rules apply
// to. The extension list is the scope; widening it is a deliberate change,
// because a rule that only ever looked at documents leaves every script it
// ships unchecked.
var textExt = map[string]bool{
	".c": true, ".h": true, ".cpp": true, ".hpp": true, ".go": true,
	".rs": true, ".py": true, ".sh": true, ".ps1": true, ".md": true,
	".txt": true, ".json": true, ".yaml": true, ".yml": true, ".toml": true,
	".cfg": true, ".ini": true, ".conf": true, ".sql": true, ".css": true,
	".html": true, ".js": true, ".ts": true,
}

// TextFile reports whether p is in scope for the character rules.
func TextFile(p string) bool {
	if textExt[strings.ToLower(path.Ext(p))] {
		return true
	}
	// Extensionless files that are still text and still ours.
	switch path.Base(p) {
	case "Makefile", "LICENSE", "docs/AGENTS.md":
		return true
	}
	return false
}

// TrackedUnder reports whether p is a tracked file or a directory containing
// one. A cited evidence directory is published when its contents are, and
// asking whether the directory itself is a tracked file always answers no.
func (t *Tree) TrackedUnder(p string) bool {
	prefix := p + "/"
	for _, f := range t.Files {
		if f == p || strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}
