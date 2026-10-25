// proof.go - what CI already proved, kept by content, and what it still has to.
//
// ⭐ A MODULE IS PROVED BY WHAT IT HOLDS, NOT BY THE PUSH THAT CARRIED IT. A
// push that changed a document ran all 1,262 rows. The proof key names
// everything a row's verdict can read: the module's committed tree, its own
// rows, the harness, the Go version and the runner. A row runs in a staged
// copy of its module alone, so nothing outside the module reaches a case. A
// CI run that finds a proof kept under that key prints it and runs none of
// that module's rows.
//
// ⛔ A KEPT PROOF CARRIES EVERY VERDICT, AND THE PLAN PRINTS THEM AGAIN. A row
// whose case skipped on the runner stays SKIPPED in every plan that reads the
// proof. A cache never turns a row that was not proved into one that was.
//
// ⛔ A MODULE IS KEPT ONLY WHOLE. Its rows are spread over every shard, so no
// one shard proves it. The keep step reads every shard's verdicts and keeps a
// module only when each of its rows has exactly one, and none failed.
//
// ⛔ THE KEY NAMES THE RUNNER, because a verdict reads it too. A case can go
// red on a new image with nothing changed here, and a SKIPPED row can become
// provable there. So the key names the image, the rows run at once and
// GOFLAGS. A new image proves every module again on its first push. It used
// not to, and a proof kept on an old image answered for the new one.
//
// ⛔ A CACHE ENTRY CANNOT BE REPLACED, SO A RED FULL RUN DELETES THEM ALL. The
// nightly reads no kept proof. When one of its rows fails, ci.yml deletes every
// kept proof, and the next push proves every row again. A flake is found that
// way too.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// The shapes of the two files this writes. ⚠ Both name their Runner since /2.
const (
	ProofSchema    = "repo-mutate-proof/2"
	VerdictsSchema = "repo-mutate-verdicts/2"
)

// Runner is what a verdict reads besides the module, the harness and the Go
// version. It is the image the runner booted, the --jobs the harness ran
// with, and GOFLAGS, which every test process inherits.
type Runner struct {
	Image   string `json:"image"`
	Jobs    int    `json:"jobs"`
	GoFlags string `json:"goflags"`
}

// check says why r cannot name a proof, or nil when it can.
//
// ⛔ NO IMAGE IS AN ERROR, NEVER AN EMPTY LINE IN THE KEY. A key that named no
// image would let a proof from one image answer on every other.
func (r Runner) check() error {
	if strings.TrimSpace(r.Image) == "" {
		return fmt.Errorf("no runner image was named, so no key can say what a verdict ran on. Pass --image")
	}
	if r.Jobs < 1 {
		return fmt.Errorf("--jobs %d names no count of rows run at once, so no key can say how the rows ran", r.Jobs)
	}
	return nil
}

// String is r as a plan and a keep step print it.
func (r Runner) String() string {
	return fmt.Sprintf("%s with --jobs %d and GOFLAGS %q", r.Image, r.Jobs, r.GoFlags)
}

// ProofDir is where kept proofs live, under the repository root. ⚠ Under
// .tmp, which is gitignored, so a restored proof never reads as a change.
const ProofDir = ".tmp/mutate-proof"

// The harness is its module, less the table: each module's key names its own
// rows. ⚠ Hashing the table whole would re-prove every module for a row
// added to one.
const (
	harnessModule = "tools/repo"
	harnessTable  = "tools/repo/mutations.json"
)

// ModuleKey is one module's proof key and where its proof is kept.
type ModuleKey struct {
	Module string
	Key    string
	// Path is the proof's file, relative to the root, in slash form.
	// ⛔ ONE PATH PER MODULE, NEVER ONE PER SLOT. A cache entry is found only
	// at the path it was saved from, and a slot moves when a module is added.
	Path string
	// Rows are its rows, by place in the table.
	Rows []int
	// Runner is the runner the key names.
	Runner Runner
}

// RowVerdict is one row's verdict as a file carries it.
type RowVerdict struct {
	Label   string `json:"label"`
	Module  string `json:"module"`
	State   string `json:"state"`
	Cases   int    `json:"cases"`
	Skipped int    `json:"skipped"`
	Reason  string `json:"reason,omitempty"`
}

