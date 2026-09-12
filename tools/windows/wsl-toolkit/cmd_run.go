package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// jobFlags are what run and matrix share, so the two cannot drift into
// accepting different spellings of the same thing.
type jobFlags struct {
	command     string
	scriptFile  string
	workspace   string
	artifactDir string
	excludes    stringList
	env         stringList
	timeout     time.Duration
	noNetwork   bool
	user        string
	platform    string
	lifecycle   string
	maxBytes    int64
	maxEntries  int
	maxOutput   int64
	asJSON      bool
	ensure      bool
	viaHelper   bool
	tick        time.Duration
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func (j *jobFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&j.command, "c", "", "the shell command to run in the container")
	fs.StringVar(&j.scriptFile, "script", "", "a file on this machine whose bytes are the command")
	fs.StringVar(&j.workspace, "workspace", "", "a directory on this machine, COPIED into the container as /work")
	fs.StringVar(&j.artifactDir, "artifacts", "", "a directory on this machine to receive whatever the container leaves in /out")
	fs.Var(&j.excludes, "exclude", "a glob to leave out of the workspace copy. Repeatable")
	fs.Var(&j.env, "env", "NAME=VALUE passed to the container. Repeatable")
	fs.DurationVar(&j.timeout, "timeout", 30*time.Minute, "how long one container may run before lifecycle cleanup; the row reports 124")
	fs.DurationVar(&j.tick, "tick", 0, "emit a heartbeat for each running job at this interval. 0 is off, and anything under 1s is raised to it")
	fs.BoolVar(&j.noNetwork, "no-network", false, "run with no network at all")
	fs.StringVar(&j.user, "user", "", "what the container runs as: a name, a uid, or uid:gid. Empty means the image's default")
	fs.StringVar(&j.platform, "platform", "", "the Linux OCI platform. Empty uses jobs.platform, then the native platform")
	fs.StringVar(&j.lifecycle, "container-lifecycle", "", "persistent keeps the stopped container and its job directory; ephemeral removes both. Empty uses jobs.container_lifecycle, whose default is persistent")
	fs.Int64Var(&j.maxBytes, "max-bytes", toolkit.DefaultWorkspaceLimits().MaxBytes, "refuse a workspace or an artifact set larger than this")
	fs.IntVar(&j.maxEntries, "max-entries", toolkit.DefaultWorkspaceLimits().MaxEntries, "refuse a workspace or an artifact set with more entries than this")
	fs.Int64Var(&j.maxOutput, "max-output", 0, "how many bytes of the command's output the ANSWER keeps. 0 uses the default. The transcript is complete whatever this says")
	fs.BoolVar(&j.asJSON, "json", false, "write a structured answer")
	fs.BoolVar(&j.ensure, "ensure-base", true, "build the base first when it is missing or unusable")
	fs.BoolVar(&j.viaHelper, "via-helper", false, "go through the local helper even when this process could call wsl.exe itself")
}

// script reads the job payload, from -c or from a file.
//
// ⚠ THE FILE'S BYTES GET THE SAME REPAIR wsl-toolkit.ps1's -CommandFile GIVES
// THEM: CRLF becomes LF in the COPY being sent, and a byte order mark is left
// out. The file on disk is never written to. A script written on Windows and run
// by /bin/sh otherwise fails on its first line with a message about a character
// nobody can see.
func (j *jobFlags) script() ([]byte, error) {
	if j.command != "" && j.scriptFile != "" {
		return nil, errors.New("-c and --script are two spellings of one argument, so passing both is refused rather than resolved by a precedence nobody would remember")
	}
	if j.command != "" {
		// ⛔ -c GETS THE SAME REPAIR AS --script, and it did not. A multi-line
		// command assembled in PowerShell carries CRLF, and /bin/sh reads the
		// carriage return as part of the last word on the line: `2>/dev/null`
		// becomes a file named `/dev/null` followed by an invisible byte, and
		// the error names a file nobody wrote. One channel repairing its
		// payload while its sibling does not is the one-gated-door shape.
		return toolkit.RepairGuestScript([]byte(j.command + "\n"))
	}
	if j.scriptFile == "" {
		return nil, errors.New("nothing to run: pass -c COMMAND or --script FILE")
	}
	raw, err := os.ReadFile(j.scriptFile)
	if err != nil {
		return nil, err
	}
	return toolkit.RepairGuestScript(raw)
}

