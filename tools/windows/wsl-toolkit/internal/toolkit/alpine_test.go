// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// -- WSL-198: a grant on an Alpine base survives a restart, in its mode ---------

// mountHelperIn is mount-helper.sh with WSL's helper, the helper it writes and
// /bin/mount under dir. They are dir's mount.drvfs and bin/mount.drvfs, and dir's
// busybox where dir holds one, or its mounts file where it does not. Each is a
// file that is there, so any readlink resolves it.
func mountHelperIn(t *testing.T, dir string) string {
	t.Helper()
	at := shellPath(t, dir)
	mountBin := at + "/mounts"
	if _, err := os.Stat(filepath.Join(dir, "busybox")); err == nil {
		mountBin = at + "/busybox"
	}
	s := string(mountHelperScript)
	for _, r := range [][2]string{
		{"HELPER=/sbin/mount.drvfs\n", "HELPER=" + at + "/mount.drvfs\n"},
		{"WRAPPER=/usr/local/bin/mount.drvfs\n", "WRAPPER=" + at + "/bin/mount.drvfs\n"},
		{"MOUNT_BIN=/bin/mount\n", "MOUNT_BIN=" + mountBin + "\n"},
	} {
		if n := strings.Count(s, r[0]); n != 1 {
			t.Fatalf("mount-helper.sh carries %q %d times, want once", r[0], n)
		}
		s = strings.Replace(s, r[0], r[1], 1)
	}
	return s
}

// mountHelperText is the helper mount-helper.sh writes, whole.
func mountHelperText(t *testing.T) string {
	t.Helper()
	const open, end = "<<'WSL_TOOLKIT_MOUNT_HELPER'\n", "\nWSL_TOOLKIT_MOUNT_HELPER\n"
	s := string(mountHelperScript)
	i, j := strings.Index(s, open), strings.Index(s, end)
	if i < 0 || j < i {
		t.Fatal("mount-helper.sh no longer writes its helper from one quoted here-document")
	}
	return s[i+len(open) : j+1]
}

// installMountHelper runs mount-helper.sh as mountHelperIn makes it, and answers
// what it said and its exit.
func installMountHelper(t *testing.T, dir string) (string, int) {
	t.Helper()
	sh, bin := grantsShell(t)
	run := filepath.Join(dir, "mount-helper-run.sh")
	if err := os.WriteFile(run, []byte(mountHelperIn(t, dir)), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, filepath.ToSlash(run))
	cmd.Env = shellEnv(bin)
	out, err := cmd.CombinedOutput()
	return string(out), exitOf(t, err)
}

// stagedBusyBoxBase is a directory with WSL's helper and a BusyBox /bin/mount.
func stagedBusyBoxBase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stage(t, dir, "mount.drvfs")
	stage(t, dir, "busybox")
	stage(t, dir, "mounts")
	return dir
}

// ⛔ THE HELPER IS WRITTEN WHERE BUSYBOX'S MOUNT LOOKS, and only where /bin/mount is
// BusyBox's and WSL's helper is there. util-linux's mount finds the helper and
// hands it ro itself. A second run finds it in place and says nothing. WSL-198.
func TestTheMountHelperIsWrittenWhereBusyBoxsMountLooks(t *testing.T) {
	grantsCase(t)
	dir := stagedBusyBoxBase(t)
	out, code := installMountHelper(t, dir)
	wrapper := filepath.Join(dir, "bin", "mount.drvfs")
	got, err := os.ReadFile(wrapper)
	if code != 0 || err != nil || string(got) != mountHelperText(t) || !strings.Contains(out, "mount helper: wrote ") {
		t.Fatalf("a BusyBox base was given the helper %q (%v), exit %d:\n%s", got, err, code, out)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(wrapper); err != nil || info.Mode().Perm() != 0o755 {
			t.Errorf("the helper's mode is %v (%v)", info.Mode().Perm(), err)
		}
	}
	if out, code := installMountHelper(t, dir); code != 0 || out != "" {
		t.Errorf("a helper already in place was written again, exit %d:\n%s", code, out)
	}
	for _, c := range []struct{ name, remove string }{
		{"a base whose /bin/mount is not BusyBox's", "busybox"},
		{"a base without WSL's helper", "mount.drvfs"},
	} {
		dir := stagedBusyBoxBase(t)
		if err := os.Remove(filepath.Join(dir, c.remove)); err != nil {
			t.Fatal(err)
		}
		out, code := installMountHelper(t, dir)
		if _, err := os.Lstat(filepath.Join(dir, "bin", "mount.drvfs")); err == nil || code != 0 || out != "" {
			t.Errorf("%s was given a helper (%v), exit %d:\n%s", c.name, err, code, out)
		}
	}
}

