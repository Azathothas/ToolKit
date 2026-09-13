// SPDX-License-Identifier: 0BSD

package checks

import "testing"

// TestTheBundleRuleComparesRatherThanRebuilds holds the flag that makes the rule
// a check. `build.ps1 -Test` without `-Check` writes both products and compares
// nothing, so a stale part and a hand-edited product each exited 0 under the gate
// with the tree rewritten underneath it. TOOL-23.
func TestTheBundleRuleComparesRatherThanRebuilds(t *testing.T) {
	compares, tests := false, false
	for _, a := range bundleArgs {
		switch a {
		case "-Check":
			compares = true
		case "-Test":
			tests = true
		}
	}
	if !compares {
		t.Fatalf("the bundle rule runs %v, which rebuilds the products instead of comparing them", bundleArgs)
	}
	if !tests {
		t.Fatalf("the bundle rule runs %v, which compares and no longer runs the selftest, the surface lock or the analyzer", bundleArgs)
	}
}
