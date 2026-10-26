// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// ⛔ WHAT THIS TOOL OWNS, AND HOW IT PROVES IT.
//
// The guard used to be `AssertOwnedDistro(name, baseName)` with every caller
// passing `cfg.Base.Name` for both, so it compared a configured value against
// itself and proved only that a name equals itself. `base.name` is editable, so
// `base ensure` would create any syntactically valid distribution and
// `base remove --yes` would unregister it, while the manual taught an agent that
// every name other than `wsl-toolkit` was structurally refused. WSL-42,
// issues 16 and 18.
//
// Ownership is now TWO facts, and both are required:
//
//	the NAME matches the prefix        a structural rule, checkable without WSL
//	the GUEST carries an identity      a proof this tool wrote, not a config
//
// ⭐ The name rule alone is what the reporter asked for and it is not enough:
// it cannot tell a distribution this tool built from one somebody imported under
// the same name. The marker alone is not enough either: a mistyped name would
// then create a real distribution that could never be removed. Together they
// close both.

// OwnedPrefix is the name every distribution this tool may own starts with.
//
// ⚠ NO `eph-` PREFIX, deliberately: DefaultBaseName carries the reason.
//
// ⛔ IT IS THE OLDER DEFAULT NAME AND NOT THE CANONICAL ONE. `wsl-toolkit` is
// both the prefix and a base this tool may still own, and `wsl-toolkit-jobs` is
// owned as a name after the prefix. WSL-167.
const OwnedPrefix = LegacyBaseName

// IdentityPath is where the marker lives inside the guest.
//
// ⛔ Under /etc rather than in the account's home. A home directory is
// writable by the account jobs run as, so a marker there is one a payload could
// forge; /etc is root-owned and nothing this tool runs afterwards is root.
const IdentityPath = "/etc/wsl-toolkit-identity.json"

// IdentitySchema versions the marker. ⛔ A stored format with no version
// mis-reads silently the day its shape changes.
const IdentitySchema = "wsl-toolkit-identity/1"

// Identity is what a distribution this tool built carries.
type Identity struct {
	Schema string    `json:"schema"`
	Name   string    `json:"name"`
	Image  string    `json:"image"`
	User   string    `json:"user"`
	Built  time.Time `json:"built"`
	// Tool is the version that built it, so a base built by an older release is
	// recognisable rather than merely present.
	Tool string `json:"tool,omitempty"`
	// Digest is the digest of the image it was built from, as the host engine
	// named it at the build. ⭐ `base.image` set to the image at this digest builds
	// the same rootfs again after the tag moved. ⚠ Empty for a base an older
	// release built, and where the engine named none. WSL-186.
	Digest string `json:"digest,omitempty"`
}

// ErrNotOwned is what every ownership refusal wraps, so a caller can tell one
// from an ordinary failure without reading a message.
var ErrNotOwned = errors.New("not a distribution this tool owns")

// ErrNoIdentity says the distribution exists, its name is one this tool may
// own, and it carries no marker. ⭐ It is a SEPARATE error because it is the
// one case with an upgrade path: a base built before markers existed is
// adoptable, and one built by somebody else is not.
var ErrNoIdentity = errors.New("carries no wsl-toolkit identity marker")

// ErrIdentityUnread says the marker could not be read at all: the guest did not
// start, the read timed out, wsl.exe failed, or `cat` failed for a reason other
// than a missing file. ⛔ It is not ErrNoIdentity, because nothing was learned
// about the marker. WSL-167, WSL-183.
var ErrIdentityUnread = errors.New("the wsl-toolkit identity marker could not be read")

// ErrGuestUnstartable says WSL did not start the guest, or the read never came
// back, so `cat` never answered. An error carrying it carries ErrIdentityUnread
// too. ⛔ A corrupt disk, or one another program holds, is this, and its fix is
// not a stamp. WSL-183.
var ErrGuestUnstartable = errors.New("the guest could not be started")

