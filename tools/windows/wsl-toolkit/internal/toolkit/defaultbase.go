// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ⛔ THE DEFAULT INSTANCE'S BASE HAS TWO NAMES. THIS FILE SAYS WHICH ONE IS MEANT,
// AND WHICH STATE DIRECTORY MAY CHANGE IT. WSL-167, rulings 32 and 33.
//
// The default base was `wsl-toolkit`, and the instance `base` was
// `wsl-toolkit-base`. Each read like the other's. A NEW default base is now
// `wsl-toolkit-jobs`, and an existing one keeps the name it was built under.
//
// NAMING reads the configuration and WSL's registrations, and nothing else:
//
//	1  base.name in the configuration in effect      the operator's own choice
//	2  one of the two names registered as a default  that name, from every home
//	3  both registered                               a refusal by name
//	4  neither, or a registry that cannot be read    `wsl-toolkit-jobs`
//
// ⭐ EVERY CALLER ON A HOST WITH ONE BASE USES IT, whatever its state directory.
// Before WSL-167 every caller acted on the registered `wsl-toolkit`, and a caller
// Windows redirects (WSL-118) still finds the same one.
//
// ⛔ OWNERSHIP decides what changes the base. A state directory owns it when WSL
// keeps its disk in that directory's base folder, as the filesystem resolves both.
// A caller that does not own it uses it, and never removes, rebuilds,
// re-provisions, repairs, stamps or records it.
//
// ⚠ NOTHING HERE STARTS WSL. Registrations are read from the registry.

// The ways the default instance's name was chosen.
const (
	NameFromConfig     = "configuration"
	NameFromRegistered = "registered"
	NameFromCanonical  = "canonical"
)

// ErrDefaultBaseAmbiguous is what a host with both default names is refused with.
var ErrDefaultBaseAmbiguous = errors.New("the default base is ambiguous")

// ErrNotOwner is what a change to a base this state directory does not own is
// refused with. WSL-167.
var ErrNotOwner = errors.New("this state directory does not own the base")

// readRegistrations answers WSL's registrations. ⚠ A variable so a case can
// answer for a host it does not have; production reads the registry.
var readRegistrations = registeredDisks

