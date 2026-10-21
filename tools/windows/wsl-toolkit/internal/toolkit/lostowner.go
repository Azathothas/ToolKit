// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"path/filepath"
)

// ⛔ A KILLED OWNER LEFT ITS RECORD OPEN FOR EVER. The owner closes a job's
// ledger record in its own deferred code, so a process that was killed never
// did, and `resources` reported a run that was interrupted, about a job that
// ended hours before. The owner's lock already says the owner is gone; a reader
// that finds the two disagreeing now closes the record and says why. WSL-121.

// OwnerLostNote is the reason a record is closed for when its owner is gone.
const OwnerLostNote = "closed when read: its owner was lost, so the process that ran the job ended without recording how it ended"

// CloseLostOwners closes the open job records under home whose owner lock is
// there and free and whose job recorded no end, and answers the ids it closed.
// Naming ids narrows it to those.
//
// ⚠ A JOB THAT RECORDED ITS END IS NOT CLOSED HERE. Its owner did not die; its
// teardown failed, and the open record is what still says so.
//
// ⚠ IT WRITES ONLY WHERE SOMETHING IS OPEN. A reader on a machine with no ledger
// creates nothing, which is the rule WSL-55 holds every reader to.
func CloseLostOwners(home string, ids ...string) ([]string, error) {
	if home == "" {
		return nil, nil
	}
	led := &Ledger{path: filepath.Join(home, "ledger.jsonl")}
	open, err := led.Open()
	if err != nil {
		return nil, err
	}
	only := map[string]bool{}
	for _, id := range ids {
		only[id] = true
	}
	var closed []string
	for _, e := range open {
		if e.Kind != "job" || !ValidJobID(e.ID) || (len(only) > 0 && !only[e.ID]) || !ownerGone(home, e.ID) {
			continue
		}
		if _, ended, err := readJobResult(filepath.Join(home, "jobs", e.ID)); err != nil || ended {
			continue
		}
		if err := led.Append(LedgerEntry{Event: "close", Kind: "job", ID: e.ID, Note: OwnerLostNote}); err != nil {
			return closed, err
		}
		closed = append(closed, e.ID)
	}
	return closed, nil
}
