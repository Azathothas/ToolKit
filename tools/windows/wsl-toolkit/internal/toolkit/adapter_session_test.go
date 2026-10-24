// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"os"
	"strings"
	"testing"
)

// probeScripts is each adapter probe the fake guest was handed for name, in order.
func probeScripts(t *testing.T, h *fakeHost, name string) []string {
	t.Helper()
	var found []string
	for _, s := range strings.Split(h.guestScripts(t), "\n-- end of script --\n") {
		if strings.Contains(s, "# "+name+" adapter, probe:") && !strings.Contains(s, "TK_PROBE_FOOTPRINT='1'") {
			found = append(found, s)
		}
	}
	return found
}

// ⛔ ONLY base ensure STARTS AN AGENT TO READ IT, AND ITS ANSWER CARRIES WHAT THAT
// FOUND. The muse probe runs a Muse session, which runs every hook the account
// registers and can take a minute, and `base status --probe` and `ready` ran it on
// every read. The ensure's own read-back is the one that may; its fact and note
// reach the state the ensure answers, because the helper route keeps the ensure's
// log in the helper. A note fails nothing. WSL-157.
func TestOnlyEnsureStartsAMuseSessionAndItsAnswerCarriesTheFinding(t *testing.T) {
	h := newFakeHost(t)
	b, s, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	b.cfg.Base.Adapters = []BaseAdapter{
		{Name: "muse"},
	}
	h.stageGuest(t, "verify=ok", "binfmt=ok", "muse=ok")
	// ⚠ THE LAUNCHER'S DIRECTORY IS ON PATH, so `muse` reaches it and the PATH
	// note stays out of the notes this case counts. WSL-167 made the default
	// base's launcher plain muse.exe, which is what that note reads back.
	t.Setenv("PATH", os.Getenv(BinDirEnv)+string(os.PathListSeparator)+os.Getenv("PATH"))

	st, err := b.EnsureWith(context.Background(), false, false)
	if err != nil {
		t.Fatalf("a Muse whose session the reporter never saw failed the ensure, and a note fails nothing: %v", err)
	}
	probes := probeScripts(t, h, "muse")
	if len(probes) != 2 {
		t.Fatalf("the ensure ran %d muse probes; want its read-back and the state's", len(probes))
	}
	if assigned([]byte(probes[0]), "TK_PROBE_SESSION") != "1" {
		t.Fatal("the ensure's read-back did not ask for a Muse session")
	}
	if assigned([]byte(probes[1]), "TK_PROBE_SESSION") != "" {
		t.Fatal("the state's read after the ensure asked for a Muse session, and a read starts none")
	}
	var muse *AdapterState
	for i := range st.Adapters {
		if st.Adapters[i].Name == "muse" {
			muse = &st.Adapters[i]
		}
	}
	if muse == nil {
		t.Fatalf("the ensure's answer carries no muse adapter: %+v", st.Adapters)
	}
	if muse.Facts["herdr_reporter_session_start"] != "missing" {
		t.Fatalf("the ensure's answer lost the session's fact: %v", muse.Facts)
	}
	if len(muse.Notes) != 1 || muse.Notes[0] != fakeMuseSessionNote {
		t.Fatalf("the ensure's answer carries notes %q; want the session's one", muse.Notes)
	}
	if len(muse.Problems) != 0 || len(st.Problems) != 0 {
		t.Fatalf("a note was read as a problem: %q %q", muse.Problems, st.Problems)
	}
	if !s.said("adapter muse: " + fakeMuseSessionNote) {
		t.Fatal("the ensure's log does not say the note")
	}

	read, err := b.Status(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := probeScripts(t, h, "muse"); len(got) != 3 || assigned([]byte(got[2]), "TK_PROBE_SESSION") != "" {
		t.Fatal("base status --probe asked for a Muse session, and a read starts none")
	}
	for _, a := range read.Adapters {
		if _, ok := a.Facts["herdr_reporter_session_start"]; ok || len(a.Notes) != 0 {
			t.Fatalf("a read answered a session it never ran: %v %q", a.Facts, a.Notes)
		}
	}
}

// ⛔ A SERVER STILL ON THE OLD herdr IS SAID ON THE STATE, WITH ITS RESTART. It was
// said only in the ensure's log, which the helper route keeps in the helper, so a
// caller read the new version while the server ran the old one. Advisory: the base
// works, and the restart ends every pane. WSL-141.
func TestAStaleHerdrServerIsAnAdvisoryWithItsRestart(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	b.cfg.Base.Adapters = []BaseAdapter{
		{Name: "herdr"},
	}
	h.stageGuest(t, "verify=ok", "binfmt=ok", "herdr=stale")
	st, err := b.Status(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	var stale *Remediation
	for i := range st.Remediations {
		if st.Remediations[i].ID == "herdr-server-stale" {
			stale = &st.Remediations[i]
		}
	}
	if stale == nil {
		t.Fatalf("a herdr server on the old binary earned no remediation: %+v", st.Remediations)
	}
	if !stale.Advisory || !strings.HasSuffix(stale.Command, "base exec --root -c 'systemctl restart wsl-toolkit-herdr.service'") ||
		!strings.Contains(stale.What, "0.9.5") || !strings.Contains(stale.Costs, "ends every herdr pane") {
		t.Fatalf("the stale server's remediation is %+v", *stale)
	}
}

// ⭐ A PROBE'S `note` LINE IS A NOTE, not a fact named note. WSL-157.
func TestAProbesNoteLineIsANote(t *testing.T) {
	st := parseAdapterProbe("muse", "installed yes\nversion 1\nnote something true\n")
	if len(st.Notes) != 1 || st.Notes[0] != "something true" || st.Facts["note"] != "" || !st.Healthy {
		t.Fatalf("a note line was read as %+v", st)
	}
}
