package toolkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ConfigSchema is the version every stored configuration carries. ⛔ A stored
// format with no version mis-reads silently the day its shape changes.
const ConfigSchema = "wsl-toolkit-config/1"

// DefaultBaseName is the WSL distribution this executable owns. ⚠ No `eph-`
// prefix on purpose: wsl-toolkit.ps1's Purge removes every distribution with
// one, and the base has to survive a purge.
const DefaultBaseName = "wsl-toolkit"

// DefaultBaseUser is the unprivileged account jobs run as inside the base.
// Root is used to provision the distribution and never afterwards.
const DefaultBaseUser = "toolkit"

// Image is one entry in the container catalog.
type Image struct {
	ID     string `json:"id"`
	Ref    string `json:"ref"`
	Libc   string `json:"libc"`
	Family string `json:"family"`
	Kind   string `json:"kind"`
	Note   string `json:"note,omitempty"`
}

// BaseConfig is what the owned distribution is built from.
type BaseConfig struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	User  string `json:"user"`
}

// Config is the stored configuration. Every field has a compiled-in default, so
// a machine with no file behaves like one holding what --write would produce.
type Config struct {
	Schema string     `json:"schema"`
	Base   BaseConfig `json:"base"`
	// Images REPLACES the built-in catalog when it has entries. One knob: a
	// list that both replaces and extends has to be read to be understood.
	Images []Image `json:"images,omitempty"`
	// Matrix names the ids the fleet runner uses when the caller names none.
	Matrix []string `json:"matrix,omitempty"`
	path   string
}

// DefaultBaseImage is the rootfs the owned distribution is built from when
// nothing selects another. BasePresets carries the alternatives and their
// measurements.
//
// ⭐ A glibc host, and not the fastest candidate. The base is where a toolchain
// gets installed when somebody wants one outside a container, and a musl host
// makes a glibc toolchain there a cross-compilation problem. ⚠ It has no bearing
// on what a CONTAINER runs.
const DefaultBaseImage = "ghcr.io/pkgforge-dev/archlinux:latest"

// BuiltinImages is the catalog.
//
// ⛔ Every reference is fully qualified. An engine resolves an unqualified name
// through its own alias table, so the same string is two different images on two
// machines and a digest pinned against one is `manifest unknown` against the
// other.
//
// ⚠ Where the maintainer publishes somewhere other than Docker Hub, that is the
// row: Docker Hub's anonymous pull limit is the likeliest cause of a red row
// that has nothing to do with the subject.
var BuiltinImages = []Image{
	// Three distinct musl userlands: different libc build, package manager and
	// coreutils, so three samples rather than one repeated.
	{ID: "alpine", Ref: "docker.io/library/alpine:latest", Libc: "musl", Family: "apk", Kind: "musl",
		Note: "busybox coreutils. Official publication is Docker Hub."},
	{ID: "void-musl", Ref: "ghcr.io/void-linux/void-musl:latest", Libc: "musl", Family: "xbps", Kind: "musl",
		Note: "the distribution's own registry, not a mirror."},
	{ID: "chimera", Ref: "docker.io/chimeralinux/chimera:latest", Libc: "musl", Family: "apk", Kind: "musl",
		Note: "musl with a BSD userland and LLVM toolchain, which is what makes it a third sample."},

	// Three glibc userlands.
	{ID: "arch", Ref: "ghcr.io/pkgforge-dev/archlinux:latest", Libc: "glibc", Family: "pacman", Kind: "glibc",
		Note: "maintained by this repository's operator. The default where a rolling glibc is wanted."},
	{ID: "debian", Ref: "docker.io/library/debian:latest", Libc: "glibc", Family: "apt", Kind: "glibc",
		Note: "the current stable release. Official publication is Docker Hub."},
	{ID: "fedora", Ref: "registry.fedoraproject.org/fedora:latest", Libc: "glibc", Family: "dnf", Kind: "glibc",
		Note: "the project's own registry rather than Docker Hub."},

	// Userlands that are neither of the two mainstream shapes.
	{ID: "gentoo", Ref: "docker.io/gentoo/stage3:latest", Libc: "glibc", Family: "portage", Kind: "niche",
		Note: "a source distribution's stage3, so nothing is prebuilt."},
	{ID: "wolfi", Ref: "cgr.dev/chainguard/wolfi-base:latest", Libc: "glibc", Family: "apk", Kind: "niche",
		Note: "apk over glibc, which no other row here is."},
	{ID: "photon", Ref: "docker.io/library/photon:latest", Libc: "glibc", Family: "tdnf", Kind: "niche",
		Note: "tdnf, and a kernel-adjacent userland tuned for hypervisors."},

	// Older, still maintained. These answer whether something builds against an
	// older libc, which a newer distribution cannot be asked.
	{ID: "rocky8", Ref: "quay.io/rockylinux/rockylinux:8", Libc: "glibc", Family: "dnf", Kind: "legacy",
		Note: "glibc 2.28. The project's own registry."},
	{ID: "ubuntu2204", Ref: "docker.io/library/ubuntu:22.04", Libc: "glibc", Family: "apt", Kind: "legacy",
		Note: "glibc 2.35, still in standard support."},
	{ID: "debian12", Ref: "docker.io/library/debian:12-slim", Libc: "glibc", Family: "apt", Kind: "legacy",
		Note: "glibc 2.36, the previous stable release."},
}

