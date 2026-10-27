// SPDX-License-Identifier: 0BSD

package parallelread

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fixtureSource is a package written to break each rule once. Every case named
// in wantFound sets something shared, and TestClean and TestSerial do not.
const fixtureSource = `package p

import (
	"fmt"
	"io"
	web "net/http"
	"os"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"

	"example.test/lib/v2"
)

var seam = 1

var _ = seam

var counter int

var guarded struct {
	mu sync.Mutex
	v  int
}

var toolExit atomic.Int64

var registry = map[string]int{}

var table = []int{1, 2}

var re = regexp.MustCompile("x")

var hook = func() { seam = 9 }

type registryT struct{ m map[string]int }

func (r registryT) add(k string) { r.m[k] = 1 }

var reg = registryT{m: map[string]int{}}

type recorder struct{}

func (recorder) Write(b []byte) (int, error) { seam = len(b); return len(b), nil }

type holder struct{ r recorder }

type box[T any] struct{ v T }

func (box[T]) set() { seam = 6 }

var gbox box[int]

func helper(t *testing.T)     { t.Setenv("X", "1") }
func swap()                   { seam = 2 }
func swapSeam()               { seam = 2 }
func par(t *testing.T)        { t.Parallel() }
func parDeep(t *testing.T)    { par(t) }
func move()                   { _ = os.Chdir("") }
func setExit()                { toolExit.Store(77) }
func run(f func())            { f() }
func setGeneric[T any](v T)   { seam = 3 }

func TestEnv(t *testing.T)       { par(t); helper(t) }
func TestSwap(t *testing.T)      { t.Parallel(); swap() }
func TestStdout(t *testing.T)    { par(t); os.Stdout = nil }
func TestMove(t *testing.T)      { par(t); move() }
func TestStore(t *testing.T)     { par(t); setExit() }
func TestMethodVal(t *testing.T) { par(t); f := toolExit.Store; f(1) }
func TestDelete(t *testing.T)    { par(t); delete(registry, "x") }
func TestClear(t *testing.T)     { par(t); clear(registry) }
func TestCopy(t *testing.T)      { par(t); copy(table, []int{3}) }
func TestAddr(t *testing.T)      { par(t); p := &counter; *p = 1 }
func TestIncrement(t *testing.T) { par(t); counter++ }
func TestRange(t *testing.T)     { par(t); for counter = range 2 {} }
func TestDeep(t *testing.T)      { parDeep(t); seam = 3 }
func TestShadow(t *testing.T)    { par(t); if true { seam := 0; _ = seam }; seam = 4 }
func TestFuncVal(t *testing.T)   { par(t); run(swapSeam) }
func TestNested(t *testing.T)    { par(t); web.DefaultClient.Timeout = 1 }
func TestField(t *testing.T)     { par(t); _ = struct{ seam int }{}; seam = 5 }
func TestVersioned(t *testing.T) { par(t); lib.Level = 2 }
func TestGeneric(t *testing.T)   { par(t); setGeneric[int](1) }
func TestGenericMethod(t *testing.T) { par(t); gbox.set() }
func TestStoredFunc(t *testing.T) { par(t); hook() }
func TestOwnMethod(t *testing.T) { par(t); reg.add("k") }
func TestLongest(t *testing.T)   { par(t); re.Longest() }
func TestInterface(t *testing.T) { par(t); var h holder; var w io.Writer = &h.r; _, _ = w.Write(nil) }
func TestTypeNamed(t *testing.T) { par(t); fmt.Fprint(recorder{}, "x") }

func TestClean(t *testing.T) {
	t.Parallel()
	seam := 3
	seam++
	guarded.v = seam
	_ = toolExit.Load()
	_ = re.MatchString("x")
	_ = registry["x"]
}
func TestSerial(t *testing.T) { helper(t); swap() }
`

