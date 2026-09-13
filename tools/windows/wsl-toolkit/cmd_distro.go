// SPDX-License-Identifier: 0BSD

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const distroUsage = `wsl-toolkit distro <list|new|run|enter|remove|purge|snapshot|replay|compare>

  A throwaway distribution is a whole WSL distribution made from an image or a
  rootfs archive, for a job whose subject is the distribution itself: its init,
  its /etc/wsl.conf, what a login shell sees. Every one is named eph-... and
  lives under the state directory, and the tool acts only on those.

  list       what this state directory made, what else carries the prefix, and
             what failed creations and snapshots left on disk
  new        import one from --image or --tarball, and optionally run a command
  run        run a non-interactive POSIX command in one
  enter      attach an interactive login shell to one
  remove     unregister one and delete its directory
  purge      remove every one this state directory made. A plan without --apply
  snapshot   export one to an archive that new --tarball TAG imports again
  replay     render a recorded wsl-toolkit-event/1 log again
  compare    measure two recorded runs side by side; report, never judge
`

func cmdDistro(ctx context.Context, args []string) (int, error) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, distroUsage)
		return exitCannot, nil
	}
	sub, rest := args[0], args[1:]
	var run func(context.Context, []string) (int, error)
	switch sub {
	case "list":
		run = cmdDistroList
	case "new":
		run = cmdDistroNew
	case "run":
		run = cmdDistroRun
	case "enter":
		run = cmdDistroEnter
	case "remove":
		run = cmdDistroRemove
	case "purge":
		run = cmdDistroPurge
	case "snapshot":
		run = cmdDistroSnapshot
	case "replay":
		run = cmdDistroReplay
	case "compare":
		run = cmdDistroCompare
	default:
		fmt.Fprint(os.Stderr, distroUsage)
		return exitCannot, fmt.Errorf("%q is not a distro subcommand", sub)
	}
	code, err := run(ctx, rest)
	return code, withoutHelperAdvice(err)
}

// withoutHelperAdvice keeps a WSL refusal from being answered with the helper.
//
// ⛔ THE HELPER DOES NOT SERVE THROWAWAY DISTRIBUTIONS, so the advice main
// prints for a refused wsl.exe would send a caller to start something that
// cannot help. The refusal is kept and the advice is replaced.
func withoutHelperAdvice(err error) error {
	if err == nil || !errors.Is(err, toolkit.ErrWslDenied) {
		return err
	}
	return fmt.Errorf("%s. The helper does not serve throwaway distributions, so make this call through the path this session uses to approve wsl.exe", err.Error())
}

func newThrowaways() (*toolkit.Throwaways, error) {
	if _, err := loadConfig(); err != nil {
		return nil, err
	}
	return toolkit.NewThrowaways(note)
}

// targetName is the one rule every subcommand that names an existing
// distribution applies: the prefix is added when it was left off, and nothing
// else about the name is changed.
func targetName(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("--name is required. wsl-toolkit distro list names the throwaway distributions")
	}
	name, _, err := toolkit.ThrowawayName(value, "")
	return name, err
}

