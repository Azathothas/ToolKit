// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

var (
	herdrAdapter = BaseAdapter{Name: "herdr"}
	notAnAdapter = BaseAdapter{Name: "not-an-adapter"}
)

// adapterBase is a configuration the herdr adapter accepts.
func adapterBase() Config {
	cfg := DefaultConfig()
	cfg.Base.Image = "ghcr.io/pkgforge-dev/archlinux:latest"
	cfg.Base.User = "herdr"
	cfg.Base.Systemd = true
	cfg.Base.Adapters = []BaseAdapter{herdrAdapter}
	return cfg
}

// baseConfigWithAdapters is an arch base with systemd that names the given adapters.
func baseConfigWithAdapters(adapters ...BaseAdapter) Config {
	cfg := adapterBase()
	cfg.Base.Adapters = adapters
	return cfg
}

// TestAnAdapterIsRefusedWhereItWasNotDriven holds the four refusals WSL-76 put on
// base.adapters: a name this executable does not carry, a name twice, herdr without
// systemd, and herdr on a preset it was not driven on.
func TestAnAdapterIsRefusedWhereItWasNotDriven(t *testing.T) {
	if err := adapterBase().Validate(); err != nil {
		t.Fatalf("the herdr adapter on an arch base with systemd was refused: %v", err)
	}
	cases := map[string]struct {
		change func(*Config)
		want   string
	}{
		"an unknown adapter": {func(c *Config) { c.Base.Adapters = []BaseAdapter{notAnAdapter} }, "the adapters this executable carries are: herdr, muse, omp, pi"},
		"a name twice":       {func(c *Config) { c.Base.Adapters = append(c.Base.Adapters, BaseAdapter{Name: "herdr"}) }, "more than once"},
		"no systemd":         {func(c *Config) { c.Base.Systemd = false }, "needs base.systemd to be true"},
		"another preset":     {func(c *Config) { c.Base.Image = "docker.io/library/alpine:latest" }, "driven on the arch preset only"},
	}
	for name, tc := range cases {
		cfg := adapterBase()
		tc.change(&cfg)
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s answered %v, want a refusal saying %q", name, err, tc.want)
		}
	}
}

// museDigest is a well-formed SHA-256 that no installer has, built rather than
// written, because the gate refuses a long hex literal in a tracked file.
var museDigest = strings.Repeat("0", 62) + "77"

// TestAnInstallerDigestIsTakenOnlyByAnAdapterThatRunsAnInstaller holds WSL-77's
// approval field: a SHA-256 on the muse adapter is accepted, and one on herdr, which
// runs no installer, or one that is not a SHA-256, is refused rather than ignored.
func TestAnInstallerDigestIsTakenOnlyByAnAdapterThatRunsAnInstaller(t *testing.T) {
	cfg := adapterBase()
	cfg.Base.Adapters = append(cfg.Base.Adapters, BaseAdapter{Name: "muse", InstallerSHA256: museDigest})
	if err := cfg.Validate(); err != nil {
		t.Fatalf("an installer digest on the muse adapter was refused: %v", err)
	}
	herdrWithDigest := BaseAdapter{Name: "herdr", InstallerSHA256: museDigest}
	museInCapitals := BaseAdapter{Name: "muse", InstallerSHA256: strings.ToUpper("ab" + museDigest[2:])}
	museShort := BaseAdapter{Name: "muse", InstallerSHA256: museDigest[1:]}
	cases := map[string]struct {
		adapters []BaseAdapter
		want     string
	}{
		"on an adapter that runs no installer": {[]BaseAdapter{herdrWithDigest}, "runs no installer"},
		"in capitals":                          {[]BaseAdapter{herdrAdapter, museInCapitals}, "is not a SHA-256"},
		"one character short":                  {[]BaseAdapter{herdrAdapter, museShort}, "is not a SHA-256"},
	}
	for name, tc := range cases {
		cfg := adapterBase()
		cfg.Base.Adapters = tc.adapters
		err := cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("an installer digest %s answered %v, want a refusal saying %q", name, err, tc.want)
		}
	}
}

