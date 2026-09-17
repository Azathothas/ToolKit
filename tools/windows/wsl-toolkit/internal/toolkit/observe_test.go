// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"strings"
	"testing"
)

// ⛔ EVERY CASE HERE READS A FUNCTION WITH NO PROCESS IN IT, on purpose. The
// reason is measured twice in this repository: finding 42 is a case that bound
// to the host it was written on, passed here and went red in `golang:1.25`, and
// it happened again in the very commit whose comment claimed to have applied
// the lesson. A rule that can only be reached by starting a container is a rule
// with no case on the host that would catch a mistake in it.

// TestAFeedThatDoesNotExistReportsAbsentAndNotZero is WSL-59's rule, and it is
// the one the whole adapter seam exists for.
//
// ⛔ MEASURED ON THIS BASE 2026-09-17, against a container running `sleep`:
// `podman stats` answers `35048.23%` of a processor and `0B / 33.44GB`. Both
// are parseable, neither is a measurement, and a report carrying them is worse
// than one carrying nothing, because a blank gets checked and a number gets
// used. The discriminator is the container's own cgroup path.
func TestAFeedThatDoesNotExistReportsAbsentAndNotZero(t *testing.T) {
	for _, tc := range []struct {
		name      string
		probe     string
		wantFeed  FeedState
		wantRes   bool
		wantState string
	}{
		{
			// The real base: the container sits in the ROOT cgroup, so podman
			// accounts for nothing and prints nonsense anyway.
			name:      "a container in the root cgroup",
			probe:     "STATE|running|0|false|/\nSTATS|35048.23%|0B / 33.44GB\n",
			wantFeed:  FeedAbsent,
			wantRes:   false,
			wantState: "running",
		},
		{
			// A base that DOES delegate: the figures mean something and are kept.
			name:      "a container with a cgroup of its own",
			probe:     "STATE|running|0|false|/libpod_parent/libpod-abc\nSTATS|12.5%|598kB / 67.11MB\n",
			wantFeed:  FeedPresent,
			wantRes:   true,
			wantState: "running",
		},
		{
			// podman said nothing, which for `inspect` means it is not there.
			// ⚠ That is a MEASUREMENT, so the lifecycle feed is present.
			name:      "a container that is gone",
			probe:     "STATE|\nSTATS|\n",
			wantFeed:  FeedAbsent,
			wantRes:   false,
			wantState: "gone",
		},
		{
			// A cgroup path that could not be read is UNKNOWN, never absent:
			// reporting the lack of a measurement as a measurement is inventing
			// the finding.
			name:      "a cgroup path that came back empty",
			probe:     "STATE|running|0|false|\nSTATS|1%|1kB / 2kB\n",
			wantFeed:  FeedUnknown,
			wantRes:   false,
			wantState: "running",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := TickFacts{Kind: "container", State: "unknown", Feeds: map[Feed]FeedState{}}
			readContainerProbe(&f, tc.probe)
			if f.State != tc.wantState {
				t.Fatalf("state is %q, want %q", f.State, tc.wantState)
			}
			if got := f.Feeds[FeedResources]; got != tc.wantFeed {
				t.Fatalf("the resource feed is %q, want %q", got, tc.wantFeed)
			}
			if tc.wantRes && f.Resources == nil {
				t.Fatal("the feed is present and nothing was kept")
			}
			// ⛔ THE HALF THAT MATTERS. A figure kept where the feed is not
			// present is the defect, and it parses perfectly.
			if !tc.wantRes && f.Resources != nil {
				t.Fatalf("a reading was kept over a feed that is %q: %+v", f.Feeds[FeedResources], *f.Resources)
			}
		})
	}
}

