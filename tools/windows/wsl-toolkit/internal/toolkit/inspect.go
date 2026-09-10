// inspect.go - what one job WAS, and what the machine was doing when it ran.
//
// ⭐ IT IS NOT A SECOND `resources`, and the difference is the whole design.
// `resources` answers "what is this tool holding now" by enumerating; this
// answers "what happened to this job, on what". If it starts listing what
// exists, it has become a copy of the other one. WSL-56.
//
// ⛔ THE OBSTACLE WSL-56 NAMED DOES NOT HOLD, and it was measured rather than
// argued. Jobs run under `podman run --rm`, so the container is gone before a
// caller could ask it anything, and the entry said whoever took this had to
// find out whether podman retained enough afterwards. It does. Measured on
// 2026-09-10 against podman 6.1.1 in the base, after a job that exited 37:
//
//	{"ContainerExitCode":37,...,"Name":"wtk-4c21ea7f6fb89b2a","Status":"died",...}
//
// The event logger is `file`, the journal survives the container, and the
// `died` and `remove` events both carry the exit code. So `--rm` stays, which
// is what stops a failed fleet leaving twelve containers behind.
//
// ⚠ TWO MEASUREMENTS THAT COST TIME, BOTH KEPT. `podman events --until 0s`
// answers NOTHING where `--stream=false` with the same `--since` answers
// everything; and `podman events` exits 0 for a container that never existed,
// so an empty journal is not by itself a refusal. This file reads emptiness
// against the record and the transcript rather than as an answer.
//
// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// InspectSchema versions the report.
const InspectSchema = "wsl-toolkit-inspect/1"

// InspectReport is one job and the machine under it. ⚠ Job is a pointer
// because `inspect` with no id is the machine half alone, and an empty struct
// there would read as a job with no facts.
type InspectReport struct {
	Schema   string        `json:"schema"`
	Job      *InspectedJob `json:"job,omitempty"`
	Engine   EngineFacts   `json:"engine"`
	Guest    GuestFacts    `json:"guest"`
	Warnings []string      `json:"warnings,omitempty"`
}

// InspectedJob is what survives one job.
type InspectedJob struct {
	ID        string `json:"id"`
	Container string `json:"container"`
	Image     string `json:"image,omitempty"`
	// Known says which sources placed this id. ⛔ An id nothing has heard of is
	// a REFUSAL and never an empty document: an inspection surface that answers
	// an empty object for an unknown id is the "refusal rendered as a
	// successful empty result" class this tool has paid for three times.
	Known []string `json:"known"`
	// Record is the ledger's open record, when the job is still in flight or
	// its close was never written.
	Record   *LedgerEntry `json:"record,omitempty"`
	Closed   bool         `json:"closed"`
	Deadline time.Time    `json:"deadline,omitempty"`
	// Transcript is the host directory holding the streams, when one is left.
	Transcript      string           `json:"transcript,omitempty"`
	TranscriptBytes int64            `json:"transcript_bytes,omitempty"`
	Events          []ContainerEvent `json:"events,omitempty"`
	// ExitCode is the container's own last exit, from the engine rather than
	// from this tool's memory of it. Nil where the journal no longer reaches
	// back that far.
	ExitCode *int `json:"exit_code,omitempty"`
	// StillHere is set when the container was NOT removed, which is the shape a
	// killed run leaves behind.
	StillHere string `json:"still_here,omitempty"`
	// GuestDirs are the job's directories inside the distribution that still
	// exist, with their sizes.
	GuestDirs []GuestJob `json:"guest_dirs,omitempty"`
}

// ContainerEvent is one lifecycle line from the engine's journal.
type ContainerEvent struct {
	Status   string    `json:"status"`
	At       time.Time `json:"at"`
	ExitCode *int      `json:"exit_code,omitempty"`
}

// EngineFacts is the engine a job ran under, as it describes itself.
//
// ⚠ THE GUEST ENGINE IS PODMAN BY CONSTRUCTION. WSL-56 warned that podman and
// docker disagree about field names and about values, which is true and is the
// HOST engine's problem: that one turns an image into a rootfs and `doctor`
// reports it. Jobs run in a rootless podman this tool installs inside the base,
// so there is one spelling here rather than two.
type EngineFacts struct {
	Reached       bool   `json:"reached"`
	Reason        string `json:"reason,omitempty"`
	Version       string `json:"version,omitempty"`
	Runtime       string `json:"runtime,omitempty"`
	StorageDriver string `json:"storage_driver,omitempty"`
	StorageBacked string `json:"storage_backing_fs,omitempty"`
	StorageRoot   string `json:"storage_root,omitempty"`
	CgroupVersion string `json:"cgroup_version,omitempty"`
	CgroupManager string `json:"cgroup_manager,omitempty"`
	Rootless      string `json:"rootless,omitempty"`
	EventLogger   string `json:"event_logger,omitempty"`
	LogDriver     string `json:"log_driver,omitempty"`
}

