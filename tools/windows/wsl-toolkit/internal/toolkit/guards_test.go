package toolkit

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The cases here are the ones that decide whether a container can reach this
// machine. Each is named for the BEHAVIOUR it asserts rather than the function
// it calls, and each is mutation-proved: deleting the guard it covers turns the
// case red. A test whose name claims more than it checks is a green suite over
// the defect it was written to catch.

func TestAnArchiveEntryThatWouldEscapeTheDestinationIsRefused(t *testing.T) {
	refused := []struct {
		name, why string
	}{
		{"/etc/passwd", "an absolute path writes wherever it says"},
		{`\windows\system32\x`, "a backslash absolute path is absolute too"},
		{"../outside.txt", "one level up is still out"},
		{"a/b/../../../outside.txt", "several levels up, past the root"},
		{"C:/Windows/System32/x", "a drive letter names another volume"},
		{`sub\..\..\escape`, "a backslash separator climbs on Windows semantics"},
		{"logs/CON.jsonl", "a Windows device name swallows what is written to it"},
		{"nul", "the same, bare"},
		{"COM9.txt", "the same, with an extension"},
		{"a\x00b", "a NUL byte truncates the name for whatever reads it next"},
	}
	for _, c := range refused {
		got, err := SafeArchiveName(c.name)
		if err == nil {
			t.Errorf("%q was accepted as %q: %s", c.name, got, c.why)
			continue
		}
		if !errors.Is(err, ErrWorkspaceRefused) {
			t.Errorf("%q was refused with an error that does not wrap ErrWorkspaceRefused: %v", c.name, err)
		}
	}
}

