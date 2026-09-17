// SPDX-License-Identifier: 0BSD

package checks

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The general-purpose files the published executable carries have ONE home and a
// generated copy, exactly as the adapters do.
//
// ⛔ WHY THERE IS A COPY AT ALL. `scripts/common/` is where a reader finds these
// files and where `docs/consumers.md` points a caller who fetches one by URL. The
// executable that runs them is several directories away and `go:embed` cannot
// reach out of its own package. A copy edited by hand, or left behind after the
// definition moved, is a binary running one script while the tree shows another -
// and unlike the adapters, THESE are also published by URL, so the two could
// disagree for every caller at once.
const (
	ShippedSource = "scripts/common/"
	ShippedCopy   = "tools/windows/wsl-toolkit/internal/toolkit/shipped/"
)

// shippedNames are the files carried, and it is a list rather than a directory
// scan of the source.
//
// ⛔ scripts/common/ HOLDS THE CHECKS TOO, and embedding those would put every
// gate script inside the published binary. A new carried file is a decision, so
// it is named here and in internal/toolkit/shipped.go's mode table.
var shippedNames = []string{"bootstrap.sh", "shell-profile.sh", "tmux.conf"}

// Shipped refuses the executable's copy disagreeing with its definition: a file
// that differs, one that is missing, and one with no definition behind it.
func Shipped(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	have := map[string]bool{}
	for _, f := range t.Files {
		if strings.HasPrefix(f, ShippedCopy) {
			rel := strings.TrimPrefix(f, ShippedCopy)
			if !strings.Contains(rel, "/") {
				have[rel] = true
			}
		}
	}
	for _, name := range shippedNames {
		src := ShippedSource + name
		dest := ShippedCopy + name
		if !t.Exists(src) {
			r.bad("%s is carried by the executable and has no definition at %s", name, src)
			continue
		}
		if !have[name] {
			r.bad("%s is missing. Write it with: cp %s %s", dest, src, dest)
			continue
		}
		delete(have, name)
		want := lf(t.Read(src))
		got := lf(t.Read(dest))
		if !bytes.Equal(want, got) {
			r.bad("%s disagrees with %s at line %d. Rewrite it with: cp %s %s",
				dest, src, firstDifferingLine(want, got), src, dest)
		}
	}
	names := make([]string, 0, len(have))
	for rel := range have {
		names = append(names, rel)
	}
	sort.Strings(names)
	for _, rel := range names {
		r.bad("%s%s is carried and is not one this repository ships. Remove it with: rm %s%s", ShippedCopy, rel, ShippedCopy, rel)
	}
	r.Extra["files"] = len(shippedNames)
	return r
}

// WriteShippedCopies is the fix half: every named file copied, through a
// temporary file in its own directory, and every carried file with no definition
// removed.
func WriteShippedCopies(root string) ([]string, error) {
	t, err := Load(root)
	if err != nil {
		return nil, err
	}
	var wrote []string
	keep := map[string]bool{}
	for _, name := range shippedNames {
		keep[name] = true
		src := filepath.Join(root, filepath.FromSlash(ShippedSource+name))
		body, err := os.ReadFile(src)
		if err != nil {
			return wrote, err
		}
		dest := filepath.Join(root, filepath.FromSlash(ShippedCopy+name))
		if current, err := os.ReadFile(dest); err == nil && bytes.Equal(lf(current), lf(body)) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return wrote, err
		}
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, lf(body), 0o644); err != nil {
			return wrote, err
		}
		if err := os.Rename(tmp, dest); err != nil {
			_ = os.Remove(tmp)
			return wrote, err
		}
		wrote = append(wrote, dest)
	}
	for _, f := range t.Files {
		if !strings.HasPrefix(f, ShippedCopy) {
			continue
		}
		rel := strings.TrimPrefix(f, ShippedCopy)
		if strings.Contains(rel, "/") || keep[rel] {
			continue
		}
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(f))); err != nil && !os.IsNotExist(err) {
			return wrote, err
		}
		wrote = append(wrote, f+" (removed)")
	}
	sort.Strings(wrote)
	return wrote, nil
}
