package toolkit

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ⛔ NO HOST DIRECTORY IS MOUNTED INTO A CONTAINER. A job gets a COPY of its
// workspace, delivered as an archive; getting anything back out is a second,
// explicit act with its own validation. A mount is a hole every path has to
// close, and a copy is a hole that does not exist.

// WorkspaceLimits bound what may cross in either direction.
//
// ⛔ Over a limit is REFUSED, never truncated. A job that ran against a tree
// missing files, and reported on it as the real one, is the outcome to avoid.
type WorkspaceLimits struct {
	MaxBytes   int64
	MaxEntries int
}

// DefaultWorkspaceLimits are deliberately generous enough for a source tree and
// far below what would fill a disk.
func DefaultWorkspaceLimits() WorkspaceLimits {
	return WorkspaceLimits{MaxBytes: 1 << 30, MaxEntries: 200000}
}

// ErrWorkspaceRefused wraps every containment and limit refusal.
var ErrWorkspaceRefused = errors.New("workspace refused")

// WorkspaceOmission is one entry left OUT of an upload, and why.
//
// ⛔ AN OMISSION IS REPORTED, NEVER SILENT. A Windows junction pointing outside
// a workspace was skipped by the walker's default branch and the job exited 0
// having never seen it, so a caller could not tell its input was incomplete.
// WSL-47, issue 26.
//
// ⭐ COUNTED RATHER THAN REFUSED, and the asymmetry with an artifact link is
// deliberate: a caller CHOOSES what it puts in /out, so a refusal there is
// actionable, while a junction somewhere in a large tree is a normal thing to
// have and failing the job over one would make the workspace feature unusable.
type WorkspaceOmission struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// maxReportedOmissions bounds what one result carries.
//
// ⚠ A TREE FULL OF LINKS WOULD OTHERWISE PUT THOUSANDS OF ROWS IN AN ANSWER.
// The COUNT is always exact; the list is the first few, and the result says so
// rather than quietly holding a prefix.
const maxReportedOmissions = 20

// WorkspaceUpload is what one upload produced.
type WorkspaceUpload struct {
	Entries  int                 `json:"entries"`
	Bytes    int64               `json:"bytes"`
	Omitted  int                 `json:"omitted"`
	Omission []WorkspaceOmission `json:"omission,omitempty"`
	// Truncated counts files that TRAVELLED but grew while they were read, so
	// the copy holds a prefix of what is on disk now.
	//
	// ⛔ IT IS NOT AN OMISSION AND MUST NOT BE COUNTED AS ONE. `Omitted` means
	// a caller's input did not arrive at all, which is the thing that makes a
	// job's conclusions wrong; a truncated file is a consistent snapshot of a
	// file something is still writing. Folding the two together would make the
	// serious number go up for the ordinary case.
	Truncated  int                 `json:"truncated,omitempty"`
	Truncation []WorkspaceOmission `json:"truncation,omitempty"`
	// ExecRestored names how many files got an executable bit the host
	// filesystem could not carry, and where each one came from. Empty when
	// nothing was restored.
	ExecRestored string `json:"exec_restored,omitempty"`
}

// note records an omission, keeping every count and the first few reasons.
func (u *WorkspaceUpload) note(rel, reason string) {
	u.Omitted++
	if len(u.Omission) < maxReportedOmissions {
		u.Omission = append(u.Omission, WorkspaceOmission{Path: rel, Reason: reason})
	}
}

// truncate records a file that arrived as a prefix of itself.
func (u *WorkspaceUpload) truncate(rel, reason string) {
	u.Truncated++
	if len(u.Truncation) < maxReportedOmissions {
		u.Truncation = append(u.Truncation, WorkspaceOmission{Path: rel, Reason: reason})
	}
}

// guestPathAlphabet is what a path handed to wsl.exe as an ARGUMENT may
// contain. An argument reaching wsl.exe is expanded before the guest sees it,
// so a dollar sign or a backtick in one is not data.
const guestPathAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./=:,@+%"

// AssertArgvSafe refuses an argument list that could be reinterpreted between
// here and the guest.
//
// ⛔ An allow-list. The hazard is not the dollar sign but whatever the value it
// expands to happens to contain, so a deny-list cannot cover it.
func AssertArgvSafe(argv []string) error {
	for _, a := range argv {
		if a == "" {
			return fmt.Errorf("%w: an empty argument", ErrWorkspaceRefused)
		}
		for _, r := range a {
			if !strings.ContainsRune(guestPathAlphabet, r) {
				return fmt.Errorf("%w: %q carries %q, which is outside the alphabet an argument to wsl.exe survives", ErrWorkspaceRefused, a, r)
			}
		}
	}
	return nil
}

