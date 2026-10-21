// changelog_test.go - a changelog stamp later than the clock is refused.
//
// ⛔ THE CASE IS THE INCIDENT: two of five stamps typed on 2026-09-17 were in the
// future, and every other rule passed them. TOOL-36.
//
// SPDX-License-Identifier: 0BSD

package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAChangelogStampLaterThanTheClockIsRefused(t *testing.T) {
	now := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	saved := changelogNow
	changelogNow = func() time.Time { return now }
	t.Cleanup(func() { changelogNow = saved })

	entry := func(stamp string) string {
		return "### " + stamp + " - a change\n\nRecord: `TOOL-36`. No deploy.\n\n"
	}
	cases := []struct {
		name  string
		stamp string
		want  int
	}{
		{name: "a stamp read from the clock", stamp: "2026-10-21T08:59:00Z", want: 0},
		{name: "a stamp inside the skew", stamp: "2026-10-21T09:05:00Z", want: 0},
		{name: "a stamp an hour ahead", stamp: "2026-10-21T10:00:00Z", want: 1},
		{name: "today's date alone", stamp: "2026-10-21", want: 0},
		{name: "tomorrow's date alone", stamp: "2026-10-22", want: 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, ChangelogFile), []byte("# Changelog\n\n## Unreleased\n\n"+entry(c.stamp)), 0o644); err != nil {
				t.Fatalf("could not write the changelog: %v", err)
			}
			r := Changelog(&Tree{Root: root, Files: []string{ChangelogFile}, cache: map[string][]byte{}})
			if r.Problems != c.want {
				t.Fatalf("problems = %d, want %d: %s", r.Problems, c.want, strings.Join(r.Detail, " | "))
			}
			if c.want > 0 && !strings.Contains(strings.Join(r.Detail, " | "), "later than the clock") {
				t.Fatalf("the finding does not say the stamp is late: %s", strings.Join(r.Detail, " | "))
			}
		})
	}
}