// GuestFacts is the distribution the engine runs in.
type GuestFacts struct {
	Distro string `json:"distro"`
	Home   string `json:"home,omitempty"`
	// Disk is the filesystem the engine's storage sits on. ⛔ A job that failed
	// for no visible reason after a long pull is usually this number.
	Disk *DiskUse `json:"disk,omitempty"`
}

// DiskUse is one filesystem's occupancy, in bytes.
type DiskUse struct {
	Mount     string `json:"mount"`
	TotalByte int64  `json:"total_bytes"`
	UsedByte  int64  `json:"used_bytes"`
	FreeByte  int64  `json:"free_bytes"`
	UsedPct   int    `json:"used_percent"`
}

// ErrUnknownJob is what an id nothing on this machine has heard of produces.
var ErrUnknownJob = fmt.Errorf("no such job")

// Inspect answers for one job id, or for the machine alone when id is empty.
//
// ⛔ IT CREATES NOTHING. Like `resources` and `logs` it is a reading, and a
// report that made the state directory it was about to describe is the defect
// WSL-55 closed.
func (r *Runner) Inspect(ctx context.Context, id string, since time.Duration) (InspectReport, error) {
	rep := InspectReport{Schema: InspectSchema}
	rep.Guest.Distro = r.cfg.Base.Name

	if id != "" {
		if err := AssertArgvSafe([]string{id}); err != nil {
			return rep, err
		}
		job, err := r.inspectJobRecord(id)
		if err != nil {
			return rep, err
		}
		rep.Job = job
	}

	facts, disk, guestHome, err := r.machineFacts(ctx)
	rep.Engine = facts
	if err != nil {
		rep.Warnings = append(rep.Warnings, "the guest engine could not be asked: "+err.Error())
	} else {
		rep.Guest.Disk = disk
		rep.Guest.Home = guestHome
	}

	if rep.Job != nil && rep.Engine.Reached {
		if err := r.inspectContainer(ctx, rep.Job, since); err != nil {
			rep.Warnings = append(rep.Warnings, "the engine's journal could not be read: "+err.Error())
		}
		if err := r.inspectGuestDirs(ctx, rep.Job, guestHome); err != nil {
			rep.Warnings = append(rep.Warnings, "the guest could not be asked what this job left: "+err.Error())
		}
	}
	if rep.Job != nil && len(rep.Job.Known) == 0 {
		// ⛔ Reached only when nothing on this machine has heard of the id: no
		// transcript, no ledger line, no container in the journal, no directory.
		return rep, fmt.Errorf("%w: %s. `wsl-toolkit logs` lists the jobs this machine still has", ErrUnknownJob, id)
	}
	return rep, nil
}

// inspectJobRecord reads what the HOST remembers: the transcript and the ledger.
func (r *Runner) inspectJobRecord(id string) (*InspectedJob, error) {
	job := &InspectedJob{ID: id, Container: "wtk-" + id}

	dir := filepath.Join(r.home, "jobs", id)
	if _, err := ResolveInside(r.home, dir); err != nil {
		// ⛔ A caller-supplied path component is how a report becomes a file
		// reader. It is resolved and contained before anything is opened.
		return nil, err
	}
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		job.Transcript = dir
		if size, _, err := dirSize(dir); err == nil {
			job.TranscriptBytes = size
		}
		job.Known = append(job.Known, "a transcript on this host")
	}

	all, err := r.ledger.All()
	if err != nil {
		return nil, err
	}
	var opened, closed *LedgerEntry
	for i := range all {
		e := all[i]
		if e.Kind != "job" || e.ID != id {
			continue
		}
		switch e.Event {
		case "open":
			opened = &all[i]
		case "close":
			closed = &all[i]
		}
	}
	if opened != nil {
		job.Record = opened
		job.Image = opened.Image
		job.Deadline = opened.Deadline
		job.Known = append(job.Known, "a record in the ledger")
	}
	job.Closed = closed != nil
	return job, nil
}

