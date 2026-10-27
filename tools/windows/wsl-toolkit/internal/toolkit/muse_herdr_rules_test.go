// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
	// U+2500 a rule. Each is named by number, as the rules file names them.
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
// as it writes herdr's rules. It answers a runner for it and the path herdr reads the
// rules at. It also answers the file of what the account was asked, and herdr's path.
//
// ⚠ THE BASE HAS NO herdr UNTIL A CASE PUTS ONE AT herdr. TK_HERDR_BIN names that one
// place, in the case's own directory and off PATH. So a herdr on the machine running
// the suite is never found in its place, and nothing but that variable finds the case's.
func museRulesHarness(t *testing.T) (run func() string, target, asked, herdr string) {
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
	herdr = filepath.Join(root, "herdr-bin", "herdr")
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
		"TK_HERDR_BIN=" + herdr,
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
	return run, target, asked, herdr
}

// TestTheMuseAdapterWritesHerdrsRulesAndHerdrReloadsThem runs the muse adapter's
// install.sh against stand-ins, with herdr in the base. WSL-158.
//
// ⭐ THE FILE IS WHERE herdr LOOKS, BYTE FOR BYTE, and a running server is asked to
// reload it, which ends no pane. A second run over the same file asks nothing.
func TestTheMuseAdapterWritesHerdrsRulesAndHerdrReloadsThem(t *testing.T) {
	run, target, asked, herdr := museRulesHarness(t)
	rules, err := adapterTree.ReadFile("adapters/muse/herdr-detection.toml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(herdr), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(herdr, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
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
// ⭐ IT RUNS ON A MACHINE WITH A herdr OF ITS OWN TOO. It skipped there, which is every
// base this tool built with the herdr adapter, because install.sh looked only at the
// machine's own paths. TK_HERDR_BIN now names the case's own.
func TestTheMuseAdapterWritesNoHerdrRulesWithoutHerdr(t *testing.T) {
	run, target, _, _ := museRulesHarness(t)
	out := run()
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) || !strings.Contains(out, "no herdr detection rules: no herdr is installed") {
		t.Fatalf("a base without herdr got the rules (%v):\n%s", err, out)
	}
}

// TestTheMuseProbeSaysWhetherHerdrLoadsItsRules runs the muse adapter's probe.sh
// against stand-ins for herdr's answer. WSL-158.
//
// ⭐ THE FILE BEING THERE IS NOT herdr READING IT. herdr ignores an override it cannot
// read, with a warning and its own rules instead. So the probe asks herdr which rules
// it uses, and says so in a note when they are not this file.
//
// ⭐ AND THE FILE SHADOWS A NEWER MANIFEST OF herdr's. A remote version other than the
// one the rules were measured against is a note naming it.
//
// ⚠ jq IS REACHED THROUGH as_account, WHOSE PATH IS FIXED. Where that PATH has a jq,
// the probe's own program reads herdr's answer. Where it has none, as in golang:1.25,
// museProbeJQ evaluates that same program's text. A field the program misnames then
// reads null on either kind of host.
func TestTheMuseProbeSaysWhetherHerdrLoadsItsRules(t *testing.T) {
	needPosix(t, "sed", "grep", "head", "cut", "cat")
	realJQ := ""
	for _, dir := range []string{"/usr/local/sbin", "/usr/local/bin", "/usr/bin", "/bin"} {
		if _, err := os.Stat(filepath.Join(dir, "jq")); err == nil {
			realJQ = "1"
		}
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
	// ⚠ ROOT'S jq IS ONLY ASKED WHETHER IT IS THERE. Every call goes through as_account.
	write("jq", "exit 0\n")
	// runuser answers herdr's server with STUB_MANIFESTS, or fails where it is empty.
	// It answers the probe's jq program with STUB_JQ_ANSWER where the account's PATH
	// has no jq, and runs everything else.
	write("runuser", `[ "$1" = -u ] && [ "$2" = herdr ] && [ "$3" = -- ] || exit 9
shift 3
case " $* " in
  *" herdr server agent-manifests "*)
    [ -n "${STUB_MANIFESTS:-}" ] || exit 1
    printf '%s\n' "$STUB_MANIFESTS"
    exit 0 ;;
  *'select(.agent == "muse")'*)
    [ -z "${STUB_REAL_JQ:-}" ] || exec "$@"
    input=$(cat)
    [ -n "$input" ] || exit 0
    printf '%s\n' "$STUB_JQ_ANSWER"
    exit 0 ;;
esac
exec "$@"
`)
	// answer is herdr's agent-manifests answer, with muse's entry as herdr 0.9.5 and
	// 5608de0 shape it: local_override_shadowing_remote always, the rest when set.
	answer := func(muse map[string]any) string {
		if muse == nil {
			return ""
		}
		muse["agent"] = "muse"
		if _, ok := muse["local_override_shadowing_remote"]; !ok {
			muse["local_override_shadowing_remote"] = false
		}
		body, err := json.Marshal(map[string]any{"id": "cli:server:agent-manifests", "result": map[string]any{
			"type": "agent_manifest_status",
			"manifests": []any{
				map[string]any{"agent": "pi", "source": "bundled", "source_kind": "bundled", "local_override_shadowing_remote": false},
				muse,
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	read := func(muse map[string]any) AdapterState {
		t.Helper()
		manifests := answer(muse)
		code, out := runSh(t, probe, bin, nil, "TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base",
			"STUB_HOME="+home, "STUB_REAL_JQ="+realJQ, "STUB_MANIFESTS="+manifests,
			"STUB_JQ_ANSWER="+museProbeJQ(t, string(probe), manifests))
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
	override := func(extra map[string]any) map[string]any {
		m := map[string]any{"source": target, "source_kind": "local override"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	if st := read(nil); st.Facts["herdr_detection_rules"] != "absent" {
		t.Fatalf("a base with no rules file was read as %q", st.Facts["herdr_detection_rules"])
	}
	if err := os.WriteFile(target, rules, 0o644); err != nil {
		t.Fatal(err)
	}
	st := read(override(nil))
	if st.Facts["herdr_detection_rules"] != "installed" || st.Facts["herdr_detection_source"] != "local override" || len(st.Notes) != 0 {
		t.Fatalf("rules herdr loads were read as %v with %v", st.Facts, st.Notes)
	}
	// ⭐ THE REMOTE MANIFEST THE RULES WERE MEASURED AGAINST IS A FACT AND NO NOTE.
	st = read(override(map[string]any{"local_override_shadowing_remote": true, "cached_remote_version": "2026.08.26.1"}))
	if st.Facts["herdr_detection_shadows"] != "2026.08.26.1" || len(st.Notes) != 0 {
		t.Fatalf("rules shadowing the manifest they were measured against were read as %v with %v", st.Facts, st.Notes)
	}
	// ⛔ A NEWER ONE IS A NOTE NAMING IT, because herdr cannot deliver its fix past them.
	st = read(override(map[string]any{"local_override_shadowing_remote": true, "cached_remote_version": "2026.10.01.1"}))
	if st.Facts["herdr_detection_shadows"] != "2026.10.01.1" || !noted(st, "shadow herdr's Muse manifest 2026.10.01.1") || strings.Contains(strings.Join(st.Problems, " "), "shadow") {
		t.Fatalf("rules shadowing a newer manifest were read as %v with %v and %v", st.Facts, st.Notes, st.Problems)
	}
	// ⚠ A NOTE AND NOT A PROBLEM: base ensure writes the same file and cannot cure it.
	// The warning is herdr's own text, so a `|` in it reaches the note whole.
	st = read(map[string]any{"source": "bundled", "source_kind": "bundled", "warning": "ignored override | it could not be compiled"})
	if st.Facts["herdr_detection_source"] != "bundled" || !noted(st, "ignored override | it could not be compiled") || strings.Contains(strings.Join(st.Problems, " "), "rules") {
		t.Fatalf("rules herdr ignores were read as %v with %v and %v", st.Facts, st.Notes, st.Problems)
	}
	if st := read(nil); st.Facts["herdr_detection_rules"] != "installed" || st.Facts["herdr_detection_source"] != "" {
		t.Fatalf("with no herdr server answering, the rules were read as %v", st.Facts)
	}
	if err := os.WriteFile(target, []byte("id = \"muse\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := read(nil); st.Facts["herdr_detection_rules"] != "other" {
		t.Fatalf("a rules file of someone else's was read as %q", st.Facts["herdr_detection_rules"])
	}
}

// museProbeJQ answers what the muse probe's own jq program prints for herdr's answer,
// for a host whose account PATH has no jq. WSL-158.
//
// ⛔ IT READS THE PROGRAM FROM THE PROBE, and evaluates the one shape that program has:
// a path to a list, a select on the agent, and one interpolated line. A field the
// probe misnames reads null here as it does in jq. A program of another shape fails
// the case on every host, so this cannot drift from the probe in silence.
func museProbeJQ(t *testing.T, probe, answer string) string {
	t.Helper()
	m := regexp.MustCompile(`jq -r '([^']*select\(\.agent == "muse"\)[^']*)'`).FindStringSubmatch(probe)
	if m == nil {
		t.Fatal("the muse probe carries no jq program that selects muse")
	}
	shape := regexp.MustCompile(`^\.(\w+)\.(\w+)\[\]\? \| select\(\.(\w+) == "muse"\) \| "(.*)"$`).FindStringSubmatch(m[1])
	if shape == nil {
		t.Fatalf("the probe's jq program %q is not the shape this case evaluates", m[1])
	}
	if answer == "" {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(answer), &doc); err != nil {
		t.Fatal(err)
	}
	result, _ := doc[shape[1]].(map[string]any)
	list, _ := result[shape[2]].([]any)
	var lines []string
	for _, item := range list {
		obj, _ := item.(map[string]any)
		if obj[shape[3]] != "muse" {
			continue
		}
		// One `\(.FIELD)` or `\(.FIELD // ALTERNATIVE)`, as jq interpolates it: a missing
		// field is null, and `//` takes its right side for null and false alike.
		lines = append(lines, regexp.MustCompile(`\\\(([^)]*)\)`).ReplaceAllStringFunc(shape[4], func(part string) string {
			field, alt, hasAlt := strings.Cut(part[2:len(part)-1], " // ")
			if !strings.HasPrefix(field, ".") {
				t.Fatalf("the probe's jq program interpolates %q, which this case does not evaluate", part)
			}
			v, ok := obj[field[1:]]
			if (!ok || v == nil || v == false) && hasAlt {
				if s, err := strconv.Unquote(alt); err == nil {
					return s
				}
				if alt == "false" {
					return "false"
				}
				t.Fatalf("the probe's jq program falls back to %q, which this case does not evaluate", alt)
			}
			switch x := v.(type) {
			case nil:
				return "null"
			case string:
				return x
			case bool:
				return strconv.FormatBool(x)
			}
			t.Fatalf("herdr's answer carries %v for %s, which this case does not evaluate", v, field)
			return ""
		}))
	}
	return strings.Join(lines, "\n")
}

// TestTheMuseFootprintCountsHerdrsRules: a base that no longer names muse still holds
// the rules file it wrote for herdr, and `base status` counts it. A file of the
// operator's own at that path is not counted. WSL-158.
func TestTheMuseFootprintCountsHerdrsRules(t *testing.T) {
	needPosix(t, "sed", "grep", "head", "cut", "du")
	probe, err := adapterScript("muse", "probe.sh")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := adapterTree.ReadFile("adapters/muse/herdr-detection.toml")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	target := filepath.Join(home, ".config", "herdr", "agent-detection", "muse.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	bin, write := stubDir(t)
	write("getent", "printf 'herdr:x:1000:1000::%s:/bin/bash\\n' \"$STUB_HOME\"\n")
	// ⭐ The account reads whose file it is, and here it is this process, on the system's
	// PATH. ⛔ Root's head is found first on root's PATH, and records a read in the home.
	// WSL-201.
	write("runuser", "[ \"$1\" = -u ] && [ \"$2\" = herdr ] && [ \"$3\" = -- ] || exit 9\nshift 3\nexec \"$@\"\n")
	realHead, err := exec.LookPath("head")
	if err != nil {
		t.Fatal(err)
	}
	rootHead := filepath.Join(t.TempDir(), "root-head")
	write("head", "for a do case $a in \"$STUB_HOME\"/*) printf 'head %s\\n' \"$*\" >> "+shellQuote(rootHead)+" ;; esac; done\nexec "+shellQuote(realHead)+" \"$@\"\n")
	// ⚠ THE DIFFERENCE BETWEEN TWO READS, because the probe also counts
	// /usr/local/bin/muse, which a base this tool built may hold.
	footprint := func(body []byte) int64 {
		t.Helper()
		if err := os.WriteFile(target, body, 0o644); err != nil {
			t.Fatal(err)
		}
		code, out := runSh(t, probe, bin, nil, "TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base", "TK_PROBE_FOOTPRINT=1", "STUB_HOME="+home)
		if code != 0 {
			t.Fatalf("probe.sh exited %d:\n%s", code, out)
		}
		n, err := strconv.ParseInt(parseAdapterProbe("muse", out).Facts["footprint"], 10, 64)
		if err != nil {
			t.Fatalf("the footprint probe answered no footprint:\n%s", out)
		}
		return n
	}
	ours := footprint(rules)
	theirs := footprint(append([]byte("id = \"muse\"\n# the operator's own rules\n"), rules...))
	if ours <= theirs {
		t.Fatalf("the rules muse wrote for herdr count %d bytes, and a file of the operator's own %d; want only muse's counted", ours, theirs)
	}
	if b, _ := os.ReadFile(rootHead); len(b) != 0 {
		t.Fatalf("root read the rules in the account's home itself:\n%s", b)
	}
}
