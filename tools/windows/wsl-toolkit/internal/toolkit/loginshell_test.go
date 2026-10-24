// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestTheAccountRowsAreReadAndOnlySlashBinShEarnsTheRemediation holds the read of
// verify.sh's two account rows and the one shell `base ensure` moves. ⛔ A
// remediation over zsh would name a command that leaves zsh where it is. WSL-143.
func TestTheAccountRowsAreReadAndOnlySlashBinShEarnsTheRemediation(t *testing.T) {
	acct := parseAccount("automount off\nlogin-shell /bin/sh\nlogin-bash /usr/bin/bash\nengine x\n")
	if acct == nil || acct.LoginShell != "/bin/sh" || acct.Bash != "/usr/bin/bash" {
		t.Fatalf("the rows read as %+v", acct)
	}
	if r, ok := loginShellRemediation(acct); !ok || r.ID != "login-shell-not-bash" || !r.Advisory || r.Command != ToolCommand("base ensure") {
		t.Fatalf("an account on /bin/sh beside bash earned %+v, %v", r, ok)
	}
	if acct := parseAccount("login-shell /bin/sh\nlogin-bash none\n"); acct == nil || acct.Bash != "" {
		t.Fatalf("a guest with no bash read as %+v", acct)
	} else if _, ok := loginShellRemediation(acct); ok {
		t.Error("an account on /bin/sh where the guest has no bash earned a remediation nothing can take")
	}
	if _, ok := loginShellRemediation(parseAccount("login-shell /usr/bin/zsh\nlogin-bash /usr/bin/bash\n")); ok {
		t.Error("an account somebody gave zsh earned a remediation that would leave it on zsh")
	}
	if acct := parseAccount("engine x\n"); acct != nil {
		t.Errorf("a guest older than the rows read as %+v rather than unknown", acct)
	}
}

// TestEveryEnsureHoldsTheAccountToBash builds through the fake WSL and holds that
// the ensure sent apply_login_shell as root, after the base answered. WSL-143.
func TestEveryEnsureHoldsTheAccountToBash(t *testing.T) {
	f := newBuildFakeWsl(t)
	b := f.base(buildFakeConfig("wsl-toolkit-shell", AutomountReadOnly), nil)
	b.archive = writeTinyArchive
	if _, err := b.EnsureWith(t.Context(), false, false); err != nil {
		t.Fatal(err)
	}
	calls := f.state().scripts()
	if at := indexOf(calls, "loginshell@root"); at < 0 || at < indexOf(calls, "verify@agent") {
		t.Fatalf("the login shell was not set after the base answered; calls %v", calls)
	}
	// ⚠ And the account is created with the shell those functions choose.
	payload := string(provisionPayload())
	if !strings.Contains(payload, "\nwanted_login_shell() {") || !strings.Contains(string(provisionScript), `TK_LOGIN_SHELL=$(wanted_login_shell)`) ||
		!strings.Contains(string(provisionScript), `--shell "$TK_LOGIN_SHELL"`) || !strings.Contains(string(provisionScript), `-s "$TK_LOGIN_SHELL"`) {
		t.Error("provisioning does not create the account with the shell loginshell.sh chooses")
	}
	// ⛔ AND A RE-PROVISIONING MOVES AN ACCOUNT THAT EXISTS, or a shell the
	// verifier refused could never be repaired: the ensure's own call comes only
	// after the base answers.
	if !strings.Contains(string(provisionScript), `login_said=$(apply_login_shell "$TK_USER")`) {
		t.Error("provisioning does not hold an existing account to the shell loginshell.sh chooses")
	}
}

