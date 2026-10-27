// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// -- a jq for the cases, because the Go image has none ------------------------------

// jqStandInEnv makes this test binary answer as jq, for the few programs the pi
// probe runs.
//
// ⛔ golang:1.25, where the Linux proof runs every case, carries no jq, and the pi
// probe reads its startup model only through jq. A case that skipped there would leave
// its rows proved nowhere, and one that used a host's jq where it has one would answer
// differently by host. So every host gets this one. WSL-135.
//
// ⚠ IT KNOWS ONLY THE PROGRAMS THE PROBE RUNS, by their text, and refuses any other
// with jq's own compile-error code, 3. A probe that changes a program fails its case
// until the change is taught here, which is the point.
const jqStandInEnv = "WTK_JQ_STAND_IN"

func init() {
	if os.Getenv(jqStandInEnv) == "1" {
		os.Exit(jqStandIn(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
	}
}

// jqFieldOrEmpty is `.KEY // ""`.
var jqFieldOrEmpty = regexp.MustCompile(`^\.([A-Za-z_]+) // ""$`)

func jqStandIn(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	raw, status := false, false
	vars := map[string]string{}
	program, programSet := "", false
	var files []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-r":
			raw = true
		case a == "-e":
			status = true
		case a == "-c":
		case a == "--arg" && i+2 < len(args):
			vars[args[i+1]] = args[i+2]
			i += 2
		case !programSet:
			program, programSet = strings.TrimSpace(a), true
		default:
			files = append(files, a)
		}
	}
	input := stdin
	if len(files) > 0 {
		f, err := os.Open(files[0])
		if err != nil {
			fmt.Fprintln(stderr, "jq: error: Could not open "+files[0])
			return 2
		}
		defer f.Close()
		input = f
	}
	var doc any
	if err := json.NewDecoder(input).Decode(&doc); err != nil {
		fmt.Fprintln(stderr, "jq: error: the input is not JSON:", err)
		return 2
	}
	out, err := jqEval(program, doc, vars)
	if err != nil {
		fmt.Fprintln(stderr, "jq stand-in: "+err.Error())
		return 3
	}
	if s, ok := out.(string); ok && raw {
		fmt.Fprintln(stdout, s)
	} else {
		b, _ := json.Marshal(out)
		fmt.Fprintln(stdout, string(b))
	}
	if status && (out == nil || out == false) {
		return 1
	}
	return 0
}

func jqEval(program string, doc any, vars map[string]string) (any, error) {
	obj, _ := doc.(map[string]any)
	models := func() []any {
		providers, _ := obj["providers"].(map[string]any)
		provider, _ := providers[vars["p"]].(map[string]any)
		list, _ := provider["models"].([]any)
		return list
	}
	if m := jqFieldOrEmpty.FindStringSubmatch(program); m != nil {
		if v, ok := obj[m[1]]; ok && v != nil && v != false {
			return v, nil
		}
		return "", nil
	}
	switch program {
	case `type == "object"`:
		return obj != nil, nil
	case `.providers | has($p)`:
		providers, ok := obj["providers"].(map[string]any)
		if !ok {
			return nil, errors.New("has() of a value that is not an object")
		}
		_, found := providers[vars["p"]]
		return found, nil
	case `(.providers[$p].models // []) | any(.id == $m)`:
		for _, m := range models() {
			if mm, _ := m.(map[string]any); mm["id"] == vars["m"] {
				return true, nil
			}
		}
		return false, nil
	case `(.providers[$p].models // []) | any(.id == $m and ((.thinkingLevelMap // {}) | has($e)))`:
		for _, m := range models() {
			mm, _ := m.(map[string]any)
			levels, _ := mm["thinkingLevelMap"].(map[string]any)
			if _, ok := levels[vars["e"]]; ok && mm["id"] == vars["m"] {
				return true, nil
			}
		}
		return false, nil
	}
	return nil, fmt.Errorf("a program this stand-in does not know: %s", program)
}

// -- the rig the agent adapters' scripts run in ------------------------------------

// agentRig is one account's home and the stand-ins an adapter script reaches, all
// under one temporary directory.
type agentRig struct {
	t                                  *testing.T
	root, home, bin, prefixBin, system string
}

