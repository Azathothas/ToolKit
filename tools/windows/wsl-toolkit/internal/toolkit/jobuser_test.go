// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"strings"
	"testing"
	"time"
)

// The accounts ubuntu:24.04 carries, as measured on the base: uid 1000 is
// already `ubuntu`, which is the case WSL-123 names.
const (
	ubuntuPasswd = "root:x:0:0:root:/root:/bin/bash\nnobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin\nubuntu:x:1000:1000:Ubuntu:/home/ubuntu:/bin/bash\n"
	ubuntuGroup  = "root:x:0:\nsudo:x:27:ubuntu\nubuntu:x:1000:\nnogroup:x:65534:\n"
)

// TestAnAccountTheImageLacksTakesAnIDTheImageDoesNotHave is WSL-123. podman
// writes no passwd entry for a uid the image already holds, so a job told to be
// `dev` at 1000 on ubuntu:24.04 ran as `ubuntu`.
func TestAnAccountTheImageLacksTakesAnIDTheImageDoesNotHave(t *testing.T) {
	a, err := readAccounts("dev", ubuntuPasswd, ubuntuGroup)
	if err != nil || a.Exists || a.UID != 1001 || a.GID != 1001 || !a.NewGroup {
		t.Errorf("dev on ubuntu:24.04 resolved to %+v, %v; want a new account at 1001:1001", a, err)
	}
	a, err = readAccounts("ubuntu", ubuntuPasswd, ubuntuGroup)
	if err != nil || !a.Exists {
		t.Errorf("an account the image has resolved to %+v, %v; want it used as the image defines it", a, err)
	}
	// ⚠ A group of the account's name is used, and none is written.
	a, err = readAccounts("dev", "root:x:0:0::/root:/bin/sh\n", "root:x:0:\ndev:x:4242:\n")
	if err != nil || a.UID != 1000 || a.GID != 4242 || a.NewGroup {
		t.Errorf("an image with a dev group resolved to %+v, %v", a, err)
	}
	// ⚠ A gid the image holds is passed over as well as a uid.
	a, _ = readAccounts("dev", "root:x:0:0::/root:/bin/sh\n", "root:x:0:\nstaff:x:1000:\n")
	if a.UID != 1001 || a.GID != 1001 {
		t.Errorf("a taken gid 1000 resolved to %+v", a)
	}
}

// TestAMadeAccountReachesPodmanAsItsOwnEntries holds the engine's half: podman's
// own passwd and group entries, a home the engine gives to the account, and an
// account the image has named as it is.
func TestAMadeAccountReachesPodmanAsItsOwnEntries(t *testing.T) {
	runner := &Runner{}
	spec := JobSpec{Image: "docker.io/library/ubuntu:24.04", ContainerLifecycle: ContainerEphemeral, User: "runner", CreateUser: true}
	spec.account = &jobAccount{Name: "runner", UID: 1001, GID: 1001, NewGroup: true}
	script := string(runner.containerScript(spec, "wtk-one", jobLayout("/home/toolkit", "one"), "marker"))
	for _, want := range []string{
		"'--user' '1001:1001'",
		"'--passwd-entry' 'runner:*:1001:1001:runner:/home/runner:/bin/sh'",
		"'--group-entry' 'runner:x:1001:'",
		"'--tmpfs' '/home/runner:rw,exec,nosuid,nodev,mode=0700,U'",
		"'--volume' '/home/toolkit/.wsl-toolkit/jobs/one/work:/work:U,Z'",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the podman command does not carry %s:\n%s", want, script)
		}
	}
	if strings.Contains(script, "'--user' 'runner'") {
		t.Errorf("a made account is also passed by its name, which the image cannot resolve:\n%s", script)
	}
	spec.account = &jobAccount{Name: "ubuntu", Exists: true}
	spec.User = "ubuntu"
	existing := string(runner.containerScript(spec, "wtk-two", jobLayout("/home/toolkit", "two"), "marker"))
	if !strings.Contains(existing, "'--user' 'ubuntu'") || strings.Contains(existing, "--passwd-entry") || strings.Contains(existing, "--tmpfs") {
		t.Errorf("an account the image has is not used as it is:\n%s", existing)
	}
}

// TestTheAccountProbeRunsTheJobsOwnImage: the image, its platform and its pull
// rule are the job's, the container is this tool's to find, and root reads.
func TestTheAccountProbeRunsTheJobsOwnImage(t *testing.T) {
	script := string(accountProbeScript(JobSpec{Image: "docker.io/library/debian:latest", Platform: "linux/arm64"}, "0123456789abcdef"))
	for _, want := range []string{"'--rm'", "'--label' 'wsl-toolkit.owner=wsl-toolkit'", "'--pull=missing'", "'--platform' 'linux/arm64'", "'--user' '0:0'", "'docker.io/library/debian:latest'", accountGroupMark, accountSudoMark} {
		if !strings.Contains(script, want) {
			t.Errorf("the probe does not carry %s:\n%s", want, script)
		}
	}
}

