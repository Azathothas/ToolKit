// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// `base sync` copies a Windows checkout into the base's own filesystem and, on
// the next call, sends only what changed. WSL-125.
//
// ⭐ WHY IT EXISTS. A grant is a 9P mount, and a build reading a tree over 9P is
// slow: a consumer measured minutes there for what takes about a second on the
// base's own disk, and copied its checkout across by hand to get it.
//
// ⭐ THE LIST IS THE WORKSPACE COPY'S. walkWorkspace is the one walk, so what a
// job's /work holds and what a sync sends are chosen by one reader of the same
// excludes, git's ignore rules and the git index's executable bits.

// SyncSchema versions `base sync --json`.
const SyncSchema = "wsl-toolkit-base-sync/1"

// ErrSyncIncomplete is a sync that changed the guest and did not finish. Every
// refusal before it is a sync that changed nothing.
var ErrSyncIncomplete = errors.New("the sync did not finish")

// SyncSpec is one `base sync`.
type SyncSpec struct {
	Distro, User string
	// HostDir is an absolute directory on this machine, and GuestDir the guest
	// path as given: absolute, ~, or under ~/.
	HostDir, GuestDir string
	Filter            WorkspaceFilter
	Limits            WorkspaceLimits
	// Delete removes guest entries no longer in the list, and Force lets
	// GuestDir be outside the account's home.
	Delete, Force bool
	Log           func(string)
}

// SyncReport is what one sync did.
type SyncReport struct {
	Schema   string `json:"schema"`
	HostDir  string `json:"host_dir"`
	GuestDir string `json:"guest_dir"`
	// Entries and Bytes are the list: every directory, file and link the copy
	// keeps, and the bytes of its files.
	Entries int   `json:"entries"`
	Bytes   int64 `json:"bytes"`
	// Sent is what travelled because it was new or changed, and SentBytes its
	// bytes. Unchanged is what the guest already held as the list has it.
	Sent      int   `json:"sent"`
	SentBytes int64 `json:"sent_bytes"`
	Unchanged int   `json:"unchanged"`
	// Deleted counts the guest entries --delete removed, Replaced those removed
	// because the list has another kind of entry there, and Kept those the guest
	// holds that the list does not, left in place.
	Deleted  int  `json:"deleted"`
	Replaced int  `json:"replaced"`
	Kept     int  `json:"kept"`
	Delete   bool `json:"delete"`
	// Created says the guest directory did not exist before this sync.
	Created      bool                `json:"created"`
	Ignored      int                 `json:"ignored,omitempty"`
	Omitted      int                 `json:"omitted,omitempty"`
	Omission     []WorkspaceOmission `json:"omission,omitempty"`
	ExecRestored string              `json:"exec_restored,omitempty"`
}

// syncTarget is the guest directory a sync writes, from what the caller gave and
// the account's home, or the refusal.
//
// ⛔ THE HOME ITSELF, THE ROOT AND THIS TOOL'S OWN DIRECTORY ARE REFUSED,
// --force or not. With --delete, a sync into the home removes every file the list
// does not hold, the account's dotfiles among them, and one into
// ~/.wsl-toolkit removes the jobs and sessions running there.
func syncTarget(given, home string, force bool) (string, error) {
	g := strings.TrimSpace(given)
	switch {
	case g == "~":
		g = home
	case strings.HasPrefix(g, "~/"):
		g = home + g[1:]
	case !strings.HasPrefix(g, "/"):
		return "", fmt.Errorf("GUEST-DIR %q is not a guest path. Pass an absolute path, or one under ~/", given)
	}
	for _, part := range strings.Split(g, "/") {
		if part == ".." {
			return "", fmt.Errorf("GUEST-DIR %q climbs with .., so where it lands is not what it says", given)
		}
	}
	clean := path.Clean(g)
	own := home + "/" + GuestRoot
	switch {
	case clean == "/":
		return "", errors.New("GUEST-DIR is the guest's root, and a sync writes a directory of its own")
	case clean == home:
		return "", fmt.Errorf("GUEST-DIR is %s, the account's home itself. A sync writes a directory of its own under it, such as ~/src/project", home)
	case clean == own || strings.HasPrefix(clean, own+"/"):
		return "", fmt.Errorf("GUEST-DIR %s is inside %s, where this tool keeps its jobs and sessions", clean, own)
	case !force && !strings.HasPrefix(clean, home+"/"):
		return "", fmt.Errorf("GUEST-DIR %s is outside the account's home %s. Pass --force to write there", clean, home)
	}
	if err := AssertArgvSafe([]string{clean}); err != nil {
		return "", err
	}
	return clean, nil
}

