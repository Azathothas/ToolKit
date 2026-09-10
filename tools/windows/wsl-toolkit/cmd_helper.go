package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const helperUsage = `wsl-toolkit helper <serve|status|stop>

  serve      listen for job requests. Start this ONCE through the approval
             path a restricted session offers, and later commands find it
  status     is one listening, and is it this build
  stop       ask a listening helper to shut down

  --detach   serve starts a background copy and returns as soon as it answers
  --json     a structured answer

  ⛔ It is a lifetime boundary, not a privilege boundary. It listens on the
  loopback interface, runs as whoever started it, and its token lives in that
  user's own state directory. It buys ONE approval instead of one per command.
`

func cmdHelper(ctx context.Context, args []string) (int, error) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, helperUsage)
		return exitCannot, nil
	}
	sub, rest := args[0], args[1:]
	fs := newFlagSet("helper " + sub)
	asJSON := fs.Bool("json", false, "write a structured answer")
	detach := fs.Bool("detach", false, "start a background copy and return once it answers")
	if err := parseArgs(fs, rest); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}

	switch sub {
	case "serve":
		if *detach {
			return helperDetach(ctx, *asJSON)
		}
		// ⚠ A second helper on one machine would overwrite the first's endpoint
		// file, so the first becomes unreachable while still holding its work.
		// This is a refusal rather than a race.
		if c, err := toolkit.DialHelper(ctx); err == nil {
			return exitCannot, fmt.Errorf("a helper is already listening on %s as pid %d", c.Endpoint().Address, c.Endpoint().PID)
		}
		server, err := toolkit.NewHelperServer(cfg, note)
		if err != nil {
			return exitCannot, err
		}
		logf("==> wsl-toolkit helper %s", versionString())
		if err := server.Serve(ctx); err != nil {
			return exitCannot, err
		}
		logf("  stopped")
		return exitOK, nil

	case "status":
		c, err := toolkit.DialHelper(ctx)
		if err != nil {
			if *asJSON {
				return exitFailed, writeJSON(map[string]any{
					"schema": toolkit.HelperSchema, "listening": false, "reason": err.Error(),
				})
			}
			logf("  no helper is answering: %s", err.Error())
			logf("  start one with: wsl-toolkit helper serve --detach")
			return exitFailed, nil
		}
		st, err := c.Status(ctx)
		if err != nil {
			return exitFailed, err
		}
		if *asJSON {
			st["listening"] = true
			st["address"] = c.Endpoint().Address
			return exitOK, writeJSON(st)
		}
		fmt.Fprintf(os.Stderr, "  listening   %s\n", c.Endpoint().Address)
		fmt.Fprintf(os.Stderr, "  pid         %v\n", st["pid"])
		fmt.Fprintf(os.Stderr, "  version     %v\n", st["version"])
		fmt.Fprintf(os.Stderr, "  base        %v as %v\n", st["base"], st["user"])
		if v, ok := st["version"].(string); ok && v != versionString() {
			// ⛔ A version mismatch is reported rather than tolerated. The
			// helper carries its OWN embedded script, so a client and a helper
			// from different builds are two different products, and a result
			// from one attributed to the other is a wrong measurement.
			fmt.Fprintf(os.Stderr, "  ! this build is %s and the helper is %s. Stop it and start it again from this binary\n", versionString(), v)
			return exitFailed, nil
		}
		return exitOK, nil

	case "stop":
		c, err := toolkit.DialHelper(ctx)
		if err != nil {
			// ⚠ STILL EXIT 0, because stopping something that is not running is
			// the outcome the caller asked for. The REASON is printed, though: it
			// used to be discarded, and DialHelper fails for three different
			// things. One of them is "something else is listening on that address
			// and reports a different pid", which is worth knowing and reads
			// nothing like "there is nothing to stop".
			logf("  no helper is answering, so there is nothing to stop: %s", err.Error())
			return exitOK, nil
		}
		if err := c.Stop(ctx); err != nil {
			return exitFailed, err
		}
		logf("  asked the helper on %s to stop", c.Endpoint().Address)
		return exitOK, nil

	default:
		fmt.Fprint(os.Stderr, helperUsage)
		return exitCannot, fmt.Errorf("%q is not a helper subcommand", sub)
	}
}

