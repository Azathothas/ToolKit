package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const baseUsage = `wsl-toolkit base <status|doors|ensure|recreate|reclaim|remove|shell|exec|agent|herdr|bootstrap|grant|revoke|attach|presets|image>

  status     is it registered, and can it actually run a container
  doors      attack its doors from the inside and report what got through
  ensure     bring it to a usable state, doing the least that achieves it
  recreate   remove it and build it again from nothing
  reclaim    give back to this machine the space its guest freed: make its disk
             sparse, then trim. WSL holds the disk while its VM runs, and the
             answer then names the one step that frees it
  remove     unregister it and delete its disk
  shell      attach an interactive shell to it, as the unprivileged account
  exec       run a non-interactive POSIX script in it, as that account
  agent      run one agent in it by name, with every argument the agent's own
  herdr      run one herdr command in it, with every argument herdr's own
  bootstrap  run the bootstrap this executable carries in it, as its account
  grant      mount one more Windows directory under /workspaces, now, with no restart
  revoke     unmount one, now, and take it out of the configuration
  attach     print the commands that reach its herdr server, from Windows and inside
  presets    the rootfs choices, what each one measured here, and which is live
  image      load an image archive, or an image this machine's engine holds,
             into the base's engine: base image load FILE|IMAGE

  --preset ID   build from a preset, or from any fully qualified reference.
                arch (default, glibc) alpine (musl, smallest) debian fedora
  --save        also make that preset the stored default for later commands
  --json        a structured answer
  --probe       status runs a real container to find out. Off by default
  --repair      ensure may clear engine run state a reboot invalidated, and
                restart a base over another configuration's live grants.
                Off by default: without it, a base needing repair is REFUSED
                with the exact command printed rather than quietly fixed
  --root        shell attaches as root instead
  --here        shell starts in this Windows directory, mounted under /mnt.
                Without it a shell starts in the guest account's home
  --yes         do not ask before removing
`

