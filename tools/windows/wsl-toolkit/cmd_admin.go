package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

func cmdDoctor(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("doctor")
	var opts toolkit.DoctorOptions
	asJSON := fs.Bool("json", false, "write a structured answer")
	fs.BoolVar(&opts.Fast, "fast", false, "find the tools without asking each one its version")
	fs.BoolVar(&opts.Net, "net", false, "probe outbound HTTPS")
	fs.StringVar(&opts.Group, "group", "", "one tool group")
	if err := fs.Parse(args); err != nil {
		return exitCannot, err
	}
	if fs.NArg() != 0 {
		return exitCannot, fmt.Errorf("doctor takes flags, not positional arguments")
	}
	if cfg, cfgErr := loadConfig(); cfgErr == nil {
		opts.Config = cfg
	} else {
		// A configuration this build cannot read is a finding rather than a
		// reason to refuse a survey: the survey is what somebody runs to work
		// out why something is wrong.
		logf("  the configuration could not be read, so the WSL section uses the defaults: %s", cfgErr.Error())
		opts.Config = toolkit.DefaultConfig()
	}
	report, err := toolkit.Doctor(ctx, opts)
	if err != nil {
		return exitCannot, err
	}
	if *asJSON {
		return exitOK, writeJSON(report)
	}
	// ⭐ A PROBE IS NOT A GATE. A missing tool is data, so this exits 0 whether
	// or not anything is missing, exactly like scripts/doctor/.
	return exitOK, toolkit.WriteDoctor(os.Stdout, report, false)
}