// ExecDirect runs a program inside a distribution with NO shell between here
// and it, so stdin is free to carry data.
//
// ⭐ Two channels, two jobs. Exec puts a SCRIPT on stdin because a script is
// arbitrary text. This puts a data STREAM there and takes a fixed argv, which
// is this executable's own and asserted against the alphabet above.
func (w *Wsl) ExecDirect(ctx context.Context, distro, user, cwd string, argv []string, stdin io.Reader, stdout, stderr io.Writer, timeout time.Duration) (int, error) {
	if len(argv) == 0 {
		return 2, errors.New("ExecDirect needs a program to run")
	}
	if err := AssertArgvSafe(argv); err != nil {
		return 2, err
	}
	if cwd != "" {
		if err := AssertArgvSafe([]string{cwd}); err != nil {
			return 2, err
		}
	}
	if user == "" {
		user = "root"
	}
	args := []string{"-d", distro, "-u", user}
	if cwd != "" {
		args = append(args, "--cd", cwd)
	}
	args = append(args, "--")
	args = append(args, argv...)

	bounded := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		bounded, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	err := runCommand(bounded, newCommand(bounded, w.Path, args...), stdin, stdout, stderr)
	if err == nil {
		return 0, nil
	}
	return ExitCode(err), err
}

// SendWorkspace copies a host directory into a guest directory as an archive.
// The guest directory is created empty first, so a job never runs against a
// merge of its own workspace and somebody else's leftovers.
func (w *Wsl) SendWorkspace(ctx context.Context, distro, user, guestDir, hostDir string, limits WorkspaceLimits, excludes []string, log func(string)) (WorkspaceUpload, error) {
	var zero WorkspaceUpload
	if err := AssertArgvSafe([]string{guestDir}); err != nil {
		return zero, err
	}
	info, err := os.Stat(hostDir)
	if err != nil {
		return zero, fmt.Errorf("the workspace to copy: %w", err)
	}
	if !info.IsDir() {
		return zero, fmt.Errorf("%s is not a directory", hostDir)
	}

	errBuf := &boundedBuffer{max: 64 << 10}
	if code, err := w.ExecDirect(ctx, distro, user, "", []string{"/bin/mkdir", "-p", guestDir}, nil, io.Discard, errBuf, 2*time.Minute); err != nil || code != 0 {
		return zero, fmt.Errorf("could not create %s in the guest (exit %d): %s", guestDir, code, firstLine(errBuf.String()))
	}

	pr, pw := io.Pipe()
	type result struct {
		up  WorkspaceUpload
		err error
	}
	done := make(chan result, 1)
	go func() {
		up, err := writeWorkspaceTar(pw, hostDir, limits, excludes)
		// ⛔ CloseWithError, not Close. A writer that stopped at a limit must
		// make the READER fail too, or the guest unpacks a truncated archive
		// without complaint.
		_ = pw.CloseWithError(err)
		done <- result{up, err}
	}()

	errBuf = &boundedBuffer{max: 64 << 10}
	code, execErr := w.ExecDirect(ctx, distro, user, "", []string{"/bin/tar", "-xf", "-", "-C", guestDir}, pr, io.Discard, errBuf, 60*time.Minute)
	res := <-done
	if res.err != nil {
		return res.up, res.err
	}
	if execErr != nil || code != 0 {
		return res.up, fmt.Errorf("unpacking the workspace in the guest exited %d: %s", code, firstLine(errBuf.String()))
	}
	if log != nil {
		// ⭐ THE HOST DIRECTORY IS NAMED, not only the guest one. `--workspace .`
		// resolves against whatever the working directory happens to be, and the
		// only way a caller can tell which tree actually travelled is to be told
		// which one it was.
		log(fmt.Sprintf("workspace: %d entries, %s copied from %s to %s", res.up.Entries, HumanBytes(res.up.Bytes), hostDir, guestDir))
		// ⭐ SAID OUT LOUD AT THE POINT IT HAPPENS, as well as carried on the
		// result. A caller reading only the human output learns the same fact.
		for _, o := range res.up.Omission {
			log("workspace: left out " + o.Path + ": " + o.Reason)
		}
		if res.up.Omitted > len(res.up.Omission) {
			log(fmt.Sprintf("workspace: and %d more entry(s) left out", res.up.Omitted-len(res.up.Omission)))
		}
		for _, t := range res.up.Truncation {
			log("workspace: " + t.Path + ": " + t.Reason)
		}
		if res.up.Truncated > len(res.up.Truncation) {
			log(fmt.Sprintf("workspace: and %d more file(s) grew while they were copied", res.up.Truncated-len(res.up.Truncation)))
		}
		// ⛔ A MODE THIS TOOL SUPPLIED IS ANNOUNCED. The alternative to saying it
		// is `chmod -R +x`, which marks data executable and reports nothing, and
		// the distance between the two is that a reader can check this one.
		if res.up.ExecRestored != "" {
			log("workspace: " + res.up.ExecRestored)
		}
	}
	return res.up, nil
}