// windowsMountTypes are the filesystem types a Windows directory reaches the
// guest through. A sync never writes one.
var windowsMountTypes = map[string]bool{"9p": true, "v9fs": true, "drvfs": true, "virtiofs": true}

// syncSurveyScript reads, for one guest directory: the filesystem of the nearest
// directory that exists on its path, read through links; the account's home
// read the same way; whether the directory exists; and, where it does, every
// entry under it with its mode, size, modification time and a link's target.
//
// ⭐ ONE CALL, and a line per entry, so the cost of a sync with nothing to send
// is one walk of the guest's own disk.
func syncSurveyScript(guest string) []byte {
	return []byte("d=" + shellQuote(guest) + "\n" + `p=$d
while [ ! -d "$p" ]; do p=${p%/*}; [ -n "$p" ] || p=/; done
real=$(cd "$p" 2>/dev/null && pwd -P) || { echo "wsl-toolkit: $p cannot be entered" >&2; exit 3; }
home=$(cd "$HOME" 2>/dev/null && pwd -P) || home=$HOME
best=; fs=
while read -r _dev mnt type _rest; do
  case "$mnt" in /) m= ;; *) m=$mnt ;; esac
  case "$real/" in "$m"/*) if [ "${#m}" -ge "${#best}" ]; then best=$m; fs=$type; fi ;; esac
done < /proc/mounts
printf 'REAL|%s\nHOME|%s\nFS|%s\n' "$real" "$home" "$fs"
if [ -L "$d" ]; then printf 'STATE|link\n'; exit 0; fi
if [ ! -e "$d" ]; then printf 'STATE|missing\n'; exit 0; fi
if [ ! -d "$d" ]; then printf 'STATE|notdir\n'; exit 0; fi
printf 'STATE|dir\n'
case "$fs" in 9p|v9fs|drvfs|virtiofs) exit 0 ;; esac
cd "$d" || exit 3
find . -mindepth 1 -exec stat -c 'E|%f|%s|%Y|%n' {} + || exit 3
find . -mindepth 1 -type l -exec sh -c 'for l; do printf "L|%s\nT|%s\n" "$l" "$(readlink "$l")"; done' sh {} + || exit 3
printf 'END\n'
`)
}

// guestEntry is one entry the guest holds under a sync's directory.
type guestEntry struct {
	mode  uint32
	size  int64
	mtime int64
	link  string
}

func (g guestEntry) isDir() bool  { return g.mode&0o170000 == 0o040000 }
func (g guestEntry) isReg() bool  { return g.mode&0o170000 == 0o100000 }
func (g guestEntry) isLink() bool { return g.mode&0o170000 == 0o120000 }

// syncSurvey is what syncSurveyScript answered.
type syncSurvey struct {
	real, home, fs, state string
	entries               map[string]guestEntry
	ended                 bool
}

