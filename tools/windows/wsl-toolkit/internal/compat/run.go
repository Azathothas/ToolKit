// SPDX-License-Identifier: 0BSD

package compat

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Getting bytes into a distro and running them, ported from the script's
// distro-file.ps1 and distro-run.ps1.

// writeDistroFile puts bytes inside a distro without a shell touching them.
//
// ⭐ THE SCRIPT THIS SENDS IS A PAYLOAD LIKE ANY OTHER, and goes through
// distroScriptCommand like every other payload, so the alphabet rule is
// checked by a machine.
//
// THE CONTENT STAYS BASE64 inside that payload. Single-quoting it would work
// for text and would need a second escaping rule for anything else.
//
// THE PATH is single-quoted AND still restricted to an absolute path of
// letters, digits, dot, dash and underscore. ⚠ The restriction is no longer
// load-bearing for the transport; it is kept as a sanity guard, because every
// path this interface writes is one it chose.
func (s *session) writeDistroFile(distro, path, content, mode string) error {
	if !transportPathShape.MatchString(path) {
		return fmt.Errorf("Refusing to write '%s' inside the distro. Paths written by this tool are "+
			"restricted to an absolute path of letters, digits, dot, dash and underscore.", path)
	}
	if !octalMode.MatchString(mode) {
		return fmt.Errorf("Mode '%s' is not an octal file mode.", mode)
	}

	b64 := base64.StdEncoding.EncodeToString([]byte(content))
	slash := strings.LastIndex(path, "/")
	dir := "/"
	if slash > 0 {
		dir = path[:slash]
	}
	qPath := shellSingleQuoted(path)
	qDir := shellSingleQuoted(dir)

	// Each step reports its own failure. Without the || exit lines a missing
	// base64 leaves an empty file and a chmod that succeeds over it, and the
	// whole thing returns 0 having written nothing.
	payload := strings.Join([]string{
		"mkdir -p " + qDir + " || exit 1",
		"echo " + b64 + " | base64 -d > " + qPath + " || exit 1",
		"chmod " + mode + " " + qPath + " || exit 1",
	}, "\n")

	rc := 0
	said, err := s.distroOutput(distro, "root", []byte(payload), &rc, "the write of "+path)
	if err != nil {
		return err
	}
	if rc != 0 {
		return fmt.Errorf("Could not write %s inside '%s' (exit %d). The likeliest cause is an "+
			"image with no base64: busybox has one and coreutils has one, but a rootfs built "+
			"from scratch may have neither. The guest said: %s", path, distro, rc, strings.TrimSpace(said))
	}
	return nil
}

var octalMode = regexp.MustCompile(`^[0-7]{3,4}$`)

// distroOutput runs a payload inside a distro and RETURNS what it printed.
// invokeInDistro is the other half of the same pair: it STREAMS, because a
// caller's command has to be visible while it runs, and this one captures,
// because the tool itself needs to read an answer.
//
// Two shapes, ONE transport: both build the command through
// distroScriptCommand.
//
// ⛔ A QUESTION IS BOUNDED BY -TimeoutSeconds. invokeInDistro is for the
// CALLER'S command, which is deliberately not.
func (s *session) distroOutput(distro, runAs string, script []byte, exitCode *int, what string) (string, error) {
	*exitCode = 1
	line, err := distroScriptCommand(script, guestScratchPath())
	if err != nil {
		return "", err
	}
	wsl, err := resolveWsl()
	if err != nil {
		return "", err
	}
	res := s.boundedCapture(wsl, []string{"-d", distro, "-u", runAs, "--", "/bin/sh", "-lc", line},
		time.Duration(s.opts.TimeoutSeconds)*time.Second)
	if res.Err != nil {
		return "", res.Err
	}
	if res.TimedOut {
		// ⛔ The wedged userspace is stopped rather than left running. Killing
		// wsl.exe on this side ends the wait, not the process in the guest.
		_, _ = s.wslCapture(wsl, []string{"--terminate", distro}, true)
		*exitCode = 124
		return res.Text, fmt.Errorf("TIMED OUT after %ds waiting for %s in '%s'. "+
			"It never answered, which is not the same as it not being installed: the distro "+
			"is registered and wsl.exe ran, and nothing came back. Its init is most likely "+
			"wedged. The distro has been terminated. Raise the bound with -TimeoutSeconds if "+
			"this machine is simply slow. Partial output: %s",
			s.opts.TimeoutSeconds, what, distro, strings.TrimSpace(res.Text))
	}
	*exitCode = res.Exit
	return res.Text, nil
}

