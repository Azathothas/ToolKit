package toolkit

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed provision.sh
var provisionScript []byte

//go:embed verify.sh
var verifyScript []byte

// BaseSpaceFloor is what the volume must have free before an import starts.
//
// ⚠ Far above wsl-toolkit.ps1's 256 MiB floor, because this distribution will
// hold an engine and a dozen images. Running out midway leaves a partial disk
// and a registered distribution that does not work.
const BaseSpaceFloor = int64(6) << 30

// BaseState is what `base status` answers.
type BaseState struct {
	Name       string    `json:"name"`
	Image      string    `json:"image"`
	BuiltFrom  string    `json:"built_from,omitempty"`
	User       string    `json:"user"`
	Registered bool      `json:"registered"`
	Running    bool      `json:"running"`
	Healthy    bool      `json:"healthy"`
	Engine     string    `json:"engine,omitempty"`
	DiskPath   string    `json:"disk_path,omitempty"`
	DiskBytes  int64     `json:"disk_bytes,omitempty"`
	DiskKnown  bool      `json:"disk_known"`
	Created    time.Time `json:"created,omitempty"`
	Problems   []string  `json:"problems,omitempty"`
}

// Base is the owned distribution's lifecycle.
type Base struct {
	cfg  Config
	home string
	wsl  *Wsl
	log  func(string)
}

// NewBase binds the lifecycle to this host. It creates the state directory,
// because every operation on it writes.
func NewBase(cfg Config, log func(string)) (*Base, error) {
	home, err := EnsureHome()
	if err != nil {
		return nil, err
	}
	w, err := FindWsl()
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = func(string) {}
	}
	return &Base{cfg: cfg, home: home, wsl: w, log: log}, nil
}

// Dir is where the distribution's disk lives.
func (b *Base) Dir() string { return filepath.Join(b.home, "base") }

// Status reads the base without changing it. ⭐ `probe` decides whether it runs
// a container: always would cost a pull on a cold machine, never could only
// report that a distribution is registered, which is not the same as usable.
func (b *Base) Status(ctx context.Context, probe bool) (BaseState, error) {
	st := BaseState{Name: b.cfg.Base.Name, Image: b.cfg.Base.Image, User: b.cfg.Base.User}
	distros, err := b.wsl.List(ctx, b.cfg.Base.Name)
	if err != nil {
		return st, err
	}
	for _, d := range distros {
		if strings.EqualFold(d.Name, b.cfg.Base.Name) {
			st.Registered, st.Running = true, d.Running
		}
	}
	st.DiskPath = filepath.Join(b.Dir(), "ext4.vhdx")
	if size, ok := FileSize(st.DiskPath); ok {
		st.DiskBytes, st.DiskKnown = size, true
	}
	if rec, err := b.readRecord(); err == nil {
		st.Created = rec.Created
		// ⛔ The configured image and the one it was built from are two places
		// for one fact, so they are compared. They drift the moment somebody
		// runs `--preset alpine` without `--save`.
		st.BuiltFrom = rec.Image
		if rec.Image != "" && rec.Image != st.Image {
			st.Problems = append(st.Problems, fmt.Sprintf(
				"it was built from %s and the configuration says %s. Run `base ensure --preset %s` to rebuild, or `--save` the one you meant",
				rec.Image, st.Image, st.Image))
		}
	}
	if !st.Registered {
		st.Problems = append(st.Problems, "not registered. Create it with: wsl-toolkit base ensure")
		return st, nil
	}
	if !probe {
		return st, nil
	}
	engine, verifyErr := b.verify(ctx)
	st.Engine = engine
	if verifyErr != nil {
		st.Problems = append(st.Problems, verifyErr.Error())
		return st, nil
	}
	st.Healthy = true
	return st, nil
}

type baseRecord struct {
	Schema  string    `json:"schema"`
	Name    string    `json:"name"`
	Image   string    `json:"image"`
	User    string    `json:"user"`
	Created time.Time `json:"created"`
}

