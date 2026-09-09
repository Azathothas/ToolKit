package toolkit

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const pathSeparator = os.PathSeparator

// ResourceReport is what this machine is holding, split by whose it is.
//
// ⛔ THE SPLIT IS THE POINT. The situation that asked for this was an agent
// finding hundreds of images on a host where the tool had never run and
// reaching for a prune. Every line says which of the three it belongs to, and
// the only things this executable will ever remove are in the first.
type ResourceReport struct {
	Schema string `json:"schema"`

	Owned    OwnedResources `json:"owned"`
	Machine  MachineHolding `json:"machine"`
	Warnings []string       `json:"warnings,omitempty"`
}

// OwnedResources is what this executable made and what it will remove.
type OwnedResources struct {
	Home           string        `json:"home"`
	HomeBytes      int64         `json:"home_bytes"`
	HomeKnown      bool          `json:"home_known"`
	BaseRegistered bool          `json:"base_registered"`
	BaseName       string        `json:"base_name"`
	BaseDiskBytes  int64         `json:"base_disk_bytes"`
	BaseDiskKnown  bool          `json:"base_disk_known"`
	GuestJobs      []GuestJob    `json:"guest_jobs"`
	GuestBytes     int64         `json:"guest_bytes"`
	GuestKnown     bool          `json:"guest_known"`
	Containers     []OwnedThing  `json:"containers"`
	Images         []OwnedThing  `json:"images"`
	OpenRecords    []LedgerEntry `json:"open_records"`
	// HostStaging is what the helper is holding on THIS machine: workspaces
	// clients uploaded and artifact sets waiting to be collected. ⛔ They
	// were invisible here and to cleanup, so a helper that stayed up grew
	// without any command being able to say by how much.
	HostStaging []HostStage `json:"host_staging,omitempty"`
}

// HostStage is one directory the helper is keeping for a client.
type HostStage struct {
	Kind    string    `json:"kind"` // upload or artifacts
	Path    string    `json:"path"`
	Bytes   int64     `json:"bytes"`
	Known   bool      `json:"known"`
	ModTime time.Time `json:"mod_time"`
}

// GuestJob is one job directory still inside the distribution.
type GuestJob struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Age   string `json:"age"`
	// ModTime is when the guest last touched it. ⚠ READ IN THE GUEST, not
	// inferred here: the directory lives inside the distribution and this host
	// has no path to it.
	ModTime time.Time `json:"mod_time,omitempty"`
}

// OwnedThing is a container or an image the engine is holding.
type OwnedThing struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// MachineHolding is what the machine holds that is NOT this executable's.
//
// ⛔ NONE OF IT IS THIS TOOL'S TO REMOVE, and the report says so on every line
// rather than in a footnote. A named volume with no container attached is not
// garbage: it is how somebody keeps data between runs.
type MachineHolding struct {
	Distros       []Distro `json:"distros"`
	EngineName    string   `json:"engine_name,omitempty"`
	EngineSummary []string `json:"engine_summary,omitempty"`
}

// ResourceSchema versions the report.
const ResourceSchema = "wsl-toolkit-resources/1"

// Resources reads what is held. It creates nothing and removes nothing.
func (r *Runner) Resources(ctx context.Context) ResourceReport {
	rep := ResourceReport{Schema: ResourceSchema}
	rep.Owned.Home = r.home
	if size, ok, err := dirSize(r.home); err == nil {
		rep.Owned.HomeBytes, rep.Owned.HomeKnown = size, ok
		if !ok {
			rep.Warnings = append(rep.Warnings, "part of "+r.home+" could not be measured, so its total is withheld")
		}
	}
	rep.Owned.BaseName = r.cfg.Base.Name

	if distros, err := r.wsl.List(ctx, r.cfg.Base.Name); err == nil {
		rep.Machine.Distros = distros
		for _, d := range distros {
			if d.Owned {
				rep.Owned.BaseRegistered = true
			}
		}
	} else {
		rep.Warnings = append(rep.Warnings, "WSL could not be asked what is registered: "+err.Error())
	}
	if size, ok := FileSize(filepath.Join(r.base.Dir(), "ext4.vhdx")); ok {
		rep.Owned.BaseDiskBytes, rep.Owned.BaseDiskKnown = size, true
	}

	rep.Owned.HostStaging = r.hostStaging()
	if open, err := r.ledger.Open(); err == nil {
		rep.Owned.OpenRecords = open
	} else {
		rep.Warnings = append(rep.Warnings, "the ledger could not be read: "+err.Error())
	}

	if rep.Owned.BaseRegistered {
		jobs, bytes, known, err := r.guestJobs(ctx)
		if err != nil {
			rep.Warnings = append(rep.Warnings, "the guest could not be asked what it is holding: "+err.Error())
		} else {
			rep.Owned.GuestJobs, rep.Owned.GuestBytes, rep.Owned.GuestKnown = jobs, bytes, known
		}
		containers, images, engineSummary, err := r.engineHolding(ctx)
		if err != nil {
			rep.Warnings = append(rep.Warnings, "the guest engine could not be asked: "+err.Error())
		} else {
			rep.Owned.Containers, rep.Owned.Images = containers, images
			rep.Machine.EngineName = "podman inside " + r.cfg.Base.Name
			rep.Machine.EngineSummary = engineSummary
		}
	}
	return rep
}