// ⛔ A FILE THERE THAT THIS TOOL DID NOT WRITE IS LEFT AS IT IS, and named. One an
// earlier build of this tool wrote is written again. WSL-198.
func TestTheMountHelperLeavesAFileItDidNotWrite(t *testing.T) {
	grantsCase(t)
	for _, c := range []struct {
		name, body string
		kept       bool
	}{
		{"another's", "#!/bin/sh\nexec /sbin/mount.drvfs \"$@\"\n", true},
		{"an earlier build's", "#!/bin/sh\n# Written by wsl-toolkit.\nexec /sbin/mount.drvfs \"$@\"\n", false},
	} {
		dir := stagedBusyBoxBase(t)
		seedFile(t, dir, "bin/mount.drvfs", c.body)
		out, code := installMountHelper(t, dir)
		got, _ := os.ReadFile(filepath.Join(dir, "bin", "mount.drvfs"))
		if code != 0 || (string(got) == c.body) != c.kept || strings.Contains(out, "is not this tool's") != c.kept {
			t.Errorf("%s helper was kept %v, want %v, exit %d:\n%s", c.name, string(got) == c.body, c.kept, code, out)
		}
	}
}

// ⛔ A LINK TO WSL'S HELPER IS REPLACED, since it finds the helper and still drops
// the ro. A link elsewhere is not this tool's, and is left. WSL-198.
func TestTheMountHelperReplacesALinkToWSLsHelper(t *testing.T) {
	grantsCase(t)
	// ⚠ NOT ON WINDOWS. Git's readlink reads a Windows symbolic link back in its own
	// form, not the one it was made with, so the helper's byte comparison of the
	// target fails there and nowhere it runs. The guest is Linux, and the Linux job
	// runs this case. Measured on CI's Windows runner, which can make the link.
	if runtime.GOOS == "windows" {
		t.Skip("a Windows symbolic link reads back through Git's readlink in another form than its target")
	}
	for _, c := range []struct {
		name, to string
		kept     bool
	}{
		{"a link to WSL's helper", "mount.drvfs", false},
		{"a link elsewhere", "busybox", true},
	} {
		dir := stagedBusyBoxBase(t)
		link := filepath.Join(dir, "bin", "mount.drvfs")
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(shellPath(t, dir)+"/"+c.to, link); err != nil {
			t.Skipf("this host makes no symbolic link: %v", err)
		}
		out, code := installMountHelper(t, dir)
		info, err := os.Lstat(link)
		if err != nil || code != 0 || (info.Mode()&os.ModeSymlink != 0) != c.kept {
			t.Errorf("%s was kept %v, want %v, exit %d:\n%s", c.name, err == nil && info.Mode()&os.ModeSymlink != 0, c.kept, code, out)
		}
	}
}

// ⛔ IT NEVER FAILS ITS RUNNER. A helper it could not write is named, and it exits
// 0, so a grant made now is not refused for it. WSL-198.
func TestTheMountHelperNeverFailsItsRunner(t *testing.T) {
	grantsCase(t)
	dir := stagedBusyBoxBase(t)
	stage(t, dir, "bin")
	out, code := installMountHelper(t, dir)
	if code != 0 || !strings.Contains(out, "⚠ mount helper: ") {
		t.Fatalf("a helper that could not be written answered exit %d:\n%s", code, out)
	}
}

