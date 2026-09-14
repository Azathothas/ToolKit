// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"path/filepath"
	"testing"
)

func adaptersTree(t *testing.T, files map[string]string) *Tree {
	t.Helper()
	root := t.TempDir()
	tree := &Tree{Root: root, cache: map[string][]byte{}}
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		tree.Files = append(tree.Files, rel)
	}
	return tree
}

// TestTheEmbeddedAdapterCopyIsRefusedWhenEitherSideMoves is WSL-76's rule under
// WSL-77's ruling: an edit to the definition alone, an edit to the copy alone, a
// missing copy and a copy with no definition all fail, and a README beside the
// definitions is not an adapter.
func TestTheEmbeddedAdapterCopyIsRefusedWhenEitherSideMoves(t *testing.T) {
	good := map[string]string{
		AdaptersSource + "README.md":         "the contract\n",
		AdaptersSource + "herdr/install.sh":  "#!/bin/sh\necho install\n",
		AdaptersSource + "herdr/config.toml": "onboarding = false\n",
		AdaptersCopy + "herdr/install.sh":    "#!/bin/sh\necho install\n",
		AdaptersCopy + "herdr/config.toml":   "onboarding = false\n",
	}
	if r := Adapters(adaptersTree(t, good)); r.Problems != 0 {
		t.Fatalf("a current copy was refused: %v", r.Detail)
	}
	for name, change := range map[string]func(map[string]string){
		"the definition edited alone": func(f map[string]string) { f[AdaptersSource+"herdr/install.sh"] = "#!/bin/sh\necho changed\n" },
		"the copy edited alone":       func(f map[string]string) { f[AdaptersCopy+"herdr/config.toml"] = "onboarding = true\n" },
		"a copy missing":              func(f map[string]string) { delete(f, AdaptersCopy+"herdr/install.sh") },
		"a copy with no definition":   func(f map[string]string) { f[AdaptersCopy+"herdr/stale.sh"] = "exit 0\n" },
	} {
		files := map[string]string{}
		for k, v := range good {
			files[k] = v
		}
		change(files)
		if r := Adapters(adaptersTree(t, files)); r.Problems == 0 {
			t.Errorf("%s passed", name)
		}
	}
}
