// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// prefixCase is a fresh state root with no instance selected and the literal
// option off.
func prefixCase(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	t.Setenv(InstanceEnv, "")
	t.Setenv(InstanceLiteralEnv, "")
	registered := distroRegistered
	t.Cleanup(func() { distroRegistered = registered })
	distroRegistered = func(context.Context, string) bool { return false }
	return home
}

// ⭐ `--instance wsl-errand-box` MAKES `errand-box`, and says so. WSL-193.
func TestANewPrefixedInstanceDropsThePrefix(t *testing.T) {
	prefixCase(t)
	for _, asked := range []string{"wsl-errand-box", "wsl-toolkit-errand-box"} {
		inst, err := ResolveInstance(context.Background(), asked)
		if err != nil {
			t.Fatalf("%s: %v", asked, err)
		}
		if inst.Name != "errand-box" || inst.Distro != "wsl-toolkit-errand-box" {
			t.Fatalf("%s resolved to instance %q, distribution %q; want errand-box, wsl-toolkit-errand-box", asked, inst.Name, inst.Distro)
		}
		warning := inst.PrefixWarning()
		for _, want := range []string{`"` + asked + `"`, `"errand-box"`, LiteralInstanceFlag} {
			if !strings.Contains(warning, want) {
				t.Fatalf("%s warned %q, which does not name %s", asked, warning, want)
			}
		}
	}
}

// ⚠ The warning is said when the instance is made, and not after.
func TestTheSamePrefixedNameReachesTheSameInstanceQuietly(t *testing.T) {
	prefixCase(t)
	first, err := ResolveInstance(context.Background(), "wsl-errand-box")
	if err != nil || first.PrefixWarning() == "" {
		t.Fatalf("the first call reached %+v, %v, and should have warned", first, err)
	}
	if err := first.RecordPrefix(); err != nil {
		t.Fatal(err)
	}
	inst, err := ResolveInstance(context.Background(), "wsl-errand-box")
	if err != nil || inst.Name != "errand-box" {
		t.Fatalf("the same name again reached %+v, %v", inst, err)
	}
	if w := inst.PrefixWarning(); w != "" {
		t.Fatalf("an instance that recorded the name was warned about again: %q", w)
	}
}

// ⛔ A prefixed name reaching an instance made under another name is said, once.
func TestAPrefixedNameReachingAnUnrecordedInstanceWarns(t *testing.T) {
	home := prefixCase(t)
	if err := os.MkdirAll(filepath.Join(home, "instances", "box"), 0o700); err != nil {
		t.Fatal(err)
	}
	inst, err := ResolveInstance(context.Background(), "wsl-box")
	if err != nil || inst.Name != "box" {
		t.Fatalf("wsl-box reached %+v, %v", inst, err)
	}
	if w := inst.PrefixWarning(); !strings.Contains(w, "reaches the existing instance") {
		t.Fatalf("wsl-box reached the existing box and warned %q", w)
	}
}

// ⛔ A prefixed instance whose distribution is registered keeps its name, even
// where this home holds no state directory for it.
func TestARegisteredPrefixedDistributionKeepsItsName(t *testing.T) {
	prefixCase(t)
	distroRegistered = func(_ context.Context, distro string) bool { return distro == "wsl-toolkit-wsl-box" }
	inst, err := ResolveInstance(context.Background(), "wsl-box")
	if err != nil || inst.Name != "wsl-box" || inst.Distro != "wsl-toolkit-wsl-box" {
		t.Fatalf("a registered prefixed instance resolved to %+v, %v", inst, err)
	}
}

// ❌ `wsl-auto` would be the allocation keyword, which a child resolves again.
func TestAPrefixedNameThatBecomesAutoIsRefused(t *testing.T) {
	prefixCase(t)
	for _, asked := range []string{"wsl-auto", "wsl-toolkit-auto"} {
		_, err := ResolveInstance(context.Background(), asked)
		if err == nil || !strings.Contains(err.Error(), "allocates") {
			t.Fatalf("%q answered %v; want a refusal naming the keyword", asked, err)
		}
	}
}

