// embed_test.go - a row on an embedded file, run on its baseline's binary.
//
// SPDX-License-Identifier: 0BSD

package mutate

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// embedGuard embeds rule.txt and refuses the empty string while it says refuse.
// ⭐ compiled is the rule as the binary was BUILT, read before any init can
// hand a row's bytes in. A case that logs it shows which binary ran.
const embedGuard = "package guard\n\nimport (\n\t_ \"embed\"\n\t\"strings\"\n)\n\n" +
	"//go:embed rule.txt\nvar rule string\n\nvar compiled = rule\n\n" +
	"func Allowed(s string) bool {\n\treturn s != \"\" || strings.TrimSpace(rule) != \"refuse\"\n}\n"

// embedTaker answers the protocol the way a package that takes it does: the
// variable, then the file. intoVar false writes the file alone, as a package
// whose test code takes the file and not the variable would.
func embedTaker(intoVar bool) string {
	take := "\trule = string(b)\n"
	if !intoVar {
		take = ""
	}
	return "package guard\n\nimport \"os\"\n\nfunc init() {\n" +
		"\tif os.Getenv(\"REPO_MUTATE_EMBED\") != \"rule.txt\" {\n\t\treturn\n\t}\n" +
		"\tb, err := os.ReadFile(os.Getenv(\"REPO_MUTATE_WITH\"))\n\tif err != nil {\n\t\treturn\n\t}\n" +
		take + "\t_ = os.WriteFile(os.Getenv(\"REPO_MUTATE_FILE\"), b, 0o600)\n}\n"
}

// embedCase logs the rule each run's binary was built with to path, and fails
// on whatever else says so.
func embedCase(path, alsoFail string) string {
	return "package guard\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestEmptyIsRefused(t *testing.T) {\n" +
		"\tf, err := os.OpenFile(" + strconv.Quote(path) + ", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)\n" +
		"\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n\t_, _ = f.WriteString(compiled)\n\t_ = f.Close()\n" +
		alsoFail +
		"\tif Allowed(\"\") {\n\t\tt.Fatal(\"the empty string was allowed\")\n\t}\n}\n"
}

// builtWith is what each run's binary was compiled with, in order.
func builtWith(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(body))
}

var embedRow = Mutation{Label: "the rule's refusal", Module: "mod", File: "rule.txt", Run: "TestEmptyIsRefused", Find: "refuse\n", Replace: "allow\n"}

// TestARowOnAnEmbeddedFileRunsOnItsBaselinesBinary: the mutated run compiles
// nothing, and the row is still proved.
//
// ⛔ READ FROM INSIDE THE CASE. The binary logs the rule it was compiled with,
// so a harness that compiled the row anyway shows "allow" there.
func TestARowOnAnEmbeddedFileRunsOnItsBaselinesBinary(t *testing.T) {
	t.Parallel()
	logged := filepath.Join(t.TempDir(), "built.log")
	root := fixtureWith(t, map[string]string{
		"guard.go": embedGuard, "rule.txt": "refuse\n", "take_test.go": embedTaker(true),
		"guard_test.go": embedCase(logged, ""),
	})
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{embedRow}}, Selection{Jobs: 1})
	if got[0].State != "ok" {
		t.Fatalf("state %q, want ok (%s)\n%s", got[0].State, got[0].Reason, log)
	}
	if built := builtWith(t, logged); strings.Join(built, " ") != "refuse refuse" {
		t.Fatalf("the runs' binaries were built with %v, want the baseline's twice and no compile of the row\n%s", built, log)
	}
	if !strings.Contains(log, "1 of 1 row(s) changed an embedded file and were settled with no compile") {
		t.Fatalf("the log does not say the row was settled with no compile:\n%s", log)
	}
	if b, err := os.ReadFile(filepath.Join(root, "mod", "rule.txt")); err != nil || string(b) != "refuse\n" {
		t.Fatalf("the tree's own file was changed: %q, %v", b, err)
	}
}

