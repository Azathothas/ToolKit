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
	// ⛔ ROOT'S mktemp IS FOUND ON PATH, so a stand-in records each file it makes, and
	// only those are checked for leftovers. /tmp is every go test run's, and a glob
	// there read another run's stderr file as this probe's.
	realMktemp, err := exec.LookPath("mktemp")
	if err != nil {
		t.Fatal(err)
	}
	made := filepath.Join(root, "made")
	write(filepath.Join(bin, "mktemp"), "#!/bin/sh\nout=$("+shellQuote(realMktemp)+" \"$@\") || exit $?\nprintf '%s\\n' \"$out\" >> "+shellQuote(made)+"\nprintf '%s\\n' \"$out\"\n", 0o755)
	// ⛔ ROOT'S rm IS FOUND ON PATH TOO, and as_account's PATH does not hold it, so a
	// removal of the account's scratch directory by root is recorded. The account's own
	// rm -rf walks it as the account. WSL-202.
	realRm, err := exec.LookPath("rm")
	if err != nil {
		t.Fatal(err)
	}
	rootRm := filepath.Join(root, "root-rm")
	write(filepath.Join(bin, "rm"), "#!/bin/sh\nfor a do\n  case $a in */wsl-toolkit-muse-probe.*) printf 'rm %s\\n' \"$*\" >> "+shellQuote(rootRm)+"; break ;; esac\ndone\nexec "+shellQuote(realRm)+" \"$@\"\n", 0o755)
	// ⛔ IN plant MODE, THE ACCOUNT RACES ROOT. Right after the account makes the
	// probe's scratch directories, a process of the account puts a link named stderr
	// in its own directory, pointing at a file it cannot write. Root's shell must
	// never open it. WSL-157.
	mode, seen := filepath.Join(root, "mode"), filepath.Join(root, "seen")
	target := filepath.Join(root, "roots-own-file")
	const targetBody = "a file only root may write\n"
	write(target, targetBody, 0o644)
	write(filepath.Join(bin, "runuser"), `#!/bin/sh
[ "$1" = -u ] && [ "$2" = herdr ] && [ "$3" = -- ] || exit 9
shift 3
case " $* " in
  *" mkdir "*/work" "*)
    if [ "$(cat `+mode+`)" = plant ]; then
      "$@" || exit $?
      for last; do :; done
      ln -s `+target+` "${last%/cache}/stderr"
      exit 0
    fi
    ;;
esac
exec "$@"
`, 0o755)

	// The stand-in for Muse. It answers a version, and for an echo session it prints a
	// record naming its session, writes down how it was started, and, while the mode file
	// says so, runs each SessionStart hook the settings file registers with Muse's
	// payload, in the cleared environment Muse gives a hook. In async mode it runs them
	// after it has answered, as a Muse that does not wait for its hooks would.
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
echo 'muse: the echo provider answered' >&2
hooks() {
  jq -r '.hooks.SessionStart[]?.hooks[]?.command' "$HOME/.config/muse/settings.json" | while read -r cmd; do
    printf '{"hook_event_name":"SessionStart","session_id":"%s","cwd":"%s","source":"startup"}' "$sid" "$PWD" |
      env -i HOME="$HOME" PATH="$PATH" WSL_TOOLKIT_HERDR_BIN=`+herdr+` sh "$cmd"
  done
}
case $(cat `+mode+`) in
  runs|plant) hooks ;;
  async) (sleep 1; hooks) </dev/null >/dev/null 2>&1 & ;;