// mountHelperRunStandIns are WSL's helper's remount and the umount, as functions,
// for the helper mount-helper.sh writes. A plain remount is the filesystem's, so
// every mount of the same source goes ro. A bind one is its mount point's alone.
// Both were measured on alpine. remount.plain.fail and remount.bind.fail fail
// one, umount.fail fails a plain unmount, and lazy.fail a lazy one.
const mountHelperRunStandIns = `
st_rows() {
  st_keep=
  st_from=
  while read -r st_source st_target st_rest; do
    if [ "$st_target" = "$2" ]; then st_from=$st_source; fi
  done < "$TK_TEST_DIR/mounts"
  while read -r st_source st_target st_type st_options st_rest; do
    case "$1" in
      ro) if [ "$st_target" = "$2" ] || { [ "$3" = all ] && [ "$st_source" = "$st_from" ]; }; then st_options="ro,${st_options#*,}"; fi ;;
      gone) if [ "$st_target" = "$2" ]; then continue; fi ;;
    esac
    st_keep="$st_keep$st_source $st_target $st_type $st_options $st_rest
"
  done < "$TK_TEST_DIR/mounts"
  printf '%s' "$st_keep" > "$TK_TEST_DIR/mounts"
}
mount() {
  printf 'mount %s\n' "$*" >> "$TK_TEST_DIR/calls"
  case "$2" in
    remount,ro) [ ! -e "$TK_TEST_DIR/remount.plain.fail" ] || return 1; st_rows ro "$3" all ;;
    remount,bind,ro) [ ! -e "$TK_TEST_DIR/remount.bind.fail" ] || return 1; st_rows ro "$3" one ;;
    *) return 2 ;;
  esac
}
umount() {
  printf 'umount %s\n' "$*" >> "$TK_TEST_DIR/calls"
  if [ "$1" = -l ]; then
    [ ! -e "$TK_TEST_DIR/lazy.fail" ] || return 1
    st_rows gone "$2"
  else
    [ ! -e "$TK_TEST_DIR/umount.fail" ] || return 1
    st_rows gone "$1"
  fi
}
`

// wslHelperStandIn is WSL's mount.drvfs: it logs its arguments, and mounts SOURCE
// at TARGET, ro where its options say so. It drops the ro while helper.drops-ro is
// there, and while helper.second is there it brings a second mount of SOURCE, as a
// bind of the grant made before a remount would.
const wslHelperStandIn = `#!/bin/sh
printf 'helper' >> "$TK_TEST_DIR/calls"
for a in "$@"; do printf ' [%s]' "$a" >> "$TK_TEST_DIR/calls"; done
printf '\n' >> "$TK_TEST_DIR/calls"
mode=rw
case ",${4:-}," in *,ro,*) mode=ro ;; esac
[ ! -e "$TK_TEST_DIR/helper.drops-ro" ] || mode=rw
printf '%s %s 9p %s,relatime,aname=drvfs;path=%s 0 0\n' "$1" "$2" "$mode" "$1" >> "$TK_TEST_DIR/mounts"
if [ -e "$TK_TEST_DIR/helper.second" ]; then
  printf '%s /run/second 9p %s,relatime,aname=drvfs;path=%s 0 0\n' "$1" "$mode" "$1" >> "$TK_TEST_DIR/mounts"
fi
`

