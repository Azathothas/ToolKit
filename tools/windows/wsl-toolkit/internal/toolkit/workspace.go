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
func (w *Wsl) SendWorkspace(ctx context.Context, distro, user, guestDir, hostDir string, limits WorkspaceLimits, excludes []string, log func(string)) (int, int64, error) {
	if err := AssertArgvSafe([]string{guestDir}); err != nil {
		return 0, 0, err
	}
	info, err := os.Stat(hostDir)
	if err != nil {
		return 0, 0, fmt.Errorf("the workspace to copy: %w", err)
	}
	if !info.IsDir() {
		return 0, 0, fmt.Errorf("%s is not a directory", hostDir)
	}

	errBuf := &boundedBuffer{max: 64 << 10}
	if code, err := w.ExecDirect(ctx, distro, user, "", []string{"/bin/mkdir", "-p", guestDir}, nil, io.Discard, errBuf, 2*time.Minute); err != nil || code != 0 {
		return 0, 0, fmt.Errorf("could not create %s in the guest (exit %d): %s", guestDir, code, firstLine(errBuf.String()))
	}

	pr, pw := io.Pipe()
	type result struct {
		entries int
		bytes   int64
		err     error
	}
	done := make(chan result, 1)
	go func() {
		entries, total, err := writeWorkspaceTar(pw, hostDir, limits, excludes)
		// ⛔ CloseWithError, not Close. A writer that stopped at a limit must
		// make the READER fail too, or the guest unpacks a truncated archive
		// without complaint.
		_ = pw.CloseWithError(err)
		done <- result{entries, total, err}
	}()

	errBuf = &boundedBuffer{max: 64 << 10}
	code, execErr := w.ExecDirect(ctx, distro, user, "", []string{"/bin/tar", "-xf", "-", "-C", guestDir}, pr, io.Discard, errBuf, 60*time.Minute)
	res := <-done
	if res.err != nil {
		return res.entries, res.bytes, res.err
	}
	if execErr != nil || code != 0 {
		return res.entries, res.bytes, fmt.Errorf("unpacking the workspace in the guest exited %d: %s", code, firstLine(errBuf.String()))
	}
	if log != nil {
		log(fmt.Sprintf("workspace: %d entries, %s copied to %s", res.entries, HumanBytes(res.bytes), guestDir))
	}
	return res.entries, res.bytes, nil
}

func writeWorkspaceTar(w io.Writer, root string, limits WorkspaceLimits, excludes []string) (int, int64, error) {
	tw := tar.NewWriter(w)
	realRoot, err := resolveExisting(root)
	if err != nil {
		return 0, 0, err
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
			// ⛔ A link out of the workspace is the hole this file removes,
			// arriving by another route. Refused rather than skipped: a
			// workspace silently missing one fails for an invisible reason.
			resolved, err := resolveExisting(LinkTargetPath(p, target))
			if err != nil {
				return err
			}
			if !hasPathPrefix(resolved, realRoot) && !pathEqual(resolved, realRoot) {
				return fmt.Errorf("%w: %s links to %s, which is outside the workspace", ErrWorkspaceRefused, slashRel, target)
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
			mode := int64(0o644)
			if info.Mode()&0o111 != 0 {
				mode = 0o755
			}
			if err := tw.WriteHeader(&tar.Header{
				Name: slashRel, Typeflag: tar.TypeReg, Mode: mode, Size: info.Size(), ModTime: info.ModTime(),
			}); err != nil {
				return err
			}
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			written, err := io.Copy(tw, f)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			// ⛔ Count what arrived rather than trusting the declared length. A
			// file that changed between the stat and the read leaves a header
			// disagreeing with its payload.
			if written != info.Size() {
				return fmt.Errorf("%w: %s changed while it was being read (%d of %d bytes)", ErrWorkspaceRefused, slashRel, written, info.Size())
			}
			return nil
		default:
			// A socket, a device or a named pipe is not workspace content, and
			// carrying one into a container would be handing it a channel.
			return nil
		}
	})
	if walkErr != nil {
		return entries, total, walkErr
	}
	if err := tw.Close(); err != nil {
		return entries, total, err
	}
	return entries, total, nil
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
			// ⛔ Not recreated. The entry after a link writes THROUGH it.
			// Recording it keeps the information and removes the mechanism.
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
