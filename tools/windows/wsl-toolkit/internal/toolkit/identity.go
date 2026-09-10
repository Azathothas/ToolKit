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
// ⚠ NO `eph-` PREFIX, deliberately: wsl-toolkit.ps1's Purge removes every
// distribution carrying one, and the base has to survive a purge.
const OwnedPrefix = DefaultBaseName

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
}

// ErrNotOwned is what every ownership refusal wraps, so a caller can tell one
// from an ordinary failure without reading a message.
var ErrNotOwned = errors.New("not a distribution this tool owns")

// ErrNoIdentity says the distribution exists, its name is one this tool may
// own, and it carries no marker. ⭐ It is a SEPARATE error because it is the
// one case with an upgrade path: a base built before markers existed is
// adoptable, and one built by somebody else is not.
var ErrNoIdentity = errors.New("carries no wsl-toolkit identity marker")

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
			"Use wsl-toolkit script -Action Remove for a throwaway distro",
			ErrNotOwned, name, OwnedPrefix, OwnedPrefix)
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
	code, err := w.ExecDirect(bounded, name, "root", "",
		[]string{"/bin/cat", IdentityPath}, nil, out, errBuf, 2*time.Minute)
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
	if code != 0 || err != nil {
		return id, fmt.Errorf("%s %w", name, ErrNoIdentity)
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
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// ⛔ THE CONTENT TRAVELS THROUGH THE ARCHIVE CHANNEL, which is the same one
	// a workspace and a job script use. It is not `sh -c "cat > path"`: an
	// argument to wsl.exe is expanded before the guest sees it and the result is
	// parsed again, so a shell redirect in an argument is the defect
	// docs/conventions/shell.md section 7 measures. AssertArgvSafe refuses that
	// spelling outright, which is the guard working rather than an obstacle.
	dir, base := IdentityPath[:strings.LastIndex(IdentityPath, "/")], IdentityPath[strings.LastIndex(IdentityPath, "/")+1:]
	pr, pw := io.Pipe()
	go func() {
		tw := tar.NewWriter(pw)
		err := tw.WriteHeader(&tar.Header{
			Name: base, Typeflag: tar.TypeReg, Mode: 0o644,
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
	code, err := w.ExecDirect(bounded, name, "root", "",
		[]string{"/bin/tar", "-xf", "-", "-C", dir}, pr, io.Discard, errBuf, 2*time.Minute)
	if err != nil || code != 0 {
		return fmt.Errorf("could not write %s in %s (exit %d): %s", IdentityPath, name, code, firstLine(errBuf.String()))
	}
	return nil
}

// MountedWindowsDrives lists the Windows drives WSL has mounted inside a
// distribution, so a warning can name what is actually reachable.
//
// ⛔ ASKED, NOT ASSUMED. `/etc/wsl.conf` says automount is enabled, and which
// drives that produces depends on what Windows has mounted at the moment the
// distribution started. A warning built from the configuration would be a claim
// about a property the command line does not enforce, which is the class
// docs/conventions/forbidden-patterns.md names.
//
// ⚠ An empty answer means "none found", including the case where the question
// could not be asked. The caller's message says "no drive is mounted", which is
// weaker than the truth in that one case and never stronger.
func MountedWindowsDrives(ctx context.Context, w *Wsl, distro string) []string {
	if err := AssertOwnedDistro(distro); err != nil {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out := &boundedBuffer{max: 64 << 10}
	code, err := w.ExecDirect(bounded, distro, "root", "",
		[]string{"/bin/ls", "-1", "/mnt"}, nil, out, nil, 30*time.Second)
	if err != nil || code != 0 {
		return nil
	}
	var drives []string
	for _, line := range strings.Split(out.String(), "\n") {
		name := strings.TrimSpace(strings.Trim(line, "\r\x00"))
		// A drive is one letter. /mnt also holds wsl, wslg and whatever else
		// somebody put there, and none of those is a Windows volume.
		if len(name) == 1 && name[0] >= 'a' && name[0] <= 'z' {
			drives = append(drives, "/mnt/"+name)
		}
	}
	return drives
}
