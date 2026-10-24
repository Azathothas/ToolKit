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
	switch {
	case len(args) == 2 && args[0] == "--list" && args[1] == "--quiet":
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
		return write(kept)
	case len(args) >= 2 && args[0] == "--import":
		return write(append(registered, args[1]))
	case len(args) == 2 && args[0] == "--terminate":
		return 0
	case len(args) > 2 && args[0] == "-d" && args[len(args)-1] == IdentityPath && has(args[1]):
		marker, err := os.ReadFile(filepath.Join(state, "identity.json"))
		if err != nil {
			return 1
		}
		_, _ = os.Stdout.Write(marker)
		return 0
	}
	script, _ := io.ReadAll(os.Stdin)
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
	case "pull", "rm":
	case "create":
		fmt.Println("0123456789abcdef0123456789abcdef")
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
	bin := t.TempDir()
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
	b := &Base{cfg: cfg, home: home, wsl: &Wsl{Path: h.wsl}, log: s.log}
	disk := filepath.Join(b.Dir(), "ext4.vhdx")
	if err := os.MkdirAll(b.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(disk, []byte("the disk of a working base"), 0o600); err != nil {
		t.Fatal(err)
	}
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

// ⭐ The other half: an engine that answers is asked first, and then the base is
// removed and built as it always was, by that same engine.
func TestAForcedRebuildAsksTheEngineBeforeItRemoves(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
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
	if asked < 0 || removed < 0 || pulled < 0 || exported < 0 || asked > removed || removed > pulled || pulled > exported {
		t.Fatalf("want the engine asked, then the base removed, then the image pulled and exported; the calls were %v", calls)
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
// provision is still the only one there is.
func TestAFailedProvisioningWithNoEngineKeepsTheBase(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, false)
	_, err := b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), fakeBaseName+" was NOT removed") {
		t.Fatalf("a failed re-provisioning with no engine answered %v; want the refusal naming the base", err)
	}
	if !s.said("re-provisioning failed") {
		t.Fatalf("the case did not reach the rebuild it is about: %v", s.lines)
	}
	assertKept(t, h, b, disk)
}