func cmdBase(ctx context.Context, args []string) (int, error) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, baseUsage)
		return exitCannot, nil
	}
	// ⛔ ASKING A GROUP FOR HELP IS NOT AN UNKNOWN SUBCOMMAND. `base --help` fell
	// through to the default arm, printed this usage and exited 2 saying
	// `"--help" is not a base subcommand` - while the top level and every one of
	// this group's own subcommands answer --help with 0. A page that tells a
	// reader to run `base --help` and also to read the exit code from the process
	// was telling them their correct command had failed. `skills/wsl-toolkit`
	// says both of those things.
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		fmt.Fprint(os.Stderr, baseUsage)
		return exitOK, flag.ErrHelp
	}
	sub, rest := args[0], args[1:]
	if sub == "presets" {
		return cmdPresets(rest)
	}
	if sub == "exec" {
		return cmdBaseExec(ctx, rest)
	}
	if sub == "doors" {
		return cmdBaseDoors(ctx, rest)
	}
	if sub == "grant" || sub == "revoke" {
		return cmdBaseGrant(ctx, sub, rest)
	}
	if sub == "agent" {
		return cmdBaseAgent(ctx, rest)
	}
	if sub == "herdr" {
		return cmdBaseHerdr(ctx, rest)
	}
	if sub == "bootstrap" {
		return cmdBaseBootstrap(ctx, rest)
	}
	if sub == "image" {
		return cmdBaseImage(ctx, rest)
	}
	flags, err := parseBaseFlags(sub, rest)
	if err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if sub == "attach" {
		// ⭐ BEFORE THE HELPER ROUTE. It reads files on this machine and starts
		// nothing, so a process that cannot reach wsl.exe can still answer it.
		return cmdBaseAttach(cfg, flags.asJSON)
	}
	switched, err := applyPreset(ctx, &cfg, flags.preset, flags.save)
	if err != nil {
		return exitCannot, err
	}
	// presetSwitch is an ensure the preset turned into a rebuild. ⛔ It rebuilds
	// only when the base, read again under its lock, is still not built from the
	// preset: a second switch that waited for the first found the base it had
	// just built and removed it anyway. An explicit `base recreate` stays
	// unconditional. WSL-169, WSL-172.
	presetSwitch := switched && sub == "ensure"
	if presetSwitch {
		// ⛔ A DISTRIBUTION'S ROOTFS CANNOT BE CHANGED UNDERNEATH IT. Switching
		// preset means rebuilding, and saying so beats a caller discovering it
		// from a build they did not expect.
		logf("  the base is registered from a different image, so this rebuilds it")
		sub = "recreate"
	}

	// status and ensure are the two a restricted caller needs; remove and shell
	// are deliberately NOT on the helper protocol. Removing a distribution and
	// attaching a terminal are not job data.
	if c, err := useHelper(ctx, flags.viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		switch sub {
		case "status":
			st, err := c.BaseStatus(ctx, flags.probe)
			if err != nil {
				return exitCannot, err
			}
			// ⛔ OWNERSHIP IS THIS STATE DIRECTORY'S, not the helper's, whose home
			// may be the owner's. WSL-167.
			if st.Naming != nil {
				st.Naming.Owner = toolkit.OwnsBase(cfg)
			}
			return verdictFor(st), renderBase(st, flags.asJSON, flags.probe)
		case "ensure", "recreate":
			st, err := c.BaseEnsureWith(ctx, sub == "recreate", flags.repair)
			if err != nil {
				return exitCannot, presetSwitchFailed(err, switched, flags.save, cfg)
			}
			return verdictFor(st), renderBase(st, flags.asJSON, true)
		default:
			return exitCannot, fmt.Errorf("this process cannot reach wsl.exe and %q is not something the helper accepts. Removing a distribution and attaching a terminal are not job data", sub)
		}
	}

	base, err := toolkit.NewBase(cfg, note)
	if err != nil {
		return exitCannot, err
	}

	switch sub {
	case "status":
		st, err := base.Status(ctx, flags.probe)
		if err != nil {
			return exitCannot, err
		}
		return verdictFor(st), renderBase(st, flags.asJSON, flags.probe)

	case "ensure":
		st, err := base.EnsureWith(ctx, false, flags.repair)
		if err != nil {
			// ⛔ THE STATE IS RENDERED BEFORE THE ERROR IS RETURNED. A refusal
			// that carries a remediation is the one case where the answer is in
			// the state and not in the message, and a caller reading --json got
			// nothing at all here before.
			_ = renderBase(st, flags.asJSON, true)
			return exitCannot, err
		}
		return verdictFor(st), renderBase(st, flags.asJSON, true)

	case "recreate":
		ensure := func(ctx context.Context, repair bool) (toolkit.BaseState, error) {
			return base.EnsureWith(ctx, true, repair)
		}
		if presetSwitch {
			ensure = base.EnsureSwitch
		}
		st, err := ensure(ctx, flags.repair)
		if err != nil {
			return exitCannot, presetSwitchFailed(err, switched, flags.save, cfg)
		}
		return verdictFor(st), renderBase(st, flags.asJSON, true)

	case "reclaim":
		return cmdBaseReclaim(ctx, base, flags.asJSON)

	case "remove":
		// ⛔ A DESTRUCTIVE ACTION ASKS, AND REFUSES WHEN NOBODY CAN ANSWER. The
		// rule `distro remove` holds too: a non-interactive session passes the
		// flag or gets a refusal, because a prompt nobody reads is a prompt that
		// approves itself.
		if !flags.yes && !isInteractive() {
			return exitCannot, fmt.Errorf("removing %s is destructive and this session is not interactive. Pass --yes", cfg.Base.Name)
		}
		if !flags.yes {
			fmt.Fprintf(os.Stderr, "Remove the distribution %q and delete its disk? [y/N] ", cfg.Base.Name)
			var answer string
			fmt.Fscanln(os.Stdin, &answer)
			if !strings.EqualFold(strings.TrimSpace(answer), "y") {
				return exitCannot, fmt.Errorf("not confirmed, so nothing was removed")
			}
		}
		if err := base.Remove(ctx); err != nil {
			return removalExit(err), err
		}
		logf("  removed %s", cfg.Base.Name)
		return exitOK, nil

	case "shell":
		user := cfg.Base.User
		if flags.asRoot {
			user = "root"
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
		shellArgs := []string{"-d", cfg.Base.Name, "-u", user}
		var extraEnv []string
		if flags.here {
			automount, err := toolkit.NormalizeAutomount(cfg.Base.Automount)
			if err != nil {
				return exitCannot, err
			}
			if automount == toolkit.AutomountOff {
				return exitCannot, fmt.Errorf("--here cannot inherit a Windows working directory while base.automount is off. Start without --here, then use a configured /workspaces mount")
			}
			drives, readErr := toolkit.MountedWindowsDrives(ctx, w, cfg.Base.Name)
			if err := hereRefusal(cfg.Base.Name, automount, drives, readErr); err != nil {
				return exitCannot, err
			}
			// ⚠ EXPLICIT, AND IT SAYS WHAT IT DID. `wsl.exe` inherits the
			// caller's Windows working directory, so this is the OLD default
			// under a flag rather than a new capability.
			note("starting in this Windows directory, mounted under /mnt")
			// ⭐ AND THE GUEST IS TOLD. scripts/common/shell-profile.sh moves an
			// interactive shell off a Windows drive, which is right for a shell
			// that merely inherited the directory and wrong for this one. From
			// inside the guest the two are identical without this mark.
			extraEnv = hereEnv(os.Getenv("WSLENV"))
		} else {
			// ⛔ THE MANUAL SAID ROOT WAS "INSIDE THE DISTRIBUTION" AND "NOT ON
			// THIS MACHINE", and `wsl.exe -d NAME -u USER` inherits the caller's
			// Windows directory, mounted WRITABLE under /mnt/c. A reporter
			// opened a root shell from a checkout, ran `test -w .`, and got
			// zero. Nothing escaped and nothing was overwritten; the manual
			// promised one thing and the binary did another, which is the whole
			// of WSL-47, issue 21. `--cd ~` starts in the guest account's home.
			shellArgs = append(shellArgs, "--cd", "~")
		}
		note("attaching to " + cfg.Base.Name + " as " + user + ". Leave with exit or Ctrl-D")
		// ⭐ A LOGIN SHELL, AND THE BEST ONE THE GUEST ACTUALLY HAS. `wsl.exe -d N -u U`
		// runs the account's passwd shell, which the provisioner leaves at /bin/sh, so
		// an interactive attach landed in sh with no line editing, no history and no
		// profile read - on a base that has bash installed. Measured 2026-09-17 on
		// wsl-toolkit-base: passwd shell /bin/sh, bash at /usr/sbin/bash, 5.3.15.
		//
		// ⛔ THE CHOICE IS MADE IN THE GUEST, NOT HERE, and it falls back twice rather
		// than once. bash where there is one; else the account's OWN configured shell,
		// which is what a guest with zsh or fish is entitled to; else /bin/sh. Every
		// arm is a LOGIN shell, which is the half that was missing before.
		//
		// ⚠ A MISSING bashrc OR profile IS NOT A FAILURE. `bash -l` with no
		// /etc/profile, ~/.bash_profile, ~/.bash_login or ~/.profile starts normally
		// and reads nothing; there is no arm for it because there is nothing to catch.
		// What IS caught is a bash that cannot be executed at all: the `exec` fails,
		// the script carries on, and the next arm runs.
		shellArgs = append(shellArgs, "--", "/bin/sh", "-c",
			`if command -v bash >/dev/null 2>&1; then exec bash -l; fi; `+
				`s=$(getent passwd "$(id -un)" 2>/dev/null | cut -d: -f7); `+
				`if [ -n "$s" ] && [ -x "$s" ]; then exec "$s" -l; fi; `+
				`exec /bin/sh -l`)

		if flags.asRoot {
			// ⭐ THE WARNING NAMES WHAT IS ACTUALLY REACHABLE, rather than
			// asserting an isolation this shell does not have. Every Windows
			// drive WSL has mounted is reachable from here whatever directory the
			// shell starts in, and saying so is the honest version of the
			// sentence the manual used to carry.
			drives, readErr := toolkit.MountedWindowsDrives(ctx, w, cfg.Base.Name)
			note(rootDrivesNote(cfg.Base.Name, drives, readErr))
			if mounts, mountErr := cfg.ResolvedBaseMounts(); mountErr == nil && len(mounts) > 0 {
				targets := make([]string, 0, len(mounts))
				for _, mount := range mounts {
					targets = append(targets, mount.Target+" ("+mount.Mode+")")
				}
				note("configured Windows directory grants: " + strings.Join(targets, ", "))
			}
			note("--root is an administration escape hatch: guest root can manually mount additional Windows paths")
		}
		return toolkit.RunForegroundEnv(ctx, w.Path, shellArgs, extraEnv)

	default:
		fmt.Fprint(os.Stderr, baseUsage)
		return exitCannot, fmt.Errorf("%q is not a base subcommand", sub)
	}
}

