// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// TestANamedInstanceReadsItsOwnConfigurationFirst is WSL-75's order: for a named
// instance its own file comes before a project's, --config still comes before both,
// and with no own file, or no instance, the project's file wins as it did.
func TestANamedInstanceReadsItsOwnConfigurationFirst(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "instances", "base")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	projectFile := filepath.Join(project, WorkingConfigName)
	ownFile := filepath.Join(home, "config.json")
	writeImage := func(path, image string) {
		t.Helper()
		body := `{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit-podbox","image":"` + image + `"}}`
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeImage(projectFile, "docker.io/library/debian:latest")
	writeImage(ownFile, "docker.io/library/alpine:latest")
	restore := chdir(t, project)
	defer restore()
	previous, previousExplicit := SelectedInstance, ExplicitConfigPath
	t.Cleanup(func() { SelectedInstance, ExplicitConfigPath = previous, previousExplicit })
	t.Setenv("WSL_TOOLKIT_HOME", home)
	SelectedInstance = Instance{Name: "podbox", Distro: "wsl-toolkit-podbox", Home: home}
	ExplicitConfigPath = ""

	loads := func(label, wantPath, wantImage string) {
		t.Helper()
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if !sameSourcePath(cfg.Path(), wantPath) || cfg.Base.Image != wantImage {
			t.Fatalf("%s: loaded %s with %s, want %s with %s", label, cfg.Path(), cfg.Base.Image, wantPath, wantImage)
		}
	}
	loads("a named instance with its own file", ownFile, "docker.io/library/alpine:latest")
	src, err := ResolveConfig()
	if err != nil || !strings.Contains(src.From, "instance's own configuration") {
		t.Fatalf("the source does not say why the instance's file won: %+v, %v", src, err)
	}

	ExplicitConfigPath = projectFile
	loads("--config over the instance's own file", projectFile, "docker.io/library/debian:latest")
	ExplicitConfigPath = ""

	if err := os.Remove(ownFile); err != nil {
		t.Fatal(err)
	}
	loads("a named instance with no own file", projectFile, "docker.io/library/debian:latest")
}

// grantBase is a configuration that accepts grants, with its file in a directory.
func grantBase(t *testing.T) Config {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Base.Automount = AutomountOff
	cfg.Base.Interop = BaseInteropOff
	cfg.path = filepath.Join(t.TempDir(), "config.json")
	return cfg
}

// TestAGrantIsRefusedWhereItWouldReplaceAnother holds PlanGrant's refusals and its
// one no-op: a target or a source already granted differently is refused, the same
// grant again changes nothing, and a base that can reach Windows drives is refused
// by the configuration's own rule.
func TestAGrantIsRefusedWhereItWouldReplaceAnother(t *testing.T) {
	cfg := grantBase(t)
	first, second := t.TempDir(), t.TempDir()
	change, err := PlanGrant(cfg, first, "/workspaces/first", "rw")
	if err != nil || change.Unchanged || len(change.Next.Base.Mounts) != 1 {
		t.Fatalf("a first grant answered %+v, %v", change, err)
	}
	cfg = change.Next
	if again, err := PlanGrant(cfg, first, "/workspaces/first", "rw"); err != nil || !again.Unchanged {
		t.Fatalf("the same grant again answered %+v, %v", again, err)
	}
	for label, attempt := range map[string]func() error{
		"another source at a granted target": func() error { _, err := PlanGrant(cfg, second, "/workspaces/first", "rw"); return err },
		"a granted source at another target": func() error { _, err := PlanGrant(cfg, first, "/workspaces/other", "rw"); return err },
		"a granted grant in another mode":    func() error { _, err := PlanGrant(cfg, first, "/workspaces/first", "ro"); return err },
	} {
		if err := attempt(); err == nil || !strings.Contains(err.Error(), "Revoke it first") {
			t.Errorf("%s answered %v, want a refusal naming base revoke", label, err)
		}
	}
	open := grantBase(t)
	open.Base.Automount = AutomountReadOnly
	if _, err := PlanGrant(open, first, "/workspaces/first", "rw"); err == nil || !strings.Contains(err.Error(), "automount") {
		t.Errorf("a grant into a base that mounts every drive answered %v", err)
	}
}

