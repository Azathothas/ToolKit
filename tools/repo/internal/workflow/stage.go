// Package workflow holds the decisions .github/workflows used to make in shell.
//
// ⛔ WHY THEY ARE HERE AND NOT IN THE YAML. A `run:` block has no case and no
// mutation row, and a workflow cannot be run anywhere but on a runner, so a
// defect in one is found by a release that fails or, worse, one that publishes
// short. One shipped: the sign count was typed into two steps and missed when
// text-tool joined the release. TOOL-32.
//
// ⭐ PURE FUNCTIONS CARRY EVERY DECISION, and each subcommand in main.go is a
// thin wrapper that reads files, calls one of them and prints. The cases test
// the functions; the YAML is left holding only the calls to gh and cosign that
// need a runner's identity.
//
// SPDX-License-Identifier: 0BSD
package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Build is one executable release.yml compiles and publishes.
type Build struct {
	// Module is the Go module to build, relative to the repository root.
	Module string
	OS     string
	Arch   string
	Name   string
}

// ReleaseBuilds is every executable a wsl-toolkit release carries, and it is
// the ONE list both the build loop and the staging assertion read.
//
// ⛔ ONE HOME. The YAML used to build from one loop and assert from a list typed
// beside it, so the two could disagree about a name and the release would fail
// at signing, having built every asset correctly.
//
// ⭐ text-tool SHIPS FOR BOTH HOSTS, because the agent that needs it runs on
// Windows and inside the base, and a tool that writes files without a shell in
// the path is no use only on one of them. Pure Go, CGO off, so the cross builds
// need nothing installed.
func ReleaseBuilds() []Build {
	var out []Build
	for _, arch := range []string{"amd64", "arm64"} {
		out = append(out, Build{Module: "tools/windows/wsl-toolkit", OS: "windows", Arch: arch, Name: "wsl-toolkit-windows-" + arch + ".exe"})
	}
	for _, goos := range []string{"windows", "linux"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := "text-tool-" + goos + "-" + arch
			if goos == "windows" {
				name += ".exe"
			}
			out = append(out, Build{Module: "tools/text-tool", OS: goos, Arch: arch, Name: name})
		}
	}
	return out
}

// HerdrAssets is herdr's four builds under the label herdr-build.yml gave them.
func HerdrAssets(label string) []string {
	var out []string
	for _, suffix := range []string{"windows-x86_64.zip", "windows-aarch64.zip", "linux-x86_64", "linux-aarch64"} {
		out = append(out, label+"-"+suffix)
	}
	return out
}

// ExpectedAssets is every name staging must leave in dist, SHA256SUMS aside.
func ExpectedAssets(herdrLabel string) []string {
	var out []string
	for _, b := range ReleaseBuilds() {
		out = append(out, b.Name)
	}
	return append(out, HerdrAssets(herdrLabel)...)
}