// DefaultConfig is what a machine with no configuration file behaves as.
func DefaultConfig() Config {
	return Config{
		Schema: ConfigSchema,
		Base:   BaseConfig{Name: DefaultBaseName, Image: DefaultBaseImage, User: DefaultBaseUser},
	}
}

// ConfigPath is where the configuration lives.
func ConfigPath() (string, error) {
	h, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, "config.json"), nil
}

// LoadConfig reads the configuration, or returns the defaults when there is no
// file. ⛔ A malformed file is a REFUSAL: silently ignoring somebody's
// configuration is how a setting nobody can see takes effect.
func LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	path, err := ConfigPath()
	if err != nil {
		return cfg, err
	}
	cfg.path = path
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	var stored Config
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&stored); err != nil {
		return cfg, fmt.Errorf("%s is not readable as %s: %w", path, ConfigSchema, err)
	}
	if stored.Schema != ConfigSchema {
		return cfg, fmt.Errorf("%s declares schema %q and this build reads %q", path, stored.Schema, ConfigSchema)
	}
	if stored.Base.Name != "" {
		cfg.Base.Name = stored.Base.Name
	}
	if stored.Base.Image != "" {
		cfg.Base.Image = stored.Base.Image
	}
	if stored.Base.User != "" {
		cfg.Base.User = stored.Base.User
	}
	cfg.Images = stored.Images
	cfg.Matrix = stored.Matrix
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Validate refuses a configuration at the point it is read rather than at the
// point it is used.
func (c Config) Validate() error {
	if !isDistroName(c.Base.Name) {
		return fmt.Errorf("base.name %q is not a usable distribution name", c.Base.Name)
	}
	for _, p := range ProtectedDistros {
		if strings.EqualFold(c.Base.Name, p) {
			return fmt.Errorf("base.name %q names a container runtime's own distribution", c.Base.Name)
		}
	}
	if strings.HasPrefix(strings.ToLower(c.Base.Name), "eph-") {
		return fmt.Errorf("base.name %q starts with the prefix wsl-toolkit.ps1's Purge removes, so the base would not survive one", c.Base.Name)
	}
	if !isShellName(c.Base.User) {
		return fmt.Errorf("base.user %q is not a usable account name", c.Base.User)
	}
	seen := map[string]bool{}
	for _, img := range c.Images {
		if err := img.Validate(); err != nil {
			return err
		}
		if seen[img.ID] {
			return fmt.Errorf("two images share the id %q", img.ID)
		}
		seen[img.ID] = true
	}
	known := map[string]bool{}
	for _, img := range c.Catalog() {
		known[img.ID] = true
	}
	for _, id := range c.Matrix {
		if !known[id] {
			return fmt.Errorf("matrix names %q, which is not in the catalog", id)
		}
	}
	return nil
}

// Validate refuses an image entry that could not be pulled or named.
func (i Image) Validate() error {
	if !isImageID(i.ID) {
		// ⚠ THE MESSAGE NAMES BOTH HALVES OF THE RULE. It used to describe
		// the character set alone, so a caller with `.hidden` was told the id
		// must be letters, digits, dot, dash or underscore, which `.hidden`
		// already is. A refusal that describes a rule the input satisfies is a
		// refusal nobody can act on.
		return fmt.Errorf("image id %q must be letters, digits, dot, dash or underscore, and must not start with a dot", i.ID)
	}
	if err := ValidateImageRef(i.Ref); err != nil {
		return fmt.Errorf("image %s: %w", i.ID, err)
	}
	return nil
}

