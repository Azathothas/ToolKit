// SPDX-License-Identifier: 0BSD

package checks

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The shared package table has ONE home and a generated copy, which is the shape
// TODO/RULES.md section 4 already describes for wsl-toolkit.ps1.
//
// ⛔ WHY THE HOME IS bootstrap.sh. That file is fetched by raw URL and run on its
// own, so it cannot read a sibling, and the table has to live inside it. The
// wsl-toolkit base provisioner is compiled into an executable, and go:embed
// cannot reach outside its own package directory, so the copy sits in the
// provisioner's package. Two hand-kept maps had already disagreed: one knew
// twelve package managers and the other six. ⚠ Nothing reads the copy yet, and
// wiring the provisioner to it is the rest of WSL-70.
const (
	PackageTableSource = "scripts/common/bootstrap.sh"
	PackageTableCopy   = "tools/windows/wsl-toolkit/internal/toolkit/packages.sh"
	packageTableBegin  = "# >>> shared package table: begin"
	packageTableEnd    = "# <<< shared package table: end"
)

const packageTableBanner = `#!/bin/sh
# ⛔ GENERATED FILE. DO NOT EDIT IT.
#
# This is the shared package table from scripts/common/bootstrap.sh, copied byte
# for byte from its begin marker line to its end marker line, for the base
# provisioner to resolve its developer names from. ⚠ Nothing reads it yet;
# wiring the provisioner to it is the rest of WSL-70. Edit the block in
# bootstrap.sh, then rewrite this with:
#
#   sh scripts/common/check.sh package-table --fix
#
# The gate's package-table check refuses this file disagreeing with that block.

`

// extractPackageTable answers the block from its begin marker line to its end
// marker line, both included, with LF endings.
//
// ⛔ A MISSING OR DOUBLED MARKER IS A REFUSAL, NOT A SHORTER BLOCK. Taking "the
// first end after the first begin" would silently publish half a table the day a
// second marker was pasted in, and the provisioner would fail on a function that
// was never copied.
func extractPackageTable(src []byte) ([]byte, error) {
	lines := strings.SplitAfter(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	begin, end := -1, -1
	for i, ln := range lines {
		switch strings.TrimRight(ln, "\n") {
		case packageTableBegin:
			if begin >= 0 {
				return nil, fmt.Errorf("%s carries the line %q twice", PackageTableSource, packageTableBegin)
			}
			begin = i
		case packageTableEnd:
			if end >= 0 {
				return nil, fmt.Errorf("%s carries the line %q twice", PackageTableSource, packageTableEnd)
			}
			end = i
		}
	}
	switch {
	case begin < 0:
		return nil, fmt.Errorf("%s has no line %q", PackageTableSource, packageTableBegin)
	case end < 0:
		return nil, fmt.Errorf("%s has no line %q", PackageTableSource, packageTableEnd)
	case end < begin:
		return nil, fmt.Errorf("%s puts %q before %q", PackageTableSource, packageTableEnd, packageTableBegin)
	}
	block := strings.Join(lines[begin:end+1], "")
	if !strings.HasSuffix(block, "\n") {
		block += "\n"
	}
	return []byte(block), nil
}

// renderPackageTableCopy is the generated file's exact bytes.
func renderPackageTableCopy(src []byte) ([]byte, error) {
	block, err := extractPackageTable(src)
	if err != nil {
		return nil, err
	}
	return append([]byte(packageTableBanner), block...), nil
}

// PackageTable refuses the provisioner's copy disagreeing with the block it is
// generated from.
func PackageTable(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	src := t.Read(PackageTableSource)
	if len(src) == 0 {
		r.bad("%s is missing, and the base provisioner's package table is generated from it", PackageTableSource)
		return r
	}
	want, err := renderPackageTableCopy(src)
	if err != nil {
		r.bad("%v", err)
		return r
	}
	got := []byte(strings.ReplaceAll(string(t.Read(PackageTableCopy)), "\r\n", "\n"))
	if len(got) == 0 {
		r.bad("%s is missing. Write it with: sh scripts/common/check.sh package-table --fix", PackageTableCopy)
		return r
	}
	if !bytes.Equal(want, got) {
		r.bad("%s disagrees with the block in %s at line %d. Rewrite it with: sh scripts/common/check.sh package-table --fix",
			PackageTableCopy, PackageTableSource, firstDifferingLine(want, got))
	}
	r.Extra["lines"] = bytes.Count(want, []byte("\n"))
	return r
}

// firstDifferingLine is 1-based, because a byte offset is true and useless.
func firstDifferingLine(a, b []byte) int {
	al := strings.Split(string(a), "\n")
	bl := strings.Split(string(b), "\n")
	for i := 0; i < len(al) && i < len(bl); i++ {
		if al[i] != bl[i] {
			return i + 1
		}
	}
	if len(al) < len(bl) {
		return len(al) + 1
	}
	return len(bl) + 1
}

// WritePackageTable is the fix half: it rewrites the generated copy, through a
// temporary file in the same directory so a killed run leaves the old copy whole.
func WritePackageTable(root string) (string, error) {
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(PackageTableSource)))
	if err != nil {
		return "", err
	}
	want, err := renderPackageTableCopy(src)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(root, filepath.FromSlash(PackageTableCopy))
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, want, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return dest, nil
}
