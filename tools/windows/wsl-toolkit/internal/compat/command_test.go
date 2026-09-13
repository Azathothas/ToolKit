// SPDX-License-Identifier: 0BSD

package compat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- the command channel -------------------------------------------------------

func TestResolveCommandBytesCollapsesTheThreeSpellings(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cmd.sh")
	if err := os.WriteFile(file, []byte("from a file\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := sessionFor(t, "-Action", "Run", "-Name", "d", "-Command", "echo hi")
	body, err := s.resolveCommandBytes()
	if err != nil || string(body) != "echo hi" {
		t.Errorf("-Command gave %q, %v", body, err)
	}

	s = sessionFor(t, "-Action", "Run", "-Name", "d", "-CommandFile", file)
	body, err = s.resolveCommandBytes()
	if err != nil || string(body) != "from a file\n" {
		t.Errorf("-CommandFile gave %q, %v", body, err)
	}

	s = sessionFor(t, "-Action", "Run", "-Name", "d", "-CommandB64", "ZWNobyBiNjQ=")
	body, err = s.resolveCommandBytes()
	if err != nil || string(body) != "echo b64" {
		t.Errorf("-CommandB64 gave %q, %v", body, err)
	}
}

func TestResolveCommandBytesRefusesTwoSpellings(t *testing.T) {
	s := sessionFor(t, "-Action", "Run", "-Name", "d", "-Command", "echo hi", "-CommandB64", "YQ==")
	if _, err := s.resolveCommandBytes(); err == nil || !strings.Contains(err.Error(), "only one of") {
		t.Errorf("two spellings of one argument were accepted: %v", err)
	}
}

func TestResolveCommandBytesNilMeansNoCommand(t *testing.T) {
	// New with no -Command is a distro that gets created and kept, so "no
	// command" must stay distinguishable from "an empty command".
	s := sessionFor(t, "-Action", "New", "-Image", "alpine:3.22")
	body, err := s.resolveCommandBytes()
	if err != nil || body != nil {
		t.Errorf("no command gave %q, %v; nil is the contract", body, err)
	}
}

func TestScriptArgWithNoCommandIsRefused(t *testing.T) {
	// ⛔ REFUSED RATHER THAN IGNORED. A caller who passed -ScriptArg and no
	// command has made a mistake this tool can see.
	s := sessionFor(t, "-Action", "New", "-Image", "alpine:3.22", "-ScriptArg", "A=1")
	if _, err := s.resolveCommandBytes(); err == nil || !strings.Contains(err.Error(), "no command to pass it to") {
		t.Errorf("-ScriptArg with no command was accepted: %v", err)
	}
}

func TestCommandFileRepairMatchesTheScript(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "payload.sh")

	// CRLF in the COPY IN TRANSIT becomes LF; the file on disk is never
	// written to.
	if err := os.WriteFile(file, []byte("one\r\ntwo\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := sessionFor(t, "-Action", "Run", "-Name", "d", "-CommandFile", file)
	body, err := s.resolveCommandBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "one\ntwo\n" {
		t.Errorf("the copy still carries CRLF: %q", body)
	}
	onDisk, _ := os.ReadFile(file)
	if string(onDisk) != "one\r\ntwo\r\n" {
		t.Errorf("THE FILE ON DISK WAS WRITTEN TO: %q", onDisk)
	}

	// A UTF-8 byte order mark is removed from the copy: it is three bytes
	// ahead of the first line, so a leading set -e is not a command.
	if err := os.WriteFile(file, append([]byte{0xEF, 0xBB, 0xBF}, []byte("set -e\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	s = sessionFor(t, "-Action", "Run", "-Name", "d", "-CommandFile", file)
	body, err = s.resolveCommandBytes()
	if err != nil || string(body) != "set -e\n" {
		t.Errorf("the byte order mark survived: %q, %v", body, err)
	}

	// UTF-16 is REFUSED by name: /bin/sh stops at the first NUL and the
	// command would do nothing and say nothing.
	if err := os.WriteFile(file, append([]byte{0xFF, 0xFE}, []byte("a\x00b\x00")...), 0o600); err != nil {
		t.Fatal(err)
	}
	s = sessionFor(t, "-Action", "Run", "-Name", "d", "-CommandFile", file)
	if _, err := s.resolveCommandBytes(); err == nil || !strings.Contains(err.Error(), "UTF-16") {
		t.Errorf("UTF-16 bytes were accepted: %v", err)
	}

	// -Verbatim sends the bytes exactly, and the UTF-16 refusal becomes a
	// warning, because refusing bytes somebody explicitly asked to send would
	// be the tool overriding them.
	s = sessionFor(t, "-Action", "Run", "-Name", "d", "-CommandFile", file, "-Verbatim")
	body, err = s.resolveCommandBytes()
	if err != nil || len(body) == 0 {
		t.Errorf("-Verbatim refused the caller's own bytes: %v", err)
	}
}

func TestVerbatimAppliesOnlyToCommandFile(t *testing.T) {
	s := sessionFor(t, "-Action", "Run", "-Name", "d", "-Command", "echo hi", "-Verbatim")
	if _, err := s.resolveCommandBytes(); err == nil || !strings.Contains(err.Error(), "-Verbatim applies to -CommandFile") {
		t.Errorf("-Verbatim beside -Command was accepted: %v", err)
	}
}

func TestScriptArgPrologueAssignsAndNeverSubstitutes(t *testing.T) {
	s := sessionFor(t, "-Action", "Run", "-Name", "d", "-Command", "true")
	prologue, err := s.scriptArgPrologue([]string{"URL=http://x/?a=1&b=2", "EMPTY="})
	if err != nil {
		t.Fatal(err)
	}
	// The value carries a quote-breaker and nothing in the caller's bytes is
	// rewritten: it is assigned, single-quoted.
	want := "URL='http://x/?a=1&b=2'\nexport URL\nEMPTY=''\nexport EMPTY\n"
	if prologue != want {
		t.Errorf("prologue = %q, want %q", prologue, want)
	}
}

func TestScriptArgRefusesNamesThatAreNotIdentifiers(t *testing.T) {
	s := sessionFor(t, "-Action", "Run", "-Name", "d", "-Command", "true", "-ScriptArg", "BAD-NAME=x")
	if _, err := s.resolveCommandBytes(); err == nil || !strings.Contains(err.Error(), "BAD-NAME") {
		t.Errorf("a non-identifier name was accepted: %v", err)
	}
}

func TestScriptArgFileIsReadRepairedAndAppliedFirst(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "args")
	if err := os.WriteFile(file, []byte("# a comment\r\n\r\nFROM_FILE=1\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := sessionFor(t, "-Action", "Run", "-Name", "d", "-Command", "true", "-ScriptArgFile", file, "-ScriptArg", "FROM_FLAG=2")
	body, err := s.resolveCommandBytes()
	if err != nil {
		t.Fatal(err)
	}
	want := "FROM_FILE='1'\nexport FROM_FILE\nFROM_FLAG='2'\nexport FROM_FLAG\ntrue"
	if string(body) != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestScriptArgFileRefusesJunkLinesByName(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "args")
	if err := os.WriteFile(file, []byte("NOT_A_PAIR\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := sessionFor(t, "-Action", "Run", "-Name", "d", "-Command", "true", "-ScriptArgFile", file)
	if _, err := s.resolveCommandBytes(); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Errorf("a junk line was accepted: %v", err)
	}
}

// -- the transport skeleton ------------------------------------------------------

func TestTheTransportSkeletonCarriesThePayload(t *testing.T) {
	line, err := distroScriptCommand([]byte("echo $HOME; echo `x`"), "/tmp/.wsl-eph-test")
	if err != nil {
		t.Fatal(err)
	}
	// The payload travels as base64, so a dollar sign, a backtick and a double
	// quote arrive byte-exact instead of being re-parsed in transit.
	if !strings.Contains(line, "|base64 -d>&8&&. /dev/fd/9") {
		t.Errorf("the skeleton is not the measured one: %s", line)
	}
	if strings.Contains(line, "$HOME") || strings.Contains(line, "`x`") {
		t.Errorf("raw payload bytes reached the command line: %s", line)
	}
}

// TestTheTransportSkeletonRefusesAnUnmeasuredCharacter is the mutation case
// for the alphabet check: plant a character the measurement never cleared and
// the builder must refuse, because the check catches edits to the SKELETON,
// not the payload.
func TestTheTransportSkeletonRefusesAnUnmeasuredCharacter(t *testing.T) {
	if _, err := distroScriptCommand([]byte("x"), "/tmp/.wsl-eph-test"); err != nil {
		t.Fatalf("the measured skeleton was refused: %v", err)
	}
	_, err := distroScriptCommand([]byte("x"), "/tmp/.wsl-eph-te$t")
	if err == nil || !strings.Contains(err.Error(), "outside the alphabet") {
		t.Errorf("a guest path outside the alphabet was accepted: %v", err)
	}
	if _, err := distroScriptCommand([]byte("x"), "relative/path"); err == nil {
		t.Error("a relative transport path was accepted")
	}
}

func TestGuestScratchPathsAreUniqueAndInsideTheAlphabet(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		p := guestScratchPath()
		if !transportPathShape.MatchString(p) {
			t.Fatalf("%s is outside the path alphabet", p)
		}
		if seen[p] {
			t.Fatalf("%s was drawn twice; two concurrent Runs would write each other's file", p)
		}
		seen[p] = true
	}
}

// -- the guest environment prologue ---------------------------------------------

func TestTheUserEnvProloguePrependsAndNeverEdits(t *testing.T) {
	payload := []byte("echo one")
	combined := addGuestUserEnvironment(payload)
	if !strings.HasPrefix(string(combined), guestUserEnvironmentPrelude()+"\n") {
		t.Error("the prologue is not the prefix")
	}
	if !strings.HasSuffix(string(combined), "echo one") {
		t.Errorf("THE CALLER'S BYTES WERE EDITED: %q", combined)
	}
	if !strings.Contains(guestUserEnvironmentPrelude(), "XDG_RUNTIME_DIR=$_wtk_run; export XDG_RUNTIME_DIR") {
		t.Error("the prologue no longer sets XDG_RUNTIME_DIR, which is the defect it exists for")
	}
}

func TestShellSingleQuotingSurvivesAQuote(t *testing.T) {
	got := shellSingleQuoted("it's")
	if got != `'it'\''s'` {
		t.Errorf("shellSingleQuoted = %q", got)
	}
}