// mountHelperFstab is a base's fstab: a read-only line of the distribution's own
// at /media/cdrom, and three grants. /workspaces/a is read-write, /workspaces/b
// read-only, and /workspaces/c read-only with no other option. A comment last
// names /workspaces/a read-only.
const mountHelperFstab = "/dev/cdrom /media/cdrom iso9660 noauto,ro 0 0\n" +
	"# wsl-toolkit mounts begin\n" +
	"C:/src/a /workspaces/a drvfs metadata,rw,uid=1000,gid=1000,umask=022,fmask=011,nofail 0 0\n" +
	"C:/src/b /workspaces/b drvfs metadata,ro,uid=1000,gid=1000,umask=022,fmask=011,nofail 0 0\n" +
	"C:/src/c /workspaces/c drvfs ro 0 0\n" +
	"# wsl-toolkit mounts end\n" +
	"#C:/old /workspaces/a drvfs metadata,ro 0 0\n"

// runMountHelper runs the helper mount-helper.sh writes, as BusyBox's mount runs it
// for one line, with args. dir holds fstab, mounts and WSL's helper, and the
// flags a case staged. It answers what the helper said and its exit.
func runMountHelper(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	sh, bin := grantsShell(t)
	at := shellPath(t, dir)
	// ⚠ EXECUTABLE, as WSL's is. The helper runs it by its path.
	if err := os.Chmod(seedFile(t, dir, "sbin/mount.drvfs", wslHelperStandIn), 0o755); err != nil {
		t.Fatal(err)
	}
	s := markedSection(t, []byte(mountHelperText(t)), "the mount helper")
	for _, r := range [][2]string{
		{"helper=/sbin/mount.drvfs\n", "helper=" + at + "/sbin/mount.drvfs\n"},
		{"fstab=/etc/fstab\n", "fstab=" + at + "/fstab\n"},
		{"mounts=/proc/mounts\n", "mounts=" + at + "/mounts\n"},
	} {
		if n := strings.Count(s, r[0]); n != 1 {
			t.Fatalf("the mount helper carries %q %d times, want once", r[0], n)
		}
		s = strings.Replace(s, r[0], r[1], 1)
	}
	run := filepath.Join(dir, "mount-helper.sh")
	if err := os.WriteFile(run, []byte(mountHelperRunStandIns+s), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, append([]string{filepath.ToSlash(run)}, args...)...)
	cmd.Env = append(shellEnv(bin), "TK_TEST_DIR="+at)
	out, err := cmd.CombinedOutput()
	return string(out), exitOf(t, err)
}

// mountHelperCase is a directory a case runs the helper in, with mountHelperFstab
// and no live mount, and a reader of its files.
func mountHelperCase(t *testing.T) (string, func(string) string) {
	t.Helper()
	dir := t.TempDir()
	return dir, grantsSeed(t, dir, mountHelperFstab, "", "")
}

// busyBoxOptions is what BusyBox's mount hands the helper for a grant's line. It
// is the line's options with ro, rw and nofail taken out, measured on alpine.
const busyBoxOptions = "metadata,uid=1000,gid=1000,umask=022,fmask=011"

// ⛔ THE HELPER IS HANDED THE ro ITS LINE ASKS FOR, so a read-only grant is
// read-only from its mount, inside WSL's own mount -a. BusyBox drops the ro, and
// a remount after the start left the grant writable until it ran. A read-write
// grant, and a comment that names its target, change nothing. WSL-198.
func TestTheMountHelperHandsWSLsHelperTheROItsLineAsks(t *testing.T) {
	grantsCase(t)
	dir, read := mountHelperCase(t)
	if out, code := runMountHelper(t, dir, "C:/src/b", "/workspaces/b", "-o", busyBoxOptions); code != 0 {
		t.Fatalf("a read-only grant's mount exited %d:\n%s", code, out)
	}
	if out, code := runMountHelper(t, dir, "C:/src/a", "/workspaces/a", "-o", busyBoxOptions); code != 0 {
		t.Fatalf("a read-write grant's mount exited %d:\n%s", code, out)
	}
	calls := read("calls")
	for _, want := range []string{
		"helper [C:/src/b] [/workspaces/b] [-o] [ro," + busyBoxOptions + "]\n",
		"helper [C:/src/a] [/workspaces/a] [-o] [" + busyBoxOptions + "]\n",
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("WSL's helper was not run as %q:\n%s", want, calls)
		}
	}
	if strings.Contains(calls, "mount ") || strings.Contains(calls, "umount ") {
		t.Errorf("a mount that came up as asked was remounted or unmounted:\n%s", calls)
	}
	mounts := read("mounts")
	if !strings.Contains(mounts, " /workspaces/b 9p ro,") || !strings.Contains(mounts, " /workspaces/a 9p rw,") {
		t.Errorf("the grants are mounted as:\n%s", mounts)
	}
}