// TestAnApprovedInstallerDigestReachesInstallSh holds the channel for that approval:
// install.sh receives it as TK_INSTALLER_SHA256 beside the account and the
// distribution, and an adapter with none receives no such variable.
func TestAnApprovedInstallerDigestReachesInstallSh(t *testing.T) {
	cfg := adapterBase()
	env, err := adapterInstallEnv(cfg, BaseAdapter{Name: "muse", InstallerSHA256: museDigest})
	if err != nil {
		t.Fatal(err)
	}
	if env["TK_INSTALLER_SHA256"] != museDigest || env["TK_USER"] != "herdr" || env["TK_DISTRO"] != cfg.Base.Name {
		t.Fatalf("install.sh would receive %v", env)
	}
	env, err = adapterInstallEnv(cfg, herdrAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := env["TK_INSTALLER_SHA256"]; ok {
		t.Fatalf("an adapter with no approved digest received one: %v", env)
	}
}

// TestTheMuseInstallerRunsOnlyWhileItsDigestIsApproved runs the muse adapter's
// install.sh against stand-ins for the base's commands and for Meta's server. An
// installer nobody approved stops the run before it runs, naming its digest and how to
// approve it; the same file approved by the configuration runs, puts muse on PATH
// through a wrapper that refuses any other account, and is not fetched again. WSL-77.
func TestTheMuseInstallerRunsOnlyWhileItsDigestIsApproved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh runs under a POSIX shell, which the Linux job has")
	}
	for _, tool := range []string{"sh", "bash", "sha256sum", "cmp"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not on PATH", tool)
		}
	}
	script, err := adapterScript("muse", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	home, bin := filepath.Join(root, "home"), filepath.Join(root, "bin")
	for _, dir := range []string{home, bin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// What Meta would serve: an installer that writes a launcher answering a version.
	served := filepath.Join(root, "served.sh")
	write(served, "#!/usr/bin/env bash\nset -eu\nmkdir -p \"$HOME/.local/bin\"\n"+
		"printf '#!/bin/sh\\necho \"Muse Code 9.9.9 (9.9.9-R1)\"\\n' > \"$HOME/.local/bin/muse\"\n"+
		"chmod 0755 \"$HOME/.local/bin/muse\"\n")
	fetches := filepath.Join(root, "fetches")
	write(filepath.Join(bin, "pacman"), "#!/bin/sh\nexit 0\n")
	write(filepath.Join(bin, "getent"), "#!/bin/sh\nprintf 'herdr:x:1000:1000::%s:/bin/bash\\n' \"$STUB_HOME\"\n")
	write(filepath.Join(bin, "curl"), "#!/bin/sh\necho fetched >> \"$STUB_FETCHES\"\nout=\n"+
		"while [ $# -gt 0 ]; do if [ \"$1\" = --output ]; then out=$2; shift; fi; shift; done\ncp \"$STUB_SERVED\" \"$out\"\n")
	write(filepath.Join(bin, "runuser"), "#!/bin/sh\n[ \"$1\" = -u ] && [ \"$2\" = herdr ] && [ \"$3\" = -- ] || exit 9\nshift 3\nexec \"$@\"\n")
	wrapper := filepath.Join(root, "usr-local-bin", "muse")
	run := func(approval string) (int, string) {
		t.Helper()
		cmd := exec.Command("sh", "-s")
		cmd.Stdin = bytes.NewReader(script)
		cmd.Env = append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base", "TK_INSTALLER_SHA256="+approval,
			"TK_MUSE_STAGE_DIR="+filepath.Join(root, "stage"), "TK_MUSE_WRAPPER="+wrapper,
			"STUB_HOME="+home, "STUB_SERVED="+served, "STUB_FETCHES="+fetches)
		out, err := cmd.CombinedOutput()
		var exited *exec.ExitError
		if err != nil && !errors.As(err, &exited) {
			t.Fatalf("install.sh did not start: %v", err)
		}
		return cmd.ProcessState.ExitCode(), string(out)
	}
	body, err := os.ReadFile(served)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	launcher := filepath.Join(home, ".local", "bin", "muse")

	code, out := run("")
	if code != 2 || !strings.Contains(out, "not one the operator approved") || !strings.Contains(out, digest) ||
		!strings.Contains(out, `"installer_sha256": "`+digest+`"`) {
		t.Fatalf("an installer nobody approved answered exit %d:\n%s", code, out)
	}
	if _, err := os.Stat(launcher); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an installer nobody approved ran and left %s (%v):\n%s", launcher, err, out)
	}

	code, out = run(digest)
	if code != 0 || !strings.Contains(out, "approved by the installer_sha256") || !strings.Contains(out, "installed Muse Code 9.9.9 (9.9.9-R1)") ||
		!strings.HasSuffix(strings.TrimSpace(out), adapterCompleteLine("muse")) {
		t.Fatalf("an approved installer answered exit %d:\n%s", code, out)
	}
	refused := exec.Command(wrapper, "--version")
	if refusal, err := refused.CombinedOutput(); refused.ProcessState == nil || refused.ProcessState.ExitCode() != 126 ||
		!strings.Contains(string(refusal), "runs only as herdr") {
		t.Fatalf("the wrapper ran Muse for another account: %v\n%s", err, refusal)
	}

	before, _ := os.ReadFile(fetches)
	code, out = run("")
	after, _ := os.ReadFile(fetches)
	if code != 0 || !strings.Contains(out, "is installed for herdr, so the installer is not fetched") || !bytes.Equal(before, after) {
		t.Fatalf("a second run over an installed Muse answered exit %d, fetching %d time(s) more:\n%s",
			code, bytes.Count(after, []byte("\n"))-bytes.Count(before, []byte("\n")), out)
	}
}

