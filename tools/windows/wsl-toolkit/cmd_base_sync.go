// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

const baseSyncUsage = `wsl-toolkit base sync [flags] HOST-DIR GUEST-DIR

  Copy a Windows checkout into the base's own filesystem, and on the next call
  send only what changed. The list is a job's workspace copy: --exclude and
  --gitignore as run takes them, with jobs.exclude and jobs.gitignore as the
  defaults. A file travels when its size, its modification time or its
  executable bit differs.

  GUEST-DIR is an absolute guest path or one under ~/, inside the account's
  home unless --force. The home itself, this tool's own directory and a
  Windows directory seen through a grant or a drive are refused.

  --delete removes guest entries the list no longer holds. A path an
  --exclude names or git ignores stays, so a build tree made in the base
  survives the next sync.

  --exclude PATTERN  leave a path out of the list. Repeatable
  --gitignore        leave out what git ignores in HOST-DIR
  --delete           remove guest entries the list no longer holds
  --force            let GUEST-DIR be outside the account's home
  --max-bytes N      refuse a list whose files hold more than N bytes
  --max-entries N    refuse a list of more than N entries
  --json             write a structured answer

  Exit 0: the guest holds the list. 1: the sync started and did not finish.
  2: it was refused, and nothing in the guest changed.
`

// cmdBaseSync copies a Windows checkout into the base's own filesystem,
// incrementally. WSL-125.
//
// ⛔ THE HELPER DOES NOT CARRY IT. It writes into the base, which is not job data,
// for the reason `base exec` gives.
func cmdBaseSync(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("base sync")
	var excludes stringList
	var gitignore, gitignoreSet bool
	fs.Var(&excludes, "exclude", "a glob to leave out of the list, matched as run's --exclude matches it. Repeatable, and added to jobs.exclude")
	fs.Var(boolFlag{&gitignore, &gitignoreSet}, "gitignore", "leave out of the list what git ignores in HOST-DIR, read from git itself. jobs.gitignore sets it for a project, and --gitignore=false turns it off")
	del := fs.Bool("delete", false, "remove guest entries the list no longer holds. A path an --exclude names or git ignores stays")
	force := fs.Bool("force", false, "let GUEST-DIR be outside the account's home. The home itself, the root, this tool's own directory and a Windows directory stay refused")
	maxBytes := fs.Int64("max-bytes", toolkit.DefaultWorkspaceLimits().MaxBytes, "refuse a list whose files hold more than this many bytes")
	maxEntries := fs.Int("max-entries", toolkit.DefaultWorkspaceLimits().MaxEntries, "refuse a list with more entries than this")
	asJSON := fs.Bool("json", false, "write a structured answer")
	names, err := parseNames(fs, args, 2)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, baseSyncUsage)
			return exitOK, nil
		}
		return exitCannot, err
	}
	if *maxBytes <= 0 || *maxEntries <= 0 {
		return exitCannot, errors.New("--max-bytes and --max-entries are positive")
	}
	if err := gitBashRewrite("GUEST-DIR", names[1], guestPath); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	host, err := filepath.Abs(msysHostPath(names[0]))
	if err != nil {
		return exitCannot, fmt.Errorf("HOST-DIR: %w", err)
	}
	if info, err := os.Stat(host); err != nil {
		return exitCannot, fmt.Errorf("HOST-DIR: %w", err)
	} else if !info.IsDir() {
		return exitCannot, fmt.Errorf("HOST-DIR %s is not a directory", host)
	}
	if err := toolkit.AssertProjectPath("HOST-DIR resolved to", host); err != nil {
		return exitCannot, err
	}
	// ⭐ THE PROJECT'S RULES ARE THE DEFAULTS, and a command line adds to them, as
	// they are for a job's copy. WSL-112.
	all := append(append(stringList(nil), cfg.Jobs.Exclude...), excludes...)
	if cfg.Jobs.Gitignore && !gitignoreSet {
		gitignore = true
	}
	if c, err := useHelper(ctx, false); err != nil {
		return exitCannot, err
	} else if c != nil {
		return exitCannot, errors.New("base sync writes into the base, which the restricted helper protocol does not accept. Make this call through the session's WSL approval path")
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
		return exitCannot, errNotRegistered(cfg.Base.Name)
	}
	rep, err := toolkit.SyncBase(ctx, w, toolkit.SyncSpec{
		Distro: cfg.Base.Name, User: cfg.Base.User, HostDir: host, GuestDir: names[1],
		Filter: toolkit.WorkspaceFilter{Excludes: toolkit.SortedExcludes(all), Gitignore: gitignore},
		Limits: toolkit.WorkspaceLimits{MaxBytes: *maxBytes, MaxEntries: *maxEntries},
		Delete: *del, Force: *force, Log: note,
	})
	if err != nil {
		if errors.Is(err, toolkit.ErrSyncIncomplete) {
			return exitFailed, err
		}
		return exitCannot, err
	}
	if *asJSON {
		return exitOK, writeJSON(rep)
	}
	logf("  sync: %d entries listed from %s, %d sent (%s), %d unchanged, %d deleted, %d kept, into %s",
		rep.Entries, rep.HostDir, rep.Sent, toolkit.HumanBytes(rep.SentBytes), rep.Unchanged, rep.Deleted, rep.Kept, rep.GuestDir)
	if rep.Ignored > 0 {
		logf("  sync: %d path(s) git ignores were left out, by --gitignore", rep.Ignored)
	}
	for _, o := range rep.Omission {
		logf("  sync: left out %s: %s", o.Path, o.Reason)
	}
	if rep.ExecRestored != "" {
		logf("  sync: %s", rep.ExecRestored)
	}
	return exitOK, nil
}

// parseNames parses a subcommand that takes exactly want names beside its flags,
// which may stand before, between or after them. WSL-125.
func parseNames(fs *flag.FlagSet, args []string, want int) ([]string, error) {
	var names []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			break
		}
		names = append(names, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(names) != want {
		return nil, fmt.Errorf("%s takes %d names, and was given %d: %s", fs.Name(), want, len(names), strings.Join(quoted(names), " "))
	}
	return names, nil
}

// quoted is each name in quotes, for a refusal that lists them.
func quoted(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, fmt.Sprintf("%q", n))
	}
	return out
}