esac
exit 0
`, 0o755)

	// read runs the probe as `base status --probe` and `ready` do, run as `base ensure` does.
	read := func(behaviour string, session bool) (AdapterState, map[string]string) {
		t.Helper()
		write(mode, behaviour+"\n", 0o644)
		_ = os.Remove(seen)
		cmd := exec.Command("sh", "-s")
		cmd.Stdin = strings.NewReader(string(probe))
		cmd.Env = append(os.Environ(),
			"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
			"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base", "STUB_HOME="+home, "TK_PROBE_SESSION=")
		if session {
			cmd.Env = append(cmd.Env, "TK_PROBE_SESSION=1")
		}
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
	run := func(behaviour string) (AdapterState, map[string]string) {
		t.Helper()
		return read(behaviour, true)
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
	if b, _ := os.ReadFile(rootRm); len(b) != 0 {
		t.Fatalf("root removed the account's scratch directory itself, through whatever it held:\n%s", b)
	}

	// ⛔ A MUSE THAT RAN NO HOOK OVER A FILE REGISTERING IT IS A NOTE NAMING ITS VERSION.
	// `base ensure` writes the same file again and cannot cure it, so as a problem it
	// failed every ensure with a remedy that loops. WSL-157.
	st, _ = run("quiet")
	if st.Facts["herdr_reporter_events"] == "none" {
		t.Fatal("the fixture registers no event, so this run cannot show the file disagreeing with Muse")
	}
	if st.Facts["herdr_reporter_session_start"] != "missing" || len(about(st, "SessionStart")) != 0 {
		t.Fatalf("a Muse that ran no hook over a file registering it was read as %q with %v",
			st.Facts["herdr_reporter_session_start"], st.Problems)
	}
	if len(st.Notes) != 1 || !strings.Contains(st.Notes[0], "Muse 9.9.9 (9.9.9-R1) ran session ") ||
		!strings.Contains(st.Notes[0], "base ensure cannot cure this") || strings.Contains(st.Notes[0], "Run: base ensure") {
		t.Fatalf("a Muse that ran no hook earned notes %q; want one naming its version and that ensure cannot cure it", st.Notes)
	}

	// ⛔ AND OVER A FILE THAT DOES NOT REGISTER IT, base ensure DOES CURE IT, so it is a
	// problem naming that remedy.
	write(settings, `{"schema_version":1,"hooks":{"Stop":`+group+`}}`+"\n", 0o600)
	st, _ = run("quiet")
	if found := about(st, "does not register it for SessionStart. Run: base ensure"); len(found) != 1 || len(st.Notes) != 0 {
		t.Fatalf("a settings file that does not register SessionStart was read as %v with notes %q", st.Problems, st.Notes)
	}
	write(settings, `{"schema_version":1,"hooks":{`+strings.Join(registered, ",")+`}}`+"\n", 0o600)

	// ⚠ A HOOK THAT RUNS AFTER exec RETURNS STILL COUNTS, read again for a few seconds.
	st, _ = run("async")
	if st.Facts["herdr_reporter_session_start"] != "reached" {
		t.Fatalf("a Muse that runs the hook just after it answers was read as %q with %q",
			st.Facts["herdr_reporter_session_start"], st.Notes)
	}

	// ⛔ ROOT NEVER OPENS A FILE IN THE ACCOUNT'S DIRECTORY. A link the account put there
	// would have turned root's write onto any file on the base. WSL-157.
	st, _ = run("plant")
	if got, err := os.ReadFile(target); err != nil || string(got) != targetBody {
		t.Fatalf("root wrote through a link the account planted in the scratch directory: %q (%v)", got, err)
	}
	if st.Facts["herdr_reporter_session_start"] != "reached" {
		t.Fatalf("the planted run was read as %q", st.Facts["herdr_reporter_session_start"])
	}
	raw, _ := os.ReadFile(made)
	var errFiles, left []string
	for _, p := range strings.Fields(string(raw)) {
		if strings.HasPrefix(filepath.Base(p), "wsl-toolkit-muse-probe-stderr.") {
			errFiles = append(errFiles, p)
			if _, err := os.Lstat(p); err == nil {
				left = append(left, p)
			}
		}
	}
	if len(errFiles) == 0 {
		t.Fatalf("root's mktemp made no stderr file through the stand-in, so a file left behind cannot be seen: %q", raw)
	}
	if len(left) != 0 {
		t.Fatalf("the probe left its stderr file behind: %v", left)
	}

	// ⛔ A READ STARTS NO SESSION. `base status --probe` and `ready` read; only `base
	// ensure` asks for the session, which runs the account's hooks. WSL-157.
	st, started = read("runs", false)
	if len(started) != 0 {
		t.Fatalf("a read started a Muse session: %v", started)
	}
	if got, ok := st.Facts["herdr_reporter_session_start"]; ok || len(st.Notes) != 0 || len(about(st, "session")) != 0 {
		t.Fatalf("a read answered a session it never ran: %q %q %v", got, st.Notes, st.Problems)
	}

	// ⛔ A MUSE THAT CANNOT ANSWER THE QUESTION LEAVES IT UNMEASURED, AND FAILS NOTHING.
	// Muse's version is not pinned, so a later one that drops a flag the probe passes
	// must not fail a `base ensure` that 7.0.0 passed. The fact says why. WSL-157.
	unmeasured := func(name, why string, st AdapterState) {
		t.Helper()
		got := st.Facts["herdr_reporter_session_start"]
		if !strings.HasPrefix(got, "unmeasured: ") || !strings.Contains(got, why) {
			t.Fatalf("%s was read as %q, and it is unmeasured naming %q", name, got, why)
		}
		if found := about(st, "session"); len(found) != 0 {
			t.Fatalf("%s was read as a problem, and a Muse that cannot run the probe's session fails nothing: %v", name, found)
		}
	}
	st, _ = run("refuse")
	unmeasured("a Muse that could not run the echo session", "exit 2: error: unknown provider echo", st)

	st, _ = run("silent")
	unmeasured("a Muse whose answer named no session", "named no session", st)
}
