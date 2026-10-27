// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
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
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// -- a jq for the cases, because the Go image has none ------------------------------

// jqStandInEnv makes this test binary answer as jq, for the few programs the pi
// probe runs.
//
// ⛔ golang:1.25, where the Linux proof runs every case, carries no jq, and the pi
// probe reads its startup model only through jq. A case that skipped there would leave
// its rows proved nowhere. One that used a host's jq where it has one would answer
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

// agentScriptBudget is how long one adapter script may take in the rig. Each takes
// well under a second. ⛔ A script that blocks, as on a FIFO, fails its case rather
// than holding the suite.
const agentScriptBudget = 30 * time.Second

// rigAccountTools are the commands an adapter script runs as the account, and the
// stand-ins run. The rig links each into the account's PATH, and nothing else.
var rigAccountTools = []string{
	"sh", "env", "cat", "readlink", "rm", "mkdir", "mktemp", "tar", "gzip", "mv", "ln", "cp",
	"chmod", "sha256sum", "head", "tr", "od", "cut", "awk", "sed", "grep", "id", "ls",
	"dirname", "basename", "wc", "du", "touch", "sleep", "printf", "echo", "true", "false",
	"test", "[",
}

// agentRig is one account's home and the stand-ins an adapter script reaches, all
// under one temporary directory.
type agentRig struct {
	t                                                      *testing.T
	root, home, bin, prefixBin, system, accountPath, stage string
}

