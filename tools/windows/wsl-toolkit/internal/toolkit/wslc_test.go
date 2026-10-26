// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// -- WSL-110, issue 35: wslc as a host engine --------------------------------

// ⭐ wslc is tried last, so a machine with podman or docker keeps its engine.
func TestWslcIsTheHostEngineTriedLast(t *testing.T) {
	if !slices.Equal(hostEngineNames, []string{"podman", "docker", "wslc"}) {
		t.Fatalf("the host engines are tried as %v; want podman, docker, then wslc", hostEngineNames)
	}
}

// ⛔ wslc 3.0.3 takes no --platform, so naming one would refuse every pull.
func TestWslcIsAskedInItsOwnSpellingAndTheOthersInTheirs(t *testing.T) {
	ref := "docker.io/library/alpine:latest"
	wslc := &Engine{Name: "wslc", Arch: "amd64"}
	if got := wslc.pullArgs(ref); !slices.Equal(got, []string{"pull", ref}) {
		t.Errorf("wslc pulls with %v", got)
	}
	if got := wslc.createArgs(ref); !slices.Equal(got, []string{"create", ref}) {
		t.Errorf("wslc creates with %v", got)
	}
	podman := &Engine{Name: "podman", Arch: "amd64"}
	if got := podman.pullArgs(ref); !slices.Equal(got, []string{"pull", "--platform", "linux/amd64", ref}) {
		t.Errorf("podman pulls with %v; the platform has to be named on every fetch, WSL-64", got)
	}
	// ⚠ MARKED, so a container a killed export left can be found. WSL-188.
	if got := podman.createArgs(ref); !slices.Equal(got, []string{"create", "--platform", "linux/amd64", "--label", ExportLabel, ref}) {
		t.Errorf("podman creates with %v", got)
	}
}

// ⛔ An engine that cannot name the architecture is read back instead, and a
// foreign one is refused before anything is exported.
func TestAnImageWslcPulledForAnotherArchitectureIsRefused(t *testing.T) {
	ref := "docker.io/library/alpine:latest"
	if err := wslcArchVerdict(ref, wslcImage{Architecture: "amd64", Os: "linux"}, "amd64"); err != nil {
		t.Fatalf("a native image was refused: %v", err)
	}
	err := wslcArchVerdict(ref, wslcImage{Architecture: "arm64", Os: "linux"}, "amd64")
	if err == nil || !strings.Contains(err.Error(), "linux/arm64") || !strings.Contains(err.Error(), "wslc rmi "+ref) {
		t.Fatalf("a foreign image answered %v; want a refusal naming what was pulled and the way past it", err)
	}
	if err := wslcArchVerdict(ref, wslcImage{Architecture: "amd64", Os: "windows"}, "amd64"); err == nil {
		t.Fatal("a Windows image was accepted as a Linux root filesystem")
	}
	if err := (&Engine{Name: "podman"}).checkArch(context.Background(), ref); err != nil {
		t.Fatalf("podman was read back, which it names on the fetch: %v", err)
	}
}