// parseSyncSurvey reads the survey's lines. ⭐ A function with no process in it,
// so its case runs on any host. A line that does not read is left out: a guest
// name with a newline in it is not one the host can hold, and leaving it unread
// means it is neither compared nor removed.
func parseSyncSurvey(out []byte) syncSurvey {
	s := syncSurvey{entries: map[string]guestEntry{}}
	// link is the entry the next T line names the target of.
	link := ""
	for _, raw := range strings.Split(string(out), "\n") {
		line := strings.TrimRight(raw, "\r")
		key, rest, ok := strings.Cut(line, "|")
		if !ok {
			if line == "END" {
				s.ended = true
			}
			continue
		}
		if key != "T" {
			link = ""
		}
		switch key {
		case "REAL":
			s.real = rest
		case "HOME":
			s.home = rest
		case "FS":
			s.fs = rest
		case "STATE":
			s.state = rest
		case "E":
			f := strings.SplitN(rest, "|", 4)
			if len(f) != 4 || !strings.HasPrefix(f[3], "./") {
				continue
			}
			mode, err1 := strconv.ParseUint(f[0], 16, 32)
			size, err2 := strconv.ParseInt(f[1], 10, 64)
			mtime, err3 := strconv.ParseInt(f[2], 10, 64)
			if err1 != nil || err2 != nil || err3 != nil {
				continue
			}
			rel := strings.TrimPrefix(f[3], "./")
			e := s.entries[rel]
			e.mode, e.size, e.mtime = uint32(mode), size, mtime
			s.entries[rel] = e
		case "L":
			if strings.HasPrefix(rest, "./") {
				link = strings.TrimPrefix(rest, "./")
			}
		case "T":
			if e, ok := s.entries[link]; ok && link != "" {
				e.link = rest
				s.entries[link] = e
			}
			link = ""
		}
	}
	return s
}

// syncPlan is what a sync does: entries to send, guest paths to remove first,
// and the counts the report carries.
type syncPlan struct {
	send []workspaceEntry
	// remove are guest paths removed before the archive is unpacked, outermost
	// first: entries --delete takes away, and entries the list replaces with
	// another kind.
	remove []string
	// conflicts are guest entries of another kind than the list's that only
	// --delete may remove, because one is a directory.
	conflicts                          []string
	unchanged, deleted, replaced, kept int
	sentBytes                          int64
}

// hostMTime is a file's modification time as the archive carries it, which is
// what the guest's copy holds once unpacked.
//
// ⚠ tar.Writer ROUNDS TO THE NEAREST SECOND when no format is named, so the host's
// own time compared with the guest's would differ in every file and send them all.
func hostMTime(info os.FileInfo) int64 {
	return info.ModTime().Round(time.Second).Unix()
}

// planSync decides what one sync sends and removes.
//
// ⭐ A FILE TRAVELS WHEN ITS SIZE, ITS MODIFICATION TIME OR ITS EXECUTABLE BIT
// DIFFERS, a link when its target does, and a directory when the guest lacks it.
//
// ⛔ --delete LEAVES WHAT THE FILTER LEAVES OUT. A path an --exclude names or git
// ignores is not in the list because it was left out, not because it is gone, so
// a build tree made in the guest survives the next sync. keep answers for those.
func planSync(list []workspaceEntry, guest map[string]guestEntry, del bool, keep func(rel string, dir bool) bool) syncPlan {
	var p syncPlan
	inList := make(map[string]bool, len(list))
	for _, e := range list {
		inList[e.rel] = true
	}
	removed := map[string]bool{}
	for _, e := range list {
		g, held := guest[e.rel]
		switch {
		case e.info.IsDir():
			switch {
			case !held:
				p.send = append(p.send, e)
			case g.isDir():
				p.unchanged++
			default:
				// A file or a link where the list has a directory.
				if del {
					p.remove = append(p.remove, e.rel)
					removed[e.rel] = true
					p.replaced++
					p.send = append(p.send, e)
				} else {
					p.conflicts = append(p.conflicts, e.rel)
				}
			}
		case e.info.Mode()&os.ModeSymlink != 0:
			switch {
			case !held:
				p.send = append(p.send, e)
			case g.isLink() && g.link == filepath.ToSlash(e.link):
				p.unchanged++
			case g.isDir() && !del:
				p.conflicts = append(p.conflicts, e.rel)
			default:
				p.remove = append(p.remove, e.rel)
				removed[e.rel] = true
				p.replaced++
				p.send = append(p.send, e)
			}
		default:
			switch {
			case !held:
				p.send = append(p.send, e)
				p.sentBytes += e.info.Size()
			case g.isReg() && g.size == e.info.Size() && g.mtime == hostMTime(e.info) && (g.mode&0o111 != 0) == (e.mode&0o111 != 0):
				p.unchanged++
			case g.isReg():
				p.send = append(p.send, e)
				p.sentBytes += e.info.Size()
			case g.isDir() && !del:
				p.conflicts = append(p.conflicts, e.rel)
			default:
				p.remove = append(p.remove, e.rel)
				removed[e.rel] = true
				p.replaced++
				p.send = append(p.send, e)
				p.sentBytes += e.info.Size()
			}
		}
	}
	// Guest entries the list does not hold, outermost first so a directory that
	// goes takes what is under it.
	var extra []string
	for rel := range guest {
		if !inList[rel] {
			extra = append(extra, rel)
		}
	}
	sort.Strings(extra)
	stays := map[string]bool{}
	under := func(rel string, set map[string]bool) bool {
		for dir := path.Dir(rel); dir != "."; dir = path.Dir(dir) {
			if set[dir] {
				return true
			}
		}
		return false
	}
	// ⚠ ONLY AN ENTRY WHOSE PARENT IS IN THE LIST IS DECIDED. Sorted, a parent
	// comes before what is under it, and what is under it shares its fate: it goes
	// with a directory that goes and stays with one that stays.
	for _, rel := range extra {
		switch {
		case under(rel, removed):
			p.deleted++
		case under(rel, stays):
			p.kept++
		case !del || keep(rel, guest[rel].isDir()):
			p.kept++
			stays[rel] = true
		default:
			p.remove = append(p.remove, rel)
			removed[rel] = true
			p.deleted++
		}
	}
	return p
}

