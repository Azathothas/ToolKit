// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// -- WSL-167: the default base's two names, and which home may change it -------

// ⛔ NO CASE READS THIS MACHINE'S REGISTRY. A developer's host has its own base
// registered, and every default load would resolve to it. A case that wants a
// registration says so.
func init() {
	readRegistrations = func() (map[string]string, error) { return map[string]string{}, nil }
}

// defaultBaseCase is a state directory and a fake registry, both the case's own.
type defaultBaseCase struct {
	home  string
	disks map[string]string
}

func newDefaultBaseCase(t *testing.T) *defaultBaseCase {
	t.Helper()
	c := &defaultBaseCase{home: t.TempDir(), disks: map[string]string{}}
	t.Setenv("WSL_TOOLKIT_HOME", c.home)
	t.Setenv(InstanceEnv, "")
	t.Chdir(t.TempDir())
	previousRead, previousSelection, previousExplicit := readRegistrations, SelectedInstance, ExplicitConfigPath
	t.Cleanup(func() {
		readRegistrations, SelectedInstance, ExplicitConfigPath = previousRead, previousSelection, previousExplicit
	})
	SelectedInstance, ExplicitConfigPath = Instance{}, ""
	readRegistrations = func() (map[string]string, error) { return c.disks, nil }
	return c
}

func (c *defaultBaseCase) load(t *testing.T) Config {
	t.Helper()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("the default configuration did not load: %v", err)
	}
	return cfg
}

// writeConfig gives the case's home a configuration file.
func (c *defaultBaseCase) writeConfig(t *testing.T, body string) {
	t.Helper()
	own, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ⭐ A FRESH HOST GETS THE CANONICAL NAME, and says where it came from.
func TestAFreshDefaultBaseIsNamedJobs(t *testing.T) {
	c := newDefaultBaseCase(t)
	cfg := c.load(t)
	if cfg.Base.Name != "wsl-toolkit-jobs" || cfg.nameFrom != NameFromCanonical {
		t.Fatalf("a fresh default base is named %q from %q, want wsl-toolkit-jobs from the canonical step", cfg.Base.Name, cfg.nameFrom)
	}
	st, err := (&Base{cfg: cfg, home: c.home}).configuredState()
	if err != nil {
		t.Fatal(err)
	}
	if st.Naming == nil || st.Naming.Legacy || st.Naming.From != NameFromCanonical || st.Naming.Canonical != "wsl-toolkit-jobs" || st.Naming.Disk != "" {
		t.Fatalf("a fresh base reports its naming as %+v", st.Naming)
	}
}

// ⭐ ONE CANDIDATE, ANY HOME: the registered name is the default's, wherever its
// disk is, and whichever state directory asks.
func TestOneRegisteredDefaultNameIsUsedFromEveryHome(t *testing.T) {
	for _, name := range []string{"wsl-toolkit", "wsl-toolkit-jobs"} {
		t.Run(name, func(t *testing.T) {
			c := newDefaultBaseCase(t)
			c.disks[name] = filepath.Join(t.TempDir(), "base")
			if cfg := c.load(t); cfg.Base.Name != name || cfg.nameFrom != NameFromRegistered {
				t.Fatalf("one registered %s resolved to %q from %q", name, cfg.Base.Name, cfg.nameFrom)
			}
			t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
			if cfg := c.load(t); cfg.Base.Name != name {
				t.Fatalf("a second home resolved one registered %s to %q", name, cfg.Base.Name)
			}
		})
	}
}

// ⛔ BOTH NAMES REGISTERED: REFUSED BY NAME, each disk named, and base.name chooses.
func TestBothDefaultNamesRegisteredAreRefusedByName(t *testing.T) {
	c := newDefaultBaseCase(t)
	legacy, canonical := filepath.Join(c.home, "base"), filepath.Join(t.TempDir(), "base")
	c.disks["wsl-toolkit"] = legacy
	c.disks["wsl-toolkit-jobs"] = canonical
	_, err := LoadConfig()
	if !errors.Is(err, ErrDefaultBaseAmbiguous) || !strings.Contains(err.Error(), "wsl-toolkit,") ||
		!strings.Contains(err.Error(), "wsl-toolkit-jobs") || !strings.Contains(err.Error(), "base.name") ||
		!strings.Contains(err.Error(), cleanBasePath(legacy)) || !strings.Contains(err.Error(), cleanBasePath(canonical)) {
		t.Fatalf("both names registered answered %v; want a refusal naming both, their disks, and base.name", err)
	}
	c.writeConfig(t, `{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit"}}`)
	if cfg := c.load(t); cfg.Base.Name != "wsl-toolkit" || cfg.nameFrom != NameFromConfig {
		t.Fatalf("an explicit base.name did not choose: %q from %q", cfg.Base.Name, cfg.nameFrom)
	}
}

// ⛔ AN INSTANCE `jobs` IS NOT A DEFAULT BASE, though it is registered as
// `wsl-toolkit-jobs`. Beside the host's `wsl-toolkit` it is no second candidate.
// Alone, the default still loads, status and doctor say whose it is, and an
// ensure is refused before it uses or builds anything. WSL-167.
func TestAnInstancesWslToolkitJobsIsNoDefaultCandidate(t *testing.T) {
	h := newFakeHost(t)
	c := newDefaultBaseCase(t)
	root := t.TempDir()
	at := filepath.Join(root, "instances", "jobs", "base")
	c.disks["wsl-toolkit"] = filepath.Join(t.TempDir(), "base")
	c.disks["wsl-toolkit-jobs"] = at
	if cfg := c.load(t); cfg.Base.Name != "wsl-toolkit" {
		t.Fatalf("a legacy base beside the jobs instance resolved to %q", cfg.Base.Name)
	}
	readRegistrations = h.registry
	h.say(t, "registered", "wsl-toolkit-jobs")
	h.say(t, "disks", "wsl-toolkit-jobs\t"+at)
	want := "$env:WSL_TOOLKIT_HOME='" + root + "'; wsl-toolkit --instance jobs base remove --yes"
	for _, file := range []string{"", `{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit-jobs"}}`} {
		if file != "" {
			c.writeConfig(t, file)
		}
		b, _ := c.from(t, h)
		st, err := b.Status(context.Background(), false)
		if err != nil || st.Naming == nil || st.Naming.Instance != "jobs" || st.Naming.Owner {
			t.Fatalf("status beside a registered jobs instance read %+v, %v; want the instance named", st.Naming, err)
		}
		if _, err := b.EnsureWith(context.Background(), false, false); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("an ensure beside a registered jobs instance answered %v; want a refusal naming %s", err, want)
		}
	}
	if i := callAt(h.calls(t), "wsl --import"); i >= 0 {
		t.Fatalf("the default built beside a registered jobs instance: %v", h.calls(t))
	}
	if strings.Contains(h.guestScripts(t), "provision-complete") {
		t.Fatal("the default provisioned the jobs instance's distribution")
	}
	f := heldByInstance(WslFacts{BaseName: "wsl-toolkit-jobs", BaseExists: true})
	if f.BaseExists || !strings.Contains(f.Held, want) || len(f.Advice) == 0 || !strings.Contains(f.Advice[0], want) {
		t.Fatalf("doctor surveyed the jobs instance as %+v; want it said to be the instance's, not the default base", f)
	}
}

