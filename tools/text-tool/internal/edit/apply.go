// SPDX-License-Identifier: 0BSD

package edit

import (
	"bytes"
	"fmt"
)

// apply carries out the one operation and returns the new bytes, how many places
// it found, and which lines it touched.
//
// ⛔ IT NEVER WRITES. The caller decides that, after checking the count against
// what the caller said they believed. Separating the two is what makes --expect
// able to refuse.
func (o *options) apply(before []byte, eol string) ([]byte, int, []int, error) {
	payload := toEOL(o.payload, eol)

	switch o.mode {
	case "write":
		return payload, 1, nil, nil
	case "eol":
		return o.applyEOL(before)
	case "append":
		// ⚠ A FILE WITH NO TRAILING NEWLINE WOULD HAVE ITS LAST LINE JOINED.
		// Saying so is not enough: this inserts the ending the file already uses.
		out := append([]byte{}, before...)
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, lineEnding(eol)...)
		}
		return append(out, payload...), 1, nil, nil
	}

	switch {
	case o.haveFind:
		return o.applyReplace(before, payload)
	case o.haveLine:
		return o.applyLine(before, payload, eol)
	case o.haveAfter, o.haveBefore:
		return o.applyInsert(before, payload, eol)
	case o.haveInsertFind:
		return o.applyInsertAt(before, payload, eol)
	case o.haveDelete:
		return o.applyDelete(before)
	case o.haveBetween:
		return o.applyBetween(before, payload, eol)
	}
	return before, 0, nil, fmt.Errorf("no operation ran, which is a defect in this tool rather than in the call")
}

// operation names what apply is about to do, in the words the usage text uses.
//
// ⛔ IT IS BESIDE THE DISPATCH ON PURPOSE. Two switches over the same set of
// flags, in two files, is a value in two places with no check that they agree;
// this one sits against the switch it describes so a new operation that forgets
// it is one screen away rather than one package away.
func (o *options) operation() string {
	switch o.mode {
	case "write", "append", "eol":
		return o.mode
	}
	switch {
	case o.haveFind:
		return "replace"
	case o.haveLine:
		return "line"
	case o.haveAfter:
		return "insert-after"
	case o.haveBefore:
		return "insert-before"
	case o.haveInsertFind && o.insertBeforeMatch:
		return "before"
	case o.haveInsertFind:
		return "after"
	case o.haveDelete:
		return "delete"
	case o.haveBetween:
		return "between"
	}
	return ""
}

// utf8BOM is the three bytes a Windows editor puts at the front of a file.
//
// ⚠ IT IS DATA, NOT AN ENCODING DECLARATION, to everything that reads bytes. A
// BOM in front of `#!/bin/sh` stops the kernel finding the interpreter, and one
// in front of a JSON document makes encoding/json refuse it, which is why
// dos2unix grew an option for it and why this tool has one.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// applyEOL converts a whole file's line endings and its byte order mark, which
// is what dos2unix and unix2dos do and the only thing they do.
//
// ⛔ THE COUNT IS LINES CONVERTED, NOT LINES IN THE FILE. `--count` over a file
// that is already in the wanted ending answers 0, so a caller can tell "nothing
// to do" from "converted everything" without comparing byte totals.
//
// ⚠ A LONE CARRIAGE RETURN IS NOT A LINE ENDING HERE. Classic Mac files use one
// and converting them is a different job with a different risk: a CR inside a
// quoted string in an otherwise LF file would be rewritten as a line break.
// toEOL only ever pairs CR with the LF that follows it.
func (o *options) applyEOL(before []byte) ([]byte, int, []int, error) {
	want := o.eol
	if want == "keep" {
		return before, 0, nil, fmt.Errorf("eol needs a target: --to lf, --to crlf, --lf or --crlf")
	}
	body := before
	hadBOM := bytes.HasPrefix(body, utf8BOM)
	if hadBOM {
		body = body[len(utf8BOM):]
	}

	// counted BEFORE the conversion, over the endings that are going to move
	converted := 0
	if want == "crlf" {
		converted = bytes.Count(body, []byte("\n")) - bytes.Count(body, []byte("\r\n"))
	} else {
		converted = bytes.Count(body, []byte("\r\n"))
	}
	out := toEOL(body, want)

	switch o.bom {
	case "add":
		out = append(append([]byte{}, utf8BOM...), out...)
	case "strip":
		// already removed
	default:
		if hadBOM {
			out = append(append([]byte{}, utf8BOM...), out...)
		}
	}
	// ⚠ A BOM THE CALLER ASKED TO ADD OR REMOVE IS A CHANGE EVEN WHEN NO LINE
	// ENDING MOVED, so it counts. Otherwise `--bom strip` on a file already in
	// the wanted ending would report 0 matches and `--expect` could not name it.
	if hadBOM != bytes.HasPrefix(out, utf8BOM) {
		converted++
	}
	return out, converted, nil, nil
}

