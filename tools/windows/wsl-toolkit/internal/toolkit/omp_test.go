package toolkit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ompConfig(sep bool) Config {
	cfg := DefaultConfig()
	omp := BaseAdapter{Name: "omp", SeparateAgentDir: sep}
	cfg.Base.Adapters = []BaseAdapter{omp}
	return cfg
}

// ⛔ REFUSED ON AN ADAPTER THAT CANNOT USE IT, rather than accepted and dropped. A
// setting silently ignored is one the operator believes is in force.
func TestSeparateAgentDirIsRefusedOnAnAdapterThatCannotTakeIt(t *testing.T) {
	cfg := DefaultConfig()
	pi := BaseAdapter{Name: "pi", SeparateAgentDir: true}
	cfg.Base.Adapters = []BaseAdapter{pi}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "separate_agent_dir") {
		t.Fatalf("Validate() = %v, want a refusal naming separate_agent_dir", err)
	}
	if err := ompConfig(true).Validate(); err != nil {
		t.Fatalf("Validate() on omp = %v, want it accepted", err)
	}
}

// ⭐ The flag has to REACH the script, and it reaches it as an environment variable
// the installer reads. A field set in the configuration and never passed is the
// defect base.adapters itself shipped with once.
func TestSeparateAgentDirReachesTheInstaller(t *testing.T) {
	on, err := adapterInstallEnv(ompConfig(true), ompConfig(true).Base.Adapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if on["TK_ADAPTER_SEPARATE_AGENT_DIR"] != "1" {
		t.Fatalf("the installer's environment carries %q, want \"1\"", on["TK_ADAPTER_SEPARATE_AGENT_DIR"])
	}
	off, err := adapterInstallEnv(ompConfig(false), ompConfig(false).Base.Adapters[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, present := off["TK_ADAPTER_SEPARATE_AGENT_DIR"]; present {
		t.Fatal("a base that did not ask to separate still carries the variable, so the refusal would never be the default")
	}
}

// ompScript is the installer the executable carries, which is the copy that runs.
func ompScript(t *testing.T) string {
	t.Helper()
	body, err := adapterScript("omp", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// ⛔ THE COLLISION REFUSAL WAS DEAD CODE, and only a login shell can reach the
// variable it turns on. as_account runs `env -i`, which clears the environment on
// purpose, so a plain `sh -c` under it reports every variable unset. Measured on
// 2026-09-17 with `export PI_CODING_AGENT_DIR` in the account's profile: a login
// shell answered the path and the adapter answered nothing, so the refusal could
// never fire on any base.
func TestTheAccountsEnvironmentIsReadThroughALoginShell(t *testing.T) {
	script := ompScript(t)
	if !strings.Contains(script, `as_account sh -lc "printf '%s' \"\${$1:-}\""`) {
		t.Fatal("account_env no longer reads a login shell, so every variable it asks about reads as unset under env -i")
	}
}

// ⛔ THE OVERRIDE IS ONLY FOR THE SEPARATED CASE. PI_CODING_AGENT_DIR is read by
// BOTH agents, so exporting it for the integration install on a base that did not
// separate makes herdr resolve pi's directory to omp's value, see one directory for
// two agents, and refuse the install it was asked for. Measured 2026-09-17.
func TestTheIntegrationOverrideIsOnlyAppliedWhenSeparating(t *testing.T) {
	script := ompScript(t)
	if !strings.Contains(script, `if [ "$OMP_SEPARATED" = yes ]; then
    set -- as_account env PI_CODING_AGENT_DIR="$OMP_AGENT_DIR" "$herdr_bin" integration install omp
  else
    set -- as_account "$herdr_bin" integration install omp
  fi`) {
		t.Fatal("the herdr integration install no longer chooses its environment by whether the base separated")
	}
}

// ⛔ THE WRAPPER IS TOLD FROM npm's SHIM BY CONTENT. npm's prefix for the account is
// $HOME/.local, so its own shim and the wrapper want the same filename; a guard that
// read the path alone called the real shim a wrapper and refused to install.
func TestTheWrapperIsDetectedByItsMarkerAndNotByItsPath(t *testing.T) {
	script := ompScript(t)
	if !strings.Contains(script, `head -3 "$OMP_WRAPPER" 2>/dev/null | grep -qF "$OMP_MARK"`) {
		t.Error("the installer no longer tells its wrapper from npm's shim by content")
	}
	if !strings.Contains(script, `mv -f "$OMP_WRAPPER" "$OMP_REAL"`) {
		t.Error("the installer no longer moves npm's shim aside, so the wrapper would have nothing to run")
	}
	// ⛔ And the wrapper must be shown to WIN, not assumed to.
	if !strings.Contains(script, `[ "$OMP_RESOLVED" = "$OMP_WRAPPER" ] ||`) {
		t.Error("the installer no longer reads back which omp the account's PATH resolves, so a shadowed wrapper would pass")
	}
}

// ⛔ omp IS A BUN PROGRAM AND THE ADAPTER MUST NOT GET BUN FROM npm. Measured
// 2026-09-17: `npm install -g --ignore-scripts bun` exits 0 and leaves a bun that
// refuses to run, because its postinstall is what downloads the binary. Getting a
// working one that way means running a third-party script that fetches an unverified
// binary, which this repository refuses.
func TestBunComesFromTheDistributionAndNotFromNpm(t *testing.T) {
	script := ompScript(t)
	if !strings.Contains(script, "pacman -S --noconfirm bun") {
		t.Error("the adapter no longer installs bun from the distribution, which omp needs to run at all")
	}
	// ⛔ CODE ONLY. The comment above the install says what npm does and why it is
	// refused, and a bare Contains matched that comment and failed over the very
	// sentence explaining the rule. A negative assertion has to read what RUNS.
	var code []string
	for _, line := range strings.Split(script, "\n") {
		if t := strings.TrimSpace(line); !strings.HasPrefix(t, "#") {
			code = append(code, line)
		}
	}
	running := strings.Join(code, "\n")
	if strings.Contains(running, "npm install -g bun") || strings.Contains(running, "install -g --ignore-scripts bun") {
		t.Error("the adapter installs bun from npm, which leaves a bun that refuses to run without its postinstall")
	}
}

// ⛔ A FIELD INSIDE AN ADAPTER IS OUTSIDE THE REFLECT WALK that guards the rest.
// TestEveryStoredBaseFieldSurvivesLoading says so in its own comment: it walks
// BaseConfig's fields and cannot see into BaseAdapter. base.adapters was once
// decoded, validated as empty and dropped; a field added inside one can be dropped
// the same way with nothing to catch it. Found by WSL-89's door sweep on 2026-09-17.
func TestSeparateAgentDirSurvivesAStoredConfiguration(t *testing.T) {
	dir := t.TempDir()
	stored := ompConfig(true)
	stored.Schema = ConfigSchema
	stored.Base.Name = DefaultBaseName
	body, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"separate_agent_dir": true`) {
		t.Fatalf("the setting does not survive being written out: %s", body)
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
	if len(got.Base.Adapters) != 1 || !got.Base.Adapters[0].SeparateAgentDir {
		t.Fatalf("loading dropped separate_agent_dir: %+v", got.Base.Adapters)
	}
}
