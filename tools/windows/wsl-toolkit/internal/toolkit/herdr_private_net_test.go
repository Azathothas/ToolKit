// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// -- WSL-159: herdr's server, and every agent it starts, in a namespace of its own ----

func herdrPrivateNetConfig(on bool) Config {
	cfg := adapterBase()
	herdr := BaseAdapter{Name: "herdr", PrivateNet: on}
	cfg.Base.Adapters = []BaseAdapter{herdr}
	return cfg
}

// ⛔ REFUSED ON AN ADAPTER THAT CANNOT USE IT, rather than accepted and dropped: a
// namespace the operator believes the agents are in, and they are not, is the worst
// answer this key could give.
func TestPrivateNetIsRefusedOnAnAdapterThatCannotTakeIt(t *testing.T) {
	cfg := baseConfigWithAdapters(herdrAdapter, BaseAdapter{Name: "pi", PrivateNet: true})
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "private_net") {
		t.Fatalf("Validate() = %v, want a refusal naming private_net", err)
	}
	if err := herdrPrivateNetConfig(true).Validate(); err != nil {
		t.Fatalf("Validate() on herdr = %v, want it accepted", err)
	}
}

// ⭐ The key has to REACH install.sh, and without it nothing does: install.sh removes
// what the key added whenever it reads no 1.
func TestPrivateNetReachesTheHerdrInstaller(t *testing.T) {
	on, err := adapterInstallEnv(herdrPrivateNetConfig(true), BaseAdapter{Name: "herdr", PrivateNet: true})
	if err != nil {
		t.Fatal(err)
	}
	if on["TK_ADAPTER_PRIVATE_NET"] != "1" {
		t.Fatalf("the installer's environment carries %q, want \"1\"", on["TK_ADAPTER_PRIVATE_NET"])
	}
	off, err := adapterInstallEnv(herdrPrivateNetConfig(false), BaseAdapter{Name: "herdr"})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := off["TK_ADAPTER_PRIVATE_NET"]; present {
		t.Fatal("a base that did not ask for a private namespace still carries the variable")
	}
}