func lineEnding(eol string) []byte {
	if eol == "crlf" {
		return []byte("\r\n")
	}
	return []byte("\n")
}

// applyReplace substitutes, literally or by regular expression.
//
// ⚠ THE SEARCH RUNS OVER THE FILE AS IT IS, line endings included, so a FIND
// spanning two lines matches whichever ending the file uses. The payload is
// converted to the file's ending first, so a replacement written with LF does not
// put a lone LF into a CRLF file.
func (o *options) applyReplace(before, payload []byte) ([]byte, int, []int, error) {
	if o.useRegex {
		re, err := regexpFor(o.find)
		if err != nil {
			return before, 0, nil, err
		}
		locs := re.FindAllIndex(before, -1)
		if len(locs) == 0 {
			return before, 0, nil, nil
		}
		out := re.ReplaceAll(before, payload)
		return out, len(locs), linesAt(before, locs), nil
	}
	find := toEOL([]byte(o.find), detectEOL(before, o.eol))
	if len(find) == 0 {
		return before, 0, nil, fmt.Errorf("--replace was given an empty string, which matches everywhere and means nothing")
	}
	n := bytes.Count(before, find)
	if n == 0 {
		return before, 0, nil, nil
	}
	var locs [][]int
	for at, from := 0, 0; ; from = at + len(find) {
		i := bytes.Index(before[from:], find)
		if i < 0 {
			break
		}
		at = from + i
		locs = append(locs, []int{at, at + len(find)})
	}
	return bytes.ReplaceAll(before, find, payload), n, linesAt(before, locs), nil
}

// maxReportedLines bounds the line numbers one report carries.
//
// ⚠ THE CAP IS VISIBLE, NOT SILENT. It used to be a bare 20 in two places,
// so a delete of 21 lines answered `matches: 21` beside a list of 20 numbers and
// the reader had to guess which was wrong. Report.LinesTruncated says the list
// is short, because a report whose two halves disagree with no explanation is
// the class this tool exists to remove.
const maxReportedLines = 20

// linesAt turns byte offsets into 1-based line numbers, at most maxReportedLines.
func linesAt(body []byte, locs [][]int) []int {
	var out []int
	for _, loc := range locs {
		if len(out) >= maxReportedLines {
			break
		}
		out = append(out, 1+bytes.Count(body[:loc[0]], []byte("\n")))
	}
	return out
}

func (o *options) applyLine(before, payload []byte, eol string) ([]byte, int, []int, error) {
	lines := splitLines(before)
	if o.line < 1 || o.line > len(lines) {
		return before, 0, nil, fmt.Errorf("--line %d, and this file has %d line(s)", o.line, len(lines))
	}
	idx := o.line - 1
	// ⚠ THE TERMINATOR IS THE OLD LINE'S, so replacing the last line of a file
	// that ends without a newline does not add one.
	repl := append([]byte{}, payload...)
	if hadNewline(lines[idx]) && !hadNewline(repl) {
		repl = append(repl, lineEnding(eol)...)
	}
	if !hadNewline(lines[idx]) {
		repl = bytes.TrimRight(repl, "\r\n")
	}
	out := append([]byte{}, bytes.Join(lines[:idx], nil)...)
	out = append(out, repl...)
	out = append(out, bytes.Join(lines[idx+1:], nil)...)
	return out, 1, []int{o.line}, nil
}

func hadNewline(line []byte) bool {
	return len(line) > 0 && line[len(line)-1] == '\n'
}

func (o *options) applyInsert(before, payload []byte, eol string) ([]byte, int, []int, error) {
	lines := splitLines(before)
	at := o.insertBefore - 1
	flag := "--insert-before"
	if o.haveAfter {
		at, flag = o.insertAfter, "--insert-after"
	}
	if at < 0 || at > len(lines) {
		return before, 0, nil, fmt.Errorf("%s %d, and this file has %d line(s)",
			flag, map[bool]int{true: o.insertAfter, false: o.insertBefore}[o.haveAfter], len(lines))
	}
	block := append([]byte{}, payload...)
	if !hadNewline(block) {
		block = append(block, lineEnding(eol)...)
	}
	// ⚠ INSERTING AFTER A LAST LINE THAT HAS NO NEWLINE would join them, so the
	// line before the insertion gets its ending first.
	head := append([]byte{}, bytes.Join(lines[:at], nil)...)
	if at > 0 && !hadNewline(lines[at-1]) {
		head = append(head, lineEnding(eol)...)
	}
	out := append(head, block...)
	out = append(out, bytes.Join(lines[at:], nil)...)
	return out, 1, []int{at + 1}, nil
}

