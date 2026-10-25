// proof_test.go - what a proof key names, and what a kept proof answers.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "-c", "core.autocrlf=false"}, args...)...)
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, errBuf.String())
	}
	return strings.TrimSpace(out.String())
}

func put(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// proofRepo is a repository with the harness and two modules, a and b, each
// with two rows, committed.
func proofRepo(t *testing.T) (string, *Table) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on PATH")
	}
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	tb := &Table{Schema: TableSchema, Mutations: []Mutation{
		{Label: "a one", Module: "mods/a", File: "a.go", Find: "x", Replace: "y", Run: "TestA"},
		{Label: "a two", Module: "mods/a", File: "a.go", Find: "x", Replace: "z", Run: "TestA2"},
		{Label: "b one", Module: "mods/b", File: "b.go", Find: "x", Replace: "y", Run: "TestB"},
		{Label: "b two", Module: "mods/b", File: "b.go", Find: "x", Replace: "z", Run: "TestB"},
	}}
	put(t, root, "tools/repo/harness.go", "package harness\n")
	put(t, root, "mods/a/a.go", "package a\n")
	put(t, root, "mods/b/b.go", "package b\n")
	commitTable(t, root, tb)
	return root, tb
}

func commitTable(t *testing.T, root string, tb *Table) {
	t.Helper()
	raw, err := json.Marshal(tb)
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, harnessTable, string(raw))
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "x")
}

func keysOf(t *testing.T, root string, tb *Table, goVersion string) map[string]string {
	t.Helper()
	keys, err := ProofKeys(root, tb, goVersion)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, k := range keys {
		out[k.Module] = k.Key
	}
	if len(out) != 2 {
		t.Fatalf("keys for %d modules, want 2: %v", len(out), out)
	}
	return out
}

// TestAProofKeyMovesWithItsOwnModuleAlone. ⛔ A module's key names its tree
// and its own rows. A change to another module, or to another module's rows,
// leaves it alone, or a row added to one module would re-prove all four.
func TestAProofKeyMovesWithItsOwnModuleAlone(t *testing.T) {
	t.Parallel()
	root, tb := proofRepo(t)
	before := keysOf(t, root, tb, "go1.25.0")

	put(t, root, "mods/a/a.go", "package a\n\nconst changed = true\n")
	gitIn(t, root, "commit", "-q", "-am", "a")
	afterCode := keysOf(t, root, tb, "go1.25.0")
	if afterCode["mods/a"] == before["mods/a"] {
		t.Error("a commit to mods/a left its key alone, so its old proof would answer for new code")
	}
	if afterCode["mods/b"] != before["mods/b"] {
		t.Error("a commit to mods/a moved the key of mods/b")
	}

	tb.Mutations[3].Replace = "w"
	commitTable(t, root, tb)
	afterRow := keysOf(t, root, tb, "go1.25.0")
	if afterRow["mods/b"] == afterCode["mods/b"] {
		t.Error("a changed row of mods/b left its key alone, so the row's new claim would read as proved")
	}
	if afterRow["mods/a"] != afterCode["mods/a"] {
		t.Error("a changed row of mods/b moved the key of mods/a")
	}
}

// TestAProofKeyMovesWithTheHarnessAndTheGoVersion. ⛔ Both decide every
// verdict, so a change to either re-proves every module.
func TestAProofKeyMovesWithTheHarnessAndTheGoVersion(t *testing.T) {
	t.Parallel()
	root, tb := proofRepo(t)
	before := keysOf(t, root, tb, "go1.25.0")
	if again := keysOf(t, root, tb, "go1.25.0"); again["mods/a"] != before["mods/a"] {
		t.Fatal("the same tree gave two keys, so no proof would ever be found")
	}
	if other := keysOf(t, root, tb, "go1.26.0"); other["mods/a"] == before["mods/a"] || other["mods/b"] == before["mods/b"] {
		t.Error("another Go version gave the same keys")
	}
	put(t, root, "tools/repo/harness.go", "package harness\n\nconst judged = true\n")
	gitIn(t, root, "commit", "-q", "-am", "harness")
	after := keysOf(t, root, tb, "go1.25.0")
	if after["mods/a"] == before["mods/a"] || after["mods/b"] == before["mods/b"] {
		t.Error("a change to the harness left a key alone, so a verdict the old harness gave would stand")
	}
}

