// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// -- WSL-167: the default base's canonical name, and an existing one kept -----

// defaultBaseCase is a state directory and a host the resolution asks, both the
// case's own. ⚠ The registry and the marker are answered here, so no case reads
// this machine's WSL.
type defaultBaseCase struct {
	home   string
	disks  map[string]string
	marked map[string]bool
	// unread is a marker read that fails without an answer, by name.
	unread map[string]error
	asked  []string
}

func newDefaultBaseCase(t *testing.T) *defaultBaseCase {
	t.Helper()
	c := &defaultBaseCase{home: t.TempDir(), disks: map[string]string{}, marked: map[string]bool{}, unread: map[string]error{}}
	t.Setenv("WSL_TOOLKIT_HOME", c.home)
	t.Setenv(InstanceEnv, "")
	t.Chdir(t.TempDir())
	previousHost, previousSelection, previousExplicit := defaultBaseHost, SelectedInstance, ExplicitConfigPath
	t.Cleanup(func() {
		defaultBaseHost, SelectedInstance, ExplicitConfigPath = previousHost, previousSelection, previousExplicit
	})
	SelectedInstance, ExplicitConfigPath = Instance{}, ""
	defaultBaseHost.disks = func() (map[string]string, error) { return c.disks, nil }
	defaultBaseHost.marked = func(_ context.Context, name string) error {
		c.asked = append(c.asked, name)
		if err := c.unread[name]; err != nil {
			return err
		}
		if c.marked[name] {
			return nil
		}
		return ErrNoIdentity
	}
	return c
}

func (c *defaultBaseCase) record(t *testing.T, name string) {
	t.Helper()
	b := &Base{cfg: Config{Base: BaseConfig{Name: name, Image: DefaultBaseImage, User: DefaultBaseUser}}, home: c.home}
	if err := b.writeRecord(); err != nil {
		t.Fatal(err)
	}
}

func (c *defaultBaseCase) load(t *testing.T) Config {
	t.Helper()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("the default configuration did not load: %v", err)
	}
	return cfg
}

// ⭐ A FRESH HOST GETS THE CANONICAL NAME, and says where it came from.
func TestAFreshDefaultBaseIsNamedJobs(t *testing.T) {
	c := newDefaultBaseCase(t)
	cfg := c.load(t)
	if cfg.Base.Name != "wsl-toolkit-jobs" {
		t.Fatalf("a fresh default base is named %q, want wsl-toolkit-jobs", cfg.Base.Name)
	}
	st, err := (&Base{cfg: cfg, home: c.home}).configuredState()
	if err != nil {
		t.Fatal(err)
	}
	if st.Naming == nil || st.Naming.Legacy || st.Naming.From != NameFromCanonical || st.Naming.Canonical != "wsl-toolkit-jobs" {
		t.Fatalf("a fresh base reports its naming as %+v", st.Naming)
	}
}

// ⛔ THE HOST THIS WAS BUILT ON: a base recorded as `wsl-toolkit` keeps that name,
// with no marker read, and status says how to move.
func TestAnExistingDefaultBaseKeepsItsRecordedName(t *testing.T) {
	c := newDefaultBaseCase(t)
	c.record(t, "wsl-toolkit")
	c.disks["wsl-toolkit"] = filepath.Join(c.home, "base")
	cfg := c.load(t)
	if cfg.Base.Name != "wsl-toolkit" {
		t.Fatalf("a base recorded as wsl-toolkit resolved to %q", cfg.Base.Name)
	}
	if len(c.asked) != 0 {
		t.Fatalf("a recorded base had its marker read, which starts the distribution: %v", c.asked)
	}
	st, err := (&Base{cfg: cfg, home: c.home}).configuredState()
	if err != nil {
		t.Fatal(err)
	}
	n := st.Naming
	if n == nil || !n.Legacy || n.From != NameFromRecord ||
		!strings.Contains(n.Move, "base remove --yes") || !strings.Contains(n.Move, "base ensure") ||
		!strings.Contains(n.Move, "wsl-toolkit-jobs") {
		t.Fatalf("a base under the older name reports %+v; want legacy, from the record, and the move", n)
	}
	for _, p := range st.Problems {
		if strings.Contains(p, "older name") {
			t.Fatalf("the older name was made a problem, which fails base status: %q", p)
		}
	}
}