func writeWorkspaceTar(w io.Writer, root string, limits WorkspaceLimits, excludes []string) (WorkspaceUpload, error) {
	var up WorkspaceUpload
	tw := tar.NewWriter(w)
	realRoot, err := resolveExisting(root)
	if err != nil {
		return up, err
	}
	bits := &execBits{index: map[string]bool{}}
	if isGitWorkspace(root) {
		bits = newExecBits(context.Background(), root)
	}
	entries := 0
	var total int64
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		slashRel := filepath.ToSlash(rel)
		if matchesAny(slashRel, excludes) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entries++
		if entries > limits.MaxEntries {
			return fmt.Errorf("%w: more than %d entries under %s", ErrWorkspaceRefused, limits.MaxEntries, root)
		}
		switch {
		case d.IsDir():
			return tw.WriteHeader(&tar.Header{
				Name: slashRel + "/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: info.ModTime(),
			})
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			// ⛔ A link out of the workspace is LEFT OUT AND NAMED, not
			// refused and not silently dropped. It used to fail the whole job,
			// which makes the workspace feature unusable on any tree that has a
			// stray link in it; the reason a caller needs is which entry did not
			// travel, and that is now on the result. WSL-47.
			resolved, err := resolveExisting(LinkTargetPath(p, target))
			if err != nil {
				up.note(slashRel, "its target could not be resolved: "+err.Error())
				entries--
				return nil
			}
			if !hasPathPrefix(resolved, realRoot) && !pathEqual(resolved, realRoot) {
				up.note(slashRel, "it links to "+target+", which is outside the workspace")
				entries--
				return nil
			}
			return tw.WriteHeader(&tar.Header{
				Name: slashRel, Typeflag: tar.TypeSymlink, Linkname: filepath.ToSlash(target),
				Mode: 0o777, ModTime: info.ModTime(),
			})
		case info.Mode().IsRegular():
			total += info.Size()
			if total > limits.MaxBytes {
				return fmt.Errorf("%w: the workspace passes %s at %s", ErrWorkspaceRefused, HumanBytes(limits.MaxBytes), slashRel)
			}
			mode := bits.mode(info.Mode(), slashRel, p)
			if err := tw.WriteHeader(&tar.Header{
				Name: slashRel, Typeflag: tar.TypeReg, Mode: mode, Size: info.Size(), ModTime: info.ModTime(),
			}); err != nil {
				return err
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			// ⛔ BOUNDED BY THE DECLARED SIZE, because a tar member's length goes
			// into its header before its bytes are read and a file that GROWS in
			// between overruns what was declared. Unbounded, the archiver
			// refused with `archive/tar: write too long`, which names the
			// archiver and not the file, and the whole job died in 475 ms:
			// measured by a consumer on 2026-09-12 against a live CodeGraph
			// index, and worked around there by excluding four sidecars by name.
			// ⚠ A background daemon appending to a log is an ordinary thing for
			// a tree to be doing, and it is not a reason to refuse a copy.
			written, err := io.Copy(tw, io.LimitReader(f, info.Size()))
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			// ⛔ Count what arrived rather than trusting the declared length. A
			// file that SHRANK leaves a header disagreeing with its payload, and
			// that is a broken archive rather than a stale snapshot, so it still
			// refuses. ⚠ The two directions are not symmetric: the growing case
			// is a truncation that is named, the shrinking case is a corruption
			// that cannot be.
			if written != info.Size() {
				return fmt.Errorf("%w: %s shrank while it was being read (%d of %d bytes)", ErrWorkspaceRefused, slashRel, written, info.Size())
			}
			if grew, err := os.Stat(p); err == nil && grew.Size() > info.Size() {
				up.truncate(slashRel, fmt.Sprintf("it grew while it was copied; the copy holds the first %s", HumanBytes(info.Size())))
			}
			return nil
		default:
			// ⛔ A WINDOWS JUNCTION LANDS HERE, and it used to be dropped in
			// silence beside the sockets and devices. Go reports one as
			// irregular rather than as a symlink, so the branch above never saw
			// it: a workspace containing one was uploaded without it and the job
			// exited 0 having never been told. WSL-47, issue 26.
			if info.Mode()&fs.ModeIrregular != 0 {
				up.note(slashRel, "it is a junction or another reparse point, which is not carried into a container")
			}
			// A socket, a device or a named pipe is not workspace content, and
			// carrying one into a container would be handing it a channel.
			entries--
			return nil
		}
	})
	if walkErr != nil {
		return up, walkErr
	}
	if err := tw.Close(); err != nil {
		return up, err
	}
	up.Entries, up.Bytes = entries, total
	up.ExecRestored = bits.report()
	return up, nil
}

