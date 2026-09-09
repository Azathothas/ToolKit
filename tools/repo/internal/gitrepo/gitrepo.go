// Package gitrepo answers the few questions every tool here starts with: where
// the repository is, what is in it, and whether the tree is clean.
//
// ⛔ ONE IMPLEMENTATION, because the shell halves each had their own. A tool
// that listed tracked files and one that listed tracked plus untracked were
// both correct and answered differently, and neither said which it was doing.
//
// SPDX-License-Identifier: 0BSD
package gitrepo

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNotARepository is what every caller reports as "could not run" rather than
// as a disagreement. ⚠ A tool that cannot find the tree checked NOTHING, and
// exiting 0 there is the worst answer any of these can give.
var ErrNotARepository = errors.New("not a git repository")

// Repo is one checkout.
type Repo struct {
	Root string
	git  string
}

// Open finds the repository containing the working directory.
func Open() (*Repo, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git was not found on PATH: %w", err)
	}
	out, err := run(git, "", "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, ErrNotARepository
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return nil, ErrNotARepository
	}
	// ⚠ EvalSymlinks, because a checkout reached through a link answers with the
	// link's path and every path this then builds would disagree with the one a
	// caller typed.
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	return &Repo{Root: root, git: git}, nil
}

// Git runs one git command in the repository and returns its stdout.
func (r *Repo) Git(args ...string) (string, error) { return run(r.git, r.Root, args...) }

// Files is every file this repository holds: tracked, plus untracked that is
// not ignored.
//
// ⛔ BOTH LISTS. A file that has never been staged is exactly when a new one is
// most likely to be the one a tool should see, and the gate's own port lost a
// planted credential to a loader that read only the tracked list.
func (r *Repo) Files() ([]string, error) {
	tracked, err := r.Git("ls-files", "-z")
	if err != nil {
		return nil, err
	}
	untracked, err := r.Git("ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, chunk := range []string{tracked, untracked} {
		for _, name := range strings.Split(chunk, "\x00") {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Dirty reports whether anything is uncommitted.
func (r *Repo) Dirty() (bool, error) {
	out, err := r.Git("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// Config reads one git configuration value, empty when it is unset.
//
// ⚠ AN UNSET KEY IS NOT AN ERROR. `git config` exits 1 for it, and a caller
// that treated that as a failure would refuse on a machine that simply has not
// been configured yet, instead of saying which key to set.
func (r *Repo) Config(key string) string {
	out, err := r.Git("config", "--get", key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// Read returns one file's bytes, relative to the repository root.
func (r *Repo) Read(rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(rel)))
}

func run(git, dir string, args ...string) (string, error) {
	cmd := exec.Command(git, args...)
	cmd.Dir = dir
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
	}
	return out.String(), nil
}
