// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var (
	herdrAdapter  = BaseAdapter{Name: "herdr"}
	zellijAdapter = BaseAdapter{Name: "zellij"}
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
		"an unknown adapter": {func(c *Config) { c.Base.Adapters = []BaseAdapter{zellijAdapter} }, "the adapters this executable carries are: herdr"},
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
	grant := BaseMount{Source: dir, Target: "/workspaces/project", Mode: BaseMountReadWrite}
	stored.Base.Mounts = []BaseMount{grant}
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