// ⭐ A DIRECTORY IS NOT A REGISTRATION. An instance `jobs` with state and no
// distribution leaves the canonical name free.
func TestAJobsInstanceDirectoryAloneDoesNotRefuseTheCanonicalName(t *testing.T) {
	c := newDefaultBaseCase(t)
	if err := os.MkdirAll(filepath.Join(c.home, "instances", "jobs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if cfg := c.load(t); cfg.Base.Name != "wsl-toolkit-jobs" {
		t.Fatalf("an unregistered jobs instance changed the default to %q", cfg.Base.Name)
	}
}

// -- the fake host: a base another state directory owns ------------------------

// ownedElsewhere is a host whose one default base, under name, has its disk, its
// marker and its SSH way in under another state directory, and a scratch home c.
func ownedElsewhere(t *testing.T, name string) (*fakeHost, *defaultBaseCase, string, string) {
	t.Helper()
	h := newFakeHost(t)
	c := newDefaultBaseCase(t)
	t.Setenv(SSHDirEnv, t.TempDir())
	t.Setenv(BinDirEnv, t.TempDir())
	readRegistrations = h.registry
	owner := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(owner, 0o700); err != nil {
		t.Fatal(err)
	}
	disk := filepath.Join(owner, "ext4.vhdx")
	if err := os.WriteFile(disk, []byte("the owner's base"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.say(t, "registered", name)
	h.say(t, "disks", name+"\t"+owner)
	marker, err := json.Marshal(Identity{
		Schema: IdentitySchema, Name: name, Image: DefaultBaseImage, User: DefaultBaseUser, Built: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	h.say(t, "identity.json", string(marker))
	return h, c, owner, disk
}

// from is the base the scratch home acts on, as the commands build it.
func (c *defaultBaseCase) from(t *testing.T, h *fakeHost) (*Base, *sayings) {
	t.Helper()
	s := &sayings{}
	return &Base{cfg: c.load(t), home: c.home, wsl: &Wsl{Path: h.wsl}, log: s.log, disks: h.registry}, s
}

// assertUntouched fails a case that unregistered, imported, exported, stamped or
// recorded anything.
func assertUntouched(t *testing.T, h *fakeHost, c *defaultBaseCase, disk string) {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(h.state, "calls"))
	for _, call := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(call, "wsl --unregister") || strings.HasPrefix(call, "wsl --import") || strings.HasPrefix(call, "podman") {
			t.Fatalf("a home that does not own the base changed it: %q in %s", call, data)
		}
	}
	if strings.Contains(h.guestScripts(t), "provision-complete") {
		t.Fatal("a home that does not own the base re-provisioned it")
	}
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("the owner's disk is gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.home, "base.json")); !os.IsNotExist(err) {
		t.Fatalf("a home that does not own the base wrote a base.json about it: %v", err)
	}
}

// ⛔ THE DEFECT THE NAME RULE IS FOR: A SECOND HOME BUILT A SECOND BASE. A home
// with no record uses the host's one base by its name, imports nothing, writes no
// record, and leaves the owner's SSH way in. WSL-167.
func TestAHomeThatDoesNotOwnTheBaseUsesItAndChangesNothing(t *testing.T) {
	for _, name := range []string{"wsl-toolkit", "wsl-toolkit-jobs"} {
		t.Run(name, func(t *testing.T) {
			h, c, owner, disk := ownedElsewhere(t, name)
			b, s := c.from(t, h)
			if b.cfg.Base.Name != name {
				t.Fatalf("a scratch home beside the host's %s resolved to %q", name, b.cfg.Base.Name)
			}
			st, err := b.Status(context.Background(), false)
			if err != nil || !st.Registered || st.Naming == nil || st.Naming.Owner || st.Naming.Disk != cleanBasePath(owner) {
				t.Fatalf("status from a scratch home reads %+v, %v; want registered, its disk named, and not the owner", st.Naming, err)
			}
			sshConfig := filepath.Join(os.Getenv(SSHDirEnv), "config")
			owners := withMarkedBlock(nil, name, []string{"Host " + name})
			if err := os.WriteFile(sshConfig, owners, 0o600); err != nil {
				t.Fatal(err)
			}
			h.stageGuest(t, "verify=ok", "binfmt=ok")
			if _, err := b.EnsureWith(context.Background(), false, false); err != nil {
				t.Fatalf("an ensure from a scratch home over the host's healthy base failed: %v", err)
			}
			if got, err := os.ReadFile(sshConfig); err != nil || string(got) != string(owners) {
				t.Fatalf("an ensure from a scratch home changed the owner's SSH config to %q, %v", got, err)
			}
			if !s.said("the adapters are left as they are") {
				t.Fatal("a scratch home's ensure skipped the adapters without saying so")
			}
			assertUntouched(t, h, c, disk)
		})
	}
}

// ⭐ THE OWNER IS THE OWNER, and status says so.
func TestTheHomeHoldingTheDiskOwnsTheBase(t *testing.T) {
	h, c, owner, _ := ownedElsewhere(t, "wsl-toolkit")
	t.Setenv("WSL_TOOLKIT_HOME", filepath.Dir(owner))
	c.home = filepath.Dir(owner)
	b, _ := c.from(t, h)
	st, err := b.Status(context.Background(), false)
	if err != nil || st.Naming == nil || !st.Naming.Owner || !st.Naming.Legacy || !strings.Contains(st.Naming.Move, "base remove --yes") {
		t.Fatalf("the owner's status reads %+v, %v; want the owner, under the older name, with the move", st.Naming, err)
	}
}

// ⛔ A HOME THAT DOES NOT OWN THE BASE NEVER REMOVES IT, and is told where to run
// the removal from. WSL-167.
func TestARemovalFromANonOwnerIsRefused(t *testing.T) {
	h, c, owner, disk := ownedElsewhere(t, "wsl-toolkit")
	b, _ := c.from(t, h)
	err := b.Remove(context.Background())
	want := "$env:WSL_TOOLKIT_HOME='" + filepath.Dir(owner) + "'; wsl-toolkit base remove --yes"
	if !errors.Is(err, ErrNotOwner) || !strings.Contains(err.Error(), "wsl-toolkit was NOT removed") ||
		!strings.Contains(err.Error(), cleanBasePath(owner)) || !strings.Contains(err.Error(), want) {
		t.Fatalf("a removal from a non-owner answered %v; want the refusal naming %s", err, want)
	}
	assertUntouched(t, h, c, disk)
}

// ⛔ A HOME THAT DOES NOT OWN THE BASE NEVER REBUILDS IT: not by recreate, and not
// by a preset switch. Refused before the export. WSL-167.
func TestARebuildFromANonOwnerIsRefused(t *testing.T) {
	h, c, owner, disk := ownedElsewhere(t, "wsl-toolkit")
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	b, _ := c.from(t, h)
	want := "$env:WSL_TOOLKIT_HOME='" + filepath.Dir(owner) + "'; wsl-toolkit base recreate"
	_, err := b.EnsureWith(context.Background(), true, false)
	if !errors.Is(err, ErrNotOwner) || !strings.Contains(err.Error(), "wsl-toolkit was NOT rebuilt") || !strings.Contains(err.Error(), want) {
		t.Fatalf("a recreate from a non-owner answered %v; want the refusal naming %s", err, want)
	}
	b.cfg.Base.Image = "docker.io/library/debian:trixie"
	if _, err := b.EnsureSwitch(context.Background(), false); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("a preset switch from a non-owner answered %v", err)
	}
	assertUntouched(t, h, c, disk)
}

// ⛔ A HOME THAT DOES NOT OWN THE BASE NEVER RE-PROVISIONS IT. A base that does not
// verify was re-provisioned from the scratch configuration, and restarted under
// every job. WSL-167.
func TestARepairInPlaceFromANonOwnerIsRefused(t *testing.T) {
	h, c, _, disk := ownedElsewhere(t, "wsl-toolkit")
	h.stageGuest(t, "binfmt=ok")
	b, _ := c.from(t, h)
	_, err := b.EnsureWith(context.Background(), false, false)
	if !errors.Is(err, ErrNotOwner) || !strings.Contains(err.Error(), "wsl-toolkit was NOT re-provisioned") {
		t.Fatalf("a re-provision from a non-owner answered %v; want the refusal", err)
	}
	assertUntouched(t, h, c, disk)
}

// ⛔ --repair FROM A HOME THAT DOES NOT OWN THE BASE IS REFUSED UP FRONT, over a
// healthy base and a failing one. WSL-167.
func TestRepairFromANonOwnerIsRefusedUpFront(t *testing.T) {
	for _, guest := range [][]string{{"verify=ok", "binfmt=ok"}, {"binfmt=ok"}} {
		t.Run(strings.Join(guest, ","), func(t *testing.T) {
			h, c, _, disk := ownedElsewhere(t, "wsl-toolkit")
			h.stageGuest(t, guest...)
			b, _ := c.from(t, h)
			_, err := b.EnsureWith(context.Background(), false, true)
			if !errors.Is(err, ErrNotOwner) || !strings.Contains(err.Error(), "wsl-toolkit was NOT repaired") ||
				!strings.Contains(err.Error(), "base ensure --repair") {
				t.Fatalf("--repair from a non-owner answered %v; want the refusal", err)
			}
			if strings.Contains(h.guestScripts(t), "TK_M1") {
				t.Fatal("--repair from a non-owner asked the guest before it refused")
			}
			assertUntouched(t, h, c, disk)
		})
	}
}

// ⛔ A HOME THAT DOES NOT OWN THE BASE NEVER STAMPS IT. A marker written from a
// scratch home's record became the owner's truth. WSL-167.
func TestAStampFromANonOwnerIsRefused(t *testing.T) {
	h, c, _, disk := ownedElsewhere(t, "wsl-toolkit")
	if err := os.Remove(filepath.Join(h.state, "identity.json")); err != nil {
		t.Fatal(err)
	}
	h.stageGuest(t, "verify=ok", "binfmt=ok")
	b, _ := c.from(t, h)
	_, err := b.EnsureWith(context.Background(), false, false)
	if !errors.Is(err, ErrNotOwner) || !strings.Contains(err.Error(), "wsl-toolkit was NOT stamped") {
		t.Fatalf("an unmarked base from a non-owner answered %v; want the refusal", err)
	}
	assertUntouched(t, h, c, disk)
}

// ⛔ A RECORD IS WRITTEN ONLY BY THE OWNER. writeRecord from a scratch home wrote
// nothing, and from the owner it writes. WSL-167.
func TestARecordIsWrittenOnlyByTheOwner(t *testing.T) {
	h, c, owner, _ := ownedElsewhere(t, "wsl-toolkit")
	b, _ := c.from(t, h)
	if err := b.writeRecord(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(c.home, "base.json")); !os.IsNotExist(err) {
		t.Fatalf("a non-owner wrote a record: %v", err)
	}
	b.home = filepath.Dir(owner)
	if err := b.writeRecord(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(b.home, "base.json")); err != nil {
		t.Fatalf("the owner wrote no record: %v", err)
	}
}

// ⛔ CLOSED: A REGISTRY THAT CANNOT BE READ, OR A REGISTRATION WITH NO DISK
// FOLDER, PROVES NO OWNER. WSL-167.
func TestAnUnprovenOwnerIsRefused(t *testing.T) {
	for _, tc := range []struct {
		why   string
		disks func() (map[string]string, error)
	}{
		{"could not be read", func() (map[string]string, error) { return nil, errors.New("access denied") }},
		{"names no disk folder", func() (map[string]string, error) { return map[string]string{fakeBaseName: ""}, nil }},
	} {
		t.Run(tc.why, func(t *testing.T) {
			h := newFakeHost(t)
			b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
			b.disks = tc.disks
			if err := b.Remove(context.Background()); !errors.Is(err, ErrNotOwner) || !strings.Contains(err.Error(), tc.why) {
				t.Fatalf("a removal whose owner is not proven answered %v; want the refusal saying it %s", err, tc.why)
			}
			h.engine(t, true)
			if _, err := b.EnsureWith(context.Background(), true, false); !errors.Is(err, ErrNotOwner) {
				t.Fatalf("a recreate whose owner is not proven answered %v", err)
			}
			assertKept(t, h, b, disk)
		})
	}
}

// ⭐ THE OWNER THROUGH A JUNCTION IS THE OWNER. WSL records the folder's real
// path, and the home was named by a link to it. WSL-167.
func TestAnOwnerReachedThroughALinkIsTheOwner(t *testing.T) {
	real, link := filepath.Join(t.TempDir(), "real"), filepath.Join(t.TempDir(), "link")
	if err := os.MkdirAll(filepath.Join(real, "base"), 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if out, err := exec.Command(os.Getenv("ComSpec"), "/c", "mklink", "/J", link, real).CombinedOutput(); err != nil {
			t.Fatalf("no junction could be made: %v: %s", err, out)
		}
	} else if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	h := newFakeHost(t)
	b, _, _ := fakeBuiltBase(t, h, DefaultBaseImage)
	t.Setenv("WSL_TOOLKIT_HOME", link)
	b.home = link
	h.say(t, "disks", fakeBaseName+"\t"+filepath.Join(real, "base"))
	if err := b.assertOwner("removed", "base remove --yes"); err != nil {
		t.Fatalf("the owner through a link was refused its own removal: %v", err)
	}
	// ⚠ THE REMOVAL ITSELF, up to the unregister. Deleting a folder through a
	// link is RemoveInside's own refusal, and not this rule's.
	_ = b.Remove(context.Background())
	if callAt(h.calls(t), "wsl --unregister "+fakeBaseName) < 0 {
		t.Fatalf("the owner's removal unregistered nothing: %v", h.calls(t))
	}
}

// ⛔ THE LEFTOVER GUARD SEES THROUGH A LINK TOO. A home named by a junction held a
// registered disk, and a string compare called it a leftover to clear. WSL-177.
func TestTheLeftoverGuardSeesThroughALink(t *testing.T) {
	real, link := filepath.Join(t.TempDir(), "real"), filepath.Join(t.TempDir(), "link")
	if err := os.MkdirAll(filepath.Join(real, "base"), 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if out, err := exec.Command(os.Getenv("ComSpec"), "/c", "mklink", "/J", link, real).CombinedOutput(); err != nil {
			t.Fatalf("no junction could be made: %v: %s", err, out)
		}
	} else if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	disk := filepath.Join(real, "base", "ext4.vhdx")
	if err := os.WriteFile(disk, []byte("a registered base's disk"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := newFakeHost(t)
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	t.Setenv("WSL_TOOLKIT_HOME", link)
	h.say(t, "registered", fakeBaseName)
	h.say(t, "disks", fakeBaseName+"\t"+filepath.Join(real, "base"))
	cfg := DefaultConfig()
	cfg.Base.Name = "wsl-toolkit-wsl170"
	b := &Base{cfg: cfg, home: link, wsl: &Wsl{Path: h.wsl}, log: func(string) {}, disks: h.registry}
	if err := b.create(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "was NOT deleted") {
		t.Fatalf("a build over a registered disk reached through a link answered %v; want the refusal", err)
	}
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("the registered disk was moved or deleted: %v", err)
	}
}

// ⛔ THE STEP CROSSES THE HELPER'S WIRE. Only the client's file makes a name the
// configuration's, and a default name the rule no longer gives is not. WSL-167.
func TestTheHelperReadsWhichStepChoseTheName(t *testing.T) {
	_ = newFakeHost(t)
	c := newDefaultBaseCase(t)
	c.disks["wsl-toolkit"] = filepath.Join(t.TempDir(), "base")
	h := &HelperServer{log: func(string) {}, runners: map[string]*Runner{}}
	for _, tc := range []struct{ name, sent, want string }{
		{"wsl-toolkit", "", NameFromRegistered},
		{"wsl-toolkit", NameFromRegistered, NameFromRegistered},
		{"wsl-toolkit-jobs", NameFromCanonical, NameFromRegistered},
		{"wsl-toolkit-jobs", "", NameFromRegistered},
		{"wsl-toolkit-jobs", NameFromConfig, NameFromConfig},
	} {
		cfg := DefaultConfig()
		cfg.Base.Name = tc.name
		// ⚠ THE RUNNER ITSELF NEEDS wsl.exe, which a Linux host has not. The step is
		// read before the runner is made, and is what this case asks about.
		_, effective, _ := h.runnerFor(&cfg, tc.sent, false)
		if effective.nameFrom != tc.want {
			t.Fatalf("the helper read %s, sent as %q, as chosen by %q; want %q", tc.name, tc.sent, effective.nameFrom, tc.want)
		}
	}
}

// ⛔ A STALE NAME OFF THE HELPER'S WIRE IS REFUSED AT THE BUILD. A client loaded
// `wsl-toolkit`, the owner removed it, and the helper imported a second
// `wsl-toolkit` into its own home. WSL-167.
func TestAStaleNameOffTheHelpersWireIsRefusedAtTheBuild(t *testing.T) {
	h, c, _, _ := ownedElsewhere(t, "wsl-toolkit")
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	b, _ := c.from(t, h)
	h.say(t, "registered", "")
	h.say(t, "disks", "")
	b.cfg.nameFrom = helperNameFrom(b.cfg, NameFromRegistered)
	_, err := b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), "the default base is now wsl-toolkit-jobs") || !strings.Contains(err.Error(), "Run it again") {
		t.Fatalf("a stale name off the wire answered %v; want the refusal", err)
	}
	if i := callAt(h.calls(t), "wsl --import"); i >= 0 {
		t.Fatalf("a stale name off the wire was imported: %v", h.calls(t))
	}
}

// ⛔ A HELPER'S GUEST CHANGES NOTHING ONLY THE OWNER MAY. The helper's home owns
// the base, and the caller that sent the request does not: no rebuild, no
// repair, no re-provision, and the owner's adapters stay. WSL-167.
func TestAHelperGuestChangesNothingOnlyTheOwnerMay(t *testing.T) {
	for _, tc := range []struct {
		why           string
		guest         []string
		force, repair bool
		refused       string
	}{
		{"recreate", []string{"verify=ok", "binfmt=ok"}, true, false, "NOT rebuilt"},
		{"repair", []string{"verify=ok", "binfmt=ok"}, false, true, "NOT repaired"},
		{"re-provision", []string{"binfmt=ok"}, false, false, "NOT re-provisioned"},
		{"adapters", []string{"verify=ok", "binfmt=ok"}, false, false, ""},
	} {
		t.Run(tc.why, func(t *testing.T) {
			h := newFakeHost(t)
			b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
			if !b.owns() {
				t.Fatal("the helper's own home does not own the base it built")
			}
			h.engine(t, true)
			roomyVolume(t, 1<<40)
			sshConfig := filepath.Join(os.Getenv(SSHDirEnv), "config")
			owners := withMarkedBlock(nil, fakeBaseName, []string{"Host " + fakeBaseName})
			if err := os.WriteFile(sshConfig, owners, 0o600); err != nil {
				t.Fatal(err)
			}
			h.stageGuest(t, tc.guest...)
			b.guest = true
			_, err := b.EnsureWith(context.Background(), tc.force, tc.repair)
			if tc.refused != "" && (!errors.Is(err, ErrNotOwner) || !strings.Contains(err.Error(), tc.refused)) {
				t.Fatalf("a guest's %s answered %v; want the refusal", tc.why, err)
			}
			if tc.refused == "" && (err != nil || !s.said("the adapters are left as they are")) {
				t.Fatalf("a guest's ensure answered %v, and skipped the adapters without saying so", err)
			}
			if got, err := os.ReadFile(sshConfig); err != nil || string(got) != string(owners) {
				t.Fatalf("a guest's %s changed the owner's SSH config to %q, %v", tc.why, got, err)
			}
			if strings.Contains(h.guestScripts(t), "provision-complete") {
				t.Fatalf("a guest's %s re-provisioned the base", tc.why)
			}
			assertKept(t, h, b, disk)
		})
	}
}

// ⛔ THE HELPER'S BASE RUNS AS THE CALLER'S GUEST, and a runner cached for the
// owner is never lent to one. WSL-167.
func TestAHelpersRunnerForAGuestIsAGuests(t *testing.T) {
	_ = newFakeHost(t)
	_ = newDefaultBaseCase(t)
	cfg := DefaultConfig()
	cfg.Base.Name = "wsl-toolkit-jobs"
	h := &HelperServer{log: func(string) {}, runners: map[string]*Runner{}}
	owners := &Runner{base: &Base{cfg: cfg}}
	// ⚠ THE KEY A CACHE WITHOUT THE STEP OR THE CALLER'S OWNERSHIP USED.
	h.runners[cfg.Fingerprint()] = owners
	r, _, err := h.runnerFor(&cfg, "", true)
	if r == owners {
		t.Fatal("the owner's cached runner was lent to a guest")
	}
	if runtime.GOOS != "windows" {
		// ⚠ A Linux host has no wsl.exe, so no runner is made at all.
		return
	}
	if err != nil || r == nil || !r.base.guest {
		t.Fatalf("a guest's runner is %+v, %v; want a guest's base", r, err)
	}
}

// ⛔ THE CLIENT SAYS WHETHER IT OWNS THE BASE, and the helper is told. A scratch
// home is a guest, the owner is not, and nothing registered is nothing to
// guard. WSL-167.
func TestTheClientTellsTheHelperItIsAGuest(t *testing.T) {
	h, c, owner, _ := ownedElsewhere(t, "wsl-toolkit")
	cfg := c.load(t)
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schema":"` + HelperSchema + `","state":{}}`))
	}))
	defer srv.Close()
	client := &HelperClient{endpoint: HelperEndpoint{Address: strings.TrimPrefix(srv.URL, "http://"), Token: "t"}, http: srv.Client(), cfg: cfg}
	_, _ = client.BaseEnsureWith(context.Background(), false, false)
	if body["guest"] != true || body["name_from"] != NameFromRegistered {
		t.Fatalf("a scratch home's ensure crossed the wire as %v; want a guest, with the step", body)
	}
	t.Setenv("WSL_TOOLKIT_HOME", filepath.Dir(owner))
	if helperGuest(cfg) {
		t.Fatal("the owner was sent to the helper as a guest")
	}
	h.say(t, "registered", "")
	h.say(t, "disks", "")
	t.Setenv("WSL_TOOLKIT_HOME", c.home)
	if helperGuest(cfg) {
		t.Fatal("a home with nothing registered was sent as a guest, and the build would install no adapter")
	}
}

