package toolkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Home is where everything this executable owns lives on the host.
//
// ⛔ NOT the script's directory. wsl-toolkit.ps1's Purge removes every distro
// under %LOCALAPPDATA%\wsl-ephemeral, and the base has to survive that.
//
//	<home>/config.json     the image catalog and base settings
//	<home>/base/           the base distro's disk, the wsl --import target
//	<home>/jobs/<id>/      one job's scratch, archive and transcript
//	<home>/ledger.jsonl    what this executable created, so cleanup can find it
//	<home>/helper.json     the local helper's endpoint and token
func Home() (string, error) {
	if v := strings.TrimSpace(os.Getenv("WSL_TOOLKIT_HOME")); v != "" {
		return filepath.Abs(v)
	}
	if runtime.GOOS == "windows" {
		if v := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); v != "" {
			return filepath.Join(v, "wsl-toolkit"), nil
		}
		return "", errors.New("LOCALAPPDATA is not set; pass --home or set WSL_TOOLKIT_HOME")
	}
	if v := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); v != "" {
		return filepath.Join(v, "wsl-toolkit"), nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no home directory and WSL_TOOLKIT_HOME is not set: %w", err)
	}
	return filepath.Join(h, ".local", "state", "wsl-toolkit"), nil
}

// EnsureHome creates the state directory. Read-only commands must not call it:
// a report that creates a directory on a machine it is only describing has
// changed the thing it was asked to measure.
func EnsureHome() (string, error) {
	h, err := Home()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(h, 0o700); err != nil {
		return "", err
	}
	return h, nil
}

// ErrOutsideRoot is what every containment refusal wraps, so a caller can tell
// a refusal from an ordinary IO failure without reading a message.
var ErrOutsideRoot = errors.New("outside the directory this tool owns")

// ResolveInside answers where a path is, and refuses anything that is not a
// strict descendant of root.
//
// ⛔ Links are resolved on BOTH sides before comparing. A prefix test over
// unresolved paths passes for a link inside root that points anywhere at all.
// Where the target does not exist yet, the nearest existing ancestor is
// resolved, because that is the directory a create would happen in.
func ResolveInside(root, path string) (string, error) {
	realRoot, err := resolveExisting(root)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %s: %w", root, err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	real, err := resolveExisting(abs)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %s: %w", path, err)
	}
	if pathEqual(real, realRoot) {
		return "", fmt.Errorf("%q is the root itself, which is %w", path, ErrOutsideRoot)
	}
	if !hasPathPrefix(real, realRoot) {
		return "", fmt.Errorf("%q resolves to %q, which is %w %q", path, real, ErrOutsideRoot, realRoot)
	}
	return real, nil
}

// resolveExisting follows symlinks as far as the path exists, then re-appends
// the parts that do not exist yet.
func resolveExisting(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var tail []string
	cur := abs
	for {
		resolved, err := RealPath(cur)
		if err == nil {
			return filepath.Join(append([]string{resolved}, tail...)...), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			// Reached the volume root without finding anything that exists.
			return filepath.Join(append([]string{cur}, tail...)...), nil
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

func pathEqual(a, b string) bool {
	a = strings.TrimRight(a, `\/`)
	b = strings.TrimRight(b, `\/`)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func hasPathPrefix(path, prefix string) bool {
	prefix = strings.TrimRight(prefix, `\/`) + string(filepath.Separator)
	if runtime.GOOS == "windows" {
		return strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix))
	}
	return strings.HasPrefix(path, prefix)
}

// RemoveInside is THE deletion in this executable, and every removal of state
// this tool owns goes through it.
//
// ⛔ The containment guard runs INSIDE it rather than beside each caller: a
// guard applied at four call sites is a guard that will one day be applied at
// three. TODO/RULES.md section 3.
//
// It reads the state back and reports what is true rather than what was
// attempted.
//
// ⚠ WHERE THE LINE IS, because the tree does hold a handful of plain
// os.Remove calls and reading them as violations would be wrong. This helper
// answers "is the path a caller reached me with inside the tree I own". The
// rollback half of a write is a different operation: `os.Rename(tmp, path)`
// fails and the same function removes the exact `tmp` it created two lines
// above. There is no caller-supplied path to contain, and the only root such a
// call could pass is the file's own directory, which makes the check
// vacuous - a guard that cannot refuse anything is theatre, and theatre is what
// this file exists to avoid rather than to spread.
//
// So: state that outlives the call goes through here. A temporary this
// function created, and removes on the failure path of creating it, does not.
func RemoveInside(root, path string) error {
	target, err := ResolveInside(root, path)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("%s is still on disk after the removal", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot tell whether %s was removed: %w", target, err)
	}
	return nil
}

// CacheDir is where a large download lives so it is fetched ONCE per user.
//
// ⛔ IT IS DELIBERATELY NOT PER-INSTANCE, and that is the whole point. `Home()`
// moves to `<root>/instances/<name>` when `--instance` selects one, so an asset
// stored under it is fetched again by every agent that runs under a different
// instance. The BSD guest image is 635 MB compressed and about 6 GB expanded;
// paying that per instance is the waste this exists to remove.
//
// ⚠ STILL UNDER THE STATE ROOT, so `resources` and `gc` walk one tree and
// nothing here is state they cannot find. That is the same ruling instanceHome
// carries, applied to a directory that is shared rather than isolated.
//
// ⭐ A caller who moved the state directory moves the cache with it, which is
// what moving a state directory is normally for. `WSL_TOOLKIT_CACHE` overrides
// both.
func CacheDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("WSL_TOOLKIT_CACHE")); v != "" {
		return filepath.Abs(v)
	}
	h, err := Home()
	if err != nil {
		return "", err
	}
	// Climb out of `instances/<name>` when this process is running under one.
	if filepath.Base(filepath.Dir(h)) == "instances" {
		h = filepath.Dir(filepath.Dir(h))
	}
	return filepath.Join(h, "cache"), nil
}
