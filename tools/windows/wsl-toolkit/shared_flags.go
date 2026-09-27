// SPDX-License-Identifier: 0BSD

package main

import (
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
)

// sharedFlags is a command group whose subcommands share one flag set. bind
// defines the whole set on fs, bound to f. reads names the flags each
// subcommand reads, keyed by the subcommand as it is typed, and "" is the group
// run with no subcommand. order is the order a refusal names the readers in.
type sharedFlags[T any] struct {
	group string
	order []string
	reads map[string][]string
	bind  func(f *T, fs *flag.FlagSet)
}

// command is the name a subcommand is typed and registered as.
func (s sharedFlags[T]) command(sub string) string {
	return strings.TrimSpace(s.group + " " + sub)
}

// parse parses one subcommand's arguments into f.
//
// ⛔ A FLAG A SUBCOMMAND DOES NOT READ IS REFUSED, NOT IGNORED, and the refusal
// names the subcommands that read it. A flag that parses and does nothing is the
// dead-configuration row of docs/conventions/forbidden-patterns.md. WSL-100.
//
// ⛔ THE SUBCOMMAND'S OWN SET HOLDS ONLY WHAT IT READS, because that set is what
// its --help prints and what the manual is generated from. `base remove --help`
// listed all nine flags of the base set, and the subcommand refuses seven of
// them. WSL-104.
//
// A subcommand that reads does not name parses the whole set, and the group's
// dispatch then refuses the subcommand itself.
func (s sharedFlags[T]) parse(sub string, f *T, args []string) error {
	name := s.command(sub)
	fs := newFlagSet(name)
	read, known := s.reads[sub]
	if !known {
		s.bind(f, fs)
		return parseArgs(fs, args)
	}
	// The whole set, bound to values nothing reads, is parsed only to find a
	// flag this subcommand does not read. An argument the whole set cannot
	// parse is left to the subcommand's own parse, which reports it.
	all := flag.NewFlagSet(name, flag.ContinueOnError)
	all.SetOutput(io.Discard)
	s.bind(new(T), all)
	if all.Parse(args) == nil {
		var refused error
		all.Visit(func(fl *flag.Flag) {
			if refused != nil || slices.Contains(read, fl.Name) {
				return
			}
			var readers []string
			for _, o := range s.order {
				if slices.Contains(s.reads[o], fl.Name) {
					readers = append(readers, s.command(o))
				}
			}
			refused = fmt.Errorf("%s does not read --%s, so it is refused rather than ignored. It is read by %s", name, fl.Name, strings.Join(readers, ", "))
		})
		if refused != nil {
			return refused
		}
	}
	whole := flag.NewFlagSet(name, flag.ContinueOnError)
	s.bind(f, whole)
	whole.VisitAll(func(fl *flag.Flag) {
		if slices.Contains(read, fl.Name) {
			fs.Var(fl.Value, fl.Name, fl.Usage)
		}
	})
	return parseArgs(fs, args)
}