// baseFlags are the values of the flag set the simple base subcommands share.
type baseFlags struct {
	asJSON, probe, repair, asRoot, here, yes, viaHelper, save bool
	preset                                                    string
}

// baseSubcommands are the subcommands that parse the shared set, in the order a
// refusal names them.
var baseSubcommands = []string{"status", "ensure", "recreate", "reclaim", "remove", "shell", "attach"}

// baseFlagsRead names the flags each of those subcommands reads. `base ensure
// --probe` stays accepted: ensure verifies the base by running a container
// whether or not it is asked to. WSL-100.
var baseFlagsRead = map[string][]string{
	"status":   {"json", "probe", "via-helper"},
	"ensure":   {"json", "probe", "repair", "via-helper", "preset", "save"},
	"recreate": {"json", "probe", "repair", "via-helper", "preset", "save"},
	"reclaim":  {"json"},
	"remove":   {"yes", "via-helper"},
	"shell":    {"root", "here", "via-helper"},
	"attach":   {"json"},
}

var baseShared = sharedFlags[baseFlags]{group: "base", order: baseSubcommands, reads: baseFlagsRead, bind: (*baseFlags).bind}

func (f *baseFlags) bind(fs *flag.FlagSet) {
	fs.BoolVar(&f.asJSON, "json", false, "write a structured answer")
	fs.BoolVar(&f.probe, "probe", false, "status runs a real container to decide whether the base is usable. ensure and recreate always do, and accept the flag")
	fs.BoolVar(&f.repair, "repair", false, "ensure and recreate may clear engine run state a reboot invalidated, and restart a base over another configuration's live grants. Off by default")
	fs.BoolVar(&f.asRoot, "root", false, "shell attaches as root")
	fs.BoolVar(&f.here, "here", false, "shell starts in this Windows directory instead of the guest account's home")
	fs.BoolVar(&f.yes, "yes", false, "remove does not ask before removing")
	fs.BoolVar(&f.viaHelper, "via-helper", false, "go through the local helper even when this process could call wsl.exe itself")
	fs.StringVar(&f.preset, "preset", "", "ensure and recreate build from this preset id or fully qualified reference")
	fs.BoolVar(&f.save, "save", false, "ensure and recreate also store the preset as the default for later commands")
}

