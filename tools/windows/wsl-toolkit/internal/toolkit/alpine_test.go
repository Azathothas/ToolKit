// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// -- WSL-198: a grant on an Alpine base survives a restart, in its mode ---------

// bootGrantsBlock is the fstab a base with a read-write grant at /workspaces/a and
// a read-only one at /workspaces/b holds, below a line of the distribution's own.
func bootGrantsBlock(t *testing.T) string {
	t.Helper()
	fstab64, _, err := renderBaseMounts([]BaseMount{
		{Source: `C:\src\a`, Target: "/workspaces/a", Mode: "rw"},
		{Source: `C:\src\b`, Target: "/workspaces/b", Mode: "ro"},
	})
	if err != nil {
		t.Fatal(err)
	}
	block, err := base64.StdEncoding.DecodeString(fstab64)
	if err != nil {
		t.Fatal(err)
	}
	return "/dev/cdrom\t/media/cdrom\tiso9660\tnoauto,ro 0 0\n# wsl-toolkit mounts begin\n" + string(block) + "# wsl-toolkit mounts end\n"
}

// bootLiveLine is a /proc/mounts row for a grant WSL mounted at target.
func bootLiveLine(target, mode string) string {
	return "C:/src" + target + " " + target + " 9p " + mode + ",relatime,aname=drvfs;path=C:/src 0 0\n"
}

// bootGrantsStandIns are mount, umount, flock and sleep, as functions so Git's sh
// on Windows drives the script too. mount logs each call with the PATH it met.
// A mount comes up rw whatever its line says, as BusyBox's was measured to, and
// a read-only remount makes it ro, or fails while the case's directory holds
// remount.fail. umount takes the row away.
const bootGrantsStandIns = `
mount() {
  printf 'mount %s | PATH=%s\n' "$*" "$PATH" >> "$BG/calls"
  if [ "$1" = -o ]; then
    [ ! -e "$BG/remount.fail" ] || return 1
    bg_target=$3
    bg_keep=
    while read -r bg_from bg_at bg_type bg_options bg_rest; do
      if [ "$bg_at" = "$bg_target" ]; then bg_options="ro,${bg_options#*,}"; fi
      bg_keep="$bg_keep$bg_from $bg_at $bg_type $bg_options $bg_rest
"
    done < "$BG/proc/mounts"
    printf '%s' "$bg_keep" > "$BG/proc/mounts"
    return 0
  fi
  printf 'C:/src%s %s 9p rw,relatime,aname=drvfs;path=C:/src 0 0\n' "$1" "$1" >> "$BG/proc/mounts"
}
umount() {
  printf 'umount %s\n' "$*" >> "$BG/calls"
  bg_keep=
  while IFS= read -r bg_line; do
    case "$bg_line" in
      *" $1 9p "*) ;;
      *) bg_keep="$bg_keep$bg_line
" ;;
    esac
  done < "$BG/proc/mounts"
  printf '%s' "$bg_keep" > "$BG/proc/mounts"
}
flock() { printf 'flock %s\n' "$*" >> "$BG/calls"; }
sleep() { printf 'sleep %s\n' "$*" >> "$BG/calls"; }
`

// bootGrantsHost is one start of an unsealed base whose boot script reads the
// grants: /etc/fstab, /proc/mounts, /run, the grant lock and WSL's mount helper
// under a directory of its own.
type bootGrantsHost struct {
	t       *testing.T
	sh, bin string
	dir, at string
}

