// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-196: registry mirrors and an engine proxy, set from the host -----------

// testRegistry is a base.registry with every field set.
func testRegistry() *RegistryConfig {
	return &RegistryConfig{
		Mirrors: map[string][]string{
			"docker.io":         {"mirror.gcr.io", "mirror.example:5000/hub"},
			"quay.io/podman":    {"mirror.example:5000"},
			"registry.local:80": {"cache.local"},
		},
		Proxy:   "http://proxy.example:3128",
		NoProxy: []string{"localhost", "127.0.0.1", ".corp.example"},
	}
}

// ⛔ base.registry IS HELD WHERE IT IS READ. Every value lands in a TOML string
// or an engine's environment, and a proxy with a password would be printed by
// config and base status and handed to every container. WSL-196.
func TestBaseRegistryIsHeldWhereItIsRead(t *testing.T) {
	if err := validateRegistry(testRegistry()); err != nil {
		t.Fatalf("a usable base.registry was refused: %v", err)
	}
	for name, c := range map[string]struct {
		r    RegistryConfig
		want string
	}{
		"a mirror with a scheme":     {RegistryConfig{Mirrors: map[string][]string{"docker.io": {"https://mirror.gcr.io"}}}, "without a scheme"},
		"a registry with a scheme":   {RegistryConfig{Mirrors: map[string][]string{"https://docker.io": {"mirror.gcr.io"}}}, "without a scheme"},
		"a registry in capitals":     {RegistryConfig{Mirrors: map[string][]string{"Docker.io": {"mirror.gcr.io"}}}, "lower case"},
		"a quote in a mirror":        {RegistryConfig{Mirrors: map[string][]string{"docker.io": {`mirror"x`}}}, "lower case"},
		"a registry with no mirror":  {RegistryConfig{Mirrors: map[string][]string{"docker.io": {}}}, "is empty"},
		"a mirror that is itself":    {RegistryConfig{Mirrors: map[string][]string{"docker.io": {"docker.io"}}}, "twice"},
		"a mirror named twice":       {RegistryConfig{Mirrors: map[string][]string{"docker.io": {"m.example", "m.example"}}}, "twice"},
		"a path with no host":        {RegistryConfig{Mirrors: map[string][]string{"docker.io": {":5000/x"}}}, "names its host first"},
		"a trailing slash":           {RegistryConfig{Mirrors: map[string][]string{"docker.io": {"m.example/"}}}, "optional path"},
		"a proxy with a password":    {RegistryConfig{Proxy: "http://me:pw@10.0.0.1:3128"}, "user or a password"},
		"a proxy with no scheme":     {RegistryConfig{Proxy: "proxy.example:3128"}, "must start with"},
		"a proxy with a path":        {RegistryConfig{Proxy: "http://proxy.example:3128/x"}, "path or a query"},
		"a proxy with a quote":       {RegistryConfig{Proxy: `http://proxy.example:3128"`}, "a quote"},
		"exceptions with no proxy":   {RegistryConfig{NoProxy: []string{"localhost"}}, "proxy is empty"},
		"an exception with a comma":  {RegistryConfig{Proxy: "http://p.example:1", NoProxy: []string{"a,b"}}, "no space, comma"},
		"an empty exception":         {RegistryConfig{Proxy: "http://p.example:1", NoProxy: []string{""}}, "no space, comma"},
		"an exception with a space":  {RegistryConfig{Proxy: "http://p.example:1", NoProxy: []string{"a b"}}, "no space, comma"},
		"a proxy that names no host": {RegistryConfig{Proxy: "http://:3128"}, "names no host"},
	} {
		r := c.r
		err := validateRegistry(&r)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s answered %v; want a refusal naming %q", name, err, c.want)
		}
		if err != nil && strings.Contains(err.Error(), ":pw@") {
			t.Errorf("%s printed the proxy's password: %v", name, err)
		}
	}
	// ⭐ THE KEY IS HELD BY Validate, so every command that reads a file refuses it.
	cfg := DefaultConfig()
	cfg.Base.Registry = &RegistryConfig{Proxy: "proxy.example"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "base.registry.proxy") {
		t.Errorf("a configuration with an unusable base.registry was validated: %v", err)
	}
}

