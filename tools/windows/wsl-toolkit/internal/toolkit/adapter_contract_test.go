// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheAdapterContractsProbeLineIsTheOneTheAgentProbesRun is WSL-145. The
// contract told an adapter author to probe with `as_account sh -lc`, which finds
// npm's own binary first and passes with no wrapper, while the three agent
// probes ran `runuser -l`. Each agent adapter's probe carries the contract's
// line, with its own name in it, so the two cannot drift apart again.
func TestTheAdapterContractsProbeLineIsTheOneTheAgentProbesRun(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "adapters", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	line := ""
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "pane_path=") {
			line = l
			break
		}
	}
	if !strings.Contains(line, "command -v NAME") {
		t.Fatalf("the contract carries no probe line that names NAME, so this case asserts nothing: %q", line)
	}
	agents := 0
	for _, spec := range adapterSpecs {
		if spec.Agent == "" {
			continue
		}
		agents++
		probe, err := adapterScript(spec.Name, "probe.sh")
		if err != nil {
			t.Fatal(err)
		}
		if want := strings.ReplaceAll(line, "NAME", spec.Agent); !strings.Contains(string(probe), want) {
			t.Errorf("the %s probe does not run the contract's line\n  %s\nso an author who copies the contract asks a different question from the probes", spec.Name, want)
		}
	}
	if agents == 0 {
		t.Fatal("no adapter names an Agent, so this case asserts nothing")
	}
}
