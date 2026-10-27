// Package embedswap hands a mutation row's bytes to a test binary that embeds
// the row's file, so the row needs no compile. TOOL-40.
//
// ⭐ THE PROTOCOL IS THREE VARIABLES, set by tools/repo's mutation harness.
// NameEnv is the file's path under the package's directory. WithEnv is a file
// holding the changed bytes, and FileEnv is the file itself in the staged copy.
//
// ⛔ THE BYTES GO WHERE THE BINARY READS THEM FIRST, AND INTO THE FILE LAST. A
// mutation compiled in changes both: most cases read the variable, and a few
// read the file from disk. The harness reads the file back as its receipt, so a
// file written means the variable took the bytes too.
//
// ⚠ ONLY A PACKAGE'S TEST FILES CALL THIS. The executable never imports it, so
// no variable of a user's environment can change a script it runs.
//
// SPDX-License-Identifier: 0BSD

package embedswap

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"testing/fstest"
)

// The protocol's variables. ⛔ tools/repo/internal/mutate/embed.go names the
// same three, and a rename on one side alone sends every row back to a compile.
const (
	NameEnv = "REPO_MUTATE_EMBED"
	WithEnv = "REPO_MUTATE_WITH"
	FileEnv = "REPO_MUTATE_FILE"
)

// Take hands the bytes a mutation row names to take, and then writes them into
// the row's file. take answers an error for a name its package does not embed.
// With no row named, Take does nothing.
func Take(take func(name string, body []byte) error) error {
	name := os.Getenv(NameEnv)
	if name == "" {
		return nil
	}
	body, err := os.ReadFile(os.Getenv(WithEnv))
	if err != nil {
		return err
	}
	if err := take(name, body); err != nil {
		return err
	}
	file := os.Getenv(FileEnv)
	if file == "" {
		return nil
	}
	// ⚠ ONCE. A case that starts its own binary again as a fake hands it the
	// same variables, and the copy already holds the bytes.
	if was, err := os.ReadFile(file); err == nil && bytes.Equal(was, body) {
		return nil
	}
	return os.WriteFile(file, body, 0o600)
}

// WithFile copies every file of tree, with name holding body.
//
// ⛔ A NAME THE TREE DOES NOT HOLD IS REFUSED, not added. The row's file is one
// the package embeds, and anything else is a row the binary cannot take.
func WithFile(tree fs.FS, name string, body []byte) (fstest.MapFS, error) {
	out := fstest.MapFS{}
	err := fs.WalkDir(tree, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(tree, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[p] = &fstest.MapFile{Data: data, Mode: info.Mode(), ModTime: info.ModTime()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	f, ok := out[name]
	if !ok {
		return nil, fmt.Errorf("%s is not a file this package embeds", name)
	}
	f.Data = body
	return out, nil
}
