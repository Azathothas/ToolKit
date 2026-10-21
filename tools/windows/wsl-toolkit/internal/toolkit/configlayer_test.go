// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// everyBool sets every boolean field of a struct to v and answers the JSON key
// of each, so a boolean added to the struct later is walked without a list.
func everyBool(t *testing.T, ptr any, v bool) []string {
	t.Helper()
	rv := reflect.ValueOf(ptr).Elem()
	var keys []string
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		if f.Kind() != reflect.Bool {
			continue
		}
		f.SetBool(v)
		key, _, _ := strings.Cut(rv.Type().Field(i).Tag.Get("json"), ",")
		keys = append(keys, key)
	}
	return keys
}

// TestABooleanAFileLeavesOutKeepsWhatIsBeneathIt is WSL-161. The loader copied
// every boolean a file decoded, named or not, so a file that left `systemd` out
// read as a file that turned it off. Every boolean the loader copies is set true
// beneath a file that names none of them, and must stay true; a file naming each
// one false must still turn it off.
func TestABooleanAFileLeavesOutKeepsWhatIsBeneathIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	restore := chdir(t, t.TempDir())
	defer restore()
	previous := compiledDefaults
	t.Cleanup(func() { compiledDefaults = previous })
	var baseKeys, jobKeys []string
	compiledDefaults = func() Config {
		c := previous()
		baseKeys = everyBool(t, &c.Base, true)
		jobKeys = everyBool(t, &c.Jobs, true)
		return c
	}
	file := filepath.Join(home, "config.json")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	write(`{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit","image":"docker.io/library/alpine:latest","user":"toolkit"}}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(baseKeys) < 2 || len(jobKeys) < 1 {
		t.Fatalf("the walk found base %v and jobs %v, and the loader copies more booleans than that", baseKeys, jobKeys)
	}
	check := func(label string, ptr any, want bool) {
		t.Helper()
		rv := reflect.ValueOf(ptr).Elem()
		for i := 0; i < rv.NumField(); i++ {
			if f := rv.Field(i); f.Kind() == reflect.Bool && f.Bool() != want {
				t.Errorf("%s: %s.%s is %v, want %v", label, rv.Type().Name(), rv.Type().Field(i).Name, f.Bool(), want)
			}
		}
	}
	check("a file that leaves every boolean out", &cfg.Base, true)
	check("a file that leaves every boolean out", &cfg.Jobs, true)

	// ⚠ AND NAMING ONE STILL WINS, false included.
	base := map[string]any{"name": "wsl-toolkit", "image": "docker.io/library/alpine:latest", "user": "toolkit"}
	for _, k := range baseKeys {
		base[k] = false
	}
	jobs := map[string]any{}
	for _, k := range jobKeys {
		jobs[k] = false
	}
	body, err := json.Marshal(map[string]any{"schema": ConfigSchema, "base": base, "jobs": jobs})
	if err != nil {
		t.Fatal(err)
	}
	write(string(body))
	cfg, err = LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	check("a file that names every boolean false", &cfg.Base, false)
	check("a file that names every boolean false", &cfg.Jobs, false)
}

// namedInstance selects a scratch instance whose own file holds own, standing
// in a working tree whose wsl-toolkit.json holds project, and answers the two
// paths.
func namedInstance(t *testing.T, own, project string) (ownFile, projectFile string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "instances", "f2")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	tree := t.TempDir()
	inner := filepath.Join(tree, "src", "deep")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	ownFile, projectFile = filepath.Join(home, "config.json"), filepath.Join(tree, WorkingConfigName)
	for path, body := range map[string]string{ownFile: own, projectFile: project} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	restore := chdir(t, inner)
	t.Cleanup(restore)
	previous, previousExplicit := SelectedInstance, ExplicitConfigPath
	t.Cleanup(func() { SelectedInstance, ExplicitConfigPath = previous, previousExplicit })
	t.Setenv("WSL_TOOLKIT_HOME", home)
	SelectedInstance = Instance{Name: "f2", Distro: "wsl-toolkit-f2", Home: home}
	ExplicitConfigPath = ""
	return ownFile, projectFile
}

// sameDir compares two host directories the way this host's filesystem does.
func sameDir(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// TestAProjectLendsItsJobsToANamedInstance is WSL-131. A named instance's own
// file wins for the base, and before this the working tree's file was then
// ignored whole, so a project's image, timeout and environment reached no job.
// It lends its jobs key by key and its floor; a key it leaves out keeps the
// instance's, a boolean included (WSL-161); and a write stores the instance's own.
func TestAProjectLendsItsJobsToANamedInstance(t *testing.T) {
	ownFile, projectFile := namedInstance(t,
		`{"schema":"wsl-toolkit-config/1","base":{"image":"docker.io/library/alpine:latest"},"jobs":{"container_lifecycle":"persistent","gitignore":true,"timeout":"20m","exclude":["own"]}}`,
		`{"schema":"wsl-toolkit-config/1","requires":"1.0.0","base":{"name":"wsl-toolkit","image":"docker.io/library/debian:latest"},"jobs":{"container_lifecycle":"ephemeral","image":"docker.io/library/rust:1","timeout":"45m","env":{"A":"1"},"workspace":"..","env_file":"job.env","cache":["cargo:/usr/local/cargo"]}}`)
	src, err := ResolveConfig()
	if err != nil {
		t.Fatal(err)
	}
	if src.Path != ownFile || src.Project != projectFile || len(src.Searched) != 1 {
		t.Fatalf("the search resolved %q, found the project %q and searched %v; want the instance's own, the project, and the order unchanged", src.Path, src.Project, src.Searched)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	// ⛔ The base is the instance's: the project's names another distribution
	// and another image, and neither is read.
	if cfg.Base.Name != "wsl-toolkit-f2" || cfg.Base.Image != "docker.io/library/alpine:latest" {
		t.Errorf("the base came from the project: %s from %s", cfg.Base.Name, cfg.Base.Image)
	}
	j := cfg.Jobs
	if j.ContainerLifecycle != ContainerEphemeral || j.Image != "docker.io/library/rust:1" || j.Timeout != "45m" || j.Env["A"] != "1" || len(j.Cache) != 1 {
		t.Errorf("the project's jobs did not reach the instance: %+v", j)
	}
	if !j.Gitignore || strings.Join(j.Exclude, " ") != "own" {
		t.Errorf("a key the project leaves out lost the instance's value: gitignore %v, exclude %v", j.Gitignore, j.Exclude)
	}
	if cfg.Requires != "1.0.0" || cfg.Project() != projectFile {
		t.Errorf("requires %q from %q; want the project's floor and its file", cfg.Requires, cfg.Project())
	}
	// ⭐ A relative path resolves against the file that named it.
	if got := cfg.JobsDir("workspace"); !sameDir(got, filepath.Dir(projectFile)) {
		t.Errorf("jobs.workspace resolves against %q, want the project's directory", got)
	}
	if got := cfg.JobsDir("exclude"); !sameDir(got, filepath.Dir(ownFile)) {
		t.Errorf("a key only the instance wrote anchors at %q, want the instance's directory", got)
	}
	// ⛔ A write stores the instance's own jobs, not what the project lent.
	if err := cfg.Write(); err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(ownFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(written), "rust") || strings.Contains(string(written), "requires") || !strings.Contains(string(written), `"20m"`) {
		t.Errorf("the instance's file was written with the project's jobs:\n%s", written)
	}
	if err := cfg.WriteTo(ownFile); err != nil {
		t.Fatal(err)
	}
	if written, _ := os.ReadFile(ownFile); strings.Contains(string(written), "rust") {
		t.Errorf("a grant's write stored the project's jobs:\n%s", written)
	}
}

// TestAProjectThatIsTheConfigurationLendsNothing holds the other orders as they
// were: with no named instance the working tree's file is the one read, whole.
func TestAProjectThatIsTheConfigurationLendsNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	tree := t.TempDir()
	body := `{"schema":"wsl-toolkit-config/1","jobs":{"image":"docker.io/library/rust:1"}}`
	if err := os.WriteFile(filepath.Join(tree, WorkingConfigName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := chdir(t, tree)
	defer restore()
	src, err := ResolveConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if src.Project != "" || cfg.Project() != "" || cfg.Jobs.Image != "docker.io/library/rust:1" {
		t.Errorf("the file the search resolved also lent itself: project %q, %q, image %q", src.Project, cfg.Project(), cfg.Jobs.Image)
	}
}

// TestEveryJobFieldAProjectWritesReachesTheInstance holds the one copy both
// layers share: every JobConfig field set in a working tree's file reaches a
// named instance's configuration.
func TestEveryJobFieldAProjectWritesReachesTheInstance(t *testing.T) {
	jobs := JobConfig{
		ContainerLifecycle: ContainerEphemeral, Platform: "linux/amd64", Workspace: ".",
		Exclude: []string{"target"}, Gitignore: true, Image: "alpine", Timeout: "5m",
		Env: map[string]string{"A": "1"}, EnvFile: "job.env", User: "1000:1000",
		MaxBytes: 4096, MaxEntries: 9, Cache: []string{"pip:/root/.cache/pip"},
	}
	v := reflect.ValueOf(jobs)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			t.Fatalf("the fixture leaves jobs.%s empty, so this case cannot see it dropped", v.Type().Field(i).Name)
		}
	}
	body, err := json.Marshal(map[string]any{"schema": ConfigSchema, "jobs": jobs})
	if err != nil {
		t.Fatal(err)
	}
	namedInstance(t, `{"schema":"wsl-toolkit-config/1"}`, string(body))
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Jobs, jobs) {
		t.Fatalf("the instance took the project's jobs as\n %+v\nwant\n %+v", cfg.Jobs, jobs)
	}
}

// TestAFloorNewerThanThisBuildIsRefusedNamingTheRemedy is WSL-129: a
// configuration's requires refuses an older build with ErrVersionFloor, naming
// this build, the floor, the file and selfupdate, before the decode can refuse
// a key a newer build added.
func TestAFloorNewerThanThisBuildIsRefusedNamingTheRemedy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	// ⚠ A cleanup, not a defer: namedInstance below moves the directory again,
	// and cleanups run in reverse, so the two restore in order.
	t.Cleanup(chdir(t, t.TempDir()))
	file := filepath.Join(home, "config.json")
	load := func(body string) error {
		t.Helper()
		if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := LoadConfig()
		return err
	}
	err := load(`{"schema":"wsl-toolkit-config/1","requires":"999.1.0","jobs":{"a_key_from_later":true}}`)
	if !errors.Is(err, ErrVersionFloor) {
		t.Fatalf("a floor of 999.1.0 answered %v", err)
	}
	for _, want := range []string{"999.1.0", Version, file, "wsl-toolkit selfupdate"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q:\n%v", want, err)
		}
	}
	if err := load(`{"schema":"wsl-toolkit-config/1","requires":"v1.0"}`); err != nil {
		t.Errorf("a floor this build meets was refused: %v", err)
	}
	if err := load(`{"schema":"wsl-toolkit-config/1","requires":"` + Version + `"}`); err != nil {
		t.Errorf("a floor equal to this build was refused: %v", err)
	}
	for _, bad := range []string{`"seven"`, `"7.1.0.0"`, `7`, `"7..1"`} {
		err := load(`{"schema":"wsl-toolkit-config/1","requires":` + bad + `}`)
		if err == nil || errors.Is(err, ErrVersionFloor) || !strings.Contains(err.Error(), "requires") {
			t.Errorf("requires %s answered %v, want a refusal of the value", bad, err)
		}
	}
	// ⚠ A floor a named instance takes from a working tree refuses too.
	namedInstance(t, `{"schema":"wsl-toolkit-config/1"}`, `{"schema":"wsl-toolkit-config/1","requires":"999.0"}`)
	if _, err := LoadConfig(); !errors.Is(err, ErrVersionFloor) {
		t.Errorf("a project's floor under a named instance answered %v", err)
	}
}

// TestAnUnknownKeyNamesANewerBuildAndSelfupdate is WSL-129's other half: every
// build refuses a key it does not know, so the refusal says a newer build may
// have written the file, and names the remedy.
func TestAnUnknownKeyNamesANewerBuildAndSelfupdate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	restore := chdir(t, t.TempDir())
	defer restore()
	file := filepath.Join(home, "config.json")
	if err := os.WriteFile(file, []byte(`{"schema":"wsl-toolkit-config/1","jobs":{"a_key_from_later":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("an unknown key was accepted")
	}
	for _, want := range []string{`unknown field "a_key_from_later"`, "newer build", Version, "wsl-toolkit selfupdate"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q:\n%v", want, err)
		}
	}
	// ⚠ A file that is broken in another way is not blamed on a newer build.
	if err := os.WriteFile(file, []byte(`{"schema":"wsl-toolkit-config/1","jobs":{"gitignore":"yes"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(); err == nil || strings.Contains(err.Error(), "newer build") {
		t.Errorf("a wrongly typed value answered %v", err)
	}
}

// TestJobDefaultsAreHeldWhereTheyAreWritten refuses each job default that
// could not mean what it says, at the point the file is read. WSL-131.
func TestJobDefaultsAreHeldWhereTheyAreWritten(t *testing.T) {
	refused := map[string]JobConfig{
		"jobs.image":       {Image: "alpine:latest"},
		"jobs.timeout":     {Timeout: "soon"},
		"negative":         {Timeout: "-1m"},
		"jobs.env":         {Env: map[string]string{"BAD-NAME": "x"}},
		"jobs.env_file":    {EnvFile: "  "},
		"white space":      {User: "dev ops"},
		"jobs.max_bytes":   {MaxBytes: -1},
		"jobs.max_entries": {MaxEntries: -1},
		"jobs.cache":       {Cache: []string{"Bad:/x"}},
	}
	for want, j := range refused {
		if err := validateJobDefaults(j, BuiltinImages); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v answered %v, want a refusal naming %q", j, err, want)
		}
	}
	ok := JobConfig{Image: "alpine", Timeout: "0", Env: map[string]string{"A_1": ""}, User: "1000:1000", MaxBytes: 1, MaxEntries: 1, Cache: []string{"a:/a"}}
	if err := validateJobDefaults(ok, BuiltinImages); err != nil {
		t.Errorf("usable defaults were refused: %v", err)
	}
	// ⭐ And a configuration holds them, and its floor, wherever it came from.
	cfg := DefaultConfig()
	cfg.Jobs.Timeout = "soon"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "jobs.timeout") {
		t.Errorf("a configuration with jobs.timeout soon validated: %v", err)
	}
	cfg = DefaultConfig()
	cfg.Requires = "seven"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "requires") {
		t.Errorf("a configuration with requires seven validated: %v", err)
	}
}

// TestAGrantsSourceCrossesTheHelperResolved is WSL-131's relative source: the
// file's path does not cross the helper protocol, so a relative source sent as
// written resolved against the helper's working directory.
func TestAGrantsSourceCrossesTheHelperResolved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	project := filepath.Join(t.TempDir(), "proj")
	if err := os.MkdirAll(filepath.Join(project, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schema":"wsl-toolkit-config/1","base":{"automount":"off","interop":"off","mounts":[{"source":"src","target":"/workspaces/src"}]}}`
	if err := os.WriteFile(filepath.Join(project, WorkingConfigName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	restore := chdir(t, project)
	defer restore()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	// ⚠ What a client sends is what newHelperClient kept.
	wire, err := json.Marshal(newHelperClient(HelperEndpoint{}, cfg).Config())
	if err != nil {
		t.Fatal(err)
	}
	var helper Config
	if err := json.Unmarshal(wire, &helper); err != nil {
		t.Fatal(err)
	}
	mounts, err := helper.ResolvedBaseMounts()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(project, "src"))
	if len(mounts) != 1 || !sameDir(mounts[0].Source, want) || mounts[0].Missing {
		t.Fatalf("the helper resolved the grant as %+v, want %s", mounts, want)
	}
}