func newAgentRig(t *testing.T) *agentRig {
	t.Helper()
	root := t.TempDir()
	r := &agentRig{t: t, root: root, home: filepath.Join(root, "home"), bin: filepath.Join(root, "bin"), system: filepath.Join(root, "usr-local-bin")}
	r.prefixBin = filepath.Join(r.home, ".local", "bin")
	for _, dir := range []string{r.bin, r.prefixBin, r.system} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	real := map[string]string{}
	for _, tool := range []string{"id", "uname"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s is not on PATH", tool)
		}
		real[tool] = p
	}
	r.write(filepath.Join(r.bin, "getent"), "#!/bin/sh\nprintf 'herdr:x:1000:1000::%s:/bin/bash\\n' "+shellQuote(r.home)+"\n")
	r.write(filepath.Join(r.bin, "id"), "#!/bin/sh\nif [ \"$1\" = -gn ]; then exec "+shellQuote(real["id"])+" -gn; fi\nexec "+shellQuote(real["id"])+" \"$@\"\n")
	r.write(filepath.Join(r.bin, "uname"), "#!/bin/sh\nif [ \"$1\" = -m ]; then echo \"${STUB_ARCH:-x86_64}\"; exit 0; fi\nexec "+shellQuote(real["uname"])+" \"$@\"\n")
	// ⭐ runuser -u runs the command as this process, and runuser -l answers what a login
	// shell would resolve, which is the wrapper this rig's system directory holds.
	r.write(filepath.Join(r.bin, "runuser"), `#!/bin/sh
case $1 in
  -u)
    [ "$2" = herdr ] && [ "$3" = -- ] || exit 9
    shift 3
    exec "$@"
    ;;
  -l)
    [ "$2" = herdr ] && [ "$3" = -c ] || exit 9
    case $4 in
      "command -v "*)
        name=${4#command -v }
        [ -x `+shellQuote(r.system)+`/"$name" ] && printf '%s\n' `+shellQuote(r.system)+`/"$name"
        exit 0
        ;;
    esac
    exit 9
    ;;
esac
exit 9
`)
	return r
}

func (r *agentRig) write(path, body string) {
	r.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		r.t.Fatal(err)
	}
}

// withJQ puts the jq stand-in where root and the account both find it first.
func (r *agentRig) withJQ() {
	r.t.Helper()
	self, err := os.Executable()
	if err != nil {
		r.t.Fatal(err)
	}
	body := "#!/bin/sh\nexec env " + jqStandInEnv + "=1 " + shellQuote(self) + " \"$@\"\n"
	r.write(filepath.Join(r.bin, "jq"), body)
	r.write(filepath.Join(r.prefixBin, "jq"), body)
}

// run runs one adapter script as `base ensure` and `base status --probe` deliver it.
func (r *agentRig) run(script []byte, env ...string) (int, string) {
	r.t.Helper()
	cmd := exec.Command("sh", "-s")
	cmd.Stdin = bytes.NewReader(script)
	cmd.Env = append(os.Environ(),
		"PATH="+r.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-jobs", "STUB_HOME="+r.home, "TK_PROBE_SESSION=")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	var exited *exec.ExitError
	if err != nil && !errors.As(err, &exited) {
		r.t.Fatalf("the script did not start: %v", err)
	}
	return cmd.ProcessState.ExitCode(), string(out)
}

// agentScriptsRun skips where an adapter's scripts cannot run at all.
func agentScriptsRun(t *testing.T, tools ...string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the adapter's scripts run under a POSIX shell, which the Linux job has")
	}
	for _, tool := range append([]string{"sh", "awk", "readlink", "head", "tr", "sha256sum", "mktemp"}, tools...) {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not on PATH", tool)
		}
	}
}

// -- WSL-135: a declared model whose key is not stored yet is a note ------------------

