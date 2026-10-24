// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ⛔ THE DEFAULT INSTANCE'S BASE HAS TWO NAMES, AND THIS FILE DECIDES WHICH ONE A
// STATE DIRECTORY MEANS. WSL-167, rulings 32 and 33.
//
// The default base was `wsl-toolkit`, and the instance `base` was
// `wsl-toolkit-base`. Each read like the other's, and agents acted on one while
// meaning the other. A NEW default base is now `wsl-toolkit-jobs`. An EXISTING one
// keeps the name it was built under, because renaming a distribution is a copy of
// its whole disk and the operator decides when to pay for that.
//
// The order, and why each step is where it is:
//
//	1  base.name in the configuration in effect     the operator's own choice
//	2  both names with their disk in this
//	   directory's base folder                       a refusal, never a guess
//	3  the state directory's base.json, its name
//	   registered with its disk here                 the record of what this directory built
//	4  a registered `wsl-toolkit` with its disk
//	   here, and the marker                          a base built before its record existed
//	5  `wsl-toolkit-jobs`, unless another
//	   directory's disk holds it                     a fresh host
//
// ⭐ EVERY STEP TIES A NAME TO THIS STATE DIRECTORY BY ITS DISK. A distribution
// name is global to the host and a state directory is not. A scratch
// WSL_TOOLKIT_HOME must not adopt the host's base under either name, and a base in
// a scratch directory must not make the operator's own directory ambiguous. A name
// registered with its disk elsewhere is refused by name, and never acted on.
//
// ⚠ NOTHING HERE STARTS WSL ON THE COMMON PATH. Registrations are read from the
// registry, and the guest's marker is asked for only in step 4, where a registered
// `wsl-toolkit` has no record to vouch for it. Only a positive absence of the
// marker demotes it there.

// DefaultNameFrom says which step chose the default instance's distribution.
const (
	NameFromConfig     = "configuration"
	NameFromRecord     = "record"
	NameFromRegistered = "registered"
	NameFromCanonical  = "canonical"
)

// ErrDefaultBaseAmbiguous is what a state directory with two candidate bases is
// refused with.
var ErrDefaultBaseAmbiguous = errors.New("the default base is ambiguous")

// defaultBaseHost is what the resolution asks of this host. ⚠ A variable so a
// case can answer for a host it does not have; production reads the registry
// and the guest.
var defaultBaseHost = struct {
	disks  func() (map[string]string, error)
	marked func(ctx context.Context, name string) error
}{
	disks: registeredDisks,
	marked: func(ctx context.Context, name string) error {
		w, err := FindWsl()
		if err != nil {
			return err
		}
		_, err = w.ReadIdentity(ctx, name)
		return err
	},
}

// IsDefaultBaseName reports whether a distribution name is one the default
// instance's base may run under: the canonical one or the older one.
func IsDefaultBaseName(name string) bool {
	return strings.EqualFold(name, DefaultBaseName) || strings.EqualFold(name, LegacyBaseName)
}

// isDefaultSelection reports whether this process acts for the default instance.
func isDefaultSelection() bool {
	return SelectedInstance.Name == DefaultInstance && SelectedInstance.Distro == ""
}

// resolveDefaultDistro is steps 2 to 5 for the state directory home: the name the
// default instance's base runs under, and which step chose it.
func resolveDefaultDistro(home string) (string, string, error) {
	recorded := ""
	if rec, err := (&Base{home: home}).readRecord(); err == nil {
		switch {
		case strings.EqualFold(rec.Name, LegacyBaseName):
			recorded = LegacyBaseName
		case strings.EqualFold(rec.Name, DefaultBaseName):
			recorded = DefaultBaseName
		}
	}
	name, from, err := pickDefaultDistro(home, recorded)
	if err != nil {
		return "", "", err
	}
	if name == DefaultBaseName {
		if err := assertNoJobsInstance(home); err != nil {
			return "", "", err
		}
	}
	return name, from, nil
}

