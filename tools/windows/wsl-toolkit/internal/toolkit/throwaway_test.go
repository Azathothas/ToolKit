// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAThrowawayNameIsThePrefixAndTheNameAlphabet(t *testing.T) {
	for _, ok := range []string{"eph-a", "eph-alpine-3.22-a1b2", "eph-x_y.z-9", "eph-" + strings.Repeat("a", 56)} {
		if err := ValidThrowawayName(ok); err != nil {
			t.Errorf("%q was refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"", "eph-", "alpine", "EPH-a", "eph-A", "eph-a.", "eph-a..b", "eph--a", "eph-a/b", `eph-a\b`,
		"eph-a b", "eph-" + strings.Repeat("a", 57), "wsl-toolkit", "podman-machine-default",
	} {
		err := ValidThrowawayName(bad)
		if err == nil {
			t.Errorf("%q was accepted", bad)
			continue
		}
		if !errors.Is(err, ErrNotOwned) {
			t.Errorf("%q was refused without ErrNotOwned, so a caller cannot tell a refusal from a failure: %v", bad, err)
		}
	}
}

// TestNoProtectedDistributionCanPassTheNameRule is the proof that the ownership
// check needs no second protected list: every container runtime's distribution
// is refused by the name rule before its disk is looked at.
func TestNoProtectedDistributionCanPassTheNameRule(t *testing.T) {
	for _, p := range ProtectedDistros {
		if ValidThrowawayName(p) == nil || ValidThrowawayName(strings.ToLower(p)) == nil {
			t.Errorf("the protected distribution %q passes the throwaway name rule", p)
		}
	}
	if IsOwnedName(ThrowawayPrefix + "x") {
		t.Error("a throwaway name is also a base or instance name, so the two ownership rules overlap")
	}
}

func TestARequestedNameGetsThePrefixAndNothingElse(t *testing.T) {
	name, drawn, err := ThrowawayName("build-box", "docker.io/library/alpine:3.22")
	if err != nil || drawn || name != "eph-build-box" {
		t.Fatalf("got %q drawn=%v err=%v, want eph-build-box as asked", name, drawn, err)
	}
	if name, _, err := ThrowawayName("eph-already", ""); err != nil || name != "eph-already" {
		t.Errorf("a prefixed name became %q, %v", name, err)
	}
	// ⛔ A NAME IN THE WRONG CASE IS REFUSED RATHER THAN LOWERED: a rewritten
	// name is one the caller cannot find again.
	if _, _, err := ThrowawayName("BuildBox", ""); err == nil {
		t.Error("an upper-case name was accepted rather than refused")
	}
}

func TestADrawnNameCarriesWhatItIsBuiltFromAndARandomSuffix(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		name, drawn, err := ThrowawayName("", "docker.io/library/alpine:3.22")
		if err != nil || !drawn {
			t.Fatalf("a draw failed: %q %v", name, err)
		}
		if !strings.HasPrefix(name, "eph-alpine-3.22-") || len(name) != len("eph-alpine-3.22-")+4 {
			t.Fatalf("the drawn name %q does not carry the repository, the tag and a four-character suffix", name)
		}
		seen[name] = true
	}
	if len(seen) < 150 {
		t.Errorf("200 draws produced %d distinct names, which is not a random suffix", len(seen))
	}
	for from, stem := range map[string]string{
		`C:\images\Rootfs Base.tar.gz`:        "rootfs-base",
		"ghcr.io/void-linux/void-musl:latest": "void-musl-latest",
		"":                                    "rootfs",
		"::::":                                "rootfs",
		"ready":                               "ready",
		"a..b--c":                             "a.b-c",
	} {
		if got := throwawayStem(from); got != stem {
			t.Errorf("throwawayStem(%q) = %q, want %q", from, got, stem)
		}
	}
}

