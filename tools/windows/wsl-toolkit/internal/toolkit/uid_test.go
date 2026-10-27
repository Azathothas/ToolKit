// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
)

// -- WSL-188: the account's ids are the ones it got ----------------------------

// accountSectionRun runs provision.sh's account creation with every command it
// calls stood in for, on an empty PATH, and answers what it printed and the
// creation's arguments. taken is the account that already holds uid 1000, or
// empty; tool is useradd or adduser, the one this guest has.
func accountSectionRun(t *testing.T, taken, tool string) (string, string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	dir := t.TempDir()
	at := filepath.ToSlash(dir)
	passwd := ""
	if taken != "" {
		passwd = taken + ":x:1000:1000::/home/" + taken + ":/bin/sh"
	}
	script := "set -eu\nTK_USER=agent\nTK_UID=1000\n" +
		"say() { printf '%s\\n' \"$*\"; }\ndie() { printf 'die: %s\\n' \"$*\"; exit 3; }\n" +
		"wanted_login_shell() { echo /bin/bash; }\n" +
		"getent() { [ \"$1 $2\" = 'passwd 1000' ] && [ -n '" + passwd + "' ] && printf '%s\\n' '" + passwd + "'; }\n" +
		"cut() { IFS=: read -r cut_first cut_rest; printf '%s\\n' \"$cut_first\"; }\n" +
		"id() { [ -e '" + at + "/made' ] || return 1; echo 1001; }\n" +
		tool + "() { printf '%s\\n' \"$*\" > '" + at + "/made'; }\n" +
		markedSection(t, provisionScript, "the account's creation")
	cmd := exec.Command(sh, "-c", script)
	empty := filepath.Join(dir, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd.Env = append(os.Environ(), "PATH="+empty)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the account section failed: %v\n%s", err, out)
	}
	made, _ := os.ReadFile(filepath.Join(dir, "made"))
	return string(out), strings.TrimSpace(string(made))
}

// ⛔ uid 1000 ONLY WHERE IT IS FREE. An image whose own account held it, as
// ubuntu's does, refused the build outright. The account takes the next free id,
// with useradd and with BusyBox's adduser.
func TestTheAccountTakesUid1000OnlyWhereItIsFree(t *testing.T) {
	parallelCase(t)
	for _, tool := range []string{"useradd", "adduser"} {
		_, made := accountSectionRun(t, "", tool)
		if !strings.Contains(made, "1000") {
			t.Errorf("%s on a free uid 1000 made the account with %q, want 1000 asked for", tool, made)
		}
		out, made := accountSectionRun(t, "ubuntu", tool)
		if strings.Contains(made, "1000") || !strings.Contains(made, "agent") {
			t.Errorf("%s with uid 1000 held by ubuntu made the account with %q, want no uid asked for", tool, made)
		}
		if !strings.Contains(out, "uid 1000 is ubuntu's") || !strings.Contains(out, "created agent as uid 1001") {
			t.Errorf("%s with uid 1000 held did not say so, or not which uid it made:\n%s", tool, out)
		}
	}
}

// ⛔ THE ID RANGE IS ONE NO OTHER ACCOUNT HOLDS. 100000 was written whoever held
// it, so an image's own account shared its ids with this account's containers.
func TestTheAccountsIdRangeStartsPastEveryOther(t *testing.T) {
	parallelCase(t)
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	dir := t.TempDir()
	at := filepath.ToSlash(dir)
	subuid := seedFile(t, dir, "subuid", "ubuntu:100000:65536\nbuild:200000:1000\n")
	subgid := seedFile(t, dir, "subgid", "")
	section := strings.NewReplacer("/etc/subuid", at+"/subuid", "/etc/subgid", at+"/subgid").
		Replace(markedSection(t, provisionScript, "the id ranges"))
	script := "set -eu\nTK_USER=agent\nsay() { printf '%s\\n' \"$*\"; }\n" + section
	if out, err := exec.Command(sh, "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("the id range section failed: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(subuid); !strings.HasSuffix(string(got), "\nagent:201000:65536\n") {
		t.Errorf("the account's subuid range overlaps another's:\n%s", got)
	}
	if got, _ := os.ReadFile(subgid); string(got) != "agent:100000:65536\n" {
		t.Errorf("an empty subgid did not get the range from 100000:\n%s", got)
	}
}

// ⛔ A GRANT NAMES THE ACCOUNT'S OWN IDS. The block said uid=1000,gid=1000 whatever
// the account was, so an account that took another uid owned none of its grants.
func TestAGrantNamesTheAccountsOwnIds(t *testing.T) {
	grantsCase(t)
	mine := BaseMount{Source: t.TempDir(), Target: "/workspaces/mine", Mode: "rw"}
	dir := t.TempDir()
	read := grantsSeed(t, dir, "", "", "")
	if err := forksafe.WriteFile(filepath.Join(dir, "account.ids"), []byte("1001 1002\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := grantsDriveIn(t, dir, GrantsWrite, []BaseMount{mine}, ""); err != nil {
		t.Fatalf("a grant for an account at 1001 answered %v:\n%s", err, out)
	}
	if fstab := read("fstab"); !strings.Contains(fstab, ",uid=1001,gid=1002,") || strings.Contains(fstab, "uid=1000") {
		t.Errorf("the block does not name the account's own ids:\n%s", fstab)
	}
}

// ⛔ AND THE VERIFIER REFUSES A GRANT MOUNTED FOR ANOTHER UID, so a base an
// earlier release granted for 1000 is re-provisioned with its account's ids.
// Options that name no uid are not judged.
func TestTheVerifierRefusesAGrantMountedForAnotherUid(t *testing.T) {
	parallelCase(t)
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	if !strings.Contains(string(verifyScript), "\n  assert_grant_owner \"$target\"\n") {
		t.Fatal("the verifier's explicit mount checks do not ask who owns the grant")
	}
	dir := t.TempDir()
	at := filepath.ToSlash(dir)
	section := strings.ReplaceAll(markedSection(t, verifyScript, "the grant owner"), "/proc/mounts", at+"/mounts")
	for _, c := range []struct {
		options string
		code    int
	}{
		{`rw,noatime,aname=drvfs;path=C:\x;uid=1000;gid=1000;metadata`, 3},
		{`rw,noatime,aname=drvfs;path=C:\x;uid=1001;gid=1001;metadata`, 0},
		{`rw,noatime,metadata,uid=1000,gid=1000`, 3},
		{`rw,noatime,aname=drvfs;path=C:\x;metadata`, 0},
	} {
		seedFile(t, dir, "mounts", `C:\134x /workspaces/x 9p `+c.options+" 0 0\n")
		cmd := exec.Command(sh, "-c", "set -eu\nid() { echo 1001; }\n"+section+"\nassert_grant_owner /workspaces/x\n")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var ee *exec.ExitError
			if !errors.As(err, &ee) {
				t.Fatal(err)
			}
			code = ee.ExitCode()
		}
		if code != c.code || (code != 0 && !strings.Contains(stderr.String(), "is mounted for uid 1000, and this account is uid 1001")) {
			t.Errorf("a grant mounted with %s answered exit %d, want %d: %s", c.options, code, c.code, stderr.String())
		}
	}
}
