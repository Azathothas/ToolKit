// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ⭐ A FAKE wsl.exe, AND IT IS THIS TEST BINARY. A Wsl whose Path names the
// binary that is running the suite reaches init below in a child process, which
// answers the call from a state file and exits before the testing package reads
// a flag. So a case drives a whole build, a removal or a job through the real
// code above Exec, on any host, with no WSL at all. WSL-150 asked for it: a build
// that fails after its import and before its marker had no deterministic case.
//
// ⚠ It answers by what a script SAYS, not by running it. A script's own last
// line is the one each caller already checks for, so the fake prints that line.

// buildFakeWslEnv names the directory the fake keeps its state in. Set, this binary
// is the fake.
const buildFakeWslEnv = "WSL_TOOLKIT_BUILD_FAKE_WSL"

func init() {
	if dir := os.Getenv(buildFakeWslEnv); dir != "" {
		os.Exit(buildFakeWslMain(dir, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
}

// buildFakeState is what the fake remembers between calls.
type buildFakeState struct {
	Distros map[string]*buildFakeDistro `json:"distros"`
	// FailImport registers the distribution and then fails, as an import that
	// broke after WSL had recorded it.
	FailImport bool `json:"fail_import,omitempty"`
	// RefuseImport fails the import before WSL records anything.
	RefuseImport bool `json:"refuse_import,omitempty"`
	// Unchanged makes provisioning and the grants say wsl.conf and the fstab
	// block are as they were, so no restart is owed. WSL-175.
	Unchanged bool `json:"unchanged,omitempty"`
	// VerifyRefusals is how many verifications are refused before they pass.
	VerifyRefusals int `json:"verify_refusals,omitempty"`
	// FailProvision answers the provisioning run with exit 1.
	FailProvision bool `json:"fail_provision,omitempty"`
	// VerifyRefusal is what WSL answers in place of the verifier, where set.
	VerifyRefusal string `json:"verify_refusal,omitempty"`
	// Passwd is what the verifier's account-passwd row says: ok where unset.
	Passwd string `json:"passwd,omitempty"`
	// LoginShell and Bash are what the verifier's account rows say.
	LoginShell string          `json:"login_shell,omitempty"`
	Bash       string          `json:"bash,omitempty"`
	Calls      []buildFakeCall `json:"calls"`
	Stale      buildFakeStale  `json:"stale"`
}

// buildFakeStale is what the stale run state check answers.
type buildFakeStale struct {
	Answer string `json:"answer,omitempty"`
}

type buildFakeDistro struct {
	Name   string `json:"name"`
	Marker string `json:"marker,omitempty"`
	// Dir is the directory WSL's registry names for the disk.
	Dir string `json:"dir,omitempty"`
}

// buildFakeCall is one call: its arguments, the account, and which script it sent.
type buildFakeCall struct {
	Args   []string `json:"args"`
	User   string   `json:"user,omitempty"`
	Script string   `json:"script,omitempty"`
	// LedgerOpenBase is whether the ledger held an open build record for the
	// distribution when an import ran.
	LedgerOpenBase bool `json:"ledger_open_base,omitempty"`
}

func buildFakeLoad(dir string) *buildFakeState {
	st := &buildFakeState{Distros: map[string]*buildFakeDistro{}}
	if b, err := os.ReadFile(filepath.Join(dir, "state.json")); err == nil {
		_ = json.Unmarshal(b, st)
	}
	if st.Distros == nil {
		st.Distros = map[string]*buildFakeDistro{}
	}
	return st
}

func buildFakeSave(dir string, st *buildFakeState) {
	b, _ := json.MarshalIndent(st, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "state.json"), b, 0o600)
}

var buildFakeAssignment = regexp.MustCompile(`(?m)^(TK_[A-Z0-9_]+)='([^']*)'; export `)

// buildFakeScriptKind names a script by the line its caller reads back.
func buildFakeScriptKind(script string) string {
	switch {
	case strings.Contains(script, "printf 'provision-complete\\n'"):
		return "provision"
	case strings.Contains(script, "grants-complete"):
		return "grants"
	case strings.Contains(script, "drives-complete"):
		return "drives"
	case strings.Contains(script, "Prove the base distribution can actually run a rootless container"):
		return "verify"
	case strings.Contains(script, "printf 'run-state current\\n'"):
		return "stale"
	case strings.Contains(script, "apply_login_shell \"$TK_USER\""):
		return "loginshell"
	case strings.Contains(script, "printf 'engines-complete\\n'"):
		return "engines"
	case strings.Contains(script, "binfmt-complete"):
		return "binfmt"
	case strings.Contains(script, "exec podman"):
		return "job"
	case strings.Contains(script, "printf 'removed\\n'"):
		return "remove"
	case strings.Contains(script, `printf '%s\n' "$HOME"`):
		return "home"
	}
	return "other"
}

func buildFakeWslMain(dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	st := buildFakeLoad(dir)
	call := buildFakeCall{Args: args}
	defer func() {
		st.Calls = append(st.Calls, call)
		buildFakeSave(dir, st)
	}()
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "--list":
		for _, d := range st.Distros {
			fmt.Fprintln(stdout, d.Name)
		}
		return 0
	case "--import":
		name := args[1]
		if st.RefuseImport {
			fmt.Fprintln(stderr, "fake: the import was refused")
			return 1
		}
		st.Distros[strings.ToLower(name)] = &buildFakeDistro{Name: name, Dir: args[2]}
		for _, e := range buildFakeOpenLedger() {
			if e.Kind == baseBuildKind && strings.EqualFold(e.ID, name) {
				call.LedgerOpenBase = true
			}
		}
		if st.FailImport {
			fmt.Fprintln(stderr, "fake: the import failed after registering")
			return 1
		}
		return 0
	case "--terminate":
		return 0
	case "--unregister":
		delete(st.Distros, strings.ToLower(args[1]))
		return 0
	case "-d":
	default:
		return 2
	}
	// -d NAME -u USER [--cd DIR] -- ARGV...
	name, user := args[1], args[3]
	call.User = user
	rest := args[4:]
	for len(rest) > 0 && rest[0] != "--" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return 2
	}
	argv := rest[1:]
	d := st.Distros[strings.ToLower(name)]
	if d == nil {
		fmt.Fprintln(stderr, "There is no distribution with the supplied name.")
		return 1
	}
	switch {
	case len(argv) == 2 && argv[0] == "/bin/cat" && argv[1] == IdentityPath:
		call.Script = "identity-read"
		if d.Marker == "" {
			fmt.Fprintln(stderr, "cat: "+IdentityPath+": No such file or directory")
			return 1
		}
		fmt.Fprint(stdout, d.Marker)
		return 0
	case len(argv) >= 5 && argv[0] == "/bin/tar" && argv[len(argv)-1] == "/etc":
		call.Script = "identity-write"
		tr := tar.NewReader(stdin)
		if _, err := tr.Next(); err != nil {
			return 1
		}
		b, _ := io.ReadAll(tr)
		d.Marker = string(b)
		return 0
	case len(argv) >= 1 && argv[0] == "/bin/sh":
	default:
		call.Script = "argv " + strings.Join(argv, " ")
		_, _ = io.Copy(io.Discard, stdin)
		return 0
	}
	raw, _ := io.ReadAll(stdin)
	script := string(raw)
	call.Script = buildFakeScriptKind(script)
	switch call.Script {
	case "provision":
		if st.FailProvision {
			fmt.Fprintln(stderr, "provision: fake failure")
			return 1
		}
		if st.Unchanged {
			fmt.Fprintln(stdout, "wsl-conf unchanged")
		}
		fmt.Fprintln(stdout, "provision-complete")
	case "grants":
		if st.Unchanged {
			fmt.Fprintln(stdout, "fstab unchanged")
		}
		fmt.Fprintln(stdout, "grants-complete")
	case "drives":
		fmt.Fprintln(stdout, "  * drive mount points removed: 0")
		fmt.Fprintln(stdout, "drives-complete")
	case "verify":
		if st.VerifyRefusals > 0 {
			st.VerifyRefusals--
			fmt.Fprintln(stderr, "Error: the fake engine is not answering yet")
			return 1
		}
		if st.VerifyRefusal != "" {
			fmt.Fprintln(stderr, st.VerifyRefusal)
			return 1
		}
		env := map[string]string{}
		for _, m := range buildFakeAssignment.FindAllStringSubmatch(script, -1) {
			env[m[1]] = m[2]
		}
		shell := valueOr(st.LoginShell, "/bin/bash")
		fmt.Fprintf(stdout, "automount %s\naccount-passwd %s\nlogin-shell %s\nlogin-bash %s\nengine podman version 9.9.9\n%s%s\n",
			env["TK_AUTOMOUNT"], valueOr(st.Passwd, "ok"), shell, valueOr(st.Bash, "none"), env["TK_M1"], env["TK_M2"])
	case "stale":
		fmt.Fprintln(stdout, valueOr(st.Stale.Answer, "run-state current"))
	case "loginshell":
		fmt.Fprintln(stdout, "login-shell /bin/bash")
	case "engines":
		// ⭐ Every ensure that owns the base runs engines.sh, and reads its last
		// line. WSL-130.
		fmt.Fprintln(stdout, "engines-complete")
	case "binfmt":
		fmt.Fprintln(stdout, "binfmt-complete")
	case "remove":
		fmt.Fprintln(stdout, "removed")
	case "home":
		fmt.Fprintln(stdout, "/var/lib/agent")
	}
	return 0
}

