// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
//	4  no record: the ONE name of the two that is
//	   registered, wherever its disk is              the host's existing base
//	5  no record, both names registered              a refusal, never a guess
//	6  no record, neither name registered            `wsl-toolkit-jobs`, a fresh host
//
// ⭐ A HOST THAT ALREADY HAS A BASE SEES NO CHANGE. Before WSL-167 every state
// directory acted on the registered `wsl-toolkit`, wherever its disk was. A caller
// Windows redirects keeps a second state directory with no record (WSL-118), and
// it must use the host's base, not build a second one. Step 4 is that, for either
// name, so a new host's `wsl-toolkit-jobs` is shared the same way.
//
// ⛔ A RECORD STILL TIES A NAME TO ITS DIRECTORY BY THE DISK. A record naming a base
// whose disk is in another state directory is refused by name, because the record
// says this directory built it and the disk says it did not.
//
// ⚠ NOTHING HERE STARTS WSL. Registrations are read from the registry, and no
// guest marker is read: reading one starts the distribution on every command, and
// the name alone decided before WSL-167 too.

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
// case can answer for a host it does not have; production reads the registry.
var defaultBaseHost = struct {
	disks func() (map[string]string, error)
}{
	disks: registeredDisks,
}

// defaultChoice is the name the default instance's base runs under, which step
// chose it, and the base folder holding its disk where that is another state
// directory's.
type defaultChoice struct {
	name, from, at string
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

// resolveDefaultDistro is steps 2 to 6 for the state directory home.
func resolveDefaultDistro(home string) (defaultChoice, error) {
	recorded := ""
	if rec, err := (&Base{home: home}).readRecord(); err == nil {
		switch {
		case strings.EqualFold(rec.Name, LegacyBaseName):
			recorded = LegacyBaseName
		case strings.EqualFold(rec.Name, DefaultBaseName):
			recorded = DefaultBaseName
		}
	}
	choice, err := pickDefaultDistro(home, recorded)
	if err != nil {
		return defaultChoice{}, err
	}
	if choice.name == DefaultBaseName {
		if err := assertNoJobsInstance(home); err != nil {
			return defaultChoice{}, err
		}
	}
	return choice, nil
}

func pickDefaultDistro(home, recorded string) (defaultChoice, error) {
	disks, err := defaultBaseHost.disks()
	if err != nil {
		// ⚠ A HOST WITH NO REGISTRY TO READ HAS NO SECOND CANDIDATE TO FIND. The
		// record still names what this directory built.
		if recorded != "" {
			return defaultChoice{name: recorded, from: NameFromRecord}, nil
		}
		return defaultChoice{name: DefaultBaseName, from: NameFromCanonical}, nil
	}
	ownDir := filepath.Join(home, "base")
	// where says whether a name is registered, and whether its disk is in this
	// state directory's base folder.
	where := func(name string) (at string, registered, here bool) {
		at, registered = registeredAt(disks, name)
		return at, registered, registered && pathEqual(cleanBasePath(at), cleanBasePath(ownDir))
	}
	legacyAt, legacyRegistered, legacyHere := where(LegacyBaseName)
	canonicalAt, canonicalRegistered, canonicalHere := where(DefaultBaseName)

	// ⛔ STEP 2: TWO CANDIDATES IN THIS DIRECTORY, REFUSED BY NAME, record or not.
	if legacyHere && canonicalHere {
		return defaultChoice{}, fmt.Errorf("%w: %s, the older name, and %s, the canonical name, both have their disk in %s. "+
			"Nothing runs, because acting on either would be a guess. "+
			"Choose one by setting base.name to it in the configuration",
			ErrDefaultBaseAmbiguous, LegacyBaseName, DefaultBaseName, ownDir)
	}

	// ⭐ STEP 3: THE RECORD. Where the recorded name is registered with its disk
	// here, it is this directory's base.
	if recorded != "" {
		at, registered, here := where(recorded)
		if registered && here {
			return defaultChoice{name: recorded, from: NameFromRecord}, nil
		}
		if registered {
			// ⛔ THE NAME IS GLOBAL TO THE HOST AND THE RECORD IS NOT. This
			// directory's record says it built a base whose disk is elsewhere, so
			// one of the two is wrong, and neither is guessed at.
			return defaultChoice{}, fmt.Errorf("%w: this state directory's record names %s, and %s is registered with its disk in %s, "+
				"not in %s. That base belongs to another state directory. Nothing runs. "+
				"Point WSL_TOOLKIT_HOME at the directory that built it, or delete %s here",
				ErrDefaultBaseAmbiguous, recorded, recorded, cleanBasePath(at), ownDir, filepath.Join(home, "base.json"))
		}
		// An unregistered record is a base that was removed: the steps below decide.
	}

	// elsewhere is the folder holding a disk, where it is not this directory's.
	elsewhere := func(at string, here bool) string {
		if here {
			return ""
		}
		return cleanBasePath(at)
	}
	switch {
	// ⛔ STEP 5: BOTH NAMES ON THE HOST AND NO RECORD TO CHOOSE, REFUSED BY NAME.
	case legacyRegistered && canonicalRegistered:
		return defaultChoice{}, fmt.Errorf("%w: this state directory, %s, has no record, and both %s, the older name, with its disk in %s, "+
			"and %s, the canonical name, with its disk in %s, are registered. "+
			"Nothing runs, because acting on either would be a guess. "+
			"Choose one by setting base.name to it in the configuration",
			ErrDefaultBaseAmbiguous, home, LegacyBaseName, cleanBasePath(legacyAt), DefaultBaseName, cleanBasePath(canonicalAt))
	// ⭐ STEP 4: THE HOST'S ONE BASE, WHEREVER ITS DISK IS. No marker is read.
	case legacyRegistered:
		return defaultChoice{name: LegacyBaseName, from: NameFromRegistered, at: elsewhere(legacyAt, legacyHere)}, nil
	case canonicalRegistered:
		return defaultChoice{name: DefaultBaseName, from: NameFromRegistered, at: elsewhere(canonicalAt, canonicalHere)}, nil
	}
	// ⭐ STEP 6: A FRESH HOST, under the canonical name.
	return defaultChoice{name: DefaultBaseName, from: NameFromCanonical}, nil
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
	// Elsewhere is the base folder of another state directory that holds this
	// base's disk. This state directory has no record, and acts on that base.
	Elsewhere string `json:"elsewhere,omitempty"`
	// Move is how to rebuild under the canonical name, and what that costs.
	Move string `json:"move,omitempty"`
}

// ElsewhereNote is what `base status` says about a base whose disk is in another
// state directory, or nothing where its disk is here.
func (n *BaseNaming) ElsewhereNote(name string) string {
	if n == nil || n.Elsewhere == "" {
		return ""
	}
	return fmt.Sprintf("this state directory has no record, so it acts on %s, whose disk is in %s, "+
		"another state directory's base folder. Nothing is built here", name, n.Elsewhere)
}

// moveAdvice is what `base status` says to a base under the older name.
//
// ⚠ NOT A RENAME, AND NOT OFFERED AS ONE. A copy by `wsl --export` and
// `--import` carries the marker of the old name, which every guard then reads as
// a copied disk, and it doubles the disk while it runs.
func moveAdvice(from, elsewhere string) string {
	steps := ToolCommand("base remove --yes") + ", then " + ToolCommand("base ensure")
	if from == NameFromConfig {
		steps = "remove base.name from the configuration, then " + steps
	}
	if elsewhere != "" {
		// ⛔ THE MOVE BELONGS TO THE DIRECTORY THAT HOLDS THE DISK. Run from here,
		// it would build the new base in this directory and leave that one's record.
		steps = "from the state directory whose base folder is " + elsewhere + ", " + steps
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
	n := &BaseNaming{Canonical: DefaultBaseName, From: from, Elsewhere: c.nameAt}
	if strings.EqualFold(c.Base.Name, LegacyBaseName) {
		n.Legacy = true
		n.Move = moveAdvice(from, c.nameAt)
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
