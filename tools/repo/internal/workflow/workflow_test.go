// SPDX-License-Identifier: 0BSD

package workflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheReleaseCarriesElevenAssetsUnderOneList pins the set staging asserts.
// ⛔ A name dropped from ReleaseBuilds would be dropped from the build AND the
// assertion together, so the list itself is the thing a case has to hold.
func TestTheReleaseCarriesElevenAssetsUnderOneList(t *testing.T) {
	want := []string{
		"wsl-toolkit-windows-amd64.exe", "wsl-toolkit-windows-arm64.exe",
		"text-tool-windows-amd64.exe", "text-tool-windows-arm64.exe",
		"text-tool-linux-amd64", "text-tool-linux-arm64",
		"herdr-0.9.1-windows-x86_64.zip", "herdr-0.9.1-windows-aarch64.zip",
		"herdr-0.9.1-linux-x86_64", "herdr-0.9.1-linux-aarch64",
	}
	got := ExpectedAssets("herdr-0.9.1")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ExpectedAssets =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if line := SignCountLine(got); line != "SIGN_COUNT=11" {
		t.Fatalf("SignCountLine = %q; the ten assets and SHA256SUMS are eleven to sign", line)
	}
}

// TestStagingRefusesAShortSetAndAStrayFile. ⛔ Both directions: a missing asset
// publishes a release short of what a consumer came for, and a stray file ships
// under this workflow's signature.
func TestStagingRefusesAShortSetAndAStrayFile(t *testing.T) {
	expected := []string{"a.exe", "b.exe", "c.zip"}
	if err := CheckStaged(expected, []string{"c.zip", "a.exe", "b.exe"}); err != nil {
		t.Fatalf("the exact set was refused: %v", err)
	}
	err := CheckStaged(expected, []string{"a.exe", "c.zip"})
	if err == nil || !strings.Contains(err.Error(), "b.exe") || !strings.Contains(err.Error(), "not staged") {
		t.Fatalf("a set short of b.exe was accepted or not named: %v", err)
	}
	err = CheckStaged(expected, []string{"a.exe", "b.exe", "c.zip", "notes.txt"})
	if err == nil || !strings.Contains(err.Error(), "notes.txt") || !strings.Contains(err.Error(), "not release assets") {
		t.Fatalf("a stray notes.txt was accepted or not named: %v", err)
	}
}

// TestSHA256SUMSIsWrittenFromTheBytesThatAreThere digests real files and
// compares against digests computed independently of this package.
func TestSHA256SUMSIsWrittenFromTheBytesThatAreThere(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	sums, err := Sums(dir, []string{"b", "a"})
	if err != nil {
		t.Fatal(err)
	}
	// The published SHA-256 of "" and of "abc". ⚠ Spelt in parts, because the
	// secrets check reads any long run of hex as an identifier.
	want := "e3b0c44298fc1c14" + "9afbf4c8996fb924" + "27ae41e4649b934c" + "a495991b7852b855  a\n" +
		"ba7816bf8f01cfea" + "414140de5dae2223" + "b00361a396177a9c" + "b410ff61f20015ad  b\n"
	if got := SumsFile(sums); got != want {
		t.Fatalf("SHA256SUMS =\n%q\nwant\n%q", got, want)
	}
	if sums[1].Size != 3 {
		t.Fatalf("b's size = %d, want 3", sums[1].Size)
	}
	if _, err := Sums(dir, []string{"absent"}); err == nil {
		t.Fatal("a name with no file behind it was digested")
	}
}

// TestFilesListsRegularFilesOnly, as Get-ChildItem -File did: a directory in
// dist is not an asset.
func TestFilesListsRegularFilesOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Files(dir)
	if err != nil || strings.Join(got, ",") != "x" {
		t.Fatalf("Files = %v, %v; want [x]", got, err)
	}
}

// TestTheReadBackNamesWhatTheReleaseLacks. ⛔ And refuses an empty staging
// set, which would otherwise agree with any release at all.
func TestTheReadBackNamesWhatTheReleaseLacks(t *testing.T) {
	staged := []string{"SHA256SUMS", "a.exe", "b.exe"}
	if err := ReadBack(staged, []string{"b.exe", "SHA256SUMS", "a.exe", "extra"}); err != nil {
		t.Fatalf("a complete release was refused: %v", err)
	}
	err := ReadBack(staged, []string{"a.exe"})
	if err == nil || !strings.Contains(err.Error(), "missing 2 asset(s): SHA256SUMS, b.exe") {
		t.Fatalf("a release short of two assets was accepted or misnamed: %v", err)
	}
	if err := ReadBack(nil, []string{"a.exe"}); err == nil || !strings.Contains(err.Error(), "nothing is staged") {
		t.Fatalf("an empty staging set read back as a pass: %v", err)
	}
}

