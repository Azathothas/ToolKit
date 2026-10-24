// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// One process at a time changes an instance's base. WSL-172.
//
// ⛔ NOTHING SERIALIZED IT. Two `base ensure` or `run --ensure-base` calls on an
// unbuilt instance both built it, and the loser rolled back by unregistering the
// winner's base; two grants each rewrote the configuration from what they read,
// and one was lost. `base ensure`, `base recreate`, `base remove`, `base grant` and
// `base revoke` now hold one lock in the instance's state directory, and a second
// caller waits for it, naming the process it waits for.

const (
	baseLockName   = "base.lock"
	baseHolderName = "base.holder"
)

// baseLockPoll is how often a waiting caller asks again, and baseLockSay how
// often it says it is still waiting.
var baseLockPoll = time.Second

const baseLockSay = 30 * time.Second

// LockBase holds the lock of the selected instance's base for a command that
// changes it outside a Base, which is `base grant` and `base revoke`: the
// configuration is read and written inside it.
func LockBase(ctx context.Context, what string, log func(string)) (func(), error) {
	home, err := EnsureHome()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = func(string) {}
	}
	return lockBaseAt(ctx, home, what, log)
}

// lock holds the base's lock for what this process is about to do to it.
func (b *Base) lock(ctx context.Context, what string) (func(), error) {
	if err := os.MkdirAll(b.home, 0o700); err != nil {
		return nil, err
	}
	return lockBaseAt(ctx, b.home, what, b.log)
}

// lockBaseAt waits for the lock in home until it is free or ctx ends.
//
// ⭐ IT WAITS, IT DOES NOT REFUSE. Two jobs that each ensure a healthy base ran
// side by side before, and refusing the second would fail a call that worked.
// Windows lets the lock go when its process ends however it ends, so a killed
// holder leaves nothing to wait for.
func lockBaseAt(ctx context.Context, home, what string, log func(string)) (func(), error) {
	lock := filepath.Join(home, baseLockName)
	holder := filepath.Join(home, baseHolderName)
	var lastSaid time.Time
	for {
		release, err := lockExclusive(lock)
		if err == nil {
			// ⚠ BEST EFFORT. The lock is the fact; the holder file is only what a
			// waiting caller reads to say whom it waits for.
			_ = writeFileAtomic(holder, []byte(fmt.Sprintf("%d\n%s\n%s\n",
				os.Getpid(), what, time.Now().UTC().Format(time.RFC3339))), 0o600)
			return func() {
				_ = os.Remove(holder)
				release()
			}, nil
		}
		if !errors.Is(err, errLockHeld) {
			return nil, fmt.Errorf("the base's lock %s: %w", lock, err)
		}
		by := readBaseHolder(holder)
		if lastSaid.IsZero() || time.Since(lastSaid) >= baseLockSay {
			log("waiting: the base is being changed by " + by)
			lastSaid = time.Now()
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%s did not start: the base is being changed by %s, and the wait ended: %w", what, by, ctx.Err())
		case <-time.After(baseLockPoll):
		}
	}
}

// readBaseHolder names the process a holder file records.
func readBaseHolder(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "another process, which recorded nothing"
	}
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n")), "\n")
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || len(lines) < 3 {
		return "another process, which recorded nothing"
	}
	return fmt.Sprintf("process %d running `%s` since %s", pid, strings.TrimSpace(lines[1]), strings.TrimSpace(lines[2]))
}
