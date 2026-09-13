// SPDX-License-Identifier: 0BSD

package compat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Space, removal, snapshots, origin records and reuse, ported from the
// script's disk.ps1 and reuse.ps1.

// assertEnoughDiskSpace refuses an import the volume cannot hold.
//
// ⚠ THE FACTOR IS MEASURED, NOT ASSUMED. Measured on a real machine, VHDX size
// on disk against the rootfs tarball that produced it:
//
//	alpine:3.22          8.2 MiB tar ->  76 MiB vhdx   9.27x
//	python:3.13-alpine  45.4 MiB tar -> 140 MiB vhdx   3.08x
//	debian:bookworm     74.3 MiB tar -> 172 MiB vhdx   2.31x
//	ubuntu:24.04        76.9 MiB tar -> 172 MiB vhdx   2.24x
//
// The cost is dominated by a FIXED FLOOR, not by a multiple: an 8 MiB rootfs
// still costs 76 MiB. So the requirement is a floor plus a multiple, and both
// are set above every measurement rather than fitted to them.
func (s *session) assertEnoughDiskSpace(tarballPath, targetDir string) error {
	st, err := os.Stat(tarballPath)
	if err != nil {
		return err
	}
	tar := st.Size()
	need := tar*importSpaceFactor + importSpaceFloor
	free, measurable := volumeFree(targetDir)

	note, refuse := spaceVerdict(need, free, measurable, targetDir)
	if note != "" {
		// ⛔ Named, not silent. A preflight that skipped is not a preflight
		// that passed, and the import is still worth attempting.
		s.log.warn(note)
	}
	if refuse != nil {
		return refuse
	}
	if note == "" {
		s.log.ok(fmt.Sprintf("space: %.0f MiB needed, %.0f MiB free",
			float64(need)/(1024*1024), float64(free)/(1024*1024)))
	}
	return nil
}

// spaceVerdict is the DECISION half of the preflight, separated so it can be
// proved on any host: a pure function of what the tarball needs, what the
// volume reports, and whether it reported at all.
func spaceVerdict(need, free int64, measurable bool, targetDir string) (string, error) {
	if !measurable {
		return fmt.Sprintf("could not read free space for '%s'; importing without the preflight. "+
			"If the volume is full, the import will leave a partial disk.", targetDir), nil
	}
	if free >= need {
		return "", nil
	}
	root := filepath.VolumeName(fullPathOr(targetDir))
	if root == "" {
		root = targetDir
	}
	return "", fmt.Errorf("NOT ENOUGH DISK SPACE to import '%s'. Need about %.0f MiB and %.0f MiB is free on the volume holding %s. "+
		"Nothing has been imported and nothing is registered. Free some space, or point LOCALAPPDATA at a volume that has it, and run this again.",
		targetDir, float64(need)/(1024*1024), float64(free)/(1024*1024), root)
}

// fullPathOr is fullPathNormalise without the refusal, for a message that
// merely wants a normalised spelling of a path that already exists.
func fullPathOr(p string) string {
	full, err := fullPathNormalise(p)
	if err != nil {
		return p
	}
	return full
}

// removeEphemeralDistro unregisters one ephemeral distro and deletes its
// disk, through the guards, in the order the guards must run.
func (s *session) removeEphemeralDistro(distro string, skipConfirm bool) error {
	// Hard guard, always first.
	if err := s.assertRemovable(distro); err != nil {
		return err
	}
	if !skipConfirm && !s.confirmDestructive(distro, "Unregister WSL distro and DELETE its disk") {
		s.log.warn("skipped " + distro)
		return nil
	}

	wsl, err := resolveWsl()
	if err != nil {
		return err
	}
	known, err := s.distroNames()
	if err != nil {
		return err
	}
	if containsString(known, distro) {
		if _, err := s.wslCapture(wsl, []string{"--terminate", distro}, true); err != nil {
			return err
		}
		if _, err := s.wslCapture(wsl, []string{"--unregister", distro}, false); err != nil {
			return err
		}
		s.log.ok("unregistered " + distro)
	} else {
		s.log.warn(distro + " was not registered")
	}

	dir := filepath.Join(s.baseDir, distro)
	if pathExists(dir) {
		remedy := fmt.Sprintf("Close it and re-run: -Action Remove -Name %s -Force.", distro)
		return s.removePathWithRetry(dir, "disk for '"+distro+"'", remedy)
	}
	return nil
}

