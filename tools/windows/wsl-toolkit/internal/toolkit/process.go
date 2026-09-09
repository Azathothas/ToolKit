// Package toolkit owns discovery, isolated jobs and their local service.
package toolkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ProcessError preserves the child's code independently from its output.
type ProcessError struct {
	Code int
	Op   string
	Err  error
}

func (e *ProcessError) Error() string { return fmt.Sprintf("%s: %v", e.Op, e.Err) }
func (e *ProcessError) Unwrap() error { return e.Err }

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var pe *ProcessError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return 2
}

type boundedBuffer struct {
	mu        sync.Mutex
	b         bytes.Buffer
	max       int
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(p) > b.max-b.b.Len() {
		p = p[:b.max-b.b.Len()]
		b.truncated = true
	}
	_, err := b.b.Write(p)
	return n, err
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func newCommand(ctx context.Context, file string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, file, args...)
	c.Env = append(os.Environ(), "WSL_UTF8=1")
	c.WaitDelay = time.Second
	configureProcess(c)
	return c
}

func runCommand(ctx context.Context, c *exec.Cmd, in io.Reader, out, stderr io.Writer) error {
	c.Stdin, c.Stdout, c.Stderr = in, out, stderr
	err := c.Run()
	if err == nil {
		return nil
	}
	code := 2
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		code = 124
	} else if errors.Is(ctx.Err(), context.Canceled) {
		code = 130
	} else {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
	}
	return &ProcessError{Code: code, Op: "process", Err: err}
}

// Output drains both streams before waiting and bounds stored diagnostics.
func Output(ctx context.Context, file string, args ...string) (string, string, error) {
	out := &boundedBuffer{max: 2 << 20}
	stderr := &boundedBuffer{max: 64 << 10}
	err := runCommand(ctx, newCommand(ctx, file, args...), nil, out, stderr)
	if err == nil && (out.truncated || stderr.truncated) {
		err = errors.New("process output exceeded the capture limit")
	}
	return out.String(), stderr.String(), err
}

// RunForeground runs a child with THIS process's own streams, and returns its
// exit code.
//
// ⛔ THE STREAMS ARE INHERITED RATHER THAN CAPTURED. A wrapper that captured and
// replayed them would break two things the wrapped script decided on purpose:
// its heartbeat goes to stderr so a caller reading stdout gets the command's
// output alone, and an application that block-buffers off a terminal buffers
// differently behind a pipe. Inheriting means the caller sees exactly what they
// would have seen running the script directly.
func RunForeground(ctx context.Context, file string, args []string) (int, error) {
	cmd := newCommand(ctx, file, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return 124, err
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return 130, err
		}
		return 2, err
	}
	return 0, nil
}

// rawCommandLine is a Windows command line built by the caller instead of by
// Go's argument escaping.
//
// ⛔ IT EXISTS FOR ONE CASE AND SHOULD NOT GROW. cmd.exe does not parse a
// command line the way CreateProcess callers assume, so a .cmd probe assembled
// as an argument LIST reached the script as something it exited 1 on. Everything
// else goes through the argument list, which is escaped correctly and cannot be
// got wrong by hand.
type rawCommandLine string

// outputRaw is Output with the option of a caller-built command line.
func outputRaw(ctx context.Context, raw rawCommandLine, file string, args ...string) (string, string, error) {
	out := &boundedBuffer{max: 2 << 20}
	stderr := &boundedBuffer{max: 64 << 10}
	cmd := newCommand(ctx, file, args...)
	if raw != "" {
		setRawCommandLine(cmd, string(raw))
	}
	err := runCommand(ctx, cmd, nil, out, stderr)
	if err == nil && (out.truncated || stderr.truncated) {
		err = errors.New("process output exceeded the capture limit")
	}
	return out.String(), stderr.String(), err
}
