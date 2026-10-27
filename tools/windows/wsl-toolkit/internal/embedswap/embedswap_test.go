// SPDX-License-Identifier: 0BSD

package embedswap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

// row hands the protocol's variables a row's bytes, and answers the file.
func row(t *testing.T, name string, body []byte) string {
	t.Helper()
	dir := t.TempDir()
	with, file := filepath.Join(dir, "with"), filepath.Join(dir, "file")
	if err := os.WriteFile(with, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("as it was\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(NameEnv, name)
	t.Setenv(WithEnv, with)
	t.Setenv(FileEnv, file)
	return file
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestTheBytesReachWhatTakesThemAndThenTheFile is the order the harness's
// receipt rests on: the file says the binary took the bytes.
func TestTheBytesReachWhatTakesThemAndThenTheFile(t *testing.T) {
	file := row(t, "verify.sh", []byte("the row's bytes\n"))
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
	if fileWhenTaken != "as it was\n" {
		t.Errorf("the file held %q before take had the bytes, so a receipt could stand for bytes nothing took", fileWhenTaken)
	}
	if got := read(t, file); got != "the row's bytes\n" {
		t.Errorf("the file was left %q, so a case that reads it saw the old script", got)
	}
}

// TestANameNothingTakesWritesNoFile: the harness then sees the file as it was,
// and compiles the row instead.
func TestANameNothingTakesWritesNoFile(t *testing.T) {
	file := row(t, "nothing-embeds-this.sh", []byte("x\n"))
	refused := errors.New("not embedded")
	if err := Take(func(string, []byte) error { return refused }); !errors.Is(err, refused) {
		t.Errorf("Take answered %v, want take's refusal", err)
	}
	if got := read(t, file); got != "as it was\n" {
		t.Errorf("a file nothing took was written: %q", got)
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
