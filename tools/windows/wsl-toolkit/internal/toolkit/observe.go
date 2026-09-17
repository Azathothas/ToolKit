// observe.go - the adapter seam between the observation layer and the thing
// being observed.
//
// ⛔ WHAT WAS WRONG. The timestamp layer, the silence heartbeat, the event log
// and the exit-code reading are container-agnostic, and none of them could
// watch a container: `RunLog` asked wsl.exe about a DISTRIBUTION and read a
// `.vhdx` off the host disk, so only `distro new` and `distro run` could feed
// it. A caller running a podman workload through `run` got a byte counter and
// nothing else. WSL-30, WSL-59.
//
// ⭐ THE OBSERVATION LAYER MUST NOT NAME A COMMAND, and that is the rule the
// whole file exists for. `RunLog` renders what an Observer hands it and asks
// neither podman nor wsl.exe anything. Which feeds exist, what they are called
// and how an exit code reads are the ADAPTER's answers, because a distribution
// and a container are not the same kind of thing: they share a kernel and
// nothing else, and one implementation that blurs them produces nonsense about
// both.
//
// ⛔ A FEED THAT DOES NOT EXIST REPORTS ABSENT, NEVER ZERO. Measured on this
// base on 2026-09-17, against a container doing nothing but `sleep`:
// `podman stats` answers `35048.23%` of a processor and `0B / 33.44GB`. Those
// are not small numbers, they are not measurements, and a report carrying them
// is worse than a report carrying nothing, because a blank gets checked and a
// number gets used. WSL-60 closed on exactly that reasoning.
//
// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Feed is one channel of facts about a running thing. They are named so an
// adapter can say a feed is ABSENT rather than answering zero for it.
type Feed string

const (
	// FeedLifecycle is what the thing's own state is called: running, exited,
	// gone.
	FeedLifecycle Feed = "lifecycle"
	// FeedOutput is where a reader can go for the bytes AFTER the run. ⚠ It is
	// not how this tool collects them: a job's streams arrive attached, through
	// the same pipe the command was started on, and that path is never absent.
	FeedOutput Feed = "output"
	// FeedResources is processor and memory accounting.
	FeedResources Feed = "resources"
	// FeedExit is the exit code, read from the thing rather than from the
	// process this side started.
	FeedExit Feed = "exit"
	// FeedDisk is a growing store this side can size without asking the guest.
	//
	// ⛔ IT IS A FEED BECAUSE A CONTAINER DOES NOT HAVE ONE, and the heartbeat
	// said `disk unreadable` about one until 2026-09-17 - which is a claim that
	// the figure was sought and could not be got, over a thing that has no disk
	// of its own to seek. Found by driving it, and it is the same error as
	// reporting 0B for memory: an absence rendered as a failed measurement.
	FeedDisk Feed = "disk"
)

// FeedState is what an adapter says about one feed.
//
// ⛔ ABSENT AND UNKNOWN ARE DIFFERENT, and keeping them apart is the whole
// reason this is not a boolean. Absent is a measurement: this engine, on this
// base, does not have the feed. Unknown is the absence of a measurement, and
// reporting it as absent would be inventing the finding.
type FeedState string

const (
	FeedPresent FeedState = "present"
	FeedAbsent  FeedState = "absent"
	FeedUnknown FeedState = "unknown"
)

// ResourceReading is what the resource feed answered.
//
// ⛔ EVERY FIELD IS A POINTER AND AN UNREAD ONE IS nil. A zero here would be
// indistinguishable from a container genuinely using nothing, which is the
// difference this whole file is about.
type ResourceReading struct {
	CPUPercent  *float64 `json:"cpu_percent,omitempty"`
	MemoryBytes *int64   `json:"memory_bytes,omitempty"`
}