// parseBaseFlags parses the shared set for one subcommand, which refuses a flag
// it does not read.
func parseBaseFlags(sub string, args []string) (baseFlags, error) {
	var f baseFlags
	if err := baseShared.parse(sub, &f, args); err != nil {
		return f, err
	}
	if f.save && f.preset == "" {
		return f, errors.New("--save stores the preset --preset names, and no --preset was given")
	}
	return f, nil
}

func verdictFor(st toolkit.BaseState) int {
	if len(st.Problems) > 0 {
		return exitFailed
	}
	return exitOK
}

// renderBase is the ONE place a base state leaves this command, and it is one
// function rather than an `if *asJSON` at five call sites for the reason
// WSL-46 measured: three of those five call sites never had the branch at all.
// `base ensure --json` and `base recreate --json` accepted the flag, wrote
// human progress to stderr, exited 0 and put NOTHING on stdout, on both routes.
// A caller assigning the answer got an empty string and a zero status.
func renderBase(st toolkit.BaseState, asJSON, probed bool) error {
	if asJSON {
		return writeJSON(st)
	}
	renderBaseState(st, probed)
	return nil
}

func renderBaseState(st toolkit.BaseState, probed bool) {
	out := os.Stderr
	fmt.Fprintf(out, "  name        %s\n", st.Name)
	// ⭐ A BASE UNDER THE OLDER NAME SAYS SO, AND HOW TO MOVE. Moving is the
	// operator's call, so this names it and changes nothing. WSL-167.
	if n := st.Naming; n != nil && n.Legacy {
		fmt.Fprintf(out, "  older name  %s\n", n.Move)
	}
	// ⭐ WHERE THE DISK IS, AND WHETHER THIS STATE DIRECTORY MAY CHANGE IT. A
	// caller that does not own the base uses it, and is refused its removal,
	// rebuild and re-provisioning. WSL-167.
	if n := st.Naming; n != nil && n.Disk != "" {
		fmt.Fprintf(out, "  disk in     %s\n", n.Disk)
		owner := "no. This state directory uses the base, and never removes, rebuilds or re-provisions it"
		if n.Owner {
			owner = "yes, this state directory"
		}
		fmt.Fprintf(out, "  owner       %s\n", owner)
	}
	// ⚠ AN OLD INSTANCE `jobs` HOLDS THE CANONICAL NAME. It is not the default
	// base, and an ensure is refused until it is removed. WSL-167.
	if n := st.Naming; n != nil && n.Instance != "" {
		fmt.Fprintf(out, "  held by     the instance %s, not the default base: %s\n", n.Instance,
			toolkit.ToolCommandFor(n.Instance, "base remove --yes"))
	}
	fmt.Fprintf(out, "  image       %s\n", st.Image)
	if st.BuiltFrom != "" && st.BuiltFrom != st.Image {
		fmt.Fprintf(out, "  built from  %s\n", st.BuiltFrom)
	}
	fmt.Fprintf(out, "  account     %s\n", st.User)
	fmt.Fprintf(out, "  automount   %s\n", automountRow(st.Access))
	fmt.Fprintf(out, "  interop     %s\n", st.Access.Interop)
	fmt.Fprintf(out, "  init        systemd %v\n", st.Access.Systemd)
	fmt.Fprintf(out, "  sudo        passwordless %v\n", st.Access.PasswordlessSudo)
	fmt.Fprintf(out, "  toolset     %s\n", st.Access.Toolset)
	fmt.Fprintf(out, "  shared wsl  /mnt/wsl %s\n", st.Access.SharedTmpfs)
	// WSL-130.
	fmt.Fprintf(out, "  engines     root engine %v, runtime dir %v, kvm %v\n", st.Access.RootEngine, st.Access.RuntimeDir, st.Access.Kvm)
	for _, mount := range st.Access.Mounts {
		fmt.Fprintf(out, "  grant       %s %s <- %s\n", mount.Mode, mount.Target, mount.Source)
	}
	if len(st.Adapters) == 0 {
		// ⚠ NOT READ BACK IS SAID, AND WHY. Under --probe the adapters are read only
		// once the engine has answered, so an empty list there is a base that did not
		// get that far, not a base with no adapter.
		why := "Pass --probe"
		if probed {
			why = "The base did not get as far as reading them"
		}
		for _, name := range st.Access.Adapters {
			fmt.Fprintf(out, "  adapter     %s, not read back. %s\n", name, why)
		}
	}
	fmt.Fprintf(out, "  registered  %v\n", st.Registered)
	fmt.Fprintf(out, "  running     %v\n", st.Running)
	if st.DiskKnown {
		fmt.Fprintf(out, "  disk        %s  %s\n", toolkit.HumanBytes(st.DiskBytes), st.DiskPath)
		// ⭐ THE SPACE IT TAKES BESIDE ITS LENGTH, because only a sparse disk gives
		// back what its guest frees. WSL-109.
		if st.DiskSparse != "" {
			fmt.Fprintf(out, "  disk space  takes %s, sparse %s, .wslconfig sparseVhd %s\n",
				toolkit.HumanBytes(st.DiskAllocated), st.DiskSparse, st.WslConfigSparseVhd)
		}
	} else {
		fmt.Fprintf(out, "  disk        not measured  %s\n", st.DiskPath)
	}
	if !probed {
		// ⛔ "registered" IS NOT "usable" AND THE REPORT SAYS SO. A rootless
		// engine reports a complete configuration and then fails at the first
		// run, so a status that only listed the distribution would be a status
		// that cannot answer the question it was asked.
		fmt.Fprintf(out, "  usable      not checked. Pass --probe to run a container and find out\n")
		return
	}
	fmt.Fprintf(out, "  usable      %v\n", st.Healthy)
	if st.Engine != "" {
		fmt.Fprintf(out, "  engine      %s\n", st.Engine)
	}
	if cg := st.Cgroup; cg != nil {
		// ⛔ THE MECHANISM IS PRINTED, not a yes or a no. A base that gains
		// systemd, a rootful engine or a whole virtual machine later answers
		// through this same row rather than needing a second one. WSL-60.
		fmt.Fprintf(out, "  cgroup      %s via %s\n", cg.Version, cg.Mechanism)
		fmt.Fprintf(out, "  limits      enforced: %s   stats usable: %s\n", cg.Enforced, cg.StatsUsable)
		// ⭐ WHO MAKES A CONTAINER'S CGROUP, AND WITH WHAT. WSL-136.
		if cg.Manager != "" {
			fmt.Fprintf(out, "  cgroup mgr  %s, delegated to the account: %s\n", cg.Manager, valueOrNone(cg.AccountControllers))
		}
	} else if probed {
		// ⚠ NOT MEASURED IS NOT THE SAME AS NO, and this is the row that says
		// which one this is.
		fmt.Fprintf(out, "  cgroup      not measured by this guest\n")
	}
	if bf := st.Binfmt; bf != nil {
		// ⭐ THE BASE'S OWN SET IS THE VERDICT, and the kernel's total is beside it,
		// because the total is every distribution's. WSL-105.
		switch {
		case bf.Recorded && len(bf.Missing) == 0:
			fmt.Fprintf(out, "  binfmt      %d of this base's %d QEMU handler(s) held, %d in the kernel, ready: %v, watcher: %s\n",
				bf.Own, bf.Own, bf.Handlers, bf.Ready, valueOrNone(bf.Watch))
		case bf.Recorded:
			fmt.Fprintf(out, "  binfmt      %d of this base's %d QEMU handler(s) held, %d in the kernel, ready: %v, gone: %s, watcher: %s\n",
				bf.Own-len(bf.Missing), bf.Own, bf.Handlers, bf.Ready, strings.Join(bf.Missing, " "), valueOrNone(bf.Watch))
		default:
			fmt.Fprintf(out, "  binfmt      %d QEMU handler(s) in the kernel, ready: %v, this base's own set not recorded yet\n", bf.Handlers, bf.Ready)
		}
	} else if probed {
		fmt.Fprintf(out, "  binfmt      not measured by this guest\n")
	}
	if st.Kvm != "" {
		fmt.Fprintf(out, "  kvm         /dev/kvm %s to the account\n", st.Kvm)
	}
	for _, a := range st.Adapters {
		verdict := "healthy"
		if !a.Healthy {
			verdict = fmt.Sprintf("%d problem(s), listed below", len(a.Problems))
		}
		fmt.Fprintf(out, "  adapter     %s %s, %s\n", a.Name, a.Version, verdict)
		// ⚠ A NOTE IS TRUE AND STOPS NOTHING, so it prints beside the adapter it
		// is about rather than in the problem list, which is what refuses.
		for _, n := range a.Notes {
			fmt.Fprintf(out, "              ⚠ %s\n", n)
		}
	}
	for _, p := range st.Problems {
		fmt.Fprintf(out, "  ! %s\n", p)
	}
	// ⭐ LAST, AND UNMISSABLE. The reader is usually an agent that has to decide
	// what to run next, and a condition reported without its command is a
	// condition the agent has to guess its way out of. WSL-61.
	for _, r := range st.Remediations {
		fmt.Fprintf(out, "\n  ⛔ %s\n", strings.ToUpper(r.ID))
		fmt.Fprintf(out, "     what   %s\n", r.What)
		fmt.Fprintf(out, "     costs  %s\n", r.Costs)
		if r.Repairable {
			fmt.Fprintf(out, "     RUN    %s\n", r.Command)
		} else {
			fmt.Fprintf(out, "     read   %s\n", r.Command)
			fmt.Fprintf(out, "     ⚠ this tool cannot repair it, and says so rather than pretending\n")
		}
	}
}

