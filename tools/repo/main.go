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
// host probe, a commit path, a licence writer and two remote readers; folding
// them in would make the gate do things that are not checks, and a gate whose
// scope drifts is one nobody can say the meaning of.
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
	"os"
	"sort"
	"strings"

	"github.com/Azathothas/ToolKit/tools/repo/internal/binfmt"
	"github.com/Azathothas/ToolKit/tools/repo/internal/deslop"
	"github.com/Azathothas/ToolKit/tools/repo/internal/license"
	"github.com/Azathothas/ToolKit/tools/repo/internal/remote"
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
		{"license", "write LICENSE from a template, with the holder filled in", runLicense},
		{"remote-items", "what is open against this repository, and does it survive checking", runRemote},
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
