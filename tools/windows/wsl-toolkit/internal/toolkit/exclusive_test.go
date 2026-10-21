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
	release, err := AcquireExclusive(context.Background(), "podssh-build", "0123456789abcdef", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	_, err = AcquireExclusive(context.Background(), "podssh-build", "fedcba9876543210", 0, nil)
	if !errors.Is(err, ErrExclusiveHeld) || !strings.Contains(err.Error(), "job 0123456789abcdef") || !strings.Contains(err.Error(), "wait 0123456789abcdef") {
		t.Fatalf("a second holder answered %v; want a refusal naming the first and the wait for it", err)
	}
	if held, by := ExclusiveHeld("podssh-build"); !held || by != "job 0123456789abcdef" {
		t.Fatalf("the probe answered held %v by %q", held, by)
	}
	other, err := AcquireExclusive(context.Background(), "another-name", "fedcba9876543210", 0, nil)
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
	release, err := AcquireExclusive(context.Background(), "build", "0123456789abcdef", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireExclusive(context.Background(), "build", "1111111111111111", 3*time.Second, nil); !errors.Is(err, ErrExclusiveHeld) || !strings.Contains(err.Error(), "not let go within 3s") {
		t.Fatalf("a wait that ran out answered %v", err)
	}
	let := make(chan struct{})
	go func() {
		<-let
		release()
	}()
	done := make(chan error, 1)
	go func() {
		r, err := AcquireExclusive(context.Background(), "build", "2222222222222222", time.Minute, nil)
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
	if ExclusiveAdvice("matrix (process 7)") != "Pass --exclusive-wait to wait for it" {
		t.Error("a holder that is not a job was offered a wait for a job")
	}
}
