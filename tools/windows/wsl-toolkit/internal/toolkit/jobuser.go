// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ⭐ A JOB CAN RUN AS AN ACCOUNT ITS IMAGE DOES NOT HAVE. WSL-123.
//
// A wrapper used to install sudo as root, run useradd, chown /work and drop its
// privileges inside every job. podman can write the account itself: --user
// UID:GID with --passwd-entry and --group-entry naming it, and a home the
// engine gives to that user. Nothing in the image changes and nothing runs as
// root to make it.

// ValidateJobUser holds --user, --create-user and --sudo to what they can mean
// together. WSL-123.
func ValidateJobUser(user string, create, sudo bool) error {
	if create {
		if user == "" {
			return errors.New("--create-user makes the account --user names, and no --user was given. Pass --user NAME")
		}
		if err := validateMadeAccountName(user); err != nil {
			return fmt.Errorf("--create-user: %w", err)
		}
	}
	if sudo {
		if user == "" {
			return errors.New("--sudo grants the account --user names, and no --user was given. Pass --user NAME, with --create-user where the image has no such account")
		}
		if _, err := sudoersRule(user); err != nil {
			return err
		}
	}
	return nil
}

// validateMadeAccountName holds a name --create-user makes to what every image's
// tools read as an account: [a-z_][a-z0-9_-]*, at most 32.
//
// ⚠ IT IS THE RULE FOR AN ACCOUNT THIS TOOL WRITES, so it is stricter than one
// for an account a base or an image already holds, and named apart from it.
func validateMadeAccountName(name string) error {
	if name == "" || len(name) > 32 {
		return fmt.Errorf("%q is not an account name: a name is 1 to 32 characters", name)
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r == '_':
		case i > 0 && (r >= '0' && r <= '9' || r == '-'):
		default:
			return fmt.Errorf("%q is not an account name: it starts with a lower-case letter or _, and holds only those, digits and -", name)
		}
	}
	return nil
}

// sudoersRule is the one line --sudo mounts: the account by name, or by uid
// where --user named a number.
func sudoersRule(user string) (string, error) {
	who, _, _ := strings.Cut(user, ":")
	if _, err := strconv.Atoi(who); err == nil {
		return "#" + who + " ALL=(ALL:ALL) NOPASSWD: ALL\n", nil
	}
	for i, r := range who {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case i > 0 && (r >= '0' && r <= '9' || r == '-' || r == '.'):
		default:
			return "", fmt.Errorf("--sudo: %q is not an account a sudoers rule can name", who)
		}
	}
	if who == "" {
		return "", errors.New("--sudo: --user names no account")
	}
	return who + " ALL=(ALL:ALL) NOPASSWD: ALL\n", nil
}

// SudoersMount is where --sudo puts its rule in the container.
//
// ⛔ IT IS NOT RE-OWNED. sudo reads a rule only when root owns it, and the file
// is the base account's, which is root inside the container. The `U` the job's
// other mounts carry would hand it to the job's own account.
const SudoersMount = "/etc/sudoers.d/wsl-toolkit-job"

// jobAccount is the account --create-user resolved for one job's image.
type jobAccount struct {
	Name string
	// Exists says the image has the account, so it is used as the image
	// defines it.
	Exists   bool
	UID, GID int
	// NewGroup says the image has no group of the account's name, so podman
	// writes one.
	NewGroup bool
	// HasSudo says the image carries sudo, which --sudo needs to mean anything.
	HasSudo bool
}

// The ids a made account may take. ⚠ The top stays inside the 65536 ids a
// rootless engine maps, and the bottom is where every image's own accounts end.
const (
	firstAccountID = 1000
	lastAccountID  = 60000
)

// readAccounts decides from an image's /etc/passwd and /etc/group.
//
// ⛔ AN ID THE IMAGE ALREADY HAS IS NOT TAKEN. podman writes no entry for a uid
// the image's passwd holds, so the job ran as that account: on ubuntu:24.04 a
// job told to be `dev` at 1000 ran as `ubuntu`. Measured here.
func readAccounts(name, passwd, group string) (jobAccount, error) {
	a := jobAccount{Name: name}
	uids := map[int]bool{}
	for _, line := range strings.Split(passwd, "\n") {
		f := strings.Split(strings.TrimSpace(line), ":")
		if len(f) < 4 {
			continue
		}
		if f[0] == name {
			a.Exists = true
			return a, nil
		}
		if uid, err := strconv.Atoi(f[2]); err == nil {
			uids[uid] = true
		}
	}
	gids := map[int]bool{}
	named := -1
	for _, line := range strings.Split(group, "\n") {
		f := strings.Split(strings.TrimSpace(line), ":")
		if len(f) < 3 {
			continue
		}
		gid, err := strconv.Atoi(f[2])
		if err != nil {
			continue
		}
		gids[gid] = true
		if f[0] == name {
			named = gid
		}
	}
	for id := firstAccountID; id <= lastAccountID; id++ {
		switch {
		case uids[id]:
		case named >= 0:
			a.UID, a.GID = id, named
			return a, nil
		case !gids[id]:
			a.UID, a.GID, a.NewGroup = id, id, true
			return a, nil
		}
	}
	return a, fmt.Errorf("the image holds every id from %d to %d, so no account can be made for %s", firstAccountID, lastAccountID, name)
}

