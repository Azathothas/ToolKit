// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ⭐ AN INSTANCE IS A NAME AND A STATE DIRECTORY TOGETHER, and that pairing is
// the whole feature. WSL-43.
//
// Two agents on one host used to share the distribution, the engine, the ledger
// and the helper endpoint, or one of them stopped using the tool. Isolation was
// ALREADY possible - `--home` moves the state directory and a stored `base.name`
// moves the distribution - and that is exactly why it was filed as a defect
// rather than as a feature: it worked because `AssertOwnedDistro` compared a
// configured name against itself, so the isolation was real and the guard under
// it was not.
//
// ⛔ IT IS NOT A SCHEDULER. No shared lock, no pool, no allocation service. Two
// agents that both ask for the same instance name get the same base, and that is
// correct.

// InstanceEnv is the environment variable that selects one.
const InstanceEnv = "WSL_TOOLKIT_INSTANCE"

// SelectedInstance is the one this process resolved, empty for the default.
//
// ⛔ IT IS WHAT MAKES THE DISTRIBUTION FOLLOW THE STATE DIRECTORY. The
// configuration's default base name reads it, so `--instance two` moves both
// halves or neither. Set once, by the command layer, before anything loads a
// configuration; a value set later would be a selection something has already
// read the wrong answer for.
var SelectedInstance Instance

// InstanceAuto is the name that means "any free one, and record which".
//
// ⚠ IT IS A SPELLING, NOT AN OMISSION. `--instance` takes a value, so "no
// name" cannot be expressed by passing the flag with nothing after it; a flag
// with no value would swallow the next argument. An agent that does not care
// passes `auto` and never has to think about it again, because the choice is
// written into the state directory it gets back.
const InstanceAuto = "auto"

// DefaultInstance is what a caller naming nothing gets.
//
// ⛔ THE EMPTY NAME, so the distribution is the bare `wsl-toolkit` and the state
// directory is the bare one. Every existing caller, every manual line and every
// acceptance case keeps working unchanged, which is the compatibility promise
// this entry makes.
const DefaultInstance = ""

// maxAutoInstances bounds the search for a free `wsl-toolkit-<N>`.
//
// ⚠ A ceiling, and a deliberate one: a machine with 64 registered instances is
// a machine where something is looping, and answering "no free instance" is a
// better outcome than the 65th distribution.
const maxAutoInstances = 64

// Instance is the resolved selection.
type Instance struct {
	// Name is the suffix, empty for the default.
	Name string `json:"name"`
	// Distro is the distribution this instance's base is registered as.
	Distro string `json:"distro"`
	// Home is the state directory: transcripts, the ledger, the helper
	// endpoint, the base disk.
	Home string `json:"home"`
}

// InstanceDistro is the distribution name for an instance suffix.
func InstanceDistro(name string) string {
	if name == DefaultInstance {
		return DefaultBaseName
	}
	return DefaultBaseName + "-" + name
}

// ValidateInstanceName refuses a name that could not be both a distribution
// suffix and a directory component.
func ValidateInstanceName(name string) error {
	if name == DefaultInstance {
		return nil
	}
	if !isInstanceName(name) {
		return fmt.Errorf("instance %q is not usable: it must be 1 to 32 characters of lower-case letters, "+
			"digits, dash or underscore, because it is both a distribution suffix and a directory name", name)
	}
	return nil
}

// ResolveInstance turns what a caller asked for into a name, a distribution and
// a state directory.
//
// ⛔ ONE FUNCTION, so the two halves cannot drift apart. The defect this
// prevents is a session where the distribution moved and the state directory did
// not, which is two agents sharing one ledger while believing they are isolated.
func ResolveInstance(ctx context.Context, asked string) (Instance, error) {
	var inst Instance
	if asked == "" {
		asked = strings.TrimSpace(os.Getenv(InstanceEnv))
	}
	asked = strings.TrimSpace(asked)

	if asked == InstanceAuto {
		name, err := allocateInstance(ctx)
		if err != nil {
			return inst, err
		}
		asked = name
	}
	if err := ValidateInstanceName(asked); err != nil {
		return inst, err
	}
	inst.Name = asked
	inst.Distro = InstanceDistro(asked)
	home, err := instanceHome(asked)
	if err != nil {
		return inst, err
	}
	inst.Home = home
	return inst, nil
}