func newBootGrantsHost(t *testing.T, fstab, mounts string) bootGrantsHost {
	t.Helper()
	sh, bin := grantsShell(t)
	d := t.TempDir()
	h := bootGrantsHost{t: t, sh: sh, bin: bin, dir: d, at: filepath.ToSlash(d)}
	for _, sub := range []string{"run", "bin", "sbin", "var/lib/wsl-toolkit", "usr/local/lib/wsl-toolkit"} {
		if err := os.MkdirAll(filepath.Join(d, filepath.FromSlash(sub)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	seedFile(t, d, "etc/fstab", fstab)
	seedFile(t, d, "proc/mounts", mounts)
	// ⚠ Files that are there, so any readlink resolves them.
	seedFile(t, d, "bin/busybox", "")
	seedFile(t, d, "bin/mount", "")
	return h
}

func (h bootGrantsHost) read(rel string) string {
	body, _ := os.ReadFile(filepath.Join(h.dir, filepath.FromSlash(rel)))
	return string(body)
}

// run starts the base once, /bin/mount BusyBox's where busybox is true, and
// answers the boot script's exit.
func (h bootGrantsHost) run(busybox bool) int {
	h.t.Helper()
	mountBin := h.at + "/bin/mount"
	if busybox {
		mountBin = h.at + "/bin/busybox"
	}
	s := "seal=on\naccount=agent\n" + markedSection(h.t, bootScript, "the boot script")
	for _, r := range [][2]string{
		{"gb_helper=/sbin/mount.drvfs\n", "gb_helper=" + h.at + "/sbin/mount.drvfs\n"},
		{"gb_link=/usr/local/bin/mount.drvfs\n", "gb_link=" + h.at + "/usr/local/bin/mount.drvfs\n"},
		{"gb_mount_bin=/bin/mount\n", "gb_mount_bin=" + mountBin + "\n"},
		{"/var/lib/", h.at + "/var/lib/"},
		{"/run/", h.at + "/run/"},
		{"/usr/local/lib/", h.at + "/usr/local/lib/"},
		{"/proc/mounts", h.at + "/proc/mounts"},
		{"/etc/", h.at + "/etc/"},
	} {
		if !strings.Contains(s, r[0]) {
			h.t.Fatalf("the boot script no longer carries %q", r[0])
		}
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	run := filepath.Join(h.dir, "boot-run.sh")
	if err := os.WriteFile(run, []byte(bootGrantsStandIns+s), 0o644); err != nil {
		h.t.Fatal(err)
	}
	cmd := exec.Command(h.sh, filepath.ToSlash(run))
	cmd.Env = append(os.Environ(), "PATH="+h.bin, "BG="+h.at)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			h.t.Fatal(err)
		}
		h.t.Logf("the boot script said:\n%s", out)
		return ee.ExitCode()
	}
	return 0
}

// ⛔ A START MOUNTS EVERY GRANT WSL DID NOT, IN ITS MODE. WSL runs `mount -a` with no
// PATH, and BusyBox's mount looks for WSL's mount.drvfs on PATH, so measured on
// alpine a restart mounted no grant. The boot script mounts each line of the
// block that is not mounted, with /sbin on PATH, and a line of the distribution's
// own is left alone. Each mount comes up writable, as BusyBox's does, so the
// read-only one is remounted read-only, with a bind remount. WSL-198.
func TestTheBootScriptMountsEachGrantWSLDidNotInItsMode(t *testing.T) {
	h := newBootGrantsHost(t, bootGrantsBlock(t), "")
	if code := h.run(false); code != 0 {
		t.Fatalf("the boot script exited %d", code)
	}
	mounts, calls := h.read("proc/mounts"), h.read("calls")
	if !strings.Contains(mounts, " /workspaces/a 9p rw,") || !strings.Contains(mounts, " /workspaces/b 9p ro,") {
		t.Fatalf("a start with no grant mounted left the mount table:\n%s\ncalls:\n%s", mounts, calls)
	}
	for _, want := range []string{"mount /workspaces/a |", "mount /workspaces/b |", "mount -o remount,bind,ro /workspaces/b |"} {
		if !strings.Contains(calls, want) {
			t.Errorf("the boot script did not call %q:\n%s", want, calls)
		}
	}
	if strings.Contains(calls, "/media/cdrom") || strings.Contains(calls, "remount,bind,ro /workspaces/a") {
		t.Errorf("the boot script touched a line it does not own, or a read-write grant:\n%s", calls)
	}
	if !strings.Contains(h.read("run/wsl-toolkit-boot.log"), "grants checked: 2 mounted here, 1 made read-only, 0 failed") {
		t.Errorf("the boot log does not say the grants were checked:\n%s", h.read("run/wsl-toolkit-boot.log"))
	}
}

// ⛔ EACH MOUNT MEETS /sbin ON PATH, where WSL keeps its helper. WSL starts the boot
// command with no PATH at all, measured on alpine, and BusyBox's mount looks for
// mount.drvfs there. The case's own PATH has no /sbin. WSL-198.
func TestTheBootScriptMountsWithWSLsHelperDirectoryOnPath(t *testing.T) {
	h := newBootGrantsHost(t, bootGrantsBlock(t), "")
	if code := h.run(false); code != 0 {
		t.Fatalf("the boot script exited %d", code)
	}
	n := 0
	for _, line := range strings.Split(h.read("calls"), "\n") {
		if !strings.HasPrefix(line, "mount ") {
			continue
		}
		n++
		path := line[strings.Index(line, "| PATH=")+len("| PATH="):]
		if !strings.Contains(":"+path+":", ":/sbin:") {
			t.Errorf("a mount met a PATH with no /sbin: %s", line)
		}
	}
	if n == 0 {
		t.Fatalf("the boot script mounted nothing:\n%s", h.read("calls"))
	}
}

// ⛔ A GRANT WSL MOUNTED IS LEFT AS IT IS, AND ONLY A READ-ONLY ONE THAT CAME UP
// WRITABLE IS REMOUNTED. With WSL's helper linked, `mount -a` mounts every grant,
// and BusyBox hands it no `ro`. WSL-198.
func TestTheBootScriptMakesAGrantWSLMountedWritableReadOnly(t *testing.T) {
	h := newBootGrantsHost(t, bootGrantsBlock(t), bootLiveLine("/workspaces/a", "rw")+bootLiveLine("/workspaces/b", "rw"))
	if code := h.run(false); code != 0 {
		t.Fatalf("the boot script exited %d", code)
	}
	calls := h.read("calls")
	if strings.Contains(calls, "mount /workspaces/") || !strings.Contains(calls, "mount -o remount,bind,ro /workspaces/b |") {
		t.Fatalf("a start whose grants WSL mounted called:\n%s", calls)
	}
	if !strings.Contains(h.read("proc/mounts"), " /workspaces/b 9p ro,") {
		t.Errorf("the read-only grant is mounted as:\n%s", h.read("proc/mounts"))
	}
	if !strings.Contains(h.read("run/wsl-toolkit-boot.log"), "grants checked: 0 mounted here, 1 made read-only, 0 failed") {
		t.Errorf("the boot log reads:\n%s", h.read("run/wsl-toolkit-boot.log"))
	}
}

// ⛔ A READ-ONLY GRANT THAT WILL NOT GO READ-ONLY IS UNMOUNTED, so nothing writes
// through it, and the boot log names it. The start still exits 0. WSL-198.
func TestTheBootScriptUnmountsAReadOnlyGrantThatStaysWritable(t *testing.T) {
	h := newBootGrantsHost(t, bootGrantsBlock(t), bootLiveLine("/workspaces/a", "rw")+bootLiveLine("/workspaces/b", "rw"))
	seedFile(t, h.dir, "remount.fail", "")
	if code := h.run(false); code != 0 {
		t.Fatalf("a remount that failed failed the start: exit %d", code)
	}
	if mounts := h.read("proc/mounts"); strings.Contains(mounts, " /workspaces/b ") || !strings.Contains(mounts, " /workspaces/a 9p rw,") {
		t.Fatalf("a read-only grant that stayed writable is still mounted, or the other grant went:\n%s", mounts)
	}
	log := h.read("run/wsl-toolkit-boot.log")
	if !strings.Contains(log, "grant /workspaces/b NOT made read-only, so it was unmounted") || !strings.Contains(log, "1 failed") {
		t.Errorf("the boot log reads:\n%s", log)
	}
}

// ⚠ THE GRANTS ARE READ UNDER THE GRANT LOCK grants.sh takes, before the first
// mount, so a grant or a revoke made as the base starts is not undone. WSL-198.
func TestTheBootScriptReadsTheGrantsUnderTheGrantLock(t *testing.T) {
	h := newBootGrantsHost(t, bootGrantsBlock(t), "")
	if code := h.run(false); code != 0 {
		t.Fatalf("the boot script exited %d", code)
	}
	calls := h.read("calls")
	lock, mount := strings.Index(calls, "flock -n 9\n"), strings.Index(calls, "mount ")
	if lock < 0 || mount < 0 || lock > mount {
		t.Fatalf("the grants were not read under the grant lock:\n%s", calls)
	}
}

// ⛔ A START LINKS WSL'S HELPER WHERE BUSYBOX LOOKS, for a base an earlier release
// granted, and only where /bin/mount is BusyBox's. The next start's `mount -a`
// then mounts the grants itself. WSL-198.
func TestTheBootScriptLinksWSLsHelperOnABusyBoxBase(t *testing.T) {
	for _, busybox := range []bool{true, false} {
		h := newBootGrantsHost(t, bootGrantsBlock(t), "")
		seedFile(t, h.dir, "sbin/mount.drvfs", "")
		if code := h.run(busybox); code != 0 {
			t.Fatalf("busybox %v: the boot script exited %d", busybox, code)
		}
		_, err := os.Lstat(filepath.Join(h.dir, "usr", "local", "bin", "mount.drvfs"))
		if (err == nil) != busybox {
			t.Errorf("busybox %v: the helper was linked %v:\n%s", busybox, err == nil, h.read("run/wsl-toolkit-boot.log"))
		}
		if busybox && runtime.GOOS != "windows" {
			if to, err := os.Readlink(filepath.Join(h.dir, "usr", "local", "bin", "mount.drvfs")); err != nil || to != h.at+"/sbin/mount.drvfs" {
				t.Errorf("the link names %q, %v", to, err)
			}
		}
	}
}

// ⛔ grants.sh LINKS WSL'S HELPER BEFORE IT WRITES A BLOCK, so a new base's first
// start mounts its grants, and only where /bin/mount is BusyBox's. The held step
// changes nothing, the link included. WSL-198.
func TestAGrantLinksWSLsMountHelperWhereBusyBoxLooks(t *testing.T) {
	grantsCase(t)
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "ro"}
	for _, c := range []struct {
		mode            string
		busybox, linked bool
	}{
		{GrantsWrite, true, true},
		{GrantsLive, true, true},
		{GrantsWrite, false, false},
		{GrantsHeld, true, false},
	} {
		dir := t.TempDir()
		grantsSeed(t, dir, "", "", "")
		stage(t, dir, "mount.drvfs")
		if c.busybox {
			stage(t, dir, "busybox")
		}
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		target := ""
		if c.mode == GrantsLive {
			target = b64(mine.Target)
		}
		out, err := grantsDriveIn(t, dir, c.mode, []BaseMount{mine}, target)
		if err != nil {
			t.Fatalf("%s, busybox %v: grants.sh answered %v:\n%s", c.mode, c.busybox, err, out)
		}
		_, statErr := os.Lstat(filepath.Join(dir, "bin", "mount.drvfs"))
		if (statErr == nil) != c.linked || strings.Contains(out, "so WSL's mount -a finds it") != c.linked {
			t.Errorf("%s, busybox %v: the helper was linked %v, want %v:\n%s", c.mode, c.busybox, statErr == nil, c.linked, out)
		}
	}
}

// grantWaitRun runs the verifier's explicit mount checks for one read-write grant
// at a directory of the case's own. The grant is mounted where mounted is true,
// and the boot log is bootLog, or absent where empty. sleep is a function that is
// the boot script mounting the grant and saying so. It answers the exit, what the
// checks said, and how often they slept.
func grantWaitRun(t *testing.T, mounted bool, bootLog string) (int, string, int) {
	t.Helper()
	sh, bin := grantsShell(t)
	d := t.TempDir()
	at := filepath.ToSlash(d)
	target := at + "/ws"
	if err := os.MkdirAll(filepath.Join(d, "ws"), 0o755); err != nil {
		t.Fatal(err)
	}
	live := bootLiveLine(target, "rw")
	mounts := ""
	if mounted {
		mounts = live
	}
	seedFile(t, d, "mounts", mounts)
	if bootLog != "" {
		seedFile(t, d, "boot.log", bootLog)
	}
	section := strings.NewReplacer("/proc/mounts", at+"/mounts", "/run/wsl-toolkit-boot.log", at+"/boot.log").
		Replace(markedSection(t, verifyScript, "the boot script's grants") + markedSection(t, verifyScript, "the explicit mounts"))
	script := "set -eu\nassert_grant_owner() { :; }\n" +
		"sleep() { printf x >> '" + at + "/slept'; printf '%s' '" + live + "' >> '" + at + "/mounts'; " +
		"printf 'grants checked: 1 mounted here, 0 made read-only, 0 failed\\n' > '" + at + "/boot.log'; }\n" + section
	run := filepath.Join(d, "verify-run.sh")
	if err := os.WriteFile(run, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, filepath.ToSlash(run))
	cmd.Env = append(os.Environ(), "PATH="+bin, "TMPDIR="+at,
		"TK_MOUNT_CHECKS="+base64.StdEncoding.EncodeToString([]byte(target))+" rw")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	slept, _ := os.ReadFile(filepath.Join(d, "slept"))
	return code, out.String(), len(slept)
}

// ⛔ THE VERIFIER WAITS FOR THE BOOT SCRIPT WHILE A GRANT IS NOT YET AS ASKED. WSL
// does not wait for a [boot] command, so a verifier run at once after a restart
// met a grant the boot script was about to mount, or to make read-only, refused
// the base and re-provisioned it. WSL-198.
func TestTheVerifierWaitsForTheBootScriptsGrants(t *testing.T) {
	code, out, slept := grantWaitRun(t, false, "")
	if code != 0 || slept != 1 || !strings.Contains(out, "mount rw ") {
		t.Fatalf("a grant the boot script mounted a moment late answered exit %d after %d wait(s):\n%s", code, slept, out)
	}
	// ⚠ A GRANT ALREADY AS ASKED IS NOT WAITED ON.
	if code, out, slept := grantWaitRun(t, true, ""); code != 0 || slept != 0 {
		t.Fatalf("a grant already mounted answered exit %d after %d wait(s):\n%s", code, slept, out)
	}
}

// ⚠ A BOOT LOG THAT SAYS THE GRANTS WERE CHECKED IS NOT WAITED ON, and a grant still
// not mounted is refused at once. WSL-198.
func TestTheVerifierDoesNotWaitOnceTheBootScriptCheckedTheGrants(t *testing.T) {
	code, out, slept := grantWaitRun(t, false, "boot\ngrants checked: 0 mounted here, 0 made read-only, 1 failed\n")
	if code != 3 || slept != 0 || !strings.Contains(out, "is a guest directory, not a live Windows mount") {
		t.Fatalf("a grant the boot script could not mount answered exit %d after %d wait(s):\n%s", code, slept, out)
	}
}

// -- WSL-199: engines.sh becomes the account through su on Alpine --------------

// ⛔ WHERE THERE IS NO runuser, engines.sh BECOMES THE ACCOUNT THROUGH su, as
// grants.sh's read-back does: su's options before the account, and the script's
// own arguments after it. It called `setpriv --reuid`, which BusyBox's setpriv does
// not have, so every ensure on alpine printed a usage block. WSL-199.
func TestEnginesShBecomesTheAccountThroughSuWhereThereIsNoRunuser(t *testing.T) {
	sh, _ := grantsShell(t)
	d := t.TempDir()
	at := filepath.ToSlash(d)
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
