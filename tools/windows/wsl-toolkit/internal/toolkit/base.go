package toolkit

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed provision.sh
var provisionScript []byte

// packagesScript is the shared package table, generated from the block in
// scripts/common/bootstrap.sh. TODO/RULES.md section 4 owns the rule.
//
//go:embed packages.sh
var packagesScript []byte

// provisionPayload is what provisioning runs: the shared package table, then the
// provisioner that resolves its developer names through it.
//
// ⛔ ONE TABLE FOR BOTH READERS. The provisioner carried a second map of package
// names that knew six managers where the table knew twelve, and a distribution that
// renamed a package had to be fixed in both. WSL-70.
func provisionPayload() []byte {
	payload := make([]byte, 0, len(packagesScript)+len(loginShellScript)+2+len(provisionScript))
	payload = append(payload, packagesScript...)
	payload = append(payload, '\n')
	// ⭐ loginshell.sh's functions come next, so the account is created with the
	// shell every later `base ensure` holds it to. WSL-143.
	payload = append(payload, loginShellScript...)
	payload = append(payload, '\n')
	return append(payload, provisionScript...)
}

//go:embed verify.sh
var verifyScript []byte

//go:embed repair.sh
var repairScript []byte

//go:embed grants.sh
var grantsScript []byte

//go:embed binfmt.sh
var binfmtScript []byte

// BaseSpaceFloor is what the volume must have free before an import starts.
//
// ⚠ Far above a throwaway distribution's 256 MiB floor, because this
// distribution will hold an engine and a dozen images. Running out midway leaves a partial disk
// and a registered distribution that does not work.
const BaseSpaceFloor = int64(6) << 30

// binfmtPinnedDigest is the index digest the tag below named when it was
// measured on 2026-10-24: the registry's answer and the sha256 of the manifest
// it served agreed. WSL-182.
const binfmtPinnedDigest = "sha256:400a4873b838d1b89194d982c45e5fb3cda4593fbfd7e08a02e76b03b21166f0"

// BinfmtImage installs QEMU interpreters in the WSL kernel.
//
// ⛔ PINNED BY DIGEST, BECAUSE IT RUNS PRIVILEGED. The installer runs as root
// with --privileged and writes F-flag interpreters into the one handler table
// every distribution shares. A tag can be pushed again, and a host with no copy
// would then run whatever the tag names now. The tag stays for a reader; the
// engine resolves the digest. WSL-182.
const BinfmtImage = "docker.io/tonistiigi/binfmt:qemu-v10.2.3-68@" + binfmtPinnedDigest

// BaseState is what `base status` answers.
type BaseState struct {
	Name       string `json:"name"`
	Image      string `json:"image"`
	BuiltFrom  string `json:"built_from,omitempty"`
	User       string `json:"user"`
	Registered bool   `json:"registered"`
	Running    bool   `json:"running"`
	Healthy    bool   `json:"healthy"`
	Engine     string `json:"engine,omitempty"`
	DiskPath   string `json:"disk_path,omitempty"`
	DiskBytes  int64  `json:"disk_bytes,omitempty"`
	DiskKnown  bool   `json:"disk_known"`
	// Probed is true when this answer ran a container in the base. ⛔ Without
	// --probe it is false, and `healthy: false` beside it is no verdict: a base
	// that is registered and cannot run anything reads the same. WSL-137.
	Probed bool `json:"probed"`
	// DiskSparse is yes or no where the disk could be read, and DiskAllocated the
	// bytes it takes on its volume. ⚠ A disk that is not sparse keeps every byte
	// its guest ever wrote, whatever the guest frees. WSL-109.
	DiskSparse    string `json:"disk_sparse,omitempty"`
	DiskAllocated int64  `json:"disk_allocated_bytes,omitempty"`
	// WslConfigSparseVhd is what .wslconfig sets for sparseVhd: true, false,
	// unset or unreadable.
	WslConfigSparseVhd string `json:"wslconfig_sparse_vhd,omitempty"`
	// Leftovers are the adapters the configuration no longer names that left
	// files in the base, read under --probe. `base recreate` removes them.
	Leftovers []AdapterLeftover `json:"leftovers,omitempty"`
	Created   time.Time         `json:"created,omitempty"`
	// Identity is what the GUEST says it is, read under --probe. ⛔ It is the
	// authority: `built_from` above comes from a file on this machine that an
	// editor can reach, and this comes from inside the distribution. WSL-42.
	Identity *Identity `json:"identity,omitempty"`
	Problems []string  `json:"problems,omitempty"`
	// Cgroup is what the guest's cgroup tree can do for the engine's account,
	// read under --probe. ⚠ Nil where the probe did not run or the guest is
	// older than the probe, which is not the same as a tree that can do nothing.
	Cgroup *CgroupState `json:"cgroup,omitempty"`
	Binfmt *BinfmtState `json:"binfmt,omitempty"`
	// Kvm is what /dev/kvm is to the account under --probe: usable, unusable or
	// absent. ⚠ Empty where it was not read. WSL-130.
	Kvm string `json:"kvm,omitempty"`
	// Account is the managed account's passwd entry, read back under --probe:
	// its login shell, and where bash is. WSL-143.
	Account *AccountState `json:"account,omitempty"`
	// Access is the configured access state. Under --probe a healthy result
	// means the configured account verified these settings after WSL restarted,
	// and AutomountGuest carries what the guest's drives are.
	Access BaseAccessState `json:"access"`
	// Remediations are the conditions this tool found, each with what leaving it
	// costs and the exact command that takes it. ⭐ A caller here is usually an
	// agent, and the command is the field it acts on.
	Remediations []Remediation `json:"remediations,omitempty"`
	// Adapters are the configured adapters as read back under --probe, both the
	// base's half and this machine's. ⚠ Nil without --probe, which is not the same
	// as none configured: Access.Adapters says what is configured.
	Adapters []AdapterState `json:"adapters,omitempty"`
	// Naming says, for the default instance, which name the base runs under, why,
	// and how to move to the canonical one. ⚠ Nil for a named instance. WSL-167.
	Naming *BaseNaming `json:"naming,omitempty"`
}

// BaseAccessState is what the base is configured to reach on Windows.
type BaseAccessState struct {
	Automount string `json:"automount"`
	// AutomountGuest is what the guest's /proc/mounts shows for the Windows drives,
	// read under --probe: off, ro, rw, or mixed where a writable mount sits beside a
	// read-only one. ⚠ Empty where the probe did not reach the drives, which is not
	// the same as off. WSL-84.
	AutomountGuest   string `json:"automount_guest,omitempty"`
	Interop          string `json:"interop"`
	Systemd          bool   `json:"systemd"`
	PasswordlessSudo bool   `json:"passwordless_sudo"`
	Toolset          string `json:"toolset"`
	// SharedTmpfs is whether /mnt/wsl, the one directory every distribution in the
	// utility VM shares, stays mounted. ⛔ A caller reading this report could not
	// tell before, and `base doors` answers about the DOOR while this answers about
	// the CONFIGURATION; a base that should close it and does not needs both.
	SharedTmpfs string      `json:"shared_tmpfs"`
	Mounts      []BaseMount `json:"mounts,omitempty"`
	Adapters    []string    `json:"adapters,omitempty"`
	// RootEngine, RuntimeDir and Kvm are WSL-130's opt-in settings, as
	// configured.
	RootEngine bool `json:"root_engine"`
	RuntimeDir bool `json:"runtime_dir"`
	Kvm        bool `json:"kvm"`
}

// Base is the owned distribution's lifecycle.
type Base struct {
	cfg  Config
	home string
	wsl  *Wsl
	log  func(string)
	// imported says THIS PROCESS's own `wsl --import` registered the distribution
	// now under the name, and it has not been stamped yet, so a rollback may
	// unregister it without the guest's proof. ⛔ It is set by the build path, after
	// the import succeeded, and by nothing else: a flag a caller could set would be
	// a way past the guard.
	//
	// ⛔ AFTER THE IMPORT, NOT BEFORE IT. It was set before the build started, so a
	// build whose import failed because another process had just registered the
	// name rolled back by unregistering THAT process's base, with no marker
	// demanded. WSL-172.
	imported bool
	// disks answers WSL's registrations, each distribution's name and the
	// directory of its disk. Nil reads the registry. WSL-177.
	disks func() (map[string]string, error)
	// guest says the caller that sent this through the helper does not own the
	// base, so nothing owner-only runs for it. ⛔ Set by the helper from the
	// request, and it only ever narrows what runs. WSL-167.
	guest bool
	// archive writes the rootfs archive a new build imports, in place of the host
	// engine. ⚠ Nil, which every real Base has, means the engine. A case sets it to
	// drive a build with no engine. WSL-150.
	archive func(ctx context.Context, tarPath string) error
}

// NewBase binds the lifecycle to this host.
//
// ⛔ IT DOES NOT CREATE THE STATE DIRECTORY, and it used to. `paths.go` states
// the rule in the function it is about - "a report that creates a directory on a
// machine it is only describing has changed the thing it was asked to measure" -
// and `base status`, a read-only report, reached EnsureHome through here. So did
// `ready`, which is the command an agent runs FIRST, on a machine it has not
// touched. WSL-55. The paths that WRITE ensure; reading a record from a
// directory that does not exist is already "no record".
func NewBase(cfg Config, log func(string)) (*Base, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	w, err := FindWsl()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = func(string) {}
	}
	return &Base{cfg: cfg, home: home, wsl: w, log: log}, nil
}

// Dir is where the distribution's disk lives.
func (b *Base) Dir() string { return filepath.Join(b.home, "base") }

// Status reads the base without changing it. ⭐ `probe` decides whether it runs
// a container: always would cost a pull on a cold machine, never could only
// report that a distribution is registered, which is not the same as usable.
func (b *Base) Status(ctx context.Context, probe bool) (BaseState, error) {
	st, err := b.configuredState()
	if err != nil {
		return st, err
	}
	return b.statusFrom(ctx, st, probe)
}

// configuredState is the half of a status this machine answers from the
// configuration alone, with no call into WSL.
func (b *Base) configuredState() (BaseState, error) {
	automount, err := NormalizeAutomount(b.cfg.Base.Automount)
	if err != nil {
		return BaseState{}, err
	}
	interop, err := NormalizeBaseInterop(b.cfg.Base.Interop)
	if err != nil {
		return BaseState{}, err
	}
	toolset, err := NormalizeBaseToolset(b.cfg.Base.Toolset)
	if err != nil {
		return BaseState{}, err
	}
	mounts, err := b.cfg.ResolvedBaseMounts()
	if err != nil {
		return BaseState{}, err
	}
	sharedTmpfs, err := NormalizeSharedTmpfs(b.cfg.Base.SharedTmpfs)
	if err != nil {
		return BaseState{}, err
	}
	st := BaseState{
		Name: b.cfg.Base.Name, Image: b.cfg.Base.Image, User: b.cfg.Base.User,
		Access: BaseAccessState{
			Automount: automount, Interop: interop, Systemd: b.cfg.Base.Systemd,
			PasswordlessSudo: b.cfg.Base.PasswordlessSudo, Toolset: toolset,
			SharedTmpfs: sharedTmpfs, Mounts: mounts,
			RootEngine: b.cfg.Base.RootEngine, RuntimeDir: b.cfg.Base.RuntimeDir, Kvm: b.cfg.Base.Kvm,
		},
		Naming: b.naming(),
	}
	for _, a := range b.cfg.Base.Adapters {
		st.Access.Adapters = append(st.Access.Adapters, a.Name)
	}
	// ⭐ A GRANT WHOSE SOURCE IS GONE IS REPORTED WITHOUT --probe, because it is
	// this machine's fact, and `base ensure` and `base grant` refuse over it until
	// it is revoked. WSL-106.
	for _, m := range mounts {
		if !m.Missing {
			continue
		}
		revoke := ToolCommand("base revoke --target " + m.Target)
		st.Problems = append(st.Problems, fmt.Sprintf(
			"the grant at %s has lost its source: %s no longer exists. Run: %s", m.Target, m.Source, revoke))
		st.Remediations = append(st.Remediations, Remediation{
			ID:         "grant-source-gone",
			What:       "the Windows directory granted at " + m.Target + ", " + m.Source + ", no longer exists",
			Costs:      "base ensure and base grant refuse until the grant is revoked, because neither can mount it",
			Command:    revoke,
			Repairable: true,
		})
	}
	return st, nil
}

