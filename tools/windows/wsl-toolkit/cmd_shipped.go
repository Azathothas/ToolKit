// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const shippedUsage = `wsl-toolkit shipped <list|cat|write>

  The general-purpose files this executable carries, so a machine with the binary
  needs nothing else: no clone, and no fetch.

  list          name, length and SHA-256 of each. --json for a structured answer
  cat NAME      write one to stdout, unchanged
  write NAME    write one into --dir, which defaults to the working directory

  --dir DIR     where write puts it
  --force       replace a file already there whose content differs
`

// cmdShipped answers what the executable carries and writes it out.
//
// ⭐ THE POINT IS THAT NOTHING HAS TO BE FETCHED. `bootstrap.sh` is published by
// URL for callers outside this tree and that contract is unchanged; what this
// removes is an operator copying it into a checkout before a base can be
// provisioned, with no way afterwards to say which version they copied.
//
// ⛔ THE DIGEST IS PART OF THE ANSWER. "the bootstrap this binary carries" is not
// a version, and two builds of this tool can carry different bytes under the same
// product version, so `list` prints what a caller can compare.
func cmdShipped(_ context.Context, args []string) (int, error) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, shippedUsage)
		return exitCannot, nil
	}
	sub, rest := args[0], args[1:]
	// ⚠ ONE FLAG SET PER SUBCOMMAND, as `base` registers its own: the manual is
	// generated from the registered sets, and a help form with none is a form the
	// manual cannot describe. A test refuses that, and it refused this.
	fs := newFlagSet("shipped " + sub)
	// ⛔ EACH SUBCOMMAND REGISTERS ONLY WHAT IT READS. Registering all three on all
	// three gave `shipped cat --json` and `shipped write --json`, which parse and
	// do nothing: a flag no code reads is a lie about a capability, and the manual
	// is generated from these sets so it would have published both.
	var asJSON, force *bool
	var dir *string
	switch sub {
	case "list":
		asJSON = fs.Bool("json", false, "write a structured answer")
	case "write":
		dir = fs.String("dir", "", "where the file is put. Default: the working directory")
		force = fs.Bool("force", false, "replace a file already there whose content differs")
	}
	if err := parseArgs(fs, rest); err != nil {
		return exitCannot, err
	}
	files, err := toolkit.ShippedFiles()
	if err != nil {
		return exitCannot, err
	}
	switch sub {
	case "list":
		if asJSON != nil && *asJSON {
			return exitOK, writeJSON(struct {
				Files []toolkit.ShippedFile `json:"files"`
			}{files})
		}
		for _, f := range files {
			fmt.Fprintf(os.Stderr, "  %-18s %8d bytes  %s\n", f.Name, f.Bytes, f.SHA256)
			fmt.Fprintf(os.Stderr, "  %-18s from %s\n", "", f.Source)
		}
		return exitOK, nil

	case "cat":
		if len(fs.Args()) != 1 {
			return exitCannot, errors.New("shipped cat takes one name: shipped cat bootstrap.sh")
		}
		body, err := toolkit.ShippedBody(fs.Args()[0])
		if err != nil {
			return exitCannot, err
		}
		// ⛔ STDOUT AND UNCHANGED. This is the answer, so nothing else goes there.
		_, err = os.Stdout.Write(body)
		return exitOK, err

	case "write":
		if len(fs.Args()) != 1 {
			return exitCannot, errors.New("shipped write takes one name: shipped write bootstrap.sh --dir .")
		}
		target := ""
		if dir != nil {
			target = *dir
		}
		if target == "" {
			target, err = os.Getwd()
			if err != nil {
				return exitCannot, err
			}
		}
		dest, wrote, err := toolkit.WriteShipped(fs.Args()[0], target, force != nil && *force)
		if err != nil {
			return exitCannot, err
		}
		if wrote {
			note("wrote " + dest)
		} else {
			note(dest + " is already the one this executable carries, so nothing was written")
		}
		return exitOK, nil

	default:
		fmt.Fprint(os.Stderr, shippedUsage)
		return exitCannot, fmt.Errorf("%q is not a shipped subcommand", sub)
	}
}

const baseBootstrapUsage = `wsl-toolkit base bootstrap [--] ARGS...

  Run the bootstrap this executable carries inside the base, as its account, with
  every argument the bootstrap's own.

  ⛔ Nothing is copied into a checkout and nothing is fetched: the script travels
  on stdin, as every other payload this tool sends does.

  Examples:
    wsl-toolkit base bootstrap -- --toolset agent
    wsl-toolkit base bootstrap -- --dry-run --toolset developer
`

// cmdBaseBootstrap runs the carried bootstrap in the base.
//
// ⛔ IT REPLACES A STEP THAT COULD NOT BE GOT RIGHT BY HAND. The worked example
// used to begin "copy bootstrap.sh into the target checkout as
// .wsl-toolkit/common/bootstrap.sh", which puts a copy of a moving file in a
// place nothing checks, in a checkout the base can only reach through a grant.
//
// ⚠ THE SCRIPT AND ITS ARGUMENTS TRAVEL SEPARATELY. The script is the payload on
// stdin and the arguments are quoted into the line that runs it, so a value that
// spells a shell operator reaches the bootstrap as a value.
func cmdBaseBootstrap(ctx context.Context, args []string) (int, error) {
	_ = newFlagSet("base bootstrap")
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprint(os.Stderr, baseBootstrapUsage)
		return exitOK, flag.ErrHelp
	}
	passed := args
	if len(passed) > 0 && passed[0] == "--" {
		passed = passed[1:]
	}
	body, err := toolkit.ShippedBody("bootstrap.sh")
	if err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if c, err := useHelper(ctx, false); err != nil {
		return exitCannot, err
	} else if c != nil {
		return exitCannot, errors.New("base bootstrap runs a script in the base, which the restricted helper protocol does not accept. Make this call through the session's WSL approval path")
	}
	w, err := toolkit.FindWsl()
	if err != nil {
		return exitCannot, err
	}
	registered, err := w.Exists(ctx, cfg.Base.Name)
	if err != nil {
		return exitCannot, err
	}
	if !registered {
		return exitCannot, fmt.Errorf("%s is not registered. Build it with: wsl-toolkit base ensure", cfg.Base.Name)
	}
	note("running the bootstrap this executable carries, " + shippedDigestNote(body))
	return baseExecResult(w.Exec(ctx, toolkit.ExecRequest{
		Distro:  cfg.Base.Name,
		User:    cfg.Base.User,
		Script:  toolkit.ShippedScript(body, passed),
		Payload: true,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}))
}

// shippedDigestNote names which bytes are about to run, abbreviated.
func shippedDigestNote(body []byte) string {
	files, err := toolkit.ShippedFiles()
	if err != nil {
		return "digest unknown"
	}
	for _, f := range files {
		if f.Name == "bootstrap.sh" && f.Bytes == len(body) {
			return fmt.Sprintf("%d bytes, SHA-256 %s", f.Bytes, strings.ToUpper(f.SHA256[:8])+"...")
		}
	}
	return "digest unknown"
}
