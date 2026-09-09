// SPDX-License-Identifier: 0BSD

package binfmt

import (
	"bytes"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name       string
		names      []string
		handlers   int
		statusFile string
	}{
		{"a machine with handlers", []string{"status", "register", "qemu-aarch64", "qemu-arm", "qemu-riscv64"}, 3, "present"},
		{"registered but not enabled", []string{"register", "qemu-aarch64"}, 1, "absent"},
		{"nothing at all", []string{}, 0, "absent"},
		{"only the switch", []string{"status", "register"}, 0, "present"},
		// ⚠ Only qemu- counts. A handler somebody else registered is not one of
		// these, and counting it would report a machine as ready for
		// cross-architecture execution that is not.
		{"a foreign handler", []string{"status", "jar", "python3.12"}, 0, "present"},
		{"blank lines from a listing", []string{"", "qemu-arm", "  ", "status"}, 1, "present"},
	}
	for _, c := range cases {
		h, sf := classify(c.names)
		if h != c.handlers || sf != c.statusFile {
			t.Errorf("%s: classify = (%d, %q), want (%d, %q)", c.name, h, sf, c.handlers, c.statusFile)
		}
	}
}

// TestELOOPIsItsOwnVerdict is the case for the state this whole check exists to
// name.
//
// ⛔ A directory that exists and cannot be read is a second filesystem stacked
// on the same path. systemd-binfmt writes underneath it and reports
// status=0/SUCCESS having registered nothing, so the unit is green and
// cross-architecture execution has never once worked. Reporting that as a
// generic "unreadable" loses the diagnosis.
func TestELOOPIsItsOwnVerdict(t *testing.T) {
	for _, s := range []string{
		"ls: cannot open directory '/proc/sys/fs/binfmt_misc': Too many levels of symbolic links",
		"open /proc/sys/fs/binfmt_misc: ELOOP",
		"TOO MANY LEVELS OF SYMBOLIC LINKS",
	} {
		if !isELOOP(s) {
			t.Errorf("isELOOP(%q) = false", s)
		}
	}
	for _, s := range []string{
		"permission denied",
		"no such file or directory",
		"",
	} {
		if isELOOP(s) {
			t.Errorf("isELOOP(%q) = true, and it is a different failure", s)
		}
	}
}

func TestVerdict(t *testing.T) {
	cases := []struct {
		name    string
		rep     Report
		require int
		problem string
		stacked bool
	}{
		{"a healthy machine", Report{Handlers: 31}, 0, "", false},
		{"zero handlers and nothing required", Report{Handlers: 0}, 0, "", false},
		{"zero handlers and one required", Report{Handlers: 0}, 1, "below-require", false},
		{"below the ceiling", Report{Handlers: 2}, 5, "below-require", false},
		{"exactly at the ceiling", Report{Handlers: 5}, 5, "", false},
		{"the stacked mount", Report{readErr: "Too many levels of symbolic links"}, 0, "stacked-mount", true},
		{"some other read failure", Report{readErr: "permission denied"}, 0, "unreadable", false},
		// ⛔ A read that failed is never below-require, whatever --require said.
		// The count is zero because nothing could be counted, and reporting the
		// ceiling would name the wrong problem.
		{"unreadable with a requirement", Report{readErr: "permission denied"}, 9, "unreadable", false},
	}
	for _, c := range cases {
		got := verdict(c.rep, Options{Require: c.require})
		if got.Problem != c.problem || got.Stacked != c.stacked {
			t.Errorf("%s: verdict = (%q, stacked=%v), want (%q, stacked=%v)", c.name, got.Problem, got.Stacked, c.problem, c.stacked)
		}
	}
}

// TestZeroHandlersIsNotAFailure is the rule scripts/README.md states: a check
// that measures an open defect must not fail the build for that defect alone,
// and it judges only past a stated ceiling.
func TestZeroHandlersIsNotAFailure(t *testing.T) {
	var out bytes.Buffer
	rep := Report{Schema: Schema, Source: "local", Kernel: "test", Handlers: 0, StatusFile: "present"}
	code := render(verdict(rep, Options{}), Options{}, &out)
	if code != 0 {
		t.Fatalf("zero handlers exited %d; a machine that never wanted cross-architecture execution is not broken", code)
	}
	if !strings.Contains(out.String(), "ZERO handlers") {
		t.Error("the report does not say that zero handlers were found")
	}
	if !strings.Contains(out.String(), "Exec format error") {
		t.Error("the report does not name the symptom a caller would actually see")
	}
}

func TestBelowRequireExitsOne(t *testing.T) {
	var out bytes.Buffer
	rep := verdict(Report{Schema: Schema, Source: "local", Handlers: 0, StatusFile: "present"}, Options{Require: 1})
	if code := render(rep, Options{Require: 1}, &out); code != 1 {
		t.Fatalf("--require 1 over zero handlers exited %d, want 1", code)
	}
}

func TestLooksUnreadable(t *testing.T) {
	for _, s := range []string{"Too many levels of symbolic links", "No such file or directory", "Not a directory", "ELOOP"} {
		if !looksUnreadable(s) {
			t.Errorf("looksUnreadable(%q) = false", s)
		}
	}
	// ⚠ A real listing must not be read as an error. This is what a working
	// machine returns, and mistaking it for a failure would report a healthy
	// host as broken.
	if looksUnreadable("status\nregister\nqemu-aarch64\nqemu-arm") {
		t.Error("a real listing was read as an error")
	}
}
