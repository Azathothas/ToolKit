// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ⭐ A JOB ID IS EASY TO USE. A caller holds the id a job printed, the name of
// its container, or the first few characters of either, and every command that
// takes an id reads all three through ResolveJobID. podbox carried a wrapper
// that cut `wtk-` off and padded nothing, and lost a job to `gc --job` with
// twelve characters of its id. WSL-120.
//
// ⛔ A FULL ID PASSES THROUGH UNCHANGED, and so does a word that is no form of
// an id at all. Every message a caller met for those before stays as it was,
// and only a prefix is looked up.

// MinJobIDPrefix is the shortest prefix a job is found by.
const MinJobIDPrefix = 4

// ContainerPrefix is what a job's container name puts before its id.
const ContainerPrefix = "wtk-"

// ErrJobPrefix is a prefix that names no job here, or more than one.
var ErrJobPrefix = errors.New("a job id prefix that names no single job")

// ErrJobOfAnotherInstance is a job another instance than the selected one ran.
var ErrJobOfAnotherInstance = errors.New("a job of another instance")

// ErrNoJobHere is a prefix that no job in this state directory, or in another
// instance's, starts. It is an ErrJobPrefix too. A command that goes through
// the helper hands the prefix on, because the helper's jobs are in the
// helper's state directory. WSL-120.
var ErrNoJobHere = errors.New("a job id prefix no job in this state directory starts")

// noJobHereError is a refusal that is both ErrJobPrefix and ErrNoJobHere.
type noJobHereError struct{ msg string }

func (e *noJobHereError) Error() string { return e.msg }
func (e *noJobHereError) Is(target error) bool {
	return target == ErrJobPrefix || target == ErrNoJobHere
}

// instanceRecordName holds the instance that claimed a job first. WSL-120.
const instanceRecordName = "instance"

// ResolveJobID turns what a caller passed for a job into the job's id.
//
// again builds the command that reaches the job under the instance that ran
// it, from that instance's name and the job's full id, for the refusal that
// names it.
//
// ⛔ A PREFIX IS LOOKED UP IN THIS STATE DIRECTORY'S JOBS, and nothing is
// guessed. One match is the job; none, or several, is a refusal that says
// which, so a cleanup can never land on a job the caller did not mean.
func ResolveJobID(home, raw string, again func(instance, id string) string) (string, error) {
	if raw == "" || ValidJobID(raw) {
		return raw, nil
	}
	prefix := raw
	if strings.HasPrefix(prefix, ContainerPrefix) {
		prefix = strings.TrimPrefix(prefix, ContainerPrefix)
		if ValidJobID(prefix) {
			return prefix, nil
		}
	}
	if !isJobIDPrefix(prefix) {
		return raw, nil
	}
	matches, err := jobsStartingWith(filepath.Join(home, "jobs"), prefix)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		matches = openJobsStartingWith(home, prefix)
	}
	switch len(matches) {
	case 0:
		return "", noJobStartsWith(raw, prefix, again)
	case 1:
		id := matches[0]
		// ⛔ THE JOB'S OWN RECORD DECIDES WHOSE IT IS. A state directory reached
		// under another selection holds jobs that ran in another base, and a
		// command acting on one here would ask the wrong distribution.
		if ran, known := RecordedInstance(home, id); known && ran != SelectedInstance.Name {
			return "", ofAnotherInstance(id, ran, again)
		}
		return id, nil
	}
	return "", fmt.Errorf("%w: %q starts %d jobs: %s. Pass more of the id", ErrJobPrefix, raw, len(matches), strings.Join(matches, ", "))
}