// ⭐ A BASE BUILT BEFORE ITS RECORD: registered as `wsl-toolkit`, its disk in this
// state directory, and the guest's marker. Without the marker it is not adopted.
func TestARegisteredMarkedDefaultBaseWithNoRecordIsKept(t *testing.T) {
	c := newDefaultBaseCase(t)
	c.disks["wsl-toolkit"] = filepath.Join(c.home, "base")
	c.marked["wsl-toolkit"] = true
	if cfg := c.load(t); cfg.Base.Name != "wsl-toolkit" {
		t.Fatalf("a marked wsl-toolkit with its disk here resolved to %q", cfg.Base.Name)
	}
	c.marked["wsl-toolkit"] = false
	if cfg := c.load(t); cfg.Base.Name != "wsl-toolkit-jobs" {
		t.Fatalf("an unmarked wsl-toolkit was adopted as %q", cfg.Base.Name)
	}
}

// ⛔ A SCRATCH STATE DIRECTORY DOES NOT ADOPT ANOTHER DIRECTORY'S BASE. The name
// is global to the host; the record and the disk tie a base to one directory.
func TestAScratchHomeDoesNotAdoptTheHostsOlderBase(t *testing.T) {
	c := newDefaultBaseCase(t)
	c.disks["wsl-toolkit"] = filepath.Join(t.TempDir(), "base")
	c.marked["wsl-toolkit"] = true
	if cfg := c.load(t); cfg.Base.Name != "wsl-toolkit-jobs" {
		t.Fatalf("a scratch home adopted another home's base as %q", cfg.Base.Name)
	}
	if len(c.asked) != 0 {
		t.Fatalf("another home's base had its marker read: %v", c.asked)
	}
}

// ⛔ A SCRATCH HOME DOES NOT ADOPT THE HOST'S CANONICAL BASE EITHER. Another
// directory's `wsl-toolkit-jobs` is refused by name, and never acted on.
func TestAScratchHomeDoesNotAdoptTheHostsCanonicalBase(t *testing.T) {
	c := newDefaultBaseCase(t)
	other := filepath.Join(t.TempDir(), "base")
	c.disks["wsl-toolkit-jobs"] = other
	_, err := LoadConfig()
	if !errors.Is(err, ErrDefaultBaseAmbiguous) || !strings.Contains(err.Error(), other) || !strings.Contains(err.Error(), "wsl-toolkit-jobs") {
		t.Fatalf("a scratch home beside another home's wsl-toolkit-jobs answered %v; want a refusal naming that directory", err)
	}
}

// ⛔ A BASE IN ANOTHER STATE DIRECTORY MAKES THIS ONE NO AMBIGUITY. The operator's
// recorded `wsl-toolkit` stays the default when a scratch home built a
// `wsl-toolkit-jobs`, and no marker is read.
func TestACanonicalBaseInAnotherHomeLeavesTheRecordedOneDefault(t *testing.T) {
	c := newDefaultBaseCase(t)
	c.record(t, "wsl-toolkit")
	c.disks["wsl-toolkit"] = filepath.Join(c.home, "base")
	c.disks["wsl-toolkit-jobs"] = filepath.Join(t.TempDir(), "base")
	c.marked["wsl-toolkit"] = true
	if cfg := c.load(t); cfg.Base.Name != "wsl-toolkit" || cfg.nameFrom != NameFromRecord {
		t.Fatalf("a recorded wsl-toolkit beside another home's wsl-toolkit-jobs resolved to %q from %q", cfg.Base.Name, cfg.nameFrom)
	}
	if len(c.asked) != 0 {
		t.Fatalf("a recorded base had its marker read: %v", c.asked)
	}
}