// buildFakeOpenLedger is the state directory's open records, as the fake reads them
// through the environment its parent gave it.
func buildFakeOpenLedger() []LedgerEntry {
	home := os.Getenv("WSL_TOOLKIT_HOME")
	if home == "" {
		return nil
	}
	open, _ := ledgerAt(home).Open()
	return open
}

// buildFakeWsl is a running fake: its handle, its directory and its state.
type buildFakeWsl struct {
	t   *testing.T
	dir string
	w   *Wsl
}

// newBuildFakeWsl makes a fake wsl.exe for one case, and a state directory for the
// tool beside it. ⛔ Every way this tool reaches the operator's own machine is
// pointed into the case's temporary directory first.
func newBuildFakeWsl(t *testing.T) *buildFakeWsl {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv(buildFakeWslEnv, dir)
	t.Setenv("WSL_TOOLKIT_HOME", filepath.Join(dir, "home"))
	t.Setenv(InstanceRootEnv, filepath.Join(dir, "home"))
	t.Setenv(SSHDirEnv, filepath.Join(dir, "ssh"))
	t.Setenv(BinDirEnv, filepath.Join(dir, "bin"))
	return &buildFakeWsl{t: t, dir: dir, w: &Wsl{Path: self}}
}

// home is the tool's state directory for the case.
func (f *buildFakeWsl) home() string { return filepath.Join(f.dir, "home") }

