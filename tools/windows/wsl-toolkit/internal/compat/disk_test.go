// SPDX-License-Identifier: 0BSD

package compat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- the architecture table -------------------------------------------------------

func TestConvertToOciArchMapsBothEnginesAndPassesTheUnknownThrough(t *testing.T) {
	cases := map[string]string{
		"x86_64": "amd64", "x86-64": "amd64", "AMD64": "amd64",
		"aarch64": "arm64", "armv8l": "arm64",
		"armv7l": "arm", "armhf": "arm", "armv6l": "arm",
		"i686": "386", "i386": "386", "x86": "386",
		// An unrecognised value passes through lowercased: a riscv64 host is a
		// legitimate answer this table has not been taught, and guessing a
		// substitute would be worse than handing the engine a name it can
		// refuse.
		"riscv64": "riscv64",
		"RISCV64": "riscv64",
	}
	for raw, want := range cases {
		if got := convertToOciArch(raw); got != want {
			t.Errorf("convertToOciArch(%q) = %q, want %q", raw, got, want)
		}
	}
}

// -- the OCI env script -------------------------------------------------------------

func TestTheOciEnvScriptCarriesEnvAndWorkdirAndNothingElse(t *testing.T) {
	cfg := imageConfig{
		Env: []string{
			"PATH=/usr/local/bin:/usr/bin",
			"LANG=C.UTF-8",
			"BROKEN-NAME=x", // not an identifier: skipped, never fatal
			"NOEQUALS",      // malformed: skipped
		},
		WorkingDir: "/work",
	}
	got := newOciEnvScript(cfg, "alpine:3.22")
	for _, want := range []string{
		"export PATH='/usr/local/bin:/usr/bin'",
		"export LANG='C.UTF-8'",
		"cd '/work' 2>/dev/null || :",
		"alpine:3.22",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the script is missing %q:\n%s", want, got)
		}
	}
	// USER and ENTRYPOINT ARE NOT carried: WSL fixes the login user at import
	// time and a login shell has no entrypoint to run, and writing either
	// into profile.d would be a setting that looks like it works.
	if strings.Contains(got, "USER") {
		t.Errorf("the script carried a USER-ish value:\n%s", got)
	}
	// A root WorkingDir is a no-op, not a cd.
	if got := newOciEnvScript(imageConfig{WorkingDir: "/"}, "x"); strings.Contains(got, "cd ") {
		t.Errorf("a root workdir produced a cd:\n%s", got)
	}
}

// -- snapshots -----------------------------------------------------------------------

func TestASnapshotTagIsAFileNameOrARefusal(t *testing.T) {
	for _, tag := range []string{"ready", "Ready-1.tar-x", strings.Repeat("a", 64)} {
		if got, err := snapshotTag(tag); err != nil || got != tag {
			t.Errorf("snapshotTag(%q) = %q, %v", tag, got, err)
		}
	}
	for _, tag := range []string{"", "   ", "/etc/passwd", "../escape", "-lead", strings.Repeat("a", 65)} {
		if _, err := snapshotTag(tag); err == nil {
			t.Errorf("snapshotTag(%q) was accepted", tag)
		}
	}
	// The reserved device names are refused BY NAME, because 'nul' silently
	// discards everything written to it and the caller would believe they had
	// a snapshot. The WHOLE tag is matched, the way the script matched it: a
	// tag of NUL.txt is a real file name, which is why it stays legal here
	// while the SINK check strips extensions.
	for _, tag := range []string{"con", "prn", "aux", "com1", "LPT9"} {
		if _, err := snapshotTag(tag); err == nil || !strings.Contains(err.Error(), "reserved device") {
			t.Errorf("the reserved name %q was accepted: %v", tag, err)
		}
	}
}

// -- origin records ---------------------------------------------------------------------

func TestAnOriginRecordRoundTripsAndRefusesToGuess(t *testing.T) {
	s := sessionFor(t, "-Action", "List")
	distro := "eph-alpine-3-22-a1b2"
	// In a live New the directory exists before this runs, because the import
	// made it; the test mirrors that order.
	if err := os.MkdirAll(filepath.Join(s.baseDir, distro), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.writeDistroOrigin(distro, "alpine:3.22"); err != nil {
		t.Fatal(err)
	}
	rec := s.readDistroOrigin(distro)
	if rec == nil || rec.Image != "alpine:3.22" {
		t.Fatalf("the record did not round-trip: %+v", rec)
	}

	// A distro with no record is NOTHING, never a guess: treating "no record"
	// as "matches whatever you asked for" would run a caller's command in a
	// distribution built from something else.
	if got := s.readDistroOrigin("eph-other-1a2b"); got != nil {
		t.Errorf("an absent record was invented: %+v", got)
	}

	// A truncated write is nothing, not a broken tool.
	path := s.originPath(distro)
	if err := os.WriteFile(path, []byte("{\"schema\":\"wsl-toolkit-orig"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := s.readDistroOrigin(distro); got != nil {
		t.Errorf("a truncated record was parsed: %+v", got)
	}

	// A record from something else is nothing.
	if err := os.WriteFile(path, []byte(`{"schema":"someone-else/1","image":"alpine:3.22"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := s.readDistroOrigin(distro); got != nil {
		t.Errorf("a foreign record was parsed: %+v", got)
	}

	// ⛔ THE PATH IS CONTAINED: a distro name carrying traversal is refused by
	// the same guard a deletion goes through.
	if err := s.writeDistroOrigin("../escape", "x"); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("an origin record was written outside the base directory: %v", err)
	}
}

// -- disk space --------------------------------------------------------------------------

func TestTheSpacePreflightRefusesAndSaysWhatIsTrue(t *testing.T) {
	// THE NUMBERS ARE THE MEASUREMENT: the floor dominates, so an 8 MiB
	// rootfs still costs 76 MiB, and both constants sit ABOVE every measured
	// import rather than fitted to them.
	tar := int64(8) << 20
	need := tar*importSpaceFactor + importSpaceFloor
	if need <= importSpaceFloor {
		t.Fatal("the multiple contributes nothing; the factor regressed to zero")
	}

	// A volume with less than the need is refused before an import can leave a
	// partial disk and a registered distro that does not work.
	note, refuse := spaceVerdict(need, importSpaceFloor-1, true, "C:/state/distro")
	if note != "" || refuse == nil || !strings.Contains(refuse.Error(), "NOT ENOUGH DISK SPACE") {
		t.Fatalf("the refusal is missing or misnamed: %q, %v", note, refuse)
	}
	if !strings.Contains(refuse.Error(), "Nothing has been imported and nothing is registered") {
		t.Errorf("the refusal does not say what is still true: %v", refuse)
	}

	// ⛔ AN UNMEASURABLE VOLUME IS THE THIRD ANSWER: the preflight is SAID to
	// be skipped and the import is still attempted, because a skipped
	// preflight is not a passing one.
	note, refuse = spaceVerdict(need, 0, false, "C:/state/distro")
	if refuse != nil || !strings.Contains(note, "without the preflight") {
		t.Fatalf("an unmeasurable volume gave %q, %v", note, refuse)
	}

	// Enough space says nothing and refuses nothing.
	note, refuse = spaceVerdict(need, need+1, true, "C:/state/distro")
	if note != "" || refuse != nil {
		t.Fatalf("enough space gave %q, %v", note, refuse)
	}
}
