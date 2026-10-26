// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// -- WSL-169: a rebuild asks the host engine before it removes the base -------

// fakeHostEnv names the state directory the fake WSL and the fake engines share.
const fakeHostEnv = "TOOLKIT_FAKE_HOST"

// fakeBaseName is the base every case here builds. ⚠ An owned name, because the
// guard in front of every WSL call refuses any other, and one no real instance
// is likely to carry. The fake WSL is the only one these cases reach.
const fakeBaseName = "wsl-toolkit-wsl169"

// init makes this test binary the fake WSL or a fake host engine, chosen by the
// name it was started under, when a case has named a state directory.
//
// ⚠ AN init AND NOT A CASE. wsl.exe and podman are called with their own
// arguments, and a test binary refuses every one of them as a flag it does not
// know before any case could run.
func init() {
	state := os.Getenv(fakeHostEnv)
	if state == "" {
		return
	}
	switch role := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"); role {
	case "wsl", "podman", "docker", "wslc":
		os.Exit(fakeHostMain(state, role, os.Args[1:]))
	}
}

// fakeHostMain records the call in the journal the case reads, then answers it.
func fakeHostMain(state, role string, args []string) int {
	f, err := os.OpenFile(filepath.Join(state, "calls"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 97
	}
	_, _ = fmt.Fprintln(f, role+" "+strings.Join(args, " "))
	if err := f.Close(); err != nil {
		return 97
	}
	if role == "wsl" {
		return fakeWslMain(state, args)
	}
	return fakeEngineMain(state, role, args)
}

// fakeWslMain answers the listing, the identity marker, an import and an
// unregister from a file of registered names.
//
// ⛔ IT RUNS NO GUEST SCRIPT. Each one is read to its end and refused, so a build
// stops at its first step inside the guest, and a verification fails as a base
// whose engine does not answer fails. A case that staged an answer for a script
// gets that answer instead, and still nothing runs.
func fakeWslMain(state string, args []string) int {
	path := filepath.Join(state, "registered")
	data, _ := os.ReadFile(path)
	registered := strings.Fields(string(data))
	has := func(name string) bool {
		for _, n := range registered {
			if strings.EqualFold(n, name) {
				return true
			}
		}
		return false
	}
	write := func(names []string) int {
		if err := os.WriteFile(path, []byte(strings.Join(names, "\n")), 0o600); err != nil {
			return 96
		}
		return 0
	}
	// disks is the fake registry: each registered name and its disk's directory,
	// kept beside the listing as WSL keeps its own.
	disks := filepath.Join(state, "disks")
	keepDisks := func(drop, addName, addDir string) int {
		data, _ := os.ReadFile(disks)
		var kept []string
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if name, _, ok := strings.Cut(line, "\t"); ok && !strings.EqualFold(name, drop) {
				kept = append(kept, line)
			}
		}
		if addName != "" {
			kept = append(kept, addName+"\t"+addDir)
		}
		if err := os.WriteFile(disks, []byte(strings.Join(kept, "\n")), 0o600); err != nil {
			return 96
		}
		return 0
	}
	said := func(name string) (string, bool) {
		data, err := os.ReadFile(filepath.Join(state, name))
		return string(data), err == nil
	}
	switch {
	case len(args) == 2 && args[0] == "--list" && args[1] == "--quiet":
		// ⚠ A ROLLED-BACK WSL, WHEN THE CASE ASKS FOR ONE: the listing in
		// UTF-16, a NUL after every letter.
		if how, ok := said("listing"); ok && strings.TrimSpace(how) == "utf16" {
			for _, n := range registered {
				for _, r := range n + "\r\n" {
					_, _ = os.Stdout.Write([]byte{byte(r), 0})
				}
			}
			return 0
		}
		for _, n := range registered {
			fmt.Println(n)
		}
		return 0
	case len(args) == 3 && args[0] == "--list" && args[1] == "--running":
		return 0
	case len(args) == 2 && args[0] == "--unregister":
		var kept []string
		for _, n := range registered {
			if !strings.EqualFold(n, args[1]) {
				kept = append(kept, n)
			}
		}
		if code := write(kept); code != 0 {
			return code
		}
		return keepDisks(args[1], "", "")
	case len(args) >= 3 && args[0] == "--import":
		// ⚠ A RACE, WHEN THE CASE ASKS FOR ONE: another process registers the
		// name first, and this import fails over it.
		if how, ok := said("import"); ok && strings.TrimSpace(how) == "race" {
			_ = write(append(registered, args[1]))
			_ = keepDisks("", args[1], filepath.Join(state, "another-process"))
			fmt.Fprintln(os.Stderr, "A distribution with the supplied name already exists.")
			return 1
		}
		// ⚠ AN IMPORT KILLED AFTER THE SERVICE REGISTERED IT: the name is
		// registered with its disk where this import put it, and the call fails.
		if how, ok := said("import"); ok && strings.TrimSpace(how) == "killed" {
			_ = write(append(registered, args[1]))
			_ = keepDisks("", args[1], args[2])
			fmt.Fprintln(os.Stderr, "the operation was cancelled")
			return 1
		}
		if code := write(append(registered, args[1])); code != 0 {
			return code
		}
		return keepDisks("", args[1], args[2])
	case len(args) == 2 && args[0] == "--terminate":
		return 0
	case len(args) > 2 && args[0] == "-d" && args[len(args)-1] == IdentityPath && has(args[1]):
		// ⚠ A MARKER THE CASE SAYS CANNOT BE READ: the guest did not start, as
		// wsl.exe says it, with no word of the path. WSL-167.
		if how, ok := said("identity"); ok && strings.TrimSpace(how) == "unreadable" {
			fmt.Fprintln(os.Stderr, "The distribution failed to start.")
			return 1
		}
		marker, err := os.ReadFile(filepath.Join(state, "identity.json"))
		if err != nil {
			// What `cat` says about a missing file: the path, so the read is
			// an answer and not a failure to ask.
			fmt.Fprintf(os.Stderr, "/bin/cat: %s: No such file or directory\n", IdentityPath)
			return 1
		}
		_, _ = os.Stdout.Write(marker)
		return 0
	}
	script, _ := io.ReadAll(os.Stdin)
	// ⚠ AN ANSWERED SCRIPT: grants.sh asked which live grants another file
	// holds, when the case has written the answer to state/held. WSL-179.
	if held, err := os.ReadFile(filepath.Join(state, "held")); err == nil && strings.Contains(string(script), "TK_GRANTS_MODE='held'") {
		_, _ = os.Stdout.Write(held)
		return 0
	}
	// ⭐ TWO GUEST SCRIPTS ANSWER WHEN THE CASE GIVES THEM AN ANSWER: the
	// provisioner and the grants step, each with the lines a case wrote for it.
	for _, guest := range []struct{ last, answer string }{
		{"provision-complete", "provision.out"},
		{"grants-complete", "grants.out"},
	} {
		if !strings.Contains(string(script), guest.last) {
			continue
		}
		if out, ok := said(guest.answer); ok {
			// ⭐ KEPT, so a case can read what the guest script was handed.
			_ = os.WriteFile(filepath.Join(state, guest.answer+".script"), script, 0o600)
			fmt.Print(out)
			return 0
		}
	}
	// ⚠ A CASE MAY STAGE AN ANSWER for the verifier, binfmt.sh or uname, and
	// fakeGuestScript gives it. WSL-174, WSL-181.
	if code, ok := fakeGuestScript(state, script); ok {
		return code
	}
	fmt.Fprintln(os.Stderr, "the fake WSL runs no guest script")
	return 1
}

