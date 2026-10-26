// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// -- WSL-188: which host engine builds a base, and where ----------------------

// foreignArch is an architecture this machine does not run.
func foreignArch() string {
	if nativeArch() == "arm64" {
		return "amd64"
	}
	return "arm64"
}

// ⛔ AN ENGINE CAN ANSWER FROM ANOTHER MACHINE. A podman connection or a Docker
// context naming a remote host built that host's rootfs, which imports here and
// then runs nothing. It is passed over, by name and host, and the refusal says
// which command names the connection.
func TestAnEngineThatBuildsForAnotherArchitectureIsPassedOver(t *testing.T) {
	h := newFakeHost(t)
	h.engine(t, true)
	h.say(t, "info-podman", foreignArch()+" linux build-box")
	_, err := FindEngine(context.Background())
	if err == nil {
		t.Fatal("a podman answering for another architecture was chosen")
	}
	for _, want := range []string{`from host "build-box"`, "linux/" + foreignArch(), "builds on another machine", "podman system connection list"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q: %v", want, err)
		}
	}
	h.engine(t, false)
	h.say(t, "engine-docker", "up")
	h.say(t, "info-docker", foreignArch()+" linux remote-context")
	if _, err := FindEngine(context.Background()); err == nil || !strings.Contains(err.Error(), "docker context ls") {
		t.Errorf("a docker answering for another architecture answered %v; want a refusal naming its context", err)
	}
}

// ⭐ AN ENGINE OF THIS ARCHITECTURE IS USED, and the build says which host it
// answers from, so a remote one is visible.
func TestAnEngineSaysTheHostItBuildsOn(t *testing.T) {
	h := newFakeHost(t)
	h.engine(t, true)
	h.say(t, "info-podman", nativeArch()+" linux podman-box")
	e, err := FindEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "podman" || e.Host != "podman-box" || e.Arch != nativeArch() {
		t.Fatalf("the engine found is %+v; want podman for %s on podman-box", e, nativeArch())
	}
	var said sayings
	if err := e.ExportRootfs(context.Background(), pullCaseRef, filepath.Join(t.TempDir(), "rootfs.tar"), said.log); err != nil {
		t.Fatal(err)
	}
	if !said.said(`podman answers from host "podman-box"`) {
		t.Errorf("the build did not say which host builds it: %v", said.lines)
	}
}

// ⛔ DOCKER IN WINDOWS-CONTAINERS MODE ANSWERS, and was chosen, and no Linux image
// can be pulled into it. It is passed over, and the next engine is asked.
func TestDockerInWindowsContainersModeIsPassedOver(t *testing.T) {
	h := newFakeHost(t)
	h.say(t, "engine-docker", "up")
	h.say(t, "info-docker", nativeArch()+" windows docker-desktop")
	h.say(t, "engine-wslc", "up")
	e, err := FindEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "wslc" {
		t.Fatalf("%s was chosen over docker in Windows-containers mode, want wslc", e.Name)
	}
	h.say(t, "engine-wslc", "down")
	if _, err := FindEngine(context.Background()); err == nil || !strings.Contains(err.Error(), "windows containers") || !strings.Contains(err.Error(), "Switch it to Linux containers") {
		t.Errorf("docker in Windows-containers mode answered %v; want a refusal saying to switch", err)
	}
}

// ⛔ wslc's ARCHITECTURE IS THE MACHINE'S. Read from the executable, the amd64
// build on an ARM64 machine named amd64, and refused wslc's own images.
func TestWslcBuildsForTheMachinesArchitecture(t *testing.T) {
	h := newFakeHost(t)
	h.say(t, "engine-wslc", "up")
	answerMachine(t, imageFileMachineARM64, true)
	e, err := FindEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "wslc" || e.Arch != "arm64" {
		t.Fatalf("on an ARM64 machine wslc is %s for %s, want wslc for arm64", e.Name, e.Arch)
	}
}

// ⛔ A HUNG podman COST 90 SECONDS PER LOOKUP before docker was asked. While
// another installed engine is left, the bound is the shorter one.
//
// ⚠ DECIDED BY WHICH ENGINE ANSWERS, NOT BY A CLOCK. The fake podman is silent
// for 15 seconds and then answers. Under the short bound it is given up and
// docker is chosen; under the long one it answers, and podman is.
func TestAHungEngineIsGivenUpSoonerWhileAnotherIsLeft(t *testing.T) {
	h := newFakeHost(t)
	h.engine(t, true)
	h.say(t, "info-silent-podman", "15s")
	h.say(t, "engine-docker", "up")
	// ⚠ wslc IS TAKEN OFF THIS CASE'S PATH, so docker is the last engine and has
	// the long bound however slowly this host starts it.
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	if err := os.Remove(filepath.Join(os.Getenv("PATH"), "wslc"+ext)); err != nil {
		t.Fatal(err)
	}
	long, short := engineProbeTimeout, engineProbeTimeoutBeforeAnother
	t.Cleanup(func() { engineProbeTimeout, engineProbeTimeoutBeforeAnother = long, short })
	engineProbeTimeout, engineProbeTimeoutBeforeAnother = time.Minute, 500*time.Millisecond
	e, err := FindEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "docker" {
		t.Fatalf("%s was chosen; a podman silent past the short bound should have been given up for docker", e.Name)
	}
}

// ⛔ A KILLED EXPORT LEFT ITS CONTAINER on the shared host engine, unmarked. The
// container is marked, and `resources --host-engine` counts the marked ones and
// prints the command that removes them, removing nothing itself.
func TestAnExportContainerIsMarkedAndALeftOneIsCounted(t *testing.T) {
	h := newFakeHost(t)
	h.engine(t, true)
	e, err := FindEngine(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := e.ExportRootfs(context.Background(), pullCaseRef, filepath.Join(t.TempDir(), "rootfs.tar"), func(string) {}); err != nil {
		t.Fatal(err)
	}
	if callAt(h.calls(t), "podman create --platform "+e.Platform()+" --label "+ExportLabel+" ") < 0 {
		t.Errorf("the export container carries no %s: %v", ExportLabel, h.calls(t))
	}
	h.say(t, "ps-podman", "0123456789ab\nba9876543210\n")
	held := ReadHostEngineHolding(context.Background())
	if held.Error != "" {
		t.Fatal(held.Error)
	}
	if held.LeftExports == nil || *held.LeftExports != 2 {
		t.Errorf("two left export containers read as %v", held.LeftExports)
	}
	want := "podman container prune --force --filter label=" + ExportLabel
	found := false
	for _, c := range held.Commands {
		found = found || c == want
	}
	if !found {
		t.Errorf("the commands do not name %q: %v", want, held.Commands)
	}
	for _, call := range h.calls(t) {
		if strings.HasPrefix(call, "podman container prune") || strings.HasPrefix(call, "podman rm") && strings.Contains(call, "0123456789ab") {
			t.Errorf("reading the host engine removed something: %s", call)
		}
	}
}
