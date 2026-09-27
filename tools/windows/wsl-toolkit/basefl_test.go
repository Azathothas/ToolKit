// SPDX-License-Identifier: 0BSD

package main

import (
	"flag"
	"slices"
	"strings"
	"testing"
)

// TestEveryBaseSubcommandRefusesAFlagItDoesNotRead walks every subcommand that
// parses the shared set against every flag in it, through the parse cmdBase
// runs. WSL-100.
func TestEveryBaseSubcommandRefusesAFlagItDoesNotRead(t *testing.T) {
	if _, err := parseBaseFlags("status", nil); err != nil {
		t.Fatal(err)
	}
	flagSets.Lock()
	set := flagSets.byName["base status"]
	flagSets.Unlock()
	var names []string
	set.VisitAll(func(f *flag.Flag) { names = append(names, f.Name) })
	if len(names) < 9 {
		t.Fatalf("the shared set has %d flags; the walk expects every one of them", len(names))
	}
	for _, name := range names {
		read := false
		for _, sub := range baseSubcommands {
			read = read || slices.Contains(baseFlagsRead[sub], name)
		}
		if !read {
			t.Errorf("--%s is in the shared set and no subcommand reads it", name)
		}
	}
	for _, sub := range baseSubcommands {
		for _, name := range names {
			args := []string{"--" + name}
			switch name {
			case "preset":
				args = []string{"--preset", "arch"}
			case "save":
				args = []string{"--save", "--preset", "arch"}
			}
			_, err := parseBaseFlags(sub, args)
			if slices.Contains(baseFlagsRead[sub], name) {
				if err != nil {
					t.Errorf("base %s refused --%s, which it reads: %v", sub, name, err)
				}
				continue
			}
			if err == nil || !strings.Contains(err.Error(), "does not read --") {
				t.Errorf("base %s accepted --%s, which it does not read: %v", sub, name, err)
			}
		}
	}
}

func TestAConsumersEnsureProbeStaysAcceptedAndSaveNeedsAPreset(t *testing.T) {
	if _, err := parseBaseFlags("ensure", []string{"--probe"}); err != nil {
		t.Errorf("base ensure --probe is refused, and a consumer passes it: %v", err)
	}
	if _, err := parseBaseFlags("ensure", []string{"--save"}); err == nil || !strings.Contains(err.Error(), "no --preset") {
		t.Errorf("--save with no --preset answered %v", err)
	}
}
