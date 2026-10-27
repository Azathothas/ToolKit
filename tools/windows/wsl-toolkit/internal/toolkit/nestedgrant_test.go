// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
)

// -- WSL-202: a grant's target is made only where it names ------------------------------

// TestANewGrantInsideAnotherOrAroundOneIsRefused plans grants that would nest with one
// the configuration holds. WSL-202.
//
// ⛔ THE OUTER GRANT IS A WINDOWS DIRECTORY, WHERE THE ACCOUNT CAN MAKE LINKS. Root's
// mkdir and chmod of the inner target followed a link the account put on the way, and
// its mount resolved the target through it. A target that only shares a prefix with
// another is not inside it. ⚠ Validation lets a nesting file load, so PlanGrant is
// where a new one stops.
func TestANewGrantInsideAnotherOrAroundOneIsRefused(t *testing.T) {
	src := t.TempDir()
	cfg := func(targets ...string) Config {
		c := grantBase(t)
		for _, target := range targets {
			c.Base.Mounts = append(c.Base.Mounts, BaseMount{Source: src, Target: target, Mode: "rw"})
		}
		return c
	}
	for _, c := range []struct{ have, grant, said string }{
		{"/workspaces/a", "/workspaces/a/inner", `nests "/workspaces/a/inner" inside "/workspaces/a"`},
		{"/workspaces/a", "/workspaces/a/./b/c/", `nests "/workspaces/a/b/c" inside "/workspaces/a"`},
		{"/workspaces/a/b", "/workspaces/a", `nests "/workspaces/a/b" inside "/workspaces/a"`},
	} {
		if _, err := PlanGrant(cfg(c.have), t.TempDir(), c.grant, "ro"); err == nil ||
			!strings.Contains(err.Error(), c.said) || !strings.Contains(err.Error(), "Grant it at another --target") {
			t.Errorf("a grant at %s beside %s answered %v; want a refusal saying %s", c.grant, c.have, err, c.said)
		}
	}
	for _, apart := range [][2]string{{"/workspaces/a", "/workspaces/ab"}, {"/workspaces/a/b", "/workspaces/a/c"}} {
		if _, err := PlanGrant(cfg(apart[0]), t.TempDir(), apart[1], "ro"); err != nil {
			t.Errorf("a grant at %s beside %s, which do not nest, was refused: %v", apart[1], apart[0], err)
		}
	}
	if err := cfg("/workspaces/a", "/workspaces/a/b").Validate(); err != nil {
		t.Errorf("a file whose grants nest was refused by validation, so its revoke could not run: %v", err)
	}
}