// ⭐ THE TWO DROP-INS ARE RENDERED HERE, the same bytes for the same key, and
// nothing for a configuration without it. WSL-196.
func TestBaseRegistryRendersTheTwoDropIns(t *testing.T) {
	r := testRegistry()
	const mirrors = `# Written by wsl-toolkit from base.registry.mirrors. WSL-196.

[[registry]]
prefix = "docker.io"
location = "docker.io"

[[registry.mirror]]
location = "mirror.gcr.io"

[[registry.mirror]]
location = "mirror.example:5000/hub"

[[registry]]
prefix = "quay.io/podman"
location = "quay.io/podman"

[[registry.mirror]]
location = "mirror.example:5000"

[[registry]]
prefix = "registry.local:80"
location = "registry.local:80"

[[registry.mirror]]
location = "cache.local"
`
	if got := mirrorsDropIn(r); got != mirrors {
		t.Errorf("the mirrors drop-in reads:\n%s\nwant:\n%s", got, mirrors)
	}
	const proxy = `# Written by wsl-toolkit from base.registry.proxy. WSL-196.
[engine]
env = ["http_proxy=http://proxy.example:3128", "https_proxy=http://proxy.example:3128", "HTTP_PROXY=http://proxy.example:3128", "HTTPS_PROXY=http://proxy.example:3128", "no_proxy=localhost,127.0.0.1,.corp.example", "NO_PROXY=localhost,127.0.0.1,.corp.example"]
`
	if got := proxyDropIn(r); got != proxy {
		t.Errorf("the proxy drop-in reads:\n%s\nwant:\n%s", got, proxy)
	}
	for _, none := range []*RegistryConfig{nil, {}} {
		if mirrorsDropIn(none) != "" || proxyDropIn(none) != "" {
			t.Errorf("%+v renders a drop-in", none)
		}
		for k, v := range registryEnv(none) {
			if v != "" {
				t.Errorf("a configuration without base.registry hands %s=%q", k, v)
			}
		}
	}
}

// ⭐ ONE TEXT FOR BOTH STEPS. Provisioning writes the drop-ins so a new base's
// first pull takes them, and every owner's ensure writes the same bytes so a base
// that works takes a changed key. Both payloads carry mirrors.sh. WSL-196.
func TestProvisioningAndEveryEnsureAreHandedTheSameRegistryDropIns(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Base.Registry = testRegistry()
	b := &Base{cfg: cfg}
	prov := b.provisionRequest(AutomountOff, BaseInteropOff, BaseToolsetNone, SharedTmpfsOn, io.Discard)
	eng := b.enginesRequest(io.Discard)
	for k, want := range map[string]string{"TK_REGISTRY_MIRRORS_B64": mirrorsDropIn(cfg.Base.Registry), "TK_REGISTRY_PROXY_B64": proxyDropIn(cfg.Base.Registry)} {
		for name, env := range map[string]map[string]string{"provisioning": prov.Env, "engines.sh": eng.Env} {
			got, err := base64.StdEncoding.DecodeString(env[k])
			if err != nil || string(got) != want {
				t.Errorf("%s is handed %s=%q (%v), want the rendered drop-in", name, k, got, err)
			}
		}
	}
	for name, script := range map[string][]byte{"provisioning": prov.Script, "engines.sh": eng.Script} {
		s := string(script)
		def, call := strings.Index(s, "apply_registry_settings() {"), strings.LastIndex(s, "apply_registry_settings ||")
		if def < 0 || call < def {
			t.Errorf("%s does not define apply_registry_settings before it calls it: defined at %d, called at %d", name, def, call)
		}
	}
	// ⛔ A configuration without the key hands both texts empty.
	b.cfg.Base.Registry = nil
	for name, env := range map[string]map[string]string{
		"provisioning": b.provisionRequest(AutomountOff, BaseInteropOff, BaseToolsetNone, SharedTmpfsOn, io.Discard).Env,
		"engines.sh":   b.enginesRequest(io.Discard).Env,
	} {
		if env["TK_REGISTRY_MIRRORS_B64"] != "" || env["TK_REGISTRY_PROXY_B64"] != "" {
			t.Errorf("%s is handed a drop-in for a configuration without base.registry", name)
		}
	}
}