// RegistrationsForCase makes WSL's registrations answer what read answers, and
// returns what puts the registry back. ⛔ FOR A CASE ONLY, and nothing outside
// a test calls it.
//
// ⛔ A CASE OUTSIDE THIS PACKAGE READ THE OPERATOR'S OWN REGISTRY. Loading a
// configuration resolves the default base's name from it, so a host with both
// default names refused the case's configuration, and the case failed with
// nothing wrong in the code. The base's name and the helper's guest flag came
// from that registry too. WSL-191.
func RegistrationsForCase(read func() (map[string]string, error)) (restore func()) {
	previous := readRegistrations
	readRegistrations = read
	return func() { readRegistrations = previous }
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

// registeredAt finds a distribution in a registry answer, as WSL compares names.
func registeredAt(disks map[string]string, name string) (string, bool) {
	for n, at := range disks {
		if strings.EqualFold(n, name) {
			return at, true
		}
	}
	return "", false
}

// instanceOf answers the instance whose base folder at is, `<root>\instances\<name>\base`.
func instanceOf(at string) (string, bool) {
	at = cleanBasePath(at)
	if at == "" || !strings.EqualFold(filepath.Base(at), "base") {
		return "", false
	}
	holder := filepath.Dir(at)
	if !strings.EqualFold(filepath.Base(filepath.Dir(holder)), "instances") {
		return "", false
	}
	return filepath.Base(holder), true
}

// candidateAt says whether name is registered as a default base, and where.
//
// ⛔ AN INSTANCE IS NOT A DEFAULT BASE. An instance `jobs` made before the
// reservation is registered as `wsl-toolkit-jobs` with its disk under
// `instances\jobs\base`.
func candidateAt(disks map[string]string, name string) (string, bool) {
	at, ok := registeredAt(disks, name)
	if !ok {
		return "", false
	}
	if _, instance := instanceOf(at); instance {
		return "", false
	}
	return cleanBasePath(at), true
}

// resolveDefaultName is steps 2 to 4: the name, and which step chose it.
func resolveDefaultName() (string, string, error) {
	return resolveDefaultNameIn(readRegistrations())
}

// resolveDefaultNameIn is resolveDefaultName over a registry answer.
func resolveDefaultNameIn(disks map[string]string, err error) (string, string, error) {
	if err != nil {
		// ⚠ NO REGISTRY TO READ: a host that is not Windows, or a case.
		return DefaultBaseName, NameFromCanonical, nil
	}
	legacyAt, legacy := candidateAt(disks, LegacyBaseName)
	canonicalAt, canonical := candidateAt(disks, DefaultBaseName)
	switch {
	case legacy && canonical:
		return "", "", fmt.Errorf("%w: both %s, with its disk in %s, and %s, with its disk in %s, are registered. "+
			"Nothing runs, because acting on either would be a guess. "+
			"Choose one by setting base.name to it in the configuration",
			ErrDefaultBaseAmbiguous, LegacyBaseName, diskWords(legacyAt), DefaultBaseName, diskWords(canonicalAt))
	case legacy:
		return LegacyBaseName, NameFromRegistered, nil
	case canonical:
		return DefaultBaseName, NameFromRegistered, nil
	}
	return DefaultBaseName, NameFromCanonical, nil
}

// diskWords is a disk folder as a message names it.
func diskWords(at string) string {
	if at == "" {
		return "no folder WSL names"
	}
	return at
}

// canonicalHeldIn refuses the default's canonical name while an instance is
// registered under it. ⚠ Only a registration counts, never a directory. WSL-167.
func canonicalHeldIn(disks map[string]string, name string) error {
	if !strings.EqualFold(name, DefaultBaseName) {
		return nil
	}
	at, ok := registeredAt(disks, DefaultBaseName)
	if !ok {
		return nil
	}
	if _, instance := instanceOf(at); !instance {
		return nil
	}
	return fmt.Errorf("the default base would be %s, and an instance with its disk in %s is registered under that name. "+
		"Nothing runs. Remove that instance, %s, or set base.name to %s",
		DefaultBaseName, cleanBasePath(at), holderCommand(at, "base remove --yes"), LegacyBaseName)
}

// assertCanonicalFree is canonicalHeldIn for the default instance's base.
//
// ⛔ AT THE ENSURE, NOT AT THE LOAD. Refused while the configuration loaded, it
// took status, logs, ready, gc and the helper from a host whose only toolkit
// registration was an old `jobs` instance. ⚠ A registry that cannot be read
// refuses nothing here, as before.
func (b *Base) assertCanonicalFree() error {
	if !isDefaultSelection() {
		return nil
	}
	disks, err := b.registrations()
	if err != nil {
		return nil
	}
	return canonicalHeldIn(disks, b.cfg.Base.Name)
}

// assertNameStillDefault reads the default name again, under the base's lock,
// before a new default base is built. WSL-167.
//
// ⛔ THE NAME WAS READ WHEN THE CONFIGURATION LOADED. A home that loaded the host's
// `wsl-toolkit`, and reached its build after the owner removed it, imported a
// second `wsl-toolkit` into itself. Only the name the rule gives now is built.
//
// ⚠ A NEW BUILD ONLY. A rebuild, with its rootfs exported, has just removed the
// base it replaces, under the name it keeps.
func (b *Base) assertNameStillDefault(rootfs string) error {
	if rootfs != "" || !isDefaultSelection() || b.cfg.nameFrom == NameFromConfig || !IsDefaultBaseName(b.cfg.Base.Name) {
		return nil
	}
	disks, readErr := b.registrations()
	name, _, err := resolveDefaultNameIn(disks, readErr)
	if err != nil {
		return fmt.Errorf("%s was NOT built: %w", b.cfg.Base.Name, err)
	}
	if !strings.EqualFold(name, b.cfg.Base.Name) {
		return fmt.Errorf("%s was NOT built: WSL's registrations changed while this waited, and the default base is now %s. "+
			"Nothing was built. Run it again", b.cfg.Base.Name, name)
	}
	if _, registered := registeredAt(disks, name); readErr == nil && registered {
		return fmt.Errorf("%s was NOT built: it was registered while this waited. Nothing was built. Run it again", name)
	}
	return canonicalHeldIn(disks, name)
}

// helperNameFrom is which step chose a name that came over the helper's wire.
// sent is the step the client read, which travels beside its configuration.
//
// ⛔ ONLY THE CLIENT'S FILE MAKES A NAME THE CONFIGURATION'S. A default name the
// rule no longer gives was read before the registry changed, and a build refuses
// it in assertNameStillDefault. Called the configuration's, it skipped that
// check, and a stale `wsl-toolkit` was imported into the helper's home.
func helperNameFrom(cfg Config, sent string) string {
	if !isDefaultSelection() || !IsDefaultBaseName(cfg.Base.Name) {
		return ""
	}
	if sent == NameFromConfig {
		return NameFromConfig
	}
	if name, from, err := resolveDefaultName(); err == nil && strings.EqualFold(name, cfg.Base.Name) {
		return from
	}
	return NameFromRegistered
}

// -- ownership -----------------------------------------------------------------

// registrations is WSL's registrations, as this base reads them.
func (b *Base) registrations() (map[string]string, error) {
	if b.disks != nil {
		return b.disks()
	}
	return readRegistrations()
}

// diskFolder is the folder WSL keeps the configured base's disk in, and whether
// the name is registered there.
func (b *Base) diskFolder() (string, bool, error) {
	disks, err := b.registrations()
	if err != nil {
		return "", false, err
	}
	at, ok := registeredAt(disks, b.cfg.Base.Name)
	return cleanBasePath(at), ok, nil
}

// assertOwner refuses a change to the configured base unless this state directory
// owns it. verb says what was not done, and args is the command that does it.
//
// ⛔ BEFORE ANYTHING CHANGES, AND CLOSED. A registry that cannot be read, and a
// registration with no disk folder, prove nothing, so both refuse.
func (b *Base) assertOwner(verb, args string) error {
	name := b.cfg.Base.Name
	// ⛔ A REQUEST FROM A CALLER THAT DOES NOT OWN THE BASE stays one through the
	// helper. The helper's home may own it, and the configuration is the caller's.
	if b.guest {
		advice := "Run it from the state directory that owns it"
		if at, registered, err := b.diskFolder(); err == nil && registered && at != "" {
			advice = ownerAdvice(name, at, args)
		}
		return fmt.Errorf("%w: %s was NOT %s. The caller that sent this through the helper does not own it, "+
			"so its configuration does not change it. %s", ErrNotOwner, name, verb, advice)
	}
	// ⛔ A REDIRECTED PROCESS WRITES INTO A PACKAGE'S COPY OF THE STATE DIRECTORY,
	// so its records and locks are not the ones the base's owner keeps. WSL-118.
	if copyAt := b.redirected(); copyAt != "" {
		return fmt.Errorf("%w: %s was NOT %s. Windows redirects this process's writes under %%LOCALAPPDATA%% into %s, "+
			"so its locks and records are not the owner's. Run it from PowerShell or a terminal outside the app",
			ErrNotOwner, name, verb, copyAt)
	}
	at, registered, err := b.diskFolder()
	if err != nil {
		return fmt.Errorf("%w: %s was NOT %s. WSL's registrations could not be read, so nothing proves this state directory, %s, owns it: %v",
			ErrNotOwner, name, verb, b.home, err)
	}
	if !registered || at == "" {
		return fmt.Errorf("%w: %s was NOT %s. WSL names no disk folder for it, so nothing proves this state directory, %s, owns it",
			ErrNotOwner, name, verb, b.home)
	}
	if samePlace(at, b.Dir()) {
		return nil
	}
	return fmt.Errorf("%w: %s was NOT %s. Its disk is in %s, and this state directory is %s. "+
		"A state directory uses a base it does not own, and never changes it. %s",
		ErrNotOwner, name, verb, at, b.home, ownerAdvice(name, at, args))
}

// owns says whether this state directory owns the configured base. ⚠ Closed: any
// doubt answers no.
func (b *Base) owns() bool { return b.assertOwner("changed", "base ensure") == nil }

// redirected is the package copy this process's writes land in, or empty.
func (b *Base) redirected() string {
	for _, c := range StateCopies(b.home) {
		if c.ThisProcess {
			return c.Path
		}
	}
	return ""
}

// clientBase is the base as a helper's client sees it: its own home and the
// registry, with no wsl.exe, which such a client may not reach.
func clientBase(cfg Config) (*Base, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	return &Base{cfg: cfg, home: home, log: func(string) {}}, nil
}

// OwnsBase says whether this process's state directory owns the configured base.
func OwnsBase(cfg Config) bool {
	b, err := clientBase(cfg)
	return err == nil && b.owns()
}

// helperGuest says whether the helper must run this client's ensure as a guest's,
// one that changes nothing only the owner may change. WSL-167.
//
// ⛔ THE HELPER CHECKS OWNERSHIP AGAINST ITS OWN HOME. A caller that does not own
// the base, or whose writes Windows redirects, had the helper rebuild, repair and
// re-provision it, and drop the owner's adapters, from the caller's
// configuration. ⚠ Any doubt answers yes.
func helperGuest(cfg Config) bool {
	b, err := clientBase(cfg)
	if err != nil {
		return true
	}
	return b.helperGuest()
}

func (b *Base) helperGuest() bool {
	if b.owns() {
		return false
	}
	// ⚠ NOTHING REGISTERED IS NOTHING TO CHANGE, so the build goes ahead as
	// before. A redirected caller still installs no adapter.
	if _, registered, err := b.diskFolder(); err == nil && !registered {
		return b.redirected() != ""
	}
	return true
}

// ownerAdvice says where to run args from, for a disk in the folder at.
func ownerAdvice(name, at, args string) string {
	if !strings.EqualFold(filepath.Base(at), "base") {
		// ⚠ MOVED WITH `wsl --manage --move`, or placed by hand: no state
		// directory's base folder holds it.
		return fmt.Sprintf("This tool did not place that disk, so no state directory owns it. "+
			"To remove it, losing its disk, run it yourself: wsl --unregister %s", name)
	}
	return "Run this from the state directory that owns it: " + holderCommand(at, args)
}

// holderCommand is the command line that runs args from the state directory whose
// base folder is at. ⚠ An instance's base folder is under its root's instances
// folder, so the command names the instance and the root.
func holderCommand(at, args string) string {
	holder := filepath.Dir(cleanBasePath(at))
	cmd := ToolCommandFor(DefaultInstance, args)
	if instance, ok := instanceOf(at); ok {
		cmd = ToolCommandFor(instance, args)
		holder = filepath.Dir(filepath.Dir(holder))
	}
	if home, err := defaultHome(); err == nil && samePlace(home, holder) {
		return "with WSL_TOOLKIT_HOME unset, the default state directory: " + cmd
	}
	return "$env:WSL_TOOLKIT_HOME='" + holder + "'; " + cmd
}

// -- what base status says -----------------------------------------------------

// BaseNaming says which name the default instance's base runs under, where its
// disk is, and whether this state directory owns it. WSL-167.
type BaseNaming struct {
	// Canonical is the name a new default base is registered as.
	Canonical string `json:"canonical"`
	// Legacy is true where the base runs under the older name.
	Legacy bool `json:"legacy"`
	// From is which step chose the name: configuration, registered or canonical.
	From string `json:"from"`
	// Disk is the folder WSL keeps the base's disk in, and empty where it is not
	// registered or the registry could not be read.
	Disk string `json:"disk,omitempty"`
	// Owner is true where that folder is this state directory's base folder.
	Owner bool `json:"owner"`
	// Instance names the instance whose distribution is registered under the
	// name, where it is one: an old `jobs`, and not the default base.
	Instance string `json:"instance,omitempty"`
	// Move is how to rebuild under the canonical name, and what that costs.
	Move string `json:"move,omitempty"`
}

// naming is the BaseNaming for this base, or nil for a named instance.
func (b *Base) naming() *BaseNaming {
	if !isDefaultSelection() || !IsDefaultBaseName(b.cfg.Base.Name) {
		return nil
	}
	from := b.cfg.nameFrom
	if from == "" {
		from = NameFromCanonical
	}
	n := &BaseNaming{Canonical: DefaultBaseName, From: from}
	if at, registered, err := b.diskFolder(); err == nil && registered {
		n.Disk = at
		n.Owner = b.owns()
		n.Instance, _ = instanceOf(at)
	}
	if strings.EqualFold(b.cfg.Base.Name, LegacyBaseName) {
		n.Legacy = true
		n.Move = moveAdvice(from)
	}
	return n
}

// moveAdvice is what `base status` says to a base under the older name.
//
// ⚠ NOT A RENAME, AND NOT OFFERED AS ONE. A copy by `wsl --export` and `--import`
// carries the marker of the old name, and it doubles the disk while it runs.
func moveAdvice(from string) string {
	steps := ToolCommand("base remove --yes") + ", then " + ToolCommand("base ensure")
	if from == NameFromConfig {
		steps = "remove base.name from the configuration, then " + steps
	}
	return fmt.Sprintf("this base runs under the older name %s, and a new one is named %s. "+
		"To move, from the state directory that owns it: %s. That deletes this base's disk, with its home, images and credentials, "+
		"and builds a new one from the image. The grants in the configuration are kept. "+
		"Nothing moves until you run it", LegacyBaseName, DefaultBaseName, steps)
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