// syncRemoveScript removes guest paths under a sync's directory, and names each
// one it could not.
func syncRemoveScript(guest string, rels []string) []byte {
	var b strings.Builder
	b.WriteString("cd " + shellQuote(guest) + " || exit 3\n")
	for _, rel := range rels {
		q := shellQuote("./" + rel)
		b.WriteString("rm -rf -- " + q + " 2>/dev/null; if [ -e " + q + " ] || [ -L " + q + " ]; then printf 'FAILED|%s\\n' " + q + "; fi\n")
	}
	b.WriteString("printf 'REMOVED\\n'\n")
	return []byte(b.String())
}

// gitKeeps answers which guest paths git's own ignore rules leave out, asked of
// the repository each path is in: the checkout's, or a repository nested in it.
//
// ⭐ GIT DECIDES, for the reason gitIgnored gives, and it is asked about paths the
// host may not have: a build tree made in the guest exists nowhere else. A
// directory is asked about with a trailing slash, which is how a rule written for
// a directory matches a path git cannot stat.
func gitKeeps(ctx context.Context, root string, nested []string, rels map[string]bool) (map[string]bool, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, errors.New("--gitignore reads git's own rules, and no git is on PATH")
	}
	// The deepest repository holding each path answers for it.
	sort.Slice(nested, func(i, j int) bool { return len(nested[i]) > len(nested[j]) })
	byRepo := map[string][]string{}
	for rel := range rels {
		repo := ""
		for _, n := range nested {
			if strings.HasPrefix(rel, n+"/") {
				repo = n
				break
			}
		}
		byRepo[repo] = append(byRepo[repo], rel)
	}
	kept := map[string]bool{}
	for repo, paths := range byRepo {
		dir := root
		if repo != "" {
			dir = filepath.Join(root, filepath.FromSlash(repo))
		}
		sort.Strings(paths)
		var in bytes.Buffer
		asked := map[string]string{}
		for _, p := range paths {
			q := strings.TrimPrefix(strings.TrimPrefix(p, repo), "/")
			if rels[p] {
				q += "/"
			}
			asked[q] = p
			in.WriteString(q)
			in.WriteByte(0)
		}
		bounded, cancel := context.WithTimeout(ctx, gitIndexTimeout)
		cmd := exec.CommandContext(bounded, git, gitArgs(dir, "check-ignore", "-z", "--stdin")...)
		cmd.Stdin = &in
		var out, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &stderr
		runErr := cmd.Run()
		cancel()
		// ⚠ EXIT 1 IS "NONE OF THEM", which is an answer and not a failure.
		var ee *exec.ExitError
		if runErr != nil && !(errors.As(runErr, &ee) && ee.ExitCode() == 1) {
			return nil, fmt.Errorf("git check-ignore could not say which guest paths git ignores in %s: %w: %s", dir, runErr, firstLine(stderr.String()))
		}
		for _, rec := range strings.Split(out.String(), "\x00") {
			if p, ok := asked[rec]; ok {
				kept[p] = true
			}
		}
	}
	return kept, nil
}