// listTree is every file under dir, by its path from dir, with its bytes.
func listTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// ⛔ PROVISIONING WRITES base.registry's DROP-INS, AND A BASE WITHOUT THE KEY IS
// BYTE FOR BYTE UNCHANGED: the same two files, and no line about either. Run in
// a real shell, mirrors.sh's functions first, as the payload carries them.
// WSL-196.
func TestProvisioningWritesTheRegistryDropInsAndNothingWithoutTheKey(t *testing.T) {
	parallelCase(t)
	without := t.TempDir()
	out := engineSettingsRun(t, without)
	files := listTree(t, without)
	if len(files) != 2 || files["etc/containers/registries.conf.d/99-wsl-toolkit.conf"] == "" ||
		files["home/.config/containers/containers.conf.d/10-wsl-toolkit.conf"] == "" {
		t.Errorf("a base without base.registry got %d file(s): %v", len(files), files)
	}
	if strings.Contains(out, "registry") || strings.Contains(out, "proxy") {
		t.Errorf("a base without base.registry was told about it:\n%s", out)
	}

	with := t.TempDir()
	r := testRegistry()
	out = engineSettingsRun(t, with, r)
	got := listTree(t, with)
	if got["etc/containers/registries.conf.d/99-wsl-toolkit-mirrors.conf"] != mirrorsDropIn(r) {
		t.Errorf("the mirrors drop-in reads %q\n%s", got["etc/containers/registries.conf.d/99-wsl-toolkit-mirrors.conf"], out)
	}
	if got["etc/containers/containers.conf.d/99-wsl-toolkit-proxy.conf"] != proxyDropIn(r) {
		t.Errorf("the proxy drop-in reads %q\n%s", got["etc/containers/containers.conf.d/99-wsl-toolkit-proxy.conf"], out)
	}
	for rel, body := range files {
		if got[rel] != body {
			t.Errorf("base.registry changed %s:\n%s", rel, got[rel])
		}
	}

	const (
		mirrors = "etc/containers/registries.conf.d/99-wsl-toolkit-mirrors.conf"
		proxy   = "etc/containers/containers.conf.d/99-wsl-toolkit-proxy.conf"
	)
	// ⭐ THE SAME KEY AGAIN WRITES NOTHING AND SAYS NOTHING, as engines.sh's own
	// files do. WSL-196.
	out = engineSettingsRun(t, with, r)
	if strings.Contains(out, "registry mirrors") || strings.Contains(out, "engine proxy") {
		t.Errorf("the same base.registry again was reported as a change:\n%s", out)
	}
	if again := listTree(t, with); again[mirrors] != mirrorsDropIn(r) || again[proxy] != proxyDropIn(r) {
		t.Errorf("the same base.registry again changed a drop-in: %v", again)
	}
	// ⛔ A KEY TAKEN AWAY TAKES BOTH FILES, and says so, on any host. The case for
	// engines.sh needs a POSIX host; this one runs in Git's sh too. WSL-196.
	narrowed := &RegistryConfig{Mirrors: map[string][]string{"docker.io": {"mirror.gcr.io"}}}
	out = engineSettingsRun(t, with, narrowed)
	if got = listTree(t, with); got[mirrors] != mirrorsDropIn(narrowed) || got[proxy] != "" ||
		!strings.Contains(out, "registry mirrors: ") || !strings.Contains(out, "engine proxy: none, so ") {
		t.Errorf("a proxy taken away left %q, or the mirrors read %q:\n%s", got[proxy], got[mirrors], out)
	}
	out = engineSettingsRun(t, with)
	got = listTree(t, with)
	if _, ok := got[mirrors]; ok || !strings.Contains(out, "registry mirrors: none, so ") {
		t.Errorf("a key taken away left the mirrors drop-in:\n%s", out)
	}
	if len(got) != len(files) {
		t.Errorf("a key taken away left %d file(s), want the %d of a base that never had it: %v", len(got), len(files), got)
	}
}

