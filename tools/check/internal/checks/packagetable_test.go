// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func packageTableTree(t *testing.T, source, copied string) *Tree {
	t.Helper()
	root := t.TempDir()
	for rel, body := range map[string]string{PackageTableSource: source, PackageTableCopy: copied} {
		if body == "" {
			continue
		}
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &Tree{Root: root, cache: map[string][]byte{}}
}

const sharedSource = "#!/bin/sh\nset -eu\n" + packageTableBegin + "\npackage_table() {\n  printf 'git git\\n'\n}\n" + packageTableEnd + "\nmain\n"

// TestTheProvisionerCopyOfThePackageTableIsRefusedWhenOneSideMoves is the case
// for two maps that had already drifted, one knowing twelve package managers and
// the other six. An edit to either side alone fails; the generated copy passes.
// WSL-70.
func TestTheProvisionerCopyOfThePackageTableIsRefusedWhenOneSideMoves(t *testing.T) {
	good, err := renderPackageTableCopy([]byte(sharedSource))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(good), "#!/bin/sh\n# ⛔ GENERATED FILE") || !strings.Contains(string(good), "printf 'git git\\n'") {
		t.Fatalf("the generated copy is not the banner and the block:\n%s", good)
	}
	if strings.Contains(string(good), "main\n") {
		t.Fatalf("the generated copy reaches past the end marker:\n%s", good)
	}

	if r := PackageTable(packageTableTree(t, sharedSource, string(good))); r.Problems != 0 {
		t.Fatalf("a current copy was refused: %v", r.Detail)
	}
	editedCopy := strings.Replace(string(good), "git git", "git gitx", 1)
	if r := PackageTable(packageTableTree(t, sharedSource, editedCopy)); r.Problems == 0 {
		t.Fatal("a hand-edited copy passed")
	}
	editedSource := strings.Replace(sharedSource, "git git", "git git os:rocky=-", 1)
	if r := PackageTable(packageTableTree(t, editedSource, string(good))); r.Problems == 0 {
		t.Fatal("a table edited in bootstrap.sh alone passed with the old copy")
	}
	if r := PackageTable(packageTableTree(t, sharedSource, "")); r.Problems == 0 {
		t.Fatal("a missing copy passed")
	}
}

// TestAMissingOrDoubledPackageTableMarkerIsRefused holds the extraction to the
// markers it names, rather than to "the first end after the first begin".
func TestAMissingOrDoubledPackageTableMarkerIsRefused(t *testing.T) {
	for name, src := range map[string]string{
		"no begin":      strings.Replace(sharedSource, packageTableBegin+"\n", "", 1),
		"no end":        strings.Replace(sharedSource, packageTableEnd+"\n", "", 1),
		"begin twice":   strings.Replace(sharedSource, packageTableBegin, packageTableBegin+"\n"+packageTableBegin, 1),
		"end before it": packageTableEnd + "\n" + strings.Replace(sharedSource, packageTableEnd+"\n", "", 1),
	} {
		if _, err := extractPackageTable([]byte(src)); err == nil {
			t.Errorf("%s: the block was extracted anyway", name)
		}
	}
}
