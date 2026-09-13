// SPDX-License-Identifier: 0BSD

package compat

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// session is one invocation of the interface: the bound parameters, the state
// directory they resolved to, the two streams, and the resolved stream-log
// settings the relay reads.
//
// ⛔ EVERYTHING TOUCHING THIS MACHINE'S DESTRUCTIVE EDGES REACHES THE GUARDS
// THROUGH THIS TYPE, so there is no second place a removal path could grow
// without them.
type session struct {
	opts    Options
	baseDir string

	log  *console
	out  io.Writer // the report stream, which is stdout for a caller
	errw io.Writer // notes, which are stderr
	in   io.Reader

	relayOff     bool
	settings     *logSettings
	commandBytes []byte // nil means NO command was given, which is different from an empty one

	// stop is the caller's cancellation, honoured by the relay and the
	// bounded waits.
	stop context.Context
}

// newSession is THE ONE construction path for a session, so the report and
// note writers exist in exactly one spelling: the console reads the same two
// writers the session holds, and a second construction that disagreed with it
// is a defect this shape cannot produce.
func newSession(o Options, baseDir string, report, notes io.Writer, stdin io.Reader, ctx context.Context) *session {
	// The action report's colour is decided ONCE, here, from the report
	// stream: a console gets colour and a redirect never does, which is the
	// rule the script's host applied for its Write-Host colours. NO_COLOR is
	// honoured for the same reason the stream log honours it.
	color := writerIsTerminal(report) && envValue("NO_COLOR") == ""
	return &session{
		opts:    o,
		baseDir: baseDir,
		log:     &console{report: report, note: notes, color: color},
		out:     report,
		errw:    notes,
		in:      stdin,
		stop:    ctx,
	}
}

// wslEnv is the environment every wsl.exe child runs with.
//
// ⛔ WSL EMITS UTF-16LE UNLESS THIS IS SET, and without it every parsed string
// is NUL-riddled. It is one variable on the child, not a change to this
// process's environment.
func wslEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if strings.HasPrefix(e, "WSL_UTF8=") {
			continue
		}
		out = append(out, e)
	}
	return append(out, "WSL_UTF8=1")
}

// wslCapture runs wsl.exe and captures both streams merged, refusing a
// non-zero exit with what it said. It is the port of Invoke-Native.
func (s *session) wslCapture(wsl string, args []string, ignoreExitCode bool) (string, error) {
	cmd := exec.Command(wsl, args...)
	cmd.Env = wslEnv()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	cmd.Stdin = nil
	err := cmd.Run()
	text := buf.String()
	if err != nil && !ignoreExitCode {
		var ee *exitStatus
		if asExitStatus(err, &ee) {
			return text, fmt.Errorf("wsl.exe %s failed (exit %d): %s", strings.Join(args, " "), ee.code, strings.TrimSpace(text))
		}
		return text, fmt.Errorf("wsl.exe %s could not be started: %v", strings.Join(args, " "), err)
	}
	return text, nil
}

// boundedResult is what a bounded wait reports, and the fields are the
// contract: "it never answered" (timedOut) is a DIFFERENT fact from "it is
// not installed" (err), and a caller must be able to tell them apart.
type boundedResult struct {
	Text     string
	Exit     int
	TimedOut bool
	Err      error
}

// boundedCapture runs a program with a HARD TIME LIMIT and returns what it
// printed.
//
// ⛔ THE STREAMS ARE READ BEFORE THE WAIT, not after. Waiting first deadlocks
// any child that fills a pipe buffer: the child blocks writing and the parent
// blocks waiting, and neither moves until the timeout.
func (s *session) boundedCapture(path string, args []string, timeout time.Duration) boundedResult {
	cmd := exec.Command(path, args...)
	cmd.Env = wslEnv()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return boundedResult{Err: err}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		code := 0
		if err != nil {
			var ee *exitStatus
			if asExitStatus(err, &ee) {
				code = ee.code
			} else {
				return boundedResult{Err: err}
			}
		}
		return boundedResult{Text: buf.String(), Exit: code}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return boundedResult{Text: buf.String(), Exit: 1, TimedOut: true}
	case <-s.stop.Done():
		_ = cmd.Process.Kill()
		<-done
		return boundedResult{Text: buf.String(), Exit: 130, TimedOut: true, Err: s.stop.Err()}
	}
}

// foreground runs a program with this process's own streams, so the guest's
// bytes reach the terminal untouched. It is the -NoTimestamps path and the
// Enter action, and nothing else.
func (s *session) foreground(ctx context.Context, path string, args []string) (int, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = wslEnv()
	// The SESSION'S streams, which are this process's own in production: the
	// bytes are handed through untouched either way, and the indirection is
	// what lets the suite hold the streams still.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s.in, s.out, s.errw
	if err := cmd.Run(); err != nil {
		var ee *exitStatus
		if asExitStatus(err, &ee) {
			return ee.code, nil
		}
		if ctx.Err() != nil {
			return 130, err
		}
		return 2, err
	}
	return 0, nil
}