// ⛔ EVERY OWNER'S ENSURE WRITES base.registry's DROP-INS, the same bytes, and a
// key taken away takes both away. A base that never had the key gets neither
// file. Run in a real shell. WSL-196.
func TestEnginesShWritesTheRegistryDropInsAndTakesThemAway(t *testing.T) {
	h := newEnginesHost(t)
	const (
		mirrors = "etc/containers/registries.conf.d/99-wsl-toolkit-mirrors.conf"
		proxy   = "etc/containers/containers.conf.d/99-wsl-toolkit-proxy.conf"
	)
	out, _ := h.run(false, false, false)
	if h.read(mirrors) != "" || h.read(proxy) != "" || strings.Contains(out, "registry mirrors") || strings.Contains(out, "engine proxy") {
		t.Fatalf("a base without base.registry got a drop-in or a line:\n%s", out)
	}
	h.registry = testRegistry()
	out, _ = h.run(false, false, false)
	if h.read(mirrors) != mirrorsDropIn(h.registry) || h.read(proxy) != proxyDropIn(h.registry) {
		t.Fatalf("engines.sh wrote mirrors %q and proxy %q\n%s", h.read(mirrors), h.read(proxy), out)
	}
	// ⭐ An ensure that changes nothing says nothing about either file.
	if out, _ = h.run(false, false, false); strings.Contains(out, "registry mirrors") || strings.Contains(out, "engine proxy") {
		t.Errorf("the same base.registry again was reported as a change:\n%s", out)
	}
	h.registry = &RegistryConfig{Mirrors: map[string][]string{"docker.io": {"mirror.gcr.io"}}}
	h.run(false, false, false)
	if h.read(mirrors) != mirrorsDropIn(h.registry) || h.read(proxy) != "" {
		t.Errorf("a proxy taken away left %q, or the mirrors read %q", h.read(proxy), h.read(mirrors))
	}
	h.registry = nil
	out, _ = h.run(false, false, false)
	if _, err := os.Stat(h.path(mirrors)); !os.IsNotExist(err) || !strings.Contains(out, "registry mirrors: none") {
		t.Errorf("a key taken away left the mirrors drop-in (%v):\n%s", err, out)
	}
}