// assertLinkStaysInside refuses an archive link whose target leaves the
// destination.
//
// ⛔ IT REASONS IN SLASHES, not in the host's separators. The archive comes from
// a Linux guest, so `../../etc/passwd` and `/etc/passwd` are what a link says,
// and `filepath` on Windows would treat a forward slash and a backslash alike
// while `path` would not. The rule is about the ARCHIVE's own grammar.
func assertLinkStaysInside(dest, rel, linkname string) error {
	if linkname == "" {
		return fmt.Errorf("%w: %s is a link with no target", ErrWorkspaceRefused, rel)
	}
	// An absolute target stands alone. Joining one onto the link's own
	// directory produces a path inside the tree by every containment test there
	// is, which is the mistake LinkTargetPath exists to name.
	if strings.HasPrefix(linkname, "/") || strings.HasPrefix(linkname, `\`) || hasDriveLetter(linkname) {
		return fmt.Errorf("%w: %s points at %q, which is an absolute path and leaves the directory this job delivered. "+
			"A link out of the tree is refused rather than converted",
			ErrWorkspaceRefused, rel, linkname)
	}
	joined := path.Join(path.Dir(filepath.ToSlash(rel)), filepath.ToSlash(linkname))
	cleaned := path.Clean(joined)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("%w: %s points at %q, which resolves to %q and leaves the directory this job delivered. "+
			"A link out of the tree is refused rather than converted",
			ErrWorkspaceRefused, rel, linkname, cleaned)
	}
	return nil
}

// hasDriveLetter says whether a name starts `C:` in either separator style.
func hasDriveLetter(s string) bool {
	if len(s) < 2 || s[1] != ':' {
		return false
	}
	c := s[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// LinkTargetPath is where a symbolic link actually points.
//
// ⛔ AN ABSOLUTE TARGET STANDS ALONE, and joining one onto the link's own
// directory is what this exists to stop. filepath.Join("/work", "/etc/passwd")
// is "/work/etc/passwd", which is inside the workspace by every containment
// test there is, so a link that pointed out of the tree was PACKED and the
// refusal the manual promises never happened. Found by the ubuntu CI job on
// 2026-09-09: the case that covers it cannot run on Windows, where making a
// symlink needs a privilege this process may not have.
func LinkTargetPath(linkPath, target string) string {
	if isRootedTarget(target) {
		return target
	}
	return filepath.Join(filepath.Dir(linkPath), target)
}

// isRootedTarget reports whether a link target names a place on its own.
//
// ⚠ filepath.IsAbs IS NOT THE TEST ON WINDOWS. There, a leading separator
// with no drive is DRIVE-RELATIVE rather than absolute, so IsAbs answers false
// for a target that still names a place outside this tree, and joining it on
// buries it under the link's own directory where every containment test says
// it is inside. Anything that looks rooted is left alone and judged on its
// own; the containment test is what decides, not this.
func isRootedTarget(target string) bool {
	if target == "" {
		return false
	}
	if target[0] == '/' || target[0] == '\\' {
		return true
	}
	return filepath.IsAbs(target) || isWindowsAbsolute(target)
}

func matchesAny(rel string, patterns []string) bool {
	for _, pat := range patterns {
		if pat == "" {
			continue
		}
		if ok, _ := path.Match(pat, rel); ok {
			return true
		}
		if ok, _ := path.Match(pat, path.Base(rel)); ok {
			return true
		}
		if strings.HasPrefix(rel+"/", strings.TrimSuffix(pat, "/")+"/") {
			return true
		}
	}
	return false
}

// FetchArtifacts copies a guest directory back to the host.
//
// ⛔ The archive comes from inside a container's reach, so every entry is
// validated before it is written. The four refusals are each an escape:
//
//	an absolute name           writes wherever it says
//	a name with .. in it       climbs out of the destination
//	a symlink leaving the tree the next entry writes THROUGH it, outside
//	a Windows device name      a file called NUL swallows what is written to it
//
// ArtifactTransfer is what one fetch produced.
//
// ⛔ ATTEMPTED AND DELIVERED ARE TWO NUMBERS, and reporting one of them under
// the other's name is WSL-46, issue 24. The count was incremented as each entry
// was READ, so a transfer that refused its third entry answered "3 artifacts"
// beside the failure that stopped it, and a caller reading the number believed
// three files had arrived. Delivered is incremented only once an entry exists
// at the destination.
type ArtifactTransfer struct {
	Attempted int   `json:"attempted"`
	Delivered int   `json:"delivered"`
	Bytes     int64 `json:"bytes"`
}

func (w *Wsl) FetchArtifacts(ctx context.Context, distro, user, guestDir, hostDir string, limits WorkspaceLimits, log func(string)) (ArtifactTransfer, error) {
	var zero ArtifactTransfer
	if err := AssertArgvSafe([]string{guestDir}); err != nil {
		return zero, err
	}
	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		return zero, err
	}
	realDest, err := resolveExisting(hostDir)
	if err != nil {
		return zero, err
	}

	pr, pw := io.Pipe()
	type result struct {
		got ArtifactTransfer
		err error
	}
	done := make(chan result, 1)
	go func() {
		got, err := extractInto(pr, realDest, limits)
		// Drain the rest so the guest's tar is never blocked writing into a
		// pipe nobody reads, which would hang the run rather than end it.
		_, _ = io.Copy(io.Discard, pr)
		_ = pr.CloseWithError(err)
		done <- result{got, err}
	}()

	errBuf := &boundedBuffer{max: 64 << 10}
	code, execErr := w.ExecDirect(ctx, distro, user, "", []string{"/bin/tar", "-cf", "-", "-C", guestDir, "."}, nil, pw, errBuf, 60*time.Minute)
	_ = pw.Close()
	res := <-done
	if res.err != nil {
		return res.got, res.err
	}
	if execErr != nil || code != 0 {
		return res.got, fmt.Errorf("packing %s in the guest exited %d: %s", guestDir, code, firstLine(errBuf.String()))
	}
	if log != nil {
		log(fmt.Sprintf("artifacts: %d entries, %s written to %s", res.got.Delivered, HumanBytes(res.got.Bytes), hostDir))
	}
	return res.got, nil
}

var windowsDeviceNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// windowsBadChars cannot appear in an NTFS file name. ⛔ EVERY ONE OF THEM IS
// LEGAL ON LINUX, which is the whole difficulty: a container is free to write
// `report<1>.txt`, and the host this tool delivers to cannot hold that name. The
// colon is the one that lost data rather than failing: `normal.txt:stream` is
// valid NTFS syntax naming an ALTERNATE DATA STREAM, so the create succeeded, a
// zero-byte `normal.txt` appeared, and the payload went into a stream the caller
// never asked for and does not see.
const windowsBadChars = `<>:"|?*`

// checkWindowsComponent applies the destination's own name grammar to one path
// component.
//
// ⛔ IT RUNS ON EVERY HOST, not only on Windows. The artifacts are for a Windows
// caller whatever machine validated them, and a rule that fired only on Windows
// is a rule the Linux job in CI could never test. SafeArchiveName already splits
// on both separators for exactly this reason.
func checkWindowsComponent(name, part string) error {
	if i := strings.IndexAny(part, windowsBadChars); i >= 0 {
		if part[i] == ':' {
			return fmt.Errorf("%w: %q names an NTFS alternate data stream. The file would be created empty and the bytes would go into a stream nothing reads", ErrWorkspaceRefused, name)
		}
		return fmt.Errorf("%w: %q contains %q, which cannot appear in a name on the host this is delivered to", ErrWorkspaceRefused, name, string(part[i]))
	}
	for _, r := range part {
		if r < 0x20 {
			return fmt.Errorf("%w: %q contains a control character (0x%02x), which cannot appear in a name on the host this is delivered to", ErrWorkspaceRefused, name, r)
		}
	}
	// ⚠ Windows STRIPS a trailing dot or space rather than refusing it, so
	// `report.` and `report` are one file and the second silently replaces the
	// first. Refusing is the only answer that does not lose one of them.
	if last := part[len(part)-1]; last == '.' || last == ' ' {
		return fmt.Errorf("%w: %q ends in %q, which the destination strips, so it would collide with the same name without it", ErrWorkspaceRefused, name, string(last))
	}
	stem := strings.ToLower(part)
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	if windowsDeviceNames[stem] {
		return fmt.Errorf("%w: %q names the Windows device %q, and writing to one discards what is written and reports success", ErrWorkspaceRefused, name, stem)
	}
	return nil
}

// destinationKey is how two archive entries are judged to name one file.
//
// ⚠ IT IS AN APPROXIMATION OF NTFS'S OWN TABLE and errs toward refusing.
// NTFS compares with an uppercase table fixed when the volume was created, which
// this cannot read; lowercasing agrees with it for every name a build produces
// and disagrees only where it would refuse a pair the volume would have kept
// apart.
func destinationKey(rel string) string {
	return strings.ToLower(filepath.ToSlash(rel))
}

// SafeArchiveName validates one archive entry name and returns the relative
// path it may be written to.
//
// ⚠ It splits on BOTH separators itself. A path helper splits on the running
// platform's, so on Linux `logs\CON.jsonl` has a base name of `logs\CON` and
// the device rule misses it. The rule is about Windows semantics whatever host
// is checking.
func SafeArchiveName(name string) (string, error) {
	clean := strings.TrimPrefix(name, "./")
	if clean == "" || clean == "." {
		return "", nil
	}
	if strings.ContainsRune(clean, 0) {
		return "", fmt.Errorf("%w: %q carries a NUL byte", ErrWorkspaceRefused, name)
	}
	if strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, `\`) {
		return "", fmt.Errorf("%w: %q is an absolute path", ErrWorkspaceRefused, name)
	}
	if len(clean) >= 2 && clean[1] == ':' {
		return "", fmt.Errorf("%w: %q names a drive", ErrWorkspaceRefused, name)
	}
	parts := strings.FieldsFunc(clean, func(r rune) bool { return r == '/' || r == '\\' })
	var kept []string
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("%w: %q climbs out of the destination", ErrWorkspaceRefused, name)
		}
		if err := checkWindowsComponent(name, part); err != nil {
			return "", err
		}
		kept = append(kept, part)
	}
	if len(kept) == 0 {
		return "", nil
	}
	return filepath.Join(kept...), nil
}

func extractInto(r io.Reader, dest string, limits WorkspaceLimits) (ArtifactTransfer, error) {
	tr := tar.NewReader(r)
	var got ArtifactTransfer
	// ⛔ THE SET IS WHY TWO NAMES CANNOT BECOME ONE FILE. The per-name grammar
	// above cannot see a collision, because a collision is a property of a PAIR.
	// A container writing /out/Result and /out/result returned one five-byte file
	// on NTFS and exit 0, because the second open with O_TRUNC replaced the first.
	written := map[string]string{}
	claim := func(rel, name string) error {
		key := destinationKey(rel)
		if first, ok := written[key]; ok {
			return fmt.Errorf("%w: %q and %q name one file on the host this is delivered to, so writing the second would discard the first", ErrWorkspaceRefused, first, name)
		}
		written[key] = name
		return nil
	}
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return got, nil
		}
		if err != nil {
			return got, err
		}
		rel, err := SafeArchiveName(hdr.Name)
		if err != nil {
			return got, err
		}
		if rel == "" {
			continue
		}
		target := filepath.Join(dest, rel)
		// Two guards over one property: the first is a string rule, the second
		// asks the filesystem. A link written by an earlier entry is only
		// visible to the second.
		if _, err := ResolveInside(dest, target); err != nil {
			return got, fmt.Errorf("%w: %s", ErrWorkspaceRefused, err)
		}
		got.Attempted++
		if got.Attempted > limits.MaxEntries {
			return got, fmt.Errorf("%w: the guest returned more than %d entries", ErrWorkspaceRefused, limits.MaxEntries)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return got, err
			}
			got.Delivered++
		case tar.TypeReg:
			if err := claim(rel, hdr.Name); err != nil {
				return got, err
			}
			got.Bytes += hdr.Size
			if got.Bytes > limits.MaxBytes {
				return got, fmt.Errorf("%w: the guest returned more than %s", ErrWorkspaceRefused, HumanBytes(limits.MaxBytes))
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return got, err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				return got, err
			}
			written, err := io.Copy(f, tr)
			closeErr := f.Close()
			if err != nil {
				return got, err
			}
			if closeErr != nil {
				return got, closeErr
			}
			if written != hdr.Size {
				return got, fmt.Errorf("%w: %s declared %d bytes and %d arrived", ErrWorkspaceRefused, rel, hdr.Size, written)
			}
			got.Delivered++
		case tar.TypeSymlink, tar.TypeLink:
			// ⛔ A LINK THAT LEAVES THE TREE IS REFUSED, which is what the
			// manual has always promised and the code did not do. It was
			// silently converted to an inert `x.link.txt` and the job exited 0,
			// so a caller could not tell its deliverables were incomplete.
			// WSL-47, issue 21.
			//
			// ⚠ NOT A TRAVERSAL HOLE, and the reporter said so plainly: nothing
			// escaped and nothing was overwritten. The defect is that the manual
			// promises a refusal and the binary performed a transformation
			// nobody was told about, which is the same class as a truncation
			// nobody is told about.
			//
			// ⭐ THE ASYMMETRY WITH A WORKSPACE IS DELIBERATE. A caller CHOOSES
			// what it puts in /out, so a refusal here is actionable; a caller
			// often does not control every entry under a workspace it points at,
			// so an omission there is counted and named instead.
			if err := assertLinkStaysInside(dest, rel, hdr.Linkname); err != nil {
				return got, err
			}
			// ⛔ Not recreated even when it stays inside. The entry after a link
			// writes THROUGH it. Recording it keeps the information and removes
			// the mechanism.
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return got, err
			}
			// ⚠ The sidecar's name is an entry name too, and a real file called
			// x.link.txt would otherwise be overwritten by the record of a link
			// called x. It goes through the same claim.
			if err := claim(rel+".link.txt", hdr.Name+".link.txt"); err != nil {
				return got, err
			}
			note := fmt.Sprintf("wsl-toolkit: the guest returned a link here, pointing at %q. "+
				"Links are recorded rather than recreated, because the entry after one writes through it.\n", hdr.Linkname)
			if err := os.WriteFile(target+".link.txt", []byte(note), 0o600); err != nil {
				return got, err
			}
			got.Delivered++
		default:
			// A device, a socket or a fifo is not an artifact.
			continue
		}
	}
}

// SortedExcludes is what a caller's exclude flags become, deduplicated so a
// report of what was skipped does not repeat itself.
func SortedExcludes(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range in {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			out = append(out, part)
		}
	}
	sort.Strings(out)
	return out
}

// RepairGuestScript makes a host-authored script runnable by /bin/sh, in the
// COPY being sent and never in the file on disk. The file is the caller's; the
// copy in transit is this tool's to make correct.
//
//	CRLF endings        turned into LF in the copy
//	a UTF-8 mark        left out of the copy
//	UTF-16              ⛔ refused by name. Its bytes carry a NUL after nearly
//	                    every character and /bin/sh stops at the first one, so
//	                    the command would do nothing and say nothing
//	a lone carriage     ⚠ kept. It is a deliberate byte, and turning it into a
//	return              newline would edit the payload rather than repair it
func RepairGuestScript(raw []byte) ([]byte, error) {
	if len(raw) >= 2 {
		if (raw[0] == 0xFF && raw[1] == 0xFE) || (raw[0] == 0xFE && raw[1] == 0xFF) {
			return nil, fmt.Errorf("%w: the script is UTF-16. /bin/sh stops at its first NUL byte, so it would do nothing and say nothing", ErrWorkspaceRefused)
		}
	}
	if len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF {
		raw = raw[3:]
	}
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\r' && i+1 < len(raw) && raw[i+1] == '\n' {
			continue
		}
		out = append(out, raw[i])
	}
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	return out, nil
}
