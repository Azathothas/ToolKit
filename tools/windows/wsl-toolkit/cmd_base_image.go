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

const baseImageUsage = `wsl-toolkit base image load FILE|IMAGE

  Load an image into the base's engine, as its account, or into root's
  engine with --root. No registry is asked.

  FILE is an image archive on this machine, as podman save or docker save
  writes one. IMAGE is a fully qualified reference this machine's own engine
  holds: it is saved to an archive here, loaded, and the archive removed. A
  name that is a file on this machine is read as FILE.

  --root   load into root's engine instead of the account's
  --json   write a structured answer
`

// cmdBaseImage is `base image load`. WSL-130.
func cmdBaseImage(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("base image")
	asRoot := fs.Bool("root", false, "load into root's engine instead of the account's")
	asJSON := fs.Bool("json", false, "write a structured answer")
	verb := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		verb, args = args[0], args[1:]
	}
	source, err := parseOneName(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, baseImageUsage)
			return exitOK, nil
		}
		return exitCannot, err
	}
	if verb != "load" {
		fmt.Fprint(os.Stderr, baseImageUsage)
		return exitCannot, fmt.Errorf("base image takes load, then a file or an image: %s", toolkit.ToolCommand("base image load FILE|IMAGE"))
	}
	if source == "" {
		return exitCannot, fmt.Errorf("base image load takes a file or an image: %s", toolkit.ToolCommand("base image load FILE|IMAGE"))
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	user := cfg.Base.User
	if *asRoot {
		user = "root"
	}
	base, err := toolkit.NewBase(cfg, note)
	if err != nil {
		return exitCannot, err
	}
	w, err := toolkit.FindWsl()
	if err != nil {
		return exitCannot, err
	}
	if exists, err := w.Exists(ctx, cfg.Base.Name); err != nil {
		return exitCannot, err
	} else if !exists {
		return exitCannot, errNotRegistered(cfg.Base.Name)
	}
	rep := toolkit.ImageLoad{Schema: toolkit.ImageLoadSchema, Source: source, Distro: cfg.Base.Name, User: user}
	archive, kind, err := imageSource(source)
	if err != nil {
		return exitCannot, err
	}
	rep.Kind = kind
	if kind == toolkit.ImageSourceHost {
		note("saving " + source + " from this machine's engine")
		saved, engine, cleanup, err := toolkit.SaveHostImage(ctx, source)
		rep.HostEngine = engine
		if err != nil {
			return exitFailed, err
		}
		defer cleanup()
		archive = saved
	}
	note("loading " + source + " into " + cfg.Base.Name + "'s engine as " + user)
	images, sent, err := base.LoadImageArchive(ctx, archive, user)
	rep.Bytes, rep.Images = sent, images
	if err != nil {
		return exitFailed, err
	}
	if *asJSON {
		return exitOK, writeJSON(rep)
	}
	for _, img := range images {
		fmt.Println(img)
	}
	logf("  loaded %d image(s), %s, into %s's engine as %s", len(images), toolkit.HumanBytes(sent), cfg.Base.Name, user)
	return exitOK, nil
}

// imageSource reads what base image load was given: a file on this machine, or
// a reference this machine's engine is asked for.
//
// ⛔ A FILE WINS, because a path is what a caller typed when there is one, and
// a reference is never a file. A name that is neither is refused with both
// readings named, rather than guessed at.
func imageSource(source string) (path, kind string, err error) {
	p := msysHostPath(source)
	if info, statErr := os.Stat(p); statErr == nil {
		if !info.Mode().IsRegular() {
			return "", "", fmt.Errorf("%s is not a file. Pass an image archive, or a fully qualified image reference", source)
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", "", err
		}
		return abs, toolkit.ImageSourceFile, nil
	}
	if err := toolkit.ValidateImageRef(source); err != nil {
		return "", "", fmt.Errorf("%q is not a file on this machine, and not an image reference this machine's engine can be asked for: %w", source, err)
	}
	return "", toolkit.ImageSourceHost, nil
}
