// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/json"
	"os"
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
	// ⛔ THE ANSWER OUTLIVES THE RECORD FOR A WHILE. A second process following
	// the job wakes after the owner lets its lock go, and the record was gone by
	// then, so `wait ID` answered that how the job ended was not recorded.
	answer, err := keepEndedAnswer(home, res.ID, dir)
	if err != nil {
		res.RecordKept = "its answer could not be kept for a process that follows it: " + err.Error()
		return
	}
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
		_ = os.Remove(answer)
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
	case res.TeardownError != "":
		// ⛔ A TEARDOWN THAT FAILED LEFT THE GUEST DIRECTORY AND MAYBE THE
		// CONTAINER, and the record was removed with its lock, so gc read the
		// leftovers as a live job until its deadline. The lock tells gc the owner
		// is gone.
		return "its teardown failed (" + res.TeardownError + "), so its guest directory and container may remain, and its record stays beside them until gc removes them"
	case res.GuestDir != "":
		return "its guest directory " + res.GuestDir + " was not removed, so its record stays beside it until gc removes both"
	}
	return ""
}

// endedDirName holds the answers of ephemeral jobs whose records were removed,
// for endedKeep. WSL-121.
const endedDirName = "ended"

// endedKeep is how long an ephemeral job's answer outlives its record.
var endedKeep = 10 * time.Minute

// keepEndedAnswer copies a job's recorded answer out of its record, marked as
// removed, before the record goes, and answers where. A record with no answer
// keeps nothing and answers "". Answers older than endedKeep are removed.
func keepEndedAnswer(home, id, dir string) (string, error) {
	sweepEndedAnswers(home)
	res, ok, err := readJobResult(dir)
	if err != nil || !ok {
		return "", err
	}
	kept := res
	kept.RecordRemoved, kept.RecordKept = true, ""
	if pathEqual(kept.Transcript, dir) {
		kept.Transcript = ""
	}
	doc, err := json.MarshalIndent(struct {
		Schema string    `json:"schema"`
		Result JobResult `json:"result"`
	}{JobResultSchema, kept}, "", "  ")
	if err != nil {
		return "", err
	}
	ended := filepath.Join(home, endedDirName)
	if err := os.MkdirAll(ended, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(ended, id+".json")
	return path, writeFileAtomic(path, append(doc, '\n'), 0o600)
}

// EndedAnswer is the answer of an ephemeral job whose record was removed less
// than endedKeep ago, and false where there is none. WSL-121.
func EndedAnswer(home, id string) (JobResult, bool) {
	if home == "" || !ValidJobID(id) {
		return JobResult{}, false
	}
	path := filepath.Join(home, endedDirName, id+".json")
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > endedKeep {
		return JobResult{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return JobResult{}, false
	}
	var doc struct {
		Schema string    `json:"schema"`
		Result JobResult `json:"result"`
	}
	if json.Unmarshal(b, &doc) != nil || doc.Schema != JobResultSchema || doc.Result.ID != id {
		return JobResult{}, false
	}
	return doc.Result, true
}

// sweepEndedAnswers removes the kept answers older than endedKeep.
func sweepEndedAnswers(home string) {
	dir := filepath.Join(home, endedDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > endedKeep {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
