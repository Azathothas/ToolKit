// repo is this repository's tool box: one binary holding the tools that are not
// gate checks.
//
// ⛔ WHY IT IS NOT SHELL. Each of these used to be written twice, in sh and in
// PowerShell, because the default host here is Windows and a POSIX script
// cannot be assumed to run on it. Keeping two implementations of one rule in
// step needed a third check that ran both halves of every pair and compared
// their answers, and that check was most of a gate taking about twelve minutes.
// One implementation has no halves to compare.
//
// ⛔ WHY IT IS NOT tools/check. That binary holds the rules this repository
// enforces over its OWN tree, and `check-gate` runs all of them. These are a
// host probe, a commit path, a licence writer, two remote readers and a
// mutation harness; folding them in would make the gate do things that are not
// checks, and a gate whose scope drifts is one nobody can say the meaning of.
//
// ⚠ `mutate` IS THE CLOSEST CALL and it stays out for the same reason. It
// proves that the tests guarding this tree are real, which is a thing to run
// deliberately when guards change, not a rule to enforce over every commit: one
// pass copies every module and runs a test suite per row, and a gate somebody
// waits minutes for is a gate they skip.
//
// Exit codes are the contract every caller reads:
//
//	0  it ran and agreed
//	1  it ran and disagreed
//	2  it could not run
//
// ⚠ A skip is neither a pass nor a failure. A tool that quietly runs nothing
// and reports success is the worst answer this codebase can give.
//
// SPDX-License-Identifier: 0BSD
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Azathothas/ToolKit/tools/repo/internal/binfmt"
	"github.com/Azathothas/ToolKit/tools/repo/internal/deslop"
	"github.com/Azathothas/ToolKit/tools/repo/internal/gitrepo"
	"github.com/Azathothas/ToolKit/tools/repo/internal/gitsync"
	"github.com/Azathothas/ToolKit/tools/repo/internal/license"
	"github.com/Azathothas/ToolKit/tools/repo/internal/linuxproof"
	"github.com/Azathothas/ToolKit/tools/repo/internal/mutate"
	toolrelease "github.com/Azathothas/ToolKit/tools/repo/internal/release"
	"github.com/Azathothas/ToolKit/tools/repo/internal/remote"
	"github.com/Azathothas/ToolKit/tools/repo/internal/workflow"
)

type command struct {
	name    string
	summary string
	run     func(args []string) int
}

func commands() []command {
	return []command{
		{"binfmt", "are binfmt_misc handlers registered in the kernel containers run against", runBinfmt},
		{"deslop", "which files in this tree address a reader as an agent", runDeslop},
		{"git-sync", "commit and push, with the identity and attribution rules enforced", runGitSync},
		{"license", "write LICENSE from a template, with the holder filled in", runLicense},
		{"linux-proof", "run ShellCheck, the go job and the added rows on Linux over a commit's tree, and record a proof for the pre-push hook", runLinuxProof},
		{"mutate", "delete each guard in the table and confirm the test named for it goes red", runMutate},
		{"mutate-keep", "keep the proof of every module whose rows all passed in CI's shards", runMutateKeep},
		{"mutate-keys", "every module's proof key, from its tree, its rows, the harness, the Go version and the runner", runMutateKeys},
		{"mutate-plan", "which modules a kept proof answers, and how many shards the rest need", runMutatePlan},
		{"nightly-prune", "the herdr nightlies older than the newest seven, by published_at, and optionally delete them", runNightlyPrune},
		{"nightly-publish", "create a herdr nightly empty, upload each staged file, and read the set back", runNightlyPublish},
		{"nightly-stage", "stage a herdr nightly's four builds and BUILD-INFO.json, assert the set, write SHA256SUMS and the sign count", runNightlyStage},
		{"pre-push", "the pre-push hook: prove each pushed tree that has no Linux proof, and refuse the push when that fails", runPrePush},
		{"remote-items", "what is open against this repository, and does it survive checking", runRemote},
		{"release", "verify and optionally tag a native wsl-toolkit release", runRelease},
		{"release-readback", "every staged asset is on the published release, by name", runReleaseReadback},
		{"release-stage", "build and stage every release asset, assert the set, write SHA256SUMS and the sign count", runReleaseStage},
	}
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		usage(os.Stderr)
		return 0
	}
	name, rest := args[0], args[1:]
	for _, c := range commands() {
		if c.name == name {
			return c.run(rest)
		}
	}
	fmt.Fprintf(os.Stderr, "repo: %q is not a command\n\n", name)
	usage(os.Stderr)
	return 2
}