// CheckStaged refuses a staging directory that is short of an expected asset
// or holds one nobody expected.
//
// ⛔ THE COUNT IS ASSERTED, BY NAME. A staging step that produced fewer assets
// than expected and exited 0 would publish a release missing the thing a
// consumer came for, and the release would look complete.
//
// ⛔ AND NOTHING ELSE IS STAGED. Every file in dist is digested, signed and
// uploaded, so a stray one would ship under this workflow's signature.
func CheckStaged(expected, present []string) error {
	have := map[string]bool{}
	for _, p := range present {
		have[p] = true
	}
	want := map[string]bool{}
	var missing, extra []string
	for _, e := range expected {
		want[e] = true
		if !have[e] {
			missing = append(missing, e)
		}
	}
	for _, p := range present {
		if !want[p] {
			extra = append(extra, p)
		}
	}
	var problems []string
	if len(missing) > 0 {
		problems = append(problems, fmt.Sprintf("%d expected asset(s) were not staged: %s", len(missing), strings.Join(missing, ", ")))
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		problems = append(problems, fmt.Sprintf("%d staged file(s) are not release assets: %s", len(extra), strings.Join(extra, ", ")))
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// Sum is one line of SHA256SUMS.
type Sum struct {
	Name   string
	Digest string
	Size   int64
}

// Sums digests the named files in dir, in name order.
//
// ⭐ THE DIGESTS ARE COMPUTED OVER THE BYTES THAT ARE UPLOADED, in the
// directory the upload reads, so a consumer verifying a download against
// SHA256SUMS is checking the same object. A digest computed anywhere else would
// be a digest of a different line ending.
func Sums(dir string, names []string) ([]Sum, error) {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	var out []Sum
	for _, name := range sorted {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		out = append(out, Sum{Name: name, Digest: hex.EncodeToString(h.Sum(nil)), Size: n})
	}
	return out, nil
}

// SumsFile is the SHA256SUMS text: `DIGEST  NAME`, LF, one trailing newline,
// the shape `sha256sum -c` reads.
func SumsFile(sums []Sum) string {
	var b strings.Builder
	for _, s := range sums {
		b.WriteString(s.Digest + "  " + s.Name + "\n")
	}
	return b.String()
}

// SignCountLine is what the later steps assert, for GITHUB_ENV.
//
// ⛔ THE COUNT THE LATER STEPS ASSERT COMES FROM HERE, and it used to be typed
// into each of them. Adding text-tool moved it from 7 to 11 and the staging
// list was updated while two literal 7s were not, so the release failed at
// signing having built every asset correctly. One home: the expected list,
// plus the SHA256SUMS staging writes.
func SignCountLine(expected []string) string {
	return fmt.Sprintf("SIGN_COUNT=%d", len(expected)+1)
}

// SumsName is the digest file staging writes beside the assets.
const SumsName = "SHA256SUMS"

// StageOptions is what `repo release-stage` is handed.
type StageOptions struct {
	Root       string
	Dist       string
	HerdrLabel string
	HerdrFrom  string
	GitHubEnv  string
	// Build makes one executable at out. Nil runs GoBuild, and a case hands a
	// fake so the staging around the compiler is proved without it.
	Build func(b Build, root, out string) error
}

// GoBuildArgs is the go command line and the environment one build runs with.
//
// ⛔ -trimpath, AND NO BUILD STAMP. Two builds of one commit produce identical
// bytes, so a consumer can rebuild and compare rather than trusting the
// publisher. A path or a timestamp compiled in would make every build unique
// and that property unavailable. ⚠ CGO off, so a cross build needs nothing
// installed.
func GoBuildArgs(b Build, out string) (args, env []string) {
	return []string{"build", "-trimpath", "-o", out, "."},
		[]string{"GOOS=" + b.OS, "GOARCH=" + b.Arch, "CGO_ENABLED=0"}
}

// GoBuild runs GoBuildArgs in b's module.
func GoBuild(b Build, root, out string, log io.Writer) error {
	args, env := GoBuildArgs(b, out)
	cmd := exec.Command("go", args...)
	cmd.Dir = filepath.Join(root, filepath.FromSlash(b.Module))
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = log, log
	return cmd.Run()
}

// Files lists the regular files directly in dir, as Get-ChildItem -File did.
func Files(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// Stage builds every executable, copies herdr's four builds, asserts the set,
// writes SHA256SUMS and appends the sign count to GITHUB_ENV. Each build runs
// with GoBuildArgs, which says why.
func Stage(opts StageOptions, out io.Writer) error {
	if opts.HerdrLabel == "" {
		return fmt.Errorf("--herdr-label is empty, so herdr's builds cannot be named")
	}
	dist := opts.Dist
	if !filepath.IsAbs(dist) {
		dist = filepath.Join(opts.Root, dist)
	}
	if err := os.MkdirAll(dist, 0o755); err != nil {
		return err
	}
	build := opts.Build
	if build == nil {
		build = func(b Build, root, to string) error { return GoBuild(b, root, to, out) }
	}
	for _, b := range ReleaseBuilds() {
		if err := build(b, opts.Root, filepath.Join(dist, b.Name)); err != nil {
			return fmt.Errorf("the %s %s/%s build failed: %w", b.Module, b.OS, b.Arch, err)
		}
	}
	for _, name := range HerdrAssets(opts.HerdrLabel) {
		if err := copyFile(filepath.Join(opts.HerdrFrom, name), filepath.Join(dist, name)); err != nil {
			return fmt.Errorf("%s was not built: %w", filepath.Join(opts.HerdrFrom, name), err)
		}
	}
	expected := ExpectedAssets(opts.HerdrLabel)
	present, err := Files(dist)
	if err != nil {
		return err
	}
	if err := CheckStaged(expected, present); err != nil {
		return err
	}
	sums, err := Sums(dist, present)
	if err != nil {
		return err
	}
	for _, s := range sums {
		fmt.Fprintf(out, "%s  %s  (%d bytes)\n", s.Digest, s.Name, s.Size)
	}
	if err := os.WriteFile(filepath.Join(dist, SumsName), []byte(SumsFile(sums)), 0o644); err != nil {
		return err
	}
	line := SignCountLine(expected)
	fmt.Fprintln(out, line)
	return appendEnv(opts.GitHubEnv, line)
}

// appendEnv adds line to the GITHUB_ENV file at path. An empty path prints only.
//
// ⛔ THE LATER STEPS ASSERT WHAT THIS WRITES. A count printed and never
// appended leaves SIGN_COUNT unset, and the signing step then compares against
// nothing.
func appendEnv(path, line string) error {
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, werr := fmt.Fprintln(f, line)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}

func copyFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}
