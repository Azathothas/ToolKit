// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// `base exec` with a client attached: the caller's command in the base, its
// streams forwarded and its exit code the answer, with what `run` gives a job
// beside it. WSL-123, WSL-124.

// ExecInEnv and ExecOutEnv name the directories `base exec --input` and
// `--artifacts` give a command, exported to it. A detached session exports the
// same two. WSL-124.
const (
	ExecInEnv  = "WSL_TOOLKIT_IN"
	ExecOutEnv = "WSL_TOOLKIT_OUT"
)

// BaseExec is one attached `base exec`.
type BaseExec struct {
	Distro string
	User   string
	// Dir is "~" or an absolute guest path.
	Dir string
	// Script is the composed command, unframed, and Env the private network
	// wrapper's variables where it has any.
	Script  []byte
	Env     map[string]string
	Timeout time.Duration
	// Inputs are placed in the call's own in directory, and ArtifactDir is the
	// host directory its out directory is copied back to. Empty means none.
	Inputs      []JobInput
	ArtifactDir string
	// Stdin is what the command reads, and nil means /dev/null.
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Log            func(string)
}

// BaseExecResult is how one attached `base exec` ended.
type BaseExecResult struct {
	Exit int
	// Artifacts is the transfer --artifacts asked for and ArtifactError its
	// failure. Retained is the guest directory kept because of that failure, and
	// ID the call's id, which `artifacts retry` takes.
	Artifacts     ArtifactTransfer
	ArtifactError string
	Retained      string
	ID            string
}

// ErrDirNotEntered is a command that did not run because its directory could
// not be entered. WSL-124.
var ErrDirNotEntered = errors.New("could not be entered")

// RunBaseExec runs one attached `base exec`.
//
// ⛔ A DIRECTORY THE COMMAND CANNOT ENTER IS REFUSED. wsl.exe answers a `--cd`
// it cannot follow by starting in `/` and exiting 0, so `base exec --dir
// /nonexistent -c pwd` printed `/` and succeeded. Measured on WSL 2.7.12. The
// command now enters its directory itself, and says whether it could, before
// anything of the caller's runs. WSL-124.
//
// ⭐ WITH NO INPUT, NO ARTIFACTS AND NO STDIN, NOTHING IS PLACED IN THE GUEST.
// The command travels framed on stdin exactly as it always has.
func RunBaseExec(ctx context.Context, w *Wsl, e BaseExec) (BaseExecResult, error) {
	var res BaseExecResult
	if e.Log == nil {
		e.Log = func(string) {}
	}
	if e.Stderr == nil {
		e.Stderr = io.Discard
	}
	started, refused, err := newStartTokens()
	if err != nil {
		return res, err
	}
	gate := &startGate{dst: e.Stderr, started: []byte(started), refused: []byte(refused)}
	script, dir, stderr := e.Script, e.Dir, e.Stderr
	if entersItself(e.Dir) {
		script = append(dirGate(e.Dir, started, refused), e.Script...)
		// ⚠ wsl.exe starts in the home, so a directory it cannot reach costs one
		// line of the shell's and none of wsl.exe's own.
		dir, stderr = "~", gate
	}
	if len(e.Inputs) == 0 && e.ArtifactDir == "" && e.Stdin == nil {
		code, runErr := w.Exec(ctx, ExecRequest{
			Distro: e.Distro, User: e.User, Script: script, Env: e.Env, Dir: dir,
			Timeout: e.Timeout, Stdout: e.Stdout, Stderr: stderr, Payload: true,
		})
		if ferr := gate.Flush(); ferr != nil && runErr == nil {
			runErr = ferr
		}
		if gate.Refused() {
			return res, dirRefusal(e)
		}
		res.Exit = code
		return res, runErr
	}
	return runBaseExecInDir(ctx, w, e, script, dir, stderr, gate)
}

// entersItself says whether a command's directory is entered by the command, and
// checked, rather than by wsl.exe. The home is wsl.exe's own default.
func entersItself(dir string) bool {
	return dir != "" && dir != "~"
}

