package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	hostEngine := fs.Bool("host-engine", false, "also report what the host's own container engine holds, which distro new --image pulls into, and print the commands that would free it without running any")
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
		if *hostEngine {
			return exitCannot, errors.New("--job narrows the report to one job, and the host engine holds nothing a job made. Pass one")
		}
	}
	narrow := func(rep toolkit.ResourceReport) toolkit.ResourceReport {
		if *hostEngine {
			// The host engine is this machine's whichever route the rest of the report took.
			rep.Machine.HostEngine = toolkit.ReadHostEngineHolding(ctx)
		}
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
	images := fs.Bool("images", false, "also prune images the engine in the base is holding. Refused beside --job or --workspace, since an image is not one job's")
	asJSON := fs.Bool("json", false, "write a structured answer")
	viaHelper := fs.Bool("via-helper", false, "go through the local helper even when this process could call wsl.exe itself")
	job, workspace := new(string), new(string)
	fs.Var(nonEmpty{job}, "job", "clean up one job id and leave the rest of the store alone. An id nothing in this instance belongs to is refused")
	fs.Var(nonEmpty{workspace}, "workspace", "clean up only the jobs that copied this directory, read as run --workspace reads it. A job that recorded no workspace is left alone")
	caches := fs.Bool("caches", false, "clean up the named caches run --cache made, and nothing else: list them, and remove them with --apply. A cache a container names is kept")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	// ⛔ --caches IS THE CACHES ALONE, so a flag that chooses among jobs or adds
	// images has nothing to act on beside it, and is refused rather than
	// ignored. A cache's last use is not recorded, so no age spares one. WSL-122.
	if *caches {
		for _, other := range []struct {
			name  string
			given bool
		}{
			{"--job", *job != ""}, {"--workspace", *workspace != ""}, {"--images", *images},
			{"--older-than", *olderThan != 0}, {"--include-live", *includeLive},
		} {
			if other.given {
				return exitCannot, fmt.Errorf("--caches cleans up the named caches alone, and %s chooses among jobs or adds images. Run gc --caches without it, and gc for the jobs", other.name)
			}
		}
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if *olderThan < 0 {
		return exitCannot, fmt.Errorf("--older-than %s is negative. Pass 0 for no age limit, or a positive duration", *olderThan)
	}
	// ⛔ AN ID IS HELD TO ITS SHAPE HERE, as the helper already held it. A
	// shortened id narrowed the plan to nothing and exited 0. WSL-115.
	if *job != "" && !toolkit.ValidJobID(*job) {
		return exitCannot, fmt.Errorf("--job %q is not a job id: an id is 16 lowercase hex characters, as run --json and logs print it", *job)
	}
	if *job != "" && *workspace != "" {
		return exitCannot, errors.New("--job and --workspace both choose what gc cleans up. Pass one")
	}
	// ⛔ AN IMAGE IS NOT ONE JOB'S OR ONE PROJECT'S. The prune takes every image
	// no container is using, so with a narrowing flag it cleaned the whole
	// store's cache. WSL-114.
	if *images && (*job != "" || *workspace != "") {
		return exitCannot, errors.New("--images prunes every image the base holds, which is not one job's or one project's. Run gc --images without --job or --workspace")
	}
	key := ""
	if *workspace != "" {
		dir, err := pathFromProject(cfg, *workspace)
		if err == nil {
			key, err = toolkit.WorkspaceKey(dir)
		}
		if err != nil {
			return exitCannot, fmt.Errorf("--workspace: %w", err)
		}
	}
	// ⚠ NAMING A JOB IS NOT A WAY OF SAYING --include-live. A job that is still
	// running is spared exactly as it would be without --job, which is what
	// WSL-36 settled for the whole store and applies unchanged to one row of it.
	policy := toolkit.CleanupPolicy{OlderThan: *olderThan, IncludeLive: *includeLive, Job: *job, Workspace: key}
	policy.Caches = *caches
	if c, err := useHelper(ctx, *viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		plan, err := c.Cleanup(ctx, *apply, policy, *images)
		return finishGC(plan, err, *asJSON)
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
		home, err := toolkit.Home()
		if err != nil {
			return exitCannot, err
		}
		if err := unregisteredJobRefusal(home, policy); err != nil {
			return exitCannot, err
		}
		logf("  %s is not registered, so there is nothing inside it to clean up", st.Name)
		if *asJSON {
			return exitOK, writeJSON(toolkit.CleanupPlan{Schema: toolkit.CleanupSchema, DryRun: !*apply})
		}
		return exitOK, nil
	}
	plan, err := runner.Cleanup(ctx, *apply, policy, *images)
	return finishGC(plan, err, *asJSON)
}

// unregisteredJobRefusal refuses a --job that nothing in this instance belongs
// to, for an instance whose base is not registered and so has no guest to ask.
//
// ⛔ THE REFUSAL WAS MADE ONLY FROM WHAT THE GUEST HOLDS. An instance with no
// base returned before it was asked, so an id from another instance exited 0
// there, which is WSL-115's own defect. A correctness review found it.
func unregisteredJobRefusal(home string, policy toolkit.CleanupPolicy) error {
	if policy.Job == "" {
		return nil
	}
	if js, err := toolkit.ReadJobState(home, policy.Job); err == nil && js.Known {
		return nil
	}
	_, err := policy.Unmatched(nil)
	return err
}

// finishGC renders a cleanup and turns it into an exit code.
//
// ⛔ A REFUSAL PRINTS NO PLAN. Nothing was attempted, and an empty document
// beside the refusal reads as a cleanup that found nothing to do. WSL-115.
func finishGC(plan toolkit.CleanupPlan, err error, asJSON bool) (int, error) {
	if errors.Is(err, toolkit.ErrUnknownJob) || errors.Is(err, toolkit.ErrWorkspaceViaHelper) {
		return exitCannot, err
	}
	if asJSON {
		if writeErr := writeJSON(plan); writeErr != nil {
			return exitCannot, writeErr
		}
	} else {
		renderCleanup(plan)
	}
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
	for _, s := range plan.Sessions {
		fmt.Fprintf(out, "  session        %s\n", s)
	}
	for _, i := range plan.Images {
		fmt.Fprintf(out, "  image          %s\n", i)
	}
	for _, c := range plan.Caches {
		fmt.Fprintf(out, "  cache          %s\n", c)
	}
	if !plan.DryRun {
		for _, r := range plan.Removed {
			fmt.Fprintf(out, "  ok             %s\n", r)
		}
	}
	for _, f := range plan.Failed {
		fmt.Fprintf(out, "  ! could not remove %s\n", f)
	}
	if plan.DryRun && len(plan.Containers)+len(plan.GuestDirs)+len(plan.HostDirs)+len(plan.Sessions)+len(plan.Images)+len(plan.Caches) == 0 {
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
	var c configFlags
	if err := configShared.parse(sub, &c, args); err != nil {
		return exitCannot, err
	}
	if sub == "validate" {
		return runConfigValidate(c.asJSON, msysHostPath(c.path))
	}
	cfg, err := loadConfig()
	if err != nil {
		// ⛔ A REFUSAL STILL SAYS WHICH FILE WON AND WHERE IT LOOKED. `config` is
		// the command a caller runs to find out why a configuration is in effect,
		// and a refused one is exactly when that question is asked. WSL-74.
		if src, srcErr := toolkit.ResolveConfig(); srcErr == nil {
			renderConfigSearch(src)
		}
		return exitCannot, err
	}
	src, err := toolkit.ResolveConfig()
	if err != nil {
		return exitCannot, err
	}
	resolved := src.Path
	if c.write {
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
	// ⛔ A SECOND STATE DIRECTORY IS A SECOND MACHINE to this tool, so the
	// report that names home names every copy of it. WSL-118.
	var copies []toolkit.StateCopy
	if root, err := toolkit.StateRoot(); err == nil {
		copies = toolkit.StateCopies(root)
	}
	if c.effective {
		// ⛔ THE EFFECTIVE CONFIGURATION AND NOTHING ELSE, so a caller can pipe
		// it into a file, edit it and pass it back with --config. `config
		// --json` carries the report AROUND the configuration; this is the
		// configuration.
		cfg.Images = cfg.Catalog()
		cfg.Matrix = cfg.MatrixDefault()
		return exitOK, writeJSON(cfg)
	}
	if c.asJSON {
		doc := map[string]any{
			"schema":       "wsl-toolkit-config-report/1",
			"path":         resolved,
			"exists":       fileExists(resolved),
			"from":         src.From,
			"searched":     src.Searched,
			"home":         home,
			"state_copies": copies,
			"instance":     toolkit.SelectedInstance.Name,
			"base":         cfg.Base,
			"jobs":         cfg.Jobs,
			"images":       cfg.Catalog(),
			"matrix":       cfg.MatrixDefault(),
			"builtin":      len(cfg.Images) == 0,
			"fingerprint":  cfg.Fingerprint(),
		}
		// WSL-131, WSL-129: present only where there is one, so a report about a
		// configuration without either reads as it did.
		if cfg.Project() != "" {
			doc["project"] = cfg.Project()
		}
		if cfg.Requires != "" {
			doc["requires"] = cfg.Requires
		}
		return exitOK, writeJSON(doc)
	}
	fmt.Println(resolved)
	fmt.Fprintf(os.Stderr, "  exists      %v\n", fileExists(resolved))
	renderConfigSearch(src)
	if cfg.Project() != "" {
		fmt.Fprintf(os.Stderr, "  project     %s lends this instance its job defaults and requires\n", cfg.Project())
	}
	if cfg.Requires != "" {
		fmt.Fprintf(os.Stderr, "  requires    wsl-toolkit %s or newer, and this is %s\n", cfg.Requires, toolkit.Version)
	}
	fmt.Fprintf(os.Stderr, "  home        %s\n", home)
	for _, sc := range copies {
		fmt.Fprintf(os.Stderr, "  ⚠ copy      %s\n", toolkit.StateCopyNote(sc))
	}
	if toolkit.SelectedInstance.Name != toolkit.DefaultInstance {
		fmt.Fprintf(os.Stderr, "  instance    %s\n", toolkit.SelectedInstance.Name)
	}
	fmt.Fprintf(os.Stderr, "  base        %s from %s as %s\n", cfg.Base.Name, cfg.Base.Image, cfg.Base.User)
	automount, err := toolkit.NormalizeAutomount(cfg.Base.Automount)
	if err != nil {
		return exitCannot, err
	}
	interop, err := toolkit.NormalizeBaseInterop(cfg.Base.Interop)
	if err != nil {
		return exitCannot, err
	}
	toolset, err := toolkit.NormalizeBaseToolset(cfg.Base.Toolset)
	if err != nil {
		return exitCannot, err
	}
	fmt.Fprintf(os.Stderr, "  access      automount %s, interop %s, systemd %v, passwordless sudo %v, toolset %s\n",
		automount, interop, cfg.Base.Systemd, cfg.Base.PasswordlessSudo, toolset)
	mounts, err := cfg.ResolvedBaseMounts()
	if err != nil {
		return exitCannot, err
	}
	for _, mount := range mounts {
		fmt.Fprintf(os.Stderr, "  grant       %s %s <- %s%s\n", mount.Mode, mount.Target, mount.Source, goneMark(mount))
	}
	for _, adapter := range cfg.Base.Adapters {
		if adapter.InstallerSHA256 != "" {
			fmt.Fprintf(os.Stderr, "  adapter     %s, and an installer approved by digest %s\n", adapter.Name, adapter.InstallerSHA256)
			continue
		}
		fmt.Fprintf(os.Stderr, "  adapter     %s\n", adapter.Name)
	}
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
	renderJobDefaults(cfg)
	fmt.Fprintf(os.Stderr, "  catalog     %d image(s), %s\n", len(cfg.Catalog()), builtinOrStored(cfg))
	fmt.Fprintf(os.Stderr, "  matrix      %d image(s) by default\n", len(cfg.MatrixDefault()))
	fmt.Fprintf(os.Stderr, "  fingerprint %s\n", cfg.Fingerprint())
	fmt.Fprintf(os.Stderr, "\n  %s puts the effective configuration on disk to edit.\n", toolkit.ToolCommand("config --write"))
	return exitOK, nil
}

// renderConfigSearch writes which file the search resolved and every place it
// looked, to stderr.
//
// ⭐ THE ORDER IS PRINTED, not only the winner. A caller with a
// wsl-toolkit.json in a parent directory silently changes which configuration is
// used, and the only honest answer to "why is it using that one" is the list of
// places that were looked at. WSL-51.
func renderConfigSearch(src toolkit.ConfigSource) {
	fmt.Fprintf(os.Stderr, "  resolved    %s, from %s\n", src.Path, src.From)
	for i, cand := range src.Searched {
		fmt.Fprintf(os.Stderr, "    %d. %s\n", i+1, cand)
	}
}

// renderJobDefaults writes each job default a configuration sets, to stderr,
// one line each, and nothing where it sets none. WSL-122, WSL-131.
func renderJobDefaults(cfg toolkit.Config) {
	j := cfg.Jobs
	if j.Image != "" {
		fmt.Fprintf(os.Stderr, "  image       %s, where run names none\n", j.Image)
	}
	if j.Timeout != "" {
		fmt.Fprintf(os.Stderr, "  timeout     %s\n", j.Timeout)
	}
	if j.User != "" {
		fmt.Fprintf(os.Stderr, "  user        %s\n", j.User)
	}
	if len(j.Env) > 0 || j.EnvFile != "" {
		names := make([]string, 0, len(j.Env))
		for name := range j.Env {
			names = append(names, name)
		}
		sort.Strings(names)
		from := ""
		if j.EnvFile != "" {
			from = ", after the file " + j.EnvFile
		}
		fmt.Fprintf(os.Stderr, "  env         %s%s\n", strings.Join(names, " "), from)
	}
	if j.MaxBytes > 0 {
		fmt.Fprintf(os.Stderr, "  max bytes   %d\n", j.MaxBytes)
	}
	if j.MaxEntries > 0 {
		fmt.Fprintf(os.Stderr, "  max entries %d\n", j.MaxEntries)
	}
	for _, c := range j.Cache {
		fmt.Fprintf(os.Stderr, "  cache       %s\n", c)
	}
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

// configFlags are the values of the flag set config and config validate share.
type configFlags struct {
	asJSON, write, effective bool
	path                     string
}

func (c *configFlags) bind(fs *flag.FlagSet) {
	fs.BoolVar(&c.asJSON, "json", false, "write a structured answer")
	fs.BoolVar(&c.write, "write", false, "write the effective configuration to disk, so it can be edited")
	fs.BoolVar(&c.effective, "effective", false, "print the configuration that WOULD be used, as JSON, without writing it")
	fs.StringVar(&c.path, "path", "", "validate this file instead of the one the search resolves")
}

// configShared names what config and config validate read. `config --path`
// reported on the configuration the search resolved and not on the file it
// named. WSL-104.
var configShared = sharedFlags[configFlags]{
	group: "config",
	order: []string{"", "validate"},
	reads: map[string][]string{"": {"json", "write", "effective"}, "validate": {"json", "path"}},
	bind:  (*configFlags).bind,
}