// TestARevokeTakesOneGrantAndKeepsTheRestAsWritten holds PlanRevoke: the named grant
// goes, a relative source left in the file keeps its spelling, and a target that
// is not granted is refused with the ones that are.
func TestARevokeTakesOneGrantAndKeepsTheRestAsWritten(t *testing.T) {
	cfg := grantBase(t)
	dir := filepath.Dir(cfg.path)
	if err := os.MkdirAll(filepath.Join(dir, "kept"), 0o755); err != nil {
		t.Fatal(err)
	}
	taken := t.TempDir()
	cfg.Base.Mounts = []BaseMount{
		{Source: "kept", Target: "/workspaces/kept", Mode: "ro"},
		{Source: taken, Target: "/workspaces/taken", Mode: "rw"},
	}
	change, err := PlanRevoke(cfg, "/workspaces/taken")
	if err != nil {
		t.Fatal(err)
	}
	if len(change.Next.Base.Mounts) != 1 || change.Next.Base.Mounts[0].Source != "kept" {
		t.Fatalf("revoking one grant left %+v", change.Next.Base.Mounts)
	}
	// ⚠ Against the resolved path: a temporary directory can be named through an 8.3
	// short form, and a grant's source is resolved to the long one.
	resolvedTaken, err := filepath.EvalSymlinks(taken)
	if err != nil {
		t.Fatal(err)
	}
	if !sameSourcePath(change.Mount.Source, resolvedTaken) || change.Mount.Mode != "rw" {
		t.Fatalf("the revoked grant was reported as %+v", change.Mount)
	}
	if _, err := PlanRevoke(cfg, "/workspaces/never"); err == nil || !strings.Contains(err.Error(), "/workspaces/kept, /workspaces/taken") {
		t.Fatalf("revoking a target never granted answered %v", err)
	}
}

// TestALiveGrantChangesTheBlockOnlyWhenNothingCanStillRefuse holds the order
// grants.sh keeps live, measured wrong on 2026-09-14: the removals come from the live
// mounts and run before the block is written, a failure after it puts the old block
// back, and write mode, which provisioning uses, writes the block and mounts nothing.
func TestALiveGrantChangesTheBlockOnlyWhenNothingCanStillRefuse(t *testing.T) {
	script := string(grantsScript)
	removal := strings.Index(script, `umount "$old"`)
	// ⚠ Write mode installs the block only where it changed, and says which. WSL-175.
	write := strings.Index(script, "if [ \"$TK_GRANTS_MODE\" = write ]; then\n")
	if !strings.Contains(script[max(write, 0):], "    install_fstab \"$WORK/fstab\"\n    printf 'fstab changed\\n'\n") {
		write = -1
	}
	live := strings.LastIndex(script, "\ninstall_fstab \"$WORK/fstab\"\n")
	if removal < 0 || write < 0 || live < 0 || live < removal {
		t.Fatalf("the live block is not written after the removals: removal at %d, write-mode block at %d, live block at %d", removal, write, live)
	}
	if !strings.Contains(script, "done < \"$WORK/live-targets\"") || strings.Contains(script, "old-targets") {
		t.Fatal("the removals are not read from the live mounts under /workspaces")
	}
	for _, refusal := range []string{`fail_back "$old is still in use`, `fail_back "$target could not be mounted`, `install_fstab "$WORK/fstab.before"`} {
		if !strings.Contains(script, refusal) {
			t.Errorf("grants.sh does not carry %s", refusal)
		}
	}
}

// TestADefaultTargetIsTheDirectorysOwnName holds the name a grant gets when none is
// passed: the directory's last element, with anything a target cannot carry turned
// into a dash.
func TestADefaultTargetIsTheDirectorysOwnName(t *testing.T) {
	for source, want := range map[string]string{
		filepath.Join("x", "ToolKit"):     "/workspaces/ToolKit",
		filepath.Join("x", "my project"):  "/workspaces/my-project",
		filepath.Join("x", "cafe & bar"):  "/workspaces/cafe-bar",
		filepath.Join("x", "repo.v2_old"): "/workspaces/repo.v2_old",
	} {
		if got, err := DefaultGrantTarget(source); err != nil || got != want {
			t.Errorf("%q answered %q (%v), want %q", source, got, err, want)
		}
	}
}

