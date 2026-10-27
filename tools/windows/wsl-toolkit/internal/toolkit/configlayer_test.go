// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
)

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
		if err := forksafe.WriteFile(path, []byte(body), 0o600); err != nil {
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
// It lends every jobs key it writes and its floor, ruling 34. A key it leaves
// out keeps the instance's value. A write stores the instance's own.
func TestAProjectLendsItsJobsToANamedInstance(t *testing.T) {
	ownFile, projectFile := namedInstance(t,
		`{"schema":"wsl-toolkit-config/1","base":{"image":"docker.io/library/alpine:latest"},"jobs":{"container_lifecycle":"persistent","gitignore":true,"timeout":"20m","user":"own","exclude":["own"]}}`,
		`{"schema":"wsl-toolkit-config/1","requires":"1.0.0","base":{"name":"wsl-toolkit","image":"docker.io/library/debian:latest"},"jobs":{"container_lifecycle":"ephemeral","image":"docker.io/library/rust:1","timeout":"45m","env":{"A":"1"},"workspace":"..","exclude":["project"],"env_file":"job.env","cache":["cargo:/usr/local/cargo"]}}`)
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
	if j.Image != "docker.io/library/rust:1" || j.Timeout != "45m" || j.Env["A"] != "1" || len(j.Cache) != 1 || j.EnvFile != "job.env" {
		t.Errorf("the project's job defaults did not reach the instance: %+v", j)
	}
	if j.User != "own" {
		t.Errorf("a job default the project leaves out lost the instance's value: user %q", j.User)
	}
	// ⛔ THE OLDER KEYS CROSS TOO, ruling 34. gitignore, which the project
	// leaves out, keeps the instance's true.
	if j.ContainerLifecycle != ContainerEphemeral || !j.Gitignore || strings.Join(j.Exclude, " ") != "project" || j.Workspace != ".." {
		t.Errorf("an older jobs key did not come from the project: %+v", j)
	}
	if cfg.Requires != "1.0.0" || cfg.Project() != projectFile {
		t.Errorf("requires %q from %q; want the project's floor and its file", cfg.Requires, cfg.Project())
	}
	// ⭐ A relative path resolves against the file that named it.
	if got := cfg.JobsDir("env_file"); !sameDir(got, filepath.Dir(projectFile)) {
		t.Errorf("jobs.env_file resolves against %q, want the project's directory", got)
	}
	if got := cfg.JobsDir("workspace"); !sameDir(got, filepath.Dir(projectFile)) {
		t.Errorf("jobs.workspace resolves against %q, want the project's directory", got)
	}
	if got := cfg.JobsDir("gitignore"); !sameDir(got, filepath.Dir(ownFile)) {
		t.Errorf("a key the project does not write anchors at %q, want the instance's directory", got)
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

// TestAProjectThatLendsNothingIsNotRead holds the additive half of WSL-131.
// Under a named instance with its own file, 7.0 never read the working tree's
// file, so a file there that does not parse, carries a key this build does not
// know, or writes no jobs key and no floor, still changes and refuses nothing.
func TestAProjectThatLendsNothingIsNotRead(t *testing.T) {
	for label, project := range map[string]string{
		"base only":      `{"schema":"wsl-toolkit-config/1","base":{"image":"docker.io/library/alpine:latest"}}`,
		"not JSON":       `{"schema":`,
		"an unknown key": `{"schema":"wsl-toolkit-config/1","jobs":{"a_key_from_later":1}}`,
		"another schema": `{"schema":"wsl-toolkit-config/0","base":{"name":"wsl-toolkit"}}`,
		"null defaults":  `{"schema":"wsl-toolkit-config/1","requires":null,"jobs":{"image":null}}`,
	} {
		t.Run(label, func(t *testing.T) {
			namedInstance(t, `{"schema":"wsl-toolkit-config/1","jobs":{"exclude":["own"]}}`, project)
			src, err := ResolveConfig()
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("a working tree's file that lends nothing refused the instance: %v", err)
			}
			if src.Project != "" || cfg.Project() != "" {
				t.Errorf("the file was read as lending: %q, %q", src.Project, cfg.Project())
			}
			def := DefaultConfig().Jobs
			if cfg.Jobs.ContainerLifecycle != def.ContainerLifecycle || cfg.Jobs.Gitignore || cfg.Jobs.Platform != "" ||
				cfg.Jobs.Workspace != "" || strings.Join(cfg.Jobs.Exclude, " ") != "own" {
				t.Errorf("the instance's jobs changed: %+v", cfg.Jobs)
			}
		})
	}
	// ⚠ A file that lends is read strictly, and its fault is the refusal.
	namedInstance(t, `{"schema":"wsl-toolkit-config/1"}`, `{"schema":"wsl-toolkit-config/1","jobs":{"image":"docker.io/library/rust:1","a_key_from_later":1}}`)
	if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), "a_key_from_later") {
		t.Errorf("a lending file with an unknown key answered %v", err)
	}
}