func cmdDistroList(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("distro list")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	t, err := newThrowaways()
	if err != nil {
		return exitCannot, err
	}
	rep, err := t.List(ctx)
	if err != nil {
		return exitCannot, err
	}
	if *asJSON {
		return exitOK, writeJSON(rep)
	}
	fmt.Fprintf(os.Stderr, "==> Throwaway distributions under %s\n", rep.Dir)
	if len(rep.Owned) == 0 {
		fmt.Fprintln(os.Stderr, "  none")
	}
	for _, d := range rep.Owned {
		state := "stopped"
		if d.Running {
			state = "running"
		}
		size := "not measured"
		if d.DiskKnown {
			size = toolkit.HumanBytes(d.DiskBytes)
		}
		from := "no origin record"
		if d.Origin != nil {
			from = firstNonEmpty(d.Origin.Image, snapshotLabel(d.Origin.Snapshot), d.Origin.Tarball)
		}
		// ⭐ THE NAME IS THE ONE THING ON STDOUT, one per line, so a caller can
		// read the list without parsing the prose around it.
		fmt.Println(d.Name)
		fmt.Fprintf(os.Stderr, "  %-8s %-12s %s\n", state, size, from)
		if d.Creating != nil {
			fmt.Fprintf(os.Stderr, "  ! another run is still creating it, since %s\n", d.Creating.Format(time.RFC3339))
		}
	}
	if len(rep.Elsewhere) > 0 {
		fmt.Fprintln(os.Stderr, "\n==> Carrying the prefix with a disk somewhere else. Named, never touched")
		for _, d := range rep.Elsewhere {
			fmt.Fprintf(os.Stderr, "  %-28s %s\n", d.Name, d.Disk)
		}
	}
	if len(rep.Others) > 0 {
		fmt.Fprintln(os.Stderr, "\n==> Every other registered distribution. Never touched")
		for _, d := range rep.Others {
			tag := ""
			if d.Protected {
				tag = "  [a container runtime's own]"
			}
			fmt.Fprintf(os.Stderr, "  %-28s %s%s\n", d.Name, runningWord(d.Running), tag)
		}
	}
	if len(rep.Leftovers) > 0 {
		fmt.Fprintln(os.Stderr, "\n==> Left by a creation or an export that did not finish. distro purge --apply removes them")
		for _, f := range rep.Leftovers {
			fmt.Fprintf(os.Stderr, "  %-9s %-12s %s\n", f.Kind, toolkit.HumanBytes(f.Bytes), f.Path)
		}
	}
	if len(rep.Snapshots) > 0 {
		fmt.Fprintln(os.Stderr, "\n==> Snapshots. Kept by purge; distro new --tarball TAG imports one")
		for _, s := range rep.Snapshots {
			fmt.Fprintf(os.Stderr, "  %-24s %-12s %s\n", s.Name, toolkit.HumanBytes(s.Bytes), s.Path)
		}
	}
	return exitOK, nil
}

func snapshotLabel(tag string) string {
	if tag == "" {
		return ""
	}
	return "snapshot " + tag
}

