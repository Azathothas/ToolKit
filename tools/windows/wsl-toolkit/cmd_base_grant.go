// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
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
	fs := newFlagSet("base " + sub)
	// ⛔ REVOKE DOES NOT DESCRIBE ITSELF IN GRANT'S WORDS. Both register one flag set,
	// so `base revoke --help` used to answer "--source: the Windows directory to grant"
	// over a command that REFUSES --source, and the help sent a reader straight into
	// the refusal. Found by writing the guide from the help and then running it.
	//
	// ⚠ THE FLAGS STAY REGISTERED FOR REVOKE ON PURPOSE. Dropping them would turn its
	// own refusal, which names --target and says why, into an unknown-flag error, and
	// that refusal is the one a caller who guessed --source should meet.
	sourceHelp := "the Windows directory to grant"
	targetHelp := "where the base sees it, under /workspaces. Default /workspaces/ and the directory's name"
	modeHelp := "ro or rw. Default ro"
	if sub == "revoke" {
		sourceHelp = "⛔ not for revoke: name the grant by --target, where the base sees it"
		targetHelp = "the /workspaces path of the grant to take away"
		modeHelp = "⛔ not for revoke: a grant is taken away whatever its mode"
	}
	source := fs.String("source", "", sourceHelp)
	target := fs.String("target", "", targetHelp)
	mode := fs.String("mode", "", modeHelp)
	asJSON := fs.Bool("json", false, "write a structured answer")
	viaHelper := fs.Bool("via-helper", false, "prove the helper refusal for a "+sub)
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	var change toolkit.GrantChange
	switch sub {
	case "grant":
		if *source == "" {
			return exitCannot, errors.New("base grant needs --source, the Windows directory to grant")
		}
		change, err = toolkit.PlanGrant(cfg, *source, *target, *mode)
	case "revoke":
		if *source != "" || *mode != "" {
			return exitCannot, errors.New("base revoke takes --target alone: a grant is named by where the base sees it")
		}
		if *target == "" {
			return exitCannot, errors.New("base revoke needs --target, the /workspaces path of the grant to take away")
		}
		change, err = toolkit.PlanRevoke(cfg, *target)
	}
	if err != nil {
		return exitCannot, err
	}
	ans := baseGrantAnswer{Schema: "wsl-toolkit-base-grant/1", Action: sub, File: cfg.Path(), Mount: change.Mount, Unchanged: change.Unchanged}
	if change.Unchanged {
		logf("  %s is already granted from %s, %s. Nothing to do", change.Mount.Target, change.Mount.Source, change.Mount.Mode)
		return renderGrant(ans, cfg, *asJSON)
	}
	if c, err := useHelper(ctx, *viaHelper); err != nil {
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
	return renderGrant(ans, change.Next, *asJSON)
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
