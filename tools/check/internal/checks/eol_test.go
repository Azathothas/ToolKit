// SPDX-License-Identifier: 0BSD

package checks

import "testing"

// ⛔ THE ROW THE OLD PARSE COULD NOT READ. `git ls-files --eol` writes the
// attribute column with a SPACE in it, so the whole point of this case is the
// `eol=crlf` half surviving. A parse that splits the row on whitespace and
// takes the field beginning `attr/` keeps `text` and drops the rest, which is
// what made the check unable to fail on any text file in the tree. TOOL-20.
func TestEOLRowKeepsTheWholeAttribute(t *testing.T) {
	cases := []struct {
		name                  string
		head                  string
		index, worktree, attr string
		ok                    bool
	}{
		{
			name:  "crlf attribute survives the space in it",
			head:  "i/lf    w/crlf  attr/text eol=crlf    ",
			index: "i/lf", worktree: "w/crlf", attr: "text eol=crlf", ok: true,
		},
		{
			name:  "lf attribute",
			head:  "i/lf    w/lf    attr/text eol=lf      ",
			index: "i/lf", worktree: "w/lf", attr: "text eol=lf", ok: true,
		},
		{
			name:  "a working tree that disagrees is still parsed",
			head:  "i/lf    w/lf    attr/text eol=crlf    ",
			index: "i/lf", worktree: "w/lf", attr: "text eol=crlf", ok: true,
		},
		{
			name:  "declared binary",
			head:  "i/none  w/none  attr/-text           ",
			index: "i/none", worktree: "w/none", attr: "-text", ok: true,
		},
		{
			name: "no attribute column at all",
			head: "i/lf    w/lf",
			ok:   false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			index, worktree, attr, ok := parseEOLRow(c.head)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if index != c.index || worktree != c.worktree || attr != c.attr {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)",
					index, worktree, attr, c.index, c.worktree, c.attr)
			}
		})
	}
}

// ⛔ THE COMPARISON THE OLD CHECK MADE WAS A TAUTOLOGY. It read the INDEX
// column and expected `i/crlf` for an `eol=crlf` file, and git normalises a
// text file to LF in the index by definition, so no text file could ever
// disagree with what it expected. This asserts the rule against the column that
// CAN disagree, which is the working tree.
func TestWantedWorktreeEndingFollowsTheAttribute(t *testing.T) {
	cases := []struct {
		attr string
		want string
	}{
		{"text eol=crlf", "w/crlf"},
		{"text eol=lf", "w/lf"},
		{"text", "w/lf"},
	}
	for _, c := range cases {
		if got := wantedWorktreeEnding(c.attr); got != c.want {
			t.Fatalf("attr %q resolved %q, want %q", c.attr, got, c.want)
		}
	}
}
