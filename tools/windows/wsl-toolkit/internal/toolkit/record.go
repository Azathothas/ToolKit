// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"path/filepath"
	"time"
)

// ⭐ --remove-record: AN EPHEMERAL JOB THAT LEAVES NOTHING. An ephemeral job
// removes its container and its guest directory, and keeps its host record: the
// transcript, the result and the lock, which `logs ID`, `wait ID` and `gc --job
// ID` read. podbox found three host directories after two jobs. WSL-121.
//
// ⛔ IT IS ASKED FOR, AND EPHEMERAL ALONE DOES NOT DO IT. A caller of an earlier
// release was told "the complete output is kept: logs ID" after an ephemeral
// job, and a wrapper ran `gc --job ID` after each one. Removing the record under
// them would turn both into a refusal, so the removal is its own flag.

// RemoveJobRecord removes an ended ephemeral job's host record, once nothing in
// it is still the only copy of something, and says on the result what it did.
//
// ⛔ A RECORD THAT HOLDS WHAT THE ANSWER DOES NOT IS KEPT, and the result says
// why. An answer whose copy of the output was cut has the complete text only in
// the transcript, and artifacts that were not delivered are fetched again by id.
func RemoveJobRecord(home string, res *JobResult) {
	if home == "" || !ValidJobID(res.ID) || res.ContainerLifecycle != ContainerEphemeral {
		return
	}
	dir := filepath.Join(home, "jobs", res.ID)
	switch {
	case res.StdoutTruncated || res.StderrTruncated:
		res.RecordKept = "the answer's copy of the output was cut at the capture limit, and the record holds the complete text"
		return
	case res.ArtifactError != "" || res.GuestDir != "":
		res.RecordKept = "its artifacts were not delivered, so its record stays beside the copy artifacts retry fetches"
		return
	}
	var err error
	// ⚠ RETRIED BRIEFLY. A follower or the watcher of --cancel-on-exit opens the
	// lock for the length of two system calls, and Windows refuses to delete a
	// file another process has open.
	for attempt := 0; attempt < 5; attempt++ {
		if err = RemoveInside(home, dir); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		res.RecordKept = "it could not be removed: " + err.Error()
		return
	}
	res.RecordRemoved = true
	if res.Transcript != "" && pathEqual(res.Transcript, dir) {
		res.Transcript = ""
	}
}