// rootDrivesNote names the Windows drive mounts a root shell reaches, each by its
// mode.
//
// ⛔ WSL-84's door sweep: it said "mounted and writable" of every drive in a base
// whose drives were all read-only, where root's touch on /mnt/c answered "Read-only
// file system".
func rootDrivesNote(distro string, drives []toolkit.WindowsDriveMount, readErr error) string {
	msg := "root here is root INSIDE " + distro + " and not on this machine"
	if readErr != nil {
		return msg + ". Which Windows drives are mounted could not be read: " + readErr.Error()
	}
	if len(drives) == 0 {
		return msg + ". No Windows drive is mounted"
	}
	var writable, readOnly []string
	for _, d := range drives {
		if d.ReadOnly {
			readOnly = append(readOnly, d.Target)
		} else {
			writable = append(writable, d.Target)
		}
	}
	if len(writable) > 0 {
		msg += ", AND these Windows drives are mounted writable: " + strings.Join(writable, " ")
	}
	if len(readOnly) > 0 {
		msg += ". These Windows drives are mounted read-only: " + strings.Join(readOnly, " ")
	}
	return msg
}

// hereRefusal refuses `base shell --here` into a base with no Windows drive mounted.
//
// ⛔ WSL-84's door sweep: on a base configured `ro` whose guest had no drive, the shell
// noted "starting in this Windows directory" and started in the account's home,
// because WSL could not reach the directory. A read that failed refuses nothing: the
// shell then starts as it did, and WSL decides.
func hereRefusal(distro, automount string, drives []toolkit.WindowsDriveMount, readErr error) error {
	if readErr != nil || len(drives) > 0 {
		return nil
	}
	return fmt.Errorf("--here needs a Windows drive mounted, and %s has none although base.automount is %s. Run: %s", distro, automount, toolkit.ToolCommand("base ensure"))
}