// VerdictFile is what `mutate --verdicts` writes for one run.
type VerdictFile struct {
	Schema   string       `json:"schema"`
	Shard    string       `json:"shard,omitempty"`
	Runner   Runner       `json:"runner"`
	Verdicts []RowVerdict `json:"verdicts"`
}

// Proof is what a proved module keeps.
type Proof struct {
	Schema   string       `json:"schema"`
	Module   string       `json:"module"`
	Key      string       `json:"key"`
	Runner   Runner       `json:"runner"`
	Verdicts []RowVerdict `json:"verdicts"`
}

// Modules is every module the table names, sorted.
func Modules(t *Table) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range t.Mutations {
		if !seen[m.Module] {
			seen[m.Module] = true
			out = append(out, m.Module)
		}
	}
	sort.Strings(out)
	return out
}

// GoVersion is the version `go test` runs under in root, as go1.25.0.
func GoVersion(root string) (string, error) {
	cmd := exec.Command("go", "env", "GOVERSION")
	cmd.Dir = root
	out, err := cmd.Output()
	v := strings.TrimSpace(string(out))
	if err != nil || v == "" {
		return "", fmt.Errorf("go env GOVERSION did not answer, so no proof key can name the toolchain: %v", err)
	}
	return v, nil
}

// ProofKeys is each module's key on rn, in Modules order.
//
// ⛔ A MODULE WITH UNCOMMITTED CHANGES IS REFUSED, and so is the harness. The
// key names HEAD, and a run proves the working tree. Keeping a proof of one
// under the name of the other is a proof of something nobody committed.
func ProofKeys(root string, t *Table, goVersion string, rn Runner) ([]ModuleKey, error) {
	if goVersion == "" {
		return nil, fmt.Errorf("no Go version, so no key can name the toolchain")
	}
	if err := rn.check(); err != nil {
		return nil, err
	}
	for _, p := range append([]string{harnessModule}, Modules(t)...) {
		dirty, err := gitOut(root, "status", "--porcelain", "--", p)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(dirty) != "" {
			return nil, fmt.Errorf("%s has uncommitted changes, and a key names HEAD: %s", p, strings.TrimSpace(dirty))
		}
	}
	harness, err := harnessHash(root)
	if err != nil {
		return nil, err
	}
	var out []ModuleKey
	for _, m := range Modules(t) {
		tree, err := gitOut(root, "rev-parse", "HEAD:"+m)
		if err != nil {
			return nil, fmt.Errorf("%s is not in HEAD: %w", m, err)
		}
		var rows []int
		rh := sha256.New()
		for i, r := range t.Mutations {
			if r.Module != m {
				continue
			}
			rows = append(rows, i)
			raw, err := json.Marshal(r)
			if err != nil {
				return nil, err
			}
			_, _ = rh.Write(append(raw, '\n'))
		}
		h := sha256.New()
		fmt.Fprintf(h, "%s\nmodule %s\n", ProofSchema, m)
		fmt.Fprintf(h, "tree %s\n", strings.TrimSpace(tree))
		fmt.Fprintf(h, "harness %s\n", harness)
		fmt.Fprintf(h, "go %s\n", goVersion)
		fmt.Fprintf(h, "rows %x\n", rh.Sum(nil))
		fmt.Fprintf(h, "image %s\n", rn.Image)
		fmt.Fprintf(h, "jobs %d\n", rn.Jobs)
		fmt.Fprintf(h, "goflags %s\n", rn.GoFlags)
		out = append(out, ModuleKey{
			Module: m,
			Key:    "mutate-proof-" + path.Base(m) + "-" + hex.EncodeToString(h.Sum(nil))[:40],
			Path:   ProofDir + "/" + moduleKey(m) + ".json",
			Rows:   rows,
			Runner: rn,
		})
	}
	return out, nil
}