func pickDefaultDistro(home, recorded string) (string, string, error) {
	disks, err := defaultBaseHost.disks()
	if err != nil {
		// ⚠ A HOST WITH NO REGISTRY TO READ HAS NO SECOND CANDIDATE TO FIND. The
		// record still names what this directory built.
		if recorded != "" {
			return recorded, NameFromRecord, nil
		}
		return DefaultBaseName, NameFromCanonical, nil
	}
	ownDir := filepath.Join(home, "base")
	// where says whether a name is registered, and whether its disk is in this
	// state directory's base folder.
	where := func(name string) (at string, registered, here bool) {
		at, registered = registeredAt(disks, name)
		return at, registered, registered && pathEqual(cleanBasePath(at), cleanBasePath(ownDir))
	}
	_, _, legacyHere := where(LegacyBaseName)
	canonicalAt, canonicalRegistered, canonicalHere := where(DefaultBaseName)

	// ⛔ STEP 2: TWO CANDIDATES FOR THIS DIRECTORY, REFUSED BY NAME. A candidate is
	// a name whose disk is in this directory's base folder. A base whose disk is in
	// another state directory belongs to that directory and is no candidate here.
	if legacyHere && canonicalHere {
		return "", "", fmt.Errorf("%w: %s, the older name, and %s, the canonical name, both have their disk in %s. "+
			"Nothing runs, because acting on either would be a guess. "+
			"Choose one by setting base.name to it in the configuration",
			ErrDefaultBaseAmbiguous, LegacyBaseName, DefaultBaseName, ownDir)
	}

	// ⭐ STEP 3: THE RECORD, with no marker read. Where the recorded name is
	// registered with its disk here, it is this directory's base, and a marker
	// read that failed for any reason must not overrule it.
	if recorded != "" {
		at, registered, here := where(recorded)
		if registered && here {
			return recorded, NameFromRecord, nil
		}
		if registered {
			// ⛔ THE NAME IS GLOBAL TO THE HOST AND THE RECORD IS NOT. A record
			// written while this directory acted on another directory's base would
			// otherwise adopt that base.
			return "", "", fmt.Errorf("%w: this state directory's record names %s, and %s is registered with its disk in %s, "+
				"not in %s. That base belongs to another state directory. Nothing runs. "+
				"Point WSL_TOOLKIT_HOME at the directory that built it, or delete %s here",
				ErrDefaultBaseAmbiguous, recorded, recorded, cleanBasePath(at), ownDir, filepath.Join(home, "base.json"))
		}
		// An unregistered record is a base that was removed: the next one is new.
	}

	// ⭐ STEP 4: A `wsl-toolkit` BUILT HERE BEFORE ITS RECORD. Only a positive
	// absence of the marker demotes it. A read that could not ask is refused,
	// because demoting on it would pick the other name on a guess.
	if legacyHere {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		markErr := defaultBaseHost.marked(ctx, LegacyBaseName)
		switch {
		case markErr == nil:
			return LegacyBaseName, NameFromRegistered, nil
		case !errors.Is(markErr, ErrNoIdentity):
			return "", "", fmt.Errorf("%s is registered with its disk in %s and its marker could not be read, "+
				"so which name this base runs under is not known: %w", LegacyBaseName, ownDir, markErr)
		}
	}

	// ⛔ STEP 5: THE CANONICAL NAME, unless another state directory holds it. A
	// scratch WSL_TOOLKIT_HOME would otherwise act on the host's own base.
	if canonicalRegistered && !canonicalHere {
		return "", "", fmt.Errorf("%w: %s is registered with its disk in %s, which is another state directory's base, "+
			"and this one, %s, has none. Nothing runs. "+
			"Point WSL_TOOLKIT_HOME at that directory, or select an instance with --instance",
			ErrDefaultBaseAmbiguous, DefaultBaseName, cleanBasePath(canonicalAt), home)
	}
	return DefaultBaseName, NameFromCanonical, nil
}

