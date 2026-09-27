// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

// A job takes two things beyond its workspace: host device nodes, passed into its
// container by the engine, and input files, copied into its own directory and
// seen read-only at /in. Neither is a mount of a host directory.

// JobDevice is one device node in the base, passed into a job's container.
// WSL-96.
type JobDevice struct {
	Host      string `json:"host"`
	Container string `json:"container,omitempty"`
	Perms     string `json:"perms,omitempty"`
}

// String is podman's own spelling: HOST[:CONTAINER[:PERMS]].
func (d JobDevice) String() string {
	s := d.Host
	if d.Container != "" || d.Perms != "" {
		c := d.Container
		if c == "" {
			c = d.Host
		}
		s += ":" + c
	}
	if d.Perms != "" {
		s += ":" + d.Perms
	}
	return s
}

// devicePathAlphabet is what a device path in the guest may hold. It is the
// alphabet a guest path handed to wsl.exe survives, with nothing that is not a
// file name character.
const devicePathAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./+"

// ParseDevice reads one --device value.
//
// ⛔ THE HOST PATH IS UNDER /dev AND NOTHING ELSE. A device is passed by the
// engine as a node, and a path outside /dev names a file, which would be a mount
// by another name.
func ParseDevice(spec string) (JobDevice, error) {
	parts := strings.Split(spec, ":")
	if len(parts) > 3 {
		return JobDevice{}, fmt.Errorf("--device %q has more than three parts. The form is HOST[:CONTAINER[:PERMS]]", spec)
	}
	d := JobDevice{Host: parts[0]}
	if len(parts) > 1 {
		d.Container = parts[1]
	}
	if len(parts) > 2 {
		d.Perms = parts[2]
	}
	if err := checkDevicePath("--device "+spec+": the host node", d.Host, true); err != nil {
		return JobDevice{}, err
	}
	if len(parts) > 1 {
		if err := checkDevicePath("--device "+spec+": the container path", d.Container, false); err != nil {
			return JobDevice{}, err
		}
	}
	if len(parts) > 2 {
		if d.Perms == "" || strings.Trim(d.Perms, "rwm") != "" || len(d.Perms) > 3 ||
			strings.Count(d.Perms, "r") > 1 || strings.Count(d.Perms, "w") > 1 || strings.Count(d.Perms, "m") > 1 {
			return JobDevice{}, fmt.Errorf("--device %q: the permissions %q are not letters from r, w and m, each at most once", spec, d.Perms)
		}
	}
	return d, nil
}

func checkDevicePath(what, p string, underDev bool) error {
	if p == "" || !strings.HasPrefix(p, "/") {
		return fmt.Errorf("%s %q is not an absolute path in the guest", what, p)
	}
	if path.Clean(p) != p {
		return fmt.Errorf("%s %q is not in its clean form, which is %q", what, p, path.Clean(p))
	}
	if underDev && !strings.HasPrefix(p, "/dev/") {
		return fmt.Errorf("%s %q is not under /dev. A device is a node there, and a file anywhere else would be a mount by another name", what, p)
	}
	for _, r := range p {
		if !strings.ContainsRune(devicePathAlphabet, r) {
			return fmt.Errorf("%s %q carries %q, which no device path holds", what, p, r)
		}
	}
	return nil
}

// deviceCheckScript refuses, in the guest and before the engine starts, a node
// that is absent, is not a device, or that the base account cannot read and
// write. The refusal is the last line of stderr, so a job that stops here is
// UNREACHED and its error names the node.
func deviceCheckScript(devices []JobDevice) string {
	var b strings.Builder
	for _, d := range devices {
		q := shellQuote(d.Host)
		fmt.Fprintf(&b, "if [ ! -e %s ]; then echo \"wsl-toolkit: --device %s: there is no such node in the base\" >&2; exit 125; fi\n", q, d.Host)
		fmt.Fprintf(&b, "if [ ! -c %s ] && [ ! -b %s ]; then echo \"wsl-toolkit: --device %s: it is not a device node\" >&2; exit 125; fi\n", q, q, d.Host)
		fmt.Fprintf(&b, "if [ ! -r %s ] || [ ! -w %s ]; then echo \"wsl-toolkit: --device %s: the base account cannot read and write it, so a container it starts cannot either: $(ls -l %s 2>/dev/null)\" >&2; exit 125; fi\n", q, q, d.Host, q)
	}
	return b.String()
}

// JobInput is one file a job reads at /in/NAME. WSL-97.
//
// Path is a file on this machine, read when the job sends it. Bytes is the
// file's content where it arrived over the helper protocol instead. Exactly one
// of the two is set.
type JobInput struct {
	Name  string `json:"name"`
	Path  string `json:"-"`
	Bytes []byte `json:"-"`
}

