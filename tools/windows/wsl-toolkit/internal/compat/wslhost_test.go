// SPDX-License-Identifier: 0BSD

package compat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- the .wslconfig reader ------------------------------------------------------

func TestTheNetworkingModeReaderAnswersWhatWSLWould(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "home")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USERPROFILE", profile)
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(profile, ".wslconfig"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// No file at all: the documented WSL default, and the answer says where it
	// came from so a caller can tell a measured answer from an assumed one.
	got := networkingMode()
	if got.Mode != "nat" || got.Source != "the WSL default, no .wslconfig" {
		t.Errorf("no file gave %+v", got)
	}

	// ⛔ A COMMENTED SETTING IS NOT A SETTING: real .wslconfig files carry the
	// alternatives commented out above the live one.
	write("# networkingMode=mirrored\n[wsl2]\nnetworkingMode=NAT\n")
	if got := networkingMode(); got.Mode != "nat" || got.Source != ".wslconfig" {
		t.Errorf("a commented alternative overrode the live setting: %+v", got)
	}

	// ⚠ THE SECTION MATTERS: [experimental] carries keys with related names.
	write("[experimental]\nnetworkingMode=mirrored\n[wsl2]\nignored=true\n")
	if got := networkingMode(); got.Mode != "nat" {
		t.Errorf("a key from another section was read: %+v", got)
	}

	// ⚠ LAST ONE WINS, because that is what an ini parser does and what WSL
	// does.
	write("[wsl2]\nnetworkingMode=mirrored\nnetworkingMode=bridged\n")
	if got := networkingMode(); got.Mode != "bridged" {
		t.Errorf("the last value did not win: %+v", got)
	}

	// A trailing comment is not part of the value.
	write("[wsl2]\nnetworkingMode=mirrored ; see docs\n")
	if got := networkingMode(); got.Mode != "mirrored" {
		t.Errorf("the comment became the value: %+v", got)
	}

	// An unreadable file is the default, never a guess.
	if err := os.WriteFile(filepath.Join(profile, ".wslconfig"), []byte("[wsl2]\nnetworkingMode=mirrored\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USERPROFILE", filepath.Join(dir, "absent"))
	if got := networkingMode(); got.Mode != "nat" || got.Path != "" {
		t.Errorf("no profile gave %+v", got)
	}
}

// -- the listing decision --------------------------------------------------------

func TestResolveDistroListingSeparatesRefusalFromEmpty(t *testing.T) {
	// A refusal carries WHY, with the NULs WSL can leave behind stripped.
	_, err := resolveDistroListing(1, []string{"Windows Subsystem for Linux has no installed distributions."},
		"WSL said no\x00")
	if err == nil || !strings.Contains(err.Error(), "WSL said no") {
		t.Fatalf("the refusal lost its reason: %v", err)
	}
	// A refusal with nothing said says the exit code instead of nothing.
	if _, err := resolveDistroListing(2, nil, ""); err == nil || !strings.Contains(err.Error(), "exited 2 and said nothing") {
		t.Fatalf("a silent refusal invented a reason: %v", err)
	}
	// An empty machine is a real answer and stays an empty list.
	names, err := resolveDistroListing(0, []string{"", "\x00"}, "")
	if err != nil || len(names) != 0 {
		t.Fatalf("an empty machine gave %v, %v", names, err)
	}
	// And the NUL belt-and-braces holds for names too.
	names, err = resolveDistroListing(0, []string{"eph-x-1a2b\x00"}, "")
	if err != nil || len(names) != 1 || names[0] != "eph-x-1a2b" {
		t.Fatalf("a NUL-riddled listing gave %v, %v", names, err)
	}
}
