package toolkit

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The cases below read scripts/common/bootstrap.sh from the tree, as
// TestNativeInventoryCoversStandaloneProbe reads the doctor probe. ⚠ No mutation row
// can hold them: `repo mutate` copies a module, and that file is in none.

func bootstrapSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../../../../scripts/common/bootstrap.sh")
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// bootstrapFunction is one function of bootstrap.sh, from its name to the closing
// brace at the start of a line.
func bootstrapFunction(t *testing.T, src, name string) string {
	t.Helper()
	start := strings.Index(src, "\n"+name+"() {\n")
	if start < 0 {
		t.Fatalf("bootstrap.sh defines no %s", name)
	}
	body := src[start+1:]
	end := strings.Index(body, "\n}\n")
	if end < 0 {
		t.Fatalf("bootstrap.sh's %s has no closing brace", name)
	}
	return body[:end+3]
}

// dashFor is the dash this host has, or a skip.
func dashFor(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("bootstrap.sh is read by a POSIX shell, and this host has none to hand it to")
	}
	dash, err := exec.LookPath("dash")
	if err != nil {
		t.Skip("the defect is dash's, and this host has no dash")
	}
	return dash
}

// TestBootstrapFetchesAnNpmArchiveIntoANamedDirectoryUnderDash runs bootstrap.sh's own
// fetch_verified_npm under dash, inside an `if` as install_codegraph calls it, with npm
// and the two digest readers stood in for.
//
// ⛔ WSL-87. The directory was read back through `read` from a pipe with no trailing
// newline, and under dash with `set -e` it came back empty: debian, debian 12, ubuntu
// 22.04 and void-musl exited 1 with `npm did not write exactly one archive into `.
// Measured on 2026-09-15.
func TestBootstrapFetchesAnNpmArchiveIntoANamedDirectoryUnderDash(t *testing.T) {
	dash := dashFor(t)
	src := bootstrapSource(t)
	work := t.TempDir()
	harness := strings.Join([]string{
		"set -eu",
		`fail() { printf 'fail: %s\n' "$*" >&2; FAILURES=$((FAILURES + 1)); }`,
		`step() { printf 'step: %s\n' "$*" >&2; }`,
		"FAILURES=0",
		"EXPECT_INTEGRITY=",
		"CODEGRAPH_MAIN='@colbymchenry/codegraph'",
		`npm() {`,
		`  np_dest=`,
		`  while [ "$#" -gt 0 ]; do`,
		`    case "$1" in`,
		`      --pack-destination) np_dest=$2; shift 2 ;;`,
		`      *) shift ;;`,
		`    esac`,
		`  done`,
		`  if [ -z "$np_dest" ] || [ ! -d "$np_dest" ]; then return 1; fi`,
		`  : > "$np_dest/package-1.0.0.tgz"`,
		`}`,
		`sri_of_file() { printf 'sha512-same'; }`,
		`npm_registry_integrity() { printf 'sha512-same'; }`,
		bootstrapFunction(t, src, "split_on"),
		bootstrapFunction(t, src, "fetch_verified_npm"),
		`if fetch_verified_npm "$WORK" '@colbymchenry/codegraph-linux-x64' 1.6.0; then`,
		`  printf 'archive=%s\n' "$FETCHED_ARCHIVE"`,
		`else`,
		`  printf 'refused failures=%s\n' "$FAILURES"`,
		`fi`,
	}, "\n") + "\n"
	cmd := exec.Command(dash, "-c", harness)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "WORK=" + work}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("the harness exited: %v; stderr %q", err, stderr.String())
	}
	want := "archive=" + filepath.ToSlash(filepath.Join(work, "colbymchenry-codegraph-linux-x64", "package-1.0.0.tgz")) + "\n"
	if stdout.String() != want {
		t.Errorf("dash answered %q, want %q; stderr %q", stdout.String(), want, stderr.String())
	}
}

// codegraphKernelCase is one kernel and architecture install_codegraph is offered.
type codegraphKernelCase struct {
	kernel  string
	arch    string
	fetched string // the package it reached for, or empty where it refused first
	says    string
}

