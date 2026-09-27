// SPDX-License-Identifier: 0BSD

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

func testJobFlags() jobFlags {
	l := toolkit.DefaultWorkspaceLimits()
	return jobFlags{maxBytes: l.MaxBytes, maxEntries: l.MaxEntries}
}

// TestAnInputIsReadAsNameThenFile is WSL-97's command half: NAME=FILE, the
// file resolved like every other host path of a job.
func TestAnInputIsReadAsNameThenFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "job.sh")
	if err := os.WriteFile(f, []byte("true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	eq := filepath.Join(dir, "b=c.toml")
	if err := os.WriteFile(eq, []byte("k = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	j := testJobFlags()
	j.inputs = stringList{"job.sh=" + f, "conf/a=" + eq}
	got, err := j.jobInputs(toolkit.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "job.sh" || got[0].Path != f || got[1].Name != "conf/a" || got[1].Path != eq {
		t.Errorf("inputs = %+v; the name ends at the FIRST equals sign", got)
	}
	for _, bad := range []string{"noequals", "=" + f, "name=", "../x=" + f, "/abs=" + f} {
		j.inputs = stringList{bad}
		if _, err := j.jobInputs(toolkit.DefaultConfig()); err == nil {
			t.Errorf("--input %q was accepted", bad)
		}
	}
}

// TestInputsOverTheHelperAreCapped holds the one limit the helper route adds:
// every input travels inside one JSON body.
func TestInputsOverTheHelperAreCapped(t *testing.T) {
	f := filepath.Join(t.TempDir(), "big")
	if err := os.WriteFile(f, bytes.Repeat([]byte{'x'}, toolkit.MaxHelperInputBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	big := []toolkit.JobInput{
		{Name: "big", Path: f},
	}
	if _, err := helperInputs(big); err == nil || !strings.Contains(err.Error(), "one request to the helper carries") {
		t.Errorf("an input past the cap answered %v", err)
	}
	small := filepath.Join(t.TempDir(), "small")
	if err := os.WriteFile(small, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	one := []toolkit.JobInput{
		{Name: "s", Path: small},
	}
	got, err := helperInputs(one)
	if err != nil || len(got) != 1 || got[0].B64 != "b2s=" {
		t.Errorf("a small input became %+v, %v", got, err)
	}
}

// TestADeviceIsReadFromEveryFlag is WSL-96's command half.
func TestADeviceIsReadFromEveryFlag(t *testing.T) {
	j := testJobFlags()
	j.devices = stringList{"/dev/kvm", "/dev/fuse:/dev/fuse:rw"}
	got, err := j.jobDevices()
	if err != nil || len(got) != 2 || got[1].Perms != "rw" {
		t.Errorf("devices = %+v, %v", got, err)
	}
	j.devices = stringList{"/etc/passwd"}
	if _, err := j.jobDevices(); err == nil {
		t.Error("a path outside /dev was accepted as a device")
	}
}