// TestAProofKeyRefusesUncommittedChanges. ⛔ The key names HEAD and a run
// proves the working tree. A proof of the one kept under the other's name is
// a proof of something nobody committed.
func TestAProofKeyRefusesUncommittedChanges(t *testing.T) {
	t.Parallel()
	root, tb := proofRepo(t)
	put(t, root, "mods/b/new.go", "package b\n")
	if _, err := ProofKeys(root, tb, "go1.25.0"); err == nil || !strings.Contains(err.Error(), "mods/b") {
		t.Fatalf("an untracked file in mods/b answered %v", err)
	}
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "b")
	put(t, root, "tools/repo/harness.go", "package harness\n\nconst edited = true\n")
	if _, err := ProofKeys(root, tb, "go1.25.0"); err == nil || !strings.Contains(err.Error(), "tools/repo") {
		t.Fatalf("an edited harness answered %v", err)
	}
}

// TestMoreModulesThanSlotsIsRefused. ⛔ A module past the last slot would be
// planned and never looked up, so it would run on every push and be kept by
// nobody.
func TestMoreModulesThanSlotsIsRefused(t *testing.T) {
	keys := []ModuleKey{{Module: "a", Key: "ka"}, {Module: "b", Key: "kb"}}
	if _, err := SlotOutputs(keys, 1, func(k ModuleKey) string { return k.Key }, "key"); err == nil || !strings.Contains(err.Error(), "ci.yml") {
		t.Fatalf("two modules in one slot answered %v", err)
	}
	got, err := SlotOutputs(keys, 3, func(k ModuleKey) string { return k.Key }, "key")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "key1=ka|key2=kb|key3=" {
		t.Fatalf("the slots read %q", got)
	}
}

// keep writes a kept proof of k holding verdicts.
func keep(t *testing.T, root string, k ModuleKey, verdicts ...RowVerdict) {
	t.Helper()
	raw, err := json.Marshal(Proof{Schema: ProofSchema, Module: k.Module, Key: k.Key, Verdicts: verdicts})
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, k.Path, string(raw))
}

func planKeys(t *testing.T) (string, *Table, []ModuleKey) {
	t.Helper()
	root, tb := proofRepo(t)
	keys, err := ProofKeys(root, tb, "go1.25.0")
	if err != nil {
		t.Fatal(err)
	}
	return root, tb, keys
}

// TestAKeptProofAnswersItsModule, and its unproved rows are printed again.
//
// ⛔ A SKIPPED ROW IN A KEPT PROOF IS STILL NAMED. A plan that printed only
// the count would turn a row nobody proved into a number.
func TestAKeptProofAnswersItsModule(t *testing.T) {
	t.Parallel()
	root, tb, keys := planKeys(t)
	keep(t, root, keys[0],
		RowVerdict{Label: "a one", Module: "mods/a", State: "ok", Cases: 1},
		RowVerdict{Label: "a two", Module: "mods/a", State: "SKIPPED", Cases: 1, Skipped: 1, Reason: "every case it names skipped itself on this host"})
	var log bytes.Buffer
	p, err := MakePlan(root, tb, keys, false, 12, &log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.Run, " ") != "mods/b" || len(p.Kept) != 1 {
		t.Fatalf("the plan runs %v and keeps %d, want mods/b and 1:\n%s", p.Run, len(p.Kept), log.String())
	}
	if !strings.Contains(log.String(), "SKIPPED: a two") || !strings.Contains(log.String(), keys[0].Key) {
		t.Fatalf("the plan does not name the kept key and the row it holds unproved:\n%s", log.String())
	}
}

