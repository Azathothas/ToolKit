// check is the repository gate. One binary, one tree walk, no POSIX layer, so
// it runs natively wherever the toolchain does and needs no PowerShell twin.
//
// It replaced a set of shell scripts, and speed was half the reason: the
// marker check spawned one awk per file and did not finish in 400 seconds over
// 28,493 files. A gate that takes seven minutes is a gate people skip.
//
// Exit codes are the contract every caller reads:
//
//	0  it ran and agreed
//	1  it ran and disagreed
//	2  it could not run
//
// A skip is neither a pass nor a failure, and a check that quietly runs
// nothing and reports success is the worst answer this codebase can give.
//
// SPDX-License-Identifier: 0BSD
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Azathothas/ToolKit/tools/check/internal/checks"
)

type check struct {
	name   string
	schema string
	run    func(*checks.Tree) checks.Result
	what   string
}

var all = []check{
	{"docs", "check-docs/1", checks.Docs, "links resolve, cited paths exist, fenced blocks parse, no orphan page"},
	{"markers", "check-markers/1", checks.Markers, "only the five allowed characters, and not too many of them"},
	{"record", "check-record/1", checks.Record, "the work record agrees with itself"},
	{"one-home", "check-one-home/1", checks.OneHome, "no long sentence in two documents"},
	{"control-bytes", "check-control-bytes/1", checks.ControlBytes, "no literal control byte in a tracked text file"},
	{"placeholders", "check-placeholders/1", checks.Placeholders, "no template placeholder survived into a real file"},
	{"shell", "check-shell/1", checks.Shell, "every tracked shell script parses"},
	{"removals", "check-removals/1", checks.Removals, "no tracked script can delete the checkout, and the guard that stops it is intact"},
	{"line-endings", "check-line-endings/1", checks.LineEndings, "the index agrees with what .gitattributes resolves"},
	{"size", "check-size/1", checks.Size, "no tracked file the destination will refuse, and none that has grown toward it"},
	{"changelog", "check-changelog/1", checks.Changelog, "every entry dated, in order, with a record and a deploy line"},
	{"secrets", "check-no-secrets/1", checks.Secrets, "no credential, and no fingerprint of a private system"},
	{"shellcheck", "check-shellcheck/1", checks.Shellcheck, "shellcheck is clean over every tracked shell script"},
	{"powershell", "check-powershell/1", checks.PowerShell, "every tracked .ps1 parses and PSScriptAnalyzer is clean over scripts/"},
	{"bundle", "check-bundle/1", checks.Bundle, "the two generated products still match the parts that build them"},
	{"go", "check-go/1", checks.GoModules, "gofmt, vet, build and test over every Go module here"},
	{"commits", "check-commits/1", checks.Commits, "no commit credits a tool: no trailer, no generated-with line, no tool name, no emoji"},
	{"hooks", "check-hooks/1", checks.Hooks, "the commit-msg hook is tracked and this checkout is running it"},
}

func main() {
	args := os.Args[1:]
	asJSON := false
	var name, msgPath string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			usage()
			os.Exit(0)
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "check: unknown argument: %s\n", a)
			os.Exit(2)
		case name == "":
			name = a
		case msgPath == "":
			msgPath = a
		default:
			fmt.Fprintf(os.Stderr, "check: unexpected argument: %s\n", a)
			os.Exit(2)
		}
	}

	// ⛔ BEFORE THE TREE IS LOADED, because a commit-msg hook runs in the
	// middle of `git commit` and has no business walking 200 files to decide
	// whether one string carries a co-author trailer.
	if name == "commit-msg" {
		if msgPath == "" {
			fmt.Fprint(os.Stderr, "check: commit-msg needs the path of the message file\n")
			os.Exit(2)
		}
		res := checks.Message(msgPath)
		if asJSON {
			emit("check-commits/1", res)
		} else {
			report("commit-msg", res)
		}
		if res.Problems > 0 {
			os.Exit(1)
		}
		os.Exit(0)
	}

	tree, err := checks.Load(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "check: not a git repository (%v)\n", err)
		os.Exit(2)
	}

	if name == "" || name == "gate" {
		os.Exit(gate(tree, asJSON))
	}
	for _, c := range all {
		if c.name != name {
			continue
		}
		res := c.run(tree)
		if asJSON {
			emit(c.schema, res)
		} else {
			report(c.name, res)
		}
		if res.Problems > 0 {
			os.Exit(1)
		}
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "check: no such check: %s\n", name)
	usage()
	os.Exit(2)
}

func usage() {
	fmt.Fprint(os.Stderr, "usage: check [CHECK] [--json]\n       check commit-msg FILE\n\nWith no CHECK, runs every one and prints a verdict.\n\n")
	for _, c := range all {
		fmt.Fprintf(os.Stderr, "  %-15s %s\n", c.name, c.what)
	}
	fmt.Fprint(os.Stderr, "\n  commit-msg      the commits rule, applied to a message that is not a commit yet\n")
	fmt.Fprint(os.Stderr, "\nexit: 0 agreed, 1 disagreed, 2 could not run\n")
}

func emit(schema string, r checks.Result) {
	m := map[string]any{"schema": schema, "problems": r.Problems}
	for k, v := range r.Extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	fmt.Println(string(b))
}

func report(name string, r checks.Result) {
	if r.Problems == 0 {
		fmt.Printf("  ok     %s\n", name)
		return
	}
	sort.Strings(r.Detail)
	for _, d := range r.Detail {
		fmt.Printf("  FAIL   %s\n", d)
	}
	fmt.Printf("\n%s: %d problems\n", name, r.Problems)
}

func gate(t *checks.Tree, asJSON bool) int {
	type row struct {
		name string
		res  checks.Result
	}
	var rows []row
	total := 0
	for _, c := range all {
		res := c.run(t)
		rows = append(rows, row{c.name, res})
		total += res.Problems
	}
	if asJSON {
		out := map[string]any{"schema": "check-gate/1", "problems": total}
		for _, r := range rows {
			out[r.name] = r.res.Problems
		}
		b, _ := json.Marshal(out)
		fmt.Println(string(b))
		if total > 0 {
			return 1
		}
		return 0
	}
	for _, r := range rows {
		if r.res.Problems == 0 {
			fmt.Printf("  ok     %-15s\n", r.name)
			continue
		}
		fmt.Printf("  FAIL   %-15s %d\n", r.name, r.res.Problems)
		sort.Strings(r.res.Detail)
		for i, d := range r.res.Detail {
			if i == 12 {
				fmt.Printf("           ... and %d more\n", len(r.res.Detail)-12)
				break
			}
			fmt.Printf("           %s\n", d)
		}
	}
	fmt.Println()
	if total > 0 {
		fmt.Printf("VERDICT: %d problems.\n", total)
		return 1
	}
	fmt.Println("VERDICT: the tree agrees with itself.")
	fmt.Println("A green gate is not the finish line. It catches mechanical regressions;")
	fmt.Println("whether a claim is true is a reading, and that belongs to the review pass.")
	return 0
}