func cmdImages(args []string) (int, error) {
	fs := newFlagSet("images")
	asJSON := fs.Bool("json", false, "write a structured answer")
	selector := fs.String("select", "", "resolve a selector the way matrix would, and print what it chose")
	if err := fs.Parse(args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	list := cfg.Catalog()
	if *selector != "" {
		list, err = cfg.SelectImages([]string{*selector})
		if err != nil {
			return exitCannot, err
		}
	}
	if *asJSON {
		return exitOK, writeJSON(map[string]any{
			"schema":  "wsl-toolkit-images/1",
			"images":  list,
			"default": cfg.MatrixDefault(),
		})
	}
	fmt.Fprintf(os.Stderr, "  %-12s %-8s %-8s %s\n", "id", "libc", "kind", "reference")
	for _, img := range list {
		fmt.Printf("  %-12s %-8s %-8s %s\n", img.ID, img.Libc, img.Kind, img.Ref)
	}
	fmt.Fprintf(os.Stderr, "\n  %d image(s). Every reference is fully qualified, because an unqualified one\n", len(list))
	fmt.Fprintf(os.Stderr, "  is resolved by the engine's own alias table and means two different images\n")
	fmt.Fprintf(os.Stderr, "  on two machines.\n")
	return exitOK, nil
}

func cmdResources(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("resources")
	asJSON := fs.Bool("json", false, "write a structured answer")
	viaHelper := fs.Bool("via-helper", false, "go through the local helper even when this process could call wsl.exe itself")
	if err := fs.Parse(args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if c, err := useHelper(ctx, *viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		rep, err := c.Resources(ctx)
		if err != nil {
			return exitCannot, err
		}
		if *asJSON {
			return exitOK, writeJSON(rep)
		}
		return exitOK, toolkit.RenderResources(os.Stdout, rep)
	}
	runner, err := toolkit.NewRunner(cfg, note)
	if err != nil {
		return exitCannot, err
	}
	rep := runner.Resources(ctx)
	if *asJSON {
		return exitOK, writeJSON(rep)
	}
	// ⛔ IT OFFERS AND IT DOES NOT DO. Every figure here is a reading; nothing
	// on this path removes anything, including the things this tool made.
	return exitOK, toolkit.RenderResources(os.Stdout, rep)
}

func cmdGC(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("gc")
	apply := fs.Bool("apply", false, "actually remove. Without it this reports what it would remove and changes nothing")
	olderThan := fs.Duration("older-than", 0, "only remove a job directory untouched for at least this long")
	images := fs.Bool("images", false, "also prune images the engine in the base is holding")
	asJSON := fs.Bool("json", false, "write a structured answer")
	viaHelper := fs.Bool("via-helper", false, "go through the local helper even when this process could call wsl.exe itself")
	if err := fs.Parse(args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if c, err := useHelper(ctx, *viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		plan, err := c.Cleanup(ctx, *apply, *olderThan, *images)
		if *asJSON {
			if writeErr := writeJSON(plan); writeErr != nil {
				return exitCannot, writeErr
			}
		} else {
			renderCleanup(plan)
		}
		return gcVerdict(plan, err)
	}
	runner, err := toolkit.NewRunner(cfg, note)
	if err != nil {
		return exitCannot, err
	}
	st, err := runner.Base().Status(ctx, false)
	if err != nil {
		return exitCannot, err
	}
	if !st.Registered {
		logf("  %s is not registered, so there is nothing inside it to clean up", st.Name)
		if *asJSON {
			return exitOK, writeJSON(toolkit.CleanupPlan{Schema: toolkit.CleanupSchema, DryRun: !*apply})
		}
		return exitOK, nil
	}
	plan, err := runner.Cleanup(ctx, *apply, *olderThan, *images)
	if *asJSON {
		if writeErr := writeJSON(plan); writeErr != nil {
			return exitCannot, writeErr
		}
		return gcVerdict(plan, err)
	}
	renderCleanup(plan)
	return gcVerdict(plan, err)
}

// gcVerdict turns a cleanup into an exit code.
//
// ⛔ A THING THAT COULD NOT BE REMOVED IS A DISAGREEMENT, NOT A SUCCESS. The
// guest script keeps going past a container it cannot remove so that one stuck
// container does not strand every directory behind it, which means the only
// place the failure can be reported is here. Exiting 0 over a non-empty Failed
// list is what let a leftover job directory pass a cleanup unremarked.
func gcVerdict(plan toolkit.CleanupPlan, err error) (int, error) {
	if err != nil {
		return exitFailed, err
	}
	if n := len(plan.Failed); n > 0 {
		return exitFailed, fmt.Errorf("%d thing(s) could not be removed, starting with %s", n, plan.Failed[0])
	}
	return exitOK, nil
}

func renderCleanup(plan toolkit.CleanupPlan) {
	out := os.Stderr
	if plan.DryRun {
		fmt.Fprintln(out, "==> What --apply would remove. Nothing has been removed.")
	} else {
		fmt.Fprintln(out, "==> Removed")
	}
	for _, c := range plan.Containers {
		fmt.Fprintf(out, "  container      %s\n", c)
	}
	for _, d := range plan.GuestDirs {
		fmt.Fprintf(out, "  guest dir      %s\n", d)
	}
	for _, d := range plan.HostDirs {
		fmt.Fprintf(out, "  host dir       %s\n", d)
	}
	for _, i := range plan.Images {
		fmt.Fprintf(out, "  image          %s\n", i)
	}
	if !plan.DryRun {
		for _, r := range plan.Removed {
			fmt.Fprintf(out, "  ok             %s\n", r)
		}
	}
	for _, f := range plan.Failed {
		fmt.Fprintf(out, "  ! could not remove %s\n", f)
	}
	if plan.DryRun && len(plan.Containers)+len(plan.GuestDirs)+len(plan.HostDirs)+len(plan.Images) == 0 {
		fmt.Fprintln(out, "  nothing")
	}
}

func cmdConfig(args []string) (int, error) {
	fs := newFlagSet("config")
	asJSON := fs.Bool("json", false, "write a structured answer")
	write := fs.Bool("write", false, "write the effective configuration to disk, so it can be edited")
	if err := fs.Parse(args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	path, err := toolkit.ConfigPath()
	if err != nil {
		return exitCannot, err
	}
	if *write {
		// ⚠ The written file carries the CURRENT catalog, so an edit starts
		// from what the tool is actually doing rather than from an empty
		// skeleton somebody has to guess the shape of.
		cfg.Images = cfg.Catalog()
		cfg.Matrix = cfg.MatrixDefault()
		if err := cfg.Write(); err != nil {
			return exitCannot, err
		}
		logf("  wrote %s", path)
	}
	if *asJSON {
		return exitOK, writeJSON(map[string]any{
			"schema":  "wsl-toolkit-config-report/1",
			"path":    path,
			"exists":  fileExists(path),
			"base":    cfg.Base,
			"images":  cfg.Catalog(),
			"matrix":  cfg.MatrixDefault(),
			"builtin": len(cfg.Images) == 0,
		})
	}
	fmt.Println(path)
	fmt.Fprintf(os.Stderr, "  exists      %v\n", fileExists(path))
	fmt.Fprintf(os.Stderr, "  base        %s from %s as %s\n", cfg.Base.Name, cfg.Base.Image, cfg.Base.User)
	fmt.Fprintf(os.Stderr, "  catalog     %d image(s), %s\n", len(cfg.Catalog()), builtinOrStored(cfg))
	fmt.Fprintf(os.Stderr, "  matrix      %d image(s) by default\n", len(cfg.MatrixDefault()))
	fmt.Fprintf(os.Stderr, "\n  wsl-toolkit config --write puts the effective configuration on disk to edit.\n")
	return exitOK, nil
}

func builtinOrStored(cfg toolkit.Config) string {
	if len(cfg.Images) == 0 {
		return "the built-in list"
	}
	return "from the configuration file"
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