// TestAFullRunReadsNoKeptProof. ⚠ The nightly and the release run every row,
// because the key does not name the runner's image.
func TestAFullRunReadsNoKeptProof(t *testing.T) {
	t.Parallel()
	root, tb, keys := planKeys(t)
	keep(t, root, keys[0],
		RowVerdict{Label: "a one", Module: "mods/a", State: "ok"},
		RowVerdict{Label: "a two", Module: "mods/a", State: "ok"})
	p, err := MakePlan(root, tb, keys, true, 12, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Run) != 2 || len(p.Kept) != 0 {
		t.Fatalf("a full run runs %v and keeps %d", p.Run, len(p.Kept))
	}
}

// TestAProofThatIsNotWholeAnswersNothing. ⛔ A proof missing a row, holding a
// failed one or kept under another key is no proof, and its module runs.
func TestAProofThatIsNotWholeAnswersNothing(t *testing.T) {
	t.Parallel()
	// ⚠ BOUND TO VARIABLES, because a slice literal of structs opens with two
	// braces, which the placeholder check reads as an unfilled template.
	one := RowVerdict{Label: "a one", Module: "mods/a", State: "ok"}
	two := RowVerdict{Label: "a two", Module: "mods/a", State: "ok"}
	theatre := two
	theatre.State = "THEATRE"
	cases := map[string]func(k ModuleKey) (ModuleKey, []RowVerdict){
		"a row short": func(k ModuleKey) (ModuleKey, []RowVerdict) {
			return k, []RowVerdict{one}
		},
		"a theatre row": func(k ModuleKey) (ModuleKey, []RowVerdict) {
			return k, []RowVerdict{one, theatre}
		},
		"another key": func(k ModuleKey) (ModuleKey, []RowVerdict) {
			k.Key += "-old"
			return k, []RowVerdict{one, two}
		},
	}
	for name, build := range cases {
		root, tb, keys := planKeys(t)
		written, verdicts := build(keys[0])
		written.Path = keys[0].Path
		keep(t, root, written, verdicts...)
		var log bytes.Buffer
		p, err := MakePlan(root, tb, keys, false, 12, &log)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Kept) != 0 || len(p.Run) != 2 {
			t.Errorf("%s: the plan kept %d and runs %v:\n%s", name, len(p.Kept), p.Run, log.String())
		}
	}
}

// TestThePlanCutsWhatRunsIntoNoMoreShardsThanGroups. ⛔ A shard with no group
// is refused by the harness, so the plan never asks for one. And a plan with
// nothing to run says so, rather than asking for an empty matrix.
func TestThePlanCutsWhatRunsIntoNoMoreShardsThanGroups(t *testing.T) {
	t.Parallel()
	root, tb, keys := planKeys(t)
	p, err := MakePlan(root, tb, keys, false, 12, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	// Three pattern groups: TestA, TestA2 and TestB.
	if got := strings.Join(p.Outputs(), "|"); got != `run=true|modules=mods/a mods/b|shards=["1/3","2/3","3/3"]` {
		t.Fatalf("the plan says %s", got)
	}
	for _, k := range keys {
		var verdicts []RowVerdict
		for _, r := range k.Rows {
			verdicts = append(verdicts, RowVerdict{Label: tb.Mutations[r].Label, Module: k.Module, State: "ok"})
		}
		keep(t, root, k, verdicts...)
	}
	var log bytes.Buffer
	p, err = MakePlan(root, tb, keys, false, 12, &log)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.Outputs(), "|"); got != "run=false|modules=|shards=[]" {
		t.Fatalf("a plan with every proof kept says %s", got)
	}
	if !strings.Contains(log.String(), "no shard runs") {
		t.Fatalf("a plan that runs nothing does not say so:\n%s", log.String())
	}
	if _, err := MakePlan(root, tb, keys, true, 0, &bytes.Buffer{}); err == nil {
		t.Fatal("a plan for no shards was made, and its rows would run nowhere")
	}
}

// writeVerdicts is one shard's verdict file in dir.
func writeVerdicts(t *testing.T, dir, name string, verdicts ...RowVerdict) {
	t.Helper()
	raw, err := json.Marshal(VerdictFile{Schema: VerdictsSchema, Verdicts: verdicts})
	if err != nil {
		t.Fatal(err)
	}
	put(t, dir, name+"/verdicts.json", string(raw))
}