// ⛔ A LINE WITH NO OTHER OPTION GETS ITS ro TOO. BusyBox then hands the helper no
// -o at all. A target whose line is not drvfs is handed on as it came, though
// that line says ro. WSL-198.
func TestTheMountHelperHandsROWhereBusyBoxHandsNoOptions(t *testing.T) {
	grantsCase(t)
	dir, read := mountHelperCase(t)
	if out, code := runMountHelper(t, dir, "C:/src/c", "/workspaces/c"); code != 0 {
		t.Fatalf("a read-only line with no other option exited %d:\n%s", code, out)
	}
	if out, code := runMountHelper(t, dir, "C:/src/d", "/media/cdrom", "-o", "metadata"); code != 0 {
		t.Fatalf("a target no drvfs line names exited %d:\n%s", code, out)
	}
	calls := read("calls")
	for _, want := range []string{
		"helper [C:/src/c] [/workspaces/c] [-o] [ro]\n",
		"helper [C:/src/d] [/media/cdrom] [-o] [metadata]\n",
	} {
		if !strings.Contains(calls, want) {
			t.Errorf("WSL's helper was not run as %q:\n%s", want, calls)
		}
	}
}

// ⛔ A MOUNT ITS HELPER LEFT WRITABLE IS REMOUNTED READ-ONLY, PLAIN FIRST. A plain
// remount makes the filesystem read-only, so a second mount of the grant made
// before it goes read-only too. A bind remount covers its own mount point alone,
// measured on alpine. WSL-198.
func TestTheMountHelperRemountsAGrantItsHelperLeftWritable(t *testing.T) {
	grantsCase(t)
	dir, read := mountHelperCase(t)
	stage(t, dir, "helper.drops-ro")
	stage(t, dir, "helper.second")
	if out, code := runMountHelper(t, dir, "C:/src/b", "/workspaces/b", "-o", busyBoxOptions); code != 0 {
		t.Fatalf("a read-only grant its helper left writable exited %d:\n%s", code, out)
	}
	mounts, calls := read("mounts"), read("calls")
	if !strings.Contains(mounts, " /workspaces/b 9p ro,") || !strings.Contains(mounts, " /run/second 9p ro,") {
		t.Fatalf("the grant, or a second mount of it, is still writable:\n%s\ncalls:\n%s", mounts, calls)
	}
	if !strings.Contains(calls, "mount -o remount,ro /workspaces/b\n") || strings.Contains(calls, "remount,bind") {
		t.Errorf("the grant was not remounted plain alone:\n%s", calls)
	}
	// ⚠ WHERE THE PLAIN ONE FAILS, a bind remount still makes the grant read-only.
	dir, read = mountHelperCase(t)
	stage(t, dir, "helper.drops-ro")
	stage(t, dir, "remount.plain.fail")
	if out, code := runMountHelper(t, dir, "C:/src/b", "/workspaces/b", "-o", busyBoxOptions); code != 0 ||
		!strings.Contains(read("mounts"), " /workspaces/b 9p ro,") || !strings.Contains(read("calls"), "mount -o remount,bind,ro /workspaces/b\n") {
		t.Fatalf("a grant whose plain remount failed exited %d, mounted as:\n%s\ncalls:\n%s\n%s", code, read("mounts"), read("calls"), out)
	}
}