func TestAnOrdinaryArchiveEntryIsAccepted(t *testing.T) {
	accepted := map[string]string{
		"file.txt":        "file.txt",
		"./file.txt":      "file.txt",
		"a/b/c.txt":       filepath.Join("a", "b", "c.txt"),
		"CONSOLE.txt":     "CONSOLE.txt", // ⚠ starts with CON and is not CON
		"a/console/x.txt": filepath.Join("a", "console", "x.txt"),
		"lpt10.txt":       "lpt10.txt", // ⚠ only 1 to 9 are devices
	}
	for name, want := range accepted {
		got, err := SafeArchiveName(name)
		if err != nil {
			t.Errorf("%q was refused: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("%q resolved to %q, expected %q", name, got, want)
		}
	}
}

// ⛔ THE DEVICE-NAME RULE IS ABOUT WINDOWS SEMANTICS WHATEVER HOST IS ASKING.
// A guard that split on the RUNNING platform's separators passed on Windows and
// failed on the ubuntu CI job for the same commit, because a backslash is an
// ordinary character there and `logs\CON.jsonl` has a base name of `logs\CON`.
func TestTheDeviceNameRuleDoesNotDependOnTheHost(t *testing.T) {
	for _, name := range []string{`logs\CON.jsonl`, `a\b\nul`, `deep\path\PRN`} {
		if _, err := SafeArchiveName(name); err == nil {
			t.Errorf("%q was accepted on %s, and it names a Windows device on every host", name, runtime.GOOS)
		}
	}
}

func TestExtractingAHostileArchiveWritesNothingOutsideTheDestination(t *testing.T) {
	dest := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, name := range []string{"../outside.txt", "/absolute.txt", "ok.txt"} {
		body := []byte("payload")
		if err := tw.WriteHeader(&tar.Header{
			Name: name, Typeflag: tar.TypeReg, Mode: 0o644,
			Size: int64(len(body)), ModTime: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	if _, _, err := extractInto(&buf, dest, DefaultWorkspaceLimits()); err == nil {
		t.Fatal("an archive whose first entry climbs out of the destination was accepted")
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("the escaping entry was written")
	}
}

// ⛔ A LINK IS RECORDED AND NEVER RECREATED. Its whole purpose is to point
// somewhere else, and the entry that follows one writes THROUGH it.
// ⛔ BOTH LINK TYPES, and the second one is here because a mutation pass caught
// this case being theatre without it. The first version wrote only a symlink
// entry, so deleting tar.TypeLink from the guard left the suite green: a hard
// link fell through to the default branch and was silently skipped, which is
// safe and is not what this case's name claims. A test whose name claims more
// than it checks is a green suite over the defect it was written to catch.
func TestALinkInAnArchiveBecomesANoteRatherThanALink(t *testing.T) {
	for _, kind := range []struct {
		flag byte
		name string
	}{
		{tar.TypeSymlink, "symbolic"},
		{tar.TypeLink, "hard"},
	} {
		dest := t.TempDir()
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		if err := tw.WriteHeader(&tar.Header{
			Name: "escape", Typeflag: kind.flag, Linkname: "/etc", Mode: 0o777, ModTime: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if _, _, err := extractInto(&buf, dest, DefaultWorkspaceLimits()); err != nil {
			t.Fatalf("%s link: %v", kind.name, err)
		}
		if info, err := os.Lstat(filepath.Join(dest, "escape")); err == nil {
			t.Fatalf("%s link: one was created at %v", kind.name, info.Name())
		}
		note, err := os.ReadFile(filepath.Join(dest, "escape.link.txt"))
		if err != nil {
			t.Fatalf("%s link: it was neither created nor recorded: %v", kind.name, err)
		}
		if !strings.Contains(string(note), "/etc") {
			t.Fatalf("%s link: the note does not say where it pointed: %q", kind.name, note)
		}
	}
}

func TestAnArchiveOverTheSizeLimitIsRefusedRatherThanTruncated(t *testing.T) {
	dest := t.TempDir()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := bytes.Repeat([]byte("x"), 2048)
	if err := tw.WriteHeader(&tar.Header{
		Name: "big.bin", Typeflag: tar.TypeReg, Mode: 0o644,
		Size: int64(len(body)), ModTime: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err := extractInto(&buf, dest, WorkspaceLimits{MaxBytes: 1024, MaxEntries: 10})
	if err == nil {
		t.Fatal("an archive over the byte ceiling was accepted")
	}
	if !errors.Is(err, ErrWorkspaceRefused) {
		t.Fatalf("the refusal does not wrap ErrWorkspaceRefused: %v", err)
	}
}

func TestAnAbsoluteLinkTargetIsNotJoinedOntoTheLinksOwnDirectory(t *testing.T) {
	// ⛔ THIS RUNS EVERYWHERE, and that is the point. The case below it needs
	// a symlink, which Windows will not let this process create, so the defect
	// it covers reached CI. Joining an absolute target onto the link's
	// directory produces a path inside the workspace by every containment test
	// there is, so the link that pointed out of the tree was packed.
	link := filepath.Join("root", "sub", "leak")
	outside := filepath.Join(string(filepath.Separator)+"elsewhere", "secret")
	if got := LinkTargetPath(link, outside); got != outside {
		t.Fatalf("an absolute target became %q, which is under the link's own directory", got)
	}
	rel := LinkTargetPath(link, filepath.Join("..", "sibling"))
	if want := filepath.Join("root", "sibling"); rel != want {
		t.Fatalf("a relative target resolved to %q, expected %q", rel, want)
	}
}

func TestAWorkspaceSymlinkPointingOutOfTheTreeIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Creating a symlink on Windows needs either developer mode or an
		// elevated process, and this asserts a property of the packer rather
		// than of the platform. It runs on the ubuntu CI job, which is the
		// second host every check in this repository earns.
		t.Skip("creating a symlink here needs a privilege this process may not have")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "leak")); err != nil {
		t.Fatal(err)
	}
	_, _, err := writeWorkspaceTar(io.Discard, root, DefaultWorkspaceLimits(), nil)
	if err == nil {
		t.Fatal("a workspace link pointing out of the tree was packed")
	}
	if !errors.Is(err, ErrWorkspaceRefused) {
		t.Fatalf("the refusal does not wrap ErrWorkspaceRefused: %v", err)
	}
}

func TestRemovingSomethingOutsideTheOwnedDirectoryIsRefused(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{victim, outside, root, filepath.Join(root, "..")} {
		if err := RemoveInside(root, target); err == nil {
			t.Errorf("RemoveInside removed %q, which is not under %q", target, root)
		} else if !errors.Is(err, ErrOutsideRoot) {
			t.Errorf("%q was refused for the wrong reason: %v", target, err)
		}
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("the file outside the root is gone: %v", err)
	}
}

func TestRemovingSomethingInsideTheOwnedDirectoryWorksAndIsReadBack(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "job", "work")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveInside(root, filepath.Join(root, "job")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "job")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the directory survived the removal: %v", err)
	}
}

func TestAnArgumentOutsideTheClearedAlphabetIsRefused(t *testing.T) {
	// ⛔ Each of these was measured as unsafe in an argument to wsl.exe: the
	// value is expanded before the guest sees it, and what it expands to is
	// then re-parsed.
	for _, bad := range []string{"$HOME/x", "a`b", `a"b`, "a'b", "a b", "a;b", "a|b", "a&b", "a\nb", ""} {
		if err := AssertArgvSafe([]string{bad}); err == nil {
			t.Errorf("%q was accepted as an argument to wsl.exe", bad)
		}
	}
	for _, ok := range []string{"/home/toolkit/.wsl-toolkit/jobs/abc123", "/bin/tar", "-xf", "-", "a=b,c@d+e%f"} {
		if err := AssertArgvSafe([]string{ok}); err != nil {
			t.Errorf("%q was refused and it is inside the cleared alphabet: %v", ok, err)
		}
	}
}

func TestOnlyTheOwnedDistributionCanBeTouched(t *testing.T) {
	const base = "wsl-toolkit"
	for _, name := range append([]string{"", "eph-something", "Ubuntu", "wsl-toolkit-2"}, ProtectedDistros...) {
		if err := AssertOwnedDistro(name, base); err == nil {
			t.Errorf("AssertOwnedDistro allowed %q while the base is %q", name, base)
		}
	}
	if err := AssertOwnedDistro("wsl-toolkit", base); err != nil {
		t.Errorf("the base itself was refused: %v", err)
	}
	// ⚠ WSL distribution names are compared case-insensitively by wsl.exe, so
	// the guard is too. A guard that was case-sensitive would let
	// WSL-TOOLKIT through as "not the base" and then act on the base.
	if err := AssertOwnedDistro("WSL-TOOLKIT", base); err != nil {
		t.Errorf("the base under another case was refused: %v", err)
	}
	if err := AssertOwnedDistro("PODMAN-MACHINE-DEFAULT", base); err == nil {
		t.Error("a protected name under another case was allowed")
	}
	// ⛔ THE PROTECTED LIST IS ASSERTED BY ITS MESSAGE, because refusal alone is
	// not what it adds. The exact-name rule below it already refuses every one
	// of these names, so a case that only checks for an error stays green with
	// the list deleted -- which is what this case did until the mutation pass
	// planted that defect and read a pass. The list exists so a plausible name
	// is refused with the reason, and the reason is the thing to test.
	for _, name := range ProtectedDistros {
		err := AssertOwnedDistro(name, base)
		if err == nil {
			t.Fatalf("%q was allowed", name)
		}
		if !strings.Contains(err.Error(), "container runtime") {
			t.Errorf("refusing %q says %q, which does not tell the reader it is a container runtime's own distribution", name, err.Error())
		}
	}
	// And the reason has to be specific to those names: an ordinary name that
	// is merely not the base must NOT claim to be a runtime's own.
	err := AssertOwnedDistro("Ubuntu", base)
	if err == nil || strings.Contains(err.Error(), "container runtime") {
		t.Errorf("refusing an ordinary name says %v, which credits it to the protected list", err)
	}
}

func TestAnUnqualifiedImageReferenceIsRefused(t *testing.T) {
	for _, bad := range []string{"", "alpine", "alpine:latest", "library/alpine:latest", "docker.io/", "alpine;rm -rf /", "docker.io/library/alpine latest"} {
		if err := ValidateImageRef(bad); err == nil {
			t.Errorf("%q was accepted, and an engine resolves it through its own alias table", bad)
		}
	}
	for _, good := range []string{
		"docker.io/library/alpine:latest",
		"ghcr.io/pkgforge-dev/archlinux:latest",
		"registry.fedoraproject.org/fedora:latest",
		"localhost/built-here:dev",
		"quay.io/rockylinux/rockylinux:8",
	} {
		if err := ValidateImageRef(good); err != nil {
			t.Errorf("%q was refused: %v", good, err)
		}
	}
}

func TestEveryBuiltinImageAndPresetIsFullyQualified(t *testing.T) {
	if len(BuiltinImages) != 12 {
		t.Fatalf("the catalog carries %d images and the issue that asked for it named twelve", len(BuiltinImages))
	}
	seen := map[string]bool{}
	for _, img := range BuiltinImages {
		if err := img.Validate(); err != nil {
			t.Errorf("catalog image %s: %v", img.ID, err)
		}
		if seen[img.ID] {
			t.Errorf("two catalog images share the id %q", img.ID)
		}
		seen[img.ID] = true
		if img.Libc != "musl" && img.Libc != "glibc" {
			t.Errorf("%s declares libc %q", img.ID, img.Libc)
		}
	}
	for _, p := range BasePresets {
		if err := ValidateImageRef(p.Ref); err != nil {
			t.Errorf("preset %s: %v", p.ID, err)
		}
		if p.Build == "" {
			t.Errorf("preset %s carries no measurement, and a row with no figure must say so rather than being blank", p.ID)
		}
	}
	if _, err := ResolveBasePreset(DefaultBaseImage); err != nil {
		t.Errorf("the default base image is not resolvable as a preset or a reference: %v", err)
	}
}

func TestASelectionThatMatchesNothingIsARefusalRatherThanAnEmptyRun(t *testing.T) {
	cfg := DefaultConfig()
	for _, sel := range []string{"nosuchimage", "libc:nosuch", "kind:nosuch"} {
		if got, err := cfg.SelectImages([]string{sel}); err == nil {
			t.Errorf("%q selected %d image(s), and a fleet over nothing reads exactly like a fleet where everything agreed", sel, len(got))
		}
	}
	musl, err := cfg.SelectImages([]string{"libc:musl"})
	if err != nil {
		t.Fatal(err)
	}
	if len(musl) != 3 {
		t.Fatalf("libc:musl selected %d image(s), and the catalog carries three", len(musl))
	}
	all, err := cfg.SelectImages(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(BuiltinImages) {
		t.Fatalf("an empty selection chose %d of %d", len(all), len(BuiltinImages))
	}
	// A selector naming one image twice yields it once: a fleet that ran a row
	// twice would report two results for one question.
	once, err := cfg.SelectImages([]string{"alpine,alpine", "libc:musl"})
	if err != nil {
		t.Fatal(err)
	}
	if len(once) != 3 {
		t.Fatalf("a repeated selector produced %d rows", len(once))
	}
}

func TestAConfigurationThatWouldLoseTheBaseIsRefused(t *testing.T) {
	// Bound rather than written inline: a Go composite literal that opens with
	// two braces reads as an unfilled template to check-placeholders.
	unqualified := Image{ID: "x", Ref: "alpine"}
	bad := []Config{
		{Base: BaseConfig{Name: "eph-toolkit", Image: DefaultBaseImage, User: "toolkit"}},
		{Base: BaseConfig{Name: "podman-machine-default", Image: DefaultBaseImage, User: "toolkit"}},
		{Base: BaseConfig{Name: "", Image: DefaultBaseImage, User: "toolkit"}},
		{Base: BaseConfig{Name: "wsl toolkit", Image: DefaultBaseImage, User: "toolkit"}},
		{Base: BaseConfig{Name: "wsl-toolkit", Image: DefaultBaseImage, User: "root user"}},
		{Base: BaseConfig{Name: "wsl-toolkit", Image: DefaultBaseImage, User: "toolkit"},
			Images: []Image{unqualified}},
		{Base: BaseConfig{Name: "wsl-toolkit", Image: DefaultBaseImage, User: "toolkit"},
			Matrix: []string{"nosuch"}},
	}
	for i, cfg := range bad {
		if err := cfg.Validate(); err == nil {
			t.Errorf("configuration %d was accepted: %+v", i, cfg.Base)
		}
	}
	if err := DefaultConfig().Validate(); err != nil {
		t.Errorf("the defaults do not validate: %v", err)
	}
}

func TestAHostAuthoredScriptIsRepairedInTheCopyAndUtf16IsRefused(t *testing.T) {
	crlf := []byte("echo one\r\necho two\r\n")
	got, err := RepairGuestScript(crlf)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte("\r")) {
		t.Fatalf("a carriage return survived: %q", got)
	}
	withMark := append([]byte{0xEF, 0xBB, 0xBF}, []byte("echo x\n")...)
	got, err = RepairGuestScript(withMark)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 'e' {
		t.Fatalf("the byte order mark survived: %q", got)
	}
	// ⛔ A lone carriage return is a DELIBERATE byte and is kept. Turning it
	// into a newline would edit the payload rather than repair the copy.
	loneCR := []byte("printf 'a\rb'\n")
	got, err = RepairGuestScript(loneCR)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("a\rb")) {
		t.Fatalf("a lone carriage return was edited out: %q", got)
	}
	utf16le := []byte{0xFF, 0xFE}
	utf16be := []byte{0xFE, 0xFF}
	for _, mark := range [][]byte{utf16le, utf16be} {
		if _, err := RepairGuestScript(append(mark, 'e', 0, 'c', 0)); err == nil {
			t.Errorf("a UTF-16 script was accepted, and /bin/sh stops at its first NUL byte")
		}
	}
	// A script with no trailing newline gets one, because the last line of a
	// script sourced without one is not run by every shell.
	got, err = RepairGuestScript([]byte("echo x"))
	if err != nil {
		t.Fatal(err)
	}
	if got[len(got)-1] != '\n' {
		t.Fatal("the repaired copy does not end in a newline")
	}
}

func TestAValueWithAQuoteInItSurvivesShellQuoting(t *testing.T) {
	cases := []string{`a'b`, `$HOME`, "back`tick`", `a"b`, "tab\there", "new\nline", `';rm -rf /;'`}
	for _, v := range cases {
		q := shellQuote(v)
		if !strings.HasPrefix(q, "'") || !strings.HasSuffix(q, "'") {
			t.Errorf("%q was not single quoted: %s", v, q)
		}
		// Reproduce what a POSIX shell does with the quoted form.
		if got := unquotePOSIX(q); got != v {
			t.Errorf("%q round tripped to %q through %s", v, got, q)
		}
	}
}

// unquotePOSIX reads a string the way /bin/sh does, written independently of
// shellQuote so the two can disagree.
//
// ⚠ THE FIRST VERSION OF THIS HELPER WAS WRONG AND THE CODE WAS RIGHT, which is
// worth the comment: it assumed every quoted run began at the top of the loop,
// so it read the `'` that REOPENS a quote after an escaped one as the start of
// something new and reported a correct value as unquoted. A test that fails for
// its own reason is the harness, and doubting it before the subject is the rule
// this repository states for a suspicious result.
//
// The three states a POSIX shell has for this are: inside single quotes, where
// only `'` is special; outside, where `\` escapes the next character; and the
// character itself.
func unquotePOSIX(q string) string {
	var out strings.Builder
	inQuote := false
	for i := 0; i < len(q); i++ {
		c := q[i]
		switch {
		case inQuote && c == '\'':
			inQuote = false
		case inQuote:
			out.WriteByte(c)
		case c == '\'':
			inQuote = true
		case c == '\\' && i+1 < len(q):
			i++
			out.WriteByte(q[i])
		default:
			out.WriteByte(c)
		}
	}
	if inQuote {
		return "<unterminated>"
	}
	return out.String()
}

func TestEnvironmentBecomesAssignmentsAndAnUnusableNameIsDropped(t *testing.T) {
	got := string(shellAssignments(map[string]string{
		"GOOD": "value with 'quote'", "2BAD": "x", "al so bad": "y", "_ok9": "z",
	}))
	if !strings.Contains(got, "GOOD=") || !strings.Contains(got, "_ok9=") {
		t.Fatalf("a usable name was dropped: %q", got)
	}
	if strings.Contains(got, "2BAD") || strings.Contains(got, "al so bad") {
		t.Fatalf("a name no shell can assign was emitted: %q", got)
	}
	if !strings.Contains(got, "export GOOD") {
		t.Fatalf("the assignment is not exported, so a child would not see it: %q", got)
	}
}

func TestTheLedgerReportsWhatWasOpenedAndNeverClosed(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	led, err := OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []LedgerEntry{
		{Event: "open", Kind: "job", ID: "aaa"},
		{Event: "open", Kind: "job", ID: "bbb"},
		{Event: "close", Kind: "job", ID: "aaa"},
		{Event: "open", Kind: "staging", ID: "ccc"},
	} {
		if err := led.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	open, err := led.Open()
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 {
		t.Fatalf("%d records read as open, expected bbb and ccc", len(open))
	}
	// ⚠ A process killed mid-write leaves a partial last line. That is normal
	// and must not make the whole ledger unreadable, because the ledger is what
	// cleanup reads to find the very resources that run leaked.
	f, err := os.OpenFile(led.Path(), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"schema":"wsl-toolkit-ledg`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	open, err = led.Open()
	if err != nil {
		t.Fatalf("a torn last line made the whole ledger unreadable: %v", err)
	}
	if len(open) != 2 {
		t.Fatalf("%d records read as open after a torn line", len(open))
	}
	n, err := led.Compact()
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("compaction kept %d records", n)
	}
}

func TestAFleetThatRanNothingIsNotAFleetWhereEverythingAgreed(t *testing.T) {
	cases := []struct {
		report MatrixReport
		want   int
	}{
		{MatrixReport{Ran: 0, Failed: 0, Unreached: 0}, 2},
		{MatrixReport{Ran: 0, Unreached: 12}, 2},
		{MatrixReport{Ran: 12, Failed: 0}, 0},
		{MatrixReport{Ran: 11, Failed: 0, Unreached: 1}, 1},
		{MatrixReport{Ran: 12, Failed: 1}, 1},
	}
	for _, c := range cases {
		if got := c.report.Verdict(); got != c.want {
			t.Errorf("ran=%d failed=%d unreached=%d gave %d, expected %d",
				c.report.Ran, c.report.Failed, c.report.Unreached, got, c.want)
		}
	}
}

func TestSizesArePrintedInTheUnitTheyAreLabelledWith(t *testing.T) {
	cases := map[int64]string{
		0: "0 B", 512: "512 B", 1024: "1.0 KiB",
		1536: "1.5 KiB", 1 << 20: "1.0 MiB", 1 << 30: "1.0 GiB",
	}
	for n, want := range cases {
		if got := HumanBytes(n); got != want {
			t.Errorf("%d rendered as %q, expected %q", n, got, want)
		}
	}
}

func TestARealPathResolvesToSomethingThatExists(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	got, err := RealPath(self)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("RealPath returned %q, which cannot be stat'd: %v", got, err)
	}
	if strings.HasPrefix(got, `\\?\`) {
		t.Fatalf("the extended-length prefix survived: %q", got)
	}
	if _, err := RealPath(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a missing path did not report as not existing: %v", err)
	}
}

func TestExcludesAreDeduplicatedAndSplit(t *testing.T) {
	got := SortedExcludes([]string{".git,node_modules", "  .git ", "target", ""})
	want := []string{".git", "node_modules", "target"}
	if len(got) != len(want) {
		t.Fatalf("got %v, expected %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, expected %v", got, want)
		}
	}
}

func TestAGuestPathIsNeverAWindowsPath(t *testing.T) {
	// ⚠ THE ANSWER MUST NOT DEPEND ON THE HOST ASKING. This read a
	// drive-rooted path as a relative name on Linux and prepended a working
	// directory, which the ubuntu CI job caught and this machine could not.
	for _, in := range []string{`C:\projects\subject`, "C:/projects/subject"} {
		got, err := WindowsPathToGuest(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != "/mnt/c/projects/subject" {
			t.Fatalf("%s became %q", in, got)
		}
	}
	got, err := WindowsPathToGuest(`C:\projects\subject`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/mnt/c/projects/subject" {
		t.Fatalf("got %q", got)
	}
	if _, err := WindowsPathToGuest(`\\server\share\x`); err == nil {
		t.Fatal("a UNC path was given a /mnt form, and it has none")
	}
}
