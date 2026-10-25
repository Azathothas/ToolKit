// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ⛔ A BUILD KILLED BEFORE ITS MARKER, AND WHY THIS FILE EXISTS. WSL-150.
//
// A build stamps the identity marker only after provisioning, and Remove demands
// that marker, so a build killed between the import and the stamp left a
// distribution nothing here would touch. Its rollback runs in the process that
// was killed. Finding 5: a fedora build stopped mid-provisioning left
// `wsl-toolkit-t86fedora` registered, `base remove` refused it, and the advice
// named `base recreate`, which refused it too. `wsl.exe --unregister` was run by
// hand.
//
// So the build records its ownership in the ledger BEFORE the import registers
// anything, and a later process may remove that distribution on two facts, both
// required:
//
//	the ledger holds this build's open record    written before the import
//	WSL keeps the distribution's disk in         this instance's base directory
//
// ⚠ The ledger alone is a file on this machine, and the name alone is a rule any
// import can satisfy. WSL's own registration is what ties the two to this disk.

// baseBuildKind is the ledger kind a base build's ownership record carries.
const baseBuildKind = "base"

// ledgerAt is the ledger of one state directory. ⚠ Base keeps its own home, so
// this reads the base's ledger, not whatever Home answers now.
func ledgerAt(home string) *Ledger {
	return &Ledger{path: filepath.Join(home, "ledger.jsonl")}
}

// recordBuildStart writes the open record a build leaves until it stamps its
// marker. ⛔ BEFORE THE IMPORT, so a build killed at any point after it can be
// found again.
func (b *Base) recordBuildStart() error {
	return ledgerAt(b.home).Append(LedgerEntry{
		Event: "open", Kind: baseBuildKind, ID: b.cfg.Base.Name, Distro: b.cfg.Base.Name,
		Image: b.cfg.Base.Image, HostDir: b.Dir(),
		Note: "imported and not yet stamped with its identity marker",
	})
}

// closeBuildRecord closes this base's open build record, when there is one.
//
// ⚠ A RECORD THAT COULD NOT BE CLOSED IS LOGGED, NOT RETURNED. The distribution
// it describes is stamped or gone by the time this runs, and refusing the
// command over a line in a log would undo nothing.
func (b *Base) closeBuildRecord(note string) {
	if _, open := b.openBuildRecord(); !open {
		return
	}
	if err := ledgerAt(b.home).Append(LedgerEntry{
		Event: "close", Kind: baseBuildKind, ID: b.cfg.Base.Name, Note: note,
	}); err != nil {
		b.log("could not close the build record for " + b.cfg.Base.Name + ": " + err.Error())
	}
}

// openBuildRecord is this base's open build record, read from its own ledger.
func (b *Base) openBuildRecord() (LedgerEntry, bool) {
	open, err := ledgerAt(b.home).Open()
	if err != nil {
		return LedgerEntry{}, false
	}
	for _, e := range open {
		if e.Kind == baseBuildKind && strings.EqualFold(e.ID, b.cfg.Base.Name) && samePath(e.HostDir, b.Dir()) {
			return e, true
		}
	}
	return LedgerEntry{}, false
}

// interruptedBuild says whether the registered distribution is this state
// directory's own build, killed before it was stamped, and why not where it is
// not.
//
// ⭐ THE LEDGER IS READ FIRST, because it is a file on this machine. A base
// that finished its build has no open record, so the registry and the guest are
// asked nothing on the path every ordinary ensure takes.
//
// ⛔ A BUILD THAT IS STILL RUNNING IS NOT A KILLED ONE. Every caller that removes
// on this answer holds the base's lock, which a live build holds from before its
// import until after its stamp, so a live build is never seen here by one of
// them. `base status` holds no lock and asks the lock instead. WSL-150, WSL-172.
//
// ⚠ ONLY `cat` NAMING THE MISSING MARKER IS AN ABSENCE (ReadIdentity, WSL-167),
// so a guest that could not be reached is never read as an unstamped build.
func (b *Base) interruptedBuild(ctx context.Context) (LedgerEntry, bool, string) {
	rec, open := b.openBuildRecord()
	if !open {
		return rec, false, "this state directory's ledger records no build of it that was left unfinished"
	}
	disk, registered, err := b.diskFolder()
	if err != nil {
		return rec, false, "WSL's registration could not be read: " + err.Error()
	}
	if !registered || disk == "" || !samePlace(disk, b.Dir()) {
		return rec, false, fmt.Sprintf("WSL keeps its disk at %s, not at %s where this tool's build put one", valueOrNothing(disk), b.Dir())
	}
	if _, err := b.wsl.ReadIdentity(ctx, b.cfg.Base.Name); !errors.Is(err, ErrNoIdentity) {
		return rec, false, "the guest did not answer that it carries no marker"
	}
	return rec, true, ""
}

// buildRunning names the process holding the base's lock, where one does. ⚠ It
// reads the lock and takes nothing, so `base status` stays a report. WSL-150.
func (b *Base) buildRunning() (string, bool) {
	held, _, err := lockProbe(filepath.Join(b.home, baseLockName))
	if err != nil || !held {
		return "", false
	}
	return readBaseHolder(filepath.Join(b.home, baseHolderName)), true
}

// interruptedBuildNote is the sentence a removal of one says.
func interruptedBuildNote(name string, rec LedgerEntry) string {
	return fmt.Sprintf("%s is this tool's own build, started %s and stopped before its identity marker was written, "+
		"as this state directory's ledger records and WSL's registration of its disk confirms",
		name, rec.At.Format("2006-01-02T15:04:05Z"))
}

// unmarkedAdvice is the rest of the sentence a registered distribution with no
// marker, no record and no interrupted build of this tool's earns. WSL-150.
//
// ⛔ IT NAMED `base recreate`, which demands the same marker and refused too.
// This tool will not remove a distribution it cannot show it built, so the one
// command that can is named, and it is a person's to run.
func unmarkedAdvice(name, why string) string {
	return fmt.Sprintf("nothing here shows this tool built it: %s. This tool will not remove it. "+
		"If it is yours to discard, `wsl --unregister %s` removes it and its disk, and then run: %s",
		why, name, ToolCommand("base ensure"))
}

func valueOrNothing(s string) string {
	if strings.TrimSpace(s) == "" {
		return "no directory"
	}
	return s
}
