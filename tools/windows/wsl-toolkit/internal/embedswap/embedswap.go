// Package embedswap hands a mutation row's bytes to a test binary that embeds
// the row's file, so the row needs no compile. TOOL-40.
//
// ⭐ THE PROTOCOL IS FOUR VARIABLES, set by tools/repo's mutation harness.
// NameEnv is the file's path under the package's directory. WithEnv is a file
// holding the changed bytes, one for each run. FileEnv is the file itself in
// the staged copy, and StagedEnv the SHA-256 of the bytes it was staged with.
//
// ⛔ THE BYTES GO WHERE THE BINARY READS THEM FIRST, AND INTO THE FILE LAST. A
// mutation compiled in changes both: most cases read the variable, and a few
// read the file from disk. The harness reads the file back as its receipt.
//
// ⛔ ONLY THE PROCESS THAT RUNS THE CASES WRITES THE RECEIPT. It sets TakenEnv
// once it has the bytes, and every process a case starts inherits it. Such a
// child takes the bytes and never writes the file. So a written file says that
// the case process took the bytes, and nothing about any other process.
//
// ⛔ A PROCESS THAT WAS NAMED A ROW AND COULD NOT TAKE IT SAYS SO. It leaves
// MissedSuffix beside WithEnv, and the harness then throws the run away. A
// child's failure would otherwise hide behind the case process's receipt.
//
// ⚠ ONLY A PACKAGE'S TEST FILES CALL THIS. The executable never imports it, so
// no variable of a user's environment can change a script it runs.
//
// SPDX-License-Identifier: 0BSD

package embedswap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing/fstest"
)

// The protocol's variables. ⛔ tools/repo/internal/mutate/embed.go names the
// same, and a rename on one side alone sends every row back to a compile.
const (
	NameEnv   = "REPO_MUTATE_EMBED"
	WithEnv   = "REPO_MUTATE_WITH"
	FileEnv   = "REPO_MUTATE_FILE"
	StagedEnv = "REPO_MUTATE_STAGED"
	// TakenEnv is set by the process that wrote the receipt, for its children.
	TakenEnv = "REPO_MUTATE_TAKEN"
)

// MissedSuffix names the file a process leaves beside WithEnv when it was
// named a row and could not take it.
const MissedSuffix = ".missed"

// Take hands the bytes a mutation row names to take. In the case process it
// then writes them into the row's file, the harness's receipt. take answers an
// error for a name its package does not embed. With no row named, Take does
// nothing.
//
// ⛔ AN ERROR MEANS THIS PROCESS RUNS ON BYTES THE ROW DID NOT NAME. The caller
// stops the process, and Take has already left the missed mark.
func Take(take func(name string, body []byte) error) error {
	name := os.Getenv(NameEnv)
	if name == "" {
		return nil
	}
	err := takeRow(name, take)
	if err != nil {
		if with := os.Getenv(WithEnv); with != "" {
			_ = os.WriteFile(with+MissedSuffix, []byte(err.Error()+"\n"), 0o600)
		}
	}
	return err
}

// MissedExit is how a process that could not take its row ends.
const MissedExit = 3

// MustTake is Take for a package's test binary, from a package variable's
// initializer.
//
// ⛔ A PROCESS THAT WAS NAMED A ROW AND COULD NOT TAKE IT STOPS. Its cases, or
// a fake's answers, would run on the old bytes. The run then fails, the missed
// mark is left, and the harness compiles the row instead.
func MustTake(take func(name string, body []byte) error) bool {
	if err := Take(take); err != nil {
		fmt.Fprintf(os.Stderr, "the embedded file a mutation row named was not taken: %v\n", err)
		os.Exit(MissedExit)
	}
	return true
}

// takeRow is Take once a row is named.
func takeRow(name string, take func(name string, body []byte) error) error {
	body, err := os.ReadFile(os.Getenv(WithEnv))
	if err != nil {
		return err
	}
	if err := take(name, body); err != nil {
		return err
	}
	// ⛔ A CHILD OF THE CASE PROCESS WRITES NO RECEIPT. Its write could stand
	// for a case process that never took the bytes.
	if os.Getenv(TakenEnv) != "" {
		return nil
	}
	file := os.Getenv(FileEnv)
	if file == "" {
		return errors.New("a mutation row was named with no file for its receipt")
	}
	// ⛔ ONLY OVER THE BYTES THE ROW WAS STAGED OVER. A process that outlived
	// its row cannot write a later row's file, nor one already put back.
	was, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if Digest(was) != os.Getenv(StagedEnv) {
		return fmt.Errorf("%s does not hold the bytes the row was staged over, so its receipt is not written", file)
	}
	if err := os.WriteFile(file, body, 0o600); err != nil {
		return err
	}
	return os.Setenv(TakenEnv, "1")
}

// Digest is how StagedEnv spells a file's bytes: SHA-256, in lowercase hex.
func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
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