// ⛔ ONE THAT WILL NOT GO READ-ONLY IS UNMOUNTED, AND ITS MOUNT FAILS. A busy one is
// detached lazily, and the message says which happened. WSL-198.
func TestTheMountHelperUnmountsAGrantThatStaysWritable(t *testing.T) {
	grantsCase(t)
	for _, c := range []struct {
		name    string
		flags   []string
		said    string
		mounted bool
	}{
		{"an unmount", nil, "so it was unmounted", false},
		{"a busy unmount", []string{"umount.fail"}, "so it was detached, since it was busy", false},
		{"no unmount at all", []string{"umount.fail", "lazy.fail"}, "so it was NOT unmounted, and it is still writable", true},
	} {
		dir, read := mountHelperCase(t)
		for _, f := range append([]string{"helper.drops-ro", "remount.plain.fail", "remount.bind.fail"}, c.flags...) {
			stage(t, dir, f)
		}
		out, code := runMountHelper(t, dir, "C:/src/b", "/workspaces/b", "-o", busyBoxOptions)
		if code != 32 || !strings.Contains(out, "/workspaces/b came up writable and would not go read-only, "+c.said) {
			t.Errorf("%s: a grant that stayed writable exited %d:\n%s", c.name, code, out)
		}
		if strings.Contains(read("mounts"), " /workspaces/b ") != c.mounted {
			t.Errorf("%s: the grant is mounted %v, want %v:\n%s", c.name, !c.mounted, c.mounted, read("mounts"))
		}
	}
}

// grantsDriveHelped is grantsDriveIn with the mount helper's payload helper64,
// handed on as it is unless it is the guest's own text.
func grantsDriveHelped(t *testing.T, dir, mode string, mounts []BaseMount, target, helper64 string) (string, error) {
	t.Helper()
	fstab64, checks, err := renderBaseMounts(mounts)
	if err != nil {
		t.Fatal(err)
	}
	from := ""
	if target != "" {
		for _, m := range mounts {
			if b64(m.Target) == target {
				from = b64(fstabEscape(filepath.ToSlash(m.Source)))
			}
		}
	}
	return grantsRun(t, dir, map[string]string{
		"TK_USER": "agent", "TK_GRANTS_MODE": mode,
		"TK_FSTAB_B64": fstab64, "TK_MOUNT_CHECKS": checks, "TK_GRANT_TARGET": target,
		"TK_GRANT_SOURCE": from, "TK_GRANT_OWNER": b64(grantsMine), "TK_GRANT_OWNER_NAME": b64(grantsMine),
		"TK_MOUNT_HELPER_B64": helper64,
	})
}

// ⛔ grants.sh WRITES THE MOUNT HELPER BEFORE IT WRITES A BLOCK, so a new base's
// first start mounts its grants. It does so only where /bin/mount is BusyBox's,
// and for a take-back's block too. The held step changes nothing. WSL-198.
func TestAGrantWritesWSLsMountHelperBeforeItsBlock(t *testing.T) {
	grantsCase(t)
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "ro"}
	for _, c := range []struct {
		mode             string
		busybox, written bool
	}{
		{GrantsWrite, true, true},
		{GrantsLive, true, true},
		{GrantsBlock, true, true},
		{GrantsWrite, false, false},
		{GrantsHeld, true, false},
	} {
		dir := t.TempDir()
		grantsSeed(t, dir, "", "", "")
		stage(t, dir, "mount.drvfs")
		if c.busybox {
			stage(t, dir, "busybox")
		}
		target := ""
		if c.mode == GrantsLive {
			target = b64(mine.Target)
		}
		out, err := grantsDriveIn(t, dir, c.mode, []BaseMount{mine}, target)
		if err != nil {
			t.Fatalf("%s, busybox %v: grants.sh answered %v:\n%s", c.mode, c.busybox, err, out)
		}
		got, readErr := os.ReadFile(filepath.Join(dir, "bin", "mount.drvfs"))
		if (readErr == nil && string(got) == mountHelperText(t)) != c.written || strings.Contains(out, "mount helper: wrote ") != c.written {
			t.Errorf("%s, busybox %v: the helper was written %v, want %v:\n%s", c.mode, c.busybox, readErr == nil, c.written, out)
		}
	}
}

