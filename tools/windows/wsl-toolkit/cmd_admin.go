package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

func cmdDoctor(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("doctor")
	var opts toolkit.DoctorOptions
	asJSON := fs.Bool("json", false, "write a structured answer")
	fs.BoolVar(&opts.Fast, "fast", false, "find the tools without asking each one its version")
	fs.BoolVar(&opts.Net, "net", false, "probe outbound HTTPS")
	fs.StringVar(&opts.Group, "group", "", "one tool group")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
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

func cmdImages(ctx context.Context, args []string) (int, error) {
	// `pull` and `warm` are subcommands rather than flags, because both ACT and
	// the bare command is a report. A report and an action behind one name,
	// separated by a flag, is how somebody pulls twelve images by accident.
	sub := ""
	if len(args) > 0 && (args[0] == "pull" || args[0] == "warm") {
		sub, args = args[0], args[1:]
	}
	fs := newFlagSet("images")
	if sub != "" {
		fs = newFlagSet("images " + sub)
	}
	asJSON := fs.Bool("json", false, "write a structured answer")
	selector := fs.String("select", "", "resolve a selector the way matrix would, and print what it chose")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if sub != "" {
		var selectors []string
		if *selector != "" {
			selectors = strings.Split(*selector, ",")
		}
		// ⭐ `warm` REPORTS AND `pull` ACTS. Warm says which references this
		// machine already holds and which it does not; pull fetches the ones it
		// does not, so a twelve-row matrix fails FIRST rather than slowly.
		return runImagesWarm(ctx, cfg, selectors, sub == "pull", *asJSON)
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
	job := fs.String("job", "", "narrow every row to one job id")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if *job != "" {
		if err := toolkit.AssertArgvSafe([]string{*job}); err != nil {
			return exitCannot, err
		}
	}
	narrow := func(rep toolkit.ResourceReport) toolkit.ResourceReport {
		if *job == "" {
			return rep
		}
		return narrowResourcesToJob(rep, *job)
	}
	if c, err := useHelper(ctx, *viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		rep, err := c.Resources(ctx)
		if err != nil {
			return exitCannot, err
		}
		rep = narrow(rep)
		if *asJSON {
			return exitOK, writeJSON(rep)
		}
		return exitOK, toolkit.RenderResources(os.Stdout, rep)
	}
	runner, err := toolkit.NewRunner(cfg, note)
	if err != nil {
		return exitCannot, err
	}
	rep := narrow(runner.Resources(ctx))
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
	olderThan := fs.Duration("older-than", 0, "only remove something untouched for at least this long. It applies to containers as well as directories")
	includeLive := fs.Bool("include-live", false, "also remove work that is running right now. Without it a live job is listed as kept and left alone")
	images := fs.Bool("images", false, "also prune images the engine in the base is holding")
	asJSON := fs.Bool("json", false, "write a structured answer")
	viaHelper := fs.Bool("via-helper", false, "go through the local helper even when this process could call wsl.exe itself")
	job := fs.String("job", "", "clean up one job id and leave the rest of the store alone")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if *olderThan < 0 {
		return exitCannot, fmt.Errorf("--older-than %s is negative. Pass 0 for no age limit, or a positive duration", *olderThan)
	}
	if *job != "" {
		if err := toolkit.AssertArgvSafe([]string{*job}); err != nil {
			return exitCannot, err
		}
	}
	// ⚠ NAMING A JOB IS NOT A WAY OF SAYING --include-live. A job that is still
	// running is spared exactly as it would be without --job, which is what
	// WSL-36 settled for the whole store and applies unchanged to one row of it.
	policy := toolkit.CleanupPolicy{OlderThan: *olderThan, IncludeLive: *includeLive, Job: *job}
	if c, err := useHelper(ctx, *viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		plan, err := c.Cleanup(ctx, *apply, policy, *images)
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
	plan, err := runner.Cleanup(ctx, *apply, policy, *images)
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
	// ⭐ WHAT WAS SPARED, AND WHY. Without it an empty plan reads the same
	// whether the machine is clean or every job on it is running, and a caller
	// who meant to reclaim space has no way to tell which.
	if len(plan.Kept) > 0 {
		fmt.Fprintln(out, "==> Kept")
		for _, k := range plan.Kept {
			fmt.Fprintf(out, "  kept           %s\n", k)
		}
	}
}

func cmdConfig(args []string) (int, error) {
	// `validate` is a subcommand and `--effective` is a flag, because the first
	// answers a question about a file and the second changes what this command
	// prints about the one it already resolved.
	sub := ""
	if len(args) > 0 && args[0] == "validate" {
		sub, args = args[0], args[1:]
	}
	name := "config"
	if sub != "" {
		name = "config " + sub
	}
	fs := newFlagSet(name)
	asJSON := fs.Bool("json", false, "write a structured answer")
	write := fs.Bool("write", false, "write the effective configuration to disk, so it can be edited")
	effective := fs.Bool("effective", false, "print the configuration that WOULD be used, as JSON, without writing it")
	path := fs.String("path", "", "validate this file instead of the one the search resolves")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	if sub == "validate" {
		if *write {
			return exitCannot, errors.New("config validate does not write. Drop --write, or run config --write on its own")
		}
		return runConfigValidate(*asJSON, *path)
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	src, err := toolkit.ResolveConfig()
	if err != nil {
		return exitCannot, err
	}
	resolved := src.Path
	if *write {
		// ⛔ --write ALWAYS writes the STATE DIRECTORY's file, never the one the
		// search resolved. A caller standing in a checkout that carries a
		// wsl-toolkit.json would otherwise have `config --write` overwrite a
		// TRACKED file with the whole built-in catalog, which is a report
		// command editing somebody's repository. WSL-51.
		if resolved, err = toolkit.ConfigPath(); err != nil {
			return exitCannot, err
		}
		// ⚠ The written file carries the CURRENT catalog, so an edit starts
		// from what the tool is actually doing rather than from an empty
		// skeleton somebody has to guess the shape of.
		cfg.Images = cfg.Catalog()
		cfg.Matrix = cfg.MatrixDefault()
		if err := cfg.Write(); err != nil {
			return exitCannot, err
		}
		logf("  wrote %s", resolved)
	}
	home, err := toolkit.Home()
	if err != nil {
		return exitCannot, err
	}
	if *effective {
		// ⛔ THE EFFECTIVE CONFIGURATION AND NOTHING ELSE, so a caller can pipe
		// it into a file, edit it and pass it back with --config. `config
		// --json` carries the report AROUND the configuration; this is the
		// configuration.
		cfg.Images = cfg.Catalog()
		cfg.Matrix = cfg.MatrixDefault()
		return exitOK, writeJSON(cfg)
	}
	if *asJSON {
		return exitOK, writeJSON(map[string]any{
			"schema":      "wsl-toolkit-config-report/1",
			"path":        resolved,
			"exists":      fileExists(resolved),
			"from":        src.From,
			"searched":    src.Searched,
			"home":        home,
			"instance":    toolkit.SelectedInstance.Name,
			"base":        cfg.Base,
			"jobs":        cfg.Jobs,
			"images":      cfg.Catalog(),
			"matrix":      cfg.MatrixDefault(),
			"builtin":     len(cfg.Images) == 0,
			"fingerprint": cfg.Fingerprint(),
		})
	}
	fmt.Println(resolved)
	fmt.Fprintf(os.Stderr, "  exists      %v\n", fileExists(resolved))
	fmt.Fprintf(os.Stderr, "  resolved    from %s\n", src.From)
	// ⭐ THE ORDER IS PRINTED, not only the winner. A caller with a
	// wsl-toolkit.json in a parent directory silently changes which
	// configuration is used, and the only honest answer to "why is it using
	// that one" is the list of places that were looked at. WSL-51.
	for i, cand := range src.Searched {
		fmt.Fprintf(os.Stderr, "    %d. %s\n", i+1, cand)
	}
	fmt.Fprintf(os.Stderr, "  home        %s\n", home)
	if toolkit.SelectedInstance.Name != toolkit.DefaultInstance {
		fmt.Fprintf(os.Stderr, "  instance    %s\n", toolkit.SelectedInstance.Name)
	}
	fmt.Fprintf(os.Stderr, "  base        %s from %s as %s\n", cfg.Base.Name, cfg.Base.Image, cfg.Base.User)
	// ⛔ THE DEFAULTS A JOB ACTUALLY RUNS UNDER WERE NOT ON THIS REPORT. `config`
	// is where a caller looks to find out what the tool will do, and the
	// container lifetime is the setting most likely to surprise one; issue 29
	// reached this repository as "if a persistent option exists, the docs are
	// lacking, why else would an agent not know about it".
	fmt.Fprintf(os.Stderr, "  jobs        %s containers, platform %s\n",
		cfg.Jobs.ContainerLifecycle, orNative(cfg.Jobs.Platform))
	if cfg.Jobs.Workspace != "" {
		fmt.Fprintf(os.Stderr, "  workspace   %s, relative to %s\n", cfg.Jobs.Workspace, filepath.Dir(resolved))
	}
	fmt.Fprintf(os.Stderr, "  catalog     %d image(s), %s\n", len(cfg.Catalog()), builtinOrStored(cfg))
	fmt.Fprintf(os.Stderr, "  matrix      %d image(s) by default\n", len(cfg.MatrixDefault()))
	fmt.Fprintf(os.Stderr, "  fingerprint %s\n", cfg.Fingerprint())
	fmt.Fprintf(os.Stderr, "\n  wsl-toolkit config --write puts the effective configuration on disk to edit.\n")
	return exitOK, nil
}

// orNative names the default platform in the words the flag uses, because an
// empty string on a report reads as a missing value rather than as a choice.
func orNative(platform string) string {
	if platform == "" {
		return "native"
	}
	return platform
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
