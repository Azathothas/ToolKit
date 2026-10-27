// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
)

// -- WSL-136: a machine-wide cap on running jobs -----------------------------

// runningJob claims a job and marks it running, and answers its owner.
func runningJob(t *testing.T, home, id string) *JobOwner {
	t.Helper()
	owner, err := ClaimJob(home, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := markAdmitted(home, id); err != nil {
		t.Fatal(err)
	}
	return owner
}

func fastRunningPoll(t *testing.T) {
	t.Helper()
	poll, say := runningPoll, runningSay
	t.Cleanup(func() { runningPoll, runningSay = poll, say })
	runningPoll, runningSay = 20*time.Millisecond, time.Hour
}

// ⭐ A job counts where its owner holds its lock and it was let in to run, in
// every instance, and nowhere else.
func TestOnlyAnAdmittedJobWithALiveOwnerCountsAsRunning(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", root)
	t.Setenv(InstanceRootEnv, root)
	selectInstance(t, DefaultInstance)
	live := runningJob(t, root, "aaaaaaaaaaaaaaaa")
	defer live.Release()
	other := runningJob(t, filepath.Join(root, "instances", "podbox"), "bbbbbbbbbbbbbbbb")
	defer other.Release()
	waiting, err := ClaimJob(root, "cccccccccccccccc")
	if err != nil {
		t.Fatal(err)
	}
	defer waiting.Release()
	killed := runningJob(t, root, "dddddddddddddddd")
	killed.Release()

	got := RunningJobs()
	if len(got) != 2 {
		t.Fatalf("%d jobs read as running, %v; want the admitted one of each instance", len(got), got)
	}
	names := runningList(got)
	if !strings.Contains(names, "job aaaaaaaaaaaaaaaa") || !strings.Contains(names, "job bbbbbbbbbbbbbbbb of instance podbox") {
		t.Errorf("the running jobs are named %q", names)
	}
}

// ⛔ An ended job's mark goes with its end.
func TestAnEndedJobIsNotMarkedRunning(t *testing.T) {
	home := t.TempDir()
	owner := runningJob(t, home, testJobID)
	if err := owner.Finish(JobResult{ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "jobs", testJobID, admittedName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an ended job still carries its running mark: %v", err)
	}
}

// ⛔ Over the cap with no wait is refused at once and says so; with a wait the
// job is let in when a running job ends, and marked running; a stop ends the
// wait.
func TestAJobOverTheCapWaitsOrIsRefusedAndSaysWhich(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", root)
	t.Setenv(InstanceRootEnv, root)
	selectInstance(t, DefaultInstance)
	fastRunningPoll(t)
	ctx := context.Background()
	busy := runningJob(t, root, "aaaaaaaaaaaaaaaa")
	me, err := ClaimJob(root, testJobID)
	if err != nil {
		busy.Release()
		t.Fatal(err)
	}
	defer me.Release()

	_, err = AdmitRunning(ctx, me, 1, 0, nil, nil)
	if !errors.Is(err, ErrOverRunningCap) || !strings.Contains(err.Error(), "refused at once") || !strings.Contains(err.Error(), "aaaaaaaaaaaaaaaa") {
		busy.Release()
		t.Fatalf("a job over the cap with no wait answered %v", err)
	}
	if err := RunningCapRefusal(1); !errors.Is(err, ErrOverRunningCap) {
		t.Errorf("the probe a detaching client makes answered %v", err)
	}

	_, err = AdmitRunning(ctx, me, 1, 5*time.Second, nil, func() bool { return true })
	if !errors.Is(err, ErrStoppedWaitingToRun) {
		busy.Release()
		t.Fatalf("a stop sent to a waiting job answered %v", err)
	}

	var said []string
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(100 * time.Millisecond)
		busy.Release()
	}()
	waited, err := AdmitRunning(ctx, me, 1, 5*time.Second, func(s string) { said = append(said, s) }, nil)
	<-released
	if err != nil {
		t.Fatalf("a job that waited for a slot answered %v", err)
	}
	if waited < 50*time.Millisecond || len(said) == 0 || !strings.Contains(said[0], "waits for a running slot") {
		t.Errorf("the job waited %s and said %q; want it to wait and say so", waited, said)
	}
	if _, err := os.Stat(filepath.Join(root, "jobs", testJobID, admittedName)); err != nil {
		t.Errorf("an admitted job carries no running mark: %v", err)
	}
	if err := RunningCapRefusal(2); err != nil {
		t.Errorf("one job running under a cap of two was refused: %v", err)
	}
}

// ⭐ A job a runner reaches is marked running, whatever owns it.
func TestARunnerMarksTheJobItRunsAsRunning(t *testing.T) {
	home := t.TempDir()
	owner, err := ClaimJob(home, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	r := &Runner{cfg: DefaultConfig(), wsl: &Wsl{Path: filepath.Join(home, "no-wsl.exe")}, home: home, log: func(string) {}}
	res := r.Run(context.Background(), JobSpec{
		ID: testJobID, Owner: owner, Image: "docker.io/library/alpine:latest",
		ContainerLifecycle: ContainerEphemeral, Platform: "linux/amd64",
	})
	if !res.Unreached {
		t.Fatalf("a runner with no wsl.exe reached the job: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(home, "jobs", testJobID, admittedName)); err != nil {
		t.Errorf("the job a runner reached is not marked running: %v", err)
	}
}

// ⛔ A configuration's cap and wait are read, and a wait that is no duration is
// refused where it is written.
func TestTheRunningCapIsReadFromAProjectsConfiguration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Jobs.MaxRunning, cfg.Jobs.MaxRunningWait = 2, "30m"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("a cap of 2 waiting 30m was refused: %v", err)
	}
	if d, err := cfg.Jobs.RunningWait(); err != nil || d != 30*time.Minute {
		t.Errorf("the wait reads %s, %v", d, err)
	}
	for _, bad := range []JobConfig{{MaxRunning: -1}, {MaxRunningWait: "soon"}, {MaxRunningWait: "-1m"}} {
		c := DefaultConfig()
		c.Jobs.MaxRunning, c.Jobs.MaxRunningWait = bad.MaxRunning, bad.MaxRunningWait
		if err := c.Validate(); err == nil {
			t.Errorf("jobs %+v was accepted", bad)
		}
	}
}

// ⛔ Each new jobs key survives loading: LoadConfig copies fields by name.
func TestTheNewJobsKeysSurviveLoading(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wsl-toolkit.json")
	body := `{"schema": "` + ConfigSchema + `", "jobs": {"container_lifecycle": "ephemeral", "max_running": 3, "max_running_wait": "10m"}}`
	if err := forksafe.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WSL_TOOLKIT_HOME", filepath.Join(dir, "state"))
	prev := ExplicitConfigPath
	ExplicitConfigPath = path
	t.Cleanup(func() { ExplicitConfigPath = prev })
	selectInstance(t, DefaultInstance)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Jobs.MaxRunning != 3 || cfg.Jobs.MaxRunningWait != "10m" {
		t.Errorf("loading kept jobs %+v", cfg.Jobs)
	}
}

// ⭐ A job let in at its first count waited not at all, and answers zero.
func TestAJobLetInAtOnceWaitedNotAtAll(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", root)
	t.Setenv(InstanceRootEnv, root)
	selectInstance(t, DefaultInstance)
	me, err := ClaimJob(root, testJobID)
	if err != nil {
		t.Fatal(err)
	}
	defer me.Release()
	waited, err := AdmitRunning(context.Background(), me, 1, time.Minute, nil, nil)
	if err != nil || waited != 0 {
		t.Errorf("a job with a free slot waited %s, %v; want 0", waited, err)
	}
}

// ⛔ A row a runner claims is held by the cap its fleet carries: refused, it
// ends unreached with the refusal; stopped while it waits, it ends with 130.
// jobs.max_running was read by run alone. WSL-136.
func TestARowTheRunnerClaimsIsHeldByTheCap(t *testing.T) {
	for _, c := range []struct {
		name    string
		err     error
		stopped bool
	}{
		{"refused", ErrOverRunningCap, false},
		{"stopped", ErrStoppedWaitingToRun, true},
	} {
		home := t.TempDir()
		r := &Runner{cfg: DefaultConfig(), wsl: &Wsl{Path: filepath.Join(home, "no-wsl.exe")}, home: home, log: func(string) {}}
		asked := false
		spec := MatrixSpec{
			ContainerLifecycle: ContainerEphemeral, Platform: "linux/amd64", RowIDs: []string{testJobID},
			Admit: func(_ context.Context, o *JobOwner) (time.Duration, error) {
				asked = o != nil && o.ID == testJobID
				return 3 * time.Second, c.err
			},
		}
		res := r.Run(context.Background(), rowAt(spec, 0, Image{ID: "alpine", Ref: "docker.io/library/alpine:latest"}, "", "", WorkspaceLimits{}))
		if !asked {
			t.Fatalf("%s: the row was not held by the cap", c.name)
		}
		if res.Stopped != c.stopped || res.Unreached == c.stopped || !strings.Contains(res.Error, c.err.Error()) {
			t.Errorf("%s: the row ended as %+v", c.name, res)
		}
		if res.WaitedToRun != 3*time.Second {
			t.Errorf("%s: the row says it waited %s", c.name, res.WaitedToRun)
		}
		st, err := ReadJobState(home, testJobID)
		if err != nil || st.Result == nil || st.Result.Error != res.Error {
			t.Errorf("%s: the row's end is recorded as %+v, %v", c.name, st.Result, err)
		}
		if _, err := os.Stat(filepath.Join(home, "jobs", testJobID, admittedName)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: a row the cap did not let in carries a running mark: %v", c.name, err)
		}
	}
}
