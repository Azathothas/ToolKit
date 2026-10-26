// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// -- WSL-183: a base that cannot start is not a base with no marker ------------

// ⛔ ONLY `cat` SAYING THE FILE IS MISSING IS AN ABSENCE. `cat` names the path
// for an I/O error on a damaged disk too, and that read as a base with no marker.
// A guest WSL did not start is told apart from one that started.
func TestOnlyAMissingMarkerIsAnAbsence(t *testing.T) {
	h := newFakeHost(t)
	_, _, _ = fakeBuiltBase(t, h, DefaultBaseImage)
	w := &Wsl{Path: h.wsl}

	h.say(t, "identity", "eio")
	_, err := w.ReadIdentity(context.Background(), fakeBaseName)
	if errors.Is(err, ErrNoIdentity) || !errors.Is(err, ErrIdentityUnread) || errors.Is(err, ErrGuestUnstartable) {
		t.Fatalf("a marker cat could not read answered %v; want a failure to read it, from a guest that started", err)
	}
	if !strings.Contains(err.Error(), "Input/output error") {
		t.Fatalf("the reason cat gave is not carried: %v", err)
	}

	h.say(t, "identity", "unreadable")
	_, err = w.ReadIdentity(context.Background(), fakeBaseName)
	if !errors.Is(err, ErrGuestUnstartable) || !errors.Is(err, ErrIdentityUnread) || errors.Is(err, ErrNoIdentity) {
		t.Fatalf("a guest WSL did not start answered %v; want a guest that could not be started", err)
	}
	if !strings.Contains(err.Error(), "The distribution failed to start") {
		t.Fatalf("WSL's own reason is not carried: %v", err)
	}
}

// ⛔ THE DEFECT: a base whose disk WSL could not start was refused by `base remove`,
// because the marker it demands could not be read, and only `wsl --unregister` by
// hand removed it. WSL's registration of the disk in this state directory's base
// folder is the proof instead.
func TestAnOwnedBaseThatCannotStartIsRemoved(t *testing.T) {
	for _, how := range []string{"unreadable", "eio"} {
		t.Run(how, func(t *testing.T) {
			h := newFakeHost(t)
			b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
			h.say(t, "identity", how)
			if err := b.Remove(context.Background()); err != nil {
				t.Fatalf("an owned base whose marker could not be read was refused: %v", err)
			}
			if h.registered(t) || callAt(h.calls(t), "wsl --unregister "+fakeBaseName) < 0 {
				t.Fatalf("the base is still registered: %v", h.calls(t))
			}
			if _, err := os.Stat(disk); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("the disk is still there: %v", err)
			}
			if !s.said("WSL keeps the disk of " + fakeBaseName + " in " + b.Dir()) {
				t.Fatalf("the removal did not say what it was removed on: %v", s.lines)
			}
		})
	}
}

// ⛔ THE PROOF IS THE DISK FOLDER, AND NOTHING LESS. A base that cannot start whose
// disk WSL keeps elsewhere is refused, and so is one whose marker names another
// distribution.
func TestABaseThatCannotStartIsRemovedOnlyOnItsDiskFolder(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.say(t, "identity", "unreadable")
	h.say(t, "disks", fakeBaseName+"\t"+t.TempDir())
	err := b.Remove(context.Background())
	if !errors.Is(err, ErrNotOwner) {
		t.Fatalf("a base whose disk is elsewhere answered %v; want the owner's refusal", err)
	}
	assertKept(t, h, b, disk)

	h = newFakeHost(t)
	b, _, disk = fakeBuiltBase(t, h, DefaultBaseImage)
	h.say(t, "identity.json", `{"schema":"`+IdentitySchema+`","name":"wsl-toolkit-another","image":"x","user":"agent"}`)
	err = b.Remove(context.Background())
	if !errors.Is(err, ErrNotOwned) {
		t.Fatalf("a copied disk answered %v; want the copy refused", err)
	}
	assertKept(t, h, b, disk)
}

// ⭐ `base recreate` goes through the same removal, so a base that cannot start is
// rebuilt rather than refused, once the image is pulled and exported.
func TestAnOwnedBaseThatCannotStartIsRebuilt(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	h.say(t, "identity", "unreadable")
	// ⚠ The fake WSL runs no guest script, so the build after the removal fails.
	_, _ = b.EnsureWith(context.Background(), true, false)
	calls := h.calls(t)
	exported := callAt(calls, "podman export")
	removed := callAt(calls, "wsl --unregister "+fakeBaseName)
	imported := callAt(calls, "wsl --import "+fakeBaseName)
	if exported < 0 || removed < exported || imported < removed {
		t.Fatalf("want the export, then the removal, then the import; the calls were %v", calls)
	}
}

// ⭐ AN ENSURE AND A PROBE NAME WHAT REMOVES IT. They said only that the marker
// could not be read, and changed nothing.
func TestABaseThatCannotStartNamesItsRemoval(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.say(t, "identity", "unreadable")
	_, err := b.EnsureWith(context.Background(), false, false)
	if err == nil || !errors.Is(err, ErrGuestUnstartable) || !strings.Contains(err.Error(), "base remove --yes") ||
		!strings.Contains(err.Error(), "Nothing was changed") {
		t.Fatalf("an ensure over a base that cannot start answered %v; want the reason and the removal named", err)
	}
	assertKept(t, h, b, disk)
	st, err := b.Status(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	named := false
	for _, p := range st.Problems {
		if strings.Contains(p, "could not be started") && strings.Contains(p, "base remove --yes") {
			named = true
		}
	}
	if !named {
		t.Fatalf("a probe of a base that cannot start reads %q; want the reason and the removal named", st.Problems)
	}
}
