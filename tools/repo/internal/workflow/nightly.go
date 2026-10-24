// SPDX-License-Identifier: 0BSD

package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// NightlyLabel is the label herdr-build.yml names a nightly's builds with.
const NightlyLabel = "herdr"

// BuildInfoName is the record a nightly publishes beside its four builds.
const BuildInfoName = "BUILD-INFO.json"

// BuildInfoSchema is BUILD-INFO.json's shape. ⚠ The adapter's nightly channel
// reads it, so a change of shape is a new schema, never an edit of this one.
const BuildInfoSchema = "herdr-nightly-build/1"

// NightlyExpected is every name a nightly's staging leaves in dist, SHA256SUMS
// aside: the four builds and BUILD-INFO.json.
func NightlyExpected() []string {
	return append(HerdrAssets(NightlyLabel), BuildInfoName)
}

// NightlyStageOptions is what `repo nightly-stage` is handed.
type NightlyStageOptions struct {
	// Builds holds herdr's four builds, as artifacts/out.
	Builds string
	// Records holds one buildinfo-*.json per build, as artifacts.
	Records   string
	Dist      string
	Tag       string
	Commit    string
	Run       string
	GitHubEnv string
}

// BuildRecords lists the buildinfo-*.json files directly in dir, in name order.
func BuildRecords(dir string) ([]string, error) {
	names, err := Files(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, n := range names {
		if strings.HasPrefix(n, "buildinfo-") && strings.HasSuffix(n, ".json") {
			out = append(out, filepath.Join(dir, n))
		}
	}
	sort.Strings(out)
	return out, nil
}

// BuildInfo is the BUILD-INFO.json text: the tag, herdr's commit, the run that
// built it, and every JSON value the build records hold, in order.
//
// ⚠ THE SHAPE IS THE ONE jq WROTE: the same keys in the same order, two-space
// indent, one trailing newline, and each record's values passed through as
// they were written, so a reader of an older nightly reads this one.
func BuildInfo(tag, commit, run string, records [][]byte) ([]byte, error) {
	builds := []json.RawMessage{}
	for i, body := range records {
		dec := json.NewDecoder(bytes.NewReader(body))
		for {
			var v json.RawMessage
			err := dec.Decode(&v)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("build record %d is not JSON: %w", i+1, err)
			}
			builds = append(builds, v)
		}
	}
	type herdr struct {
		Repository string `json:"repository"`
		Commit     string `json:"commit"`
	}
	info := struct {
		Schema string            `json:"schema"`
		Tag    string            `json:"tag"`
		Herdr  herdr             `json:"herdr"`
		Run    string            `json:"run"`
		Builds []json.RawMessage `json:"builds"`
	}{BuildInfoSchema, tag, herdr{"herdrdev/herdr", commit}, run, builds}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(info); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// NightlyStage copies the four builds, writes BUILD-INFO.json, asserts the set,
// writes SHA256SUMS and appends the sign count to GITHUB_ENV.
//
// ⛔ THE COUNTS ARE ONE LIST'S, as release-stage's are. herdr-nightly.yml typed
// 4, 5 and 6 into three steps, so a fifth target would have been added to the
// copy loop and missed in the counts, and the nightly would fail at signing
// having built every asset correctly. TOOL-32.
func NightlyStage(opts NightlyStageOptions, out io.Writer) error {
	for flag, v := range map[string]string{"--tag": opts.Tag, "--commit": opts.Commit, "--run": opts.Run} {
		if v == "" {
			return fmt.Errorf("%s is empty, and BUILD-INFO.json would name nothing for it", flag)
		}
	}
	if err := os.MkdirAll(opts.Dist, 0o755); err != nil {
		return err
	}
	builds := HerdrAssets(NightlyLabel)
	for _, name := range builds {
		if err := copyFile(filepath.Join(opts.Builds, name), filepath.Join(opts.Dist, name)); err != nil {
			return fmt.Errorf("%s was not built: %w", filepath.Join(opts.Builds, name), err)
		}
	}
	// ⛔ ONE RECORD PER BUILD. A build that left no record is a nightly whose
	// BUILD-INFO.json cannot say how one of its files was made.
	records, err := BuildRecords(opts.Records)
	if err != nil {
		return err
	}
	if len(records) != len(builds) {
		return fmt.Errorf("expected %d build records in %s and found %d", len(builds), opts.Records, len(records))
	}
	var bodies [][]byte
	for _, r := range records {
		body, err := os.ReadFile(r)
		if err != nil {
			return err
		}
		bodies = append(bodies, body)
	}
	info, err := BuildInfo(opts.Tag, opts.Commit, opts.Run, bodies)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(opts.Dist, BuildInfoName), info, 0o644); err != nil {
		return err
	}
	expected := NightlyExpected()
	present, err := Files(opts.Dist)
	if err != nil {
		return err
	}
	if err := CheckStaged(expected, present); err != nil {
		return err
	}
	sums, err := Sums(opts.Dist, present)
	if err != nil {
		return err
	}
	text := SumsFile(sums)
	if err := os.WriteFile(filepath.Join(opts.Dist, SumsName), []byte(text), 0o644); err != nil {
		return err
	}
	fmt.Fprint(out, text)
	line := SignCountLine(expected)
	fmt.Fprintln(out, line)
	return appendEnv(opts.GitHubEnv, line)
}

