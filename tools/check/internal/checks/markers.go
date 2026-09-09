// SPDX-License-Identifier: 0BSD

package checks

import (
	"sort"
	"strings"
	"unicode"
)

// The whole allowlist. Three prose markers and two status glyphs, and there
// is no third tier: an explicit five-character list is something a check can
// hold, and a rule phrased as a principle ("no anthropomorphic symbols") is
// one that moves every time somebody argues for one more glyph.
const (
	Stop = '⛔' // a rule already broken, or one whose violation is unrecoverable
	Star = '⭐' // reach for this first
	Warn = '⚠' // a trap: it works until it does not, and the failure is quiet
	Pass = '✅' // machine output and result tables only
	Fail = '❌' // the same
)

var allowed = map[rune]bool{Stop: true, Star: true, Warn: true, Pass: true, Fail: true}

// A marker never reports a result and a status glyph never carries a rule,
// but only the density of the prose markers is counted: a result table is
// allowed to be a column of glyphs.
var prose = map[rune]bool{Stop: true, Star: true, Warn: true}

// MarkerCeiling is a refusal of the unreadable rather than a target. A page
// at 25 is already dense.
const MarkerCeiling = 30

// UnicodeFixtures carry non-ASCII on purpose: they are round-trip data an
// assertion reads. Nothing in this tree is one yet, and the map is here so the
// first one is exempted BY NAME with its reason rather than by widening a rule.
//
// Punctuation is never data: an em dash in a comment is not a fixture, and
// listing a file for one is an exemption that hides real findings.
var UnicodeFixtures = map[string]string{}

type markerFile struct {
	path    string
	markers int
	lines   int
	density float64
}

// Markers holds the five-character allowlist, the no-stacking rule and the
// density ceiling over every tracked text file this project writes.
func Markers(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	var files []markerFile
	total, worstD := 0, 0.0

	for _, f := range t.Ours() {
		if !TextFile(f) {
			continue
		}
		b := t.Read(f)
		if len(b) == 0 {
			continue
		}
		fixture := UnicodeFixtures[f] != ""
		lines := Lines(b)
		// ⚠ A UTF-8 BYTE ORDER MARK IS NOT A STRAY CHARACTER. Every .ps1 in
		// this tree carries one on purpose, because Windows PowerShell 5.1
		// decodes a BOM-less file as the system ANSI code page. Counting it as
		// a finding would report the fix as the defect.
		if len(lines) > 0 {
			lines[0].Text = strings.TrimPrefix(lines[0].Text, "\ufeff")
		}
		markdown := strings.HasSuffix(strings.ToLower(f), ".md")
		fence := false
		nonblank, count := 0, 0
		type stray struct {
			r rune
			n int
		}
		var strays []stray
		for _, ln := range lines {
			if strings.TrimSpace(ln.Text) != "" {
				nonblank++
			}
			// A specimen inside a fenced block or a code span is permitted:
			// a page that bans a character cannot otherwise name it.
			if markdown && fenceLine(ln.Text) {
				fence = !fence
				continue
			}
			if markdown && fence {
				continue
			}
			bare := stripCode(ln.Text)
			var prev rune
			for _, c := range bare {
				if c < unicode.MaxASCII {
					prev = c
					continue
				}
				switch {
				case allowed[c]:
					if prose[c] {
						count++
					}
					if c == prev {
						r.bad("%s:%d: stacked marker %q; escalating a marker is how the vocabulary stops meaning anything", f, ln.N, string([]rune{c, c}))
					}
				case fixture:
					// round-trip data, exempt by name
				default:
					strays = append(strays, stray{c, ln.N})
				}
				prev = c
			}
		}
		// Three findings in full, then a roll-up. The roll-up names every
		// distinct codepoint rather than only a count: a summary that hides
		// which character was found is a check whose output cannot be acted
		// on, and it hides a newly introduced one behind a backlog.
		for i, s := range strays {
			if i == 3 {
				break
			}
			r.bad("%s:%d: %q (U+%04X) is not one of the five allowed characters", f, s.n, string(s.r), s.r)
		}
		if len(strays) > 3 {
			seen := map[rune]int{}
			var order []rune
			for _, s := range strays[3:] {
				if seen[s.r] == 0 {
					order = append(order, s.r)
				}
				seen[s.r]++
			}
			var parts []string
			for _, c := range order {
				parts = append(parts, sprintf("U+%04X x%d", c, seen[c]))
			}
			r.bad("%s: and %d more outside the allowlist: %s", f, len(strays)-3, strings.Join(parts, ", "))
		}
		if count == 0 {
			continue
		}
		// ⛔ INTEGER DIVISION, because that is the rule this repository has
		// enforced from the start and a port is not the place to tighten one.
		// A page at 30.8 was inside the ceiling yesterday; making it a finding
		// today would be this change failing a tree nobody had touched.
		lines99 := nonblank
		if lines99 < 1 {
			lines99 = 1
		}
		d := float64(count * 100 / lines99)
		total += count
		if d > worstD {
			worstD = d
		}
		files = append(files, markerFile{f, count, lines99, d})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].density > files[j].density })
	for _, f := range files {
		if f.density > MarkerCeiling {
			r.bad("%s: %.1f markers per 100 non-blank lines, over the ceiling of %d (%d markers, %d lines)",
				f.path, f.density, MarkerCeiling, f.markers, f.lines)
		}
	}
	r.Extra["files"] = len(files)
	r.Extra["markers"] = total
	r.Extra["ceiling"] = MarkerCeiling
	r.Extra["worst_density"] = round1(worstD)
	return r
}

// stripCode removes backtick spans so a specimen does not count as a marker.
func stripCode(s string) string {
	if !strings.Contains(s, "`") {
		return s
	}
	var b strings.Builder
	in := false
	for _, c := range s {
		if c == '`' {
			in = !in
			continue
		}
		if !in {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// fenceLine reports whether a markdown line opens or closes a fenced block.
func fenceLine(s string) bool {
	return strings.HasPrefix(strings.TrimLeft(s, " 	"), "```")
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