// TestBootstrapNamesAKernelCodegraphPublishesNoPackageFor runs bootstrap.sh's own
// install_codegraph under dash with the fetch, the registry and npm stood in for.
//
// ⛔ WSL-87's door sweep, 2026-09-15. The function read the architecture and not the
// kernel, so an amd64 FreeBSD, NetBSD or OpenBSD host resolved the linux-x64 package
// and fetched it. A route that cannot exist is named before the fetch, as the too-old
// npm is.
func TestBootstrapNamesAKernelCodegraphPublishesNoPackageFor(t *testing.T) {
	dash := dashFor(t)
	src := bootstrapSource(t)
	linux := codegraphKernelCase{kernel: "Linux", arch: "x86_64", fetched: "@colbymchenry/codegraph-linux-x64"}
	linuxArm := codegraphKernelCase{kernel: "Linux", arch: "aarch64", fetched: "@colbymchenry/codegraph-linux-arm64"}
	freeBSD := codegraphKernelCase{kernel: "FreeBSD", arch: "amd64", says: "codegraph publishes a Linux package only, and this kernel is FreeBSD"}
	netBSD := codegraphKernelCase{kernel: "NetBSD", arch: "amd64", says: "codegraph publishes a Linux package only, and this kernel is NetBSD"}
	openBSD := codegraphKernelCase{kernel: "OpenBSD", arch: "amd64", says: "codegraph publishes a Linux package only, and this kernel is OpenBSD"}
	darwin := codegraphKernelCase{kernel: "Darwin", arch: "arm64", says: "codegraph publishes a Linux package only, and this kernel is Darwin"}
	elsewhere := codegraphKernelCase{kernel: "Linux", arch: "riscv64", says: "codegraph publishes no Linux package for riscv64"}
	for _, c := range []codegraphKernelCase{linux, linuxArm, freeBSD, netBSD, openBSD, darwin, elsewhere} {
		prefix := t.TempDir()
		harness := strings.Join([]string{
			"set -eu",
			`fail() { printf 'fail: %s\n' "$*" >&2; FAILURES=$((FAILURES + 1)); }`,
			`step() { printf 'step: %s\n' "$*" >&2; }`,
			"FAILURES=0",
			"DRY_RUN=0",
			"CODEGRAPH_VERSION=",
			"CODEGRAPH_MAIN='@colbymchenry/codegraph'",
			"KERNEL=" + c.kernel,
			"ARCH=" + c.arch,
			`PREFIX=$WORK`,
			`have() { return 0; }`,
			`first_line() { printf '10.0.0'; }`,
			`npm_registry_version() { printf '1.6.0'; }`,
			`npm() { printf 'npm %s\n' "$*" >&2; }`,
			`fetch_verified_npm() { printf 'fetch: %s\n' "$2"; FETCHED_ARCHIVE=$1/one.tgz; }`,
			bootstrapFunction(t, src, "npm_packs_to_a_directory"),
			bootstrapFunction(t, src, "install_codegraph"),
			`if install_codegraph latest; then printf 'installed=%s\n' "$CODEGRAPH_VERSION"; else printf 'refused failures=%s\n' "$FAILURES"; fi`,
		}, "\n") + "\n"
		cmd := exec.Command(dash, "-c", harness)
		cmd.Env = []string{"PATH=/usr/bin:/bin", "WORK=" + prefix}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s %s: the harness exited: %v; stderr %q", c.kernel, c.arch, err, stderr.String())
		}
		fetched := ""
		for _, line := range strings.Split(stdout.String(), "\n") {
			if rest, ok := strings.CutPrefix(line, "fetch: "); ok && fetched == "" {
				fetched = rest
			}
		}
		if fetched != c.fetched {
			t.Errorf("%s %s reached for %q, want %q", c.kernel, c.arch, fetched, c.fetched)
		}
		if c.says == "" {
			if !strings.Contains(stdout.String(), "installed=1.6.0") {
				t.Errorf("%s %s answered %q, want an install", c.kernel, c.arch, stdout.String())
			}
			continue
		}
		if !strings.Contains(stdout.String(), "refused failures=1") {
			t.Errorf("%s %s answered %q, want one refusal", c.kernel, c.arch, stdout.String())
		}
		if !strings.Contains(stderr.String(), c.says) {
			t.Errorf("%s %s said %q, want a line carrying %q", c.kernel, c.arch, stderr.String(), c.says)
		}
	}
}

// TestBootstrapNamesAnNpmTooOldToPackToADirectory holds the versions measured on
// 2026-09-15: npm 6.14.11 and 7.17.0 have no --pack-destination, 7.18.0 and 7.18.1 do.
func TestBootstrapNamesAnNpmTooOldToPackToADirectory(t *testing.T) {
	dash := dashFor(t)
	src := bootstrapSource(t)
	versions := []struct {
		version string
		packs   bool
	}{
		{"6.14.11", false}, {"7.9.0", false}, {"7.17.0", false}, {"7.18.0", true}, {"7.18.1", true},
		{"9.2.0", true}, {"10.9.2", true}, {"12.0.1", true}, {"", false}, {"npm", false},
	}
	var harness strings.Builder
	harness.WriteString(bootstrapFunction(t, src, "npm_packs_to_a_directory"))
	for _, v := range versions {
		harness.WriteString("if npm_packs_to_a_directory '" + v.version + "'; then echo yes; else echo no; fi\n")
	}
	out, err := exec.Command(dash, "-c", harness.String()).Output()
	if err != nil {
		t.Fatalf("the harness exited: %v", err)
	}
	answers := strings.Fields(string(out))
	if len(answers) != len(versions) {
		t.Fatalf("%d answers for %d versions: %q", len(answers), len(versions), out)
	}
	for i, v := range versions {
		if (answers[i] == "yes") != v.packs {
			t.Errorf("npm %q answered %s, want packs=%v", v.version, answers[i], v.packs)
		}
	}
}
