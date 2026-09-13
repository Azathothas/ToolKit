// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const distroUsage = `wsl-toolkit distro <list|new|run|enter|remove|purge|snapshot>

  A throwaway distribution is a whole WSL distribution made from an image or a
  rootfs archive, for a job whose subject is the distribution itself: its init,
  its /etc/wsl.conf, what a login shell sees. Every one is named eph-... and
  lives under the state directory, and the tool acts only on those.

  list       what this state directory made, what else carries the prefix, and
             what failed creations and snapshots left on disk
  new        import one from --image or --tarball, and optionally run a command
  run        run a non-interactive POSIX script in one
  enter      attach an interactive login shell to one
  remove     unregister one and delete its directory
  purge      remove every one this state directory made. A plan without --apply
  snapshot   export one to an archive that new --tarball TAG imports again
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
			from = firstNonEmpty(d.Origin.Image, "snapshot "+d.Origin.Snapshot, d.Origin.Tarball)
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

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" && v != "snapshot " {
			return v
		}
	}
	return ""
}

// commandFlags are what new and run share, so the two cannot drift into
// accepting different spellings of one command.
type commandFlags struct {
	command    string
	scriptFile string
	env        stringList
	user       string
	timeout    time.Duration
	tick       time.Duration
}

func (c *commandFlags) bind(fs *flag.FlagSet, defaultTimeout time.Duration) {
	fs.StringVar(&c.command, "c", "", "the POSIX shell command to run, as a login shell")
	fs.StringVar(&c.scriptFile, "script", "", "a file on this machine whose bytes are the command")
	fs.Var(&c.env, "env", "NAME=VALUE set for the command. Repeatable")
	fs.StringVar(&c.user, "user", "root", "the account the command runs as")
	fs.DurationVar(&c.timeout, "timeout", defaultTimeout, "how long the command may run before the distribution is terminated and the answer is 124. 0 means no deadline")
	fs.DurationVar(&c.tick, "tick", 0, "emit a heartbeat on stderr at this interval while the command runs. 0 is off, and anything under 1s is raised to it")
}

// payload is the command, or nil when none was given, which is different from
// an empty one.
func (c *commandFlags) payload(cfg toolkit.Config) ([]byte, map[string]string, error) {
	if c.command == "" && c.scriptFile == "" {
		if len(c.env) > 0 {
			return nil, nil, errors.New("--env sets variables for a command, and no command was given")
		}
		return nil, nil, nil
	}
	if c.scriptFile != "" {
		resolved, err := pathFromProject(cfg, c.scriptFile)
		if err != nil {
			return nil, nil, fmt.Errorf("--script: %w", err)
		}
		c.scriptFile = resolved
	}
	script, err := guestScript(c.command, c.scriptFile)
	if err != nil {
		return nil, nil, err
	}
	env := map[string]string{}
	for _, pair := range c.env {
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, nil, fmt.Errorf("--env %q is not NAME=VALUE", pair)
		}
		if !toolkit.ValidEnvName(name) {
			return nil, nil, fmt.Errorf("--env %q: %q is not a usable environment name. A name is letters, digits and underscores, and does not start with a digit", pair, name)
		}
		env[name] = value
	}
	return script, env, nil
}

