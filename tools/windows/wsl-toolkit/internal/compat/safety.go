// SPDX-License-Identifier: 0BSD

package compat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The naming and safety guards, ported from the script's safety.ps1.
//
// The SAFETY MODEL is the reason this interface is allowed to be destructive
// at all, and every destructive path goes through the same choke points:
//
//  1. Every distro it creates is named with a fixed prefix (default 'eph-').
//  2. It REFUSES to remove any distro whose name lacks that prefix.
//  3. It REFUSES to remove any name on an explicit protected list, prefix or
//     not: podman-machine-default and the Docker Desktop distros are
//     protected, so a mistake here cannot destroy a container runtime.
//  4. Directory deletion is confined to a strict child of the base directory;
//     the base directory itself and anything outside it can never be a target.

// prefix is the distro-name prefix. ⛔ IT IS NOT RENAMED WITH THE TOOL, and
// that is the decision rather than an oversight: 'eph-' names state that
// already exists on machines, and renaming it makes List and Purge blind to
// every distro created before the rename, orphaning multi-gigabyte VHDX files.
const prefix = "eph-"

// protectedNames must NEVER be unregistered, even when somebody prefixes them.
var protectedNames = []string{
	"podman-machine-default",
	"docker-desktop",
	"docker-desktop-data",
	"rancher-desktop",
	"rancher-desktop-data",
	// The base distro the wsl-toolkit EXECUTABLE owns. It has no 'eph-'
	// prefix, so the prefix rule already refuses it; naming it here as well is
	// the second guard, and the place a reader finds out the two tools share a
	// machine.
	"wsl-toolkit",
}

// importSpaceFactor and importSpaceFloor are what --import costs on the target
// volume, as a floor plus a multiple of the rootfs tarball. ⛔ BOTH ARE SET
// ABOVE EVERY MEASUREMENT rather than fitted to them: a tight preflight
// refuses an import that would have worked, which is a worse failure than the
// one it prevents. The floor is what matters, because an 8 MiB rootfs still
// costs 76 MiB.
const (
	importSpaceFactor = 2
	importSpaceFloor  = int64(256) << 20
)

func isProtectedName(distro string) bool {
	for _, p := range protectedNames {
		if strings.EqualFold(distro, p) {
			return true
		}
	}
	return false
}

// assertRemovable is the single choke point for every destructive path.
func (s *session) assertRemovable(distro string) error {
	if strings.TrimSpace(distro) == "" {
		return fmt.Errorf("Refusing to remove: empty distro name.")
	}
	if isProtectedName(distro) {
		return fmt.Errorf("REFUSING to remove protected distro '%s'. This is a hard guard.", distro)
	}
	if !strings.HasPrefix(distro, prefix) {
		return fmt.Errorf("REFUSING to remove '%s': it does not start with '%s'. This tool only removes distros it created.", distro, prefix)
	}
	return nil
}

// assertInsideBaseDir guarantees a path slated for deletion is a STRICT child
// of the base directory. Without it, an empty or crafted distro name could
// resolve the target to the base directory itself or, with traversal,
// somewhere else entirely.
func (s *session) assertInsideBaseDir(path string) error {
	baseFull, err := fullPathNormalise(trimSeparators(s.baseDir) + string(filepath.Separator))
	if err != nil {
		return fmt.Errorf("REFUSING to delete '%s': the state directory is not usable (%v).", path, err)
	}
	full, err := fullPathNormalise(path)
	if err != nil {
		return fmt.Errorf("REFUSING to delete '%s': it is not a usable path.", path)
	}
	if strings.EqualFold(trimSeparators(full), trimSeparators(baseFull)) {
		return fmt.Errorf("REFUSING to delete the base directory itself (%s).", full)
	}
	if !strings.HasPrefix(strings.ToLower(full), strings.ToLower(baseFull)) {
		return fmt.Errorf("REFUSING to delete '%s': outside %s.", full, s.baseDir)
	}
	return nil
}