// orphanTarball is a rootfs tarball sitting loose in the base directory. New
// writes one there and removes it in a finally, and a finally does not run on
// every hard interrupt, so an interrupted run can leave several hundred MiB
// that nothing reported.
//
// IT CANNOT TELL AN ORPHAN FROM A RUNNING NEW, and it does not pretend to:
// that is why the report carries the last-written time instead of a verdict.
type orphanTarball struct {
	Name    string
	Path    string
	Bytes   int64
	Written time.Time
}

func (s *session) orphanTarballs() []orphanTarball {
	if s.baseDir == "" {
		return nil
	}
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil
	}
	var out []orphanTarball
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".tar") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.IsDir() {
			continue
		}
		out = append(out, orphanTarball{
			Name:    e.Name(),
			Path:    filepath.Join(s.baseDir, e.Name()),
			Bytes:   info.Size(),
			Written: info.ModTime().UTC(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// directorySizeBytes reports bytes under a directory, with false when it
// cannot be measured.
//
// ⛔ THE CONTAINMENT GUARD, ON A READ. It is not here to protect a deletion;
// nothing here deletes. It is here because the path is built from a distro
// name wsl.exe reported, and a name carrying a traversal would send a
// recursive walk somewhere it has no business being.
func (s *session) directorySizeBytes(path string) (int64, bool) {
	if err := s.assertInsideBaseDir(path); err != nil {
		return 0, false
	}
	st, err := os.Lstat(path)
	if err != nil {
		return 0, false
	}
	if !st.IsDir() {
		return 0, false
	}
	var sum int64
	err = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			// An entry that vanished between the listing and the walk is not
			// a reason to report the whole directory unreadable.
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			sum += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, false
	}
	return sum, true
}

// -- snapshots ---------------------------------------------------------------

// snapshotDir is where a snapshot lives.
//
// ⭐ A SUBDIRECTORY. The orphan walk enumerates `*.tar` in the base directory
// and nowhere else, so a snapshot kept beside a distro's own rootfs would be
// reported as an orphan and removed by the next Purge. A caller who thought a
// snapshot was durable would lose it, and the loss would look like the tool
// working. Distinguished by STRUCTURE, not by a name pattern.
func (s *session) snapshotDir() (string, error) {
	if s.baseDir == "" {
		return "", fmt.Errorf("No state directory: LOCALAPPDATA is unset and no -StateDir was given.")
	}
	return filepath.Join(s.baseDir, "snapshots"), nil
}

// snapshotTag validates a tag that can be a file name, or refuses saying why.
//
// ⛔ VALIDATED BEFORE IT IS JOINED TO A PATH. A caller-supplied path component
// is how a tag becomes a write anywhere on the disk, and the reserved device
// names are refused by name because 'nul' silently discards everything
// written to it: a caller would believe they had a snapshot.
func snapshotTag(tag string) (string, error) {
	if strings.TrimSpace(tag) == "" {
		return "", fmt.Errorf("A snapshot tag is required. Pass -As <tag>.")
	}
	t := strings.TrimSpace(tag)
	if len(t) > 64 || !tagShape.MatchString(t) {
		return "", fmt.Errorf("'%s' is not a usable snapshot tag. Use 1 to 64 characters of letters, digits, "+
			"dot, dash or underscore, starting with a letter or a digit.", tag)
	}
	switch strings.ToLower(t) {
	case "con", "prn", "aux", "nul", "com1", "com2", "com3", "com4", "com5", "com6",
		"com7", "com8", "com9", "lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9":
		return "", fmt.Errorf("'%s' is a Windows reserved device name, so a file of that name is not a file. Pick another tag.", t)
	}
	return t, nil
}

var tagShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func (s *session) snapshotPath(tag string) (string, error) {
	dir, err := s.snapshotDir()
	if err != nil {
		return "", err
	}
	t, err := snapshotTag(tag)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, t+".tar"), nil
}

// snapshot is every snapshot on this machine, oldest name first. Reports
// rather than judges: List, Doctor and Purge all read this one function.
type snapshotFile struct {
	Name  string
	Path  string
	Bytes int64
}

func (s *session) snapshots() []snapshotFile {
	// ⛔ THROUGH THE ONE RESOLVER: a second spelling of the snapshots
	// directory is a second place for the subdirectory decision to drift,
	// which is the defect the subdirectory exists to answer.
	dir, err := s.snapshotDir()
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []snapshotFile
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".tar") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.IsDir() {
			continue
		}
		out = append(out, snapshotFile{Name: e.Name(), Path: filepath.Join(dir, e.Name()), Bytes: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// -- origin records and reuse ------------------------------------------------

func (s *session) originPath(distro string) string {
	return filepath.Join(filepath.Join(s.baseDir, distro), "origin.json")
}

// writeDistroOrigin records what a distro was made from, beside its disk.
//
// ⛔ A FILE, NOT THE NAME. The generated distro name carries a sanitised
// fragment of the image reference, and reading the image back out of it would
// be a value re-parsed out of a mutable name: a stored thing's identity is a
// stable opaque token, never something recovered from a label somebody can
// change. `alpine:3.22` and `alpine:3.21` sanitise to names that differ by one
// character, and -Name lets a caller pick a name with no relation to the image
// at all.
//
// ⛔ VERSIONED AND SELF-DESCRIBING. A positional record that changes shape
// mis-reads silently, and this one decides whether a caller's command runs in
// a distribution built from a different image.
func (s *session) writeDistroOrigin(distro, imageRef string) error {
	if imageRef == "" {
		return nil
	}
	path := s.originPath(distro)
	if err := s.assertInsideBaseDir(path); err != nil {
		return err
	}
	body, err := json.MarshalIndent(struct {
		Schema  string `json:"schema"`
		Image   string `json:"image"`
		Created string `json:"created"`
	}{
		Schema:  "wsl-toolkit-origin/1",
		Image:   imageRef,
		Created: now().UTC().Format(time.RFC3339Nano),
	}, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	// ⚠ Written atomically, as a sibling then renamed. A killed write
	// otherwise leaves a truncated JSON that -Reuse would refuse to parse,
	// which reads as a broken tool rather than as an interrupted run.
	temp := path + "." + randomSuffix(12) + ".tmp"
	if err := os.WriteFile(temp, body, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}

type originRecord struct {
	Image   string `json:"image"`
	Created string `json:"created"`
}

// readDistroOrigin is what one distro was built from, or nothing.
//
// ⚠ AN UNREADABLE OR UNKNOWN RECORD IS NOTHING AND NEVER A GUESS. A distro
// created before this file existed has none, and treating "no record" as
// "matches whatever you asked for" would run a caller's command in a
// distribution built from something else.
func (s *session) readDistroOrigin(distro string) *originRecord {
	path := s.originPath(distro)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var rec originRecord
	if json.Unmarshal(body, &rec) != nil {
		return nil
	}
	// The schema field is checked by shape: a record that does not declare it
	// is not this tool's.
	if !strings.Contains(string(body), `"wsl-toolkit-origin/1"`) {
		return nil
	}
	if rec.Image == "" {
		return nil
	}
	return &rec
}

// reusableDistro is a registered ephemeral distro built from this exact image
// reference, newest first.
//
// ⛔ AN EXACT MATCH ON THE REFERENCE, not a resolved digest and not a tag
// prefix. `alpine:latest` yesterday and `alpine:latest` today can be two
// different images, and this cannot tell them apart; what it promises is that
// the caller ASKED for the same thing, which is the claim the record actually
// supports.
type reusableDistro struct {
	Name  string
	When  time.Time
	Image string
}

func (s *session) findReusableDistro(imageRef string) *reusableDistro {
	if s.baseDir == "" {
		return nil
	}
	if _, err := os.Lstat(s.baseDir); err != nil {
		return nil
	}
	registered, err := s.distroNames()
	if err != nil {
		return nil
	}
	var best *reusableDistro
	for _, name := range registered {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rec := s.readDistroOrigin(name)
		if rec == nil {
			continue
		}
		if rec.Image != imageRef {
			continue
		}
		when, err := time.Parse(time.RFC3339Nano, rec.Created)
		if err != nil {
			when = time.Time{}
		}
		if best == nil || when.After(best.When) {
			best = &reusableDistro{Name: name, When: when, Image: rec.Image}
		}
	}
	return best
}

func formatDistroAge(found *reusableDistro) string {
	if found.When.IsZero() {
		return "age unknown"
	}
	return formatDuration(dur(now().Sub(found.When))) + " old"
}

// containsString is the port of PowerShell's -contains, which compares
// strings CASE-INSENSITIVELY. A member check that compared exactly would
// treat a distro wsl reports as "EPH-X" as absent from a list holding
// "eph-x", and the removal guard would then take the wrong branch.
func containsString(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}