// runBaseExecInDir runs an attached command that has a directory of its own in
// the guest: its inputs, its output and, for --stdin, the command itself.
//
// ⭐ THE DIRECTORY IS A JOB'S, under the account's jobs root with an open record
// in the ledger, so `gc` spares it while the command runs and collects what a
// killed client left. ⚠ Under another account's home, `gc` does not look.
func runBaseExecInDir(ctx context.Context, w *Wsl, e BaseExec, script []byte, dir string, stderr io.Writer, gate *startGate) (BaseExecResult, error) {
	var res BaseExecResult
	home, err := accountHome(ctx, w, e.Distro, e.User)
	if err != nil {
		return res, err
	}
	id, err := newJobID()
	if err != nil {
		return res, err
	}
	res.ID = id
	paths := jobLayout(home, id)
	if err := AssertArgvSafe([]string{paths.dir}); err != nil {
		return res, err
	}
	// ⚠ ENSURED, because this writes: the record below is the first thing a
	// machine that never ran a job keeps in its state directory.
	if _, err := EnsureHome(); err != nil {
		return res, err
	}
	ledger, err := OpenLedger()
	if err != nil {
		return res, err
	}
	deadline := time.Time{}
	if e.Timeout > 0 {
		deadline = time.Now().Add(e.Timeout).UTC()
	}
	// ⭐ Recorded BEFORE anything exists, for the reason a job's record is.
	if err := ledger.Append(LedgerEntry{
		Event: "open", Kind: "job", ID: id, Distro: e.Distro,
		GuestDir: paths.dir, HostDir: e.ArtifactDir, Deadline: deadline, Note: "base exec as " + e.User,
	}); err != nil {
		return res, fmt.Errorf("could not record the command's directory before making it: %w", err)
	}
	keep := false
	defer func() {
		note := ""
		if keep {
			// ⚠ THE WORDS RetainedGuestDir READS, so `artifacts retry` finds it.
			note = "the job finished and its directory was kept: its artifacts could not be fetched"
		} else {
			bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
			defer cancel()
			out, errText, code, rmErr := w.Capture(bounded, e.Distro, e.User, []byte(GuestRemoveScript(paths.root, paths.dir)), 5*time.Minute)
			if rmErr != nil || code != 0 || !strings.Contains(out, "removed") {
				e.Log(fmt.Sprintf("could not remove %s after the command (exit %d): %s", paths.dir, code, guestFailure(errText, out)))
				return
			}
		}
		if err := ledger.Append(LedgerEntry{Event: "close", Kind: "job", ID: id, Note: note}); err != nil {
			e.Log("could not close the ledger record for " + id + ": " + err.Error())
		}
	}()

	made := []string{"/bin/mkdir", "-p", paths.dir}
	if e.ArtifactDir != "" {
		made = append(made, paths.out)
	}
	errBuf := &boundedBuffer{max: 64 << 10}
	if code, err := w.ExecDirect(ctx, e.Distro, e.User, "", made, nil, io.Discard, errBuf, 2*time.Minute); err != nil || code != 0 {
		return res, fmt.Errorf("could not create %s in the guest (exit %d): %s", paths.dir, code, guestFailure(errBuf.String()))
	}

	var exports strings.Builder
	if len(e.Inputs) > 0 {
		exports.WriteString(ExecInEnv + "=" + shellQuote(paths.in) + "; export " + ExecInEnv + "\n")
	}
	if e.ArtifactDir != "" {
		exports.WriteString(ExecOutEnv + "=" + shellQuote(paths.out) + "; export " + ExecOutEnv + "\n")
	}
	script = append([]byte(exports.String()), script...)

	// ⭐ ONE ARCHIVE carries the inputs and, for --stdin, the command.
	var files []JobInput
	for _, in := range e.Inputs {
		files = append(files, JobInput{Name: "in/" + in.Name, Path: in.Path, Bytes: in.Bytes})
	}
	if e.Stdin != nil {
		files = append(files, JobInput{Name: "payload.sh", Bytes: append(shellAssignments(e.Env), script...)})
	}
	if len(files) > 0 {
		errBuf = &boundedBuffer{max: 64 << 10}
		if _, err := pumpArchive(
			func(pw io.Writer) (WorkspaceUpload, error) { return writeInputsTar(pw, files) },
			func(pr io.Reader) (int, error) {
				return w.ExecDirect(ctx, e.Distro, e.User, "", []string{"/bin/tar", "-xf", "-", "-C", paths.dir}, pr, io.Discard, errBuf, 10*time.Minute)
			},
			func() string { return errBuf.String() }); err != nil {
			return res, fmt.Errorf("the command's files: %w", err)
		}
	}

	var code int
	var runErr error
	if e.Stdin != nil {
		code, runErr = w.execFile(ctx, e.Distro, e.User, dir, paths.dir+"/payload.sh", e.Stdin, e.Stdout, stderr, e.Timeout)
	} else {
		code, runErr = w.Exec(ctx, ExecRequest{
			Distro: e.Distro, User: e.User, Script: script, Env: e.Env, Dir: dir,
			Timeout: e.Timeout, Stdout: e.Stdout, Stderr: stderr, Payload: true,
		})
	}
	if ferr := gate.Flush(); ferr != nil && runErr == nil {
		runErr = ferr
	}
	if gate.Refused() {
		return res, dirRefusal(e)
	}
	res.Exit = code
	if e.ArtifactDir != "" {
		got, err := w.FetchArtifacts(ctx, e.Distro, e.User, paths.out, e.ArtifactDir, DefaultWorkspaceLimits(), e.Log)
		res.Artifacts = got
		if err != nil {
			// ⛔ THE GUEST COPY STAYS, as a job's does: it is the only copy left.
			res.ArtifactError, res.Retained = err.Error(), paths.dir
			keep = true
		}
	}
	return res, runErr
}

