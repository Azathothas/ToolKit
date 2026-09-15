package toolkit

import (
	"fmt"
	"sort"
	"strings"
)

// BasePreset is a rootfs the owned distribution can be built from, with its cost
// measured rather than described. ⭐ Switching host userland is one flag.
type BasePreset struct {
	ID    string `json:"id"`
	Ref   string `json:"ref"`
	Libc  string `json:"libc"`
	Note  string `json:"note"`
	Build string `json:"build"` // what it measured here, with its conditions
}

// BasePresets are the ones this tool has built and verified.
//
// ⛔ Every `build` figure was taken, not estimated, on one Windows 11 Pro 26200
// host with a warm image cache, from `base ensure` to a rootless container
// returning a marker. A row with no figure says so rather than carrying a
// plausible number.
//
// ⚠ TWO SETS OF CONDITIONS, AND EACH IS DATED, because they are not comparable.
// The 2026-09-09 figures were taken with no toolset; the 2026-09-15 ones with
// `--toolset developer`, automount and interop off, which installs thirteen more
// commands and is what a base for agents is actually built with. A mirror's speed
// moves these more than the distribution does: fedora's answered in tens of KiB/s
// on the day. Disk was not re-measured on the second date.
var BasePresets = []BasePreset{
	{
		ID: "arch", Ref: "ghcr.io/pkgforge-dev/archlinux:latest", Libc: "glibc",
		Note:  "the default. glibc, rolling, and the image this repository's operator maintains, so its podman is the newest of the three.",
		Build: "2026-09-09, no toolset: 31s, 940 MiB disk, podman 6.1.1, and about 38s more on a cold pull of its 540.8 MiB rootfs. 2026-09-15, toolset developer: 77s, podman 6.1.1.",
	},
	{
		ID: "alpine", Ref: "docker.io/library/alpine:latest", Libc: "musl",
		Note:  "the smallest. ⚠ musl, so a glibc toolchain running IN THE BASE is not native. It has no bearing on what a CONTAINER runs.",
		Build: "2026-09-09, no toolset: 28s, 204 MiB disk, podman 5.8.6. 2026-09-15, toolset developer: 173s, podman 5.8.6.",
	},
	{
		ID: "debian", Ref: "docker.io/library/debian:latest", Libc: "glibc",
		Note:  "glibc and stable. ⚠ Its podman does not depend on passt, so the provisioning installs it and says which rootless network path it chose.",
		Build: "2026-09-09, no toolset: 37s, 556 MiB disk, podman 5.4.2. ⚠ It then did not build at all until WSL-86: its podman shells out to nft and this arm installed no firewall package. 2026-09-15, toolset developer: 94s, podman 5.4.2.",
	},
	{
		ID: "fedora", Ref: "registry.fedoraproject.org/fedora:latest", Libc: "glibc",
		Note:  "glibc, and the closest of these to the engine's own upstream.",
		Build: "2026-09-15, no toolset: 48s, 588 MiB disk, podman 5.8.4. ⚠ With the developer toolset its mirror answered in tens of KiB/s and one build took 420s and another ran out of the 30m budget. Its first build here: before WSL-86 its imported newuidmap carried no capability and no container ran.",
	},
}

// ResolveBasePreset turns a preset id or a fully qualified reference into an
// image. ⛔ Anything else is a refusal naming what is available: falling back to
// the default over a typo would build a host nobody asked for.
func ResolveBasePreset(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("no preset or reference given")
	}
	for _, p := range BasePresets {
		if strings.EqualFold(p.ID, value) {
			return p.Ref, nil
		}
	}
	if err := ValidateImageRef(value); err != nil {
		ids := make([]string, 0, len(BasePresets))
		for _, p := range BasePresets {
			ids = append(ids, p.ID)
		}
		sort.Strings(ids)
		return "", fmt.Errorf("%q is not a preset (%s) and not a fully qualified reference: %w",
			value, strings.Join(ids, " "), err)
	}
	return value, nil
}

// PresetForRef names the preset a reference belongs to, or an empty string for a
// custom one. It is used only to render a report.
func PresetForRef(ref string) string {
	for _, p := range BasePresets {
		if p.Ref == ref {
			return p.ID
		}
	}
	return ""
}