// fakeEngineMain is podman, docker or wslc. Only podman answers, and only while
// the case has said the engine is up; it then exports a rootfs of a few bytes.
func fakeEngineMain(state, role string, args []string) int {
	up, _ := os.ReadFile(filepath.Join(state, "engine"))
	if role != "podman" || strings.TrimSpace(string(up)) != "up" {
		switch role {
		case "podman":
			fmt.Fprintln(os.Stderr, "Cannot connect to Podman. Please verify your connection to the Linux system using `podman system connection list`")
			return 125
		case "docker":
			fmt.Fprintln(os.Stderr, "Cannot connect to the Docker daemon. Is the docker daemon running?")
			return 1
		}
		fmt.Fprintln(os.Stderr, "wslc: the container service did not answer")
		return 1
	}
	if len(args) == 0 {
		return 125
	}
	switch args[0] {
	case "info":
		fmt.Println(runtime.GOARCH)
	case "pull":
		if refused, _ := os.ReadFile(filepath.Join(state, "pull")); strings.TrimSpace(string(refused)) == "refused" {
			fmt.Fprintln(os.Stderr, "Error: initializing source: reading manifest: manifest unknown")
			return 125
		}
		// ⭐ WSL-176: silent for the span the case wrote, as podman is while one
		// layer downloads, then done.
		if quiet, err := os.ReadFile(filepath.Join(state, "pull-silent")); err == nil {
			if d, err := time.ParseDuration(strings.TrimSpace(string(quiet))); err == nil {
				time.Sleep(d)
			}
			fmt.Println("Writing manifest to image destination")
		}
	case "rm":
	case "create":
		fmt.Println("fake-export-container")
	case "export":
		fmt.Println("a rootfs of a few bytes")
	default:
		return 125
	}
	return 0
}