func newAgentRig(t *testing.T) *agentRig {
	t.Helper()
	root := t.TempDir()
	r := &agentRig{
		t: t, root: root, home: filepath.Join(root, "home"), bin: filepath.Join(root, "bin"),
		system: filepath.Join(root, "usr-local-bin"), accountPath: filepath.Join(root, "account-path"),
		stage: filepath.Join(root, "stage"),
	}
	r.prefixBin = filepath.Join(r.home, ".local", "bin")
	for _, dir := range []string{r.bin, r.prefixBin, r.system, r.accountPath} {
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
	// ⛔ THE ACCOUNT'S PATH HOLDS THESE AND THE ACCOUNT'S OWN PREFIX, NOTHING ELSE. The
	// scripts name /usr/bin and /bin, and a host's own bun there answered for omp. So the
	// case read the host rather than the script. WSL-138.
	for _, tool := range rigAccountTools {
		if p, err := exec.LookPath(tool); err == nil {
			if err := os.Symlink(p, filepath.Join(r.accountPath, tool)); err != nil {
				t.Fatal(err)
			}
		}
	}
	r.write(filepath.Join(r.bin, "getent"), "#!/bin/sh\nprintf 'herdr:x:1000:1000::%s:/bin/bash\\n' "+shellQuote(r.home)+"\n")
	r.write(filepath.Join(r.bin, "id"), "#!/bin/sh\nif [ \"$1\" = -gn ]; then exec "+shellQuote(real["id"])+" -gn; fi\nexec "+shellQuote(real["id"])+" \"$@\"\n")
	r.write(filepath.Join(r.bin, "uname"), "#!/bin/sh\nif [ \"$1\" = -m ]; then echo \"${STUB_ARCH:-x86_64}\"; exit 0; fi\nexec "+shellQuote(real["uname"])+" \"$@\"\n")
	// ⚠ root's chown names the account, and the host has no account herdr.
	r.write(filepath.Join(r.bin, "chown"), "#!/bin/sh\nexit 0\n")
	// ⭐ runuser -u runs the command as this process, on the rig's account PATH. runuser -l
	// answers what a login shell would resolve, which is the wrapper in this rig's system
	// directory.
	r.write(filepath.Join(r.bin, "runuser"), `#!/bin/sh
case $1 in
  -u)
    [ "$2" = herdr ] && [ "$3" = -- ] || exit 9
    shift 3
    swapped=
    for a do
      shift
      case $swapped$a in
        PATH=*) set -- "$@" `+shellQuote("PATH="+r.prefixBin+":"+r.accountPath)+`; swapped=yes ;;
        *) set -- "$@" "$a" ;;
      esac
    done
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

func (r *agentRig) link(target, path string) {
	r.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		r.t.Fatal(err)
	}
}

// fifo makes a named pipe at path. ⚠ The cleanup opens it for writing without waiting,
// which lets go a reader a broken script left blocked on it.
func (r *agentRig) fifo(path string) {
	r.t.Helper()
	if out, err := exec.Command("mkfifo", path).CombinedOutput(); err != nil {
		r.t.Skipf("mkfifo could not make a named pipe: %v %s", err, out)
	}
	r.t.Cleanup(func() {
		if f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = f.Close()
		}
	})
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
// Every path the scripts write outside the account's home is under the rig.
func (r *agentRig) run(script []byte, env ...string) (int, string) {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), agentScriptBudget)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-s")
	cmd.WaitDelay = time.Second
	cmd.Stdin = bytes.NewReader(script)
	cmd.Env = append(os.Environ(),
		"PATH="+r.bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-jobs", "STUB_HOME="+r.home, "TK_PROBE_SESSION=",
		"TK_PI_STAGE_DIR="+r.stage, "TK_PI_WRAPPER="+filepath.Join(r.system, "pi"),
		"TK_OMP_STAGE_DIR="+r.stage, "TK_OMP_WRAPPER="+filepath.Join(r.system, "omp"),
		"TK_OMP_MUSL_MARK="+filepath.Join(r.root, "alpine-release"))
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		r.t.Fatalf("the script was still running after %s, so something in it blocked:\n%s", agentScriptBudget, out)
	}
	var exited *exec.ExitError
	if err != nil && !errors.As(err, &exited) {
		r.t.Fatalf("the script did not start: %v", err)
	}
	return cmd.ProcessState.ExitCode(), string(out)
}

// scriptPin answers what an adapter script assigns NAME at the start of a line. It reads
// pins as the herdr pin case does, so a pin has one home: the script. ⛔ A digest it
// answers is a SHA-256, or the case stops.
func scriptPin(t *testing.T, script []byte, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^` + name + `=(\S+)$`).FindSubmatch(script)
	if m == nil {
		t.Fatalf("install.sh assigns no %s at the start of a line", name)
	}
	if strings.Contains(name, "SHA256") && !installerDigestRE.Match(m[1]) {
		t.Fatalf("install.sh pins %s as %q, which is not a SHA-256", name, m[1])
	}
	return string(m[1])
}

// agentScriptsRun skips where an adapter's scripts cannot run at all.
//
// ⚠ NOT ON WINDOWS, and the Linux job proves these rows. The scripts run in a Linux base.
// They read symbolic links back with readlink -f, which this host cannot make. The rig
// hands them paths a Windows drive letter breaks on a colon-separated PATH.
func agentScriptsRun(t *testing.T, tools ...string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the adapter's scripts run in a Linux base, on symbolic links and POSIX paths, so the Linux job runs this case")
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
// `No models available`. So the guide's own order read as an undeclared model: the
// provider was declared, and its key was still the operator's to store. The probe told
// the operator to declare it again. A declared model whose provider has no key is a
// note with `credential absent`. An undeclared one stays a problem.
func TestThePiProbeTellsADeclaredModelWithoutAKeyFromAnUndeclaredOne(t *testing.T) {
	agentScriptsRun(t)
	probe, err := adapterScript("pi", "probe.sh")
	if err != nil {
		t.Fatal(err)
	}
	r := newAgentRig(t)
	r.withJQ()
	r.write(filepath.Join(r.system, "pi"), "#!/bin/sh\n# Written by wsl-toolkit's pi adapter. base ensure rewrites it.\n")
	// The stand-in for pi answers what its home says pi would. That is the listing a key
	// gives, and the `pi auth check --model` verdict for each provider's models.
	r.write(filepath.Join(r.prefixBin, "pi"), `#!/bin/sh
case $1 in
  --version) echo 1.4.0 ;;
  --list-models)
    if [ -f "$HOME/.stub-listed" ]; then cat "$HOME/.stub-listed"; exit 0; fi
    echo 'No models available. Use /login to log into a provider via OAuth or API key.'
    ;;
  auth)
    # ⛔ --model, as pi 1.4.0 was measured: --provider answers not_ready for any name.
    [ "$2 $3" = "check --model" ] || exit 64
    provider=${4%%/*}
    if [ -f "$HOME/.stub-auth-$provider" ]; then cat "$HOME/.stub-auth-$provider"; exit 1; fi
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
	withModels := true
	state := func(provider, model, effort, listed string, auth map[string]string) AdapterState {
		t.Helper()
		r.write(filepath.Join(agent, "settings.json"), fmt.Sprintf(`{"defaultProvider":%q,"defaultModel":%q,"defaultThinkingLevel":%q}`, provider, model, effort))
		_ = os.Remove(models)
		if withModels {
			r.write(models, declared)
		}
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

	// ⭐ THE GUIDE'S OWN ORDER: declared in step 5, its key not stored yet. The note names
	// the check the probe trusts, by model.
	st := state("muse-gateway", "muse-spark-1.3-contributor", "max", "", map[string]string{"muse-gateway": "not_ready"})
	if !st.Healthy || st.Facts["credential"] != "absent" || len(st.Notes) != 1 ||
		!strings.Contains(st.Notes[0], "muse-gateway/muse-spark-1.3-contributor is declared in "+models) ||
		!strings.Contains(st.Notes[0], "pi auth check --model muse-gateway/muse-spark-1.3-contributor answers ready") {
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
	if !st.Healthy || st.Facts["credential"] != "absent" || len(st.Notes) != 1 || !strings.Contains(st.Notes[0], "a provider pi carries itself") ||
		!strings.Contains(st.Notes[0], "pi auth check --model anthropic/claude-opus-4-5 answers ready") {
		t.Fatalf("a built-in provider with no key read as healthy=%v notes=%q problems=%q", st.Healthy, st.Notes, st.Problems)
	}

	// ⛔ A PROVIDER A models.json THAT EXISTS DOES NOT DECLARE, which pi answers invalid
	// for, is a problem.
	st = state("nosuch", "x", "high", "", nil)
	if st.Healthy || about(st, "so it knows no such provider") != 1 || len(st.Notes) != 0 {
		t.Fatalf("a provider pi does not know read as notes=%q problems=%q", st.Notes, st.Problems)
	}

	// ⭐ AND THE GUIDE'S STEP 3: a new base, the profile's model, no models.json yet. Nothing
	// is declared, so it is a note naming the file and step 5, and base ensure exits 0.
	withModels = false
	st = state("muse-gateway", "muse-spark-1.3-contributor", "max", "", nil)
	if !st.Healthy || st.Facts["credential"] != "absent" || len(st.Notes) != 1 ||
		!strings.Contains(st.Notes[0], models+" does not exist yet") || !strings.Contains(st.Notes[0], "step 5 of the muse-code guide") {
		t.Fatalf("a new base's pi, whose provider is not declared yet, read as healthy=%v notes=%q problems=%q", st.Healthy, st.Notes, st.Problems)
	}
}

// TestTheMuseCodeGuideRunsInItsOwnOrder holds the faults of the muse-code guide that the
// ordered pass of WSL-135 found in its prose, beside the probe's.
//
// ⛔ EACH ONE WAS TRUE OF AN EARLIER TOOL, OR TRUE OUT OF ORDER. Step 3 promised every
// adapter healthy and pi read a problem there. Step 4 revoked the grant step 8 works in,
// and Muse was never told to trust it. Step 9 named the exit 8.0.0 moved to ExitTool.
// Step 10 needed a variable step 8 set.
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
	// ⭐ A NEW BASE'S pi READS A NOTE, NOT A PROBLEM, so step 3's ensure exits 0.
	if s := step(3); !strings.Contains(s, "Every adapter must read `healthy`") || strings.Contains(s, "exits 2") ||
		!strings.Contains(s, "pi reads `healthy` with one note") {
		t.Errorf("step 3 does not say what a new base answers: every adapter healthy, pi with a note until step 5\n%s", s)
	}
	// ⛔ THE CHECK STEP 5 NAMES IS THE ONE THE PROBE TRUSTS. pi answers --provider with
	// not_ready for any name.
	if s := step(5); !strings.Contains(s, "pi auth check --model muse-gateway/muse-spark-1.3-contributor") || strings.Contains(s, "pi auth check --provider") {
		t.Errorf("step 5 checks the key with a form the probe does not trust:\n%s", s)
	}
	// ⛔ MUSE BLOCKS ON A DIRECTORY IT WAS NOT TOLD TO TRUST, and base ensure tells it about
	// each grant. Driven on a scratch base: with the grant made after step 3's ensure, step
	// 8's `agent start muse` answered agent_not_ready and exited 1.
	if s := step(4); strings.Index(s, "wsl-toolkit base ensure") < strings.Index(s, "wsl-toolkit base grant --source") {
		t.Errorf("step 4 grants a directory and runs no base ensure after it, so Muse blocks on it in step 8:\n%s", s)
	}
	for n := 1; n <= 9; n++ {
		if strings.Contains(step(n), "base revoke") {
			t.Errorf("step %d revokes a grant, and steps 8 and 9 work in it", n)
		}
	}
	// ⛔ AND THE PANE STEP 8 LEFT IN THE GRANT IS CLOSED FIRST. Driven on a scratch base:
	// with the workspace open, revoke answered `target is busy` and exited 1.
	s := step(10)
	if !strings.Contains(s, "wsl-toolkit base revoke --target") ||
		strings.Index(s, "workspace close") < 0 || strings.Index(s, "workspace close") > strings.Index(s, "base revoke --target") {
		t.Errorf("step 10 does not close step 8's workspace before it revokes the grant that workspace holds busy:\n%s", s)
	}
	// ⚠ `$made` LIVES IN STEP 8'S SESSION. A reader in a new one reads the id again.
	if !strings.Contains(s, "wsl-toolkit base herdr -- workspace list") {
		t.Errorf("step 10 closes the workspace by a variable step 8's session set, and says nothing for a new session:\n%s", s)
	}
	if s := step(9); !strings.Contains(s, fmt.Sprintf("`muse` with no argument answers exit %d", ExitTool)) {
		t.Errorf("step 9 names an exit for a bare launcher that is not the tool's own, %d:\n%s", ExitTool, s)
	}
}

// -- WSL-138: each agent's own release, and npm only when it cannot be fetched ------

// releaseRig adds what an installer reaches. That is a curl that serves one file, fails,
// or is stopped by a signal. It is an npm that records what it was asked, and links a
// package's launcher as npm does or fails. It is a pacman that records what it was asked.
type releaseRig struct {
	*agentRig
	served, fetches, npmLog, pacmanLog, curlFails, curlSignals, npmFails string
}

func newReleaseRig(t *testing.T, binName, versionPrefix string) *releaseRig {
	t.Helper()
	r := &releaseRig{agentRig: newAgentRig(t)}
	r.served = filepath.Join(r.root, "served")
	r.fetches = filepath.Join(r.root, "fetches")
	r.npmLog = filepath.Join(r.root, "npm.log")
	r.pacmanLog = filepath.Join(r.root, "pacman.log")
	r.curlFails = filepath.Join(r.root, "curl-fails")
	r.curlSignals = filepath.Join(r.root, "curl-signals")
	r.npmFails = filepath.Join(r.root, "npm-fails")
	// ⚠ A SIGNALLED FETCH stops the script itself, the `sh -s` above this curl, with TERM.
	r.write(filepath.Join(r.bin, "curl"), `#!/bin/sh
out=
for a in "$@"; do
  case $a in https://*) printf '%s\n' "$a" >> `+shellQuote(r.fetches)+` ;; esac
done
if [ -f `+shellQuote(r.curlSignals)+` ]; then
  pid=$PPID
  while [ "${pid:-1}" -gt 1 ]; do
    if [ "$(tr '\0' ' ' < /proc/$pid/cmdline 2>/dev/null)" = "sh -s " ]; then
      kill -TERM "$pid"
      break
    fi
    pid=$(awk '{ print $4 }' /proc/$pid/stat 2>/dev/null)
  done
  exit 1
fi
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
if [ -f ` + shellQuote(r.npmFails) + ` ]; then
  echo 'npm ERR! code E404' >&2
  exit 1
fi
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

// signalled holds an installer whose fetch a signal stopped. ⛔ It exits with the
// signal's code. It leaves no partial file, and it does not take npm, because nothing
// failed to be fetched.
func (r *releaseRig) signalled(script []byte, asset string, env ...string) {
	r.t.Helper()
	r.write(r.curlSignals, "")
	code, out := r.run(script, env...)
	if code != 143 {
		r.t.Fatalf("a fetch a TERM stopped answered exit %d, and a stopped run answers 143:\n%s", code, out)
	}
	r.installerHasNotRunNpm("a fetch a TERM stopped", out)
	if left, _ := filepath.Glob(filepath.Join(r.stage, asset+".*")); len(left) != 0 {
		r.t.Fatalf("a fetch a TERM stopped left %q", left)
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
// ⛔ THE ROUTE RULING 4 SET FOR MUSE, applied to pi's own release. The asset is saved,
// and its length and digest are printed. The saved file is unpacked only while the
// digest is the pinned one. A changed digest is refused with exit 2, and npm is NOT tried
// in its place. npm is taken only when the asset cannot be fetched at all.
func TestThePiAdapterTakesItsOwnReleaseAndNpmOnlyWhenTheReleaseCannotBeFetched(t *testing.T) {
	agentScriptsRun(t, "tar", "gzip", "cp", "ln")
	script, err := adapterScript("pi", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	asset := piReleaseAsset(t, "9.9.9")

	// ⛔ THE PINNED RELEASE, and bytes that are not it: refused, kept, never unpacked.
	release := scriptPin(t, script, "PI_RELEASE")
	for _, c := range []struct{ arch, asset, digest string }{
		{"x86_64", "pi-linux-x64.tar.gz", scriptPin(t, script, "PI_X86_64_PINNED_SHA256")},
		{"aarch64", "pi-linux-arm64.tar.gz", scriptPin(t, script, "PI_AARCH64_PINNED_SHA256")},
	} {
		r := newReleaseRig(t, "pi", "")
		served := r.serve(asset)
		code, out := r.run(script, "STUB_ARCH="+c.arch)
		if code != 2 || !strings.Contains(out, "is not the one this adapter pins, so it was not unpacked and npm was not tried") ||
			!strings.Contains(out, "pinned      pi "+release+", SHA-256 "+c.digest) ||
			!strings.Contains(out, fmt.Sprintf("%d bytes, SHA-256 %s", len(asset), served)) ||
			!strings.Contains(out, `add "version": "`+release+`", "sha256": {"`+c.arch+`": "`+served+`"} to the pi entry in base.adapters`) {
			t.Fatalf("a %s asset that is not pi %s's answered exit %d:\n%s", c.arch, release, code, out)
		}
		if got := strings.TrimSpace(r.read(r.fetches)); got != "https://github.com/earendil-works/pi/releases/download/v"+release+"/"+c.asset {
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

	// ⭐ A RELEASE WHOSE DIGEST IS IN FORCE is unpacked by the account and linked. The base
	// held an earlier release, and npm's pi from before WSL-138.
	r := newReleaseRig(t, "pi", "")
	digest := r.serve(asset)
	moved := []string{"TK_ADAPTER_VERSION=9.9.9", "TK_ADAPTER_SHA256_X86_64=" + digest, "TK_ADAPTER_SHA256_AARCH64=" + digest}
	releases := filepath.Join(r.home, ".local", "share", "wsl-toolkit", "pi")
	r.write(filepath.Join(releases, "9.9.8", "pi", "pi"), "#!/bin/sh\necho 9.9.8\n")
	npmPi := filepath.Join(r.home, ".local", "lib", "node_modules", "@earendil-works", "pi-coding-agent")
	r.write(filepath.Join(npmPi, "dist", "cli.js"), "#!/bin/sh\necho 9.9.7\n")
	r.link(filepath.Join(npmPi, "dist", "cli.js"), filepath.Join(r.prefixBin, "pi"))
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
	// ⚠ npm's PACKAGE GOES ONCE THE RELEASE IS IN USE. Nothing reads it, and the leftover
	// probe no longer finds it through the launcher.
	if _, err := os.Stat(npmPi); !errors.Is(err, os.ErrNotExist) || !strings.Contains(out, "removed "+npmPi+", npm's pi") {
		t.Fatalf("npm's pi stayed beside the release in use (%v):\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Dir(npmPi)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("npm's empty scope for pi stayed (%v)", err)
	}

	// ⭐ A SECOND RUN FETCHES NOTHING, and takes away an asset an earlier refusal kept.
	kept := filepath.Join(r.stage, "pi-linux-x64.tar.gz")
	r.write(kept, "an asset a refusal kept\n")
	before := r.read(r.fetches)
	code, out = r.run(script, moved...)
	if code != 0 || r.read(r.fetches) != before || !strings.Contains(out, "from its release, unpacked from the asset this adapter pins") {
		t.Fatalf("a second run over the release answered exit %d, fetching %q after %q:\n%s", code, r.read(r.fetches), before, out)
	}
	if _, err := os.Stat(kept); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an asset a refusal kept stayed beside the pinned release in use (%v)", err)
	}

	// ⛔ npm ONLY WHEN THE RELEASE CANNOT BE FETCHED, at the same version.
	r = newReleaseRig(t, "pi", "")
	r.write(r.curlFails, "")
	code, out = r.run(script, moved...)
	if code != 0 || !strings.Contains(out, "could not be fetched from https://github.com/earendil-works/pi/releases/download/v9.9.9/") ||
		!strings.Contains(r.read(r.npmLog), "npm install -g --ignore-scripts @earendil-works/pi-coding-agent@9.9.9") ||
		!strings.Contains(out, "installed pi 9.9.9 from npm") {
		t.Fatalf("a release that could not be fetched answered exit %d, npm %q:\n%s", code, r.read(r.npmLog), out)
	}

	// ⛔ AND A FAILED npm LEAVES THE pi THAT WAS THERE. The release's link went for npm, and
	// it comes back.
	r = newReleaseRig(t, "pi", "")
	r.write(r.curlFails, "")
	r.write(r.npmFails, "")
	earlier := filepath.Join(r.home, ".local", "share", "wsl-toolkit", "pi", "9.9.8", "pi", "pi")
	r.write(earlier, "#!/bin/sh\necho 9.9.8\n")
	r.link(earlier, filepath.Join(r.prefixBin, "pi"))
	code, out = r.run(script, moved...)
	if code != 3 || !strings.Contains(out, "npm could not install @earendil-works/pi-coding-agent@9.9.9") {
		t.Fatalf("an npm that failed answered exit %d:\n%s", code, out)
	}
	if back, err := os.Readlink(filepath.Join(r.prefixBin, "pi")); err != nil || back != earlier {
		t.Fatalf("a failed npm left the account's pi at %q (%v), and it was %q", back, err, earlier)
	}

	// ⛔ A SIGNAL DURING THE FETCH IS NOT A FETCH FAILURE.
	newReleaseRig(t, "pi", "").signalled(script, "pi-linux-x64.tar.gz", moved...)
}

// TestTheOmpAdapterTakesItsOwnReleaseAndNpmOnlyWhenTheReleaseCannotBeFetched runs the
// omp adapter's install.sh the same way. WSL-138.
//
// ⭐ omp's RELEASE IS ONE BINARY WITH ITS OWN RUNTIME, so a base that takes it installs
// no Bun. Only the npm fallback is a Bun program.
func TestTheOmpAdapterTakesItsOwnReleaseAndNpmOnlyWhenTheReleaseCannotBeFetched(t *testing.T) {
	agentScriptsRun(t, "cp", "mkfifo")
	script, err := adapterScript("omp", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	binary := []byte("#!/bin/sh\necho omp/9.9.9\n")

	release := scriptPin(t, script, "OMP_RELEASE")
	for _, c := range []struct{ arch, asset, digest string }{
		{"x86_64", "omp-linux-x64", scriptPin(t, script, "OMP_X86_64_PINNED_SHA256")},
		{"aarch64", "omp-linux-arm64", scriptPin(t, script, "OMP_AARCH64_PINNED_SHA256")},
	} {
		r := newReleaseRig(t, "omp", "omp/")
		served := r.serve(binary)
		code, out := r.run(script, "STUB_ARCH="+c.arch)
		if code != 2 || !strings.Contains(out, "is not the one this adapter pins, so it was not installed and npm was not tried") ||
			!strings.Contains(out, "pinned      omp "+release+", SHA-256 "+c.digest) ||
			!strings.Contains(out, `add "version": "`+release+`", "sha256": {"`+c.arch+`": "`+served+`"} to the omp entry in base.adapters`) {
			t.Fatalf("a %s binary that is not omp %s's answered exit %d:\n%s", c.arch, release, code, out)
		}
		if got := strings.TrimSpace(r.read(r.fetches)); got != "https://github.com/can1357/oh-my-pi/releases/download/v"+release+"/"+c.asset {
			t.Fatalf("the pinned release on %s was fetched from %q", c.arch, got)
		}
		r.installerHasNotRunNpm("a refused binary", out)
		if _, err := os.Lstat(filepath.Join(r.prefixBin, "omp")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("a refused binary left an omp for the account (%v):\n%s", err, out)
		}
	}

	// ⭐ A RELEASE WHOSE DIGEST IS IN FORCE replaces npm's omp from before WSL-138.
	r := newReleaseRig(t, "omp", "omp/")
	digest := r.serve(binary)
	moved := []string{"TK_ADAPTER_VERSION=9.9.9", "TK_ADAPTER_SHA256_X86_64=" + digest, "TK_ADAPTER_SHA256_AARCH64=" + digest}
	npmOmp := filepath.Join(r.home, ".local", "lib", "node_modules", "@oh-my-pi", "pi-coding-agent")
	r.write(filepath.Join(npmOmp, "dist", "cli.js"), "#!/bin/sh\necho omp/9.9.7\n")
	r.link(filepath.Join(npmOmp, "dist", "cli.js"), filepath.Join(r.prefixBin, "omp"))
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
	if _, err := os.Stat(npmOmp); !errors.Is(err, os.ErrNotExist) || !strings.Contains(out, "removed "+npmOmp+", npm's omp") {
		t.Fatalf("npm's omp stayed beside the release in use (%v):\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Dir(npmOmp)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("npm's empty scope for omp stayed (%v)", err)
	}

	kept := filepath.Join(r.stage, "omp-linux-x64")
	r.write(kept, "a binary a refusal kept\n")
	before := r.read(r.fetches)
	code, out = r.run(script, moved...)
	if code != 0 || r.read(r.fetches) != before || !strings.Contains(out, "from its release, whose digest this adapter pins") {
		t.Fatalf("a second run over the release answered exit %d, fetching %q after %q:\n%s", code, r.read(r.fetches), before, out)
	}
	if _, err := os.Stat(kept); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a binary a refusal kept stayed beside the pinned release in use (%v)", err)
	}

	// ⛔ npm ONLY WHEN THE RELEASE CANNOT BE FETCHED, and that omp is a Bun program.
	r = newReleaseRig(t, "omp", "omp/")
	r.write(r.curlFails, "")
	code, out = r.run(script, moved...)
	if code != 0 || !strings.Contains(out, "could not be fetched from https://github.com/can1357/oh-my-pi/releases/download/v9.9.9/") ||
		!strings.Contains(r.read(r.npmLog), "npm install -g --ignore-scripts @oh-my-pi/pi-coding-agent@9.9.9") ||
		!strings.Contains(r.read(r.pacmanLog), "bun") || !strings.Contains(out, "installed omp 9.9.9 from npm") {
		t.Fatalf("a release that could not be fetched answered exit %d, npm %q, pacman %q:\n%s", code, r.read(r.npmLog), r.read(r.pacmanLog), out)
	}

	// ⛔ AND A FAILED npm LEAVES THE omp THAT WAS THERE, moved aside and back.
	r = newReleaseRig(t, "omp", "omp/")
	r.write(r.curlFails, "")
	r.write(r.npmFails, "")
	r.write(filepath.Join(r.prefixBin, "omp"), "#!/bin/sh\necho omp/9.9.8\n")
	code, out = r.run(script, moved...)
	if code != 3 || !strings.Contains(out, "npm could not install @oh-my-pi/pi-coding-agent@9.9.9") {
		t.Fatalf("an npm that failed answered exit %d:\n%s", code, out)
	}
	if got := r.read(filepath.Join(r.prefixBin, "omp")); got != "#!/bin/sh\necho omp/9.9.8\n" {
		t.Fatalf("a failed npm left the account's omp holding %q", got)
	}
	if _, err := os.Lstat(filepath.Join(r.prefixBin, ".omp.before-npm")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a failed npm left the moved omp aside (%v)", err)
	}

	// ⚠ A musl BASE HAS NO PINNED BUILD, so it fetches nothing and takes npm.
	r = newReleaseRig(t, "omp", "omp/")
	r.write(filepath.Join(r.root, "alpine-release"), "3.22.0\n")
	code, out = r.run(script, moved...)
	if code != 0 || r.read(r.fetches) != "" || !strings.Contains(out, "carry no build this adapter pins for this machine") ||
		!strings.Contains(r.read(r.npmLog), "@oh-my-pi/pi-coding-agent@9.9.9") {
		t.Fatalf("a musl base answered exit %d, fetching %q, npm %q:\n%s", code, r.read(r.fetches), r.read(r.npmLog), out)
	}

	// ⭐ A BASE THAT SEPARATED omp's DIRECTORY keeps its wrapper. The release goes beside it,
	// and a second run reads the release there and fetches nothing.
	r = newReleaseRig(t, "omp", "omp/")
	r.serve(binary)
	r.write(filepath.Join(r.home, ".profile"), "PI_CODING_AGENT_DIR=$HOME/.pi/agent\nexport PI_CODING_AGENT_DIR\n")
	separated := append([]string{"TK_ADAPTER_SEPARATE_AGENT_DIR=1"}, moved...)
	code, out = r.run(script, separated...)
	if code != 0 || !strings.Contains(out, "installed omp 9.9.9 for herdr from its release binary") ||
		!strings.Contains(out, "wrote the wrapper, which gives omp alone") {
		t.Fatalf("a separated base answered exit %d:\n%s", code, out)
	}
	before = r.read(r.fetches)
	code, out = r.run(script, separated...)
	if code != 0 || r.read(r.fetches) != before || !strings.Contains(out, "from its release, whose digest this adapter pins") ||
		!strings.Contains(out, "the omp wrapper is already in place") {
		t.Fatalf("a second run on a separated base answered exit %d, fetching %q after %q:\n%s", code, r.read(r.fetches), before, out)
	}
	if got := r.read(filepath.Join(r.prefixBin, "omp-npm")); got != string(binary) {
		t.Fatalf("the program behind the wrapper holds %q, and it must be the release binary", got)
	}

	// ⛔ A FIFO WHERE omp WAS holds nothing. The marker is read only from a regular file.
	r = newReleaseRig(t, "omp", "omp/")
	r.serve(binary)
	r.fifo(filepath.Join(r.prefixBin, "omp"))
	code, out = r.run(script, moved...)
	if code != 0 || r.read(filepath.Join(r.prefixBin, "omp")) != string(binary) {
		t.Fatalf("a FIFO at the account's omp answered exit %d:\n%s", code, out)
	}

	// ⛔ A SIGNAL DURING THE FETCH IS NOT A FETCH FAILURE.
	newReleaseRig(t, "omp", "omp/").signalled(script, "omp-linux-x64", moved...)
}

// TestTheOmpProbeAsksForBunOnlyWhereOmpIsABunProgram holds the probe to the two
// routes. WSL-138.
//
// ⛔ A RELEASE BINARY IS ONE ELF FILE WITH ITS OWN RUNTIME. A probe that asked every omp
// for Bun would fail every base that took the official route, because that route
// installs none. npm's omp is still a Bun program, and with no Bun it cannot start.
func TestTheOmpProbeAsksForBunOnlyWhereOmpIsABunProgram(t *testing.T) {
	agentScriptsRun(t, "od", "mkfifo")
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
	const elf = "\x7fELF\x02\x01\x01 not a program this case runs\n"

	// ⭐ THE RELEASE: an ELF file, and no Bun anywhere the account looks.
	r.write(launcher, elf)
	st := read()
	if st.Facts["source"] != "release" || bun(st) != 0 || len(st.Notes) != 0 {
		t.Fatalf("a release binary read as source=%q, notes %q, problems %q", st.Facts["source"], st.Notes, st.Problems)
	}

	// ⭐ THE RELEASE BEHIND THE SEPARATING WRAPPER is read where the wrapper runs it.
	r.write(launcher, "#!/bin/sh\n# wsl-toolkit omp adapter wrapper\nexec omp-npm \"$@\"\n")
	r.write(filepath.Join(r.prefixBin, "omp-npm"), elf)
	st = read()
	if st.Facts["source"] != "release" || bun(st) != 0 {
		t.Fatalf("a release behind the wrapper read as source=%q, problems %q", st.Facts["source"], st.Problems)
	}
	if err := os.Remove(filepath.Join(r.prefixBin, "omp-npm")); err != nil {
		t.Fatal(err)
	}

	// ⛔ npm's omp, a Bun program, with no Bun: the problem, and where it came from.
	pkg := filepath.Join(r.home, ".local", "lib", "node_modules", "@oh-my-pi", "pi-coding-agent", "dist", "cli.js")
	r.write(pkg, "#!/usr/bin/env bun\nconsole.log('omp/9.9.9')\n")
	if err := os.Remove(launcher); err != nil {
		t.Fatal(err)
	}
	r.link(pkg, launcher)
	st = read()
	if st.Facts["source"] != "npm" || bun(st) != 1 || len(st.Notes) != 1 || !strings.Contains(st.Notes[0], "came from npm") {
		t.Fatalf("npm's omp with no Bun read as source=%q, notes %q, problems %q", st.Facts["source"], st.Notes, st.Problems)
	}

	// ⛔ A FIFO AT THE ACCOUNT'S omp holds nothing. Each read of it waits for a regular file.
	if err := os.Remove(launcher); err != nil {
		t.Fatal(err)
	}
	r.fifo(launcher)
	if st = read(); st.Facts["source"] != "other" {
		t.Fatalf("a FIFO at the account's omp read as source=%q", st.Facts["source"])
	}
}

// TestThePiAndOmpFootprintsCountWhatTheirReleasesLeave holds each probe's footprint to
// what the WSL-138 installers can leave. That is an asset a refusal kept, and npm's
// package from before the release. ⛔ `base status` names a dropped adapter's leftovers
// from this answer alone.
func TestThePiAndOmpFootprintsCountWhatTheirReleasesLeave(t *testing.T) {
	agentScriptsRun(t, "du")
	for _, c := range []struct{ name, asset, pkg string }{
		{"pi", "pi-linux-x64.tar.gz", "@earendil-works/pi-coding-agent"},
		{"omp", "omp-linux-x64", "@oh-my-pi/pi-coding-agent"},
	} {
		probe, err := adapterScript(c.name, "probe.sh")
		if err != nil {
			t.Fatal(err)
		}
		r := newAgentRig(t)
		r.write(filepath.Join(r.stage, c.asset), strings.Repeat("a", 64<<10))
		r.write(filepath.Join(r.home, ".local", "lib", "node_modules", c.pkg, "cli.js"), strings.Repeat("b", 128<<10))
		code, out := r.run(probe, "TK_PROBE_FOOTPRINT=1")
		st := parseAdapterProbe(c.name, out)
		footprint, err := strconv.ParseInt(st.Facts["footprint"], 10, 64)
		if code != 0 || err != nil || st.Facts["installed"] != "no" || footprint < 192<<10 {
			t.Fatalf("%s: a kept asset of 64 KiB and npm's package of 128 KiB read as exit %d, footprint %q:\n%s", c.name, code, st.Facts["footprint"], out)
		}
		if left, ok := parseLeftover(c.name, out); !ok || left.Bytes != footprint {
			t.Fatalf("%s: base status would not name what the adapter left: %+v %v", c.name, left, ok)
		}
	}
}