// execFile runs a guest script file with stdin carrying the caller's bytes.
//
// ⛔ THE COMMAND IS A FILE, because stdin carries the caller's data. Framed on
// stdin, a command cannot also read it, and an unframed script loses its own
// lines to the first command that reads. The file's path is this tool's own,
// asserted against the alphabet an argument survives. WSL-124.
func (w *Wsl) execFile(ctx context.Context, distro, user, dir, file string, stdin io.Reader, stdout, stderr io.Writer, timeout time.Duration) (int, error) {
	if err := AssertArgvSafe([]string{file}); err != nil {
		return 2, err
	}
	args := []string{"-d", distro, "-u", user}
	if dir != "" {
		args = append(args, "--cd", dir)
	}
	args = append(args, "--", "/bin/sh", file)
	bounded := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		bounded, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if err := runCommand(bounded, newCommand(bounded, w.Path, args...), stdin, stdout, stderr); err != nil {
		return ExitCode(err), err
	}
	return 0, nil
}

// dirRefusal names a directory the command could not enter.
func dirRefusal(e BaseExec) error {
	return fmt.Errorf("--dir %s %w in %s as %s, so the command did not run. The guest's own reason is the line above. Create it, or name a directory that exists",
		e.Dir, ErrDirNotEntered, e.Distro, e.User)
}

// newStartTokens draws the two words an attached command's first line writes:
// one when it has entered its directory, one when it could not.
func newStartTokens() (started, refused string, err error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", fmt.Errorf("no cryptographic randomness for the command's start marker: %w", err)
	}
	h := hex.EncodeToString(raw[:])
	return "wtk-exec-started-" + h, "wtk-exec-no-dir-" + h, nil
}

// dirGate is the line in front of an attached command that enters its directory,
// and writes to stderr which of the two words is true.
func dirGate(dir, started, refused string) []byte {
	return []byte("cd " + shellQuote(dir) + " || { printf '%s' " + shellQuote(refused) + " >&2; exit 2; }\n" +
		"printf '%s' " + shellQuote(started) + " >&2\n")
}