func usage(w *os.File) {
	cs := commands()
	sort.Slice(cs, func(i, j int) bool { return cs[i].name < cs[j].name })
	width := 0
	for _, c := range cs {
		if len(c.name) > width {
			width = len(c.name)
		}
	}
	fmt.Fprintln(w, "repo <command> [flags]")
	fmt.Fprintln(w)
	for _, c := range cs {
		fmt.Fprintf(w, "  %-*s  %s\n", width, c.name, c.summary)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Every command takes --json where its answer has structure.")
	fmt.Fprintln(w, "Exit codes: 0 it agreed, 1 it disagreed, 2 it could not run.")
}

// newFlagSet gives every subcommand the same parse, including the refusal of a
// stray word.
//
// ⛔ GO'S FLAG PACKAGE STOPS AT THE FIRST ARGUMENT THAT IS NOT A FLAG and leaves
// everything after it in Args(), so a caller who wrote a word in the middle got
// a run that ignored it AND every option after it, at exit 0.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("repo "+name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

func parseArgs(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	extra := fs.Args()
	if len(extra) == 0 {
		return nil
	}
	msg := fmt.Sprintf("%s takes flags, not positional arguments, and %q is one", fs.Name(), extra[0])
	if len(extra) > 1 {
		msg += fmt.Sprintf(". Everything after it was never parsed, starting with %q", extra[1])
	}
	return fmt.Errorf("%s", msg)
}

// exitFor turns a parse outcome into an exit code. ⭐ Asking for help is not a
// failure: the defaults have already been printed by the time flag returns it.
func exitFor(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	if strings.Contains(err.Error(), flag.ErrHelp.Error()) {
		return 0, true
	}
	fmt.Fprintf(os.Stderr, "repo: %s\n", err)
	return 2, true
}

func runDeslop(args []string) int {
	fs := newFlagSet("deslop")
	var opts deslop.Options
	fs.BoolVar(&opts.JSON, "json", false, "write a structured answer")
	fs.BoolVar(&opts.Apply, "apply", false, "actually remove the files listed. Refused on a dirty tree")
	dry := fs.Bool("dry-run", false, "the default. Accepted so the intent can be written out")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	if *dry {
		opts.Apply = false
	}
	return deslop.Run(opts, os.Stdout, os.Stderr)
}

// runMutate proves the guards rather than counting them.
//
// ⛔ THIS USED TO BE A SESSION-LOCAL SCRIPT under .tmp/, which is
// gitignored. Three records cited it as the command that closed them, and none
// of those commands could be run by anyone reading the record afterwards.
// Evidence that evaporates with the session is not evidence.
//
// ⚠ TWO WAYS TO NAME ROWS, AND THEY ARE NOT THE SAME. --only matches part of a
// label, which suits a person at a prompt; one label can be part of another,
// and then both run. --label and --labels-file match whole labels, for a caller
// that computed the rows it wants.
func runMutate(args []string) int {
	a, err := mutateFlags(args)
	if code, done := exitFor(err); done {
		return code
	}
	repo, t, code := openTable("mutate", a.table)
	if code != 0 {
		return code
	}
	verdicts, err := mutate.Run(repo.Root, t, a.sel, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mutate: %v\n", err)
		return 2
	}
	return finishMutate(a, t, verdicts, os.Stdout, os.Stderr)
}

// finishMutate writes the verdict file when one is asked for, then the tally.
//
// ⭐ THE FILE IS WRITTEN BEFORE THE TALLY, whatever it says. CI's keep step
// reads the files of shards that passed, and a failed shard's file is still
// worth reading. ⛔ A FILE THAT COULD NOT BE WRITTEN IS "COULD NOT RUN". The
// keep step would find a shard missing and keep nothing.
func finishMutate(a mutateArgs, t *mutate.Table, verdicts []mutate.Verdict, out, errOut io.Writer) int {
	if a.verdicts != "" {
		if err := mutate.WriteVerdicts(a.verdicts, t, a.sel.Shard, a.runner, verdicts); err != nil {
			fmt.Fprintf(errOut, "mutate: --verdicts: %v\n", err)
			return 2
		}
	}
	return mutate.Report(out, verdicts)
}

// openTable finds the repository and loads the table, or says why not.
func openTable(name, table string) (*gitrepo.Repo, *mutate.Table, int) {
	repo, err := gitrepo.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return nil, nil, 2
	}
	if table == "" {
		table = filepath.Join(repo.Root, "tools", "repo", "mutations.json")
	}
	t, err := mutate.Load(table)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return nil, nil, 2
	}
	return repo, t, 0
}

