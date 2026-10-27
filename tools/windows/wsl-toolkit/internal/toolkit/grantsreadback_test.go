// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-188: a live grant is read back as the account, on every base ----------

// stage puts an empty marker file in dir.
func stage(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// ⛔ ALPINE HAS NO runuser, so every live grant there failed its read-back. Where
// runuser is not there, su runs the read-back as the account, in both modes.
func TestAGrantIsReadBackThroughSuWhereThereIsNoRunuser(t *testing.T) {
	grantsCase(t)
	for _, mode := range []string{"rw", "ro"} {
		mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: mode}
		dir := t.TempDir()
		read := grantsSeed(t, dir, "", "", "")
		stage(t, dir, "runuser.gone")
		out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target))
		if err != nil || !strings.Contains(out, "grant "+mode+" /workspaces/mine") {
			t.Fatalf("a %s grant on a base with no runuser answered %v:\n%s", mode, err, out)
		}
		if read("su.log") != "su agent\n" {
			t.Errorf("a %s grant on a base with no runuser was not read back through su: %q", mode, read("su.log"))
		}
	}
}

// ⛔ A READ-ONLY GRANT THAT COULD NOT BE READ BACK PASSED. A write that never ran
// and a write the mount refused both exit nonzero, so an account that could not
// be become read as a mount it could not write. It is refused now, in both modes,
// and the grant's own mount is taken away.
//
// ⚠ THE REFUSAL NAMES THE TOOL THAT RAN AND ITS EXIT. It blamed runuser and su
// both, when su is asked only where runuser is not there. WSL-188.
func TestAGrantThatCannotBeReadBackIsRefused(t *testing.T) {
	grantsCase(t)
	for _, c := range []struct {
		mode    string
		gone    bool
		because string
	}{
		{"ro", false, "runuser exited 1, and no shell as the account answered"},
		{"rw", false, "runuser exited 1, and no shell as the account answered"},
		{"ro", true, "there is no runuser, and su exited 1, and no shell as the account answered"},
	} {
		mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: c.mode}
		dir := t.TempDir()
		read := grantsSeed(t, dir, "", "", "")
		stage(t, dir, "account.silent")
		if c.gone {
			stage(t, dir, "runuser.gone")
		}
		out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target))
		if err == nil || !strings.Contains(out, "/workspaces/mine could not be read back as agent: "+c.because) {
			t.Fatalf("a %s grant the account could not read back, runuser gone %v, answered %v:\n%s", c.mode, c.gone, err, out)
		}
		if read("mounts") != "" || read("owners") != "" {
			t.Errorf("a %s grant that was not read back left its mount or its row:\nmounts:\n%s\nowners:\n%s", c.mode, read("mounts"), read("owners"))
		}
	}
}

// ⛔ THE READ-BACK'S PROBE ANSWERS IN A REAL SHELL, both ways. It wrote with `:`, a
// special builtin, and a failed redirect on one ends a POSIX shell: measured with
// bash as sh on arch and BusyBox's ash on alpine, a write the mount refused
// answered nothing, so every read-only grant was refused as not read back. The
// stand-ins answer for the account, so only a real shell holds this. WSL-188.
//
// ⚠ IN POSIX MODE, as a guest's sh is. Git's sh on Windows is bash with POSIX mode
// off, which survives that redirect, so the case asks for the mode where the
// shell has it. dash and ash have no such option, and are POSIX already.
func TestTheReadBackProbeAnswersInARealShell(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	script := string(grantsScript)
	const lead = "\nprobe_write='"
	start := strings.Index(script, lead)
	if start < 0 {
		t.Fatal("grants.sh no longer sets probe_write")
	}
	probe := script[start+len(lead):]
	probe = probe[:strings.Index(probe, "'\n")]
	dir := t.TempDir()
	for _, c := range []struct{ path, want string }{
		{filepath.ToSlash(filepath.Join(dir, "probe")), "writable"},
		// ⚠ A WRITE THAT CANNOT LAND, as on a read-only mount: its directory is not there.
		{filepath.ToSlash(filepath.Join(dir, "no-such-dir", "probe")), "refused"},
	} {
		out, err := exec.Command(sh, "-c", "(set -o posix) 2>/dev/null && set -o posix\n"+probe, "sh", c.path).Output()
		if err != nil || strings.TrimSpace(string(out)) != c.want {
			t.Errorf("the probe over %s answered %q, %v; want %s and exit 0", c.path, out, err, c.want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "probe")); !os.IsNotExist(err) {
		t.Errorf("the probe left its file: %v", err)
	}
}

// ⛔ A READ-ONLY GRANT WHOSE MOUNT CAME UP WRITABLE IS REMOUNTED READ-ONLY. BusyBox's
// mount drops ro for a Windows directory, measured on alpine, so every read-only
// grant there was refused, and its account could write in it until then. A
// remount read-only holds, also measured. A read-write grant is left as it is.
// WSL-188.
//
// ⚠ A BIND REMOUNT, as the boot script makes one: the mount point goes read-only
// and the filesystem is asked nothing. WSL-198.
func TestAReadOnlyGrantThatCameUpWritableIsRemountedReadOnly(t *testing.T) {
	grantsCase(t)
	for _, mode := range []string{"ro", "rw"} {
		mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: mode}
		dir := t.TempDir()
		read := grantsSeed(t, dir, "", "", "")
		stage(t, dir, "mount.drops-ro")
		out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target))
		if err != nil || !strings.Contains(out, "grant "+mode+" /workspaces/mine") {
			t.Fatalf("a %s grant whose mount drops ro answered %v:\n%s", mode, err, out)
		}
		remounted := strings.Contains(read("log"), "mount -o remount,bind,ro /workspaces/mine")
		if remounted != (mode == "ro") {
			t.Errorf("a %s grant whose mount drops ro was remounted read-only: %v\n%s", mode, remounted, read("log"))
		}
		if !strings.Contains(read("mounts"), " /workspaces/mine drvfs "+mode+",") {
			t.Errorf("a %s grant whose mount drops ro is mounted as:\n%s", mode, read("mounts"))
		}
	}
}

// ⛔ A READ-ONLY GRANT THE ACCOUNT CAN WRITE IS REFUSED, as it always was, now
// from the account's own answer.
func TestAReadOnlyGrantTheAccountCanWriteIsRefused(t *testing.T) {
	grantsCase(t)
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "ro"}
	dir := t.TempDir()
	read := grantsSeed(t, dir, "", "", "")
	stage(t, dir, "account.writes")
	out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target))
	if err == nil || !strings.Contains(out, "/workspaces/mine is writable by agent and granted read-only") {
		t.Fatalf("a read-only grant the account can write answered %v:\n%s", err, out)
	}
	if read("mounts") != "" {
		t.Errorf("a refused read-only grant left its mount:\n%s", read("mounts"))
	}
}
