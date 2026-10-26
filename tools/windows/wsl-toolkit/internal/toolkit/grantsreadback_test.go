// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"os"
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
func TestAGrantThatCannotBeReadBackIsRefused(t *testing.T) {
	grantsCase(t)
	for _, mode := range []string{"ro", "rw"} {
		mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: mode}
		dir := t.TempDir()
		read := grantsSeed(t, dir, "", "", "")
		stage(t, dir, "account.silent")
		out, err := grantsDrive(t, dir, []BaseMount{mine}, b64(mine.Target))
		if err == nil || !strings.Contains(out, "/workspaces/mine could not be read back as agent") {
			t.Fatalf("a %s grant the account could not read back answered %v:\n%s", mode, err, out)
		}
		if read("mounts") != "" || read("owners") != "" {
			t.Errorf("a %s grant that was not read back left its mount or its row:\nmounts:\n%s\nowners:\n%s", mode, read("mounts"), read("owners"))
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
