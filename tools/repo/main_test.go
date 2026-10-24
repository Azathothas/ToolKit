// SPDX-License-Identifier: 0BSD

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeIn(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestReleaseReadbackAnswersByExitCode. ⛔ The workflows read only the exit:
// 0 every staged file is on the release, 1 one is missing, 2 it could not
// read what it was handed. TOOL-32.
func TestReleaseReadbackAnswersByExitCode(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	writeIn(t, dist, "a.exe", "a")
	writeIn(t, dist, "SHA256SUMS", "s")
	full := writeIn(t, root, "full.txt", "a.exe\nSHA256SUMS\n")
	short := writeIn(t, root, "short.txt", "a.exe\n")
	if code := runReleaseReadback([]string{"--dist", dist, "--reported", full}); code != 0 {
		t.Fatalf("a complete release: exit %d", code)
	}
	if code := runReleaseReadback([]string{"--dist", dist, "--reported", short}); code != 1 {
		t.Fatalf("a release short of SHA256SUMS: exit %d, want 1", code)
	}
	if code := runReleaseReadback([]string{"--dist", dist}); code != 2 {
		t.Fatalf("no --reported: exit %d, want 2", code)
	}
	if code := runReleaseReadback([]string{"--dist", filepath.Join(root, "absent"), "--reported", full}); code != 2 {
		t.Fatalf("a dist that is not there: exit %d, want 2", code)
	}
}

// TestNightlyPruneDeletesOnlyWithARepository. ⛔ --delete without --repo would
// hand gh whatever repository its own context names.
func TestNightlyPruneDeletesOnlyWithARepository(t *testing.T) {
	root := t.TempDir()
	list := writeIn(t, root, "releases.json", `[{"tag_name":"herdr-nightly-a","prerelease":true,"published_at":"2026-09-16T07:15:16Z"}]`)
	if code := runNightlyPrune([]string{"--releases", list, "--keep", "1"}); code != 0 {
		t.Fatalf("a listing with nothing to prune: exit %d", code)
	}
	if code := runNightlyPrune([]string{"--releases", list, "--delete"}); code != 2 {
		t.Fatalf("--delete with no --repo: exit %d, want 2", code)
	}
	if code := runNightlyPrune([]string{"--keep", "1"}); code != 2 {
		t.Fatalf("no --releases: exit %d, want 2", code)
	}
	if code := runNightlyPrune([]string{"--releases", list, "--keep", "0"}); code != 1 {
		t.Fatalf("--keep 0: exit %d, want 1", code)
	}
}

// TestNightlyStageAndPublishRefuseAnEmptyName: a nightly with no tag would
// stage a record naming nothing, and publish to no release.
func TestNightlyStageAndPublishRefuseAnEmptyName(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	if code := runNightlyStage([]string{"--dist", dist, "--commit", "c", "--run", "r"}); code != 1 {
		t.Fatalf("nightly-stage with no --tag: exit %d, want 1", code)
	}
	if code := runNightlyPublish([]string{"--dist", dist, "--repo", "o/r", "--target", "t", "--notes-file", "n"}); code != 1 {
		t.Fatalf("nightly-publish with no --tag: exit %d, want 1", code)
	}
}