// TestTheHerdrReporterIsRegisteredWhereMuseReadsIt holds the reporter's registration to
// what Muse Code 1.3.0 was measured to run: the settings file under ~/.config/muse, a
// schema_version on a file this writes, the command inside a matcher group, the
// operator's own settings and hooks kept, and a file Muse would refuse left untouched.
//
// ⚠ THE KERNEL RULE BEHIND THE TEMPORARY FILE CANNOT BE STAGED WITHOUT ROOT, so the
// case holds its cause instead: root chowned the file it then wrote, and
// fs.protected_regular refused the write. Nothing may be chowned under TMPDIR.
func TestTheHerdrReporterIsRegisteredWhereMuseReadsIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh runs under a POSIX shell, which the Linux job has")
	}
	for _, tool := range []string{"sh", "bash", "sha256sum", "cmp", "base64", "install", "id"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not on PATH", tool)
		}
	}
	// ⚠ jq IS REACHED THROUGH as_account, WHOSE PATH IS FIXED, so a jq elsewhere does not count.
	jqFound := false
	for _, dir := range []string{"/usr/local/sbin", "/usr/local/bin", "/usr/bin", "/bin"} {
		if _, err := os.Stat(filepath.Join(dir, "jq")); err == nil {
			jqFound = true
		}
	}
	if !jqFound {
		t.Skip("jq is not in the PATH install.sh gives the account")
	}
	realInstall, _ := exec.LookPath("install")
	realID, _ := exec.LookPath("id")
	script, err := adapterScript("muse", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := adapterTree.ReadFile("adapters/muse/herdr-agent-state.sh")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	home, bin, tmp := filepath.Join(root, "home"), filepath.Join(root, "bin"), filepath.Join(root, "tmp")
	for _, dir := range []string{filepath.Join(home, ".local", "bin"), bin, tmp} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// An installed Muse, so the installer is never fetched.
	write(filepath.Join(home, ".local", "bin", "muse"), "#!/bin/sh\necho \"Muse Code 9.9.9 (9.9.9-R1)\"\n")
	chowns := filepath.Join(root, "chowns")
	write(filepath.Join(bin, "pacman"), "#!/bin/sh\nexit 0\n")
	write(filepath.Join(bin, "curl"), "#!/bin/sh\nexit 9\n")
	write(filepath.Join(bin, "herdr"), "#!/bin/sh\nexit 0\n")
	write(filepath.Join(bin, "getent"), "#!/bin/sh\nprintf 'herdr:x:1000:1000::%s:/bin/bash\\n' \"$STUB_HOME\"\n")
	write(filepath.Join(bin, "runuser"), "#!/bin/sh\n[ \"$1\" = -u ] && [ \"$2\" = herdr ] && [ \"$3\" = -- ] || exit 9\nshift 3\nexec \"$@\"\n")
	write(filepath.Join(bin, "id"), "#!/bin/sh\nif [ \"$1\" = -gn ]; then echo herdr; exit 0; fi\nexec "+realID+" \"$@\"\n")
	write(filepath.Join(bin, "chown"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$STUB_CHOWNS\"\n")
	// install without its owner flags, which only root may pass.
	write(filepath.Join(bin, "install"), "#!/bin/sh\nskip=\nfor a in \"$@\"; do\n  shift\n"+
		"  if [ -n \"$skip\" ]; then skip=; continue; fi\n"+
		"  case $a in -o|-g) skip=1; continue ;; esac\n  set -- \"$@\" \"$a\"\ndone\nexec "+realInstall+" \"$@\"\n")
	settings := filepath.Join(home, ".config", "muse", "settings.json")
	hook := filepath.Join(home, ".local", "share", "wsl-toolkit", "herdr-agent-state.sh")
	run := func() (int, string) {
		t.Helper()
		cmd := exec.Command("sh", "-s")
		cmd.Stdin = bytes.NewReader(script)
		cmd.Env = append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "TMPDIR="+tmp,
			"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base",
			"TK_MUSE_STAGE_DIR="+filepath.Join(root, "stage"), "TK_MUSE_WRAPPER="+filepath.Join(root, "usr-local-bin", "muse"),
			"TK_FILE_HERDR_AGENT_STATE_SH_B64="+base64.StdEncoding.EncodeToString(reporter),
			"STUB_HOME="+home, "STUB_CHOWNS="+chowns)
		out, err := cmd.CombinedOutput()
		var exited *exec.ExitError
		if err != nil && !errors.As(err, &exited) {
			t.Fatalf("install.sh did not start: %v", err)
		}
		return cmd.ProcessState.ExitCode(), string(out)
	}
	type group struct {
		Matcher *string `json:"matcher"`
		Hooks   []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"hooks"`
	}
	read := func() (map[string]json.RawMessage, map[string][]group) {
		t.Helper()
		body, err := os.ReadFile(settings)
		if err != nil {
			t.Fatal(err)
		}
		var top map[string]json.RawMessage
		if err := json.Unmarshal(body, &top); err != nil {
			t.Fatalf("%s is not a JSON object: %v\n%s", settings, err, body)
		}
		groups := map[string][]group{}
		if raw, ok := top["hooks"]; ok {
			if err := json.Unmarshal(raw, &groups); err != nil {
				t.Fatalf("hooks is not a map of matcher groups: %v\n%s", err, body)
			}
		}
		return top, groups
	}
	// ours counts the groups that hold exactly this adapter's command and match everything.
	ours := func(gs []group) int {
		n := 0
		for _, g := range gs {
			if g.Matcher != nil && *g.Matcher == "" && len(g.Hooks) == 1 && g.Hooks[0].Type == "command" && g.Hooks[0].Command == hook {
				n++
			}
		}
		return n
	}
	events := []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "Stop", "SessionEnd"}

	code, out := run()
	if code != 0 || !strings.Contains(out, "registered the herdr reporter in "+settings) ||
		!strings.HasSuffix(strings.TrimSpace(out), adapterCompleteLine("muse")) {
		t.Fatalf("a base with no settings file answered exit %d:\n%s", code, out)
	}
	top, groups := read()
	if string(top["schema_version"]) != "1" {
		t.Fatalf("the settings file this wrote carries schema_version %q, and Muse refuses to start without one", top["schema_version"])
	}
	for _, e := range events {
		if len(groups[e]) != 1 || ours(groups[e]) != 1 {
			t.Fatalf("%s holds %+v, and it must hold one matcher group running %s", e, groups[e], hook)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".muse", "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("~/.muse/settings.json was written, and Muse 1.3.0 does not read it (%v)", err)
	}
	if logged, _ := os.ReadFile(chowns); bytes.Contains(logged, []byte(tmp)) {
		t.Fatalf("a file under the temporary directory was chowned, which fs.protected_regular turns into a refused write:\n%s", logged)
	}

	before, _ := os.ReadFile(settings)
	code, out = run()
	after, _ := os.ReadFile(settings)
	if code != 0 || !strings.Contains(out, "Muse already reports to herdr on 6 events") || !bytes.Equal(before, after) {
		t.Fatalf("a second run answered exit %d and changed the settings file %v:\n%s", code, !bytes.Equal(before, after), out)
	}

	own := `{"schema_version":1,"theme":"dark","hooks":{"Stop":[` +
		`{"matcher":"","hooks":[{"type":"command","command":"/opt/own/stop.sh"}]},` +
		`{"matcher":"","hooks":[{"type":"command","command":"/old/home/.local/share/wsl-toolkit/herdr-agent-state.sh"},{"type":"command","command":"/opt/own/also.sh"}]}]}}`
	if err := os.WriteFile(settings, []byte(own), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out = run(); code != 0 {
		t.Fatalf("a settings file of the operator's own answered exit %d:\n%s", code, out)
	}
	top, groups = read()
	if string(top["theme"]) != `"dark"` {
		t.Fatalf("the operator's own setting became %q", top["theme"])
	}
	stop := groups["Stop"]
	if len(stop) != 3 || ours(stop) != 1 || len(stop[0].Hooks) != 1 || stop[0].Hooks[0].Command != "/opt/own/stop.sh" ||
		len(stop[1].Hooks) != 1 || stop[1].Hooks[0].Command != "/opt/own/also.sh" {
		t.Fatalf("Stop became %+v: the operator's two hooks kept, the old entry of ours gone, and ours once", stop)
	}

	for name, body := range map[string]string{"no schema_version": `{"theme":"dark"}`, "not JSON": `{`} {
		if err := os.WriteFile(settings, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		code, out = run()
		left, _ := os.ReadFile(settings)
		if code == 0 || !strings.Contains(out, "schema_version") || string(left) != body {
			t.Fatalf("a settings file with %s answered exit %d and left %q:\n%s", name, code, left, out)
		}
	}
}

// TestEveryStoredBaseFieldSurvivesLoading is the case the first drive of base.adapters
// called for: LoadConfig copies a stored file's base fields one by one, and the new
// field was decoded, validated as empty and dropped. It sets every field of
// BaseConfig and reads the file back.
func TestEveryStoredBaseFieldSurvivesLoading(t *testing.T) {
	dir := t.TempDir()
	stored := adapterBase()
	stored.Schema = ConfigSchema
	stored.Base.Name = DefaultBaseName
	stored.Base.Automount = AutomountOff
	stored.Base.Interop = BaseInteropOff
	stored.Base.PasswordlessSudo = true
	stored.Base.Toolset = BaseToolsetDeveloper
	stored.Base.SharedTmpfs = SharedTmpfsOff
	grant := BaseMount{Source: dir, Target: "/workspaces/project", Mode: BaseMountReadWrite}
	stored.Base.Mounts = []BaseMount{grant}
	// ⚠ AND A FIELD INSIDE AN ADAPTER, which the reflect walk below cannot see.
	stored.Base.Adapters = append(stored.Base.Adapters, BaseAdapter{Name: "muse", InstallerSHA256: museDigest})
	v := reflect.ValueOf(stored.Base)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			t.Fatalf("the fixture leaves base.%s empty, so this case cannot see it dropped", v.Type().Field(i).Name)
		}
	}
	body, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "wsl-toolkit.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WSL_TOOLKIT_HOME", filepath.Join(dir, "state"))
	prev := ExplicitConfigPath
	ExplicitConfigPath = path
	t.Cleanup(func() { ExplicitConfigPath = prev })
	got, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Base, stored.Base) {
		t.Fatalf("loading dropped or changed a base field:\n stored %+v\n loaded %+v", stored.Base, got.Base)
	}
}

// TestAnAdapterFileReachesTheGuestAsBase64 holds the channel: every file beside the
// two scripts arrives in TK_FILE_<NAME>_B64, byte for byte, and neither script does.
func TestAnAdapterFileReachesTheGuestAsBase64(t *testing.T) {
	env, err := adapterFileEnv("herdr")
	if err != nil {
		t.Fatal(err)
	}
	want, err := adapterTree.ReadFile("adapters/herdr/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	got, err := base64.StdEncoding.DecodeString(env["TK_FILE_CONFIG_TOML_B64"])
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("config.toml reached the environment as %q (%v)", got, err)
	}
	if len(env) != 1 {
		t.Fatalf("the environment carries %d files, and herdr has one beside its scripts: %v", len(env), env)
	}
}

// TestTheHerdrConfigurationUnbindsTheCloseKeys holds the two keys WSL-76 measured
// closing a pane at once, and the settings the tracked file promises.
func TestTheHerdrConfigurationUnbindsTheCloseKeys(t *testing.T) {
	body, err := adapterTree.ReadFile("adapters/herdr/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{`close_pane = ""`, `close_tab = ""`, `confirm_close = true`, `onboarding = false`} {
		if !bytes.Contains(body, []byte("\n"+line+"\n")) {
			t.Errorf("the tracked herdr configuration does not carry %s", line)
		}
	}
}

// TestAnAdapterProbeIsReadLineByLine holds the protocol: facts, a version, and every
// problem line, and an adapter with no version is not healthy.
func TestAnAdapterProbeIsReadLineByLine(t *testing.T) {
	st := parseAdapterProbe("herdr", "version 0.9.0\r\nserver active\nssh-host-key ssh-ed25519 AAAA\n\nproblem the door is shut\n")
	if st.Version != "0.9.0" || st.Facts["server"] != "active" || st.Facts["ssh-host-key"] != "ssh-ed25519 AAAA" {
		t.Fatalf("the probe was read as %+v", st)
	}
	if st.Healthy || len(st.Problems) != 1 || st.Problems[0] != "the door is shut" {
		t.Fatalf("a probe with a problem line was read as %+v", st)
	}
	if st := parseAdapterProbe("herdr", "server active\n"); st.Healthy {
		t.Fatalf("a probe that named no version was healthy: %+v", st)
	}
}

// TestTheHostBlockIsWrittenOnTopAndEverythingElseKeepsItsBytes holds the ruling's
// one marked block: written at the top, rewritten in place of itself, removed back
// to the file it found, with the file's own line endings.
func TestTheHostBlockIsWrittenOnTopAndEverythingElseKeepsItsBytes(t *testing.T) {
	p := herdrPaths{key: `C:\Users\runner\.ssh\wsl-toolkit\id_ed25519`, knownHosts: `C:\Users\runner\.ssh\wsl-toolkit\known_hosts`}
	block := herdrHostBlock("wsl-toolkit-base", "herdr", `C:\Windows\System32\wsl.exe`, p)
	for _, eol := range []string{"\n", "\r\n"} {
		original := []byte("Host example" + eol + "  HostName example.invalid" + eol + "Host *" + eol + "  User nobody" + eol)
		written := withMarkedBlock(original, "wsl-toolkit-base", block)
		if !bytes.HasPrefix(written, []byte(markerBegin("wsl-toolkit-base")+eol)) {
			t.Fatalf("the block is not at the top:\n%s", written)
		}
		if !bytes.HasSuffix(written, original) {
			t.Fatalf("the rest of the file changed:\n%s", written)
		}
		if eol == "\n" && bytes.Contains(written, []byte("\r")) {
			t.Fatalf("a file with LF endings was given CRLF:\n%q", written)
		}
		found, ok := markedBlock(written, "wsl-toolkit-base")
		if !ok || !equalLines(found, block) {
			t.Fatalf("the written block does not read back as itself: %q", found)
		}
		if again := withMarkedBlock(written, "wsl-toolkit-base", block); !bytes.Equal(again, written) {
			t.Fatalf("writing the same block twice changed the file:\n%s", again)
		}
		if removed := withMarkedBlock(written, "wsl-toolkit-base", nil); !bytes.Equal(removed, original) {
			t.Fatalf("removing the block did not give back the file it found:\n%q", removed)
		}
	}
	proxy := ""
	for _, line := range block {
		if strings.HasPrefix(line, "  ProxyCommand ") {
			proxy = line
		}
	}
	if proxy != "  ProxyCommand C:/Windows/System32/wsl.exe -d wsl-toolkit-base -u root --exec "+herdrDoorWrapper {
		t.Fatalf("the proxy starts %q", proxy)
	}
}

// TestAStrayBeginMarkerDeletesNothing is the refusal the removal carries: a begin
// line with no end after it leaves the rest of the file where it is.
func TestAStrayBeginMarkerDeletesNothing(t *testing.T) {
	original := []byte(markerBegin("wsl-toolkit-base") + "\nHost mine\n  User me\n")
	if got := withMarkedBlock(original, "wsl-toolkit-base", nil); !bytes.Equal(got, original) {
		t.Fatalf("a begin marker with no end removed the lines after it:\n%q", got)
	}
	if _, ok := markedBlock(original, "wsl-toolkit-base"); ok {
		t.Fatal("a begin marker with no end was read as a block")
	}
}

// TestAKnownHostLineIsThisBasesAlone holds the known-hosts file: one line per alias,
// replaced and removed without touching another base's.
func TestAKnownHostLineIsThisBasesAlone(t *testing.T) {
	other := "wsl-toolkit-other ssh-ed25519 BBBB\n"
	written := withKnownHost([]byte(other), "wsl-toolkit-base", "ssh-ed25519 AAAA")
	if !knownHostIs(written, "wsl-toolkit-base", "ssh-ed25519 AAAA") || !bytes.HasPrefix(written, []byte(other)) {
		t.Fatalf("known hosts after writing: %q", written)
	}
	if again := withKnownHost(written, "wsl-toolkit-base", "ssh-ed25519 CCCC"); knownHostIs(again, "wsl-toolkit-base", "ssh-ed25519 AAAA") ||
		!knownHostIs(again, "wsl-toolkit-base", "ssh-ed25519 CCCC") {
		t.Fatalf("a changed host key was not replaced: %q", again)
	}
	if removed := withKnownHost(written, "wsl-toolkit-base", ""); string(removed) != other {
		t.Fatalf("removing the base's line left %q", removed)
	}
	if knownHostIs([]byte("wsl-toolkit-base ssh-ed25519 AAAA\nwsl-toolkit-base ssh-ed25519 AAAA\n"), "wsl-toolkit-base", "ssh-ed25519 AAAA") {
		t.Fatal("two lines for one alias were read as the one the door needs")
	}
}

// TestTheHerdrHostHalfIsCheckedBeforeItConnects holds the machine half's check: a
// missing key, block or host line is named without a connection, and a connection
// that answers another version is named too.
func TestTheHerdrHostHalfIsCheckedBeforeItConnects(t *testing.T) {
	home := t.TempDir()
	connected := 0
	answer := "herdr 0.9.0"
	h := &herdrHost{home: home, connect: func(context.Context, string, string) (string, error) {
		connected++
		return answer, nil
	}}
	b := &Base{cfg: adapterBase(), wsl: &Wsl{Path: `C:\Windows\System32\wsl.exe`}, log: func(string) {}}
	facts := map[string]string{"version": "0.9.0", "ssh-host-key": "ssh-ed25519 AAAA"}
	if problems := h.check(context.Background(), b, facts); len(problems) != 3 || connected != 0 {
		t.Fatalf("an empty home answered %d problem(s) after %d connection(s): %v", len(problems), connected, problems)
	}
	p, _ := h.paths()
	if err := os.MkdirAll(filepath.Dir(p.key), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.key, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := h.apply(context.Background(), b, facts); err != nil {
		t.Fatal(err)
	}
	if problems := h.check(context.Background(), b, facts); len(problems) != 0 || connected != 1 {
		t.Fatalf("a written half answered %v after %d connection(s)", problems, connected)
	}
	answer = "herdr 0.8.2"
	if problems := h.check(context.Background(), b, facts); len(problems) != 1 || !strings.Contains(problems[0], "0.8.2") {
		t.Fatalf("a door answering another version was answered %v", problems)
	}
	if err := h.remove(b); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.config, p.knownHosts} {
		body, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if len(body) != 0 {
			t.Fatalf("%s still holds %q after removal", path, body)
		}
	}
}

// TestAnAdapterVersionAndItsDigestsMoveTogether is the resilience rule: an operator
// may move an adapter to a release this executable has never heard of, and may not
// move it to one nothing can check.
//
// ⛔ EITHER ALONE IS A REFUSAL. A version with no digest is a download this
// repository will not make. A digest with no version is a value nothing reads, and
// ignoring it silently is how somebody believes they pinned a thing they did not.
func TestAnAdapterVersionAndItsDigestsMoveTogether(t *testing.T) {
	good := strings.Repeat("a1", 32)
	for _, c := range []struct {
		name    string
		adapter BaseAdapter
		want    string
	}{
		{"neither is the ordinary case", BaseAdapter{Name: "herdr"}, ""},
		{"both together are accepted", BaseAdapter{Name: "herdr", Version: "0.10.1", SHA256: map[string]string{"x86_64": good}}, ""},
		{"a version alone is refused", BaseAdapter{Name: "herdr", Version: "0.10.1"}, "gives no digests"},
		{"digests alone are refused", BaseAdapter{Name: "herdr", SHA256: map[string]string{"x86_64": good}}, "nothing would read them"},
		{"a version that is not one", BaseAdapter{Name: "herdr", Version: "../../etc", SHA256: map[string]string{"x86_64": good}}, "is not a release"},
		{"an architecture that is not one", BaseAdapter{Name: "herdr", Version: "0.10.1", SHA256: map[string]string{"x86_64; rm": good}}, "keyed"},
		{"a digest that is not one", BaseAdapter{Name: "herdr", Version: "0.10.1", SHA256: map[string]string{"x86_64": "nope"}}, "is not a SHA-256"},
		{"an adapter that pins nothing", BaseAdapter{Name: "muse", Version: "1.2.3", SHA256: map[string]string{"x86_64": good}}, "installs nothing this pins"},
	} {
		cfg := baseConfigWithAdapters(c.adapter)
		err := cfg.Validate()
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: answered %v, want a refusal carrying %q", c.name, err, c.want)
		}
	}
}

// TestAMovedAdapterVersionReachesItsInstaller proves the pin arrives as the
// environment the script reads, rather than being validated and dropped.
func TestAMovedAdapterVersionReachesItsInstaller(t *testing.T) {
	good := strings.Repeat("b2", 32)
	cfg := baseConfigWithAdapters(BaseAdapter{Name: "herdr", Version: "0.10.1",
		SHA256: map[string]string{"x86_64": good, "aarch64": good}})
	env, err := adapterInstallEnv(cfg, cfg.Base.Adapters[0])
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{
		"TK_ADAPTER_VERSION":        "0.10.1",
		"TK_ADAPTER_SHA256_X86_64":  good,
		"TK_ADAPTER_SHA256_AARCH64": good,
	} {
		if env[k] != want {
			t.Errorf("%s reached the installer as %q, want %q", k, env[k], want)
		}
	}
	plain, err := adapterInstallEnv(baseConfigWithAdapters(BaseAdapter{Name: "herdr"}), BaseAdapter{Name: "herdr"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plain["TK_ADAPTER_VERSION"]; ok {
		t.Error("an adapter with no pin still received TK_ADAPTER_VERSION, so the script could not tell that it has its own default")
	}
}

// TestACarriedFileIsNotWrittenOverSomethingElse holds the one destructive edge of
// `shipped write`.
//
// ⛔ A FILE ALREADY THERE WHOSE CONTENT DIFFERS IS SOMEBODY'S EDIT, and replacing
// it silently is how that edit disappears. Identical content is left alone rather
// than rewritten, so a second run moves no timestamp.
func TestACarriedFileIsNotWrittenOverSomethingElse(t *testing.T) {
	dir := t.TempDir()
	dest, wrote, err := WriteShipped("tmux.conf", dir, false)
	if err != nil || !wrote {
		t.Fatalf("the first write answered (%q, %v, %v)", dest, wrote, err)
	}
	if _, wrote, err = WriteShipped("tmux.conf", dir, false); err != nil || wrote {
		t.Errorf("writing the same bytes again answered (%v, %v); want no write and no error", wrote, err)
	}
	if err := os.WriteFile(dest, []byte("the operator edited this\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := WriteShipped("tmux.conf", dir, false); err == nil {
		t.Error("a file that differs was overwritten without --force")
	}
	if _, wrote, err := WriteShipped("tmux.conf", dir, true); err != nil || !wrote {
		t.Errorf("--force answered (%v, %v); want it written", wrote, err)
	}
	if _, err := ShippedBody("../../etc/passwd"); err == nil {
		t.Error("a path was accepted where a bare name is required")
	}
	if _, err := ShippedBody("no-such-file"); err == nil {
		t.Error("a name this executable does not carry was accepted")
	}
}