// MaxHelperInputBytes bounds the inputs one job sends over the helper protocol.
// They travel inside a JSON body the helper caps at maxRequestBytes, and base64
// adds a third.
const MaxHelperInputBytes = 2 << 20

// ValidateInputName refuses a name that is not a relative path of plain
// components, so no input can climb out of /in or hide behind a dot.
func ValidateInputName(name string) error {
	if name == "" {
		return fmt.Errorf("--input needs NAME=FILE, and the name is empty")
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("--input name %q is absolute. A name is a path under /in, such as job.sh or conf/app.toml", name)
	}
	for _, part := range strings.Split(name, "/") {
		switch part {
		case "", ".", "..":
			return fmt.Errorf("--input name %q holds an empty, . or .. component. A name is plain components under /in", name)
		}
		for _, r := range part {
			if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-.+=@,", r) {
				return fmt.Errorf("--input name %q carries %q. A name is letters, digits and _-.+=@, with / between components", name, r)
			}
		}
	}
	return nil
}

// CheckInputs refuses a set of inputs that cannot all arrive as named: a name
// twice, one name inside another's path, a file that is not a regular file, or
// a set past the limits a workspace is held to.
func CheckInputs(inputs []JobInput, limits WorkspaceLimits) error {
	names := map[string]bool{}
	var total int64
	for _, in := range inputs {
		if err := ValidateInputName(in.Name); err != nil {
			return err
		}
		if names[in.Name] {
			return fmt.Errorf("--input names %q twice", in.Name)
		}
		names[in.Name] = true
		size := int64(len(in.Bytes))
		if in.Path != "" {
			info, err := os.Stat(deviceSafePath(in.Path))
			if err != nil {
				return fmt.Errorf("--input %s=%s: %w", in.Name, in.Path, err)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("--input %s=%s is not a regular file. A directory travels as the workspace", in.Name, in.Path)
			}
			size = info.Size()
		}
		total += size
	}
	for name := range names {
		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			if names[dir] {
				return fmt.Errorf("--input %q is a file and %q needs it to be a directory", dir, name)
			}
		}
	}
	if total > limits.MaxBytes {
		return fmt.Errorf("%w: the inputs hold %s, which passes %s", ErrWorkspaceRefused, HumanBytes(total), HumanBytes(limits.MaxBytes))
	}
	if len(inputs) > limits.MaxEntries {
		return fmt.Errorf("%w: %d inputs, which passes %d entries", ErrWorkspaceRefused, len(inputs), limits.MaxEntries)
	}
	return nil
}

// writeInputsTar writes the inputs as one archive, each file mode 0644 with its
// directories made first. A host file travels through the member writer the
// workspace uses, so it gets the same bound and the same naming of a failure.
func writeInputsTar(w io.Writer, inputs []JobInput) (WorkspaceUpload, error) {
	var up WorkspaceUpload
	tw := tar.NewWriter(w)
	sorted := append([]JobInput(nil), inputs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	made := map[string]bool{}
	now := time.Now()
	for _, in := range sorted {
		for _, dir := range parentDirs(in.Name) {
			if made[dir] {
				continue
			}
			made[dir] = true
			if err := tw.WriteHeader(&tar.Header{Name: dir + "/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: now}); err != nil {
				return up, memberError("/in/"+dir+"/", "writing its archive header", err)
			}
		}
		if in.Path != "" {
			info, err := os.Stat(deviceSafePath(in.Path))
			if err != nil {
				return up, memberError("/in/"+in.Name, "reading its attributes", err)
			}
			if err := writeRegularMember(tw, &up, in.Path, in.Name, info, 0o644); err != nil {
				return up, err
			}
			up.Entries++
			up.Bytes += info.Size()
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: in.Name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(in.Bytes)), ModTime: now}); err != nil {
			return up, memberError("/in/"+in.Name, "writing its archive header", err)
		}
		if _, err := tw.Write(in.Bytes); err != nil {
			return up, memberError("/in/"+in.Name, "copying its bytes", err)
		}
		up.Entries++
		up.Bytes += int64(len(in.Bytes))
	}
	if err := tw.Close(); err != nil {
		return up, fmt.Errorf("the inputs could not finish their archive: %w", err)
	}
	return up, nil
}

// parentDirs is every directory above a name, outermost first.
func parentDirs(name string) []string {
	var dirs []string
	for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
		dirs = append([]string{dir}, dirs...)
	}
	return dirs
}