// ⛔ A MOUNT HELPER THAT COULD NOT BE WRITTEN REFUSES NOTHING. grants.sh died over
// it before the block mode's take-back, so a take-back after a failed write of the
// configuration did not run. Its install failing, or a payload that will not
// decode, is named, and the grant and the take-back go on. WSL-198.
func TestAGrantWhoseMountHelperFailsIsNotRefused(t *testing.T) {
	grantsCase(t)
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"}
	for _, c := range []struct {
		name, mode, helper64, said string
	}{
		{"a take-back whose install exits 7", GrantsBlock, b64("exit 7\n"), "⚠ mount helper: its install exited 7"},
		{"a grant whose install exits 7", GrantsLive, b64("exit 7\n"), "⚠ mount helper: its install exited 7"},
		{"a take-back whose payload will not decode", GrantsBlock, "@@@", "⚠ mount helper: its payload could not be decoded"},
	} {
		dir := t.TempDir()
		read := grantsSeed(t, dir, "", "", "")
		target := ""
		if c.mode == GrantsLive {
			target = b64(mine.Target)
		}
		out, err := grantsDriveHelped(t, dir, c.mode, []BaseMount{mine}, target, c.helper64)
		if err != nil || !strings.Contains(out, c.said) || !strings.Contains(out, "grants-complete") {
			t.Errorf("%s answered %v:\n%s", c.name, err, out)
		}
		if !strings.Contains(read("fstab"), " /workspaces/mine drvfs ") {
			t.Errorf("%s left the block:\n%s", c.name, read("fstab"))
		}
	}
}

// ⛔ engines.sh WRITES THE MOUNT HELPER AT EVERY ENSURE, so an alpine base an
// earlier release granted mounts its grants at its next start. grants.sh writes
// it only when a grant changes. An install that fails is named, and the ensure
// goes on. WSL-198.
func TestEnginesShWritesWSLsMountHelper(t *testing.T) {
	grantsCase(t)
	sh, bin := grantsShell(t)
	run := func(dir, helper64 string) string {
		t.Helper()
		script := "say() { printf '  * %s\\n' \"$*\"; }\n" + markedSection(t, enginesScript, "the mount helper's install") + "printf 'section-complete\\n'\n"
		path := filepath.Join(dir, "engines-run.sh")
		if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(sh, filepath.ToSlash(path))
		cmd.Env = append(shellEnv(bin), "TK_MOUNT_HELPER_B64="+helper64)
		out, err := cmd.CombinedOutput()
		if code := exitOf(t, err); code != 0 || !strings.Contains(string(out), "section-complete") {
			t.Fatalf("the install section exited %d:\n%s", code, out)
		}
		return string(out)
	}
	dir := stagedBusyBoxBase(t)
	out := run(dir, base64.StdEncoding.EncodeToString([]byte(mountHelperIn(t, dir))))
	if got, err := os.ReadFile(filepath.Join(dir, "bin", "mount.drvfs")); err != nil || string(got) != mountHelperText(t) {
		t.Fatalf("an ensure on a BusyBox base wrote the helper %q (%v):\n%s", got, err, out)
	}
	if out := run(t.TempDir(), b64("exit 7\n")); !strings.Contains(out, "⚠ mount helper: its install exited 7") {
		t.Errorf("an install that failed was not named:\n%s", out)
	}
}

