// Command wsl-toolkit is the single-file entry point for Linux work on a Windows
// host. It carries wsl-toolkit.ps1 inside itself and adds a host survey, one
// owned WSL distribution running a rootless engine, container jobs that get a
// COPY of a workspace and never a mount, a fleet runner, and a cleanup that
// removes only what this executable made.
//
// tools/windows/wsl-toolkit/wsl-toolkit.md is the manual.
//
// ⛔ EVERYTHING THIS PROGRAM SAYS GOES TO STDERR. stdout carries the answer
// alone, so a caller can assign it.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/script"
	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// Exit codes, and they mean four different things. `script` and `run` forward
// somebody else's code instead.
//
// ⛔ ONE HOME, and it is toolkit, where the meanings are written down. They
// were literals in the package that produces a result and named constants here,
// which is a value in two places with nothing checking that they agree.
const (
	exitOK      = toolkit.ExitOK
	exitFailed  = toolkit.ExitFailed
	exitCannot  = toolkit.ExitCannot
	exitTimeout = toolkit.ExitTimeout
)

var quiet bool

// The helper package reads the product version through this hook rather than
// importing the script package, which would make the two depend on each other
// for one string.
func init() { toolkit.ScriptVersion = script.Version }

// logf writes progress. ⛔ Never to stdout.
func logf(format string, args ...any) {
	if quiet {
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func note(s string) { logf("  %s", s) }

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:]))
}

func usage() string {
	return strings.Join([]string{
		"wsl-toolkit " + versionString(),
		"",
		"  doctor      what this host is and what is really installed, resolved rather than guessed",
		"  script      run the embedded wsl-toolkit.ps1, forwarding every argument unchanged",
		"  base        the one WSL distribution this tool owns, which hosts a rootless engine",
		"  images      the container catalog, fully qualified",
		"  run         one command in one container, with a COPY of a workspace and no host mount",
		"  matrix      one command across a set of images, commissioned and decommissioned together",
		"  resources   what this tool is holding, and what the machine is holding that is not its",
		"  gc          remove what this tool made. Reports first; --apply acts",
		"  logs        the complete output a job produced, past whatever the answer kept",
		"  helper      the opt-in local helper, for a caller that cannot reach wsl.exe itself",
		"  config      where the configuration is, and what it currently says",
		"  ready       one answer to whether this agent can run isolated Linux jobs here",
		"  selfupdate  move this executable to a published release, verifying it first",
		"  version     the product version, which is the embedded script's",
		"",
		"Global: --instance N  one isolated instance: distribution wsl-toolkit-N and",
		"                      its own state. `auto` picks the lowest free one.",
		"                      Also WSL_TOOLKIT_INSTANCE.",
		"        --home DIR    the state directory (also WSL_TOOLKIT_HOME)",
		"        --config F    a configuration file, ahead of every search step",
		"        --quiet       progress off. Answers still go to stdout",
		"",
		"Every command takes --json where an answer has structure.",
		"Progress goes to stderr; stdout carries the answer alone.",
	}, "\n")
}

func versionString() string {
	v, err := script.Version()
	if err != nil {
		return "(the embedded script's version could not be read)"
	}
	return v
}

