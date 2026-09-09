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
  --root        shell attaches as root instead
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
	asRoot := fs.Bool("root", false, "attach as root")
	yes := fs.Bool("yes", false, "do not ask before removing")
	viaHelper := fs.Bool("via-helper", false, "go through the local helper even when this process could call wsl.exe itself")
	preset := fs.String("preset", "", "build from this preset id or fully qualified reference")
	save := fs.Bool("save", false, "also store the preset as the default for later commands")
	if sub == "presets" {
		return cmdPresets(rest)
	}
	if err := fs.Parse(rest); err != nil {
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
			if *asJSON {
				return verdictFor(st), writeJSON(st)
			}
			renderBaseState(st, *probe)
			return verdictFor(st), nil
		case "ensure", "recreate":
			st, err := c.BaseEnsure(ctx, sub == "recreate")
			if err != nil {
				return exitCannot, err
			}
			renderBaseState(st, true)
			return verdictFor(st), nil
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
		if *asJSON {
			return verdictFor(st), writeJSON(st)
		}
		renderBaseState(st, *probe)
		return verdictFor(st), nil

	case "ensure":
		st, err := base.Ensure(ctx, false)
		if err != nil {
			return exitCannot, err
		}
		renderBaseState(st, true)
		return verdictFor(st), nil

	case "recreate":
		st, err := base.Ensure(ctx, true)
		if err != nil {
			return exitCannot, err
		}
		renderBaseState(st, true)
		return verdictFor(st), nil

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
		note("attaching to " + cfg.Base.Name + " as " + user + ". Leave with exit or Ctrl-D")
		return toolkit.RunForeground(ctx, w.Path, []string{"-d", cfg.Base.Name, "-u", user})

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
	for _, p := range st.Problems {
		fmt.Fprintf(out, "  ! %s\n", p)
	}
}

func isInteractive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