// statusFrom completes a configured state with what WSL and, under probe, the
// guest answer.
func (b *Base) statusFrom(ctx context.Context, st BaseState, probe bool) (BaseState, error) {
	distros, err := b.wsl.List(ctx, b.cfg.Base.Name)
	if err != nil {
		return st, err
	}
	for _, d := range distros {
		if strings.EqualFold(d.Name, b.cfg.Base.Name) {
			st.Registered, st.Running = true, d.Running
		}
	}
	st.DiskPath = filepath.Join(b.Dir(), "ext4.vhdx")
	// ⛔ THE DISK WSL NAMES, NOT THIS HOME'S FOLDER. A state directory that uses a
	// base it does not own reported a disk in its own empty folder as not measured.
	// WSL-167.
	if at, registered, err := b.diskFolder(); err == nil && registered && at != "" {
		st.DiskPath = filepath.Join(at, "ext4.vhdx")
	}
	if size, ok := FileSize(st.DiskPath); ok {
		st.DiskBytes, st.DiskKnown = size, true
		if r, ok := diskSparseFacts(&st); ok {
			st.Remediations = append(st.Remediations, r)
		}
	}
	if rec, err := b.readRecord(); err == nil {
		st.Created = rec.Created
		// ⛔ The configured image and the one it was built from are two places
		// for one fact, so they are compared. They drift the moment somebody
		// runs `--preset alpine` without `--save`.
		st.BuiltFrom = rec.Image
		// ⚠ THIS IS THE RECORD, and the record is not the authority. Under
		// --probe the guest's own marker replaces both this value and this
		// problem below; without one, the record is the best answer available
		// and the message says which it is.
		if rec.Image != "" && rec.Image != st.Image {
			st.Problems = append(st.Problems, fmt.Sprintf(
				"this machine's record says it was built from %s and the configuration says %s. "+
					"Run `base status --probe` to ask the guest, then `base recreate` to rebuild",
				rec.Image, st.Image))
		}
		if rec.User != "" && rec.User != st.User {
			st.Problems = append(st.Problems, fmt.Sprintf(
				"this machine's record says the managed account is %q and the configuration says %q. "+
					"Run `base status --probe` to ask the guest, then `base recreate` to rebuild",
				rec.User, st.User))
		}
	}
	if !st.Registered {
		st.Problems = append(st.Problems, "not registered. Create it with: "+ToolCommand("base ensure"))
		return st, nil
	}
	if !probe {
		return st, nil
	}
	// ⭐ ASKED OF THE GUEST, and only under --probe because it costs a wsl.exe
	// round trip. A status that reports the record alone reports what this
	// machine believes, which is exactly what issue 18 showed to be wrong.
	if id, err := b.wsl.ReadIdentity(ctx, b.cfg.Base.Name); err == nil {
		st.Identity = &id
		st.BuiltFrom = id.Image
		// ⛔ THE RECORD-BASED PROBLEM IS DROPPED, not added to. The record and
		// the marker say the same thing whenever `ensure` has run, so keeping
		// both produced two lines about one disagreement and left a reader
		// wondering which of the two the tool believed. The marker is the
		// authority, so it is the one that speaks.
		st.Problems = withoutRecordDrift(st.Problems)
		if id.Image != st.Image {
			st.Problems = append(st.Problems, fmt.Sprintf(
				"the guest was built from %s and the configuration says %s. Run `base recreate` to rebuild it, or change the configuration back",
				id.Image, st.Image))
		}
		if problem := managedAccountDrift(id.User, st.User); problem != "" {
			st.Problems = append(st.Problems, problem)
			return st, nil
		}
	} else if errors.Is(err, ErrNoIdentity) {
		if rec, ok, _ := b.interruptedBuild(ctx); ok {
			if by, running := b.buildRunning(); running {
				// ⛔ A LIVE BUILD IS NOT ADVISED AWAY. It holds the base's lock until
				// it stamps its marker. WSL-150.
				st.Problems = append(st.Problems, fmt.Sprintf(
					"%s carries no identity marker yet, and it is being changed by %s, which may be its build. Run this again once that ends",
					b.cfg.Base.Name, by))
			} else {
				// ⭐ WSL-150: this tool's own build, stopped before its marker.
				st.Problems = append(st.Problems, interruptedBuildNote(b.cfg.Base.Name, rec)+
					". Run "+ToolCommand("base remove --yes")+" to remove it, and then "+ToolCommand("base ensure")+" to build it again")
			}
		} else {
			st.Problems = append(st.Problems, fmt.Sprintf(
				"%s carries no identity marker, so this tool cannot confirm it built it. `base ensure` adopts one it has a record for", b.cfg.Base.Name))
		}
	} else {
		st.Problems = append(st.Problems, err.Error())
	}
	// ⭐ THE ANSWER SAYS IT RAN A CONTAINER. Without --probe `healthy` is false
	// and is no verdict, and a caller reading JSON could not tell. WSL-137.
	st.Probed = true
	report, verifyErr := b.verify(ctx)
	st.Engine = report.Engine
	st.Cgroup = report.Cgroup
	st.Binfmt = report.Binfmt
	st.Account = report.Account
	st.Access.AutomountGuest = report.Automount
	if verifyErr != nil {
		st.Problems = append(st.Problems, verifyErr.Error())
		// ⭐ A FAILURE IS CLASSIFIED HERE AND NOT ONLY IN ensure, because
		// `base status --probe` is what an agent runs to find out what is wrong.
		// A report that names the condition and not the command is the half of
		// this that was already there.
		if r, ok := staleRunStateRemediation(verifyErr.Error()); ok {
			st.Remediations = append(st.Remediations, r)
		}
		if r, ok := accountStartRemediation(verifyErr.Error()); ok {
			st.Remediations = append(st.Remediations, r)
		}
		return st, nil
	}
	st.Healthy = true
	// ⛔ A CAPABILITY FINDING IS NOT A HEALTH FAILURE. The base runs containers;
	// what it cannot do is account for them, and reporting that as unusable would
	// refuse work over a limitation most jobs never reach. WSL-60.
	if r, ok := cgroupRemediation(report.Cgroup, b.cfg.Base.Systemd); ok {
		st.Remediations = append(st.Remediations, r)
	}
	// ⭐ WHAT /dev/kvm IS TO THE ACCOUNT, and a problem where it was asked to be
	// usable and is not. WSL-130.
	st.Kvm = report.Kvm
	if p := kvmProblem(b.cfg.Base.Kvm, report.Kvm, b.cfg.Base.User); p != "" {
		st.Problems = append(st.Problems, p)
	}
	if r, ok := binfmtRemediation(report.Binfmt); ok {
		st.Remediations = append(st.Remediations, r)
	}
	if r, ok := loginShellRemediation(report.Account); ok {
		st.Remediations = append(st.Remediations, r)
	}
	if r, ok := accountPasswdRemediation(report.Account); ok {
		st.Remediations = append(st.Remediations, r)
	}
	// ⚠ AN ADAPTER'S PROBLEM IS A PROBLEM, NOT AN UNUSABLE BASE. `healthy` answers
	// whether a container runs; a broken adapter makes the verdict 1 through the
	// problems list, with the adapter named.
	st.Adapters = b.probeAdapters(ctx)
	for _, a := range st.Adapters {
		for _, p := range a.Problems {
			st.Problems = append(st.Problems, "adapter "+a.Name+": "+p)
		}
		if r, ok := herdrStaleRemediation(a); ok {
			st.Remediations = append(st.Remediations, r)
		}
	}
	st.Leftovers = b.adapterLeftovers(ctx)
	for _, l := range st.Leftovers {
		st.Remediations = append(st.Remediations, leftoverRemediation(l))
	}
	return st, nil
}

// diskSparseFacts reads the disk's sparse attribute and the space it takes, and
// what .wslconfig sets, and answers the remediation a disk that is not sparse
// earns. WSL-109.
func diskSparseFacts(st *BaseState) (Remediation, bool) {
	st.WslConfigSparseVhd = WslConfigSparseVhd()
	sparse, allocated, ok := DiskAllocation(st.DiskPath)
	if !ok {
		return Remediation{}, false
	}
	st.DiskAllocated = allocated
	if sparse {
		st.DiskSparse = "yes"
		return Remediation{}, false
	}
	st.DiskSparse = "no"
	return Remediation{
		ID:   "disk-not-sparse",
		What: "the base's disk is not sparse, so the space its guest frees stays in " + st.DiskPath,
		Costs: "the disk only grows: a build deleted in the base keeps its space on this machine until the base is rebuilt. " +
			"WSL makes it sparse only while its VM is stopped, and base reclaim names that step",
		Command:    ToolCommand("base reclaim"),
		Repairable: true,
		Advisory:   true,
	}, true
}

