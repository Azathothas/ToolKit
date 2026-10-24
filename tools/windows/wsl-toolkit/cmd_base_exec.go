// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const baseExecUsage = `wsl-toolkit base exec (-c COMMAND | --command-base64 BASE64 | --script FILE)

  Run a non-interactive POSIX script directly in the configured base. The
  script starts in the guest account's home unless --dir names a guest path,
  and its stdin is /dev/null unless --stdin passes this process's own. Its
  stdout, stderr and exit code are forwarded unchanged.

  --dir names a guest path, or a Windows directory, which is mapped to where
  the base sees it: its grant, or its drive mount when base.automount is ro or
  rw. --here does the same for this Windows directory. A directory the command
  cannot enter is refused, and the command does not run.

  --user NAME runs it as that account of the base, and --root as root.

  --input NAME=FILE places a file at NAME in a directory of the command's own,
  exported as WSL_TOOLKIT_IN. --artifacts DIR copies back what the command
  leaves in the directory exported as WSL_TOOLKIT_OUT, and makes DIR only when
  something arrives.

  --detach starts it with no client attached and answers with a session id.
  The base runs it in a process group of its own, keeps its two streams and
  its exit code, and holds --timeout itself. wsl-toolkit logs ID --follow,
  wait ID and stop ID reach it, and the first of them to see it end delivers
  its --artifacts.
