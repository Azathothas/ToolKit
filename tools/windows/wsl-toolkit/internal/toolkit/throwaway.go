// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// ⭐ A THROWAWAY DISTRIBUTION is a whole WSL distribution made from an image or
// a rootfs archive, used, and removed. It is what a job needs when the thing
// under test is the distribution itself: its init, its /etc/wsl.conf, what a
// login shell sees. The base and its containers answer every other question.
//
// ⛔ OWNERSHIP IS WHERE THE DISK LIVES, read from WSL's own registration. The
// name prefix alone proves nothing: any tool can register `eph-anything`, and a
// purge that trusted the prefix would unregister a distribution some other run
// made under some other state directory. A distribution is this tool's when
// its registered disk directory is inside `<home>/distros`, which only this
// tool writes to, and every destructive call asks that question first.

// ThrowawayPrefix is the name every throwaway distribution carries.
const ThrowawayPrefix = "eph-"

// ThrowawaySpaceFloor is what an import needs free beyond twice its archive.
//
// ⚠ Measured for the predecessor of this command: an 8 MiB rootfs cost 76 MiB
// of disk and a 77 MiB one cost 172 MiB, so the floor dominates rather than a
// multiple. The floor sits above every measurement rather than fitted to them.
const ThrowawaySpaceFloor = int64(256) << 20

// The schemas this file writes. ⛔ Every stored or exchanged shape carries one.
const (
	ThrowawayListSchema     = "wsl-toolkit-distros/1"
	ThrowawayNewSchema      = "wsl-toolkit-distro-new/1"
	ThrowawayRemoveSchema   = "wsl-toolkit-distro-remove/1"
	ThrowawayPurgeSchema    = "wsl-toolkit-distro-purge/1"
	ThrowawaySnapshotSchema = "wsl-toolkit-distro-snapshot/1"
	ThrowawayOriginSchema   = "wsl-toolkit-distro-origin/1"
	throwawayCreatingSchema = "wsl-toolkit-distro-creating/1"
)

// throwawayCreatingTTL is how long a creation marker protects what it names.
//
// ⚠ A MARKER OUTLIVES A KILLED RUN, so it cannot protect forever: past this age
// the creation it describes is assumed dead and purge collects what it left.
const throwawayCreatingTTL = 2 * time.Hour

// snapshotExportTimeout bounds one `wsl --export`. ⚠ It is also how long purge
// treats a partial export as one that may still be written, because no export
// outlives it.
const snapshotExportTimeout = 30 * time.Minute

// snapshotFloor is the smallest export accepted as a snapshot. A rootfs archive
// is megabytes at the least, and a file under this is an export that failed
// while reporting success.
const snapshotFloor = int64(1) << 20

// refusal marks an error as this tool declining to act before it changed
// anything, which a command answers with exit 2, apart from an attempt that
// failed partway, which it answers with exit 1.
type refusal struct{ error }

func (r refusal) Unwrap() error { return r.error }

func refuse(format string, args ...any) error { return refusal{fmt.Errorf(format, args...)} }

// IsRefusal reports whether an error is a refusal rather than a failed attempt.
// A distribution this tool does not own is one.
func IsRefusal(err error) bool {
	var r refusal
	return errors.As(err, &r) || errors.Is(err, ErrNotOwned)
}

var throwawayNameShape = regexp.MustCompile(`^eph-[a-z0-9]([a-z0-9._-]{0,54}[a-z0-9])?$`)

// ValidThrowawayName is the rule for a name this tool may create or act on.
//
// ⛔ LOWER CASE ONLY. WSL compares distribution names case-insensitively while
// the state directory named for one is case-preserving, so `eph-A` and `eph-a`
// would be one distribution with two directories. ⛔ NO TRAILING DOT and no
// `..`, because the name is a Windows directory name as well.
func ValidThrowawayName(name string) error {
	if !throwawayNameShape.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("%w: %q is not a throwaway distribution name. A name is %s followed by lower-case letters, "+
			"digits, dots, dashes or underscores, ends in a letter or a digit, and is at most 60 characters",
			ErrNotOwned, name, ThrowawayPrefix)
	}
	return nil
}

// ThrowawayName decides the name a new throwaway distribution gets: the one the
// caller asked for, with the prefix added when it was left off, or one drawn
// from what it is built from.
//
// ⚠ THE PREFIX IS ADDED AND NOTHING ELSE IS CHANGED. A name in the wrong case
// is refused rather than lowered, because a name silently rewritten is a name a
// caller cannot find again.
func ThrowawayName(requested, builtFrom string) (name string, drawn bool, err error) {
	if r := strings.TrimSpace(requested); r != "" {
		if !strings.HasPrefix(r, ThrowawayPrefix) {
			r = ThrowawayPrefix + r
		}
		return r, false, ValidThrowawayName(r)
	}
	suffix, err := randomBase36(4)
	if err != nil {
		return "", true, err
	}
	stem := throwawayStem(builtFrom)
	name = ThrowawayPrefix + stem + "-" + suffix
	return name, true, ValidThrowawayName(name)
}

// throwawayStem is the readable part of a drawn name: an image's repository and
// tag, or an archive's file name, reduced to the name alphabet.
func throwawayStem(builtFrom string) string {
	s := strings.TrimSpace(builtFrom)
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		s = s[i+1:]
	}
	for _, ext := range []string{".tar.gz", ".tgz", ".tar.xz", ".tar"} {
		if strings.HasSuffix(strings.ToLower(s), ext) {
			s = s[:len(s)-len(ext)]
			break
		}
	}
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") || strings.Contains(out, "..") {
		out = strings.ReplaceAll(strings.ReplaceAll(out, "--", "-"), "..", ".")
	}
	if len(out) > 32 {
		out = out[:32]
	}
	out = strings.Trim(out, "-._")
	if out == "" {
		return "rootfs"
	}
	return out
}

// randomBase36 draws n characters from a cryptographic source. ⛔ Never a
// timestamp and never a weak generator: two runs drawing in the same instant
// must not collide by construction.
func randomBase36(n int) (string, error) {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	out := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := range out {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("no cryptographic randomness for a name: %w", err)
		}
		out[i] = alphabet[v.Int64()]
	}
	return string(out), nil
}

// throwawayOwnership decides whether a registered distribution is this tool's
// to act on.
//
// ⛔ TWO FACTS, AND BOTH ARE REQUIRED. The name is a throwaway name, and the disk
// directory WSL registered for it resolves strictly inside `distrosDir`. The
// second is the one that proves anything; the first makes a refusal say why.
// ⚠ No container runtime's protected distribution carries the prefix, so the
// name rule refuses every one of them before the disk is looked at, and a case
// asserts that rather than a second list restating it.
func throwawayOwnership(name, registeredDisk, distrosDir string) error {
	if err := ValidThrowawayName(name); err != nil {
		return err
	}
	if strings.TrimSpace(registeredDisk) == "" {
		return fmt.Errorf("%w: WSL records no disk directory for %s, so nothing proves this tool made it", ErrNotOwned, name)
	}
	if _, err := ResolveInside(distrosDir, registeredDisk); err != nil {
		return fmt.Errorf("%w: %s keeps its disk at %s, outside %s, so another run or another tool made it. "+
			"This tool does not touch it", ErrNotOwned, name, registeredDisk, distrosDir)
	}
	return nil
}