func (r *Runner) guestJobs(ctx context.Context) ([]GuestJob, int64, bool, error) {
	guestHome, err := r.guestHome(ctx)
	if err != nil {
		return nil, 0, false, err
	}
	root := guestHome + "/" + GuestRoot
	script := fmt.Sprintf(`root=%s
[ -d "$root" ] || exit 0
for d in "$root"/jobs/* "$root"/staging/*; do
  [ -d "$d" ] || continue
  size=$(du -sk "$d" 2>/dev/null | cut -f1) || size=
  [ -n "$size" ] || size=-1
  mtime=$(stat -c %%Y "$d" 2>/dev/null) || mtime=
  [ -n "$mtime" ] || mtime=-1
  printf '%%s\t%%s\t%%s\n' "$size" "$mtime" "$d"
done
`, shellQuote(root))
	out, stderr, code, err := r.baseCapture(ctx, []byte(script), 3*time.Minute)
	if err != nil || code != 0 {
		return nil, 0, false, fmt.Errorf("exit %d: %s", code, firstLine(stderr+out))
	}
	var jobs []GuestJob
	var total int64
	known := true
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sizeStr, rest, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		mtimeStr, path, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		// ⚠ An unreadable timestamp stays the ZERO TIME rather than becoming
		// now. Cleanup reads a zero as "no age to check", and reading it as the
		// present would spare an abandoned directory forever.
		var mod time.Time
		var epoch int64
		if _, err := fmt.Sscanf(mtimeStr, "%d", &epoch); err == nil && epoch > 0 {
			mod = time.Unix(epoch, 0)
		}
		var kb int64
		if _, err := fmt.Sscanf(sizeStr, "%d", &kb); err != nil || kb < 0 {
			// ⚠ A directory whose size could not be read is named and the total
			// is withheld. A total that silently counts an unreadable directory
			// as zero is a number somebody acts on.
			known = false
			jobs = append(jobs, GuestJob{Path: path, Bytes: -1, ModTime: mod})
			continue
		}
		jobs = append(jobs, GuestJob{Path: path, Bytes: kb * 1024, ModTime: mod})
		total += kb * 1024
	}
	return jobs, total, known, nil
}

func (r *Runner) engineHolding(ctx context.Context) ([]OwnedThing, []OwnedThing, []string, error) {
	script := `printf 'CONTAINERS\n'
podman ps -a --filter label=` + JobLabel + ` --format '{{.ID}}\t{{.Names}}\t{{.Status}}\t{{.Image}}' 2>/dev/null || :
printf 'IMAGES\n'
podman images --format '{{.ID}}\t{{.Repository}}:{{.Tag}}\t{{.Size}}' 2>/dev/null || :
printf 'SUMMARY\n'
podman system df --format '{{.Type}}\t{{.Total}}\t{{.Active}}\t{{.Size}}\t{{.Reclaimable}}' 2>/dev/null || :
`
	out, stderr, code, err := r.baseCapture(ctx, []byte(script), 5*time.Minute)
	if err != nil || code != 0 {
		return nil, nil, nil, fmt.Errorf("exit %d: %s", code, firstLine(stderr+out))
	}
	var containers, images []OwnedThing
	var summary []string
	section := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch strings.TrimSpace(line) {
		case "CONTAINERS", "IMAGES", "SUMMARY":
			section = strings.TrimSpace(line)
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		switch section {
		case "CONTAINERS":
			if len(fields) >= 4 {
				containers = append(containers, OwnedThing{ID: fields[0], Name: fields[1], Detail: fields[2] + " " + fields[3]})
			}
		case "IMAGES":
			if len(fields) >= 3 {
				images = append(images, OwnedThing{ID: fields[0], Name: fields[1], Detail: fields[2]})
			}
		case "SUMMARY":
			summary = append(summary, strings.Join(fields, "  "))
		}
	}
	return containers, images, summary, nil
}

