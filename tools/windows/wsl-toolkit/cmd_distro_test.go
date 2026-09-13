// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

func TestTheThreeSpellingsOfACommandAreExclusiveAndEachIsRepaired(t *testing.T) {
	c := commandFlags{command: "true", commandB64: base64.StdEncoding.EncodeToString([]byte("true\n"))}
	if _, err := c.payload(toolkit.Config{}); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("two spellings: %v", err)
	}
	c = commandFlags{commandB64: base64.StdEncoding.EncodeToString([]byte("printf 'ok'\r\n"))}
	p, err := c.payload(toolkit.Config{})
	if err != nil || string(p.script) != "printf 'ok'\n" {
		t.Fatalf("base64 payload = %q, %v", p.script, err)
	}
	c = commandFlags{commandB64: "not-base64!"}
	if _, err := c.payload(toolkit.Config{}); err == nil || !strings.Contains(err.Error(), "not valid base64") {
		t.Fatalf("invalid base64: %v", err)
	}
	c = commandFlags{}
	if p, err := c.payload(toolkit.Config{}); err != nil || p.script != nil {
		t.Fatalf("no command is nil, not an empty one: %q, %v", p.script, err)
	}
}

func TestVerbatimSendsTheBytesAndStillRefusesWhatTheChannelCannotCarry(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(file, []byte("\xEF\xBB\xBFa\r\nb\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := commandFlags{scriptFile: file, verbatim: true}
	p, err := c.payload(toolkit.Config{})
	if err != nil || string(p.script) != "\xEF\xBB\xBFa\r\nb\r\n" {
		t.Fatalf("verbatim = %q, %v", p.script, err)
	}
	if err := os.WriteFile(file, []byte{0xFF, 0xFE, 'e', 0, 'c', 0}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.payload(toolkit.Config{}); err == nil || !strings.Contains(err.Error(), "UTF-16") {
		t.Fatalf("a UTF-16 file was sent verbatim: %v", err)
	}
	for label, flags := range map[string]commandFlags{
		"verbatim with no command": {verbatim: true},
		"env with no command":      {env: stringList{"A=1"}},
		"env-file with no command": {envFile: file},
		"user-env with no command": {userEnv: true},
	} {
		if _, err := flags.payload(toolkit.Config{}); err == nil || !strings.Contains(err.Error(), "no command") {
			t.Errorf("%s: %v", label, err)
		}
	}
}

func TestTheEnvFileAndTheFlagsLandInOneOrderedList(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "pairs.env")
	if err := os.WriteFile(file, []byte("# comment\r\nA=1\r\nB=2\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := commandFlags{command: "true", envFile: file, env: stringList{"C=3", "A=override"}}
	p, err := c.payload(toolkit.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range p.env {
		got = append(got, e.Name+"="+e.Value)
	}
	if strings.Join(got, "|") != "A=1|B=2|C=3|A=override" {
		t.Fatalf("pairs = %q", got)
	}
	c = commandFlags{command: "true", envFile: filepath.Join(dir, "missing.env")}
	if _, err := c.payload(toolkit.Config{}); err == nil {
		t.Fatal("a missing env file was read as empty")
	}
}

func TestHostAddressExpandsOnlyInsideValuesAndIsResolvedOnce(t *testing.T) {
	calls := 0
	resolve := func() (toolkit.HostAddress, error) {
		calls++
		return toolkit.HostAddress{Address: "172.20.1.1"}, nil
	}
	env := []toolkit.EnvPair{{Name: "URL", Value: "https://@hostaddress:443/"}, {Name: "PLAIN", Value: "none"}, {Name: "B", Value: "@hostaddress"}}
	if err := expandHostAddress(env, resolve); err != nil {
		t.Fatal(err)
	}
	if env[0].Value != "https://172.20.1.1:443/" || env[1].Value != "none" || env[2].Value != "172.20.1.1" || calls != 1 {
		t.Fatalf("env %+v after %d resolution(s)", env, calls)
	}
	calls = 0
	plain := toolkit.EnvPair{Name: "A", Value: "x"}
	if err := expandHostAddress([]toolkit.EnvPair{plain}, resolve); err != nil || calls != 0 {
		t.Fatalf("an environment without the token asked for the address %d time(s): %v", calls, err)
	}
	failing := func() (toolkit.HostAddress, error) { return toolkit.HostAddress{}, errors.New("no adapter") }
	token := toolkit.EnvPair{Name: "A", Value: "@hostaddress"}
	if err := expandHostAddress([]toolkit.EnvPair{token}, failing); err == nil {
		t.Fatal("an address that could not be resolved was left as the literal token")
	}
}

func TestANameGetsThePrefixAndNothingElse(t *testing.T) {
	if got, err := targetName("build-box"); err != nil || got != "eph-build-box" {
		t.Fatalf("targetName = %q, %v", got, err)
	}
	for _, bad := range []string{"", " ", "Build-Box", "eph-a/b"} {
		if _, err := targetName(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestLogValidationForADryRunOpensNothing(t *testing.T) {
	dir := t.TempDir()
	events := filepath.Join(dir, "not-created", "events.jsonl")
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	var l logFlags
	l.bind(fs)
	if err := fs.Parse([]string{"--event-log", events, "--log-profile", "ci"}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.settings(toolkit.Config{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(events)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resolving the settings created the sink's directory: %v", err)
	}
	fs = flag.NewFlagSet("x", flag.ContinueOnError)
	l = logFlags{}
	l.bind(fs)
	if err := fs.Parse([]string{"--stream-log", "logs/NUL.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.settings(toolkit.Config{}); err == nil || !strings.Contains(err.Error(), "device NUL") {
		t.Fatalf("a device name was accepted as a log path: %v", err)
	}
}

func TestAnExplicitFlagBesideAProfileIsReadAsExplicit(t *testing.T) {
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	var l logFlags
	l.bind(fs)
	if err := fs.Parse([]string{"--log-profile", "ci", "--color", "always", "--tick", "0"}); err != nil {
		t.Fatal(err)
	}
	s, err := l.settings(toolkit.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Color || s.Tick != 0 {
		t.Fatalf("colour %v tick %s: the profile overruled what was typed", s.Color, s.Tick)
	}
}

// TestADryRunPlanCarriesNoCommandTextAndNoEnvironmentValue guards the reason a
// plan names sizes, a digest and variable names rather than content.
func TestADryRunPlanCarriesNoCommandTextAndNoEnvironmentValue(t *testing.T) {
	secret := toolkit.EnvPair{Name: "TOKEN", Value: "hunter2-in-a-value"}
	spec := toolkit.ThrowawaySpec{User: "root", Script: []byte("echo hunter2-in-the-command\n"),
		Env: []toolkit.EnvPair{secret}, Timeout: time.Minute}
	plan := newDistroPlan("run", "eph-test")
	plan.addCommand("eph-test", spec, commandPayload{script: spec.Script, env: spec.Env, source: "the -c command"}, toolkit.LogSettings{})
	var b strings.Builder
	for _, s := range plan.Steps {
		b.WriteString(s)
	}
	b.WriteString(plan.Command.Invocation + strings.Join(plan.Command.EnvNames, ","))
	if strings.Contains(b.String(), "hunter2") {
		t.Fatalf("the plan repeats a command or a value: %s", b.String())
	}
	if plan.Command.Bytes == 0 || len(plan.Command.SHA256) != 64 || plan.Command.EnvNames[0] != "TOKEN" || plan.Command.Stdin != "/dev/null" {
		t.Fatalf("the plan does not identify what would run: %+v", plan.Command)
	}
}

func TestPurgeRefusesDryRunBesideApply(t *testing.T) {
	// ⛔ AN EMPTY STATE DIRECTORY, so a purge that got past the refusal, which is
	// exactly what the mutation of this guard produces, has nothing to remove.
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	code, err := cmdDistroPurge(context.Background(), []string{"--dry-run", "--apply"})
	if code != exitCannot || err == nil || !strings.Contains(err.Error(), "opposite") {
		t.Fatalf("code %d, %v", code, err)
	}
}