// TestKeepKeepsOnlyAWholeModule. ⛔ A module is proved by every shard
// together, so one whose verdicts are short is not kept, and the step fails.
func TestKeepKeepsOnlyAWholeModule(t *testing.T) {
	t.Parallel()
	root, tb, keys := planKeys(t)
	dir := t.TempDir()
	writeVerdicts(t, dir, "shard-1", RowVerdict{Label: "a one", Module: "mods/a", State: "ok"}, RowVerdict{Label: "b one", Module: "mods/b", State: "ok"})
	writeVerdicts(t, dir, "shard-2", RowVerdict{Label: "a two", Module: "mods/a", State: "SKIPPED"})
	var log bytes.Buffer
	kept, short, err := Keep(root, tb, keys, dir, &log)
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 || kept[0].Module != "mods/a" || !short {
		t.Fatalf("kept %v, short %v; want mods/a alone, and short for mods/b:\n%s", kept, short, log.String())
	}
	if _, why := readProof(root, tb, keys[0]); why != "" {
		t.Fatalf("the kept proof of mods/a does not read back: %s", why)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(keys[1].Path))); err == nil {
		t.Fatal("mods/b was written a proof with a row short")
	}
}

// TestKeepRefusesARowTwoShardsClaim, a row filed under another module, a file
// of another shape, and a directory with no verdict file. ⛔ Each says the
// files are not this run's, and a keep that read them would keep a guess.
func TestKeepRefusesARowTwoShardsClaim(t *testing.T) {
	t.Parallel()
	root, tb, keys := planKeys(t)
	dir := t.TempDir()
	writeVerdicts(t, dir, "shard-1", RowVerdict{Label: "a one", Module: "mods/a", State: "ok"})
	writeVerdicts(t, dir, "shard-2", RowVerdict{Label: "a one", Module: "mods/a", State: "ok"}, RowVerdict{Label: "a two", Module: "mods/a", State: "ok"})
	if _, _, err := Keep(root, tb, keys, dir, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "a one") {
		t.Fatalf("a row in two files answered %v", err)
	}
	moved := t.TempDir()
	writeVerdicts(t, moved, "shard-1", RowVerdict{Label: "a one", Module: "mods/b", State: "ok"})
	if _, _, err := Keep(root, tb, keys, moved, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "a one") {
		t.Fatalf("a row filed under another module answered %v", err)
	}
	other := t.TempDir()
	put(t, other, "shard-1/verdicts.json", `{"schema":"repo-mutate-verdicts/0","verdicts":[]}`)
	if _, _, err := Keep(root, tb, keys, other, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("a file of another shape answered %v", err)
	}
	if _, _, err := Keep(root, tb, keys, t.TempDir(), &bytes.Buffer{}); err == nil {
		t.Fatal("an empty directory kept nothing and passed")
	}
	if _, _, err := Keep(root, tb, keys, "", &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "named") {
		t.Fatalf("no directory answered %v", err)
	}
}

// TestAVerdictFileNamesEachRowsModule. ⛔ The keep step sorts verdicts into
// modules by it, and a row with none would be kept by no module.
func TestAVerdictFileNamesEachRowsModule(t *testing.T) {
	_, tb := proofRepo(t)
	file := filepath.Join(t.TempDir(), "v.json")
	proved := Verdict{Label: "b two", State: "ok", Cases: 1}
	if err := WriteVerdicts(file, tb, Shard{K: 2, N: 3}, []Verdict{proved}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var f VerdictFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.Schema != VerdictsSchema || f.Shard != "2/3" || len(f.Verdicts) != 1 || f.Verdicts[0].Module != "mods/b" {
		t.Fatalf("the file holds %+v", f)
	}
	stray := Verdict{Label: "no such row"}
	if err := WriteVerdicts(file, tb, Shard{}, []Verdict{stray}); err == nil {
		t.Fatal("a verdict for a row the table does not hold was written")
	}
}