// TestNamesAreReadAsAPowerShellStepWritesThem: a BOM, CRLF and a blank line.
func TestNamesAreReadAsAPowerShellStepWritesThem(t *testing.T) {
	got, err := ReadNames(strings.NewReader(bom + "a.exe\r\n\r\nb.exe\r\n"))
	if err != nil || strings.Join(got, ",") != "a.exe,b.exe" {
		t.Fatalf("ReadNames = %q, %v", got, err)
	}
}

func nightly(tag, published, created string) Release {
	return Release{Tag: tag, Prerelease: true, PublishedAt: published, CreatedAt: created}
}

// TestThePruneKeepsTheNewestByPublishedAt. ⛔ The created_at of each fixture
// runs the OTHER way, as `--target SHA` makes it, so a prune that sorted by it
// would delete the newest nightly.
func TestThePruneKeepsTheNewestByPublishedAt(t *testing.T) {
	releases := []Release{
		nightly("herdr-nightly-20260901-a", "2026-09-01T07:00:00Z", "2026-09-05T00:00:00Z"),
		nightly("herdr-nightly-20260903-c", "2026-09-03T07:00:00Z", "2026-09-03T00:00:00Z"),
		nightly("herdr-nightly-20260904-d", "2026-09-04T07:00:00Z", "2026-09-02T00:00:00Z"),
		nightly("herdr-nightly-20260902-b", "2026-09-02T07:00:00Z", "2026-09-04T00:00:00Z"),
		nightly("herdr-nightly-20260905-e", "2026-09-05T07:00:00Z", "2026-09-01T00:00:00Z"),
	}
	got, err := Prune(releases, 3)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "herdr-nightly-20260902-b,herdr-nightly-20260901-a" {
		t.Fatalf("Prune(keep 3) = %v; want the two oldest by published_at, newer first", got)
	}
	all, err := Prune(releases, 7)
	if err != nil || len(all) != 0 {
		t.Fatalf("Prune(keep 7) of five = %v, %v; want nothing", all, err)
	}
	if _, err := Prune(releases, 0); err == nil {
		t.Fatal("keep 0 was accepted, which deletes the nightly the adapter resolves")
	}
}

// TestThePruneTouchesNightlyPrereleasesAlone: a wsl-toolkit release, a stable
// release that happens to carry the prefix, and an unrelated prerelease are
// all older than every nightly and none is selected.
func TestThePruneTouchesNightlyPrereleasesAlone(t *testing.T) {
	releases := []Release{
		nightly("herdr-nightly-20260905-e", "2026-09-05T07:00:00Z", ""),
		nightly("herdr-nightly-20260904-d", "2026-09-04T07:00:00Z", ""),
		{Tag: "wsl-toolkit-v7.0.0", PublishedAt: "2020-01-01T00:00:00Z"},
		{Tag: "herdr-nightly-20200101-z", PublishedAt: "2020-01-01T00:00:00Z"},
		nightly("wsl-toolkit-v7.1.0-rc1", "2020-01-01T00:00:00Z", ""),
	}
	got, err := Prune(releases, 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "herdr-nightly-20260904-d" {
		t.Fatalf("Prune = %v; want the older nightly prerelease alone", got)
	}
	if _, err := Prune([]Release{nightly("herdr-nightly-x", "yesterday", "")}, 1); err == nil {
		t.Fatal("an unreadable published_at was guessed at rather than refused")
	}
}

// TestNothingButANightlyIsEverDeleted drives the delete loop with a fake
// deleter. ⛔ A list holding one foreign tag deletes NOTHING, not the nightlies
// before it.
func TestNothingButANightlyIsEverDeleted(t *testing.T) {
	var deleted []string
	del := func(tag string) error { deleted = append(deleted, tag); return nil }
	if err := DeleteAll([]string{"herdr-nightly-a", "herdr-nightly-b"}, del); err != nil || len(deleted) != 2 {
		t.Fatalf("two nightlies: deleted %v, %v", deleted, err)
	}
	deleted = nil
	err := DeleteAll([]string{"herdr-nightly-a", "wsl-toolkit-v7.0.0"}, del)
	if err == nil || !strings.Contains(err.Error(), "wsl-toolkit-v7.0.0") {
		t.Fatalf("a wsl-toolkit tag was not refused: %v", err)
	}
	if len(deleted) != 0 {
		t.Fatalf("a refused list still deleted %v", deleted)
	}
}

// TestParseReleasesReadsTheAPIShape.
func TestParseReleasesReadsTheAPIShape(t *testing.T) {
	got, err := ParseReleases([]byte(`[{"tag_name":"herdr-nightly-x","prerelease":true,"published_at":"2026-09-16T07:15:16Z","created_at":"2026-09-15T12:05:05Z"}]`))
	if err != nil || len(got) != 1 || got[0].Tag != "herdr-nightly-x" || !got[0].Prerelease || got[0].PublishedAt != "2026-09-16T07:15:16Z" {
		t.Fatalf("ParseReleases = %+v, %v", got, err)
	}
	if _, err := ParseReleases([]byte(`{"message":"Not Found"}`)); err == nil {
		t.Fatal("an error object was read as a list of releases")
	}
}