// mutateArgs is mutate's command line, read.
type mutateArgs struct {
	table, verdicts string
	sel             mutate.Selection
	// runner is what the verdict file records the rows ran on.
	runner mutate.Runner
}

// runnerOf is the runner a proof names. It is the image a flag names, the
// --jobs the shards run with, and this process's GOFLAGS.
func runnerOf(image string, jobs int) mutate.Runner {
	return mutate.Runner{Image: strings.TrimSpace(image), Jobs: jobs, GoFlags: os.Getenv("GOFLAGS")}
}

// mutateFlags reads mutate's command line into the table path and selection.
// ⚠ Apart from runMutate so a case can read it with no repository around it.
func mutateFlags(args []string) (mutateArgs, error) {
	fs := newFlagSet("mutate")
	var labels, modules stringList
	table := fs.String("table", "", "the mutation table. Empty reads tools/repo/mutations.json from the repository root")
	verdicts := fs.String("verdicts", "", "a file to write every verdict to, with its module, as CI's keep step reads them")
	only := fs.String("only", "", "run just the mutations whose label contains this")
	fs.Var(&labels, "label", "run the row with exactly this label. Repeatable. An unknown label is an error")
	labelsFile := fs.String("labels-file", "", "a file of exact labels, one per line, LF or CRLF")
	fs.Var(&modules, "module", "run only this module's rows, as tools/check. Repeatable")
	shardFlag := fs.String("shard", "", "K/N: run the K-th of N shares of the table, so N runners prove it together")
	jobs := fs.Int("jobs", 0, fmt.Sprintf("how many rows run at once, each worker in its own staged copy. Zero is half this host's processors, %d here", mutate.DefaultJobs()))
	image := fs.String("image", "", "the runner image the rows run on, as CI's $ImageOS/$ImageVersion. The verdict file records it")
	if err := parseArgs(fs, args); err != nil {
		return mutateArgs{}, err
	}
	if *jobs < 0 {
		return mutateArgs{}, fmt.Errorf("mutate: --jobs %d is below zero", *jobs)
	}
	// ⛔ REFUSED BEFORE ANY ROW RUNS. The keep step keeps a proof only under the
	// image its rows ran on. A file naming none would keep nothing after a
	// whole shard's work.
	if *verdicts != "" && strings.TrimSpace(*image) == "" {
		return mutateArgs{}, fmt.Errorf("mutate: --verdicts needs --image, because a proof is kept only for the image its rows ran on")
	}
	ran := *jobs
	if ran == 0 {
		ran = mutate.DefaultJobs()
	}
	shard, err := mutate.ParseShard(*shardFlag)
	if err != nil {
		return mutateArgs{}, fmt.Errorf("mutate: %w", err)
	}
	if *labelsFile != "" {
		f, err := os.Open(*labelsFile)
		if err != nil {
			return mutateArgs{}, fmt.Errorf("mutate: %w", err)
		}
		got, err := mutate.ReadLabels(f)
		f.Close()
		if err != nil {
			return mutateArgs{}, fmt.Errorf("mutate: --labels-file %s: %w", *labelsFile, err)
		}
		labels = append(labels, got...)
	}
	sel := mutate.Selection{Only: *only, Labels: labels, Modules: modules, Shard: shard, Jobs: *jobs}
	return mutateArgs{table: *table, verdicts: *verdicts, sel: sel, runner: runnerOf(*image, ran)}, nil
}