// startGate holds a command's stderr back until its first line says whether the
// command starts, takes that word out, and then passes everything through.
//
// ⭐ NOTHING IS HELD ONCE THE COMMAND STARTS. The word is written before any of
// the caller's command runs, so after it the stream is the command's own and is
// forwarded byte for byte. Before it there is only what wsl.exe and the shell
// say first.
//
// ⚠ A GATE THAT NEVER HEARS EITHER WORD opens at maxGateHold rather than holding
// a stream for ever, and at the end it releases what it held.
type startGate struct {
	mu               sync.Mutex
	dst              io.Writer
	started, refused []byte
	buf              []byte
	open, wasRefused bool
}

// maxGateHold is the most a gate holds before it gives up looking.
const maxGateHold = 64 << 10

func (g *startGate) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.open {
		if _, err := g.dst.Write(p); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	g.buf = append(g.buf, p...)
	for _, word := range [][]byte{g.started, g.refused} {
		i := bytes.Index(g.buf, word)
		if i < 0 {
			continue
		}
		g.open, g.wasRefused = true, bytes.Equal(word, g.refused)
		rest := append(append([]byte(nil), g.buf[:i]...), g.buf[i+len(word):]...)
		g.buf = nil
		if len(rest) > 0 {
			if _, err := g.dst.Write(rest); err != nil {
				return 0, err
			}
		}
		return len(p), nil
	}
	if len(g.buf) > maxGateHold {
		g.open = true
		held := g.buf
		g.buf = nil
		if _, err := g.dst.Write(held); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Flush releases what the gate still holds, once the stream is over.
func (g *startGate) Flush() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.open = true
	if len(g.buf) == 0 {
		return nil
	}
	held := g.buf
	g.buf = nil
	_, err := g.dst.Write(held)
	return err
}

// Refused says the command's first line could not enter its directory.
func (g *startGate) Refused() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.wasRefused
}

// AccountUID answers the uid of a guest account, and false where the base has no
// such account. It asks as asker, an account the base is known to have.
//
// ⛔ ASKED BEFORE THE COMMAND RUNS. wsl.exe answers a name the base does not have
// with `Wsl/WSL_E_USER_NOT_FOUND` and exit 0xffffffff, a code a caller cannot
// tell from a command's own failure. WSL-123.
func AccountUID(ctx context.Context, w *Wsl, distro, asker, name string) (int, bool, error) {
	if err := ValidateAccountName(name); err != nil {
		return 0, false, err
	}
	out, stderr, code, err := w.Capture(ctx, distro, asker, []byte("id -u "+shellQuote(name)+" 2>/dev/null || exit 3\n"), 2*time.Minute)
	if code == 3 {
		return 0, false, nil
	}
	if err != nil || code != 0 {
		return 0, false, fmt.Errorf("could not ask %s whether it has the account %s (exit %d): %s", distro, name, code, guestFailure(stderr, out))
	}
	uid, convErr := strconv.Atoi(strings.TrimSpace(firstLine(out)))
	if convErr != nil {
		return 0, false, fmt.Errorf("%s answered %q for the uid of %s, which is not a number", distro, firstLine(out), name)
	}
	return uid, true, nil
}

// ValidateAccountName refuses a name that is not a guest account's: letters,
// digits, dot, underscore and dash, starting with a letter or an underscore, at
// most 32 characters. WSL-123.
func ValidateAccountName(name string) error {
	if name == "" || len(name) > 32 {
		return fmt.Errorf("--user %q is not an account name: it is empty or longer than 32 characters", name)
	}
	for i, r := range name {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && (r >= '0' && r <= '9' || r == '-' || r == '.'):
		default:
			return fmt.Errorf("--user %q is not an account name. A name is letters, digits, dot, underscore and dash, and starts with a letter or an underscore", name)
		}
	}
	return nil
}