// fakeHost is one case's fake WSL and engines, and the journal they share.
type fakeHost struct {
	state string
	wsl   string
}

// newFakeHost puts this test binary on a PATH of its own as wsl, podman, docker
// and wslc.
//
// ⛔ ALL THREE ENGINES ARE FAKES, and none is left to the host. FindEngine
// passes over one that does not answer, so a real docker on a CI runner would
// answer for the fake podman that was told not to.
func newFakeHost(t *testing.T) *fakeHost {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// ⚠ NOT t.TempDir. On a GitHub Windows runner a fake that has just exited
	// stays locked past t.TempDir's cleanup, which then fails a case whose every
	// assertion passed. The fakes are not what is under test, so their removal is
	// tried once and a locked directory is left to the runner's temp.
	// ⛔ NEVER A WAIT. A 30-second retry here cost every case that runner's full
	// 30 seconds, and the package passed go test's 10-minute limit.
	bin, err := os.MkdirTemp("", "fakehost-bin-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(bin); err != nil {
			t.Logf("the fakes in %s are still locked and are left behind: %v", bin, err)
		}
	})
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	for _, role := range []string{"wsl", "podman", "docker", "wslc"} {
		linkOrCopyExecutable(t, self, filepath.Join(bin, role+ext))
	}
	h := &fakeHost{state: t.TempDir(), wsl: filepath.Join(bin, "wsl"+ext)}
	t.Setenv(fakeHostEnv, h.state)
	t.Setenv("PATH", bin)
	// ⚠ ResolveExecutable also looks in fixed directories under these roots,
	// so they are the case's own and only the fakes can be found there.
	for _, root := range []string{"USERPROFILE", "ProgramData", "ProgramFiles", "LOCALAPPDATA"} {
		t.Setenv(root, t.TempDir())
	}
	return h
}