func run(ctx context.Context, args []string) int {
	// The global flags are read before the subcommand so --home applies to the
	// state directory every subcommand resolves at startup.
	var rest []string
	instance := ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--quiet" || args[i] == "-q":
			quiet = true
		case args[i] == "--home":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "wsl-toolkit: --home needs a directory")
				return exitCannot
			}
			i++
			os.Setenv("WSL_TOOLKIT_HOME", args[i])
		case strings.HasPrefix(args[i], "--home="):
			os.Setenv("WSL_TOOLKIT_HOME", strings.TrimPrefix(args[i], "--home="))
		case args[i] == "--instance":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "wsl-toolkit: --instance needs a name, or "+toolkit.InstanceAuto)
				return exitCannot
			}
			i++
			instance = args[i]
		case strings.HasPrefix(args[i], "--instance="):
			instance = strings.TrimPrefix(args[i], "--instance=")
		case args[i] == "--config":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "wsl-toolkit: --config needs a file")
				return exitCannot
			}
			i++
			toolkit.ExplicitConfigPath = args[i]
		case strings.HasPrefix(args[i], "--config="):
			toolkit.ExplicitConfigPath = strings.TrimPrefix(args[i], "--config=")
		default:
			rest = append(rest, args[i])
			// ⛔ Everything after the subcommand belongs to it. `script` forwards
			// verbatim, and a scanner that kept reading would eat one of its
			// arguments.
			if !strings.HasPrefix(args[i], "-") {
				rest = append(rest, args[i+1:]...)
				i = len(args)
			}
		}
	}
	if len(rest) == 0 || rest[0] == "help" || rest[0] == "--help" || rest[0] == "-h" {
		fmt.Fprintln(os.Stderr, usage())
		return exitOK
	}
	// ⛔ THE INSTANCE IS RESOLVED BEFORE THE SUBCOMMAND RUNS, because it decides
	// the state directory every subcommand reads at startup and the distribution
	// name the configuration defaults to. Resolving it inside a command would be
	// resolving it after something had already read the wrong home. WSL-43.
	if code, err := applyInstance(ctx, instance); err != nil {
		fmt.Fprintln(os.Stderr, "wsl-toolkit: "+err.Error())
		return code
	}
	cmd, cmdArgs := rest[0], rest[1:]
	var err error
	var code int
	switch cmd {
	case "version", "--version":
		code, err = cmdVersion(cmdArgs)
	case "doctor":
		code, err = cmdDoctor(ctx, cmdArgs)
	case "script":
		code, err = cmdScript(ctx, cmdArgs)
	case "base":
		code, err = cmdBase(ctx, cmdArgs)
	case "images":
		code, err = cmdImages(cmdArgs)
	case "run":
		code, err = cmdRun(ctx, cmdArgs)
	case "matrix":
		code, err = cmdMatrix(ctx, cmdArgs)
	case "resources":
		code, err = cmdResources(ctx, cmdArgs)
	case "gc":
		code, err = cmdGC(ctx, cmdArgs)
	case "logs":
		code, err = cmdLogs(cmdArgs)
	case "helper":
		code, err = cmdHelper(ctx, cmdArgs)
	case "config":
		code, err = cmdConfig(cmdArgs)
	case "ready":
		code, err = cmdReady(ctx, cmdArgs)
	case "selfupdate":
		code, err = cmdSelfUpdate(ctx, cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "wsl-toolkit: %q is not a command\n\n%s\n", cmd, usage())
		return exitCannot
	}
	if errors.Is(err, flag.ErrHelp) {
		// ⭐ ASKING FOR HELP IS NOT A FAILURE. The flag package has already
		// printed the defaults by the time it returns this, so repeating it as
		// "wsl-toolkit: flag: help requested" beside exit 2 tells a reader their
		// correct command was wrong.
		return exitOK
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wsl-toolkit: "+err.Error())
		if errors.Is(err, toolkit.ErrWslDenied) {
			fmt.Fprintln(os.Stderr, deniedAdvice)
		}
		if code == exitOK {
			code = exitCannot
		}
	}
	return code
}

// applyInstance turns --instance, WSL_TOOLKIT_INSTANCE or a working tree's
// pointer into the two things an instance IS: a state directory and a
// distribution name.
//
// ⭐ IT SETS THE ENVIRONMENT RATHER THAN THREADING A VALUE. Everything below
// resolves the state directory through toolkit.Home, and the configuration
// defaults its distribution name from the same selection, so setting both here
// is what makes it impossible for one to move without the other. WSL-43.
//
// ⚠ THE POINTER IS READ ONLY WHERE NOTHING ELSE SAID. A flag, then the
// environment, then a `.wsl-toolkit/instance.json` in the working directory or a
// parent, then the default. A pointer that overrode an explicit flag would be a
// file in a checkout silently deciding which machine an agent talks to.
func applyInstance(ctx context.Context, asked string) (int, error) {
	if asked == "" && strings.TrimSpace(os.Getenv(toolkit.InstanceEnv)) == "" {
		cwd, err := os.Getwd()
		if err == nil {
			named, at, err := toolkit.ReadPointer(cwd)
			if err != nil {
				return exitCannot, err
			}
			if at != "" {
				asked = named
				note("instance " + describeInstance(named) + ", from " + at)
			}
		}
	}
	if asked == "" && strings.TrimSpace(os.Getenv(toolkit.InstanceEnv)) == "" {
		return exitOK, nil
	}
	inst, err := toolkit.ResolveInstance(ctx, asked)
	if err != nil {
		return exitCannot, err
	}
	if inst.Name == toolkit.DefaultInstance {
		return exitOK, nil
	}
	os.Setenv("WSL_TOOLKIT_HOME", inst.Home)
	os.Setenv(toolkit.InstanceEnv, inst.Name)
	// ⛔ The DISTRIBUTION is set through the configuration's default rather than
	// written to a file. An instance that rewrote somebody's config.json to
	// record its own name would make the selection sticky, and a flag is not a
	// setting.
	toolkit.SelectedInstance = inst
	note("instance " + inst.Name + ": distribution " + inst.Distro + ", state " + inst.Home)
	return exitOK, nil
}

