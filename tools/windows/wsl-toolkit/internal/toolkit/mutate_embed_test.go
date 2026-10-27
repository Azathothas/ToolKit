// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/embedswap"
)

// ⭐ A MUTATION ROW ON AN EMBEDDED FILE RUNS ON THE BINARY ITS BASELINE BUILT.
// tools/repo's harness hands this binary the changed bytes instead of compiling
// the package again. A compile of this package and its cases is most of a row.
// internal/embedswap reads the harness's variables. TOOL-40.
//
// ⛔ A VARIABLE, NOT AN init. Go sets every package variable before any init
// runs, and several inits here start a fake and stop. A fake an earlier file's
// init started read the old script. TestAFakeAnInitStartsReadsTheRowsBytes.
var _ = takeMutationRow()

// takeMutationRow hands this binary the bytes a mutation row names.
//
// ⛔ A PROCESS THAT WAS NAMED A ROW AND COULD NOT TAKE IT STOPS, in
// embedswap.MustTake. TestABinaryThatCannotTakeItsRowStops.
func takeMutationRow() bool { return embedswap.MustTake(takeEmbedded) }

// embeddedBytes is every file this package embeds as a []byte, by its pattern.
// internal/toolkitsource holds it to the go:embed lines.
func embeddedBytes() map[string]*[]byte {
	return map[string]*[]byte{
		"provision.sh":    &provisionScript,
		"boot.sh":         &bootScript,
		"packages.sh":     &packagesScript,
		"verify.sh":       &verifyScript,
		"repair.sh":       &repairScript,
		"grants.sh":       &grantsScript,
		"mount-helper.sh": &mountHelperScript,
		"binfmt.sh":       &binfmtScript,
		"doors.sh":        &doorsScript,
		"drives.sh":       &drivesScript,
		"engines.sh":      &enginesScript,
		"loginshell.sh":   &loginShellScript,
		"mirrors.sh":      &mirrorsScript,
		"private-net.sh":  &privateNetScript,
		"stale.sh":        &staleScript,
	}
}

// embeddedTrees is every directory this package embeds, by its pattern.
func embeddedTrees() map[string]*embedTree {
	return map[string]*embedTree{
		"adapters": &adapterTree,
		"shipped":  &shippedTree,
	}
}

// takeEmbedded puts a row's bytes in the variable that embeds name: a script's
// own, or a copy of the directory that holds it.
func takeEmbedded(name string, body []byte) error {
	if p, ok := embeddedBytes()[name]; ok {
		*p = body
		return nil
	}
	root, _, _ := strings.Cut(name, "/")
	tree, ok := embeddedTrees()[root]
	if !ok {
		return fmt.Errorf("%s is not a file this package embeds", name)
	}
	copied, err := embedswap.WithFile(*tree, name, body)
	if err != nil {
		return err
	}
	*tree = copied
	return nil
}

// TestEveryEmbeddedFileCanTakeAMutation holds each entry of the two tables to
// its file: a script's own bytes, or a directory with files in it.
//
// ⚠ THE TABLES' MATCH TO THE go:embed LINES IS READ FROM THE SOURCE, in
// internal/toolkitsource. That reading needs go/types, and this binary does not
// link it. Every mutation row's run links this binary again. TOOL-40.
func TestEveryEmbeddedFileCanTakeAMutation(t *testing.T) {
	for name, p := range embeddedBytes() {
		want, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(*p, want) {
			t.Errorf("the entry for %s is not its file's bytes", name)
		}
	}
	for name, tree := range embeddedTrees() {
		var held []string
		err := fs.WalkDir(*tree, name, func(p string, e fs.DirEntry, err error) error {
			if err == nil && !e.IsDir() {
				held = append(held, p)
			}
			return err
		})
		if err != nil || len(held) == 0 {
			t.Errorf("the entry for %s holds no file of it: %v", name, err)
		}
	}
}

// TestAMutationRowsBytesReachTheVariableAndTheFile is what the harness relies on
// when it runs a row on the binary its baseline built.
func TestAMutationRowsBytesReachTheVariableAndTheFile(t *testing.T) {
	savedVerify, savedTree := verifyScript, adapterTree
	t.Cleanup(func() { verifyScript, adapterTree = savedVerify, savedTree })
	dir := t.TempDir()
	hand := func(name string, body []byte) string {
		t.Helper()
		with := filepath.Join(dir, "with-"+path.Base(name))
		file := filepath.Join(dir, path.Base(name))
		if err := os.WriteFile(with, body, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("as it was\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(embedswap.NameEnv, name)
		t.Setenv(embedswap.WithEnv, with)
		t.Setenv(embedswap.FileEnv, file)
		t.Setenv(embedswap.StagedEnv, embedswap.Digest([]byte("as it was\n")))
		// ⚠ EACH HAND-IN IS A CASE PROCESS'S OWN, not a child of the last.
		t.Setenv(embedswap.TakenEnv, "")
		return file
	}
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}

	mutated := []byte("#!/bin/sh\necho the row's verify.sh\n")
	file := hand("verify.sh", mutated)
	if err := embedswap.Take(takeEmbedded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(verifyScript, mutated) {
		t.Error("verify.sh's variable did not take the row's bytes, so its cases ran on the old script")
	}
	if got := read(file); got != string(mutated) {
		t.Errorf("the file was left %q, so a case that reads it saw the old script", got)
	}

	probe := []byte("#!/bin/sh\necho the row's probe.sh\n")
	file = hand("adapters/muse/probe.sh", probe)
	if err := embedswap.Take(takeEmbedded); err != nil {
		t.Fatal(err)
	}
	if got, err := adapterScript("muse", "probe.sh"); err != nil || !bytes.Equal(got, probe) {
		t.Errorf("muse's probe.sh read %q, %v, not the row's bytes", got, err)
	}
	if got := read(file); got != string(probe) {
		t.Errorf("the file was left %q, so a case that reads it saw the old script", got)
	}
	// ⛔ THE REST OF THE TREE IS THE EMBEDDED ONE. A copy that lost a file would
	// fail an adapter's case for a reason the row did not cause.
	for _, name := range []string{"install.sh", "herdr-agent-state.sh"} {
		want, err := savedTree.ReadFile("adapters/muse/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := adapterTree.ReadFile("adapters/muse/" + name); err != nil || !bytes.Equal(got, want) {
			t.Errorf("muse's %s changed when probe.sh was handed in: %v", name, err)
		}
	}
	was, _ := savedTree.ReadDir("adapters/muse")
	now, err := adapterTree.ReadDir("adapters/muse")
	if err != nil || len(now) != len(was) {
		t.Errorf("muse's directory lists %d entries after the swap and %d before: %v", len(now), len(was), err)
	}

	// ⛔ A NAME THIS PACKAGE DOES NOT EMBED WRITES NO FILE, so the harness sees
	// the row was not taken and compiles it.
	file = hand("nothing-embeds-this.sh", []byte("x\n"))
	if err := embedswap.Take(takeEmbedded); err == nil {
		t.Error("a file this package does not embed was taken")
	}
	if got := read(file); got != "as it was\n" {
		t.Errorf("a file this package does not embed was written: %q", got)
	}
}