// hereMarker is the variable name `base shell --here` sets in the guest, and that
// scripts/common/shell-profile.sh reads to leave such a shell where it started.
const hereMarker = "WSL_TOOLKIT_HERE"

// hereEnv is the environment `base shell --here` adds so the guest can tell a
// shell that ASKED for the Windows directory from one that merely inherited it.
//
// ⛔ WSLENV IS THE ONLY DOOR, and it is a list this process does not own. A
// variable not named in it does not cross into the guest at all, and a WSLENV
// written rather than appended to would drop whatever the caller had put there.
// The `/u` flag is "only on the way in", which is the only direction this is for.
//
// ⚠ IT IS NOT ADDED TWICE. A caller whose own WSLENV already names it gets their
// spelling kept, flags and all, because a second entry for one name is a list
// whose meaning depends on which end wsl.exe reads first.
func hereEnv(wslenv string) []string {
	out := []string{hereMarker + "=1"}
	for _, entry := range strings.Split(wslenv, ":") {
		name := entry
		if slash := strings.IndexByte(name, '/'); slash >= 0 {
			name = name[:slash]
		}
		if name == hereMarker {
			return out
		}
	}
	joined := hereMarker + "/u"
	if trimmed := strings.Trim(wslenv, ":"); trimmed != "" {
		joined = trimmed + ":" + joined
	}
	return append(out, "WSLENV="+joined)
}

