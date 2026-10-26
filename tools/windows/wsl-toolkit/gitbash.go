// SPDX-License-Identifier: 0BSD

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// Git Bash, and every MSYS program, rewrites an argument that begins with a slash
// into a Windows path before the program it starts can read it: `--dir /root/z`
// arrives as `C:/Program Files/Git/root/z`, and `/tmp/x` as the Windows temporary
// directory. A value meant for the GUEST then names a path the guest does not
// have, and the guest fails with 127 naming a path nobody typed. No code here can
// undo the rewrite, because it happens before `main` runs. The code can recognise
// it and refuse, naming the cause and the ways past it. WSL-99.

// guestPath is a flag whose value is a path in the guest. It can never hold a
// Windows path, so any drive letter followed by a forward slash is a rewrite.
const guestPath = true

// guestText is a flag whose value is text the guest reads, such as an
// environment value or an agent's argument. It may hold a Windows path on
// purpose, so only a value that begins where MSYS maps a mount is a rewrite.
const guestText = false

// gitBashRewrite refuses a guest value that MSYS rewrote. It answers nil
// wherever MSYSTEM is not set, because only an MSYS program rewrites.
//
// ⚠ IT IS FOR GUEST VALUES ONLY. A host path such as `--workspace` is rewritten
// on purpose and stays accepted.
func gitBashRewrite(flagName, value string, path bool) error {
	if strings.TrimSpace(os.Getenv("MSYSTEM")) == "" {
		return nil
	}
	v := value
	if name, rest, ok := strings.Cut(value, "="); ok && !path && toolkit.ValidEnvName(name) {
		v = rest
	}
	typed := msysOrigin(v)
	if typed == "" && !(path && isDriveSlashPath(v)) {
		return nil
	}
	of := ""
	if typed != "" {
		of = " of " + strings.TrimSuffix(value, v) + typed
	}
	return fmt.Errorf("%s %q is Git Bash's rewrite%s: MSYS turns an argument that begins with / into a Windows path before this program starts, "+
		"and the guest has no such path. Pass MSYS_NO_PATHCONV=1 and MSYS2_ARG_CONV_EXCL='*' on the call, "+
		"or pass the command as --command-base64 or --script. With the two set, a host path in MSYS's spelling, such as --workspace \"$PWD\", is still read", flagName, value, of)
}

// msysHostPath is a HOST path a caller in Git Bash wrote the MSYS way, read the
// way MSYS would have rewritten it: `/c/x` on drive C, `/tmp/x` in the temporary
// directory, and any other `/x` under the MSYS root. Anything else, and anything
// outside an MSYS program, is returned unchanged.
//
// ⭐ IT IS WHAT LETS A GIT BASH SCRIPT CALL THIS TOOL DIRECTLY. With
// MSYS_NO_PATHCONV=1 a guest path survives, and `--workspace "$PWD"` arrives as
// `/c/Users/...`, which Windows reads as a directory on the current drive's root.
// The podssh project carried a PowerShell bridge, a `win_path` and a scratch file
// per script to get past exactly that. WSL-111.
//
// ⛔ HOST PATHS ONLY. A guest value that begins with `/` is the guest's own path
// and is never translated.
func msysHostPath(value string) string {
	if strings.TrimSpace(os.Getenv("MSYSTEM")) == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return value
	}
	rest := value[1:]
	head, tail, _ := strings.Cut(rest, "/")
	if len(head) == 1 && ((head[0] >= 'a' && head[0] <= 'z') || (head[0] >= 'A' && head[0] <= 'Z')) {
		return strings.ToUpper(head) + ":/" + tail
	}
	if head == "tmp" {
		if t := forwardSlashed(os.Getenv("TEMP")); t != "" {
			return strings.TrimSuffix(t+"/"+tail, "/")
		}
		return value
	}
	if root := msysRoot(); root != "" {
		return root + "/" + rest
	}
	return value
}