// harnessHash names the harness as HEAD holds it: every blob under
// tools/repo but the table.
func harnessHash(root string) (string, error) {
	out, err := gitOut(root, "ls-tree", "-r", "-z", "HEAD", "--", harnessModule)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n := 0
	for _, entry := range strings.Split(out, "\x00") {
		_, p, ok := strings.Cut(entry, "\t")
		if !ok || p == harnessTable {
			continue
		}
		_, _ = io.WriteString(h, entry+"\n")
		n++
	}
	if n == 0 {
		return "", fmt.Errorf("HEAD holds nothing under %s, so no key can name the harness", harnessModule)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func gitOut(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

// SlotOutputs are the lines a workflow reads its slots from: module, key and
// path for each slot, empty past the last module.
//
// ⛔ MORE MODULES THAN SLOTS IS AN ERROR THAT SAYS WHERE TO ADD ONE. A module
// past the last slot would be planned and never looked up, so it would run on
// every push and be kept by nobody, with nothing said.
func SlotOutputs(keys []ModuleKey, slots int, value func(ModuleKey) string, name string) ([]string, error) {
	if len(keys) > slots {
		return nil, fmt.Errorf("the table names %d modules and the workflow has %d proof slots. Add a slot to every proof step in .github/workflows/ci.yml", len(keys), slots)
	}
	var out []string
	for i := 0; i < slots; i++ {
		v := ""
		if i < len(keys) {
			v = value(keys[i])
		}
		out = append(out, fmt.Sprintf("%s%d=%s", name, i+1, v))
	}
	return out, nil
}

// Plan is what one CI run proves and what it reads from kept proofs.
type Plan struct {
	Full   bool
	Kept   []Proof
	Run    []string
	Rows   int
	Shards int
}

// Outputs are the lines the workflow reads the plan from.
func (p Plan) Outputs() []string {
	var shards []string
	for k := 1; k <= p.Shards; k++ {
		shards = append(shards, fmt.Sprintf("%d/%d", k, p.Shards))
	}
	list, _ := json.Marshal(shards)
	if shards == nil {
		list = []byte("[]")
	}
	return []string{
		"run=" + fmt.Sprint(len(p.Run) > 0),
		"modules=" + strings.Join(p.Run, " "),
		"shards=" + string(list),
	}
}

// MakePlan reads each module's kept proof under root and decides what runs.
// maxShards is the most runners the rows that run are cut into.
//
// ⛔ A PROOF THAT DOES NOT READ AS WHOLE IS NO PROOF. Its module runs, and why
// is printed. ⚠ A full run reads none, which is what the nightly and the
// release do.
func MakePlan(root string, t *Table, keys []ModuleKey, full bool, maxShards int, w io.Writer) (Plan, error) {
	if maxShards < 1 {
		return Plan{}, fmt.Errorf("at most %d shards would prove nothing", maxShards)
	}
	p := Plan{Full: full}
	var rows []int
	for _, k := range keys {
		if full {
			fmt.Fprintf(w, "  runs:  %s, %d row(s), because a full run reads no kept proof\n", k.Module, len(k.Rows))
		} else if pr, why := readProof(root, t, k); why == "" {
			p.Kept = append(p.Kept, pr)
			writeKept(w, pr)
			continue
		} else {
			fmt.Fprintf(w, "  runs:  %s, %d row(s), because %s\n", k.Module, len(k.Rows), why)
		}
		p.Run = append(p.Run, k.Module)
		rows = append(rows, k.Rows...)
	}
	sort.Ints(rows)
	p.Rows = len(rows)
	if len(rows) > 0 {
		p.Shards = min(maxShards, len(groupRows(t, rows)))
	}
	switch {
	case len(rows) == 0:
		fmt.Fprintf(w, "\nevery module's proof is kept under the key printed above, so no shard runs\n")
	default:
		fmt.Fprintf(w, "\n%d row(s) of %d module(s) run in %d shard(s)\n", p.Rows, len(p.Run), p.Shards)
	}
	return p, nil
}

// writeKept prints a kept proof with its key, and every row in it that was not
// proved, by name.
func writeKept(w io.Writer, pr Proof) {
	ok, skipped := 0, []RowVerdict{}
	for _, v := range pr.Verdicts {
		if v.State == "ok" {
			ok++
		} else {
			skipped = append(skipped, v)
		}
	}
	fmt.Fprintf(w, "  kept:  %s, %d of %d row(s) proved on %s, under %s\n", pr.Module, ok, len(pr.Verdicts), pr.Runner, pr.Key)
	for _, v := range skipped {
		fmt.Fprintf(w, "    SKIPPED: %s: %s, so it is unproved rather than proved or theatre\n", v.Label, v.Reason)
	}
}

// readProof reads k's kept proof, or says why there is none to use.
func readProof(root string, t *Table, k ModuleKey) (Proof, string) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(k.Path)))
	if os.IsNotExist(err) {
		return Proof{}, "no proof is kept under " + k.Key
	}
	if err != nil {
		return Proof{}, "its kept proof could not be read: " + err.Error()
	}
	var pr Proof
	if err := json.Unmarshal(raw, &pr); err != nil {
		return Proof{}, "its kept proof is not readable: " + err.Error()
	}
	if pr.Schema != ProofSchema || pr.Module != k.Module || pr.Key != k.Key {
		return Proof{}, fmt.Sprintf("its kept proof names %s %s %s, not %s %s %s", pr.Schema, pr.Module, pr.Key, ProofSchema, k.Module, k.Key)
	}
	if why := whole(t, k, pr.Verdicts); why != "" {
		return Proof{}, "its kept proof is not whole: " + why
	}
	return pr, ""
}