// herdrFile answers one of the herdr adapter's carried files, as the executable
// carries it.
func herdrFile(t *testing.T, name string) string {
	t.Helper()
	body, err := adapterTree.ReadFile("adapters/herdr/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// TestTheHerdrDropInPutsTheServerInTheNamespaceAndHoldsItThere holds the five lines
// the confinement is made of, and the names the five files share.
//
// ⛔ EACH LINE IS A WAY OUT WHEN IT IS MISSING. Without NetworkNamespacePath the server
// starts in the shared namespace. Without Requires and After it starts there when the
// namespace could not be linked. Without NoNewPrivileges an agent's sudo leaves it.
// Without InaccessiblePaths an agent's `systemd-run --user` starts a process outside.
func TestTheHerdrDropInPutsTheServerInTheNamespaceAndHoldsItThere(t *testing.T) {
	dropin := herdrFile(t, "private-net.conf")
	for _, line := range []string{
		"Requires=wsl-toolkit-herdr-net.service",
		"After=wsl-toolkit-herdr-net.service",
		"NetworkNamespacePath=/run/netns/wsl-toolkit-herdr",
		"NoNewPrivileges=yes",
		"InaccessiblePaths=-/run/user",
	} {
		if !strings.Contains(dropin, "\n"+line+"\n") {
			t.Errorf("private-net.conf does not carry the line %s", line)
		}
	}
	unit := herdrFile(t, "wsl-toolkit-herdr-net.service")
	script := herdrFile(t, "private-net.sh")
	install := herdrFile(t, "install.sh")
	probe := herdrFile(t, "probe.sh")
	// ⚠ ONE NAMESPACE, ONE SCRIPT AND ONE PID FILE, named alike in every file. A name
	// that drifts in one of them leaves a server in a namespace nothing links.
	for _, want := range []struct{ file, body, text string }{
		{"wsl-toolkit-herdr-net.service", unit, "ExecStart=/usr/local/lib/wsl-toolkit/herdr-private-net start\n"},
		{"wsl-toolkit-herdr-net.service", unit, "PIDFile=/run/wsl-toolkit-herdr-net.pid\n"},
		{"wsl-toolkit-herdr-net.service", unit, "Type=forking\n"},
		{"private-net.sh", script, "NETNS=wsl-toolkit-herdr\n"},
		{"private-net.sh", script, "PIDFILE=${TK_HERDR_NET_PIDFILE:-/run/wsl-toolkit-herdr-net.pid}\n"},
		{"install.sh", install, "LIB_DIR=${TK_HERDR_LIB_DIR:-/usr/local/lib/wsl-toolkit}\n"},
		{"install.sh", install, "NET_SCRIPT=$LIB_DIR/herdr-private-net\n"},
		{"install.sh", install, "NET_UNIT=wsl-toolkit-herdr-net.service\n"},
		{"install.sh", install, "NET_DROPIN=$UNIT_DIR/$UNIT.d/private-net.conf\n"},
		{"probe.sh", probe, "NETNS=wsl-toolkit-herdr\n"},
		{"probe.sh", probe, "NET_DROPIN=${TK_HERDR_UNIT_DIR:-/etc/systemd/system}/$UNIT.d/private-net.conf\n"},
	} {
		if !strings.Contains(want.body, want.text) {
			t.Errorf("%s does not carry %q", want.file, strings.TrimSpace(want.text))
		}
	}
	// ⛔ NOT StopWhenUnneeded: removing the drop-in would stop the link under a server
	// that is still in the namespace.
	if strings.Contains(unit, "\nStopWhenUnneeded=") {
		t.Error("wsl-toolkit-herdr-net.service stops when unneeded, so turning the key off cuts a running server off")
	}
}

// TestTheConfiguredStateCarriesPrivateNet: the status row reads the configuration,
// so `base status` says what the key gives up without a probe. WSL-159.
func TestTheConfiguredStateCarriesPrivateNet(t *testing.T) {
	for _, on := range []bool{false, true} {
		st, err := (&Base{cfg: herdrPrivateNetConfig(on), home: t.TempDir()}).configuredState()
		if err != nil {
			t.Fatal(err)
		}
		if st.Access.HerdrPrivateNet != on {
			t.Errorf("a configuration with private_net %v answered herdr_private_net %v", on, st.Access.HerdrPrivateNet)
		}
	}
	// ⚠ ONLY THE HERDR ENTRY'S KEY COUNTS. Validate refuses it anywhere else, and a
	// state read from a configuration Validate never saw must not claim it either.
	cfg := herdrPrivateNetConfig(false)
	cfg.Base.Adapters = append(cfg.Base.Adapters, BaseAdapter{Name: "pi", PrivateNet: true})
	st, err := (&Base{cfg: cfg, home: t.TempDir()}).configuredState()
	if err != nil {
		t.Fatal(err)
	}
	if st.Access.HerdrPrivateNet {
		t.Error("a private_net on the pi entry made herdr's namespace read as private")
	}
}

// TestAHerdrServerOutsideItsNamespaceEarnsTheRestart: the probe's facts become the one
// command that ends the condition, and a base that did not ask earns nothing.
//
// ⛔ THE RESTART IS ADVISORY. base ensure never restarts a running server. A
// remediation `ready` names as the way forward would send a caller round a command
// that ends every pane. The link that is down is not: base ensure starts it. WSL-159.
func TestAHerdrServerOutsideItsNamespaceEarnsTheRestart(t *testing.T) {
	read := func(facts map[string]string) (Remediation, bool) {
		return herdrNetRemediation(AdapterState{Name: "herdr", Facts: facts}, true)
	}
	r, ok := read(map[string]string{"private-net": "on", "server-netns": "shared"})
	if !ok || r.ID != "herdr-private-net-pending" || !strings.Contains(r.Command, "systemctl restart wsl-toolkit-herdr.service") {
		t.Fatalf("a server in the shared namespace under the key earned %+v, %v", r, ok)
	}
	if !r.Advisory || !r.Repairable {
		t.Fatalf("the restart that moves the server is not an advisory one: %+v", r)
	}
	r, ok = read(map[string]string{"private-net": "on", "server-netns": "private", "private-net-link": "failed"})
	if !ok || r.ID != "herdr-private-net-down" || r.Advisory || !strings.Contains(r.Command, "systemctl start wsl-toolkit-herdr-net.service") {
		t.Fatalf("a namespace with no link earned %+v, %v", r, ok)
	}
	for _, facts := range []map[string]string{
		{"private-net": "on", "server-netns": "private", "private-net-link": "active"},
		{"private-net": "on", "server-netns": "unknown"},
	} {
		if r, ok := read(facts); ok {
			t.Errorf("%v earned %+v, and it needs nothing run", facts, r)
		}
	}
	// ⚠ A BASE THAT NEVER ASKED, and whose probe printed none of the namespace's facts.
	for _, facts := range []map[string]string{{"server": "active"}, {"private-net": "off", "server-netns": "private"}} {
		if r, ok := herdrNetRemediation(AdapterState{Name: "herdr", Facts: facts}, false); ok {
			t.Errorf("%v without the key earned %+v, and it needs nothing run", facts, r)
		}
	}
}

// TestAPrivateNetKeyNotYetEnsuredEarnsTheEnsure: the probe's private-net is the drop-in
// the base carries, and the configuration may say otherwise until base ensure runs.
//
// ⛔ THE CONFIGURATION WINS OVER THE DROP-IN. A key added and not ensured read as off,
// and earned nothing while the panes reached the Windows host. A key removed and not
// ensured earned a restart, which would put the server INTO the namespace. WSL-159.
func TestAPrivateNetKeyNotYetEnsuredEarnsTheEnsure(t *testing.T) {
	for _, c := range []struct {
		configured bool
		facts      map[string]string
	}{
		{true, map[string]string{"server": "active"}},
		{false, map[string]string{"private-net": "on", "server-netns": "shared", "private-net-link": "inactive"}},
	} {
		r, ok := herdrNetRemediation(AdapterState{Name: "herdr", Facts: c.facts}, c.configured)
		if !ok || r.ID != "herdr-private-net-unapplied" || !strings.HasSuffix(r.Command, "base ensure") || !r.Advisory {
			t.Errorf("private_net %v in the configuration over %v earned %+v, %v; want base ensure", c.configured, c.facts, r, ok)
		}
	}
	// ⚠ A PROBE THAT ANSWERED NOTHING IS NOT A BASE WITHOUT THE KEY.
	if r, ok := herdrNetRemediation(AdapterState{Name: "herdr"}, true); ok {
		t.Errorf("a probe that answered no fact earned %+v", r)
	}
}

// -- the guest halves, under a POSIX shell ---------------------------------------------

// stubDir makes a directory of stand-ins and answers a writer for them.
func stubDir(t *testing.T) (string, func(name, body string)) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// runSh runs a script under sh with the given environment and stand-ins first on
// PATH, and answers its exit code and its combined output.
func runSh(t *testing.T, script []byte, bin string, args []string, env ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{"-s", "--"}, args...)...)
	cmd.Stdin = strings.NewReader(string(script))
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	var exited *exec.ExitError
	if err != nil && !errors.As(err, &exited) {
		t.Fatalf("the script did not start: %v", err)
	}
	return cmd.ProcessState.ExitCode(), string(out)
}