// CleanupPlan is what a cleanup would do, and what it did.
type CleanupPlan struct {
	Schema     string   `json:"schema"`
	DryRun     bool     `json:"dry_run"`
	Containers []string `json:"containers"`
	GuestDirs  []string `json:"guest_dirs"`
	HostDirs   []string `json:"host_dirs"`
	Images     []string `json:"images,omitempty"`
	Removed    []string `json:"removed,omitempty"`
	Failed     []string `json:"failed,omitempty"`
	// Kept is what was spared and why. ⛔ A DRY RUN THAT ONLY LISTS WHAT IT
	// WOULD REMOVE cannot be checked: the caller sees an empty plan and cannot
	// tell "nothing is here" from "everything here is in use".
	Kept []string `json:"kept,omitempty"`
}

// CleanupSchema versions the plan.
const CleanupSchema = "wsl-toolkit-cleanup/1"

// dirSize walks a directory and reports its total, and whether every part of it
// could be read.
func dirSize(root string) (int64, bool, error) {
	var total int64
	complete := true
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			complete = false
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			complete = false
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return total, false, err
	}
	return total, complete, nil
}

// RenderResources writes the human-readable report.
func RenderResources(w io.Writer, rep ResourceReport) error {
	p := func(format string, args ...any) error {
		_, err := fmt.Fprintf(w, format, args...)
		return err
	}
	if err := p("==> What this tool made, and the only things it will ever remove\n"); err != nil {
		return err
	}
	if rep.Owned.HomeKnown {
		if err := p("  state directory   %-12s %s\n", HumanBytes(rep.Owned.HomeBytes), rep.Owned.Home); err != nil {
			return err
		}
	} else {
		if err := p("  state directory   %-12s %s\n", "not measured", rep.Owned.Home); err != nil {
			return err
		}
	}
	base := "not registered"
	if rep.Owned.BaseRegistered {
		base = "registered"
	}
	disk := "not measured"
	if rep.Owned.BaseDiskKnown {
		disk = HumanBytes(rep.Owned.BaseDiskBytes)
	}
	if err := p("  base distro       %-12s %s (%s)\n", disk, rep.Owned.BaseName, base); err != nil {
		return err
	}
	// ⛔ THE HELPER'S OWN DIRECTORIES ARE PART OF WHAT IS HELD. They were on
	// disk and in nothing's report, so a helper that stayed up grew and no
	// command could say by how much.
	for _, st := range rep.Owned.HostStaging {
		size := "not measured"
		if st.Known {
			size = HumanBytes(st.Bytes)
		}
		if err := p("  host %-9s    %-12s %s\n", st.Kind, size, st.Path); err != nil {
			return err
		}
	}
	if len(rep.Owned.GuestJobs) == 0 {
		if err := p("  guest job dirs    none\n"); err != nil {
			return err
		}
	} else {
		for _, j := range rep.Owned.GuestJobs {
			size := "not measured"
			if j.Bytes >= 0 {
				size = HumanBytes(j.Bytes)
			}
			if err := p("  guest job dir     %-12s %s\n", size, j.Path); err != nil {
				return err
			}
		}
		if rep.Owned.GuestKnown {
			if err := p("  guest total       %s across %d directory(s)\n", HumanBytes(rep.Owned.GuestBytes), len(rep.Owned.GuestJobs)); err != nil {
				return err
			}
		} else {
			if err := p("  guest total       withheld: one directory could not be measured\n"); err != nil {
				return err
			}
		}
	}
	for _, c := range rep.Owned.Containers {
		if err := p("  container         %s %s\n", c.Name, c.Detail); err != nil {
			return err
		}
	}
	if len(rep.Owned.OpenRecords) > 0 {
		if err := p("  open records      %d, so a run was interrupted. wsl-toolkit gc --apply clears them\n", len(rep.Owned.OpenRecords)); err != nil {
			return err
		}
	}

	if err := p("\n==> What else WSL has registered. Named, never touched\n"); err != nil {
		return err
	}
	for _, d := range rep.Machine.Distros {
		if d.Owned {
			continue
		}
		state := "stopped"
		if d.Running {
			state = "running"
		}
		if err := p("  %-28s %s\n", d.Name, state); err != nil {
			return err
		}
	}

	if len(rep.Machine.EngineSummary) > 0 {
		if err := p("\n==> What the engine in the base is holding. None of it removed without --apply\n"); err != nil {
			return err
		}
		for _, line := range rep.Machine.EngineSummary {
			if err := p("  %s\n", line); err != nil {
				return err
			}
		}
	}
	for _, warn := range rep.Warnings {
		if err := p("\n  ! %s\n", warn); err != nil {
			return err
		}
	}
	return nil
}