// TestThePiProbeTellsADeclaredModelWithoutAKeyFromAnUndeclaredOne runs the pi probe
// against a stand-in for pi that answers as pi 1.4.0 was measured to. WSL-135.
//
// ⛔ pi LISTS ONLY THE MODELS OF A PROVIDER IT HOLDS A KEY FOR. With no key it prints
// `No models available`. So the guide's own order, the provider declared and its key
// still the operator's to store, read as an undeclared model, and the probe told the
// operator to declare it again. A declared model whose provider has no key is a note
// with `credential absent`, and an undeclared one stays a problem.
func TestThePiProbeTellsADeclaredModelWithoutAKeyFromAnUndeclaredOne(t *testing.T) {
	agentScriptsRun(t)
	probe, err := adapterScript("pi", "probe.sh")
	if err != nil {
		t.Fatal(err)
	}
	r := newAgentRig(t)
	r.withJQ()
	r.write(filepath.Join(r.system, "pi"), "#!/bin/sh\n# Written by wsl-toolkit's pi adapter. base ensure rewrites it.\n")
	// The stand-in for pi answers what its home says pi would: the listing a key gives,
	// and each provider's `pi auth check` verdict.
	r.write(filepath.Join(r.prefixBin, "pi"), `#!/bin/sh
case $1 in
  --version) echo 1.4.0 ;;
  --list-models)
    if [ -f "$HOME/.stub-listed" ]; then cat "$HOME/.stub-listed"; exit 0; fi
    echo 'No models available. Use /login to log into a provider via OAuth or API key.'
    ;;
  auth)
    [ "$2 $3" = "check --provider" ] || exit 64
    if [ -f "$HOME/.stub-auth-$4" ]; then cat "$HOME/.stub-auth-$4"; exit 1; fi
    echo invalid
    exit 2
    ;;
  *) exit 64 ;;
esac
`)
	agent := filepath.Join(r.home, ".pi", "agent")
	models := filepath.Join(agent, "models.json")
	const declared = `{"providers":{"muse-gateway":{"baseUrl":"https://gateway.invalid","apiKey":"$MUSE_GATEWAY_API_KEY",` +
		`"api":"anthropic-messages","models":[{"id":"muse-spark-1.3-contributor","thinkingLevelMap":{"xhigh":"xhigh","max":"max"}}]}}}`
	const header = "provider      model                       context  max-out  thinking  images\n"
	state := func(provider, model, effort, listed string, auth map[string]string) AdapterState {
		t.Helper()
		r.write(filepath.Join(agent, "settings.json"), fmt.Sprintf(`{"defaultProvider":%q,"defaultModel":%q,"defaultThinkingLevel":%q}`, provider, model, effort))
		r.write(models, declared)
		for _, stale := range []string{".stub-listed", ".stub-auth-muse-gateway", ".stub-auth-anthropic"} {
			_ = os.Remove(filepath.Join(r.home, stale))
		}
		if listed != "" {
			r.write(filepath.Join(r.home, ".stub-listed"), header+listed)
		}
		for p, verdict := range auth {
			r.write(filepath.Join(r.home, ".stub-auth-"+p), verdict+"\n")
		}
		code, out := r.run(probe)
		if code != 0 {
			t.Fatalf("probe.sh exited %d, and a probe exits 0 whatever it finds:\n%s", code, out)
		}
		return parseAdapterProbe("pi", out)
	}
	about := func(st AdapterState, word string) int {
		n := 0
		for _, p := range st.Problems {
			if strings.Contains(p, word) {
				n++
			}
		}
		return n
	}
	const undeclared = "its own catalogue does not list it"

	// ⭐ THE GUIDE'S OWN ORDER: declared in step 5, its key not stored yet.
	st := state("muse-gateway", "muse-spark-1.3-contributor", "max", "", map[string]string{"muse-gateway": "not_ready"})
	if !st.Healthy || st.Facts["credential"] != "absent" || len(st.Notes) != 1 ||
		!strings.Contains(st.Notes[0], "muse-gateway/muse-spark-1.3-contributor is declared in "+models) ||
		!strings.Contains(st.Notes[0], "pi auth check --provider muse-gateway answers ready") {
		t.Fatalf("a declared model whose key is not stored read as healthy=%v credential=%q notes=%q problems=%q",
			st.Healthy, st.Facts["credential"], st.Notes, st.Problems)
	}

	// ⛔ A MODEL THE DECLARED PROVIDER DOES NOT CARRY stays a problem, key or no key.
	st = state("muse-gateway", "muse-spark-9", "high", "", map[string]string{"muse-gateway": "not_ready"})
	if st.Healthy || about(st, undeclared) != 1 || st.Facts["credential"] != "absent" || len(st.Notes) != 0 {
		t.Fatalf("an undeclared model with no key read as credential=%q notes=%q problems=%q", st.Facts["credential"], st.Notes, st.Problems)
	}

	// ⭐ WITH THE KEY, pi LISTS IT, and there is nothing to say.
	st = state("muse-gateway", "muse-spark-1.3-contributor", "max",
		"muse-gateway  muse-spark-1.3-contributor  1.0M     128K     yes       yes\n", nil)
	if !st.Healthy || st.Facts["credential"] != "present" || len(st.Notes) != 0 {
		t.Fatalf("a listed model read as healthy=%v credential=%q notes=%q problems=%q", st.Healthy, st.Facts["credential"], st.Notes, st.Problems)
	}

	// ⛔ A KEY AND NO SUCH MODEL: pi lists the provider's others and not this one.
	st = state("muse-gateway", "muse-spark-9", "high",
		"muse-gateway  muse-spark-1.3-contributor  1.0M     128K     yes       yes\n", nil)
	if st.Healthy || about(st, undeclared) != 1 || st.Facts["credential"] != "present" || len(st.Notes) != 0 {
		t.Fatalf("an undeclared model of a keyed provider read as credential=%q notes=%q problems=%q", st.Facts["credential"], st.Notes, st.Problems)
	}

	// ⚠ A PROVIDER pi CARRIES ITSELF answers not_ready with no key, and models.json
	// does not name it. Its catalogue waits for the key, so it is a note.
	st = state("anthropic", "claude-opus-4-5", "high", "", map[string]string{"anthropic": "not_ready"})
	if !st.Healthy || st.Facts["credential"] != "absent" || len(st.Notes) != 1 || !strings.Contains(st.Notes[0], "a provider pi carries itself") {
		t.Fatalf("a built-in provider with no key read as healthy=%v notes=%q problems=%q", st.Healthy, st.Notes, st.Problems)
	}

	// ⛔ A PROVIDER NOBODY DECLARED, which pi answers invalid for, is a problem.
	st = state("nosuch", "x", "high", "", nil)
	if st.Healthy || about(st, "so it knows no such provider") != 1 || len(st.Notes) != 0 {
		t.Fatalf("a provider pi does not know read as notes=%q problems=%q", st.Notes, st.Problems)
	}
}

