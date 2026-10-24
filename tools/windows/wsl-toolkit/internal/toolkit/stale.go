// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"
)

//go:embed stale.sh
var staleScript []byte

// staleRunStateBudget bounds the check: the lock stale.sh waits on for 120s, and
// the deletion after it.
const staleRunStateBudget = 3 * time.Minute

// staleRunStatePayload is what `run`, `matrix` and `base exec` send before their
// first container: the runtime prologue, stale.sh, then repair.sh, which stale.sh
// reaches only over a boot id the engine recorded for another boot of WSL.
// WSL-137.
//
// ⛔ ONE DELETION, TWO DOORS. repair.sh's four rules and its lock hold this path
// as they hold `base ensure --repair`, and its fifth holds it to the root the
// stale record was read under.
func staleRunStatePayload() []byte {
	payload := append([]byte{}, guestRuntimePrologue()...)
	payload = append(payload, staleScript...)
	payload = append(payload, '\n')
	return append(payload, repairScript...)
}

// ClearStaleRunState clears the engine's run state in a base when a restart of
// WSL left it naming another boot, and does nothing otherwise. It answers
// whether it cleared anything, and logs only when there was something to say.
//
// ⛔ FINDING 67, AND WHY THE JOB COMMANDS DO THIS THEMSELVES. `wsl --shutdown`
// stales the engine's boot id in every base that holds one, and only `base
// ensure --repair` cleared it, one base at a time. So after a restart every job
// on the default base exited 2 before anything could start, over a deletion
// podman itself had already prescribed. WSL-137.
//
// ⚠ A CHECK THAT FAILED STOPS NOTHING. The caller goes on, and a container the
// engine still refuses fails with the engine's own words, as it did before.
func ClearStaleRunState(ctx context.Context, w *Wsl, distro, user string, log func(string)) bool {
	out, stderr, code, err := w.Capture(ctx, distro, user, staleRunStatePayload(), staleRunStateBudget)
	cleared, note := staleRunStateVerdict(out, stderr, code, err)
	if note != "" && log != nil {
		log(note)
	}
	return cleared
}

// staleRunStateVerdict reads what the payload printed: whether it cleared the
// state, and the sentence a caller prints, which is empty where the state was
// current.
//
// ⭐ A FUNCTION WITH NO PROCESS IN IT, so every answer has a case on any host.
//
// ⛔ THE DELETION IS READ FROM repair.sh's OWN LAST LINE, which it prints only
// after reading each removal back. An exit code of 0 alone is not a repair.
func staleRunStateVerdict(out, stderr string, code int, err error) (bool, string) {
	failed := err != nil || code != 0
	switch {
	case !failed && strings.Contains(out, "repaired run-state"):
		// ⭐ WHAT WAS REMOVED IS NAMED, from repair.sh's own read-back lines.
		return true, "the engine's run state named another boot of WSL, so it was cleared before this started, " +
			"as " + ToolCommand("base ensure --repair") + " would have: " + removedRunState(out)
	case !failed && strings.Contains(out, "run-state current"):
		return false, ""
	case strings.Contains(out, "run-state stale"):
		// ⚠ STALE AND STILL THERE: no flock, or the clear refused. Either way a
		// container will be refused, and the command that clears it is named.
		reason := "this base has no flock to clear it safely"
		if failed {
			reason = fmt.Sprintf("clearing it failed (exit %d): %s", code, guestFailure(out, stderr))
		}
		return false, "the engine's run state names another boot of WSL and " + reason +
			", so a container will be refused. Run: " + ToolCommand("base ensure --repair")
	case failed:
		return false, fmt.Sprintf("the engine's run state could not be checked for a stale boot id (exit %d): %s",
			code, guestFailure(out, stderr))
	}
	return false, "the stale boot id check exited 0 without saying what it found: " + guestFailure(out, stderr)
}

// removedRunState lists the directories repair.sh read back as removed.
func removedRunState(out string) string {
	var removed []string
	for _, line := range strings.Split(out, "\n") {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "removed "); ok {
			removed = append(removed, path)
		}
	}
	if len(removed) == 0 {
		return "repair.sh named nothing it removed"
	}
	return "removed " + strings.Join(removed, " and ")
}