// proofArgs is the command line the three CI proof steps share.
type proofArgs struct {
	table, output, image string
	slots, jobs          int
}

// proofFlags adds the shared flags to fs, which the caller parses.
//
// ⛔ --image AND --jobs NAME THE SHARDS' RUNNER, and every step passes the
// same. The plan looks a proof up under them, and the keep step saves one.
func proofFlags(fs *flag.FlagSet) *proofArgs {
	a := &proofArgs{}
	fs.StringVar(&a.table, "table", "", "the mutation table. Empty reads tools/repo/mutations.json from the repository root")
	fs.StringVar(&a.output, "github-output", "", "a file to append the step's outputs to, as GITHUB_OUTPUT. Empty prints them")
	fs.IntVar(&a.slots, "slots", 0, "how many modules the workflow looks a proof up for")
	fs.StringVar(&a.image, "image", "", "the runner image the shards run on, as CI's $ImageOS/$ImageVersion")
	fs.IntVar(&a.jobs, "jobs", 0, "the --jobs every shard runs mutate with")
	return a
}

// runner is the shards' runner, as the flags name it.
func (a *proofArgs) runner() mutate.Runner { return runnerOf(a.image, a.jobs) }

// proofKeys opens the table and computes every module's key, refusing a
// workflow with too few slots.
func proofKeys(name string, a *proofArgs) (*gitrepo.Repo, *mutate.Table, []mutate.ModuleKey, int) {
	repo, t, code := openTable(name, a.table)
	if code != 0 {
		return nil, nil, nil, code
	}
	keys, err := moduleKeys(repo.Root, t, a.slots, a.runner())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		return nil, nil, nil, 2
	}
	return repo, t, keys, 0
}

func moduleKeys(root string, t *mutate.Table, slots int, rn mutate.Runner) ([]mutate.ModuleKey, error) {
	goVersion, err := mutate.GoVersion(root)
	if err != nil {
		return nil, err
	}
	keys, err := mutate.ProofKeys(root, t, goVersion, rn)
	if err != nil {
		return nil, err
	}
	if _, err := mutate.SlotOutputs(keys, slots, func(k mutate.ModuleKey) string { return k.Key }, "key"); err != nil {
		return nil, err
	}
	return keys, nil
}

