// SPDX-License-Identifier: 0BSD

package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// WSL-169. `base ensure --preset X --save` writes X before the rebuild runs, so
// a refused rebuild leaves the old base under a configuration that already
// names X. The same command run again finds no switch, so the refusal says
// that `base recreate` is what switches it now.
func TestARefusedPresetSwitchSaysTheConfigurationIsAlreadyWritten(t *testing.T) {
	cfg := toolkit.Config{Base: toolkit.BaseConfig{Name: "wsl-toolkit-wsl169", Image: "docker.io/library/debian:trixie"}}
	refused := errors.New("wsl-toolkit-wsl169 was NOT removed: a rebuild exports its rootfs through a host engine, and no engine answered")

	err := presetSwitchFailed(refused, true, true, cfg)
	if !errors.Is(err, refused) {
		t.Fatalf("the rebuild's own refusal is not carried: %v", err)
	}
	for _, want := range []string{"already builds wsl-toolkit-wsl169 from docker.io/library/debian:trixie", "will not switch it", "base recreate"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("a refused switch under --save does not say %q:\n%v", want, err)
		}
	}

	// ⚠ NOTHING WAS WRITTEN without --save, and nothing was switched without a
	// new preset, so the same command run again still means what it said.
	if got := presetSwitchFailed(refused, true, false, cfg); got != refused {
		t.Errorf("a switch that was not saved changed the refusal: %v", got)
	}
	if got := presetSwitchFailed(refused, false, true, cfg); got != refused {
		t.Errorf("a saved preset that switched nothing changed the refusal: %v", got)
	}
	if got := presetSwitchFailed(nil, true, true, cfg); got != nil {
		t.Errorf("a rebuild that succeeded returned an error: %v", got)
	}
}
