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
// ⛔ THE EMPTY NAME, so the state directory is the bare one. Its distribution is
// `wsl-toolkit-jobs` on a fresh host, and an existing base keeps the name it was
// built under, `wsl-toolkit` included: defaultbase.go is that rule. WSL-167.
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
	// StrippedFrom is the name the caller gave when a prefix was dropped from
	// it, empty otherwise. WSL-193.
	StrippedFrom string `json:"-"`
	// Fresh says the state directory did not exist when the name was resolved.
	Fresh bool `json:"-"`
}

// InstanceDistro is the distribution name for an instance suffix.
//
// ⚠ FOR THE DEFAULT IT IS THE CANONICAL NAME, the one a NEW default base takes.
// An existing default base may run under `wsl-toolkit`, and the configuration
// says which. WSL-167.
func InstanceDistro(name string) string {
	if name == DefaultInstance {
		return DefaultBaseName
	}
	return OwnedPrefix + "-" + name
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
	stripped, err := stripInstancePrefix(asked)
	if err != nil {
		return inst, err
	}
	if stripped != asked {
		inst.StrippedFrom = asked
		asked = stripped
	}
	if err := ValidateInstanceName(asked); err != nil {
		if inst.StrippedFrom != "" {
			return inst, fmt.Errorf("instance %q becomes %q without its prefix, and %w. %s keeps the name as given",
				inst.StrippedFrom, asked, err, LiteralInstanceFlag)
		}
		return inst, err
	}
	home, err := instanceHome(asked)
	if err != nil {
		return inst, err
	}
	if _, statErr := os.Stat(home); statErr != nil {
		inst.Fresh = true
	}
	if inst.StrippedFrom != "" && IsReservedInstance(asked) {
		return inst, fmt.Errorf("instance %q becomes %q without its prefix, and %q is reserved: its distribution "+
			"would be %s, which reads as the default instance's base. Name the instance for what it holds, "+
			"or pass %s to keep %q as given", inst.StrippedFrom, asked, asked, InstanceDistro(asked),
			LiteralInstanceFlag, inst.StrippedFrom)
	}
	// ⛔ A RESERVED NAME IS REFUSED FOR A NEW INSTANCE AND NOT FOR AN EXISTING ONE.
	// An instance made before the reservation has to stay reachable, or nothing
	// could remove it. Its state directory is the proof it exists. WSL-167.
	if IsReservedInstance(asked) {
		if _, statErr := os.Stat(home); statErr != nil {
			return inst, reservedRefusal(asked)
		}
	}
	inst.Name = asked
	inst.Distro = InstanceDistro(asked)
	inst.Home = home
	return inst, nil
}

// -- the prefix a caller may type -------------------------------------------

// InstanceLiteralEnv, set to 1, keeps an instance's name as given. WSL-193.
const InstanceLiteralEnv = "WSL_TOOLKIT_INSTANCE_LITERAL"

// LiteralInstanceFlag is the global option that does the same.
const LiteralInstanceFlag = "--literal-instance"

// instancePrefixes are what a caller types when naming an instance after the
// distribution it wants. The longer one is tried first, so `wsl-toolkit-x`
// becomes `x` and never `toolkit-x`.
var instancePrefixes = []string{OwnedPrefix + "-", "wsl-"}

// stripInstancePrefix is the name a NEW instance takes when the caller wrote a
// distribution's prefix into it.
//
// ⭐ `--instance wsl-errand-box` MAKES `errand-box`, distribution
// `wsl-toolkit-errand-box`. The caller named the distribution they wanted, and
// the literal name would register `wsl-toolkit-wsl-errand-box`, which nobody
// asked for. The operator ruled to strip and warn. WSL-193.
//
// ⛔ AN INSTANCE THAT ALREADY EXISTS UNDER THE PREFIXED NAME KEEPS IT. Its state
// directory is the proof, as it is for a reserved name, and an instance this
// stopped reaching would be one nothing could remove.
//
// ⚠ THE REMAINDER MAY NOT CARRY A PREFIX OF ITS OWN. Stripping once more would
// surprise a caller, and stripping only once leaves a name a child process
// would strip again. Refusing keeps one resolution for one name.
func stripInstancePrefix(asked string) (string, error) {
	if asked == DefaultInstance || os.Getenv(InstanceLiteralEnv) == "1" {
		return asked, nil
	}
	if ValidateInstanceName(asked) == nil {
		if home, err := instanceHome(asked); err == nil {
			if _, statErr := os.Stat(home); statErr == nil {
				return asked, nil
			}
		}
	}
	if asked == OwnedPrefix {
		return "", barePrefixRefusal(asked)
	}
	for _, prefix := range instancePrefixes {
		rest, found := strings.CutPrefix(asked, prefix)
		if !found {
			continue
		}
		if rest == "" {
			return "", barePrefixRefusal(asked)
		}
		if strings.HasPrefix(rest, "wsl-") {
			return "", fmt.Errorf("instance %q still starts with a prefix after %q is dropped from it, "+
				"so its distribution would still double the prefix. Name it without one, "+
				"or pass %s to keep it as given", asked, prefix, LiteralInstanceFlag)
		}
		return rest, nil
	}
	return asked, nil
}

func barePrefixRefusal(asked string) error {
	return fmt.Errorf("instance %q is only a prefix and names nothing: its distribution would be %s. "+
		"Name the instance for what it holds, as --instance errand-box does for %s",
		asked, OwnedPrefix+"-"+asked, InstanceDistro("errand-box"))
}

// PrefixWarning is what a caller is told when a prefix was dropped from the
// name of an instance this call creates, and empty otherwise.
//
// ⚠ IT IS SAID ONCE, WHEN THE INSTANCE IS NEW. Later calls with the same
// prefixed name reach the same instance, and repeating the warning on each of
// them would be noise about a choice already made.
func (i Instance) PrefixWarning() string {
	if i.StrippedFrom == "" || !i.Fresh {
		return ""
	}
	return fmt.Sprintf("⚠ instance %q is made as %q, distribution %s, rather than %s. "+
		"Pass %s or set %s=1 to keep %q as given",
		i.StrippedFrom, i.Name, i.Distro, OwnedPrefix+"-"+i.StrippedFrom,
		LiteralInstanceFlag, InstanceLiteralEnv, i.StrippedFrom)
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
	return claimInstance(registered)
}

// claimInstance claims the lowest `<N>` with no distribution registered under
// its name, by creating its state directory.
//
// ⛔ THE DIRECTORY IS THE CLAIM. A free name was only looked at, so two
// `--instance auto` callers at once both chose the same one and shared one base.
// os.Mkdir lets exactly one of them have it, as a throwaway's claim does.
// WSL-172.
func claimInstance(registered map[string]bool) (string, error) {
	for i := 1; i <= maxAutoInstances; i++ {
		name := strconv.Itoa(i)
		if registered[strings.ToLower(InstanceDistro(name))] {
			continue
		}
		home, err := instanceHome(name)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(home), 0o700); err != nil {
			return "", err
		}
		if err := os.Mkdir(home, 0o700); err == nil {
			return name, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", err
		}
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