// writeOutputs appends lines to a GITHUB_OUTPUT file, or prints them.
func writeOutputs(file string, lines []string) error {
	body := strings.Join(lines, "\n") + "\n"
	if file == "" {
		_, err := fmt.Print(body)
		return err
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// runMutateKeys is CI's first plan step: every module's proof key, and the
// path its proof is kept at, one slot each.
func runMutateKeys(args []string) int {
	fs := newFlagSet("mutate-keys")
	a := proofFlags(fs)
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	_, _, keys, code := proofKeys("mutate-keys", a)
	if code != 0 {
		return code
	}
	var lines []string
	for _, f := range []struct {
		name  string
		value func(mutate.ModuleKey) string
	}{
		{"module", func(k mutate.ModuleKey) string { return k.Module }},
		{"key", func(k mutate.ModuleKey) string { return k.Key }},
		{"path", func(k mutate.ModuleKey) string { return k.Path }},
	} {
		got, _ := mutate.SlotOutputs(keys, a.slots, f.value, f.name)
		lines = append(lines, got...)
	}
	for _, k := range keys {
		fmt.Printf("  %s, %d row(s): %s\n", k.Module, len(k.Rows), k.Key)
	}
	if err := writeOutputs(a.output, lines); err != nil {
		fmt.Fprintf(os.Stderr, "mutate-keys: %v\n", err)
		return 2
	}
	return 0
}

// runMutatePlan is CI's second plan step: which modules a kept proof answers,
// and how many shards the rest are cut into.
func runMutatePlan(args []string) int {
	fs := newFlagSet("mutate-plan")
	a := proofFlags(fs)
	shards := fs.Int("shards", 0, "the most shards the rows that run are cut into")
	full := fs.Bool("full", false, "read no kept proof and run every module, as the nightly and the release do")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	repo, t, keys, code := proofKeys("mutate-plan", a)
	if code != 0 {
		return code
	}
	p, err := mutate.MakePlan(repo.Root, t, keys, *full, *shards, os.Stdout)
	if err == nil {
		err = writeOutputs(a.output, p.Outputs())
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "mutate-plan: %v\n", err)
		return 2
	}
	return 0
}

// runMutateKeep is CI's last proof step: the proof of every module whose rows
// all passed in the shards, written where the workflow saves it from.
func runMutateKeep(args []string) int {
	fs := newFlagSet("mutate-keep")
	a := proofFlags(fs)
	dir := fs.String("verdicts", "", "a directory holding every shard's verdict file")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	repo, t, keys, code := proofKeys("mutate-keep", a)
	if code != 0 {
		return code
	}
	kept, short, err := mutate.Keep(repo.Root, t, keys, *dir, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mutate-keep: %v\n", err)
		return 2
	}
	lines, code := keepOutputs(keys, kept, short, a.slots)
	paths, _ := mutate.SlotOutputs(keys, a.slots, func(k mutate.ModuleKey) string { return k.Path }, "path")
	if err := writeOutputs(a.output, append(lines, paths...)); err != nil {
		fmt.Fprintf(os.Stderr, "mutate-keep: %v\n", err)
		return 2
	}
	if code != 0 {
		fmt.Fprintln(os.Stderr, "mutate-keep: a module that ran is not whole, so it is not kept and every later push runs it again")
	}
	return code
}

// keepOutputs names each slot's key to save, empty where nothing was kept,
// and the exit code.
//
// ⛔ A MODULE THAT RAN AND WAS NOT KEPT FAILS THE STEP. Every shard passed, so
// its missing verdicts are a missing file. A green step would hide that the
// module runs again on every push.
func keepOutputs(keys, kept []mutate.ModuleKey, short bool, slots int) ([]string, int) {
	saved := map[string]bool{}
	for _, k := range kept {
		saved[k.Module] = true
	}
	lines, _ := mutate.SlotOutputs(keys, slots, func(k mutate.ModuleKey) string {
		if saved[k.Module] {
			return k.Key
		}
		return ""
	}, "save")
	code := 0
	if short {
		code = 1
	}
	return lines, code
}

func runLicense(args []string) int {
	fs := newFlagSet("license")
	var opts license.Options
	fs.StringVar(&opts.ID, "id", "", "the SPDX id, as named by --list")
	fs.StringVar(&opts.Holder, "holder", "", "the copyright holder. Empty reads git config user.name and refuses if that is unset too")
	fs.StringVar(&opts.Year, "year", "", "the copyright year. Empty is this year, UTC")
	fs.StringVar(&opts.Out, "out", "LICENSE", "where to write it")
	fs.BoolVar(&opts.Force, "force", false, "write a licence whose text carries somebody else's copyright, after editing the notice by hand")
	fs.BoolVar(&opts.List, "list", false, "the licences this knows how to fill, and which have text on disk")
	fs.BoolVar(&opts.JSON, "json", false, "write a structured answer")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	return license.Run(opts, os.Stdout, os.Stderr)
}

func runBinfmt(args []string) int {
	fs := newFlagSet("binfmt")
	var opts binfmt.Options
	fs.BoolVar(&opts.JSON, "json", false, "write a structured answer")
	fs.StringVar(&opts.Distro, "distro", "podman-machine-default", "the WSL distribution to read the kernel through, on a host with no binfmt_misc of its own")
	fs.IntVar(&opts.Require, "require", 0, "fail below this many handlers. Zero reports and does not judge")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	return binfmt.Run(opts, os.Stdout, os.Stderr)
}

func runRemote(args []string) int {
	fs := newFlagSet("remote-items")
	var opts remote.Options
	fs.BoolVar(&opts.JSON, "json", false, "write a structured answer. The document goes to stdout and the report to stderr")
	fs.StringVar(&opts.Repo, "repo", "", "OWNER/NAME. Empty means the repository this checkout belongs to")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	return remote.Run(opts, os.Stdout, os.Stderr)
}

func runRelease(args []string) int {
	fs := newFlagSet("release")
	var opts toolrelease.Options
	fs.BoolVar(&opts.Publish, "publish", false, "create and push the annotated tag after every refusal passes")
	fs.StringVar(&opts.Remote, "remote", "origin", "the configured Azathothas/ToolKit remote whose live main must equal HEAD and receive the tag")
	fs.BoolVar(&opts.JSON, "json", false, "write a structured readiness report")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	return toolrelease.Run(opts, os.Stdout, os.Stderr)
}

// stringList is a flag that may be repeated.
//
// ⚠ REPEATABLE RATHER THAN COMMA-SEPARATED. A comma is a legal character in a
// path, and splitting on one turns a correct pathspec into a set of pathspecs
// that match nothing.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func runGitSync(args []string) int {
	fs := newFlagSet("git-sync")
	var opts gitsync.Options
	var paths, gates stringList
	fs.StringVar(&opts.Message, "message", "", "the commit subject")
	fs.StringVar(&opts.BodyFile, "body-file", "", "a file whose bytes become the commit body")
	fs.StringVar(&opts.Name, "name", "", "the author and committer name. Empty reads git config")
	fs.StringVar(&opts.Email, "email", "", "the author and committer email. Empty reads git config")
	fs.StringVar(&opts.Branch, "branch", "", "the branch to push. Empty is the current one")
	fs.Var(&paths, "path", "a pathspec to stage instead of everything. Repeatable")
	fs.Var(&gates, "gate", "a command to run before the push. Repeatable")
	fs.BoolVar(&opts.NoPush, "no-push", false, "commit and stop")
	fs.BoolVar(&opts.PushOnly, "push-only", false, "push what is already committed")
	fs.BoolVar(&opts.Check, "check", false, "the read-only half: check the message and HEAD, change nothing")
	fs.BoolVar(&opts.SkipGates, "skip-gates", false, "push with no proof the tree is green")
	fs.BoolVar(&opts.NoCI, "no-ci", false, "add a CI skip marker deliberately")
	fs.BoolVar(&opts.JSON, "json", false, "write a structured answer")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	opts.Paths, opts.Gates = paths, gates
	return gitsync.Run(opts, os.Stdout, os.Stderr)
}