func linkOrCopyExecutable(t *testing.T, from, to string) {
	t.Helper()
	if os.Link(from, to) == nil {
		return
	}
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

// engine says whether the fake podman answers.
func (h *fakeHost) engine(t *testing.T, up bool) {
	t.Helper()
	state := "down"
	if up {
		state = "up"
	}
	if err := os.WriteFile(filepath.Join(h.state, "engine"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
}

// registry is the fake WSL's registrations, as registeredDisks answers them.
func (h *fakeHost) registry() (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(h.state, "disks"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if name, dir, ok := strings.Cut(line, "\t"); ok {
			out[name] = dir
		}
	}
	return out, nil
}

// say writes what one fake answers: a guest script's lines, a refused pull, a
// raced import or the registry.
func (h *fakeHost) say(t *testing.T, name, what string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(h.state, name), []byte(what), 0o600); err != nil {
		t.Fatal(err)
	}
}

// calls is the journal: one line per fake invocation, in order.
func (h *fakeHost) calls(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.state, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n")), "\n")
}

// registered says whether the fake WSL still lists the base.
func (h *fakeHost) registered(t *testing.T) bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(h.state, "registered"))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range strings.Fields(string(data)) {
		if n == fakeBaseName {
			return true
		}
	}
	return false
}

// callAt is the position of the first call that starts with prefix, or -1.
func callAt(calls []string, prefix string) int {
	for i, c := range calls {
		if strings.HasPrefix(c, prefix) {
			return i
		}
	}
	return -1
}

// sayings collects what a base logs. ⚠ Under a lock, because a guest script's
// output reaches the logger from the copier goroutines.
type sayings struct {
	mu    sync.Mutex
	lines []string
}

func (s *sayings) log(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, line)
}

func (s *sayings) said(sub string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

// fakeBuiltBase is a base this tool built, registered with the fake WSL: its
// disk, its record and its identity marker.
func fakeBuiltBase(t *testing.T, h *fakeHost, image string) (*Base, *sayings, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	// ⚠ The removal takes this machine's half of every adapter away, so the
	// directories it would edit are the case's own.
	t.Setenv(SSHDirEnv, t.TempDir())
	t.Setenv(BinDirEnv, t.TempDir())
	cfg := DefaultConfig()
	cfg.Base.Name = fakeBaseName
	cfg.Base.Image = image
	s := &sayings{}
	b := &Base{cfg: cfg, home: home, wsl: &Wsl{Path: h.wsl}, log: s.log, disks: h.registry}
	disk := filepath.Join(b.Dir(), "ext4.vhdx")
	if err := os.MkdirAll(b.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(disk, []byte("the disk of a working base"), 0o600); err != nil {
		t.Fatal(err)
	}
	// ⚠ REGISTERED BEFORE ITS RECORD IS WRITTEN: only the owner records. WSL-167.
	h.say(t, "disks", fakeBaseName+"\t"+b.Dir())
	if err := b.writeRecord(); err != nil {
		t.Fatal(err)
	}
	marker, err := json.Marshal(Identity{
		Schema: IdentitySchema, Name: fakeBaseName, Image: image, User: cfg.Base.User, Built: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.state, "identity.json"), marker, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.state, "registered"), []byte(fakeBaseName), 0o600); err != nil {
		t.Fatal(err)
	}
	h.say(t, "disks", fakeBaseName+"\t"+b.Dir())
	return b, s, disk
}

// assertKept fails a case whose base was touched: still registered, never
// unregistered, its disk and its record where they were.
func assertKept(t *testing.T, h *fakeHost, b *Base, disk string) {
	t.Helper()
	calls := h.calls(t)
	if i := callAt(calls, "wsl --unregister"); i >= 0 {
		t.Fatalf("the base was unregistered (call %d of %v)", i, calls)
	}
	if !h.registered(t) {
		t.Fatal("the base is no longer registered")
	}
	if _, err := os.Stat(disk); err != nil {
		t.Fatalf("the base's disk is gone: %v", err)
	}
	if _, err := b.readRecord(); err != nil {
		t.Fatalf("the base's record is gone: %v", err)
	}
}

// ⛔ THE DEFECT: a forced rebuild removed the base and only then asked for a host
// engine, so an engine that did not answer left no base at all. podssh lost its
// jobs base this way. The refusal carries the engine's own reason.
func TestAForcedRebuildWithNoEngineKeepsTheBase(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, false)
	// ⚠ THE VOLUME IS THE CASE'S, or a host with little free would refuse at the
	// floor WSL-180 checks first, and never reach the engine. WSL-180.
	roomyVolume(t, 1<<40)
	_, err := b.EnsureWith(context.Background(), true, false)
	if err == nil || !strings.Contains(err.Error(), fakeBaseName+" was NOT removed") ||
		!strings.Contains(err.Error(), "podman is installed at") || !strings.Contains(err.Error(), "Cannot connect to Podman") {
		t.Fatalf("a forced rebuild with no engine answered %v; want the refusal naming the base and podman's reason", err)
	}
	assertKept(t, h, b, disk)
	if callAt(h.calls(t), "podman info") < 0 {
		t.Fatalf("the engine was never asked: %v", h.calls(t))
	}
	if s.said("rebuilding from nothing") {
		t.Fatal("the refused rebuild still said it was rebuilding")
	}
}