// ⛔ A MARKER READ THAT COULD NOT ASK NEVER PICKS THE OTHER NAME. With a record
// it is never asked; with none, the command is refused.
func TestAnUnreadableMarkerNeverDemotesTheOlderBase(t *testing.T) {
	c := newDefaultBaseCase(t)
	c.record(t, "wsl-toolkit")
	c.disks["wsl-toolkit"] = filepath.Join(c.home, "base")
	c.disks["wsl-toolkit-jobs"] = filepath.Join(t.TempDir(), "base")
	timedOut := fmt.Errorf("%w: wsl-toolkit, exit 1: %v", ErrIdentityUnread, context.DeadlineExceeded)
	c.unread["wsl-toolkit"] = timedOut
	if cfg := c.load(t); cfg.Base.Name != "wsl-toolkit" {
		t.Fatalf("a recorded wsl-toolkit whose marker timed out resolved to %q", cfg.Base.Name)
	}
	if err := os.Remove(filepath.Join(c.home, "base.json")); err != nil {
		t.Fatal(err)
	}
	delete(c.disks, "wsl-toolkit-jobs")
	if cfg, err := LoadConfig(); err == nil || !errors.Is(err, ErrIdentityUnread) {
		t.Fatalf("an unrecorded wsl-toolkit whose marker timed out resolved to %q, %v; want a refusal", cfg.Base.Name, err)
	}
}

// ⛔ A RECORD NAMING A BASE WHOSE DISK IS IN ANOTHER DIRECTORY IS REFUSED. An
// older version acting from a scratch home wrote such records about the
// operator's base.
func TestARecordNamingAnotherHomesBaseIsRefused(t *testing.T) {
	c := newDefaultBaseCase(t)
	c.record(t, "wsl-toolkit")
	other := filepath.Join(t.TempDir(), "base")
	c.disks["wsl-toolkit"] = other
	c.marked["wsl-toolkit"] = true
	_, err := LoadConfig()
	if !errors.Is(err, ErrDefaultBaseAmbiguous) || !strings.Contains(err.Error(), other) ||
		!strings.Contains(err.Error(), filepath.Join(c.home, "base")) {
		t.Fatalf("a record naming another home's base answered %v; want a refusal naming both directories", err)
	}
	// ⭐ AN UNREGISTERED RECORD IS A REMOVED BASE, and the next one is new.
	delete(c.disks, "wsl-toolkit")
	if cfg := c.load(t); cfg.Base.Name != "wsl-toolkit-jobs" {
		t.Fatalf("a record of a removed wsl-toolkit resolved to %q", cfg.Base.Name)
	}
}

