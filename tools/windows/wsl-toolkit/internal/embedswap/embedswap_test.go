// SPDX-License-Identifier: 0BSD

package embedswap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// staged is the bytes a row's file holds before the row is taken.
const staged = "as it was\n"

// row hands the protocol's variables a row's bytes, as the harness does for a
// case process, and answers the file and the bytes' own file.
func row(t *testing.T, name string, body []byte) (file, with string) {
	t.Helper()
	dir := t.TempDir()
	with, file = filepath.Join(dir, "with"), filepath.Join(dir, "file")
	if err := os.WriteFile(with, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(staged), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(NameEnv, name)
	t.Setenv(WithEnv, with)
	t.Setenv(FileEnv, file)
	t.Setenv(StagedEnv, Digest([]byte(staged)))
	t.Setenv(TakenEnv, "")
	return file, with
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// missed is whether a process left the missed mark beside with.
func missed(t *testing.T, with string) bool {
	t.Helper()
	_, err := os.Stat(with + MissedSuffix)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return err == nil
}

// TestTheBytesReachWhatTakesThemAndThenTheFile is the order the harness's
// receipt rests on: the file says the binary took the bytes.
func TestTheBytesReachWhatTakesThemAndThenTheFile(t *testing.T) {
	file, with := row(t, "verify.sh", []byte("the row's bytes\n"))
	var took string
	var fileWhenTaken string
	err := Take(func(name string, body []byte) error {
		took = name + ": " + string(body)
		fileWhenTaken = read(t, file)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if took != "verify.sh: the row's bytes\n" {
		t.Errorf("take was handed %q", took)
	}
	if fileWhenTaken != staged {
		t.Errorf("the file held %q before take had the bytes, so a receipt could stand for bytes nothing took", fileWhenTaken)
	}
	if got := read(t, file); got != "the row's bytes\n" {
		t.Errorf("the file was left %q, so a case that reads it saw the old script", got)
	}
	if missed(t, with) {
		t.Error("a process that took its row left the missed mark")
	}
}

// TestTheCaseProcessMarksItsChildren: the process that wrote the receipt sets
// TakenEnv, and every process a case starts inherits it.
func TestTheCaseProcessMarksItsChildren(t *testing.T) {
	row(t, "verify.sh", []byte("the row's bytes\n"))
	if err := Take(func(string, []byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if os.Getenv(TakenEnv) == "" {
		t.Error("the case process did not mark its children, so a child would write a receipt too")
	}
}

// TestAChildTakesTheBytesAndWritesNoReceipt: a case starts its binary again as
// a fake, with the case process's variables.
//
// ⛔ ONLY THE CASE PROCESS WRITES THE RECEIPT. A child that wrote it would
// stand for a case process whose own take failed, and its cases ran on the old
// bytes.
func TestAChildTakesTheBytesAndWritesNoReceipt(t *testing.T) {
	file, with := row(t, "verify.sh", []byte("the row's bytes\n"))
	t.Setenv(TakenEnv, "1")
	took := false
	if err := Take(func(string, []byte) error { took = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if !took {
		t.Error("the child did not take the bytes, so its variable held the old script")
	}
	if got := read(t, file); got != staged {
		t.Errorf("a child wrote the receipt: %q", got)
	}
	if missed(t, with) {
		t.Error("a child that took its row left the missed mark")
	}
}

// TestAFileNotAsStagedTakesNoReceipt: the file holds bytes the row was not
// staged over, as a later row's or one already put back.
//
// ⛔ NO WRITE, AND THE RUN IS MARKED MISSED. A process that outlived its row
// would otherwise write its bytes into the next row's copy.
func TestAFileNotAsStagedTakesNoReceipt(t *testing.T) {
	file, with := row(t, "verify.sh", []byte("the row's bytes\n"))
	if err := os.WriteFile(file, []byte("a later row's bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Take(func(string, []byte) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "does not hold the bytes the row was staged over") {
		t.Errorf("Take answered %v, want a refusal of the file", err)
	}
	if got := read(t, file); got != "a later row's bytes\n" {
		t.Errorf("a file not as staged was written: %q", got)
	}
	if !missed(t, with) {
		t.Error("a process that wrote no receipt left no missed mark")
	}
}

// TestAProcessThatCannotReadTheBytesSaysSo: the bytes' file cannot be read,
// as on Windows when another process holds it.
//
// ⛔ THE MISSED MARK IS WHAT A CHILD'S FAILURE LEAVES. The case process may
// have written its receipt already, and only the mark sends the row to a
// compile.
func TestAProcessThatCannotReadTheBytesSaysSo(t *testing.T) {
	for _, child := range []bool{false, true} {
		file, with := row(t, "verify.sh", []byte("the row's bytes\n"))
		if child {
			t.Setenv(TakenEnv, "1")
		}
		if err := os.Remove(with); err != nil {
			t.Fatal(err)
		}
		called := false
		if err := Take(func(string, []byte) error { called = true; return nil }); err == nil {
			t.Errorf("child %v: bytes that could not be read were taken", child)
		}
		if called {
			t.Errorf("child %v: take was called with no bytes", child)
		}
		if !missed(t, with) {
			t.Errorf("child %v: a process that could not take its row left no missed mark", child)
		}
		if got := read(t, file); got != staged {
			t.Errorf("child %v: the file was written: %q", child, got)
		}
	}
}

// TestANameNothingTakesWritesNoFile: the harness then sees the file as it was,
// and compiles the row instead.
func TestANameNothingTakesWritesNoFile(t *testing.T) {
	file, with := row(t, "nothing-embeds-this.sh", []byte("x\n"))
	refused := errors.New("not embedded")
	if err := Take(func(string, []byte) error { return refused }); !errors.Is(err, refused) {
		t.Errorf("Take answered %v, want take's refusal", err)
	}
	if got := read(t, file); got != staged {
		t.Errorf("a file nothing took was written: %q", got)
	}
	if !missed(t, with) {
		t.Error("a name nothing took left no missed mark")
	}
}

// TestNoRowNamedTakesNothing: an ordinary run of a test binary changes nothing.
func TestNoRowNamedTakesNothing(t *testing.T) {
	t.Setenv(NameEnv, "")
	called := false
	if err := Take(func(string, []byte) error { called = true; return nil }); err != nil || called {
		t.Fatalf("with no row named, Take answered %v and called take: %v", err, called)
	}
}

// TestWithFileChangesOneFileAndKeepsTheRest: a copy that lost a file would fail
// a case for a reason the row did not cause.
func TestWithFileChangesOneFileAndKeepsTheRest(t *testing.T) {
	tree := fstest.MapFS{
		"adapters/muse/probe.sh":   {Data: []byte("old probe\n"), Mode: 0o444},
		"adapters/muse/install.sh": {Data: []byte("install\n"), Mode: 0o444},
		"adapters/pi/probe.sh":     {Data: []byte("pi\n"), Mode: 0o444},
	}
	got, err := WithFile(tree, "adapters/muse/probe.sh", []byte("new probe\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"adapters/muse/probe.sh": "new probe\n", "adapters/muse/install.sh": "install\n", "adapters/pi/probe.sh": "pi\n"}
	if len(got) != len(want) {
		t.Fatalf("the copy holds %d files, want %d", len(got), len(want))
	}
	for name, body := range want {
		if f, ok := got[name]; !ok || string(f.Data) != body || f.Mode != 0o444 {
			t.Errorf("%s reads %v in the copy, want %q with mode 0444", name, f, body)
		}
	}
	if string(tree["adapters/muse/probe.sh"].Data) != "old probe\n" {
		t.Error("the tree itself was changed, not a copy")
	}
	if _, err := WithFile(tree, "adapters/muse/none.sh", nil); err == nil {
		t.Error("a path the tree does not hold was added to it")
	}
}