func (b *Base) recordPath() string { return filepath.Join(b.home, "base.json") }

func (b *Base) readRecord() (baseRecord, error) {
	var rec baseRecord
	data, err := os.ReadFile(b.recordPath())
	if err != nil {
		return rec, err
	}
	return rec, jsonUnmarshal(data, &rec)
}

func (b *Base) writeRecord() error {
	rec := baseRecord{
		Schema: "wsl-toolkit-base/1", Name: b.cfg.Base.Name,
		Image: b.cfg.Base.Image, User: b.cfg.Base.User, Created: time.Now().UTC(),
	}
	data, err := jsonMarshalIndent(rec)
	if err != nil {
		return err
	}
	return writeFileAtomic(b.recordPath(), data, 0o600)
}

// Ensure brings the base into a state where a job can run, doing the least that
// achieves it.
//
// ⭐ The recovery path as well as the create path. The base exists to be wrecked:
// a registered-but-unusable distribution is re-provisioned in place, and only
// one that will not provision is removed and rebuilt.
func (b *Base) Ensure(ctx context.Context, force bool) (BaseState, error) {
	st, err := b.Status(ctx, false)
	if err != nil {
		return st, err
	}
	if st.Registered && force {
		b.log("rebuilding from nothing, as asked")
		if err := b.Remove(ctx); err != nil {
			return st, err
		}
		st.Registered = false
	}
	if st.Registered {
		b.log(b.cfg.Base.Name + " is registered; checking whether it can run a container")
		if engine, err := b.verify(ctx); err == nil {
			b.log("the engine answers: " + engine)
			// ⛔ Refreshed on every path that leaves a usable base. A record
			// written once describes the first build forever.
			if err := b.writeRecord(); err != nil {
				return st, err
			}
			return b.Status(ctx, true)
		} else {
			b.log("it cannot: " + err.Error())
			b.log("re-provisioning in place")
		}
		if err := b.provision(ctx); err != nil {
			b.log("re-provisioning failed: " + err.Error())
			b.log("removing and rebuilding")
			if err := b.Remove(ctx); err != nil {
				return st, err
			}
		} else {
			if engine, err := b.verify(ctx); err != nil {
				return st, fmt.Errorf("re-provisioned and it still cannot run a container: %w", err)
			} else {
				b.log("the engine answers: " + engine)
			}
			if err := b.writeRecord(); err != nil {
				return st, err
			}
			return b.Status(ctx, true)
		}
	}
	if err := b.create(ctx); err != nil {
		return st, err
	}
	if err := b.provision(ctx); err != nil {
		return st, err
	}
	engine, err := b.verify(ctx)
	if err != nil {
		return st, fmt.Errorf("built and it cannot run a container: %w", err)
	}
	b.log("the engine answers: " + engine)
	if err := b.writeRecord(); err != nil {
		return st, err
	}
	return b.Status(ctx, true)
}

