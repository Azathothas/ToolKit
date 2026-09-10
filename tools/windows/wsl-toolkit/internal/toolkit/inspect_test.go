// inspect_test.go - the readers `inspect` is built out of.
//
// ⛔ EVERY CASE HERE IS OVER TEXT A REAL ENGINE PRODUCED, pasted rather than
// invented. The two fixtures below were captured on 2026-09-10 from podman
// 6.1.1 inside the base, after a job that exited 37. A parser proved against
// output somebody wrote from memory is proved against their memory.
//
// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// realEvents is `podman events --since 1h --stream=false --filter container=... --format json`,
// verbatim, for a job that ran and exited 37 under `--rm`.
const realEvents = `EVENTS
{"ID":"9adc7e38985000039","Image":"docker.io/library/alpine:latest","Name":"wtk-4c21ea7f6fb89b2a","Status":"create","time":1789030870,"timeNano":1789030870719719549,"Type":"container","Attributes":{"wsl-toolkit.owner":"wsl-toolkit"}}
{"ID":"9adc7e38985000039","Image":"docker.io/library/alpine:latest","Name":"wtk-4c21ea7f6fb89b2a","Status":"start","time":1789030870,"timeNano":1789030870792362226,"Type":"container","Attributes":{"wsl-toolkit.owner":"wsl-toolkit"}}
{"ContainerExitCode":37,"ID":"9adc7e38985000039","Image":"docker.io/library/alpine:latest","Name":"wtk-4c21ea7f6fb89b2a","Status":"died","time":1789030870,"timeNano":1789030870819333184,"Type":"container","Attributes":{"wsl-toolkit.owner":"wsl-toolkit"}}
{"ContainerExitCode":37,"ID":"9adc7e38985000039","Image":"docker.io/library/alpine:latest","Name":"wtk-4c21ea7f6fb89b2a","Status":"remove","time":1789030870,"timeNano":1789030870865555206,"Type":"container","Attributes":{"wsl-toolkit.owner":"wsl-toolkit"}}
PRESENT
`

// TestTheContainersLastExitOutlivesTheContainer is the case for the whole
// command. WSL-56 said `run --rm` removes the container before anything can
// read its last state, and named that as the obstacle to measure first. The
// container IS gone; the journal is not, and it carries the exit code.
func TestTheContainersLastExitOutlivesTheContainer(t *testing.T) {
	job := &InspectedJob{ID: "4c21ea7f6fb89b2a", Container: "wtk-4c21ea7f6fb89b2a"}
	readEventSections(job, realEvents)
	if len(job.Events) != 4 {
		t.Fatalf("read %d event(s) from four lines: %+v", len(job.Events), job.Events)
	}
	if job.ExitCode == nil {
		t.Fatal("the container's own exit code was not read, so this command answers nothing a transcript does not")
	}
	if *job.ExitCode != 37 {
		t.Errorf("exit code %d, want 37", *job.ExitCode)
	}
	if job.Image != "docker.io/library/alpine:latest" {
		t.Errorf("image %q, want the one the journal names", job.Image)
	}
	// ⛔ THE ORDER IS THE JOURNAL'S, and `died` must be last of the ones that
	// carry a code: an exit read off `create` would be a nil dereference and an
	// exit read off any other event would be somebody else's number.
	if job.Events[len(job.Events)-1].Status != "remove" {
		t.Errorf("the last event is %q, want remove", job.Events[len(job.Events)-1].Status)
	}
}

// TestOneJobsEventsAreNotAnotherJobs. ⛔ podman's --filter name= is a SUBSTRING
// match, so a short id would collect a longer one's lifecycle and report its
// exit code as this job's. Attribution is the entire point of this report.
func TestOneJobsEventsAreNotAnotherJobs(t *testing.T) {
	job := &InspectedJob{ID: "4c21ea7f", Container: "wtk-4c21ea7f"}
	readEventSections(job, realEvents)
	if len(job.Events) != 0 {
		t.Fatalf("a prefix of another job's id collected %d of its events", len(job.Events))
	}
	if job.ExitCode != nil {
		t.Fatalf("another job's exit code %d was reported as this one's", *job.ExitCode)
	}
}

// TestAContainerThatWasNeverRemovedIsSaidSo covers the shape a killed run
// leaves: `--rm` never fired, so `podman ps -a` still has it.
func TestAContainerThatWasNeverRemovedIsSaidSo(t *testing.T) {
	job := &InspectedJob{ID: "abc", Container: "wtk-abc"}
	readEventSections(job, "EVENTS\nPRESENT\nExited (137) 3 minutes ago  docker.io/library/alpine:latest\n")
	if job.StillHere == "" {
		t.Fatal("a container that outlived its job was not reported, which is the one state gc is for")
	}
}

// realInfo is the ENGINE section a live podman produced, with the DISK line
// `df -Pk` wrote under it.
const realInfo = `ENGINE
6.1.1
crun
overlay
extfs
/home/toolkit/.local/share/containers/storage
v2
cgroupfs
true
file
journald
DISK
/dev/sdf        1055762868 3569084 998490312       1% /
`