func TestOwnershipIsWhereTheDiskLivesAndNotTheName(t *testing.T) {
	root := t.TempDir()
	distros := filepath.Join(root, "distros")
	if err := os.MkdirAll(filepath.Join(distros, "eph-mine"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := throwawayOwnership("eph-mine", filepath.Join(distros, "eph-mine"), distros); err != nil {
		t.Errorf("a distribution whose disk is inside the state directory was refused: %v", err)
	}
	for label, disk := range map[string]string{
		"a sibling whose name starts with the directory's": filepath.Join(root, "distrosX", "eph-mine"),
		"somewhere else entirely":                          filepath.Join(root, "wsl-ephemeral", "eph-mine"),
		"the directory itself":                             distros,
		"nothing recorded":                                 "",
		"a traversal out of it":                            filepath.Join(distros, "..", "eph-mine"),
	} {
		err := throwawayOwnership("eph-mine", disk, distros)
		if err == nil {
			t.Errorf("%s was accepted as owned: %s", label, disk)
			continue
		}
		if !errors.Is(err, ErrNotOwned) {
			t.Errorf("%s was refused without ErrNotOwned: %v", label, err)
		}
	}
	if err := throwawayOwnership("not-a-throwaway", filepath.Join(distros, "eph-mine"), distros); err == nil {
		t.Error("a name without the prefix was owned because its disk was in the right place")
	}
}

func TestWslsRecordedDirectoryBecomesAnOrdinaryPath(t *testing.T) {
	want := filepath.Clean(`D:\state\wsl-toolkit\distros\eph-a`)
	if got := cleanBasePath(`\\?\D:\state\wsl-toolkit\distros\eph-a\`); got != want {
		t.Errorf("cleanBasePath = %q, want %q", got, want)
	}
	if got := cleanBasePath("   "); got != "" {
		t.Errorf("an empty record became %q", got)
	}
}

func TestASnapshotTagIsAFileNameOrARefusal(t *testing.T) {
	for _, ok := range []string{"ready", "Ready-1.2_x", strings.Repeat("a", 64), "console"} {
		if got, err := SnapshotTag(ok); err != nil || got != ok {
			t.Errorf("SnapshotTag(%q) = %q, %v", ok, got, err)
		}
	}
	for _, bad := range []string{
		"", " ", "../escape", "a/b", `a\b`, ".hidden", "-lead", "trailing.", "a..b", "with space",
		strings.Repeat("a", 65), "nul", "NUL", "nul.tar", "con.x", "com1", "LPT9.backup",
	} {
		if _, err := SnapshotTag(bad); err == nil {
			t.Errorf("the tag %q was accepted", bad)
		}
	}
}

func TestTheImageEnvironmentIsQuotedAndASkippedEntryIsNamed(t *testing.T) {
	profile, carried, skipped := ociEnvProfile(imageConfig{
		Env:        []string{"PATH=/usr/local/bin:/usr/bin", "QUOTE=it's $HOME `x`", "BAD-NAME=x", "NOEQUALS", "=novalue"},
		WorkingDir: "/work dir",
	}, "docker.io/library/alpine:3.22")
	for _, want := range []string{
		"export PATH='/usr/local/bin:/usr/bin'",
		`export QUOTE='it'\''s $HOME ` + "`x`'",
		"cd '/work dir' 2>/dev/null || :",
		"# docker.io/library/alpine:3.22",
	} {
		if !strings.Contains(profile, want) {
			t.Errorf("the profile is missing %q:\n%s", want, profile)
		}
	}
	if strings.Join(carried, ",") != "PATH,QUOTE" {
		t.Errorf("carried %v, want PATH and QUOTE", carried)
	}
	// ⛔ A SKIPPED ENTRY IS NAMED: silence reads as carried.
	if len(skipped) != 3 {
		t.Errorf("skipped %v, want the three unusable entries each named", skipped)
	}
	if strings.Contains(profile, "BAD-NAME") {
		t.Error("an entry that is not a shell name reached the profile")
	}
	if p, _, _ := ociEnvProfile(imageConfig{WorkingDir: "/"}, "x"); strings.Contains(p, "cd ") {
		t.Error("a root working directory produced a cd")
	}
}

func TestPurgeKeepsSnapshotsElsewhereRunningAndCreating(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	started := now.Add(-5 * time.Minute)
	cands := []purgeCandidate{
		{kind: "distro", name: "eph-stopped", path: "d1"},
		{kind: "distro", name: "eph-running", path: "d2", running: true},
		{kind: "distro", name: "eph-creating", path: "d3", creating: &started},
		{kind: "leftover", name: "eph-orphan", path: "eph-orphan.tar"},
		{kind: "leftover", name: "eph-building", path: "eph-building.tar", creating: &started},
		{kind: "leftover", name: ".ready.abc.partial", path: "snapshots/.ready.abc.partial", writing: &started},
		{kind: "snapshot", name: "ready", path: "snapshots/ready.tar"},
		{kind: "elsewhere", name: "eph-pgb", path: `C:\elsewhere\eph-pgb`},
	}
	names := func(cs []purgeCandidate) string {
		var out []string
		for _, c := range cs {
			out = append(out, c.name)
		}
		return strings.Join(out, ",")
	}
	remove, kept := selectPurge(cands, false, now)
	if got := names(remove); got != "eph-stopped,eph-orphan" {
		t.Errorf("without --include-live the purge removes %q, want the stopped distribution and the orphan", got)
	}
	if len(kept) != 6 {
		t.Errorf("kept %d item(s), want 6 each with a reason: %v", len(kept), kept)
	}
	remove, kept = selectPurge(cands, true, now)
	if got := names(remove); got != "eph-stopped,eph-running,eph-creating,eph-orphan,eph-building,.ready.abc.partial" {
		t.Errorf("with --include-live the purge removes %q", got)
	}
	// ⛔ NOTHING MOVES A SNAPSHOT OR A DISTRIBUTION MADE ELSEWHERE INTO A PURGE.
	for _, c := range remove {
		if c.kind == "snapshot" || c.kind == "elsewhere" {
			t.Errorf("--include-live removes %s %s", c.kind, c.name)
		}
	}
	if len(kept) != 2 {
		t.Errorf("with --include-live kept %v, want the snapshot and the distribution made elsewhere", kept)
	}
}

func TestAPartialExportIsALeftoverOnlyOnceNoExportCanBeWritingIt(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	rep := ThrowawayReport{Leftovers: []HeldFile{
		{Kind: "partial", Name: ".fresh.a.partial", Path: "snapshots/.fresh.a.partial", ModTime: now.Add(-time.Minute)},
		{Kind: "partial", Name: ".stale.b.partial", Path: "snapshots/.stale.b.partial", ModTime: now.Add(-snapshotExportTimeout - time.Second)},
		{Kind: "rootfs", Name: "eph-orphan", Path: "eph-orphan.tar", ModTime: now.Add(-time.Minute)},
	}}
	noMarker := func(string) (time.Time, bool) { return time.Time{}, false }
	writing := map[string]bool{}
	for _, c := range purgeCandidates(rep, noMarker, now) {
		writing[c.name] = c.writing != nil
	}
	if !writing[".fresh.a.partial"] {
		t.Error("an export written a minute ago is purged as a leftover, under the export that may still be writing it")
	}
	if writing[".stale.b.partial"] || writing["eph-orphan"] {
		t.Errorf("only a partial export inside the export's own bound is live: %v", writing)
	}
}

func TestADistributionAnotherRunIsCreatingCannotBeRunEnteredOrSnapshotted(t *testing.T) {
	distros, elsewhere := t.TempDir(), t.TempDir()
	started := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	rep := ThrowawayReport{
		Owned: []ThrowawayDistro{
			{Name: "eph-ready", Disk: filepath.Join(distros, "eph-ready")},
			{Name: "eph-building", Disk: filepath.Join(distros, "eph-building"), Creating: &started},
		},
		Elsewhere: []ThrowawayDistro{
			{Name: "eph-pgb", Disk: filepath.Join(elsewhere, "eph-pgb")},
		},
	}
	if d, err := ownedIn(rep, "eph-ready", distros); err != nil || d.Name != "eph-ready" {
		t.Fatalf("a finished distribution this tool owns was refused: %v", err)
	}
	if _, err := ownedIn(rep, "EPH-BUILDING", distros); err == nil || !strings.Contains(err.Error(), "being created by another run") {
		t.Errorf("a distribution another run is still creating was handed out: %v", err)
	}
	if _, err := ownedIn(rep, "eph-pgb", distros); !errors.Is(err, ErrNotOwned) {
		t.Errorf("a distribution whose disk is elsewhere was not refused as not owned: %v", err)
	}
	if _, err := ownedIn(rep, "eph-missing", distros); err == nil {
		t.Error("a name nothing registered was handed out")
	}
}

func TestAnOriginRecordForAnotherNameIsACopyAndReadsAsNothing(t *testing.T) {
	tw := &Throwaways{dir: t.TempDir(), now: time.Now}
	if err := os.MkdirAll(tw.distroDir("eph-a"), 0o700); err != nil {
		t.Fatal(err)
	}
	o := ThrowawayOrigin{Schema: ThrowawayOriginSchema, Name: "eph-a", Image: "docker.io/library/alpine:3.22", Created: time.Now().UTC()}
	if err := tw.writeOrigin(o); err != nil {
		t.Fatal(err)
	}
	if got := tw.readOrigin("eph-a"); got == nil || got.Image != o.Image {
		t.Fatalf("the record did not round trip: %+v", got)
	}
	copied := o
	copied.Name = "eph-b"
	data, _ := json.Marshal(copied)
	if err := os.WriteFile(tw.originPath("eph-a"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := tw.readOrigin("eph-a"); got != nil {
		t.Errorf("a record naming another distribution was read as this one's: %+v", got)
	}
	if err := os.WriteFile(tw.originPath("eph-a"), []byte(`{"schema":"someone-else/1","name":"eph-a"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := tw.readOrigin("eph-a"); got != nil {
		t.Errorf("a record with another schema was read: %+v", got)
	}
}

func TestACreationMarkerProtectsForATimeAndThenDoesNot(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	tw := &Throwaways{dir: t.TempDir(), now: func() time.Time { return now }}
	if err := tw.writeMarker("eph-a"); err != nil {
		t.Fatal(err)
	}
	if _, fresh := tw.creating("eph-a"); !fresh {
		t.Error("a marker written now does not protect what it names")
	}
	tw.now = func() time.Time { return now.Add(throwawayCreatingTTL + time.Second) }
	if _, fresh := tw.creating("eph-a"); fresh {
		t.Error("a marker older than its lifetime still protects, so a killed run blocks purge forever")
	}
	if _, fresh := tw.creating("eph-none"); fresh {
		t.Error("a name with no marker reads as being created")
	}
}

func TestHeldFilesSeparateLeftoversFromSnapshots(t *testing.T) {
	tw := &Throwaways{dir: t.TempDir(), now: time.Now}
	mk := func(rel string, dir bool) {
		p := filepath.Join(tw.dir, rel)
		if dir {
			if err := os.MkdirAll(p, 0o700); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mk("eph-registered", true)
	mk("eph-orphan", true)
	mk("eph-orphan.tar", false)
	mk("eph-orphan.creating", false)
	mk("snapshots/ready.tar", false)
	mk("snapshots/.ready.abc.partial", false)
	mk("unrelated.txt", false)
	leftovers, snaps := tw.heldFiles(map[string]bool{"eph-registered": true})
	kinds := map[string]string{}
	for _, f := range leftovers {
		kinds[f.Kind+":"+f.Name] = f.Path
	}
	for _, want := range []string{"directory:eph-orphan", "rootfs:eph-orphan", "marker:eph-orphan", "partial:.ready.abc.partial"} {
		if kinds[want] == "" {
			t.Errorf("the leftover %s is not reported: %v", want, kinds)
		}
	}
	if len(leftovers) != 4 {
		t.Errorf("reported %d leftover(s), want 4. A registered distribution's directory or an unrelated file is not one: %v", len(leftovers), kinds)
	}
	if len(snaps) != 1 || snaps[0].Name != "ready" {
		t.Errorf("snapshots = %+v, want the one complete archive", snaps)
	}
}

func TestSpecValidationRefusesEveryFlagThatWouldDoNothing(t *testing.T) {
	script := []byte("true\n")
	base := ThrowawaySpec{Image: "docker.io/library/alpine:3.22", User: "root"}
	oneEnv := []EnvPair{
		{Name: "A", Value: "1"},
	}
	badEnv := []EnvPair{
		{Name: "BAD-NAME", Value: "1"},
	}
	cases := map[string]ThrowawaySpec{
		"neither source":           {User: "root"},
		"both sources":             {Image: base.Image, Tarball: "x.tar", User: "root"},
		"reuse without an image":   {Tarball: "x.tar", Reuse: true, User: "root"},
		"reuse and ephemeral":      {Image: base.Image, Reuse: true, Ephemeral: true, Script: script, User: "root"},
		"reuse and systemd":        {Image: base.Image, Reuse: true, Systemd: true, User: "root"},
		"reuse and a name":         {Image: base.Image, Reuse: true, Name: "eph-a", User: "root"},
		"oci-env from an archive":  {Tarball: "x.tar", OciEnv: true, User: "root"},
		"ephemeral with nothing":   {Image: base.Image, Ephemeral: true, User: "root"},
		"env with no command":      {Image: base.Image, Env: oneEnv, User: "root"},
		"user-env with no command": {Image: base.Image, UserEnv: true, User: "root"},
		"a log with no command":    {Image: base.Image, Log: &RunLog{}, User: "root"},
		"a negative timeout":       {Image: base.Image, Script: script, Timeout: -time.Second, User: "root"},
		"a negative tick":          {Image: base.Image, Script: script, Tick: -time.Second, User: "root"},
		"a probe bound too short":  {Image: base.Image, ProbeTimeout: time.Second, User: "root"},
		"an empty user":            {Image: base.Image},
		"a user outside argv":      {Image: base.Image, User: "root$(id)"},
		"an unusable env name":     {Image: base.Image, Script: script, Env: badEnv, User: "root"},
	}
	for label, spec := range cases {
		if err := spec.Validate(); err == nil {
			t.Errorf("%s was accepted", label)
		}
	}
	for label, spec := range map[string]ThrowawaySpec{
		"an image alone":           base,
		"an archive with a script": {Tarball: "x.tar", Script: script, User: "root", Ephemeral: true},
		"reuse with a command":     {Image: base.Image, Reuse: true, Script: script, User: "root"},
	} {
		if err := spec.Validate(); err != nil {
			t.Errorf("%s was refused: %v", label, err)
		}
	}
}

func TestTheWslConfigParserReadsOnlyTheLiveSetting(t *testing.T) {
	for body, want := range map[string]string{
		"":                                    "",
		"[wsl2]\nnetworkingMode=mirrored\n":   "mirrored",
		"# networkingMode=mirrored\n[wsl2]\n": "",
		"[experimental]\nnetworkingMode=mirrored\n[wsl2]\nmemory=4GB\n": "",
		"[wsl2]\nnetworkingMode=mirrored\nnetworkingMode=nat\n":         "nat",
		"[wsl2]\r\nnetworkingMode = Mirrored ; comment\r\n":             "mirrored",
		"[wsl2]\nnetworkingMode=nat mirrored\n":                         "nat",
		"[WSL2]\nnetworkingmode=bridged\n":                              "bridged",
		"[wsl2]\n;networkingMode=mirrored\nnetworkingMode=\n":           "",
	} {
		if got := parseWslConfigMode(body); got != want {
			t.Errorf("parseWslConfigMode(%q) = %q, want %q", body, got, want)
		}
	}
}

func TestTheNatAdapterIsFoundByPrefixAndOnlyWhenUp(t *testing.T) {
	v4 := net.ParseIP("172.23.96.1")
	v6 := net.ParseIP("fe80::1")
	adapters := []adapterInfo{
		{Name: "Ethernet", Up: true, Addrs: []net.IP{net.ParseIP("192.168.1.9")}},
		{Name: "vEthernet (WSL)", Up: false, Addrs: []net.IP{net.ParseIP("10.0.0.1")}},
		{Name: "vEthernet (WSL (Hyper-V firewall))", Up: true, Addrs: []net.IP{v6, v4}},
	}
	addr, name, ok := pickWSLAdapter(adapters)
	if !ok || addr != "172.23.96.1" || name != "vEthernet (WSL (Hyper-V firewall))" {
		t.Errorf("picked %q on %q (ok=%v), want the IPv4 address of the adapter that is up", addr, name, ok)
	}
	if _, _, ok := pickWSLAdapter(adapters[:2]); ok {
		t.Error("an adapter that is down, or one that is not WSL's, was picked")
	}
}