// runReleaseStage is release.yml's staging step. The decisions live in
// internal/workflow, which says why each one is there. TOOL-32.
func runReleaseStage(args []string) int {
	fs := newFlagSet("release-stage")
	var opts workflow.StageOptions
	fs.StringVar(&opts.Dist, "dist", "dist", "the staging directory, relative to the repository root")
	fs.StringVar(&opts.HerdrLabel, "herdr-label", "", "the label herdr-build.yml named herdr's four builds with, as herdr-0.9.1")
	fs.StringVar(&opts.HerdrFrom, "herdr-from", "herdr-artifacts/out", "where herdr's four builds were downloaded to, relative to the repository root")
	fs.StringVar(&opts.GitHubEnv, "github-env", "", "a file to append SIGN_COUNT=N to, as GITHUB_ENV. Empty prints it only")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	repo, err := gitrepo.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "release-stage: %v\n", err)
		return 2
	}
	opts.Root = repo.Root
	if !filepath.IsAbs(opts.HerdrFrom) {
		opts.HerdrFrom = filepath.Join(repo.Root, filepath.FromSlash(opts.HerdrFrom))
	}
	if err := workflow.Stage(opts, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "release-stage: %v. Nothing is published from a staging step that failed.\n", err)
		return 1
	}
	return 0
}

