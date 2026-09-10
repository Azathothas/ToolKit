// hooks.go - the commit-msg hook is present in the tree and installed here.
//
// ⛔ WHY THIS CHECK EXISTS AND WHY IT IS NOT OPTIONAL. `commits` is the only
// rule in this gate whose subject is `git log` rather than the tracked tree.
// Every session's procedure is "run the gate, then commit", so at the moment
// the gate runs, the commit being made DOES NOT EXIST. That rule reads only old
// commits and is always green. It has never once prevented the defect it names;
// it has only reported it afterwards, from the next gate run or from CI, by
// which time the commit is pushed. On a protected branch, undoing that costs a
// maintainer turning a branch protection off and on again.
//
// ⭐ It has now happened twice, and commits.go records both. The hook is the
// instrument that can actually refuse; this check is what stops the instrument
// from being one more thing a fresh checkout forgets.
//
// ⚠ A HOOK IS NOT CLONED. git will not install one for you and there is no
// setting in a tracked file that turns it on, which is exactly why a repository
// that merely SHIPS a hook has a preference rather than a rule.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// ⚠ THE EXECUTABLE BIT IS DELIBERATELY NOT CHECKED. git on Windows runs a
// hook through its bundled sh whatever the mode says, and the mode in the index
// is what a POSIX checkout restores. Reading the working copy's mode here would
// report a problem that does not exist for half the people who run this.

// HooksDir is where this repository keeps its tracked hooks, and the value
// core.hooksPath has to resolve to.
const HooksDir = ".githooks"

// hookInstall is the one command that fixes an uninstalled checkout. It is
// quoted verbatim in the finding, because a rule whose remedy the reader has to
// go and look up is a rule people work around.
const hookInstall = "git config core.hooksPath " + HooksDir

// Hooks refuses a tree whose commit-msg hook is missing, and a checkout that is
// not running it.
func Hooks(t *Tree) Result {
	r := Result{Extra: map[string]any{}}

	// 1. The hook is TRACKED. This half is a property of the tree and is
	// therefore checkable anywhere, including in a CI checkout.
	rel := HooksDir + "/commit-msg"
	tracked := false
	for _, f := range t.Files {
		if f == rel {
			tracked = true
			break
		}
	}
	if !tracked {
		r.bad("%s is not tracked, so there is nothing for a checkout to install", rel)
		r.Extra["installed"] = false
		return r
	}

	// 2. It calls the check rather than reimplementing it. ⛔ A hook holding a
	// SECOND copy of the rule is how the two drift, and the drift is invisible
	// until the day they disagree.
	if !strings.Contains(string(t.Read(rel)), "commit-msg \"$1\"") {
		r.bad("%s does not call `check commit-msg` on the message git handed it, so it holds its own copy of a rule that lives in tools/check", rel)
	}

	// 3. THIS CHECKOUT runs it. ⚠ Not a skip when the answer is inconvenient:
	// a checkout that is not running the hook is one where the rule is a
	// preference, and that is the finding, not an exemption.
	out, err := exec.Command("git", "-C", t.Root, "config", "--get", "core.hooksPath").Output()
	got := strings.TrimSpace(string(out))
	switch {
	case err != nil || got == "":
		r.bad("this checkout has no core.hooksPath, so %s is not running here. Fix it with: %s", rel, hookInstall)
		r.Extra["installed"] = false
	case !sameHookPath(t.Root, got):
		r.bad("core.hooksPath is %q and the tracked hooks are in %s, so this checkout runs someone else's hooks. Fix it with: %s", got, HooksDir, hookInstall)
		r.Extra["installed"] = false
	default:
		r.Extra["installed"] = true
	}
	return r
}

// sameHookPath compares what git reports against the tracked directory, with
// both made absolute. ⚠ git accepts a relative path, an absolute one, and on
// Windows either slash, so a string comparison against ".githooks" would refuse
// three spellings of a correct answer.
func sameHookPath(root, got string) bool {
	want, err := filepath.Abs(filepath.Join(root, HooksDir))
	if err != nil {
		return false
	}
	have := got
	if !filepath.IsAbs(have) {
		have = filepath.Join(root, have)
	}
	have, err = filepath.Abs(have)
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(want), filepath.Clean(have))
}
