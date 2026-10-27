// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// -- WSL-158: herdr's detection rules for Muse ---------------------------------------

// TestTheMuseRulesMatchTheScreenMuseDraws holds the two rules the muse adapter's file
// changes from herdr's own, against the lines Muse Code 1.4.13 drew in a herdr pane.
//
// ⛔ herdr's OWN MANIFEST MATCHED NEITHER: its prompt rule knows only `⟩`, and its status
// rule lists no `max`. `herdr agent explain` answered `rule: none` for that screen, and
// `idle_prompt` once this file was in place.
// ⭐ EVERY OTHER RULE OF herdr's IS KEPT, so the file cannot lose the blocked and
// working states herdr already detected on 1.4.13.
func TestTheMuseRulesMatchTheScreenMuseDraws(t *testing.T) {
	body, err := adapterTree.ReadFile("adapters/muse/herdr-detection.toml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "\nid = \"muse\"\n") {
		t.Fatal("the rules do not name the muse agent, and herdr ignores an override whose id does not match")
	}
	for _, id := range []string{"workspace_trust_blocked", "pick_request_blocked", "menu_overlay",
		"working_esc_interrupt", "blocked_approval", "idle_prompt", "idle_status_fallback"} {
		if !strings.Contains(text, "\nid = \""+id+"\"\n") {
			t.Errorf("the rules drop herdr's own %s", id)
		}
	}
	// rule answers the line_regex patterns of one rule, as Go reads them.
	rule := func(id string) []*regexp.Regexp {
		t.Helper()
		start := strings.Index(text, "\nid = \""+id+"\"\n")
		if start < 0 {
			t.Fatalf("no rule %s", id)
		}
		end := strings.Index(text[start+1:], "[[rules]]")
		section := text[start:]
		if end >= 0 {
			section = text[start : start+1+end]
		}
		var res []*regexp.Regexp
		for _, m := range regexp.MustCompile(`line_regex = \['([^']*)'\]`).FindAllStringSubmatch(section, -1) {
			res = append(res, regexp.MustCompile(m[1]))
		}
		if len(res) == 0 {
			t.Fatalf("rule %s carries no line_regex", id)
		}
		return res
	}
	matches := func(res []*regexp.Regexp, line string) bool {
		for _, re := range res {
			if re.MatchString(line) {
				return true
			}
		}
		return false
	}
	// ⚠ THE SCREEN'S OWN CHARACTERS AS CODE POINTS: U+276F is Muse 1.4.13's prompt,
	// U+27E9 the older one, U+00B7 the status line's dot, U+25C7 a turn's mark and
	// U+2500 a rule. This tree keeps such characters out of its text.
	const now, older, dot = "\u276f", "\u27e9", " \u00b7 "
	prompt := rule("idle_prompt")
	for _, line := range []string{now, now + " say hello", older, "  " + older + " draft"} {
		if !matches(prompt, line) {
			t.Errorf("idle_prompt does not match Muse's prompt line %q", line)
		}
	}
	status := rule("idle_status_fallback")
	for _, line := range []string{"  muse-spark-1.3-contributor" + dot + "max" + dot + "~" + dot + "Auto-review",
		"  echo" + dot + "~" + dot + "Auto-review", "  m" + dot + "ultra" + dot + "~"} {
		if !matches(status, line) {
			t.Errorf("idle_status_fallback does not match Muse's status line %q", line)
		}
	}
	for _, line := range []string{"\u25c7 Thinking (4s" + dot + "esc to interrupt)", strings.Repeat("\u2500", 7)} {
		if matches(prompt, line) || matches(status, line) {
			t.Errorf("an idle rule matches %q, which is not an idle screen's line", line)
		}
	}
}

