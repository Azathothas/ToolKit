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
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// jobFlags are what run and matrix share, so the two cannot drift into
// accepting different spellings of the same thing.
type jobFlags struct {
	command       string
	commandB64    string
	scriptFile    string
	workspace     string
	artifactDir   string
	excludes      stringList
	gitignore     bool
	gitignoreSet  bool
	exclusive     string
	exclusiveWait time.Duration
	env           stringList
	envFile       string
	devices       stringList
	inputs        stringList
	timeout       time.Duration
	noNetwork     bool
	user          string
	platform      string
	lifecycle     string
	maxBytes      int64
	maxEntries    int
	maxOutput     int64
	asJSON        bool
	ensure        bool
	viaHelper     bool
	tick          time.Duration

	// WSL-122, WSL-128, WSL-123, WSL-130: named caches, executable inputs, an
	// account the image need not have, and capabilities.
	caches     stringList
	inputExecs stringList
	createUser bool
	sudo       bool
	capAdd     stringList
	// named is every flag the command line gave, so a project's default yields
	// to a flag even where its value is the flag's own default. WSL-131.
	named map[string]bool
	// memory and cpus are --memory and --cpus as typed, and resources what they
	// read as once check has run. WSL-136.
	memory    string
	cpus      string
	resources toolkit.ResourceLimits
}

// boolFlag is a boolean flag that also records that the command line named it,
// so a project's default yields to what a caller said, false included. WSL-112.
type boolFlag struct{ value, set *bool }

func (b boolFlag) String() string {
	if b.value == nil {
		return "false"
	}
	return strconv.FormatBool(*b.value)
}

func (b boolFlag) Set(s string) error {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	*b.value, *b.set = v, true
	return nil
}

// IsBoolFlag lets --gitignore stand alone, as a flag.BoolVar does.
func (b boolFlag) IsBoolFlag() bool { return true }

// nonEmpty is a string flag that refuses an empty value, for a flag whose empty
// value would widen what it exists to narrow.
//
// ⛔ AN EMPTY VALUE READ AS THE FLAG NOT GIVEN. `gc --workspace "$PROJECT"
// --apply` with PROJECT unset cleaned up every project's jobs, and `run
// --exclusive "$NAME"` with NAME unset held nothing. A safety review found the
// first.
type nonEmpty struct{ value *string }

func (n nonEmpty) String() string {
	if n.value == nil {
		return ""
	}
	return *n.value
}

func (n nonEmpty) Set(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("an empty value is refused: it would read as the flag not given, and widen what the flag narrows")
	}
	*n.value = s
	return nil
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func (j *jobFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&j.command, "c", "", "the shell command to run in the container")
	fs.StringVar(&j.commandB64, "command-base64", "", "the command as base64 of its bytes, for a caller that must keep every shell away from it")
	fs.StringVar(&j.scriptFile, "script", "", "a file on this machine whose bytes are the command, or - to read them from stdin")
	fs.StringVar(&j.workspace, "workspace", "", "a directory on this machine, COPIED into the container as /work")
	fs.StringVar(&j.artifactDir, "artifacts", "", "a directory on this machine to receive whatever the container leaves in /out")
	fs.Var(&j.excludes, "exclude", "a glob to leave out of the workspace copy, matched against the whole relative path, against the base name at any depth, and as a directory and everything under it. * does not cross a slash. Repeatable, and added to jobs.exclude")
	fs.Var(nonEmpty{&j.exclusive}, "exclusive", "hold this name for the whole job, across every instance of this state directory, and refuse to start while another job holds it. A project names its builds so two cannot run at once")
	fs.DurationVar(&j.exclusiveWait, "exclusive-wait", 0, "with --exclusive, wait this long for the name rather than refusing at once")
	fs.Var(boolFlag{&j.gitignore, &j.gitignoreSet}, "gitignore", "leave out of the workspace copy what git ignores in it, read from git itself. The workspace must be in a git work tree, and one its own repository ignores is refused. jobs.gitignore sets it for a project, and --gitignore=false turns it off")
	fs.Var(&j.env, "env", "NAME=VALUE passed to the container. Repeatable, and it wins over jobs.env")
	fs.StringVar(&j.envFile, "env-file", "", "a file on this machine of NAME=VALUE lines passed to the container. --env is applied after it, so a --env wins. jobs.env_file and jobs.env come before both")
	fs.Var(&j.devices, "device", "a device node in the base passed into the container, HOST[:CONTAINER[:PERMS]]. Repeatable. The base account must be able to read and write it")
	fs.Var(&j.inputs, "input", "NAME=FILE: a file on this machine the container reads at /in/NAME, read-only and byte for byte. Repeatable")
	fs.DurationVar(&j.timeout, "timeout", 30*time.Minute, "how long one container may run before lifecycle cleanup; the row reports 124. jobs.timeout sets the default")
	fs.DurationVar(&j.tick, "tick", 0, "emit a heartbeat for each running job at this interval. 0 is off, and anything under 1s is raised to it")
	fs.BoolVar(&j.noNetwork, "no-network", false, "run with no network at all")
	fs.StringVar(&j.memory, "memory", "", "a memory limit for the container, such as 512m or 2g, in podman's meaning. jobs.memory sets one for a project. The answer's limits block says whether the base enforced it")
	fs.StringVar(&j.cpus, "cpus", "", "a processor limit for the container, such as 1.5. jobs.cpus sets one for a project. The answer's limits block says whether the base enforced it")
	fs.StringVar(&j.user, "user", "", "what the container runs as: a name, a uid, or uid:gid. Empty uses jobs.user, then the image's default")
	fs.StringVar(&j.platform, "platform", "", "the Linux OCI platform. Empty uses jobs.platform, then the native platform")
	fs.StringVar(&j.lifecycle, "container-lifecycle", "", "persistent keeps the stopped container and its job directory; ephemeral removes both. Empty uses jobs.container_lifecycle, whose default is persistent")
	fs.Int64Var(&j.maxBytes, "max-bytes", toolkit.DefaultWorkspaceLimits().MaxBytes, "refuse a workspace or an artifact set larger than this. jobs.max_bytes sets the default")
	fs.IntVar(&j.maxEntries, "max-entries", toolkit.DefaultWorkspaceLimits().MaxEntries, "refuse a workspace or an artifact set with more entries than this. jobs.max_entries sets the default")
	fs.Int64Var(&j.maxOutput, "max-output", 0, "how many bytes of the command's output the ANSWER keeps. 0 uses the default. The transcript is complete whatever this says")
	fs.BoolVar(&j.asJSON, "json", false, "write a structured answer")
	fs.BoolVar(&j.ensure, "ensure-base", true, "build the base first when it is missing or unusable")
	fs.BoolVar(&j.viaHelper, "via-helper", false, "go through the local helper even when this process could call wsl.exe itself")
	// WSL-122, WSL-128, WSL-123, WSL-130.
	fs.Var(&j.caches, "cache", "NAME:/path: a named cache, the podman volume wtk-cache-NAME in the base, made on first use, mounted at /path and kept between jobs. Repeatable, and added to jobs.cache. gc --caches --apply removes it")
	fs.Var(&j.inputExecs, "input-exec", "NAME=FILE: as --input, but the copy has CRLF turned into LF and mode 0755, so the job runs /in/NAME by its shebang. FILE must start with #!. Repeatable")
	fs.BoolVar(&j.createUser, "create-user", false, "with --user NAME, run as NAME where the image has no such account: podman writes it, with a home of its own under /home. An image that has NAME uses its own")
	fs.BoolVar(&j.sudo, "sudo", false, "with --user, mount a read-only rule in /etc/sudoers.d that grants that account passwordless sudo. It needs sudo in the image")
	fs.Var(&j.capAdd, "cap-add", "a Linux capability the container gets, such as NET_ADMIN, or ALL. Repeatable, and none is added without it")
}

