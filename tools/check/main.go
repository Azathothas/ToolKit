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
	"io"
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
	{"ste", "check-ste/1", checks.STE, "the countable half of ASD-STE100 over the documents a reader follows"},
	{"skills", "check-skills/1", checks.Skills, "every skill stands alone and names only commands the tool really has"},
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
	{"consumer", "check-consumer/1", checks.Consumer, "consumer.ps1 drives a binary built from this tree, so a refusal is caught before a tag"},
	{"package-table", "check-package-table/1", checks.PackageTable, "the base provisioner's copy of the shared package table matches bootstrap.sh"},
	{"adapters", "check-adapters/1", checks.Adapters, "the executable's copy of every wsl-toolkit adapter matches its definition"},
	{"shipped", "check-shipped/1", checks.Shipped, "the executable's copy of every file it carries matches the one this repository publishes"},
	{"go", "check-go/1", checks.GoModules, "gofmt, vet, build and test over every Go module here"},
	{"mutations", "check-mutations/1", checks.Mutations, "every row of the mutation table still reaches the guard it names"},
	{"commits", "check-commits/1", checks.Commits, "no commit credits a tool: no trailer, no generated-with line, no tool name, no emoji"},
	{"hooks", "check-hooks/1", checks.Hooks, "the commit-msg hook is tracked and this checkout is running it"},
}

func main() {
	args := os.Args[1:]
	asJSON := false
	fix := false
	var name, msgPath string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "--fix":
			fix = true
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
			report(os.Stdout, "commit-msg", res)
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

	// ⛔ A CHECK IS READ ONLY UNLESS IT IS ASKED TO FIX, and only a check that has
	// a fix accepts the flag. scripts/README.md's contract, point 5. A gate run
	// never writes: `--fix` with no check named is refused rather than applied to
	// whatever happens to support it.
	if fix {
		switch name {
		case "package-table":
			dest, err := checks.WritePackageTable(tree.Root)
			if err != nil {
				fmt.Fprintf(os.Stderr, "check: the package table copy could not be written: %v\n", err)
				os.Exit(2)
			}
			fmt.Fprintf(os.Stderr, "check: wrote %s\n", dest)
		case "adapters":
			wrote, err := checks.WriteAdapters(tree.Root)
			if err != nil {
				fmt.Fprintf(os.Stderr, "check: the adapter copies could not be written: %v\n", err)
				os.Exit(2)
			}
			for _, dest := range wrote {
				fmt.Fprintf(os.Stderr, "check: wrote %s\n", dest)
			}
		default:
			fmt.Fprintf(os.Stderr, "check: --fix applies to two checks, package-table and adapters, and %q was named\n", name)
			os.Exit(2)
		}
		tree, err = checks.Load(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "check: not a git repository (%v)\n", err)
			os.Exit(2)
		}
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
			report(os.Stdout, c.name, res)
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
	fmt.Fprint(os.Stderr, "usage: check [CHECK] [--json]\n       check package-table --fix\n       check adapters --fix\n       check commit-msg FILE\n\nWith no CHECK, runs every one and prints a verdict.\n\n")
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

// skipReason is why a check ran nothing, when it ran nothing.
//
// ⛔ A SKIP IS NOT A PASS AND IS NEVER PRINTED AS ONE. Both of the gate's outputs
// used to read only the problem count, so a host with no shellcheck and no Go
// toolchain printed `ok shellcheck` and `ok go` and a JSON document saying 0 for
// each, over two checks that had not looked at anything. TOOL-24.
func skipReason(r checks.Result) (string, bool) {
	why, ok := r.Extra["skipped"].(string)
	return why, ok && why != "" && r.Problems == 0
}

func report(w io.Writer, name string, r checks.Result) {
	if why, ok := skipReason(r); ok {
		fmt.Fprintf(w, "  skip   %s: %s\n", name, why)
		return
	}
	if r.Problems == 0 {
		fmt.Fprintf(w, "  ok     %s\n", name)
		return
	}
	sort.Strings(r.Detail)
	for _, d := range r.Detail {
		fmt.Fprintf(w, "  FAIL   %s\n", d)
	}
	fmt.Fprintf(w, "\n%s: %d problems\n", name, r.Problems)
}

type gateRow struct {
	name string
	res  checks.Result
}

func gate(t *checks.Tree, asJSON bool) int {
	var rows []gateRow
	for _, c := range all {
		rows = append(rows, gateRow{c.name, c.run(t)})
	}
	return renderGate(os.Stdout, rows, asJSON)
}

// renderGate writes the verdict and returns the exit code. A skip still exits 0,
// because "this host cannot run that check" is not a defect in the tree, and it
// is named in both outputs so nobody reads it as agreement.
func renderGate(w io.Writer, rows []gateRow, asJSON bool) int {
	total := 0
	skipped := map[string]string{}
	for _, r := range rows {
		total += r.res.Problems
		if why, ok := skipReason(r.res); ok {
			skipped[r.name] = why
		}
	}
	if asJSON {
		out := map[string]any{"schema": "check-gate/1", "problems": total}
		for _, r := range rows {
			out[r.name] = r.res.Problems
		}
		if len(skipped) > 0 {
			out["skipped"] = skipped
		}
		b, _ := json.Marshal(out)
		fmt.Fprintln(w, string(b))
		if total > 0 {
			return 1
		}
		return 0
	}
	for _, r := range rows {
		if why, ok := skipped[r.name]; ok {
			fmt.Fprintf(w, "  skip   %-15s %s\n", r.name, why)
			continue
		}
		if r.res.Problems == 0 {
			fmt.Fprintf(w, "  ok     %-15s\n", r.name)
			continue
		}
		fmt.Fprintf(w, "  FAIL   %-15s %d\n", r.name, r.res.Problems)
		sort.Strings(r.res.Detail)
		for i, d := range r.res.Detail {
			if i == 12 {
				fmt.Fprintf(w, "           ... and %d more\n", len(r.res.Detail)-12)
				break
			}
			fmt.Fprintf(w, "           %s\n", d)
		}
	}
	fmt.Fprintln(w)
	if total > 0 {
		fmt.Fprintf(w, "VERDICT: %d problems.\n", total)
		return 1
	}
	if len(skipped) > 0 {
		fmt.Fprintf(w, "VERDICT: the tree agrees with itself on %d of %d checks. %d could not run on this host and say nothing about it.\n",
			len(rows)-len(skipped), len(rows), len(skipped))
	} else {
		fmt.Fprintln(w, "VERDICT: the tree agrees with itself.")
	}
	fmt.Fprintln(w, "A green gate is not the finish line. It catches mechanical regressions;")
	fmt.Fprintln(w, "whether a claim is true is a reading, and that belongs to the review pass.")
	return 0
}