// whole says why verdicts are not one passing verdict for each of k's rows,
// or nothing when they are.
//
// ⛔ ok AND SKIPPED ARE THE TWO A PROOF HOLDS. THEATRE and BROKEN fail a run,
// and a run that failed keeps nothing.
func whole(t *Table, k ModuleKey, verdicts []RowVerdict) string {
	want := map[string]bool{}
	for _, r := range k.Rows {
		want[t.Mutations[r].Label] = true
	}
	seen := map[string]bool{}
	for _, v := range verdicts {
		switch {
		case !want[v.Label]:
			return fmt.Sprintf("%q is not a row of %s", v.Label, k.Module)
		case seen[v.Label]:
			return fmt.Sprintf("%q has two verdicts", v.Label)
		case v.State != "ok" && v.State != "SKIPPED":
			return fmt.Sprintf("%q is %s", v.Label, v.State)
		}
		seen[v.Label] = true
	}
	if len(seen) != len(want) {
		return fmt.Sprintf("%d of %d row(s) have a verdict", len(seen), len(want))
	}
	return ""
}

// WriteVerdicts writes one run's verdicts, each with its module, and the
// runner they ran on, for the keep step to read.
func WriteVerdicts(file string, t *Table, shard Shard, rn Runner, verdicts []Verdict) error {
	module := map[string]string{}
	for _, m := range t.Mutations {
		module[m.Label] = m.Module
	}
	f := VerdictFile{Schema: VerdictsSchema, Runner: rn, Verdicts: []RowVerdict{}}
	if shard.N > 1 {
		f.Shard = fmt.Sprintf("%d/%d", shard.K, shard.N)
	}
	for _, v := range verdicts {
		m, ok := module[v.Label]
		if !ok {
			return fmt.Errorf("%q is not a row of the table", v.Label)
		}
		f.Verdicts = append(f.Verdicts, RowVerdict{Label: v.Label, Module: m, State: v.State, Cases: v.Cases, Skipped: v.Skipped, Reason: v.Reason})
	}
	raw, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(file, append(raw, '\n'), 0o600)
}