// ⛔ A FILE NAMING THE CANONICAL NAME MEETS THE JOBS GUARD TOO.
func TestAFileNamingTheCanonicalNameBesideAJobsInstanceIsRefused(t *testing.T) {
	c := newDefaultBaseCase(t)
	if err := os.MkdirAll(filepath.Join(c.home, "instances", "jobs"), 0o700); err != nil {
		t.Fatal(err)
	}
	own, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte(`{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit-jobs"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "instance jobs") {
		t.Fatalf("a file naming wsl-toolkit-jobs beside a jobs instance answered %v", err)
	}
}

// ⛔ TWO CANDIDATES ARE REFUSED BY NAME, and an explicit base.name chooses.
func TestTwoDefaultBasesAreRefusedByName(t *testing.T) {
	c := newDefaultBaseCase(t)
	c.disks["wsl-toolkit"] = filepath.Join(c.home, "base")
	c.disks["wsl-toolkit-jobs"] = filepath.Join(c.home, "base")
	c.marked["wsl-toolkit"] = true
	_, err := LoadConfig()
	if !errors.Is(err, ErrDefaultBaseAmbiguous) || !strings.Contains(err.Error(), "wsl-toolkit,") ||
		!strings.Contains(err.Error(), "wsl-toolkit-jobs") || !strings.Contains(err.Error(), "base.name") {
		t.Fatalf("two candidate bases answered %v; want a refusal naming both and the way to choose", err)
	}
	own, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte(`{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := c.load(t)
	if cfg.Base.Name != "wsl-toolkit" || cfg.nameFrom != NameFromConfig {
		t.Fatalf("an explicit base.name did not choose: %q from %q", cfg.Base.Name, cfg.nameFrom)
	}
}

// ⛔ THE INSTANCE `jobs` WOULD SHARE THE CANONICAL NAME, so its state refuses it.
func TestAnExistingJobsInstanceRefusesTheCanonicalName(t *testing.T) {
	c := newDefaultBaseCase(t)
	if err := os.MkdirAll(filepath.Join(c.home, "instances", "jobs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "instance jobs") {
		t.Fatalf("a jobs instance beside the canonical name answered %v", err)
	}
}

// ⛔ `base` AND `jobs` ARE REFUSED FOR A NEW INSTANCE, and an existing one stays
// reachable so it can be removed.
func TestReservedInstancesAreRefusedForCreationNotUse(t *testing.T) {
	c := newDefaultBaseCase(t)
	for _, name := range []string{"base", "jobs"} {
		_, err := ResolveInstance(context.Background(), name)
		if err == nil || !strings.Contains(err.Error(), "reserved") || !strings.Contains(err.Error(), "wsl-toolkit-"+name) {
			t.Fatalf("a new instance %q answered %v; want the reservation and why", name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(c.home, "instances", "base"), 0o700); err != nil {
		t.Fatal(err)
	}
	inst, err := ResolveInstance(context.Background(), "base")
	if err != nil || inst.Distro != "wsl-toolkit-base" {
		t.Fatalf("an existing instance base is no longer reachable: %+v, %v", inst, err)
	}
}

// ⛔ A RECREATE OF A RESERVED INSTANCE IS REFUSED BEFORE IT REMOVES ANYTHING.
func TestAReservedInstanceIsNeverRebuilt(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	previous := SelectedInstance
	t.Cleanup(func() { SelectedInstance = previous })
	SelectedInstance = Instance{Name: "base", Distro: fakeBaseName, Home: b.home}
	_, err := b.EnsureWith(context.Background(), true, false)
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("a recreate of a reserved instance answered %v", err)
	}
	assertKept(t, h, b, disk)
	if err := b.create(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("a build of a reserved instance answered %v", err)
	}
}

// ⛔ ONLY `cat` SAYING THE FILE IS MISSING IS AN ABSENCE. A guest that did not
// start is a failure to ask, and reading it as an absence demoted a recorded base
// and stamped a guest nobody reached.
func TestAMarkerReadThatCouldNotAskIsNotAnAbsence(t *testing.T) {
	h := newFakeHost(t)
	_, _, _ = fakeBuiltBase(t, h, DefaultBaseImage)
	w := &Wsl{Path: h.wsl}
	if _, err := w.ReadIdentity(context.Background(), fakeBaseName); err != nil {
		t.Fatalf("a marked guest answered %v", err)
	}
	h.say(t, "identity", "unreadable")
	_, err := w.ReadIdentity(context.Background(), fakeBaseName)
	if !errors.Is(err, ErrIdentityUnread) || errors.Is(err, ErrNoIdentity) {
		t.Fatalf("a guest that did not start answered %v; want a failure to ask, not an absence", err)
	}
	if err := os.Remove(filepath.Join(h.state, "identity")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(h.state, "identity.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ReadIdentity(context.Background(), fakeBaseName); !errors.Is(err, ErrNoIdentity) {
		t.Fatalf("a guest with no marker answered %v; want an absence", err)
	}
}

// ⛔ AN ADAPTER ON THE DEFAULT BASE NAMES THE TOOL WITH NO --instance. The
// canonical name `wsl-toolkit-jobs` matched the instance glob, and the adapter
// printed `--instance jobs`, which is reserved and refused.
func TestAdaptersNameTheDefaultBaseWithNoInstance(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh to run the adapters' case with")
	}
	block := regexp.MustCompile(`(?s)case \$TK_DISTRO in\n.*?\nesac\n`)
	for _, name := range []string{"pi", "omp", "muse"} {
		script, err := adapterScript(name, "install.sh")
		if err != nil {
			t.Fatal(err)
		}
		found := block.Find(script)
		if found == nil {
			t.Fatalf("%s/install.sh has no case on TK_DISTRO", name)
		}
		for distro, want := range map[string]string{
			"wsl-toolkit-jobs":       "wsl-toolkit",
			"wsl-toolkit":            "wsl-toolkit",
			"wsl-toolkit-errand-box": "wsl-toolkit --instance errand-box",
		} {
			cmd := exec.Command(sh, "-c", string(found)+`printf '%s' "$tool"`)
			cmd.Env = append(os.Environ(), "TK_DISTRO="+distro)
			out, err := cmd.Output()
			if err != nil || string(out) != want {
				t.Fatalf("%s/install.sh on %s names %q (%v); want %q", name, distro, out, err, want)
			}
		}
	}
}
