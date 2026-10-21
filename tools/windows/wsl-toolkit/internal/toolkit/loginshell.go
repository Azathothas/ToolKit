// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	_ "embed"
	"strings"
	"time"
)

// loginShellScript defines wanted_login_shell and apply_login_shell. WSL-143.
//
//go:embed loginshell.sh
var loginShellScript []byte

// AccountState is the managed account's passwd entry as verify.sh read it back,
// under --probe. ⚠ Nil where the probe did not run or the guest is older than
// the rows, which is not the same as an account with no shell.
type AccountState struct {
	// LoginShell is the shell the passwd entry names, which an SSH session and
	// `wsl.exe -d NAME -u USER` with no command start.
	LoginShell string `json:"login_shell"`
	// Bash is where the account's PATH finds bash, or empty where it finds none.
	Bash string `json:"bash,omitempty"`
	// Passwd is what the account reads of its own passwd entry: ok, unreadable,
	// or no-entry. ⚠ Empty where the guest is older than the row. WSL-137.
	Passwd string `json:"passwd,omitempty"`
}

// parseAccount reads verify.sh's account rows. ⚠ Only a run that printed the
// login shell or the passwd row answers; one that printed neither answers nil.
func parseAccount(out string) *AccountState {
	var st *AccountState
	account := func() *AccountState {
		if st == nil {
			st = &AccountState{}
		}
		return st
	}
	bash := ""
	for _, line := range strings.Split(out, "\n") {
		key, value, _ := strings.Cut(strings.TrimSpace(line), " ")
		value = strings.TrimSpace(value)
		switch key {
		case "login-shell":
			account().LoginShell = value
		case "account-passwd":
			state, _, _ := strings.Cut(value, " ")
			account().Passwd = state
		case "login-bash":
			if value != "none" {
				bash = value
			}
		}
	}
	if st != nil {
		st.Bash = bash
	}
	return st
}

// accountPasswdRemediation is the finding an account that cannot read its own
// passwd entry earns. WSL-137.
//
// ⛔ ADVISORY, BECAUSE A CONTAINER RUNS EITHER WAY. The verdict of `base status
// --probe` and what `ready` and `base ensure` do with a base are as they were:
// this names the condition and the command, and changes nothing else.
func accountPasswdRemediation(acct *AccountState) (Remediation, bool) {
	if acct == nil {
		return Remediation{}, false
	}
	const costs = "a program run as the account cannot name a user: ls -l prints numbers, " +
		"and ssh, sudo and git can refuse. A container runs either way"
	switch acct.Passwd {
	case "unreadable":
		return Remediation{
			ID:         "account-passwd",
			What:       "the managed account cannot read /etc/passwd",
			Costs:      costs,
			Command:    ToolCommand("base exec --root -c 'chmod 0644 /etc/passwd'"),
			Repairable: true,
			Advisory:   true,
		}, true
	case "no-entry":
		return Remediation{
			ID:         "account-passwd",
			What:       "/etc/passwd holds no entry the managed account can read for its own uid",
			Costs:      costs,
			Command:    ToolCommand("base recreate"),
			Repairable: false,
			Advisory:   true,
		}, true
	}
	return Remediation{}, false
}

// loginShellRemediation is the finding an account still on /bin/sh earns where
// the guest has bash: a base built before 7.1.0, which `base ensure` moves.
//
// ⛔ ONLY /bin/sh, BECAUSE IT IS THE SHELL `base ensure` MOVES. A remediation
// over zsh would name a command that leaves zsh where it is.
func loginShellRemediation(acct *AccountState) (Remediation, bool) {
	if acct == nil || acct.LoginShell != "/bin/sh" || acct.Bash == "" {
		return Remediation{}, false
	}
	return Remediation{
		ID:   "login-shell-not-bash",
		What: "the managed account's login shell is /bin/sh, and this base has bash at " + acct.Bash,
		Costs: "an SSH session and wsl.exe with no command start in sh, with no line editing, " +
			"no history and no bash login files. A container runs either way",
		Command:    ToolCommand("base ensure"),
		Repairable: true,
		Advisory:   true,
	}, true
}

// accountStartRemediation classifies a verification WSL refused before any of it
// ran, because it could not start anything as the managed account. WSL-137.
//
// ⛔ wsl.exe RUNS EVERY COMMAND THROUGH THE ACCOUNT'S LOGIN SHELL. Measured on a
// scratch base: with the passwd shell set to a path that is no program, the
// verifier answered `execvpe(/bin/no-such-shell) failed` before a line of it ran,
// and so does every job and `base exec`. So verify.sh's own passwd checks can only
// speak once WSL has started it, and these two are WSL's words.
//
// ⭐ `base ensure` REPAIRS BOTH. Re-provisioning runs as root, creates an account
// with no entry, and replaces a login shell that is no program.
func accountStartRemediation(msg string) (Remediation, bool) {
	var what string
	switch {
	case strings.Contains(msg, "execvpe(") && strings.Contains(msg, "failed"):
		what = "WSL cannot start the managed account's login shell, which its passwd entry names"
	case strings.Contains(msg, "getpwnam(") && strings.Contains(msg, "failed"):
		what = "WSL cannot find the managed account's passwd entry"
	default:
		return Remediation{}, false
	}
	return Remediation{
		ID:   "account-cannot-start",
		What: what + ", and wsl.exe runs every command as the account through that shell",
		Costs: "nothing runs as the account: no container, no base exec and no agent. " +
			"If root's entry is gone too, only base recreate rebuilds it",
		Command:    ToolCommand("base ensure"),
		Repairable: true,
	}, true
}

// applyLoginShell gives the managed account bash as its login shell where the
// guest has bash. It runs in every `base ensure`, so a base an older release
// built is moved by the next ensure. WSL-143.
//
// ⚠ A FAILURE IS LOGGED AND STOPS NOTHING. The base runs containers whatever its
// account's shell is, and `base status --probe` names the shell it still has.
func (b *Base) applyLoginShell(ctx context.Context) {
	script := append([]byte("set -eu\n"), loginShellScript...)
	script = append(script, []byte("\napply_login_shell \"$TK_USER\"\n")...)
	out, stderr, code, err := b.captureAs(ctx, "root", script, map[string]string{"TK_USER": b.cfg.Base.User}, 2*time.Minute)
	if err != nil || code != 0 {
		b.log("the account's login shell was not set: " + guestFailure(stderr, out))
		return
	}
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "login-shell ") {
			b.log(strings.Replace(line, "login-shell ", "login shell: ", 1))
		}
	}
}