// withoutRecordDrift removes the problem the host record raised, for the case
// where the guest has since answered the same question with authority.
func withoutRecordDrift(problems []string) []string {
	out := problems[:0]
	for _, p := range problems {
		if strings.HasPrefix(p, recordDriftPrefix) || strings.HasPrefix(p, recordUserDriftPrefix) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// recordDriftPrefix is how that problem is recognised. ⚠ A prefix rather than
// a whole-string match, because the message names two images and neither is
// known here.
const recordDriftPrefix = "this machine's record says it was built from "
const recordUserDriftPrefix = "this machine's record says the managed account is "

func managedAccountDrift(built, configured string) string {
	if built == configured {
		return ""
	}
	return fmt.Sprintf(
		"this distribution was built for account %q and the configuration says %q. "+
			"Changing the managed account requires: %s",
		built, configured, ToolCommand("base recreate"))
}

type baseRecord struct {
	Schema  string    `json:"schema"`
	Name    string    `json:"name"`
	Image   string    `json:"image"`
	User    string    `json:"user"`
	Created time.Time `json:"created"`
}

func (b *Base) recordPath() string { return filepath.Join(b.home, "base.json") }

func (b *Base) readRecord() (baseRecord, error) {
	var rec baseRecord
	data, err := os.ReadFile(b.recordPath())
	if err != nil {
		return rec, err
	}
	return rec, jsonUnmarshal(data, &rec)
}

// RecordedBaseImage is the image this machine's record says the configured base
// was built from, or an empty string where there is no record for that name. It
// reads and creates nothing beyond the record.
func RecordedBaseImage(cfg Config) string {
	home, err := Home()
	if err != nil {
		return ""
	}
	rec, err := (&Base{cfg: cfg, home: home}).readRecord()
	if err != nil || !strings.EqualFold(rec.Name, cfg.Base.Name) {
		return ""
	}
	return rec.Image
}

func (b *Base) writeRecord() error {
	// ⛔ ONLY BY THE STATE DIRECTORY THAT OWNS THE BASE. A record written by a home
	// that only used it claimed a base another home built. WSL-167.
	if !b.owns() {
		return nil
	}
	if _, err := EnsureHome(); err != nil {
		return err
	}
	rec := baseRecord{
		Schema: "wsl-toolkit-base/1", Name: b.cfg.Base.Name,
		Image: b.cfg.Base.Image, User: b.cfg.Base.User, Created: time.Now().UTC(),
	}
	data, err := jsonMarshalIndent(rec)
	if err != nil {
		return err
	}
	return writeFileAtomic(b.recordPath(), data, 0o600)
}

// Ensure brings the base into a state where a job can run, doing the least that
// achieves it.
//
// ⭐ The recovery path as well as the create path. A registered-but-unusable
// distribution is re-provisioned in place.
//
// ⛔ A REGISTERED BASE IS NEVER REMOVED HERE UNLESS THE CALLER ASKED FOR A REBUILD.
// This comment used to say "the base exists to be wrecked", and the code removed
// and rebuilt every base whose re-provisioning failed: a stalled mirror, a full
// disk or a leftover package lock cost the base's home, credentials and images.
// A failed re-provision now returns its error, keeps the base and names
// `base recreate`. WSL-171.
func (b *Base) Ensure(ctx context.Context, force bool) (BaseState, error) {
	return b.EnsureWith(ctx, force, false)
}

// EnsureWith is Ensure with the repair switch.
//
// ⛔ REPAIR IS OPT IN AND IT ALWAYS WILL BE. Without it this names the condition
// and the exact command and takes no deletion, because a tool that removes
// engine state to make a probe pass is one deletion away from removing something
// else. Ruled 2026-09-10. WSL-61.
func (b *Base) EnsureWith(ctx context.Context, force, repair bool) (BaseState, error) {
	if force {
		return b.ensure(ctx, rebuildAlways, repair)
	}
	return b.ensure(ctx, rebuildNever, repair)
}

// EnsureSwitch is Ensure for a `--preset` switch: it rebuilds a registered base
// only when, read under the base's lock, the base was not built from the
// configured image.
//
// ⛔ THE SWITCH WAS DECIDED BEFORE THE LOCK. Two `base ensure --preset X` calls
// both read the old record and both became a rebuild, so the second waited, then
// removed and rebuilt the base the first had just built from X, with whatever a
// job had started in it since. WSL-169, WSL-172.
func (b *Base) EnsureSwitch(ctx context.Context, repair bool) (BaseState, error) {
	return b.ensure(ctx, rebuildSwitch, repair)
}

// rebuild says when an ensure removes a registered base and builds it again.
type rebuild int

const (
	// rebuildNever keeps a registered base: `base ensure`.
	rebuildNever rebuild = iota
	// rebuildAlways rebuilds it: `base recreate`, which the caller asked for.
	rebuildAlways
	// rebuildSwitch rebuilds it unless it is built from the configured image.
	rebuildSwitch
)

func (b *Base) ensure(ctx context.Context, mode rebuild, repair bool) (BaseState, error) {
	st, err := b.configuredState()
	if err != nil {
		return st, err
	}
	// ⛔ BEFORE ANYTHING ELSE, AND THE ORDER IS THE SAFETY OF IT. A grant whose
	// source is gone fails the provisioner, and below, every base under --force
	// is REMOVED AND REBUILT. So refusing any later would have spent the whole
	// base on a directory that went away. WSL-106.
	if err := GoneGrantsError(st.Access.Mounts); err != nil {
		return st, err
	}
	what := "base ensure"
	switch mode {
	case rebuildAlways:
		what = "base recreate"
	case rebuildSwitch:
		what = "base ensure --preset"
	}
	release, err := b.lock(ctx, what)
	if err != nil {
		return st, err
	}
	defer release()
	// ⛔ THE GRANTS ARE READ AGAIN UNDER THE LOCK. `base grant` and `base revoke`
	// change them while holding it, so a build that waited for one went on with
	// the grants it read before: a revoked grant was written back into the fstab
	// block and mounted again at the next start. WSL-172.
	if changed, err := b.adoptStoredGrants(); err != nil {
		return st, err
	} else if changed {
		if st, err = b.configuredState(); err != nil {
			return st, err
		}
		if gone := GoneGrantsError(st.Access.Mounts); gone != nil {
			return st, gone
		}
	}
	// ⛔ READ UNDER THE LOCK. A status read before it describes a base another
	// process may have built, rebuilt or removed while this one waited. WSL-172.
	st, err = b.statusFrom(ctx, st, false)
	if err != nil {
		return st, err
	}
	force := mode == rebuildAlways
	if mode == rebuildSwitch && st.Registered {
		if built := b.builtImage(ctx); built == b.cfg.Base.Image {
			b.log(b.cfg.Base.Name + " is already built from " + built + ", so it is not rebuilt")
		} else {
			force = true
		}
	}
	// ⛔ A RESERVED INSTANCE IS USED AND REMOVED, AND NEVER BUILT. The refusal comes
	// before a rebuild removes anything, so a recreate cannot cost the base it then
	// may not build again. WSL-167.
	if IsReservedInstance(SelectedInstance.Name) && (!st.Registered || force) {
		return st, reservedRefusal(SelectedInstance.Name)
	}
	// ⛔ THE DEFAULT NEVER BUILDS OR TAKES OVER AN INSTANCE'S `wsl-toolkit-jobs`. An
	// instance `jobs` made before the reservation is registered under the default's
	// canonical name. Refused here, where an ensure would use or build it, and not
	// where the configuration loads, so status, logs and gc still answer. WSL-167.
	if err := b.assertCanonicalFree(); err != nil {
		return st, err
	}
	// ⛔ --repair DELETES THE ENGINE'S RUN STATE AND MAY RE-PROVISION, so only the
	// owner asks for it, healthy base or not. Refused before anything is read from
	// the guest. WSL-167.
	if repair && st.Registered {
		if err := b.assertOwner("repaired", "base ensure --repair"); err != nil {
			return st, err
		}
	}
	// rootfs is the archive a rebuild exported before it removed the base it
	// replaces, and empty where nothing was removed. WSL-169.
	var rootfs string
	if st.Registered && force {
		// ⛔ ONLY THE OWNER REBUILDS. Before prepareRebuild, so nothing is
		// exported, unregistered or imported. WSL-167.
		if err := b.assertOwner("rebuilt", "base recreate"); err != nil {
			return st, err
		}
		if rootfs, err = b.prepareRebuild(ctx); err != nil {
			return st, err
		}
		defer b.dropRootfs(rootfs)
		b.log("rebuilding from nothing, as asked")
		if err := b.remove(ctx); err != nil {
			return st, err
		}
		st.Registered = false
	}
	if st.Registered {
		// ⭐ SAID FIRST, AND SAID PLAINLY, because the reader is usually an
		// agent that has just asked for a base and is about to be told it
		// already has one. A line that reads like progress is a line an agent
		// scrolls past; this one names the distribution and says nothing is
		// being built. WSL-61.
		b.log("this base ALREADY EXISTS: " + b.cfg.Base.Name + " is registered. Nothing will be created; checking what it is")
		// ⛔ WHAT IT IS COMES BEFORE WHETHER IT WORKS. A health probe runs an
		// Alpine CONTAINER successfully inside whatever the distribution is; it
		// proves the engine works and identifies nothing. WSL-42, issue 18: a
		// base built from Arch, with the config since changed to Alpine, was
		// RELABELLED Alpine because the probe passed, and `base status` then
		// reported no drift over a guest whose /etc/os-release still said Arch.
		if err := b.reconcileIdentity(ctx); err != nil {
			return st, err
		}
		if report, err := b.verify(ctx); err == nil {
			b.log("the engine answers: " + report.Engine)
			for _, h := range report.Held {
				b.log("another configuration's grant stays mounted: " + h)
			}
			return b.finishEnsure(ctx, st)
		} else {
			b.log("it cannot: " + err.Error())
			last := err
			// ⛔ RE-PROVISIONING CANNOT CLEAR STATE THAT IS THE ENGINE'S, and
			// this used to try anyway, report honestly that it still could not
			// run a container, and stop. That left the operator to run by hand a
			// deletion podman itself had already prescribed. WSL-61.
			if r, ok := staleRunStateRemediation(err.Error()); ok {
				if !repair {
					b.log("")
					b.log("⛔ THIS IS NOT FIXED BY REBUILDING, and this command will not fix it either.")
					b.log("   " + r.What)
					b.log("   " + r.Costs)
					b.log("   RUN THIS INSTEAD:  " + r.Command)
					b.log("")
					st.Remediations = append(st.Remediations, r)
					// ⛔ THE ENGINE'S OWN MESSAGE IS CARRIED, not replaced.
					// A caller downstream classifies from the text, and a
					// refusal that drops what podman said cannot be recognised
					// as the condition it is refusing over: `ready` answered
					// with `base ensure`, the command that had just refused.
					// Caught by the case that asserts otherwise, before it
					// shipped.
					return st, StaleRefusalError(r, err)
				}
				b.log("clearing the engine run state this boot invalidated, as asked")
				if out, err := b.repairRunState(ctx); err != nil {
					return st, fmt.Errorf("--repair could not clear the engine run state: %w", err)
				} else {
					for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
						if line != "" {
							b.log("  " + strings.TrimSpace(line))
						}
					}
				}
				// ⛔ THE STATE IS READ BACK, and the repair is not reported as a
				// success until a container has actually run.
				if report, err := b.verify(ctx); err == nil {
					b.log("the engine answers: " + report.Engine)
					return b.finishEnsure(ctx, st)
				} else {
					b.log("it still cannot after the repair: " + err.Error())
					last = err
				}
			}
			// ⛔ A PROBE THAT NEVER RAN ITS CONTAINER SAID NOTHING ABOUT THE BASE.
			// Re-provisioning over a registry that refused the probe image ran a
			// full upgrade, restarted WSL's VM under every job, and could end in a
			// rebuild. This refuses and keeps the base as it is. WSL-174.
			var nj *ProbeNotJudgedError
			if errors.As(last, &nj) {
				b.log("⛔ THE BASE WAS NOT JUDGED, so it is left as it is: " + nj.Error())
				return st, b.notJudgedRefusal(nj)
			}
			// ⛔ NO RESTART OVER ANOTHER CONFIGURATION'S LIVE GRANTS UNLESS ASKED.
			// Its grant is not in this file's block, so the restart dropped its
			// checkout and ended every agent in the base, and that file's next
			// ensure did the same back. The grant lock is held through the
			// provisioning, so no grant lands between the read and the restart.
			// WSL-179.
			release, err := LockGrants(ctx, b.log)
			if err != nil {
				return st, err
			}
			defer release()
			if held, err := b.heldGrants(ctx); err != nil {
				b.log("the grant ledger could not be read (" + err.Error() + "), so no grant is known to be held")
			} else if refusal := HeldGrantsError(b.cfg.Base.Name, held); refusal != nil {
				if !repair {
					return st, refusal
				}
				for _, h := range held {
					b.log("restarting over another configuration's grant, as asked: " + h.Target + ", granted by " + h.File)
				}
			}
			// ⛔ ONLY THE OWNER RE-PROVISIONS. It writes the guest's settings from
			// this configuration and restarts the base under every job. After the
			// refusals above, which change nothing, so their diagnoses still show.
			// WSL-167.
			if err := b.assertOwner("re-provisioned", "base ensure"); err != nil {
				return st, err
			}
			b.log("re-provisioning in place")
		}
		restarted, err := b.provision(ctx)
		if err != nil {
			return st, reprovisionFailed(b.cfg.Base.Name, err)
		}
		report, err := b.verify(ctx)
		if err != nil && !restarted {
			// ⚠ A RESTART IS STILL THE ANSWER WHEN NOTHING ELSE WAS. Provisioning
			// left wsl.conf and the fstab block as they were, so it did not stop
			// the base; a base that still does not verify gets the restart every
			// re-provision used to end in. WSL-175.
			b.log("it still cannot: " + err.Error())
			if err := b.restart(ctx); err != nil {
				return st, err
			}
			report, err = b.verify(ctx)
		}
		if err != nil {
			return st, fmt.Errorf("re-provisioned and it still does not verify: %w", err)
		}
		b.log("the engine answers: " + report.Engine)
		return b.finishEnsure(ctx, st)
	}
	if err := b.create(ctx, rootfs); err != nil {
		return st, b.rollbackImported(err)
	}
	// ⚠ A NEW IMPORT IS RESTARTED WHATEVER THE SCRIPTS SAID. Nothing runs in it
	// yet, and its first start ran before any of its settings were written.
	restarted, err := b.provision(ctx)
	if err == nil && !restarted {
		err = b.restart(ctx)
	}
	if err != nil {
		return st, b.rollbackImported(err)
	}
	// ⭐ STAMPED BEFORE IT IS VERIFIED. The marker says what this tool BUILT,
	// which is a fact by the time provisioning has finished; whether it can run
	// a container is a different question and the answer to it is not identity.
	if err := b.wsl.WriteIdentity(ctx, b.cfg.Base.Name, Identity{
		Image: b.cfg.Base.Image, User: b.cfg.Base.User, Tool: toolVersion(),
	}); err != nil {
		return st, b.rollbackImported(err)
	}
	b.imported = false
	b.closeBuildRecord("stamped with its identity marker")
	report, err := b.verify(ctx)
	if err != nil {
		return st, fmt.Errorf("built and it does not verify: %w", err)
	}
	b.log("the engine answers: " + report.Engine)
	if err := b.writeRecord(); err != nil {
		return st, err
	}
	return b.finishEnsure(ctx, st)
}

// adoptStoredGrants replaces the grants this base was handed with the ones its
// configuration file names now, and says whether they differed. A base with no
// file keeps what it was handed. WSL-172.
//
// ⚠ ONLY THE GRANTS. They are the one part of the file another command changes
// under the base's lock; the image a `--preset` gave this call is its own.
func (b *Base) adoptStoredGrants() (bool, error) {
	path := b.cfg.Path()
	if path == "" {
		return false, nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	stored, err := loadConfigFile(path, b.cfg.Source())
	if err != nil {
		return false, fmt.Errorf("the configuration could not be read again under the base's lock: %w", err)
	}
	if reflect.DeepEqual(stored.Base.Mounts, b.cfg.Base.Mounts) {
		return false, nil
	}
	b.log("the grants changed while this waited; using the ones " + path + " names now")
	b.cfg.Base.Mounts = stored.Base.Mounts
	return true, nil
}

// builtImage is the image the base was built from: its record's, or else its
// marker's, or an empty string where neither answers.
func (b *Base) builtImage(ctx context.Context) string {
	if rec, err := b.readRecord(); err == nil && strings.EqualFold(rec.Name, b.cfg.Base.Name) && rec.Image != "" {
		return rec.Image
	}
	if id, err := b.wsl.ReadIdentity(ctx, b.cfg.Base.Name); err == nil {
		return id.Image
	}
	return ""
}

// finishEnsure applies the configured adapters to a base whose engine answers,
// and reads the base back.
//
// ⛔ A FAILED ADAPTER IS RETURNED WITH THE STATE, NOT INSTEAD OF IT. The engine
// works and the record is written by the time this runs, so the caller is told
// both what failed and what the base is.
func (b *Base) finishEnsure(ctx context.Context, st BaseState) (BaseState, error) {
	// ⭐ EVERY ENSURE HOLDS THE ACCOUNT TO bash WHERE THE GUEST HAS IT, so a base
	// an older release built with /bin/sh is moved by its next ensure. WSL-143.
	b.applyLoginShell(ctx)
	// ⭐ THE HANDLERS COME BEFORE THE ADAPTERS AND THEIR FAILURE STOPS NOTHING ELSE. They are
	// registered in every ensure, because another distribution can empty the table
	// while this one runs, and a base whose handlers could not be registered is
	// still a base whose adapters should be installed. WSL-105.
	//
	// ⭐ THE ENGINES COME BEFORE BOTH, and their failure stops neither. binfmt.sh
	// names root's cgroup manager and events backend itself, WSL-168, so it does
	// not wait on root_engine. A failed engines step is returned with the
	// adapters' error, because only an opt-in setting fails it. WSL-130.
	enginesErr := b.ensureEngines(ctx)
	// ⛔ THE HANDLERS FAIL NO ENSURE. A registry that refused the installer, or a base
	// whose root podman could not start it, made every `base ensure` exit 2 over a
	// base whose engine had just run a container, native work included. A job
	// that asks for a foreign platform still registers its own handler. WSL-181.
	//
	// ⛔ AN ADVISORY, NOT A PROBLEM. The first repair made it a problem, and a
	// problem is a verdict: `base ensure` still exited 1, and `ready --ensure`
	// read a base that runs native containers as not ready. It is a capability
	// finding, which is a note, as WSL-60 rules. WSL-181.
	binfmtErr := b.ensureBinfmt(ctx)
	if binfmtErr != nil {
		b.log(binfmtErr.Error())
	}
	withBinfmt := func(s BaseState) BaseState {
		if binfmtErr != nil {
			s.Remediations = append(s.Remediations, binfmtStepRemediation(binfmtErr))
		}
		return s
	}
	ensured, adaptersErr := b.applyAdapters(ctx)
	if err := errors.Join(enginesErr, adaptersErr); err != nil {
		if probed, statusErr := b.Status(ctx, true); statusErr == nil {
			return withBinfmt(keepEnsureFindings(probed, ensured)), err
		}
		return withBinfmt(st), err
	}
	probed, err := b.Status(ctx, true)
	return withBinfmt(keepEnsureFindings(probed, ensured)), err
}

// ensureBinfmt registers the base's QEMU handlers when any is gone, records the
// base's own set, and on a systemd base installs the watcher that registers them
// again. binfmt.sh carries the rule; this only runs it. WSL-105.
func (b *Base) ensureBinfmt(ctx context.Context) error {
	out := &prefixWriter{prefix: "", to: b.logWriter()}
	code, err := b.wsl.Exec(ctx, ExecRequest{
		Distro: b.cfg.Base.Name, User: "root", Script: binfmtScript,
		Env: map[string]string{
			"TK_BINFMT_IMAGE": BinfmtImage,
			"TK_SYSTEMD":      strconv.FormatBool(b.cfg.Base.Systemd),
			"TK_SELF_B64":     base64.StdEncoding.EncodeToString(binfmtScript),
		},
		Timeout: 20 * time.Minute, Stdout: out, Stderr: out,
	})
	out.Flush()
	if err != nil || code != 0 {
		return fmt.Errorf("the QEMU handlers: binfmt.sh exited %d: %s", code, lastToolLine(out.seen.String(), "binfmt: "))
	}
	if !strings.Contains(out.seen.String(), "binfmt-complete") {
		return errors.New("the QEMU handlers: binfmt.sh exited 0 without reaching its last line")
	}
	return nil
}

// lastToolLine is the last line a guest script wrote with its own prefix, which
// is its reason, or a note that it wrote none.
func lastToolLine(seen, prefix string) string {
	reason := "it printed no reason"
	for _, line := range strings.Split(seen, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			reason = strings.TrimPrefix(strings.TrimSpace(line), prefix)
		}
	}
	return reason
}

// rollbackImported removes a distribution this process imported but did not
// stamp. A fresh context is deliberate: cancellation of the build must not
// cancel the operation that prevents a nameless disk being stranded.
//
// ⛔ NOTHING THIS PROCESS DID NOT IMPORT. A build that failed before its own
// import succeeded registered nothing, and whatever is registered under the name
// now is another process's, so it is left alone, disk and all. WSL-172.
func (b *Base) rollbackImported(cause error) error {
	if !b.imported {
		return cause
	}
	b.log("the new base did not finish; removing the incomplete distribution")
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := b.remove(cleanupCtx); err != nil {
		return fmt.Errorf("%w; the incomplete distribution could not be removed: %v", cause, err)
	}
	b.imported = false
	b.log("removed the incomplete distribution")
	return cause
}

// importedInto says whether WSL now has the configured name registered with its
// disk in dir. ⚠ An unreadable registry answers no, which leaves the
// distribution where it is, as before. WSL-172.
func (b *Base) importedInto(dir string) bool {
	disks := b.disks
	if disks == nil {
		disks = registeredDisks
	}
	registered, err := disks()
	if err != nil {
		return false
	}
	for name, at := range registered {
		if strings.EqualFold(name, b.cfg.Base.Name) && at != "" && pathEqual(cleanBasePath(at), cleanBasePath(dir)) {
			return true
		}
	}
	return false
}

// reprovisionFailed is what a failed re-provision of a registered base answers.
//
// ⛔ THE BASE IS KEPT AND THE REBUILD IS NAMED. A rebuild costs the base's home,
// its credentials and its images, so it is the caller's to ask for. WSL-171.
//
// ⚠ A GRANT WHOSE DIRECTORY WENT AWAY WHILE ENSURE RAN keeps its own refusal,
// which names the revoke: a rebuild would refuse over the same grant. WSL-106.
func reprovisionFailed(name string, err error) error {
	if errors.Is(err, ErrGrantSourceGone) {
		return err
	}
	return fmt.Errorf("re-provisioning %s failed, and it was NOT removed: %w. Its home, credentials and images are as they were. "+
		"Fix the cause and run this again, or rebuild it from nothing, losing them, with: %s",
		name, err, ToolCommand("base recreate"))
}

// reconcileIdentity compares what the guest says it is against the record and
// the configuration, and refuses rather than papering over a disagreement.
//
// ⭐ THE COMMENT THAT USED TO BE HERE ARGUED FOR ITSELF THREE LINES ABOVE THE
// DEFECT. It read: "Refreshed on every path that leaves a usable base. A record
// written once describes the first build forever." Somebody reasoned about this
// and reached a conclusion that is half right. A record refreshed on every
// healthy path follows the CONFIG, and the thing it claims to describe is the
// GUEST. It is replaced rather than deleted, because the next reader will have
// the same thought. WSL-42, issue 18.
//
// Three outcomes and no fourth:
//
//	the guest carries a marker      the record is rewritten from THE MARKER
//	it carries none and we built it the marker is written and it is said out loud
//	anything else                   a refusal naming the command that rebuilds
func (b *Base) reconcileIdentity(ctx context.Context) error {
	id, err := b.wsl.ReadIdentity(ctx, b.cfg.Base.Name)
	switch {
	case err == nil:
		// ⛔ THE MARKER WINS. The record lives on the host where an editor can
		// reach it; the marker is inside the distribution and was written by a
		// provisioning run. They can disagree, and this is which one is true.
		if err := b.writeRecordFrom(id); err != nil {
			return err
		}
		if problem := managedAccountDrift(id.User, b.cfg.Base.User); problem != "" {
			b.log(problem)
			return errors.New(problem)
		}
		if id.Image != b.cfg.Base.Image {
			// ⛔ REPORTED AND LEFT VISIBLE. It does not rebuild: a rebuild
			// destroys the thing the operator needs to look at, and `recreate`
			// already exists for the case where they want one.
			b.log(fmt.Sprintf("this distribution was built from %s and the configuration says %s",
				id.Image, b.cfg.Base.Image))
			b.log("run `" + ToolCommand("base recreate") + "` to rebuild it, or change the configuration back")
		}
		return nil

	case errors.Is(err, ErrNoIdentity):
		// The upgrade path, and it is part of this entry rather than a follow-up:
		// a base built before markers existed has none.
		// ⛔ ONLY THE OWNER STAMPS. A home that only used the base stamped it from
		// a record of its own, and the owner then took that marker as the truth.
		// WSL-167.
		if err := b.assertOwner("stamped", "base ensure"); err != nil {
			return err
		}
		// ⛔ A BUILD OF THIS TOOL'S KILLED BEFORE ITS MARKER IS REFUSED, NOT
		// ADOPTED AND NOT REMOVED HERE. It never finished provisioning, so a
		// stamp would name a base that was never built, and a removal is the
		// caller's to ask for (WSL-171). The refusal names the command that
		// removes it. WSL-150.
		if build, ok, _ := b.interruptedBuild(ctx); ok {
			return fmt.Errorf("%s. It is not used as it is. Run %s to remove it, and then %s to build it again",
				interruptedBuildNote(b.cfg.Base.Name, build), ToolCommand("base remove --yes"), ToolCommand("base ensure"))
		}
		rec, recErr := b.readRecord()
		if recErr != nil {
			// ⛔ THIS NAMED `base recreate`, which demands the same marker and
			// refused it too. WSL-150.
			_, _, why := b.interruptedBuild(ctx)
			return fmt.Errorf("%s is registered, carries no wsl-toolkit identity marker and has no record here either, and %s",
				b.cfg.Base.Name, unmarkedAdvice(b.cfg.Base.Name, why))
		}
		b.log(b.cfg.Base.Name + " predates the identity marker and this tool's own record describes it; stamping it")
		if err := b.wsl.WriteIdentity(ctx, b.cfg.Base.Name, Identity{
			Image: rec.Image, User: rec.User, Built: rec.Created, Tool: toolVersion(),
		}); err != nil {
			return err
		}
		b.log("wrote " + IdentityPath)
		return nil

	default:
		return err
	}
}

// writeRecordFrom writes the host record from what the GUEST said, so the two
// cannot drift apart by the record following the configuration.
//
// ⛔ AND ONLY BY THE STATE DIRECTORY THAT OWNS THE BASE. WSL-167.
func (b *Base) writeRecordFrom(id Identity) error {
	if !b.owns() {
		return nil
	}
	if _, err := EnsureHome(); err != nil {
		return err
	}
	rec := baseRecord{
		Schema: "wsl-toolkit-base/1", Name: b.cfg.Base.Name,
		Image: id.Image, User: id.User, Created: id.Built,
	}
	data, err := jsonMarshalIndent(rec)
	if err != nil {
		return err
	}
	return writeFileAtomic(b.recordPath(), data, 0o600)
}

// toolVersion is the product version where it can be read, and an empty string
// where it cannot. ⚠ A marker with no version is still a marker; refusing to
// stamp a distribution because a version string could not be read would trade a
// working base for a cosmetic field.
func toolVersion() string {
	return Version
}

// rebuildEngine answers the host engine a rebuild will export with, or refuses
// the rebuild while nothing has been removed.
//
// ⛔ THE REMOVAL CAME FIRST AND THE ENGINE SECOND. A rebuild unregistered the
// base, then asked for a host engine, and an engine that did not answer left
// the caller with no base at all. podssh lost its jobs base this way and built
// it again by hand. So the engine is asked here, through the same resolution
// and the same deadline the export uses, and create is handed the one that
// answered. WSL-169.
//
// ⚠ THE IMAGE REFERENCE IS READ FIRST, because the export refuses one that is
// not fully qualified and reading it costs nothing.
func (b *Base) rebuildEngine(ctx context.Context) (*Engine, error) {
	if err := ValidateImageRef(b.cfg.Base.Image); err != nil {
		return nil, fmt.Errorf("%s was NOT removed: base.image cannot be exported: %w", b.cfg.Base.Name, err)
	}
	b.log("asking the host engine before " + b.cfg.Base.Name + " is removed")
	engine, err := FindEngine(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s was NOT removed: a rebuild exports its rootfs through a host engine, and %w", b.cfg.Base.Name, err)
	}
	return engine, nil
}

// prepareRebuild does everything a rebuild can do before the base it replaces is
// removed, and answers the rootfs archive the import will take.
//
// ⛔ PREPARE, THEN COMMIT. An engine that answers was not enough: the rebuild
// removed the base and only then pulled the image, exported it and measured the
// space, so a registry that refused the pull or a volume too small for the
// import left no base at all. The pull, the export and the space check come
// first now, and any of them refusing keeps the base. WSL-169.
//
// ⚠ THE OLD DISK COUNTS AS FREE. The removal frees it before the import, so a
// rebuild that fits today is not refused for the space the base it replaces holds.
func (b *Base) prepareRebuild(ctx context.Context) (string, error) {
	engine, err := b.rebuildEngine(ctx)
	if err != nil {
		return "", err
	}
	b.log(fmt.Sprintf("host engine: %s, platform %s", engine.Name, engine.Platform()))
	rootfs, err := b.exportRootfs(ctx, engine, b.diskBytes())
	if err != nil {
		return "", fmt.Errorf("%s was NOT removed: %w", b.cfg.Base.Name, err)
	}
	return rootfs, nil
}

// diskBytes is what the base's disk takes on its volume, or its size where that
// cannot be read, or 0.
func (b *Base) diskBytes() int64 {
	disk := filepath.Join(b.Dir(), "ext4.vhdx")
	if _, allocated, ok := DiskAllocation(disk); ok {
		return allocated
	}
	if size, ok := FileSize(disk); ok {
		return size
	}
	return 0
}

// exportRootfs exports the configured image into an archive of this process's
// own and checks the volume holds its import, with `freed` bytes about to be
// given back. It answers the archive, or removes it and refuses.
//
// ⛔ ONE ARCHIVE PER PROCESS. Every build wrote `base-rootfs.tar` in the state
// directory, truncating it, so a second build truncated the first one's archive
// under its import and then deleted it. WSL-172.
func (b *Base) exportRootfs(ctx context.Context, engine *Engine, freed int64) (string, error) {
	if _, err := EnsureHome(); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(b.home, "base-rootfs-*.tar")
	if err != nil {
		return "", err
	}
	rootfs := f.Name()
	if err := f.Close(); err != nil {
		b.dropRootfs(rootfs)
		return "", err
	}
	if err := engine.ExportRootfs(ctx, b.cfg.Base.Image, rootfs, b.log); err != nil {
		b.dropRootfs(rootfs)
		return "", err
	}
	if err := assertImportSpaceFreeing(b.Dir(), rootfs, BaseSpaceFloor, freed, b.log); err != nil {
		b.dropRootfs(rootfs)
		return "", err
	}
	return rootfs, nil
}

// dropRootfs removes an archive exportRootfs made, and says so where it cannot.
func (b *Base) dropRootfs(rootfs string) {
	if err := os.Remove(rootfs); err != nil && !errors.Is(err, os.ErrNotExist) {
		b.log("the rootfs archive is still on disk: " + rootfs)
	}
}

// create imports the base from its image. ⭐ `rootfs` is the archive a rebuild
// exported before it removed anything; empty means nothing was removed, and the
// archive is made here.
func (b *Base) create(ctx context.Context, rootfs string) error {
	// ⛔ A RESERVED INSTANCE IS NEVER BUILT. One made before the reservation stays
	// reachable so it can be removed, and building it again is creation. WSL-167.
	if IsReservedInstance(SelectedInstance.Name) {
		return reservedRefusal(SelectedInstance.Name)
	}
	if err := b.assertNameStillDefault(rootfs); err != nil {
		return err
	}
	// ⭐ THE FIRST THING THAT WRITES, so this is where the state directory is
	// brought into existence. Everything above it reads.
	if _, err := EnsureHome(); err != nil {
		return err
	}
	dir := b.Dir()
	if _, err := os.Stat(dir); err == nil {
		// ⛔ Never import over a leftover directory: wsl --import into one that
		// already holds a disk is how two distributions share one file. And never
		// delete one a registered distribution keeps its disk in. WSL-177.
		if err := b.assertNoDiskAt(dir); err != nil {
			return fmt.Errorf("%s is left over from an earlier build: %w", dir, err)
		}
		if err := b.assertRecordNames(dir); err != nil {
			return fmt.Errorf("%s is left over from an earlier build: %w", dir, err)
		}
		if err := b.clearLeftover(dir); err != nil {
			return fmt.Errorf("%s is left over from an earlier build and could not be cleared: %w", dir, err)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if rootfs == "" {
		var err error
		if rootfs, err = b.freshRootfs(ctx); err != nil {
			return err
		}
		defer b.dropRootfs(rootfs)
	}
	// ⛔ OWNERSHIP IS RECORDED BEFORE THE IMPORT REGISTERS ANYTHING, so a build
	// killed before its marker is still this tool's to remove. WSL-150.
	if err := b.recordBuildStart(); err != nil {
		return fmt.Errorf("could not record the build before importing it: %w", err)
	}
	b.log("importing as WSL2 distribution " + b.cfg.Base.Name)
	if err := b.wsl.Import(ctx, b.cfg.Base.Name, dir, rootfs); err != nil {
		// ⚠ AN IMPORT THAT FAILED CAN STILL HAVE REGISTERED. wsl.exe killed by
		// its bound or by a cancel leaves the service to finish it, and nothing
		// then cleared the distribution up: the next ensure refused it as
		// unmarked, and so did `base remove`. The disk in the directory this
		// process just made, under the lock, is the proof the import was its own;
		// a name alone is not. WSL-172.
		//
		// ⛔ THE BUILD RECORD STAYS OPEN WHEN THIS ANSWERS NO. No is also what an
		// unreadable registry answers, and what a registration the service has
		// not finished yet answers. A record closed here left that late
		// registration unmarked and unrecorded, which only `wsl --unregister`
		// removed. An open record over nothing registered removes nothing, since
		// interruptedBuild also demands WSL's registration of the disk here. The
		// next build's stamp, or a removal, closes it. WSL-150.
		if b.importedInto(dir) {
			b.imported = true
		}
		return err
	}
	// ⛔ FROM HERE THIS PROCESS OWNS THE DISTRIBUTION IT IS BUILDING, marker or
	// not, so a build that dies before stamping can still be cleared up. WSL-172.
	b.imported = true
	return nil
}

// assertRecordNames refuses to clear dir while this machine's record says the
// base in it is a distribution other than the configured one. WSL-177.
//
// ⛔ THE RECORD NAMES WHOSE DISK IT IS. A rename, or a configuration naming
// another distribution, left the configured name unlisted, and the disk of the
// base the record describes was deleted as a leftover.
func (b *Base) assertRecordNames(dir string) error {
	rec, err := b.readRecord()
	if err != nil || rec.Name == "" || strings.EqualFold(rec.Name, b.cfg.Base.Name) {
		return nil
	}
	return fmt.Errorf("%s was NOT deleted: %s records it as the disk of %s, and the configuration names %s. "+
		"If %s is this base under another name, set base.name to it",
		dir, b.recordPath(), rec.Name, b.cfg.Base.Name, rec.Name)
}

// leftoverDisk is the file a leftover directory holds that makes it a disk.
const leftoverDisk = "ext4.vhdx"

// clearLeftover makes way for an import in dir. ⚠ A directory holding a disk
// is moved aside and the new name said, and anything else is deleted as before.
//
// ⛔ AN ORPHANED DISK IS MOVED, NOT DELETED. Nothing registered keeps it, but it
// can still be a base whose registration was lost, with a home, credentials and
// images in it. The build needs the directory, not the disk gone. WSL-177.
func (b *Base) clearLeftover(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, leftoverDisk)); err != nil {
		if err := RemoveInside(b.home, dir); err != nil {
			return err
		}
		b.log("removed a leftover " + dir)
		return nil
	}
	aside := filepath.Join(b.home, "base.orphaned-"+time.Now().UTC().Format("20060102T150405Z"))
	if err := os.Rename(dir, aside); err != nil {
		return err
	}
	b.log("⚠ moved a leftover disk aside to " + aside + ". No registered distribution keeps it. Delete it once nothing in it is needed")
	return nil
}

// freshRootfs exports the configured image into an archive of this process's own
// for a new build, which removed nothing, and answers the archive.
func (b *Base) freshRootfs(ctx context.Context) (string, error) {
	if b.archive != nil {
		if _, err := EnsureHome(); err != nil {
			return "", err
		}
		rootfs := filepath.Join(b.home, fmt.Sprintf("base-rootfs-case-%d.tar", os.Getpid()))
		if err := b.archive(ctx, rootfs); err != nil {
			b.dropRootfs(rootfs)
			return "", err
		}
		return rootfs, nil
	}
	engine, err := FindEngine(ctx)
	if err != nil {
		return "", fmt.Errorf("the base is built from an OCI image, so a host engine is required: %w", err)
	}
	b.log(fmt.Sprintf("host engine: %s, platform %s", engine.Name, engine.Platform()))
	return b.exportRootfs(ctx, engine, 0)
}

// assertImportSpace is the one preflight in front of every `wsl --import`: a
// floor plus twice the rootfs archive, measured on the volume that will hold the
// disk.
func assertImportSpace(dir, tarPath string, floor int64, log func(string)) error {
	return assertImportSpaceFreeing(dir, tarPath, floor, 0, log)
}

// freeSpace is FreeSpace, and a case's stand-in for it.
var freeSpace = FreeSpace

// assertImportSpaceFreeing is assertImportSpace for an import that follows a
// removal giving `freed` bytes back to the same volume.
func assertImportSpaceFreeing(dir, tarPath string, floor, freed int64, log func(string)) error {
	size, ok := FileSize(tarPath)
	if !ok {
		return fmt.Errorf("cannot measure %s", tarPath)
	}
	need := floor + 2*size
	free, ok, err := freeSpace(dir)
	if err != nil {
		return err
	}
	if !ok {
		// ⚠ "could not measure" is a third answer, and treating it as either of
		// the other two is a lie or a needless refusal.
		log("free space on this volume could not be read; importing anyway")
		return nil
	}
	if freed > 0 {
		log(fmt.Sprintf("space: %s wanted, %s free and %s more once the old disk is removed",
			HumanBytes(need), HumanBytes(free), HumanBytes(freed)))
		if free+freed < need {
			return fmt.Errorf("NOT ENOUGH DISK SPACE for %s. About %s is wanted, and %s is free on the volume holding it with %s more once the old disk is removed",
				dir, HumanBytes(need), HumanBytes(free), HumanBytes(freed))
		}
		return nil
	}
	log(fmt.Sprintf("space: %s wanted, %s free", HumanBytes(need), HumanBytes(free)))
	if free < need {
		return fmt.Errorf("NOT ENOUGH DISK SPACE for %s. About %s is wanted and %s is free on the volume holding it. "+
			"Nothing has been imported and nothing is registered", dir, HumanBytes(need), HumanBytes(free))
	}
	return nil
}

// provision runs the provisioner and writes the grants block, and answers whether
// it restarted the distribution to apply them.
func (b *Base) provision(ctx context.Context) (bool, error) {
	b.log("provisioning: a rootless engine and the " + b.cfg.Base.User + " account")
	automount, err := NormalizeAutomount(b.cfg.Base.Automount)
	if err != nil {
		return false, err
	}
	interop, err := NormalizeBaseInterop(b.cfg.Base.Interop)
	if err != nil {
		return false, err
	}
	toolset, err := NormalizeBaseToolset(b.cfg.Base.Toolset)
	if err != nil {
		return false, err
	}
	sharedTmpfs, err := NormalizeSharedTmpfs(b.cfg.Base.SharedTmpfs)
	if err != nil {
		return false, err
	}
	// ⚠ Resolved before anything runs, so a grant whose source is gone refuses the
	// provisioning rather than half of it.
	if _, _, _, err := baseMountPayloads(b.cfg); err != nil {
		return false, err
	}
	// ⛔ A RESTART AN EARLIER RUN OWED IS STILL OWED. A run that wrote wsl.conf or
	// the fstab block and then failed before its restart left them as this run
	// writes them, so this run would read "unchanged" over settings WSL never read.
	// The note is written before anything runs and only a restart clears it. WSL-175.
	owed := b.oweRestart()
	out := &prefixWriter{prefix: "", to: b.logWriter()}
	code, err := b.wsl.Exec(ctx, b.provisionRequest(automount, interop, toolset, sharedTmpfs, out))
	out.Flush()
	if err != nil || code != 0 {
		return false, fmt.Errorf("provisioning exited %d: %w", code, err)
	}
	said := out.seen.String()
	if !strings.Contains(said, "provision-complete") {
		// ⛔ The script prints that line last, so its absence over a zero exit is
		// a step that exited 0 having done nothing.
		return false, errors.New("provisioning exited 0 without reaching its last line")
	}
	// ⭐ THE BLOCK IS WRITTEN BY THE SCRIPT `base grant` RUNS LIVE, so provisioning
	// and a live grant cannot write two different blocks.
	grants, err := b.applyGrants(ctx, b.cfg, GrantsWrite, BaseMount{})
	if err != nil {
		return false, err
	}
	// ⛔ ONLY WHAT WSL READS AT START IS A REASON TO STOP THE BASE. Every
	// re-provision ended in a restart, so a transient verify failure stopped every
	// job and agent running in the base over settings that had not changed. The
	// scripts say whether wsl.conf or the fstab block changed. WSL-175.
	if !owed && !startSettingsChanged(said, grants) {
		b.log("/etc/wsl.conf and the fstab block are as they were, so the base is not restarted")
		b.restartPaid()
		// ⚠ THE SWEEP STILL RUNS, as it did inside provision.sh on every run
		// before it moved to drives.sh. WSL-153.
		return false, b.sweepDrivesWhenOff(ctx)
	}
	return true, b.restart(ctx)
}

// restartOwedName is the note, in the instance's state directory, that a
// provisioning run started and no restart has followed it yet. WSL-175.
const restartOwedName = "base.restart-owed"

// oweRestart notes that this provisioning run may change what WSL reads at
// start, and answers whether an earlier run left a restart owed.
//
// ⚠ A NOTE THAT CANNOT BE WRITTEN OWES THE RESTART, as every re-provision did
// before WSL-175.
func (b *Base) oweRestart() bool {
	note := filepath.Join(b.home, restartOwedName)
	_, statErr := os.Stat(note)
	if err := os.MkdirAll(b.home, 0o700); err != nil {
		return true
	}
	if err := writeFileAtomic(note, []byte(b.cfg.Base.Name+"\n"), 0o600); err != nil {
		return true
	}
	return statErr == nil
}

// restartPaid clears the note once nothing is owed.
func (b *Base) restartPaid() {
	if err := os.Remove(filepath.Join(b.home, restartOwedName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		b.log("the note that a restart is owed is still on disk: " + err.Error())
	}
}

// restart stops the distribution so WSL reads /etc/wsl.conf and /etc/fstab again
// at its next start.
func (b *Base) restart(ctx context.Context) error {
	// WSL reads /etc/wsl.conf at start, so without this restart the settings
	// just written appear to have been applied and are not.
	b.log("restarting so WSL re-reads /etc/wsl.conf")
	if err := b.wsl.Terminate(ctx, b.cfg.Base.Name); err != nil {
		return err
	}
	b.restartPaid()
	// ⛔ THE DRIVE MOUNT POINTS ARE SWEPT AFTER THE RESTART, NOT BEFORE IT, and
	// before the verifier reads them. drives.sh says what sweeping first cost.
	// WSL-153, WSL-155.
	return b.sweepDrivesWhenOff(ctx)
}

// startSettingsChanged says whether provisioning changed what WSL reads when the
// distribution starts.
//
// ⚠ A SCRIPT THAT DID NOT SAY COUNTS AS A CHANGE. Only the two lines that say
// nothing changed keep the base running; anything else restarts it, as before.
func startSettingsChanged(provisionOut, grantsOut string) bool {
	return !hasLine(provisionOut, "wsl-conf unchanged") || !hasLine(grantsOut, "fstab unchanged")
}

// hasLine says whether out carries line as a whole line.
func hasLine(out, line string) bool {
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}

// provisionRequest is the one provisioning run: the payload, as root, with the
// configuration's settings in its environment.
func (b *Base) provisionRequest(automount, interop, toolset, sharedTmpfs string, out io.Writer) ExecRequest {
	return ExecRequest{
		Distro: b.cfg.Base.Name,
		User:   "root",
		Script: provisionPayload(),
		Env: map[string]string{
			"TK_USER":              b.cfg.Base.User,
			"TK_UID":               "1000",
			"TK_AUTOMOUNT":         automount,
			"TK_INTEROP":           interop,
			"TK_SYSTEMD":           strconv.FormatBool(b.cfg.Base.Systemd),
			"TK_PASSWORDLESS_SUDO": strconv.FormatBool(b.cfg.Base.PasswordlessSudo),
			"TK_TOOLSET":           toolset,
			"TK_SHARED_TMPFS":      sharedTmpfs,
		},
		Timeout: 30 * time.Minute,
		Stdout:  out,
		Stderr:  out,
	}
}

// baseMountPayloads renders explicit grants for /etc/fstab and for the
// post-restart probe. Both are encoded before they cross into the guest, so a
// Windows path never becomes shell source.
func baseMountPayloads(cfg Config) (fstab64, checks string, mounts []BaseMount, err error) {
	mounts, err = cfg.ResolvedBaseMounts()
	if err != nil {
		return "", "", nil, err
	}
	// ⛔ PROVISIONING AND `base ensure` COME THROUGH HERE, and a source that is
	// gone cannot be mounted or read back, so the one refusal for them is here
	// rather than at each caller. The live step of `base grant` and `base revoke`
	// renders through liveMountPayloads instead. WSL-106.
	if err := GoneGrantsError(mounts); err != nil {
		return "", "", nil, err
	}
	fstab64, checks, err = renderBaseMounts(mounts)
	if err != nil {
		return "", "", nil, err
	}
	return fstab64, checks, mounts, nil
}

// presentGrantChecks is the read-back the verifier runs for the grants whose
// source still exists. ⛔ A gone grant is left out of the CHECK and not of the
// report: `base status` names it with its revoke, and a probe that failed the
// whole engine over it would call a working base unusable. WSL-106.
func presentGrantChecks(cfg Config) (string, error) {
	_, checks, err := liveMountPayloads(cfg)
	return checks, err
}

// goneGrantTargets are the targets of grants whose Windows directory is gone,
// base64 each and space-separated, for the verifier to tolerate and not check.
//
// ⛔ A GONE GRANT'S MOUNT CAN OUTLIVE ITS DIRECTORY until the base restarts,
// and with automount off the verifier refused every Windows mount it was not
// told of, so `base status --probe` called a working base unhealthy over a grant
// it already reported with its revoke. A correctness review found it. WSL-106.
func goneGrantTargets(cfg Config) (string, error) {
	mounts, err := cfg.ResolvedBaseMounts()
	if err != nil {
		return "", err
	}
	var out []string
	for _, m := range mounts {
		if m.Missing {
			out = append(out, base64.StdEncoding.EncodeToString([]byte(m.Target)))
		}
	}
	return strings.Join(out, " "), nil
}

// liveMountPayloads renders the grants whose sources exist, for the live step of
// `base grant` and `base revoke` and for the verifier.
//
// ⛔ A GONE GRANT IS LEFT OUT HERE AND NOT REFUSED. It cannot be mounted, and
// refusing over it made every revoke refuse while two grants were gone, each
// refusal naming the other's revoke, so neither could be removed. `base grant`
// still refuses beside a gone grant, in PlanGrant, before it reaches this.
// WSL-106.
func liveMountPayloads(cfg Config) (fstab64, checks string, err error) {
	mounts, err := cfg.ResolvedBaseMounts()
	if err != nil {
		return "", "", err
	}
	present := mounts[:0:0]
	for _, m := range mounts {
		if !m.Missing {
			present = append(present, m)
		}
	}
	return renderBaseMounts(present)
}

// renderBaseMounts writes the fstab block and the read-back lines for a set of
// grants whose sources exist.
func renderBaseMounts(mounts []BaseMount) (fstab64, checks string, err error) {
	var fstab strings.Builder
	var verify strings.Builder
	for _, mount := range mounts {
		source := filepath.ToSlash(mount.Source)
		if runtimeVolume := filepath.VolumeName(mount.Source); runtimeVolume != "" && strings.HasPrefix(runtimeVolume, `\\`) {
			return "", "", fmt.Errorf("base.mounts source %q is a network path; this release grants local Windows directories only", mount.Source)
		}
		fmt.Fprintf(&fstab, "%s %s drvfs metadata,%s,uid=1000,gid=1000,umask=022,fmask=011,nofail 0 0\n",
			fstabEscape(source), fstabEscape(mount.Target), mount.Mode)
		fmt.Fprintf(&verify, "%s %s\n",
			base64.StdEncoding.EncodeToString([]byte(mount.Target)), mount.Mode)
	}
	if fstab.Len() > 0 {
		fstab64 = base64.StdEncoding.EncodeToString([]byte(fstab.String()))
	}
	return fstab64, strings.TrimSpace(verify.String()), nil
}

// fstabEscape uses the escape sequences fstab accepts for fields. Sources are
// already canonical host paths and targets are confined to /workspaces.
func fstabEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\134`)
	value = strings.ReplaceAll(value, " ", `\040`)
	value = strings.ReplaceAll(value, "\t", `\011`)
	value = strings.ReplaceAll(value, "#", `\043`)
	return value
}

// EnsurePlatform restores the QEMU handler for one non-native Linux platform.
//
// ⚠ THE TABLE IS EVERY DISTRIBUTION'S, AND IT EMPTIES WHILE THIS BASE RUNS. A
// registration ends when WSL's utility VM stops, and also when another
// distribution unregisters everything, which the Podman machine does as it stops,
// with the VM and this base still running. This covers one job's platform;
// binfmt.sh and its watcher cover the rest. WSL-105.
func (b *Base) EnsurePlatform(ctx context.Context, platform string) error {
	normalized, err := NormalizePlatform(platform)
	if err != nil || normalized == "" {
		return err
	}
	// ⭐ THE NATIVE PLATFORM COSTS NOTHING TO PREPARE, and it is now the value a
	// caller who asked for nothing gets, so this runs on every job. Answering it
	// here keeps the common path free of a guest round trip.
	if normalized == NativePlatform() {
		return nil
	}
	target, handler, err := platformBinfmt(normalized)
	if err != nil {
		return err
	}
	out, stderr, code, runErr := b.captureAs(ctx, "root", []byte("uname -m\n"), nil, 2*time.Minute)
	if runErr != nil || code != 0 {
		return fmt.Errorf("could not read the base architecture (exit %d): %s", code, guestFailure(stderr, out))
	}
	if nativePlatformArch(strings.TrimSpace(firstLine(out))) == target {
		return nil
	}
	// ⭐ binfmt.sh registers one platform when TK_ARCH names it, with the root
	// podman the ensure uses. WSL-181.
	out, stderr, code, runErr = b.captureAs(ctx, "root", binfmtScript, map[string]string{
		"TK_ARCH": target, "TK_HANDLER": handler, "TK_BINFMT_IMAGE": BinfmtImage,
	}, 20*time.Minute)
	if runErr != nil || code != 0 {
		return fmt.Errorf("could not prepare %s (exit %d): %s", normalized, code, guestFailure(stderr, out))
	}
	if !strings.Contains(out, "binfmt-ready "+handler) {
		return fmt.Errorf("the %s handler did not report that it is ready", handler)
	}
	return nil
}

func platformBinfmt(platform string) (arch, handler string, err error) {
	parts := strings.Split(platform, "/")
	arch = parts[1]
	if arch == "arm" {
		return "arm", "qemu-arm", nil
	}
	handlers := map[string]string{
		"386": "qemu-i386", "amd64": "qemu-x86_64", "arm64": "qemu-aarch64",
		"loong64": "qemu-loongarch64", "mips64": "qemu-mips64", "mips64le": "qemu-mips64el",
		"ppc64le": "qemu-ppc64le", "riscv64": "qemu-riscv64", "s390x": "qemu-s390x",
	}
	handler = handlers[arch]
	if handler == "" {
		return "", "", fmt.Errorf("%s has no QEMU handler mapping", platform)
	}
	return arch, handler, nil
}

func nativePlatformArch(value string) string {
	switch value {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	case "i386", "i686":
		return "386"
	default:
		return value
	}
}

// verifyReport is what verify.sh printed, each row read by its name.
type verifyReport struct {
	Engine  string
	Cgroup  *CgroupState
	Binfmt  *BinfmtState
	Account *AccountState
	// Automount is what the guest's drives are, from the script's `automount` row.
	Automount string
	// Held are the live mounts the grant ledger gives another configuration file,
	// each "TARGET, granted by FILE". WSL-179.
	Held []string
	// Kvm is the script's `kvm` row. WSL-130.
	Kvm string
}

// verify runs a container as the unprivileged account and reads what came back.
//
// ⭐ THE REPORT IS NOT A HEALTH VERDICT. The error says whether this base works;
// the CgroupState says what it can account for while it works, and a base with no
// delegation is healthy and limited rather than broken. WSL-60.
//
// ⚠ A REFUSAL STILL CARRIES THE DRIVES. The script reads them before any check can
// refuse, so a base whose drives disagree with its configuration reports what the
// guest has beside the refusal. WSL-84.
func (b *Base) verify(ctx context.Context) (verifyReport, error) {
	half := func() string {
		var raw [6]byte
		if _, err := rand.Read(raw[:]); err != nil {
			// A failure to read randomness is not a reason to refuse to verify,
			// and it is reported through the marker rather than swallowed.
			return "nrandom"
		}
		return hex.EncodeToString(raw[:])
	}
	m1, m2 := half(), half()
	env, err := verifyEnv(b.cfg, m1, m2)
	if err != nil {
		return verifyReport{}, err
	}
	out, stderr, code, err := b.captureAs(ctx, b.cfg.Base.User, verifyScript, env, 20*time.Minute)
	drives := parseAutomount(out)
	if err != nil || code != 0 {
		if nj, ok := probeNotJudged(VerifyImage, code, stderr); ok {
			return verifyReport{Automount: drives}, nj
		}
		if local, ok := probePullFailure(VerifyImage, code, stderr); ok {
			return verifyReport{Automount: drives}, local
		}
		return verifyReport{Automount: drives}, verifyError(b.cfg.Base.User, code, out, stderr)
	}
	// ⛔ Compared with whitespace removed: a tty wraps a long line, so a marker
	// that arrived correctly can fail an exact match.
	flat := strings.Join(strings.Fields(out), "")
	if !strings.Contains(flat, m1+m2) {
		return verifyReport{Automount: drives}, fmt.Errorf("the container ran and did not return the marker: %s", guestFailure(out, stderr))
	}
	engine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "engine ") {
			engine = strings.TrimSpace(strings.TrimPrefix(line, "engine "))
		}
	}
	// ⚠ `engine ` MATCHES BEFORE `engine-rootless ` DOES NOT, and it does not:
	// the prefix compared carries a trailing space and that row's key does not.
	return verifyReport{Engine: engine, Cgroup: parseCapabilities(out), Binfmt: parseBinfmt(out),
		Account: parseAccount(out), Automount: drives, Held: parseHeldMounts(out), Kvm: parseKvm(out)}, nil
}

// verifyEnv is what verify.sh is handed for a configuration, with the two halves
// of the marker its container joins.
func verifyEnv(cfg Config, m1, m2 string) (map[string]string, error) {
	automount, err := NormalizeAutomount(cfg.Base.Automount)
	if err != nil {
		return nil, err
	}
	interop, err := NormalizeBaseInterop(cfg.Base.Interop)
	if err != nil {
		return nil, err
	}
	toolset, err := NormalizeBaseToolset(cfg.Base.Toolset)
	if err != nil {
		return nil, err
	}
	sharedTmpfs, err := NormalizeSharedTmpfs(cfg.Base.SharedTmpfs)
	if err != nil {
		return nil, err
	}
	checks, err := presentGrantChecks(cfg)
	if err != nil {
		return nil, err
	}
	gone, err := goneGrantTargets(cfg)
	if err != nil {
		return nil, err
	}
	owner, _ := grantOwner(cfg)
	return map[string]string{
		"TK_IMAGE":             VerifyImage,
		"TK_M1":                m1,
		"TK_M2":                m2,
		"TK_AUTOMOUNT":         automount,
		"TK_INTEROP":           interop,
		"TK_SYSTEMD":           strconv.FormatBool(cfg.Base.Systemd),
		"TK_PASSWORDLESS_SUDO": strconv.FormatBool(cfg.Base.PasswordlessSudo),
		"TK_TOOLSET":           toolset,
		"TK_SHARED_TMPFS":      sharedTmpfs,
		"TK_MOUNT_CHECKS":      checks,
		"TK_MOUNT_GONE":        gone,
		"TK_CONFIG_OWNER":      owner,
	}, nil
}

// parseHeldMounts reads verify.sh's `held-mount TARGET FILE64` rows.
func parseHeldMounts(out string) []string {
	var held []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[0] != "held-mount" {
			continue
		}
		name, err := base64.StdEncoding.DecodeString(f[2])
		if err != nil {
			continue
		}
		held = append(held, f[1]+", granted by "+string(name))
	}
	return held
}

// verifyRefused is the exit code verify.sh gives its own refusals.
const verifyRefused = 3

// verifyNotJudged is the exit code verify.sh gives a probe image it could not
// pull. WSL-174.
const verifyNotJudged = 4

// ProbeNotJudgedError is a health probe that never reached its container: the
// image it runs was absent and the registry did not hand it over.
//
// ⛔ IT IS NOT A VERDICT ON THE BASE. The probe pulled inside the run, so Docker
// Hub refusing one address read as a base that cannot run a container, and
// `base ensure` answered it with a full upgrade, a restart of WSL's VM that
// ended every job, and possibly a rebuild. Ensure refuses over this and keeps
// the base, and a status reports it as a problem that names the registry. WSL-174.
type ProbeNotJudgedError struct {
	Image    string
	Registry string
	Reason   string
}

func (e *ProbeNotJudgedError) Error() string {
	return fmt.Sprintf("not judged: registry %s unreachable, so the health probe could not pull %s: %s",
		e.Registry, e.Image, e.Reason)
}

// probeNotJudged reads verify.sh's pull failure: its own exit code and its own
// `verify-pull:` line, both, as a refusal needs both. WSL-174.
//
// ⛔ ONLY THE REGISTRY'S OWN FAILURE IS NOT JUDGED. Every failed pull read as
// "registry unreachable", and Ensure then refused the re-provisioning that
// repairs a pull the base broke itself: a lost /etc/subuid range answers
// `insufficient UIDs or GIDs`, and a full disk answers `no space left on
// device`. Both are the base's, and both keep the re-provisioning they had.
// WSL-174.
//
// ⛔ AN UNKNOWN REASON IS NOT JUDGED. The first repair listed the registry's
// words and judged every other reason against the base, so `unexpected EOF`, a
// GOAWAY or `context deadline exceeded` re-provisioned, and a provisioning that
// failed too removed the base. The base's own causes are the short, known list,
// and anything else refuses and keeps the base. WSL-174.
func probeNotJudged(image string, code int, stderr string) (*ProbeNotJudgedError, bool) {
	if code != verifyNotJudged {
		return nil, false
	}
	reason, ok := lastPrefixed(stderr, "verify-pull: ")
	if !ok || basePullFailure(reason) {
		return nil, false
	}
	return &ProbeNotJudgedError{Image: image, Registry: imageRegistry(image), Reason: reason}, true
}

// probePullFailure is verify.sh's pull failure whose cause is the base, which an
// ordinary verify failure reports so that Ensure re-provisions. WSL-174.
func probePullFailure(image string, code int, stderr string) (error, bool) {
	if code != verifyNotJudged {
		return nil, false
	}
	reason, ok := lastPrefixed(stderr, "verify-pull: ")
	if !ok || !basePullFailure(reason) {
		return nil, false
	}
	return fmt.Errorf("the health probe could not pull %s, and the cause is in the base, not the registry: %s", image, reason), true
}

// basePullMarks are the words of a pull the base broke itself, lower case: its
// ID ranges, its disk, and its own storage and configuration files. Re-provisioning
// repairs each. WSL-174.
var basePullMarks = []string{
	"insufficient uids or gids", "newuidmap", "newgidmap",
	"no space left on device", "disk quota exceeded", "read-only file system",
	"permission denied", "operation not permitted", "no such file or directory",
}

// networkPullMarks are the words of a pull that failed on the way to the
// registry. ⚠ They outrank a base mark: `dial tcp ...: permission denied` is a
// connection refused on the way out, not storage this base cannot write.
var networkPullMarks = []string{
	"pinging container registry", "dial tcp", "net/http", "lookup ",
}

// basePullFailure is true only when a pull's reason names a cause the base
// itself answers, and nothing on the way to the registry.
func basePullFailure(reason string) bool {
	lower := strings.ToLower(reason)
	for _, mark := range networkPullMarks {
		if strings.Contains(lower, mark) {
			return false
		}
	}
	for _, mark := range basePullMarks {
		if strings.Contains(lower, mark) {
			return true
		}
	}
	return false
}

// trustPullMarks are the words of a pull refused over trust: a certificate the
// base did not accept, or credentials the registry did not. Each can be the
// base's own, a CA bundle that was removed or an auth.json gone stale. WSL-174.
var trustPullMarks = []string{"x509:", "tls:", "unauthorized", "authentication required"}

// imageRegistry is the registry host a reference names, or docker.io where the
// first component is not a host, as an engine reads it.
func imageRegistry(ref string) string {
	host, _, ok := strings.Cut(ref, "/")
	if ok && (strings.Contains(host, ".") || strings.Contains(host, ":") || host == "localhost") {
		return host
	}
	return "docker.io"
}

// notJudgedRefusal is what Ensure answers when the probe was not judged: the
// probe's words, and that nothing was changed.
//
// ⛔ A REFUSAL OVER TRUST NAMES WHERE TO LOOK IN THE BASE. A certificate or a
// credential the registry refused can be the base's own, and waiting for the
// registry then never ends, which left `base recreate`, a removal, as the only
// way on. The refusal names the two files and the shell that reaches them, and
// nothing that removes the base. WSL-174.
func (b *Base) notJudgedRefusal(nj *ProbeNotJudgedError) error {
	err := fmt.Errorf("%w. %s was NOT re-provisioned, restarted or rebuilt. Run %s again once %s answers",
		nj, b.cfg.Base.Name, ToolCommand("base ensure"), nj.Registry)
	lower := strings.ToLower(nj.Reason)
	for _, mark := range trustPullMarks {
		if strings.Contains(lower, mark) {
			return fmt.Errorf("%w. If %s answers other machines, the cause can be in this base: "+
				"its CA bundle under /etc/ssl/certs, or the account's containers/auth.json under "+
				"$XDG_RUNTIME_DIR or ~/.config. Read both with %s, then run %s again. Neither needs a rebuild",
				err, nj.Registry, ToolCommand("base shell --root"), ToolCommand("base ensure"))
		}
	}
	return err
}

// verifyError is what a verification that did not pass says.
//
// ⚠ EXIT 3 WITH A `verify:` LINE IS THE SCRIPT'S OWN REFUSAL, made before a
// container runs, so it names the setting the guest disagrees with. WSL-84: a
// drive refusal read "a container did not run", which sends a reader after the
// engine. Any other failure keeps the engine's words, which the stale run state
// remediation classifies from.
//
// ⛔ THE CODE, NOT THE ERROR. Exec answers a guest's nonzero exit with a
// ProcessError beside its code, so a refusal arrives with an error too, and the
// first build that asked for no error never recognised one on a real guest.
//
// ⛔ WSL'S OWN LINES ARE NOT THE GUEST'S. After a restart wsl.exe wrote `wsl: Failed
// to start the systemd user session for 'agent'` ahead of the verifier's refusal,
// and the first line named that: a passwordless sudo refusal read "a container did
// not run as agent (exit 3): wsl: ...". WSL-85. The refusal is the last `verify:`
// line, and any other failure is named by guestReason: the last stderr line that
// is not wsl.exe's. WSL-152.
func verifyError(user string, code int, stdout, stderr string) error {
	if code == verifyRefused {
		if refusal, ok := lastPrefixed(stderr, "verify: "); ok {
			return fmt.Errorf("the base does not match its configuration, checked as %s: %s", user, refusal)
		}
	}
	// ⛔ THROUGH guestReason, which steps over podman's warning as well as
	// wsl.exe's own line. Finding 15: fedora's `newuidmap: write to uid_map
	// failed` sat behind a shared-mount warning, and this took the FIRST guest
	// line, which was the warning. The last guest line is the engine's reason.
	// ⚠ STDERR FIRST, NOT THE LAST LINE OF BOTH: verify.sh prints its rows on
	// stdout before `podman run`, so a last line across both streams named a row
	// and lost the `boot ID` words staleRunStateRemediation reads. WSL-152.
	detail := guestReason(stderr, stdout)
	return fmt.Errorf("a container did not run as %s (exit %d): %s", user, code, detail)
}

// guestReason is the line worth reporting out of a guest command that writes
// its report on stdout and its failure on stderr.
//
// ⛔ THE ERROR STREAM WINS. guestFailure takes the last guest line across every
// stream it is given, so a stdout row printed before a failure beat the
// engine's error. Here the last guest line of stderr is the reason, and stdout
// is read only when stderr carries no guest line. WSL-152.
func guestReason(stderr, stdout string) string {
	if lines := guestLines(stderr); len(lines) > 0 {
		return lines[len(lines)-1]
	}
	return guestFailure(stdout, stderr)
}

// guestLines are the lines of a guest command's stream that are not blank and not
// wsl.exe's own, which it writes on the same stream beginning `wsl: `.
func guestLines(s string) []string {
	var lines []string
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' }) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "wsl: ") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// guestFailure is the line worth reporting out of a guest command's output.
//
// ⛔ WHAT WAS WRONG, IN TWENTY-FOUR PLACES. A failed guest command was named by
// the FIRST line of its output, and the first line is regularly not the
// program's. wsl.exe prints its own before the guest says anything - a base
// built with automount off produced 93 `wsl: Failed to translate` lines - and
// podman prints a warning before its error: fedora's
// `newuidmap: write to uid_map failed` sat behind a shared-mount warning and
// needed a diagnostic build to read. TODO/PROGRESS.md findings 12, 13 and 15.
//
// ⭐ THE KNOWLEDGE ALREADY EXISTED AND WAS APPLIED ONCE. `WSL-85` wrote
// guestLines to drop wsl.exe's lines and used it at the verifier alone, so
// twenty-three other messages kept naming the wrong line. This is that filter
// with one home and every caller through it.
//
// ⚠ THE LAST GUEST LINE, NOT THE FIRST. A shell that fails prints its context
// first and its reason last, and `engineFailure` in job.go already reached the
// same conclusion for podman by measurement. Where nothing survives the filter
// the raw first line is returned rather than an empty string, because a message
// that says nothing is worse than one naming a wrapper's line.
func guestFailure(streams ...string) string {
	var lines []string
	for _, s := range streams {
		lines = append(lines, guestLines(s)...)
	}
	if len(lines) > 0 {
		return lines[len(lines)-1]
	}
	// Nothing but a wrapper's own lines: say what there was rather than
	// nothing at all.
	for _, s := range streams {
		if t := firstLine(s); t != "" {
			return t
		}
	}
	return "the guest said nothing"
}

// ⛔ THERE WAS A NOISE FILTER HERE AND THE MUTATION HARNESS DELETED IT.
// It skipped `WARN[`, `Trying to pull `, `Copying blob ` and their siblings
// before taking a line. Removing it left all eight cases GREEN, because in
// every capture measured the noise comes BEFORE the error and taking the LAST
// guest line already steps over it. A guard that cannot be made to matter is
// not a guard: TODO/PROGRESS.md finding 59 says so, and says the fix is to
// delete the redundancy rather than to strengthen the case around it.
//
// ⚠ WHAT WOULD BRING IT BACK is a capture where a warning comes LAST, after the
// error. That is plausible - podman prints cleanup warnings - and it has not
// been measured here, so it is not built on.

// lastPrefixed answers the last guest line of s that starts with prefix, without it.
func lastPrefixed(s, prefix string) (string, bool) {
	lines := guestLines(s)
	for i := len(lines) - 1; i >= 0; i-- {
		if rest, ok := strings.CutPrefix(lines[i], prefix); ok {
			return rest, true
		}
	}
	return "", false
}

// parseAutomount reads the drives verify.sh measured from its `automount` row.
//
// ⚠ ONLY A VALUE THE SCRIPT PRINTS IS TAKEN. A row cut short answers empty, and an
// empty answer is never read as off: off is a measurement, and empty is none.
func parseAutomount(out string) string {
	for _, line := range strings.Split(out, "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), "automount ")
		if !ok {
			continue
		}
		switch value = strings.TrimSpace(value); value {
		case AutomountOff, AutomountReadOnly, AutomountReadWrite, AutomountMixed:
			return value
		}
		return ""
	}
	return ""
}

// VerifyImage is what the health check runs: small, in the catalog, and pulled
// only when it is absent, so a warm base costs nothing. The pull is its own
// bounded step, and `gc --images` keeps the image. WSL-174.
const VerifyImage = "docker.io/library/alpine:latest"

// captureAs runs a script as an account inside the base with the runtime
// directory every rootless engine needs already set.
func (b *Base) captureAs(ctx context.Context, user string, script []byte, env map[string]string, timeout time.Duration) (string, string, int, error) {
	full := map[string]string{}
	for k, v := range env {
		full[k] = v
	}
	out := &boundedBuffer{max: 8 << 20}
	errBuf := &boundedBuffer{max: 512 << 10}
	code, err := b.wsl.Exec(ctx, ExecRequest{
		Distro: b.cfg.Base.Name, User: user,
		Script: append(guestRuntimePrologue(), script...),
		Env:    full, Timeout: timeout, Stdout: out, Stderr: errBuf,
	})
	return out.String(), errBuf.String(), code, err
}

// guestRuntimePrologue gives the account the runtime directory rootless podman
// needs, which neither WSL nor runuser sets.
//
// ⚠ Not a copy of `distro run --user-env`'s prologue. That one prepares an
// arbitrary imported distribution; this runs in one this executable provisioned,
// where the account and the paths are known.
func guestRuntimePrologue() []byte {
	return []byte(`_tk_uid=$(id -u)
_tk_run=/tmp/wsl-toolkit-run-$_tk_uid
(umask 077; mkdir "$_tk_run") 2>/dev/null || :
if [ -L "$_tk_run" ] || [ ! -d "$_tk_run" ]; then
    echo "wsl-toolkit: $_tk_run is not a private directory" >&2; exit 2
fi
chmod 700 "$_tk_run" || exit 2
XDG_RUNTIME_DIR=$_tk_run; export XDG_RUNTIME_DIR
TMPDIR=${TMPDIR:-/tmp}; export TMPDIR
unset _tk_uid _tk_run
`)
}

// Remove unregisters the base and deletes its disk, under the base's lock.
func (b *Base) Remove(ctx context.Context) error {
	release, err := b.lock(ctx, "base remove")
	if err != nil {
		return err
	}
	defer release()
	return b.remove(ctx)
}

// remove is Remove for a caller that already holds the base's lock.
func (b *Base) remove(ctx context.Context) error {
	exists, err := b.wsl.Exists(ctx, b.cfg.Base.Name)
	if err != nil {
		return err
	}
	// ⛔ ONLY THE OWNER REMOVES, and nothing below runs for anyone else: no
	// unregister, no deletion, no SSH Host block dropped. ⚠ Not for this process's
	// own import, which it is clearing up. WSL-167.
	if exists && !b.imported {
		if err := b.assertOwner("removed", "base remove --yes"); err != nil {
			return err
		}
	}
	if exists {
		b.log("unregistering " + b.cfg.Base.Name)
		// ⚠ The marker is DEMANDED unless this tool is clearing up a build of
		// its own that never got far enough to write one. `b.imported` is set by
		// the one path that knows that: this process's own import.
		err := b.wsl.Unregister(ctx, b.cfg.Base.Name, !b.imported)
		// ⭐ OR BY A LATER PROCESS, where the build that knew it was killed. The
		// ledger's record and WSL's registration of the disk prove it. WSL-150.
		if errors.Is(err, ErrNoIdentity) {
			rec, ok, why := b.interruptedBuild(ctx)
			if !ok {
				return fmt.Errorf("%w, and %s", err, unmarkedAdvice(b.cfg.Base.Name, why))
			}
			b.log(interruptedBuildNote(b.cfg.Base.Name, rec))
			err = b.wsl.Unregister(ctx, b.cfg.Base.Name, false)
		}
		if err != nil {
			return err
		}
	}
	dir := b.Dir()
	if _, err := os.Stat(dir); err == nil {
		// ⛔ THE DIRECTORY IS NAME-INDEPENDENT AND THE LISTING IS NOT. A base whose
		// configured name is not listed, after a rename or under a shared state
		// directory, had its disk deleted here with no unregister in front of it,
		// whoever's disk it was. WSL's own registrations decide. WSL-177.
		if err := b.assertNoDiskAt(dir); err != nil {
			return err
		}
		// ⚠ ONLY WHERE NOTHING WAS UNREGISTERED. A marked base under the
		// configured name has just proved whose disk this is. WSL-177.
		if !exists {
			if err := b.assertRecordNames(dir); err != nil {
				return err
			}
		}
		// wsl --unregister releases the disk asynchronously, so a removal right
		// afterwards can lose the race against a handle about to close.
		var lastErr error
		for attempt := 1; attempt <= 5; attempt++ {
			if lastErr = RemoveInside(b.home, dir); lastErr == nil {
				b.log("deleted " + dir)
				break
			}
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		if lastErr != nil {
			return fmt.Errorf("the disk is still on disk after five attempts: %w", lastErr)
		}
	}
	// ⛔ Through the one deletion. The record outlives the call that wrote it,
	// which is the line RemoveInside's own comment draws.
	if err := RemoveInside(b.home, b.recordPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// The distribution the build record named is gone, so the record closes.
	// WSL-150.
	b.closeBuildRecord("the distribution was removed")
	// ⭐ A WAY IN TO A DISTRIBUTION THAT IS GONE GOES WITH IT. The Host block and
	// the known host key would otherwise name a distribution that no longer exists,
	// and then the next one built under that name, with a different host key.
	return b.removeAdapterHosts()
}

// assertNoDiskAt refuses to delete dir while WSL has any distribution registered
// with its disk there, and while WSL's registrations cannot be read. WSL-177.
//
// ⛔ AN UNREADABLE REGISTRY IS A REFUSAL, NOT AN EMPTY ONE. Reading it as nothing
// registered would delete the disk the check exists to protect.
func (b *Base) assertNoDiskAt(dir string) error {
	registered, err := b.registrations()
	if err != nil {
		return fmt.Errorf("%s was NOT deleted: WSL's registrations could not be read, so nothing proves no distribution keeps its disk there: %w", dir, err)
	}
	names := make([]string, 0, len(registered))
	for name := range registered {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		// ⚠ FOLDED, so a disk reached through a junction is still found. WSL-167.
		if at := cleanBasePath(registered[name]); at != "" && samePlace(at, dir) {
			return fmt.Errorf("%s was NOT deleted: WSL has %s registered with its disk there, and the configuration names %s. "+
				"If %s is this base under another name, set base.name to it",
				dir, name, b.cfg.Base.Name, name)
		}
	}
	return nil
}

// prefixWriter relays a child's output through the logger and keeps a copy, so
// a caller can assert on what was said rather than on the exit code.
//
// ⛔ IT IS WRITTEN TO FROM TWO GOROUTINES. provision passes the same
// instance as both Stdout and Stderr, and os/exec runs one copier per stream
// when the writer is not an *os.File. `seen` is a boundedBuffer and locks
// itself; `partial` did not, so the two copiers raced on the same slice. The
// symptom would be an interleaved or dropped provisioning line, in the one
// place whose output decides whether the base is usable.
//
// ⚠ `partial` IS BOUNDED TOO. It holds whatever has arrived since the last
// newline, and a child that writes megabytes without one would grow it without
// limit. Past the ceiling the held text is flushed as its own line rather than
// dropped: a long line is still information, and losing it silently is the
// failure this whole type exists to avoid.
type prefixWriter struct {
	mu      sync.Mutex
	prefix  string
	to      func(string)
	seen    boundedBuffer
	partial []byte
}

// maxPartialLine is how much unterminated output is held before it is flushed
// as a line of its own.
const maxPartialLine = 64 << 10

func (w *prefixWriter) logTo(s string) {
	if w.to != nil {
		w.to(s)
	}
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	if w.seen.max == 0 {
		w.seen.max = 4 << 20
	}
	// ⚠ The bounded copy cannot fail the write. Its whole job is to stop
	// early, and telling the child its output could not be written because a
	// diagnostic buffer is full would kill a provisioning run over nothing.
	_, _ = w.seen.Write(p)
	w.partial = append(w.partial, p...)
	var lines []string
	for {
		i := indexByte(w.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(w.partial[:i]), "\r")
		w.partial = w.partial[i+1:]
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(w.partial) > maxPartialLine {
		lines = append(lines, string(w.partial))
		w.partial = w.partial[:0]
	}
	w.mu.Unlock()

	// ⛔ THE LOGGER IS CALLED OUTSIDE THE LOCK. It is a caller-supplied
	// function, and holding a lock across one is how an unrelated callback
	// deadlocks a provisioning run.
	for _, line := range lines {
		w.logTo(w.prefix + line)
	}
	return len(p), nil
}

// Flush emits whatever arrived after the last newline.
//
// ⚠ A child whose final line has no terminator would otherwise have it held
// forever, which is exactly the line a failing step tends to end on.
func (w *prefixWriter) Flush() {
	w.mu.Lock()
	rest := strings.TrimRight(string(w.partial), "\r")
	w.partial = w.partial[:0]
	w.mu.Unlock()
	if rest != "" {
		w.logTo(w.prefix + rest)
	}
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func (b *Base) logWriter() func(string) { return b.log }

// CgroupState is what the guest's cgroup tree can actually do for the account
// the engine runs as, read under `--probe`.
//
// ⛔ THE MECHANISM IS NAMED RATHER THAN INFERRED FROM THE HOST, and that is the
// whole reason this is a struct and not a boolean. Delegation can arrive four
// ways, and this repository may use any of them: systemd's `user@.service`, an
// explicitly handed-over subtree, a rootful engine, or a full virtual machine
// that simply has one. A field that said "WSL, so no" would be wrong the day the
// base changes shape, and it would be wrong silently. WSL-60.
//
// ⚠ EVERY FIELD THAT COULD BE UNKNOWN IS A STRING AND CAN SAY SO. A boolean
// cannot distinguish "no" from "not measured", and the difference is the whole
// finding: `podman stats` reporting `0B` over a container using memory is worse
// than reporting nothing, because a blank gets checked and a number gets used.
type CgroupState struct {
	Version     string `json:"version"`   // v2, v1, none
	Mechanism   string `json:"mechanism"` // systemd, delegated, rootful, none, unknown
	Delegated   bool   `json:"delegated"`
	Controllers string `json:"controllers,omitempty"`
	Self        string `json:"self,omitempty"`
	// Limits is what a container actually got when one was asked for 64 MiB:
	// the byte figure the guest read back, `unknown` where the container could
	// not read it, or `unreadable` where the probe itself could not run.
	Limits string `json:"limits,omitempty"`
	// Enforced and StatsUsable are the two answers a caller acts on. ⭐ They are
	// derived from the measurement above, never from the version number.
	Enforced    string `json:"limits_enforced"` // yes, no, unknown
	StatsUsable string `json:"stats_usable"`    // yes, no, unknown
	// Manager is the engine's own cgroup manager, systemd or cgroupfs, and
	// AccountControllers what the account's user manager was delegated. WSL-136.
	Manager            string `json:"manager,omitempty"`
	AccountControllers string `json:"account_controllers,omitempty"`
	// accountUnused is a user manager the account could make a cgroup in,
	// beside an engine whose manager is not systemd. Only the remediation reads
	// it. WSL-136.
	accountUnused bool
}

// BinfmtState is what the WSL kernel's QEMU handler table holds for this base.
type BinfmtState struct {
	// Handlers counts every `qemu-*` entry in the kernel, whichever distribution
	// registered it.
	Handlers int `json:"handlers"`
	// Own is how many handlers `base ensure` recorded as this base's own, and
	// Missing names the ones the kernel no longer holds. ⚠ Own is 0 and Recorded
	// false on a base no ensure of this version has run on.
	Own      int      `json:"own"`
	Missing  []string `json:"missing,omitempty"`
	Recorded bool     `json:"recorded"`
	// Watch is the watcher unit's state: active, or what systemd answered, or
	// none where there is no unit.
	Watch string `json:"watch,omitempty"`
	// Ready is true when none of this base's own handlers is missing.
	// ⛔ It was `Handlers > 0`, so it followed another distribution's lifetime: a
	// base with every handler of its own gone read ready while the Podman machine
	// ran. WSL-105.
	Ready bool `json:"ready"`
}

func parseBinfmt(out string) *BinfmtState {
	var st *BinfmtState
	rows := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		key, value, _ := strings.Cut(line, " ")
		switch key {
		case "binfmt-handlers", "binfmt-own", "binfmt-watch":
			rows[key] = strings.TrimSpace(value)
		case "binfmt-missing":
			rows[key] = strings.TrimSpace(value)
			rows["missing-seen"] = "yes"
		}
	}
	var count int
	if _, err := fmt.Sscanf(rows["binfmt-handlers"], "%d", &count); err != nil {
		return nil
	}
	st = &BinfmtState{Handlers: count, Watch: rows["binfmt-watch"]}
	if own, err := strconv.Atoi(rows["binfmt-own"]); err == nil && own > 0 && rows["missing-seen"] == "yes" {
		st.Own, st.Recorded = own, true
		st.Missing = strings.Fields(rows["binfmt-missing"])
		st.Ready = len(st.Missing) == 0
		return st
	}
	// ⚠ NO RECORD, so nothing here says which handlers are this base's own, and
	// readiness cannot be more than whether the kernel holds any. `base ensure`
	// records the set, and `base status` names it as the step that is missing.
	st.Ready = count > 0
	return st
}

// binfmtStepRemediation is the finding an ensure whose QEMU step failed earns:
// advisory, as native work runs without it. WSL-181.
func binfmtStepRemediation(err error) Remediation {
	return Remediation{
		ID:   "binfmt-step-failed",
		What: "the QEMU step of this ensure did not complete: " + err.Error(),
		Costs: "a container for another architecture fails `Exec format error` until its handler is registered. " +
			"A job that asks for a foreign platform registers its own",
		Command:    ToolCommand("base ensure"),
		Repairable: true,
		Advisory:   true,
	}
}

// binfmtRemediation is the finding a base whose own handlers are gone earns.
func binfmtRemediation(bf *BinfmtState) (Remediation, bool) {
	switch {
	case bf == nil:
		return Remediation{}, false
	case bf.Recorded && len(bf.Missing) > 0:
		return Remediation{
			ID:   "binfmt-handlers-gone",
			What: fmt.Sprintf("%d of this base's %d QEMU handlers are gone from the kernel: %s", len(bf.Missing), bf.Own, strings.Join(bf.Missing, " ")),
			Costs: "a container for that architecture fails `Exec format error` in base exec, base shell, a herdr pane or an agent. " +
				"Another distribution removed them; the Podman machine does when it stops",
			Command:    ToolCommand("base ensure"),
			Repairable: true,
			Advisory:   true,
		}, true
	case !bf.Recorded:
		return Remediation{
			ID:         "binfmt-handlers-unrecorded",
			What:       "no ensure of this version has recorded which QEMU handlers are this base's own, so readiness counts every distribution's",
			Costs:      "a handler of this base's can be gone while another distribution's keeps it reading ready",
			Command:    ToolCommand("base ensure"),
			Repairable: true,
			Advisory:   true,
		}, true
	}
	return Remediation{}, false
}

// Remediation is one condition this tool found, what leaving it costs, and the
// exact command that takes it.
//
// ⭐ IT CARRIES THE COMMAND, NOT A DESCRIPTION OF ONE. The reader here is
// usually an agent, and an agent that is told "the run state is stale" has to
// guess what to do next; one that is handed `wsl-toolkit base ensure --repair`
// does not. WSL-61.
//
// ⛔ `Repairable` false is a real and common answer. A condition this tool
// cannot fix is still reported, with the command that can, because the failure
// this whole shape exists to prevent is a caller being told something is wrong
// and not what to run.
type Remediation struct {
	ID         string `json:"id"`
	What       string `json:"what"`
	Costs      string `json:"costs"`
	Command    string `json:"command"`
	Repairable bool   `json:"repairable"`
	// Advisory says the base runs a native job without it: space, tidiness,
	// or a foreign architecture. ⛔ `ready` NEVER NAMES ONE AS THE WAY FORWARD.
	// Every remediation 7.0 added is repairable and was listed before stale run
	// state, so `ready` sent a base the engine refused to `base reclaim`. A
	// contract review found it.
	Advisory bool `json:"advisory,omitempty"`
}

// parseCapabilities reads the rows verify.sh prints after its marker.
//
// ⚠ EVERY ROW IS OPTIONAL. An older guest, or one whose probe could not run,
// prints none of them, and the answer is then a nil CgroupState rather than a
// struct full of zero values that reads like a measurement.
func parseCapabilities(out string) *CgroupState {
	rows := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		for _, key := range []string{
			"cgroup-version", "cgroup-controllers", "cgroup-self",
			"cgroup-delegated", "cgroup-under", "cgroup-limit", "engine-rootless",
			"cgroup-manager", "cgroup-account-controllers", "cgroup-account-delegated",
		} {
			if strings.HasPrefix(line, key+" ") {
				rows[key] = strings.TrimSpace(strings.TrimPrefix(line, key+" "))
			}
		}
	}
	if len(rows) == 0 {
		return nil
	}
	cg := &CgroupState{
		Version:     valueOr(rows["cgroup-version"], "unknown"),
		Controllers: strings.TrimSpace(strings.TrimSuffix(rows["cgroup-controllers"], "-")),
		Self:        rows["cgroup-self"],
		Limits:      valueOr(rows["cgroup-limit"], "unknown"),
		Delegated:   rows["cgroup-delegated"] == "yes",
		// WSL-136.
		Manager:            strings.TrimSpace(strings.TrimSuffix(rows["cgroup-manager"], "unknown")),
		AccountControllers: strings.TrimSpace(strings.TrimSuffix(rows["cgroup-account-controllers"], "-")),
	}
	// ⛔ THE ACCOUNT'S USER MANAGER COUNTS ONLY WHERE THE ENGINE USES IT. A
	// writable user@UID.service beside podman's cgroupfs manager gives no
	// container a cgroup, and reading it as delegation said `delegated` and
	// `stats usable` over a limit the container never got. WSL-136.
	cg.accountUnused = rows["cgroup-account-delegated"] == "yes" && cg.Manager != "systemd"
	viaAccount := rows["cgroup-account-delegated"] == "yes" && cg.Manager == "systemd"
	if viaAccount {
		cg.Delegated = true
	}
	rootless := rows["engine-rootless"]
	switch {
	case rootless == "false":
		cg.Mechanism = "rootful"
	case cg.Delegated && rows["cgroup-under"] == "systemd":
		cg.Mechanism = "systemd"
	case viaAccount:
		cg.Mechanism = "systemd"
	case cg.Delegated:
		cg.Mechanism = "delegated"
	case rows["cgroup-delegated"] == "no":
		cg.Mechanism = "none"
	default:
		cg.Mechanism = "unknown"
	}
	// ⭐ ENFORCEMENT IS READ FROM THE CONTAINER, not concluded from the
	// mechanism. A limit that is accepted and not applied is the defect WSL-60
	// exists for, and the only thing that can see it is a container that asked
	// for one and reported what it got.
	switch limit := cg.Limits; {
	// ⚠ NOBODY COULD LOOK. Not the same as a limit that was ignored, and
	// reporting it as one would be a claim about something unmeasured.
	case limit == "" || limit == "unknown" || limit == "unreadable" || limit == "nocgroupfs":
		cg.Enforced = "unknown"
	// ⛔ THE CONTAINER ASKED FOR 64 MiB AND FOUND NO LIMIT FILE IN ITS OWN
	// CGROUP, which is what a container sitting in the ROOT cgroup sees: root
	// has no memory.max by definition. That is the measurement, and `missing` is
	// the answer this repository's own base gives today. WSL-60.
	case limit == "max" || limit == "missing":
		cg.Enforced = "no"
	default:
		if _, err := strconv.ParseInt(limit, 10, 64); err == nil {
			cg.Enforced = "yes"
		} else {
			cg.Enforced = "unknown"
		}
	}
	// ⚠ STATS AND LIMITS STAND OR FALL TOGETHER, and that is a measurement
	// rather than an assumption: both are read out of the container's own
	// cgroup, so a tree with no cgroup per container has neither.
	switch {
	case cg.Mechanism == "unknown":
		cg.StatsUsable = "unknown"
	case cg.Delegated || cg.Mechanism == "rootful":
		cg.StatsUsable = "yes"
	default:
		cg.StatsUsable = "no"
	}
	return cg
}

func valueOr(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return fallback
	}
	return s
}

// cgroupRemediation is the finding a base without delegation earns.
//
// ⛔ IT IS NOT REPAIRABLE HERE AND IT SAYS SO. The delegation is systemd's
// user@.service, which `base ensure` hands cpuset and io to as well. A base
// without systemd has nothing to delegate it. A base with systemd and no D-Bus
// has no user manager at all, measured on a Debian 13 base 7.0.0 built, and
// this tool does not install one. So the finding names what is missing rather
// than a command that would not take it. WSL-136.
func cgroupRemediation(cg *CgroupState, systemd bool) (Remediation, bool) {
	if cg == nil || cg.Enforced != "no" {
		return Remediation{}, false
	}
	r := Remediation{
		ID: "cgroup-delegation",
		What: "this base has no cgroup delegation for " + "the engine's account" +
			", so podman creates no cgroup per container",
		Costs: "a memory or cpu limit is ACCEPTED AND NOT ENFORCED, `podman stats` reports 0B, " +
			"and an out-of-memory kill cannot be told apart from any other exit 137. " +
			"A base with systemd delegates them once base.systemd is true and base ensure has run",
		Command:    ToolCommand("base status --probe --json") + "  # the cgroup block carries the mechanism",
		Repairable: false,
	}
	if systemd {
		r.What = "this base runs systemd and its account's user manager holds no delegated cgroup, so podman creates no cgroup per container"
		r.Costs = "a memory or cpu limit is ACCEPTED AND NOT ENFORCED, and `podman stats` reports 0B. " +
			"The user manager runs only where the base has D-Bus, and base ensure delegates to it once it does"
	}
	// ⛔ A DELEGATED USER MANAGER THE ENGINE DOES NOT USE IS SAID AS THAT, not
	// as one that holds nothing. WSL-136.
	if systemd && cg.accountUnused {
		r.What = "this base's account holds a delegated cgroup under its user manager, and its podman uses the " +
			valueOr(cg.Manager, "unknown") + " manager, so podman creates no cgroup per container"
		r.Costs = "a memory or cpu limit is ACCEPTED AND NOT ENFORCED, and `podman stats` reports 0B. " +
			"base ensure gives podman the systemd manager once the account's user manager has a session bus"
	}
	return r, true
}

// StaleRunStateRemediation is exported so the command layer can answer with the
// right command when it holds an error and not a state. ⛔ `ready` needs it:
// answering an ensure failure with `base ensure` sends a caller round the loop
// that just refused them.
func StaleRunStateRemediation(msg string) (Remediation, bool) { return staleRunStateRemediation(msg) }

// staleRunStateRemediation classifies a failed health probe.
//
// ⭐ IT MATCHES WHAT THE ENGINE SAID, not a file this tool went looking for.
// podman names the condition and names the directories, so the classification is
// a read of its own message rather than a guess about its state.
func staleRunStateRemediation(msg string) (Remediation, bool) {
	if !strings.Contains(strings.ToLower(msg), "boot id") {
		return Remediation{}, false
	}
	r := Remediation{
		ID:         "stale-run-state",
		What:       "the engine's cached boot id is not this boot's, so every container is refused before it starts",
		Costs:      "nothing runs. Re-provisioning does not clear it, because the state is the engine's and not the distribution's",
		Command:    ToolCommand("base ensure --repair"),
		Repairable: true,
	}
	// ⛔ THE CONDITION IS HOST-WIDE AND THIS USED TO ANSWER FOR ONE BASE.
	// `wsl --shutdown` invalidates the cached boot id in EVERY distribution
	// that holds an engine. On 2026-09-17 a session met it on
	// `wsl-toolkit-base`, repaired that one, recorded exactly that, and left
	// the default `wsl-toolkit` base - which every `run` and `matrix` uses -
	// carrying the same damage into the next session, where the first
	// container check exited 2 before anything could run. Finding 67.
	if note := siblingRepairNote(SelectedInstance.Name, managedInstances()); note != "" {
		r.Costs += ". " + note
	}
	return r, true
}

// siblingRepairNote names the other instances this tool manages, because the
// cause is host-wide and repairing one says nothing about the rest.
//
// ⛔ IT IS A FUNCTION WITH NO FILESYSTEM IN IT, so every branch has a case.
// Finding 42 twice over is why anything shaped like a rule gets separated from
// the thing that reads the disk.
func siblingRepairNote(current string, managed []string) string {
	var others []string
	for _, name := range managed {
		if name != current && name != "" {
			others = append(others, name)
		}
	}
	if len(others) == 0 {
		return ""
	}
	sort.Strings(others)
	var cmds []string
	for _, name := range others {
		cmds = append(cmds, "wsl-toolkit --instance "+name+" base ensure --repair")
	}
	// ⚠ "LIKELY", NOT "IS". This says what the cause implies, and it does not
	// claim to have probed the others: a note that asserted damage it never
	// measured would be the fabricated-number row.
	return "⚠ THIS IS A HOST-WIDE EVENT and `wsl --shutdown` invalidates it in every " +
		"distribution holding an engine, so " + strings.Join(others, ", ") +
		" managed here are LIKELY to need the same repair, unprobed: " + strings.Join(cmds, "; ")
}

// managedInstances lists the instance names this state directory holds.
//
// ⚠ AN UNREADABLE STATE DIRECTORY ANSWERS NONE rather than failing. This feeds
// a note on a remediation, and a diagnosis that could not be produced must not
// take down the diagnosis it was decorating.
func managedInstances() []string {
	home, err := Home()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(home, "instances"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// repairRunState clears the engine run state this boot invalidated, by running
// repair.sh inside the distribution as the engine's own account.
//
// ⛔ THE PATHS ARE THE GUEST'S, NOT THIS PROCESS'S. repair.sh resolves them from
// `$XDG_RUNTIME_DIR`, which is what podman itself resolves, so an account
// configured differently is cleared correctly rather than confidently wrongly.
// Nothing here takes a path from a caller, so there is no path to contain, and
// TODO/RULES.md section 3 is the rule that says where that line is drawn.
func (b *Base) repairRunState(ctx context.Context) (string, error) {
	out, stderr, code, err := b.captureAs(ctx, b.cfg.Base.User, repairScript, nil, 2*time.Minute)
	return out, repairRunStateVerdict(out, stderr, code, err)
}

// repairRunStateVerdict reads what repair.sh printed for `base ensure --repair`.
//
// ⭐ A FUNCTION WITH NO PROCESS IN IT, so every answer has a case on any host.
func repairRunStateVerdict(out, stderr string, code int, err error) error {
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("repair exited %d: %s", code, guestFailure(stderr, out))
	}
	// ⛔ THE EFFECT IS ASSERTED, not inferred from the exit code. repair.sh
	// reads every removal back and refuses to exit 0 over one that is still
	// there, and this refuses to report a repair the script did not announce.
	if strings.Contains(out, "repaired run-state") {
		return nil
	}
	// ⭐ ANOTHER CALLER CLEARED IT FIRST. repair.sh read the record again under
	// the run-state lock and found it naming this boot, so it removed nothing
	// under the engine that caller started. The probe after this answers whether
	// the base works, as it does after a clear. WSL-137.
	if strings.Contains(out, "run-state current: another caller cleared it first") {
		return nil
	}
	return fmt.Errorf("repair exited 0 without saying what it did: %s", guestFailure(out, stderr))
}

// StaleRefusalError is the error Ensure returns when it will not repair.
//
// ⛔ IT WRAPS THE ENGINE'S OWN MESSAGE AND THAT IS THE POINT OF THE FUNCTION. A
// caller downstream classifies this condition from the text: `ready` reads it to
// decide whether to answer with `base ensure` or with `base ensure --repair`. A
// refusal that states its own opinion and drops what podman said cannot be
// recognised as the condition it is refusing over, so `ready` answered with the
// command that had just refused. WSL-61.
//
// ⚠ It is a function rather than an inline Errorf so a case can send its output
// back through the classifier and prove the round trip, which a hand-written
// fixture cannot: the fixture does not move when the producer does.
func StaleRefusalError(r Remediation, cause error) error {
	return fmt.Errorf("the engine's run state is stale and --repair was not given. Run: %s (%w)", r.Command, cause)
}
