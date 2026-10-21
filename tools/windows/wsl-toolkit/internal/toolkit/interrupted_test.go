// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// writeTinyArchive is a build's rootfs step with no host engine: a file the
// import is handed and that nothing reads.
func writeTinyArchive(_ context.Context, tarPath string) error {
	return os.WriteFile(tarPath, []byte("rootfs"), 0o600)
}

// buildRecords is every ledger line about this base's build, as event and note.
func buildRecords(t *testing.T, home, name string) []string {
	t.Helper()
	all, err := ledgerAt(home).All()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range all {
		if e.Kind == baseBuildKind && strings.EqualFold(e.ID, name) {
			out = append(out, e.Event)
		}
	}
	return out
}

// TestAFailedCreateIsRolledBackWithoutItsMarker drives a fresh build through the
// fake WSL whose import registers the distribution and then fails. ⛔ The build
// never stamped its marker, so the rollback must remove what it registered
// without asking for one. Finding 5, WSL-150.
func TestAFailedCreateIsRolledBackWithoutItsMarker(t *testing.T) {
	f := newBuildFakeWsl(t)
	f.update(func(s *buildFakeState) { s.FailImport = true })
	var logged []string
	b := f.base(buildFakeConfig("wsl-toolkit-roll", AutomountReadOnly), func(s string) { logged = append(logged, s) })
	b.archive = writeTinyArchive
	_, err := b.EnsureWith(t.Context(), false, false)
	if err == nil || !strings.Contains(err.Error(), "wsl --import failed") {
		t.Fatalf("a failed import answered %v", err)
	}
	st := f.state()
	if len(st.Distros) != 0 {
		t.Fatalf("the incomplete distribution is still registered; calls %v", st.scripts())
	}
	if !strings.Contains(strings.Join(logged, "\n"), "removed the incomplete distribution") {
		t.Errorf("the rollback did not say it removed the distribution: %q", logged)
	}
	// ⭐ THE OWNERSHIP RECORD WAS IN THE LEDGER WHEN THE IMPORT RAN, and the
	// rollback closed it.
	for _, c := range st.Calls {
		if len(c.Args) > 0 && c.Args[0] == "--import" && !c.LedgerOpenBase {
			t.Error("the import ran before the ledger recorded the build, so a build killed there could not be found again")
		}
	}
	if got := strings.Join(buildRecords(t, f.home(), "wsl-toolkit-roll"), ","); got != "open,close" {
		t.Errorf("the build's ledger records read %q, want open,close", got)
	}
}

// stageInterruptedBuild leaves the fake as a build killed between its import and
// its marker leaves a machine: registered, unstamped, and an open record.
func stageInterruptedBuild(t *testing.T, f *buildFakeWsl, b *Base) {
	t.Helper()
	name := b.cfg.Base.Name
	f.update(func(s *buildFakeState) { s.Distros[strings.ToLower(name)] = &buildFakeDistro{Name: name} })
	if err := os.MkdirAll(b.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := b.recordBuildStart(); err != nil {
		t.Fatal(err)
	}
}

// TestABuildKilledBeforeItsMarkerCanBeRemovedLater holds the later process's
// half: `base remove` removes the build on the ledger's record and WSL's
// registration of its disk, and refuses a distribution either fact does not
// cover, naming what can remove it. ⛔ The advice named `base recreate`, which
// refused it too. Finding 5, WSL-150.
func TestABuildKilledBeforeItsMarkerCanBeRemovedLater(t *testing.T) {
	f := newBuildFakeWsl(t)
	b := f.base(buildFakeConfig("wsl-toolkit-killed", AutomountReadOnly), nil)
	stageInterruptedBuild(t, f, b)
	b.disks = func() (map[string]string, error) { return map[string]string{"wsl-toolkit-killed": b.Dir()}, nil }

	st, err := b.Status(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(st.Problems, "\n"), "stopped before its identity marker was written") {
		t.Errorf("status did not name the interrupted build: %q", st.Problems)
	}

	if err := b.Remove(t.Context()); err != nil {
		t.Fatalf("the interrupted build was refused: %v", err)
	}
	if len(f.state().Distros) != 0 {
		t.Fatal("the interrupted build is still registered")
	}
	if got := strings.Join(buildRecords(t, f.home(), "wsl-toolkit-killed"), ","); got != "open,close" {
		t.Errorf("the build's ledger records read %q, want open,close", got)
	}

	// ⛔ THE DISK ELSEWHERE IS NOT THIS BUILD, record or none.
	g := newBuildFakeWsl(t)
	other := g.base(buildFakeConfig("wsl-toolkit-killed", AutomountReadOnly), nil)
	stageInterruptedBuild(t, g, other)
	other.disks = func() (map[string]string, error) {
		return map[string]string{"wsl-toolkit-killed": `D:\somebody\else`}, nil
	}
	err = other.Remove(t.Context())
	if err == nil || !errors.Is(err, ErrNoIdentity) {
		t.Fatalf("a distribution whose disk is elsewhere was not refused: %v", err)
	}
	if strings.Contains(err.Error(), "base recreate") || !strings.Contains(err.Error(), "wsl --unregister wsl-toolkit-killed") {
		t.Errorf("the refusal named the wrong way through: %v", err)
	}
	if len(g.state().Distros) != 1 {
		t.Fatal("a distribution nothing proved was removed")
	}
}

// TestEnsureRemovesAnInterruptedBuildAndBuildsAgain holds `base ensure`'s half:
// the build its own rollback would have removed is removed, and the build runs
// again. WSL-150.
func TestEnsureRemovesAnInterruptedBuildAndBuildsAgain(t *testing.T) {
	f := newBuildFakeWsl(t)
	var logged []string
	b := f.base(buildFakeConfig("wsl-toolkit-again", AutomountReadOnly), func(s string) { logged = append(logged, s) })
	stageInterruptedBuild(t, f, b)
	b.disks = func() (map[string]string, error) { return map[string]string{"wsl-toolkit-again": b.Dir()}, nil }
	stop := errors.New("no engine in this case")
	b.archive = func(context.Context, string) error { return stop }
	_, err := b.EnsureWith(t.Context(), false, false)
	if !errors.Is(err, stop) {
		t.Fatalf("ensure did not go on to build again: %v", err)
	}
	if len(f.state().Distros) != 0 {
		t.Fatal("the interrupted build is still registered")
	}
	if !strings.Contains(strings.Join(logged, "\n"), "Removing it and building again") {
		t.Errorf("ensure did not say what it removed: %q", logged)
	}
}

// TestAnUnmarkedDistributionWithNoRecordNamesTheCommandThatRemovesIt holds the
// advice ensure gives. ⛔ It named `base recreate`, which demands the marker and
// refused. WSL-150.
func TestAnUnmarkedDistributionWithNoRecordNamesTheCommandThatRemovesIt(t *testing.T) {
	f := newBuildFakeWsl(t)
	b := f.base(buildFakeConfig("wsl-toolkit-stray", AutomountReadOnly), nil)
	f.update(func(s *buildFakeState) { s.Distros["wsl-toolkit-stray"] = &buildFakeDistro{Name: "wsl-toolkit-stray"} })
	_, err := b.EnsureWith(t.Context(), false, false)
	if err == nil {
		t.Fatal("an unmarked distribution with no record was adopted")
	}
	if strings.Contains(err.Error(), "base recreate") || !strings.Contains(err.Error(), "wsl --unregister wsl-toolkit-stray") {
		t.Errorf("the refusal named the wrong way through: %v", err)
	}
	if len(f.state().Distros) != 1 {
		t.Fatal("an unmarked distribution nothing proved was removed")
	}
}
