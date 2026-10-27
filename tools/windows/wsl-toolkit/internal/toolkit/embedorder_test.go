// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/embedswap"
	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
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

// fakeWithRow starts this binary as the fake above, named a row on verify.sh.
// taken says whether it starts as a child of a case process that took the row.
func fakeWithRow(t *testing.T, taken bool, with, file string) ([]byte, error) {
	t.Helper()
	mark := ""
	if taken {
		mark = "1"
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	// ⚠ LAST, so a row's own variables in this run's environment are overridden.
	cmd.Env = append(os.Environ(), embedOrderEnv+"=1", embedswap.NameEnv+"=verify.sh",
		embedswap.WithEnv+"="+with, embedswap.FileEnv+"="+file,
		embedswap.StagedEnv+"="+embedswap.Digest([]byte("as it was\n")), embedswap.TakenEnv+"="+mark)
	return cmd.Output()
}

// rowFiles writes a row's bytes and the file they are staged over.
func rowFiles(t *testing.T, body []byte) (with, file string) {
	t.Helper()
	dir := t.TempDir()
	with, file = filepath.Join(dir, "with"), filepath.Join(dir, "verify.sh")
	if err := forksafe.WriteFile(with, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := forksafe.WriteFile(file, []byte("as it was\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return with, file
}

// TestAFakeAnInitStartsReadsTheRowsBytes: a mutation row runs on the binary its
// baseline built, and a case may start that binary again as a fake.
//
// ⛔ THE FAKE READS THE ROW'S SCRIPT, as it would from a compile of the row. A
// fake that read the old one could fail its case for a reason the row did not
// cause. The harness would then certify a guard nothing tested.
//
// ⛔ AND IT WRITES NO RECEIPT. Only the case process does.
func TestAFakeAnInitStartsReadsTheRowsBytes(t *testing.T) {
	body := []byte("#!/bin/sh\necho the row's verify.sh\n")
	with, file := rowFiles(t, body)
	out, err := fakeWithRow(t, true, with, file)
	if err != nil {
		t.Fatalf("the fake did not run: %v", err)
	}
	if !bytes.Equal(out, body) {
		t.Fatalf("a fake an init started read %d bytes of verify.sh, not the row's %d", len(out), len(body))
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "as it was\n" {
		t.Fatalf("a fake wrote the receipt: %q, %v", got, err)
	}
}

// TestABinaryThatCannotTakeItsRowStops: the row's bytes cannot be read, in the
// case process and in a child of it.
//
// ⛔ IT STOPS BEFORE ANY CASE OR FAKE RUNS, AND LEAVES THE MISSED MARK. Run on,
// it would answer with the old script, and a case could go red for that.
func TestABinaryThatCannotTakeItsRowStops(t *testing.T) {
	for _, taken := range []bool{false, true} {
		with, file := rowFiles(t, []byte("the row's bytes\n"))
		if err := os.Remove(with); err != nil {
			t.Fatal(err)
		}
		out, err := fakeWithRow(t, taken, with, file)
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != embedswap.MissedExit {
			t.Errorf("child %v: a binary that could not take its row answered %v and printed %q", taken, err, out)
		}
		if _, err := os.Stat(with + embedswap.MissedSuffix); err != nil {
			t.Errorf("child %v: no missed mark was left: %v", taken, err)
		}
	}
}