// gitBashRewriteAny applies gitBashRewrite to each value of a repeatable flag
// or an argument list.
func gitBashRewriteAny(flagName string, values []string, path bool) error {
	for _, v := range values {
		if err := gitBashRewrite(flagName, v, path); err != nil {
			return err
		}
	}
	return nil
}

// isDriveSlashPath is a drive letter, a colon and a forward slash: the shape
// MSYS writes, and a shape no guest path has.
func isDriveSlashPath(v string) bool {
	if len(v) < 3 || v[1] != ':' || v[2] != '/' {
		return false
	}
	c := v[0]
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// msysOrigin is what the caller typed, where a value begins at a directory MSYS
// maps a mount to: its own root, where `/` goes, and the temporary directory,
// where `/tmp` goes. Empty where the value begins at neither.
func msysOrigin(v string) string {
	for _, m := range []struct{ at, mount string }{{msysRoot(), ""}, {forwardSlashed(os.Getenv("TEMP")), "/tmp"}} {
		if m.at == "" || len(v) < len(m.at) || !strings.EqualFold(v[:len(m.at)], m.at) {
			continue
		}
		rest := v[len(m.at):]
		if rest != "" && rest[0] != '/' {
			continue
		}
		return m.mount + "/" + strings.TrimLeft(rest, "/")
	}
	return ""
}

// settleHome rewrites WSL_TOOLKIT_HOME, whether --home or the caller's
// environment set it, as one absolute host path, and exports it again.
//
// ⛔ EXPORTED, NOT ONLY READ. The helper and every detached child inherit the
// variable, and a child resolving `/d/x` or a relative path on its own could
// land somewhere else. A path read once and handed on is one state root. WSL-187.
func settleHome() error {
	// ⛔ THE CACHE IS SETTLED WITH IT, at the one place both callers already
	// reach. WSL_TOOLKIT_CACHE went through filepath.Abs alone, so from Git Bash
	// `/d/cache` named `C:\d\cache`. WSL-195.
	if err := settleCache(); err != nil {
		return err
	}
	v := strings.TrimSpace(os.Getenv("WSL_TOOLKIT_HOME"))
	if v == "" {
		return nil
	}
	abs, err := filepath.Abs(msysHostPath(v))
	if err != nil {
		return fmt.Errorf("the state directory %q cannot be resolved: %w", v, err)
	}
	return os.Setenv("WSL_TOOLKIT_HOME", abs)
}

// settleCache rewrites WSL_TOOLKIT_CACHE as one absolute host path, read the way
// MSYS maps it, and exports it again, as settleHome does the state directory.
//
// ⭐ ONE CACHE PER HOST IS WHAT IT IS FOR. A cache named twice is fetched twice,
// and the BSD guest image it holds is 635 MB. WSL-195.
func settleCache() error {
	v := strings.TrimSpace(os.Getenv("WSL_TOOLKIT_CACHE"))
	if v == "" {
		return nil
	}
	cache, err := filepath.Abs(msysHostPath(v))
	if err != nil {
		return fmt.Errorf("the cache directory %q cannot be resolved: %w", v, err)
	}
	return os.Setenv("WSL_TOOLKIT_CACHE", cache)
}

// msysRoot is the directory MSYS maps `/` to, in the forward-slash form its
// rewrite writes, or empty where this environment does not say. Git Bash sets
// EXEPATH to that root's bin directory.
//
// ⚠ IT SPLITS ON BOTH SEPARATORS ITSELF. The value is a Windows path wherever
// the suite runs, and a path helper splits on the running host's separator.
func msysRoot() string {
	exe := strings.TrimRight(strings.TrimSpace(os.Getenv("EXEPATH")), `\/`)
	i := strings.LastIndexAny(exe, `\/`)
	if i <= 0 {
		return ""
	}
	return forwardSlashed(exe[:i])
}

// forwardSlashed is a Windows path in the form MSYS writes, or empty for one
// that is not drive-rooted.
func forwardSlashed(p string) string {
	p = strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(p), `\`, "/"), "/")
	if !isDriveSlashPath(p + "/") {
		return ""
	}
	return p
}
