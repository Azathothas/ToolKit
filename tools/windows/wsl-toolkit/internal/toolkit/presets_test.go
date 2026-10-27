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

// ⛔ WSL-197: EVERY PRESET ALSO CARRIES A STOCK HOST'S FIGURES. The first set was
// taken under a custom `.wslconfig`, so each preset was built again with none,
// cold, warm and with the developer toolset, and each says what it measured.
func TestEveryPresetCarriesAStockHostFigure(t *testing.T) {
	for _, p := range BasePresets {
		at := strings.Index(p.Build, presetStock)
		if presetStock == "" || at < 0 {
			t.Errorf("preset %s carries no stock host's figure: %s", p.ID, p.Build)
			continue
		}
		stock := p.Build[at+len(presetStock):]
		for _, want := range []string{"s cold", "s warm", "s developer", "MiB disk", "podman "} {
			if !strings.Contains(stock, want) {
				t.Errorf("preset %s's stock figure does not say %q: %s", p.ID, want, stock)
			}
		}
	}
	for _, want := range []string{"stock host", "no .wslconfig", "WSL 3.0.3"} {
		if !strings.Contains(presetStock, want) {
			t.Errorf("the stock figures do not name %q of the host they were taken on: %s", want, presetStock)
		}
	}
}