// instanceHome is where an instance's state lives.
//
// ⛔ --home SETS THE ROOT AND THE INSTANCE STILL GETS ITS OWN DIRECTORY UNDER
// IT. The first version of this let an explicit --home win outright, on the
// reasoning that a caller who named a directory named it on purpose. That
// reasoning is wrong in the one case this whole entry is about:
// `--home X --instance one` and `--home X --instance two` would have been two
// distributions sharing ONE ledger, one helper endpoint and one transcript
// directory, while each caller believed it was isolated. A test written for the
// pairing is what caught it, before any of it shipped.
//
// ⛔ UNDER the state home, not beside it. `gc`, `resources` and every report
// walk one root, and an instance outside it would be state nothing could find.
// WSL-51 ruled the same way for a working directory's config: the state home is
// the single store.
func instanceHome(name string) (string, error) {
	base, err := Home()
	if err != nil {
		return "", err
	}
	if name == DefaultInstance {
		return base, nil
	}
	return filepath.Join(base, "instances", name), nil
}

// allocateInstance picks the lowest free `wsl-toolkit-<N>`.
//
// ⛔ FREE MEANS BOTH: no distribution registered under the name AND no state
// directory holding one. Either alone would hand an agent an instance whose
// other half already belongs to somebody.
func allocateInstance(ctx context.Context) (string, error) {
	w, err := FindWsl()
	if err != nil {
		return "", fmt.Errorf("cannot allocate an instance without WSL: %w", err)
	}
	registered := map[string]bool{}
	names, err := w.listNames(ctx)
	if err != nil {
		return "", fmt.Errorf("cannot allocate an instance without asking WSL what exists: %w", err)
	}
	for _, n := range names {
		registered[strings.ToLower(strings.TrimSpace(n))] = true
	}
	for i := 1; i <= maxAutoInstances; i++ {
		name := strconv.Itoa(i)
		if registered[strings.ToLower(InstanceDistro(name))] {
			continue
		}
		home, err := instanceHome(name)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(home); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("every instance from 1 to %d is taken, which is a machine where something is looping. "+
		"wsl-toolkit resources says what is held; wsl-toolkit gc --apply releases it", maxAutoInstances)
}

// -- the pointer directory ---------------------------------------------------

// PointerDir is the directory a working tree may carry to say which instance it
// belongs to.
//
// ⛔ IT IS A POINTER AND NOT A STORE, and that ruling is what dissolved the name
// collision WSL-51 raised. `GuestRoot` is already `.wsl-toolkit`, inside the
// guest, and a host directory of the same name holding an instance's transcripts
// and ledger would be one name for two unrelated things in a tool whose whole
// difficulty is which side of the boundary something is on. This holds a file
// naming an instance; the state itself stays under one home per instance, so a
// checkout deleted mid-job loses a pointer rather than a running job's ledger,
// and `gc` keeps one place to look.
const PointerDir = ".wsl-toolkit"

// PointerFile is the file inside it.
const PointerFile = "instance.json"

// PointerSchema versions that file.
const PointerSchema = "wsl-toolkit-pointer/1"

// Pointer is what a working tree stores.
type Pointer struct {
	Schema   string `json:"schema"`
	Instance string `json:"instance"`
}

// ReadPointer looks for a pointer in a directory or the nearest parent that has
// one, and returns the instance it names.
func ReadPointer(start string) (string, string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", "", err
	}
	for {
		path := filepath.Join(dir, PointerDir, PointerFile)
		data, err := os.ReadFile(path)
		if err == nil {
			var p Pointer
			if err := json.Unmarshal(data, &p); err != nil {
				return "", path, fmt.Errorf("%s does not parse: %w", path, err)
			}
			if p.Schema != PointerSchema {
				return "", path, fmt.Errorf("%s declares schema %q and this build reads %q", path, p.Schema, PointerSchema)
			}
			if err := ValidateInstanceName(p.Instance); err != nil {
				return "", path, fmt.Errorf("%s: %w", path, err)
			}
			return p.Instance, path, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", path, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", nil
		}
		dir = parent
	}
}

// WritePointer records an instance in a working tree.
func WritePointer(dir, instance string) (string, error) {
	if err := ValidateInstanceName(instance); err != nil {
		return "", err
	}
	target := filepath.Join(dir, PointerDir)
	if err := os.MkdirAll(target, 0o700); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(Pointer{Schema: PointerSchema, Instance: instance}, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(target, PointerFile)
	return path, writeFileAtomic(path, append(data, '\n'), 0o600)
}