// isJobIDPrefix says whether s can be the start of an id and is not a whole one.
func isJobIDPrefix(s string) bool {
	if len(s) < MinJobIDPrefix || len(s) >= 16 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

// jobsStartingWith lists the job directories under dir whose id starts with
// prefix, in order.
func jobsStartingWith(dir, prefix string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the jobs in %s could not be listed: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && ValidJobID(e.Name()) && strings.HasPrefix(e.Name(), prefix) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// openJobsStartingWith lists the jobs whose record is open in home's ledger
// and whose id starts with prefix, in order.
//
// ⛔ A JOB WHOSE HOST DIRECTORY IS GONE CAN STILL HOLD A CONTAINER. Its open
// record is what names it, and a prefix found nothing to reach it by but the
// whole id. It is asked only when no host directory matched, so a record left
// open does not make a prefix that names one directory ambiguous. WSL-120.
func openJobsStartingWith(home, prefix string) []string {
	led := &Ledger{path: filepath.Join(home, "ledger.jsonl")}
	open, err := led.Open()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, e := range open {
		if e.Kind == "job" && ValidJobID(e.ID) && strings.HasPrefix(e.ID, prefix) && !seen[e.ID] {
			seen[e.ID] = true
			out = append(out, e.ID)
		}
	}
	sort.Strings(out)
	return out
}

// noJobStartsWith is the refusal for a prefix nothing here starts.
//
// ⭐ ANOTHER INSTANCE IS ASKED BEFORE THE REFUSAL IS WRITTEN. A job started
// with --instance and named without it was answered "no such job", and the
// caller had nothing to go on. Its id is still not used here: the refusal names
// the command that reaches it where it is.
func noJobStartsWith(raw, prefix string, again func(instance, id string) string) error {
	elsewhere := jobsElsewhere(prefix)
	switch len(elsewhere) {
	case 0:
		return &noJobHereError{msg: fmt.Sprintf("%v: %q is not a job id here: no job in this state directory starts with it. %s lists the jobs it has",
			ErrJobPrefix, raw, ToolCommand("logs"))}
	case 1:
		return ofAnotherInstance(elsewhere[0].id, elsewhere[0].instance, again)
	}
	names := make([]string, 0, len(elsewhere))
	for _, j := range elsewhere {
		names = append(names, j.id+" of "+describeInstance(j.instance))
	}
	return fmt.Errorf("%w: no job here starts with %q, and %d jobs of other instances do: %s. Pass more of the id, with that instance's --instance",
		ErrJobPrefix, raw, len(elsewhere), strings.Join(names, ", "))
}

// ofAnotherInstance is the refusal for a job another instance ran, with the
// command that reaches it there.
func ofAnotherInstance(id, instance string, again func(instance, id string) string) error {
	msg := fmt.Sprintf("%v: job %s was run by %s, and this command selects %s", ErrJobOfAnotherInstance, id,
		describeInstance(instance), describeInstance(SelectedInstance.Name))
	if again != nil {
		msg += ". Run: " + again(instance, id)
	}
	return &jobInstanceError{msg: msg}
}

// jobInstanceError carries ErrJobOfAnotherInstance with its own message.
type jobInstanceError struct{ msg string }

func (e *jobInstanceError) Error() string { return e.msg }
func (e *jobInstanceError) Unwrap() error { return ErrJobOfAnotherInstance }

// describeInstance names an instance the way a refusal reads it.
func describeInstance(name string) string {
	if name == DefaultInstance {
		return "the default instance"
	}
	return "instance " + name
}

// instanceJob is one job of another instance.
type instanceJob struct{ instance, id string }

// jobsElsewhere lists the jobs of every other instance under this state root
// whose id starts with prefix.
func jobsElsewhere(prefix string) []instanceJob {
	var out []instanceJob
	for _, h := range InstanceHomes() {
		if h.Name == SelectedInstance.Name {
			continue
		}
		ids, err := jobsStartingWith(filepath.Join(h.Home, "jobs"), prefix)
		if err != nil {
			continue
		}
		for _, id := range ids {
			out = append(out, instanceJob{instance: h.Name, id: id})
		}
	}
	return out
}

// InstanceHome is one instance's name and state directory.
type InstanceHome struct {
	Name string
	Home string
}

// InstanceHomes is every instance under this process's state root, the default
// one first, then the named ones in order.
//
// ⚠ IT READS DIRECTORIES AND REGISTERS NOTHING. An instance whose base was
// never built still has a state directory once it ran anything, and that is
// what holds its jobs.
func InstanceHomes() []InstanceHome {
	root, err := StateRoot()
	if err != nil {
		return nil
	}
	out := []InstanceHome{
		{Name: DefaultInstance, Home: root},
	}
	entries, err := os.ReadDir(filepath.Join(root, "instances"))
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() && isInstanceName(e.Name()) {
			out = append(out, InstanceHome{Name: e.Name(), Home: filepath.Join(root, "instances", e.Name())})
		}
	}
	sort.SliceStable(out[1:], func(i, j int) bool { return out[1+i].Name < out[1+j].Name })
	return out
}

// RecordInstance writes the instance this process selected beside the job's
// lock, unless a claim before it already did. WSL-120.
//
// ⛔ THE FIRST CLAIM'S RECORD STANDS. A second process records an end by
// claiming a job whose owner is gone, and it must not rewrite whose job it was.
func (o *JobOwner) RecordInstance() error {
	if o == nil {
		return nil
	}
	path := filepath.Join(o.Dir, instanceRecordName)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return writeFileAtomic(path, []byte(SelectedInstance.Name+"\n"), 0o600)
}

// RecordedInstance is the instance that ran a job, where the job says: the
// record its claim wrote, or the distribution a detached session names. known
// is false for a job that says nothing, which a release before this one wrote.
func RecordedInstance(home, id string) (name string, known bool) {
	if home == "" || !ValidJobID(id) {
		return "", false
	}
	if b, err := os.ReadFile(filepath.Join(home, "jobs", id, instanceRecordName)); err == nil {
		return strings.TrimSpace(string(b)), true
	}
	if rec, ok, err := ReadSessionRecord(home, id); err == nil && ok {
		return instanceOfDistro(rec.Distro)
	}
	return "", false
}

// instanceOfDistro is the instance a base distribution belongs to.
func instanceOfDistro(distro string) (string, bool) {
	if distro == DefaultBaseName {
		return DefaultInstance, true
	}
	if name, ok := strings.CutPrefix(distro, DefaultBaseName+"-"); ok && isInstanceName(name) {
		return name, true
	}
	return "", false
}