// TestAProjectThatIsTheConfigurationLendsNothing holds the other orders as they
// were: with no named instance the working tree's file is the one read, whole.
func TestAProjectThatIsTheConfigurationLendsNothing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	tree := t.TempDir()
	body := `{"schema":"wsl-toolkit-config/1","jobs":{"image":"docker.io/library/rust:1"}}`
	if err := forksafe.WriteFile(filepath.Join(tree, WorkingConfigName), []byte(body), 0o600); err != nil {
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

// olderOwn is a named instance's own file that sets each older jobs key, so a
// project's value can be told from the instance's.
const olderOwn = `{"schema":"wsl-toolkit-config/1","jobs":{"container_lifecycle":"persistent","platform":"linux/arm64","workspace":"own","exclude":["own"],"gitignore":true,"max_running":3}}`

// TestEachOlderKeyReachesTheInstanceFromAProject is ruling 34: a working tree's
// file lends a named instance container_lifecycle, platform, workspace, exclude
// and gitignore too. 7.1 kept the instance's own and named the key.
func TestEachOlderKeyReachesTheInstanceFromAProject(t *testing.T) {
	for key, c := range map[string]struct {
		value string
		got   func(JobConfig) string
		want  string
	}{
		"container_lifecycle": {`"ephemeral"`, func(j JobConfig) string { return j.ContainerLifecycle }, "ephemeral"},
		"platform":            {`"linux/amd64"`, func(j JobConfig) string { return j.Platform }, "linux/amd64"},
		"workspace":           {`".."`, func(j JobConfig) string { return j.Workspace }, ".."},
		"exclude":             {`["project","target"]`, func(j JobConfig) string { return strings.Join(j.Exclude, " ") }, "project target"},
		"gitignore":           {`false`, func(j JobConfig) string { return strconv.FormatBool(j.Gitignore) }, "false"},
	} {
		t.Run(key, func(t *testing.T) {
			namedInstance(t, olderOwn, `{"schema":"wsl-toolkit-config/1","jobs":{"`+key+`":`+c.value+`}}`)
			cfg, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got := c.got(cfg.Jobs); got != c.want {
				t.Errorf("jobs.%s reached the instance as %q, want the project's %q", key, got, c.want)
			}
			if strings.Join(cfg.ProjectLends(), " ") != "jobs."+key {
				t.Errorf("the project lends %v, want jobs.%s", cfg.ProjectLends(), key)
			}
		})
	}
}

// TestAnExplicitZeroAProjectWritesIsHonoured: gitignore false, exclude [] and
// max_running 0 are values the project states, so the instance's true, list
// and cap give way. A nonzero test would keep the instance's. Ruling 34.
func TestAnExplicitZeroAProjectWritesIsHonoured(t *testing.T) {
	namedInstance(t, olderOwn, `{"schema":"wsl-toolkit-config/1","jobs":{"gitignore":false,"exclude":[],"max_running":0}}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Jobs.Gitignore || len(cfg.Jobs.Exclude) != 0 || cfg.Jobs.MaxRunning != 0 {
		t.Errorf("an explicit zero from the project lost to the instance: gitignore %v, exclude %v, max_running %d",
			cfg.Jobs.Gitignore, cfg.Jobs.Exclude, cfg.Jobs.MaxRunning)
	}
}

// TestAKeyTheProjectDoesNotWriteKeepsTheInstances: a project that writes only
// an image, or writes a key as null, leaves every other key as the instance's
// own file says.
func TestAKeyTheProjectDoesNotWriteKeepsTheInstances(t *testing.T) {
	namedInstance(t, olderOwn, `{"schema":"wsl-toolkit-config/1","jobs":{"image":"alpine","gitignore":null,"exclude":null}}`)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	j := cfg.Jobs
	if j.Image != "alpine" {
		t.Fatalf("the project's image did not reach the instance: %q", j.Image)
	}
	if j.ContainerLifecycle != ContainerPersistent || j.Platform != "linux/arm64" || j.Workspace != "own" ||
		strings.Join(j.Exclude, " ") != "own" || !j.Gitignore || j.MaxRunning != 3 {
		t.Errorf("a key the project does not write lost the instance's value: %+v", j)
	}
	if strings.Join(cfg.ProjectLends(), " ") != "jobs.image" {
		t.Errorf("the project lends %v, want jobs.image alone", cfg.ProjectLends())
	}
}

// TestEveryJobFieldAProjectWritesReachesTheInstance holds the one copy both
// layers share. Every JobConfig field is a jobs key a working tree lends a
// named instance, ruling 34, and it reaches the instance. Where its type can
// tell, an explicit zero reaches it too.
//
// ⛔ A FIELD ADDED LATER MUST BE LENT. The fixture sets every field, so a new one
// left empty fails here, and one set and not in jobKeys fails below.
func TestEveryJobFieldAProjectWritesReachesTheInstance(t *testing.T) {
	jobs := JobConfig{
		ContainerLifecycle: ContainerEphemeral, Platform: "linux/amd64", Workspace: ".",
		Exclude: []string{"target"}, Gitignore: true, Image: "alpine", Timeout: "5m",
		Env: map[string]string{"A": "1"}, EnvFile: "job.env", User: "1000:1000",
		MaxBytes: 4096, MaxEntries: 9, Cache: []string{"pip:/root/.cache/pip"},
		MaxRunning: 2, MaxRunningWait: "30m", Memory: "512m", CPUs: 1.5,
	}
	lent := map[string]bool{}
	for _, k := range jobKeys {
		lent[k] = true
	}
	v := reflect.ValueOf(jobs)
	if v.NumField() != len(jobKeys) {
		t.Fatalf("JobConfig has %d fields and jobKeys lists %d", v.NumField(), len(jobKeys))
	}
	for i := 0; i < v.NumField(); i++ {
		key, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("json"), ",")
		if v.Field(i).IsZero() {
			t.Fatalf("the fixture leaves jobs.%s empty, so this case cannot see it dropped", key)
		}
		if !lent[key] {
			t.Fatalf("jobs.%s is not in jobKeys, so a project cannot lend it", key)
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
	got := reflect.ValueOf(cfg.Jobs)
	for i := 0; i < v.NumField(); i++ {
		key, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("json"), ",")
		if !reflect.DeepEqual(got.Field(i).Interface(), v.Field(i).Interface()) {
			t.Errorf("jobs.%s reached the instance as %v, want %v", key, got.Field(i).Interface(), v.Field(i).Interface())
		}
	}
	// ⭐ An explicit zero over an instance that sets every key. A string's zero
	// is not set, so the instance keeps its own there.
	zeros := map[string]any{}
	for i := 0; i < v.NumField(); i++ {
		key, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("json"), ",")
		switch f := v.Type().Field(i).Type; f.Kind() {
		case reflect.Slice:
			zeros[key] = reflect.MakeSlice(f, 0, 0).Interface()
		case reflect.Map:
			zeros[key] = reflect.MakeMap(f).Interface()
		default:
			zeros[key] = reflect.Zero(f).Interface()
		}
	}
	project, err := json.Marshal(map[string]any{"schema": ConfigSchema, "jobs": zeros})
	if err != nil {
		t.Fatal(err)
	}
	namedInstance(t, string(body), string(project))
	if cfg, err = LoadConfig(); err != nil {
		t.Fatal(err)
	}
	got = reflect.ValueOf(cfg.Jobs)
	for i := 0; i < v.NumField(); i++ {
		key, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("json"), ",")
		field := got.Field(i)
		switch field.Kind() {
		case reflect.String:
			if field.Interface() != v.Field(i).Interface() {
				t.Errorf("jobs.%s written as \"\" replaced the instance's %v with %v", key, v.Field(i).Interface(), field.Interface())
			}
		case reflect.Slice, reflect.Map:
			if field.Len() != 0 {
				t.Errorf("jobs.%s written empty reached the instance as %v", key, field.Interface())
			}
		default:
			if !field.IsZero() {
				t.Errorf("jobs.%s written as its zero reached the instance as %v", key, field.Interface())
			}
		}
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
		if err := forksafe.WriteFile(file, []byte(body), 0o600); err != nil {
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
	if err := forksafe.WriteFile(file, []byte(`{"schema":"wsl-toolkit-config/1","jobs":{"a_key_from_later":1}}`), 0o600); err != nil {
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
	if err := forksafe.WriteFile(file, []byte(`{"schema":"wsl-toolkit-config/1","jobs":{"gitignore":"yes"}}`), 0o600); err != nil {
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
	if err := forksafe.WriteFile(filepath.Join(project, WorkingConfigName), []byte(body), 0o600); err != nil {
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