func (f *buildFakeWsl) state() *buildFakeState { return buildFakeLoad(f.dir) }

func (f *buildFakeWsl) update(change func(*buildFakeState)) {
	st := buildFakeLoad(f.dir)
	change(st)
	buildFakeSave(f.dir, st)
}

// base is a Base on the fake, as NewBase would make one on a real host.
func (f *buildFakeWsl) base(cfg Config, log func(string)) *Base {
	if log == nil {
		log = func(string) {}
	}
	if err := os.MkdirAll(f.home(), 0o700); err != nil {
		f.t.Fatal(err)
	}
	b := &Base{cfg: cfg, home: f.home(), wsl: f.w, log: log}
	// ⭐ WSL's registry is the fake's: each registered name and its disk's
	// directory, so an unregister is seen by the next read.
	b.disks = func() (map[string]string, error) {
		out := map[string]string{}
		for _, d := range f.state().Distros {
			out[d.Name] = d.Dir
		}
		return out, nil
	}
	return b
}

// buildFakeConfig is a configuration the fake can build: no grants, no adapters.
func buildFakeConfig(name, automount string) Config {
	return Config{Base: BaseConfig{
		Name: name, Image: "docker.io/library/alpine:latest", User: "agent",
		Automount: automount, Interop: BaseInteropOff, Toolset: BaseToolsetNone,
	}}
}

// scripts is the kind of every call, in order, with its account.
func (s *buildFakeState) scripts() []string {
	var out []string
	for _, c := range s.Calls {
		switch {
		case c.Script != "":
			out = append(out, c.Script+"@"+c.User)
		case len(c.Args) > 0:
			out = append(out, c.Args[0])
		}
	}
	return out
}

// TestTheBuildFakeWslAnswersAsTheCodeAboveItExpects holds the fake to the shape the
// cases below lean on, so a fake that drifted fails here rather than inside a
// case about something else.
func TestTheBuildFakeWslAnswersAsTheCodeAboveItExpects(t *testing.T) {
	f := newBuildFakeWsl(t)
	if names, err := f.w.listNames(t.Context()); err != nil || len(names) != 0 {
		t.Fatalf("an empty fake listed %v, %v", names, err)
	}
	f.update(func(s *buildFakeState) { s.Distros["wsl-toolkit-fake"] = &buildFakeDistro{Name: "wsl-toolkit-fake"} })
	if ok, err := f.w.Exists(t.Context(), "wsl-toolkit-fake"); err != nil || !ok {
		t.Fatalf("a registered fake distribution reads exists %v, %v", ok, err)
	}
	if _, err := f.w.ReadIdentity(t.Context(), "wsl-toolkit-fake"); err == nil || !strings.Contains(err.Error(), ErrNoIdentity.Error()) {
		t.Fatalf("an unstamped fake distribution answered its identity with %v", err)
	}
	if err := f.w.WriteIdentity(t.Context(), "wsl-toolkit-fake", Identity{Image: "x", User: "agent"}); err != nil {
		t.Fatal(err)
	}
	if id, err := f.w.ReadIdentity(t.Context(), "wsl-toolkit-fake"); err != nil || id.User != "agent" {
		t.Fatalf("a stamped fake distribution answered %+v, %v", id, err)
	}
}
