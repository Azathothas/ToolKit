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
	// ⛔ COMPARED WITH WHAT THE BASE WAS BUILT FROM, NOT WITH THE CONFIGURATION.
	// The two differ after a `--preset` without `--save`, or after a saved switch
	// whose rebuild was refused, and the configuration then decided wrongly both
	// ways: a preset the base already had rebuilt it, and the one it lacked did
	// not. The record answers where there is one. WSL-169.
	built := cfg.Base.Image
	if recorded := toolkit.RecordedBaseImage(*cfg); recorded != "" {
		built = recorded
	}
	changed := ref != built
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

// presetSwitchFailed is the error a rebuild that a preset switch asked for
// returns, with what the switch has already written.
//
// ⛔ --save WRITES THE IMAGE BEFORE THE REBUILD RUNS. A rebuild that is refused
// keeps the old base, as it must, but the stored configuration already names
// the new image, and a reader of the configuration would believe it switched.
// So the caller is told what the file says and that the base was not switched.
// WSL-169.
//
// ⚠ RUNNING THE SAME COMMAND AGAIN SWITCHES IT NOW. The preset is compared with
// the image the base was built from, not with the configuration, so this no
// longer says that only `base recreate` can.
func presetSwitchFailed(err error, switched, saved bool, cfg toolkit.Config) error {
	if err == nil || !switched || !saved {
		return err
	}
	return fmt.Errorf("%w. The stored configuration already builds %s from %s, and the base was not switched. Once that is fixed, run this command again or `%s`",
		err, cfg.Base.Name, cfg.Base.Image, toolkit.ToolCommand("base recreate"))
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
	fmt.Fprintf(os.Stderr, "  Switch with: %s\n", toolkit.ToolCommand("base ensure --preset ID [--save]"))
	fmt.Fprintf(os.Stderr, "  A fully qualified reference works anywhere a preset id does.\n")
	return exitOK, nil
}