// TestTheMuseCodeGuideRunsInItsOwnOrder holds the three faults of the muse-code guide
// that the ordered pass of WSL-135 found in its prose, beside the probe's fourth.
//
// ⛔ EACH ONE WAS TRUE OF AN EARLIER TOOL, OR TRUE OUT OF ORDER. Step 3 promised every
// adapter healthy on a base whose pi provider step 5 declares. Step 4 revoked the grant
// step 8 works in. Step 9 named the exit 8.0.0 moved to ExitTool.
func TestTheMuseCodeGuideRunsInItsOwnOrder(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "examples", "muse-code", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	guide := strings.ReplaceAll(string(b), "\r\n", "\n")
	step := func(n int) string {
		t.Helper()
		head := fmt.Sprintf("\n## %d. ", n)
		i := strings.Index(guide, head)
		if i < 0 {
			t.Fatalf("the guide has no step %d", n)
		}
		rest := guide[i+len(head):]
		if j := strings.Index(rest, "\n## "); j >= 0 {
			rest = rest[:j]
		}
		return rest
	}
	if s := step(3); strings.Contains(s, "Every adapter must read `healthy`") || !strings.Contains(s, "pi reads one problem and `base ensure` exits 2") {
		t.Errorf("step 3 promises what a new base does not answer: pi reads one problem there until step 5\n%s", s)
	}
	for n := 1; n <= 9; n++ {
		if strings.Contains(step(n), "base revoke") {
			t.Errorf("step %d revokes a grant, and steps 8 and 9 work in it", n)
		}
	}
	if !strings.Contains(step(10), "wsl-toolkit base revoke --target") {
		t.Error("the guide no longer ends by taking its grant away")
	}
	if s := step(9); !strings.Contains(s, fmt.Sprintf("`muse` with no argument answers exit %d", ExitTool)) {
		t.Errorf("step 9 names an exit for a bare launcher that is not the tool's own, %d:\n%s", ExitTool, s)
	}
}

// -- WSL-138: each agent's own release, and npm only when it cannot be fetched ------

// releaseRig adds what an installer reaches: a curl that serves one file or fails, an
// npm that records what it was asked and links a package's launcher as npm does, and a
// pacman that records what it was asked.
type releaseRig struct {
	*agentRig
	stage, served, fetches, npmLog, pacmanLog, curlFails string
}

