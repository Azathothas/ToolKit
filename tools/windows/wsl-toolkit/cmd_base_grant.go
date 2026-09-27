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

// baseGrantAnswer is what `base grant` and `base revoke` answer.
type baseGrantAnswer struct {
	Schema    string              `json:"schema"`
	Action    string              `json:"action"`
	File      string              `json:"file"`
	Mount     toolkit.BaseMount   `json:"mount"`
	Unchanged bool                `json:"unchanged"`
	Live      bool                `json:"live"`
	Grants    []toolkit.BaseMount `json:"grants"`
}

// grantFlags are the values of the flag set base grant and base revoke share.
type grantFlags struct {
	source, target, mode string
	asJSON, viaHelper    bool
}

// grantShared is that set, and revoke reads --target alone: a grant is named by
// where the base sees it.
//
// ⛔ REVOKE DOES NOT DESCRIBE ITSELF IN GRANT'S WORDS. `base revoke --help` once
// answered "--source: the Windows directory to grant" over a command that
// refuses --source, and the help sent a reader straight into the refusal. Found
// by writing the guide from the help and then running it. Its help now lists
// only what it reads, and --target in its own words. WSL-104.
func grantShared(sub string) sharedFlags[grantFlags] {
	targetHelp := "where the base sees it, under /workspaces. Default /workspaces/ and the directory's name"
	if sub == "revoke" {
		targetHelp = "the /workspaces path of the grant to take away"
	}
	return sharedFlags[grantFlags]{
		group: "base",
		order: []string{"grant", "revoke"},
		reads: map[string][]string{
			"grant":  {"source", "target", "mode", "json", "via-helper"},
			"revoke": {"target", "json", "via-helper"},
		},
		bind: func(g *grantFlags, fs *flag.FlagSet) {
			fs.StringVar(&g.source, "source", "", "the Windows directory to grant")
			fs.StringVar(&g.target, "target", "", targetHelp)
			fs.StringVar(&g.mode, "mode", "", "ro or rw. Default ro")
			fs.BoolVar(&g.asJSON, "json", false, "write a structured answer")
			fs.BoolVar(&g.viaHelper, "via-helper", false, "prove the helper refusal for a "+sub)
		},
	}
}

// cmdBaseGrant adds one Windows directory to the base, or takes one away, without a
// restart.
//
// ⛔ THE BASE CHANGES FIRST AND THE FILE SECOND, AND A FILE THAT CANNOT BE WRITTEN
// TAKES THE CHANGE BACK. A grant written to the configuration and refused live
// would be applied by the next ensure through a restart, which is what this command
// exists to avoid; a live grant the file does not name is refused by the next probe
// as an unconfigured mount. Either half alone is a base that disagrees with itself.
//
// ⭐ NO RESTART, SO NOTHING RUNNING IN THE BASE STOPS. That is the point of it: a
// restart ends every agent in the base. WSL-75.
func cmdBaseGrant(ctx context.Context, sub string, args []string) (int, error) {
	var g grantFlags
	if err := grantShared(sub).parse(sub, &g, args); err != nil {
		return exitCannot, err
	}
	if err := gitBashRewrite("--target", g.target, guestPath); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	var change toolkit.GrantChange
	switch sub {
	case "grant":
		if g.source == "" {
			return exitCannot, errors.New("base grant needs --source, the Windows directory to grant")
		}
		change, err = toolkit.PlanGrant(cfg, g.source, g.target, g.mode)
	case "revoke":
		if g.target == "" {
			return exitCannot, errors.New("base revoke needs --target, the /workspaces path of the grant to take away")
		}
		change, err = toolkit.PlanRevoke(cfg, g.target)
	}
	if err != nil {
		return exitCannot, err
	}
	ans := baseGrantAnswer{Schema: "wsl-toolkit-base-grant/1", Action: sub, File: cfg.Path(), Mount: change.Mount, Unchanged: change.Unchanged}
	if change.Unchanged {
		logf("  %s is already granted from %s, %s. Nothing to do", change.Mount.Target, change.Mount.Source, change.Mount.Mode)
		return renderGrant(ans, cfg, g.asJSON)
	}
	if c, err := useHelper(ctx, g.viaHelper); err != nil {
		return exitCannot, err
	} else if c != nil {
		return exitCannot, fmt.Errorf("base %s mounts a Windows directory into the base, which the restricted helper protocol does not accept. Make this call through the session's WSL approval path", sub)
	}
	w, err := toolkit.FindWsl()
	if err != nil {
		return exitCannot, err
	}
	registered, err := w.Exists(ctx, cfg.Base.Name)
	if err != nil {
		return exitCannot, err
	}
	if registered {
		base, err := toolkit.NewBase(change.Next, note)
		if err != nil {
			return exitCannot, err
		}
		if _, err := base.ChangeGrantsLive(ctx, change.Next); err != nil {
			// ⚠ NOTHING WAS WRITTEN. The file still says what the base had.
			return exitFailed, fmt.Errorf("%s. %s is unchanged", strings.TrimRight(err.Error(), "."), cfg.Path())
		}
		ans.Live = true
	} else {
		note(cfg.Base.Name + " is not registered, so only the configuration changes. base ensure mounts what it names")
	}
	if err := change.Next.WriteTo(cfg.Path()); err != nil {
		if registered {
			if base, baseErr := toolkit.NewBase(cfg, note); baseErr == nil {
				if _, backErr := base.ChangeGrantsLive(ctx, cfg); backErr != nil {
					return exitCannot, fmt.Errorf("%s could not be written (%v), and taking the live change back failed too: %w", cfg.Path(), err, backErr)
				}
			}
		}
		return exitCannot, fmt.Errorf("%s could not be written, so the live change was taken back: %w", cfg.Path(), err)
	}
	logf("  %s %s %s <- %s, written to %s", sub, change.Mount.Mode, change.Mount.Target, change.Mount.Source, cfg.Path())
	return renderGrant(ans, change.Next, g.asJSON)
}

func renderGrant(ans baseGrantAnswer, cfg toolkit.Config, asJSON bool) (int, error) {
	grants, err := cfg.ResolvedBaseMounts()
	if err != nil {
		return exitCannot, err
	}
	ans.Grants = grants
	if ans.Grants == nil {
		ans.Grants = []toolkit.BaseMount{}
	}
	if asJSON {
		return exitOK, writeJSON(ans)
	}
	for _, g := range grants {
		fmt.Fprintf(os.Stderr, "  grant       %s %s <- %s\n", g.Mode, g.Target, g.Source)
	}
	return exitOK, nil
}
