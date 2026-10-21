// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"fmt"
	"path"
	"strings"
)

// A job can keep two more things beyond what it hands back: a named cache that
// outlives it, and a Linux capability it was given on purpose.

// JobCache is one named cache a job mounts: a podman volume in the base, made on
// its first use and kept between jobs, so a toolchain installed once is there
// for the next job. WSL-122.
type JobCache struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// CacheVolumePrefix begins every cache volume's name, so `resources` and
// `gc --caches` find exactly them.
const CacheVolumePrefix = "wtk-cache-"

// CacheLabel is stamped on a cache volume beside JobLabel, carrying its name.
const CacheLabel = "wsl-toolkit.cache"

// Volume is the podman volume a cache lives in.
func (c JobCache) Volume() string { return CacheVolumePrefix + c.Name }

// String is the spelling --cache takes, NAME:/path.
func (c JobCache) String() string { return c.Name + ":" + c.Path }

// maxCacheName bounds a cache's name, so its volume's name stays one a reader
// can type.
const maxCacheName = 63

// ValidateCacheName holds a cache's name to `[a-z0-9][a-z0-9_.-]*`, at most 63.
func ValidateCacheName(name string) error {
	if name == "" || len(name) > maxCacheName {
		return fmt.Errorf("--cache name %q must be 1 to %d characters", name, maxCacheName)
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case i > 0 && (r == '_' || r == '.' || r == '-'):
		default:
			return fmt.Errorf("--cache name %q must start with a lower-case letter or a digit, and hold only those, _, . and -", name)
		}
	}
	return nil
}

// cachePathAlphabet is what a cache's mount point may hold: a file name's
// characters, and nothing podman's volume syntax reads as a separator.
const cachePathAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./+@"

// ParseCache reads one NAME:/path.
//
// ⛔ THE MOUNT POINT IS NOT ONE THIS TOOL GIVES THE JOB. /work, /out, /in and
// /job.sh are the job's own, and a cache over one would hide it. A kernel
// filesystem is refused for the same reason.
func ParseCache(spec string) (JobCache, error) {
	name, p, ok := strings.Cut(spec, ":")
	if !ok {
		return JobCache{}, fmt.Errorf("--cache %q is not NAME:/path: a name, a colon, then where the job sees it", spec)
	}
	if err := ValidateCacheName(name); err != nil {
		return JobCache{}, err
	}
	if !strings.HasPrefix(p, "/") || p == "/" {
		return JobCache{}, fmt.Errorf("--cache %q: %q is not an absolute path below /", spec, p)
	}
	if path.Clean(p) != p {
		return JobCache{}, fmt.Errorf("--cache %q: %q is not in its clean form, which is %q", spec, p, path.Clean(p))
	}
	for _, r := range p {
		if !strings.ContainsRune(cachePathAlphabet, r) {
			return JobCache{}, fmt.Errorf("--cache %q: %q carries %q, which a mount point here does not hold", spec, p, r)
		}
	}
	for _, own := range []string{"/work", "/out", "/in", "/job.sh"} {
		if p == own || (own != "/work" && own != "/out" && strings.HasPrefix(p, own+"/")) {
			return JobCache{}, fmt.Errorf("--cache %q: %s is the job's own, and a cache there would hide it", spec, own)
		}
	}
	for _, kernel := range []string{"/proc", "/sys", "/dev"} {
		if p == kernel || strings.HasPrefix(p, kernel+"/") {
			return JobCache{}, fmt.Errorf("--cache %q: %s is a kernel filesystem, and a cache cannot be mounted over it", spec, kernel)
		}
	}
	return JobCache{Name: name, Path: p}, nil
}