func (c *commandFlags) ticker() func(toolkit.TickEvent) {
	return tickPrinter(jobFlags{tick: c.tick})
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
	asJSON := fs.Bool("json", false, "write a structured answer. The command's output then goes to stderr")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	script, env, err := c.payload(cfg)
	if err != nil {
		return exitCannot, err
	}
	spec := toolkit.ThrowawaySpec{
		Name: *name, User: c.user, Script: script, Env: env, Timeout: c.timeout,
		Systemd: *systemd, OciEnv: *ociEnv, Reuse: *reuse, Ephemeral: *ephemeral,
		Tick: c.tick, OnTick: c.ticker(), Stdout: os.Stdout, Stderr: os.Stderr,
	}
	if *asJSON {
		// ⛔ UNDER --json STDOUT CARRIES THE ANSWER ALONE, so the command's own
		// stdout is written where every other word this program says goes.
		spec.Stdout = os.Stderr
	}
	if *image != "" {
		if spec.Image, err = resolveImage(cfg, *image); err != nil {
			return exitCannot, err
		}
	}
	if spec.Tarball, err = tarballArg(cfg, *tarball); err != nil {
		return exitCannot, fmt.Errorf("--tarball: %w", err)
	}
	if err := spec.Validate(); err != nil {
		return exitCannot, err
	}
	t, err := toolkit.NewThrowaways(note)
	if err != nil {
		return exitCannot, err
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
// not a guest that exited 2.
func commandVerdict(name string, out toolkit.CommandOutcome) (int, error) {
	if out.Error != "" {
		return exitCannot, errors.New(out.Error)
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
	name := fs.String("name", "", "the throwaway distribution to run in")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	if *name == "" {
		return exitCannot, errors.New("--name is required. wsl-toolkit distro list names the throwaway distributions")
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	script, env, err := c.payload(cfg)
	if err != nil {
		return exitCannot, err
	}
	if script == nil {
		return exitCannot, errors.New("nothing to run: pass -c COMMAND or --script FILE")
	}
	t, err := toolkit.NewThrowaways(note)
	if err != nil {
		return exitCannot, err
	}
	out, err := t.Run(ctx, *name, toolkit.ThrowawaySpec{
		User: c.user, Script: script, Env: env, Timeout: c.timeout,
		Tick: c.tick, OnTick: c.ticker(), Stdout: os.Stdout, Stderr: os.Stderr,
	})
	if err != nil {
		return exitCannot, err
	}
	return commandVerdict(*name, out)
}

func cmdDistroEnter(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("distro enter")
	name := fs.String("name", "", "the throwaway distribution to attach to")
	user := fs.String("user", "root", "the account the shell runs as")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	if *name == "" {
		return exitCannot, errors.New("--name is required. wsl-toolkit distro list names the throwaway distributions")
	}
	if err := toolkit.AssertArgvSafe([]string{*user}); err != nil {
		return exitCannot, err
	}
	t, err := newThrowaways()
	if err != nil {
		return exitCannot, err
	}
	if _, err := t.Owned(ctx, *name); err != nil {
		return exitCannot, err
	}
	w, err := toolkit.FindWsl()
	if err != nil {
		return exitCannot, err
	}
	note("attaching to " + *name + " as " + *user + ". Leave with exit or Ctrl-D")
	// ⚠ `--cd ~` STARTS IN THE ACCOUNT'S HOME. wsl.exe otherwise inherits this
	// process's Windows directory, mounted under /mnt, which is not where a
	// shell inside a throwaway distribution belongs.
	return toolkit.RunForeground(ctx, w.Path, []string{"-d", *name, "-u", *user, "--cd", "~"})
}

func cmdDistroRemove(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("distro remove")
	name := fs.String("name", "", "the throwaway distribution to remove")
	yes := fs.Bool("yes", false, "do not ask before removing")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	if *name == "" {
		return exitCannot, errors.New("--name is required. wsl-toolkit distro list names the throwaway distributions")
	}
	if err := toolkit.ValidThrowawayName(*name); err != nil {
		return exitCannot, err
	}
	// ⛔ A DESTRUCTIVE ACTION ASKS, AND REFUSES WHEN NOBODY CAN ANSWER, which is
	// the rule `base remove` holds: a prompt nobody reads approves itself.
	if !*yes && !isInteractive() {
		return exitCannot, fmt.Errorf("removing %s is destructive and this session is not interactive. Pass --yes", *name)
	}
	t, err := newThrowaways()
	if err != nil {
		return exitCannot, err
	}
	if !*yes {
		fmt.Fprintf(os.Stderr, "Unregister %q and delete its directory? [y/N] ", *name)
		var answer string
		fmt.Fscanln(os.Stdin, &answer)
		if !strings.EqualFold(strings.TrimSpace(answer), "y") {
			return exitCannot, errors.New("not confirmed, so nothing was removed")
		}
	}
	res, err := t.Remove(ctx, *name, false)
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
	logf("  removed %s", *name)
	return exitOK, nil
}

func cmdDistroPurge(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("distro purge")
	apply := fs.Bool("apply", false, "actually remove. Without it this reports what it would remove and changes nothing")
	includeLive := fs.Bool("include-live", false, "also remove a distribution that is running, or that another run is still creating")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
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
	name := fs.String("name", "", "the throwaway distribution to export")
	tag := fs.String("tag", "", "what the snapshot is called. distro new --tarball TAG imports it")
	force := fs.Bool("force", false, "replace a snapshot that already has this tag")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	if *name == "" || *tag == "" {
		return exitCannot, errors.New("--name and --tag are both required")
	}
	if _, err := toolkit.SnapshotTag(*tag); err != nil {
		return exitCannot, err
	}
	t, err := newThrowaways()
	if err != nil {
		return exitCannot, err
	}
	res, err := t.Snapshot(ctx, *name, *tag, *force)
	if err != nil {
		return exitCannot, err
	}
	if *asJSON {
		return exitOK, writeJSON(res)
	}
	fmt.Println(res.Path)
	return exitOK, nil
}
