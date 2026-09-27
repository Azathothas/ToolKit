// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// -- WSL-96: a device node, passed by the engine ------------------------------

func TestADeviceIsReadInPodmansOwnSpelling(t *testing.T) {
	good := map[string]JobDevice{
		"/dev/kvm":              {Host: "/dev/kvm"},
		"/dev/kvm:/dev/kvm0":    {Host: "/dev/kvm", Container: "/dev/kvm0"},
		"/dev/kvm:/dev/kvm:rw":  {Host: "/dev/kvm", Container: "/dev/kvm", Perms: "rw"},
		"/dev/net/tun:/dev/tun": {Host: "/dev/net/tun", Container: "/dev/tun"},
	}
	for spec, want := range good {
		got, err := ParseDevice(spec)
		if err != nil || got != want {
			t.Errorf("ParseDevice(%q) = %+v, %v; want %+v", spec, got, err, want)
			continue
		}
		if got.String() != spec {
			t.Errorf("%q reads back as %q", spec, got.String())
		}
	}
	refused := map[string]string{
		"kvm":                "not an absolute path",
		"/etc/passwd":        "not under /dev",
		"/dev/../etc/shadow": "not in its clean form",
		"/dev/kvm:/x:rwx":    "permissions",
		"/dev/kvm:/x:rr":     "permissions",
		"/dev/k$m":           "no device path holds",
		"/dev/a:/b:rw:x":     "more than three parts",
		"/dev/kvm:":          "not an absolute path",
		"/dev/kvm:relative":  "not an absolute path",
	}
	for spec, want := range refused {
		if _, err := ParseDevice(spec); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseDevice(%q) = %v, want a refusal naming %q", spec, err, want)
		}
	}
}

func TestDevicesInputsAndTheEngineDeadlineReachPodman(t *testing.T) {
	runner := &Runner{}
	script := string(runner.containerScript(JobSpec{
		Image: "docker.io/library/alpine:latest", ContainerLifecycle: ContainerEphemeral,
		Devices: []JobDevice{
			{Host: "/dev/kvm"},
			{Host: "/dev/fuse", Container: "/dev/fuse", Perms: "rw"},
		},
		Inputs: []JobInput{
			{Name: "job.sh", Bytes: []byte("true\n")},
		},
		Timeout: 60 * time.Second,
	}, "wtk-one", jobLayout("/home/toolkit", "one"), "marker"))
	for _, want := range []string{
		"'--device' '/dev/kvm'",
		"'--device' '/dev/fuse:/dev/fuse:rw'",
		"'--volume' '/home/toolkit/.wsl-toolkit/jobs/one/in:/in:ro,Z'",
		"'--timeout' '90'",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the podman command does not carry %s:\n%s", want, script)
		}
	}
	// ⛔ The device check runs BEFORE the engine, so a refusal is unreached.
	check, engine := strings.Index(script, "--device /dev/kvm: there is no such node"), strings.Index(script, "exec podman")
	if check < 0 || engine < 0 || check > engine {
		t.Errorf("the device check does not precede the engine:\n%s", script)
	}
	without := string(runner.containerScript(JobSpec{Image: "docker.io/library/alpine:latest", ContainerLifecycle: ContainerEphemeral},
		"wtk-two", jobLayout("/home/toolkit", "two"), "marker"))
	for _, absent := range []string{"--device", "/in:ro", "'--timeout'"} {
		if strings.Contains(without, absent) {
			t.Errorf("a job with no devices, no inputs and no deadline still carries %s", absent)
		}
	}
}

// TestTheDeviceCheckRefusesInARealShell runs the generated check where the host
// has a POSIX shell: a device the account can open passes, an absent node and a
// file that is not a device are refused by name before anything starts.
func TestTheDeviceCheckRefusesInARealShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the check is POSIX shell, and this host has none to run it in")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	notADevice := filepath.Join(t.TempDir(), "plain")
	if err := os.WriteFile(notADevice, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(d JobDevice) (int, string) {
		cmd := exec.Command(sh, "-c", deviceCheckScript([]JobDevice{d})+"exit 0\n")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), stderr.String()
		}
		return 0, stderr.String()
	}
	if code, msg := run(JobDevice{Host: "/dev/null"}); code != 0 {
		t.Errorf("/dev/null is a device anyone can open and the check answered %d: %s", code, msg)
	}
	if code, msg := run(JobDevice{Host: "/dev/wsl-toolkit-absent"}); code != 125 || !strings.Contains(msg, "no such node") {
		t.Errorf("an absent node answered %d: %s", code, msg)
	}
	if code, msg := run(JobDevice{Host: notADevice}); code != 125 || !strings.Contains(msg, "not a device node") {
		t.Errorf("a plain file answered %d: %s", code, msg)
	}
}

// -- WSL-97: a job's own input files at /in ------------------------------------