// helperDetach starts a background copy of this executable and waits until it
// answers.
//
// ⭐ IT WAITS ON THE CONDITION, never on a guessed duration. A sleep long enough
// on a fast machine is a race on a slow one, and the failure it produces is a
// caller reporting "no helper" about one that started a moment later.
func helperDetach(ctx context.Context, asJSON bool) (int, error) {
	if c, err := toolkit.DialHelper(ctx); err == nil {
		logf("  a helper is already listening on %s", c.Endpoint().Address)
		if asJSON {
			return exitOK, writeJSON(map[string]any{
				"schema": toolkit.HelperSchema, "listening": true,
				"address": c.Endpoint().Address, "started": false,
			})
		}
		return exitOK, nil
	}
	self, err := os.Executable()
	if err != nil {
		return exitCannot, err
	}
	args := []string{"helper", "serve"}
	if home := os.Getenv("WSL_TOOLKIT_HOME"); home != "" {
		args = append([]string{"--home", home}, args...)
	}
	cmd := exec.Command(self, args...)
	cmd.Stdout, cmd.Stderr = nil, nil
	toolkit.DetachProcess(cmd)
	if err := cmd.Start(); err != nil {
		return exitCannot, err
	}
	// ⛔ THE CHILD IS RELEASED, not waited on. A parent that kept the handle
	// would be a parent whose exit takes the helper with it, which is the
	// opposite of what detaching is for.
	if err := cmd.Process.Release(); err != nil {
		logf("  could not release the background process handle: %s", err.Error())
	}
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		c, err := toolkit.DialHelper(ctx)
		if err == nil {
			logf("  helper listening on %s as pid %d", c.Endpoint().Address, c.Endpoint().PID)
			if asJSON {
				return exitOK, writeJSON(map[string]any{
					"schema": toolkit.HelperSchema, "listening": true,
					"address": c.Endpoint().Address, "pid": c.Endpoint().PID, "started": true,
				})
			}
			return exitOK, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return exitCannot, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return exitCannot, fmt.Errorf("a background helper was started and did not answer within 30s: %w", lastErr)
}

// route is which of the two paths a command takes.
type route int

const (
	routeDirect route = iota // this process calls wsl.exe itself
	routeHelper              // the helper calls it on this process's behalf
	routeRefuse              // neither: report what went wrong
)

// decideRoute is the whole routing rule, with nothing in it that touches the
// machine, so every branch has a test.
//
// ⛔ THE INPUT IS THE PROBE'S ANSWER, NOT A PATH LOOKUP. Deciding on
// FindWsl succeeding meant deciding on "a wsl.exe exists", so a process that was
// refused by WSL took the direct route, failed, and was advised to start the
// helper it already had.
func decideRoute(probeErr error, helperReachable bool) route {
	switch {
	case probeErr == nil:
		return routeDirect
	case !errors.Is(probeErr, toolkit.ErrWslDenied) && !errors.Is(probeErr, toolkit.ErrWslMissing):
		// Not a refusal and not an absence. Whatever it is, a helper does not
		// fix it, and hiding it behind a route change is how a broken install
		// reads as a sandbox.
		return routeRefuse
	case helperReachable:
		return routeHelper
	default:
		// ⭐ No helper to reach, so the direct path runs and reports its own
		// refusal. That message names both ways forward; a refusal invented here
		// would name neither.
		return routeDirect
	}
}

// useHelper decides whether a command should go through the helper.
//
// ⭐ THE RULE IS: THIS PROCESS FIRST, THE HELPER WHEN THIS PROCESS IS REFUSED.
// A caller that can reach wsl.exe pays nothing for the helper existing, and a
// caller that cannot gets it without having to know it was there. An explicit
// --via-helper forces it, for proving the path works.
func useHelper(ctx context.Context, forced bool) (*toolkit.HelperClient, error) {
	if forced {
		return toolkit.DialHelper(ctx)
	}
	probeErr := toolkit.ProbeWsl(ctx)
	if probeErr == nil {
		return nil, nil
	}
	c, dialErr := toolkit.DialHelper(ctx)
	switch decideRoute(probeErr, dialErr == nil) {
	case routeHelper:
		note("this process cannot reach wsl.exe: " + probeErr.Error())
		note("using the helper on " + c.Endpoint().Address)
		return c, nil
	case routeRefuse:
		return nil, probeErr
	default:
		return nil, nil
	}
}