`

type baseExecFlags struct {
	command    string
	commandB64 string
	scriptFile string
	env        stringList
	envFile    string
	dir        string
	timeout    time.Duration
	asRoot     bool
	viaHelper  bool
	privateNet bool
	detach     bool
	asJSON     bool
	// user, inputs, artifacts, stdin and here are what `run` gives a job and
	// `base exec` lacked. WSL-123, WSL-124.
	user      string
	inputs    stringList
	artifacts string
	stdin     bool
	here      bool
}

func (e *baseExecFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&e.command, "c", "", "the POSIX shell command to run in the base")
	fs.StringVar(&e.commandB64, "command-base64", "", "the command as base64 of its bytes, for a caller that must keep every shell away from it")
	fs.StringVar(&e.scriptFile, "script", "", "a file on this machine whose bytes are the command, or - to read them from stdin")
	fs.Var(&e.env, "env", "NAME=VALUE exported to the command. Repeatable, and applied after --env-file")
	fs.StringVar(&e.envFile, "env-file", "", "a file on this machine of NAME=VALUE lines exported to the command")
	fs.StringVar(&e.dir, "dir", "~", "working directory: a guest path, or a Windows directory mapped to where the base sees it. Default is the guest home. A directory the command cannot enter is refused")
	fs.DurationVar(&e.timeout, "timeout", 30*time.Minute, "how long the command may run. 0 means no deadline")
	fs.BoolVar(&e.asRoot, "root", false, "run as root instead of the configured account")
	fs.BoolVar(&e.viaHelper, "via-helper", false, "prove the helper refusal for arbitrary base commands")
	fs.BoolVar(&e.privateNet, "private-net", false, "run in a network namespace of the account's own, with the Windows host and the private ranges refused. ⛔ No container can run inside it")
	fs.BoolVar(&e.detach, "detach", false, "start the command with no client attached and answer with a session id. wsl-toolkit logs ID --follow, wait ID and stop ID reach it")
	fs.BoolVar(&e.asJSON, "json", false, "with --detach, write the session as a structured answer")
	fs.StringVar(&e.user, "user", "", "run as this account of the base instead of the configured one. The base must have it")
	fs.Var(&e.inputs, "input", "NAME=FILE: a file on this machine, read from the current directory, placed at NAME in a directory of the command's own that WSL_TOOLKIT_IN names. Repeatable")
	fs.StringVar(&e.artifacts, "artifacts", "", "a directory on this machine to receive what the command leaves in the directory WSL_TOOLKIT_OUT names. It is made only when something arrives")
	fs.BoolVar(&e.stdin, "stdin", false, "pass this process's stdin to the command, a pipe or a file. Refused when stdin is a terminal")
	fs.BoolVar(&e.here, "here", false, "start in the guest directory this Windows directory is seen at: its grant, or its drive mount when base.automount is ro or rw")
}

func (e baseExecFlags) request(cfg toolkit.Config) (toolkit.ExecRequest, error) {
	payload, err := guestScript(e.command, e.commandB64, e.scriptFile)
	if err != nil {
		return toolkit.ExecRequest{}, err
	}
	// ⭐ THE VARIABLES BECOME PART OF THE COMMAND, through the one writer `distro`
	// uses, so they reach it the same way attached, detached and in a private
	// namespace, and no value is ever typed into a shell. WSL-111.
	pairs, err := e.envPairs(cfg)
	if err != nil {
		return toolkit.ExecRequest{}, err
	}
	payload = toolkit.ComposePayload(payload, pairs, false)
	if e.timeout < 0 {
		return toolkit.ExecRequest{}, fmt.Errorf("--timeout %s is negative. Pass 0 for no deadline, or a positive duration", e.timeout)
	}
	dir, err := e.guestDir(cfg)
	if err != nil {
		return toolkit.ExecRequest{}, err
	}
	user, err := e.account(cfg)
	if err != nil {
		return toolkit.ExecRequest{}, err
	}
	var env map[string]string
	if e.privateNet {
		// ⛔ ROOT IS REFUSED HERE, and it is not squeamishness. The namespace is
		// what confines the ACCOUNT; a root payload can unmount, re-mount and
		// re-enter whatever it likes, so wrapping it would put a boundary around
		// something that can step over it and report that it had been confined.
		if user == "root" {
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

// account is who the command runs as: the configured account, root with --root,
// or the account --user names. WSL-123.
func (e baseExecFlags) account(cfg toolkit.Config) (string, error) {
	switch {
	case e.asRoot && e.user != "":
		return "", errors.New("--root and --user both name who the command runs as. --root is --user root, so pass one")
	case e.asRoot:
		return "root", nil
	case e.user != "":
		if err := toolkit.ValidateBaseAccountName(e.user); err != nil {
			return "", err
		}
		return e.user, nil
	}
	return cfg.Base.User, nil
}

// guestDir is where the command starts: the guest home, a guest path, or a
// Windows directory mapped to where the base sees it. WSL-124.
//
// ⭐ A WINDOWS PATH IS READ AS ONE BEFORE GIT BASH'S RULE IS ASKED. A guest path
// that MSYS rewrote starts at the MSYS root or the temporary directory, and is
// still refused with the rewrite named; any other drive path is a directory on
// this machine.
func (e baseExecFlags) guestDir(cfg toolkit.Config) (string, error) {
	dir := strings.TrimSpace(e.dir)
	if e.here {
		if dir != "~" {
			return "", errors.New("--here and --dir both name where the command starts, so pass one")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return windowsDirInGuest(cfg, cwd, "--here")
	}
	if windowsDirArg(dir) {
		return windowsDirInGuest(cfg, dir, "--dir")
	}
	if err := gitBashRewrite("--dir", dir, guestPath); err != nil {
		return "", err
	}
	if dir != "~" && !strings.HasPrefix(dir, "/") {
		return "", fmt.Errorf("--dir %q is not a guest absolute path or a Windows directory. Pass ~, a path beginning with /, or a drive path such as C:\\work\\project", e.dir)
	}
	return dir, nil
}

// windowsDirArg says whether a --dir value names a Windows directory: a drive
// letter, a colon and a separator, and not a guest path Git Bash rewrote into
// that shape.
func windowsDirArg(v string) bool {
	if len(v) < 3 || v[1] != ':' || (v[2] != '\\' && v[2] != '/') {
		return false
	}
	if c := v[0]; !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') {
		return false
	}
	return strings.TrimSpace(os.Getenv("MSYSTEM")) == "" || msysOrigin(v) == ""
}

// windowsDirInGuest answers where the base sees a Windows directory: through the
// grant that covers it when base.automount is off, and at its drive's mount when
// the drives are mounted. WSL-124.
//
// ⛔ A DIRECTORY NO GRANT COVERS IS REFUSED, with the grant that would cover it.
// The base cannot see it, and a command started somewhere else instead would
// answer about the wrong tree.
func windowsDirInGuest(cfg toolkit.Config, winDir, flagName string) (string, error) {
	automount, err := toolkit.NormalizeAutomount(cfg.Base.Automount)
	if err != nil {
		return "", err
	}
	if automount != toolkit.AutomountOff {
		// ⚠ WindowsPathToGuest reads a drive path by its own grammar on any host,
		// and resolves only what is not one.
		return toolkit.WindowsPathToGuest(winDir)
	}
	dir, err := toolkit.GrantedGuestDir(cfg, winDir)
	if errors.Is(err, toolkit.ErrNotGranted) {
		return "", fmt.Errorf("%s %s is not granted to %s, and base.automount is off, so the base cannot see it. Grant it, then run this again:\n  %s",
			flagName, winDir, cfg.Base.Name, toolkit.GrantCommand(winDir))
	}
	return dir, err
}

// execInputs reads every --input NAME=FILE, each file from the current
// directory, as --script and --env-file are. WSL-124.
func (e baseExecFlags) execInputs() ([]toolkit.JobInput, error) {
	var out []toolkit.JobInput
	for _, v := range e.inputs {
		name, file, ok := strings.Cut(v, "=")
		if !ok || file == "" {
			return nil, fmt.Errorf("--input %q is not NAME=FILE: a name in the command's input directory, one equals sign, then a file on this machine", v)
		}
		if err := toolkit.ValidateInputName(name); err != nil {
			return nil, err
		}
		p, err := filepath.Abs(msysHostPath(file))
		if err != nil {
			return nil, fmt.Errorf("--input %s: %w", v, err)
		}
		out = append(out, toolkit.JobInput{Name: name, Path: p})
	}
	if err := toolkit.CheckInputs(out, toolkit.DefaultWorkspaceLimits()); err != nil {
		return nil, err
	}
	return out, nil
}

// artifactDir resolves --artifacts from the current directory, and refuses one
// that is not a project's. WSL-124.
func (e baseExecFlags) artifactDir() (string, error) {
	if e.artifacts == "" {
		return "", nil
	}
	p, err := filepath.Abs(msysHostPath(e.artifacts))
	if err != nil {
		return "", fmt.Errorf("--artifacts: %w", err)
	}
	if err := toolkit.AssertProjectPath("--artifacts resolved to", p); err != nil {
		return "", err
	}
	return p, nil
}

// stdinRefusal refuses --stdin where there is nothing to pass. WSL-124.
//
// ⛔ A TERMINAL IS REFUSED. This channel has no screen, so a command reading a
// console would wait on a person nobody told to type. `base shell` is the
// interactive path.
func (e baseExecFlags) stdinRefusal(console bool) error {
	switch {
	case !e.stdin:
		return nil
	case e.scriptFile == stdinScript:
		return errors.New("--stdin passes stdin to the command, and --script - has already read it as the command. Pass the command with -c or a --script file")
	case console:
		return errors.New("--stdin passes a pipe or a file to the command, and this stdin is a terminal. Pipe the bytes in, or run the command in " + toolkit.ToolCommand("base shell"))
	}
	return nil
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
	if err := opts.stdinRefusal(isConsole(os.Stdin)); err != nil {
		return exitCannot, err
	}
	req, err := opts.request(cfg)
	if err != nil {
		return exitCannot, err
	}
	inputs, err := opts.execInputs()
	if err != nil {
		return exitCannot, err
	}
	artifactDir, err := opts.artifactDir()
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
		return exitCannot, errNotRegistered(cfg.Base.Name)
	}
	if err := checkExecAccount(ctx, w, cfg, req.User, opts.privateNet); err != nil {
		return exitCannot, err
	}
	// ⭐ THE ACCOUNT'S ENGINE STATE IS CLEARED FIRST WHERE A RESTART OF WSL LEFT
	// IT STALE, attached or detached, so a command that reaches the engine this
	// tool's jobs run on is not refused. It is that account's engine whichever
	// account runs the command. WSL-137.
	toolkit.ClearStaleRunState(ctx, w, cfg.Base.Name, cfg.Base.User, note)
	if opts.detach {
		stdin, err := opts.detachedStdin(os.Stdin)
		if err != nil {
			return exitCannot, err
		}
		return detachSession(ctx, w, req, opts, inputs, artifactDir, stdin)
	}
	run := toolkit.BaseExec{
		Distro: req.Distro, User: req.User, Dir: req.Dir, Script: req.Script, Env: req.Env,
		Timeout: req.Timeout, Inputs: inputs, ArtifactDir: artifactDir,
		Stdout: os.Stdout, Stderr: os.Stderr, Log: note,
	}
	if opts.stdin {
		run.Stdin = os.Stdin
	}
	res, runErr := toolkit.RunBaseExec(ctx, w, run)
	return attachedExecExit(res, runErr, req.User == cfg.Base.User, artifactDir)
}

// attachedExecExit is the exit an attached base exec answers: 2 for a directory
// the command could not enter, the command's own exit, and 1 for a command that
// succeeded and delivered nothing it was asked for. retryable says the account
// is the base's own, whose kept output `artifacts retry` can fetch. WSL-124.
func attachedExecExit(res toolkit.BaseExecResult, runErr error, retryable bool, artifactDir string) (int, error) {
	if errors.Is(runErr, toolkit.ErrDirNotEntered) {
		return exitCannot, runErr
	}
	code, err := baseExecResult(res.Exit, runErr)
	if res.ArtifactError != "" {
		logf("  ! artifacts: %s", res.ArtifactError)
		retained := "  ! the output is still in the guest at " + res.Retained
		if retryable {
			retained += ". Fetch it with: " + toolkit.ToolCommand("artifacts retry "+res.ID+" --to "+toolkit.QuoteForDisplay(artifactDir))
		}
		logf("%s", retained)
		// ⛔ THE COMMAND'S OWN FAILURE WINS, and a command that succeeded and
		// delivered nothing it was asked for exits 1, as a job does.
		if code == exitOK && err == nil {
			code = exitFailed
		}
	}
	return code, err
}

// detachedStdin is what a detached session's command reads: nil without
// --stdin, so the session keeps /dev/null, and the whole of r with it. WSL-124.
func (e baseExecFlags) detachedStdin(r io.Reader) ([]byte, error) {
	if !e.stdin {
		return nil, nil
	}
	return readDetachedStdin(r)
}

// checkExecAccount refuses an account the base does not have, and a private
// namespace for an account that is root by its uid. WSL-123.
func checkExecAccount(ctx context.Context, w *toolkit.Wsl, cfg toolkit.Config, user string, privateNet bool) error {
	if user == cfg.Base.User || user == "root" {
		return nil
	}
	uid, ok, err := toolkit.AccountUID(ctx, w, cfg.Base.Name, cfg.Base.User, user)
	if err != nil {
		return err
	}
	return accountRefusal(cfg.Base.Name, user, uid, ok, privateNet)
}

// accountRefusal is what the base's answer about an account refuses: one it
// lacks, and a private namespace for one that is root by its uid. WSL-123.
func accountRefusal(distro, user string, uid int, ok, privateNet bool) error {
	if !ok {
		return fmt.Errorf("%s has no account %q, so nothing ran. Make it as root first, for example: %s",
			distro, user, toolkit.ToolCommand("base exec --root -c 'useradd -m "+user+"'"))
	}
	if privateNet && uid == 0 {
		return fmt.Errorf("--private-net and --user %s are contradictory: %s is uid 0, guest root can leave the namespace it is put in, so the confinement would be a claim rather than a fact", user, user)
	}
	return nil
}

// maxDetachedStdin bounds what --stdin hands a detached session. It is a file in
// the session's directory, so it is read whole before the session starts.
const maxDetachedStdin = 64 << 20

// readDetachedStdin reads what a detached session's command will read.
//
// ⛔ READ WHOLE, BEFORE THE SESSION STARTS. A detached session has no client to
// relay a pipe through, so the bytes travel as a file in its directory.
func readDetachedStdin(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxDetachedStdin+1))
	if err != nil {
		return nil, fmt.Errorf("--stdin: reading stdin: %w", err)
	}
	if len(raw) > maxDetachedStdin {
		return nil, fmt.Errorf("--stdin with --detach carried more than %s, which travels whole as a file. Pass a larger file with --input", toolkit.HumanBytes(maxDetachedStdin))
	}
	if raw == nil {
		raw = []byte{}
	}
	return raw, nil
}

// detachSession starts a base exec with no client attached. WSL-95.
//
// ⛔ THE PAYLOAD IS THE ONE THE ATTACHED PATH WOULD SEND, environment and
// private network wrapping included, so --detach changes who waits and nothing
// about what runs. Its inputs, its output directory and its stdin travel in the
// same archive as the payload. WSL-124.
func detachSession(ctx context.Context, w *toolkit.Wsl, req toolkit.ExecRequest, opts baseExecFlags, inputs []toolkit.JobInput, artifactDir string, stdin []byte) (int, error) {
	home, err := toolkit.Home()
	if err != nil {
		return exitCannot, err
	}
	rec, err := toolkit.StartSession(ctx, w, home, toolkit.SessionStart{
		Distro: req.Distro, User: req.User, Dir: req.Dir,
		Payload: toolkit.SessionPayload(req), Timeout: opts.timeout,
		Inputs: inputs, ArtifactDir: artifactDir, Stdin: stdin,
	})
	if err != nil {
		return exitCannot, err
	}
	followCmd, waitCmd, stopCmd := jobHints(rec.ID)
	if opts.asJSON {
		doc := map[string]any{
			"schema": DetachedSchema, "id": rec.ID, "kind": "session",
			"distro": rec.Distro, "user": rec.User, "guest_dir": rec.GuestDir,
			"follow": followCmd,
			"wait":   waitCmd,
			"stop":   stopCmd,
		}
		if rec.ArtifactDir != "" {
			doc["artifact_dir"] = rec.ArtifactDir
		}
		return exitOK, writeJSON(doc)
	}
	fmt.Println(rec.ID)
	logf("  session %s runs in %s as %s, with no client attached. Follow: %s. Wait: %s. Stop: %s",
		rec.ID, rec.Distro, rec.User, followCmd, waitCmd, stopCmd)
	if rec.ArtifactDir != "" {
		logf("  its output reaches %s when %s or %s sees it end", rec.ArtifactDir, waitCmd, followCmd)
	}
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

// envPairs reads --env-file, then every --env, in that order, so a later value
// for a name wins the way a shell would make it win.
func (e baseExecFlags) envPairs(cfg toolkit.Config) ([]toolkit.EnvPair, error) {
	var pairs []toolkit.EnvPair
	if e.envFile != "" {
		// ⚠ READ AS ITS --script IS, from the current directory. `base exec` is
		// not a job, and a relative --env-file that followed the project while
		// its --script did not named two directories in one command. A contract
		// review found it.
		path, err := filepath.Abs(msysHostPath(e.envFile))
		if err != nil {
			return nil, fmt.Errorf("--env-file: %w", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("--env-file: %w", err)
		}
		filePairs, err := toolkit.ParseEnvFile(data, path)
		if err != nil {
			return nil, fmt.Errorf("--env-file %w", err)
		}
		pairs = append(pairs, filePairs...)
	}
	for _, raw := range e.env {
		if err := gitBashRewrite("--env", raw, guestText); err != nil {
			return nil, err
		}
		p, err := toolkit.ParseEnvPair(raw)
		if err != nil {
			return nil, fmt.Errorf("--env %w", err)
		}
		pairs = append(pairs, p)
	}
	return pairs, nil
}