// TestANonOwnerEnsureLeavesTheLoginShell holds the owner gate. ⛔ The account is
// named by the caller's configuration, and an ensure from a state directory that
// does not own the base changed it in the owner's base as root. WSL-143, WSL-167.
func TestANonOwnerEnsureLeavesTheLoginShell(t *testing.T) {
	f := newBuildFakeWsl(t)
	var logged []string
	b := f.base(buildFakeConfig("wsl-toolkit-notmine", AutomountReadOnly), func(s string) { logged = append(logged, s) })
	b.archive = writeTinyArchive
	if _, err := b.EnsureWith(t.Context(), false, false); err != nil {
		t.Fatal(err)
	}
	f.update(func(s *buildFakeState) {
		s.Distros["wsl-toolkit-notmine"].Dir = filepath.Join(f.dir, "owner", "wsl-toolkit-notmine")
		s.Calls = nil
	})
	logged = nil
	if _, err := b.EnsureWith(t.Context(), false, false); err != nil {
		t.Fatalf("an ensure from a state directory that uses the base failed: %v; calls %v", err, f.state().scripts())
	}
	if at := indexOf(f.state().scripts(), "loginshell@root"); at >= 0 {
		t.Fatalf("a non-owner ensure changed the account's login shell; calls %v", f.state().scripts())
	}
	if !strings.Contains(strings.Join(logged, "\n"), "the account's login shell is left as it is") {
		t.Errorf("the ensure did not say it left the login shell: %q", logged)
	}
}