// readNamed records which flags the command line gave. WSL-131.
func (j *jobFlags) readNamed(fs *flag.FlagSet) {
	j.named = map[string]bool{}
	fs.Visit(func(f *flag.Flag) { j.named[f.Name] = true })
}

// script reads the job payload, from -c or from a file.
//
// ⚠ THE FILE'S BYTES GET THE SAME REPAIR EVERY CALLER'S COMMAND GETS: CRLF
// becomes LF in the COPY being sent, and a byte order mark is left out. The file
// on disk is never written to. A script written on Windows and run
// by /bin/sh otherwise fails on its first line with a message about a character
// nobody can see.
func (j *jobFlags) script() ([]byte, error) {
	return guestScript(j.command, j.commandB64, j.scriptFile)
}

// guestScript reads the one command a caller named, through -c, --command-base64
// or --script, and repairs the copy that is sent.
//
// ⛔ ONE READER FOR run, matrix AND base exec. `distro` had --command-base64 and
// these three did not, so a caller whose shell reaches into a command had a
// channel on one command and none on its siblings. WSL-99.
func guestScript(command, commandB64, scriptFile string) ([]byte, error) {
	given := 0
	for _, v := range []string{command, commandB64, scriptFile} {
		if v != "" {
			given++
		}
	}
	if given > 1 {
		return nil, errors.New("-c, --command-base64 and --script are three spellings of one command, so exactly one may be passed rather than one chosen by a precedence nobody would remember")
	}
	switch {
	case command != "":
		if err := gitBashRewrite("-c", command, guestPath); err != nil {
			return nil, err
		}
		// ⛔ -c GETS THE SAME REPAIR AS --script. A multi-line command assembled
		// in PowerShell carries CRLF, and /bin/sh reads the carriage return as
		// part of the last word on the line: `2>/dev/null` becomes a file named
		// `/dev/null` followed by an invisible byte.
		return toolkit.RepairGuestScript([]byte(command + "\n"))
	case commandB64 != "":
		decoded, err := base64.StdEncoding.Strict().DecodeString(commandB64)
		if err != nil {
			return nil, fmt.Errorf("--command-base64 is not valid base64: %w", err)
		}
		return toolkit.RepairGuestScript(decoded)
	case scriptFile == stdinScript:
		raw, err := readStdinScript()
		if err != nil {
			return nil, err
		}
		return toolkit.RepairGuestScript(raw)
	case scriptFile != "":
		raw, err := os.ReadFile(msysHostPath(scriptFile))
		if err != nil {
			return nil, err
		}
		return toolkit.RepairGuestScript(raw)
	}
	return nil, errors.New("nothing to run: pass -c COMMAND, --command-base64 BASE64 or --script FILE")
}

// stdinScript is the --script value that reads the command from this process's
// own stdin. WSL-111.
const stdinScript = "-"

// maxStdinScript bounds a command read from stdin, as a file is bounded by its
// size on disk.
const maxStdinScript = 64 << 20

// readStdinScript reads the command a caller piped in.
//
// ⭐ IT IS WHAT REMOVES A SCRATCH FILE PER SCRIPT. A Git Bash caller wrote each
// generated script to a file under its own /tmp, which is not a path a Windows
// program can open, and translated the path to hand it over. A pipe has no path.
// ⛔ AN EMPTY PIPE IS A REFUSAL, not an empty command that exits 0.
func readStdinScript() ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, maxStdinScript+1))
	if err != nil {
		return nil, fmt.Errorf("--script -: reading stdin: %w", err)
	}
	if len(raw) > maxStdinScript {
		return nil, fmt.Errorf("--script -: stdin carried more than %s, which is more than a command", toolkit.HumanBytes(maxStdinScript))
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("--script - read nothing from stdin, so there is no command to run. Pipe the script in, or pass -c")
	}
	return raw, nil
}

