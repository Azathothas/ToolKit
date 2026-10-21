// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// InstanceRootEnv names the state root a process resolved an instance from, for
// the processes it starts. The default instance's home is the root itself.
const InstanceRootEnv = "WSL_TOOLKIT_INSTANCE_ROOT"

// StateRoot is the state directory every instance lives under.
func StateRoot() (string, error) {
	if v := strings.TrimSpace(os.Getenv(InstanceRootEnv)); v != "" {
		return v, nil
	}
	return Home()
}

// ErrExclusiveHeld is a job refused because another job holds its --exclusive
// name.
var ErrExclusiveHeld = errors.New("another job holds this --exclusive name")

var exclusiveNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidateExclusiveName refuses a name that cannot be a file name on every host.
func ValidateExclusiveName(name string) error {
	if !exclusiveNameRE.MatchString(name) {
		return fmt.Errorf("--exclusive %q is not a name: 1 to 64 letters, digits, dots, dashes or underscores, starting with a letter or a digit", name)
	}
	return nil
}

// exclusivePoll is how often a waiting job asks again, and exclusiveSay how
// often it says it is still waiting.
const (
	exclusivePoll = 2 * time.Second
	exclusiveSay  = 30 * time.Second
)

// AcquireExclusive holds a machine-wide lock named name for as long as the
// returned release has not run and this process lives, and records the job that
// holds it so a refused one can name it. WSL-113.
//
// ⭐ WINDOWS LETS THE LOCK GO WHEN THE PROCESS ENDS, however it ends, which is
// the owner lock's rule. So a killed job leaves nothing to remove, where a lock
// directory a consumer made itself stayed until somebody deleted it.
//
// ⛔ MACHINE-WIDE, NOT PER INSTANCE. Every instance's base runs in the one WSL
// VM and spends the one machine's memory, which is what three builds at once
// nearly exhausted, so the lock lives under the state root every instance shares.
//
// ⚠ wait 0 refuses at once. A positive wait polls, says every 30 seconds that it
// is still waiting and for whom, and refuses when it runs out.
func AcquireExclusive(ctx context.Context, name, jobID string, wait time.Duration, log func(string)) (func(), error) {
	if err := ValidateExclusiveName(name); err != nil {
		return nil, err
	}
	root, err := StateRoot()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	lock := filepath.Join(dir, name+".lock")
	holder := filepath.Join(dir, name+".holder")
	deadline := time.Now().Add(wait)
	lastSaid := time.Now()
	for {
		release, err := lockExclusive(lock)
		if err == nil {
			// ⚠ BEST EFFORT. The lock is the fact; the name beside it is only
			// what a refused job reads to say who it waited for.
			_ = writeFileAtomic(holder, []byte(jobID+"\n"), 0o600)
			return release, nil
		}
		if !errors.Is(err, errLockHeld) {
			return nil, fmt.Errorf("--exclusive %s: %w", name, err)
		}
		by := exclusiveHolder(holder)
		if !time.Now().Before(deadline) {
			if wait > 0 {
				return nil, fmt.Errorf("%w: %s is held by %s, and it was not let go within %s", ErrExclusiveHeld, name, by, wait)
			}
			return nil, fmt.Errorf("%w: %s is held by %s. %s", ErrExclusiveHeld, name, by, ExclusiveAdvice(by))
		}
		if log != nil && time.Since(lastSaid) >= exclusiveSay {
			log(fmt.Sprintf("still waiting for --exclusive %s, held by %s", name, by))
			lastSaid = time.Now()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(exclusivePoll):
		}
	}
}

// exclusiveHolder is what a lock names as its holder: "job ID", a fleet, or a
// phrase saying none is recorded.
func exclusiveHolder(path string) string {
	b, err := os.ReadFile(path)
	id := strings.TrimSpace(string(b))
	switch {
	case err != nil || id == "":
		return "a job that recorded no id"
	case jobIDRE.MatchString(id):
		return "job " + id
	}
	return id
}

var jobIDRE = regexp.MustCompile(`^[0-9a-f]{16}$`)

// ExclusiveAdvice is the step a refused caller takes: wait for the job that
// holds the name where it is one, or pass --exclusive-wait.
func ExclusiveAdvice(holder string) string {
	if id, ok := strings.CutPrefix(holder, "job "); ok && jobIDRE.MatchString(id) {
		return "Wait for it with: " + ToolCommand("wait "+id) + ", or pass --exclusive-wait to wait for it"
	}
	return "Pass --exclusive-wait to wait for it"
}

// ExclusiveHeld answers whether a job holds name now, and which, for a caller
// that refuses before it starts anything. ⚠ It is a probe and not a claim:
// AcquireExclusive in the job's owner is what decides.
func ExclusiveHeld(name string) (bool, string) {
	root, err := StateRoot()
	if err != nil || ValidateExclusiveName(name) != nil {
		return false, ""
	}
	dir := filepath.Join(root, "locks")
	held, _, err := lockProbe(filepath.Join(dir, name+".lock"))
	if err != nil || !held {
		return false, ""
	}
	return true, exclusiveHolder(filepath.Join(dir, name+".holder"))
}