func describeInstance(name string) string {
	if name == toolkit.DefaultInstance {
		return "the default"
	}
	return name
}

// deniedAdvice names the two ways forward. A caller meeting E_ACCESSDENIED from
// wsl.exe cannot tell a sandbox from a broken install, and Windows says neither.
const deniedAdvice = `
  This process may call wsl.exe and was refused. That is a property of how this
  process was started, not of WSL: the same call made through the approval path
  a harness offers usually succeeds.

  Two ways forward, and both are supported:
    1. Make this call the way this session normally asks for permission.
    2. Start the helper ONCE through that path and let it do the WSL work:
         wsl-toolkit helper serve --detach
       Later commands find it automatically and need no further approval.
       wsl-toolkit helper status says whether one is listening.`

// flagSets records every flag set this process built, so a test can ask what the
// surface actually is rather than restating it.
//
// ⭐ IT EXISTS TO STOP THE MANUAL DRIFTING FROM THE BINARY. A claim audit
// found `--user` in the code and in neither manual once already; a check that
// reads the real flag sets cannot be got wrong by anybody adding a flag, because
// the flag registers itself here by being created.
var flagSets = struct {
	sync.Mutex
	byName map[string]*flag.FlagSet
}{byName: map[string]*flag.FlagSet{}}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	flagSets.Lock()
	flagSets.byName[name] = fs
	flagSets.Unlock()
	return fs
}

// parseArgs parses one subcommand's flags and refuses what flag.Parse tolerates.
//
// ⛔ GO'S FLAG PACKAGE STOPS AT THE FIRST ARGUMENT THAT IS NOT A FLAG and leaves
// everything after it in Args(). So `run --image alpine -c 'echo hi' stray
// --via-helper` ran the command, ignored the stray word, and ignored the routing
// flag written after it, all at exit 0. A caller who put a flag last got a job
// on a route they did not choose and nothing said so.
//
// ⭐ Every subcommand parses through here, so one added later cannot forget the
// check. `doctor` had it and the other seven did not, which is exactly the shape
// a guard applied per call site produces.
func parseArgs(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	extra := fs.Args()
	if len(extra) == 0 {
		return nil
	}
	// ⚠ The stray word is what STOPPED the parse, so nothing after it was
	// looked at either. Naming only the first would have a caller fix a
	// three-word mistake one word per run.
	msg := fmt.Sprintf("%s takes flags, not positional arguments, and %q is one", fs.Name(), extra[0])
	if len(extra) > 1 {
		msg += fmt.Sprintf(". Everything after it was never parsed, starting with %q", extra[1])
	}
	return errors.New(msg)
}

// writeJSON puts a structured answer on stdout. It is the only thing in this
// program that writes there, apart from a forwarded child's own stream.
func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func cmdVersion(args []string) (int, error) {
	fs := newFlagSet("version")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	v, err := script.Version()
	if err != nil {
		return exitCannot, err
	}
	if *asJSON {
		return exitOK, writeJSON(map[string]any{
			"schema":            "wsl-toolkit-version/1",
			"version":           v,
			"script_sha256":     script.Digest(),
			"script_bytes":      script.Size(),
			"script_reversible": script.StoredIsReversible(),
		})
	}
	fmt.Println(v)
	return exitOK, nil
}

func loadConfig() (toolkit.Config, error) {
	cfg, err := toolkit.LoadConfig()
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}