func newReleaseRig(t *testing.T, binName, versionPrefix string) *releaseRig {
	t.Helper()
	r := &releaseRig{agentRig: newAgentRig(t)}
	r.stage = filepath.Join(r.root, "stage")
	r.served = filepath.Join(r.root, "served")
	r.fetches = filepath.Join(r.root, "fetches")
	r.npmLog = filepath.Join(r.root, "npm.log")
	r.pacmanLog = filepath.Join(r.root, "pacman.log")
	r.curlFails = filepath.Join(r.root, "curl-fails")
	r.write(filepath.Join(r.bin, "curl"), `#!/bin/sh
out=
for a in "$@"; do
  case $a in https://*) printf '%s\n' "$a" >> `+shellQuote(r.fetches)+` ;; esac
done
if [ -f `+shellQuote(r.curlFails)+` ]; then
  echo 'curl: (6) Could not resolve host: github.com' >&2
  exit 6
fi
while [ $# -gt 0 ]; do if [ "$1" = --output ]; then out=$2; shift; fi; shift; done
cp `+shellQuote(r.served)+` "$out"
`)
	// ⚠ npm runs as the account, under env -i, so it carries every path it needs.
	npm := `#!/bin/sh
printf 'npm %s\n' "$*" >> ` + shellQuote(r.npmLog) + `
[ "$1 $2 $3" = "install -g --ignore-scripts" ] || exit 0
spec=$4
pkg=${spec%@*}
version=${spec##*@}
dir=$npm_config_prefix/lib/node_modules/$pkg
mkdir -p "$dir" "$npm_config_prefix/bin"
printf '#!/bin/sh\necho ` + versionPrefix + `%s\n' "$version" > "$dir/cli.js"
chmod 0755 "$dir/cli.js"
if [ -e "$npm_config_prefix/bin/` + binName + `" ] || [ -L "$npm_config_prefix/bin/` + binName + `" ]; then
  echo "npm ERR! EEXIST: file already exists $npm_config_prefix/bin/` + binName + `" >&2
  exit 1
fi
ln -s "$dir/cli.js" "$npm_config_prefix/bin/` + binName + `"
`
	r.write(filepath.Join(r.bin, "npm"), npm)
	r.write(filepath.Join(r.prefixBin, "npm"), npm)
	// pacman records what it was asked, and a bun it was asked for answers a version.
	r.write(filepath.Join(r.bin, "pacman"), "#!/bin/sh\nprintf 'pacman %s\\n' \"$*\" >> "+shellQuote(r.pacmanLog)+
		"\ncase \" $* \" in *' bun '*) printf '#!/bin/sh\\necho 1.4.2\\n' > "+shellQuote(filepath.Join(r.prefixBin, "bun"))+
		"; chmod 0755 "+shellQuote(filepath.Join(r.prefixBin, "bun"))+" ;; esac\nexit 0\n")
	return r
}

func (r *releaseRig) serve(body []byte) string {
	r.t.Helper()
	if err := os.WriteFile(r.served, body, 0o644); err != nil {
		r.t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(body))
}