// ⭐ The other half: an engine that answers is asked first, the image is pulled
// and exported by that same engine, and only then is the base removed and built.
// ⛔ The removal came before the pull until WSL-169's widening: prepare, then
// commit.
func TestAForcedRebuildAsksTheEngineBeforeItRemoves(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	// ⚠ The volume is the case's, so the floor before the engine passes on any
	// host. WSL-180.
	roomyVolume(t, 1<<40)
	// ⚠ The fake WSL runs no guest script, so the build stops at its first step
	// inside the guest, or at this volume's space check before the import.
	if _, err := b.EnsureWith(context.Background(), true, false); err == nil {
		t.Fatal("a build on a fake WSL that runs no guest script reported success")
	}
	calls := h.calls(t)
	asked := callAt(calls, "podman info")
	removed := callAt(calls, "wsl --unregister "+fakeBaseName)
	pulled := callAt(calls, "podman pull --platform linux/"+runtime.GOARCH+" "+DefaultBaseImage)
	exported := callAt(calls, "podman export")
	if asked < 0 || removed < 0 || pulled < 0 || exported < 0 || asked > pulled || pulled > exported || exported > removed {
		t.Fatalf("want the engine asked, then the image pulled and exported, then the base removed; the calls were %v", calls)
	}
	asks := 0
	for _, c := range calls {
		if strings.HasPrefix(c, "podman info") {
			asks++
		}
	}
	if asks != 1 {
		t.Fatalf("the engine was asked %d times; the export should use the one asked before the removal: %v", asks, calls)
	}
	if _, err := os.Stat(disk); err == nil {
		t.Fatal("the old disk survived a rebuild that removed the base")
	}
	t.Logf("DRIVEN: the calls, in order: %s", strings.Join(calls, " | "))
}

// ⚠ An image reference the export would refuse is read before anything is
// removed, since reading it costs nothing.
func TestAForcedRebuildWithAnUnqualifiedImageKeepsTheBase(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, "archlinux:latest")
	h.engine(t, true)
	_, err := b.EnsureWith(context.Background(), true, false)
	if err == nil || !strings.Contains(err.Error(), fakeBaseName+" was NOT removed") || !strings.Contains(err.Error(), "not fully qualified") {
		t.Fatalf("a forced rebuild of an unqualified image answered %v; want the refusal naming the reference", err)
	}
	assertKept(t, h, b, disk)
	if i := callAt(h.calls(t), "podman"); i >= 0 {
		t.Fatalf("the engine was asked about an image the export refuses: %v", h.calls(t))
	}
}

// ⛔ The same removal behind a failed re-provisioning: a base that will not
// provision is still the only one there is. ⚠ Since WSL-171 nothing is rebuilt
// there at all, so the engine is not asked either.
func TestAFailedProvisioningWithNoEngineKeepsTheBase(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, false)
	_, err := b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), fakeBaseName+" failed, and it was NOT removed") {
		t.Fatalf("a failed re-provisioning with no engine answered %v; want the refusal naming the base", err)
	}
	if !s.said("re-provisioning in place") {
		t.Fatalf("the case did not reach the re-provisioning it is about: %v", s.lines)
	}
	assertKept(t, h, b, disk)
}
