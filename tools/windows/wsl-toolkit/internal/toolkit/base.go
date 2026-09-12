package toolkit

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed provision.sh
var provisionScript []byte

//go:embed verify.sh
var verifyScript []byte

//go:embed repair.sh
var repairScript []byte

// BaseSpaceFloor is what the volume must have free before an import starts.
//
// ⚠ Far above wsl-toolkit.ps1's 256 MiB floor, because this distribution will
// hold an engine and a dozen images. Running out midway leaves a partial disk
// and a registered distribution that does not work.
const BaseSpaceFloor = int64(6) << 30

// BinfmtImage installs QEMU interpreters in the WSL kernel.
const BinfmtImage = "docker.io/tonistiigi/binfmt:qemu-v10.2.3-68"

// BaseState is what `base status` answers.
type BaseState struct {
	Name       string    `json:"name"`
	Image      string    `json:"image"`
	BuiltFrom  string    `json:"built_from,omitempty"`
	User       string    `json:"user"`
	Registered bool      `json:"registered"`
	Running    bool      `json:"running"`
	Healthy    bool      `json:"healthy"`
	Engine     string    `json:"engine,omitempty"`
	DiskPath   string    `json:"disk_path,omitempty"`
	DiskBytes  int64     `json:"disk_bytes,omitempty"`
	DiskKnown  bool      `json:"disk_known"`
	Created    time.Time `json:"created,omitempty"`
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
	// Remediations are the conditions this tool found, each with what leaving it
	// costs and the exact command that takes it. ⭐ A caller here is usually an
	// agent, and the command is the field it acts on.
	Remediations []Remediation `json:"remediations,omitempty"`
}

// Base is the owned distribution's lifecycle.
type Base struct {
	cfg  Config
	home string
	wsl  *Wsl
	log  func(string)
	// unmarked says this process built the distribution now registered and did
	// not get as far as stamping it, so Remove may unregister it without the
	// guest's proof. ⛔ It is set by the build path and by nothing else: a
	// flag a caller could set would be a way past the guard.
	unmarked bool
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
	st := BaseState{Name: b.cfg.Base.Name, Image: b.cfg.Base.Image, User: b.cfg.Base.User}
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
	if size, ok := FileSize(st.DiskPath); ok {
		st.DiskBytes, st.DiskKnown = size, true
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
	}
	if !st.Registered {
		st.Problems = append(st.Problems, "not registered. Create it with: wsl-toolkit base ensure")
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
	} else if errors.Is(err, ErrNoIdentity) {
		st.Problems = append(st.Problems, fmt.Sprintf(
			"%s carries no identity marker, so this tool cannot confirm it built it. `base ensure` adopts one it has a record for", b.cfg.Base.Name))
	} else {
		st.Problems = append(st.Problems, err.Error())
	}
	engine, caps, binfmt, verifyErr := b.verify(ctx)
	st.Engine = engine
	st.Cgroup = caps
	st.Binfmt = binfmt
	if verifyErr != nil {
		st.Problems = append(st.Problems, verifyErr.Error())
		// ⭐ A FAILURE IS CLASSIFIED HERE AND NOT ONLY IN ensure, because
		// `base status --probe` is what an agent runs to find out what is wrong.
		// A report that names the condition and not the command is the half of
		// this that was already there.
		if r, ok := staleRunStateRemediation(verifyErr.Error()); ok {
			st.Remediations = append(st.Remediations, r)
		}
		return st, nil
	}
	st.Healthy = true
	// ⛔ A CAPABILITY FINDING IS NOT A HEALTH FAILURE. The base runs containers;
	// what it cannot do is account for them, and reporting that as unusable would
	// refuse work over a limitation most jobs never reach. WSL-60.
	if r, ok := cgroupRemediation(caps); ok {
		st.Remediations = append(st.Remediations, r)
	}
	return st, nil
}

