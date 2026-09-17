package toolkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func agentConfig(name, model, effort string) Config {
	cfg := DefaultConfig()
	one := BaseAdapter{Name: name, Model: model, Effort: effort}
	cfg.Base.Adapters = []BaseAdapter{one}
	return cfg
}

// ⭐ max IS THE DEFAULT, and it is the default because it is the one word all three
// agents take. A base that names no effort still gets one, because the point of the
// field is that an agent herdr starts begins where the operator meant it to.
func TestEveryAgentAdapterDefaultsToMaxEffort(t *testing.T) {
	for _, name := range []string{"muse", "pi", "omp"} {
		cfg := agentConfig(name, "", "")
		env, err := adapterInstallEnv(cfg, cfg.Base.Adapters[0])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if env["TK_ADAPTER_EFFORT"] != "max" {
			t.Fatalf("%s: TK_ADAPTER_EFFORT = %q, want max", name, env["TK_ADAPTER_EFFORT"])
		}
		if _, ok := env["TK_ADAPTER_MODEL"]; ok {
			t.Fatalf("%s: a configuration naming no model still sent one: %q", name, env["TK_ADAPTER_MODEL"])
		}
	}
}

// ⭐ THE CONFIGURED VALUES REACH THE SCRIPT. A field set in a configuration and never
// passed is the defect base.adapters shipped with once.
func TestAModelAndEffortReachTheInstaller(t *testing.T) {
	cfg := agentConfig("pi", "muse-gateway/muse-spark-1.3-contributor", "high")
	env, err := adapterInstallEnv(cfg, cfg.Base.Adapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if env["TK_ADAPTER_MODEL"] != "muse-gateway/muse-spark-1.3-contributor" {
		t.Fatalf("TK_ADAPTER_MODEL = %q", env["TK_ADAPTER_MODEL"])
	}
	if env["TK_ADAPTER_EFFORT"] != "high" {
		t.Fatalf("TK_ADAPTER_EFFORT = %q", env["TK_ADAPTER_EFFORT"])
	}
}

// ⛔ herdr IS NOT AN AGENT, so a model or an effort on it is refused rather than
// dropped. An adapter that accepts a setting it cannot use is one the operator believes
// is in force.
func TestAModelOnAnAdapterThatInstallsNoAgentIsRefused(t *testing.T) {
	cfg := DefaultConfig()
	withModel := BaseAdapter{Name: "herdr", Model: "muse-spark-1.3-contributor"}
	cfg.Base.Adapters = []BaseAdapter{withModel}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("Validate() = %v, want a refusal naming model", err)
	}
	withEffort := BaseAdapter{Name: "herdr", Effort: "max"}
	cfg.Base.Adapters = []BaseAdapter{withEffort}
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "effort") {
		t.Fatalf("Validate() = %v, want a refusal naming effort", err)
	}
	env, err := adapterInstallEnv(DefaultConfig(), BaseAdapter{Name: "herdr"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := env["TK_ADAPTER_EFFORT"]; ok {
		t.Fatal("herdr was sent an effort, and it installs no agent")
	}
}

// ⛔ EACH AGENT'S VOCABULARY IS ITS OWN, measured from its own help in a base on
// 2026-09-17. A word one agent does not know is refused here rather than sent to it and
// rejected at every start.
func TestEachAgentRefusesAnEffortWordItsOwnCLIDoesNotTake(t *testing.T) {
	cases := []struct {
		adapter string
		word    string
		takes   bool
	}{
		{"muse", "ultra", true},
		{"pi", "ultra", false},
		{"omp", "ultra", false},
		{"pi", "off", true},
		{"muse", "off", false},
		{"omp", "off", false},
		{"omp", "auto", true},
		{"pi", "auto", false},
		{"muse", "auto", false},
		{"muse", "max", true},
		{"pi", "max", true},
		{"omp", "max", true},
	}
	for _, c := range cases {
		err := agentConfig(c.adapter, "", c.word).Validate()
		if c.takes && err != nil {
			t.Fatalf("%s effort %q: %v, want it accepted", c.adapter, c.word, err)
		}
		if !c.takes {
			if err == nil {
				t.Fatalf("%s took effort %q, and its own CLI does not", c.adapter, c.word)
			}
			if !strings.Contains(err.Error(), c.word) {
				t.Fatalf("%s effort %q: the refusal does not name the word: %v", c.adapter, c.word, err)
			}
		}
	}
}

// ⛔ A MODEL ID IS ONE ARGUMENT. It reaches the agent as one, so whitespace in it would
// silently become two and the second would be read as something else.
func TestAModelCarryingWhitespaceIsRefused(t *testing.T) {
	err := agentConfig("muse", "two words", "").Validate()
	if err == nil || !strings.Contains(err.Error(), "whitespace") {
		t.Fatalf("Validate() = %v, want a refusal naming whitespace", err)
	}
}

// museWrapperSource pulls the wrapper out of the adapter that writes it, so this drives
// the SHIPPED text rather than a copy of it that could drift.
func museWrapperSource(t *testing.T) string {
	t.Helper()
	b, err := adapterTree.ReadFile("adapters/muse/install.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	open := "cat > \"$wrapper_tmp\" <<WRAPPER\n"
	i := strings.Index(s, open)
	if i < 0 {
		t.Fatal("the muse adapter no longer writes its wrapper from a heredoc named WRAPPER")
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, "\nWRAPPER\n")
	if j < 0 {
		t.Fatal("the wrapper heredoc has no end")
	}
	return rest[:j+1]
}

// museWrapperFile renders the shipped wrapper with a stub in the launcher's place.
func museWrapperFile(t *testing.T, model, effort, subcommands string) (shell, path string) {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX sh on this machine to drive the wrapper with")
	}
	dir := t.TempDir()
	launcher := filepath.Join(dir, "launcher")
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.NewReplacer(
		"\\$", "$",
		"$TK_USER", "nobody",
		"${TK_ADAPTER_MODEL:-}", model,
		"${TK_ADAPTER_EFFORT:-}", effort,
		"$MUSE_SUBCOMMANDS", subcommands,
		"$LAUNCHER", filepath.ToSlash(launcher),
	).Replace(museWrapperSource(t))
	// ⛔ The account guard is the adapter's, and this machine is not that account, so it
	// is cut rather than worked around. Its absence is a failure, not a skip: a wrapper
	// that stopped guarding would run Muse as anybody.
	g := strings.Index(body, "if [ \"$(id -un)\"")
	if g < 0 {
		t.Fatal("the wrapper no longer guards on the account, and this test cut nothing")
	}
	e := strings.Index(body[g:], "\nfi\n")
	if e < 0 {
		t.Fatal("the account guard has no end")
	}
	body = body[:g] + body[g+e+len("\nfi\n"):]
	path = filepath.Join(dir, "muse")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return sh, path
}

// ⛔ MUSE REFUSES A ROOT OPTION IN FRONT OF A SUBCOMMAND, and refuses a repeated
// --model, both measured on Muse Code 1.3.0 on 2026-09-17: `muse --model M config
// status` exits 2 with `unknown argument config`, and `muse --model a --model b` exits 2
// with `cannot be used multiple times`. So the wrapper reads the line before it adds
// anything, and these are the shapes it has to get right.
func TestTheMuseWrapperPlacesTheDefaultsWhereMuseTakesThem(t *testing.T) {
	const subs = "resume exec config export trace skills plugins sandbox schema serve session-message mcp auth login logout init "
	sh, wrapper := museWrapperFile(t, "muse-spark-1.3-contributor", "max", subs)

	const d = "--model muse-spark-1.3-contributor --reasoning-effort max"
	cases := []struct {
		args []string
		want string
	}{
		// The TUI, which is what herdr starts: the defaults go in front.
		{nil, d},
		// ⚠ Muse takes exactly ONE positional, measured the same day: `muse how do I
		// use exec mode` answers `unknown argument how`. So a prompt is one argument,
		// and a word inside it is never read as a subcommand.
		{[]string{"how do I use exec mode"}, d + " how do I use exec mode"},
		{[]string{"--workspace", "/srv"}, d + " --workspace /srv"},
		// exec and resume take them, and only AFTER the subcommand.
		{[]string{"exec", "--json", "hello"}, "exec " + d + " --json hello"},
		{[]string{"resume", "--last"}, "resume " + d + " --last"},
		{[]string{"--provider", "meta", "exec", "go"}, "--provider meta exec " + d + " go"},
		// ⛔ Every other subcommand takes neither, and `muse login` is the one the
		// operator has to be able to run.
		{[]string{"config", "status"}, "config status"},
		{[]string{"login"}, "login"},
		{[]string{"sandbox", "setup"}, "sandbox setup"},
		{[]string{"auth", "--help"}, "auth --help"},
		// ⛔ A flag the caller passed is never joined by a second.
		{[]string{"--model", "mine"}, "--reasoning-effort max --model mine"},
		{[]string{"--model=mine"}, "--reasoning-effort max --model=mine"},
		{[]string{"--reasoning-effort", "low"}, "--model muse-spark-1.3-contributor --reasoning-effort low"},
		{[]string{"--model", "mine", "--reasoning-effort", "low"}, "--model mine --reasoning-effort low"},
		{[]string{"exec", "--model", "mine", "go"}, "exec --reasoning-effort max --model mine go"},
	}
	for _, c := range cases {
		out, err := exec.Command(sh, append([]string{wrapper}, c.args...)...).Output()
		if err != nil {
			t.Fatalf("muse %v: %v", c.args, err)
		}
		if got := strings.TrimSpace(string(out)); got != c.want {
			t.Fatalf("muse %v\n got: %s\nwant: %s", c.args, got, c.want)
		}
	}
}

// ⛔ NO SUBCOMMAND LIST MEANS NO INJECTION. A wrapper that guessed the list would break
// `muse login` the first time Muse renamed a command, and signing in is the operator's
// only door.
func TestTheMuseWrapperAddsNothingWhenItKnowsNoSubcommands(t *testing.T) {
	sh, wrapper := museWrapperFile(t, "", "", "")
	for _, args := range [][]string{{"login"}, {"exec", "hello"}, nil} {
		out, err := exec.Command(sh, append([]string{wrapper}, args...)...).Output()
		if err != nil {
			t.Fatalf("muse %v: %v", args, err)
		}
		if got := strings.TrimSpace(string(out)); got != strings.TrimSpace(strings.Join(args, " ")) {
			t.Fatalf("with no defaults and no list, muse %v became %q", args, got)
		}
	}
}
