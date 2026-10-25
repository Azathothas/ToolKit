// SPDX-License-Identifier: 0BSD

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/repo/internal/mutate"
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

// TestMutateReadsExactLabelsFromAFile: --labels-file and --label add up, and a
// file written on Windows reads the same as one written on Linux.
//
// ⛔ A CALLER'S LIST OF ROWS IS THE WHOLE SELECTION. Dropping the file would run
// the whole table, and keeping its CR would name rows that do not exist.
func TestMutateReadsExactLabelsFromAFile(t *testing.T) {
	list := writeIn(t, t.TempDir(), "labels", "first row\r\nsecond row\n")
	a, err := mutateFlags([]string{"--label", "named row", "--labels-file", list, "--jobs", "3"})
	if err != nil {
		t.Fatal(err)
	}
	sel := a.sel
	if got := strings.Join(sel.Labels, "|"); got != "named row|first row|second row" {
		t.Fatalf("the selection names %q", got)
	}
	if sel.Jobs != 3 {
		t.Fatalf("--jobs 3 read as %d", sel.Jobs)
	}
	empty := writeIn(t, t.TempDir(), "labels", "\n")
	if _, err := mutateFlags([]string{"--labels-file", empty}); err == nil {
		t.Fatal("a labels file with no label was accepted, and it would select every row")
	}
}

// TestMutateRefusesJobsBelowZero. ⚠ Zero means the default, and a number below
// it is a typo that would quietly mean the same.
//
// ⚠ READ THROUGH mutateFlags, not runMutate. runMutate also refuses a
// directory that is not a repository, and a staged copy is not one. An exit of
// 2 there would not say which refusal answered.
func TestMutateRefusesJobsBelowZero(t *testing.T) {
	if _, err := mutateFlags([]string{"--jobs", "-1"}); err == nil || !strings.Contains(err.Error(), "--jobs -1") {
		t.Fatalf("--jobs -1 answered %v", err)
	}
}

// TestMutateWritesItsVerdictsBeforeTheTally. ⛔ CI's keep step reads every
// shard's file. A shard that passed and wrote none leaves every module it
// touched unkept, and every later push runs them again.
func TestMutateWritesItsVerdictsBeforeTheTally(t *testing.T) {
	file := filepath.Join(t.TempDir(), "verdicts.json")
	a, err := mutateFlags([]string{"--verdicts", file, "--shard", "1/2"})
	if err != nil {
		t.Fatal(err)
	}
	row := mutate.Mutation{Label: "a guard", Module: "tools/x"}
	tb := &mutate.Table{Schema: mutate.TableSchema, Mutations: []mutate.Mutation{row}}
	proved := mutate.Verdict{Label: "a guard", State: "ok", Cases: 1}
	var out, errOut bytes.Buffer
	if code := finishMutate(a, tb, []mutate.Verdict{proved}, &out, &errOut); code != 0 {
		t.Fatalf("an ok row: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("no verdict file was written: %v", err)
	}
	if !strings.Contains(string(raw), `"module": "tools/x"`) || !strings.Contains(string(raw), `"shard": "1/2"`) {
		t.Fatalf("the verdict file holds %s", raw)
	}
}

// TestAKeepThatLeftAModuleShortFails. ⛔ Every shard passed, so a module that
// ran and was not kept is a missing file, not a pass.
func TestAKeepThatLeftAModuleShortFails(t *testing.T) {
	keys := []mutate.ModuleKey{{Module: "tools/a", Key: "ka"}, {Module: "tools/b", Key: "kb"}}
	lines, code := keepOutputs(keys, keys[:1], true, 3)
	if code != 1 {
		t.Fatalf("a module left short: exit %d, want 1", code)
	}
	if got := strings.Join(lines, "|"); got != "save1=ka|save2=|save3=" {
		t.Fatalf("the slots to save read %s", got)
	}
	if _, code := keepOutputs(keys, keys, false, 2); code != 0 {
		t.Fatalf("every module kept: exit %d", code)
	}
}
