// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const baseExecUsage = `wsl-toolkit base exec (-c COMMAND | --command-base64 BASE64 | --script FILE)

  Run a non-interactive POSIX script directly in the configured base. The
  script starts in the guest account's home unless --dir names a guest path,
  and its stdin is /dev/null. Its stdout, stderr and exit code are forwarded
  unchanged.

  --detach starts it with no client attached and answers with a session id.
  The base runs it in a process group of its own, keeps its two streams and
  its exit code, and holds --timeout itself. wsl-toolkit logs ID --follow,
  wait ID and stop ID reach it.
`

type baseExecFlags struct {
	command    string
	commandB64 string
	scriptFile string
	dir        string
	timeout    time.Duration
	asRoot     bool
	viaHelper  bool
	privateNet bool
	detach     bool
	asJSON     bool
}

func (e *baseExecFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&e.command, "c", "", "the POSIX shell command to run in the base")
	fs.StringVar(&e.commandB64, "command-base64", "", "the command as base64 of its bytes, for a caller that must keep every shell away from it")
	fs.StringVar(&e.scriptFile, "script", "", "a file on this machine whose bytes are the command")
	fs.StringVar(&e.dir, "dir", "~", "working directory inside the guest. Default is its home")
	fs.DurationVar(&e.timeout, "timeout", 30*time.Minute, "how long the command may run. 0 means no deadline")
	fs.BoolVar(&e.asRoot, "root", false, "run as root instead of the configured account")
	fs.BoolVar(&e.viaHelper, "via-helper", false, "prove the helper refusal for arbitrary base commands")
	fs.BoolVar(&e.privateNet, "private-net", false, "run in a network namespace of the account's own, with the Windows host and the private ranges refused. ⛔ No container can run inside it")
	fs.BoolVar(&e.detach, "detach", false, "start the command with no client attached and answer with a session id. wsl-toolkit logs ID --follow, wait ID and stop ID reach it")
	fs.BoolVar(&e.asJSON, "json", false, "with --detach, write the session as a structured answer")
}

func (e baseExecFlags) request(cfg toolkit.Config) (toolkit.ExecRequest, error) {
	payload, err := guestScript(e.command, e.commandB64, e.scriptFile)
	if err != nil {
		return toolkit.ExecRequest{}, err
	}
	if e.timeout < 0 {
		return toolkit.ExecRequest{}, fmt.Errorf("--timeout %s is negative. Pass 0 for no deadline, or a positive duration", e.timeout)
	}
	dir := strings.TrimSpace(e.dir)
	if err := gitBashRewrite("--dir", dir, guestPath); err != nil {
		return toolkit.ExecRequest{}, err
	}
	if dir != "~" && !strings.HasPrefix(dir, "/") {
		return toolkit.ExecRequest{}, fmt.Errorf("--dir %q is not a guest absolute path. Pass ~ or a path beginning with /", e.dir)
	}
	user := cfg.Base.User
	if e.asRoot {
		user = "root"
	}
	var env map[string]string
	if e.privateNet {
		// ⛔ ROOT IS REFUSED HERE, and it is not squeamishness. The namespace is
		// what confines the ACCOUNT; a root payload can unmount, re-mount and
		// re-enter whatever it likes, so wrapping it would put a boundary around
		// something that can step over it and report that it had been confined.
		if e.asRoot {
			return toolkit.ExecRequest{}, errors.New("--private-net and --root are contradictory: guest root can leave the namespace it is put in, so the confinement would be a claim rather than a fact")
		}
		host, hostErr := toolkit.ResolveHostAddress()
		address := ""
		if hostErr == nil {
			address = host.Address
		}
		wrapped, wrappedEnv, err := toolkit.PrivateNetPayload(payload, address)
		if err != nil {
			return toolkit.ExecRequest{}, err
		}
		payload, env = wrapped, wrappedEnv
	}
	return toolkit.ExecRequest{
		Distro:  cfg.Base.Name,
		User:    user,
		Script:  payload,
		Env:     env,
		Dir:     dir,
		Timeout: e.timeout,
		// ⛔ FRAMED, so a command that reads stdin cannot eat the lines after it.
		Payload: true,
	}, nil
}

func cmdBaseExec(ctx context.Context, args []string) (int, error) {
	var opts baseExecFlags
	fs := newFlagSet("base exec")
	opts.bind(fs)
	if err := parseArgs(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, baseExecUsage)
			return exitOK, nil
		}
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	req, err := opts.request(cfg)
	if err != nil {
		return exitCannot, err
	}
	if opts.asJSON && !opts.detach {
		return exitCannot, errors.New("base exec forwards the command's own streams, so --json has nothing to describe. It writes the session with --detach")
	}
	if c, err := useHelper(ctx, opts.viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		return exitCannot, errors.New("base exec runs arbitrary commands and is not accepted by the restricted helper protocol. Make this call through the session's WSL approval path")
	}
	w, err := toolkit.FindWsl()
	if err != nil {
		return exitCannot, err
	}
	exists, err := w.Exists(ctx, cfg.Base.Name)
	if err != nil {
		return exitCannot, err
	}
	if !exists {
		return exitCannot, fmt.Errorf("%s is not registered. Build it with: wsl-toolkit base ensure", cfg.Base.Name)
	}
	if opts.detach {
		return detachSession(ctx, w, req, opts)
	}
	req.Stdout, req.Stderr = os.Stdout, os.Stderr
	return baseExecResult(w.Exec(ctx, req))
}

// detachSession starts a base exec with no client attached. WSL-95.
//
// ⛔ THE PAYLOAD IS THE ONE THE ATTACHED PATH WOULD SEND, environment and
// private network wrapping included, so --detach changes who waits and nothing
// about what runs.
func detachSession(ctx context.Context, w *toolkit.Wsl, req toolkit.ExecRequest, opts baseExecFlags) (int, error) {
	home, err := toolkit.Home()
	if err != nil {
		return exitCannot, err
	}
	rec, err := toolkit.StartSession(ctx, w, home, toolkit.SessionStart{
		Distro: req.Distro, User: req.User, Dir: req.Dir,
		Payload: toolkit.SessionPayload(req), Timeout: opts.timeout,
	})
	if err != nil {
		return exitCannot, err
	}
	if opts.asJSON {
		return exitOK, writeJSON(map[string]any{
			"schema": DetachedSchema, "id": rec.ID, "kind": "session",
			"distro": rec.Distro, "user": rec.User, "guest_dir": rec.GuestDir,
			"follow": "wsl-toolkit logs " + rec.ID + " --follow",
			"wait":   "wsl-toolkit wait " + rec.ID,
			"stop":   "wsl-toolkit stop " + rec.ID,
		})
	}
	fmt.Println(rec.ID)
	logf("  session %s runs in %s as %s, with no client attached. Follow: wsl-toolkit logs %s --follow. Wait: wsl-toolkit wait %s. Stop: wsl-toolkit stop %s",
		rec.ID, rec.Distro, rec.User, rec.ID, rec.ID, rec.ID)
	return exitOK, nil
}

// baseExecResult forwards the guest's own exit status silently and surfaces
// every other failure.
//
// ⛔ A GUEST EXIT IS AN ANSWER AND A FAILED START IS AN ERROR. The first version
// dropped any error that was not a deadline or a cancellation, so a `wsl.exe`
// that could not be started exited 2 with its reason discarded, which reads
// exactly like a guest script that ran `exit 2`.
func baseExecResult(code int, runErr error) (int, error) {
	if runErr == nil {
		return code, nil
	}
	var exited *exec.ExitError
	if code != exitTimeout && code != 130 && errors.As(runErr, &exited) {
		return code, nil
	}
	return code, runErr
}