func needPosix(t *testing.T, tools ...string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the guest half runs under a POSIX shell, which the Linux job has")
	}
	for _, tool := range append([]string{"sh"}, tools...) {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not on PATH", tool)
		}
	}
}

// TestThePrivateNetScriptWritesTheRulesAndLinksNothingElse runs private-net.sh against
// stand-ins for ip, nft and pasta. WSL-159.
//
// ⛔ WHAT IT HOLDS IS WHAT `base exec --private-net` WAS MEASURED MISSING: there pasta
// carried every loopback port of the shared namespace in, and the resolver's address
// was accepted whole. Here pasta forwards nothing either way, the gateway is refused,
// and the resolver is reached on port 53 alone.
func TestThePrivateNetScriptWritesTheRulesAndLinksNothingElse(t *testing.T) {
	needPosix(t, "sed", "mktemp")
	script := []byte(herdrFile(t, "private-net.sh"))
	root := t.TempDir()
	netns, rules, asked := filepath.Join(root, "netns"), filepath.Join(root, "rules"), filepath.Join(root, "asked")
	resolv := filepath.Join(root, "resolv.conf")
	if err := os.MkdirAll(netns, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resolv, []byte("# WSL's\nnameserver 10.255.255.254\nnameserver 172.23.96.1\nsearch lan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin, write := stubDir(t)
	write("nft", "exit 0\n")
	// ip records every call, makes the namespace's name, and hands nft's file to the
	// case. It answers the shared namespace's default route and the pids STUB_PIDS names.
	write("ip", `printf '%s\n' "$*" >> "$STUB_ASKED"
case "$1 $2" in
  "netns add") : > "$STUB_NETNS/$3" ;;
  "netns delete") rm -f "$STUB_NETNS/$3" ;;
  "netns pids") [ -z "${STUB_PIDS:-}" ] || printf '%s\n' "$STUB_PIDS" ;;
  "netns exec")
    shift 3
    if [ "$1" = nft ] && [ "$2" = -f ]; then cp "$3" "$STUB_RULES"; fi ;;
  "-4 route") printf 'default via 172.23.96.1 dev eth0 proto kernel onlink\n' ;;
esac
exit 0
`)
	write("pasta", `printf 'pasta %s\n' "$*" >> "$STUB_ASKED"
exit 0
`)
	env := []string{"TK_NETNS_DIR=" + netns, "TK_HERDR_NET_PIDFILE=" + filepath.Join(root, "pid"),
		"TK_RESOLV_CONF=" + resolv, "STUB_NETNS=" + netns, "STUB_RULES=" + rules, "STUB_ASKED=" + asked}

	code, out := runSh(t, script, bin, []string{"start"}, env...)
	if code != 0 {
		t.Fatalf("start exited %d:\n%s", code, out)
	}
	got, err := os.ReadFile(rules)
	if err != nil {
		t.Fatalf("no rules reached nft: %v\n%s", err, out)
	}
	text := string(got)
	// ⛔ IN THIS ORDER, because an nftables chain runs in order.
	order := []string{
		"delete table inet wsl_toolkit_private\n",
		"ip daddr 10.255.255.254 udp dport 53 accept\n",
		"ip daddr 10.255.255.254 tcp dport 53 accept\n",
		"ip daddr 172.23.96.1 udp dport 53 accept\n",
		"ip daddr 172.23.96.1 drop\n",
		"ip daddr { 10.0.0.0/8, 100.64.0.0/10, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16 } drop\n",
	}
	at := 0
	for _, want := range order {
		i := strings.Index(text[at:], want)
		if i < 0 {
			t.Fatalf("the rules do not carry %q after what comes before it:\n%s", strings.TrimSpace(want), text)
		}
		at += i + len(want)
	}
	// ⛔ A RESOLVER THAT IS THE WINDOWS HOST IS OPEN ON PORT 53 ALONE.
	if regexp.MustCompile(`(?m)ip daddr \S+ accept$`).MatchString(text) {
		t.Fatalf("a resolver's address is accepted whole, which opens the Windows host where it is the resolver:\n%s", text)
	}
	calls, _ := os.ReadFile(asked)
	var pasta string
	for _, line := range strings.Split(string(calls), "\n") {
		if strings.HasPrefix(line, "pasta ") {
			pasta = " " + line + " "
		}
	}
	for _, flag := range []string{" -4 ", " --config-net ", " --netns " + filepath.Join(netns, "wsl-toolkit-herdr") + " ",
		" -t none ", " -u none ", " -T none ", " -U none ", " --no-map-gw ", " --runas 0:0 "} {
		if !strings.Contains(pasta, flag) {
			t.Errorf("pasta was started without %q: %s", strings.TrimSpace(flag), pasta)
		}
	}
	if !strings.Contains(string(calls), "netns add wsl-toolkit-herdr") {
		t.Errorf("start made no namespace:\n%s", calls)
	}

	// ⛔ A STOP UNDER A RUNNING SERVER KEEPS THE NAMESPACE, so the next start relinks it.
	_ = os.Remove(asked)
	if code, out := runSh(t, script, bin, []string{"stop"}, append(env, "STUB_PIDS=4242")...); code != 0 {
		t.Fatalf("stop exited %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(netns, "wsl-toolkit-herdr")); err != nil {
		t.Fatal("a stop removed the namespace while a process was still in it")
	}
	if code, out := runSh(t, script, bin, []string{"stop"}, env...); code != 0 {
		t.Fatalf("stop exited %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(netns, "wsl-toolkit-herdr")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a stop left an empty namespace behind")
	}

	// ⭐ rules WRITES THE RULES INTO A RUNNING NAMESPACE AND NOTHING ELSE: no pasta, and
	// no new namespace. install.sh runs it when this script changes under a server.
	// WSL-159.
	if code, out := runSh(t, script, bin, []string{"rules"}, env...); code == 0 {
		t.Fatalf("rules wrote into a namespace that is not there:\n%s", out)
	}
	if err := os.WriteFile(filepath.Join(netns, "wsl-toolkit-herdr"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(asked)
	_ = os.Remove(rules)
	if code, out := runSh(t, script, bin, []string{"rules"}, env...); code != 0 {
		t.Fatalf("rules exited %d:\n%s", code, out)
	}
	calls, _ = os.ReadFile(asked)
	if got, err := os.ReadFile(rules); err != nil || !strings.Contains(string(got), "100.64.0.0/10") {
		t.Fatalf("rules wrote no rule set into the namespace (%v):\n%s", err, calls)
	}
	for _, unwanted := range []string{"pasta ", "netns add ", "sysctl"} {
		if strings.Contains(string(calls), unwanted) {
			t.Errorf("rules ran %q, and it touches nothing but the rules:\n%s", unwanted, calls)
		}
	}
}

// TestPrivateNetAddsADropInAndLeavesTheServersUnitAlone runs the herdr adapter's
// install.sh as far as enabling its unit, against stand-ins, with the key and without
// it. WSL-159.
//
// ⭐ WITHOUT THE KEY NOTHING CHANGES: the server's own unit is the same bytes either way,
// and the three files the key adds are there only while it is on.
// ⛔ A SERVER STILL IN THE NAMESPACE KEEPS ITS LINK when the key goes: the drop-in goes,
// and the unit that links the namespace stays until no process is left in it.
func TestPrivateNetAddsADropInAndLeavesTheServersUnitAlone(t *testing.T) {
	needPosix(t, "cat", "cut", "mktemp", "sha256sum", "base64", "cp", "mkdir", "chmod", "mv", "rm", "rmdir", "sed")
	script, err := adapterScript("herdr", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	files, err := adapterFileEnv("herdr")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	home, lib, units, dest := filepath.Join(root, "home"), filepath.Join(root, "lib"), filepath.Join(root, "units"), filepath.Join(root, "bin-herdr")
	for _, dir := range []string{home, lib, units, dest} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	asked := filepath.Join(root, "asked")
	served := []byte("a herdr the case serves\n")
	sum := sha256.Sum256(served)
	bin, write := stubDir(t)
	realCat, _ := exec.LookPath("cat")
	for _, name := range []string{"pacman", "sshd", "ssh-keygen", "ss", "pasta", "nft", "chown"} {
		write(name, "exit 0\n")
	}
	write("cat", "if [ \"$1\" = /proc/1/comm ]; then echo systemd; exit 0; fi\nexec "+realCat+" \"$@\"\n")
	write("getent", "printf 'herdr:x:1000:1000::%s:/bin/bash\\n' \"$STUB_HOME\"\n")
	write("id", "echo herdr\n")
	write("uname", "echo x86_64\n")
	write("curl", "while [ $# -gt 0 ]; do if [ \"$1\" = --output ]; then printf 'a herdr the case serves\\n' > \"$2\"; shift; fi; shift; done\n")
	// install makes directories and copies files, whatever owner it is asked for.
	// ⚠ THE DOOR'S DIRECTORY IS WHERE THE CASE STOPS THE SCRIPT. Every unit is written
	// by then and the server step has spoken. Nothing is yet written outside the case's
	// directory.
	write("install", `case " $* " in *" /etc/wsl-toolkit "*) echo 'install: stopped here by the case' >&2; exit 7 ;; esac
dir=; mode=
while [ $# -gt 0 ]; do
  case $1 in
    -d) dir=1 ;;
    -o|-g) shift ;;
    -m) shift; mode=$1 ;;
    *) break ;;
  esac
  shift
done
if [ -n "$dir" ]; then mkdir -p "$@"; exit; fi
cp "$1" "$2" && { [ -z "$mode" ] || chmod "$mode" "$2"; }
`)
	// systemctl records every call, in order. The server's unit is active where
	// STUB_ACTIVE says so, and the namespace's unit where STUB_NET_ACTIVE does.
	write("systemctl", `printf 'systemctl %s\n' "$*" >> "$STUB_ASKED"
case $1 in
  is-active)
    case " $* " in
      *" wsl-toolkit-herdr-net.service "*) [ "${STUB_NET_ACTIVE:-}" = yes ] ;;
      *) [ "${STUB_ACTIVE:-}" = yes ] ;;
    esac
    exit ;;
  is-enabled) echo masked ;;
esac
exit 0
`)
	// ip records every call but pids, and answers the pids STUB_PIDS names.
	write("ip", `if [ "$1 $2" = 'netns pids' ]; then [ -z "${STUB_PIDS:-}" ] || printf '%s\n' "$STUB_PIDS"; exit 0; fi
printf 'ip %s\n' "$*" >> "$STUB_ASKED"
exit 0
`)
	netns := filepath.Join(root, "netns")
	if err := os.MkdirAll(netns, 0o755); err != nil {
		t.Fatal(err)
	}
	resolv := filepath.Join(root, "resolv.conf")
	if err := os.WriteFile(resolv, []byte("nameserver 10.255.255.254\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := []string{"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base", "TK_SSH_CLIENT_KEY=ssh-ed25519 AAAA",
		"TK_HERDR_BIN=" + filepath.Join(dest, "herdr"), "TK_HERDR_LIB_DIR=" + lib, "TK_HERDR_UNIT_DIR=" + units,
		"TK_NETNS_DIR=" + netns, "TK_RESOLV_CONF=" + resolv,
		"TK_ADAPTER_VERSION=0.10.1", "TK_ADAPTER_SHA256_X86_64=" + hex.EncodeToString(sum[:]),
		"STUB_HOME=" + home, "STUB_ASKED=" + asked, "STUB_ACTIVE=yes", "STUB_NET_ACTIVE=yes"}
	for k, v := range files {
		env = append(env, k+"="+v)
	}
	run := func(extra ...string) string {
		t.Helper()
		_ = os.Remove(asked)
		code, out := runSh(t, script, bin, nil, append(append([]string(nil), env...), extra...)...)
		if code != 7 || !strings.Contains(out, "stopped here by the case") {
			t.Fatalf("install.sh exited %d, and the case stops it at the door's directory:\n%s", code, out)
		}
		return out
	}
	read := func(path string) (string, bool) {
		b, err := os.ReadFile(path)
		return string(b), err == nil
	}
	added := map[string]string{
		filepath.Join(lib, "herdr-private-net"):                                 "private-net.sh",
		filepath.Join(units, "wsl-toolkit-herdr-net.service"):                   "wsl-toolkit-herdr-net.service",
		filepath.Join(units, "wsl-toolkit-herdr.service.d", "private-net.conf"): "private-net.conf",
	}

	out := run("TK_ADAPTER_PRIVATE_NET=1")
	withKey, ok := read(filepath.Join(units, "wsl-toolkit-herdr.service"))
	if !ok {
		t.Fatalf("no server unit was written:\n%s", out)
	}
	for path, name := range added {
		got, ok := read(path)
		if !ok || got != herdrFile(t, name) {
			t.Errorf("with the key, %s is not the adapter's %s:\n%s", path, name, out)
		}
	}
	if info, err := os.Stat(filepath.Join(lib, "herdr-private-net")); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("the namespace's script is not executable: %v", err)
	}
	if !strings.Contains(out, "keeps the network namespace it started in") {
		t.Errorf("a running server under a newly written drop-in was not told it keeps its namespace:\n%s", out)
	}

	// The key goes while a process is still in the namespace.
	out = run("STUB_PIDS=4242")
	if _, ok := read(filepath.Join(units, "wsl-toolkit-herdr.service.d", "private-net.conf")); ok {
		t.Fatalf("the drop-in outlived the key, so the server's next start is still private:\n%s", out)
	}
	for _, path := range []string{filepath.Join(lib, "herdr-private-net"), filepath.Join(units, "wsl-toolkit-herdr-net.service")} {
		if _, ok := read(path); !ok {
			t.Errorf("%s went while a process was still in the namespace, which cuts a running server off:\n%s", path, out)
		}
	}

	// And again once nothing is in it.
	out = run()
	for path := range added {
		if _, ok := read(path); ok {
			t.Errorf("%s outlived the key once the namespace was empty:\n%s", path, out)
		}
	}
	if _, err := os.Stat(filepath.Join(units, "wsl-toolkit-herdr.service.d")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the drop-in's directory outlived the key")
	}
	without, _ := read(filepath.Join(units, "wsl-toolkit-herdr.service"))
	if without != withKey {
		t.Fatalf("the server's own unit changed with the key:\nwith\n%s\nwithout\n%s", withKey, without)
	}
	if strings.Contains(without, "NetworkNamespacePath") {
		t.Fatal("the server's own unit names a namespace, so the key cannot leave it")
	}
	if strings.Contains(out, "keeps the network namespace it started in") {
		t.Errorf("an ensure that changed nothing about the server's namespace told it to restart:\n%s", out)
	}

	// ⛔ THE KEY GOES WHILE IT IS STILL PENDING: the server runs in the shared namespace,
	// and no process is in the private one. systemd must read the units again before
	// the namespace's unit stops. Until then the server still requires that unit, and
	// the stop would stop the server and end every pane. WSL-159.
	run("TK_ADAPTER_PRIVATE_NET=1")
	out = run()
	recorded, _ := os.ReadFile(asked)
	calls := string(recorded)
	reload, stop := strings.Index(calls, "systemctl daemon-reload\n"), strings.Index(calls, "systemctl stop wsl-toolkit-herdr-net.service\n")
	if stop < 0 || reload < 0 || reload > stop {
		t.Fatalf("the namespace's unit was stopped before systemd read the units without the drop-in:\n%s\n%s", calls, out)
	}
	if strings.Contains(calls, "systemctl start wsl-toolkit-herdr.service") || strings.Contains(calls, "systemctl restart") {
		t.Fatalf("turning the key off started or restarted a running server:\n%s", calls)
	}
}

// TestPrivateNetKeepsARunningNamespaceCurrent runs the herdr adapter's install.sh with
// the key, over a namespace a process is in. WSL-159.
//
// ⭐ base ensure CURES WHAT IT CAN WITHOUT ENDING A PANE. A link that is down is started
// again, and a changed rule set is written into the running namespace. Neither
// restarts the server, and before this the rules waited for WSL's VM to restart.
func TestPrivateNetKeepsARunningNamespaceCurrent(t *testing.T) {
	needPosix(t, "cat", "cut", "mktemp", "sha256sum", "base64", "cp", "mkdir", "chmod", "mv", "rm", "rmdir", "sed")
	script, err := adapterScript("herdr", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	files, err := adapterFileEnv("herdr")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	home, lib, units, dest, netns := filepath.Join(root, "home"), filepath.Join(root, "lib"), filepath.Join(root, "units"), filepath.Join(root, "bin-herdr"), filepath.Join(root, "netns")
	for _, dir := range []string{home, lib, units, dest, netns} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// ⚠ A NAMESPACE THE SERVER IS IN, as `ip netns add` leaves its name.
	if err := os.WriteFile(filepath.Join(netns, "wsl-toolkit-herdr"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	resolv := filepath.Join(root, "resolv.conf")
	if err := os.WriteFile(resolv, []byte("nameserver 10.255.255.254\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	asked := filepath.Join(root, "asked")
	served := []byte("a herdr the case serves\n")
	sum := sha256.Sum256(served)
	bin, write := stubDir(t)
	realCat, _ := exec.LookPath("cat")
	for _, name := range []string{"pacman", "sshd", "ssh-keygen", "ss", "pasta", "nft", "chown"} {
		write(name, "exit 0\n")
	}
	write("cat", "if [ \"$1\" = /proc/1/comm ]; then echo systemd; exit 0; fi\nexec "+realCat+" \"$@\"\n")
	write("getent", "printf 'herdr:x:1000:1000::%s:/bin/bash\\n' \"$STUB_HOME\"\n")
	write("id", "echo herdr\n")
	write("uname", "echo x86_64\n")
	write("curl", "while [ $# -gt 0 ]; do if [ \"$1\" = --output ]; then printf 'a herdr the case serves\\n' > \"$2\"; shift; fi; shift; done\n")
	write("install", `case " $* " in *" /etc/wsl-toolkit "*) echo 'install: stopped here by the case' >&2; exit 7 ;; esac
dir=; mode=
while [ $# -gt 0 ]; do
  case $1 in
    -d) dir=1 ;;
    -o|-g) shift ;;
    -m) shift; mode=$1 ;;
    *) break ;;
  esac
  shift
done
if [ -n "$dir" ]; then mkdir -p "$@"; exit; fi
cp "$1" "$2" && { [ -z "$mode" ] || chmod "$mode" "$2"; }
`)
	write("systemctl", `printf 'systemctl %s\n' "$*" >> "$STUB_ASKED"
case $1 in
  is-active)
    case " $* " in
      *" wsl-toolkit-herdr-net.service "*) [ "${STUB_NET_ACTIVE:-}" = yes ] ;;
      *) exit 0 ;;
    esac
    exit ;;
  is-enabled) echo masked ;;
esac
exit 0
`)
	write("ip", `if [ "$1 $2" = 'netns pids' ]; then printf '4242\n'; exit 0; fi
printf 'ip %s\n' "$*" >> "$STUB_ASKED"
exit 0
`)
	env := []string{"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base", "TK_SSH_CLIENT_KEY=ssh-ed25519 AAAA",
		"TK_HERDR_BIN=" + filepath.Join(dest, "herdr"), "TK_HERDR_LIB_DIR=" + lib, "TK_HERDR_UNIT_DIR=" + units,
		"TK_NETNS_DIR=" + netns, "TK_RESOLV_CONF=" + resolv, "TK_ADAPTER_PRIVATE_NET=1",
		"TK_ADAPTER_VERSION=0.10.1", "TK_ADAPTER_SHA256_X86_64=" + hex.EncodeToString(sum[:]),
		"STUB_HOME=" + home, "STUB_ASKED=" + asked}
	for k, v := range files {
		env = append(env, k+"="+v)
	}
	run := func(extra ...string) (string, string) {
		t.Helper()
		_ = os.Remove(asked)
		code, out := runSh(t, script, bin, nil, append(append([]string(nil), env...), extra...)...)
		if code != 7 || !strings.Contains(out, "stopped here by the case") {
			t.Fatalf("install.sh exited %d, and the case stops it at the door's directory:\n%s", code, out)
		}
		calls, _ := os.ReadFile(asked)
		return out, string(calls)
	}

	// The first ensure writes the script, with the link up: its rules go in in place.
	out, calls := run("STUB_NET_ACTIVE=yes")
	if !strings.Contains(calls, "ip netns exec wsl-toolkit-herdr nft -f ") || !strings.Contains(out, "wrote the changed rules into the running namespace") {
		t.Fatalf("a changed script's rules did not reach the running namespace:\n%s\n%s", calls, out)
	}
	if strings.Contains(calls, "systemctl start wsl-toolkit-herdr-net.service") || strings.Contains(calls, "pasta") {
		t.Fatalf("a namespace whose link is up was linked again:\n%s", calls)
	}
	// The same script again writes nothing into it.
	if _, calls = run("STUB_NET_ACTIVE=yes"); strings.Contains(calls, "nft -f") {
		t.Fatalf("an unchanged script rewrote the running namespace's rules:\n%s", calls)
	}
	// ⛔ A LINK THAT IS DOWN UNDER A SERVER IN THE NAMESPACE IS STARTED AGAIN.
	out, calls = run()
	if !strings.Contains(calls, "systemctl start wsl-toolkit-herdr-net.service\n") || !strings.Contains(out, "ends no pane") {
		t.Fatalf("a namespace with a process in it and no link was left without one:\n%s\n%s", calls, out)
	}
	if strings.Contains(calls, "systemctl start wsl-toolkit-herdr.service") || strings.Contains(calls, "systemctl restart") {
		t.Fatalf("relinking the namespace started or restarted the server:\n%s", calls)
	}
}

// TestTheHerdrProbeSaysWhichNamespaceTheServerIsIn runs the herdr adapter's probe.sh
// against stand-ins for systemd and the kernel's namespace links. WSL-159.
//
// ⛔ THE SERVER'S NAMESPACE, NOT ITS UNIT'S: a key turned on reaches a running server
// only at its next start, and until then the agents are in the shared namespace.
func TestTheHerdrProbeSaysWhichNamespaceTheServerIsIn(t *testing.T) {
	needPosix(t, "sed", "grep", "cut")
	probe, err := adapterScript("herdr", "probe.sh")
	if err != nil {
		t.Fatal(err)
	}
	units := t.TempDir()
	bin, write := stubDir(t)
	write("systemctl", `case "$1" in
  show) echo 4242 ;;
  is-active) [ "$2" = wsl-toolkit-herdr-net.service ] && { echo "${STUB_LINK:-active}"; exit 0; }; echo active ;;
  is-enabled) echo masked ;;
esac
exit 0
`)
	write("runuser", "exit 0\n")
	write("ip", "if [ \"$1 $2\" = 'netns identify' ]; then printf '%s\\n' \"${STUB_IDENTIFY:-}\"; fi\nexit 0\n")
	write("readlink", `case $1 in
  /proc/1/ns/net) echo 'net:[4026531833]' ;;
  /proc/4242/ns/net) echo "${STUB_NS:-net:[4026531833]}" ;;
esac
`)
	dropin := filepath.Join(units, "wsl-toolkit-herdr.service.d", "private-net.conf")
	read := func(on bool, extra ...string) AdapterState {
		t.Helper()
		_ = os.RemoveAll(filepath.Dir(dropin))
		if on {
			if err := os.MkdirAll(filepath.Dir(dropin), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dropin, []byte(herdrFile(t, "private-net.conf")), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		code, out := runSh(t, probe, bin, nil, append([]string{"TK_USER=herdr", "TK_HERDR_UNIT_DIR=" + units}, extra...)...)
		if code != 0 {
			t.Fatalf("probe.sh exited %d, and a probe exits 0 whatever it finds:\n%s", code, out)
		}
		return parseAdapterProbe("herdr", out)
	}
	about := func(st AdapterState, word string) []string {
		var found []string
		for _, p := range st.Problems {
			if strings.Contains(p, word) {
				found = append(found, p)
			}
		}
		return found
	}
	noted := func(st AdapterState, word string) bool {
		for _, n := range st.Notes {
			if strings.Contains(n, word) {
				return true
			}
		}
		return false
	}
	private := []string{"STUB_IDENTIFY=wsl-toolkit-herdr", "STUB_NS=net:[4026532353]"}

	st := read(true, private...)
	if st.Facts["private-net"] != "on" || st.Facts["server-netns"] != "private" || len(about(st, "namespace")) != 0 {
		t.Fatalf("a server in its namespace under the key was read as %v with %v", st.Facts, st.Problems)
	}
	if !noted(st, "rootless containers and sudo") {
		t.Fatalf("a base under the key was not told what it gives up: %v", st.Notes)
	}
	// ⚠ AND THAT THE PANES ARE ALL IT CONFINES.
	if !noted(st, "processes in herdr's panes") || !noted(st, "interop") || !noted(st, "base agent") {
		t.Fatalf("a base under the key was told the confinement reaches past the panes: %v", st.Notes)
	}

	// ⛔ A SERVER STILL IN THE SHARED NAMESPACE IS A NOTE, AND THE ENSURE PASSES. base
	// ensure never restarts a running server, so it cannot cure this. As a problem it
	// failed every ensure, ready and job until a restart ended every pane. WSL-181.
	// ⚠ The stand-ins carry no herdr and no door, so every read has their problems.
	// What counts is that the key adds none.
	without := read(false)
	st = read(true)
	if st.Facts["server-netns"] != "shared" || len(about(st, "namespace")) != 0 || !noted(st, "still runs in the shared network namespace") {
		t.Fatalf("a server still in the shared namespace under the key was read as %v with %v and %v", st.Facts, st.Problems, st.Notes)
	}
	if strings.Join(st.Problems, "\n") != strings.Join(without.Problems, "\n") {
		t.Fatalf("a server base ensure never restarts added problems, which fail the ensure:\nwith the key\n%s\nwithout it\n%s",
			strings.Join(st.Problems, "\n"), strings.Join(without.Problems, "\n"))
	}
	if r, ok := herdrNetRemediation(st, true); !ok || r.ID != "herdr-private-net-pending" || !r.Advisory {
		t.Fatalf("that read earned %+v, %v", r, ok)
	}
	st = read(true, append(private, "STUB_LINK=failed")...)
	if len(about(st, "no way out")) != 1 {
		t.Fatalf("a namespace whose link failed was read as %v with %v", st.Facts, st.Problems)
	}
	st = read(false, private...)
	if st.Facts["private-net"] != "off" || len(about(st, "namespace")) != 0 || !noted(st, "keeps its private network namespace") {
		t.Fatalf("a server still in the namespace after the key went was read as %v with %v and %v", st.Facts, st.Problems, st.Notes)
	}
	// ⭐ A BASE THAT NEVER ASKED ANSWERS WHAT IT ANSWERED BEFORE THE KEY EXISTED.
	st = read(false)
	for _, fact := range []string{"private-net", "server-netns", "private-net-link"} {
		if v, ok := st.Facts[fact]; ok {
			t.Errorf("a base that never asked answered the fact %s %s", fact, v)
		}
	}
	if len(about(st, "namespace")) != 0 || noted(st, "namespace") {
		t.Fatalf("a base that never asked was read as %v with %v and %v", st.Facts, st.Problems, st.Notes)
	}
}