// IsOwnedName is the structural half: `wsl-toolkit`, or `wsl-toolkit-<suffix>`.
//
// ⚠ It is deliberately not "has the prefix". `wsl-toolkitorama` has the prefix
// and is somebody else's distribution; the boundary is the whole name or a name
// followed by a hyphen.
//
// ⛔ IT IS CASE-INSENSITIVE AND ValidateInstanceName IS NOT, and the two answer
// different questions. This one asks "may this tool ACT on this distribution",
// and wsl.exe compares names case-insensitively, so a guard that was
// case-sensitive would refuse to touch the tool's own base the day something
// spelled it back differently. ValidateInstanceName asks "may this tool CREATE
// this name", where lower case is required because the distribution name is
// case-insensitive and the state directory named for it is not.
func IsOwnedName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == OwnedPrefix {
		return true
	}
	if !strings.HasPrefix(name, OwnedPrefix+"-") {
		return false
	}
	return isInstanceName(strings.TrimPrefix(name, OwnedPrefix+"-"))
}

// isInstanceName is the rule for the part after the prefix. It is a PATH
// COMPONENT as well as a distribution name, because an instance's state lives
// in a directory named for it.
func isInstanceName(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	// ⛔ Lower case only, and that is not tidiness. WSL compares distribution
	// names case-insensitively while a Windows path preserves case, so
	// `wsl-toolkit-A` and `wsl-toolkit-a` would be ONE distribution sharing TWO
	// state directories, and each would report the other's jobs as missing.
	return true
}

// ValidateOwnedName is the CREATE rule: a name this tool may bring into
// existence, as opposed to one it may act on.
//
// ⛔ IT IS STRICTER THAN IsOwnedName ON PURPOSE, and the difference is case. A
// distribution name is case-insensitive to wsl.exe and the state directory named
// for it is case-preserving on Windows, so `wsl-toolkit-A` and `wsl-toolkit-a`
// would be ONE distribution with TWO state directories, each reporting the
// other's jobs as missing. Refusing to create one costs nothing; refusing to
// ACT on one that already exists would leave a distribution nothing can remove.
func ValidateOwnedName(name string) error {
	if err := AssertOwnedDistro(name); err != nil {
		return err
	}
	if name != strings.ToLower(name) {
		return fmt.Errorf("%w: %q is not lower case, and a distribution name is case-insensitive to WSL "+
			"while the state directory named for it is not, so two spellings would be one distribution "+
			"with two state directories", ErrNotOwned, name)
	}
	return nil
}

// AssertOwnedDistro is the structural guard in front of every WSL call that
// changes a distribution.
//
// ⛔ It takes ONE name now. It used to take the name and the configured base
// name and compare them, which is a comparison of a value with itself at every
// call site in this program.
func AssertOwnedDistro(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: refusing to act on an empty distribution name", ErrNotOwned)
	}
	for _, p := range ProtectedDistros {
		if strings.EqualFold(name, p) {
			return fmt.Errorf("%w: REFUSING to touch %q, which is a container runtime's own distribution", ErrNotOwned, name)
		}
	}
	if !IsOwnedName(name) {
		return fmt.Errorf("%w: REFUSING to touch %q. This tool owns %s and %s-<instance> and nothing else. "+
			"A throwaway distribution is removed with %s",
			ErrNotOwned, name, OwnedPrefix, OwnedPrefix, ToolCommand("distro remove"))
	}
	return nil
}