// Observer is what the observation layer asks about the thing a command is
// running in. ⭐ One implementation per KIND of thing, and there are two.
type Observer interface {
	// Kind is the word a reader sees: "distro" or "container".
	Kind() string
	// Facts reads the feeds that are present. ⚠ It is called with the relay's
	// lock RELEASED, because it can take seconds, and a heartbeat that held the
	// lock would stall the command's own output behind itself.
	Facts() TickFacts
	// Diagnose says how an exit code reads for this kind of thing.
	//
	// ⛔ IT IS THE ADAPTER'S BECAUSE THE WORDING IS NOT SHARED. "WSL reports the
	// distribution as stopped, which is consistent with the virtual machine
	// having gone away" is true of a distribution and meaningless about a
	// container, and a layer that wrote one sentence for both would be wrong for
	// whichever it was not written for.
	Diagnose(code int, f TickFacts) string
}

// DistroObserver watches one WSL distribution. It is what RunLog has always
// done, named and moved behind the seam rather than rewritten.
type DistroObserver struct {
	Name string
	Read func() TickFacts
}

func (d *DistroObserver) Kind() string { return "distro" }

func (d *DistroObserver) Facts() TickFacts {
	f := TickFacts{Kind: "distro", State: "unknown"}
	if d.Read != nil {
		f = d.Read()
	}
	f.Kind = "distro"
	if f.ResourceNote == "" {
		f.ResourceNote = "WSL does not account for a distribution separately; the disk figure is what this side can size"
	}
	if f.Feeds == nil {
		// A distribution has a lifecycle and an exit code, and it has no
		// per-container accounting to report at all - the disk figure is a
		// different thing and travels in its own field.
		f.Feeds = map[Feed]FeedState{
			FeedLifecycle: FeedPresent, FeedExit: FeedPresent,
			FeedOutput: FeedPresent, FeedResources: FeedAbsent,
			FeedDisk: FeedPresent,
		}
	}
	return f
}

func (d *DistroObserver) Diagnose(code int, f TickFacts) string {
	return DiagnoseExit(code, f.State)
}

// ContainerLogDriver is the driver a job's container is created with.
//
// ⛔ IT IS NAMED BY THE ADAPTER AND NEVER LEFT TO THE BASE'S DEFAULT, and the
// default is a silent zero. Measured on this base 2026-09-17: under `journald`,
// `podman logs` on a job's container prints 0 bytes on stdout AND 0 on stderr
// and exits 0; under `k8s-file` the same container answers 6 and 6. "Exits 0
// having produced nothing it was asked for" is the first row this repository
// refuses, and a caller who goes to the engine for a job's output must not have
// to know which driver the base happened to be configured with.
//
// ⚠ IT IS NOT HOW THIS TOOL COLLECTS OUTPUT. A job's streams arrive ATTACHED,
// on the pipe `podman run` was started on, and they cannot be lost to a log
// driver at all - which is why the output feed is present whatever this says.
// What this buys is the SECOND route: a container still there after the run.
const ContainerLogDriver = "k8s-file"

// containerProbeBudget bounds one heartbeat's question to the engine.
//
// ⚠ AND THE HEARTBEAT IS WHY ONE IS AFFORDABLE AT ALL. tick.go refuses a poll
// loop against the engine and it is right to: a five-second TickEvent across
// twelve rows would be 2.4 questions a second. This fires only after the
// command has been SILENT for the configured span, which is 30 seconds under a
// rendering profile and off by default, so a quiet job costs the engine two
// questions a minute and a talkative one costs it none.
const containerProbeBudget = 20 * time.Second

// ContainerObserver watches one podman container inside the base.
//
// ⛔ IT IS BUILT PER JOB AND HOLDS NO STATE ABOUT THE BASE. An adapter that
// cached "this base cannot account for containers" would be answering about the
// base a previous job ran on. The discriminator is read per container, and it
// is the container's own cgroup path.
type ContainerObserver struct {
	Name string
	// Ask runs a script in the base and returns its stdout, stderr and exit
	// code. ⛔ THE EXIT CODE COMES FROM THE PROCESS, never through a pipe: a
	// probe written for this file piped podman to `head` and read 0 over an
	// error, which is absolute 5 of this repository's own router.
	Ask func(ctx context.Context, script []byte) (string, string, int, error)
}

func (c *ContainerObserver) Kind() string { return "container" }

