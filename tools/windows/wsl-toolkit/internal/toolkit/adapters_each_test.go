// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"strings"
	"testing"
)

// -- WSL-107, issue 37 item 3: one adapter does not stop the others ----------

// ⛔ base ensure returned at pi's read-back, so omp was never installed and its
// probe then reported five problems for a program that was not there.
func TestEveryAdapterIsTriedWhenOneBeforeItFails(t *testing.T) {
	adapters := []BaseAdapter{{Name: "herdr"}, {Name: "pi"}, {Name: "omp"}}
	var tried []string
	err := applyEach(adapters, func(string) {}, func(a BaseAdapter) error {
		tried = append(tried, a.Name)
		if a.Name == "pi" {
			return errors.New("adapter pi is installed, and reading it back found: a startup model pi does not list")
		}
		return nil
	})
	if strings.Join(tried, " ") != "herdr pi omp" {
		t.Fatalf("the adapters tried were %v; every one after a failure must still be tried", tried)
	}
	if err == nil || !strings.Contains(err.Error(), "1 of 3 adapter(s) did not finish") || !strings.Contains(err.Error(), "adapter pi") {
		t.Fatalf("the answer was %v; want one error naming the adapter that failed", err)
	}
	if err := applyEach(adapters, func(string) {}, func(BaseAdapter) error { return nil }); err != nil {
		t.Fatalf("three adapters that all finished answered %v", err)
	}
}

func TestAnAdapterThatIsNotInstalledIsOneProblemAndNotFive(t *testing.T) {
	st := parseAdapterProbe("omp", "installed no\nproblem omp is not installed in this base. Run: base ensure\n")
	if len(st.Problems) != 1 || !strings.Contains(st.Problems[0], "not installed") {
		t.Fatalf("an adapter that is not installed read as %v", st.Problems)
	}
	if st.Healthy {
		t.Fatal("an adapter that is not installed read as healthy")
	}
	st = parseAdapterProbe("omp", "installed yes\n")
	if len(st.Problems) != 1 || st.Problems[0] != "the probe named no version" {
		t.Fatalf("an installed adapter whose probe named no version read as %v", st.Problems)
	}
}

// TestEachAgentProbeAsksWhetherItIsInstalledFirst holds the order inside the
// probes: the question comes before every check that only means something for an
// installed agent, and a probe asked for its footprint answers it and stops.
func TestEachAgentProbeAsksWhetherItIsInstalledFirst(t *testing.T) {
	for _, name := range []string{"pi", "omp", "muse"} {
		probe, err := adapterScript(name, "probe.sh")
		if err != nil {
			t.Fatal(err)
		}
		s := string(probe)
		asked := strings.Index(s, "printf 'installed %s\\n' \"$installed\"")
		stop := strings.Index(s, "is not installed in this base. Run: base ensure\"\n  exit 0\n")
		login := strings.Index(s, `runuser -l "$TK_USER" -c "command -v `)
		footprint := strings.Index(s, "if [ \"${TK_PROBE_FOOTPRINT:-}\" = 1 ]; then")
		if asked < 0 || stop < 0 || login < 0 || footprint < 0 {
			t.Fatalf("%s: the probe does not carry the installed question, its stop, the footprint branch and the login-shell check (%d %d %d %d)", name, asked, stop, footprint, login)
		}
		if !(asked < footprint && footprint < stop && stop < login) {
			t.Errorf("%s: the probe asks whether it is installed at %d, answers a footprint at %d, stops at %d, and reaches the login shell at %d; the first three must come first", name, asked, footprint, stop, login)
		}
		if name == "omp" {
			if bun := strings.Index(s, "bun_version=$(as_account bun --version"); bun < stop {
				t.Errorf("omp: the bun check at %d comes before the installed stop at %d, so a missing omp reads as a missing bun", bun, stop)
			}
		}
	}
}