// engineInfoScript surveys the guest engine and the filesystem under it.
//
// ⛔ ONE GUEST INVOCATION. Each `podman info --format` is cheap inside the
// distribution and a wsl.exe crossing is not, so the whole survey is one script
// rather than one call per field.
//
// ⚠ EVERY FIELD IS ALLOWED TO BE BLANK. A key this podman does not carry leaves
// an empty line rather than taking the survey down: a report that answers
// nothing because one field was renamed is worse than one with a gap in it.
const engineInfoScript = `printf 'ENGINE\n'
for f in '{{.Version.Version}}' '{{.Host.OCIRuntime.Name}}' '{{.Store.GraphDriverName}}' \
         '{{index .Store.GraphStatus "Backing Filesystem"}}' '{{.Store.GraphRoot}}' \
         '{{.Host.CgroupsVersion}}' '{{.Host.CgroupManager}}' '{{.Host.Security.Rootless}}' \
         '{{.Host.EventLogger}}' '{{.Host.LogDriver}}'; do
  printf '%s\n' "$(podman info --format "$f" 2>/dev/null | head -1)"
done
printf 'DISK\n'
`

// machineFacts asks the guest engine what it is, and the guest what it is on.
func (r *Runner) machineFacts(ctx context.Context) (EngineFacts, *DiskUse, string, error) {
	guestHome, err := r.guestHome(ctx)
	if err != nil {
		return EngineFacts{Reason: err.Error()}, nil, "", err
	}
	script := engineInfoScript + "df -Pk " + shellQuote(guestHome) + " 2>/dev/null | tail -n +2 | head -1 || :\n"
	out, stderr, code, err := r.baseCapture(ctx, []byte(script), 3*time.Minute)
	if err != nil || code != 0 {
		return EngineFacts{Reason: firstLine(stderr + out)}, nil, guestHome,
			fmt.Errorf("exit %d: %s", code, firstLine(stderr+out))
	}
	facts, disk := parseMachineFacts(out)
	if !facts.Reached {
		facts.Reason = "podman in " + r.cfg.Base.Name + " answered nothing to `info`"
	}
	return facts, disk, guestHome, nil
}

// parseMachineFacts reads the survey's two sections.
func parseMachineFacts(out string) (EngineFacts, *DiskUse) {
	var engine []string
	diskLine := ""
	section := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch strings.TrimSpace(line) {
		case "ENGINE", "DISK":
			section = strings.TrimSpace(line)
			continue
		}
		switch section {
		case "ENGINE":
			engine = append(engine, strings.TrimSpace(line))
		case "DISK":
			if strings.TrimSpace(line) != "" && diskLine == "" {
				diskLine = line
			}
		}
	}
	at := func(i int) string {
		if i < len(engine) {
			return engine[i]
		}
		return ""
	}
	facts := EngineFacts{
		Version: at(0), Runtime: at(1), StorageDriver: at(2), StorageBacked: at(3),
		StorageRoot: at(4), CgroupVersion: at(5), CgroupManager: at(6),
		Rootless: at(7), EventLogger: at(8), LogDriver: at(9),
	}
	// ⛔ REACHED MEANS THE ENGINE ANSWERED, not that the script exited 0. The
	// script exits 0 with ten blank lines when podman is missing, and reporting
	// that as a reached engine with no version is the "refusal rendered as an
	// empty success" shape this file is written against.
	facts.Reached = facts.Version != ""
	return facts, parseDiskLine(diskLine)
}

// parseDiskLine reads one `df -Pk` row. ⚠ POSIX -P is what guarantees ONE line
// per filesystem: without it a long device name wraps and the numbers land on
// the next line, where a field-index reader silently takes the wrong ones.
func parseDiskLine(line string) *DiskUse {
	f := strings.Fields(line)
	if len(f) < 6 {
		return nil
	}
	kb := func(s string) int64 {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n < 0 {
			return -1
		}
		return n * 1024
	}
	total, used, free := kb(f[1]), kb(f[2]), kb(f[3])
	if total < 0 || used < 0 || free < 0 {
		return nil
	}
	pct := 0
	if v, err := strconv.Atoi(strings.TrimSuffix(f[4], "%")); err == nil {
		pct = v
	}
	return &DiskUse{Mount: f[5], TotalByte: total, UsedByte: used, FreeByte: free, UsedPct: pct}
}

