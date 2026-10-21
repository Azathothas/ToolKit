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

// ErrStoppedWaiting is a job that `wsl-toolkit stop` ended while it waited for
// its --exclusive name.
var ErrStoppedWaiting = errors.New("stopped by wsl-toolkit stop while it waited for its --exclusive name")

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
//
// ⛔ stopped IS ASKED AT EVERY POLL, and nil means nothing can stop the wait but
// its context. A `stop` sent to a waiting job was recorded and not read: the job
// waited out its whole --exclusive-wait, then ran or failed with exit 2, where
// `stop` promised 130.
func AcquireExclusive(ctx context.Context, name, jobID string, wait time.Duration, log func(string), stopped func() bool) (func(), error) {
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
			// ⚠ BEST EFFORT. The lock is the fact; the names beside it are only
			// what a refused job reads to say who it waited for, and where.
			_ = writeFileAtomic(holder, []byte(jobID+"\n"+SelectedInstance.Name+"\n"), 0o600)
			return release, nil
		}
		if !errors.Is(err, errLockHeld) {
			return nil, fmt.Errorf("--exclusive %s: %w", name, err)
		}
		by := readLockHolder(holder)
		if stopped != nil && stopped() {
			return nil, fmt.Errorf("%w: %s is held by %s", ErrStoppedWaiting, name, by)
		}
		if !time.Now().Before(deadline) {
			if wait > 0 {
				return nil, fmt.Errorf("%w: %s is held by %s, and it was not let go within %s", ErrExclusiveHeld, name, by, wait)
			}
			return nil, fmt.Errorf("%w: %s is held by %s. %s", ErrExclusiveHeld, name, by, by.advice())
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

// lockHolder is what a lock's holder file names: the job or the fleet that holds
// it, and the instance that one runs in.
//
// ⛔ THE INSTANCE IS RECORDED BESIDE THE ID. Every instance shares the lock, so a
// refused caller built the `wait` for the holder under its OWN instance, and a
// job held from another one answered that `wait` with "no such job". A
// correctness review found it. WSL-116.
type lockHolder struct{ id, instance string }

// readLockHolder reads a holder file. A file that is not there, or names
// nothing, reads as a holder that recorded no id.
func readLockHolder(path string) lockHolder {
	b, err := os.ReadFile(path)
	if err != nil {
		return lockHolder{}
	}
	lines := strings.SplitN(strings.TrimSpace(string(b)), "\n", 2)
	h := lockHolder{id: strings.TrimSpace(lines[0])}
	if len(lines) == 2 {
		h.instance = strings.TrimSpace(lines[1])
	}
	return h
}

// String is the holder as a refusal names it.
func (h lockHolder) String() string {
	switch {
	case h.id == "":
		return "a job that recorded no id"
	case !jobIDRE.MatchString(h.id):
		return h.id
	case h.instance != "" && h.instance != DefaultInstance:
		return "job " + h.id + " of instance " + h.instance
	}
	return "job " + h.id
}

var jobIDRE = regexp.MustCompile(`^[0-9a-f]{16}$`)

// advice is the step a refused caller takes: wait for the job that holds the
// name, in the instance that holds it, or pass --exclusive-wait.
func (h lockHolder) advice() string {
	if jobIDRE.MatchString(h.id) {
		return "Wait for it with: " + ToolCommandFor(h.instance, "wait "+h.id) + ", or pass --exclusive-wait to wait for it"
	}
	return "Pass --exclusive-wait to wait for it"
}

// ExclusiveRefusal is the refusal a job gets while another holds name, and nil
// while none does, for a caller that refuses before it starts anything. ⚠ It is
// a probe and not a claim: AcquireExclusive in the job's owner is what decides.
func ExclusiveRefusal(name string) error {
	root, err := StateRoot()
	if err != nil || ValidateExclusiveName(name) != nil {
		return nil
	}
	dir := filepath.Join(root, "locks")
	held, _, err := lockProbe(filepath.Join(dir, name+".lock"))
	if err != nil || !held {
		return nil
	}
	by := readLockHolder(filepath.Join(dir, name+".holder"))
	return fmt.Errorf("%w: %s is held by %s. %s", ErrExclusiveHeld, name, by, by.advice())
}