func TestAnInputNameIsAPlainPathUnderIn(t *testing.T) {
	for _, ok := range []string{"job.sh", "conf/app.toml", "a/b/c-1.2_x+y=z@w,v"} {
		if err := ValidateInputName(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "/etc/passwd", "../x", "a/../b", "a//b", "./a", "a/.", "a b", "a$b"} {
		if err := ValidateInputName(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestASetOfInputsThatCannotArriveAsNamedIsRefused(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	limits := DefaultWorkspaceLimits()
	cases := map[string][]JobInput{
		"twice": {
			{Name: "a", Path: file},
			{Name: "a", Path: file},
		},
		"needs it to be": {
			{Name: "a", Path: file},
			{Name: "a/b", Path: file},
		},
		"not a regular": {
			{Name: "d", Path: dir},
		},
		"which passes 5 B": {
			{Name: "big", Path: file},
		},
		"entries": {
			{Name: "x", Bytes: []byte("1")},
			{Name: "y", Bytes: []byte("2")},
		},
		"no such file": {
			{Name: "gone", Path: filepath.Join(dir, "gone")},
		},
	}
	for want, inputs := range cases {
		l := limits
		switch want {
		case "which passes 5 B":
			l.MaxBytes = 5
		case "entries":
			l.MaxEntries = 1
		}
		err := CheckInputs(inputs, l)
		msg := ""
		if err != nil {
			msg = err.Error()
		}
		if want == "no such file" {
			if err == nil {
				t.Errorf("an input that does not exist was accepted")
			}
			continue
		}
		if !strings.Contains(msg, want) {
			t.Errorf("case %q: err = %v", want, err)
		}
	}
	twoInOne := []JobInput{
		{Name: "a/b", Path: file},
		{Name: "a/c", Bytes: []byte("x")},
	}
	if err := CheckInputs(twoInOne, limits); err != nil {
		t.Errorf("two files in one directory were refused: %v", err)
	}
}

func TestInputsTravelByteForByteWithTheirDirectories(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "job.sh")
	body := []byte("#!/bin/sh\r\necho keep the CRLF\r\n")
	if err := os.WriteFile(file, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	up, err := writeInputsTar(&archive, []JobInput{
		{Name: "job.sh", Path: file},
		{Name: "conf/deep/app.toml", Bytes: []byte("k = 1\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if up.Entries != 2 {
		t.Errorf("entries = %d, want 2", up.Entries)
	}
	got := map[string]string{}
	tr := tar.NewReader(&archive)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		got[hdr.Name] = string(b)
	}
	want := map[string]string{"conf/": "", "conf/deep/": "", "conf/deep/app.toml": "k = 1\n", "job.sh": string(body)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("archive = %q, want %q", got, want)
	}
}

// TestEveryMatrixFieldReachesItsRows sets every field MatrixSpec shares with
// JobSpec and reads it back from a row. A field the fleet takes and a row drops
// is a flag that works on run and does nothing on matrix.
func TestEveryMatrixFieldReachesItsRows(t *testing.T) {
	// A row takes the STAGED copy, its own artifact directory and the resolved
	// limits, each an argument of rowSpec, and the excludes were applied when the
	// fleet staged the workspace every row copies.
	rowOwn := map[string]bool{"Workspace": true, "ArtifactDir": true, "Limits": true, "Excludes": true}
	var spec MatrixSpec
	sv := reflect.ValueOf(&spec).Elem()
	jt := reflect.TypeOf(JobSpec{})
	shared := 0
	for i := 0; i < sv.NumField(); i++ {
		name := sv.Type().Field(i).Name
		if _, ok := jt.FieldByName(name); !ok {
			continue
		}
		if rowOwn[name] {
			continue
		}
		shared++
		f := sv.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString("x-" + name)
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int, reflect.Int64:
			f.SetInt(7)
		}
	}
	// ⚠ The slices, the map and the function are set by name. A new one the two
	// types share stays empty here, so the check below names it until it is
	// added to this list, which is the point.
	spec.Devices = []JobDevice{
		{Host: "/dev/kvm"},
	}
	spec.Inputs = []JobInput{
		{Name: "job.sh", Bytes: []byte("true")},
	}
	spec.Env = map[string]string{"A": "b"}
	spec.Script = []byte("true")
	spec.OnTick = func(TickEvent) {}
	row := rowSpec(spec, Image{ID: "alpine", Ref: "docker.io/library/alpine:latest"}, "/staged", "", DefaultWorkspaceLimits())
	rv := reflect.ValueOf(row)
	for i := 0; i < sv.NumField(); i++ {
		name := sv.Type().Field(i).Name
		jf := rv.FieldByName(name)
		if !jf.IsValid() || rowOwn[name] {
			continue
		}
		if jf.IsZero() {
			t.Errorf("MatrixSpec.%s is set and a row's JobSpec.%s is empty", name, name)
		}
	}
	if shared < 10 {
		t.Fatalf("only %d shared fields were walked; the walk expects every one", shared)
	}
}
