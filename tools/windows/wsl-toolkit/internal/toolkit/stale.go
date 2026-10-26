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

// staleRunStateBudget bounds the check: the lock repair.sh waits on for 120s, and
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
	return append(payload, runStateChecks()...)
}

// runStateChecks is the payload after its prologue: bootmark, then stale.sh,
// then repair.sh.
//
// ⛔ bootmark IS THE SESSIONS' OWN, so a session and the run state name a start
// of the base in one way. stale.sh holds a current state to this start and reads
// a record held to another as stale, and repair.sh holds a cleared one to it.
// `wsl --terminate` keeps the boot id, so the boot id alone missed it. WSL-178.
func runStateChecks() []byte {
	payload := append([]byte{}, sessionProcStart...)
	payload = append(payload, staleScript...)
	payload = append(payload, '\n')
	return append(payload, repairScript...)
}

// repairPayload is what `base ensure --repair` sends after its prologue:
// bootmark, then repair.sh, which holds what it cleared to this start. WSL-178.
func repairPayload() []byte {
	return append([]byte(sessionProcStart), repairScript...)
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
	// ⛔ AN EARLIER START'S STATE IS SAID AS THAT, and podman does not refuse it:
	// it reads that start's containers as running instead. WSL-178.
	earlier := strings.Contains(out, "run-state stale: the engine recorded an earlier start of this base")
	switch {
	case !failed && strings.Contains(out, "repaired run-state"):
		// ⭐ WHAT WAS REMOVED IS NAMED, from repair.sh's own read-back lines.
		// ⚠ `base ensure --repair` never reaches an earlier start's state, so it
		// is not named for one.
		if earlier {
			return true, "the engine's run state was left by an earlier start of this base, so it was cleared before this started: " +
				removedRunState(out)
		}
		return true, "the engine's run state named another boot of WSL, so it was cleared before this started, " +
			"as " + ToolCommand("base ensure --repair") + " would have: " + removedRunState(out)
	case !failed && strings.Contains(out, "run-state current"):
		return false, ""
	case strings.Contains(out, "run-state stale"):
		// ⚠ STALE AND STILL THERE: no flock, a container on it, or the clear
		// refused. The guest says which, and the command that clears it is named.
		reason := "this base has no flock to clear it safely"
		if why, ok := keptRunState(out); ok {
			reason = why
		}
		if failed {
			reason = fmt.Sprintf("clearing it failed (exit %d): %s", code, guestFailure(out, stderr))
		}
		// ⚠ NO COMMAND IS NAMED FOR AN EARLIER START'S STATE. podman does not
		// refuse it, so the probe passes and `base ensure --repair` never reaches
		// its clear. The next job or start clears it once nothing keeps it.
		if earlier {
			return false, "the engine's run state was left by an earlier start of this base and " + reason +
				", so podman may read that start's containers as running. The next job or start of the base clears it once nothing keeps it"
		}
		return false, "the engine's run state names another boot of WSL and " + reason +
			", so a container will be refused. Run: " + ToolCommand("base ensure --repair")
	case failed:
		return false, fmt.Sprintf("the engine's run state could not be checked for a stale boot id (exit %d): %s",
			code, guestFailure(out, stderr))
	}
	return false, "the stale boot id check exited 0 without saying what it found: " + guestFailure(out, stderr)
}

// keptRunState is why the guest kept a stale state, from its last `run-state
// kept:` line. WSL-178.
func keptRunState(out string) (string, bool) {
	why, found := "", false
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "run-state kept: "); ok && v != "" {
			why, found = v, true
		}
	}
	return why, found
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
