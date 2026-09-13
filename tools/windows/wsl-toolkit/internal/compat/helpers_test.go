// SPDX-License-Identifier: 0BSD

package compat

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// sessionFor builds a session the way Run does, without touching the host:
// no WSL is resolved, no console is written to, and the state directory is a
// temp directory the test owns.
func sessionFor(t *testing.T, args ...string) *session {
	t.Helper()
	o, err := Parse(args)
	if err != nil {
		t.Fatalf("the test's own arguments were refused: %v", err)
	}
	s := &session{
		opts:    o,
		baseDir: t.TempDir(),
		log:     &console{report: io.Discard, note: io.Discard},
		out:     io.Discard,
		errw:    io.Discard,
		stop:    context.Background(),
	}
	s.relayOff = o.NoTimestamps || o.TimestampProfile == "raw"
	t.Cleanup(func() {
		resolveWsl = findWsl
		lookupWSLAdapter = scanWSLAdapter
	})
	return s
}

// writeFile writes a file with 0600 permissions, for state the tests own.
func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o600)
}

// writeFileExec writes a script and marks it executable, for the fake engines.
func writeFileExec(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o755)
}

// osMkdirAll creates directories for state the tests own.
func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}

// stubWsl points resolveWsl at a script. The script receives the arguments
// after "-d NAME -u USER --" as $1.. and its output is what WSL said.
func stubWsl(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "wsl")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := resolveWsl
	resolveWsl = func() (string, error) { return path, nil }
	t.Cleanup(func() { resolveWsl = previous })
}
