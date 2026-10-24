// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"path/filepath"
	"time"
)

// ⭐ AN EPHEMERAL JOB LEAVES NOTHING. The help of --container-lifecycle said
// that ephemeral removes both the container and the job directory, and the job
// directory stayed until `gc`: podbox found three host directories after two
// jobs. The record now goes once the answer is ready, and the answer says so.
// A caller that needs `logs ID` or `gc --job ID` after the job runs it
// persistent. WSL-121, as ruled.

// DetachedRecordKept is why a detached ephemeral job keeps its host record.
const DetachedRecordKept = "a detached job's answer is its record, which wait and logs --follow read after the job has ended"

// RemoveJobRecord removes an ended ephemeral job's host record, once nothing in
// it is still the only copy of something, and says on the result what it did.
// lifecycle is the job's own, which a result refused before its container does
// not carry.
//
// ⛔ A RECORD THAT HOLDS WHAT THE ANSWER DOES NOT IS KEPT, and the result says
// why. An answer whose copy of the output was cut has the complete text only in
// the transcript, and artifacts that were not delivered are fetched again by id.
func RemoveJobRecord(home, lifecycle string, res *JobResult) {
	if home == "" || !ValidJobID(res.ID) || lifecycle != ContainerEphemeral || res.RecordKept != "" {
		return
	}
	dir := filepath.Join(home, "jobs", res.ID)
	if why := recordStillNeeded(*res); why != "" {
		res.RecordKept = why
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

// recordStillNeeded says why a job's record holds something its answer does
// not, or "".
//
// ⚠ EACH REASON NAMES ITS OWN CAUSE. A guest directory left behind was read as
// artifacts not delivered, which a job that asked for none could not be.
func recordStillNeeded(res JobResult) string {
	switch {
	case res.StdoutTruncated || res.StderrTruncated:
		return "the answer's copy of the output was cut at the capture limit, and the record holds the complete text"
	case res.ArtifactError != "":
		return "its artifacts were not delivered, so its record stays beside the copy artifacts retry fetches"
	case res.GuestDir != "":
		return "its guest directory " + res.GuestDir + " was not removed, so its record stays beside it until gc removes both"
	}
	return ""
}