func (j *jobFlags) envMap() (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range j.env {
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("--env %q is not NAME=VALUE", pair)
		}
		// ⛔ Refused HERE, where the caller's own spelling is still available.
		// The container invocation silently skipped a name it could not use, so
		// `--env BAD-NAME=x` was accepted, dropped, and the job ran without it.
		if !toolkit.ValidEnvName(name) {
			return nil, fmt.Errorf("--env %q: %q is not a usable environment name. A name is letters, digits and underscores, and does not start with a digit", pair, name)
		}
		out[name] = value
	}
	return out, nil
}

// check refuses the values that were accepted and then quietly meant something
// else. ⚠ A NEGATIVE TIMEOUT DISABLED THE DEADLINE: the code applied one only
// when the duration was positive, so `--timeout -1s` ran unbounded, which is the
// opposite of what a caller writing a negative number could possibly want.
func (j *jobFlags) check() error {
	if j.timeout < 0 {
		return fmt.Errorf("--timeout %s is negative. Pass 0 for no deadline, or a positive duration", j.timeout)
	}
	if j.maxBytes <= 0 {
		return fmt.Errorf("--max-bytes %d is not a size. Pass a positive number of bytes", j.maxBytes)
	}
	if j.maxEntries <= 0 {
		return fmt.Errorf("--max-entries %d is not a count. Pass a positive number of entries", j.maxEntries)
	}
	if j.maxOutput < 0 {
		return fmt.Errorf("--max-output %d is negative. Pass 0 for the default, or a positive number of bytes", j.maxOutput)
	}
	if j.lifecycle != "" && j.lifecycle != toolkit.ContainerPersistent && j.lifecycle != toolkit.ContainerEphemeral {
		return fmt.Errorf("--container-lifecycle %q must be %q or %q", j.lifecycle, toolkit.ContainerPersistent, toolkit.ContainerEphemeral)
	}
	return nil
}

// applyConfig selects job defaults and resolves host paths before a job starts.
func (j *jobFlags) applyConfig(cfg toolkit.Config) error {
	workspaceFromConfig := false
	if j.lifecycle == "" {
		j.lifecycle = cfg.Jobs.ContainerLifecycle
	}
	if j.platform == "" {
		j.platform = cfg.Jobs.Platform
	}
	if j.workspace == "" && cfg.Jobs.Workspace != "" {
		j.workspace = cfg.Jobs.Workspace
		workspaceFromConfig = true
	}
	platform, err := toolkit.NormalizePlatform(j.platform)
	if err != nil {
		return fmt.Errorf("--platform: %w", err)
	}
	if platform == "" {
		// ⛔ RESOLVED, NEVER LEFT EMPTY. An empty platform meant no --platform on
		// the podman command line, and podman then selected whatever variant of
		// the image the local store already held. NativePlatform carries the
		// measurement that found it.
		platform = toolkit.NativePlatform()
	}
	j.platform = platform
	if j.workspace != "" {
		if workspaceFromConfig {
			j.workspace, err = pathFromConfig(cfg, j.workspace)
		} else {
			j.workspace, err = pathFromProject(cfg, j.workspace)
		}
		if err != nil {
			return fmt.Errorf("--workspace: %w", err)
		}
		// ⛔ CHECKED AFTER RESOLUTION, on the path that will actually be read.
		// The argument a caller typed is `.`, and `.` is never the thing that is
		// unsafe; what it resolved to is.
		if err := toolkit.AssertProjectPath("--workspace resolved to", j.workspace); err != nil {
			return err
		}
	}
	if j.scriptFile != "" {
		j.scriptFile, err = pathFromProject(cfg, j.scriptFile)
		if err != nil {
			return fmt.Errorf("--script: %w", err)
		}
	}
	if j.artifactDir != "" {
		j.artifactDir, err = pathFromProject(cfg, j.artifactDir)
		if err != nil {
			return fmt.Errorf("--artifacts: %w", err)
		}
		// ⚠ THE ARTIFACT DIRECTORY IS WRITTEN TO, which makes the same mistake
		// worse here than on the workspace: one walks a drive and the other
		// scatters a job's output across a home directory.
		if err := toolkit.AssertProjectPath("--artifacts resolved to", j.artifactDir); err != nil {
			return err
		}
	}
	return nil
}