// containerProbe is what one heartbeat asks. ONE round trip, because each one
// crosses wsl.exe and a second question would double the cost of a tick.
//
// ⛔ `--until` IS NEVER PASSED TO `podman events`, and nothing here reaches for
// it. Measured on 2026-09-17: `--since 5m --until 0s` returns ZERO lines and
// exits 0, because `--until` is an INSTANT and not a duration back from now, so
// `0s` is an empty window; `--until 1h` blocks until killed, waiting out an hour
// in the future. Both are shapes this repository refuses - one exits 0 having
// done nothing, the other reads with no deadline. This probe asks `inspect`,
// which needs no window at all.
func (c *ContainerObserver) containerProbe() []byte {
	n := shellQuote(c.Name)
	return []byte("" +
		"printf 'STATE|'\n" +
		"podman inspect --type container --format " +
		"'{{.State.Status}}|{{.State.ExitCode}}|{{.State.OOMKilled}}|{{.State.CgroupPath}}' " + n + " 2>/dev/null || printf '\\n'\n" +
		"printf 'STATS|'\n" +
		"podman stats --no-stream --format '{{.CPUPerc}}|{{.MemUsage}}' " + n + " 2>/dev/null || printf '\\n'\n")
}

func (c *ContainerObserver) Facts() TickFacts {
	f := TickFacts{Kind: "container", State: "unknown", Feeds: map[Feed]FeedState{
		// ⭐ THE OUTPUT FEED IS PRESENT AND IT IS NOT `podman logs`. A job's
		// streams arrive attached, on the pipe `podman run` was started on, so
		// they cannot be lost to a log driver. ⚠ `podman logs` for the same
		// container is a different and worse route: the base's default driver
		// is `journald`, under which it prints 0 bytes on both streams and exits
		// 0 - measured 2026-09-17, against 6 bytes each under `k8s-file`.
		FeedOutput:    FeedPresent,
		FeedLifecycle: FeedUnknown,
		FeedExit:      FeedUnknown,
		FeedResources: FeedUnknown,
		// ⛔ A CONTAINER HAS NO DISK OF ITS OWN THIS SIDE CAN SIZE. The figure
		// the heartbeat reports is a distribution's .vhdx, read off the Windows
		// filesystem; a container's layers live inside the base and asking the
		// guest for them is a second round trip for a number nothing acts on.
		FeedDisk: FeedAbsent,
	}}
	if c.Ask == nil {
		return f
	}
	ctx, cancel := context.WithTimeout(context.Background(), containerProbeBudget)
	defer cancel()
	out, _, code, err := c.Ask(ctx, c.containerProbe())
	if err != nil || code != 0 {
		return f
	}
	readContainerProbe(&f, out)
	return f
}

// readContainerProbe fills the facts from the probe's two lines.
//
// ⭐ IT IS A FUNCTION WITH NO PROCESS IN IT, so its case runs on any host. A
// rule that can only be reached by starting a container is a rule with no case,
// which is finding 42's lesson in this repository twice over.
func readContainerProbe(f *TickFacts, out string) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		switch {
		case strings.HasPrefix(line, "STATE|"):
			readContainerState(f, strings.TrimPrefix(line, "STATE|"))
		case strings.HasPrefix(line, "STATS|"):
			readContainerStats(f, strings.TrimPrefix(line, "STATS|"))
		}
	}
}

func readContainerState(f *TickFacts, body string) {
	if strings.TrimSpace(body) == "" {
		// ⭐ podman answered nothing, which for `inspect` means the container is
		// not there. That is a MEASUREMENT and the lifecycle feed is present.
		f.State, f.Feeds[FeedLifecycle] = "gone", FeedPresent
		f.Feeds[FeedResources] = FeedAbsent
		f.ResourceNote = "the container is not there, so there is nothing to account for"
		return
	}
	parts := strings.Split(body, "|")
	if len(parts) < 4 {
		return
	}
	f.State, f.Feeds[FeedLifecycle] = parts[0], FeedPresent
	if n, err := strconv.Atoi(parts[1]); err == nil {
		f.ExitCode, f.Feeds[FeedExit] = &n, FeedPresent
	}
	f.OOMKilled = parts[2] == "true"
	// ⛔ THE ROOT CGROUP IS THE DISCRIMINATOR, AND IT IS READ PER CONTAINER.
	// A container whose cgroup path is `/` is sitting in the root cgroup, which
	// has no per-container accounting by definition, so every figure `podman
	// stats` produces for it is meaningless. Measured on this base 2026-09-17:
	// CgroupPath `/`, and stats for a sleeping container answering
	// `35048.23%` and `0B / 33.44GB`. WSL-60's capability row is the same fact
	// asked about the base; this asks it about the thing being watched.
	switch strings.TrimSpace(parts[3]) {
	case "":
		f.Feeds[FeedResources] = FeedUnknown
	case "/":
		f.Feeds[FeedResources] = FeedAbsent
		f.ResourceNote = "this container is in the ROOT cgroup, so podman accounts for nothing and every figure it prints is invented"
	default:
		f.Feeds[FeedResources] = FeedPresent
	}
}