// ReadIdentity asks the guest what it is.
//
// ⛔ It reads the GUEST and never the host record. The record is a file on this
// machine that an editor can reach; the marker is inside the distribution and
// was written by a provisioning run. Where the two disagree the marker wins,
// which is why this exists at all.
func (w *Wsl) ReadIdentity(ctx context.Context, name string) (Identity, error) {
	var id Identity
	if err := AssertOwnedDistro(name); err != nil {
		return id, err
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out := &boundedBuffer{max: 64 << 10}
	errBuf := &boundedBuffer{max: 32 << 10}
	// `cat` and nothing else: no shell expansion of the path, and a missing file
	// is a nonzero exit rather than an empty answer that reads as a marker with
	// no fields in it.
	//
	// ⛔ IN THE C LOCALE, SET HERE. Its words decide below whether the marker is
	// missing, and nothing held a guest to English: a guest whose locale is German
	// read a missing marker as one that could not be read, so `base ensure`
	// refused the stamp and `base remove` passed over the ledger. `env` keeps the
	// argv free of a shell, and `cat` still names itself. WSL-183.
	code, err := w.ExecDirect(bounded, name, "root", "",
		[]string{"/usr/bin/env", "LC_ALL=C", "/bin/cat", IdentityPath}, nil, out, errBuf, 2*time.Minute)
	// ⛔ A NONZERO EXIT IS AN ANSWER, NOT A FAILURE TO ASK. `cat` on a missing
	// file exits 1, and ExecDirect reports every nonzero exit as an error too,
	// so reading `err != nil` first reported "could not read the marker" about a
	// distribution that had simply never been stamped. That is this tree's own
	// recurring class: a refusal and an absence rendered as one outcome.
	//
	// ⚠ The two are separated by ASKING WSL what went wrong rather than by the
	// exit code, because a denied wsl.exe and a missing file both exit nonzero.
	if wslErr := classifyWslFailure(out.String(), errBuf.String(), err); wslErr != nil && errors.Is(wslErr, ErrWslDenied) {
		return id, wslErr
	}
	// ⛔ ONLY `cat` SAYING THE FILE IS MISSING IS AN ABSENCE. A timeout, a guest
	// that would not start or a broken install also exit nonzero, and none of them
	// says the marker is not there. Read as an absence, they demoted a base this
	// tool built and stamped a guest it could not reach. `cat` names the path it
	// could not open, and wsl.exe failing to start a guest does not. WSL-167.
	//
	// ⛔ AND ONLY ITS "No such file". `cat` names the path for an I/O error on a
	// damaged disk too, and that was read as a guest with no marker. The guest
	// started, and nothing was learned about the marker. WSL-183.
	// ⚠ The words are the C locale's, which the read above sets.
	// ⚠ BusyBox's `cat` names no path for a read error. Measured on Alpine's, with
	// an EIO: `cat: read error: I/O error`, exit 1. So the guest is known to have
	// started by `cat` naming itself on stderr, and not by the path. WSL-183.
	if code == 1 && bounded.Err() == nil && catAnswered(errBuf.String()) {
		if strings.Contains(errBuf.String(), IdentityPath) && strings.Contains(errBuf.String(), "No such file") {
			return id, fmt.Errorf("%s %w", name, ErrNoIdentity)
		}
		return id, fmt.Errorf("%w: %s started, and cat could not read %s: %s", ErrIdentityUnread, name, IdentityPath, guestFailure(errBuf.String()))
	}
	// ⛔ EVERYTHING ELSE IS A GUEST THAT DID NOT ANSWER: WSL refused to start it,
	// or the read never came back. WSL-183.
	if code != 0 || err != nil {
		return id, fmt.Errorf("%w: %w: %s, exit %d: %s", ErrIdentityUnread, ErrGuestUnstartable, name, code, guestFailure(out.String(), errBuf.String()))
	}
	if err := json.Unmarshal([]byte(out.String()), &id); err != nil {
		return id, fmt.Errorf("%s carries a %s that does not parse: %w", name, IdentityPath, err)
	}
	if id.Schema != IdentitySchema {
		return id, fmt.Errorf("%s carries an identity declaring schema %q and this build reads %q",
			name, id.Schema, IdentitySchema)
	}
	if !strings.EqualFold(id.Name, name) {
		// ⛔ A marker naming another distribution is a COPIED disk, not an
		// owned one. `wsl --export` and `--import` under a new name carries the
		// marker with it, and adopting it would let one command remove a
		// distribution built as something else.
		return id, fmt.Errorf("%w: %s carries an identity built as %q, so it is a copy rather than this distribution",
			ErrNotOwned, name, id.Name)
	}
	return id, nil
}

// catAnswered says `cat` itself wrote to stderr. GNU's names itself as it was
// called, `/bin/cat:`, and BusyBox's as `cat:`. wsl.exe failing to start a guest
// writes neither. WSL-183.
func catAnswered(stderr string) bool {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "/bin/cat: ") || strings.HasPrefix(line, "cat: ") {
			return true
		}
	}
	return false
}

// WriteIdentity stamps a distribution as this tool's. It runs as root, at the
// end of provisioning, which is the only moment this tool is root in a guest.
func (w *Wsl) WriteIdentity(ctx context.Context, name string, id Identity) error {
	if err := AssertOwnedDistro(name); err != nil {
		return err
	}
	id.Schema = IdentitySchema
	id.Name = name
	if id.Built.IsZero() {
		id.Built = time.Now().UTC()
	}
	data, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return w.writeGuestFile(ctx, name, IdentityPath, data, 0o644)
}