// ValidateImageRef refuses a reference that is not fully qualified, or that
// carries a character a shell or an engine would read as syntax. ⛔ An
// unqualified name fails as a manifest error rather than a naming one.
func ValidateImageRef(ref string) error {
	if strings.TrimSpace(ref) == "" {
		return errors.New("an image reference cannot be empty")
	}
	if strings.ContainsAny(ref, " \t\r\n'\"`$;&|<>()*?[]{}\\") {
		return fmt.Errorf("%q carries a character an engine or a shell would read as syntax", ref)
	}
	host, rest, ok := strings.Cut(ref, "/")
	if !ok {
		return fmt.Errorf("%q is not fully qualified: it names no registry, so the engine's own alias table decides what it means", ref)
	}
	if !strings.Contains(host, ".") && !strings.Contains(host, ":") && host != "localhost" {
		return fmt.Errorf("%q is not fully qualified: %q is not a registry host", ref, host)
	}
	if rest == "" {
		return fmt.Errorf("%q names a registry and no repository", ref)
	}
	return nil
}

// isImageID is the rule for a name this tool uses as a PATH COMPONENT.
//
// ⛔ A NAME MADE ONLY OF DOTS IS NOT A NAME. `matrix --artifacts out` writes
// each row into `out/<id>`, so an id of `..` put a fleet's output in the parent
// of the directory the caller named, and `.` put every row in one place. Both
// passed the character rule, because a dot is a legal character in `debian12`
// and in `ubuntu-24.04`. The config is the caller's own file, so this is not a
// privilege boundary; it is a name that means something other than what it
// looks like, which this tree validates at the point it is READ.
func isImageID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	if strings.HasPrefix(s, ".") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return false
		}
	}
	return true
}

// isDistroName is the same rule for a distribution, and for the same reason:
// the base's own state lives under a directory named for it.
func isDistroName(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	if strings.HasPrefix(s, ".") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return false
		}
	}
	return true
}

// Catalog is the effective image list: the stored one when it has entries, the
// built-in one otherwise.
func (c Config) Catalog() []Image {
	if len(c.Images) > 0 {
		return c.Images
	}
	return BuiltinImages
}

// MatrixDefault is what a fleet runs when the caller names nothing: the stored
// selection, or the whole catalog.
func (c Config) MatrixDefault() []string {
	if len(c.Matrix) > 0 {
		out := make([]string, len(c.Matrix))
		copy(out, c.Matrix)
		return out
	}
	catalog := c.Catalog()
	out := make([]string, 0, len(catalog))
	for _, img := range catalog {
		out = append(out, img.ID)
	}
	return out
}

// SelectImages resolves a caller's selection into catalog entries. A selector is
// an id, a `libc:` term, a `kind:` term, or `all`.
//
// ⛔ An unknown selector is a refusal naming what is available. A fleet that
// matched nothing and exited 0 reads exactly like one where everything agreed.
func (c Config) SelectImages(selectors []string) ([]Image, error) {
	catalog := c.Catalog()
	byID := map[string]Image{}
	for _, img := range catalog {
		byID[img.ID] = img
	}
	if len(selectors) == 0 {
		selectors = c.MatrixDefault()
	}
	var out []Image
	seen := map[string]bool{}
	add := func(img Image) {
		if !seen[img.ID] {
			seen[img.ID] = true
			out = append(out, img)
		}
	}
	for _, raw := range selectors {
		for _, sel := range strings.Split(raw, ",") {
			sel = strings.TrimSpace(sel)
			if sel == "" {
				continue
			}
			switch {
			case sel == "all":
				for _, img := range catalog {
					add(img)
				}
			case strings.HasPrefix(sel, "libc:"):
				want := strings.TrimPrefix(sel, "libc:")
				hit := false
				for _, img := range catalog {
					if strings.EqualFold(img.Libc, want) {
						add(img)
						hit = true
					}
				}
				if !hit {
					return nil, fmt.Errorf("no catalog image has libc %q", want)
				}
			case strings.HasPrefix(sel, "kind:"):
				want := strings.TrimPrefix(sel, "kind:")
				hit := false
				for _, img := range catalog {
					if strings.EqualFold(img.Kind, want) {
						add(img)
						hit = true
					}
				}
				if !hit {
					return nil, fmt.Errorf("no catalog image has kind %q", want)
				}
			default:
				img, ok := byID[sel]
				if !ok {
					ids := make([]string, 0, len(catalog))
					for _, i := range catalog {
						ids = append(ids, i.ID)
					}
					sort.Strings(ids)
					return nil, fmt.Errorf("%q is not a catalog image. Available: %s", sel, strings.Join(ids, " "))
				}
				add(img)
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("the selection resolved to no images, and a run over nothing cannot report a result")
	}
	return out, nil
}

// Write stores the configuration, atomically.
func (c Config) Write() error {
	home, err := EnsureHome()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "config.json")
	c.Schema = ConfigSchema
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0o600)
}

// writeFileAtomic writes through a temp file in the SAME directory, then a
// rename, so a killed process leaves the old file intact. Same directory
// matters: a rename across volumes is a copy and loses the guarantee.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