// TestTheHeartbeatSaysWhichKindOfThingItIsWatching. The line read "distro"
// whatever it was watching, and the first container it watched would have been
// reported as a distribution, in a state a distribution cannot be in.
func TestTheHeartbeatSaysWhichKindOfThingItIsWatching(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"container", "container"},
		{"distro", "distro"},
		{"", "unknown-kind"},
	} {
		if got := kindWord(tc.in); got != tc.want {
			t.Fatalf("kindWord(%q) is %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestTheResourceColumnSaysWhyItIsEmpty. A missing column reads as "nobody
// asked"; the reason is the adapter's, because "no per-container cgroup here"
// is true of a container and the wrong subject for a distribution.
func TestTheResourceColumnSaysWhyItIsEmpty(t *testing.T) {
	absent := TickFacts{Feeds: map[Feed]FeedState{FeedResources: FeedAbsent}, ResourceNote: "the reason"}
	if got := resourceText(absent); !strings.Contains(got, "absent") || !strings.Contains(got, "the reason") {
		t.Fatalf("an absent feed reads %q", got)
	}
	unknown := TickFacts{Feeds: map[Feed]FeedState{FeedResources: FeedUnknown}}
	if got := resourceText(unknown); !strings.Contains(got, "unknown") {
		t.Fatalf("an unknown feed reads %q", got)
	}
	// ⛔ ABSENT AND UNKNOWN MUST NOT RENDER THE SAME. They are different
	// answers, and a reader who cannot tell them apart has been told neither.
	if resourceText(absent) == resourceText(unknown) {
		t.Fatal("absent and unknown render identically")
	}
	cpu, mem := 12.5, int64(598000)
	present := TickFacts{Feeds: map[Feed]FeedState{FeedResources: FeedPresent},
		Resources: &ResourceReading{CPUPercent: &cpu, MemoryBytes: &mem}}
	if got := resourceText(present); !strings.Contains(got, "cpu 12.5%") || !strings.Contains(got, "mem ") {
		t.Fatalf("a present feed reads %q", got)
	}
	// A feed nobody asked about prints nothing at all.
	if got := resourceText(TickFacts{}); got != "" {
		t.Fatalf("an unasked feed reads %q", got)
	}
}

// TestAContainerIsNotDiagnosedInADistributionsWords.
//
// ⛔ THE SENTENCES ARE NOT SHARED. DiagnoseExit explains a signal by asking
// whether the WSL virtual machine went away, which is useful about a
// distribution and simply the wrong subject for a container.
func TestAContainerIsNotDiagnosedInADistributionsWords(t *testing.T) {
	c := &ContainerObserver{Name: "wtk-x"}
	rootCgroup := TickFacts{State: "exited", Feeds: map[Feed]FeedState{FeedResources: FeedAbsent}}
	got := c.Diagnose(137, rootCgroup)
	// ⭐ THE ONE THING A READER OF A 137 HERE HAS TO KNOW: on a base with no
	// per-container cgroup the engine's OOMKilled flag is false whatever
	// happened, so "not OOMKilled" is not evidence.
	if !strings.Contains(got, "CANNOT be told apart") {
		t.Fatalf("a 137 on a base with no cgroup reads %q", got)
	}
	if strings.Contains(got, "virtual machine") {
		t.Fatalf("the container diagnosis borrowed the distribution's words: %q", got)
	}
	oom := TickFacts{State: "exited", OOMKilled: true, Feeds: map[Feed]FeedState{FeedResources: FeedPresent}}
	if got := c.Diagnose(137, oom); !strings.Contains(got, "OOMKilled") {
		t.Fatalf("an engine-reported OOM kill reads %q", got)
	}
	if got := c.Diagnose(0, rootCgroup); got != "" {
		t.Fatalf("exit 0 was diagnosed: %q", got)
	}
	if got := c.Diagnose(37, rootCgroup); !strings.Contains(got, "payload's own") {
		t.Fatalf("an ordinary exit reads %q", got)
	}
}

// TestADistributionKeepsItsOwnDiagnosisAndItsDiskFeed. The adapter for a
// distribution is what RunLog always did, moved behind the seam rather than
// rewritten, and this holds it to that.
func TestADistributionKeepsItsOwnDiagnosisAndItsDiskFeed(t *testing.T) {
	size := int64(1234)
	d := &DistroObserver{Name: "eph-x", Read: func() TickFacts {
		return TickFacts{State: "running", DiskBytes: &size}
	}}
	if d.Kind() != "distro" {
		t.Fatalf("kind is %q", d.Kind())
	}
	f := d.Facts()
	if f.Kind != "distro" {
		t.Fatalf("the facts do not name the kind: %+v", f)
	}
	// ⛔ A DISTRIBUTION HAS A DISK AND A CONTAINER DOES NOT, which is why the
	// disk is a feed. The heartbeat said `disk unreadable` about a container
	// until 2026-09-17: a claim that the figure was sought over a thing with no
	// disk of its own to seek.
	if f.Feeds[FeedDisk] != FeedPresent {
		t.Fatalf("a distribution's disk feed is %q", f.Feeds[FeedDisk])
	}
	if (&ContainerObserver{}).Kind() != "container" {
		t.Fatal("the container adapter does not name its kind")
	}
	if got := d.Diagnose(139, TickFacts{State: "stopped"}); !strings.Contains(got, "virtual machine") {
		t.Fatalf("a distribution lost its own diagnosis: %q", got)
	}
}

// TestTheProbeNeverAsksPodmanForAWindowItCannotAnswer.
//
// ⛔ MEASURED 2026-09-17: `podman events --since 5m --until 0s` returns ZERO
// lines and exits 0, because --until is an INSTANT and not a duration back from
// now; `--until 1h` blocks until killed. One exits 0 having done nothing, the
// other reads with no deadline, and this repository refuses both.
func TestTheProbeNeverAsksPodmanForAWindowItCannotAnswer(t *testing.T) {
	script := string((&ContainerObserver{Name: "wtk-abc"}).containerProbe())
	if strings.Contains(script, "--until") {
		t.Fatalf("the probe reaches for --until:\n%s", script)
	}
	// It must still ask about the right container, or it is asking nothing.
	if !strings.Contains(script, "wtk-abc") {
		t.Fatalf("the probe does not name its container:\n%s", script)
	}
	// ⭐ ONE ROUND TRIP. Each crosses wsl.exe, and a second question would
	// double what a heartbeat costs.
	if n := strings.Count(script, "podman "); n != 2 {
		t.Fatalf("the probe runs podman %d times and one heartbeat is worth two:\n%s", n, script)
	}
}

// TestTheFeedSummaryIsOrdered. A map's iteration order would reorder the column
// between two ticks of one run, and a reader comparing two lines would read the
// difference as a change.
func TestTheFeedSummaryIsOrdered(t *testing.T) {
	feeds := map[Feed]FeedState{
		FeedExit: FeedPresent, FeedResources: FeedAbsent,
		FeedLifecycle: FeedPresent, FeedOutput: FeedPresent, FeedDisk: FeedAbsent,
	}
	want := "lifecycle present, output present, resources absent, disk absent, exit present"
	for i := 0; i < 8; i++ {
		if got := FeedSummary(feeds); got != want {
			t.Fatalf("run %d: %q, want %q", i, got, want)
		}
	}
	if got := FeedSummary(nil); got != "" {
		t.Fatalf("no feeds reads %q", got)
	}
}

// TestASizeIsReadInTheUNITSTheEnginePrinted.
//
// ⛔ THE TWO FAMILIES MEAN DIFFERENT NUMBERS AND BOTH ARE IN USE. podman's
// MemUsage goes through docker's units.HumanSize, which is DECIMAL: the
// `33.44GB` measured on this host is 33,440,000,000 bytes, 31.1 GiB on a 32 GiB
// machine. Reading it as binary reports a tenth less memory than there is.
func TestASizeIsReadInTheUNITSTheEnginePrinted(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int64
		ok   bool
	}{
		{"0B", 0, true},
		{"598kB", 598000, true},
		{"33.44GB", 33440000000, true},
		{"1KiB", 1024, true},
		{"1MiB", 1048576, true},
		{" 12.5 MB ", 12500000, true},
		{"", 0, false},
		{"lots", 0, false},
		{"12ZB", 0, false},
	} {
		got, ok := ParseHumanBytes(tc.in)
		if ok != tc.ok {
			t.Fatalf("ParseHumanBytes(%q) ok=%v, want %v", tc.in, ok, tc.ok)
		}
		if ok && got != tc.want {
			t.Fatalf("ParseHumanBytes(%q) is %d, want %d", tc.in, got, tc.want)
		}
	}
}