func (b *Base) create(ctx context.Context) error {
	dir := b.Dir()
	if _, err := os.Stat(dir); err == nil {
		// ⛔ Never import over a leftover directory: wsl --import into one that
		// already holds a disk is how two distributions share one file.
		if err := RemoveInside(b.home, dir); err != nil {
			return fmt.Errorf("%s is left over from an earlier build and could not be removed: %w", dir, err)
		}
		b.log("removed a leftover " + dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	engine, err := FindEngine(ctx)
	if err != nil {
		return fmt.Errorf("the base is built from an OCI image, so a host engine is required: %w", err)
	}
	b.log(fmt.Sprintf("host engine: %s, platform %s", engine.Name, engine.Platform()))

	tarPath := filepath.Join(b.home, "base-rootfs.tar")
	defer func() {
		if err := os.Remove(tarPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			b.log("the rootfs archive is still on disk: " + tarPath)
		}
	}()
	if err := engine.ExportRootfs(ctx, b.cfg.Base.Image, tarPath, b.log); err != nil {
		return err
	}
	if err := b.assertSpace(tarPath); err != nil {
		return err
	}
	b.log("importing as WSL2 distribution " + b.cfg.Base.Name)
	if err := b.wsl.Import(ctx, b.cfg.Base.Name, dir, tarPath, b.cfg.Base.Name); err != nil {
		return err
	}
	return nil
}

// assertSpace refuses an import the volume cannot hold, BEFORE anything is
// registered.
func (b *Base) assertSpace(tarPath string) error {
	size, ok := FileSize(tarPath)
	if !ok {
		return fmt.Errorf("cannot measure %s", tarPath)
	}
	need := BaseSpaceFloor + 2*size
	free, ok, err := FreeSpace(b.Dir())
	if err != nil {
		return err
	}
	if !ok {
		// ⚠ "could not measure" is a third answer, and treating it as either of
		// the other two is a lie or a needless refusal.
		b.log("free space on this volume could not be read; importing anyway")
		return nil
	}
	b.log(fmt.Sprintf("space: %s wanted, %s free", HumanBytes(need), HumanBytes(free)))
	if free < need {
		return fmt.Errorf("NOT ENOUGH DISK SPACE for %s. About %s is wanted and %s is free on the volume holding it. "+
			"Nothing has been imported and nothing is registered", b.Dir(), HumanBytes(need), HumanBytes(free))
	}
	return nil
}

func (b *Base) provision(ctx context.Context) error {
	b.log("provisioning: a rootless engine and the " + b.cfg.Base.User + " account")
	out := &prefixWriter{prefix: "", to: b.logWriter()}
	code, err := b.wsl.Exec(ctx, ExecRequest{
		Distro: b.cfg.Base.Name,
		User:   "root",
		Script: provisionScript,
		Env: map[string]string{
			"TK_USER": b.cfg.Base.User,
			"TK_UID":  "1000",
		},
		Timeout: 30 * time.Minute,
		Stdout:  out,
		Stderr:  out,
	})
	if err != nil || code != 0 {
		return fmt.Errorf("provisioning exited %d: %w", code, err)
	}
	if !strings.Contains(out.seen.String(), "provision-complete") {
		// ⛔ The script prints that line last, so its absence over a zero exit is
		// a step that exited 0 having done nothing.
		return errors.New("provisioning exited 0 without reaching its last line")
	}
	// WSL reads /etc/wsl.conf at start, so without this restart the settings
	// just written appear to have been applied and are not.
	b.log("restarting so WSL re-reads /etc/wsl.conf")
	if err := b.wsl.Terminate(ctx, b.cfg.Base.Name, b.cfg.Base.Name); err != nil {
		return err
	}
	return nil
}

// verify runs a real container as the unprivileged account and returns the
// engine's own version line.
func (b *Base) verify(ctx context.Context) (string, error) {
	half := func() string {
		var raw [6]byte
		if _, err := rand.Read(raw[:]); err != nil {
			// A failure to read randomness is not a reason to refuse to verify,
			// and it is reported through the marker rather than swallowed.
			return "nrandom"
		}
		return hex.EncodeToString(raw[:])
	}
	m1, m2 := half(), half()
	out, stderr, code, err := b.captureAs(ctx, b.cfg.Base.User, verifyScript, map[string]string{
		"TK_IMAGE": VerifyImage,
		"TK_M1":    m1,
		"TK_M2":    m2,
	}, 20*time.Minute)
	if err != nil || code != 0 {
		return "", fmt.Errorf("a container did not run as %s (exit %d): %s", b.cfg.Base.User, code, firstLine(stderr+out))
	}
	// ⛔ Compared with whitespace removed: a tty wraps a long line, so a marker
	// that arrived correctly can fail an exact match.
	flat := strings.Join(strings.Fields(out), "")
	if !strings.Contains(flat, m1+m2) {
		return "", fmt.Errorf("the container ran and did not return the marker: %s", firstLine(out+stderr))
	}
	engine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "engine ") {
			engine = strings.TrimSpace(strings.TrimPrefix(line, "engine "))
		}
	}
	return engine, nil
}