func (r *releaseRig) read(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func (r *releaseRig) installerHasNotRunNpm(name, out string) {
	r.t.Helper()
	if log := r.read(r.npmLog); log != "" {
		r.t.Fatalf("%s ran npm, and npm is taken only when the release cannot be fetched:\n%s\n%s", name, log, out)
	}
}

// piReleaseAsset is a pi release laid out as pi's own: one directory, pi/, with the
// program in it.
func piReleaseAsset(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	program := "#!/bin/sh\necho " + version + "\n"
	for _, h := range []*tar.Header{
		{Name: "pi/", Mode: 0o755, Typeflag: tar.TypeDir},
		{Name: "pi/pi", Mode: 0o755, Size: int64(len(program)), Typeflag: tar.TypeReg},
	} {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(program)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestThePiAdapterTakesItsOwnReleaseAndNpmOnlyWhenTheReleaseCannotBeFetched runs the pi
// adapter's install.sh against stand-ins for GitHub, npm and the base's commands.
// WSL-138.
//
// ⛔ THE ROUTE RULING 4 SET FOR MUSE, applied to pi's own release: the asset is saved,
// its length and digest are printed, and the saved file is unpacked only while the
// digest is the pinned one. A changed digest is refused with exit 2 and how to approve
// it, and npm is NOT tried in its place. npm is taken only when the asset cannot be
// fetched at all.
func TestThePiAdapterTakesItsOwnReleaseAndNpmOnlyWhenTheReleaseCannotBeFetched(t *testing.T) {
	agentScriptsRun(t, "tar", "gzip", "cp", "ln")
	script, err := adapterScript("pi", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	asset := piReleaseAsset(t, "9.9.9")
	env := func(r *releaseRig, extra ...string) []string {
		return append([]string{"TK_PI_STAGE_DIR=" + r.stage, "TK_PI_WRAPPER=" + filepath.Join(r.system, "pi")}, extra...)
	}

	// ⛔ THE PINNED RELEASE, and bytes that are not it: refused, kept, never unpacked.
	for _, c := range []struct{ arch, asset, digest string }{
		{"x86_64", "pi-linux-x64.tar.gz", "f2e4e4329603223d03260cdb8bd5033dbdc1f73ba58ab8939db2e56546e115d2"},
		{"aarch64", "pi-linux-arm64.tar.gz", "fa5829c7ef83ffeaebba015715c8306220f89b2aa8a00154efbba98dd5440156"},
	} {
		r := newReleaseRig(t, "pi", "")
		served := r.serve(asset)
		code, out := r.run(script, env(r, "STUB_ARCH="+c.arch)...)
		if code != 2 || !strings.Contains(out, "is not the one this adapter pins, so it was not unpacked and npm was not tried") ||
			!strings.Contains(out, "pinned      pi 1.4.0, SHA-256 "+c.digest) ||
			!strings.Contains(out, fmt.Sprintf("%d bytes, SHA-256 %s", len(asset), served)) ||
			!strings.Contains(out, `add "version": "1.4.0", "sha256": {"`+c.arch+`": "`+served+`"} to the pi entry in base.adapters`) {
			t.Fatalf("a %s asset that is not pi 1.4.0's answered exit %d:\n%s", c.arch, code, out)
		}
		if got := strings.TrimSpace(r.read(r.fetches)); got != "https://github.com/earendil-works/pi/releases/download/v1.4.0/"+c.asset {
			t.Fatalf("the pinned release on %s was fetched from %q", c.arch, got)
		}
		r.installerHasNotRunNpm("a refused asset", out)
		if _, err := os.Stat(filepath.Join(r.stage, c.asset)); err != nil {
			t.Fatalf("a refused asset was not kept to be read: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(r.prefixBin, "pi")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("a refused asset left a pi for the account (%v):\n%s", err, out)
		}
	}

	// ⭐ A RELEASE WHOSE DIGEST IS IN FORCE is unpacked by the account and linked.
	r := newReleaseRig(t, "pi", "")
	digest := r.serve(asset)
	moved := env(r, "TK_ADAPTER_VERSION=9.9.9", "TK_ADAPTER_SHA256_X86_64="+digest, "TK_ADAPTER_SHA256_AARCH64="+digest)
	releases := filepath.Join(r.home, ".local", "share", "wsl-toolkit", "pi")
	r.write(filepath.Join(releases, "9.9.8", "pi", "pi"), "#!/bin/sh\necho 9.9.8\n")
	code, out := r.run(script, moved...)
	if code != 0 || !strings.HasSuffix(strings.TrimSpace(out), adapterCompleteLine("pi")) ||
		!strings.Contains(out, "installed pi 9.9.9 for herdr from its release asset, digest verified") {
		t.Fatalf("a release whose digest is in force answered exit %d:\n%s", code, out)
	}
	got, err := filepath.EvalSymlinks(filepath.Join(r.prefixBin, "pi"))
	want, _ := filepath.EvalSymlinks(filepath.Join(releases, "9.9.9", "pi", "pi"))
	if err != nil || want == "" || got != want {
		t.Fatalf("the account's pi resolves to %q (%v), and it must be the unpacked release %q", got, err, want)
	}
	r.installerHasNotRunNpm("a release that was fetched", out)
	if _, err := os.Stat(filepath.Join(releases, "9.9.8")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an earlier release nothing reads was left in place (%v)", err)
	}
	if _, err := os.Stat(filepath.Join(r.stage, "pi-linux-x64.tar.gz")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the saved asset stayed after it was unpacked (%v)", err)
	}

	// ⭐ A SECOND RUN FETCHES NOTHING.
	before := r.read(r.fetches)
	code, out = r.run(script, moved...)
	if code != 0 || r.read(r.fetches) != before || !strings.Contains(out, "from its release, unpacked from the asset this adapter pins") {
		t.Fatalf("a second run over the release answered exit %d, fetching %q after %q:\n%s", code, r.read(r.fetches), before, out)
	}

	// ⛔ npm ONLY WHEN THE RELEASE CANNOT BE FETCHED, at the same version.
	r = newReleaseRig(t, "pi", "")
	r.write(r.curlFails, "")
	code, out = r.run(script, env(r, "TK_ADAPTER_VERSION=9.9.9", "TK_ADAPTER_SHA256_X86_64="+digest, "TK_ADAPTER_SHA256_AARCH64="+digest)...)
	if code != 0 || !strings.Contains(out, "could not be fetched from https://github.com/earendil-works/pi/releases/download/v9.9.9/") ||
		!strings.Contains(r.read(r.npmLog), "npm install -g --ignore-scripts @earendil-works/pi-coding-agent@9.9.9") ||
		!strings.Contains(out, "installed pi 9.9.9 from npm") {
		t.Fatalf("a release that could not be fetched answered exit %d, npm %q:\n%s", code, r.read(r.npmLog), out)
	}
}

// TestTheOmpAdapterTakesItsOwnReleaseAndNpmOnlyWhenTheReleaseCannotBeFetched runs the
// omp adapter's install.sh the same way. WSL-138.
//
// ⭐ omp's RELEASE IS ONE BINARY WITH ITS OWN RUNTIME, so a base that takes it installs
// no Bun. Only the npm fallback is a Bun program.
func TestTheOmpAdapterTakesItsOwnReleaseAndNpmOnlyWhenTheReleaseCannotBeFetched(t *testing.T) {
	agentScriptsRun(t, "cp")
	script, err := adapterScript("omp", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	binary := []byte("#!/bin/sh\necho omp/9.9.9\n")
	env := func(r *releaseRig, extra ...string) []string {
		return append([]string{"TK_OMP_STAGE_DIR=" + r.stage, "TK_OMP_WRAPPER=" + filepath.Join(r.system, "omp")}, extra...)
	}

	for _, c := range []struct{ arch, asset, digest string }{
		{"x86_64", "omp-linux-x64", "85d9daf3a05fc76e8cd425ae1904eb576af8d21cc85fcf4690742626e07ccfa4"},
		{"aarch64", "omp-linux-arm64", "eee8e586274cef2e725c218b2b28b48ca9965d46dd58f60998cb347eca4ad0cf"},
	} {
		r := newReleaseRig(t, "omp", "omp/")
		served := r.serve(binary)
		code, out := r.run(script, env(r, "STUB_ARCH="+c.arch)...)
		if code != 2 || !strings.Contains(out, "is not the one this adapter pins, so it was not installed and npm was not tried") ||
			!strings.Contains(out, "pinned      omp 18.12.6, SHA-256 "+c.digest) ||
			!strings.Contains(out, `add "version": "18.12.6", "sha256": {"`+c.arch+`": "`+served+`"} to the omp entry in base.adapters`) {
			t.Fatalf("a %s binary that is not omp 18.12.6's answered exit %d:\n%s", c.arch, code, out)
		}
		if got := strings.TrimSpace(r.read(r.fetches)); got != "https://github.com/can1357/oh-my-pi/releases/download/v18.12.6/"+c.asset {
			t.Fatalf("the pinned release on %s was fetched from %q", c.arch, got)
		}
		r.installerHasNotRunNpm("a refused binary", out)
		if _, err := os.Lstat(filepath.Join(r.prefixBin, "omp")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("a refused binary left an omp for the account (%v):\n%s", err, out)
		}
	}

	r := newReleaseRig(t, "omp", "omp/")
	digest := r.serve(binary)
	moved := env(r, "TK_ADAPTER_VERSION=9.9.9", "TK_ADAPTER_SHA256_X86_64="+digest, "TK_ADAPTER_SHA256_AARCH64="+digest)
	code, out := r.run(script, moved...)
	if code != 0 || !strings.HasSuffix(strings.TrimSpace(out), adapterCompleteLine("omp")) ||
		!strings.Contains(out, "installed omp 9.9.9 for herdr from its release binary, digest verified") ||
		!strings.Contains(out, "carries its own runtime, so no Bun is installed for it") {
		t.Fatalf("a release whose digest is in force answered exit %d:\n%s", code, out)
	}
	if got := r.read(filepath.Join(r.prefixBin, "omp")); got != string(binary) {
		t.Fatalf("the account's omp holds %q, and it must be the release binary", got)
	}
	r.installerHasNotRunNpm("a release that was fetched", out)
	if strings.Contains(r.read(r.pacmanLog), "bun") {
		t.Fatalf("a base that took omp's release installed Bun: %s", r.read(r.pacmanLog))
	}

	before := r.read(r.fetches)
	code, out = r.run(script, moved...)
	if code != 0 || r.read(r.fetches) != before || !strings.Contains(out, "from its release, whose digest this adapter pins") {
		t.Fatalf("a second run over the release answered exit %d, fetching %q after %q:\n%s", code, r.read(r.fetches), before, out)
	}

	r = newReleaseRig(t, "omp", "omp/")
	r.write(r.curlFails, "")
	code, out = r.run(script, env(r, "TK_ADAPTER_VERSION=9.9.9", "TK_ADAPTER_SHA256_X86_64="+digest, "TK_ADAPTER_SHA256_AARCH64="+digest)...)
	if code != 0 || !strings.Contains(out, "could not be fetched from https://github.com/can1357/oh-my-pi/releases/download/v9.9.9/") ||
		!strings.Contains(r.read(r.npmLog), "npm install -g --ignore-scripts @oh-my-pi/pi-coding-agent@9.9.9") ||
		!strings.Contains(r.read(r.pacmanLog), "bun") || !strings.Contains(out, "installed omp 9.9.9 from npm") {
		t.Fatalf("a release that could not be fetched answered exit %d, npm %q, pacman %q:\n%s", code, r.read(r.npmLog), r.read(r.pacmanLog), out)
	}
}

// TestTheOmpProbeAsksForBunOnlyWhereOmpIsABunProgram holds the probe to the two
// routes. WSL-138.
//
// ⛔ A RELEASE BINARY IS ONE ELF FILE WITH ITS OWN RUNTIME. A probe that asked every omp
// for Bun would fail every base that took the official route, because that route
// installs none. npm's omp is still a Bun program, and with no Bun it cannot start.
func TestTheOmpProbeAsksForBunOnlyWhereOmpIsABunProgram(t *testing.T) {
	agentScriptsRun(t, "od")
	for _, dir := range []string{"/usr/local/sbin", "/usr/local/bin", "/usr/bin", "/bin"} {
		if _, err := os.Stat(filepath.Join(dir, "bun")); err == nil {
			t.Skipf("%s has a bun, so the account always finds one and this case cannot fail", dir)
		}
	}
	probe, err := adapterScript("omp", "probe.sh")
	if err != nil {
		t.Fatal(err)
	}
	r := newAgentRig(t)
	r.write(filepath.Join(r.system, "omp"), "#!/bin/sh\n# Written by wsl-toolkit's omp adapter. base ensure rewrites it.\n")
	launcher := filepath.Join(r.prefixBin, "omp")
	read := func() AdapterState {
		t.Helper()
		code, out := r.run(probe)
		if code != 0 {
			t.Fatalf("probe.sh exited %d, and a probe exits 0 whatever it finds:\n%s", code, out)
		}
		return parseAdapterProbe("omp", out)
	}
	bun := func(st AdapterState) int {
		n := 0
		for _, p := range st.Problems {
			if strings.Contains(p, "Bun program") {
				n++
			}
		}
		return n
	}

	// ⭐ THE RELEASE: an ELF file, and no Bun anywhere the account looks.
	r.write(launcher, "\x7fELF\x02\x01\x01 not a program this case runs\n")
	st := read()
	if st.Facts["source"] != "release" || bun(st) != 0 || len(st.Notes) != 0 {
		t.Fatalf("a release binary read as source=%q, notes %q, problems %q", st.Facts["source"], st.Notes, st.Problems)
	}

	// ⛔ npm's omp, a Bun program, with no Bun: the problem, and where it came from.
	pkg := filepath.Join(r.home, ".local", "lib", "node_modules", "@oh-my-pi", "pi-coding-agent", "dist", "cli.js")
	r.write(pkg, "#!/usr/bin/env bun\nconsole.log('omp/9.9.9')\n")
	if err := os.Remove(launcher); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(pkg, launcher); err != nil {
		t.Fatal(err)
	}
	st = read()
	if st.Facts["source"] != "npm" || bun(st) != 1 || len(st.Notes) != 1 || !strings.Contains(st.Notes[0], "came from npm") {
		t.Fatalf("npm's omp with no Bun read as source=%q, notes %q, problems %q", st.Facts["source"], st.Notes, st.Problems)
	}
}
