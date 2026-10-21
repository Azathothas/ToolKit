// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestTheMuseProbeAsksMuseWhetherItRunsTheReporter runs the muse adapter's probe.sh
// against a stand-in for Muse and the real herdr reporter. Finding 26, WSL-157.
//
// ⛔ THE SETTINGS FILE IS THE SAME IN EVERY RUN, and it registers the reporter for all
// six events. A probe that read that file called every run healthy. What differs is
// whether the stand-in runs the SessionStart hook the file names, as Muse would, and
// the probe has to say which.
//
// ⚠ THE RUNS SHARE ONE HOME, IN ORDER. The second runs over the first one's log line, so
// a probe that took any SessionStart as this session's would pass it.
func TestTheMuseProbeAsksMuseWhetherItRunsTheReporter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("probe.sh runs under a POSIX shell, which the Linux job has")
	}
	for _, tool := range []string{"sh", "timeout", "mktemp", "readlink"} {
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
		t.Skip("jq is not in the PATH probe.sh gives the account")
	}
	probe, err := adapterScript("muse", "probe.sh")
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := adapterTree.ReadFile("adapters/muse/herdr-agent-state.sh")
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	home, bin := filepath.Join(root, "home"), filepath.Join(root, "bin")
	hook := filepath.Join(home, ".local", "share", "wsl-toolkit", "herdr-agent-state.sh")
	settings := filepath.Join(home, ".config", "muse", "settings.json")
	for _, dir := range []string{bin, filepath.Join(home, ".local", "bin"), filepath.Dir(hook), filepath.Dir(settings)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, body string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(hook, string(reporter), 0o755)
	group := `[{"matcher":"","hooks":[{"type":"command","command":"` + hook + `"}]}]`
	events := []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PermissionRequest", "Stop", "SessionEnd"}
	var registered []string
	for _, e := range events {
		registered = append(registered, `"`+e+`":`+group)
	}
	write(settings, `{"schema_version":1,"hooks":{`+strings.Join(registered, ",")+`}}`+"\n", 0o600)

	herdr := filepath.Join(bin, "herdr")
	write(herdr, "#!/bin/sh\nif [ \"$1 $2\" = 'workspace list' ]; then echo '{\"result\":{\"workspaces\":[]}}'; fi\nexit 0\n", 0o755)
	write(filepath.Join(bin, "getent"), "#!/bin/sh\nprintf 'herdr:x:1000:1000::%s:/bin/bash\\n' \"$STUB_HOME\"\n", 0o755)
	write(filepath.Join(bin, "runuser"), "#!/bin/sh\n[ \"$1\" = -u ] && [ \"$2\" = herdr ] && [ \"$3\" = -- ] || exit 9\nshift 3\nexec \"$@\"\n", 0o755)

	// The stand-in for Muse. It answers a version, and for an echo session it prints a
	// record naming its session, writes down how it was started, and, while the mode file
	// says so, runs each SessionStart hook the settings file registers with Muse's
	// payload, in the cleared environment Muse gives a hook.
	mode, seen := filepath.Join(root, "mode"), filepath.Join(root, "seen")
	write(filepath.Join(home, ".local", "bin", "muse"), `#!/bin/sh
if [ "$1" = --version ]; then echo 'Muse Code 9.9.9 (9.9.9-R1)'; exit 0; fi
[ "$1" = exec ] || exit 64
case " $* " in *" --provider echo "*) ;; *) exit 65 ;; esac
case " $* " in *" --no-session-log "*) ;; *) exit 66 ;; esac
sid=$(cat /proc/sys/kernel/random/uuid)
{
  printf 'cwd %s\n' "$PWD"
  printf 'stdin %s\n' "$(readlink /proc/$$/fd/0)"
  printf 'data %s\n' "${XDG_DATA_HOME:-}"
  printf 'cache %s\n' "${XDG_CACHE_HOME:-}"
} > `+seen+`
case $(cat `+mode+`) in
  refuse) echo 'error: unknown provider echo' >&2; exit 2 ;;
  silent) printf '{"schema_version":1,"stream":{"kind":"run","id":"r1"}}\n'; exit 0 ;;
esac
printf '{"schema_version":1,"stream":{"kind":"session","id":"%s"},"payload_type":"run.lifecycle.started"}\n' "$sid"
if [ "$(cat `+mode+`)" = runs ]; then
  jq -r '.hooks.SessionStart[]?.hooks[]?.command' "$HOME/.config/muse/settings.json" | while read -r cmd; do
    printf '{"hook_event_name":"SessionStart","session_id":"%s","cwd":"%s","source":"startup"}' "$sid" "$PWD" |
      env -i HOME="$HOME" PATH="$PATH" WSL_TOOLKIT_HERDR_BIN=`+herdr+` sh "$cmd"
  done
fi
exit 0
`, 0o755)

	run := func(behaviour string) (AdapterState, map[string]string) {
		t.Helper()
		write(mode, behaviour+"\n", 0o644)
		_ = os.Remove(seen)
		cmd := exec.Command("sh", "-s")
		cmd.Stdin = strings.NewReader(string(probe))
		cmd.Env = append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base", "STUB_HOME="+home)
		out, err := cmd.CombinedOutput()
		var exited *exec.ExitError
		if err != nil && !errors.As(err, &exited) {
			t.Fatalf("probe.sh did not start: %v", err)
		}
		if code := cmd.ProcessState.ExitCode(); code != 0 {
			t.Fatalf("probe.sh exited %d, and a probe exits 0 whatever it finds:\n%s", code, out)
		}
		started := map[string]string{}
		if body, err := os.ReadFile(seen); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
				k, v, _ := strings.Cut(line, " ")
				started[k] = v
			}
		}
		return parseAdapterProbe("muse", string(out)), started
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

	st, started := run("runs")
	if st.Facts["herdr_reporter_session_start"] != "reached" || len(about(st, "SessionStart")) != 0 {
		t.Fatalf("a Muse that runs the hook was read as %q with %v", st.Facts["herdr_reporter_session_start"], st.Problems)
	}
	scratch := filepath.Dir(started["cwd"])
	if !strings.HasPrefix(filepath.Base(scratch), "wsl-toolkit-muse-probe.") || started["cwd"] == home {
		t.Fatalf("Muse ran in %q, and it must run in a fresh directory no herdr pane has", started["cwd"])
	}
	if started["stdin"] != "/dev/null" {
		t.Fatalf("Muse's stdin was %q, and this script reaches its shell on stdin", started["stdin"])
	}
	for _, k := range []string{"data", "cache"} {
		if !strings.HasPrefix(started[k], scratch+"/") {
			t.Fatalf("Muse's %s directory was %q, and the session must leave nothing of Muse's in the account's home", k, started[k])
		}
	}
	if _, err := os.Stat(scratch); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the probe left its scratch directory %s behind (%v)", scratch, err)
	}

	st, _ = run("quiet")
	if st.Facts["herdr_reporter_events"] == "none" {
		t.Fatal("the fixture registers no event, so this run cannot show the file disagreeing with Muse")
	}
	if st.Facts["herdr_reporter_session_start"] == "reached" || len(about(st, "never reached the herdr reporter")) != 1 {
		t.Fatalf("a Muse that ran no hook over a file registering it was read as %q with %v",
			st.Facts["herdr_reporter_session_start"], st.Problems)
	}

	st, _ = run("refuse")
	if len(about(st, "exit 2: error: unknown provider echo")) != 1 {
		t.Fatalf("a Muse that could not run the echo session was read as %v", st.Problems)
	}

	st, _ = run("silent")
	if len(about(st, "named no session")) != 1 {
		t.Fatalf("a Muse whose answer named no session was read as %v", st.Problems)
	}
}
