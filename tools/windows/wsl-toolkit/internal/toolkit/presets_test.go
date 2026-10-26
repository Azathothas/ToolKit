// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"strings"
	"testing"
)

// ⛔ WSL-188: EVERY PRESET'S FIGURE NAMES THE HOST IT WAS TAKEN ON. The figures
// came from a `.wslconfig` with a custom kernel, cgroup v1 off, an idle timeout
// and DNS tunneling, and read as a stock host's. Each one says so.
func TestEveryPresetFigureNamesTheHostItWasTakenOn(t *testing.T) {
	for _, p := range BasePresets {
		for _, want := range []string{".wslconfig", "7.2.0 kernel", "cgroup_no_v1=all", "vmIdleTimeout", "dnsTunneling"} {
			if !strings.Contains(p.Build, want) {
				t.Errorf("preset %s's figure does not name %q of the host it was taken on: %s", p.ID, want, p.Build)
			}
		}
	}
}
