// docs_test.go - a path a live page names in backticks has to exist, and so
// does a line it cites.
//
// ⛔ THE RULE THESE CASES HOLD NEVER RAN FROM 2026-09-09 TO 2026-10-21. The
// port to one binary left its block empty, and docs/conventions/prose.md kept
// saying every cited path exists. `TOOL-30`. So one case runs the whole check,
// which is the only way to prove the rule is called and not only that it works.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// treeWithPages writes files by repository path. A path in untracked is
// written to disk and left out of the tracked list.
func treeWithPages(t *testing.T, tracked map[string]string, untracked ...string) *Tree {
	t.Helper()
	root := t.TempDir()
	tree := &Tree{Root: root, cache: map[string][]byte{}}
	write := func(p, body string) {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("could not make the directory for %s: %v", p, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("could not write %s: %v", p, err)
		}
	}
	for p, body := range tracked {
		write(p, body)
		tree.Files = append(tree.Files, p)
	}
	for _, p := range untracked {
		write(p, "kept on this machine only\n")
	}
	return tree
}

func TestACitedPathAndLineMustExist(t *testing.T) {
	const code = "package a\n\nfunc A() {}\n"
	cases := []struct {
		name       string
		page       string
		file       string
		want       int
		wantDetail string
	}{
		{name: "a path that exists", page: "See `tools/a/a.go`.\n", want: 0},
		{name: "a directory that exists", page: "See `tools/a/`.\n", want: 0},
		{name: "a path that does not", page: "See `tools/a/gone.go`.\n", want: 1, wantDetail: "cited path does not exist -> `tools/a/gone.go`"},
		{name: "a line inside the file", page: "See `tools/a/a.go:3`.\n", want: 0},
		{name: "a range inside the file", page: "See `tools/a/a.go:1-3`.\n", want: 0},
		{name: "a line past the end", page: "See `tools/a/a.go:40`.\n", want: 1, wantDetail: "cites line 40 and the file has 3"},
		{name: "a path on this machine only", page: "See `tools/a/local.go`.\n", want: 1, wantDetail: "on this machine and not in the repository"},
		{name: "a line about another project's tree", page: "`someone/project` keeps `tools/elsewhere.go`.\n", want: 0},
		{name: "a token that is no path", page: "Run `go test ./...` with `$HOME/x` and `/usr/lib/y`.\n", want: 0},
		{name: "the changelog carries history", file: "CHANGELOG.md", page: "Moved `tools/a/gone.go`.\n", want: 0},
		{name: "an entry file carries history", file: "TODO/wsl.md", page: "Read `tools/a/gone.go`.\n", want: 0},
		{name: "a reference sweep reads another tree", file: "docs/reference-sweeps/x.md", page: "Upstream has `tools/a/gone.go`.\n", want: 0},
		{name: "the record is live", file: "TODO/PROGRESS.md", page: "Next: `tools/a/gone.go`.\n", want: 1, wantDetail: "TODO/PROGRESS.md:1:"},
		{name: "a superseded page names the tree as it was", file: "docs/HISTORY/x.md", page: "It was `tools/a/gone.go`.\n", want: 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := c.file
			if file == "" {
				file = "README.md"
			}
			tracked := map[string]string{"tools/a/a.go": code, file: c.page}
			// A page under docs/ that nothing links to is its own finding.
			if strings.HasPrefix(file, "docs/") {
				tracked["README.md"] = "[x](" + file + ")\n"
			}
			r := Docs(treeWithPages(t, tracked, "tools/a/local.go"))
			if r.Problems != c.want {
				t.Fatalf("problems = %d, want %d: %s", r.Problems, c.want, strings.Join(r.Detail, " | "))
			}
			if c.wantDetail != "" && !strings.Contains(strings.Join(r.Detail, " | "), c.wantDetail) {
				t.Fatalf("no finding said %q: %s", c.wantDetail, strings.Join(r.Detail, " | "))
			}
		})
	}
}
