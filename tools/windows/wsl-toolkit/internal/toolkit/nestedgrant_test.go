// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// -- WSL-202: a grant's target is made only where it names ------------------------------

// TestAConfigurationRefusesAGrantTargetInsideAnother validates configurations whose
// grants nest, and plans a grant inside another. WSL-202.
//
// ⛔ THE OUTER GRANT IS A WINDOWS DIRECTORY, WHERE THE ACCOUNT CAN MAKE LINKS. Root's
// mkdir and chmod of the inner target followed a link the account put on the way, and
// its mount resolved the target through it. A target that only shares a prefix with
// another is not inside it.
func TestAConfigurationRefusesAGrantTargetInsideAnother(t *testing.T) {
	src := t.TempDir()
	cfg := func(targets ...string) Config {
		c := grantBase(t)
		for _, target := range targets {
			c.Base.Mounts = append(c.Base.Mounts, BaseMount{Source: src, Target: target, Mode: "rw"})
		}
		return c
	}
	for _, nested := range [][]string{
		{"/workspaces/a", "/workspaces/a/b"},
		{"/workspaces/a/b/c", "/workspaces/a"},
		{"/workspaces/x", "/workspaces/a/b", "/workspaces/a/./b/c/"},
	} {
		err := cfg(nested...).Validate()
		if err == nil || !strings.Contains(err.Error(), "base.mounts nests") || !strings.Contains(err.Error(), "Grant it at another --target") {
			t.Errorf("base.mounts %q validated as %v; want a refusal naming the nesting", nested, err)
		}
	}
	for _, apart := range [][]string{
		{"/workspaces/a", "/workspaces/ab"},
		{"/workspaces/a/b", "/workspaces/a/c"},
	} {
		if err := cfg(apart...).Validate(); err != nil {
			t.Errorf("base.mounts %q, which do not nest, was refused: %v", apart, err)
		}
	}
	if _, err := PlanGrant(cfg("/workspaces/a"), t.TempDir(), "/workspaces/a/inner", "ro"); err == nil ||
		!strings.Contains(err.Error(), `nests "/workspaces/a/inner" inside "/workspaces/a"`) {
		t.Errorf("a grant inside another was planned: %v", err)
	}
}

// TestAGrantNestedWithAnotherFilesGrantIsRefused drives grants.sh where the ledger gives
// another configuration file a target that nests with this one's. WSL-202.
//
// ⛔ VALIDATION READS ONE FILE, AND THE LEDGER READS THEM ALL. A grant inside another
// file's live grant, or around a grant whose line the block holds, is refused before
// anything is made or mounted. ⚠ A row whose grant is neither live nor in the block
// refuses nothing, so a stale row costs no grant.
func TestAGrantNestedWithAnotherFilesGrantIsRefused(t *testing.T) {
	grantsShell(t)
	src, other := t.TempDir(), t.TempDir()
	theirs := "/cfg/theirs.json"
	inner := BaseMount{Source: src, Target: "/workspaces/outer/inner", Mode: "rw"}
	outerLive := liveLine(other, "/workspaces/outer", "rw")
	owners := "/workspaces/outer " + b64(theirs) + " " + b64(theirs) + "\n"
	dir := t.TempDir()
	read := grantsSeed(t, dir, "", outerLive, owners)
	out, err := grantsDrive(t, dir, []BaseMount{inner}, b64(inner.Target))
	if err == nil || !strings.Contains(out, "/workspaces/outer/inner and /workspaces/outer, which "+theirs+" grants, sit one inside the other") {
		t.Fatalf("a grant inside another file's live grant answered %v:\n%s", err, out)
	}
	if read("log") != "" || read("mounts") != outerLive || read("owners") != owners {
		t.Fatalf("a refused nested grant changed the base:\nlog:\n%s\nmounts:\n%s\nowners:\n%s", read("log"), read("mounts"), read("owners"))
	}

	// ⛔ AND AROUND ONE: a write whose target holds another file's grant in the block.
	theirsInner := BaseMount{Source: other, Target: "/workspaces/outer/inner", Mode: "ro"}
	block64, _, err := renderBaseMounts([]BaseMount{theirsInner})
	if err != nil {
		t.Fatal(err)
	}
	block, err := base64.StdEncoding.DecodeString(block64)
	if err != nil {
		t.Fatal(err)
	}
	fstab := "# wsl-toolkit mounts begin\n" + string(block) + "# wsl-toolkit mounts end\n"
	dir = t.TempDir()
	read = grantsSeed(t, dir, fstab, "", "/workspaces/outer/inner "+b64(theirs)+" "+b64(theirs)+"\n")
	outer := BaseMount{Source: src, Target: "/workspaces/outer", Mode: "rw"}
	out, err = grantsDriveIn(t, dir, GrantsWrite, []BaseMount{outer}, "")
	if err == nil || !strings.Contains(out, "/workspaces/outer and /workspaces/outer/inner, which "+theirs+" grants, sit one inside the other") {
		t.Fatalf("a write around another file's grant in the block answered %v:\n%s", err, out)
	}
	if read("fstab") != fstab {
		t.Fatalf("a refused write changed the block:\n%s", read("fstab"))
	}

	// ⭐ A STALE ROW: neither live nor in the block, so the write goes on.
	dir = t.TempDir()
	read = grantsSeed(t, dir, "", "", "/workspaces/outer/inner "+b64(theirs)+" "+b64(theirs)+"\n")
	out, err = grantsDriveIn(t, dir, GrantsWrite, []BaseMount{outer}, "")
	if err != nil || !strings.Contains(out, "grants-complete") {
		t.Fatalf("a stale ledger row refused a write: %v\n%s", err, out)
	}
	if !strings.Contains(read("fstab"), " /workspaces/outer drvfs ") {
		t.Fatalf("the write did not take its own target:\n%s", read("fstab"))
	}
}

