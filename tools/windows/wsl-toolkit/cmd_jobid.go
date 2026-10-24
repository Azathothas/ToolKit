// SPDX-License-Identifier: 0BSD

package main

import (
	"errors"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// resolveJobID reads the job a command was given, in whichever form the caller
// holds it: the id, its container's name, or a unique prefix of either. sub
// and args are the command as it was typed, so a refusal can print it again
// under the instance that ran the job. WSL-120.
//
// ⛔ EVERY COMMAND THAT TAKES A JOB CALLS THIS BEFORE IT READS ANYTHING ELSE, so
// `logs`, `wait`, `stop`, `inspect`, `artifacts retry`, `gc --job` and
// `resources --job` cannot disagree about what one string names.
func resolveJobID(sub string, args []string, raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	home, err := toolkit.Home()
	if err != nil {
		return "", err
	}
	return toolkit.ResolveJobID(home, raw, func(instance, id string) string {
		return toolkit.ToolCommandFor(instance, sub+" "+commandWords(withJobID(args, raw, id)))
	})
}

// resolveRoutedJobID is resolveJobID for a command that may go through the
// helper. A prefix nothing in this state directory starts is answered as
// itself, with the refusal the direct route gives it in notHere, so the helper
// can read it among its own jobs. WSL-120.
func resolveRoutedJobID(sub string, args []string, raw string) (id string, notHere, err error) {
	id, err = resolveJobID(sub, args, raw)
	if errors.Is(err, toolkit.ErrNoJobHere) {
		return raw, err, nil
	}
	return id, nil, err
}

// withJobID is a command's arguments with the id a caller passed replaced by
// the job's full id, as a value of its own or after a flag's `=`.
func withJobID(args []string, raw, id string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		switch {
		case a == raw:
			out[i] = id
		case strings.HasPrefix(a, "-") && strings.HasSuffix(a, "="+raw):
			out[i] = strings.TrimSuffix(a, raw) + id
		default:
			out[i] = a
		}
	}
	return out
}

// commandWords joins arguments into one line a shell reads back as the same
// words. A word a shell would split or expand is single-quoted, with a quote
// inside it doubled, which is PowerShell's spelling.
func commandWords(args []string) string {
	words := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"'`$&|;<>()*?[]{}\\,") {
			a = "'" + strings.ReplaceAll(a, "'", "''") + "'"
		}
		words[i] = a
	}
	return strings.Join(words, " ")
}