// cleanBasePath turns WSL's recorded directory into an ordinary path.
func cleanBasePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, `\\?\`)
	// ⚠ BOTH SEPARATORS, trimmed by hand. The path package trims only the
	// running host's, and the suite runs on a host where a backslash is a letter.
	p = strings.TrimRight(p, `\/`)
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}

var snapshotTagShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// SnapshotTag validates a tag before it becomes a file name.
//
// ⛔ VALIDATED BEFORE IT IS JOINED TO A PATH, and a Windows device name is
// refused by the part before its first dot: `nul` and `nul.x` are both the
// device, and a snapshot written to one is a snapshot that was never kept.
func SnapshotTag(tag string) (string, error) {
	t := strings.TrimSpace(tag)
	if !snapshotTagShape.MatchString(t) || strings.Contains(t, "..") || strings.HasSuffix(t, ".") {
		return "", fmt.Errorf("%q is not a usable snapshot tag. A tag is 1 to 64 letters, digits, dots, dashes or "+
			"underscores, starting with a letter or a digit and not ending in a dot", tag)
	}
	stem := strings.ToUpper(t)
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	if windowsDeviceName(stem) {
		return "", fmt.Errorf("%q names the Windows device %s, and a file of that name is not a file. Pick another tag", tag, stem)
	}
	return t, nil
}

func windowsDeviceName(upper string) bool {
	switch upper {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) {
		return upper[3] >= '1' && upper[3] <= '9'
	}
	return false
}

// ThrowawayOrigin is what a throwaway distribution was made from, kept beside
// its disk.
//
// ⛔ A RECORD, NEVER THE NAME. A drawn name carries a readable fragment of an
// image reference, and reading the image back out of it would be a value
// re-parsed out of a label a caller can choose.
type ThrowawayOrigin struct {
	Schema   string    `json:"schema"`
	Name     string    `json:"name"`
	Image    string    `json:"image,omitempty"`
	Tarball  string    `json:"tarball,omitempty"`
	Snapshot string    `json:"snapshot,omitempty"`
	Created  time.Time `json:"created"`
	Tool     string    `json:"tool,omitempty"`
}

// ThrowawayDistro is one registered throwaway distribution.
type ThrowawayDistro struct {
	Name      string           `json:"name"`
	Running   bool             `json:"running"`
	Disk      string           `json:"disk"`
	DiskBytes int64            `json:"disk_bytes,omitempty"`
	DiskKnown bool             `json:"disk_known"`
	Origin    *ThrowawayOrigin `json:"origin,omitempty"`
	// Creating is when another run started creating it, while that marker is
	// fresh. ⚠ Absent means no creation is in progress, not that none ever was.
	Creating *time.Time `json:"creating,omitempty"`
}

// HeldFile is a file or directory under `<home>/distros` that is not a
// registered distribution's disk.
type HeldFile struct {
	Kind    string    `json:"kind"` // rootfs, directory, marker, partial or snapshot
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Bytes   int64     `json:"bytes"`
	Known   bool      `json:"known"`
	ModTime time.Time `json:"mod_time"`
}

// ThrowawayReport is what `distro list` answers.
//
// ⛔ THE SPLIT IS THE POINT. `owned` is everything this state directory made and
// will act on. `elsewhere` carries the prefix and keeps its disk somewhere else,
// and is named so a reader knows it exists and knows this tool will not touch it.
type ThrowawayReport struct {
	Schema    string            `json:"schema"`
	Dir       string            `json:"dir"`
	Owned     []ThrowawayDistro `json:"owned"`
	Elsewhere []ThrowawayDistro `json:"elsewhere"`
	// Others is every registered distribution without the prefix, named so a
	// reader sees the whole machine and knows none of it is this tool's.
	Others    []OtherDistro `json:"others"`
	Leftovers []HeldFile    `json:"leftovers"`
	Snapshots []HeldFile    `json:"snapshots"`
}

// OtherDistro is a registered distribution this lifecycle never touches.
type OtherDistro struct {
	Name      string `json:"name"`
	Running   bool   `json:"running"`
	Protected bool   `json:"protected"`
}

// Throwaways is the lifecycle of the throwaway distributions one state
// directory owns.
type Throwaways struct {
	dir   string
	wsl   *Wsl
	log   func(string)
	disks func() (map[string]string, error)
	now   func() time.Time
	// readDir and sleep are os.ReadDir and time.Sleep when nil. A case replaces
	// them to produce an entry that vanishes mid-walk, or a registration that
	// outlives its unregister, neither of which a real machine does on demand.
	readDir func(string) ([]os.DirEntry, error)
	sleep   func(time.Duration)
}

// NewThrowaways binds the lifecycle to this host. ⛔ It creates nothing, so a
// report built from it changes nothing it describes.
func NewThrowaways(log func(string)) (*Throwaways, error) {
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
	return &Throwaways{dir: ThrowawayDir(home), wsl: w, log: log, disks: registeredDisks, now: time.Now}, nil
}

// ThrowawayDir is where a state directory keeps its throwaway distributions.
func ThrowawayDir(home string) string { return filepath.Join(home, "distros") }

// Dir is where this lifecycle keeps its distributions.
func (t *Throwaways) Dir() string { return t.dir }

func (t *Throwaways) distroDir(name string) string { return filepath.Join(t.dir, name) }
func (t *Throwaways) snapshotDir() string          { return filepath.Join(t.dir, "snapshots") }
func (t *Throwaways) originPath(name string) string {
	return filepath.Join(t.distroDir(name), "origin.json")
}
func (t *Throwaways) markerPath(name string) string {
	return filepath.Join(t.dir, name+".creating")
}

// List reads what this state directory's throwaway distributions are, and what
// else carries the prefix. It creates and removes nothing.
func (t *Throwaways) List(ctx context.Context) (ThrowawayReport, error) {
	rep := ThrowawayReport{Schema: ThrowawayListSchema, Dir: t.dir, Owned: []ThrowawayDistro{}, Elsewhere: []ThrowawayDistro{},
		Others: []OtherDistro{}, Leftovers: []HeldFile{}, Snapshots: []HeldFile{}}
	disks, err := t.disks()
	if err != nil {
		return rep, err
	}
	running, err := t.wsl.listRunningNames(ctx)
	if err != nil {
		return rep, err
	}
	live := map[string]bool{}
	for _, n := range running {
		live[strings.ToLower(n)] = true
	}
	names := make([]string, 0, len(disks))
	for n := range disks {
		names = append(names, n)
	}
	sort.Strings(names)
	ownedDirs := map[string]bool{}
	for _, name := range names {
		if !strings.HasPrefix(strings.ToLower(name), ThrowawayPrefix) {
			other := OtherDistro{Name: name, Running: live[strings.ToLower(name)]}
			for _, p := range ProtectedDistros {
				if strings.EqualFold(name, p) {
					other.Protected = true
				}
			}
			rep.Others = append(rep.Others, other)
			continue
		}
		d := ThrowawayDistro{Name: name, Running: live[strings.ToLower(name)], Disk: disks[name]}
		if throwawayOwnership(name, disks[name], t.dir) != nil {
			rep.Elsewhere = append(rep.Elsewhere, d)
			continue
		}
		d.DiskBytes, d.DiskKnown = FileSize(filepath.Join(disks[name], "ext4.vhdx"))
		d.Origin = t.readOrigin(name)
		if since, fresh := t.creating(name); fresh {
			d.Creating = &since
		}
		ownedDirs[strings.ToLower(name)] = true
		rep.Owned = append(rep.Owned, d)
	}
	rep.Leftovers, rep.Snapshots, err = t.heldFiles(ownedDirs)
	if err != nil {
		return rep, err
	}
	return rep, nil
}

func (t *Throwaways) readDirectory(path string) ([]os.DirEntry, error) {
	if t.readDir != nil {
		return t.readDir(path)
	}
	return os.ReadDir(path)
}

// vanished reports an entry that was listed and was gone by the time it was
// looked at.
//
// ⛔ ANOTHER RUN REMOVED IT, which is not an unreadable inventory. A creation
// removes its marker and its rootfs archive, and an export renames its partial
// into place, while this walk is between listing a directory and reading an
// entry. Failing the whole walk for that would fail every command of a second
// run sharing the state directory, so the entry is skipped and every other
// error still stops the walk.
func vanished(err error) bool { return errors.Is(err, os.ErrNotExist) }

// heldFiles walks `<home>/distros` for what is not a registered disk.
func (t *Throwaways) heldFiles(ownedDirs map[string]bool) (leftovers, snapshots []HeldFile, err error) {
	leftovers, snapshots = []HeldFile{}, []HeldFile{}
	entries, err := t.readDirectory(t.dir)
	if errors.Is(err, os.ErrNotExist) {
		if _, statErr := os.Lstat(t.dir); errors.Is(statErr, os.ErrNotExist) {
			return leftovers, snapshots, nil
		}
	}
	if err != nil {
		return leftovers, snapshots, fmt.Errorf("read throwaway state %s: %w", t.dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		path := filepath.Join(t.dir, name)
		info, err := e.Info()
		if vanished(err) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read throwaway state %s: %w", path, err)
		}
		switch {
		case e.IsDir() && name == "snapshots":
			snaps, err := t.readDirectory(path)
			if vanished(err) {
				continue
			}
			if err != nil {
				return nil, nil, fmt.Errorf("read snapshot state %s: %w", path, err)
			}
			for _, s := range snaps {
				si, err := s.Info()
				if vanished(err) {
					continue
				}
				if err != nil {
					return nil, nil, fmt.Errorf("read snapshot state %s: %w", filepath.Join(path, s.Name()), err)
				}
				if s.IsDir() {
					continue
				}
				held := HeldFile{Path: filepath.Join(path, s.Name()), Bytes: si.Size(), Known: true, ModTime: si.ModTime().UTC()}
				switch {
				case strings.HasSuffix(s.Name(), ".partial"):
					// ⚠ AN EXPORT THAT WAS KILLED leaves this, and it is a leftover
					// rather than a snapshot: nothing can import a truncated archive.
					held.Kind, held.Name = "partial", s.Name()
					leftovers = append(leftovers, held)
				case strings.HasSuffix(s.Name(), ".tar"):
					held.Kind, held.Name = "snapshot", strings.TrimSuffix(s.Name(), ".tar")
					snapshots = append(snapshots, held)
				}
			}
		case e.IsDir() && strings.HasPrefix(name, ThrowawayPrefix):
			if ownedDirs[strings.ToLower(name)] {
				continue
			}
			size, known, _ := dirSize(path)
			leftovers = append(leftovers, HeldFile{Kind: "directory", Name: name, Path: path, Bytes: size, Known: known, ModTime: info.ModTime().UTC()})
		case !e.IsDir() && strings.HasSuffix(name, ".tar"):
			leftovers = append(leftovers, HeldFile{Kind: "rootfs", Name: strings.TrimSuffix(name, ".tar"), Path: path,
				Bytes: info.Size(), Known: true, ModTime: info.ModTime().UTC()})
		case !e.IsDir() && strings.HasSuffix(name, ".creating"):
			leftovers = append(leftovers, HeldFile{Kind: "marker", Name: strings.TrimSuffix(name, ".creating"), Path: path,
				Bytes: info.Size(), Known: true, ModTime: info.ModTime().UTC()})
		}
	}
	return leftovers, snapshots, nil
}

// Owned answers one name, and refuses it unless this tool owns it and WSL has
// it registered.
func (t *Throwaways) Owned(ctx context.Context, name string) (ThrowawayDistro, error) {
	if err := ValidThrowawayName(name); err != nil {
		return ThrowawayDistro{}, err
	}
	rep, err := t.List(ctx)
	if err != nil {
		return ThrowawayDistro{}, err
	}
	return ownedIn(rep, name, t.dir)
}

// ownedIn is Owned's answer from a report already read.
//
// ⛔ A DISTRIBUTION ANOTHER RUN IS STILL CREATING IS REFUSED, as removal refuses
// it and --reuse skips it. That run may yet restart it for systemd, write its
// image environment, or roll it back, so a command, a shell or a snapshot taken
// now acts on something that is not finished being made.
func ownedIn(rep ThrowawayReport, name, distrosDir string) (ThrowawayDistro, error) {
	for _, d := range rep.Owned {
		if strings.EqualFold(d.Name, name) {
			if d.Creating != nil {
				return ThrowawayDistro{}, refuse("%s is being created by another run, started %s. Wait for that run to finish",
					d.Name, d.Creating.Format(time.RFC3339))
			}
			return d, nil
		}
	}
	for _, d := range rep.Elsewhere {
		if strings.EqualFold(d.Name, name) {
			return ThrowawayDistro{}, throwawayOwnership(d.Name, d.Disk, distrosDir)
		}
	}
	return ThrowawayDistro{}, refuse("%s is not a registered throwaway distribution. wsl-toolkit distro list names the ones this tool made", name)
}

func (t *Throwaways) readOrigin(name string) *ThrowawayOrigin {
	data, err := os.ReadFile(t.originPath(name))
	if err != nil {
		return nil
	}
	var o ThrowawayOrigin
	if json.Unmarshal(data, &o) != nil {
		return nil
	}
	// ⛔ A RECORD FOR ANOTHER NAME IS A COPY, and it describes something else.
	if o.Schema != ThrowawayOriginSchema || !strings.EqualFold(o.Name, name) {
		return nil
	}
	return &o
}

type creatingMarker struct {
	Schema  string    `json:"schema"`
	Name    string    `json:"name"`
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
}

// creating reports whether a creation marker for this name is fresh.
func (t *Throwaways) creating(name string) (time.Time, bool) {
	data, err := os.ReadFile(t.markerPath(name))
	if err != nil {
		return time.Time{}, false
	}
	var m creatingMarker
	if json.Unmarshal(data, &m) != nil || m.Schema != throwawayCreatingSchema || m.Started.IsZero() {
		return time.Time{}, false
	}
	return m.Started, t.now().Sub(m.Started) < throwawayCreatingTTL
}

// ThrowawaySpec is one `distro new`, or the command half of one `distro run`.
type ThrowawaySpec struct {
	// Image is a fully qualified reference and Tarball is a host path or a
	// snapshot tag. Exactly one is set.
	Image   string
	Tarball string
	Name    string
	User    string
	// Script is the caller's command, repaired, and nil when no command was asked
	// for, which is different from an empty one. Env and UserEnv are composed
	// around it by ComposePayload when it runs.
	Script    []byte
	Env       []EnvPair
	UserEnv   bool
	Timeout   time.Duration
	Systemd   bool
	OciEnv    bool
	Reuse     bool
	Ephemeral bool
	// ProbeTimeout bounds each question this tool asks the new distribution for
	// itself: the smoke probe, systemd's PID 1 and the image configuration. The
	// caller's command is bounded by Timeout and never by this.
	ProbeTimeout time.Duration
	// Log, when set, relays the command and records it, and carries the
	// heartbeat. It is opened by the caller before anything is created and begun
	// when the command starts.
	Log    *RunLog
	Stdout io.Writer
	Stderr io.Writer
}

// DefaultProbeTimeout is how long a question this tool asks a new distribution
// may take before the distribution is treated as wedged.
const DefaultProbeTimeout = 2 * time.Minute

// Validate refuses every combination in which a flag the caller typed would do
// nothing, before anything is created.
//
// ⛔ REFUSED, NEVER IGNORED. A setting no code reads is a caller believing they
// asked for something.
func (s ThrowawaySpec) Validate() error {
	switch {
	case (s.Image == "") == (s.Tarball == ""):
		return errors.New("pass exactly one of --image and --tarball: a distribution is built from one or the other")
	case s.Reuse && s.Image == "":
		return errors.New("--reuse finds a distribution built from an --image, and an archive carries no reference to match")
	case s.Reuse && s.Ephemeral:
		return errors.New("--reuse keeps a distribution to run in again and --ephemeral removes it afterwards. Pass one")
	case s.Reuse && (s.Systemd || s.OciEnv || s.Name != ""):
		return errors.New("--reuse runs in a distribution that already exists, so --systemd, --oci-env and --name would do nothing")
	case s.OciEnv && s.Image == "":
		return errors.New("--oci-env reads an image's configuration, and a rootfs archive carries none")
	case s.Ephemeral && s.Script == nil:
		return errors.New("--ephemeral removes the distribution once its command ends, and no command was given")
	case len(s.Env) > 0 && s.Script == nil:
		return errors.New("--env sets variables for a command, and no command was given")
	case s.UserEnv && s.Script == nil:
		return errors.New("--user-env prepares an environment for a command, and no command was given")
	case s.Log != nil && s.Script == nil:
		return errors.New("the logging options observe a command, and no command was given")
	case s.Timeout < 0:
		return fmt.Errorf("--timeout %s is negative. Pass 0 for no deadline, or a positive duration", s.Timeout)
	case s.ProbeTimeout != 0 && (s.ProbeTimeout < 5*time.Second || s.ProbeTimeout > time.Hour):
		return fmt.Errorf("--probe-timeout %s is outside 5s to 1h", s.ProbeTimeout)
	}
	if err := assertGuestUser(s.User); err != nil {
		return err
	}
	for _, p := range s.Env {
		if !isShellName(p.Name) {
			return fmt.Errorf("--env %q is not a usable environment name", p.Name)
		}
	}
	return nil
}

func (s ThrowawaySpec) probeTimeout() time.Duration {
	if s.ProbeTimeout > 0 {
		return s.ProbeTimeout
	}
	return DefaultProbeTimeout
}

func assertGuestUser(user string) error {
	if strings.TrimSpace(user) == "" {
		return errors.New("--user is empty. Pass an account that exists in the image, or leave it at root")
	}
	return AssertArgvSafe([]string{user})
}

// CommandOutcome is what one command inside a throwaway distribution did.
type CommandOutcome struct {
	Schema      string `json:"schema,omitempty"`
	Name        string `json:"name,omitempty"`
	Exit        int    `json:"exit"`
	TimedOut    bool   `json:"timed_out"`
	Cancelled   bool   `json:"cancelled"`
	StdoutBytes int64  `json:"stdout_bytes"`
	StderrBytes int64  `json:"stderr_bytes"`
	DurationMS  int64  `json:"duration_ms"`
	Error       string `json:"error,omitempty"`
	// LogError names where the relay could not write: this process's own
	// streams, or a log the caller asked for. The command's own exit is still Exit.
	LogError string `json:"log_error,omitempty"`
}

// ThrowawayRunSchema versions the answer `distro run --json` writes.
const ThrowawayRunSchema = "wsl-toolkit-distro-run/1"

// ThrowawayResult is what `distro new` answers.
type ThrowawayResult struct {
	Schema     string          `json:"schema"`
	Name       string          `json:"name"`
	Disk       string          `json:"disk"`
	Origin     ThrowawayOrigin `json:"origin"`
	Reused     bool            `json:"reused"`
	OS         string          `json:"os,omitempty"`
	Systemd    bool            `json:"systemd"`
	OciEnv     []string        `json:"oci_env,omitempty"`
	OciSkipped []string        `json:"oci_env_skipped,omitempty"`
	Command    *CommandOutcome `json:"command,omitempty"`
	Removed    bool            `json:"removed"`
	DurationMS int64           `json:"duration_ms"`
}

// Create makes one throwaway distribution and, when asked, runs a command in
// it.
//
// ⛔ WHAT IT LEAVES WHEN IT FAILS: nothing it made. A failure after the name was
// claimed unregisters the distribution and removes its directory, and the
// temporary rootfs archive is removed on every path out, success included.
func (t *Throwaways) Create(ctx context.Context, spec ThrowawaySpec) (ThrowawayResult, error) {
	started := t.now()
	res := ThrowawayResult{Schema: ThrowawayNewSchema}
	if spec.User == "" {
		spec.User = "root"
	}
	if err := spec.Validate(); err != nil {
		return res, err
	}

	if spec.Reuse {
		found, err := t.FindReusable(ctx, spec.Image)
		if err != nil {
			return res, err
		}
		if found != nil {
			// ⛔ IT SAYS WHICH IT DID, EVERY TIME. A reused distribution carries
			// whatever the previous run left in it.
			t.log(fmt.Sprintf("reusing %s, built from %s at %s. It carries whatever the previous run left in it",
				found.Name, spec.Image, found.Origin.Created.Format(time.RFC3339)))
			res.Name, res.Disk, res.Reused, res.Origin = found.Name, found.Disk, true, *found.Origin
			if spec.Script != nil {
				out := t.runIn(ctx, found.Name, spec)
				res.Command = &out
			}
			res.DurationMS = t.now().Sub(started).Milliseconds()
			return res, nil
		}
		t.log("no registered distribution was built from " + spec.Image + ", so one is imported")
	}

	pre, err := t.Preflight(ctx, spec)
	if err != nil {
		return res, err
	}
	rootfs, snapshot, engine := pre.Rootfs, pre.Snapshot, pre.Engine
	builtFrom := spec.Image
	if builtFrom == "" {
		builtFrom = spec.Tarball
	}
	name, err := t.claim(spec.Name, builtFrom)
	if err != nil {
		return res, err
	}
	dir := t.distroDir(name)
	res.Name, res.Disk = name, dir
	res.Origin = ThrowawayOrigin{Schema: ThrowawayOriginSchema, Name: name, Image: spec.Image, Snapshot: snapshot, Tool: toolVersion()}
	if spec.Tarball != "" && snapshot == "" {
		res.Origin.Tarball = rootfs
	}

	if err := t.writeMarker(name); err != nil {
		t.rollback(name, false)
		return res, err
	}
	defer t.removeMarker(name)

	registered := false
	fail := func(err error) (ThrowawayResult, error) {
		t.log("creation failed, so " + name + " is removed again")
		t.rollback(name, registered)
		return res, err
	}

	if engine != nil {
		rootfs = filepath.Join(t.dir, name+".tar")
		// ⛔ ON EVERY PATH OUT. The predecessor of this command removed it only
		// after a success, so every failed import left its archive behind.
		defer func() {
			if err := RemoveInside(t.dir, rootfs); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.log("the temporary rootfs archive is still on disk: " + err.Error())
			}
		}()
		t.log(fmt.Sprintf("host engine: %s, platform %s", engine.Name, engine.Platform()))
		if err := engine.ExportRootfs(ctx, spec.Image, rootfs, t.log); err != nil {
			return fail(err)
		}
	}
	if err := assertImportSpace(dir, rootfs, ThrowawaySpaceFloor, t.log); err != nil {
		return fail(err)
	}
	t.log("importing " + name)
	if err := t.wsl.importDistro(ctx, name, dir, rootfs); err != nil {
		return fail(err)
	}
	registered = true
	res.Origin.Created = t.now().UTC()
	if err := t.writeOrigin(res.Origin); err != nil {
		return fail(err)
	}

	osName, err := t.smoke(ctx, name, spec.probeTimeout())
	if err != nil {
		return fail(err)
	}
	res.OS = osName
	t.log(name + " is up: " + osName)

	if spec.Systemd {
		if err := t.enableSystemd(ctx, name, spec.probeTimeout()); err != nil {
			return fail(err)
		}
		res.Systemd = true
	}
	if spec.OciEnv {
		carried, skipped, err := t.carryOciEnv(ctx, name, engine, spec.Image, spec.probeTimeout())
		if err != nil {
			return fail(err)
		}
		res.OciEnv, res.OciSkipped = carried, skipped
	}

	if spec.Script != nil {
		out := t.runIn(ctx, name, spec)
		res.Command = &out
	}
	if spec.Ephemeral {
		// ⛔ THE TEARDOWN SURVIVES A CANCELLED COMMAND. A caller who interrupted
		// an ephemeral run asked for nothing to be left, and that is still true.
		teardown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Minute)
		defer cancel()
		if _, err := t.Remove(teardown, name, true); err != nil {
			res.DurationMS = t.now().Sub(started).Milliseconds()
			return res, fmt.Errorf("the command finished and %s could not be removed: %w", name, err)
		}
		res.Removed = true
	}
	res.DurationMS = t.now().Sub(started).Milliseconds()
	return res, nil
}

// NewPreflight is what a `distro new` resolves before it changes anything.
type NewPreflight struct {
	// Engine is the host engine an --image is pulled with, and nil for --tarball.
	Engine *Engine
	// Rootfs is the archive a --tarball names, and Snapshot the tag when it named
	// a snapshot.
	Rootfs   string
	Snapshot string
}

// Preflight answers every refusal a `distro new` makes before it changes
// anything: the archive or snapshot a --tarball names, a host engine for an
// --image, and a --name that is already taken.
//
// ⛔ THE PLAN AND THE CREATION READ THIS ONE ANSWER, so a dry run cannot pass
// what the real run refuses. Measured before it existed: a tag this state
// directory does not hold, a taken name and a host with no engine each exited 0
// under --dry-run, and 2 without it.
func (t *Throwaways) Preflight(ctx context.Context, spec ThrowawaySpec) (NewPreflight, error) {
	var pre NewPreflight
	var err error
	if pre.Rootfs, pre.Snapshot, err = t.resolveTarball(spec.Tarball); err != nil {
		return pre, err
	}
	if spec.Image != "" {
		if pre.Engine, err = FindEngine(ctx); err != nil {
			return pre, refuse("a distribution built from an image needs a host engine: %w", err)
		}
	}
	if strings.TrimSpace(spec.Name) != "" {
		name, _, err := ThrowawayName(spec.Name, "")
		if err != nil {
			return pre, err
		}
		if err := t.nameFree(name); err != nil {
			return pre, err
		}
	}
	return pre, nil
}

// nameFree refuses a requested name that is registered or already has a
// directory.
//
// ⚠ A CHECK AND NOT THE CLAIM. Another run can take the name after this
// answers, which is why claim takes it with os.Mkdir.
func (t *Throwaways) nameFree(name string) error {
	disks, err := t.disks()
	if err != nil {
		return err
	}
	for existing := range disks {
		if strings.EqualFold(existing, name) {
			return refuse("%s is already registered. Choose another --name, or remove it first", name)
		}
	}
	if _, err := os.Lstat(t.distroDir(name)); err == nil {
		return refuse("%s already has a directory under %s. Choose another --name, or remove it first", name, t.dir)
	}
	return nil
}

// resolveTarball answers the archive a `--tarball` value names: a path names a
// file, and a bare word names a snapshot this state directory holds.
//
// ⛔ ONLY A PATH IS LOOKED FOR ON DISK. The command layer resolves a relative
// path against the project and hands over an absolute one, and hands a bare word
// on only when no file of that name is there. Statting the word again here would
// read it against whatever directory this process started in, which is a second
// resolution of one value that can disagree with the first.
func (t *Throwaways) resolveTarball(value string) (path, snapshot string, err error) {
	if value == "" {
		return "", "", nil
	}
	if filepath.IsAbs(value) || strings.ContainsAny(value, `/\`) {
		if st, statErr := os.Stat(value); statErr == nil && !st.IsDir() {
			abs, err := filepath.Abs(value)
			return abs, "", err
		}
	} else if tag, tagErr := SnapshotTag(value); tagErr == nil {
		p := filepath.Join(t.snapshotDir(), tag+".tar")
		if st, statErr := os.Stat(p); statErr == nil && !st.IsDir() {
			t.log("--tarball " + value + " names the snapshot " + p + ", which carries whatever that distribution held")
			return p, tag, nil
		}
	}
	return "", "", refuse("--tarball %q is neither a file nor a snapshot tag this state directory holds. "+
		"wsl-toolkit distro list names the snapshots", value)
}

// claim decides the name and takes it, by creating the directory its disk will
// live in.
//
// ⭐ os.Mkdir IS THE CLAIM. Two runs asking for one name cannot both create one
// directory, so the loser is refused rather than importing over the winner.
func (t *Throwaways) claim(requested, builtFrom string) (string, error) {
	disks, err := t.disks()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(t.dir, 0o700); err != nil {
		return "", err
	}
	for attempt := 1; attempt <= 8; attempt++ {
		name, drawn, err := ThrowawayName(requested, builtFrom)
		if err != nil {
			return "", err
		}
		taken := false
		for existing := range disks {
			if strings.EqualFold(existing, name) {
				taken = true
			}
		}
		if !taken {
			err = os.Mkdir(t.distroDir(name), 0o700)
			if err == nil {
				return name, nil
			}
			if !errors.Is(err, os.ErrExist) {
				return "", err
			}
		}
		// ⛔ A NAME THE CALLER GAVE is their answer being wrong, and using a
		// different one would be worse than refusing. A DRAWN one is drawn again.
		if !drawn {
			return "", refuse("%s is already registered or already has a directory under %s. Choose another --name, or remove it first", name, t.dir)
		}
	}
	return "", errors.New("eight drawn names were all taken, which a four-character suffix makes implausible. Pass --name")
}

func (t *Throwaways) writeMarker(name string) error {
	data, err := json.MarshalIndent(creatingMarker{Schema: throwawayCreatingSchema, Name: name, PID: os.Getpid(), Started: t.now().UTC()}, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(t.markerPath(name), append(data, '\n'), 0o600)
}

func (t *Throwaways) removeMarker(name string) {
	if err := RemoveInside(t.dir, t.markerPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.log("the creation marker is still on disk: " + err.Error())
	}
}

func (t *Throwaways) writeOrigin(o ThrowawayOrigin) error {
	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(t.originPath(o.Name), append(data, '\n'), 0o600)
}

// rollback removes what a failed creation made. It reports what it could not
// remove rather than failing again, because the error the caller needs is the
// one that started the rollback.
func (t *Throwaways) rollback(name string, registered bool) {
	cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if registered {
		_ = t.wsl.terminateDistro(cleanup, name)
		if err := t.wsl.unregisterDistro(cleanup, name); err != nil {
			t.log("rollback could not unregister " + name + ": " + err.Error())
		}
	}
	if err := t.removeDir(name); err != nil {
		t.log("rollback could not remove " + t.distroDir(name) + ": " + err.Error())
	}
}

// removeDir removes a distribution's directory through the one deletion, and
// waits out the disk WSL releases after an unregister.
func (t *Throwaways) removeDir(name string) error {
	dir := t.distroDir(name)
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	var lastErr error
	for attempt := 1; attempt <= 5; attempt++ {
		if lastErr = RemoveInside(t.dir, dir); lastErr == nil {
			return nil
		}
		t.pause(time.Duration(attempt) * 300 * time.Millisecond)
	}
	return lastErr
}

// smokeMarker is assembled by the guest from two pieces, so no command text can
// satisfy the check that looks for it.
const smokeMarker = "wsl-toolkit-ready"

var smokeScript = []byte(`printf '%s-%s\n' wsl-toolkit ready
if [ -r /etc/os-release ]; then ( . /etc/os-release; printf 'os=%s\n' "${PRETTY_NAME:-${NAME:-unknown}}" ); fi
`)

// smoke proves the imported distribution can run a command at all, through the
// same framed channel a caller's command uses, so a guest that cannot carry a
// command fails here, at creation, rather than at a later run whose exit code
// would look like the command's own.
func (t *Throwaways) smoke(ctx context.Context, name string, timeout time.Duration) (string, error) {
	outBuf, errBuf := &boundedBuffer{max: 64 << 10}, &boundedBuffer{max: 64 << 10}
	code, err := t.wsl.Exec(ctx, ExecRequest{Distro: name, User: "root", Script: smokeScript, Login: true, Payload: true,
		Timeout: timeout, Stdout: outBuf, Stderr: errBuf})
	out, stderr := outBuf.String(), errBuf.String()
	if err != nil || code != 0 || !strings.Contains(out, smokeMarker) {
		return "", fmt.Errorf("%s was imported and cannot run a command (exit %d): %s. The rootfs needs a POSIX shell at /bin/sh, and "+
			"the command channel needs /dev/fd", name, code, firstLine(stderr+out))
	}
	osName := "unknown"
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "os="); ok && v != "" {
			osName = v
		}
	}
	return osName, nil
}

// enableSystemd writes /etc/wsl.conf, restarts the distribution so WSL reads
// it, and then CHECKS that systemd is PID 1.
//
// ⛔ THE CHECK IS THE POINT. Most OCI images ship no systemd, and a switch that
// wrote a file nothing acted on would be a flag that lies.
func (t *Throwaways) enableSystemd(ctx context.Context, name string, timeout time.Duration) error {
	t.log("enabling systemd through /etc/wsl.conf")
	if err := t.wsl.writeGuestFile(ctx, name, "/etc/wsl.conf", []byte("[boot]\nsystemd=true\n"), 0o644); err != nil {
		return err
	}
	if err := t.wsl.terminateDistro(ctx, name); err != nil {
		return err
	}
	out, stderr, code, err := t.wsl.Capture(ctx, name, "root", []byte("cat /proc/1/comm\n"), timeout)
	pid1 := strings.TrimSpace(out)
	if err != nil || code != 0 {
		return fmt.Errorf("%s restarted and PID 1 could not be read (exit %d): %s", name, code, firstLine(stderr+out))
	}
	if pid1 != "systemd" {
		return fmt.Errorf("--systemd was asked for and PID 1 in %s is %q. The image does not ship systemd, which most do not. "+
			"Nothing is left registered", name, pid1)
	}
	t.log("systemd is PID 1")
	return nil
}

type imageConfig struct {
	Env        []string `json:"Env"`
	WorkingDir string   `json:"WorkingDir"`
}

// carryOciEnv writes the image's ENV and WORKDIR where a login shell reads them.
//
// ⚠ AN EXPORTED FILESYSTEM CARRIES NO IMAGE CONFIGURATION, so without this a
// throwaway distribution runs with WSL's environment rather than the image's.
// USER and ENTRYPOINT are not carried: WSL fixes the login account per call and
// a login shell has no entrypoint to run.
func (t *Throwaways) carryOciEnv(ctx context.Context, name string, engine *Engine, ref string, timeout time.Duration) ([]string, []string, error) {
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	out, stderr, err := Output(bounded, engine.Path, "image", "inspect", ref, "--format", "{{json .Config}}")
	if err != nil {
		return nil, nil, fmt.Errorf("%s image inspect %s: %w: %s", engine.Name, ref, err, firstLine(stderr))
	}
	var cfg imageConfig
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &cfg); err != nil {
		return nil, nil, fmt.Errorf("the configuration of %s does not parse: %w", ref, err)
	}
	profile, carried, skipped := ociEnvProfile(cfg, ref)
	for _, s := range skipped {
		t.log("--oci-env skipped " + s)
	}
	code, err := t.wsl.Exec(ctx, ExecRequest{Distro: name, User: "root", Script: []byte("mkdir -p /etc/profile.d\n"), Timeout: timeout})
	if err != nil || code != 0 {
		return nil, nil, fmt.Errorf("could not create /etc/profile.d in %s (exit %d)", name, code)
	}
	if err := t.wsl.writeGuestFile(ctx, name, "/etc/profile.d/10-oci-env.sh", []byte(profile), 0o644); err != nil {
		return nil, nil, err
	}
	t.log(fmt.Sprintf("carried %d image environment variable(s) into /etc/profile.d/10-oci-env.sh", len(carried)))
	return carried, skipped, nil
}

// ociEnvProfile renders the image configuration as a profile script.
//
// ⛔ A SKIPPED ENTRY IS NAMED, never dropped: a variable the caller asked to
// carry and silence would read as carried.
func ociEnvProfile(cfg imageConfig, ref string) (profile string, carried, skipped []string) {
	lines := []string{
		"# Written by wsl-toolkit distro new --oci-env, from the configuration of",
		"# " + ref,
	}
	for _, e := range cfg.Env {
		k, v, ok := strings.Cut(e, "=")
		switch {
		case !ok || k == "":
			skipped = append(skipped, fmt.Sprintf("%q, which is not NAME=VALUE", e))
		case !isShellName(k):
			skipped = append(skipped, fmt.Sprintf("%q, which is not a shell variable name", k))
		default:
			lines = append(lines, "export "+k+"="+shellQuote(v))
			carried = append(carried, k)
		}
	}
	if cfg.WorkingDir != "" && cfg.WorkingDir != "/" {
		lines = append(lines, "cd "+shellQuote(cfg.WorkingDir)+" 2>/dev/null || :")
	}
	return strings.Join(lines, "\n") + "\n", carried, skipped
}

// runIn runs a caller's command in a throwaway distribution, with its streams
// forwarded unchanged and its exit code as the answer.
//
// ⛔ A DEADLINE OR A CANCELLATION TERMINATES THE DISTRIBUTION. Stopping wsl.exe
// ends the wait on this side and not the process in the guest, which would carry
// on holding the disk and a processor with nobody reading it.
func (t *Throwaways) runIn(ctx context.Context, name string, spec ThrowawaySpec) CommandOutcome {
	var outN, errN atomic.Int64
	started := time.Now()
	stdout, stderr := spec.Stdout, spec.Stderr
	if spec.Log != nil {
		// ⭐ THE HEARTBEAT IS THE RELAY'S. It fires on silence rather than on a
		// timer, and reads the distribution's state and disk when it does.
		//
		// ⚠ THE ADAPTER SAYS WHAT THIS IS. observe.go owns the seam; before it,
		// the relay reported every subject as a distribution, which was true of
		// every subject it had.
		spec.Log.Begin(name, &DistroObserver{Name: name, Read: t.tickFacts(name)})
		stdout, stderr = spec.Log.Stdout(), spec.Log.Stderr()
	}
	code, err := t.wsl.Exec(ctx, ExecRequest{
		Distro: name, User: spec.User, Script: ComposePayload(spec.Script, spec.Env, spec.UserEnv),
		Timeout: spec.Timeout, Login: true, Payload: true,
		Stdout: &countingWriter{to: stdout, count: &outN},
		Stderr: &countingWriter{to: stderr, count: &errN},
	})
	elapsed := time.Since(started)
	out := CommandOutcome{Exit: code, StdoutBytes: outN.Load(), StderrBytes: errN.Load(), DurationMS: elapsed.Milliseconds()}
	if err != nil {
		t.classifyRun(ctx, name, spec, elapsed, err, &out)
	}
	if spec.Log != nil {
		if lerr := spec.Log.Finish(RunOutcome{Exit: out.Exit, TimedOut: out.TimedOut, Cancelled: out.Cancelled,
			Timeout: spec.Timeout, StartError: out.Error}); lerr != nil {
			out.LogError = lerr.Error()
		}
	}
	return out
}

// classifyRun reads what an Exec error means for one command.
//
// ⚠ THE CODE ALONE CANNOT SAY WHICH HAPPENED. A killed wsl.exe also ends in an
// exit status, and a guest may exit 124 or 130 of its own accord, so the deadline
// and the caller's context are read rather than the number.
func (t *Throwaways) classifyRun(ctx context.Context, name string, spec ThrowawaySpec, elapsed time.Duration, err error, out *CommandOutcome) {
	var exited *exec.ExitError
	switch {
	case out.Exit == ExitTimeout && spec.Timeout > 0 && elapsed >= spec.Timeout:
		out.TimedOut = true
	case out.Exit == 130 && ctx.Err() != nil:
		out.Cancelled = true
	case errors.As(err, &exited):
		return
	default:
		out.Error = err.Error()
		return
	}
	stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	if terr := t.wsl.terminateDistro(stop, name); terr != nil {
		out.Error = "the command was stopped and " + name + " could not be terminated: " + terr.Error()
	}
}

// tickFacts reads what a heartbeat reports about one distribution.
//
// ⭐ --list --quiet and --running --quiet, never --verbose, whose state word is
// localised. Each question is bounded, and an answer that could not be read is
// the word unknown rather than a guess.
func (t *Throwaways) tickFacts(name string) func() TickFacts {
	return func() TickFacts {
		f := TickFacts{Kind: "distro", State: "unknown"}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if names, err := t.wsl.listNames(ctx); err == nil {
			f.State = "not registered"
			for _, n := range names {
				if strings.EqualFold(n, name) {
					f.State = "stopped"
				}
			}
			if f.State == "stopped" {
				running, err := t.wsl.listRunningNames(ctx)
				if err != nil {
					f.State = "unknown"
				}
				for _, n := range running {
					if strings.EqualFold(n, name) {
						f.State = "running"
					}
				}
			}
		}
		if size, ok := FileSize(filepath.Join(t.distroDir(name), "ext4.vhdx")); ok {
			f.DiskBytes = &size
		}
		return f
	}
}

// Run runs a command in a registered throwaway distribution this tool owns.
func (t *Throwaways) Run(ctx context.Context, name string, spec ThrowawaySpec) (CommandOutcome, error) {
	if spec.User == "" {
		spec.User = "root"
	}
	if err := assertGuestUser(spec.User); err != nil {
		return CommandOutcome{}, err
	}
	if spec.Timeout < 0 {
		return CommandOutcome{}, fmt.Errorf("--timeout %s is negative. Pass 0 for no deadline, or a positive duration", spec.Timeout)
	}
	if _, err := t.Owned(ctx, name); err != nil {
		return CommandOutcome{}, err
	}
	return t.runIn(ctx, name, spec), nil
}

// ThrowawayRemoval is what `distro remove` answers.
type ThrowawayRemoval struct {
	Schema       string `json:"schema"`
	Name         string `json:"name"`
	Unregistered bool   `json:"unregistered"`
	Deleted      string `json:"deleted,omitempty"`
}

// removalTarget is what one removal acts on: the registration, when there is
// one, and the directory.
//
// ⛔ THE PLAN AND THE REMOVAL READ THIS ONE SELECTION, so `remove --dry-run`
// cannot refuse what the removal would delete, or describe what it would refuse.
func (t *Throwaways) removalTarget(name string, ignoreCreating bool) (registered string, err error) {
	if err := ValidThrowawayName(name); err != nil {
		return "", err
	}
	disks, err := t.disks()
	if err != nil {
		return "", err
	}
	disk := ""
	for n, d := range disks {
		if strings.EqualFold(n, name) {
			registered, disk = n, d
		}
	}
	if registered != "" {
		if err := throwawayOwnership(registered, disk, t.dir); err != nil {
			return "", err
		}
	} else if _, statErr := os.Lstat(t.distroDir(name)); errors.Is(statErr, os.ErrNotExist) {
		return "", refuse("%s is not registered and %s holds nothing for it", name, t.dir)
	}
	if since, fresh := t.creating(name); fresh && !ignoreCreating {
		return "", refuse("%s is being created by another run, started %s. Wait for it, or remove it with distro purge --apply --include-live",
			name, since.Format(time.RFC3339))
	}
	return registered, nil
}

// PlanRemoval says what Remove would do, through the selection Remove reads.
func (t *Throwaways) PlanRemoval(name string) ([]string, error) {
	registered, err := t.removalTarget(name, false)
	if err != nil {
		return nil, err
	}
	var steps []string
	if registered != "" {
		steps = append(steps, "terminate and unregister it: wsl.exe --unregister "+registered, "read the registration back until it is gone")
	} else {
		steps = append(steps, name+" is not registered, so only what a failed creation left is removed")
	}
	return append(steps, "delete "+t.distroDir(name)+" through the one containment-checked deletion, and read it back"), nil
}

// SnapshotPath is where a validated tag's archive lives.
func (t *Throwaways) SnapshotPath(tag string) string {
	return filepath.Join(t.snapshotDir(), tag+".tar")
}

// Remove unregisters one owned throwaway distribution and deletes its directory,
// then reads both back.
//
// ⛔ A CREATION IN PROGRESS IS REFUSED unless ignoreCreating says otherwise:
// removing a distribution another run is still building fails that run in a way
// it cannot explain.
func (t *Throwaways) Remove(ctx context.Context, name string, ignoreCreating bool) (ThrowawayRemoval, error) {
	res := ThrowawayRemoval{Schema: ThrowawayRemoveSchema, Name: name}
	registered, err := t.removalTarget(name, ignoreCreating)
	if err != nil {
		return res, err
	}
	if registered != "" {
		t.log("unregistering " + registered)
		_ = t.wsl.terminateDistro(ctx, registered)
		if err := t.wsl.unregisterDistro(ctx, registered); err != nil {
			return res, err
		}
		if err := t.waitUnregistered(registered); err != nil {
			return res, err
		}
		res.Unregistered = true
	}
	if err := t.removeDir(name); err != nil {
		return res, fmt.Errorf("%s is still on disk: %w", t.distroDir(name), err)
	}
	res.Deleted = t.distroDir(name)
	if err := RemoveInside(t.dir, t.markerPath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.log("the creation marker is still on disk: " + err.Error())
	}
	return res, nil
}

// waitUnregistered reads WSL's registrations back until a name is gone.
//
// ⛔ `wsl --unregister` ANSWERING IS NOT THE SAME FACT AS THE REGISTRATION BEING
// GONE, and a removal that reported one for the other is the delete this tool
// reports without checking. WSL releases it asynchronously, so the read is
// repeated with a growing pause before it is called a failure.
func (t *Throwaways) waitUnregistered(name string) error {
	for attempt := 1; attempt <= 5; attempt++ {
		after, err := t.disks()
		if err != nil {
			return err
		}
		gone := true
		for n := range after {
			if strings.EqualFold(n, name) {
				gone = false
			}
		}
		if gone {
			return nil
		}
		t.pause(time.Duration(attempt) * 300 * time.Millisecond)
	}
	return fmt.Errorf("wsl --unregister %s answered and the distribution is still registered", name)
}

func (t *Throwaways) pause(d time.Duration) {
	if t.sleep != nil {
		t.sleep(d)
		return
	}
	time.Sleep(d)
}

// PurgePlan is what `distro purge` would remove, and what it did.
//
// ⛔ THE PLAN AND THE APPLY READ ONE SELECTION, so a dry run cannot list what
// the apply spares or spare what the apply removes.
type PurgePlan struct {
	Schema    string   `json:"schema"`
	DryRun    bool     `json:"dry_run"`
	Distros   []string `json:"distros"`
	Leftovers []string `json:"leftovers"`
	Kept      []string `json:"kept"`
	Removed   []string `json:"removed,omitempty"`
	Failed    []string `json:"failed,omitempty"`
}

type purgeCandidate struct {
	kind     string // distro, leftover, snapshot or elsewhere
	name     string
	path     string
	running  bool
	creating *time.Time
	// writing is when a partial export was last written, while an export could
	// still be writing it.
	writing *time.Time
}

// purgeCandidates is everything a report names, as purge weighs it.
func purgeCandidates(rep ThrowawayReport, creating func(string) (time.Time, bool), now time.Time) []purgeCandidate {
	var cands []purgeCandidate
	for _, d := range rep.Owned {
		cands = append(cands, purgeCandidate{kind: "distro", name: d.Name, path: d.Disk, running: d.Running, creating: d.Creating})
	}
	for _, d := range rep.Elsewhere {
		cands = append(cands, purgeCandidate{kind: "elsewhere", name: d.Name, path: d.Disk})
	}
	for _, f := range rep.Leftovers {
		c := purgeCandidate{kind: "leftover", name: f.Name, path: f.Path}
		if since, fresh := creating(f.Name); fresh {
			c.creating = &since
		}
		// ⛔ A PARTIAL EXPORT IS ONLY A LEFTOVER ONCE NO EXPORT CAN BE WRITING IT.
		// It carries no creation marker, so without this a purge beside a running
		// `distro snapshot` deleted the archive that export was writing.
		if f.Kind == "partial" && now.Sub(f.ModTime) < snapshotExportTimeout {
			at := f.ModTime
			c.writing = &at
		}
		cands = append(cands, c)
	}
	for _, s := range rep.Snapshots {
		cands = append(cands, purgeCandidate{kind: "snapshot", name: s.Name, path: s.Path})
	}
	return cands
}

// selectPurge decides what a purge removes and what it keeps, and why.
func selectPurge(cands []purgeCandidate, includeLive bool, now time.Time) (remove []purgeCandidate, kept []string) {
	for _, c := range cands {
		switch {
		case c.kind == "snapshot":
			kept = append(kept, c.path+": a snapshot is kept on purpose. Delete the file to remove it")
		case c.kind == "elsewhere":
			kept = append(kept, c.name+": its disk is at "+c.path+", outside this state directory, so this tool does not touch it")
		case c.creating != nil && !includeLive:
			kept = append(kept, fmt.Sprintf("%s: another run started creating it %s ago. Pass --include-live to remove it anyway",
				c.name, now.Sub(*c.creating).Round(time.Second)))
		case c.writing != nil && !includeLive:
			kept = append(kept, fmt.Sprintf("%s: an export wrote to it %s ago and may still be writing it. Pass --include-live to remove it anyway",
				c.path, now.Sub(*c.writing).Round(time.Second)))
		case c.running && !includeLive:
			kept = append(kept, c.name+": running right now. Pass --include-live to remove it anyway")
		default:
			remove = append(remove, c)
		}
	}
	return remove, kept
}

// Purge removes every throwaway distribution this state directory owns and what
// failed creations left, or with apply false, says what it would.
func (t *Throwaways) Purge(ctx context.Context, apply, includeLive bool) (PurgePlan, error) {
	plan := PurgePlan{Schema: ThrowawayPurgeSchema, DryRun: !apply, Distros: []string{}, Leftovers: []string{}, Kept: []string{}}
	rep, err := t.List(ctx)
	if err != nil {
		return plan, err
	}
	now := t.now()
	remove, kept := selectPurge(purgeCandidates(rep, t.creating, now), includeLive, now)
	plan.Kept = append(plan.Kept, kept...)
	for _, c := range remove {
		if c.kind == "distro" {
			plan.Distros = append(plan.Distros, c.name)
		} else {
			plan.Leftovers = append(plan.Leftovers, c.path)
		}
	}
	if !apply {
		return plan, nil
	}
	for _, c := range remove {
		if c.kind == "distro" {
			if _, err := t.Remove(ctx, c.name, includeLive); err != nil {
				plan.Failed = append(plan.Failed, c.name+": "+err.Error())
				continue
			}
			plan.Removed = append(plan.Removed, c.name)
			continue
		}
		if err := RemoveInside(t.dir, c.path); err != nil {
			plan.Failed = append(plan.Failed, c.path+": "+err.Error())
			continue
		}
		plan.Removed = append(plan.Removed, c.path)
	}
	if len(plan.Failed) > 0 {
		return plan, fmt.Errorf("%d item(s) could not be removed, starting with %s", len(plan.Failed), plan.Failed[0])
	}
	return plan, nil
}

// SnapshotResult is what `distro snapshot` answers.
type SnapshotResult struct {
	Schema   string `json:"schema"`
	Name     string `json:"name"`
	Tag      string `json:"tag"`
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Replaced bool   `json:"replaced"`
}

// Snapshot exports an owned throwaway distribution to an archive `distro new
// --tarball TAG` can import.
//
// ⚠ A SNAPSHOT CARRIES WHATEVER THE DISTRIBUTION HELD, a credential a command
// left behind included. It is an unencrypted archive on this machine's disk.
func (t *Throwaways) Snapshot(ctx context.Context, name, tag string, force bool) (SnapshotResult, error) {
	res := SnapshotResult{Schema: ThrowawaySnapshotSchema, Name: name}
	tag, err := SnapshotTag(tag)
	if err != nil {
		return res, refusal{err}
	}
	res.Tag = tag
	if _, err := t.Owned(ctx, name); err != nil {
		return res, err
	}
	out := filepath.Join(t.snapshotDir(), tag+".tar")
	res.Path = out
	if _, err := os.Lstat(out); err == nil && !force {
		return res, refuse("a snapshot tagged %s already exists at %s. Pass --force to replace it", tag, out)
	}
	if err := os.MkdirAll(t.snapshotDir(), 0o700); err != nil {
		return res, err
	}
	suffix, err := randomBase36(8)
	if err != nil {
		return res, err
	}
	// ⚠ A SIBLING TEMPORARY, RENAMED INTO PLACE. A killed export otherwise
	// leaves a truncated archive under the tag, and the next import of it fails
	// days later in a way nothing explains.
	partial := filepath.Join(t.snapshotDir(), "."+tag+"."+suffix+".partial")
	bounded, cancel := context.WithTimeout(ctx, snapshotExportTimeout)
	defer cancel()
	t.log("exporting " + name + " as snapshot " + tag)
	if outText, stderr, err := Output(bounded, t.wsl.Path, "--export", name, partial); err != nil {
		_ = os.Remove(partial)
		return res, fmt.Errorf("wsl --export %s: %w: %s", name, classifyWslFailure(outText, stderr, err), firstLine(outText+stderr))
	}
	size, err := acceptExport(name, partial)
	if err != nil {
		return res, err
	}
	if res.Replaced, err = publishSnapshot(partial, out, force); err != nil {
		_ = os.Remove(partial)
		return res, err
	}
	res.Bytes = size
	t.log(fmt.Sprintf("snapshot %s: %s at %s. It carries whatever %s held", tag, HumanBytes(size), out, name))
	return res, nil
}