// SyncBase copies HostDir into GuestDir in the base, sending only what changed.
func SyncBase(ctx context.Context, w *Wsl, s SyncSpec) (SyncReport, error) {
	rep := SyncReport{Schema: SyncSchema, HostDir: s.HostDir, Delete: s.Delete}
	if s.Log == nil {
		s.Log = func(string) {}
	}
	if s.Limits.MaxBytes == 0 {
		s.Limits = DefaultWorkspaceLimits()
	}
	home, err := accountHome(ctx, w, s.Distro, s.User)
	if err != nil {
		return rep, err
	}
	guest, err := syncTarget(s.GuestDir, home, s.Force)
	if err != nil {
		return rep, err
	}
	rep.GuestDir = guest

	// The list first: a tree past a limit is refused before the guest is touched.
	var up WorkspaceUpload
	var list []workspaceEntry
	var nested []string
	if err := walkWorkspace(s.HostDir, s.Limits, s.Filter, &up, func(e workspaceEntry) error {
		list = append(list, e)
		if e.info.IsDir() {
			if _, err := os.Lstat(filepath.Join(e.abs, ".git")); err == nil {
				nested = append(nested, e.rel)
			}
		}
		return nil
	}); err != nil {
		return rep, err
	}
	rep.Entries, rep.Bytes, rep.Ignored, rep.Omitted, rep.Omission, rep.ExecRestored =
		up.Entries, up.Bytes, up.Ignored, up.Omitted, up.Omission, up.ExecRestored

	out := &boundedBuffer{max: 256 << 20}
	errBuf := &boundedBuffer{max: 64 << 10}
	code, runErr := w.Exec(ctx, ExecRequest{Distro: s.Distro, User: s.User, Script: syncSurveyScript(guest), Stdout: out, Stderr: errBuf, Timeout: 30 * time.Minute})
	if runErr != nil || code != 0 {
		return rep, fmt.Errorf("could not read %s in the guest (exit %d): %s", guest, code, guestFailure(errBuf.String(), out.String()))
	}
	if out.Truncated() {
		return rep, fmt.Errorf("the guest's list of %s passed %s, which is more than a sync reads. Sync into a directory of its own", guest, HumanBytes(256<<20))
	}
	sv := parseSyncSurvey([]byte(out.String()))
	switch {
	case sv.real == "" || sv.state == "":
		return rep, fmt.Errorf("the guest gave no answer for %s", guest)
	case windowsMountTypes[sv.fs]:
		return rep, fmt.Errorf("GUEST-DIR %s is on a Windows directory (%s, through %s), and a sync writes the base's own disk. Writing there would write the Windows files it reads", guest, sv.fs, sv.real)
	case !s.Force && sv.real != sv.home && !strings.HasPrefix(sv.real+"/", sv.home+"/"):
		return rep, fmt.Errorf("GUEST-DIR %s leads through a link to %s, which is outside the account's home %s. Pass --force to write there", guest, sv.real, sv.home)
	case sv.state == "link":
		return rep, fmt.Errorf("GUEST-DIR %s is a link. Name the directory it points at", guest)
	case sv.state == "notdir":
		return rep, fmt.Errorf("GUEST-DIR %s exists and is not a directory", guest)
	case sv.state == "dir" && !sv.ended:
		return rep, fmt.Errorf("the guest's list of %s stopped before its end: %s", guest, guestFailure(errBuf.String()))
	}
	rep.Created = sv.state == "missing"

	keep := func(rel string, dir bool) bool { return matchesAny(rel, s.Filter.Excludes) }
	if s.Delete && s.Filter.Gitignore {
		ask := map[string]bool{}
		for rel, g := range sv.entries {
			ask[rel] = g.isDir()
		}
		ignored, err := gitKeeps(ctx, s.HostDir, nested, onlyExtra(ask, list))
		if err != nil {
			return rep, err
		}
		keep = func(rel string, dir bool) bool { return ignored[rel] || matchesAny(rel, s.Filter.Excludes) }
	}
	plan := planSync(list, sv.entries, s.Delete, keep)
	if len(plan.conflicts) > 0 {
		shown := plan.conflicts
		if len(shown) > 5 {
			shown = shown[:5]
		}
		return rep, fmt.Errorf("%d path(s) are a directory on one side and not on the other, such as %s. A sync with --delete replaces them, and without it nothing was changed",
			len(plan.conflicts), strings.Join(shown, ", "))
	}
	if len(plan.remove) > 0 {
		out, errText, code, err := w.Capture(ctx, s.Distro, s.User, syncRemoveScript(guest, plan.remove), 30*time.Minute)
		if err != nil || code != 0 || !strings.Contains(out, "REMOVED") {
			return rep, fmt.Errorf("%w: could not remove what the list no longer holds in %s (exit %d): %s", ErrSyncIncomplete, guest, code, guestFailure(errText, out))
		}
		var failed []string
		for _, line := range strings.Split(out, "\n") {
			if p, ok := strings.CutPrefix(strings.TrimSpace(line), "FAILED|"); ok {
				failed = append(failed, p)
			}
		}
		if len(failed) > 0 {
			return rep, fmt.Errorf("%w: %d path(s) in %s could not be removed, such as %s", ErrSyncIncomplete, len(failed), guest, failed[0])
		}
	}
	rep.Deleted, rep.Replaced, rep.Kept, rep.Unchanged = plan.deleted, plan.replaced, plan.kept, plan.unchanged
	errBuf = &boundedBuffer{max: 64 << 10}
	if code, err := w.ExecDirect(ctx, s.Distro, s.User, "", []string{"/bin/mkdir", "-p", guest}, nil, io.Discard, errBuf, 2*time.Minute); err != nil || code != 0 {
		return rep, fmt.Errorf("%w: could not create %s in the guest (exit %d): %s", ErrSyncIncomplete, guest, code, guestFailure(errBuf.String()))
	}
	if len(plan.send) > 0 {
		errBuf = &boundedBuffer{max: 64 << 10}
		sent, err := pumpArchive(
			func(pw io.Writer) (WorkspaceUpload, error) {
				var u WorkspaceUpload
				tw := tar.NewWriter(pw)
				for _, e := range plan.send {
					if err := writeWorkspaceEntry(tw, &u, e); err != nil {
						return u, err
					}
				}
				if err := tw.Close(); err != nil {
					return u, fmt.Errorf("the sync could not finish its archive: %w", err)
				}
				return u, nil
			},
			func(pr io.Reader) (int, error) {
				return w.ExecDirect(ctx, s.Distro, s.User, "", []string{"/bin/tar", "-xf", "-", "-C", guest}, pr, io.Discard, errBuf, 60*time.Minute)
			},
			func() string { return errBuf.String() })
		if err != nil {
			return rep, fmt.Errorf("%w: %v", ErrSyncIncomplete, err)
		}
		for _, t := range sent.Truncation {
			s.Log("sync: " + t.Path + ": " + t.Reason)
		}
	}
	rep.Sent, rep.SentBytes = len(plan.send), plan.sentBytes
	return rep, nil
}

// onlyExtra is the guest paths the list does not hold whose parent it does, which
// is where --delete decides: a path under one of them goes with it.
func onlyExtra(guest map[string]bool, list []workspaceEntry) map[string]bool {
	inList := make(map[string]bool, len(list))
	for _, e := range list {
		inList[e.rel] = true
	}
	out := map[string]bool{}
	for rel, dir := range guest {
		if inList[rel] {
			continue
		}
		if parent := path.Dir(rel); parent == "." || inList[parent] {
			out[rel] = dir
		}
	}
	return out
}