// VerifyImage is what the health check runs: small, in the catalog, and pulled
// with --pull=missing so a warm base costs nothing.
const VerifyImage = "docker.io/library/alpine:latest"

// captureAs runs a script as an account inside the base with the runtime
// directory every rootless engine needs already set.
func (b *Base) captureAs(ctx context.Context, user string, script []byte, env map[string]string, timeout time.Duration) (string, string, int, error) {
	full := map[string]string{}
	for k, v := range env {
		full[k] = v
	}
	out := &boundedBuffer{max: 8 << 20}
	errBuf := &boundedBuffer{max: 512 << 10}
	code, err := b.wsl.Exec(ctx, ExecRequest{
		Distro: b.cfg.Base.Name, User: user,
		Script: append(guestRuntimePrologue(), script...),
		Env:    full, Timeout: timeout, Stdout: out, Stderr: errBuf,
	})
	return out.String(), errBuf.String(), code, err
}

// guestRuntimePrologue gives the account the runtime directory rootless podman
// needs, which neither WSL nor runuser sets.
//
// ⚠ Not a copy of wsl-toolkit.ps1's -UserEnv prologue. That one prepares an
// arbitrary imported distribution; this runs in one this executable provisioned,
// where the account and the paths are known.
func guestRuntimePrologue() []byte {
	return []byte(`_tk_uid=$(id -u)
_tk_run=/tmp/wsl-toolkit-run-$_tk_uid
(umask 077; mkdir "$_tk_run") 2>/dev/null || :
if [ -L "$_tk_run" ] || [ ! -d "$_tk_run" ]; then
    echo "wsl-toolkit: $_tk_run is not a private directory" >&2; exit 2
fi
chmod 700 "$_tk_run" || exit 2
XDG_RUNTIME_DIR=$_tk_run; export XDG_RUNTIME_DIR
TMPDIR=${TMPDIR:-/tmp}; export TMPDIR
unset _tk_uid _tk_run
`)
}

// Remove unregisters the base and deletes its disk.
func (b *Base) Remove(ctx context.Context) error {
	exists, err := b.wsl.Exists(ctx, b.cfg.Base.Name)
	if err != nil {
		return err
	}
	if exists {
		b.log("unregistering " + b.cfg.Base.Name)
		if err := b.wsl.Unregister(ctx, b.cfg.Base.Name, b.cfg.Base.Name); err != nil {
			return err
		}
	}
	dir := b.Dir()
	if _, err := os.Stat(dir); err == nil {
		// wsl --unregister releases the disk asynchronously, so a removal right
		// afterwards can lose the race against a handle about to close.
		var lastErr error
		for attempt := 1; attempt <= 5; attempt++ {
			if lastErr = RemoveInside(b.home, dir); lastErr == nil {
				b.log("deleted " + dir)
				break
			}
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
		}
		if lastErr != nil {
			return fmt.Errorf("the disk is still on disk after five attempts: %w", lastErr)
		}
	}
	if err := os.Remove(b.recordPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// prefixWriter relays a child's output through the logger and keeps a copy, so
// a caller can assert on what was said rather than on the exit code.
type prefixWriter struct {
	prefix  string
	to      func(string)
	seen    boundedBuffer
	partial []byte
}

func (w *prefixWriter) logTo(s string) {
	if w.to != nil {
		w.to(s)
	}
}

func (w *prefixWriter) Write(p []byte) (int, error) {
	if w.seen.max == 0 {
		w.seen.max = 4 << 20
	}
	if _, err := w.seen.Write(p); err != nil {
		return 0, err
	}
	w.partial = append(w.partial, p...)
	for {
		i := indexByte(w.partial, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(w.partial[:i]), "\r")
		w.partial = w.partial[i+1:]
		if line != "" {
			w.logTo(w.prefix + line)
		}
	}
	return len(p), nil
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func (b *Base) logWriter() func(string) { return b.log }