// runReleaseReadback is the publish read-back both release workflows end with.
func runReleaseReadback(args []string) int {
	fs := newFlagSet("release-readback")
	dist := fs.String("dist", "dist", "the staging directory whose every file must be on the release")
	reported := fs.String("reported", "", "a file holding the names the release reports, one per line, as gh release view --json assets --jq '.assets[].name' writes them")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	if *reported == "" {
		fmt.Fprintln(os.Stderr, "release-readback: --reported is required, and it is what the release says it holds")
		return 2
	}
	staged, err := workflow.Files(*dist)
	if err != nil {
		fmt.Fprintf(os.Stderr, "release-readback: %v\n", err)
		return 2
	}
	f, err := os.Open(*reported)
	if err != nil {
		fmt.Fprintf(os.Stderr, "release-readback: %v\n", err)
		return 2
	}
	names, err := workflow.ReadNames(f)
	f.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "release-readback: %v\n", err)
		return 2
	}
	if err := workflow.ReadBack(staged, names); err != nil {
		fmt.Fprintf(os.Stderr, "release-readback: %v\n", err)
		return 1
	}
	fmt.Printf("every one of %d staged asset(s) is on the release, which reports %d\n", len(staged), len(names))
	return 0
}

// runNightlyPrune is herdr-nightly.yml's prune.
func runNightlyPrune(args []string) int {
	fs := newFlagSet("nightly-prune")
	releases := fs.String("releases", "", "a file holding gh api repos/OWNER/NAME/releases as it answered")
	keep := fs.Int("keep", 7, "how many of the newest nightlies stay")
	del := fs.Bool("delete", false, "delete the old ones with gh. Without it they are printed and nothing is deleted")
	ghRepo := fs.String("repo", "", "OWNER/NAME, which --delete hands gh")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	if *releases == "" {
		fmt.Fprintln(os.Stderr, "nightly-prune: --releases is required")
		return 2
	}
	if *del && *ghRepo == "" {
		fmt.Fprintln(os.Stderr, "nightly-prune: --delete needs --repo OWNER/NAME, so gh deletes from the repository it was meant to")
		return 2
	}
	body, err := os.ReadFile(*releases)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nightly-prune: %v\n", err)
		return 2
	}
	list, err := workflow.ParseReleases(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nightly-prune: %v\n", err)
		return 2
	}
	old, err := workflow.Prune(list, *keep)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nightly-prune: %v\n", err)
		return 1
	}
	if !*del {
		for _, tag := range old {
			fmt.Println(tag)
		}
		return 0
	}
	err = workflow.DeleteAll(old, func(tag string) error {
		cmd := exec.Command("gh", "release", "delete", tag, "--repo", *ghRepo, "--yes", "--cleanup-tag")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "nightly-prune: %v\n", err)
		return 1
	}
	if len(old) == 0 {
		fmt.Println("pruned: nothing")
	} else {
		fmt.Println("pruned: " + strings.Join(old, " "))
	}
	return 0
}