// writeGuestFile puts bytes at an absolute path inside a distribution, as root.
//
// ⛔ THE CONTENT TRAVELS THROUGH THE ARCHIVE CHANNEL, which is the same one a
// workspace and a job script use. It is not `sh -c "cat > path"`: an argument
// to wsl.exe is expanded before the guest sees it and the result is parsed
// again, so a shell redirect in an argument is the defect
// docs/conventions/shell.md section 7 measures. AssertArgvSafe refuses that
// spelling outright, which is the guard working rather than an obstacle.
//
// ⚠ IT CARRIES NO OWNERSHIP GUARD. Each caller applies the rule for the
// distribution it writes into before it gets here.
func (w *Wsl) writeGuestFile(ctx context.Context, distro, guestPath string, data []byte, mode int64) error {
	slash := strings.LastIndex(guestPath, "/")
	if !strings.HasPrefix(guestPath, "/") || slash < 0 || slash == len(guestPath)-1 {
		return fmt.Errorf("%q is not an absolute guest file path", guestPath)
	}
	dir, base := guestPath[:slash], guestPath[slash+1:]
	if dir == "" {
		dir = "/"
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	pr, pw := io.Pipe()
	go func() {
		tw := tar.NewWriter(pw)
		err := tw.WriteHeader(&tar.Header{
			Name: base, Typeflag: tar.TypeReg, Mode: mode,
			Size: int64(len(data)), ModTime: time.Now(),
		})
		if err == nil {
			_, err = tw.Write(data)
		}
		if err == nil {
			err = tw.Close()
		}
		_ = pw.CloseWithError(err)
	}()
	errBuf := &boundedBuffer{max: 32 << 10}
	code, err := w.ExecDirect(bounded, distro, "root", "",
		[]string{"/bin/tar", "-xf", "-", "-C", dir}, pr, io.Discard, errBuf, 2*time.Minute)
	if err != nil || code != 0 {
		return fmt.Errorf("could not write %s in %s (exit %d): %s", guestPath, distro, code, guestFailure(errBuf.String()))
	}
	return nil
}

// WindowsDriveMount is one mount at or below a one-letter directory under /mnt, as
// the guest's /proc/mounts shows it.
type WindowsDriveMount struct {
	Target   string
	ReadOnly bool
}

// MountedWindowsDrives lists the Windows drive mounts inside a distribution, so a
// warning can name what is actually reachable, and how.
//
// ⛔ ASKED, NOT ASSUMED. `/etc/wsl.conf` says automount is enabled, and which
// drives that produces depends on what Windows has mounted at the moment the
// distribution started. A warning built from the configuration would be a claim
// about a property the command line does not enforce, which is the class
// docs/conventions/forbidden-patterns.md names.
//
// ⛔ THE MOUNTS, NOT THE DIRECTORIES. This read the one-letter directories under
// /mnt, and `base shell --root` then called every drive "mounted and writable" in a
// base whose drives were all read-only, where root's touch answered "Read-only file
// system". Measured on 2026-09-14 by WSL-84's door sweep. It reads the same mounts
// verify.sh counts, by the same rule.
//
// ⚠ NONE FOUND AND NOT ASKED ARE TWO ANSWERS. An error means the table could not be
// read, and a caller says so rather than "no drive is mounted".
func MountedWindowsDrives(ctx context.Context, w *Wsl, distro string) ([]WindowsDriveMount, error) {
	if err := AssertOwnedDistro(distro); err != nil {
		return nil, err
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out := &boundedBuffer{max: 256 << 10}
	code, err := w.ExecDirect(bounded, distro, "root", "",
		[]string{"/bin/cat", "/proc/mounts"}, nil, out, nil, 30*time.Second)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("the mounts of %s could not be read (exit %d): %v", distro, code, err)
	}
	return parseWindowsDriveMounts(out.String()), nil
}

// parseWindowsDriveMounts reads the Windows drive mounts out of a mounts table. A
// drive is a one-letter directory under /mnt; /mnt also holds wsl, wslg and whatever
// else somebody put there, and none of those is a Windows volume.
func parseWindowsDriveMounts(table string) []WindowsDriveMount {
	var drives []WindowsDriveMount
	for _, line := range strings.Split(table, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		rest, ok := strings.CutPrefix(fields[1], "/mnt/")
		if !ok || rest == "" || !isDriveLetter(rest[0]) || (len(rest) > 1 && rest[1] != '/') {
			continue
		}
		mode, _, _ := strings.Cut(fields[3], ",")
		drives = append(drives, WindowsDriveMount{Target: fields[1], ReadOnly: mode == "ro"})
	}
	return drives
}

func isDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