// podmanArgs is what the engine is told for this account.
func (a jobAccount) podmanArgs() []string {
	if a.Exists {
		return []string{"--user", a.Name}
	}
	args := []string{
		"--user", fmt.Sprintf("%d:%d", a.UID, a.GID),
		"--passwd-entry", fmt.Sprintf("%s:*:%d:%d:%s:/home/%s:/bin/sh", a.Name, a.UID, a.GID, a.Name, a.Name),
	}
	if a.NewGroup {
		args = append(args, "--group-entry", fmt.Sprintf("%s:x:%d:", a.Name, a.GID))
	}
	// ⚠ THE HOME IS A TMPFS THE ENGINE GIVES TO THE ACCOUNT. podman refuses uid=
	// and gid= as tmpfs options and takes U, which is measured here. A cache at a
	// path under it outlives the job; the rest is gone with it.
	return append(args, "--tmpfs", "/home/"+a.Name+":rw,exec,nosuid,nodev,mode=0700,U")
}

// The marks between the three answers the probe gives.
const (
	accountGroupMark = "#wsl-toolkit-group"
	accountSudoMark  = "#wsl-toolkit-sudo"
)

// accountProbeScript reads an image's accounts with the job's own image,
// platform and pull rule, as root in a container of its own that is removed.
//
// ⭐ IT CARRIES THIS TOOL'S LABEL, so a probe whose caller was killed is one gc
// finds.
func accountProbeScript(spec JobSpec, id string) []byte {
	args := []string{
		"run", "--rm", "--name", "wtk-" + id + "-accounts", "--label", JobLabel,
		"--pull=missing", "--network", "none", "--log-driver", "none",
		"--user", "0:0", "--entrypoint", "/bin/sh",
	}
	if spec.Platform != "" {
		args = append(args, "--platform", spec.Platform)
	}
	args = append(args, spec.Image, "-c",
		"cat /etc/passwd 2>/dev/null; echo '"+accountGroupMark+"'; cat /etc/group 2>/dev/null; echo '"+accountSudoMark+"'; command -v sudo 2>/dev/null || :")
	var b strings.Builder
	b.WriteString("exec podman")
	for _, a := range args {
		b.WriteString(" ")
		b.WriteString(shellQuote(a))
	}
	b.WriteString("\n")
	return []byte(b.String())
}

// jobBudget is what a job's deadline leaves for its container once the account
// probe has run.
//
// ⛔ ONE DEADLINE FOR THE PULL AND THE RUN. Without --create-user the pull is
// inside `podman run`, and so inside the deadline. The probe makes that pull
// first, and the job then had its whole deadline again. WSL-123.
func jobBudget(timeout, spent time.Duration) time.Duration {
	if timeout <= 0 {
		return timeout
	}
	if left := timeout - spent; left > time.Second {
		return left
	}
	return time.Second
}

// resolveAccount asks the image which accounts it has, and decides.
func (r *Runner) resolveAccount(ctx context.Context, spec JobSpec, id string) (jobAccount, error) {
	// ⚠ BOUNDED BY THE JOB'S OWN DEADLINE where it has one, since the pull an
	// ordinary job makes inside that deadline is made here instead.
	timeout := 30 * time.Minute
	if spec.Timeout > 0 {
		timeout = spec.Timeout
	}
	out, stderr, code, err := r.baseCapture(ctx, accountProbeScript(spec, id), timeout)
	if err != nil || code != 0 {
		return jobAccount{}, fmt.Errorf("--create-user could not read the accounts of %s (exit %d): %s", spec.Image, code, engineFailure(stderr, err))
	}
	passwd, rest, _ := strings.Cut(out, accountGroupMark)
	group, sudo, _ := strings.Cut(rest, accountSudoMark)
	a, err := readAccounts(spec.User, passwd, group)
	a.HasSudo = strings.TrimSpace(sudo) != ""
	return a, err
}
