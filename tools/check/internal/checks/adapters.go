// SPDX-License-Identifier: 0BSD

package checks

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The wsl-toolkit adapter definitions have ONE home and a generated copy.
// TODO/RULES.md section 4 owns the rule, and WSL-77's ruling chose it.
//
// ⛔ WHY THERE IS A COPY AT ALL. The definitions sit beside the tool's examples,
// where a reader finds them, and the base lifecycle that runs them is a package
// further down; go:embed cannot reach out of its own package directory. A copy
// edited by hand is a guest running one script while the tree shows another.
const (
	AdaptersSource = "tools/windows/wsl-toolkit/adapters/"
	AdaptersCopy   = "tools/windows/wsl-toolkit/internal/toolkit/adapters/"
)

// adapterFiles answers every file of every adapter under a root, as NAME/FILE.
//
// ⚠ ONLY ONE LEVEL OF DIRECTORY, AND NOTHING BESIDE IT. A README at the top of the
// source describes the contract and is not an adapter, so it is not copied.
func adapterFiles(t *Tree, root string) map[string]string {
	out := map[string]string{}
	for _, f := range t.Files {
		if !strings.HasPrefix(f, root) {
			continue
		}
		rel := strings.TrimPrefix(f, root)
		if strings.Count(rel, "/") != 1 {
			continue
		}
		out[rel] = f
	}
	return out
}

// Adapters refuses the executable's copy of an adapter disagreeing with its
// definition: a file that differs, one that is missing, and one with no source.
func Adapters(t *Tree) Result {
	r := Result{Extra: map[string]any{}}
	src := adapterFiles(t, AdaptersSource)
	cp := adapterFiles(t, AdaptersCopy)
	if len(src) == 0 {
		r.bad("%s holds no adapter definition, and the executable embeds its copy from there", AdaptersSource)
		return r
	}
	names := make([]string, 0, len(src))
	for rel := range src {
		names = append(names, rel)
	}
	sort.Strings(names)
	for _, rel := range names {
		dest, ok := cp[rel]
		if !ok {
			r.bad("%s%s is missing. Write it with: sh scripts/common/check.sh adapters --fix", AdaptersCopy, rel)
			continue
		}
		want := lf(t.Read(src[rel]))
		got := lf(t.Read(dest))
		if !bytes.Equal(want, got) {
			r.bad("%s disagrees with %s at line %d. Rewrite it with: sh scripts/common/check.sh adapters --fix",
				dest, src[rel], firstDifferingLine(want, got))
		}
	}
	for rel, dest := range cp {
		if _, ok := src[rel]; !ok {
			r.bad("%s has no definition in %s. Rewrite the copy with: sh scripts/common/check.sh adapters --fix", dest, AdaptersSource)
		}
	}
	r.Extra["files"] = len(src)
	return r
}

func lf(b []byte) []byte { return []byte(strings.ReplaceAll(string(b), "\r\n", "\n")) }

// WriteAdapters is the fix half: every definition file copied, each through a
// temporary file in its own directory, and every copied file with no definition
// removed.
func WriteAdapters(root string) ([]string, error) {
	t, err := Load(root)
	if err != nil {
		return nil, err
	}
	var wrote []string
	for rel, source := range adapterFiles(t, AdaptersSource) {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(source)))
		if err != nil {
			return wrote, err
		}
		dest := filepath.Join(root, filepath.FromSlash(AdaptersCopy+rel))
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
	src := adapterFiles(t, AdaptersSource)
	for rel, copied := range adapterFiles(t, AdaptersCopy) {
		if _, ok := src[rel]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(copied))); err != nil && !os.IsNotExist(err) {
			return wrote, err
		}
		wrote = append(wrote, copied+" (removed)")
	}
	sort.Strings(wrote)
	return wrote, nil
}