// automountRow is the configured automount and, once a probe has read them, what
// the guest's drives are.
//
// ⛔ WSL-84: this row printed the configured `ro` over a guest whose /mnt/c was
// writable, so a report read as the setting is kept apart from the drives read back.
func automountRow(access toolkit.BaseAccessState) string {
	if access.AutomountGuest == "" {
		return access.Automount
	}
	return access.Automount + ", and the guest's drives read " + access.AutomountGuest
}

func isInteractive() bool { return isConsole(os.Stdin) }

// valueOrNone is a row's value, or none where the guest named nothing.
func valueOrNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "none"
	}
	return s
}

// cmdBaseReclaim gives the space the base's guest freed back to this machine.
//
// ⛔ IT NEVER RUNS `wsl --shutdown`. WSL makes a disk sparse only while its VM is
// stopped, and stopping it stops every distribution on the machine, somebody
// else's included. So where WSL holds the disk the answer is exit 1 with that
// step named, and the step stays the operator's. WSL-109.
func cmdBaseReclaim(ctx context.Context, base *toolkit.Base, asJSON bool) (int, error) {
	res, err := base.Reclaim(ctx)
	code := exitOK
	switch {
	case errors.Is(err, toolkit.ErrDiskHeld):
		code = exitFailed
	case err != nil:
		code = exitCannot
	}
	if asJSON {
		if res.Schema != "" && (err == nil || code == exitFailed) {
			if werr := writeJSON(res); werr != nil {
				return exitCannot, werr
			}
		}
		return code, err
	}
	if res.Disk != "" {
		logf("  disk        %s", res.Disk)
	}
	if res.AllocatedBefore > 0 || res.SparseBefore {
		logf("  before      sparse %v, takes %s", res.SparseBefore, toolkit.HumanBytes(res.AllocatedBefore))
	}
	if res.Refused != "" {
		logf("  ⛔ %s", res.Refused)
	}
	if err == nil {
		trimmed := "fstrim reported no figure"
		if res.TrimmedKnown {
			trimmed = "fstrim discarded " + toolkit.HumanBytes(res.Trimmed)
		}
		logf("  after       sparse %v, takes %s; %s", res.SparseAfter, toolkit.HumanBytes(res.AllocatedAfter), trimmed)
	}
	return code, err
}

// removalExit is the exit code a failed `base remove` answers.
//
// ⛔ A REFUSAL BY OWNERSHIP IS EXIT 2, as every other refusal of a change is. It
// changed nothing, and exit 1 says something was tried and failed. WSL-167.
func removalExit(err error) int {
	if errors.Is(err, toolkit.ErrNotOwner) {
		return exitCannot
	}
	return exitFailed
}