// acceptExport refuses an export too small to be a rootfs archive, and removes
// it, so an export that failed while answering success is not kept under a tag.
func acceptExport(name, partial string) (int64, error) {
	size, ok := FileSize(partial)
	if !ok || size < snapshotFloor {
		_ = os.Remove(partial)
		return size, fmt.Errorf("wsl --export %s answered and wrote %d bytes, which is not a rootfs archive", name, size)
	}
	return size, nil
}

// publishSnapshot moves a finished export to its tag.
//
// ⛔ WITHOUT force IT NEVER REPLACES. The tag was free when the export began,
// and an export takes minutes, so another run can write the same tag meanwhile.
// A hard link is created only where nothing is, so the archive that got there
// first is kept and this export is refused. A volume that cannot link is checked
// and renamed, which is the same answer with a moment between the two.
//
// ⛔ WITH force IT IS ONE RENAME, so a failed replacement leaves the previous
// complete archive in place rather than no archive at all.
func publishSnapshot(partial, out string, force bool) (replaced bool, err error) {
	_, statErr := os.Lstat(out)
	exists := statErr == nil
	if force {
		return exists, os.Rename(partial, out)
	}
	if err := os.Link(partial, out); err == nil {
		// The export is kept under its tag; a partial left behind by a failed
		// removal here is a leftover purge collects, not a lost archive.
		_ = os.Remove(partial)
		return false, nil
	} else if errors.Is(err, os.ErrExist) {
		return false, refuse("a snapshot tagged %s appeared at %s while this export ran. It is kept, and this export is discarded. Pass --force to replace it", strings.TrimSuffix(filepath.Base(out), ".tar"), out)
	}
	if exists {
		return false, refuse("a snapshot already exists at %s. It is kept, and this export is discarded. Pass --force to replace it", out)
	}
	return false, os.Rename(partial, out)
}

// findReusable is the newest owned distribution built from exactly this
// reference, or nil.
//
// ⛔ AN EXACT MATCH ON THE REFERENCE the caller asked for, not a resolved
// digest: `alpine:latest` yesterday and today can be two images, and what the
// record supports is that the same thing was ASKED for.
func (t *Throwaways) FindReusable(ctx context.Context, ref string) (*ThrowawayDistro, error) {
	rep, err := t.List(ctx)
	if err != nil {
		return nil, err
	}
	var best *ThrowawayDistro
	for i := range rep.Owned {
		d := rep.Owned[i]
		if d.Origin == nil || d.Origin.Image != ref || d.Creating != nil {
			continue
		}
		if best == nil || d.Origin.Created.After(best.Origin.Created) {
			best = &d
		}
	}
	return best, nil
}
