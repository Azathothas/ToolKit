// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// -- WSL-109, issue 37 item 5: the space a base frees ------------------------

func TestSparseVhdIsReadFromItsOwnSectionAndTheLastLineWins(t *testing.T) {
	body := "[wsl2]\nsparseVhd=true\n[experimental]\n#sparseVhd=true\n; sparseVhd=true\nsparseVhd = false # for now\nSPARSEVHD=True\n"
	if got := parseWslConfigKey(body, "experimental", "sparseVhd"); got != "true" {
		t.Fatalf("sparseVhd read %q; want the last live line under [experimental]", got)
	}
	if got := parseWslConfigKey("[experimental]\n#sparseVhd=true\n", "experimental", "sparseVhd"); got != "" {
		t.Fatalf("a commented setting read as %q", got)
	}
	if got := parseWslConfigMode("[wsl2]\nnetworkingMode=mirrored\n[experimental]\nnetworkingMode=nat\n"); got != "mirrored" {
		t.Fatalf("networkingMode read %q through the shared reader", got)
	}
}

// ⭐ The operator's own fstrim answer, from issue 37, and busybox's spelling.
func TestTrimmedBytesAreReadInBothSpellings(t *testing.T) {
	if n, ok := parseTrimmed("/: 991 GiB (1064071028736 bytes) trimmed on /dev/sdc\n"); !ok || n != 1064071028736 {
		t.Fatalf("util-linux's line read as %d, %v", n, ok)
	}
	if n, ok := parseTrimmed("/: 4096 bytes trimmed\n"); !ok || n != 4096 {
		t.Fatalf("busybox's line read as %d, %v", n, ok)
	}
	if _, ok := parseTrimmed("fstrim: /: the discard operation is not supported\n"); ok {
		t.Fatal("a refusal read as a figure")
	}
}

func TestALeftoverAdapterIsOneThatIsInstalledOrTakesSpace(t *testing.T) {
	if l, ok := parseLeftover("pi", "installed yes\nfootprint 471859200\n"); !ok || l.Bytes != 471859200 || l.Name != "pi" {
		t.Fatalf("an installed pi read as %+v, %v", l, ok)
	}
	if _, ok := parseLeftover("pi", "installed no\nfootprint 0\n"); ok {
		t.Fatal("a pi that left nothing read as a leftover")
	}
	if l, ok := parseLeftover("muse", "installed no\nfootprint 8192\n"); !ok || l.Bytes != 8192 {
		t.Fatalf("a muse that left configuration behind read as %+v, %v", l, ok)
	}
	if _, ok := parseLeftover("omp", "installed yes\n"); ok {
		t.Fatal("a probe that named no footprint read as a measurement")
	}
	r := leftoverRemediation(AdapterLeftover{Name: "pi", Bytes: 471859200})
	if !strings.HasSuffix(r.Command, "base recreate") || !strings.Contains(r.What, "450.0 MiB") || !r.Advisory {
		t.Fatalf("a leftover earned %+v", r)
	}
}

// ⭐ The answer WSL 3.0.3 gave on 2026-10-21 to --set-sparse while its VM held
// the disk, as wsl.exe writes it with WSL_UTF8.
func TestWslHoldingTheDiskIsNamedWithTheOperatorsStep(t *testing.T) {
	for _, code := range []string{"Wsl/Service/ERROR_SHARING_VIOLATION", "Wsl/Service/WSL_E_DISTRO_NOT_STOPPED"} {
		answer := "The operation could not be completed because the VHD is currently in use. To force WSL to stop use: wsl.exe --shutdown\r\nError code: " + code + "\r\n"
		err := sparseFailure("wsl-toolkit-base", answer, "", errors.New("exit status 4294967295"))
		if !errors.Is(err, ErrDiskHeld) {
			t.Fatalf("WSL holding the disk, %s, was classified as %v", code, err)
		}
	}
	if other := sparseFailure("wsl-toolkit-base", "something else", "", errors.New("exit status 1")); errors.Is(other, ErrDiskHeld) {
		t.Fatalf("another failure was classified as the disk being held: %v", other)
	}
	if sparseFailure("wsl-toolkit-base", "", "", nil) != nil {
		t.Fatal("a success answered an error")
	}
	advice := DiskHeldAdvice()
	for _, want := range []string{"wsl --shutdown", "EVERY distribution", "never runs it", "sparseVhd=true under [experimental]"} {
		if !strings.Contains(advice, want) {
			t.Errorf("the advice does not say %q: %s", want, advice)
		}
	}
}

func TestADiskThatIsNotSparseEarnsTheReclaim(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the sparse attribute is read through Windows")
	}
	t.Setenv("USERPROFILE", t.TempDir())
	disk := filepath.Join(t.TempDir(), "ext4.vhdx")
	if err := os.WriteFile(disk, make([]byte, 64<<10), 0o600); err != nil {
		t.Fatal(err)
	}
	st := BaseState{DiskPath: disk}
	r, ok := diskSparseFacts(&st)
	if !ok || r.ID != "disk-not-sparse" || !strings.HasSuffix(r.Command, "base reclaim") || !r.Advisory {
		t.Fatalf("a disk that is not sparse earned %+v, %v", r, ok)
	}
	if st.DiskSparse != "no" || st.DiskAllocated <= 0 || st.WslConfigSparseVhd != "unset" {
		t.Fatalf("the facts read %q, %d, %q", st.DiskSparse, st.DiskAllocated, st.WslConfigSparseVhd)
	}
}