// podmanEvent is the shape podman writes with --format json.
type podmanEvent struct {
	Name     string `json:"Name"`
	Status   string `json:"Status"`
	TimeNano int64  `json:"timeNano"`
	ExitCode *int   `json:"ContainerExitCode"`
	Image    string `json:"Image"`
}

// inspectContainer reads the engine's journal for this job's container, and
// asks whether the container is somehow still there.
func (r *Runner) inspectContainer(ctx context.Context, job *InspectedJob, since time.Duration) error {
	name := shellQuote(job.Container)
	window := strconv.FormatInt(int64(since/time.Second), 10) + "s"
	// ⚠ --stream=false, NOT --until. `--until 0s` returns an EMPTY journal for
	// events that are certainly there; measured on 2026-09-10.
	script := "printf 'EVENTS\\n'\n" +
		"podman events --since " + window + " --stream=false --filter container=" + name +
		" --format json 2>/dev/null || :\n" +
		"printf 'PRESENT\\n'\n" +
		"podman ps -a --filter name=" + name + " --format '{{.Status}} {{.Image}}' 2>/dev/null || :\n"
	out, stderr, code, err := r.baseCapture(ctx, []byte(script), 3*time.Minute)
	if err != nil || code != 0 {
		return fmt.Errorf("exit %d: %s", code, firstLine(stderr+out))
	}
	readEventSections(job, out)
	sort.SliceStable(job.Events, func(i, j int) bool { return job.Events[i].At.Before(job.Events[j].At) })
	if len(job.Events) > 0 {
		job.Known = append(job.Known, "the engine's journal")
	}
	if job.StillHere != "" {
		job.Known = append(job.Known, "a container that was never removed")
	}
	return nil
}

// readEventSections fills the job from the journal survey's output.
func readEventSections(job *InspectedJob, out string) {
	section := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		switch line {
		case "EVENTS", "PRESENT":
			section = line
			continue
		}
		if line == "" {
			continue
		}
		switch section {
		case "EVENTS":
			var e podmanEvent
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				continue
			}
			// ⛔ THE NAME IS CHECKED AGAIN HERE. podman's --filter name= is a
			// SUBSTRING match, so a journal read for wtk-4c21 would attribute
			// wtk-4c21ea7f's events to it, and attribution is the entire point
			// of this report.
			if e.Name != job.Container {
				continue
			}
			job.Events = append(job.Events, ContainerEvent{
				Status: e.Status, At: time.Unix(0, e.TimeNano).UTC(), ExitCode: e.ExitCode,
			})
			if e.Image != "" && job.Image == "" {
				job.Image = e.Image
			}
			if e.ExitCode != nil && e.Status == "died" {
				job.ExitCode = e.ExitCode
			}
		case "PRESENT":
			job.StillHere = line
		}
	}
}

// inspectGuestDirs reports this job's directories inside the distribution.
func (r *Runner) inspectGuestDirs(ctx context.Context, job *InspectedJob, guestHome string) error {
	if guestHome == "" {
		return nil
	}
	root := guestHome + "/" + GuestRoot
	script := fmt.Sprintf(`for d in %s/jobs/%s %s/staging/%s; do
  [ -d "$d" ] || continue
  size=$(du -sk "$d" 2>/dev/null | cut -f1) || size=
  [ -n "$size" ] || size=-1
  printf '%%s\t%%s\n' "$size" "$d"
done
`, shellQuote(root), shellQuote(job.ID), shellQuote(root), shellQuote(job.ID))
	out, stderr, code, err := r.baseCapture(ctx, []byte(script), 3*time.Minute)
	if err != nil || code != 0 {
		return fmt.Errorf("exit %d: %s", code, firstLine(stderr+out))
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sizeStr, path, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		var kb int64
		if _, err := fmt.Sscanf(sizeStr, "%d", &kb); err != nil || kb < 0 {
			// ⚠ A directory whose size could not be read is NAMED with a
			// negative size rather than counted as zero, for the reason
			// resources.go carries: a total that silently counts an unreadable
			// directory as empty is a number somebody acts on.
			job.GuestDirs = append(job.GuestDirs, GuestJob{Path: path, Bytes: -1})
			continue
		}
		job.GuestDirs = append(job.GuestDirs, GuestJob{Path: path, Bytes: kb * 1024})
	}
	if len(job.GuestDirs) > 0 {
		job.Known = append(job.Known, "a directory in the guest")
	}
	return nil
}