// ⭐ ONE TEXT FOR BOTH RUNNERS: every grants.sh run and every engines.sh run is
// handed mount-helper.sh whole. WSL-198.
func TestTheMountHelperIsHandedToBothItsRunners(t *testing.T) {
	cfg := grantBase(t)
	for _, mode := range []string{GrantsWrite, GrantsBlock, GrantsHeld} {
		env, err := grantsEnv(cfg, mode, BaseMount{})
		if err != nil {
			t.Fatal(err)
		}
		if got, err := base64.StdEncoding.DecodeString(env["TK_MOUNT_HELPER_B64"]); err != nil || !bytes.Equal(got, mountHelperScript) {
			t.Errorf("grants.sh in %s mode is not handed mount-helper.sh (%v)", mode, err)
		}
	}
	b := &Base{cfg: DefaultConfig()}
	if got, err := base64.StdEncoding.DecodeString(b.enginesRequest(io.Discard).Env["TK_MOUNT_HELPER_B64"]); err != nil || !bytes.Equal(got, mountHelperScript) {
		t.Errorf("engines.sh is not handed mount-helper.sh (%v)", err)
	}
}

// -- WSL-199: engines.sh becomes the account through su on Alpine --------------

// ⛔ WHERE THERE IS NO runuser, engines.sh BECOMES THE ACCOUNT THROUGH su, as
// grants.sh's read-back does. su takes its options before the account, and the
// script's own arguments after it. It called `setpriv --reuid`, which BusyBox's
// setpriv does not have, so every ensure on alpine printed a usage block. WSL-199.
func TestEnginesShBecomesTheAccountThroughSuWhereThereIsNoRunuser(t *testing.T) {
	sh, _ := grantsShell(t)
	d := t.TempDir()
	at := shellPath(t, d)
	if err := os.MkdirAll(filepath.Join(d, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	// ⚠ su PERMUTES, as grantsStandIns says, and runs the shell by the case's own
	// path, since PATH names an empty directory.
	standIns := `
su() {
  printf 'su' >> "$BG/calls"
  for su_a in "$@"; do printf ' [%s]' "$su_a" >> "$BG/calls"; done
  printf '\n' >> "$BG/calls"
  su_left=$#
  su_cmd=
  while [ "$su_left" -gt 0 ]; do
    case "$1" in
      -s) shift 2; su_left=$((su_left - 2)) ;;
      -c) su_cmd=$2; shift 2; su_left=$((su_left - 2)) ;;
      *) su_arg=$1; shift; set -- "$@" "$su_arg"; su_left=$((su_left - 1)) ;;
    esac
  done
  shift
  "$SH" -c "$su_cmd" "$@"
}
setpriv() { printf 'setpriv %s\n' "$*" >> "$BG/calls"; return 1; }
`
	// The probe's $0 to $3 are the account's shell's own.
	probe := `as_account 'printf "%s %s %s\n" "$0" "$1" "$2" > "$3"' wtk-manager one two "$BG/ran"` + "\n"
	script := standIns + "PATH=" + at + "/empty\nTK_USER=agent\n" + markedSection(t, enginesScript, "as the account") + probe
	run := filepath.Join(d, "engines-run.sh")
	if err := os.WriteFile(run, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, filepath.ToSlash(run))
	cmd.Env = append(os.Environ(), "BG="+at, "SH="+filepath.ToSlash(sh))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("as_account with no runuser failed: %v\n%s", err, out)
	}
	calls, _ := os.ReadFile(filepath.Join(d, "calls"))
	ran, _ := os.ReadFile(filepath.Join(d, "ran"))
	if strings.Contains(string(calls), "setpriv") {
		t.Fatalf("engines.sh asked setpriv, which BusyBox's cannot answer:\n%s", calls)
	}
	if !strings.HasPrefix(string(calls), "su [-s] [/bin/sh] [-c] [") || !strings.Contains(string(calls), "] [agent] [wtk-manager] [one] [two] [") {
		t.Errorf("su was not asked with its options before the account:\n%s", calls)
	}
	if string(ran) != "wtk-manager one two\n" {
		t.Errorf("the script ran as the account with %q, want its name and arguments", ran)
	}
}