// TestABinaryThatDoesNotTakeTheFileIsCompiled: a package whose test code does
// not answer the protocol gets the old route, and says so.
func TestABinaryThatDoesNotTakeTheFileIsCompiled(t *testing.T) {
	t.Parallel()
	logged := filepath.Join(t.TempDir(), "built.log")
	root := fixtureWith(t, map[string]string{
		"guard.go": embedGuard, "rule.txt": "refuse\n", "guard_test.go": embedCase(logged, ""),
	})
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{embedRow}}, Selection{Jobs: 1})
	if got[0].State != "ok" {
		t.Fatalf("state %q, want ok (%s)\n%s", got[0].State, got[0].Reason, log)
	}
	if built := builtWith(t, logged); strings.Join(built, " ") != "refuse refuse allow" {
		t.Fatalf("the runs' binaries were built with %v, want the baseline, the handed-in run, then a compile of the row\n%s", built, log)
	}
	if !strings.Contains(log, "did not take the embedded file it was handed") {
		t.Fatalf("the log does not say the binary took nothing:\n%s", log)
	}
}

// TestARunThatTookNothingCannotCertifyAGuard: the case fails in the handed-in
// run for a reason that is not the row's, and the binary took nothing.
//
// ⛔ THE FILE READ BACK IS THE RECEIPT. Without it, that failure read as the
// guard going red, and a change no case looks at was certified.
func TestARunThatTookNothingCannotCertifyAGuard(t *testing.T) {
	t.Parallel()
	logged := filepath.Join(t.TempDir(), "built.log")
	flake := "\tif os.Getenv(\"REPO_MUTATE_WITH\") != \"\" {\n\t\tt.Fatal(\"a failure that is not the row's\")\n\t}\n"
	root := fixtureWith(t, map[string]string{
		"guard.go": embedGuard, "rule.txt": "refuse\n", "guard_test.go": embedCase(logged, flake),
	})
	row := embedRow
	row.Label, row.Replace = "a blank line no case reads", "refuse\n\n"
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{row}}, Selection{Jobs: 1})
	if got[0].State != "THEATRE" {
		t.Fatalf("state %q, want THEATRE (%s)\n%s", got[0].State, got[0].Reason, log)
	}
}

// TestAGreenHandedInRunIsCompiled: a package that takes the file and not the
// variable stays green on the handed-in run, and the row is compiled.
//
// ⛔ A RUN THAT STAYED GREEN IS ASKED AGAIN THE OLD WAY. Keeping its THEATRE
// would accuse a guard its case does catch.
func TestAGreenHandedInRunIsCompiled(t *testing.T) {
	t.Parallel()
	logged := filepath.Join(t.TempDir(), "built.log")
	root := fixtureWith(t, map[string]string{
		"guard.go": embedGuard, "rule.txt": "refuse\n", "take_test.go": embedTaker(false),
		"guard_test.go": embedCase(logged, ""),
	})
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{embedRow}}, Selection{Jobs: 1})
	if got[0].State != "ok" {
		t.Fatalf("state %q, want ok (%s)\n%s", got[0].State, got[0].Reason, log)
	}
	if built := builtWith(t, logged); strings.Join(built, " ") != "refuse refuse allow" {
		t.Fatalf("the runs' binaries were built with %v, want the row compiled after its handed-in run stayed green\n%s", built, log)
	}
}

// TestASkipItsBaselineMadeIsKeptFromAHandedInRun: this host cannot run the
// case, mutated or not, and the row is not compiled to learn that again.
//
// ⭐ ASKED AGAIN, EVERY SKIPPED ROW RAN TWICE. On Windows many toolkit rows on
// a script skip, and a sample of 35 took 364 s against 333 s before. TOOL-40.
func TestASkipItsBaselineMadeIsKeptFromAHandedInRun(t *testing.T) {
	t.Parallel()
	logged := filepath.Join(t.TempDir(), "built.log")
	root := fixtureWith(t, map[string]string{
		"guard.go": embedGuard, "rule.txt": "refuse\n", "take_test.go": embedTaker(true),
		"guard_test.go": embedCase(logged, "\tt.Skip(\"this host cannot run it\")\n"),
	})
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{embedRow}}, Selection{Jobs: 1})
	if got[0].State != "SKIPPED" {
		t.Fatalf("state %q, want SKIPPED (%s)\n%s", got[0].State, got[0].Reason, log)
	}
	if built := builtWith(t, logged); strings.Join(built, " ") != "refuse refuse" {
		t.Fatalf("the runs' binaries were built with %v, want the baseline's twice and no compile of the row\n%s", built, log)
	}
}

