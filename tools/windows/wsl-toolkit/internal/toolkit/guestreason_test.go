// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"strings"
	"testing"
	"time"
)

// -- WSL-152: a guest failure is named by its stderr reason, not by a row -----

// wantGuestReason fails a case whose message named the row or the warning the
// fake guest printed before its reason, rather than the reason.
func wantGuestReason(t *testing.T, what, msg string) {
	t.Helper()
	if !strings.Contains(msg, fakeGuestReason) {
		t.Errorf("%s did not name the guest's reason %q: %s", what, fakeGuestReason, msg)
	}
	for _, wrong := range []string{fakeGuestRow, fakeGuestWarn} {
		if strings.Contains(msg, wrong) {
			t.Errorf("%s named %q rather than the reason: %s", what, wrong, msg)
		}
	}
}

// TestAThrowawaySmokeFailureIsNamedByTheGuestsReason holds the smoke step of a
// throwaway import. It read the first line of stderr and stdout, which was a
// warning, and then the last line of both, which was a stdout row.
func TestAThrowawaySmokeFailureIsNamedByTheGuestsReason(t *testing.T) {
	h := newFakeHost(t)
	h.stageGuest(t, "fail=warned")
	tw := &Throwaways{dir: t.TempDir(), wsl: &Wsl{Path: h.wsl}, log: func(string) {}, now: time.Now}
	_, err := tw.smoke(context.Background(), "wsl-toolkit-tmp-smoke", time.Minute)
	if err == nil {
		t.Fatal("a guest that failed passed the smoke step")
	}
	wantGuestReason(t, "the smoke step", err.Error())
}

// TestAThrowawayPid1FailureIsNamedByTheGuestsReason holds the read of PID 1
// after `--systemd` restarts a throwaway.
func TestAThrowawayPid1FailureIsNamedByTheGuestsReason(t *testing.T) {
	h := newFakeHost(t)
	h.stageGuest(t, "fail=warned")
	tw := &Throwaways{dir: t.TempDir(), wsl: &Wsl{Path: h.wsl}, log: func(string) {}, now: time.Now}
	err := tw.enableSystemd(context.Background(), "wsl-toolkit-tmp-smoke", time.Minute)
	if err == nil || !strings.Contains(err.Error(), "PID 1 could not be read") {
		t.Fatalf("a guest that failed after the restart answered %v; want the PID 1 refusal", err)
	}
	wantGuestReason(t, "the PID 1 read", err.Error())
}

// TestAnAdapterProbeFailureIsNamedByTheGuestsReason holds an adapter's probe,
// which prints its rows on stdout and read stderr's FIRST line, a warning.
func TestAnAdapterProbeFailureIsNamedByTheGuestsReason(t *testing.T) {
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	h.stageGuest(t, "fail=warned")
	spec, ok := lookupAdapter("pi")
	if !ok {
		t.Fatal("the pi adapter is not known")
	}
	st := b.probeAdapter(context.Background(), spec)
	if len(st.Problems) != 1 || !strings.HasPrefix(st.Problems[0], "the probe exited 125: ") {
		t.Fatalf("a probe that failed answered %q; want one problem naming exit 125", st.Problems)
	}
	wantGuestReason(t, "the adapter probe", st.Problems[0])
}