// TestAConfigurationThatNestsLoadsAndNeverMountsTheInnerGrant loads a file an earlier
// release could write, whose grants nest, and asks everything that mounts. WSL-202.
//
// ⛔ REFUSED AT LOAD, EVERY COMMAND FAILED, `base revoke` INCLUDED, and the refusal
// named a remedy nobody could run. The fstab block still held both lines, so WSL's
// mount -a mounted the inner grant through whatever link the account left in the outer
// one. It loads now, marked as a gone source is: nothing renders its line, status names
// it with its revoke, and the revoke runs.
func TestAConfigurationThatNestsLoadsAndNeverMountsTheInnerGrant(t *testing.T) {
	cfg := grantBase(t)
	cfg.Base.Mounts = []BaseMount{
		{Source: t.TempDir(), Target: "/workspaces/a", Mode: "rw"},
		{Source: t.TempDir(), Target: "/workspaces/a/./b/", Mode: "ro"},
		{Source: t.TempDir(), Target: "/workspaces/ab", Mode: "ro"},
	}
	if err := cfg.WriteTo(cfg.Path()); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfigFile(cfg.Path(), "the case")
	if err != nil {
		t.Fatalf("a configuration whose grants nest did not load, so its revoke cannot run: %v", err)
	}
	mounts, err := loaded.ResolvedBaseMounts()
	if err != nil || len(mounts) != 3 || mounts[0].Inside != "" || mounts[1].Inside != "/workspaces/a" || mounts[2].Inside != "" {
		t.Fatalf("the grants resolved to %+v, %v; want /workspaces/a/b alone marked inside /workspaces/a", mounts, err)
	}
	// ⛔ NOTHING THAT MOUNTS RENDERS ITS LINE OR READS IT BACK.
	for label, render := range map[string]func() (string, string, error){
		"base ensure": func() (string, string, error) {
			f, c, _, err := baseMountPayloads(loaded)
			return f, c, err
		},
		"base grant, base revoke and the verifier": func() (string, string, error) { return liveMountPayloads(loaded) },
	} {
		fstab64, checks, err := render()
		fstab, _ := base64.StdEncoding.DecodeString(fstab64)
		if err != nil || strings.Contains(string(fstab), " /workspaces/a/b ") || strings.Contains(checks, b64("/workspaces/a/b")) ||
			!strings.Contains(string(fstab), " /workspaces/a ") || !strings.Contains(string(fstab), " /workspaces/ab ") {
			t.Errorf("%s renders %q and checks %q (%v); want every grant but the inner one", label, fstab, checks, err)
		}
	}
	// ⭐ STATUS NAMES IT WITH ITS REVOKE, and base ensure exits 1 over it.
	st, err := (&Base{cfg: loaded, home: t.TempDir()}).configuredState()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Problems) != 1 || !strings.Contains(st.Problems[0], "the grant at /workspaces/a/b sits inside the grant at /workspaces/a") ||
		!strings.Contains(st.Problems[0], "base revoke --target /workspaces/a/b") ||
		len(st.Remediations) != 1 || st.Remediations[0].ID != "grant-nested" || !strings.Contains(st.Remediations[0].Command, "base revoke --target /workspaces/a/b") {
		t.Fatalf("status reads %q %+v; want the nested grant named with its revoke", st.Problems, st.Remediations)
	}
	// ⛔ base grant REFUSES BESIDE IT, as beside a gone grant, and names the revoke.
	if _, err := PlanGrant(loaded, t.TempDir(), "/workspaces/c", "ro"); !errors.Is(err, ErrGrantNested) ||
		!strings.Contains(err.Error(), "base revoke --target /workspaces/a/b") {
		t.Errorf("a grant beside the nested one answered %v; want the refusal naming its revoke", err)
	}
	// ⭐ THE REVOKE RUNS, and what it leaves nests nothing.
	change, err := PlanRevoke(loaded, "/workspaces/a/b")
	if err != nil {
		t.Fatalf("revoking the nested grant was refused: %v", err)
	}
	left, err := change.Next.ResolvedBaseMounts()
	if err != nil || len(left) != 2 || NestedGrantsError(left) != nil {
		t.Fatalf("the revoke left %+v, %v", left, err)
	}
	// ⚠ THE MARK IS THE READER'S, so neither a stored file nor the helper's copy holds it.
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	if err := loaded.Write(); err != nil {
		t.Fatal(err)
	}
	home, err := EnsureHome()
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := os.ReadFile(filepath.Join(home, "config.json"))
	portable, _ := json.Marshal(loaded.Portable().Base.Mounts)
	if strings.Contains(string(stored), `"inside"`) || strings.Contains(string(portable), `"inside"`) || !strings.Contains(string(stored), "/workspaces/a/b") {
		t.Fatalf("a stored or a sent configuration carries the mark, or lost the grant:\n%s\n%s", stored, portable)
	}
	// ⚠ A RE-PROVISION THAT grants.sh REFUSED OVER ONE keeps that refusal, which names
	// the revoke, rather than naming a rebuild.
	nested := fmt.Errorf("the grants step in base: %w: it sits inside another", ErrGrantNested)
	if got := reprovisionFailed("base", nested); got != nested {
		t.Errorf("the nested refusal %v was rewritten as %v", nested, got)
	}
}

