// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-183: a base that cannot start is not a base with no marker ------------

// ⛔ ONLY `cat` SAYING THE FILE IS MISSING IS AN ABSENCE. `cat` names the path
// for an I/O error on a damaged disk too, and that read as a base with no marker.
// A guest WSL did not start is told apart from one that started, BusyBox's `cat`
// included, which names no path for a read error.
func TestOnlyAMissingMarkerIsAnAbsence(t *testing.T) {
	h := newFakeHost(t)
	_, _, _ = fakeBuiltBase(t, h, DefaultBaseImage)
	w := &Wsl{Path: h.wsl}

	for _, how := range []string{"eio", "busybox-eio"} {
		h.say(t, "identity", how)
		_, err := w.ReadIdentity(context.Background(), fakeBaseName)
		if errors.Is(err, ErrNoIdentity) || !errors.Is(err, ErrIdentityUnread) || errors.Is(err, ErrGuestUnstartable) {
			t.Fatalf("%s: a marker cat could not read answered %v; want a failure to read it, from a guest that started", how, err)
		}
		if !strings.Contains(err.Error(), "Input/output error") && !strings.Contains(err.Error(), "I/O error") {
			t.Fatalf("%s: the reason cat gave is not carried: %v", how, err)
		}
	}

	h.say(t, "identity", "unreadable")
	_, err := w.ReadIdentity(context.Background(), fakeBaseName)
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

// firstWayForward is the remediation `ready` names first: the first one that is
// repairable and not advisory.
func firstWayForward(st BaseState) (Remediation, bool) {
	for _, r := range st.Remediations {
		if r.Repairable && !r.Advisory {
			return r, true
		}
	}
	return Remediation{}, false
}

// ⛔ THE DEFECT: `ready` over a base whose marker could not be read named `base
// ensure`, which refuses that base, and under `ready --ensure` it named the
// command that had just failed. The probe, the ensure's refusal and the helper's
// answer each carry the rebuild as the way forward, and what it costs.
func TestWhatReadyReadsOfABaseWhoseMarkerCannotBeRead(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.say(t, "identity", "unreadable")
	wantRebuild := func(how string, st BaseState) {
		t.Helper()
		r, ok := firstWayForward(st)
		if !ok || r.ID != UnreadMarkerID || !strings.HasSuffix(r.Command, "base recreate") {
			t.Fatalf("%s: ready would name %+v; want the rebuild, from %+v", how, r, st.Remediations)
		}
		if !strings.Contains(r.Costs, "home, credentials and images") {
			t.Fatalf("%s: the rebuild does not say what it costs: %q", how, r.Costs)
		}
	}

	st, err := b.Status(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	wantRebuild("ready", st)

	st, err = b.EnsureWith(context.Background(), false, false)
	if err == nil || !errors.Is(err, ErrIdentityUnread) {
		t.Fatalf("an ensure over a base that cannot start answered %v; want the refusal", err)
	}
	wantRebuild("ready --ensure", st)

	code, msg, st := helperStateOver(t, b)(`{}`)
	if code == http.StatusOK || !strings.Contains(msg, ErrIdentityUnread.Error()) {
		t.Fatalf("an ensure through the helper answered %d %q; want the refusal", code, msg)
	}
	wantRebuild("ready --ensure through the helper", st)
	assertKept(t, h, b, disk)
}

// ⛔ A LIVE BUILD IS NOT ADVISED AWAY. A marker read that fails while another
// process holds the base's lock may be that build's restart, and the probe told
// the reader to remove the base it was making.
func TestAProbeDuringABuildDoesNotAdviseAnUnreadBaseAway(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	h.say(t, "identity", "unreadable")
	release, err := lockBaseAt(context.Background(), b.home, "base recreate", func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	st, err := b.Status(context.Background(), true)
	release()
	if err != nil {
		t.Fatal(err)
	}
	said := strings.Join(st.Problems, "\n")
	if strings.Contains(said, "base remove") || !strings.Contains(said, "running `base recreate`") || !strings.Contains(said, "once that ends") {
		t.Fatalf("a probe during a build said %q", st.Problems)
	}
	if _, ok := remediationNamed(st, UnreadMarkerID); ok {
		t.Fatalf("a probe during a build names the rebuild: %+v", st.Remediations)
	}
	// ⭐ AND ONCE THE LOCK IS FREE, THE SAME BASE NAMES ITS REMOVAL.
	st, err = b.Status(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := remediationNamed(st, UnreadMarkerID); !ok || !strings.Contains(strings.Join(st.Problems, "\n"), "base remove --yes") {
		t.Fatalf("a probe after the build said %q, %+v", st.Problems, st.Remediations)
	}
}

// ⛔ A MISSING MARKER IS AN ABSENCE IN ANY LANGUAGE. Only `cat`'s English said so,
// and nothing held a guest to it, so a German guest's missing marker read as one
// that could not be read: the removal passed over the ledger and went on the disk
// folder alone. The read sets the C locale itself.
func TestAMissingMarkerInAnotherLanguageIsStillAnAbsence(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	if err := os.Remove(filepath.Join(h.state, "identity.json")); err != nil {
		t.Fatal(err)
	}
	h.say(t, "identity", "german")
	if _, err := b.wsl.ReadIdentity(context.Background(), fakeBaseName); !errors.Is(err, ErrNoIdentity) || errors.Is(err, ErrIdentityUnread) {
		t.Fatalf("a German guest's missing marker answered %v; want no marker", err)
	}
	// ⚠ No ledger record says this tool's build stopped before its marker, so the
	// removal is refused, as for any base with no marker.
	if err := b.Remove(context.Background()); err == nil || !errors.Is(err, ErrNoIdentity) {
		t.Fatalf("removing a German guest with no marker answered %v; want the refusal", err)
	}
	assertKept(t, h, b, disk)
}

// ⛔ A MARKER THAT DOES NOT PARSE IS NOT ONE THAT COULD NOT BE READ. It was read,
// and it says nothing this tool wrote, so the removal on the disk folder alone does
// not reach it.
func TestAMarkerThatDoesNotParseStillRefusesARemoval(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.say(t, "identity.json", "{this is not the marker")
	err := b.Remove(context.Background())
	if err == nil || !strings.Contains(err.Error(), "does not parse") || errors.Is(err, ErrIdentityUnread) {
		t.Fatalf("removing a base whose marker does not parse answered %v; want the refusal", err)
	}
	assertKept(t, h, b, disk)
}