// runNightlyStage is herdr-nightly.yml's staging step. TOOL-32.
func runNightlyStage(args []string) int {
	fs := newFlagSet("nightly-stage")
	var opts workflow.NightlyStageOptions
	fs.StringVar(&opts.Builds, "builds", "artifacts/out", "where herdr's four builds were downloaded to")
	fs.StringVar(&opts.Records, "records", "artifacts", "where the four buildinfo-*.json records were downloaded to")
	fs.StringVar(&opts.Dist, "dist", "dist", "the staging directory")
	fs.StringVar(&opts.Tag, "tag", "", "the nightly's tag, as herdr-nightly-20261024-abc1234")
	fs.StringVar(&opts.Commit, "commit", "", "the herdr commit the nightly was built from")
	fs.StringVar(&opts.Run, "run", "", "the URL of the workflow run that built it")
	fs.StringVar(&opts.GitHubEnv, "github-env", "", "a file to append SIGN_COUNT=N to, as GITHUB_ENV. Empty prints it only")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	if err := workflow.NightlyStage(opts, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "nightly-stage: %v. Nothing is published from a staging step that failed.\n", err)
		return 1
	}
	return 0
}

// runNightlyPublish is herdr-nightly.yml's publish step. TOOL-37.
func runNightlyPublish(args []string) int {
	fs := newFlagSet("nightly-publish")
	var opts workflow.NightlyPublishOptions
	fs.StringVar(&opts.Tag, "tag", "", "the nightly's tag")
	fs.StringVar(&opts.Repo, "repo", "", "OWNER/NAME, which gh is handed")
	fs.StringVar(&opts.Target, "target", "", "the commit the tag is made on when the prerelease is created")
	fs.StringVar(&opts.NotesFile, "notes-file", "", "the prerelease's notes")
	fs.StringVar(&opts.Dist, "dist", "dist", "the staging directory whose every file is uploaded")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	gh := func(a ...string) (string, error) {
		cmd := exec.Command("gh", a...)
		var out, errBuf strings.Builder
		cmd.Stdout, cmd.Stderr = &out, &errBuf
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("gh %s: %w: %s", strings.Join(a[:2], " "), err, strings.TrimSpace(errBuf.String()))
		}
		return out.String(), nil
	}
	if err := workflow.NightlyPublish(opts, gh, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "nightly-publish: %v\n", err)
		return 1
	}
	return 0
}

// runLinuxProof proves a commit's tree on Linux before it is pushed. TOOL-33.
func runLinuxProof(args []string) int {
	fs := newFlagSet("linux-proof")
	var opts linuxproof.Options
	fs.StringVar(&opts.Rev, "rev", "HEAD", "the commit whose tree is proved")
	fs.StringVar(&opts.RowsSince, "rows-since", "", "run every mutation row added to tools/repo/mutations.json between this ref and --rev. Empty counts them from the newest commit --remote is known to hold, as the pre-push hook does")
	fs.StringVar(&opts.Remote, "remote", "origin", "the remote whose tracked refs the added rows are counted from")
	fs.StringVar(&opts.WSLToolkit, "wsl-toolkit", "", "the wsl-toolkit executable the jobs run through. Empty finds wsl-toolkit on PATH, or on a Linux host runs them natively")
	if code, done := exitFor(parseArgs(fs, args)); done {
		return code
	}
	return linuxproof.Run(opts, os.Stdout, os.Stderr)
}

// runPrePush is what .githooks/pre-push calls with git's own arguments.
//
// ⚠ POSITIONAL, NOT FLAGS, because git hands a pre-push hook the remote's name
// and URL as $1 and $2 and the refs on stdin, and the hook forwards them as
// they came.
func runPrePush(args []string) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprintln(os.Stderr, "repo pre-push REMOTE [URL] < git's update lines")
		fmt.Fprintln(os.Stderr, "  Proves each pushed tree that has no proof from repo linux-proof, and refuses the push when that fails. Called by .githooks/pre-push.")
		return 0
	}
	return linuxproof.PrePush(args, os.Stdin, os.Stdout, os.Stderr, "")
}