func TestTheEngineDescribesItself(t *testing.T) {
	facts, disk := parseMachineFacts(realInfo)
	if !facts.Reached {
		t.Fatal("an engine that answered every field was reported as not reached")
	}
	for _, c := range []struct{ got, want, what string }{
		{facts.Version, "6.1.1", "version"},
		{facts.Runtime, "crun", "runtime"},
		{facts.StorageDriver, "overlay", "storage driver"},
		{facts.StorageBacked, "extfs", "backing filesystem"},
		{facts.CgroupVersion, "v2", "cgroup version"},
		{facts.CgroupManager, "cgroupfs", "cgroup manager"},
		{facts.Rootless, "true", "rootless"},
		{facts.EventLogger, "file", "event logger"},
		{facts.LogDriver, "journald", "log driver"},
	} {
		if c.got != c.want {
			t.Errorf("%s is %q, want %q", c.what, c.got, c.want)
		}
	}
	if disk == nil {
		t.Fatal("the df line was not read, and disk pressure is one of the five things this command exists to answer")
	}
	if disk.Mount != "/" || disk.UsedPct != 1 {
		t.Errorf("disk is %+v, want / at 1%%", disk)
	}
	if disk.TotalByte != 1055762868*1024 {
		t.Errorf("total is %d bytes, and df reports kibibytes", disk.TotalByte)
	}
}

// TestAnEngineThatAnsweredNothingIsNotAReachedEngine.
//
// ⛔ THE SURVEY EXITS 0 WITH BLANK LINES when podman is missing, because every
// line of it ends in `|| :` so one absent field cannot take the report down.
// Reading that as a reached engine with no version is the "refusal rendered as
// an empty success" shape this whole file is written against.
func TestAnEngineThatAnsweredNothingIsNotAReachedEngine(t *testing.T) {
	facts, disk := parseMachineFacts("ENGINE\n\n\n\n\n\n\n\n\n\n\nDISK\n")
	if facts.Reached {
		t.Fatal("ten blank lines were read as an engine that answered")
	}
	if disk != nil {
		t.Fatalf("a missing df line produced a disk figure: %+v", disk)
	}
}

// TestADfLineThatCannotBeTrustedIsNotANumber. ⚠ A truncated or wrapped row must
// produce nothing rather than a plausible figure: this number is one somebody
// acts on, and half a row parsed by field index is how the wrong column
// becomes "free space".
func TestADfLineThatCannotBeTrustedIsNotANumber(t *testing.T) {
	for _, bad := range []string{
		"",
		"/dev/sdf",
		"/dev/sdf 1055762868 3569084",
		"/dev/sdf notanumber 3569084 998490312 1% /",
		"/dev/sdf -1 3569084 998490312 1% /",
	} {
		if got := parseDiskLine(bad); got != nil {
			t.Errorf("parseDiskLine(%q) invented %+v", bad, got)
		}
	}
}

// TestTheHostHalfIsAnsweredWithNoMachineAtAll.
//
// ⛔ THE DOOR SWEEP THAT PRODUCED THIS. `inspect` built a Runner, a Runner
// calls FindWsl, and a host with no wsl.exe therefore got exit 2 and no answer
// at all - including for the transcript and the ledger record, which are on
// this machine's own disk. `logs` reads the same transcript with no Runner
// whatever, so the command that says more was the one that could say nothing.
func TestTheHostHalfIsAnsweredWithNoMachineAtAll(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "jobs", "abc123"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "jobs", "abc123", "stdout.log"), []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	led, err := OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := led.Append(LedgerEntry{Event: "open", Kind: "job", ID: "abc123", Image: "docker.io/library/alpine:latest"}); err != nil {
		t.Fatal(err)
	}

	rep, err := InspectHostOnly("abc123", "no wsl.exe on this host")
	if err != nil {
		t.Fatalf("the host half refused an id this host remembers: %v", err)
	}
	if rep.Job == nil {
		t.Fatal("no job in the report")
	}
	if rep.Job.Transcript == "" || rep.Job.TranscriptBytes == 0 {
		t.Errorf("the transcript was not reported: %+v", rep.Job)
	}
	if rep.Job.Image != "docker.io/library/alpine:latest" {
		t.Errorf("the ledger record was not read: image %q", rep.Job.Image)
	}
	if len(rep.Job.Known) != 2 {
		t.Errorf("known = %v, want the transcript and the ledger", rep.Job.Known)
	}
	// ⛔ AND THE UNREACHABLE HALF IS NAMED AS UNREACHABLE. A report that simply
	// omitted the engine would read as a machine with no engine.
	if rep.Engine.Reached || rep.Engine.Reason == "" {
		t.Errorf("the engine is reported as %+v, and it was never asked", rep.Engine)
	}
}

// TestTheHostHalfStillRefusesAnIdItHasNeverHeardOf. ⛔ Answering less must not
// become answering anything: the refusal is the same one the full path gives.
func TestTheHostHalfStillRefusesAnIdItHasNeverHeardOf(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	if _, err := InspectHostOnly("nothinghasheardofthis", "no wsl.exe"); !errors.Is(err, ErrUnknownJob) {
		t.Fatalf("an unknown id produced %v, want ErrUnknownJob", err)
	}
}
