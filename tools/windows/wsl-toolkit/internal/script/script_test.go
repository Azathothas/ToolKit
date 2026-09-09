package script

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The reconstruction is the whole reason the stored copy may differ from the
// product at all. If it stopped being reversible the executable would run a
// script that is not the released one, and nothing else would say so.
func TestStoredCopyReconstructsTheProduct(t *testing.T) {
	if !StoredIsReversible() {
		t.Fatal("the stored copy carries a lone carriage return, so LF to CRLF is no longer its inverse")
	}
	tracked := filepath.Join("..", "..", "..", "..", "..",
		"scripts", "windows", "wsl-toolkit", "wsl-toolkit.ps1")
	want, err := os.ReadFile(tracked)
	if err != nil {
		t.Fatalf("the tracked product is what this is reconstructing: %v", err)
	}
	got := Bytes()
	if !bytes.Equal(got, want) {
		t.Fatalf("reconstructed %d bytes, the tracked product is %d; run build.ps1", len(got), len(want))
	}
	if Size() != len(want) {
		t.Fatalf("Size reports %d over %d bytes", Size(), len(want))
	}
}

func TestBytesCannotBeMutatedThroughItsResult(t *testing.T) {
	first := Bytes()
	if len(first) == 0 {
		t.Fatal("the embedded product is empty")
	}
	first[0] ^= 0xFF
	if second := Bytes(); second[0] == first[0] {
		t.Fatal("a caller mutated the package's own copy")
	}
}

func TestVersionIsReadFromTheScriptRatherThanDeclaredHere(t *testing.T) {
	v, err := Version()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stored, []byte("'"+v+"'")) {
		t.Fatalf("version %q is not the one the script assigns", v)
	}
}

func TestDigestCoversTheProductAndNotTheStoredCopy(t *testing.T) {
	if len(Digest()) != 64 {
		t.Fatalf("digest is %d characters", len(Digest()))
	}
	// A digest taken over the stored bytes would never match a release's
	// SHA256SUMS, and the two are the same length of hex either way, so the
	// only way to tell them apart is to check which object it covers.
	if bytes.Equal(stored, Bytes()) {
		t.Skip("the product and the stored copy are identical on this tree, so this case cannot discriminate")
	}
	if Digest() == digestOf(stored) {
		t.Fatal("Digest covers the stored copy rather than the product")
	}
}