// TestAGrantNestedWithAnotherFilesGrantIsRefused drives grants.sh where the ledger gives
// another configuration file a target that nests with this one's. WSL-202.
//
// ⛔ PlanGrant READS ONE FILE, AND THE LEDGER READS THEM ALL. A live grant inside another
// file's live grant, or around a grant whose line the block holds, is refused before
// anything is made or mounted. ⭐ A write around one leaves that file's inner line out
// of the block and names its revoke. ⚠ A row whose grant is neither live nor in the block
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

	// ⛔ AND AROUND ONE: a live grant whose target holds another file's grant in the block.
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
	out, err = grantsDrive(t, dir, []BaseMount{outer}, b64(outer.Target))
	if err == nil || !strings.Contains(out, "/workspaces/outer and /workspaces/outer/inner, which "+theirs+" grants, sit one inside the other") {
		t.Fatalf("a live grant around another file's grant in the block answered %v:\n%s", err, out)
	}
	if read("fstab") != fstab || read("log") != "" {
		t.Fatalf("a refused grant changed the block or the mounts:\n%s\nlog:\n%s", read("fstab"), read("log"))
	}

	// ⭐ A WRITE AROUND ONE GOES ON, and the block loses the other file's inner line,
	// which WSL's mount -a resolved through the outer grant at every start. WSL-202.
	out, err = grantsDriveIn(t, dir, GrantsWrite, []BaseMount{outer}, "")
	if err != nil || !strings.Contains(out, "grants-complete") ||
		!strings.Contains(out, "left /workspaces/outer/inner out of the fstab block: it sits inside /workspaces/outer") ||
		!strings.Contains(out, "Run: wsl-toolkit --config "+theirs+" base revoke --target /workspaces/outer/inner") {
		t.Fatalf("a write around another file's grant in the block answered %v:\n%s", err, out)
	}
	if len(blockLinesAt(t, dir, theirsInner.Target)) != 0 || len(blockLinesAt(t, dir, outer.Target)) != 1 {
		t.Fatalf("the write kept the inner line, or lost its own:\n%s", read("fstab"))
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
	if err := forksafe.WriteFile(mounts, []byte("D:/src "+filepath.Join(ws, "live")+" drvfs rw,noatime 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	section := strings.ReplaceAll(markedSection(t, grantsScript, "each target, where it names"), "/proc/mounts", mounts)
	script := "die() { printf 'die: %s\\n' \"$*\"; exit 3; }\n" +
		"cd() { case \"$2\" in */shut) return 1 ;; esac; command cd \"$@\"; }\n" + section + "target_ready \"$1\"\n"
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
	if err := forksafe.WriteFile(filepath.Join(ws, "file"), []byte("a file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// ⚠ EACH REFUSAL SAYS WHAT STANDS THERE. Every one read "a link or not a directory"
	// and said to remove it, of a real directory under a linked parent too. WSL-202.
	linked := filepath.Join(dir, "linked")
	if err := os.Symlink(ws, linked); err != nil {
		t.Fatal(err)
	}
	refused := map[string]string{
		filepath.Join(ws, "via", "inner"):  filepath.Join(ws, "via") + " is a link, so",
		filepath.Join(ws, "at"):            filepath.Join(ws, "at") + " is a link, so",
		filepath.Join(ws, "dangling"):      filepath.Join(ws, "dangling") + " is a link, so",
		filepath.Join(ws, "file", "inner"): filepath.Join(ws, "file") + " is not a directory, so",
		filepath.Join(ws, "live", "inner"): "sits inside " + filepath.Join(ws, "live") + ", a Windows directory mounted live",
		filepath.Join(linked, "kept", "x"): filepath.Join(linked, "kept") + " resolves to " + kept + ", because " + linked + " is a link, so",
	}
	// ⚠ ROOT ENTERS A DIRECTORY OF MODE 000, so the case's cd refuses one by its name, as
	// a Windows mount root that root cannot enter refuses it.
	shut := filepath.Join(ws, "shut")
	if err := os.Mkdir(shut, 0o755); err != nil {
		t.Fatal(err)
	}
	refused[filepath.Join(shut, "x")] = shut + " is a directory root could not enter, so"
	for target, want := range refused {
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

// TestABlockThatNestsLosesItsInnerLine drives grants.sh over a block an earlier release
// wrote, whose lines nest: two other files' lines, and this file's line inside another
// file's. WSL-202.
//
// ⛔ THE BLOCK KEPT BOTH LINES UNTIL SOMEONE REVOKED ONE, and WSL's mount -a mounted the
// inner grant through whatever link the account left in the outer one at every start.
// Every mode now writes the block without the inner line and names the revoke. Write mode
// refuses once it has, over a line of its own file, and a revoke beside such a grant runs.
func TestABlockThatNestsLosesItsInnerLine(t *testing.T) {
	grantsCase(t)
	outer := BaseMount{Source: t.TempDir(), Target: "/workspaces/x", Mode: "rw"}
	inner := BaseMount{Source: t.TempDir(), Target: "/workspaces/x/y", Mode: "ro"}
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"}
	const innerFile = "/cfg/inner.json"
	old := blockOf(blockLine(t, outer), blockLine(t, inner))
	owners := ownedBy(outer.Target, grantsTheirs) + ownedBy(inner.Target, innerFile)
	revoke := "Run: wsl-toolkit --config " + innerFile + " base revoke --target /workspaces/x/y"
	for _, mode := range []string{GrantsWrite, GrantsBlock} {
		dir := t.TempDir()
		read := grantsSeed(t, dir, old, "", owners)
		out, err := grantsDriveIn(t, dir, mode, []BaseMount{mine}, "")
		if err != nil || !strings.Contains(out, "grants-complete") || !strings.Contains(out, revoke) ||
			!strings.Contains(out, "left /workspaces/x/y out of the fstab block: it sits inside /workspaces/x") {
			t.Fatalf("%s over a block that nests answered %v:\n%s", mode, err, out)
		}
		if len(blockLinesAt(t, dir, inner.Target)) != 0 || len(blockLinesAt(t, dir, outer.Target)) != 1 || len(blockLinesAt(t, dir, mine.Target)) != 1 {
			t.Fatalf("%s kept the inner line, or lost another:\n%s", mode, read("fstab"))
		}
		// ⭐ THE NEXT START MOUNTS THE OUTER GRANT, AND NOT THE INNER ONE.
		grantsRestart(t, dir)
		if strings.Contains(read("mounts"), " "+inner.Target+" ") || !strings.Contains(read("mounts"), " "+outer.Target+" ") {
			t.Fatalf("after %s a restart mounted:\n%s", mode, read("mounts"))
		}
	}

	// ⛔ THIS FILE'S LINE INSIDE ANOTHER FILE'S GRANT: the block and the ledger lose it,
	// and the write refuses once they have, naming this file's revoke.
	dir := t.TempDir()
	read := grantsSeed(t, dir, blockOf(blockLine(t, outer), blockLine(t, inner), blockLine(t, mine)), "",
		ownedBy(outer.Target, grantsTheirs)+ownedBy(inner.Target, grantsMine)+ownedBy(mine.Target, grantsMine))
	out, err := grantsDriveIn(t, dir, GrantsWrite, []BaseMount{inner, mine}, "")
	want := "grants: /workspaces/x/y and /workspaces/x, which " + grantsTheirs + " grants, sit one inside the other, where the account can make links on the way, " +
		"so /workspaces/x/y is not in the fstab block and is not mounted. Run: wsl-toolkit --config " + grantsMine + " base revoke --target /workspaces/x/y"
	if err == nil || !strings.Contains(out, want) {
		t.Fatalf("a write of a grant inside another file's answered %v:\n%s", err, out)
	}
	if len(blockLinesAt(t, dir, inner.Target)) != 0 || len(blockLinesAt(t, dir, outer.Target)) != 1 || len(blockLinesAt(t, dir, mine.Target)) != 1 {
		t.Fatalf("the refused write kept its inner line, or lost another:\n%s", read("fstab"))
	}
	if strings.Contains(read("owners"), inner.Target+" ") || !strings.Contains(read("owners"), mine.Target+" ") {
		t.Fatalf("the ledger kept the grant it left out, or lost another:\n%s", read("owners"))
	}
	// ⚠ THE REFUSAL REACHES base ensure AS ONE THAT NAMES THE REVOKE, and not a rebuild.
	said := strings.TrimPrefix(want, "grants: ")
	if err := grantsStepError("base", said); !errors.Is(err, ErrGrantNested) || reprovisionFailed("base", err) != err {
		t.Fatalf("the grants step's refusal read as %v, and a failed re-provision as %v", err, reprovisionFailed("base", err))
	}
	if err := grantsStepError("base", "could not make /workspaces/z"); errors.Is(err, ErrGrantNested) {
		t.Fatalf("another refusal of the grants step read as a nested grant: %v", err)
	}

	// ⛔ AND WHERE THE OTHER FILE'S GRANT IS LIVE AND ITS LINE GONE, which the block's own
	// read cannot see: this file's line is still left out.
	dir = t.TempDir()
	read = grantsSeed(t, dir, blockOf(blockLine(t, inner), blockLine(t, mine)), liveLine(outer.Source, outer.Target, "rw"),
		ownedBy(outer.Target, grantsTheirs)+ownedBy(inner.Target, grantsMine)+ownedBy(mine.Target, grantsMine))
	out, err = grantsDriveIn(t, dir, GrantsWrite, []BaseMount{inner, mine}, "")
	if err == nil || !strings.Contains(out, want) {
		t.Fatalf("a write of a grant inside another file's live grant answered %v:\n%s", err, out)
	}
	if len(blockLinesAt(t, dir, inner.Target)) != 0 || len(blockLinesAt(t, dir, mine.Target)) != 1 {
		t.Fatalf("the refused write kept its inner line, or lost another:\n%s", read("fstab"))
	}

	// ⭐ A REVOKE BESIDE SUCH A GRANT RUNS. It refused over the grant it was not changing,
	// as the gone grants of WSL-106 once each refused the other's revoke.
	dir = t.TempDir()
	gone := BaseMount{Source: t.TempDir(), Target: "/workspaces/gone", Mode: "rw"}
	read = grantsSeed(t, dir, blockOf(blockLine(t, outer), blockLine(t, inner), blockLine(t, gone)),
		liveLine(outer.Source, outer.Target, "rw")+liveLine(gone.Source, gone.Target, "rw"),
		ownedBy(outer.Target, grantsTheirs)+ownedBy(inner.Target, grantsMine)+ownedBy(gone.Target, grantsMine))
	out, err = grantsDrive(t, dir, []BaseMount{inner}, b64(gone.Target))
	if err != nil || !strings.Contains(out, "grants-complete") || strings.Contains(read("mounts"), " "+gone.Target+" ") {
		t.Fatalf("a revoke beside a grant inside another file's answered %v:\n%s\nmounts:\n%s", err, out, read("mounts"))
	}
	if len(blockLinesAt(t, dir, inner.Target)) != 0 || len(blockLinesAt(t, dir, gone.Target)) != 0 {
		t.Fatalf("the revoke's block kept a line it should not:\n%s", read("fstab"))
	}
}

// TestTheVerifierNamesABlockThatNests runs verify.sh's read of the fstab block over a
// block whose lines nest, one that does not, and no fstab at all. WSL-202.
//
// ⛔ A BLOCK AN EARLIER RELEASE WROTE CAN HOLD A NESTED PAIR, and a probe that passed over
// it left that pair to WSL's mount -a at every start. It names the inner line with the
// revoke the ledger gives, or the file to edit, and its refusal sends base ensure on to
// write the block again.
func TestTheVerifierNamesABlockThatNests(t *testing.T) {
	sh, bin := grantsShell(t)
	run := func(fstab, owners string) (string, error) {
		t.Helper()
		dir := t.TempDir()
		at := shellPath(t, dir)
		if fstab != "" {
			if err := forksafe.WriteFile(filepath.Join(dir, "fstab"), []byte(fstab), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := forksafe.WriteFile(filepath.Join(dir, "owners"), []byte(owners), 0o644); err != nil {
			t.Fatal(err)
		}
		script := strings.ReplaceAll(markedSection(t, verifyScript, "the block's nesting"), "/etc/fstab", at+"/fstab")
		script = strings.ReplaceAll(script, "/var/lib/wsl-toolkit/grant-owners", at+"/owners")
		file := filepath.Join(dir, "nest.sh")
		if err := forksafe.WriteFile(file, []byte("set -eu\n"+script+"echo passed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(sh, filepath.ToSlash(file))
		cmd.Env = shellEnv(bin)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	outer := BaseMount{Source: t.TempDir(), Target: "/workspaces/x", Mode: "rw"}
	inner := BaseMount{Source: t.TempDir(), Target: "/workspaces/x/y", Mode: "ro"}
	apart := BaseMount{Source: t.TempDir(), Target: "/workspaces/xy", Mode: "ro"}
	nested := "/etc/hosts-like line\n" + blockOf(blockLine(t, inner), blockLine(t, outer))
	out, err := run(nested, ownedBy(inner.Target, "/cfg/inner.json"))
	if err == nil || !strings.Contains(out, "verify: the fstab block mounts /workspaces/x/y inside /workspaces/x") ||
		!strings.Contains(out, "Run: wsl-toolkit --config /cfg/inner.json base revoke --target /workspaces/x/y") {
		t.Fatalf("a block that nests answered %v:\n%s", err, out)
	}
	if out, err := run(nested, ""); err == nil || !strings.Contains(out, "Run base ensure, which writes the block without it, or remove its line from") {
		t.Fatalf("a block that nests with no ledger row answered %v:\n%s", err, out)
	}
	for label, fstab := range map[string]string{
		"a block that does not nest": blockOf(blockLine(t, outer), blockLine(t, apart)),
		"no fstab":                   "",
	} {
		if out, err := run(fstab, ""); err != nil || !strings.Contains(out, "passed") {
			t.Errorf("%s answered %v:\n%s", label, err, out)
		}
	}
}
