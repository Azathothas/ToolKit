// SPDX-License-Identifier: 0BSD

package license

import (
	"strings"
	"testing"
)

// TestBareStyleDoesNotRewriteTheWarrantyClause is the case for the defect that
// actually shipped.
//
// ⛔ 0BSD uses the bare word AUTHOR as its placeholder, and the same word
// appears twice more in its warranty clause. A global replace turned a licence
// into a document that disclaims warranties on behalf of a named person, and
// the placeholder check reported success over it, because a placeholder had not
// survived: it had been over-applied.
func TestBareStyleDoesNotRewriteTheWarrantyClause(t *testing.T) {
	src := strings.Join([]string{
		"Copyright (C) YEAR by AUTHOR EMAIL",
		"",
		`THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES`,
		"WITH REGARD TO THIS SOFTWARE. IN NO EVENT SHALL THE AUTHOR BE LIABLE",
		"FOR ANY SPECIAL DAMAGES IN ANY YEAR.",
	}, "\n")

	got := Fill(src, StyleBare, "Some Name", "2026")

	if want := "Copyright (C) 2026 by Some Name"; strings.Split(got, "\n")[0] != want {
		t.Errorf("first line is %q, want %q", strings.Split(got, "\n")[0], want)
	}
	if n := strings.Count(got, "Some Name"); n != 1 {
		t.Errorf("the holder appears %d times, want 1: it belongs in the copyright line and nowhere else", n)
	}
	if !strings.Contains(got, "THE AUTHOR DISCLAIMS ALL WARRANTIES") {
		t.Error("the warranty clause was rewritten. That is the defect this style rule exists to prevent")
	}
	if !strings.Contains(got, "SHALL THE AUTHOR BE LIABLE") {
		t.Error("the liability clause was rewritten")
	}
	if !strings.Contains(got, "IN ANY YEAR.") {
		t.Error("the word YEAR was replaced outside the first line")
	}

	// And the over-replacement guard catches it when the rule is wrong.
	if len(OverReplaced(src, got)) != 0 {
		t.Errorf("a correct fill was reported as over-replaced: %v", OverReplaced(src, got))
	}
	global := strings.NewReplacer("YEAR", "2026", "AUTHOR", "Some Name").Replace(src)
	over := OverReplaced(src, global)
	if len(over) == 0 {
		t.Fatal("a global replace over the warranty clause was NOT reported. This is the guard that failed once already")
	}
}

func TestSurvivingPlaceholders(t *testing.T) {
	for _, p := range placeholders {
		text := "line one\nnotice with " + p + " in it\nline three"
		got := SurvivingPlaceholders(text)
		if len(got) != 1 || got[0].Line != 2 {
			t.Errorf("%q: got %v, want one finding on line 2", p, got)
		}
	}
	if got := SurvivingPlaceholders("nothing to see"); len(got) != 0 {
		t.Errorf("a clean text reported %v", got)
	}
}

func TestAngleAndSquareStyles(t *testing.T) {
	angle := Fill("Copyright <year> <copyright holders> and <owner>", StyleAngle, "Some Name", "2026")
	if angle != "Copyright 2026 Some Name and Some Name" {
		t.Errorf("angle: %q", angle)
	}
	square := Fill("Copyright [yyyy] [name of copyright owner]", StyleSquare, "Some Name", "2026")
	if square != "Copyright 2026 Some Name" {
		t.Errorf("square: %q", square)
	}
	// ⛔ A style with nothing to fill must copy the text UNCHANGED. The three
	// FSF licences and the two dedications depend on it.
	none := "Copyright (C) 2007 Free Software Foundation, Inc. <https://fsf.org/>"
	if Fill(none, StyleNone, "Some Name", "2026") != none {
		t.Error("the none style edited the text")
	}
}

// TestEveryTableRowHasAStyleTheFillerKnows stops a row being added with a style
// nothing implements, which would silently fall through to copying the text.
func TestEveryTableRowHasAStyleTheFillerKnows(t *testing.T) {
	known := map[Style]bool{
		StyleAngle: true, StyleSquare: true, StyleBare: true,
		StyleNone: true, StyleFSF: true, StyleInstance: true,
	}
	seen := map[string]bool{}
	for _, e := range Table() {
		if !known[e.Style] {
			t.Errorf("%s has style %q, which Fill does not implement", e.ID, e.Style)
		}
		if seen[e.ID] {
			t.Errorf("%s appears twice in the table", e.ID)
		}
		seen[e.ID] = true
	}
	if len(Table()) != 12 {
		t.Errorf("the table has %d rows and the header says twelve", len(Table()))
	}
	// ⛔ Five of the twelve are refusals or no-ops. If that count moves, a
	// licence has been reclassified and the reason belongs in the note.
	refusing := 0
	for _, e := range Table() {
		if e.Style == StyleFSF || e.Style == StyleInstance {
			refusing++
			if e.Note == "" {
				t.Errorf("%s is refused and carries no note saying why", e.ID)
			}
		}
	}
	if refusing != 4 {
		t.Errorf("%d licences are refused, want 4: three FSF texts and one instance", refusing)
	}
}
