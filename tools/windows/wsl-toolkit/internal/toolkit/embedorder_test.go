// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/embedswap"
)

// embedOrderEnv starts this binary as a fake that prints verify.sh and stops.
//
// ⛔ THIS FILE SORTS BEFORE mutate_embed_test.go, so its init runs first. It
// stands for the fakes an init starts, as bsd_test.go's and ledger_test.go's do.
// TOOL-40.
const embedOrderEnv = "WTK_TEST_EMBED_ORDER"

func init() {
	if os.Getenv(embedOrderEnv) != "" {
		_, _ = os.Stdout.Write(verifyScript)
		os.Exit(0)
	}
}

// TestAFakeAnInitStartsReadsTheRowsBytes: a mutation row runs on the binary its
// baseline built, and a case may start that binary again as a fake.
//
// ⛔ THE FAKE READS THE ROW'S SCRIPT, as it would from a compile of the row. A
// fake that read the old one could fail its case for a reason the row did not
// cause. The harness would then certify a guard nothing tested.
func TestAFakeAnInitStartsReadsTheRowsBytes(t *testing.T) {
	dir := t.TempDir()
	with, file := filepath.Join(dir, "with"), filepath.Join(dir, "verify.sh")
	body := []byte("#!/bin/sh\necho the row's verify.sh\n")
	if err := os.WriteFile(with, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("as it was\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	// ⚠ LAST, so a row's own variables in this run's environment are overridden.
	cmd.Env = append(os.Environ(), embedOrderEnv+"=1", embedswap.NameEnv+"=verify.sh",
		embedswap.WithEnv+"="+with, embedswap.FileEnv+"="+file)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("the fake did not run: %v", err)
	}
	if !bytes.Equal(out, body) {
		t.Fatalf("a fake an init started read %d bytes of verify.sh, not the row's %d", len(out), len(body))
	}
}
