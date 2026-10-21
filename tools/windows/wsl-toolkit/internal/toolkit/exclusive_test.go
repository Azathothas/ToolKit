// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// -- WSL-113: one job per name, machine-wide ---------------------------------

// ⭐ A second holder is refused naming the first, and a refused caller is told the
// one command that waits for it.
func TestASecondJobWithTheSameExclusiveNameIsRefusedNamingTheFirst(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	t.Setenv(InstanceRootEnv, "")
	release, err := AcquireExclusive(context.Background(), "podssh-build", "0123456789abcdef", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, err = AcquireExclusive(context.Background(), "podssh-build", "fedcba9876543210", 0, nil, nil)
	if !errors.Is(err, ErrExclusiveHeld) || !strings.Contains(err.Error(), "job 0123456789abcdef") || !strings.Contains(err.Error(), "wait 0123456789abcdef") {
		t.Fatalf("a second holder answered %v; want a refusal naming the first and the wait for it", err)
	}
	if err := ExclusiveRefusal("podssh-build"); !errors.Is(err, ErrExclusiveHeld) || !strings.Contains(err.Error(), "job 0123456789abcdef") {
		t.Fatalf("the probe answered %v", err)
	}
	if err := ExclusiveRefusal("another-name"); err != nil {
		t.Fatalf("the probe refused a name nobody holds: %v", err)
	}
	other, err := AcquireExclusive(context.Background(), "another-name", "fedcba9876543210", 0, nil, nil)
	if err != nil {
		t.Fatalf("another name was refused: %v", err)
	}
	other()
}

// ⛔ A waiter takes the lock once it is let go, and gives up when its wait runs
// out, saying for whom it waited.
func TestAJobThatWaitsTakesTheNameWhenItIsLetGo(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	t.Setenv(InstanceRootEnv, "")
	release, err := AcquireExclusive(context.Background(), "build", "0123456789abcdef", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireExclusive(context.Background(), "build", "1111111111111111", 3*time.Second, nil, nil); !errors.Is(err, ErrExclusiveHeld) || !strings.Contains(err.Error(), "not let go within 3s") {
		t.Fatalf("a wait that ran out answered %v", err)
	}
	let := make(chan struct{})
	go func() {
		<-let
		release()
	}()
	done := make(chan error, 1)
	go func() {
		r, err := AcquireExclusive(context.Background(), "build", "2222222222222222", time.Minute, nil, nil)
		if r != nil {
			r()
		}
		done <- err
	}()
	close(let)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a waiter did not take the name once it was let go: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("a waiter was still waiting 30 seconds after the name was let go")
	}
}

// ⛔ A stop sent to a job that waits for its name ends the wait at the next poll,
// where the job waited out its whole --exclusive-wait. A correctness review
// found it.
func TestAStopEndsAJobWaitingForItsName(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	t.Setenv(InstanceRootEnv, "")
	release, err := AcquireExclusive(context.Background(), "build", "0123456789abcdef", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	started := time.Now()
	_, err = AcquireExclusive(context.Background(), "build", "1111111111111111", 6*time.Second, nil, func() bool { return true })
	if !errors.Is(err, ErrStoppedWaiting) || !strings.Contains(err.Error(), "job 0123456789abcdef") {
		t.Fatalf("a stopped waiter answered %v; want the stopped refusal naming the holder", err)
	}
	if waited := time.Since(started); waited > 4*time.Second {
		t.Fatalf("a stopped waiter took %s to notice", waited)
	}
}

// ⛔ Every instance shares the lock, so a holder in another instance is waited for
// in ITS instance: a `wait` built for the refused caller's own answered "no such
// job". A correctness review found it.
func TestAHolderInAnotherInstanceIsWaitedForThere(t *testing.T) {
	root := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", root)
	t.Setenv(InstanceRootEnv, root)
	previous := SelectedInstance
	t.Cleanup(func() { SelectedInstance = previous })
	SelectedInstance = Instance{Name: "podbox", Distro: "wsl-toolkit-podbox"}
	release, err := AcquireExclusive(context.Background(), "build", "0123456789abcdef", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	SelectedInstance = Instance{Name: DefaultInstance}
	_, err = AcquireExclusive(context.Background(), "build", "1111111111111111", 0, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "job 0123456789abcdef of instance podbox") ||
		!strings.Contains(err.Error(), "wsl-toolkit --instance podbox wait 0123456789abcdef") {
		t.Fatalf("a holder in another instance was named as %v", err)
	}
}

// ⭐ The owner that waits for its name reads the same stop record a running job
// reads, with no runner of its own yet.
func TestAStopIsReadWithoutARunner(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	l, err := OpenLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(LedgerEntry{Event: "stop", Kind: "job", ID: "0123456789abcdef"}); err != nil {
		t.Fatal(err)
	}
	if !StopRecorded("0123456789abcdef") {
		t.Error("a recorded stop was not read")
	}
	if StopRecorded("fedcba9876543210") {
		t.Error("another job's stop was read as this one's")
	}
}

func TestAnExclusiveNameMustBeAFileName(t *testing.T) {
	for _, bad := range []string{"", "../x", "a b", "con:x", "-lead", strings.Repeat("a", 65)} {
		if ValidateExclusiveName(bad) == nil {
			t.Errorf("%q was accepted as a name", bad)
		}
	}
	for _, good := range []string{"podssh", "podssh-build.linux_x86-64", "B1"} {
		if err := ValidateExclusiveName(good); err != nil {
			t.Errorf("%q was refused: %v", good, err)
		}
	}
	if (lockHolder{id: "matrix (process 7)"}).advice() != "Pass --exclusive-wait to wait for it" {
		t.Error("a holder that is not a job was offered a wait for a job")
	}
}