// TestTheAccountProbeSpendsTheJobsDeadline: the probe's pull is the pull the
// container would have made inside its deadline, so the container gets what is
// left, never less than a second, and a job with no deadline keeps none.
func TestTheAccountProbeSpendsTheJobsDeadline(t *testing.T) {
	for _, c := range []struct{ timeout, spent, want time.Duration }{
		{10 * time.Minute, 4 * time.Minute, 6 * time.Minute},
		{10 * time.Minute, 0, 10 * time.Minute},
		{time.Minute, 2 * time.Minute, time.Second},
		{0, time.Minute, 0},
	} {
		if got := jobBudget(c.timeout, c.spent); got != c.want {
			t.Errorf("jobBudget(%s, %s) = %s, want %s", c.timeout, c.spent, got, c.want)
		}
	}
}

// TestCreateUserAndSudoNameAnAccount: each needs --user, --create-user makes a
// name and nothing else, and the sudo rule names the account or its uid.
func TestCreateUserAndSudoNameAnAccount(t *testing.T) {
	refused := []struct {
		user         string
		create, sudo bool
		want         string
	}{
		{"", true, false, "no --user was given"},
		{"", false, true, "no --user was given"},
		{"1000:1000", true, false, "not an account name"},
		{"Dev", true, false, "not an account name"},
		{strings.Repeat("a", 33), true, false, "1 to 32"},
		{"dev ops", false, true, "a sudoers rule can name"},
	}
	for _, c := range refused {
		if err := ValidateJobUser(c.user, c.create, c.sudo); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("ValidateJobUser(%q, %v, %v) = %v, want a refusal naming %q", c.user, c.create, c.sudo, err, c.want)
		}
	}
	for _, ok := range []struct {
		user         string
		create, sudo bool
	}{{"dev", true, true}, {"_build-1", true, false}, {"1000:1000", false, true}, {"", false, false}} {
		if err := ValidateJobUser(ok.user, ok.create, ok.sudo); err != nil {
			t.Errorf("ValidateJobUser(%q, %v, %v) was refused: %v", ok.user, ok.create, ok.sudo, err)
		}
	}
	for user, want := range map[string]string{"dev": "dev ALL=(ALL:ALL) NOPASSWD: ALL\n", "1000:1000": "#1000 ALL=(ALL:ALL) NOPASSWD: ALL\n", "ubuntu:adm": "ubuntu ALL=(ALL:ALL) NOPASSWD: ALL\n"} {
		if got, err := sudoersRule(user); err != nil || got != want {
			t.Errorf("sudoersRule(%q) = %q, %v; want %q", user, got, err, want)
		}
	}
}

// TestAJobFromTheHelperIsHeldToTheAccountRules: Run refuses --create-user or
// --sudo without --user before it reaches the guest, for a request that
// arrived over the helper.
func TestAJobFromTheHelperIsHeldToTheAccountRules(t *testing.T) {
	res := (&Runner{}).Run(t.Context(), JobSpec{
		Image: "docker.io/library/alpine:latest", ContainerLifecycle: ContainerEphemeral, Sudo: true,
	})
	if !res.Unreached || !strings.Contains(res.Error, "--sudo") {
		t.Errorf("a --sudo job with no --user answered %+v", res)
	}
}

// TestTheSudoRuleIsMountedReadOnlyAndRoots: sudo reads a rule only when root
// owns it, so it carries no U, as every other mount of a job with --user does.
func TestTheSudoRuleIsMountedReadOnlyAndRoots(t *testing.T) {
	runner := &Runner{}
	spec := JobSpec{Image: "docker.io/library/debian:latest", ContainerLifecycle: ContainerEphemeral, User: "dev", Sudo: true}
	script := string(runner.containerScript(spec, "wtk-one", jobLayout("/home/toolkit", "one"), "marker"))
	if !strings.Contains(script, "'--volume' '/home/toolkit/.wsl-toolkit/jobs/one/sudoers:"+SudoersMount+":ro,Z'") {
		t.Errorf("the rule is not mounted read-only and root's:\n%s", script)
	}
	spec.Sudo = false
	if without := string(runner.containerScript(spec, "wtk-two", jobLayout("/home/toolkit", "two"), "marker")); strings.Contains(without, "sudoers") {
		t.Errorf("a job without --sudo mounts a rule:\n%s", without)
	}
}
