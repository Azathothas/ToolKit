package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	maxBytes    int64
	maxEntries  int
	asJSON      bool
	ensure      bool
	viaHelper   bool
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
	fs.DurationVar(&j.timeout, "timeout", 30*time.Minute, "how long one container may run before it is killed and the row reports 124")
	fs.BoolVar(&j.noNetwork, "no-network", false, "run with no network at all")
	fs.StringVar(&j.user, "user", "", "what the container runs as: a name, a uid, or uid:gid. Empty means the image's default")
	fs.Int64Var(&j.maxBytes, "max-bytes", toolkit.DefaultWorkspaceLimits().MaxBytes, "refuse a workspace or an artifact set larger than this")
	fs.IntVar(&j.maxEntries, "max-entries", toolkit.DefaultWorkspaceLimits().MaxEntries, "refuse a workspace or an artifact set with more entries than this")
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
		return []byte(j.command + "\n"), nil
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
		out[name] = value
	}
	return out, nil
}

func (j *jobFlags) limits() toolkit.WorkspaceLimits {
	return toolkit.WorkspaceLimits{MaxBytes: j.maxBytes, MaxEntries: j.maxEntries}
}

func cmdRun(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("run")
	var j jobFlags
	j.bind(fs)
	image := fs.String("image", "", "a catalog id or a fully qualified reference")
	if err := fs.Parse(args); err != nil {
		return exitCannot, err
	}
	if *image == "" {
		return exitCannot, errors.New("--image is required. wsl-toolkit images lists the catalog")
	}
	payload, err := j.script()
	if err != nil {
		return exitCannot, err
	}
	env, err := j.envMap()
	if err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	ref, err := resolveImage(cfg, *image)
	if err != nil {
		return exitCannot, err
	}
	if j.workspace != "" {
		if j.workspace, err = filepath.Abs(j.workspace); err != nil {
			return exitCannot, err
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
	res := runner.Run(ctx, toolkit.JobSpec{
		Image: ref, Script: payload, Workspace: j.workspace, Excludes: toolkit.SortedExcludes(j.excludes),
		ArtifactDir: j.artifactDir, Env: env, Timeout: j.timeout, Network: !j.noNetwork,
		Limits: j.limits(), Label: *image, User: j.user,
	})
	return reportJob(res, j.asJSON)
}

// reportJob is the one renderer, so a job run through the helper and a job run
// directly are reported identically.
func reportJob(res toolkit.JobResult, asJSON bool) (int, error) {
	if asJSON {
		return jobVerdict(res), writeJSON(res)
	}
	// ⛔ THE CONTAINER'S OWN OUTPUT GOES TO STDOUT AND NOTHING ELSE DOES, so a
	// caller can read a value off it.
	os.Stdout.WriteString(res.Stdout)
	os.Stderr.WriteString(res.Stderr)
	if res.Error != "" {
		logf("  ! %s", res.Error)
	}
	logf("  %s exited %d in %s", res.Label, res.Exit, res.Duration.Round(time.Millisecond))
	return jobVerdict(res), nil
}

func jobVerdict(res toolkit.JobResult) int {
	switch {
	case res.Unreached:
		return exitCannot
	case res.TimedOut:
		return exitTimeout
	default:
		// ⭐ The container's own exit code is forwarded verbatim, which is the
		// whole point of running one. A wrapper that flattened it to 1 would
		// make every downstream test read the same.
		return res.Exit
	}
}

func cmdMatrix(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("matrix")
	var j jobFlags
	j.bind(fs)
	images := fs.String("images", "", "catalog ids, libc:musl, kind:legacy or all. Comma separated. Empty means the configured default")
	parallel := fs.Int("parallel", toolkit.DefaultParallel, "how many rows run at once")
	transcripts := fs.String("transcripts", "", "a directory to write one transcript per row into")
	if err := fs.Parse(args); err != nil {
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
	cfg, err := loadConfig()
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
	if j.workspace != "" {
		if j.workspace, err = filepath.Abs(j.workspace); err != nil {
			return exitCannot, err
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
	logf("  %d image(s), %d at a time, %s per row", len(selected), *parallel, j.timeout)
	report, err := runner.RunMatrix(ctx, toolkit.MatrixSpec{
		Images: selected, Script: payload, Workspace: j.workspace,
		Excludes: toolkit.SortedExcludes(j.excludes), ArtifactDir: j.artifactDir,
		Env: env, Timeout: j.timeout, Network: !j.noNetwork, Parallel: *parallel,
		Limits: j.limits(), Transcripts: *transcripts, User: j.user,
	})
	if err != nil {
		return exitCannot, err
	}
	return reportMatrix(report, j.asJSON)
}

// reportMatrix is the one renderer, for the same reason reportJob is.
func reportMatrix(report toolkit.MatrixReport, asJSON bool) (int, error) {
	if asJSON {
		return report.Verdict(), writeJSON(report)
	}
	if err := toolkit.RenderMatrix(os.Stderr, report); err != nil {
		return exitCannot, err
	}
	return report.Verdict(), nil
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
	if _, err := runner.Base().Ensure(ctx, false); err != nil {
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