func readContainerStats(f *TickFacts, body string) {
	// ⛔ NOTHING IS KEPT WHERE THE FEED IS NOT PRESENT. podman answers whatever
	// the root cgroup makes it answer, and carrying that through because it
	// parsed would be the whole defect this file was written to remove.
	if f.Feeds[FeedResources] != FeedPresent {
		return
	}
	parts := strings.Split(strings.TrimSpace(body), "|")
	if len(parts) < 2 {
		return
	}
	r := &ResourceReading{}
	if v, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(parts[0]), "%"), 64); err == nil {
		r.CPUPercent = &v
	}
	// `MemUsage` is "used / limit". Only the used half is a reading.
	used, _, _ := strings.Cut(parts[1], "/")
	if b, ok := ParseHumanBytes(strings.TrimSpace(used)); ok {
		r.MemoryBytes = &b
	}
	if r.CPUPercent != nil || r.MemoryBytes != nil {
		f.Resources = r
	}
}

// Diagnose says how a container's exit code reads.
//
// ⛔ IT DOES NOT BORROW THE DISTRIBUTION'S SENTENCES. `DiagnoseExit` explains a
// signal by asking whether the WSL virtual machine went away, which is a true
// and useful thing to say about a distribution and simply the wrong subject for
// a container: the engine reports an out-of-memory kill directly, and this tool
// already knows that a base with no cgroup delegation cannot tell that kill from
// any other 137.
func (c *ContainerObserver) Diagnose(code int, f TickFacts) string {
	switch {
	case code == 0:
		return ""
	case f.OOMKilled:
		return "the engine reports this container as OOMKilled, so exit " +
			strconv.Itoa(code) + " is the out-of-memory killer and not the payload's own answer"
	case code == 137:
		if f.Feeds[FeedResources] == FeedAbsent {
			return "exit 137 is 128+9, SIGKILL. ⚠ This base has no per-container cgroup, so an " +
				"out-of-memory kill CANNOT be told apart from any other SIGKILL here, and the engine's " +
				"OOMKilled flag is false for both. Read `wsl-toolkit base status --probe` for the mechanism"
		}
		return "exit 137 is 128+9, SIGKILL, and the engine does not report this container as OOMKilled"
	case code > 128 && code < 160:
		sig := code - 128
		name := map[int]string{2: "SIGINT", 11: "SIGSEGV", 15: "SIGTERM"}[sig]
		if name == "" {
			name = "signal " + strconv.Itoa(sig)
		}
		return "exit " + strconv.Itoa(code) + " is 128+" + strconv.Itoa(sig) + ", which is " + name +
			" and nothing more. The container's own state is " + f.State
	}
	return "exit " + strconv.Itoa(code) + " is the payload's own, passed through unchanged"
}

// FeedSummary renders the feeds in a fixed order, so two ticks of one run read
// the same way and a map's iteration order cannot reorder a report.
func FeedSummary(feeds map[Feed]FeedState) string {
	if len(feeds) == 0 {
		return ""
	}
	var parts []string
	for _, f := range []Feed{FeedLifecycle, FeedOutput, FeedResources, FeedDisk, FeedExit} {
		if s, ok := feeds[f]; ok {
			parts = append(parts, string(f)+" "+string(s))
		}
	}
	return strings.Join(parts, ", ")
}
