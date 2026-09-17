// SPDX-License-Identifier: 0BSD

package checks

import (
	"strings"
	"testing"
)

// home assembles a Windows home path at run time. ⚠ Written literally, each
// fixture below would be a finding in this very file, because the rule reads
// every file in the tree and exempts only the one that holds its patterns.
func home(sep, name, rest string) string {
	return strings.Join([]string{"C:", "Users", name, rest}, sep)
}

// TestAWindowsHomePathIsFoundWithEitherSeparator is the case for a rule that
// could only ever see a forward slash. `[\/]` in a Go character class is an
// escaped `/`, so a path written the way Windows writes it passed the gate. The
// published tree carried a real username that way. TOOL-25.
func TestAWindowsHomePathIsFoundWithEitherSeparator(t *testing.T) {
	for _, s := range []string{
		home(`\`, "alice", "Downloads"),
		home("/", "alice", "Downloads"),
		`"disk_path": "` + home(`\\`, "alice", "AppData") + `"`,
	} {
		m := homePathRe.FindString(s)
		if m == "" || genericHome(m) {
			t.Errorf("%s was not reported as a private home path (match %q)", s, m)
		}
	}
}

// TestAGenericWindowsHomeIsNotAFingerprint holds the other direction, which is
// the one that decides whether anybody keeps the rule switched on.
func TestAGenericWindowsHomeIsNotAFingerprint(t *testing.T) {
	for _, s := range []string{
		home(`\`, "RUNNER~1", "AppData"),
		home(`\`, "runneradmin", "AppData"),
		home(`\`, "USER", "Downloads"),
		home(`\`, "...", "snapshots"),
		"/home/toolkit/.config",
	} {
		if m := homePathRe.FindString(s); m != "" && !genericHome(m) {
			t.Errorf("%s was reported as a private home path (match %q)", s, m)
		}
	}
}

// at builds an address from parts, so this file carries no literal one.
func at(local, domain string) string { return local + "@" + domain }

// TestASystemdUnitIsNotAnEmailAddress narrows a leak rule that fired on correct
// documentation.
//
// ⛔ IT REPORTED `user@1000.service` AS AN ADDRESS IN SEVEN PLACES the first time
// a document in this tree explained one, and the document was right. A rule that
// fires on correct content is a rule somebody switches off, which is how a real
// leak gets through later.
//
// ⚠ THE NEGATIVES MATTER MORE THAN THE POSITIVES HERE. This narrows a rule whose
// whole job is to catch a leak, so every case below that expects `false` is
// holding the narrowing to systemd's own suffixes and nothing wider.
func TestASystemdUnitIsNotAnEmailAddress(t *testing.T) {
	cases := []struct {
		in     string
		exempt bool
	}{
		{"user@1000.service", true},
		{"getty@tty1.service", true},
		{"podman@user.socket", true},
		{"blockdev@sda1.device", true},
		{"user@1000.slice", true},
		{"backup@daily.timer", true},

		// ⛔ STILL REPORTED. None of these is a unit, and the rule exists for them.
		//
		// ⚠ ASSEMBLED RATHER THAN WRITTEN OUT, and this file is NOT exempt from
		// the rule it tests. Exempting a test file from a leak check is exactly
		// where a real credential would hide, so the fixtures are built from
		// parts and the whole file stays under the rule.
		{at("somebody", "example.com"), false},
		{at("first.last", "corp.co.uk"), false},
		{at("ops", "internal.services"), false},
		{at("a", "b.servicedesk.io"), false},

		// ⭐ the two exemptions that were already here, unchanged
		{at("somebody", "users.noreply.github.com"), true},
		{at("noreply", "anything.com"), true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			m := emailRe.FindString(c.in)
			if m == "" {
				t.Fatalf("the address pattern did not match %q at all, so this case asserts nothing", c.in)
			}
			if got := exemptEmail(m); got != c.exempt {
				t.Fatalf("exemptEmail(%q) = %v, want %v", m, got, c.exempt)
			}
		})
	}
}