// removePathWithRetry is THE one deletion in this interface. Every path that
// removes something on disk goes through here, so the containment guard cannot
// be applied to one of them and forgotten on another.
//
// It deletes, then READS THE STATE BACK, and reports what is true rather than
// what was attempted. The retry is not decoration: 'wsl --unregister' releases
// the VHDX asynchronously, so the delete immediately after it can lose the
// race against a handle that is about to be closed anyway.
func (s *session) removePathWithRetry(path, what, remedy string) error {
	// ⛔ inside the helper, not beside it.
	if err := s.assertInsideBaseDir(path); err != nil {
		return err
	}
	const attempts = 5
	for i := 1; i <= attempts; i++ {
		// The read-back below is the verdict; a failed attempt is not where
		// success is decided.
		_ = os.RemoveAll(path)
		if _, err := os.Lstat(path); err != nil {
			s.log.ok("deleted " + path)
			return nil
		}
		if i < attempts {
			time.Sleep(time.Duration(200*i) * time.Millisecond)
		}
	}
	msg := fmt.Sprintf("FAILED to delete the %s at '%s'. It is STILL THERE after %d attempts.", what, path, attempts)
	if remedy != "" {
		msg += " " + remedy
	}
	return fmt.Errorf("%s Something is holding it open: WSL releases the disk asynchronously, and an "+
		"explorer window, a shell whose working directory is inside it, or an indexer will "+
		"each do it too.", msg)
}

// convertToSafeName makes a caller's name safe to be a path component.
func convertToSafeName(raw string) string {
	lower := strings.ToLower(raw)
	var b strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	s := b.String()
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", ".")
	}
	return strings.Trim(s, "-.")
}

// newDistroName draws a name from an image reference, or 'rootfs' from
// nothing.
func newDistroName(fromImage string) string {
	stem := "rootfs"
	if strings.TrimSpace(fromImage) != "" {
		stem = convertToSafeName(fromImage)
	}
	if strings.TrimSpace(stem) == "" {
		stem = "rootfs"
	}
	if len(stem) > 32 {
		stem = strings.Trim(stem[:32], "-.")
	}
	return prefix + stem + "-" + randomSuffix(4)
}

// resolveNewDistroName decides the name for a distro about to be created, and
// the ONE place that decides whether a collision is an error or a retry:
//
//	a name the CALLER gave   a collision is their answer being wrong, and
//	                         silently using a different name would be worse
//	                         than refusing. It refuses.
//	a name this tool DREW    a collision is a coin landing badly. Drawing
//	                         again is the whole answer.
//
// The registered list is read ONCE; re-reading per attempt would cost a
// wsl.exe call per draw to defend against a distro appearing in the
// microseconds between two draws, and --import refuses anyway if it happens.
func (s *session) resolveNewDistroName(requested, fromImage string) (string, error) {
	existing, err := s.distroNames()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(requested) != "" {
		named, err := s.resolveDistroName(requested, "")
		if err != nil {
			return "", err
		}
		for _, e := range existing {
			if e == named {
				return "", fmt.Errorf("Distro '%s' already exists. Choose another -Name or remove it first.", named)
			}
		}
		return named, nil
	}
	const attempts = 8
	for i := 1; i <= attempts; i++ {
		drawn := newDistroName(fromImage)
		taken := false
		for _, e := range existing {
			if e == drawn {
				taken = true
				break
			}
		}
		if !taken {
			return drawn, nil
		}
		s.log.warn(fmt.Sprintf("generated name '%s' is already taken; drawing again (%d of %d)", drawn, i, attempts))
	}
	return "", fmt.Errorf("Could not draw an unused distro name in %d attempts. The suffix is four "+
		"characters from a 36-symbol alphabet, so this should not happen with fewer than "+
		"a few hundred thousand 'eph-' distros registered. Check 'wsl --list --quiet', or "+
		"pass -Name yourself.", attempts)
}

// resolveDistroName prefix-forces a caller's name, or draws one from the image.
func (s *session) resolveDistroName(requested, fromImage string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		return newDistroName(fromImage), nil
	}
	n := convertToSafeName(requested)
	if strings.TrimSpace(n) == "" {
		return "", fmt.Errorf("Name '%s' sanitises to nothing.", requested)
	}
	if !strings.HasPrefix(n, prefix) {
		n = prefix + n
	}
	return n, nil
}