// ParseCaches reads a list of NAME:/path and refuses two caches at one path.
// what names the list in a refusal: --cache, or jobs.cache.
func ParseCaches(specs []string, what string) ([]JobCache, error) {
	var out []JobCache
	at := map[string]string{}
	for _, s := range specs {
		c, err := ParseCache(s)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		if other, ok := at[c.Path]; ok {
			return nil, fmt.Errorf("%s mounts %s twice, as %s and as %s. A path holds one cache", what, c.Path, other, c.Name)
		}
		at[c.Path] = c.Name
		out = append(out, c)
	}
	return out, nil
}

// cacheCreateScript makes each cache's volume where it is missing, in the guest
// and before the engine starts. A volume that cannot be made is the job's last
// stderr line, so the job is UNREACHED and its error names the cache.
//
// ⭐ IT IS MADE WITH THIS TOOL'S LABEL, so `resources` and `gc --caches` find
// it. A volume podman made on its own for a `--volume` would carry none.
//
// ⚠ A SECOND JOB CAN MAKE IT FIRST. The create that loses that race fails, and
// the volume then exists, so existence is asked again before a refusal.
func cacheCreateScript(caches []JobCache) string {
	var b strings.Builder
	made := map[string]bool{}
	for _, c := range caches {
		if made[c.Name] {
			continue
		}
		made[c.Name] = true
		v := shellQuote(c.Volume())
		fmt.Fprintf(&b, "if ! podman volume exists %s; then\n", v)
		fmt.Fprintf(&b, "  if ! tk_err=$(podman volume create --label %s --label %s %s 2>&1 >/dev/null) && ! podman volume exists %s; then\n",
			shellQuote(JobLabel), shellQuote(CacheLabel+"="+c.Name), v, v)
		fmt.Fprintf(&b, "    echo \"wsl-toolkit: --cache %s: podman could not make the volume %s: $tk_err\" >&2; exit 125\n", c.Name, c.Volume())
		b.WriteString("  fi\nfi\n")
	}
	return b.String()
}

// linuxCapabilities is every capability the kernel names, without CAP_.
var linuxCapabilities = []string{
	"AUDIT_CONTROL", "AUDIT_READ", "AUDIT_WRITE", "BLOCK_SUSPEND", "BPF",
	"CHECKPOINT_RESTORE", "CHOWN", "DAC_OVERRIDE", "DAC_READ_SEARCH", "FOWNER",
	"FSETID", "IPC_LOCK", "IPC_OWNER", "KILL", "LEASE", "LINUX_IMMUTABLE",
	"MAC_ADMIN", "MAC_OVERRIDE", "MKNOD", "NET_ADMIN", "NET_BIND_SERVICE",
	"NET_BROADCAST", "NET_RAW", "PERFMON", "SETFCAP", "SETGID", "SETPCAP",
	"SETUID", "SYSLOG", "SYS_ADMIN", "SYS_BOOT", "SYS_CHROOT", "SYS_MODULE",
	"SYS_NICE", "SYS_PACCT", "SYS_PTRACE", "SYS_RAWIO", "SYS_RESOURCE",
	"SYS_TIME", "SYS_TTY_CONFIG", "WAKE_ALARM",
}

// ParseCapability reads one --cap-add: a capability the kernel names, with or
// without CAP_, in any case, or ALL. It answers podman's spelling. WSL-130.
//
// ⛔ A NAME THE KERNEL DOES NOT HAVE IS REFUSED HERE. podman refuses it too, but
// only once the job is claimed, as a container that never started.
//
// ⚠ A capability is the container's, inside the base account's user namespace.
// It is not one over this machine, and not one over the base.
func ParseCapability(v string) (string, error) {
	name := strings.ToUpper(strings.TrimSpace(v))
	if name == "ALL" {
		return name, nil
	}
	name = strings.TrimPrefix(name, "CAP_")
	for _, c := range linuxCapabilities {
		if c == name {
			return "CAP_" + name, nil
		}
	}
	return "", fmt.Errorf("--cap-add %q is not a Linux capability. Pass one such as NET_ADMIN or SYS_PTRACE, or ALL", v)
}
