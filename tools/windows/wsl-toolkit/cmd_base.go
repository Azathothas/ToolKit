package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const baseUsage = `wsl-toolkit base <status|ensure|recreate|remove|shell|presets>

  status     is it registered, and can it actually run a container
  ensure     bring it to a usable state, doing the least that achieves it
  recreate   remove it and build it again from nothing
  remove     unregister it and delete its disk
  shell      attach an interactive shell to it, as the unprivileged account
  presets    the rootfs choices, what each one measured here, and which is live

  --preset ID   build from a preset, or from any fully qualified reference.
                arch (default, glibc) alpine (musl, smallest) debian fedora
  --save        also make that preset the stored default for later commands
  --json        a structured answer
  --probe       status runs a real container to find out. Off by default
  --repair      ensure may clear engine run state a reboot invalidated.
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
	sub, rest := args[0], args[1:]
	fs := newFlagSet("base " + sub)
	asJSON := fs.Bool("json", false, "write a structured answer")
	probe := fs.Bool("probe", false, "run a real container to decide whether the base is usable")
	repair := fs.Bool("repair", false, "ensure may clear engine run state a reboot invalidated. Off by default")
	asRoot := fs.Bool("root", false, "attach as root")
	here := fs.Bool("here", false, "base shell starts in this Windows directory instead of the guest account's home")
	yes := fs.Bool("yes", false, "do not ask before removing")
	viaHelper := fs.Bool("via-helper", false, "go through the local helper even when this process could call wsl.exe itself")
	preset := fs.String("preset", "", "build from this preset id or fully qualified reference")
	save := fs.Bool("save", false, "also store the preset as the default for later commands")
	if sub == "presets" {
		return cmdPresets(rest)
	}
	if err := parseArgs(fs, rest); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	switched, err := applyPreset(ctx, &cfg, *preset, *save)
	if err != nil {
		return exitCannot, err
	}
	if switched && sub == "ensure" {
		// ⛔ A DISTRIBUTION'S ROOTFS CANNOT BE CHANGED UNDERNEATH IT. Switching
		// preset means rebuilding, and saying so beats a caller discovering it
		// from a build they did not expect.
		logf("  the base is registered from a different image, so this rebuilds it")
		sub = "recreate"
	}

	// status and ensure are the two a restricted caller needs; remove and shell
	// are deliberately NOT on the helper protocol. Removing a distribution and
	// attaching a terminal are not job data.
	if c, err := useHelper(ctx, *viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		switch sub {
		case "status":
			st, err := c.BaseStatus(ctx, *probe)
			if err != nil {
				return exitCannot, err
			}
			return verdictFor(st), renderBase(st, *asJSON, *probe)
		case "ensure", "recreate":
			st, err := c.BaseEnsureWith(ctx, sub == "recreate", *repair)
			if err != nil {
				return exitCannot, err
			}
			return verdictFor(st), renderBase(st, *asJSON, true)
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
		st, err := base.Status(ctx, *probe)
		if err != nil {
			return exitCannot, err
		}
		return verdictFor(st), renderBase(st, *asJSON, *probe)

	case "ensure":
		st, err := base.EnsureWith(ctx, false, *repair)
		if err != nil {
			// ⛔ THE STATE IS RENDERED BEFORE THE ERROR IS RETURNED. A refusal
			// that carries a remediation is the one case where the answer is in
			// the state and not in the message, and a caller reading --json got
			// nothing at all here before.
			_ = renderBase(st, *asJSON, true)
			return exitCannot, err
		}
		return verdictFor(st), renderBase(st, *asJSON, true)

	case "recreate":
		st, err := base.EnsureWith(ctx, true, *repair)
		if err != nil {
			return exitCannot, err
		}
		return verdictFor(st), renderBase(st, *asJSON, true)

	case "remove":
		// ⛔ A DESTRUCTIVE ACTION ASKS, AND REFUSES WHEN NOBODY CAN ANSWER. The
		// same rule wsl-toolkit.ps1 holds: a non-interactive session passes the
		// flag or gets a refusal, because a prompt nobody reads is a prompt that
		// approves itself.
		if !*yes && !isInteractive() {
			return exitCannot, fmt.Errorf("removing %s is destructive and this session is not interactive. Pass --yes", cfg.Base.Name)
		}
		if !*yes {
			fmt.Fprintf(os.Stderr, "Remove the distribution %q and delete its disk? [y/N] ", cfg.Base.Name)
			var answer string
			fmt.Fscanln(os.Stdin, &answer)
			if !strings.EqualFold(strings.TrimSpace(answer), "y") {
				return exitCannot, fmt.Errorf("not confirmed, so nothing was removed")
			}
		}
		if err := base.Remove(ctx); err != nil {
			return exitFailed, err
		}
		logf("  removed %s", cfg.Base.Name)
		return exitOK, nil

	case "shell":
		user := cfg.Base.User
		if *asRoot {
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
			return exitCannot, fmt.Errorf("%s is not registered. Build it with: wsl-toolkit base ensure", cfg.Base.Name)
		}
		shellArgs := []string{"-d", cfg.Base.Name, "-u", user}
		if *here {
			// ⚠ EXPLICIT, AND IT SAYS WHAT IT DID. `wsl.exe` inherits the
			// caller's Windows working directory, so this is the OLD default
			// under a flag rather than a new capability.
			note("starting in this Windows directory, mounted under /mnt")
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
		if *asRoot {
			// ⭐ THE WARNING NAMES WHAT IS ACTUALLY REACHABLE, rather than
			// asserting an isolation this shell does not have. Every Windows
			// drive WSL has mounted is writable from here whatever directory the
			// shell starts in, and saying so is the honest version of the
			// sentence the manual used to carry.
			if drives := toolkit.MountedWindowsDrives(ctx, w, cfg.Base.Name); len(drives) > 0 {
				note("root here is root INSIDE " + cfg.Base.Name + " and not on this machine, AND these Windows drives are mounted and writable: " + strings.Join(drives, " "))
			} else {
				note("root here is root INSIDE " + cfg.Base.Name + " and not on this machine. No Windows drive is mounted")
			}
		}
		return toolkit.RunForeground(ctx, w.Path, shellArgs)

	default:
		fmt.Fprint(os.Stderr, baseUsage)
		return exitCannot, fmt.Errorf("%q is not a base subcommand", sub)
	}
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
	fmt.Fprintf(out, "  image       %s\n", st.Image)
	if st.BuiltFrom != "" && st.BuiltFrom != st.Image {
		fmt.Fprintf(out, "  built from  %s\n", st.BuiltFrom)
	}
	fmt.Fprintf(out, "  account     %s\n", st.User)
	fmt.Fprintf(out, "  registered  %v\n", st.Registered)
	fmt.Fprintf(out, "  running     %v\n", st.Running)
	if st.DiskKnown {
		fmt.Fprintf(out, "  disk        %s  %s\n", toolkit.HumanBytes(st.DiskBytes), st.DiskPath)
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
	} else if probed {
		// ⚠ NOT MEASURED IS NOT THE SAME AS NO, and this is the row that says
		// which one this is.
		fmt.Fprintf(out, "  cgroup      not measured by this guest\n")
	}
	if bf := st.Binfmt; bf != nil {
		fmt.Fprintf(out, "  binfmt      %d QEMU handler(s), ready: %v\n", bf.Handlers, bf.Ready)
	} else if probed {
		fmt.Fprintf(out, "  binfmt      not measured by this guest\n")
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

func isInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