// GH runs one gh command and returns what it wrote to stdout.
type GH func(args ...string) (string, error)

// NightlyPublishOptions is what `repo nightly-publish` is handed.
type NightlyPublishOptions struct {
	Tag, Repo, Target, NotesFile, Dist string
}

// NightlyPublish creates the prerelease empty, uploads each staged file and
// reads the set back.
//
// ⛔ CREATED EMPTY, THEN EACH ASSET UPLOADED. `gh release create TAG dist/*`
// uploads inside the call that creates, and an upload that fails deletes the
// release it just made, so a nightly lost the whole prerelease to one asset.
// A release that already exists is uploaded into with --clobber, so a re-run
// finishes what a failed one started. TOOL-37.
//
// ⛔ AND THE RUN FAILS WHEN THE READ-BACK IS SHORT. The count this step used to
// print never failed the run. ReadBack names each staged file the prerelease
// lacks.
func NightlyPublish(opts NightlyPublishOptions, gh GH, out io.Writer) error {
	for flag, v := range map[string]string{"--tag": opts.Tag, "--repo": opts.Repo, "--target": opts.Target, "--notes-file": opts.NotesFile} {
		if v == "" {
			return fmt.Errorf("%s is empty", flag)
		}
	}
	if !strings.HasPrefix(opts.Tag, NightlyPrefix) {
		return fmt.Errorf("refusing to publish %q, which is not a %s tag", opts.Tag, NightlyPrefix)
	}
	staged, err := Files(opts.Dist)
	if err != nil {
		return err
	}
	if len(staged) == 0 {
		return fmt.Errorf("%s holds no asset to upload", opts.Dist)
	}
	if _, err := gh("release", "view", opts.Tag, "--repo", opts.Repo); err != nil {
		if _, err := gh("release", "create", opts.Tag, "--repo", opts.Repo, "--prerelease",
			"--title", opts.Tag, "--notes-file", opts.NotesFile, "--target", opts.Target); err != nil {
			return fmt.Errorf("creating %s: %w", opts.Tag, err)
		}
	}
	for _, name := range staged {
		if _, err := gh("release", "upload", opts.Tag, filepath.Join(opts.Dist, name), "--repo", opts.Repo, "--clobber"); err != nil {
			return fmt.Errorf("uploading %s: %w. The prerelease stays, and a re-run uploads into it", name, err)
		}
	}
	reported, err := gh("release", "view", opts.Tag, "--repo", opts.Repo, "--json", "assets", "--jq", ".assets[].name")
	if err != nil {
		return fmt.Errorf("could not read back %s: %w", opts.Tag, err)
	}
	names, err := ReadNames(strings.NewReader(reported))
	if err != nil {
		return err
	}
	if err := ReadBack(staged, names); err != nil {
		return err
	}
	fmt.Fprintf(out, "published %s with %d asset(s), each read back\n", opts.Tag, len(staged))
	return nil
}