// TestAFileHandedInIsPutBackForTheNextRow: the binary wrote the row's bytes
// into the staged copy, and the next group on the worker runs on the copy.
//
// ⛔ PUT BACK, OR THE NEXT BASELINE IS BUILT WITH THE LAST ROW'S BYTES. Its
// case then fails unmutated, and the group is refused for a row it never ran.
func TestAFileHandedInIsPutBackForTheNextRow(t *testing.T) {
	t.Parallel()
	logged := filepath.Join(t.TempDir(), "built.log")
	still := "package guard\n\nimport \"testing\"\n\nfunc TestStillRefuses(t *testing.T) {\n" +
		"\tif Allowed(\"\") {\n\t\tt.Fatal(\"the empty string was allowed\")\n\t}\n}\n"
	root := fixtureWith(t, map[string]string{
		"guard.go": embedGuard, "rule.txt": "refuse\n", "take_test.go": embedTaker(true),
		"guard_test.go": embedCase(logged, ""), "still_test.go": still,
	})
	next := Mutation{Label: "a comment beside the rule", Module: "mod", File: "guard.go", Run: "TestStillRefuses", Find: "var compiled = rule\n", Replace: "var compiled = rule // unread\n"}
	got, log := runFixture(t, root, &Table{Schema: TableSchema, Mutations: []Mutation{embedRow, next}}, Selection{Jobs: 1})
	if got[0].State != "ok" || got[1].State != "THEATRE" {
		t.Fatalf("the rows are %s and %s (%s), want ok and THEATRE\n%s", got[0].State, got[1].State, got[1].Reason, log)
	}
}

// TestOnlyAFileTheCasesPackageEmbedsIsHandedIn reads the patterns as go:embed
// does, for one package holding the cases.
func TestOnlyAFileTheCasesPackageEmbedsIsHandedIn(t *testing.T) {
	t.Parallel()
	dest := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":            "module example.test\n\ngo 1.25.0\n",
		"guard.go":          "package guard\n\n//go:embed rule.txt\nvar rule string\n",
		"guard_test.go":     "package guard\n\n//go:embed other.txt\nvar other string\n",
		"sub/sub.go":        "package sub\n\nimport \"embed\"\n\n//go:embed \"data\" all:keep\nvar data embed.FS\n",
		"sub/data/a/run.sh": "x\n",
	} {
		p := filepath.Join(dest, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, sub := []string{"example.test"}, []string{"example.test/sub"}
	cases := []struct {
		file string
		pkgs []string
		want string
	}{
		{"rule.txt", root, "rule.txt"},
		{"sub/data/a/run.sh", sub, "data/a/run.sh"},
		{"sub/keep/b.txt", sub, "keep/b.txt"},
		// ⛔ A pattern only a test file declares embeds nothing the binary's
		// package code reads.
		{"other.txt", root, ""},
		{"guard.go", root, ""},
		{"rule.txt", []string{"example.test", "example.test/sub"}, ""},
		{"rule.txt", sub, ""},
		{"sub/data/a/run.sh", root, ""},
		{"sub/data/a/run.sh", []string{"elsewhere.test/sub"}, ""},
	}
	for _, c := range cases {
		got, ok := embeddedName(dest, Mutation{File: c.file}, c.pkgs)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("%s with cases in %v: %q, %v, want %q", c.file, c.pkgs, got, ok, c.want)
		}
	}
}