// enableDistroSystemd writes /etc/wsl.conf, restarts the distro so WSL reads
// it, and then CHECKS THAT SYSTEMD IS ACTUALLY PID 1.
//
// ⛔ THE CHECK IS THE POINT, not the write. A switch that writes a file nothing
// acts on is a flag that lies: the caller would come away believing they have
// systemd. Measured on real images, most OCI bases do NOT ship systemd at all.
//
// --terminate is what makes wsl.conf take effect. A distro already running
// keeps the init it started with.
func (s *session) enableDistroSystemd(distro string) error {
	s.log.step("Enabling systemd via /etc/wsl.conf")
	if err := s.writeDistroFile(distro, "/etc/wsl.conf", "[boot]\nsystemd=true\n", "0644"); err != nil {
		return err
	}

	wsl, err := resolveWsl()
	if err != nil {
		return err
	}
	if _, err := s.wslCapture(wsl, []string{"--terminate", distro}, true); err != nil {
		return err
	}

	// Delimited on purpose: the answer is matched inside a marker rather than
	// by comparing the whole captured text.
	rc := 0
	probe := `printf "WSLEPH_PID1[%s]" "$(cat /proc/1/comm 2>/dev/null)"`
	text, err := s.distroOutput(distro, "root", []byte(probe), &rc, "systemd to come up")
	if err != nil {
		return err
	}
	pid1 := ""
	if m := pid1Shape.FindStringSubmatch(text); m != nil {
		pid1 = m[1]
	}
	if pid1 == "systemd" {
		s.log.ok("systemd is PID 1")
		return nil
	}
	return fmt.Errorf("-Systemd was asked for and this distro is NOT running systemd: PID 1 is "+
		"'%s'. The likeliest cause is an image that does not ship systemd, and most "+
		"do not. Nothing is left registered. Use an image that ships systemd, or drop -Systemd.", pid1)
}

var pid1Shape = regexp.MustCompile(`WSLEPH_PID1\[([^\]]*)\]`)

// relaySinks is where the relay writes. The guest's own stdout goes to the
// first, everything else to the second, and that split is the contract.
type relaySinks struct {
	Out io.Writer
	Err io.Writer
}

// relayProc is a started child the relay loop drives.
type relayProc struct {
	stdout io.Reader
	stderr io.Reader
	wait   func() int
	kill   func()
}

// startRelayProcess starts wsl.exe with piped streams for the relay.
func (s *session) startRelayProcess(ctx context.Context, wsl string, args []string) (*relayProc, error) {
	cmd := exec.CommandContext(ctx, wsl, args...)
	cmd.Env = wslEnv()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &relayProc{
		stdout: stdout,
		stderr: stderr,
		wait: func() int {
			err := cmd.Wait()
			if err == nil {
				return 0
			}
			var ee *exitStatus
			if asExitStatus(err, &ee) {
				return ee.code
			}
			return 1
		},
		kill: func() { _ = cmd.Process.Kill() },
	}, nil
}

// invokeInDistro is the ONE path that runs a caller's command inside a
// distro. New and Run both go through it, so an inner exit code cannot be
// propagated by one action and dropped by the other.
//
// ⭐ ONE BRANCH, AND IT IS THE ONLY PLACE THE STREAM LOG IS TURNED ON OR OFF.
// New and Run both arrive here, so the log cannot be on for one action and off
// for the other.
//
// ⛔ IT READS the resolved relayOff, NOT the -NoTimestamps switch alone. There
// are TWO spellings of "no relay", -NoTimestamps and -TimestampProfile raw,
// and Run folds them into one variable.
func (s *session) invokeInDistro(ctx context.Context, distro, runAs string, script []byte, exitCode *int) error {
	// Set before anything, and non-zero: "it never answered" is a failure,
	// not a pass.
	*exitCode = 1
	if !s.relayOff {
		return s.invokeInDistroLogged(ctx, distro, runAs, script, exitCode)
	}

	// -NoTimestamps: the child inherits this process's handles, so the guest's
	// bytes reach the terminal without passing through this process at all.
	// Nothing here can re-encode, re-order or buffer them, which is the point
	// of having the switch rather than a relay that promises not to.
	line, err := distroScriptCommand(script, guestScratchPath())
	if err != nil {
		return err
	}
	wsl, err := resolveWsl()
	if err != nil {
		return err
	}
	code, runErr := s.foreground(ctx, wsl, []string{"-d", distro, "-u", runAs, "--", "/bin/sh", "-lc", line})
	*exitCode = code
	return runErr
}