// wantFound is each finding the fixture must give, as the case and what it
// sets.
var wantFound = []string{
	"TestEnv sets Setenv", "TestSwap sets seam", "TestStdout sets os.Stdout", "TestMove sets Chdir",
	"TestStore sets toolExit.Store", "TestMethodVal sets toolExit.Store", "TestDelete sets registry",
	"TestClear sets registry", "TestCopy sets table", "TestAddr sets counter", "TestIncrement sets counter", "TestRange sets counter",
	"TestDeep sets seam", "TestShadow sets seam", "TestFuncVal sets seam", "TestNested sets http.DefaultClient",
	"TestField sets seam", "TestVersioned sets lib.Level", "TestGeneric sets seam", "TestGenericMethod sets seam", "TestStoredFunc sets seam",
	"TestOwnMethod sets reg.add", "TestLongest sets re.Longest", "TestInterface sets seam", "TestTypeNamed sets seam",
}

// fixture writes a module holding package p and the /v2 package it imports.
func fixture(t *testing.T, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":        "module example.test\n\ngo 1.25.0\n",
		"p/p_test.go":   fixtureSource,
		"p/p.go":        "package p\n",
		"lib/v2/lib.go": "package lib\n\nvar Level = 1\n",
	}
	for name, body := range extra {
		files[name] = body
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "p")
}

// TestTheParallelReadingFindsWhatACaseSets holds SharedWrites to the fixture.
//
// ⛔ EACH SHAPE HERE WAS ONE THE NAME-MATCHING READING MISSED: a write after an
// inner local of the same name, or after a field of it. Also a case made
// parallel two helpers down, and a method that sets an atomic. Also delete on a
// package map, another package's field, a function passed as a value, an
// import ending in /v2, and a generic call.
//
// ⚠ WHAT IT DOES NOT SEE, which the README names too. It misses a map, slice or
// pointer a package variable holds, once copied into a local or a parameter and
// written there. It misses a write through what a call
// returned, as f().x = 1. It misses a value method of another package's type
// that writes through a map, slice or pointer its value holds. It misses a
// function of another package that changes that package's own state, as
// log.SetOutput. The environment and the working directory are the exceptions
// it reads. It does not read the files this host does not build, and CI's
// Linux job reads those.
func TestTheParallelReadingFindsWhatACaseSets(t *testing.T) {
	p, err := Load(fixture(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	found, parallel := p.SharedWrites(map[string]bool{"guarded": true})
	if parallel != len(wantFound)+1 {
		t.Errorf("%d parallel case(s) were read, want %d", parallel, len(wantFound)+1)
	}
	got := strings.Join(found, "\n")
	for _, want := range wantFound {
		if !strings.Contains(got, want+" at ") {
			t.Errorf("the reading did not find %q", want)
		}
	}
	for _, clean := range []string{"TestClean", "TestSerial"} {
		if strings.Contains(got, clean+" ") {
			t.Errorf("the reading named %s, which sets nothing shared or is not parallel", clean)
		}
	}
	if t.Failed() {
		sort.Strings(found)
		t.Logf("the reading found:\n%s", strings.Join(found, "\n"))
	}
}

// TestAnAllowedMethodIsNotAFinding: a name with its method allows that method
// alone.
func TestAnAllowedMethodIsNotAFinding(t *testing.T) {
	p, err := Load(fixture(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	found, _ := p.SharedWrites(map[string]bool{"toolExit.Store": true, "re.Longest": true})
	got := strings.Join(found, "\n")
	if strings.Contains(got, "toolExit.Store") || strings.Contains(got, "re.Longest") {
		t.Errorf("an allowed method was named:\n%s", got)
	}
	if !strings.Contains(got, "TestSwap sets seam") {
		t.Errorf("allowing two methods hid a variable they do not name:\n%s", got)
	}
}

// TestAPackageWithAnExternalTestPackageIsRefused: its cases run in the same
// binary, and this reading would not see them.
func TestAPackageWithAnExternalTestPackageIsRefused(t *testing.T) {
	dir := fixture(t, map[string]string{"p/x_test.go": "package p_test\n"})
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "external test package") {
		t.Fatalf("Load answered %v, want a refusal naming the external test package", err)
	}
}