// ⛔ A printed command for a prefixed instance keeps the name literal, so it
// reaches that instance before its state directory exists.
func TestAPrintedCommandKeepsAPrefixedInstanceLiteral(t *testing.T) {
	prefixCase(t)
	if got := ToolCommandFor("wsl-box", "base ensure"); got != "wsl-toolkit --literal-instance --instance wsl-box base ensure" {
		t.Fatalf("ToolCommandFor printed %q", got)
	}
	if got := ToolCommandFor("box", "base ensure"); got != "wsl-toolkit --instance box base ensure" {
		t.Fatalf("ToolCommandFor printed %q", got)
	}
	previous := SelectedInstance
	t.Cleanup(func() { SelectedInstance = previous })
	SelectedInstance = Instance{Name: "other"}
	err := instanceMatches("wsl-toolkit-wsl-box", "config.json")
	if err == nil || !strings.Contains(err.Error(), "pass --literal-instance --instance wsl-box") {
		t.Fatalf("a configuration naming wsl-toolkit-wsl-box advised %v", err)
	}
}

// ⛔ An instance made under a prefixed name before this change keeps it.
func TestAnExistingPrefixedInstanceKeepsItsName(t *testing.T) {
	home := prefixCase(t)
	if err := os.MkdirAll(filepath.Join(home, "instances", "wsl-errand-box"), 0o700); err != nil {
		t.Fatal(err)
	}
	inst, err := ResolveInstance(context.Background(), "wsl-errand-box")
	if err != nil || inst.Name != "wsl-errand-box" || inst.Distro != "wsl-toolkit-wsl-errand-box" {
		t.Fatalf("an existing prefixed instance resolved to %+v, %v", inst, err)
	}
	if w := inst.PrefixWarning(); w != "" {
		t.Fatalf("an existing instance was warned about: %q", w)
	}
}

// ⭐ WSL_TOOLKIT_INSTANCE_LITERAL=1 keeps the name as given.
func TestTheLiteralOptionKeepsTheNameAsGiven(t *testing.T) {
	prefixCase(t)
	t.Setenv(InstanceLiteralEnv, "1")
	inst, err := ResolveInstance(context.Background(), "wsl-demo2")
	if err != nil || inst.Name != "wsl-demo2" || inst.Distro != "wsl-toolkit-wsl-demo2" {
		t.Fatalf("a literal name resolved to %+v, %v", inst, err)
	}
	if w := inst.PrefixWarning(); w != "" {
		t.Fatalf("a literal name was warned about: %q", w)
	}
}

// ❌ A name that is only a prefix names nothing.
func TestABarePrefixIsRefused(t *testing.T) {
	prefixCase(t)
	for _, asked := range []string{"wsl-", "wsl-toolkit-", "wsl-toolkit"} {
		_, err := ResolveInstance(context.Background(), asked)
		if err == nil || !strings.Contains(err.Error(), "only a prefix") {
			t.Fatalf("%q answered %v; want a refusal saying it is only a prefix", asked, err)
		}
	}
}

// ❌ `wsl-base` would be the reserved `base`, so it is refused, and the refusal
// says what it became. It is refused even where the instance `base` exists.
func TestAPrefixedNameThatBecomesReservedIsRefused(t *testing.T) {
	home := prefixCase(t)
	if err := os.MkdirAll(filepath.Join(home, "instances", "base"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ asked, became string }{{"wsl-base", "base"}, {"wsl-jobs", "jobs"}, {"wsl-toolkit-base", "base"}} {
		_, err := ResolveInstance(context.Background(), c.asked)
		if err == nil || !strings.Contains(err.Error(), "reserved") || !strings.Contains(err.Error(), `becomes "`+c.became+`"`) {
			t.Fatalf("%q answered %v; want a refusal naming the reserved %q", c.asked, err, c.became)
		}
	}
}

// ❌ A remainder that still carries a prefix, or that is not a usable name, is
// refused, saying why.
func TestAPrefixedNameThatStaysUnusableIsRefused(t *testing.T) {
	prefixCase(t)
	if _, err := ResolveInstance(context.Background(), "wsl-wsl-box"); err == nil || !strings.Contains(err.Error(), "still starts with a prefix") {
		t.Fatalf("wsl-wsl-box answered %v", err)
	}
	long := "wsl-" + strings.Repeat("x", 33)
	if _, err := ResolveInstance(context.Background(), long); err == nil || !strings.Contains(err.Error(), "without its prefix") {
		t.Fatalf("%s answered %v; want a refusal naming what it became", long, err)
	}
}
