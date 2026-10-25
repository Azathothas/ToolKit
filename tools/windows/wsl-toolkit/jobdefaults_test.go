// SPDX-License-Identifier: 0BSD

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// parsedJob is a run's job flags as the command line gave them.
func parsedJob(t *testing.T, args ...string) jobFlags {
	t.Helper()
	var j jobFlags
	fs := newFlagSet("run-defaults-check")
	j.bind(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	j.readNamed(fs)
	return j
}

// TestAProjectsJobDefaultsYieldToTheCommandLine is WSL-131: jobs.timeout,
// jobs.user, jobs.max_bytes, jobs.max_entries and jobs.cache apply where the
// command line names nothing, and a flag it gives wins, even one whose value is
// the flag's own default.
func TestAProjectsJobDefaultsYieldToTheCommandLine(t *testing.T) {
	cfg := toolkit.DefaultConfig()
	cfg.Jobs.Timeout, cfg.Jobs.User = "45m", "1000:1000"
	cfg.Jobs.MaxBytes, cfg.Jobs.MaxEntries = 4096, 7
	cfg.Jobs.Cache = []string{"cargo:/usr/local/cargo", "pip:/root/.cache/pip"}

	j := parsedJob(t)
	if err := j.applyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if j.timeout != 45*time.Minute || j.user != "1000:1000" || j.maxBytes != 4096 || j.maxEntries != 7 ||
		strings.Join(j.caches, " ") != "cargo:/usr/local/cargo pip:/root/.cache/pip" {
		t.Errorf("a job naming nothing took %s, %q, %d, %d, %v", j.timeout, j.user, j.maxBytes, j.maxEntries, j.caches)
	}

	l := toolkit.DefaultWorkspaceLimits()
	j = parsedJob(t, "--timeout", "30m", "--user", "", "--max-bytes", "1073741824", "--max-entries", "200000", "--cache", "other:/root/.cache/pip")
	if err := j.applyConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if j.timeout != 30*time.Minute || j.user != "" || j.maxBytes != l.MaxBytes || j.maxEntries != l.MaxEntries {
		t.Errorf("the command line lost to the project: %s, %q, %d, %d", j.timeout, j.user, j.maxBytes, j.maxEntries)
	}
	// ⚠ A cache at a path the command line names is the command line's.
	if strings.Join(j.caches, " ") != "cargo:/usr/local/cargo other:/root/.cache/pip" {
		t.Errorf("the caches merged as %v", j.caches)
	}
}

// TestTheProjectsEnvironmentComesFirst: jobs.env_file, then jobs.env, then
// --env-file, then --env, so every value the command line gives wins. WSL-131.
func TestTheProjectsEnvironmentComesFirst(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "project.env")
	if err := os.WriteFile(file, []byte("FROM_FILE=project\nSHARED=file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := toolkit.DefaultConfig()
	cfg.Jobs.EnvFile = file
	cfg.Jobs.Env = map[string]string{"SHARED": "map", "MAPPED": "yes"}
	j := parsedJob(t, "--env", "MAPPED=cli")
	got, err := j.envMap(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got["FROM_FILE"] != "project" || got["SHARED"] != "map" || got["MAPPED"] != "cli" {
		t.Errorf("the environment merged as %v", got)
	}
	cfg.Jobs.EnvFile = filepath.Join(dir, "missing.env")
	if _, err := j.envMap(cfg); err == nil || !strings.Contains(err.Error(), "jobs.env_file") {
		t.Errorf("a jobs.env_file that is not there answered %v", err)
	}
}

// TestAProjectsEnvFileResolvesAgainstItsOwnFile: under a named instance a
// working tree's jobs.env_file resolves against that tree's file, not against
// the instance's own or the working directory. WSL-131.
func TestAProjectsEnvFileResolvesAgainstItsOwnFile(t *testing.T) {
	home := filepath.Join(t.TempDir(), "instances", "f2")
	tree := t.TempDir()
	for dir, file := range map[string]string{home: "config.json", tree: toolkit.WorkingConfigName} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := `{"schema":"wsl-toolkit-config/1"}`
		if dir == tree {
			body = `{"schema":"wsl-toolkit-config/1","jobs":{"env_file":"job.env"}}`
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tree, "job.env"), []byte("FROM=project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(tree, "src")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WSL_TOOLKIT_HOME", home)
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	toolkit.SelectedInstance = toolkit.Instance{Name: "f2", Distro: "wsl-toolkit-f2", Home: home}
	was, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(inner); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(was) })
	cfg, err := toolkit.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	j := testJobFlags()
	got, err := j.envMap(cfg)
	if err != nil || got["FROM"] != "project" {
		t.Errorf("the project's jobs.env_file read as %v, %v; want the file beside its own configuration", got, err)
	}
}

// TestRunTakesItsImageFromTheProject: --image wins, jobs.image is the default,
// and with neither the refusal names both. WSL-131.
func TestRunTakesItsImageFromTheProject(t *testing.T) {
	cfg := toolkit.DefaultConfig()
	if _, err := runImage("", cfg); err == nil || !strings.Contains(err.Error(), "jobs.image") {
		t.Errorf("no image anywhere answered %v", err)
	}
	cfg.Jobs.Image = "debian"
	if got, err := runImage("", cfg); err != nil || got != "debian" {
		t.Errorf("jobs.image gave %q, %v", got, err)
	}
	if got, err := runImage("alpine", cfg); err != nil || got != "alpine" {
		t.Errorf("--image beside jobs.image gave %q, %v", got, err)
	}
}

// TestAnExecutableInputIsReadAsNameThenFile is WSL-128's command half.
func TestAnExecutableInputIsReadAsNameThenFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(f, []byte("#!/bin/sh\r\ntrue\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	j := testJobFlags()
	j.inputs, j.inputExecs = stringList{"data=" + f}, stringList{"run.sh=" + f}
	got, err := j.jobInputs(toolkit.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Exec || !got[1].Exec || got[1].Name != "run.sh" || got[1].Path != f {
		t.Errorf("inputs = %+v; want the plain one, then the executable one", got)
	}
	for _, bad := range []string{"noequals", "run.sh=", "../x=" + f} {
		j.inputExecs = stringList{bad}
		if _, err := j.jobInputs(toolkit.DefaultConfig()); err == nil {
			t.Errorf("--input-exec %q was accepted", bad)
		}
	}
	// ⭐ The mark reaches the helper with the bytes.
	wire, err := helperInputs(got)
	if err != nil || len(wire) != 2 || wire[0].Exec || !wire[1].Exec {
		t.Errorf("the helper is sent %+v, %v", wire, err)
	}
	// ⚠ One name is one input, across both flags.
	j.inputs, j.inputExecs = stringList{"run.sh=" + f}, stringList{"run.sh=" + f}
	if _, err := j.jobInputs(toolkit.DefaultConfig()); err == nil {
		t.Error("one name given as --input and --input-exec was accepted")
	}
}

// TestCreateUserSudoAndCapabilitiesAreReadWhereTheyAreTyped: --create-user and
// --sudo without --user are refused by check, and each --cap-add is read into
// podman's spelling or refused. WSL-123, WSL-130.
func TestCreateUserSudoAndCapabilitiesAreReadWhereTheyAreTyped(t *testing.T) {
	for _, args := range [][]string{{"--create-user"}, {"--sudo"}, {"--user", "1000", "--create-user"}} {
		j := parsedJob(t, args...)
		if err := j.check(); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
	ok := parsedJob(t, "--user", "dev", "--create-user", "--sudo")
	if err := ok.check(); err != nil {
		t.Errorf("--user dev --create-user --sudo was refused: %v", err)
	}
	j := parsedJob(t, "--cap-add", "net_admin", "--cap-add", "ALL")
	got, err := j.jobCapabilities()
	if err != nil || strings.Join(got, " ") != "CAP_NET_ADMIN ALL" {
		t.Errorf("the capabilities read as %v, %v", got, err)
	}
	bad := parsedJob(t, "--cap-add", "NET_WIZARD")
	if _, err := bad.jobCapabilities(); err == nil {
		t.Error("a capability the kernel does not name was accepted")
	}
}

// TestACacheIsReportedAndNarrowedToTheJobThatNamesIt: `resources` names each
// cache with its size and what removes it, and --job keeps the caches that
// job's container names. WSL-122.
func TestACacheIsReportedAndNarrowedToTheJobThatNamesIt(t *testing.T) {
	var rep toolkit.ResourceReport
	rep.Owned.Caches = []toolkit.CacheVolume{
		{Name: "cargo", Volume: "wtk-cache-cargo", Bytes: 2048, Containers: []string{"wtk-0123456789abcdef"}},
		{Name: "pip", Volume: "wtk-cache-pip", Bytes: -1},
		{Name: "other", Volume: "wtk-cache-other", Bytes: 1, Containers: []string{"wtk-ffffffffffffffff"}},
	}
	var b strings.Builder
	if err := toolkit.RenderResources(&b, rep); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"cache             2.0 KiB      cargo in volume wtk-cache-cargo, named by wtk-0123456789abcdef", "not measured", "gc --caches --apply"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("the report does not carry %q:\n%s", want, b.String())
		}
	}
	narrowed := narrowResourcesToJob(rep, "0123456789abcdef")
	if len(narrowed.Owned.Caches) != 1 || narrowed.Owned.Caches[0].Name != "cargo" {
		t.Errorf("--job kept the caches %+v", narrowed.Owned.Caches)
	}
}

// TestTheConfigReportNamesTheProjectAndTheFloor: `config --json` gains project
// and requires where there are, and keeps its shape where there are not.
// WSL-129, WSL-131.
func TestTheConfigReportNamesTheProjectAndTheFloor(t *testing.T) {
	project := lendingInstance(t, `{"schema":"wsl-toolkit-config/1","requires":"1.0","jobs":{"image":"alpine","workspace":"..","exclude":["x"]}}`)
	report := func() map[string]any {
		t.Helper()
		var doc map[string]any
		out := captureStdout(t, func() {
			if code, err := cmdConfig([]string{"--json"}); code != exitOK || err != nil {
				t.Errorf("config --json answered %d, %v", code, err)
			}
		})
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("config --json is not JSON: %v\n%s", err, out)
		}
		return doc
	}
	doc := report()
	if doc["project"] != project || doc["requires"] != "1.0" {
		t.Errorf("the report names project %v and requires %v", doc["project"], doc["requires"])
	}
	// ⭐ Each key the project lent is named beside its file.
	if lends, _ := json.Marshal(doc["project_lends"]); string(lends) != `["jobs.workspace","jobs.exclude","jobs.image","requires"]` {
		t.Errorf("the report names the lent keys as %s", lends)
	}
	if _, ok := doc["project_held"]; ok {
		t.Errorf("the report still names held keys: %v", doc["project_held"])
	}
	if err := os.Remove(project); err != nil {
		t.Fatal(err)
	}
	doc = report()
	if _, ok := doc["project"]; ok {
		t.Errorf("a report with no project carries the key: %v", doc["project"])
	}
	if _, ok := doc["project_lends"]; ok {
		t.Errorf("a report with no project names lent keys: %v", doc["project_lends"])
	}
	if _, ok := doc["requires"]; ok {
		t.Errorf("a report with no floor carries the key: %v", doc["requires"])
	}
}

// lendingInstance selects a scratch instance with its own file, standing in a
// working tree whose wsl-toolkit.json holds project, and answers that file.
func lendingInstance(t *testing.T, project string) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "instances", "f2")
	tree := t.TempDir()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte(`{"schema":"wsl-toolkit-config/1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tree, toolkit.WorkingConfigName)
	if err := os.WriteFile(path, []byte(project), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WSL_TOOLKIT_HOME", home)
	previous := toolkit.SelectedInstance
	t.Cleanup(func() { toolkit.SelectedInstance = previous })
	toolkit.SelectedInstance = toolkit.Instance{Name: "f2", Distro: "wsl-toolkit-f2", Home: home}
	was, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tree); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(was) })
	return path
}

// captureStderr is what fn writes to stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	done := make(chan struct{})
	go func() { _, _ = buf.ReadFrom(r); close(done) }()
	fn()
	_ = w.Close()
	os.Stderr = old
	<-done
	return buf.String()
}

// TestAJobTakesTheOlderKeysALendingProjectWrites is ruling 34. Under a named
// instance a project that writes jobs.exclude, container_lifecycle and
// gitignore had them ignored, with a note. A job now takes them, and `config`
// names each key beside the file.
func TestAJobTakesTheOlderKeysALendingProjectWrites(t *testing.T) {
	project := lendingInstance(t, `{"schema":"wsl-toolkit-config/1","jobs":{"image":"alpine","exclude":["x"],"container_lifecycle":"ephemeral","gitignore":true}}`)
	cfg, err := toolkit.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	j := parsedJob(t)
	said := captureStderr(t, func() {
		if err := j.applyConfig(cfg); err != nil {
			t.Error(err)
		}
	})
	if j.lifecycle != toolkit.ContainerEphemeral || !j.gitignore || strings.Join(j.excludes, " ") != "x" {
		t.Errorf("a job under a lending project ran %s, gitignore %v, excluding %v", j.lifecycle, j.gitignore, j.excludes)
	}
	if strings.Contains(said, "does not take") {
		t.Errorf("a job under a lending project still says a key is held:\n%s", said)
	}
	said = captureStderr(t, func() {
		_ = captureStdout(t, func() {
			if code, err := cmdConfig(nil); code != exitOK || err != nil {
				t.Errorf("config answered %d, %v", code, err)
			}
		})
	})
	want := project + " lends this instance jobs.container_lifecycle, jobs.exclude, jobs.gitignore, jobs.image"
	if !strings.Contains(said, want) {
		t.Errorf("config under a lending project does not say %q:\n%s", want, said)
	}
}

// TestGcCachesIsTheCachesAlone is WSL-122: beside a flag that chooses among jobs
// or adds images, --caches is refused with exit 2 before anything is read.
func TestGcCachesIsTheCachesAlone(t *testing.T) {
	for _, extra := range [][]string{
		{"--job", "0123456789abcdef"}, {"--workspace", "."}, {"--images"}, {"--older-than", "1h"}, {"--include-live"},
	} {
		code, err := cmdGC(context.Background(), append([]string{"--caches"}, extra...))
		if code != exitCannot || err == nil || !strings.Contains(err.Error(), extra[0]) {
			t.Errorf("gc --caches %v answered %d, %v", extra, code, err)
		}
	}
}

// TestAFloorIsExitTwoFromEveryCommandThatReadsTheConfiguration is WSL-129: a
// configuration whose requires names a newer build refuses each command that
// reads it with exit 2, whatever that command answers otherwise.
func TestAFloorIsExitTwoFromEveryCommandThatReadsTheConfiguration(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	t.Setenv(toolkit.InstanceEnv, "")
	tree := t.TempDir()
	body := `{"schema":"wsl-toolkit-config/1","requires":"999.0.0"}`
	if err := os.WriteFile(filepath.Join(tree, toolkit.WorkingConfigName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tree); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	for _, args := range [][]string{
		{"config"}, {"config", "validate"}, {"run", "--image", "alpine", "-c", "true"},
		{"matrix", "-c", "true"}, {"gc"}, {"resources"}, {"base", "status"}, {"images"},
	} {
		if code := run(context.Background(), args); code != exitCannot {
			t.Errorf("%v answered %d under a floor of 999.0.0, want 2", args, code)
		}
	}
	r := ReadyReport{}
	_, err = toolkit.LoadConfig()
	r.noteConfigRefusal(err)
	if readyVerdict(r) != exitCannot || len(r.Remediation) != 1 || !strings.HasSuffix(r.Remediation[0], "selfupdate") {
		t.Errorf("ready under the floor answers %d with %v", readyVerdict(r), r.Remediation)
	}
	r = ReadyReport{}
	r.noteConfigRefusal(os.ErrInvalid)
	if readyVerdict(r) != exitFailed || r.Remediation[0] != toolkit.ToolCommand("config") {
		t.Errorf("ready over another refusal answers %d with %v", readyVerdict(r), r.Remediation)
	}
}