// TestALiveGrantChangeNamesItsOneTarget holds what grants.sh is handed: live, the
// one target the grant or revoke changes, and a live change with none refused.
// Write mode, which provisioning uses, names none. WSL-179.
func TestALiveGrantChangeNamesItsOneTarget(t *testing.T) {
	cfg := grantBase(t)
	cfg.Base.Mounts = []BaseMount{
		{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"},
	}
	env, err := grantsEnv(cfg, GrantsLive, BaseMount{Target: "/workspaces/mine"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := base64.StdEncoding.DecodeString(env["TK_GRANT_TARGET"]); err != nil || string(got) != "/workspaces/mine" {
		t.Fatalf("a live grant handed TK_GRANT_TARGET %q (%v), want /workspaces/mine", env["TK_GRANT_TARGET"], err)
	}
	if _, err := grantsEnv(cfg, GrantsLive, BaseMount{}); err == nil || !strings.Contains(err.Error(), "names its one target") {
		t.Fatalf("a live change with no target answered %v, want a refusal", err)
	}
	if env, err := grantsEnv(cfg, GrantsWrite, BaseMount{}); err != nil || env["TK_GRANT_TARGET"] != "" {
		t.Fatalf("write mode answered %q, %v; want no target", env["TK_GRANT_TARGET"], err)
	}
}

// grantsDrive runs grants.sh live under a POSIX shell, with /etc/fstab and
// /proc/mounts in dir and mount, umount and runuser standing in. A stand-in mount
// adds the target to the mounts file from the source and in the mode the fstab
// gives it, and fails once when dir holds mount.fail. umount fails once when dir
// holds umount.fail, and runuser fails while it holds runuser.fail. The grant
// ledger is dir's owners file, and the caller's configuration file is grantsMine.
func grantsDrive(t *testing.T, dir string, mounts []BaseMount, target string) (string, error) {
	t.Helper()
	return grantsDriveIn(t, dir, GrantsLive, mounts, target)
}

// grantsMine is the configuration file a drive of grants.sh runs for.
const grantsMine = "/cfg/mine.json"

// grantsDriveIn is grantsDrive in any mode.
func grantsDriveIn(t *testing.T, dir, mode string, mounts []BaseMount, target string) (string, error) {
	t.Helper()
	return grantsDriveAs(t, dir, mode, mounts, target, grantsMine)
}

// grantsDriveAs is grantsDriveIn for the configuration file owner. The grant's
// source is the one mounts names at target, and for a revoke, which names none,
// the one live at target: the directory the caller's configuration granted there.
func grantsDriveAs(t *testing.T, dir, mode string, mounts []BaseMount, target, owner string) (string, error) {
	t.Helper()
	want, _ := base64.StdEncoding.DecodeString(target)
	from := ""
	for _, m := range mounts {
		if m.Target == string(want) {
			from = fstabEscape(filepath.ToSlash(m.Source))
		}
	}
	if from == "" && len(want) > 0 {
		live, _ := os.ReadFile(filepath.Join(dir, "mounts"))
		for _, line := range strings.Split(string(live), "\n") {
			if f := strings.Fields(line); len(f) > 1 && f[1] == string(want) {
				from = f[0]
			}
		}
	}
	return grantsDriveFrom(t, dir, mode, mounts, target, owner, from)
}

// grantsDriveFrom is grantsDriveAs with the grant's source named, as fstab
// escapes it.
func grantsDriveFrom(t *testing.T, dir, mode string, mounts []BaseMount, target, owner, from string) (string, error) {
	t.Helper()
	source64 := ""
	if from != "" {
		source64 = b64(from)
	}
	fstab64, checks, err := renderBaseMounts(mounts)
	if err != nil {
		t.Fatal(err)
	}
	return grantsRun(t, dir, map[string]string{
		"TK_USER": "agent", "TK_GRANTS_MODE": mode,
		"TK_FSTAB_B64": fstab64, "TK_MOUNT_CHECKS": checks, "TK_GRANT_TARGET": target,
		"TK_GRANT_SOURCE": source64, "TK_GRANT_OWNER": b64(owner), "TK_GRANT_OWNER_NAME": b64(owner),
	})
}

// grantsStandIns are mount, umount and runuser, and the two commands that would
// touch the host. umount fails once when dir holds umount.fail, and on its Nth call
// when dir's umount.failat holds N. With umount.gone it takes the mount away
// before it fails, as when another process unmounted it first. WSL-189.
const grantsStandIns = `
mkdir() { :; }
chmod() { :; }
runuser() { [ ! -e "$TK_TEST_DIR/runuser.fail" ]; }
mount() {
  printf 'mount %s\n' "$1" >> "$TK_TEST_DIR/log"
  if [ -e "$TK_TEST_DIR/mount.fail" ]; then
    rm -f "$TK_TEST_DIR/mount.fail"
    printf 'the stand-in mount failed\n' >&2
    return 1
  fi
  m=$(grep " $1 drvfs " "$TK_TEST_DIR/fstab" | sed 's/.* drvfs metadata,\([a-z]*\),.*/\1/')
  s=$(grep " $1 drvfs " "$TK_TEST_DIR/fstab" | cut -d' ' -f1)
  printf '%s %s drvfs %s,noatime 0 0\n' "$s" "$1" "$m" >> "$TK_TEST_DIR/mounts"
}
umount() {
  printf 'umount %s\n' "$1" >> "$TK_TEST_DIR/log"
  umounts=$(($(cat "$TK_TEST_DIR/umount.count" 2>/dev/null || echo 0) + 1))
  printf '%s\n' "$umounts" > "$TK_TEST_DIR/umount.count"
  if [ -e "$TK_TEST_DIR/umount.fail" ] || [ "$(cat "$TK_TEST_DIR/umount.failat" 2>/dev/null || :)" = "$umounts" ]; then
    rm -f "$TK_TEST_DIR/umount.fail"
    if [ -e "$TK_TEST_DIR/umount.gone" ]; then
      grep -v " $1 drvfs " "$TK_TEST_DIR/mounts" > "$TK_TEST_DIR/mounts.new" || :
      mv "$TK_TEST_DIR/mounts.new" "$TK_TEST_DIR/mounts"
    fi
    printf 'the stand-in umount failed\n' >&2
    return 1
  fi
  grep -v " $1 drvfs " "$TK_TEST_DIR/mounts" > "$TK_TEST_DIR/mounts.new" || :
  mv "$TK_TEST_DIR/mounts.new" "$TK_TEST_DIR/mounts"
}
`

// grantsRun runs grants.sh under grantsShell, handed env as the guest hands it,
// with dir's fstab, mounts and owners files for the guest's and grantsStandIns.
//
// ⚠ FROM A FILE, AND WITH FORWARD SLASHES. A Windows command line holds 32767
// characters, which the script with its stand-ins nears, and a backslash in a path
// is an escape to the shell. WSL-189, WSL-190.
func grantsRun(t *testing.T, dir string, env map[string]string) (string, error) {
	t.Helper()
	sh, bin := grantsShell(t)
	at := filepath.ToSlash(dir)
	script := strings.ReplaceAll(string(grantsScript), "/etc/fstab", at+"/fstab")
	script = strings.ReplaceAll(script, "/proc/mounts", at+"/mounts")
	script = strings.ReplaceAll(script, "/var/lib/wsl-toolkit/grant-owners", at+"/owners")
	script = strings.ReplaceAll(script, "/var/lib/wsl-toolkit/grants.lock", at+"/grants.lock")
	script = strings.Replace(script, "set -eu\n", "set -eu\n"+grantsStandIns, 1)
	run := filepath.Join(dir, "grants-run.sh")
	if err := os.WriteFile(run, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, filepath.ToSlash(run))
	cmd.Env = append(shellEnv(bin), "TK_TEST_DIR="+at)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// grantsShell is the POSIX shell grants.sh and verify.sh are driven under, and on
// Windows the directory its tools come from, or a skip.
//
// ⭐ ON WINDOWS IT IS GIT FOR WINDOWS' sh, found beside the git on PATH. A drive
// needs sh, grep, sed, cut, awk, base64 and mktemp, which Git carries, and flock,
// which it does not and which grants.sh runs without. Skipped there, every WSL-189
// and WSL-190 case passed with its fix reverted. WSL-189, WSL-190.
//
// ⚠ KEPT ONCE FOUND, so a case that puts the fake host's PATH in place first still
// finds it. A miss is not kept.
func grantsShell(t *testing.T) (sh, bin string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		return posixShell(t), ""
	}
	gitShell.mu.Lock()
	defer gitShell.mu.Unlock()
	if gitShell.sh != "" {
		return gitShell.sh, filepath.Dir(gitShell.sh)
	}
	if git, err := exec.LookPath("git"); err == nil {
		dir := filepath.Dir(git)
		for range 3 {
			dir = filepath.Dir(dir)
			candidate := filepath.Join(dir, "usr", "bin", "sh.exe")
			if _, err := os.Stat(candidate); err == nil {
				gitShell.sh = candidate
				return candidate, filepath.Dir(candidate)
			}
		}
	}
	t.Skip("no Git for Windows sh beside the git on PATH, so no POSIX shell to drive the script under")
	return "", ""
}

// gitShell is the sh.exe grantsShell found.
var gitShell struct {
	mu sync.Mutex
	sh string
}

// shellEnv is this process's environment with bin first on PATH, where bin is set.
// ⚠ A later PATH wins over the Path Windows names it, since exec drops a key's
// earlier copies.
func shellEnv(bin string) []string {
	env := os.Environ()
	if bin != "" {
		env = append(env, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	return env
}

// TestALiveGrantOrRevokeLeavesAnotherCallersMount drives a base holding a mount
// another caller's configuration made. ⛔ A grant, then a revoke, from this caller
// unmounted it, because this configuration did not name it, and a grant mounted a
// grant this configuration named and another caller had revoked. WSL-179.
func TestALiveGrantOrRevokeLeavesAnotherCallersMount(t *testing.T) {
	posixShell(t)
	dir := t.TempDir()
	theirs := "drvfs /workspaces/theirs drvfs rw,noatime 0 0\n"
	if err := os.WriteFile(filepath.Join(dir, "mounts"), []byte(theirs), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fstab"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	mine := BaseMount{Source: src, Target: "/workspaces/mine", Mode: "rw"}
	named := BaseMount{Source: src, Target: "/workspaces/named", Mode: "ro"}
	own := base64.StdEncoding.EncodeToString([]byte(mine.Target))
	read := func(name string) string {
		t.Helper()
		body, _ := os.ReadFile(filepath.Join(dir, name))
		return string(body)
	}

	out, err := grantsDrive(t, dir, []BaseMount{mine, named}, own)
	if err != nil || !strings.Contains(out, "grants-complete") {
		t.Fatalf("the grant failed: %v\n%s", err, out)
	}
	log := read("log")
	if strings.Contains(log, "/workspaces/theirs") || !strings.Contains(read("mounts"), theirs) {
		t.Fatalf("a grant touched another caller's mount:\n%s", log)
	}
	if strings.Contains(log, "/workspaces/named") || !strings.Contains(log, "mount /workspaces/mine") {
		t.Fatalf("a grant mounted more or less than its own target:\n%s", log)
	}

	out, err = grantsDrive(t, dir, []BaseMount{named}, own)
	if err != nil || !strings.Contains(out, "grants-complete") {
		t.Fatalf("the revoke failed: %v\n%s", err, out)
	}
	log = read("log")
	if strings.Contains(log, "umount /workspaces/theirs") || !strings.Contains(read("mounts"), theirs) {
		t.Fatalf("a revoke unmounted another caller's mount:\n%s", log)
	}
	if !strings.Contains(log, "umount /workspaces/mine") || strings.Contains(read("mounts"), "/workspaces/mine") {
		t.Fatalf("the revoke did not unmount its own target:\n%s", log)
	}

	if out, err := grantsDrive(t, dir, []BaseMount{named}, ""); err == nil || !strings.Contains(out, "TK_GRANT_TARGET is required") {
		t.Fatalf("a live change with no target answered %v:\n%s", err, out)
	}
}

// TestALiveChangeThatMountedNothingIsTakenBackByItsBlockAlone holds how a grant
// whose file cannot be written is taken back. ⛔ A live step whose target another
// configuration had already mounted, as asked, mounted nothing, and the live
// take-back by the previous configuration unmounted that configuration's
// checkout. The block alone is taken back instead. WSL-179.
func TestALiveChangeThatMountedNothingIsTakenBackByItsBlockAlone(t *testing.T) {
	out := "  * explicit Windows directories: 2\ngrant rw /workspaces/shared\nunchanged /workspaces/shared\ngrants-complete\n"
	if liveGrantChanged(out, "/workspaces/shared") {
		t.Fatalf("a live step that printed unchanged read as a change:\n%s", out)
	}
	if !liveGrantChanged("  * mounted /workspaces/shared rw\ngrants-complete\n", "/workspaces/shared") {
		t.Fatal("a live step that mounted its target read as no change")
	}
	if !liveGrantChanged(out, "/workspaces/other") {
		t.Fatal("another target's unchanged line was read as this one's")
	}
	if got := grantsTakeBackMode(false); got != GrantsBlock {
		t.Fatalf("a step that mounted nothing is taken back in %q, want %q", got, GrantsBlock)
	}
	if got := grantsTakeBackMode(true); got != GrantsLive {
		t.Fatalf("a step that mounted its target is taken back in %q, want %q", got, GrantsLive)
	}
	cfg := grantBase(t)
	cfg.Base.Mounts = []BaseMount{
		{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"},
	}
	if env, err := grantsEnv(cfg, GrantsBlock, BaseMount{Target: "/workspaces/shared"}); err != nil || env["TK_GRANTS_MODE"] != GrantsBlock || env["TK_FSTAB_B64"] == "" {
		t.Fatalf("block mode answered %v, %v; want the block and no refusal", env, err)
	}
}

// TestATakeBackLeavesAMountItsChangeDidNotMake drives grants.sh over a target
// already mounted rw that the ledger gives this file, though its configuration no
// longer names it. A grant of the same target rw mounts nothing and says so, and
// the block take-back leaves the mount, the ledger, and rewrites the block. ⛔ The
// live take-back unmounted it. WSL-179.
func TestATakeBackLeavesAMountItsChangeDidNotMake(t *testing.T) {
	posixShell(t)
	dir := t.TempDir()
	src := t.TempDir()
	theirs := fstabEscape(filepath.ToSlash(src)) + " /workspaces/shared drvfs rw,noatime 0 0\n"
	owners := "/workspaces/shared " + b64(grantsMine) + " " + b64(grantsMine) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "mounts"), []byte(theirs), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fstab"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "owners"), []byte(owners), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := BaseMount{Source: src, Target: "/workspaces/mine", Mode: "rw"}
	shared := BaseMount{Source: src, Target: "/workspaces/shared", Mode: "rw"}
	own := base64.StdEncoding.EncodeToString([]byte(shared.Target))
	read := func(name string) string {
		t.Helper()
		body, _ := os.ReadFile(filepath.Join(dir, name))
		return string(body)
	}

	out, err := grantsDrive(t, dir, []BaseMount{mine, shared}, own)
	if err != nil || !strings.Contains(out, "grants-complete") {
		t.Fatalf("the grant failed: %v\n%s", err, out)
	}
	if liveGrantChanged(out, shared.Target) || read("log") != "" {
		t.Fatalf("a grant over a target already live as asked did not say it changed nothing:\n%s\nlog:\n%s", out, read("log"))
	}

	out, err = grantsDriveIn(t, dir, GrantsBlock, []BaseMount{mine}, own)
	if err != nil || !strings.Contains(out, "grants-complete") {
		t.Fatalf("the block take-back failed: %v\n%s", err, out)
	}
	if read("log") != "" || !strings.Contains(read("mounts"), theirs) || read("owners") != owners {
		t.Fatalf("the block take-back touched a live mount or the ledger:\nlog:\n%s\nmounts:\n%s\nowners:\n%s", read("log"), read("mounts"), read("owners"))
	}
	if fstab := read("fstab"); strings.Contains(fstab, " /workspaces/shared ") || !strings.Contains(fstab, " /workspaces/mine ") {
		t.Fatalf("the block take-back did not write the previous configuration's block:\n%s", fstab)
	}

	out, err = grantsDrive(t, dir, []BaseMount{mine, shared}, base64.StdEncoding.EncodeToString([]byte(mine.Target)))
	if err != nil || !liveGrantChanged(out, mine.Target) {
		t.Fatalf("a grant that mounted its target read as no change: %v\n%s", err, out)
	}
	out, err = grantsDrive(t, dir, []BaseMount{shared}, base64.StdEncoding.EncodeToString([]byte(mine.Target)))
	if err != nil || !liveGrantChanged(out, mine.Target) {
		t.Fatalf("a revoke that unmounted its target read as no change: %v\n%s", err, out)
	}
}