// invokeInDistroLogged runs the caller's command with the stream log on: a
// stamp on every line, a heartbeat while there are none, and an optional bound
// on the whole thing.
//
// THE COST OF DOING THIS AT ALL, stated here because it is the one thing a
// caller can be surprised by. Relaying means the guest's stdout is a PIPE
// rather than whatever this process inherited, so an application that
// block-buffers when it is not writing to a terminal will buffer. Its lines
// then arrive late and carry the time THIS process received them.
// -NoTimestamps hands the handles straight through and is byte-exact.
//
// THE EXIT CODE IS THE COMMAND'S, unchanged. Nothing here decides it except
// -CommandTimeoutSeconds, which is opt-in, has no default, and reports 124 the
// way coreutils' timeout does.
func (s *session) invokeInDistroLogged(ctx context.Context, distro, runAs string, script []byte, exitCode *int) error {
	*exitCode = 1
	line, err := distroScriptCommand(script, guestScratchPath())
	if err != nil {
		return err
	}
	wsl, err := resolveWsl()
	if err != nil {
		return err
	}

	started := time.Now()
	elapsed := func() dur { return dur(time.Since(started)) }

	st := newStreamState(distro, s.settings, elapsed, s.out, s.errw)

	// ⛔ THE SINKS ARE OPENED BEFORE THE PROCESS STARTS. A reserved device name
	// or an unwritable directory has to be refused while nothing has happened
	// yet; discovering it after the guest has run for ten minutes means the
	// run is over and the log the caller asked for does not exist.
	if s.settings.TextPath != "" {
		st.text, err = newTextSink(s.settings.TextPath, s.settings.TextOverwrite)
		if err != nil {
			return err
		}
	}
	if s.settings.EventPath != "" {
		st.events, err = newEventSink(s.settings.EventPath, distro)
		if err != nil {
			closeSink(st.text)
			return err
		}
	}

	proc, err := s.startRelayProcess(ctx, wsl, []string{"-d", distro, "-u", runAs, "--", "/bin/sh", "-lc", line})
	if err != nil {
		closeSink(st.text)
		closeSink(st.events)
		return err
	}

	hitDeadline := false
	runErr := relayLoop(st, s, proc, &hitDeadline)
	if hitDeadline {
		*exitCode = 124
	} else {
		*exitCode = proc.wait()
	}

	// ⭐ A NON-ZERO CODE GETS A READING, not just a number. 137 is the code
	// from the incident this layer was built for, and on its own it says
	// "killed by signal 9" and stops. The distro's state is read HERE, once,
	// while the answer still describes the moment the command ended.
	if why := getExitCodeDiagnosis(*exitCode, s.distroRunState(distro)); why != "" {
		st.writeLine("note", why, false, "inf")
	}
	if st.events != nil {
		st.events.writeRecord("EXIT", elapsed(), "obs", "", "", false, map[string]any{
			"exit_code": *exitCode, "timed_out": hitDeadline,
		})
	}
	closeSink(st.text)
	closeSink(st.events)
	return runErr
}