// registeredAt finds a distribution in a registry answer, as WSL compares names.
func registeredAt(disks map[string]string, name string) (string, bool) {
	for n, at := range disks {
		if strings.EqualFold(n, name) {
			return at, true
		}
	}
	return "", false
}

// assertNoJobsInstance refuses the canonical name while an instance called `jobs`
// already has state under home.
//
// ⛔ THAT INSTANCE'S DISTRIBUTION IS `wsl-toolkit-jobs` TOO. Before the rename it
// was an ordinary instance name, so a host may hold one, and the default would then
// take over its base.
func assertNoJobsInstance(home string) error {
	dir, err := instanceHomeUnder(home, "jobs")
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("the default base would be %s, and the instance jobs at %s holds that name already. "+
			"Remove that instance with %s, or set base.name to %s to keep the default on the older name",
			DefaultBaseName, dir, "wsl-toolkit --instance jobs base remove --yes", LegacyBaseName)
	}
	return nil
}

// BaseNaming says which name the default instance's base runs under, and how to
// move to the canonical one. WSL-167.
type BaseNaming struct {
	// Canonical is the name a new default base is registered as.
	Canonical string `json:"canonical"`
	// From is which step chose the name: configuration, record, registered or
	// canonical.
	From string `json:"from"`
	// Legacy is true where the base runs under the older name.
	Legacy bool `json:"legacy"`
	// Move is how to rebuild under the canonical name, and what that costs.
	Move string `json:"move,omitempty"`
}

// moveAdvice is what `base status` says to a base under the older name.
//
// ⚠ NOT A RENAME, AND NOT OFFERED AS ONE. A copy by `wsl --export` and
// `--import` carries the marker of the old name, which every guard then reads as
// a copied disk, and it doubles the disk while it runs.
func moveAdvice(from string) string {
	steps := ToolCommand("base remove --yes") + ", then " + ToolCommand("base ensure")
	if from == NameFromConfig {
		steps = "remove base.name from the configuration, then " + steps
	}
	return fmt.Sprintf("this base runs under the older name %s, and a new one is named %s. "+
		"To move: %s. That deletes this base's disk, with its home, images and credentials, "+
		"and builds a new one from the image. The grants in the configuration are kept. "+
		"Nothing moves until you run it", LegacyBaseName, DefaultBaseName, steps)
}

// naming is the BaseNaming for a configuration, or nil for a named instance.
func (c Config) naming() *BaseNaming {
	if !isDefaultSelection() || !IsDefaultBaseName(c.Base.Name) {
		return nil
	}
	from := c.nameFrom
	if from == "" {
		from = NameFromCanonical
	}
	n := &BaseNaming{Canonical: DefaultBaseName, From: from}
	if strings.EqualFold(c.Base.Name, LegacyBaseName) {
		n.Legacy = true
		n.Move = moveAdvice(from)
	}
	return n
}

// -- reserved instance names -------------------------------------------------

// ReservedInstances are names no new instance may take.
//
// ⛔ EACH ONE READS AS THE DEFAULT'S BASE. `base` was the instance whose
// distribution `wsl-toolkit-base` was mistaken for the default's, and `jobs` would
// be registered as `wsl-toolkit-jobs`, the default's own canonical name.
var ReservedInstances = []string{"base", "jobs"}

// IsReservedInstance reports whether name is one of them.
func IsReservedInstance(name string) bool {
	for _, r := range ReservedInstances {
		if name == r {
			return true
		}
	}
	return false
}

// reservedRefusal is what creating one is refused with.
func reservedRefusal(name string) error {
	return fmt.Errorf("instance %q is reserved and no new instance may take it. Its distribution would be %s, "+
		"which reads as the default instance's base, and agents mistook one for the other. "+
		"The default base needs no --instance. Name an instance for what it holds instead",
		name, InstanceDistro(name))
}