func (o *options) applyDelete(before []byte) ([]byte, int, []int, error) {
	lines := splitLines(before)
	if o.deleteFrom < 1 || o.deleteFrom > len(lines) {
		return before, 0, nil, fmt.Errorf("--delete %d, and this file has %d line(s)", o.deleteFrom, len(lines))
	}
	to := o.deleteTo
	if to > len(lines) {
		return before, 0, nil, fmt.Errorf("--delete %d,%d, and this file has %d line(s)", o.deleteFrom, to, len(lines))
	}
	out := append([]byte{}, bytes.Join(lines[:o.deleteFrom-1], nil)...)
	out = append(out, bytes.Join(lines[to:], nil)...)
	var touched []int
	for n := o.deleteFrom; n <= to && len(touched) < maxReportedLines; n++ {
		touched = append(touched, n)
	}
	return out, to - o.deleteFrom + 1, touched, nil
}

// applyBetween replaces from the line matching A to the line matching B, which is
// awk's /a/,/b/ and sed's addressing.
//
// ⛔ IT TAKES THE FIRST RANGE AND COUNTS THEM ALL. Replacing every range would
// be one operation doing an unbounded number of edits, and the count this
// reports is what --expect checks, so a file with two ranges refuses rather than
// silently changing the first.
func (o *options) applyBetween(before, payload []byte, eol string) ([]byte, int, []int, error) {
	matchA, err := lineMatcher(o.betweenA, o.useRegex)
	if err != nil {
		return before, 0, nil, err
	}
	matchB, err := lineMatcher(o.betweenB, o.useRegex)
	if err != nil {
		return before, 0, nil, err
	}
	lines := splitLines(before)
	var ranges [][2]int
	for i := 0; i < len(lines); i++ {
		if !matchA(lines[i]) {
			continue
		}
		for j := i; j < len(lines); j++ {
			if j > i && matchB(lines[j]) {
				ranges = append(ranges, [2]int{i, j})
				i = j
				break
			}
		}
	}
	if len(ranges) == 0 {
		return before, 0, nil, nil
	}
	if len(ranges) > 1 {
		// Counted, not applied: the caller's --expect decides what happens next.
		return before, len(ranges), []int{ranges[0][0] + 1}, nil
	}
	r := ranges[0]
	block := append([]byte{}, payload...)
	if !hadNewline(block) && hadNewline(lines[r[1]]) {
		block = append(block, lineEnding(eol)...)
	}
	out := append([]byte{}, bytes.Join(lines[:r[0]], nil)...)
	out = append(out, block...)
	out = append(out, bytes.Join(lines[r[1]+1:], nil)...)
	return out, 1, []int{r[0] + 1, r[1] + 1}, nil
}

func lineMatcher(pattern string, useRegex bool) (func([]byte) bool, error) {
	if useRegex {
		re, err := regexpFor(pattern)
		if err != nil {
			return nil, err
		}
		return func(line []byte) bool { return re.Match(trimEnding(line)) }, nil
	}
	want := []byte(pattern)
	return func(line []byte) bool { return bytes.Contains(trimEnding(line), want) }, nil
}

func trimEnding(line []byte) []byte { return bytes.TrimRight(line, "\r\n") }

// applyInsertAt puts the payload beside every line matching a pattern, and
// LEAVES THAT LINE WHERE IT IS.
//
// ⛔ THIS EXISTS BECAUSE --replace EATS ITS ANCHOR. The way to add a paragraph
// above a heading with a substitution is to search for the heading and replace
// it with the paragraph PLUS the heading, and forgetting the second half
// deletes the heading. That is not a hypothetical: it happened three times in
// one session in this repository, twice removing a Go function's declaration
// and leaving its body orphaned, and once removing a document's section
// heading. The operation that means "put this here" should not be spelled as a
// substitution that has to rebuild what it matched.
//
// ⚠ --expect IS REQUIRED, for the same reason --replace requires it: a pattern
// names no place of its own, so a pattern that matched nothing, or matched four
// times, is a different edit from the one the caller asked for.
func (o *options) applyInsertAt(before, payload []byte, eol string) ([]byte, int, []int, error) {
	match, err := lineMatcher(o.insertFind, o.useRegex)
	if err != nil {
		return before, 0, nil, err
	}
	lines := splitLines(before)
	block := append([]byte{}, payload...)
	if !hadNewline(block) {
		block = append(block, lineEnding(eol)...)
	}

	var out []byte
	var touched []int
	matches := 0
	for i, ln := range lines {
		hit := match(ln)
		if hit && o.insertBeforeMatch {
			out = append(out, block...)
		}
		// ⚠ THE ANCHOR LINE THAT HAS NO TRAILING NEWLINE gets one before an
		// insertion after it, or the payload would be joined onto it.
		if hit && !o.insertBeforeMatch && !hadNewline(ln) {
			out = append(out, ln...)
			out = append(out, lineEnding(eol)...)
		} else {
			out = append(out, ln...)
		}
		if hit && !o.insertBeforeMatch {
			out = append(out, block...)
		}
		if hit {
			matches++
			if len(touched) < maxReportedLines {
				touched = append(touched, i+1)
			}
		}
	}
	return out, matches, touched, nil
}
