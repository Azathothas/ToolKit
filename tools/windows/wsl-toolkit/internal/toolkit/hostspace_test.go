// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-180: the host volume the dynamic disk grows into is watched ---------

// hostSpaceAdvisory is the host-space-low finding in a state, if there is one.
func hostSpaceAdvisory(st BaseState) (Remediation, bool) {
	for _, r := range st.Remediations {
		if r.ID == "host-space-low" {
			return r, true
		}
	}
	return Remediation{}, false
}

// ⛔ NOTHING WATCHED IT. FreeSpace was read only before an import, so a volume
// that filled under a working base gave the guest I/O errors and an ext4
// read-only remount, with no word before. `base status`, and `ready` through it,
// carry the free space and an advisory under the floor, and a job says so as it
// starts.
func TestTheVolumeTheDiskGrowsIntoIsWatched(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)

	roomyVolume(t, 1<<30)
	st, err := b.Status(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if st.HostFree == nil || *st.HostFree != 1<<30 {
		t.Fatalf("base status carries %v as the host's free space, want 1 GiB", st.HostFree)
	}
	low, ok := hostSpaceAdvisory(st)
	if !ok {
		t.Fatalf("a volume with 1 GiB free under a %s floor raised no advisory: %+v", HumanBytes(BaseSpaceFloor), st.Remediations)
	}
	if !low.Advisory || low.Repairable || !strings.Contains(low.What, disk) || !strings.Contains(low.What, "1.0 GiB free") ||
		!strings.Contains(low.Costs, "read only") || low.Command != ToolCommand("gc") {
		t.Errorf("the advisory reads %+v", low)
	}
	if len(st.Problems) != 0 {
		t.Errorf("a low volume became a problem, which would turn ready's verdict: %v", st.Problems)
	}

	roomyVolume(t, BaseSpaceFloor)
	st, err = b.Status(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := hostSpaceAdvisory(st); ok || st.HostFree == nil {
		t.Errorf("a volume at the floor answered %v free and %+v", st.HostFree, st.Remediations)
	}

	// ⭐ A JOB SAYS SO AS IT STARTS, and runs on: it may fit.
	run := func() []string {
		t.Helper()
		var said sayings
		r := &Runner{cfg: b.cfg, base: b, wsl: &Wsl{Path: filepath.Join(t.TempDir(), "no-wsl.exe")}, home: b.home, log: said.log}
		res := r.Run(context.Background(), JobSpec{
			Image: "docker.io/library/alpine:latest", ContainerLifecycle: ContainerEphemeral, Platform: "linux/amd64",
		})
		if !res.Unreached {
			t.Fatalf("a runner with no wsl.exe reached the job: %+v", res)
		}
		if strings.Contains(res.Error, "volume") {
			t.Fatalf("a low volume refused the job: %s", res.Error)
		}
		return said.lines
	}
	roomyVolume(t, 1<<30)
	if lines := run(); !(&sayings{lines: lines}).said("⚠ the volume holding " + disk + " has 1.0 GiB free") {
		t.Errorf("a job starting on a low volume did not say so: %v", lines)
	}
	roomyVolume(t, BaseSpaceFloor)
	if lines := run(); (&sayings{lines: lines}).said("the volume holding") {
		t.Errorf("a job starting at the floor warned anyway: %v", lines)
	}
}

// ⚠ The floor before the engine counts the disk a rebuild removes, as the
// import's own check does, and a volume it cannot read refuses nothing.
func TestTheFloorBeforeTheEngineCountsTheOldDisk(t *testing.T) {
	dir := t.TempDir()
	roomyVolume(t, 95)
	if err := assertSpaceFloor(dir, 100, 5); err != nil {
		t.Errorf("a volume at the floor once the old disk is removed was refused: %v", err)
	}
	if err := assertSpaceFloor(dir, 100, 4); err == nil || !strings.Contains(err.Error(), "once the old disk is removed") {
		t.Errorf("a volume under the floor even then answered %v", err)
	}
	if err := assertSpaceFloor(dir, 100, 0); err == nil || !strings.Contains(err.Error(), "Nothing was pulled") {
		t.Errorf("a new build under the floor answered %v", err)
	}
	was := freeSpace
	freeSpace = func(string) (int64, bool, error) { return 0, false, nil }
	t.Cleanup(func() { freeSpace = was })
	if err := assertSpaceFloor(dir, 100, 0); err != nil {
		t.Errorf("a volume that could not be read was refused: %v", err)
	}
}

// ⛔ THE EXPORT WROTE BEFORE ANY SPACE CHECK, so a volume that could never hold
// the import took the pull and the archive first. Under the floor, a forced
// rebuild, a new build and `distro new --image` refuse before the engine is
// asked, and the rebuild keeps the base.
func TestABuildUnderTheSpaceFloorAsksNoEngine(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	h.engine(t, true)
	roomyVolume(t, 1)
	noEngine := func(what string) {
		t.Helper()
		if i := callAt(h.calls(t), "podman"); i >= 0 {
			t.Fatalf("%s under the floor asked the engine: %v", what, h.calls(t))
		}
	}

	_, err := b.EnsureWith(context.Background(), true, false)
	if err == nil || !strings.Contains(err.Error(), fakeBaseName+" was NOT removed") || !strings.Contains(err.Error(), "before anything is pulled") {
		t.Fatalf("a forced rebuild under the floor answered %v", err)
	}
	assertKept(t, h, b, disk)
	noEngine("a forced rebuild")
	if s.said("asking the host engine") {
		t.Fatalf("the rebuild said it asked the engine: %v", s.lines)
	}

	// The same base, gone: a new build.
	h.say(t, "registered", "")
	h.say(t, "disks", "")
	if err := os.RemoveAll(b.Dir()); err != nil {
		t.Fatal(err)
	}
	_, err = b.EnsureWith(context.Background(), false, false)
	if err == nil || !strings.Contains(err.Error(), "NOT ENOUGH DISK SPACE") || !strings.Contains(err.Error(), "before anything is pulled") {
		t.Fatalf("a new build under the floor answered %v", err)
	}
	noEngine("a new build")

	tw := &Throwaways{dir: t.TempDir(), wsl: &Wsl{Path: h.wsl}, log: func(string) {}, disks: h.registry}
	if _, err := tw.Preflight(context.Background(), ThrowawaySpec{Image: "docker.io/library/alpine:latest"}); err == nil ||
		!strings.Contains(err.Error(), "before anything is pulled") {
		t.Fatalf("distro new --image under the floor answered %v", err)
	}
	noEngine("distro new --image")
	// ⚠ An archive already on disk is measured by the import's own check, so a
	// --tarball is not refused here.
	if _, err := tw.Preflight(context.Background(), ThrowawaySpec{}); err != nil {
		t.Errorf("a preflight with no image was refused over the space: %v", err)
	}
}

// ⛔ A JOB THE HELPER STARTS UNDER THE FLOOR TELLS ITS CALLER. The line went to
// the helper's own log, which is its stderr, and a detached helper discards
// that. So `run` and `matrix` through the helper said nothing on a low volume.
// It now crosses the stream as a log event, which the client prints.
func TestAJobTheHelperStartsUnderTheFloorTellsItsCaller(t *testing.T) {
	h := newFakeHost(t)
	b, _, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	roomyVolume(t, 1<<30)
	var helperSaid sayings
	srv := &HelperServer{cfg: b.cfg, log: helperSaid.log, runners: map[string]*Runner{},
		runner: &Runner{cfg: b.cfg, base: b, wsl: &Wsl{Path: filepath.Join(t.TempDir(), "no-wsl.exe")}, home: b.home, log: helperSaid.log}}
	script := base64.StdEncoding.EncodeToString([]byte("true\n"))
	for _, c := range []struct {
		route, body string
		serve       func(http.ResponseWriter, *http.Request)
	}{
		{"run", `{"image":"docker.io/library/alpine:latest","script_b64":"` + script + `","network":false,"platform":"` + NativePlatform() + `"}`, srv.handleRun},
		{"matrix", `{"images":["alpine"],"script_b64":"` + script + `","network":false,"platform":"` + NativePlatform() + `"}`, srv.handleMatrix},
	} {
		rec := httptest.NewRecorder()
		c.serve(rec, httptest.NewRequest(http.MethodPost, "/v1/"+c.route, strings.NewReader(c.body)))
		var notes []string
		if err := ReadHelperEvents(rec.Body, func(ev HelperEvent) error {
			if ev.Kind == "log" {
				notes = append(notes, ev.Text)
			}
			return nil
		}); err != nil {
			t.Fatalf("%s: the helper's answer did not read: %v", c.route, err)
		}
		if !(&sayings{lines: notes}).said("⚠ the volume holding " + disk + " has 1.0 GiB free") {
			t.Errorf("%s through the helper under the floor sent its caller %v", c.route, notes)
		}
	}
	if helperSaid.said("the volume holding") {
		t.Errorf("the warning went to the helper's own log, which a detached helper discards: %v", helperSaid.lines)
	}
}

// ⛔ AN IMAGE SAVED FROM THE ENGINE IS HELD TO A FLOOR TOO. `base image load
// IMAGE` wrote the engine's `save` archive into the state directory with no
// check, as a build's export did, and asked the engine first.
func TestAnImageSavedFromTheEngineIsRefusedUnderTheFloor(t *testing.T) {
	h := newFakeHost(t)
	h.engine(t, true)
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	roomyVolume(t, ThrowawaySpaceFloor-1)
	_, _, _, err := SaveHostImage(context.Background(), "docker.io/library/alpine:latest")
	if err == nil || !strings.Contains(err.Error(), "NOT ENOUGH DISK SPACE") || !strings.Contains(err.Error(), "Nothing was saved") {
		t.Fatalf("a save under the floor answered %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(h.state, "calls")); err == nil && strings.Contains(string(data), "podman") {
		t.Fatalf("a save under the floor asked the engine: %s", data)
	}
}