// TestATargetInsideALiveWindowsMountIsRefused drives a grant whose target sits under a
// live DrvFS mount the ledger names for nobody, as a mount made before the ledger would
// be. WSL-202.
//
// ⛔ THE ACCOUNT CAN MAKE LINKS IN A WINDOWS DIRECTORY MOUNTED LIVE. Root's mkdir -p and
// chmod followed one there, so a target inside any live DrvFS mount is refused.
func TestATargetInsideALiveWindowsMountIsRefused(t *testing.T) {
	grantsShell(t)
	src, other := t.TempDir(), t.TempDir()
	dir := t.TempDir()
	live := liveLine(other, "/workspaces/old", "rw")
	read := grantsSeed(t, dir, "", live, "")
	inner := BaseMount{Source: src, Target: "/workspaces/old/inner", Mode: "ro"}
	out, err := grantsDrive(t, dir, []BaseMount{inner}, b64(inner.Target))
	if err == nil || !strings.Contains(out, "/workspaces/old/inner sits inside /workspaces/old, a Windows directory mounted live") {
		t.Fatalf("a grant inside a live Windows mount answered %v:\n%s", err, out)
	}
	if read("log") != "" || read("mounts") != live {
		t.Fatalf("a refused grant changed the base:\nlog:\n%s\nmounts:\n%s", read("log"), read("mounts"))
	}
}

// TestATargetIsMadeOnlyWhereItNames runs grants.sh's target_ready over real directories:
// a link on the way, a link at the target, a link to nothing, a file on the way, a live
// Windows mount above, a directory already there and one it makes. WSL-202.
//
// ⛔ ROOT'S mkdir -p AND chmod 755 FOLLOWED EVERY LINK, and chmod reached a directory
// that was already there, the live mount of its own grant too. Each refusal leaves what
// the link names as it was, and a directory root did not make keeps its mode.
func TestATargetIsMadeOnlyWhereItNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("grants.sh runs in a Linux base, on symbolic links and modes, so the Linux job runs this case")
	}
	sh := posixShell(t)
	dir := t.TempDir()
	ws, outside := filepath.Join(dir, "workspaces"), filepath.Join(dir, "outside")
	mounts := filepath.Join(dir, "mounts")
	for _, d := range []string{ws, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(mounts, []byte("D:/src "+filepath.Join(ws, "live")+" drvfs rw,noatime 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	section := strings.ReplaceAll(markedSection(t, grantsScript, "each target, where it names"), "/proc/mounts", mounts)
	script := "die() { printf 'die: %s\\n' \"$*\"; exit 3; }\n" + section + "target_ready \"$1\"\n"
	ready := func(target string) (string, bool) {
		t.Helper()
		out, err := exec.Command(sh, "-c", script, "sh", target).CombinedOutput()
		return string(out), err == nil
	}
	mode := func(p string) os.FileMode {
		t.Helper()
		info, err := os.Lstat(p)
		if err != nil {
			t.Fatal(err)
		}
		return info.Mode().Perm()
	}

	// ⭐ A NEW TARGET IS MADE, mode 755, and one already there keeps its mode.
	if out, ok := ready(filepath.Join(ws, "new", "deep")); !ok || mode(filepath.Join(ws, "new", "deep")) != 0o755 {
		t.Fatalf("a new target was not made 0755: %s", out)
	}
	kept := filepath.Join(ws, "kept")
	if err := os.Mkdir(kept, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(kept, 0o700); err != nil {
		t.Fatal(err)
	}
	if out, ok := ready(kept); !ok || mode(kept) != 0o700 {
		t.Fatalf("a directory root did not make was given %v: %s", mode(kept), out)
	}

	away := filepath.Join(outside, "away")
	if err := os.Mkdir(away, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(away, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{"via", "at"} {
		if err := os.Symlink(away, filepath.Join(ws, link)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(outside, "nothing"), filepath.Join(ws, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "file"), []byte("a file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string]string{
		filepath.Join(ws, "via", "inner"):  "is a link or not a directory",
		filepath.Join(ws, "at"):            "is a link or not a directory",
		filepath.Join(ws, "dangling"):      "is a link or not a directory",
		filepath.Join(ws, "file", "inner"): "is a link or not a directory",
		filepath.Join(ws, "live", "inner"): "sits inside " + filepath.Join(ws, "live") + ", a Windows directory mounted live",
	} {
		out, ok := ready(target)
		if ok || !strings.Contains(out, "die: ") || !strings.Contains(out, want) {
			t.Errorf("target %s answered %v, want a refusal saying %q:\n%s", target, ok, want, out)
		}
	}
	if entries, _ := os.ReadDir(away); len(entries) != 0 || mode(away) != 0o750 {
		t.Errorf("what a link names changed: %d entries, mode %v", len(entries), mode(away))
	}
	if _, err := os.Lstat(filepath.Join(outside, "nothing")); err == nil {
		t.Error("a target was made through a link to nothing")
	}
	if _, err := os.Lstat(filepath.Join(ws, "live", "inner")); err == nil {
		t.Error("a target was made inside a live Windows mount")
	}
}