// museRulesHarness stands in for a base around the muse adapter's install.sh, as far
// as it writes herdr's rules, and answers a runner for it, a writer for its stand-ins,
// the path herdr reads the rules at, and the file of what the account was asked.
func museRulesHarness(t *testing.T) (run func() string, write func(name, body string), target, asked string) {
	t.Helper()
	needPosix(t, "sha256sum", "base64", "mktemp", "cp", "mkdir", "chmod", "rm", "sed", "cut")
	script, err := adapterScript("muse", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := adapterTree.ReadFile("adapters/muse/herdr-detection.toml")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	asked = filepath.Join(root, "asked")
	if err := os.MkdirAll(filepath.Join(home, ".local", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	// An installed Muse, so the installer is never fetched.
	if err := os.WriteFile(filepath.Join(home, ".local", "bin", "muse"), []byte("#!/bin/sh\necho \"Muse Code 9.9.9 (9.9.9-R1)\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin, write := stubDir(t)
	for _, name := range []string{"pacman", "chown", "bwrap"} {
		write(name, "exit 0\n")
	}
	write("curl", "exit 9\n")
	write("getent", "printf 'herdr:x:1000:1000::%s:/bin/bash\\n' \"$STUB_HOME\"\n")
	write("id", "echo herdr\n")
	// runuser records what the account is asked, and answers for herdr's server as a
	// running one would: the account's PATH is fixed, and no herdr is on it here.
	write("runuser", `[ "$1" = -u ] && [ "$2" = herdr ] && [ "$3" = -- ] || exit 9
shift 3
printf 'runuser %s\n' "$*" >> "$STUB_ASKED"
case " $* " in *" herdr server "*) exit 0 ;; esac
exec "$@"
`)
	// install makes directories and copies files, whatever owner it is asked for.
	write("install", `dir=; mode=
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
	env := []string{"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base", "STUB_HOME=" + home, "STUB_ASKED=" + asked,
		"TK_MUSE_STAGE_DIR=" + filepath.Join(root, "stage"), "TK_MUSE_WRAPPER=" + filepath.Join(root, "usr-local-bin", "muse"),
		"TK_FILE_HERDR_DETECTION_TOML_B64=" + base64.StdEncoding.EncodeToString(rules)}
	target = filepath.Join(home, ".config", "herdr", "agent-detection", "muse.toml")
	run = func() string {
		t.Helper()
		_ = os.Remove(asked)
		code, out := runSh(t, script, bin, nil, env...)
		if code != 0 || !strings.HasSuffix(strings.TrimSpace(out), adapterCompleteLine("muse")) {
			t.Fatalf("install.sh exited %d:\n%s", code, out)
		}
		return out
	}
	return run, write, target, asked
}

// TestTheMuseAdapterWritesHerdrsRulesAndHerdrReloadsThem runs the muse adapter's
// install.sh against stand-ins, with herdr in the base. WSL-158.
//
// ⭐ THE FILE IS WHERE herdr LOOKS, BYTE FOR BYTE, and a running server is asked to
// reload it, which ends no pane. A second run over the same file asks nothing.
func TestTheMuseAdapterWritesHerdrsRulesAndHerdrReloadsThem(t *testing.T) {
	run, write, target, asked := museRulesHarness(t)
	rules, err := adapterTree.ReadFile("adapters/muse/herdr-detection.toml")
	if err != nil {
		t.Fatal(err)
	}
	write("herdr", "exit 0\n")
	out := run()
	got, err := os.ReadFile(target)
	if err != nil || string(got) != string(rules) {
		t.Fatalf("the rules at %s are not the adapter's file (%v):\n%s", target, err, out)
	}
	calls, _ := os.ReadFile(asked)
	if !strings.Contains(string(calls), " herdr server reload-agent-manifests") || !strings.Contains(out, "the herdr server reloaded it") {
		t.Fatalf("a running herdr was not asked to reload the rules it reads at its start:\n%s\n%s", calls, out)
	}
	out = run()
	calls, _ = os.ReadFile(asked)
	if !strings.Contains(out, "are in place") || strings.Contains(string(calls), "reload-agent-manifests") {
		t.Fatalf("a second run over the same rules reloaded or rewrote them:\n%s\n%s", calls, out)
	}
	// ⭐ A FILE OF THE OPERATOR'S OWN IS LEFT AS IT IS, and a hand edit of ours is not.
	own := "id = \"muse\"\n# the operator's own rules\n"
	if err := os.WriteFile(target, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	out = run()
	if got, _ := os.ReadFile(target); string(got) != own || !strings.Contains(out, "is not this adapter's") {
		t.Fatalf("the operator's own rules became %q:\n%s", got, out)
	}
	edited := strings.Replace(string(rules), "priority = 700", "priority = 701", 1)
	if err := os.WriteFile(target, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	run()
	if got, _ := os.ReadFile(target); string(got) != string(rules) {
		t.Fatal("a hand edit of the adapter's rules outlived an ensure")
	}
}

// TestTheMuseAdapterWritesNoHerdrRulesWithoutHerdr: a rule file for a reader that is
// not there is a file nobody owns, so a base without herdr gets none. WSL-158.
//
// ⚠ IT SKIPS ON A MACHINE WITH A herdr OF ITS OWN, where install.sh also looks, because
// there a base without one cannot be stood in for and the case could not fail.
func TestTheMuseAdapterWritesNoHerdrRulesWithoutHerdr(t *testing.T) {
	if _, err := os.Stat("/usr/local/bin/herdr"); err == nil {
		t.Skip("this machine has /usr/local/bin/herdr, so a base without herdr cannot be stood in for")
	}
	if _, err := exec.LookPath("herdr"); err == nil {
		t.Skip("this machine has a herdr on PATH, so a base without herdr cannot be stood in for")
	}
	run, _, target, _ := museRulesHarness(t)
	out := run()
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) || !strings.Contains(out, "no herdr detection rules: no herdr is installed") {
		t.Fatalf("a base without herdr got the rules (%v):\n%s", err, out)
	}
}

// TestTheMuseProbeSaysWhetherHerdrLoadsItsRules runs the muse adapter's probe.sh
// against stand-ins for herdr's answer. WSL-158.
//
// ⭐ THE FILE BEING THERE IS NOT herdr READING IT. herdr ignores an override it cannot
// read, with a warning and its own rules instead, so the probe asks herdr which rules
// it uses, and says so in a note when they are not this file.
func TestTheMuseProbeSaysWhetherHerdrLoadsItsRules(t *testing.T) {
	needPosix(t, "sed", "grep", "head", "cut")
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
	rules, err := adapterTree.ReadFile("adapters/muse/herdr-detection.toml")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	launcher := filepath.Join(home, ".local", "bin", "muse")
	target := filepath.Join(home, ".config", "herdr", "agent-detection", "muse.toml")
	for _, dir := range []string{filepath.Dir(launcher), filepath.Dir(target)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(launcher, []byte("#!/bin/sh\necho 'Muse Code 9.9.9 (9.9.9-R1)'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bin, write := stubDir(t)
	write("getent", "printf 'herdr:x:1000:1000::%s:/bin/bash\\n' \"$STUB_HOME\"\n")
	// runuser answers for herdr's server as the case says, and runs everything else.
	write("runuser", `[ "$1" = -u ] && [ "$2" = herdr ] && [ "$3" = -- ] || exit 9
shift 3
case " $* " in
  *" herdr server agent-manifests "*)
    [ -n "${STUB_KIND:-}" ] || exit 1
    printf '{"result":{"manifests":[{"agent":"pi","source_kind":"remote"},{"agent":"muse","source_kind":"%s"%s}]}}\n' "$STUB_KIND" "${STUB_WARNING:-}"
    exit 0 ;;
esac
exec "$@"
`)
	read := func(extra ...string) AdapterState {
		t.Helper()
		code, out := runSh(t, probe, bin, nil, append([]string{"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base", "STUB_HOME=" + home}, extra...)...)
		if code != 0 {
			t.Fatalf("probe.sh exited %d, and a probe exits 0 whatever it finds:\n%s", code, out)
		}
		return parseAdapterProbe("muse", out)
	}
	noted := func(st AdapterState, word string) bool {
		for _, n := range st.Notes {
			if strings.Contains(n, word) {
				return true
			}
		}
		return false
	}

	if st := read(); st.Facts["herdr_detection_rules"] != "absent" {
		t.Fatalf("a base with no rules file was read as %q", st.Facts["herdr_detection_rules"])
	}
	if err := os.WriteFile(target, rules, 0o644); err != nil {
		t.Fatal(err)
	}
	st := read("STUB_KIND=local override")
	if st.Facts["herdr_detection_rules"] != "installed" || st.Facts["herdr_detection_source"] != "local override" || noted(st, "herdr reads its own Muse rules") {
		t.Fatalf("rules herdr loads were read as %v with %v", st.Facts, st.Notes)
	}
	st = read("STUB_KIND=bundled", `STUB_WARNING=,"warning":"ignored override because it could not be compiled"`)
	// ⚠ A NOTE AND NOT A PROBLEM: base ensure writes the same file and cannot cure it.
	if st.Facts["herdr_detection_source"] != "bundled" || !noted(st, "could not be compiled") || strings.Contains(strings.Join(st.Problems, " "), "rules") {
		t.Fatalf("rules herdr ignores were read as %v with %v and %v", st.Facts, st.Notes, st.Problems)
	}
	if st := read(); st.Facts["herdr_detection_rules"] != "installed" || st.Facts["herdr_detection_source"] != "" {
		t.Fatalf("with no herdr server answering, the rules were read as %v", st.Facts)
	}
	if err := os.WriteFile(target, []byte("id = \"muse\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := read(); st.Facts["herdr_detection_rules"] != "other" {
		t.Fatalf("a rules file of someone else's was read as %q", st.Facts["herdr_detection_rules"])
	}
}