// ⛔ A GUEST THAT CANNOT TAKE A SUM STILL GETS THE NEW FILE. Two empty sums would
// match, so a base without sha256sum would keep a stale drop-in and say nothing.
// Run in a real shell, with sha256sum refused. WSL-196.
func TestARegistryDropInIsWrittenWhereNoSumCanBeTaken(t *testing.T) {
	parallelCase(t)
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	path := seedFile(t, t.TempDir(), "registries.conf.d/99-wsl-toolkit-mirrors.conf", "a stale drop-in\n")
	want := mirrorsDropIn(testRegistry())
	script := "set -eu\nsay() { printf '%s\\n' \"$*\"; }\nsha256sum() { return 127; }\n" + string(mirrorsScript) +
		"\nregistry_drop_in '" + filepath.ToSlash(path) + "' '" + base64.StdEncoding.EncodeToString([]byte(want)) + "' 'registry mirrors'\n"
	out, err := exec.Command(sh, "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("the drop-in writer failed: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(path); string(got) != want || !strings.Contains(string(out), "registry mirrors: ") {
		t.Errorf("a guest without sha256sum kept %q and said %q", got, out)
	}
}

// ⛔ A PROBE THAT COULD NOT PULL IS HANDED base.registry BEFORE IT IS REFUSED. The
// probe pulls before engines.sh runs, so a base that reaches its registry only
// through the key never got it, and every ensure refused it. The owner writes
// both drop-ins, and a change is probed once more. The base is kept as it is:
// no re-provisioning and no restart. WSL-196.
func TestAProbeThatCouldNotPullIsHandedBaseRegistryFirst(t *testing.T) {
	h := newFakeHost(t)
	b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
	b.cfg.Base.Registry = testRegistry()
	h.stageGuest(t, "verify=unpullable-until-registry", "registry=changed", "binfmt=ok")
	if _, err := b.EnsureWith(context.Background(), false, false); err != nil {
		t.Fatalf("a base that reaches its registry through base.registry answered %v\n%s", err, strings.Join(s.lines, "\n"))
	}
	ran, err := os.ReadFile(filepath.Join(h.state, "registry.script"))
	if err != nil || assigned(ran, "TK_REGISTRY_MIRRORS_B64") != registryEnv(b.cfg.Base.Registry)["TK_REGISTRY_MIRRORS_B64"] {
		t.Fatalf("the registry run was not handed base.registry's drop-ins: %v", err)
	}
	if !s.said("registry mirrors: "+MirrorsDropIn) || !s.said("asked once more") || s.said(registryComplete) {
		t.Errorf("the ensure did not say what it wrote and that it asked again: %v", s.lines)
	}
	if s.said("re-provisioning") || callAt(h.calls(t), "wsl --terminate") >= 0 {
		t.Errorf("the ensure re-provisioned or restarted the base: %v", s.lines)
	}
	assertKept(t, h, b, disk)
}

// ⛔ ONLY THE OWNER WRITES THEM, AND DROP-INS THAT CHANGE NOTHING KEEP THE
// REFUSAL. A guest's ensure leaves them as the owner set them, as it leaves the
// engines. An owner whose drop-ins are current is refused as before, and its
// probe is not asked again. WSL-196, WSL-174.
func TestAProbeThatCouldNotPullKeepsItsRefusalWhereNothingChanges(t *testing.T) {
	for _, guest := range []bool{true, false} {
		h := newFakeHost(t)
		b, s, disk := fakeBuiltBase(t, h, DefaultBaseImage)
		b.cfg.Base.Registry = testRegistry()
		b.guest = guest
		h.stageGuest(t, "verify=unpullable", "registry=same", "binfmt=ok")
		_, err := b.EnsureWith(context.Background(), false, false)
		var nj *ProbeNotJudgedError
		if !errors.As(err, &nj) {
			t.Fatalf("guest %v: a probe that could not pull answered %v; want it not judged", guest, err)
		}
		_, statErr := os.Stat(filepath.Join(h.state, "registry.script"))
		if ran := statErr == nil; ran == guest {
			t.Errorf("guest %v: the registry run ran %v; only the owner's ensure runs it", guest, ran)
		}
		if s.said("asked once more") {
			t.Errorf("guest %v: the probe was asked again where nothing changed: %v", guest, s.lines)
		}
		assertKept(t, h, b, disk)
	}
}

// ⭐ base status AND config REPORT THE KEY WHERE IT IS SET, and a base without it
// reports as before: no field and no row. WSL-196.
func TestBaseStatusReportsTheRegistryKeyOnlyWhereItIsSet(t *testing.T) {
	cfg := DefaultConfig()
	// ⚠ No registration is read: the naming half of a status asks this, not WSL.
	b := &Base{cfg: cfg, home: t.TempDir(), disks: func() (map[string]string, error) { return map[string]string{}, nil }}
	st, err := b.configuredState()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(st.Access)
	if err != nil {
		t.Fatal(err)
	}
	if st.Access.Registry != nil || strings.Contains(string(raw), "registry") || RegistryRows(cfg.Base.Registry) != nil {
		t.Errorf("a base without base.registry reports %s", raw)
	}
	base, err := json.Marshal(cfg.Base)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(base), "registry") {
		t.Errorf("a configuration without base.registry writes %s", base)
	}
	b.cfg.Base.Registry = testRegistry()
	if st, err = b.configuredState(); err != nil {
		t.Fatal(err)
	}
	if st.Access.Registry == nil || st.Access.Registry.Proxy != "http://proxy.example:3128" {
		t.Errorf("base status does not carry base.registry: %+v", st.Access.Registry)
	}
	rows := RegistryRows(b.cfg.Base.Registry)
	if len(rows) != 4 || rows[0] != [2]string{"mirror", "docker.io through mirror.gcr.io, mirror.example:5000/hub, then docker.io itself"} ||
		rows[3] != [2]string{"proxy", "http://proxy.example:3128, not for localhost, 127.0.0.1, .corp.example"} {
		t.Errorf("the rows read %q", rows)
	}
}

// ⛔ A KEY THAT STATES NOTHING IS NO KEY. `"registry": {}` loads as a file without
// it, so its fingerprint and its reports do not move. WSL-196.
func TestAnEmptyRegistryKeyLoadsAsNone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wsl-toolkit.json")
	if err := os.WriteFile(path, []byte(`{"schema":"`+ConfigSchema+`","base":{"registry":{}},"jobs":{"container_lifecycle":"persistent"}}`), 0o600); err != nil {
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
	if got.Base.Registry != nil {
		t.Errorf("an empty base.registry loaded as %+v", got.Base.Registry)
	}
}