func runningWord(running bool) string {
	if running {
		return "running"
	}
	return "stopped"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// commandFlags are what new and run share, so the two cannot drift into
// accepting different spellings of one command.
type commandFlags struct {
	command    string
	commandB64 string
	scriptFile string
	verbatim   bool
	env        stringList
	envFile    string
	user       string
	userEnv    bool
	timeout    time.Duration
	log        logFlags
}

func (c *commandFlags) bind(fs *flag.FlagSet, defaultTimeout time.Duration) {
	fs.StringVar(&c.command, "c", "", "the POSIX shell command to run, as a login shell whose stdin is /dev/null")
	fs.StringVar(&c.commandB64, "command-base64", "", "the command as base64 of its bytes, for a caller that must keep every shell away from it")
	fs.StringVar(&c.scriptFile, "script", "", "a file on this machine whose bytes are the command")
	fs.BoolVar(&c.verbatim, "verbatim", false, "send the command's bytes exactly, without turning CRLF into LF or dropping a byte order mark")
	fs.Var(&c.env, "env", "NAME=VALUE assigned before the command. Repeatable, applied after --env-file; @hostaddress in VALUE is resolved at run time")
	fs.StringVar(&c.envFile, "env-file", "", "a file of NAME=VALUE lines assigned before the command. Blank lines and lines starting with # are skipped, and @hostaddress in VALUE is resolved as it is for --env")
	fs.StringVar(&c.user, "user", "root", "the account the command runs as")
	fs.BoolVar(&c.userEnv, "user-env", false, "prepare a private runtime directory and a deduplicated PATH before the command, without sourcing an account's rc files")
	fs.DurationVar(&c.timeout, "timeout", defaultTimeout, "how long the command may run before the distribution is terminated and the answer is 124. 0 means no deadline")
	c.log.bind(fs)
}

// commandPayload is a caller's command, read and repaired, with its environment.
type commandPayload struct {
	script []byte
	env    []toolkit.EnvPair
	source string
}

// payload reads the one command a caller named, or reports that none was named,
// which is different from an empty command.
//
// ⛔ EVERY SPELLING GETS THE SAME REPAIR, and --verbatim turns it off for every
// spelling. A command assembled on Windows carries CRLF whichever flag carried it,
// and one channel repairing its payload while its sibling does not is the
// one-gated-door shape.
func (c *commandFlags) payload(cfg toolkit.Config) (commandPayload, error) {
	var p commandPayload
	var given []string
	for flagName, present := range map[string]bool{"-c": c.command != "", "--command-base64": c.commandB64 != "", "--script": c.scriptFile != ""} {
		if present {
			given = append(given, flagName)
		}
	}
	if len(given) == 0 {
		switch {
		case len(c.env) > 0 || c.envFile != "":
			return p, errors.New("--env and --env-file set variables for a command, and no command was given")
		case c.userEnv:
			return p, errors.New("--user-env prepares an environment for a command, and no command was given")
		case c.verbatim:
			return p, errors.New("--verbatim sends a command's bytes exactly, and no command was given")
		}
		return p, nil
	}
	if len(given) > 1 {
		return p, errors.New("-c, --command-base64 and --script are three spellings of one command, so exactly one may be passed")
	}
	var raw []byte
	switch {
	case c.command != "":
		p.source, raw = "the -c command", []byte(c.command)
		if !c.verbatim {
			raw = append(raw, '\n')
		}
	case c.commandB64 != "":
		decoded, err := base64.StdEncoding.Strict().DecodeString(c.commandB64)
		if err != nil {
			return p, fmt.Errorf("--command-base64 is not valid base64: %w", err)
		}
		p.source, raw = "the --command-base64 command", decoded
	default:
		path, err := pathFromProject(cfg, c.scriptFile)
		if err != nil {
			return p, fmt.Errorf("--script: %w", err)
		}
		if raw, err = os.ReadFile(path); err != nil {
			return p, fmt.Errorf("--script: %w", err)
		}
		p.source = path
	}
	if c.verbatim {
		if toolkit.IsUTF16(raw) {
			return p, fmt.Errorf("%s is UTF-16, and --verbatim cannot send it: the command channel cannot carry the NUL byte after nearly every character. Save it as UTF-8", p.source)
		}
		if bytes.IndexByte(raw, 0) >= 0 {
			return p, fmt.Errorf("%s: %w", p.source, toolkit.ErrPayloadNUL)
		}
		if bytes.IndexByte(raw, '\r') >= 0 {
			note(p.source + " carries carriage returns and --verbatim was passed, so they are sent as they are. /bin/sh reads a carriage return as part of the last word on a line")
		}
		p.script = raw
	} else {
		repaired, rep, err := toolkit.RepairGuestScriptReport(raw)
		if err != nil {
			return p, fmt.Errorf("%s: %w", p.source, err)
		}
		if bytes.IndexByte(repaired, 0) >= 0 {
			return p, fmt.Errorf("%s: %w", p.source, toolkit.ErrPayloadNUL)
		}
		if rep.BOMRemoved {
			note(p.source + " carried a UTF-8 byte order mark, which is left out of the copy being sent")
		}
		if rep.CRLF > 0 {
			note(fmt.Sprintf("%s has %d CRLF line ending(s), and the copy being sent uses LF. Nothing on disk was written. --verbatim sends the bytes exactly", p.source, rep.CRLF))
		}
		p.script = repaired
	}
	if c.envFile != "" {
		path, err := pathFromProject(cfg, c.envFile)
		if err != nil {
			return p, fmt.Errorf("--env-file: %w", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return p, fmt.Errorf("--env-file: %w", err)
		}
		pairs, err := toolkit.ParseEnvFile(data, path)
		if err != nil {
			return p, fmt.Errorf("--env-file %w", err)
		}
		p.env = append(p.env, pairs...)
	}
	for _, raw := range c.env {
		pair, err := toolkit.ParseEnvPair(raw)
		if err != nil {
			return p, fmt.Errorf("--env %w", err)
		}
		p.env = append(p.env, pair)
	}
	if err := expandHostAddress(p.env, toolkit.ResolveHostAddress); err != nil {
		return p, err
	}
	return p, nil
}

// expandHostAddress replaces @hostaddress in every value, resolving it once.
//
// ⭐ THE ONE SUBSTITUTION THAT HAPPENS, and only inside values. A caller wiring
// a guest to a fixture on this host needs an address WSL assigns and changes, and
// `wsl-toolkit hostaddress` already answers it without creating anything.
func expandHostAddress(env []toolkit.EnvPair, resolve func() (toolkit.HostAddress, error)) error {
	var addr string
	for i := range env {
		if !strings.Contains(env[i].Value, "@hostaddress") {
			continue
		}
		if addr == "" {
			ans, err := resolve()
			if err != nil {
				return fmt.Errorf("resolve @hostaddress in --env %s: %w", env[i].Name, err)
			}
			addr = ans.Address
		}
		env[i].Value = strings.ReplaceAll(env[i].Value, "@hostaddress", addr)
		note(fmt.Sprintf("--env %s: @hostaddress resolved to %s. WSL assigns that address and changes it, so it is read at run time and not recorded", env[i].Name, addr))
	}
	return nil
}

// tarballArg resolves --tarball the way every other path flag resolves, and
// leaves a bare word that names no file as a snapshot tag for the lifecycle.
//
// ⚠ A FILE WINS OVER A TAG OF THE SAME SPELLING, so a caller holding an archive
// called `ready` in the project directory gets that archive and not a snapshot.
func tarballArg(cfg toolkit.Config, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	resolved, err := pathFromProject(cfg, value)
	if err != nil {
		return "", err
	}
	if fileExists(resolved) || strings.ContainsAny(value, `/\`) {
		return resolved, nil
	}
	if _, tagErr := toolkit.SnapshotTag(value); tagErr == nil {
		return value, nil
	}
	return resolved, nil
}

func cmdDistroNew(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("distro new")
	var c commandFlags
	c.bind(fs, 30*time.Minute)
	image := fs.String("image", "", "a catalog id or a fully qualified reference to import")
	tarball := fs.String("tarball", "", "a rootfs archive on this machine, or the tag of a snapshot this state directory holds")
	name := fs.String("name", "", "the distribution's name. The eph- prefix is added when it is left off. Empty draws one")
	systemd := fs.Bool("systemd", false, "boot it with systemd as PID 1, and refuse an image that cannot")
	ociEnv := fs.Bool("oci-env", false, "carry the image's ENV and WORKDIR into /etc/profile.d, where a login shell reads them")
	reuse := fs.Bool("reuse", false, "run in the newest distribution this state directory built from the same --image rather than importing another, and say so")
	ephemeral := fs.Bool("ephemeral", false, "remove the distribution once the command ends")
	probeTimeout := fs.Duration("probe-timeout", toolkit.DefaultProbeTimeout, "how long each question this tool asks the new distribution may take: the smoke probe, systemd and the image configuration. 5s to 1h")
	dryRun := fs.Bool("dry-run", false, "validate every option and print the plan, changing nothing in WSL and opening no log file")
	asJSON := fs.Bool("json", false, "write a structured answer. The command's output then goes to stderr")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	p, err := c.payload(cfg)
	if err != nil {
		return exitCannot, err
	}
	logSettings, err := c.log.settings(cfg)
	if err != nil {
		return exitCannot, err
	}
	liveOut := io.Writer(os.Stdout)
	if *asJSON {
		// ⛔ UNDER --json STDOUT CARRIES THE ANSWER ALONE, so the command's own
		// stdout is written where every other word this program says goes.
		liveOut = os.Stderr
	}
	spec := toolkit.ThrowawaySpec{
		Name: *name, User: c.user, Script: p.script, Env: p.env, UserEnv: c.userEnv, Timeout: c.timeout,
		Systemd: *systemd, OciEnv: *ociEnv, Reuse: *reuse, Ephemeral: *ephemeral, ProbeTimeout: *probeTimeout,
		Stdout: liveOut, Stderr: os.Stderr,
	}
	if *image != "" {
		if spec.Image, err = resolveImage(cfg, *image); err != nil {
			return exitCannot, err
		}
	}
	if spec.Tarball, err = tarballArg(cfg, *tarball); err != nil {
		return exitCannot, fmt.Errorf("--tarball: %w", err)
	}
	if logSettings.Active() && p.script == nil {
		return exitCannot, errors.New("the logging options observe a command, and no command was given")
	}
	if err := spec.Validate(); err != nil {
		return exitCannot, err
	}
	t, err := toolkit.NewThrowaways(note)
	if err != nil {
		return exitCannot, err
	}
	if *dryRun {
		plan, err := planNew(ctx, t, spec, p, logSettings)
		if err != nil {
			return exitCannot, err
		}
		return exitOK, renderDistroPlan(plan, *asJSON)
	}
	if logSettings.Active() {
		runLog, err := toolkit.OpenRunLog(logSettings, liveOut, os.Stderr)
		if err != nil {
			return exitCannot, err
		}
		defer runLog.Abort()
		spec.Log = runLog
	}
	res, err := t.Create(ctx, spec)
	if err != nil {
		if *asJSON && res.Name != "" {
			_ = writeJSON(res)
		}
		return exitCannot, err
	}
	if *asJSON {
		if werr := writeJSON(res); werr != nil {
			return exitCannot, werr
		}
	} else if res.Command == nil {
		fmt.Println(res.Name)
	}
	if res.Command == nil {
		if !*asJSON {
			logf("  %s is ready. Run in it with: wsl-toolkit distro run --name %s -c 'uname -a'", res.Name, res.Name)
		}
		return exitOK, nil
	}
	return commandVerdict(res.Name, *res.Command)
}

// commandVerdict turns a command's outcome into this process's exit code.
//
// ⭐ THE GUEST'S CODE IS FORWARDED VERBATIM, and a deadline is 124 whatever
// the guest was doing. A command that could not be started at all is an error,
// not a guest that exited 2, and a log the caller asked for that could not be
// written is an error too: the command's own code is named in it.
func commandVerdict(name string, out toolkit.CommandOutcome) (int, error) {
	if out.Error != "" {
		return exitCannot, errors.New(out.Error)
	}
	if out.LogError != "" {
		return exitCannot, fmt.Errorf("the command in %s ended with exit %d, and the log it was relayed into could not be written: %s", name, out.Exit, out.LogError)
	}
	if out.TimedOut {
		logf("  the command in %s reached its deadline and the distribution was terminated", name)
		return exitTimeout, nil
	}
	if out.Cancelled {
		return 130, nil
	}
	return out.Exit, nil
}

func cmdDistroRun(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("distro run")
	var c commandFlags
	c.bind(fs, 30*time.Minute)
	nameFlag := fs.String("name", "", "the throwaway distribution to run in. The eph- prefix is added when it is left off")
	dryRun := fs.Bool("dry-run", false, "validate ownership and every option, and print the plan without running the command or opening a log file")
	asJSON := fs.Bool("json", false, "write a structured answer. The command's output then goes to stderr")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	name, err := targetName(*nameFlag)
	if err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	p, err := c.payload(cfg)
	if err != nil {
		return exitCannot, err
	}
	if p.script == nil {
		return exitCannot, errors.New("nothing to run: pass -c COMMAND, --command-base64 or --script FILE")
	}
	logSettings, err := c.log.settings(cfg)
	if err != nil {
		return exitCannot, err
	}
	t, err := toolkit.NewThrowaways(note)
	if err != nil {
		return exitCannot, err
	}
	liveOut := io.Writer(os.Stdout)
	if *asJSON {
		liveOut = os.Stderr
	}
	spec := toolkit.ThrowawaySpec{User: c.user, Script: p.script, Env: p.env, UserEnv: c.userEnv, Timeout: c.timeout,
		Stdout: liveOut, Stderr: os.Stderr}
	if *dryRun {
		if _, err := t.Owned(ctx, name); err != nil {
			return exitCannot, err
		}
		plan := newDistroPlan("run", name)
		plan.addCommand(name, spec, p, logSettings)
		return exitOK, renderDistroPlan(plan, *asJSON)
	}
	if logSettings.Active() {
		runLog, err := toolkit.OpenRunLog(logSettings, liveOut, os.Stderr)
		if err != nil {
			return exitCannot, err
		}
		defer runLog.Abort()
		spec.Log = runLog
	}
	out, err := t.Run(ctx, name, spec)
	if err != nil {
		return exitCannot, err
	}
	if *asJSON {
		out.Schema, out.Name = toolkit.ThrowawayRunSchema, name
		if werr := writeJSON(out); werr != nil {
			return exitCannot, werr
		}
	}
	return commandVerdict(name, out)
}

func cmdDistroEnter(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("distro enter")
	nameFlag := fs.String("name", "", "the throwaway distribution to attach to. The eph- prefix is added when it is left off")
	user := fs.String("user", "root", "the account the shell runs as")
	dryRun := fs.Bool("dry-run", false, "validate ownership and print the attach plan without starting an interactive shell")
	asJSON := fs.Bool("json", false, "write the --dry-run plan as a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	if *asJSON && !*dryRun {
		return exitCannot, errors.New("--json structures the --dry-run plan, and an interactive shell has no structured answer")
	}
	name, err := targetName(*nameFlag)
	if err != nil {
		return exitCannot, err
	}
	if err := toolkit.AssertArgvSafe([]string{*user}); err != nil {
		return exitCannot, err
	}
	t, err := newThrowaways()
	if err != nil {
		return exitCannot, err
	}
	if _, err := t.Owned(ctx, name); err != nil {
		return exitCannot, err
	}
	if *dryRun {
		plan := newDistroPlan("enter", name)
		plan.step("attach an interactive login shell: wsl.exe -d " + name + " -u " + *user + " --cd ~")
		plan.step("no command, no relay and no deadline: the guest's login shell owns the terminal")
		return exitOK, renderDistroPlan(plan, *asJSON)
	}
	w, err := toolkit.FindWsl()
	if err != nil {
		return exitCannot, err
	}
	note("attaching to " + name + " as " + *user + ". Leave with exit or Ctrl-D")
	// ⚠ `--cd ~` STARTS IN THE ACCOUNT'S HOME. wsl.exe otherwise inherits this
	// process's Windows directory, mounted under /mnt, which is not where a
	// shell inside a throwaway distribution belongs.
	return toolkit.RunForeground(ctx, w.Path, []string{"-d", name, "-u", *user, "--cd", "~"})
}

func cmdDistroRemove(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("distro remove")
	nameFlag := fs.String("name", "", "the throwaway distribution to remove. The eph- prefix is added when it is left off")
	yes := fs.Bool("yes", false, "do not ask before removing")
	dryRun := fs.Bool("dry-run", false, "validate ownership and print the removal plan without unregistering or deleting")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	name, err := targetName(*nameFlag)
	if err != nil {
		return exitCannot, err
	}
	t, err := newThrowaways()
	if err != nil {
		return exitCannot, err
	}
	if *dryRun {
		steps, err := t.PlanRemoval(name)
		if err != nil {
			return exitCannot, err
		}
		plan := newDistroPlan("remove", name)
		for _, s := range steps {
			plan.step(s)
		}
		return exitOK, renderDistroPlan(plan, *asJSON)
	}
	// ⛔ A DESTRUCTIVE ACTION ASKS, AND REFUSES WHEN NOBODY CAN ANSWER, which is
	// the rule `base remove` holds: a prompt nobody reads approves itself.
	if !*yes && !isInteractive() {
		return exitCannot, fmt.Errorf("removing %s is destructive and this session is not interactive. Pass --yes", name)
	}
	if !*yes {
		fmt.Fprintf(os.Stderr, "Unregister %q and delete its directory? [y/N] ", name)
		var answer string
		fmt.Fscanln(os.Stdin, &answer)
		if !strings.EqualFold(strings.TrimSpace(answer), "y") {
			return exitCannot, errors.New("not confirmed, so nothing was removed")
		}
	}
	res, err := t.Remove(ctx, name, false)
	if err != nil {
		// ⛔ A REFUSAL IS NOT A FAILED REMOVAL. One says this tool will not act
		// and the other says it tried, and the two codes keep them apart.
		if errors.Is(err, toolkit.ErrNotOwned) {
			return exitCannot, err
		}
		return exitFailed, err
	}
	if *asJSON {
		return exitOK, writeJSON(res)
	}
	logf("  removed %s", name)
	return exitOK, nil
}

func cmdDistroPurge(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("distro purge")
	apply := fs.Bool("apply", false, "actually remove. Without it this reports what it would remove and changes nothing")
	dryRun := fs.Bool("dry-run", false, "the default. Accepted so the non-mutating intent can be written out, and refused beside --apply")
	includeLive := fs.Bool("include-live", false, "also remove a distribution that is running, or that another run is still creating")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	// ⛔ TWO CONTRADICTORY INTENTS ARE REFUSED, not resolved by a precedence.
	if *dryRun && *apply {
		return exitCannot, errors.New("--dry-run and --apply ask for opposite things. Pass one")
	}
	t, err := newThrowaways()
	if err != nil {
		return exitCannot, err
	}
	plan, err := t.Purge(ctx, *apply, *includeLive)
	if *asJSON {
		if werr := writeJSON(plan); werr != nil {
			return exitCannot, werr
		}
	} else {
		renderPurge(plan)
	}
	if err != nil {
		if len(plan.Failed) > 0 {
			return exitFailed, err
		}
		return exitCannot, err
	}
	return exitOK, nil
}

func renderPurge(plan toolkit.PurgePlan) {
	out := os.Stderr
	if plan.DryRun {
		fmt.Fprintln(out, "==> What --apply would remove. Nothing has been removed.")
	} else {
		fmt.Fprintln(out, "==> Removed")
	}
	for _, d := range plan.Distros {
		fmt.Fprintf(out, "  distribution   %s\n", d)
	}
	for _, f := range plan.Leftovers {
		fmt.Fprintf(out, "  leftover       %s\n", f)
	}
	if len(plan.Distros)+len(plan.Leftovers) == 0 {
		fmt.Fprintln(out, "  nothing")
	}
	for _, f := range plan.Failed {
		fmt.Fprintf(out, "  ! could not remove %s\n", f)
	}
	if len(plan.Kept) > 0 {
		fmt.Fprintln(out, "==> Kept")
		for _, k := range plan.Kept {
			fmt.Fprintf(out, "  kept           %s\n", k)
		}
	}
}

func cmdDistroSnapshot(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("distro snapshot")
	nameFlag := fs.String("name", "", "the throwaway distribution to export. The eph- prefix is added when it is left off")
	tag := fs.String("tag", "", "what the snapshot is called. distro new --tarball TAG imports it")
	force := fs.Bool("force", false, "replace a snapshot that already has this tag")
	dryRun := fs.Bool("dry-run", false, "validate ownership and the tag, and print the export plan without writing an archive")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	name, err := targetName(*nameFlag)
	if err != nil {
		return exitCannot, err
	}
	if *tag == "" {
		return exitCannot, errors.New("--tag is required: it is what distro new --tarball TAG imports")
	}
	if _, err := toolkit.SnapshotTag(*tag); err != nil {
		return exitCannot, err
	}
	t, err := newThrowaways()
	if err != nil {
		return exitCannot, err
	}
	if *dryRun {
		if _, err := t.Owned(ctx, name); err != nil {
			return exitCannot, err
		}
		path := t.SnapshotPath(*tag)
		plan := newDistroPlan("snapshot", name)
		if fileExists(path) {
			if !*force {
				return exitCannot, fmt.Errorf("a snapshot tagged %s already exists at %s. Pass --force to replace it", *tag, path)
			}
			plan.step("replace " + path)
		}
		plan.step("export the distribution to a sibling temporary, check it is at least a megabyte, and rename it to " + path)
		plan.step("the archive carries whatever the distribution holds, a credential a command left included, unencrypted")
		return exitOK, renderDistroPlan(plan, *asJSON)
	}
	res, err := t.Snapshot(ctx, name, *tag, *force)
	if err != nil {
		return exitCannot, err
	}
	if *asJSON {
		return exitOK, writeJSON(res)
	}
	fmt.Println(res.Path)
	return exitOK, nil
}
