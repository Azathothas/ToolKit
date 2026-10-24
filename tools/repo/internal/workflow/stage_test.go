// SPDX-License-Identifier: 0BSD

package workflow

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(n+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// TestStageWritesTheSumsAndTheSignCountForTheLaterSteps drives Stage with a
// fake compiler. ⛔ The count the signing step asserts reaches GITHUB_ENV, and
// SHA256SUMS covers every asset.
func TestStageWritesTheSumsAndTheSignCountForTheLaterSteps(t *testing.T) {
	root := t.TempDir()
	herdr := filepath.Join(root, "herdr-out")
	writeFiles(t, herdr, HerdrAssets("herdr-0.9.1")...)
	env := filepath.Join(root, "github-env")
	var built []string
	opts := StageOptions{
		Root: root, Dist: "dist", HerdrLabel: "herdr-0.9.1", HerdrFrom: herdr, GitHubEnv: env,
		Build: func(b Build, _, out string) error {
			built = append(built, b.Name)
			return os.WriteFile(out, []byte(b.Name), 0o644)
		},
	}
	var log bytes.Buffer
	if err := Stage(opts, &log); err != nil {
		t.Fatalf("Stage: %v\n%s", err, log.String())
	}
	if len(built) != len(ReleaseBuilds()) {
		t.Fatalf("built %v; want every one of ReleaseBuilds", built)
	}
	if got := readFile(t, env); got != "SIGN_COUNT=11\n" {
		t.Fatalf("GITHUB_ENV holds %q; want SIGN_COUNT=11", got)
	}
	sums := readFile(t, filepath.Join(root, "dist", SumsName))
	if n := strings.Count(sums, "\n"); n != 10 {
		t.Fatalf("SHA256SUMS has %d lines; want the ten assets:\n%s", n, sums)
	}
	for _, name := range ExpectedAssets("herdr-0.9.1") {
		if !strings.Contains(sums, "  "+name+"\n") {
			t.Errorf("SHA256SUMS does not cover %s", name)
		}
	}

	// ⛔ A herdr build that was never downloaded stops staging before any
	// count is written.
	root2 := t.TempDir()
	writeFiles(t, filepath.Join(root2, "herdr-out"), HerdrAssets("herdr-0.9.1")[1:]...)
	opts.Root, opts.HerdrFrom, opts.GitHubEnv = root2, filepath.Join(root2, "herdr-out"), filepath.Join(root2, "github-env")
	err := Stage(opts, &log)
	if err == nil || !strings.Contains(err.Error(), "was not built") {
		t.Fatalf("a missing herdr build: %v", err)
	}
	if _, err := os.Stat(opts.GitHubEnv); err == nil {
		t.Fatal("a staging that failed still wrote a sign count")
	}

	// A build that fails is named, and staging stops there.
	opts.Build = func(b Build, _, _ string) error { return errors.New("boom") }
	if err := Stage(opts, &log); err == nil || !strings.Contains(err.Error(), "build failed") {
		t.Fatalf("a failed build: %v", err)
	}
}

// TestEveryBuildIsTrimmedAndCgoFree. ⛔ Two builds of one commit are the same
// bytes only without a path compiled in.
func TestEveryBuildIsTrimmedAndCgoFree(t *testing.T) {
	args, env := GoBuildArgs(Build{OS: "linux", Arch: "arm64"}, "out")
	if strings.Join(args, " ") != "build -trimpath -o out ." {
		t.Fatalf("go %v", args)
	}
	if strings.Join(env, " ") != "GOOS=linux GOARCH=arm64 CGO_ENABLED=0" {
		t.Fatalf("env %v", env)
	}
}

func nightlyFixture(t *testing.T) NightlyStageOptions {
	t.Helper()
	root := t.TempDir()
	art := filepath.Join(root, "artifacts")
	writeFiles(t, filepath.Join(art, "out"), HerdrAssets(NightlyLabel)...)
	for i, n := range []string{"windows-x86_64", "linux-x86_64", "windows-aarch64", "linux-aarch64"} {
		body := `{"asset":"` + n + `","n":` + string(rune('1'+i)) + `}` + "\n"
		if err := os.WriteFile(filepath.Join(art, "buildinfo-"+n+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return NightlyStageOptions{
		Builds: filepath.Join(art, "out"), Records: art, Dist: filepath.Join(root, "dist"),
		Tag: "herdr-nightly-20261024-abc1234", Commit: "abc1234", Run: "https://example.invalid/run/1",
		GitHubEnv: filepath.Join(root, "github-env"),
	}
}

// TestTheNightlyStagesFourBuildsAndItsRecord. ⛔ The counts the later steps
// assert come from one list: four builds and BUILD-INFO.json under
// SHA256SUMS, six files to sign.
func TestTheNightlyStagesFourBuildsAndItsRecord(t *testing.T) {
	opts := nightlyFixture(t)
	var log bytes.Buffer
	if err := NightlyStage(opts, &log); err != nil {
		t.Fatalf("NightlyStage: %v", err)
	}
	if got := readFile(t, opts.GitHubEnv); got != "SIGN_COUNT=6\n" {
		t.Fatalf("GITHUB_ENV holds %q; want SIGN_COUNT=6", got)
	}
	sums := readFile(t, filepath.Join(opts.Dist, SumsName))
	if n := strings.Count(sums, "\n"); n != 5 || !strings.Contains(sums, "  "+BuildInfoName+"\n") {
		t.Fatalf("SHA256SUMS covers %d files; want the four builds and %s:\n%s", n, BuildInfoName, sums)
	}

	// A nightly with no tag would publish a record naming nothing.
	opts = nightlyFixture(t)
	opts.Tag = ""
	if err := NightlyStage(opts, &log); err == nil || !strings.Contains(err.Error(), "--tag is empty") {
		t.Fatalf("no tag: %v", err)
	}

	// ⛔ A build with no record is refused, before anything is summed.
	opts = nightlyFixture(t)
	if err := os.Remove(filepath.Join(opts.Records, "buildinfo-linux-aarch64.json")); err != nil {
		t.Fatal(err)
	}
	if err := NightlyStage(opts, &log); err == nil || !strings.Contains(err.Error(), "expected 4 build records") {
		t.Fatalf("three records for four builds: %v", err)
	}
	if _, err := os.Stat(filepath.Join(opts.Dist, SumsName)); err == nil {
		t.Fatal("a refused nightly still wrote SHA256SUMS")
	}

	opts = nightlyFixture(t)
	if err := os.Remove(filepath.Join(opts.Builds, "herdr-linux-x86_64")); err != nil {
		t.Fatal(err)
	}
	if err := NightlyStage(opts, &log); err == nil || !strings.Contains(err.Error(), "was not built") {
		t.Fatalf("a missing build: %v", err)
	}
}

// TestBuildInfoKeepsTheShapeJqWrote: the keys in jq's order, two-space indent,
// every record's values in record order.
func TestBuildInfoKeepsTheShapeJqWrote(t *testing.T) {
	got, err := BuildInfo("herdr-nightly-x", "abc", "https://example.invalid/r", [][]byte{[]byte(`{"a":1}`), []byte("{\"b\":\"<2>\"}\n{\"c\":3}\n")})
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "schema": "herdr-nightly-build/1",
  "tag": "herdr-nightly-x",
  "herdr": {
    "repository": "herdrdev/herdr",
    "commit": "abc"
  },
  "run": "https://example.invalid/r",
  "builds": [
    {
      "a": 1
    },
    {
      "b": "<2>"
    },
    {
      "c": 3
    }
  ]
}
`
	if string(got) != want {
		t.Fatalf("BUILD-INFO.json =\n%s\nwant\n%s", got, want)
	}
	if _, err := BuildInfo("t", "c", "r", [][]byte{[]byte("{not json")}); err == nil {
		t.Fatal("a record that is not JSON was accepted")
	}
}

type fakeGH struct {
	calls    []string
	exists   bool
	reported string
	fail     string
}

func (f *fakeGH) run(args ...string) (string, error) {
	call := strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if f.fail != "" && strings.Contains(call, f.fail) {
		return "", errors.New("gh failed")
	}
	switch {
	case len(args) > 2 && args[1] == "view" && strings.Contains(call, "--json"):
		return f.reported, nil
	case len(args) > 1 && args[1] == "view":
		if !f.exists {
			return "", errors.New("release not found")
		}
	}
	return "", nil
}

// TestTheNightlyIsCreatedEmptyThenUploadedAndReadBack. ⛔ The create carries
// no asset, so a failed upload cannot delete the prerelease; each staged file
// is uploaded; and a short read-back fails the run. TOOL-37.
func TestTheNightlyIsCreatedEmptyThenUploadedAndReadBack(t *testing.T) {
	dist := filepath.Join(t.TempDir(), "dist")
	writeFiles(t, dist, "SHA256SUMS", "herdr-linux-x86_64")
	opts := NightlyPublishOptions{Tag: "herdr-nightly-x", Repo: "o/r", Target: "sha", NotesFile: "notes.md", Dist: dist}
	gh := &fakeGH{reported: "herdr-linux-x86_64\nSHA256SUMS\n"}
	var out bytes.Buffer
	if err := NightlyPublish(opts, gh.run, &out); err != nil {
		t.Fatalf("NightlyPublish: %v", err)
	}
	var creates, uploads int
	for _, c := range gh.calls {
		if strings.HasPrefix(c, "release create") {
			creates++
			if strings.Contains(c, dist) {
				t.Fatalf("the create uploads inside the call that creates: %s", c)
			}
		}
		if strings.HasPrefix(c, "release upload") {
			uploads++
		}
	}
	if creates != 1 || uploads != 2 {
		t.Fatalf("calls %q; want one empty create and an upload per staged file", gh.calls)
	}

	gh = &fakeGH{exists: true, reported: "SHA256SUMS\n"}
	err := NightlyPublish(opts, gh.run, &out)
	if err == nil || !strings.Contains(err.Error(), "missing 1 asset(s): herdr-linux-x86_64") {
		t.Fatalf("a short read-back passed: %v", err)
	}
	for _, c := range gh.calls {
		if strings.HasPrefix(c, "release create") {
			t.Fatalf("an existing prerelease was created again: %q", gh.calls)
		}
	}

	gh = &fakeGH{fail: "upload"}
	if err := NightlyPublish(opts, gh.run, &out); err == nil || !strings.Contains(err.Error(), "uploading") {
		t.Fatalf("a failed upload: %v", err)
	}
	// Each refusal below has a gh that would answer a full read-back, so the
	// refusal alone can fail it.
	full := func() *fakeGH { return &fakeGH{reported: "herdr-linux-x86_64\nSHA256SUMS\n"} }
	noRepo := opts
	noRepo.Repo = ""
	if err := NightlyPublish(noRepo, full().run, &out); err == nil || !strings.Contains(err.Error(), "--repo is empty") {
		t.Fatalf("no --repo: %v", err)
	}
	opts.Tag = "wsl-toolkit-v7.1.0"
	if err := NightlyPublish(opts, full().run, &out); err == nil || !strings.Contains(err.Error(), "refusing to publish") {
		t.Fatalf("a wsl-toolkit tag was published as a nightly: %v", err)
	}
}