// Keep reads every verdict file under dir and writes the proof of each module
// whose rows all have one passing verdict. It returns the modules kept, and
// whether a module that ran was left whole.
//
// ⛔ A ROW CLAIMED BY TWO FILES IS AN ERROR. Every row belongs to one shard,
// so two verdicts for one row say the files are not this run's.
// ⛔ A MODULE THAT RAN AND IS NOT WHOLE IS A FAILURE, not a quiet miss. Every
// shard passed, so a missing verdict is a missing file.
//
// ⛔ A MODULE IS KEPT ONLY UNDER THE RUNNER ITS ROWS RAN ON. A shard that ran
// on another image, or with another --jobs or GOFLAGS, proved nothing for this
// key. ⚠ It is printed and not kept, and the step does not fail. Runners move
// to a new image a few at a time, so one run can hold two images.
func Keep(root string, t *Table, keys []ModuleKey, dir string, w io.Writer) (kept []ModuleKey, short bool, err error) {
	files, err := verdictFiles(dir)
	if err != nil {
		return nil, false, err
	}
	at := map[string]int{}
	for i, m := range t.Mutations {
		at[m.Label] = i
	}
	byModule := map[string][]RowVerdict{}
	seen := map[string]string{}
	// ranOn is each module's runners, with a file that ran on each.
	ranOn := map[string]map[Runner]string{}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return nil, false, err
		}
		var f VerdictFile
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, false, fmt.Errorf("%s is not a verdict file: %w", file, err)
		}
		if f.Schema != VerdictsSchema {
			return nil, false, fmt.Errorf("%s declares schema %q and this build reads %q", file, f.Schema, VerdictsSchema)
		}
		for _, v := range f.Verdicts {
			i, ok := at[v.Label]
			if !ok || t.Mutations[i].Module != v.Module {
				return nil, false, fmt.Errorf("%s holds %q of %s, which is not a row of this table", file, v.Label, v.Module)
			}
			if other, ok := seen[v.Label]; ok {
				return nil, false, fmt.Errorf("%q has a verdict in %s and in %s, and a row belongs to one shard", v.Label, other, file)
			}
			seen[v.Label] = file
			byModule[v.Module] = append(byModule[v.Module], v)
			if ranOn[v.Module] == nil {
				ranOn[v.Module] = map[Runner]string{}
			}
			ranOn[v.Module][f.Runner] = file
		}
	}
	for _, k := range keys {
		got := byModule[k.Module]
		if len(got) == 0 {
			fmt.Fprintf(w, "  not run here: %s\n", k.Module)
			continue
		}
		if why := elsewhere(k, ranOn[k.Module]); why != "" {
			fmt.Fprintf(w, "  NOT KEPT: %s, because %s\n", k.Module, why)
			continue
		}
		if why := whole(t, k, got); why != "" {
			fmt.Fprintf(w, "  NOT KEPT: %s, because %s\n", k.Module, why)
			short = true
			continue
		}
		sort.Slice(got, func(a, b int) bool { return at[got[a].Label] < at[got[b].Label] })
		pr := Proof{Schema: ProofSchema, Module: k.Module, Key: k.Key, Runner: k.Runner, Verdicts: got}
		raw, err := json.MarshalIndent(pr, "", "  ")
		if err != nil {
			return nil, false, err
		}
		p := filepath.Join(root, filepath.FromSlash(k.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return nil, false, err
		}
		if err := os.WriteFile(p, append(raw, '\n'), 0o600); err != nil {
			return nil, false, err
		}
		writeKept(w, pr)
		kept = append(kept, k)
	}
	return kept, short, nil
}

// elsewhere names each runner other than k's that rows of k's module ran on,
// with a file that ran there. It is empty when every row ran on k's runner.
func elsewhere(k ModuleKey, ran map[Runner]string) string {
	var files []string
	for r, file := range ran {
		if r != k.Runner {
			files = append(files, fmt.Sprintf("%s ran on %s", file, r))
		}
	}
	if len(files) == 0 {
		return ""
	}
	sort.Strings(files)
	return fmt.Sprintf("%s, and its key names %s", strings.Join(files, "; "), k.Runner)
}

// verdictFiles is every .json file under dir, sorted.
//
// ⛔ NO FILE IS AN ERROR. A keep step with nothing to read would keep nothing
// and pass, and every later push would run the whole table again.
func verdictFiles(dir string) ([]string, error) {
	if dir == "" {
		return nil, fmt.Errorf("no directory of verdict files was named, so nothing can be kept")
	}
	var out []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() && strings.HasSuffix(info.Name(), ".json") {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s holds no verdict file, so nothing can be kept", dir)
	}
	sort.Strings(out)
	return out, nil
}
