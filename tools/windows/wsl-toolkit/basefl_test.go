// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"flag"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestEveryBaseSubcommandRefusesAFlagItDoesNotRead walks every subcommand that
// parses the shared set against every flag in it, through the parse cmdBase
// runs. WSL-100.
func TestEveryBaseSubcommandRefusesAFlagItDoesNotRead(t *testing.T) {
	all := flag.NewFlagSet("base", flag.ContinueOnError)
	new(baseFlags).bind(all)
	var names []string
	all.VisitAll(func(f *flag.Flag) { names = append(names, f.Name) })
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

// sharedGroup is one group whose subcommands share a flag set, seen through
// what a test needs of it.
type sharedGroup struct {
	whole func() *flag.FlagSet
	parse func(sub string, args []string) error
	order []string
	reads map[string][]string
	name  func(sub string) string
}

func groupOf[T any](s sharedFlags[T]) sharedGroup {
	return sharedGroup{
		whole: func() *flag.FlagSet {
			fs := flag.NewFlagSet(s.group, flag.ContinueOnError)
			s.bind(new(T), fs)
			return fs
		},
		parse: func(sub string, args []string) error { return s.parse(sub, new(T), args) },
		order: s.order,
		reads: s.reads,
		name:  s.command,
	}
}

// TestEverySharedSetPublishesAndAcceptsOnlyWhatEachSubcommandReads holds every
// set that several subcommands share to one table written here, apart from the
// tables the code reads. The set a subcommand registers is what
// its --help prints and what the manual is generated from, so a flag in it that
// the subcommand refuses sends a reader into the refusal. WSL-100, WSL-104.
func TestEverySharedSetPublishesAndAcceptsOnlyWhatEachSubcommandReads(t *testing.T) {
	want := map[string][]string{
		"base status":     {"json", "probe", "via-helper"},
		"base ensure":     {"json", "preset", "probe", "repair", "save", "via-helper"},
		"base recreate":   {"json", "preset", "probe", "repair", "save", "via-helper"},
		"base remove":     {"via-helper", "yes"},
		"base shell":      {"here", "root", "via-helper"},
		"base attach":     {"json"},
		"base grant":      {"json", "mode", "source", "target", "via-helper"},
		"base revoke":     {"json", "target", "via-helper"},
		"helper serve":    {"detach", "json"},
		"helper status":   {"json"},
		"helper stop":     nil,
		"config":          {"effective", "json", "write"},
		"config validate": {"json", "path"},
	}
	groups := []sharedGroup{groupOf(baseShared), groupOf(grantShared("grant")), groupOf(helperShared), groupOf(configShared)}
	seen := 0
	for _, g := range groups {
		var names []string
		g.whole().VisitAll(func(f *flag.Flag) { names = append(names, f.Name) })
		for _, sub := range g.order {
			command := g.name(sub)
			seen++
			if err := g.parse(sub, nil); err != nil {
				t.Fatalf("%s with no arguments: %v", command, err)
			}
			flagSets.Lock()
			set := flagSets.byName[command]
			flagSets.Unlock()
			var published []string
			set.VisitAll(func(f *flag.Flag) { published = append(published, f.Name) })
			if !slices.Equal(published, want[command]) {
				t.Errorf("%s --help and the manual name %v, and it reads %v", command, published, want[command])
			}
			for _, name := range names {
				args := []string{"--" + name}
				if fl := g.whole().Lookup(name); !isBoolFlag(fl) {
					args = append(args, "x")
				}
				err := g.parse(sub, args)
				if slices.Contains(want[command], name) {
					if err != nil && strings.Contains(err.Error(), "does not read") {
						t.Errorf("%s refused --%s, which it reads: %v", command, name, err)
					}
					continue
				}
				if err == nil || !strings.Contains(err.Error(), command+" does not read --"+name+",") {
					t.Errorf("%s answered --%s, which it does not read, with %v", command, name, err)
					continue
				}
				for _, o := range g.order {
					other := g.name(o)
					if other != command && slices.Contains(want[other], name) && !strings.Contains(err.Error(), other) {
						t.Errorf("%s refused --%s without naming %s, which reads it: %v", command, name, other, err)
					}
				}
			}
		}
	}
	if seen != len(want) {
		t.Fatalf("the groups have %d subcommands and the table names %d", seen, len(want))
	}
}

func isBoolFlag(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// TestAForegroundHelperRefusesJSON: serve reads --json with --detach alone,
// because a helper in the foreground writes its log and no answer. WSL-104.
//
// ⚠ WITHOUT THE REFUSAL THIS CALL STARTS A HELPER, so it runs in a state
// directory of its own, on a context that is already over, and it gives up
// rather than waiting on a server.
func TestAForegroundHelperRefusesJSON(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	type answer struct {
		code int
		err  error
	}
	done := make(chan answer, 1)
	go func() {
		code, err := cmdHelper(ctx, []string{"serve", "--json"})
		done <- answer{code, err}
	}()
	select {
	case a := <-done:
		if a.code != exitCannot || a.err == nil || !strings.Contains(a.err.Error(), "--json with --detach alone") {
			t.Fatalf("helper serve --json answered %d, %v", a.code, a.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("helper serve --json started a helper rather than refusing")
	}
	if code, err := cmdHelper(ctx, []string{"stop", "--json"}); code != exitCannot || err == nil || errors.Is(err, flag.ErrHelp) {
		t.Fatalf("helper stop --json answered %d, %v", code, err)
	}
}
