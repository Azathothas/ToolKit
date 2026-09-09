package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// applyPreset resolves --preset onto the configuration for this invocation, and
// records it when the caller asked for it to stick.
//
// ⭐ TWO SEPARATE ACTS, AND KEEPING THEM APART IS THE POINT. `--preset debian`
// builds a Debian base now; `--preset debian --save` also makes it what every
// later command means by the base. A flag that silently rewrote the stored
// configuration would make one experiment change every run afterwards.
//
// ⚠ Switching preset on a base that is already registered REBUILDS it, because
// a distribution's rootfs cannot be changed underneath it. The caller is told
// what is about to happen rather than finding out from a long build.
func applyPreset(ctx context.Context, cfg *toolkit.Config, preset string, save bool) (bool, error) {
	if preset == "" {
		return false, nil
	}
	ref, err := toolkit.ResolveBasePreset(preset)
	if err != nil {
		return false, err
	}
	changed := ref != cfg.Base.Image
	cfg.Base.Image = ref
	if save {
		if err := cfg.Write(); err != nil {
			return changed, err
		}
		logf("  the stored configuration now builds the base from %s", ref)
	}
	if !changed {
		logf("  the base is already built from %s", ref)
		return false, nil
	}
	logf("  base image: %s", ref)
	return true, nil
}

func cmdPresets(args []string) (int, error) {
	fs := newFlagSet("base presets")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	if *asJSON {
		return exitOK, writeJSON(map[string]any{
			"schema":  "wsl-toolkit-presets/1",
			"presets": toolkit.BasePresets,
			"current": cfg.Base.Image,
			"default": toolkit.DefaultBaseImage,
		})
	}
	for _, p := range toolkit.BasePresets {
		marker := " "
		if p.Ref == cfg.Base.Image {
			marker = "*"
		}
		fmt.Printf("%s %-8s %-6s %s\n", marker, p.ID, p.Libc, p.Ref)
		fmt.Fprintf(os.Stderr, "      %s\n", p.Note)
		fmt.Fprintf(os.Stderr, "      measured here: %s\n", p.Build)
	}
	if toolkit.PresetForRef(cfg.Base.Image) == "" {
		fmt.Printf("* %-8s %-6s %s\n", "custom", "-", cfg.Base.Image)
	}
	fmt.Fprintf(os.Stderr, "\n  * is what the base is built from now.\n")
	fmt.Fprintf(os.Stderr, "  Switch with: wsl-toolkit base ensure --preset ID [--save]\n")
	fmt.Fprintf(os.Stderr, "  A fully qualified reference works anywhere a preset id does.\n")
	return exitOK, nil
}