// relayLoop is the poll loop. Tests drive it with pipes over bytes in memory,
// so the loop's own decisions are proved without wsl.exe.
//
// THE STREAMS ARE READ BEFORE THE PROCESS IS WAITED ON. Waiting first
// deadlocks any child that fills a pipe buffer: the child blocks writing and
// the parent blocks waiting.
//
// ⭐ THE LOOP POLLS, IT DOES NOT BLOCK ON ONE STREAM. A select over both
// readers with a timeout is what leaves the tick and the flush bound free to
// fire while the guest is silent, which is the whole point of the layer: the
// PowerShell original sat in Task::WaitAny with the same 250ms timeout for the
// same reason.
func relayLoop(st *streamState, s *session, proc *relayProc, hitDeadline *bool) error {
	const poll = 250 * time.Millisecond
	flush := streamFlushMs * time.Millisecond
	var tickEvery dur
	if st.settings.TickSeconds > 0 {
		tickEvery = dur(st.settings.TickSeconds) * time.Second
	}
	var deadline dur
	if s.opts.CommandTimeoutSeconds > 0 {
		deadline = dur(s.opts.CommandTimeoutSeconds) * time.Second
	}

	type readEvent struct {
		tag   string
		chunk []byte
		err   error
	}
	events := make(chan readEvent, 4)
	pump := func(tag string, r io.Reader) {
		buf := make([]byte, 8192)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				events <- readEvent{tag: tag, chunk: chunk}
			}
			if err != nil {
				events <- readEvent{tag: tag, err: err}
				return
			}
		}
	}
	type pipe struct {
		tag     string
		pending string
		since   dur
		done    bool
	}
	streams := map[string]*pipe{
		"out": {tag: "out"},
		"err": {tag: "err"},
	}
	go pump("out", proc.stdout)
	go pump("err", proc.stderr)

	live := 2
	for live > 0 {
		nowD := st.started()

		if deadline > 0 && nowD >= deadline {
			*hitDeadline = true
			// THE DISTRO IS TERMINATED, NOT LEFT RUNNING. Killing wsl.exe on
			// this side ends the wait and not the process in the guest, which
			// would carry on holding the disk and the CPU with nobody reading
			// it.
			st.writeLine("tick",
				fmt.Sprintf("TIMED OUT after %s: -CommandTimeoutSeconds %d was reached. "+
					"Terminating '%s'. The exit code is 124, which is what coreutils' "+
					"timeout reports.", formatDuration(st.started()), s.opts.CommandTimeoutSeconds, st.distro),
				false, "obs")
			proc.kill()
			_, _ = s.wslTerminate(st.distro)
			return nil
		}

		// An unterminated line that has sat this long is shown early, marked
		// as unterminated. A prompt waiting on stdin that will never arrive is
		// exactly this shape, and it is the one case where showing nothing
		// means waiting forever.
		for _, p := range streams {
			if p.done || p.pending == "" {
				continue
			}
			if nowD-p.since < flush {
				continue
			}
			st.writeLine(p.tag, p.pending, true, "obs")
			st.counts[p.tag].Lines++
			p.pending = ""
			st.lastLine = nowD
			st.lastTick = nowD
		}

		if tickEvery > 0 && nowD-st.lastLine >= tickEvery && nowD-st.lastTick >= tickEvery {
			st.writeTick(s)
			st.quiet = true
		}

		select {
		case ev := <-events:
			p := streams[ev.tag]
			if p == nil {
				continue
			}
			if ev.err != nil {
				p.done = true
				live--
				if p.pending != "" {
					st.writeLine(p.tag, p.pending, true, "obs")
					st.counts[p.tag].Lines++
					p.pending = ""
				}
				continue
			}
			// ⭐ SILENCE ENDING IS ITSELF A LINE. A run that recovered at four
			// minutes and a run that never recovered are the same picture in a
			// log that only ever reports the alarm.
			if st.quiet {
				st.writeSilenceEnd(st.started() - st.lastLine)
				st.quiet = false
			}
			st.counts[p.tag].Bytes += int64(len(ev.chunk))
			had := p.pending != ""
			ready, rest := splitStreamChunk(p.pending + string(ev.chunk))
			for _, l := range ready {
				// ⛔ CONSUMED, NOT RELAYED, and only when the caller named a
				// token. A partial line is never a progress report: it has no
				// terminator yet, so the label could still be arriving.
				if s.opts.ProgressPrefix != "" && !l.Partial {
					if prog := readProgressLine(l.Text, s.opts.ProgressPrefix); prog != nil {
						prog.At = st.started()
						st.progress = prog
						if st.events != nil {
							data := map[string]any{"progress_percent": prog.Percent, "stream": p.tag}
							if prog.Label != "" {
								data["progress_label"] = prog.Label
							}
							st.events.writeRecord("PROGRESS", st.started(), "obs", "", "", false, data)
						}
						continue
					}
				}
				st.writeLine(p.tag, l.Text, l.Partial, "obs")
				st.counts[p.tag].Lines++
			}
			p.pending = rest
			if p.pending != "" && (!had || len(ready) > 0) {
				p.since = st.started()
			}
			st.lastLine = st.started()
			st.lastTick = st.lastLine
		case <-time.After(poll):
			// The poll is what lets the deadline, the flush and the tick fire
			// while the guest is silent.
		case <-s.stop.Done():
			// The caller cancelled the whole run. Kill the child and report
			// the interruption as itself.
			proc.kill()
			_, _ = s.wslTerminate(st.distro)
			return s.stop.Err()
		}
	}
	return nil
}

// streamFlushMs is how long an UNTERMINATED line may sit in the relay before
// the stream log shows it early, marked as unterminated. A constant rather
// than a flag, in milliseconds. 2000 is comfortably longer than a progress
// bar's redraw interval, so an ordinary '\r' meter is never split by it.
const streamFlushMs = 2000

func (s *session) wslTerminate(distro string) (string, error) {
	wsl, err := resolveWsl()
	if err != nil {
		return "", err
	}
	return s.wslCapture(wsl, []string{"--terminate", distro}, true)
}

// closeSink closes, and never fails the run while doing it. A sink that fails
// to close at the end of a run must not turn a command that succeeded into a
// run that reported an error.
func closeSink(c io.Closer) {
	if c == nil {
		return
	}
	_ = c.Close()
}