// withoutRecordDrift removes the problem the host record raised, for the case
// where the guest has since answered the same question with authority.
func withoutRecordDrift(problems []string) []string {
	out := problems[:0]
	for _, p := range problems {
		if strings.HasPrefix(p, recordDriftPrefix) {
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

func (b *Base) writeRecord() error {
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
// ⭐ The recovery path as well as the create path. The base exists to be wrecked:
// a registered-but-unusable distribution is re-provisioned in place, and only
// one that will not provision is removed and rebuilt.
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
	st, err := b.Status(ctx, false)
	if err != nil {
		return st, err
	}
	if st.Registered && force {
		b.log("rebuilding from nothing, as asked")
		if err := b.Remove(ctx); err != nil {
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
		if engine, _, _, err := b.verify(ctx); err == nil {
			b.log("the engine answers: " + engine)
			return b.Status(ctx, true)
		} else {
			b.log("it cannot: " + err.Error())
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
				if engine, _, _, err := b.verify(ctx); err == nil {
					b.log("the engine answers: " + engine)
					return b.Status(ctx, true)
				} else {
					b.log("it still cannot after the repair: " + err.Error())
				}
			}
			b.log("re-provisioning in place")
		}
		if err := b.provision(ctx); err != nil {
			b.log("re-provisioning failed: " + err.Error())
			b.log("removing and rebuilding")
			if err := b.Remove(ctx); err != nil {
				return st, err
			}
		} else {
			if engine, _, _, err := b.verify(ctx); err != nil {
				return st, fmt.Errorf("re-provisioned and it still cannot run a container: %w", err)
			} else {
				b.log("the engine answers: " + engine)
			}
			return b.Status(ctx, true)
		}
	}
	// ⛔ FROM HERE THIS PROCESS OWNS THE DISTRIBUTION IT IS BUILDING, marker or
	// not, so a build that dies before stamping can still be cleared up.
	b.unmarked = true
	if err := b.create(ctx); err != nil {
		return st, err
	}
	if err := b.provision(ctx); err != nil {
		return st, err
	}
	// ⭐ STAMPED BEFORE IT IS VERIFIED. The marker says what this tool BUILT,
	// which is a fact by the time provisioning has finished; whether it can run
	// a container is a different question and the answer to it is not identity.
	if err := b.wsl.WriteIdentity(ctx, b.cfg.Base.Name, Identity{
		Image: b.cfg.Base.Image, User: b.cfg.Base.User, Tool: toolVersion(),
	}); err != nil {
		return st, err
	}
	b.unmarked = false
	engine, _, _, err := b.verify(ctx)
	if err != nil {
		return st, fmt.Errorf("built and it cannot run a container: %w", err)
	}
	b.log("the engine answers: " + engine)
	if err := b.writeRecord(); err != nil {
		return st, err
	}
	return b.Status(ctx, true)
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
		if id.Image != b.cfg.Base.Image {
			// ⛔ REPORTED AND LEFT VISIBLE. It does not rebuild: a rebuild
			// destroys the thing the operator needs to look at, and `recreate`
			// already exists for the case where they want one.
			b.log(fmt.Sprintf("this distribution was built from %s and the configuration says %s",
				id.Image, b.cfg.Base.Image))
			b.log("run `wsl-toolkit base recreate` to rebuild it, or change the configuration back")
		}
		return nil

	case errors.Is(err, ErrNoIdentity):
		// The upgrade path, and it is part of this entry rather than a follow-up:
		// a base built before markers existed has none.
		rec, recErr := b.readRecord()
		if recErr != nil {
			return fmt.Errorf("%s is registered, carries no wsl-toolkit identity marker and has no record here either, "+
				"so this tool cannot tell whether it built it. Remove it yourself, or run: wsl-toolkit base recreate",
				b.cfg.Base.Name)
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
func (b *Base) writeRecordFrom(id Identity) error {
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
	if ScriptVersion == nil {
		return ""
	}
	v, err := ScriptVersion()
	if err != nil {
		return ""
	}
	return v
}

func (b *Base) create(ctx context.Context) error {
	// ⭐ THE FIRST THING THAT WRITES, so this is where the state directory is
	// brought into existence. Everything above it reads.
	if _, err := EnsureHome(); err != nil {
		return err
	}
	dir := b.Dir()
	if _, err := os.Stat(dir); err == nil {
		// ⛔ Never import over a leftover directory: wsl --import into one that
		// already holds a disk is how two distributions share one file.
		if err := RemoveInside(b.home, dir); err != nil {
			return fmt.Errorf("%s is left over from an earlier build and could not be removed: %w", dir, err)
		}
		b.log("removed a leftover " + dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	engine, err := FindEngine(ctx)
	if err != nil {
		return fmt.Errorf("the base is built from an OCI image, so a host engine is required: %w", err)
	}
	b.log(fmt.Sprintf("host engine: %s, platform %s", engine.Name, engine.Platform()))

	tarPath := filepath.Join(b.home, "base-rootfs.tar")
	defer func() {
		if err := os.Remove(tarPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			b.log("the rootfs archive is still on disk: " + tarPath)
		}
	}()
	if err := engine.ExportRootfs(ctx, b.cfg.Base.Image, tarPath, b.log); err != nil {
		return err
	}
	if err := b.assertSpace(tarPath); err != nil {
		return err
	}
	b.log("importing as WSL2 distribution " + b.cfg.Base.Name)
	if err := b.wsl.Import(ctx, b.cfg.Base.Name, dir, tarPath); err != nil {
		return err
	}
	return nil
}

// assertSpace refuses an import the volume cannot hold, BEFORE anything is
// registered.
func (b *Base) assertSpace(tarPath string) error {
	size, ok := FileSize(tarPath)
	if !ok {
		return fmt.Errorf("cannot measure %s", tarPath)
	}
	need := BaseSpaceFloor + 2*size
	free, ok, err := FreeSpace(b.Dir())
	if err != nil {
		return err
	}
	if !ok {
		// ⚠ "could not measure" is a third answer, and treating it as either of
		// the other two is a lie or a needless refusal.
		b.log("free space on this volume could not be read; importing anyway")
		return nil
	}
	b.log(fmt.Sprintf("space: %s wanted, %s free", HumanBytes(need), HumanBytes(free)))
	if free < need {
		return fmt.Errorf("NOT ENOUGH DISK SPACE for %s. About %s is wanted and %s is free on the volume holding it. "+
			"Nothing has been imported and nothing is registered", b.Dir(), HumanBytes(need), HumanBytes(free))
	}
	return nil
}

func (b *Base) provision(ctx context.Context) error {
	b.log("provisioning: a rootless engine and the " + b.cfg.Base.User + " account")
	automount, err := NormalizeAutomount(b.cfg.Base.Automount)
	if err != nil {
		return err
	}
	out := &prefixWriter{prefix: "", to: b.logWriter()}
	code, err := b.wsl.Exec(ctx, ExecRequest{
		Distro: b.cfg.Base.Name,
		User:   "root",
		Script: provisionScript,
		Env: map[string]string{
			"TK_USER":         b.cfg.Base.User,
			"TK_UID":          "1000",
			"TK_BINFMT_IMAGE": BinfmtImage,
			"TK_AUTOMOUNT":    automount,
		},
		Timeout: 30 * time.Minute,
		Stdout:  out,
		Stderr:  out,
	})
	out.Flush()
	if err != nil || code != 0 {
		return fmt.Errorf("provisioning exited %d: %w", code, err)
	}
	if !strings.Contains(out.seen.String(), "provision-complete") {
		// ⛔ The script prints that line last, so its absence over a zero exit is
		// a step that exited 0 having done nothing.
		return errors.New("provisioning exited 0 without reaching its last line")
	}
	// WSL reads /etc/wsl.conf at start, so without this restart the settings
	// just written appear to have been applied and are not.
	b.log("restarting so WSL re-reads /etc/wsl.conf")
	if err := b.wsl.Terminate(ctx, b.cfg.Base.Name); err != nil {
		return err
	}
	return nil
}

// EnsurePlatform restores the QEMU handler for one non-native Linux platform.
// WSL removes kernel registrations when its utility virtual machine stops.
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
		return fmt.Errorf("could not read the base architecture (exit %d): %s", code, firstLine(stderr+out))
	}
	if nativePlatformArch(strings.TrimSpace(firstLine(out))) == target {
		return nil
	}
	script := []byte(`set -eu
handler=/proc/sys/fs/binfmt_misc/$TK_HANDLER
if [ -e "$handler" ]; then
  printf 'binfmt-ready %s\n' "$TK_HANDLER"
  exit 0
fi
podman run --rm --privileged --pull=missing "$TK_BINFMT_IMAGE" --install "$TK_ARCH"
[ -e "$handler" ] || { printf 'handler %s was not registered\n' "$TK_HANDLER" >&2; exit 3; }
printf 'binfmt-ready %s\n' "$TK_HANDLER"
`)
	out, stderr, code, runErr = b.captureAs(ctx, "root", script, map[string]string{
		"TK_ARCH": target, "TK_HANDLER": handler, "TK_BINFMT_IMAGE": BinfmtImage,
	}, 20*time.Minute)
	if runErr != nil || code != 0 {
		return fmt.Errorf("could not prepare %s (exit %d): %s", normalized, code, firstLine(stderr+out))
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

// verify runs a real container as the unprivileged account and returns the
// engine's own version line.
// verify runs a container as the unprivileged account and reads what came back.
//
// ⭐ IT RETURNS THREE THINGS AND THE THIRD IS NOT A HEALTH VERDICT. The engine
// string and the error say whether this base works; the CgroupState says what it
// can account for while it works, and a base with no delegation is healthy and
// limited rather than broken. WSL-60.
func (b *Base) verify(ctx context.Context) (string, *CgroupState, *BinfmtState, error) {
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
	out, stderr, code, err := b.captureAs(ctx, b.cfg.Base.User, verifyScript, map[string]string{
		"TK_IMAGE": VerifyImage,
		"TK_M1":    m1,
		"TK_M2":    m2,
	}, 20*time.Minute)
	if err != nil || code != 0 {
		return "", nil, nil, fmt.Errorf("a container did not run as %s (exit %d): %s", b.cfg.Base.User, code, firstLine(stderr+out))
	}
	// ⛔ Compared with whitespace removed: a tty wraps a long line, so a marker
	// that arrived correctly can fail an exact match.
	flat := strings.Join(strings.Fields(out), "")
	if !strings.Contains(flat, m1+m2) {
		return "", nil, nil, fmt.Errorf("the container ran and did not return the marker: %s", firstLine(out+stderr))
	}
	engine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "engine ") {
			engine = strings.TrimSpace(strings.TrimPrefix(line, "engine "))
		}
	}
	// ⚠ `engine ` MATCHES BEFORE `engine-rootless ` DOES NOT, and it does not:
	// the prefix compared carries a trailing space and that row's key does not.
	return engine, parseCapabilities(out), parseBinfmt(out), nil
}

// VerifyImage is what the health check runs: small, in the catalog, and pulled
// with --pull=missing so a warm base costs nothing.
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
// ⚠ Not a copy of wsl-toolkit.ps1's -UserEnv prologue. That one prepares an
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

// Remove unregisters the base and deletes its disk.
func (b *Base) Remove(ctx context.Context) error {
	exists, err := b.wsl.Exists(ctx, b.cfg.Base.Name)
	if err != nil {
		return err
	}
	if exists {
		b.log("unregistering " + b.cfg.Base.Name)
		// ⚠ The marker is DEMANDED unless this tool is clearing up a build of
		// its own that never got far enough to write one. `b.unmarked` is set by
		// the one path that knows that: a provisioning run that failed.
		if err := b.wsl.Unregister(ctx, b.cfg.Base.Name, !b.unmarked); err != nil {
			return err
		}
	}
	dir := b.Dir()
	if _, err := os.Stat(dir); err == nil {
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
}

// BinfmtState is the QEMU handler count in the current WSL kernel.
type BinfmtState struct {
	Handlers int  `json:"handlers"`
	Ready    bool `json:"ready"`
}

func parseBinfmt(out string) *BinfmtState {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "binfmt-handlers ") {
			continue
		}
		var count int
		if _, err := fmt.Sscanf(strings.TrimPrefix(line, "binfmt-handlers "), "%d", &count); err != nil {
			return nil
		}
		return &BinfmtState{Handlers: count, Ready: count > 0}
	}
	return nil
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
	}
	rootless := rows["engine-rootless"]
	switch {
	case rootless == "false":
		cg.Mechanism = "rootful"
	case cg.Delegated && rows["cgroup-under"] == "systemd":
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
// ⛔ IT IS NOT REPAIRABLE AND IT SAYS SO. Handing the account a cgroup subtree
// means a privileged write into a root-owned tree, and the ruling on 2026-09-10
// was to report first and decide that separately. What this returns is the
// finding and what it costs; what it deliberately does not return is a command
// that would take it.
func cgroupRemediation(cg *CgroupState) (Remediation, bool) {
	if cg == nil || cg.Enforced != "no" {
		return Remediation{}, false
	}
	return Remediation{
		ID: "cgroup-delegation",
		What: "this base has no cgroup delegation for " + "the engine's account" +
			", so podman creates no cgroup per container",
		Costs: "a memory or cpu limit is ACCEPTED AND NOT ENFORCED, `podman stats` reports 0B, " +
			"and an out-of-memory kill cannot be told apart from any other exit 137",
		Command:    "wsl-toolkit base status --probe --json  # the cgroup block carries the mechanism",
		Repairable: false,
	}, true
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
	return Remediation{
		ID:         "stale-run-state",
		What:       "the engine's cached boot id is not this boot's, so every container is refused before it starts",
		Costs:      "nothing runs. Re-provisioning does not clear it, because the state is the engine's and not the distribution's",
		Command:    "wsl-toolkit base ensure --repair",
		Repairable: true,
	}, true
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
	if err != nil {
		return out, err
	}
	if code != 0 {
		return out, fmt.Errorf("repair exited %d: %s", code, firstLine(stderr+out))
	}
	// ⛔ THE EFFECT IS ASSERTED, not inferred from the exit code. repair.sh
	// reads every removal back and refuses to exit 0 over one that is still
	// there, and this refuses to report a repair the script did not announce.
	if !strings.Contains(out, "repaired run-state") {
		return out, fmt.Errorf("repair exited 0 without saying what it did: %s", firstLine(out+stderr))
	}
	return out, nil
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