func (j *jobFlags) envMap(cfg toolkit.Config) (map[string]string, error) {
	out := map[string]string{}
	// ⭐ THE PROJECT'S DEFAULTS COME FIRST, its file and then its names, so every
	// value the command line gives wins over them. WSL-131.
	if cfg.Jobs.EnvFile != "" {
		path, err := pathFromJobs(cfg, "env_file", cfg.Jobs.EnvFile)
		if err != nil {
			return nil, fmt.Errorf("jobs.env_file: %w", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("jobs.env_file: %w", err)
		}
		pairs, err := toolkit.ParseEnvFile(data, path)
		if err != nil {
			return nil, fmt.Errorf("jobs.env_file: %w", err)
		}
		for _, pair := range pairs {
			out[pair.Name] = pair.Value
		}
	}
	for name, value := range cfg.Jobs.Env {
		out[name] = value
	}
	// ⭐ A FILE CARRIES VALUES NO SHELL REACHES. MSYS rewrites the value of an
	// argument that looks like a path, and a file's bytes travel untouched. The
	// reader is the one `distro --env-file` uses, so both read one format. WSL-111.
	if j.envFile != "" {
		path, err := pathFromProject(cfg, j.envFile)
		if err != nil {
			return nil, fmt.Errorf("--env-file: %w", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("--env-file: %w", err)
		}
		pairs, err := toolkit.ParseEnvFile(data, path)
		if err != nil {
			return nil, fmt.Errorf("--env-file %w", err)
		}
		for _, pair := range pairs {
			out[pair.Name] = pair.Value
		}
	}
	for _, pair := range j.env {
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("--env %q is not NAME=VALUE", pair)
		}
		if err := gitBashRewrite("--env", pair, guestText); err != nil {
			return nil, err
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

// jobDevices reads every --device. WSL-96.
func (j *jobFlags) jobDevices() ([]toolkit.JobDevice, error) {
	var out []toolkit.JobDevice
	for _, v := range j.devices {
		if err := gitBashRewrite("--device", v, guestPath); err != nil {
			return nil, err
		}
		d, err := toolkit.ParseDevice(v)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// jobInputs reads every --input NAME=FILE, resolves each file the way every
// other host path of a job is resolved, and refuses a set that cannot arrive as
// named. WSL-97.
func (j *jobFlags) jobInputs(cfg toolkit.Config) ([]toolkit.JobInput, error) {
	var out []toolkit.JobInput
	for _, v := range j.inputs {
		name, file, ok := strings.Cut(v, "=")
		if !ok || file == "" {
			return nil, fmt.Errorf("--input %q is not NAME=FILE: a name under /in, one equals sign, then a file on this machine", v)
		}
		if err := toolkit.ValidateInputName(name); err != nil {
			return nil, err
		}
		p, err := pathFromProject(cfg, file)
		if err != nil {
			return nil, fmt.Errorf("--input %s: %w", v, err)
		}
		out = append(out, toolkit.JobInput{Name: name, Path: p})
	}
	// ⭐ AN EXECUTABLE INPUT IS READ THE SAME WAY, and its copy is repaired and
	// made 0755 where it is written. WSL-128.
	for _, v := range j.inputExecs {
		name, file, ok := strings.Cut(v, "=")
		if !ok || file == "" {
			return nil, fmt.Errorf("--input-exec %q is not NAME=FILE: a name under /in, one equals sign, then a file on this machine", v)
		}
		if err := toolkit.ValidateInputName(name); err != nil {
			return nil, err
		}
		p, err := pathFromProject(cfg, file)
		if err != nil {
			return nil, fmt.Errorf("--input-exec %s: %w", v, err)
		}
		out = append(out, toolkit.JobInput{Name: name, Path: p, Exec: true})
	}
	if err := toolkit.CheckInputs(out, j.limits()); err != nil {
		return nil, err
	}
	return out, nil
}

// jobCaches reads every cache the job mounts: jobs.cache and each --cache, which
// applyConfig has merged. WSL-122.
func (j *jobFlags) jobCaches() ([]toolkit.JobCache, error) {
	return toolkit.ParseCaches(j.caches, "--cache")
}

// mergeCaches is a project's caches, less any at a path the command line names,
// then the command line's. WSL-122, WSL-131.
func mergeCaches(defaults, given []string) stringList {
	taken := map[string]bool{}
	for _, g := range given {
		if _, p, ok := strings.Cut(g, ":"); ok {
			taken[p] = true
		}
	}
	var out stringList
	for _, d := range defaults {
		if _, p, ok := strings.Cut(d, ":"); ok && taken[p] {
			continue
		}
		out = append(out, d)
	}
	return append(out, given...)
}

// jobCapabilities reads every --cap-add into podman's spelling. WSL-130.
func (j *jobFlags) jobCapabilities() ([]string, error) {
	var out []string
	for _, v := range j.capAdd {
		c, err := toolkit.ParseCapability(v)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
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
	resources, err := toolkit.ParseResourceLimits(j.memory, j.cpus)
	if err != nil {
		return err
	}
	j.resources = resources
	if j.exclusive != "" {
		if err := toolkit.ValidateExclusiveName(j.exclusive); err != nil {
			return err
		}
	}
	if j.exclusiveWait < 0 {
		return fmt.Errorf("--exclusive-wait %s is negative. Pass 0 to refuse at once, or a positive duration", j.exclusiveWait)
	}
	if j.exclusiveWait > 0 && j.exclusive == "" {
		return errors.New("--exclusive-wait waits for an --exclusive name, and none was given")
	}
	// ⛔ THE HELPER RUNS THE JOB, so the lock would be held by a process that is
	// not the job's owner. Refused, as --detach is. WSL-113.
	if j.viaHelper {
		if err := j.helperRouteRefusal(); err != nil {
			return err
		}
	}
	if j.lifecycle != "" && j.lifecycle != toolkit.ContainerPersistent && j.lifecycle != toolkit.ContainerEphemeral {
		return fmt.Errorf("--container-lifecycle %q must be %q or %q", j.lifecycle, toolkit.ContainerPersistent, toolkit.ContainerEphemeral)
	}
	// WSL-123: --create-user and --sudo name an account through --user.
	if err := toolkit.ValidateJobUser(j.user, j.createUser, j.sudo); err != nil {
		return err
	}
	return nil
}

// helperRouteRefusal answers a flag the helper route cannot honour.
//
// ⛔ THE HELPER OWNS A JOB IT RUNS, so an --exclusive lock would be held by no
// process at all. check() refuses an explicit --via-helper at once. The route
// useHelper chose, because this process cannot reach wsl.exe, ran the job with no
// lock and no word, until both helper entry points asked this first. WSL-113.
func (j jobFlags) helperRouteRefusal() error {
	// ⛔ A LIMIT IS READ BACK INSIDE THE CONTAINER BY THE PROCESS THAT RUNS IT,
	// and the helper protocol carries neither the limit nor the answer, so a job
	// through it would run unlimited and say nothing. WSL-136.
	if j.memory != "" || j.cpus != "" {
		return errors.New("--memory and --cpus, or jobs.memory and jobs.cpus, do not cross the helper: the job would run with no limit. Run it where wsl.exe can be called, without --via-helper")
	}
	switch {
	case j.exclusive == "":
		return nil
	case j.viaHelper:
		return errors.New("--exclusive is held by the process that owns the job, and a job through the helper is owned by the helper. Run it without --via-helper")
	}
	return errors.New("--exclusive is held by the process that owns the job, and this process cannot reach wsl.exe, so the job would go through the helper, which owns it. Run it where wsl.exe can be called, or without --exclusive")
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
	// ⭐ A PROJECT'S LIMITS APPLY WHERE THE COMMAND LINE NAMES NONE. WSL-136.
	if j.memory == "" {
		j.memory = cfg.Jobs.Memory
	}
	if j.cpus == "" {
		j.cpus = toolkit.FormatCPUs(cfg.Jobs.CPUs)
	}
	if j.workspace == "" && cfg.Jobs.Workspace != "" {
		j.workspace = cfg.Jobs.Workspace
		workspaceFromConfig = true
	}
	// ⭐ A PROJECT STATES WHAT ITS COPY LEAVES OUT ONCE, in its own configuration,
	// and a command line adds to it. A list in a shell string was how a consumer
	// met both of its globbing traps. WSL-112.
	j.excludes = append(append(stringList(nil), cfg.Jobs.Exclude...), j.excludes...)
	// ⛔ THE COMMAND LINE WINS, false included. The project's true replaced an
	// explicit --gitignore=false, which parsed and was then ignored. A correctness
	// review found it.
	if cfg.Jobs.Gitignore && !j.gitignoreSet {
		j.gitignore = true
	}
	// ⭐ A PROJECT'S JOB DEFAULTS, where the command line named nothing. A flag
	// the command line gives wins even where its value is the flag's own
	// default, so `--timeout 30m` beats a jobs.timeout of an hour. WSL-131.
	if !j.named["timeout"] {
		if d, ok := cfg.Jobs.JobTimeout(); ok {
			j.timeout = d
		}
	}
	if !j.named["user"] && cfg.Jobs.User != "" {
		j.user = cfg.Jobs.User
	}
	if !j.named["max-bytes"] && cfg.Jobs.MaxBytes > 0 {
		j.maxBytes = cfg.Jobs.MaxBytes
	}
	if !j.named["max-entries"] && cfg.Jobs.MaxEntries > 0 {
		j.maxEntries = cfg.Jobs.MaxEntries
	}
	j.caches = mergeCaches(cfg.Jobs.Cache, j.caches)
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
			// ⛔ AGAINST THE FILE THAT NAMED IT. A working tree's file lends a named
			// instance its workspace, and a relative one resolved against the
			// instance's own directory instead. A review found it. WSL-131.
			j.workspace, err = pathFromJobs(cfg, "workspace", j.workspace)
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
	if j.scriptFile != "" && j.scriptFile != stdinScript {
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
	value = msysHostPath(value)
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Abs(filepath.Join(filepath.Dir(cfg.Path()), value))
}

// pathFromJobs resolves a path a configuration's jobs.KEY names against the
// file that named it, which is a working tree's own where that file lent its
// jobs to a named instance. WSL-131.
func pathFromJobs(cfg toolkit.Config, key, value string) (string, error) {
	value = msysHostPath(value)
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Abs(filepath.Join(cfg.JobsDir(key), value))
}

// pathFromProject is the one resolver for a host path a job command was given.
// ⭐ An MSYS spelling from Git Bash is read here, once, for every flag. WSL-111.
func pathFromProject(cfg toolkit.Config, value string) (string, error) {
	value = msysHostPath(value)
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
	// ⛔ A DETACHED OWNER HAS NO READER. Its stdout and stderr are its own log,
	// and the job's bytes are in the transcript a follower reads. WSL-94.
	if os.Getenv(toolkit.DetachedEnv) != "" {
		return nil, nil
	}
	if j.asJSON {
		return nil, os.Stderr
	}
	return os.Stdout, os.Stderr
}

func cmdRun(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("run")
	var j jobFlags
	j.bind(fs)
	// ⭐ THE OBSERVATION LAYER IS BOUND HERE AND NOT IN jobFlags, and that is
	// deliberate. `matrix` shares jobFlags, and a fleet renders ROWS: twelve
	// relays interleaving their timestamped lines into one terminal is not a
	// reading of anything, and a flag a command accepts and does not read is the
	// dead-config row in docs/conventions/forbidden-patterns.md. A fleet's
	// per-row record is `--transcripts`; WSL-59 says so where a reader will look.
	var l logFlags
	l.bindSinks(fs)
	image := fs.String("image", "", "a catalog id or a fully qualified reference. Empty uses jobs.image")
	detach := fs.Bool("detach", false, "start the job and answer with its id at once. A detached copy of this program owns the job; wsl-toolkit logs ID --follow, wait ID and stop ID reach it")
	// WSL-121 and WSL-136: what the owning process does beyond running the job.
	var o ownerFlags
	o.bind(fs)
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	j.readNamed(fs)
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if *image, err = runImage(*image, cfg); err != nil {
		return exitCannot, err
	}
	if err := j.applyConfig(cfg); err != nil {
		return exitCannot, err
	}
	if err := j.check(); err != nil {
		return exitCannot, err
	}
	if err := o.settle(cfg, j.lifecycle); err != nil {
		return exitCannot, err
	}
	if j.viaHelper {
		if err := o.helperRefusal(true); err != nil {
			return exitCannot, err
		}
	}
	payload, err := j.script()
	if err != nil {
		return exitCannot, err
	}
	env, err := j.envMap(cfg)
	if err != nil {
		return exitCannot, err
	}
	devices, err := j.jobDevices()
	if err != nil {
		return exitCannot, err
	}
	inputs, err := j.jobInputs(cfg)
	if err != nil {
		return exitCannot, err
	}
	caches, err := j.jobCaches()
	if err != nil {
		return exitCannot, err
	}
	caps, err := j.jobCapabilities()
	if err != nil {
		return exitCannot, err
	}
	ref, err := resolveImage(cfg, *image)
	if err != nil {
		return exitCannot, err
	}
	// ⭐ ONE --tick, ROUTED. jobFlags owns the flag on this command, and the
	// relay reads the same value, so a caller who asked to hear about a quiet
	// job hears about it once from whichever reporter is active.
	l.tick = j.tick
	s, err := l.settings(cfg)
	if err != nil {
		return exitCannot, err
	}
	// ⭐ EVERY REFUSAL ABOVE HAS RUN BEFORE A DETACHED OWNER IS STARTED, so a
	// caller learns about a bad flag here and not from a log it has to go and
	// read. WSL-94.
	if *detach && os.Getenv(toolkit.DetachedEnv) == "" {
		if j.viaHelper {
			return exitCannot, errors.New("--detach starts a copy of this program that owns the job and reaches wsl.exe itself, and a job through the helper ends with its client. " +
				"Start it through the session's WSL approval path, or run it attached with --via-helper")
		}
		// ⛔ A COMMAND READ FROM STDIN IS HANDED OVER AS A FILE. The owner is a
		// copy of this program run with the same arguments and no stdin, so a second
		// `--script -` would read nothing there. WSL-111.
		// ⭐ REFUSED HERE, AT ONCE, while the caller is still attached to hear it.
		// The owner holds the lock; this is a probe so a refusal is not a log line
		// somebody has to go and read. WSL-113.
		if j.exclusive != "" && j.exclusiveWait == 0 {
			if err := toolkit.ExclusiveRefusal(j.exclusive); err != nil {
				return exitCannot, err
			}
		}
		// WSL-136: refused here too, at once, before an owner exists.
		if o.maxRunning > 0 && o.maxRunningWait == 0 {
			if err := toolkit.RunningCapRefusal(o.maxRunning); err != nil {
				return exitCannot, err
			}
		}
		// WSL-121: the owner watches this client's callers, which it cannot see.
		var handOver []string
		var callers []toolkit.ProcessIdentity
		if o.cancelOnExit {
			chain, kv, err := callersEnv()
			if err != nil {
				return exitCannot, err
			}
			callers, handOver = append([]toolkit.ProcessIdentity{}, chain...), []string{kv}
		}
		var fromStdin []byte
		if j.scriptFile == stdinScript {
			fromStdin = payload
		}
		return detachRun(ctx, j.asJSON, fromStdin, handOver, callers)
	}
	if c, err := useHelper(ctx, j.viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		if err := o.helperRefusal(j.viaHelper); err != nil {
			return exitCannot, err
		}
		if j.ensure {
			if err := helperEnsureBase(ctx, c.BaseEnsure); err != nil {
				return exitCannot, err
			}
		}
		// ⛔ THE HELPER ROUTE CANNOT CARRY THE RELAY, AND IT REFUSES RATHER THAN
		// DROPPING IT. The relay watches a container through the base it is in,
		// and a helper runs the job on ITS machine, so the adapter here would be
		// asking this host about a container that is somewhere else. A flag
		// silently ignored is the dead-config row; a refusal names the reason.
		if s.Active() {
			return exitCannot, errors.New("the observation flags do not cross the helper: the job runs on the helper's machine and its container cannot be watched from here. " +
				"Run without --via-helper, or read the job afterwards with: " + toolkit.ToolCommand("inspect JOB"))
		}
		res, err := helperRunJob(ctx, c, j, ref, *image, payload, env, devices, inputs)
		if err != nil {
			// ⚠ A CALL CANCELLED BEFORE THE HELPER TOOK THE JOB ON answers 130,
			// as the direct route does. One cancelled after is the job's own
			// end, which RunStream answers. WSL-127.
			return cannotOrStopped(ctx), err
		}
		return reportJob(res, j.asJSON)
	}
	// ⭐ THE JOB IS CLAIMED BEFORE ANYTHING ELSE HAPPENS, so a second process can
	// name it, follow it and stop it from its first second, and a refusal after
	// this point is recorded as the job's end rather than lost. WSL-94.
	owner, err := claimRun()
	if err != nil {
		return exitCannot, err
	}
	refuse := func(err error) (int, error) {
		res := toolkit.JobResult{ID: owner.ID, Label: *image, Image: ref, Exit: exitCannot, Unreached: true, Error: err.Error()}
		return o.refuseClaimed(owner, res, j.asJSON, err)
	}
	// ⭐ THE PROJECT IS RECORDED WITH THE CLAIM, so `gc --workspace` finds even a
	// job refused before it copied anything. WSL-114.
	if j.workspace != "" {
		key, err := toolkit.WorkspaceKey(j.workspace)
		if err == nil {
			err = owner.RecordWorkspace(key)
		}
		if err != nil {
			logf("  ! could not record the workspace of job %s, so gc --workspace will not find it: %s", owner.ID, err.Error())
		}
	}
	followCmd, _, stopCmd := jobHints(owner.ID)
	logf("  job %s. Follow it with: %s. Stop it with: %s", owner.ID, followCmd, stopCmd)
	// ⭐ THE WATCHER STARTS WITH THE CLAIM, so a caller that exits while the job
	// waits for its name or its turn ends that wait too. WSL-121.
	if o.cancelOnExit {
		callers, err := callersOf()
		if err == nil {
			err = startWatchdog([]string{owner.ID}, callers)
		}
		if err != nil {
			return refuse(err)
		}
	}
	// ⭐ HELD BY THE PROCESS THAT OWNS THE JOB, attached or detached, for the
	// job's whole life, and let go by Windows however this process ends. WSL-113.
	if j.exclusive != "" {
		// ⛔ A STOP IS READ WHILE THE JOB WAITS FOR ITS NAME, and the job then ends
		// as a stopped job does, with 130, rather than waiting out --exclusive-wait.
		release, err := toolkit.AcquireExclusive(ctx, j.exclusive, owner.ID, j.exclusiveWait, note,
			func() bool { return toolkit.StopRecorded(owner.ID) })
		if errors.Is(err, toolkit.ErrStoppedWaiting) {
			res := toolkit.JobResult{ID: owner.ID, Label: *image, Image: ref, Exit: 130, Cancelled: true, Stopped: true, Error: err.Error()}
			res.Seal()
			o.finish(owner, &res)
			return reportJob(res, j.asJSON)
		}
		if err != nil {
			return refuse(err)
		}
		defer release()
	}
	// ⭐ HELD AFTER ITS NAME AND BEFORE ANYTHING IS SPENT ON IT, so a job waiting
	// for its turn copies nothing and holds no running slot. WSL-136.
	waited, ended, code, err := o.admit(ctx, owner, *image, ref, j.asJSON)
	if ended {
		return code, err
	}
	runner, err := toolkit.NewRunner(cfg, note)
	if err != nil {
		return refuse(err)
	}
	if err := ensureBase(ctx, runner, j.ensure); err != nil {
		return refuse(err)
	}
	if err := runner.Base().EnsurePlatform(ctx, j.platform); err != nil {
		return refuse(err)
	}
	liveOut, liveErr := j.sinks()
	// ⛔ THE RELAY IS OPENED ONLY WHERE THE CALLER ASKED FOR ONE. Opening one
	// unconditionally would put a rendering layer in front of every job's
	// output, and a caller reading a value off stdout would get a prefix.
	var relay *toolkit.RunLog
	if s.Active() {
		if relay, err = toolkit.OpenRunLog(s, liveOut, liveErr); err != nil {
			return refuse(err)
		}
		// ⚠ ABORTED, NOT CLOSED, WHERE THE JOB NEVER BEGAN. Abort removes a sink
		// file this call created, so a run refused before its container leaves no
		// empty log behind; Finish inside the job is what closes a real one.
		defer relay.Abort()
	}
	res := runner.Run(ctx, toolkit.JobSpec{
		ID: owner.ID, Owner: owner,
		Image: ref, Script: payload, Workspace: j.workspace, Excludes: toolkit.SortedExcludes(j.excludes), Gitignore: j.gitignore,
		ArtifactDir: j.artifactDir, Env: env, Timeout: j.timeout, Network: !j.noNetwork,
		Platform: j.platform, ContainerLifecycle: j.lifecycle,
		Limits: j.limits(), Label: *image, User: j.user, Devices: devices, Inputs: inputs,
		Stdout: liveOut, Stderr: liveErr, MaxOutput: j.maxOutput,
		OnTick: jobTick(j, relay), TickEvery: j.tick, Log: relay,
		Caches: caches, CreateUser: j.createUser, Sudo: j.sudo, CapAdd: caps,
		Resources: j.resources,
	})
	res.WaitedToRun = waited
	// ⛔ SEALED AND RECORDED BEFORE IT IS RENDERED, so a follower reading the
	// record and this process reporting it name one verdict.
	res.Seal()
	// ⭐ AN EPHEMERAL JOB'S RECORD GOES ONCE THE ANSWER IS READY, and the answer
	// says so. WSL-121.
	o.finish(owner, &res)
	return reportJob(res, j.asJSON)
}

// runImage is the image `run` runs: --image, or the configuration's jobs.image.
//
// ⭐ IT IS READ AFTER THE CONFIGURATION, so a project's jobs.image is the default
// a command line names nothing over. WSL-131.
func runImage(given string, cfg toolkit.Config) (string, error) {
	if given != "" {
		return given, nil
	}
	if cfg.Jobs.Image != "" {
		return cfg.Jobs.Image, nil
	}
	return "", errors.New("--image is required. " + toolkit.ToolCommand("images") + " lists the catalog, and jobs.image sets a default")
}

// claimRun takes the owner lock for the job this process is about to run, under
// the id a detaching caller chose or a fresh one.
func claimRun() (*toolkit.JobOwner, error) {
	home, err := toolkit.Home()
	if err != nil {
		return nil, err
	}
	id, err := toolkit.PreassignedJobID()
	if err != nil {
		return nil, err
	}
	if id == "" {
		if id, err = toolkit.NewJobID(); err != nil {
			return nil, err
		}
	}
	return toolkit.ClaimJob(home, id)
}

// DetachedSchema versions `run --detach --json`.
const DetachedSchema = "wsl-toolkit-detached/1"

// detachRun starts a detached copy of this program that owns the job, waits
// until it holds the job's lock, and answers with the job's id.
//
// ⛔ THE COPY RUNS THE SAME COMMAND LINE THROUGH THE SAME CODE as an attached
// run, so artifacts, teardown and the verdict do not change. It learns its id
// and that it is detached from two variables, never from a flag a caller could
// pass. Its own progress goes to owner.log beside the transcript.
func detachRun(ctx context.Context, asJSON bool, fromStdin []byte, handOver []string, watched []toolkit.ProcessIdentity) (int, error) {
	home, err := toolkit.Home()
	if err != nil {
		return exitCannot, err
	}
	id, err := toolkit.NewJobID()
	if err != nil {
		return exitCannot, err
	}
	if err := os.MkdirAll(filepath.Join(home, "jobs", id), 0o700); err != nil {
		return exitCannot, err
	}
	logPath := toolkit.OwnerLogPath(home, id)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return exitCannot, err
	}
	exe, err := os.Executable()
	if err != nil {
		_ = logFile.Close()
		return exitCannot, err
	}
	args := os.Args[1:]
	if fromStdin != nil {
		script := filepath.Join(home, "jobs", id, "stdin-script.sh")
		if err := os.WriteFile(script, fromStdin, 0o600); err != nil {
			_ = logFile.Close()
			return exitCannot, fmt.Errorf("could not hand the command read from stdin to the detached owner: %w", err)
		}
		if args, err = replaceStdinScript(args, script); err != nil {
			_ = logFile.Close()
			return exitCannot, err
		}
	}
	cmd, leftJobObject, err := toolkit.StartDetached(func() *exec.Cmd {
		c := exec.Command(exe, args...)
		c.Env = append(append(os.Environ(), toolkit.JobIDEnv+"="+id, toolkit.DetachedEnv+"=1"), handOver...)
		c.Stdout, c.Stderr = logFile, logFile
		return c
	})
	_ = logFile.Close()
	if err != nil {
		return exitCannot, fmt.Errorf("could not start the detached owner: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.Now().Add(detachClaimLimit)
	for {
		st, err := toolkit.ReadJobState(home, id)
		if err != nil {
			return exitCannot, err
		}
		if st.Owned || st.Result != nil {
			break
		}
		select {
		case <-exited:
			if st, _ := toolkit.ReadJobState(home, id); st.Result != nil {
				return reportDetached(home, id, leftJobObject, asJSON, watched...)
			}
			return exitCannot, fmt.Errorf("the detached owner ended before it took the job. Its log: %s", logPath)
		case <-ctx.Done():
			// ⚠ THE OWNER IS ALREADY STARTED, and the interrupt does not reach it.
			// The id is named so the caller can still find the job.
			return 130, fmt.Errorf("interrupted while the detached owner took job %s. The job can still run: %s", id, toolkit.ToolCommand("logs "+id))
		case <-time.After(100 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return exitCannot, fmt.Errorf("the detached owner did not take the job within %s. Its log: %s", detachClaimLimit, logPath)
		}
	}
	return reportDetached(home, id, leftJobObject, asJSON, watched...)
}

// detachClaimLimit bounds the wait for a detached owner to take its job. The
// claim is the first thing the owner does, in milliseconds.
const detachClaimLimit = 60 * time.Second

// jobObjectWarning is what a caller learns when the detached owner could not
// leave the Windows job object this process runs in.
const jobObjectWarning = "the job object this process runs in refused to release the detached owner, so the owner ends when that object closes. " +
	"base exec --detach runs in the base and does not depend on it"

// reportDetached answers with the id of a job a detached owner took.
//
// ⭐ watched are the callers --cancel-on-exit ties the job to, and the answer
// names them, in `cancel_on_exit` under --json. WSL-121.
func reportDetached(home, id string, leftJobObject, asJSON bool, watched ...toolkit.ProcessIdentity) (int, error) {
	followCmd, waitCmd, stopCmd := jobHints(id)
	if asJSON {
		doc := map[string]any{
			"schema": DetachedSchema, "id": id, "kind": "job",
			"owner_log": toolkit.OwnerLogPath(home, id),
			"follow":    followCmd,
			"wait":      waitCmd,
			"stop":      stopCmd,
		}
		if !leftJobObject {
			doc["warning"] = jobObjectWarning
		}
		if watched != nil {
			doc["cancel_on_exit"] = watched
		}
		return exitOK, writeJSON(doc)
	}
	fmt.Println(id)
	logf("  job %s runs detached. Follow: %s. Wait: %s. Stop: %s", id, followCmd, waitCmd, stopCmd)
	if !leftJobObject {
		logf("  ! %s", jobObjectWarning)
	}
	if watched != nil {
		logf("  %s", cancelLine(id, watched))
	}
	return exitOK, nil
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
		logf("  container %s is kept with its job directory. Remove it with: %s", res.Container, toolkit.ToolCommand("gc --job "+res.ID+" --apply"))
	}
	if line := limitsLine(res.Limits); line != "" {
		logf("  %s", line)
	}
	logf("  %s %s in %s", res.Label, endedAs(res, res.Exit), res.Duration.Round(time.Millisecond))
	if hint := transcriptHint(res); hint != "" {
		logf("  %s", hint)
	}
	if line := keptLine(res); line != "" {
		logf("  %s", line)
	}
	if line := waitedLine(res); line != "" {
		logf("  %s", line)
	}
	return jobVerdict(res), nil
}

// endedAs says how a job ended, in words a reader cannot take for its payload's
// own code. code is the number a job that ran is said to have exited with.
// WSL-127.
//
// ⛔ "exited 2" WAS PRINTED for a job that never started and for one whose end
// was lost, beside a process that answered 125, and it read as the payload's 2.
func endedAs(res toolkit.JobResult, code int) string {
	switch {
	case res.Unreached:
		return "did not start"
	case res.ToolFailed:
		return "started, and this tool lost how it ended"
	}
	return fmt.Sprintf("exited %d", code)
}

// limitsLine says what a job's limits were and whether its container was held
// to them. WSL-136.
//
// ⛔ A LIMIT NOT ENFORCED IS SAID AS LOUDLY AS AN ERROR. A job that asked for
// 64 MiB and got none ran on a promise, and its caller reads the job as bounded.
func limitsLine(l *toolkit.LimitsReport) string {
	if l == nil {
		return ""
	}
	var asked []string
	if l.MemoryBytes > 0 {
		asked = append(asked, "memory "+toolkit.HumanBytes(l.MemoryBytes))
	}
	if l.CPUs > 0 {
		asked = append(asked, toolkit.FormatCPUs(l.CPUs)+" cpus")
	}
	what := strings.Join(asked, " and ")
	switch l.Enforced {
	case toolkit.LimitsEnforced:
		return "limits " + what + ": enforced, read in the container's own cgroup"
	case toolkit.LimitsUnenforced:
		return "! limits " + what + ": NOT enforced, the container's cgroup carries no such limit. " +
			toolkit.ToolCommand("base status --probe") + " names the base's cgroup mechanism"
	}
	return "limits " + what + ": whether they were enforced is unknown, because the container did not say"
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
	return "the complete output is kept: " + toolkit.ToolCommand("logs "+res.ID)
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
	// WSL-121: what the owning process does beyond running the rows.
	var o ownerFlags
	o.bind(fs)
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	j.readNamed(fs)
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
	if err := o.settle(cfg, j.lifecycle); err != nil {
		return exitCannot, err
	}
	if j.viaHelper {
		if err := o.helperRefusal(true); err != nil {
			return exitCannot, err
		}
	}
	payload, err := j.script()
	if err != nil {
		return exitCannot, err
	}
	env, err := j.envMap(cfg)
	if err != nil {
		return exitCannot, err
	}
	devices, err := j.jobDevices()
	if err != nil {
		return exitCannot, err
	}
	inputs, err := j.jobInputs(cfg)
	if err != nil {
		return exitCannot, err
	}
	caches, err := j.jobCaches()
	if err != nil {
		return exitCannot, err
	}
	caps, err := j.jobCapabilities()
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
		if err := o.helperRefusal(j.viaHelper); err != nil {
			return exitCannot, err
		}
		if j.ensure {
			if err := helperEnsureBase(ctx, c.BaseEnsure); err != nil {
				return exitCannot, err
			}
		}
		ids := make([]string, 0, len(selected))
		for _, img := range selected {
			ids = append(ids, img.ID)
		}
		report, err := helperRunMatrix(ctx, c, j, ids, *parallel, payload, env, devices, inputs, *transcripts)
		if err != nil {
			return exitCannot, err
		}
		return reportMatrix(report, j.asJSON)
	}
	// ⭐ A FLEET'S WATCHER KNOWS EVERY ROW BEFORE ANY RUNS, so a row its caller
	// left behind never starts its container. WSL-121.
	var rows []string
	if o.cancelOnExit {
		ids, err := rowIDs(len(selected))
		if err != nil {
			return exitCannot, err
		}
		callers, err := toolkit.Callers()
		if err != nil {
			return exitCannot, fmt.Errorf("--cancel-on-exit could not read which process started this one: %w", err)
		}
		if err := startWatchdog(ids, callers); err != nil {
			return exitCannot, err
		}
		rows = ids
	}
	// ⭐ A FLEET HOLDS THE NAME ONCE, for every row, because its rows run in this
	// one process. WSL-113.
	if j.exclusive != "" {
		release, err := toolkit.AcquireExclusive(ctx, j.exclusive, fmt.Sprintf("matrix (process %d)", os.Getpid()), j.exclusiveWait, note, nil)
		if err != nil {
			return exitCannot, err
		}
		defer release()
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
		Excludes: toolkit.SortedExcludes(j.excludes), Gitignore: j.gitignore, ArtifactDir: j.artifactDir,
		Env: env, Timeout: j.timeout, Network: !j.noNetwork, Parallel: *parallel,
		Platform: j.platform, ContainerLifecycle: j.lifecycle,
		Limits: j.limits(), Transcripts: *transcripts, User: j.user, Devices: devices, Inputs: inputs,
		MaxOutput: j.maxOutput, OnRow: rowPrinter(),
		OnTick: tickPrinter(j), TickEvery: j.tick,
		RowIDs: rows, Admit: o.rowAdmit(),
		Caches: caches, CreateUser: j.createUser, Sudo: j.sudo, CapAdd: caps,
		Resources: j.resources,
	})
	if err != nil {
		return exitCannot, err
	}
	// ⭐ EACH EPHEMERAL ROW'S RECORD GOES ONCE THE FLEET'S ANSWER IS READY.
	// WSL-121.
	o.leaveRows(&report)
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

// jobTick picks which heartbeat a job gets.
//
// ⛔ NEVER BOTH. The relay's fires on SILENCE and reads the container's own
// state; the fleet's fires on a TIMER and reports byte counts. Two heartbeats
// about one job, at the same interval, is a reader being told the same thing
// twice in two vocabularies - and the relay's is the one that can say what the
// container is doing, which is the whole of WSL-59.
func jobTick(j jobFlags, relay *toolkit.RunLog) func(toolkit.TickEvent) {
	if relay != nil {
		return nil
	}
	return tickPrinter(j)
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
		for _, line := range rowLines(row) {
			logf("%s", line)
		}
	}
}

// rowLines is what one finished matrix row prints.
//
// ⛔ A ROW'S LIMITS ARE SAID UNDER IT, as run says a job's. Only each JSON row
// carried them, so a matrix read by eye did not say that a limit went
// unenforced. WSL-136.
func rowLines(row toolkit.JobResult) []string {
	mark := "ok"
	switch {
	case row.Unreached:
		mark = "unreached"
	case row.Failed():
		mark = "FAILED"
	}
	lines := []string{fmt.Sprintf("  %-9s %-14s exit %-3d %s", mark, row.Label, row.Exit, row.Duration.Round(time.Millisecond))}
	if line := limitsLine(row.Limits); line != "" {
		lines = append(lines, "            "+line)
	}
	return lines
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
		return fmt.Errorf("%s is not registered and --ensure-base is off. Build it with: %s", st.Name, toolkit.ToolCommand("base ensure"))
	}
	logf("==> %s is not registered; building it first", st.Name)
	if _, err := runner.EnsureBase(ctx, false); err != nil {
		return err
	}
	return nil
}

// helperEnsureBase is the helper route's ensure, asked before `run` or `matrix`
// sends its job.
//
// ⛔ A STALE BOOT ID IS PASSED, NOT RETURNED. The helper's unflagged ensure
// probes with a container, meets the refusal a restart of WSL leaves, and
// answers with it. Returning it stopped the job before the helper's Runner.Run,
// which is where the state is cleared, so after `wsl --shutdown` this route
// still exited 2. Plain `base ensure` still clears only with --repair. WSL-137.
//
// ⚠ What the helper removed is named in the helper's own log, not here.
func helperEnsureBase(ctx context.Context, ensure func(context.Context, bool) (toolkit.BaseState, error)) error {
	_, err := ensure(ctx, false)
	if err == nil {
		return nil
	}
	if _, stale := toolkit.StaleRunStateRemediation(err.Error()); stale {
		note("the base's engine state names another boot of WSL, so the helper clears it before the job's container, and the helper's log names what it removed")
		return nil
	}
	return err
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

// replaceStdinScript names a file where a command line reads its script from
// stdin, in whichever spelling the caller used. WSL-111.
func replaceStdinScript(args []string, file string) ([]string, error) {
	out := make([]string, 0, len(args))
	found := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case (a == "--script" || a == "-script") && i+1 < len(args) && args[i+1] == stdinScript:
			out = append(out, a, file)
			i++
			found = true
		case a == "--script="+stdinScript || a == "-script="+stdinScript:
			out = append(out, strings.SplitN(a, "=", 2)[0]+"="+file)
			found = true
		default:
			out = append(out, a)
		}
	}
	if !found {
		return nil, errors.New("--script - was read and the command line that names it was not found, so the detached owner would have read an empty stdin")
	}
	return out, nil
}

// filter is what the workspace copy leaves out, as one value both routes send.
func (j *jobFlags) filter() toolkit.WorkspaceFilter {
	return toolkit.WorkspaceFilter{Excludes: toolkit.SortedExcludes(j.excludes), Gitignore: j.gitignore}
}