// ⛔ A PROCESS WINDOWS REDIRECTS INTO A PACKAGE'S COPY OF THE STATE DIRECTORY IS
// REFUSED THE CHANGES. Its home is the real one by name, so the path says owner,
// and its writes land in the copy. WSL-167, WSL-118.
func TestARedirectedProcessIsRefusedTheChanges(t *testing.T) {
	h := newFakeHost(t)
	local := os.Getenv("LOCALAPPDATA")
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	home := filepath.Join(local, "wsl-toolkit")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WSL_TOOLKIT_HOME", home)
	b.home = home
	if err := os.MkdirAll(b.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	disk = filepath.Join(b.Dir(), "ext4.vhdx")
	if err := os.WriteFile(disk, []byte("the disk of a working base"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.say(t, "disks", fakeBaseName+"\t"+b.Dir())
	if err := b.writeRecord(); err != nil {
		t.Fatal(err)
	}
	copyDir := filepath.Join(local, "Packages", "Claude_test", "LocalCache", "Local", "wsl-toolkit")
	if err := os.MkdirAll(copyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(home, "base.json"), filepath.Join(copyDir, "base.json")); err != nil {
		t.Skipf("no hard link to stand in for the redirect: %v", err)
	}
	err := b.Remove(context.Background())
	if !errors.Is(err, ErrNotOwner) || !strings.Contains(err.Error(), "PowerShell") || !strings.Contains(err.Error(), copyDir) {
		t.Fatalf("a redirected removal answered %v; want the refusal naming the copy and PowerShell", err)
	}
	assertKept(t, h, b, disk)
}

// ⛔ A NEW BUILD READS THE NAME AGAIN UNDER THE LOCK. A home that loaded the
// host's `wsl-toolkit`, and reached its build after the owner removed it, imported
// a second `wsl-toolkit` into itself. WSL-167.
func TestABuildReadsTheDefaultNameAgain(t *testing.T) {
	h, c, _, _ := ownedElsewhere(t, "wsl-toolkit")
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	b, _ := c.from(t, h)
	h.say(t, "registered", "")
	h.say(t, "disks", "")
	_, err := b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), "the default base is now wsl-toolkit-jobs") || !strings.Contains(err.Error(), "Run it again") {
		t.Fatalf("a build under a name read before the owner removed it answered %v; want the refusal", err)
	}
	if i := callAt(h.calls(t), "wsl --import"); i >= 0 {
		t.Fatalf("a build under a stale name imported: %v", h.calls(t))
	}
}

// ⭐ NEITHER NAME REGISTERED: THE CANONICAL NAME IS BUILT, in this home.
func TestAFreshHostBuildsWslToolkitJobs(t *testing.T) {
	h := newFakeHost(t)
	c := newDefaultBaseCase(t)
	readRegistrations = h.registry
	h.engine(t, true)
	roomyVolume(t, 1<<40)
	b, _ := c.from(t, h)
	_, _ = b.EnsureWith(context.Background(), false, false)
	if callAt(h.calls(t), "wsl --import wsl-toolkit-jobs "+b.Dir()) < 0 {
		t.Fatalf("a fresh host did not build wsl-toolkit-jobs in this home: %v", h.calls(t))
	}
}

// ⭐ THE COMMAND A REFUSAL NAMES, in each of its forms.
func TestTheOwnerAdviceNamesTheCommandToRun(t *testing.T) {
	_ = newFakeHost(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	home, err := defaultHome()
	if err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	for at, want := range map[string]string{
		filepath.Join(home, "base"):                       "with WSL_TOOLKIT_HOME unset, the default state directory: wsl-toolkit base remove --yes",
		filepath.Join(home, "instances", "jobs", "base"):  "with WSL_TOOLKIT_HOME unset, the default state directory: wsl-toolkit --instance jobs base remove --yes",
		filepath.Join(other, "base"):                      "$env:WSL_TOOLKIT_HOME='" + other + "'; wsl-toolkit base remove --yes",
		filepath.Join(other, "instances", "muse", "base"): "$env:WSL_TOOLKIT_HOME='" + other + "'; wsl-toolkit --instance muse base remove --yes",
		filepath.Join(other, "moved-disk"):                "wsl --unregister wsl-toolkit",
	} {
		if got := ownerAdvice("wsl-toolkit", at, "base remove --yes"); !strings.Contains(got, want) {
			t.Fatalf("the advice for a disk in %s reads %q; want %q", at, got, want)
		}
	}
}

// ⛔ AN AMBIGUOUS DEFAULT BASE IS SURVEYED AS ONE, with both candidates, and an
// instance's `wsl-toolkit-jobs` is none of them. WSL-167.
func TestDoctorReportsAnAmbiguousDefaultBase(t *testing.T) {
	_ = newFakeHost(t)
	c := newDefaultBaseCase(t)
	legacy, canonical := filepath.Join(t.TempDir(), "base"), filepath.Join(t.TempDir(), "base")
	c.disks["wsl-toolkit"] = legacy
	c.disks["wsl-toolkit-jobs"] = canonical
	_, unresolved := LoadConfig()
	if !errors.Is(unresolved, ErrDefaultBaseAmbiguous) {
		t.Fatalf("both names loaded with %v", unresolved)
	}
	f := wslFactsFor(context.Background(), DoctorOptions{Config: DefaultConfig(), Unresolved: unresolved})
	if f.Ambiguous == "" || f.BaseName != "" || f.Candidates["wsl-toolkit"] != cleanBasePath(legacy) ||
		f.Candidates["wsl-toolkit-jobs"] != cleanBasePath(canonical) {
		t.Fatalf("doctor surveyed %+v; want the ambiguity with both names and their disks", f)
	}
	for _, a := range f.Advice {
		if strings.HasPrefix(a, "the base distribution is not registered") {
			t.Fatalf("doctor advised building a base on a host with two: %q", f.Advice)
		}
	}
	line := ambiguousLine(f.Candidates)
	if !strings.Contains(line, "wsl-toolkit (disk in "+cleanBasePath(legacy)+")") || !strings.Contains(line, "wsl-toolkit-jobs (disk in "+cleanBasePath(canonical)+")") {
		t.Fatalf("the base distro row reads %q", line)
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
// start is a failure to ask, and reading it as an absence stamped a guest nobody
// reached.
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
