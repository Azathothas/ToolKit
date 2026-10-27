// embed.go - a row on an embedded file runs on the binary its baseline built.
//
// ⭐ A COMPILE IS MOST OF A ROW ON A LARGE PACKAGE. A row that changes a file the
// package embeds changes no Go source. Yet the change reached the binary only
// through a new compile of the whole package and its cases. So the harness hands
// the changed bytes to the test binary instead. TOOL-40.
//
// ⭐ THE PROTOCOL IS THREE VARIABLES. REPO_MUTATE_EMBED is the file's path under
// the package's directory, and REPO_MUTATE_WITH a file holding the changed
// bytes. REPO_MUTATE_FILE is the file itself in the staged copy. The package's
// own test code puts the bytes in the embedding variable, then in the file,
// before any case runs. internal/toolkit is the one package here that does, with
// tools/windows/wsl-toolkit/internal/embedswap.
//
// ⛔ THE FILE IS READ BACK, AND IT IS THE RECEIPT. A binary that does not know
// the protocol leaves the file as it was, and its run is thrown away. Without
// the receipt, a case that failed by chance in a run nothing changed would
// certify the guard.
//
// ⛔ ONLY "ok" AND "SKIPPED" ARE KEPT FROM SUCH A RUN. Any other verdict is asked
// again the old way, on a compile, so a THEATRE or BROKEN row reads as it always
// did. A binary that missed a reader of the bytes can only stay green.
//
// ⚠ ONLY WHERE ONE PACKAGE HOLDS THE ROW'S CASES AND EMBEDS THE FILE. The binary
// writes the file while its `go test` runs. A second package in that run could
// still be compiling, and could read the file half written.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// The protocol's variables. ⛔ The test code that answers them names the same
// three, and a rename on one side alone sends every row back to a compile.
const (
	embedNameEnv = "REPO_MUTATE_EMBED"
	embedWithEnv = "REPO_MUTATE_WITH"
	embedFileEnv = "REPO_MUTATE_FILE"
)

// embeddedName answers the row's file as the package's go:embed pattern spells
// it, when the one package holding the row's cases embeds it.
func embeddedName(dest string, m Mutation, pkgs []string) (string, bool) {
	if len(pkgs) != 1 || strings.HasSuffix(m.File, ".go") {
		return "", false
	}
	mod := modulePath(filepath.Join(dest, "go.mod"))
	var dir string
	switch {
	case mod == "":
		return "", false
	case pkgs[0] == mod:
	case strings.HasPrefix(pkgs[0], mod+"/"):
		dir = strings.TrimPrefix(pkgs[0], mod+"/")
	default:
		return "", false
	}
	name := path.Clean(filepath.ToSlash(m.File))
	if dir != "" {
		under, ok := strings.CutPrefix(name, dir+"/")
		if !ok {
			return "", false
		}
		name = under
	}
	for _, p := range embedPatterns(filepath.Join(dest, filepath.FromSlash(dir))) {
		if embedMatches(p, name) {
			return name, true
		}
	}
	return "", false
}

// modulePath reads the module line of a go.mod.
func modulePath(gomod string) string {
	f, err := os.Open(gomod)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	return ""
}

// embedPatterns is every go:embed pattern in a package directory's own files.
func embedPatterns(dir string) []string {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil
	}
	var out []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, ln := range strings.Split(string(raw), "\n") {
			rest, ok := strings.CutPrefix(strings.TrimSpace(ln), "//go:embed ")
			if !ok {
				continue
			}
			for _, p := range strings.Fields(rest) {
				if u, err := strconv.Unquote(p); err == nil {
					p = u
				}
				out = append(out, strings.TrimPrefix(p, "all:"))
			}
		}
	}
	return out
}

// embedMatches is whether a pattern embeds name: the file itself, or a
// directory that holds it.
func embedMatches(pattern, name string) bool {
	for at := name; at != "." && at != "/"; at = path.Dir(at) {
		if ok, _ := path.Match(pattern, at); ok {
			return true
		}
	}
	return false
}

// onBaselineBinary runs the row with its bytes handed to the test binary. It
// answers false when the run settles nothing, and the row is then compiled.
func (wk *worker) onBaselineBinary(dest, name string, body, changed []byte, m Mutation, b baseline, args []string) (Verdict, bool) {
	file := filepath.Join(dest, filepath.FromSlash(m.File))
	dir := filepath.Join(wk.slot.dir, "embed")
	with := filepath.Join(dir, "changed")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Verdict{}, false
	}
	if err := os.WriteFile(with, changed, 0o600); err != nil {
		return Verdict{}, false
	}
	env := []string{embedNameEnv + "=" + name, embedWithEnv + "=" + with, embedFileEnv + "=" + file}
	r, err := wk.goTestEnv(dest, m.File, env, args...)
	got, readErr := os.ReadFile(file)
	took := readErr == nil && bytes.Equal(got, changed)
	// ⛔ PUT BACK BEFORE ANYTHING ELSE RUNS IN THE COPY, and read back.
	if werr := os.WriteFile(file, body, 0o600); werr != nil {
		wk.drop(m.Module)
		return Verdict{Label: m.Label, State: "BROKEN", Reason: "the embedded file could not be put back: " + werr.Error()}, true
	}
	if back, rerr := os.ReadFile(file); rerr != nil || !bytes.Equal(back, body) {
		wk.drop(m.Module)
		return Verdict{Label: m.Label, State: "BROKEN", Reason: "the embedded file did not read back as it was staged"}, true
	}
	if !took {
		wk.p.note(fmt.Sprintf("  compiled: the binary of %q did not take the embedded file it was handed, so the row runs on a new compile\n", m.Label))
		return Verdict{}, false
	}
	v := judge(Verdict{Label: m.Label}, b, r, err)
	// ⭐ A SKIP IS KEPT TOO, and judge calls a run SKIPPED only where its
	// baseline skipped every case as well. That host cannot run the cases, and a
	// compile of the same bytes could not change it. Asked again, every skipped
	// row ran twice. TOOL-40.
	if v.State != "ok" && v.State != "SKIPPED" {
		return Verdict{}, false
	}
	wk.p.embedded()
	return v, true
}