// TestAStatusSaysWhetherItRanAContainer holds the label. ⛔ `healthy: false`
// without --probe is no verdict, and a caller reading JSON could not tell it from a
// base that cannot run anything. WSL-137.
func TestAStatusSaysWhetherItRanAContainer(t *testing.T) {
	f := newBuildFakeWsl(t)
	b := f.base(buildFakeConfig("wsl-toolkit-label", AutomountReadOnly), nil)
	f.update(func(s *buildFakeState) { s.Distros["wsl-toolkit-label"] = &buildFakeDistro{Name: "wsl-toolkit-label"} })
	if err := f.w.WriteIdentity(t.Context(), "wsl-toolkit-label", Identity{Image: b.cfg.Base.Image, User: "agent"}); err != nil {
		t.Fatal(err)
	}
	plain, err := b.Status(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Probed || !strings.Contains(string(doc), `"probed":false`) {
		t.Fatalf("a status without --probe does not say so: %s", doc)
	}
	probed, err := b.Status(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !probed.Probed || !probed.Healthy {
		t.Fatalf("a probed status reads probed %v, healthy %v, problems %q", probed.Probed, probed.Healthy, probed.Problems)
	}
}

// standInUsermod rewrites the shell field of one account in the staged passwd
// file, with the shell alone, as `usermod -s SHELL USER` would.
const standInUsermod = `[ "$1" = -s ] || exit 2
shell=$2 user=$3 body=''
while IFS= read -r line || [ -n "$line" ]; do
  case "$line" in
    "$user":*) line=${line%:*}:$shell ;;
  esac
  body="$body$line
"
done < "$TK_PASSWD"
printf '%s' "$body" > "$TK_PASSWD"`

// TestApplyLoginShellMovesOnlySlashBinSh runs loginshell.sh in a real shell over a
// staged passwd file, with usermod and bash stood in for.
func TestApplyLoginShellMovesOnlySlashBinSh(t *testing.T) {
	posixShell(t)
	type shellCase struct {
		name, now, want, says string
		bash                  bool
		// last says the account's line is the file's last, with no newline.
		last bool
	}
	cases := []shellCase{
		{name: "the shell older releases gave it", now: "/bin/sh", want: "BASH", bash: true, says: "was /bin/sh"},
		{name: "a shell somebody chose", now: "ZSH", want: "ZSH", bash: true, says: "left as it is"},
		{name: "bash already", now: "BASH", want: "BASH", bash: true},
		{name: "a guest with no bash", now: "/bin/sh", want: "/bin/sh", says: "has no bash"},
		{name: "a shell that is not a program any more", now: "/bin/no-such-shell", want: "BASH", bash: true, says: "was /bin/no-such-shell"},
		{name: "a broken shell where the guest has no bash", now: "/bin/no-such-shell", want: "/bin/sh", says: "was /bin/no-such-shell"},
		{name: "a last line with no newline", now: "/bin/sh", want: "BASH", bash: true, says: "was /bin/sh", last: true},
	}
	for _, c := range cases {
		dir := t.TempDir()
		bin := filepath.Join(dir, "bin")
		if err := os.Mkdir(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		bash := filepath.Join(dir, "bash")
		zsh := filepath.Join(dir, "zsh")
		writeStandIn(t, dir, "zsh", "exit 0")
		if c.bash {
			writeStandIn(t, dir, "bash", "exit 0")
		}
		writeStandIn(t, bin, "usermod", standInUsermod)
		subst := strings.NewReplacer("BASH", bash, "ZSH", zsh)
		passwd := filepath.Join(dir, "passwd")
		body := "root:x:0:0:root:/root:/bin/sh\nagent:x:1000:1000::/var/lib/agent:" + subst.Replace(c.now)
		if !c.last {
			body += "\n"
		}
		if err := os.WriteFile(passwd, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		script := string(loginShellScript)
		for from, to := range map[string]string{
			"for wls_candidate in /bin/bash /usr/bin/bash; do": "for wls_candidate in " + bash + "; do",
		} {
			if strings.Count(script, from) != 1 {
				t.Fatalf("loginshell.sh names %q %d times, want once", from, strings.Count(script, from))
			}
			script = strings.Replace(script, from, to, 1)
		}
		if n := strings.Count(script, "done < /etc/passwd"); n != 2 {
			t.Fatalf("loginshell.sh reads /etc/passwd %d times, want twice", n)
		}
		script = strings.ReplaceAll(script, "done < /etc/passwd", `done < "$TK_PASSWD"`)
		cmd := exec.Command("/bin/sh", "-c", "set -eu\n"+script+"\napply_login_shell agent\n")
		// ⛔ PATH IS THE STAND-IN DIRECTORY, or this host's own bash and usermod answer.
		cmd.Env = []string{"PATH=" + bin, "TK_PASSWD=" + passwd}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			var exited *exec.ExitError
			if !errors.As(err, &exited) {
				t.Fatal(err)
			}
			t.Errorf("%s: exit %d, stderr %q", c.name, exited.ExitCode(), stderr.String())
			continue
		}
		after, err := os.ReadFile(passwd)
		if err != nil {
			t.Fatal(err)
		}
		if want := "agent:x:1000:1000::/var/lib/agent:" + subst.Replace(c.want) + "\n"; !strings.HasSuffix(string(after), want) {
			t.Errorf("%s: the passwd file reads %q, want it to end %q", c.name, after, want)
		}
		if !strings.HasPrefix(string(after), "root:x:0:0:root:/root:/bin/sh\n") {
			t.Errorf("%s: root's entry was changed: %q", c.name, after)
		}
		if c.says != "" && !strings.Contains(stdout.String(), c.says) {
			t.Errorf("%s: said %q, want %q", c.name, stdout.String(), c.says)
		}
	}
}

// passwdSection is verify.sh's passwd section reading a staged file.
func passwdSection(t *testing.T, passwd string) string {
	t.Helper()
	script := string(verifyScript)
	begin := strings.Index(script, "# >>> the account's passwd entry: begin\n")
	end := strings.Index(script, "# <<< the account's passwd entry: end\n")
	if begin < 0 || end < begin {
		t.Fatalf("the verifier's passwd section is not marked: begin at %d, end at %d", begin, end)
	}
	section := script[begin:end]
	if n := strings.Count(section, "/etc/passwd"); n < 2 {
		t.Fatalf("the passwd section names /etc/passwd %d times", n)
	}
	return strings.ReplaceAll(section, "/etc/passwd", passwd)
}

// TestTheVerifierChecksTheAccountsPasswdEntry runs verify.sh's passwd section as
// this process's own account, over staged passwd files. ⛔ A base whose passwd
// file had gone bad was first named by a container refused in words about user
// namespaces. WSL-137.
func TestTheVerifierChecksTheAccountsPasswdEntry(t *testing.T) {
	posixShell(t)
	uid := strconv.Itoa(os.Getuid())
	home := t.TempDir()
	// ⚠ ROOT'S LINE IS STAGED ONLY WHERE THIS PROCESS IS NOT ROOT. A container
	// case runs as uid 0, and the staged account then stands in for root.
	others := "root:x:0:0:root:/root:/bin/sh\n"
	if uid == "0" {
		others = "daemon:x:1:1::/usr/sbin:/usr/sbin/nologin\n"
	}
	run := func(entry string) (string, string, int) {
		t.Helper()
		passwd := filepath.Join(t.TempDir(), "passwd")
		if err := os.WriteFile(passwd, []byte(others+entry), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("/bin/sh", "-c", "set -eu\n"+passwdSection(t, passwd))
		cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + home}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := 0
		if err := cmd.Run(); err != nil {
			var exited *exec.ExitError
			if !errors.As(err, &exited) {
				t.Fatal(err)
			}
			code = exited.ExitCode()
		}
		return stdout.String(), stderr.String(), code
	}
	entry := func(home, shell string) string {
		return fmt.Sprintf("agent:x:%s:%s::%s:%s\n", uid, uid, home, shell)
	}
	out, stderr, code := run(entry(home, "/bin/sh"))
	if code != 0 || !strings.Contains(out, "account-passwd ok\n") || !strings.Contains(out, "login-shell /bin/sh\n") || !strings.Contains(out, "login-bash ") {
		t.Fatalf("a whole entry answered exit %d, %q, %q", code, out, stderr)
	}
	// ⚠ A LAST LINE WITH NO NEWLINE IS STILL THE ACCOUNT'S ENTRY.
	if out, stderr, code := run(strings.TrimSuffix(entry(home, "/bin/sh"), "\n")); code != 0 || !strings.Contains(out, "account-passwd ok\n") {
		t.Errorf("an entry on a last line with no newline answered exit %d, %q, %q", code, out, stderr)
	}
	// ⛔ A ROW, NOT A REFUSAL: the verifier goes on, and the verdict is unchanged.
	if out, stderr, code := run(""); code != 0 || !strings.Contains(out, "account-passwd no-entry "+uid+"\n") {
		t.Errorf("no entry for the account: exit %d, %q, %q", code, out, stderr)
	}
	// ⚠ ROOT READS A FILE OF ANY MODE, so a file this case cannot read is staged
	// as one that is not there, which `-r` refuses for root too. The scratch base
	// measured a 0600 file as the account.
	cmd := exec.Command("/bin/sh", "-c", "set -eu\n"+passwdSection(t, filepath.Join(t.TempDir(), "no-passwd")))
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + home}
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "account-passwd unreadable\n") {
		t.Errorf("an unreadable passwd file answered %v, %q", err, out)
	}
}

// TestAnAccountThatCannotReadItsEntryIsAnAdvisory holds the finding and the
// verdict beside it. ⛔ A base whose account cannot read /etc/passwd runs its
// containers, and it answered `usable true` before this release, so it still
// does: the finding is advisory and names the command. WSL-137.
func TestAnAccountThatCannotReadItsEntryIsAnAdvisory(t *testing.T) {
	acct := parseAccount("account-passwd unreadable\nlogin-bash /usr/bin/bash\n")
	if acct == nil || acct.Passwd != "unreadable" || acct.LoginShell != "" {
		t.Fatalf("the rows read as %+v", acct)
	}
	r, ok := accountPasswdRemediation(acct)
	if !ok || r.ID != "account-passwd" || !r.Advisory || !strings.Contains(r.Command, "chmod 0644 /etc/passwd") {
		t.Fatalf("an account that cannot read /etc/passwd earned %+v, %v", r, ok)
	}
	if r, ok := accountPasswdRemediation(parseAccount("account-passwd no-entry 1000\n")); !ok || !r.Advisory || r.Repairable {
		t.Errorf("an account with no entry earned %+v, %v", r, ok)
	}
	if _, ok := accountPasswdRemediation(parseAccount("account-passwd ok\nlogin-shell /bin/bash\n")); ok {
		t.Error("an account that reads its entry earned a finding")
	}
	f := newBuildFakeWsl(t)
	b := f.base(buildFakeConfig("wsl-toolkit-passwd", AutomountReadOnly), nil)
	f.update(func(s *buildFakeState) {
		s.Distros["wsl-toolkit-passwd"] = &buildFakeDistro{Name: "wsl-toolkit-passwd"}
		s.Passwd = "unreadable"
	})
	if err := f.w.WriteIdentity(t.Context(), "wsl-toolkit-passwd", Identity{Image: b.cfg.Base.Image, User: "agent"}); err != nil {
		t.Fatal(err)
	}
	st, err := b.Status(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range st.Remediations {
		found = found || r.ID == "account-passwd"
	}
	if !st.Healthy || !found || st.Account == nil || st.Account.Passwd != "unreadable" {
		t.Fatalf("an account that cannot read /etc/passwd reads healthy %v, account %+v, remediations %+v", st.Healthy, st.Account, st.Remediations)
	}
}

// TestAnAccountWSLCannotStartNamesTheRepair holds the classification of WSL's
// own refusal, which comes before verify.sh can run. ⛔ Measured: a login shell
// at a path that is no program answered `execvpe(...) failed` for every command
// as the account. WSL-137.
func TestAnAccountWSLCannotStartNamesTheRepair(t *testing.T) {
	for _, msg := range []string{
		"a container did not run as toolkit (exit 1): <3>WSL (9956 - Relay) ERROR: CreateProcessCommon:875: execvpe(/bin/no-such-shell) failed: No such file or directory",
		"a container did not run as toolkit (exit 1): <3>WSL (1 - ) ERROR: CreateProcessParseCommon:711: getpwnam(toolkit) failed 0",
	} {
		r, ok := accountStartRemediation(msg)
		if !ok || r.ID != "account-cannot-start" || r.Advisory || r.Command != ToolCommand("base ensure") {
			t.Errorf("%q earned %+v, %v", msg, r, ok)
		}
	}
	if _, ok := accountStartRemediation("a container did not run as toolkit (exit 125): Error: short-name resolution enforced"); ok {
		t.Error("an engine refusal was read as an account WSL cannot start")
	}
	// ⭐ AND `base status --probe` CARRIES IT, through the fake WSL.
	f := newBuildFakeWsl(t)
	b := f.base(buildFakeConfig("wsl-toolkit-noshell", AutomountReadOnly), nil)
	f.update(func(s *buildFakeState) {
		s.Distros["wsl-toolkit-noshell"] = &buildFakeDistro{Name: "wsl-toolkit-noshell"}
		s.VerifyRefusal = "<3>WSL (281 - Relay) ERROR: CreateProcessCommon:875: execvpe(/bin/no-such-shell) failed: No such file or directory"
	})
	if err := f.w.WriteIdentity(t.Context(), "wsl-toolkit-noshell", Identity{Image: b.cfg.Base.Image, User: "agent"}); err != nil {
		t.Fatal(err)
	}
	st, err := b.Status(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range st.Remediations {
		found = found || r.ID == "account-cannot-start"
	}
	if st.Healthy || !found {
		t.Fatalf("a login shell WSL cannot start reads healthy %v, remediations %+v", st.Healthy, st.Remediations)
	}
}