func pathFromConfig(cfg toolkit.Config, value string) (string, error) {
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Abs(filepath.Join(filepath.Dir(cfg.Path()), value))
}

func pathFromProject(cfg toolkit.Config, value string) (string, error) {
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	if cfg.Source() == "--config" || cfg.Source() == "the working directory or a parent" {
		return pathFromConfig(cfg, value)
	}
	return filepath.Abs(value)
}

func (j *jobFlags) limits() toolkit.WorkspaceLimits {
	return toolkit.WorkspaceLimits{MaxBytes: j.maxBytes, MaxEntries: j.maxEntries}
}

// sinks are where the container's own bytes go WHILE IT RUNS.
//
// ⛔ UNDER --json THERE IS NO LIVE STDOUT, and that is not an oversight.
// This process's stdout carries the structured answer, so a container writing to
// it as well would produce a document nothing can parse. The complete output is
// on the result and, whatever its size, in the transcript the result names.
func (j *jobFlags) sinks() (out, errw io.Writer) {
	if j.asJSON {
		return nil, os.Stderr
	}
	return os.Stdout, os.Stderr
}

func cmdRun(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("run")
	var j jobFlags
	j.bind(fs)
	image := fs.String("image", "", "a catalog id or a fully qualified reference")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	if *image == "" {
		return exitCannot, errors.New("--image is required. wsl-toolkit images lists the catalog")
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if err := j.applyConfig(cfg); err != nil {
		return exitCannot, err
	}
	if err := j.check(); err != nil {
		return exitCannot, err
	}
	payload, err := j.script()
	if err != nil {
		return exitCannot, err
	}
	env, err := j.envMap()
	if err != nil {
		return exitCannot, err
	}
	ref, err := resolveImage(cfg, *image)
	if err != nil {
		return exitCannot, err
	}
	if c, err := useHelper(ctx, j.viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		if j.ensure {
			if _, err := c.BaseEnsure(ctx, false); err != nil {
				return exitCannot, err
			}
		}
		res, err := helperRunJob(ctx, c, j, ref, *image, payload, env)
		if err != nil {
			return exitCannot, err
		}
		return reportJob(res, j.asJSON)
	}
	runner, err := toolkit.NewRunner(cfg, note)
	if err != nil {
		return exitCannot, err
	}
	if err := ensureBase(ctx, runner, j.ensure); err != nil {
		return exitCannot, err
	}
	if err := runner.Base().EnsurePlatform(ctx, j.platform); err != nil {
		return exitCannot, err
	}
	liveOut, liveErr := j.sinks()
	res := runner.Run(ctx, toolkit.JobSpec{
		Image: ref, Script: payload, Workspace: j.workspace, Excludes: toolkit.SortedExcludes(j.excludes),
		ArtifactDir: j.artifactDir, Env: env, Timeout: j.timeout, Network: !j.noNetwork,
		Platform: j.platform, ContainerLifecycle: j.lifecycle,
		Limits: j.limits(), Label: *image, User: j.user,
		Stdout: liveOut, Stderr: liveErr, MaxOutput: j.maxOutput,
		OnTick: tickPrinter(j), TickEvery: j.tick,
	})
	return reportJob(res, j.asJSON)
}

// reportJob is the one renderer, so a job run through the helper and a job run
// directly are reported identically.
func reportJob(res toolkit.JobResult, asJSON bool) (int, error) {
	// ⛔ SEALED HERE, at the point of rendering. A helper-run result is
	// produced on one machine and amended on this one when the artifact
	// download fails, so a verdict computed where the job ran would be stale in
	// exactly the case the field exists for.
	res.Seal()
	if asJSON {
		return res.EffectiveExit, writeJSON(res)
	}
	// ⛔ NOTHING IS REPLAYED HERE. The container's bytes went to this
	// process's own streams as they were written, so printing them again would
	// double every line. stdout still carries the container's output and nothing
	// else, which is what lets a caller read a value off it.
	if res.StdoutTruncated || res.StderrTruncated {
		// ⚠ SAID OUT LOUD. The in-memory copy is bounded and the transcript
		// is not, and a caller who never learns which one they are reading is
		// the caller this defect was filed by.
		logf("  ! the recorded copy of this job's output was cut at the capture limit; the complete text is under %s", res.Transcript)
	}
	if res.Error != "" {
		logf("  ! %s", res.Error)
	}
	if res.ArtifactError != "" {
		logf("  ! artifacts: %s", res.ArtifactError)
		switch res.RetainedKind {
		case "guest":
			logf("  ! the output is still in the guest at %s", res.Retained)
		case "helper":
			logf("  ! the helper is still holding artifact set %s", res.Retained)
		}
	}
	if res.Container != "" {
		logf("  container %s is kept with its job directory. Remove it with: wsl-toolkit gc --job %s --apply", res.Container, res.ID)
	}
	logf("  %s exited %d in %s", res.Label, res.Exit, res.Duration.Round(time.Millisecond))
	if hint := transcriptHint(res); hint != "" {
		logf("  %s", hint)
	}
	return jobVerdict(res), nil
}

// transcriptHint is the one line that makes `logs` reachable.
//
// ⛔ THE JOB ID WAS NOWHERE IN THE HUMAN OUTPUT. `run` printed the image
// label and the exit code, `matrix` printed a table of labels, and the id a
// transcript is filed under appeared only under --json. So `wsl-toolkit logs
// JOB-...` could not be typed without first running `wsl-toolkit logs` with no
// arguments to go looking for it, and a feature nobody can find is one that was
// not shipped.
//
// ⚠ IT NAMES THE COMMAND, not the directory. A path is what this tool knows
// and a command is what the reader wants next, and the path is one `logs` call
// away for anyone who wants it.
func transcriptHint(res toolkit.JobResult) string {
	if res.Transcript == "" || res.ID == "" {
		return ""
	}
	if res.StdoutTruncated || res.StderrTruncated {
		// Already said, with the path, by the line about the capture limit.
		return ""
	}
	return "the complete output is kept: wsl-toolkit logs " + res.ID
}

// jobVerdict is toolkit.JobResult.Verdict, kept as a name this file already
// uses. ⛔ The rule itself moved into the result, because a structured answer
// that cannot state its own verdict is the defect WSL-46 is about.
func jobVerdict(res toolkit.JobResult) int { return res.Verdict() }

func cmdMatrix(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("matrix")
	var j jobFlags
	j.bind(fs)
	images := fs.String("images", "", "catalog ids, libc:musl, kind:legacy or all. Comma separated. Empty means the configured default")
	parallel := fs.Int("parallel", toolkit.DefaultParallel, "how many rows run at once")
	transcripts := fs.String("transcripts", "", "a directory to write one transcript per row into")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if err := j.applyConfig(cfg); err != nil {
		return exitCannot, err
	}
	if err := j.check(); err != nil {
		return exitCannot, err
	}
	if *parallel < 1 {
		return exitCannot, fmt.Errorf("--parallel %d would run no rows at all. Pass 1 or more", *parallel)
	}
	payload, err := j.script()
	if err != nil {
		return exitCannot, err
	}
	env, err := j.envMap()
	if err != nil {
		return exitCannot, err
	}
	var selectors []string
	if *images != "" {
		selectors = strings.Split(*images, ",")
	}
	selected, err := cfg.SelectImages(selectors)
	if err != nil {
		return exitCannot, err
	}
	if *transcripts != "" {
		if *transcripts, err = pathFromProject(cfg, *transcripts); err != nil {
			return exitCannot, fmt.Errorf("--transcripts: %w", err)
		}
	}
	if c, err := useHelper(ctx, j.viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		if j.ensure {
			if _, err := c.BaseEnsure(ctx, false); err != nil {
				return exitCannot, err
			}
		}
		ids := make([]string, 0, len(selected))
		for _, img := range selected {
			ids = append(ids, img.ID)
		}
		report, err := helperRunMatrix(ctx, c, j, ids, *parallel, payload, env, *transcripts)
		if err != nil {
			return exitCannot, err
		}
		return reportMatrix(report, j.asJSON)
	}
	runner, err := toolkit.NewRunner(cfg, note)
	if err != nil {
		return exitCannot, err
	}
	if err := ensureBase(ctx, runner, j.ensure); err != nil {
		return exitCannot, err
	}
	if err := runner.Base().EnsurePlatform(ctx, j.platform); err != nil {
		return exitCannot, err
	}
	logf("  %d image(s), %d at a time, %s per row", len(selected), *parallel, j.timeout)
	report, err := runner.RunMatrix(ctx, toolkit.MatrixSpec{
		Images: selected, Script: payload, Workspace: j.workspace,
		Excludes: toolkit.SortedExcludes(j.excludes), ArtifactDir: j.artifactDir,
		Env: env, Timeout: j.timeout, Network: !j.noNetwork, Parallel: *parallel,
		Platform: j.platform, ContainerLifecycle: j.lifecycle,
		Limits: j.limits(), Transcripts: *transcripts, User: j.user,
		MaxOutput: j.maxOutput, OnRow: rowPrinter(),
		OnTick: tickPrinter(j), TickEvery: j.tick,
	})
	if err != nil {
		return exitCannot, err
	}
	return reportMatrix(report, j.asJSON)
}

// reportMatrix is the one renderer, for the same reason reportJob is.
func reportMatrix(report toolkit.MatrixReport, asJSON bool) (int, error) {
	// Every row is sealed for the same reason one job's result is: the fleet's
	// rows are amended on this machine after a helper download fails.
	for i := range report.Rows {
		report.Rows[i].Seal()
	}
	if asJSON {
		return report.Verdict(), writeJSON(report)
	}
	if err := toolkit.RenderMatrix(os.Stderr, report); err != nil {
		return exitCannot, err
	}
	return report.Verdict(), nil
}

// tickPrinter renders a heartbeat, or returns nil when nobody asked for one.
//
// ⛔ NIL WHEN --tick IS 0, so the ticker is never started rather than started
// and discarded. A goroutine per row whose output goes nowhere is a cost with
// no reader.
//
// ⚠ UNDER --json IT GOES TO STDERR LIKE EVERYTHING ELSE THIS PROGRAM SAYS.
// stdout carries the answer alone, and a heartbeat interleaved with a JSON
// document would produce something nothing can parse.
func tickPrinter(j jobFlags) func(toolkit.TickEvent) {
	if j.tick <= 0 {
		return nil
	}
	var mu sync.Mutex
	return func(t toolkit.TickEvent) {
		mu.Lock()
		defer mu.Unlock()
		left := ""
		if t.RemainingMS != nil {
			left = fmt.Sprintf(", %s of its deadline left",
				(time.Duration(*t.RemainingMS) * time.Millisecond).Round(time.Second))
		}
		logf("  ~ %s running %s%s, %d/%d bytes out/err",
			t.Label, (time.Duration(t.ElapsedMS) * time.Millisecond).Round(time.Second), left,
			t.StdoutBytes, t.StderrBytes)
	}
}

// rowPrinter reports a fleet row as it finishes.
//
// ⭐ It is the difference between a fleet that says nothing for seventy
// seconds and one a caller can watch. The table still prints at the end; this is
// what happens before it.
//
// ⚠ Rows finish on several goroutines at once, so the writer is locked. Two
// rows finishing in the same instant would otherwise interleave inside one line.
func rowPrinter() func(toolkit.JobResult) {
	var mu sync.Mutex
	done := 0
	return func(row toolkit.JobResult) {
		mu.Lock()
		defer mu.Unlock()
		done++
		mark := "ok"
		switch {
		case row.Unreached:
			mark = "unreached"
		case row.Failed():
			mark = "FAILED"
		}
		logf("  %-9s %-14s exit %-3d %s", mark, row.Label, row.Exit, row.Duration.Round(time.Millisecond))
	}
}

func ensureBase(ctx context.Context, runner *toolkit.Runner, allowed bool) error {
	st, err := runner.Base().Status(ctx, false)
	if err != nil {
		return err
	}
	if st.Registered {
		return nil
	}
	if !allowed {
		return fmt.Errorf("%s is not registered and --ensure-base is off. Build it with: wsl-toolkit base ensure", st.Name)
	}
	logf("==> %s is not registered; building it first", st.Name)
	if _, err := runner.EnsureBase(ctx, false); err != nil {
		return err
	}
	return nil
}

func resolveImage(cfg toolkit.Config, name string) (string, error) {
	for _, img := range cfg.Catalog() {
		if img.ID == name {
			return img.Ref, nil
		}
	}
	if err := toolkit.ValidateImageRef(name); err != nil {
		return "", fmt.Errorf("%q is not a catalog id and not a usable reference: %w", name, err)
	}
	return name, nil
}
