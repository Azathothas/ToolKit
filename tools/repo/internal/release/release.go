// Package release verifies and tags the native wsl-toolkit product.
//
// SPDX-License-Identifier: 0BSD
package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Azathothas/ToolKit/tools/repo/internal/gitrepo"
)

const (
	Schema      = "repo-release/1"
	versionFile = "tools/windows/wsl-toolkit/internal/toolkit/version.go"
)

var versionPattern = regexp.MustCompile(`(?m)^const Version = "([0-9]+\.[0-9]+\.[0-9]+)"$`)

type Options struct {
	Publish bool
	Remote  string
	JSON    bool
}

type Report struct {
	Schema    string   `json:"schema"`
	Product   string   `json:"product"`
	Version   string   `json:"version,omitempty"`
	Tag       string   `json:"tag,omitempty"`
	Remote    string   `json:"remote"`
	Ready     bool     `json:"ready"`
	Published bool     `json:"published"`
	Problems  []string `json:"problems,omitempty"`
}

func version(body []byte) (string, error) {
	matches := versionPattern.FindAllSubmatch(body, -1)
	if len(matches) != 1 {
		return "", fmt.Errorf("%s carries %d product version declarations and exactly one is required", versionFile, len(matches))
	}
	return string(matches[0][1]), nil
}

func command(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

func verifyTree(root string) error {
	var name string
	var args []string
	if runtime.GOOS == "windows" {
		name, args = "pwsh", []string{"-NoProfile", "-NonInteractive", "-File", filepath.Join(root, "scripts", "common", "check-gate.ps1")}
	} else {
		name, args = "sh", []string{filepath.Join(root, "scripts", "common", "check-gate.sh")}
	}
	out, err := command(root, name, args...)
	if err != nil {
		if out == "" {
			out = err.Error()
		}
		return fmt.Errorf("the repository gate did not pass: %s", out)
	}
	return nil
}

// Run performs every read-only refusal before optionally creating one tag.
func Run(opts Options, out, errOut io.Writer) int {
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	report := Report{Schema: Schema, Product: "wsl-toolkit", Remote: opts.Remote}
	repo, err := gitrepo.Open()
	if err != nil {
		fmt.Fprintf(errOut, "release: %v\n", err)
		return 2
	}
	body, err := repo.Read(versionFile)
	if err != nil {
		fmt.Fprintf(errOut, "release: %v\n", err)
		return 2
	}
	report.Version, err = version(body)
	if err != nil {
		report.Problems = append(report.Problems, err.Error())
	} else {
		report.Tag = "wsl-toolkit-v" + report.Version
	}

	if err := verifyTree(repo.Root); err != nil {
		report.Problems = append(report.Problems, err.Error())
	}
	if dirty, err := repo.Dirty(); err != nil {
		fmt.Fprintf(errOut, "release: git status: %v\n", err)
		return 2
	} else if dirty {
		report.Problems = append(report.Problems, "the working tree is not clean, so a release would match no commit")
	}
	head, err := repo.Git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintf(errOut, "release: %v\n", err)
		return 2
	}
	remoteBranches, err := repo.Git("branch", "-r", "--contains", strings.TrimSpace(head))
	if err != nil {
		fmt.Fprintf(errOut, "release: could not prove HEAD is remote: %v\n", err)
		return 2
	}
	if strings.TrimSpace(remoteBranches) == "" {
		report.Problems = append(report.Problems, "HEAD is not on any remote branch, so CI cannot check it out")
	}
	if report.Tag != "" {
		if local, err := repo.Git("tag", "--list", report.Tag); err != nil {
			fmt.Fprintf(errOut, "release: %v\n", err)
			return 2
		} else if strings.TrimSpace(local) != "" {
			report.Problems = append(report.Problems, "tag "+report.Tag+" already exists locally; bump Version instead of moving it")
		}
		remote, err := repo.Git("ls-remote", "--tags", opts.Remote, "refs/tags/"+report.Tag)
		if err != nil {
			fmt.Fprintf(errOut, "release: could not prove the remote tag is free: %v\n", err)
			return 2
		}
		if strings.TrimSpace(remote) != "" {
			report.Problems = append(report.Problems, "tag "+report.Tag+" already exists on "+opts.Remote+"; bump Version instead of moving it")
		}
	}

	report.Ready = len(report.Problems) == 0
	if report.Ready && opts.Publish {
		if _, err := repo.Git("tag", "-a", report.Tag, "-m", "wsl-toolkit "+report.Version); err != nil {
			fmt.Fprintf(errOut, "release: creating %s: %v\n", report.Tag, err)
			return 2
		}
		if _, err := repo.Git("push", opts.Remote, "refs/tags/"+report.Tag); err != nil {
			_, removeErr := repo.Git("tag", "-d", report.Tag)
			if removeErr != nil {
				fmt.Fprintf(errOut, "release: pushing %s failed and its local rollback also failed: %v; %v\n", report.Tag, err, removeErr)
				return 2
			}
			fmt.Fprintf(errOut, "release: pushing %s failed; the local tag was removed again: %v\n", report.Tag, err)
			return 2
		}
		report.Published = true
	}

	if opts.JSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(errOut, "release: %v\n", err)
			return 2
		}
	} else {
		fmt.Fprintf(out, "wsl-toolkit %s -> %s\n", report.Version, report.Tag)
		for _, problem := range report.Problems {
			fmt.Fprintln(errOut, "  ! "+problem)
		}
		switch {
		case report.Published:
			fmt.Fprintf(out, "published %s to %s; release.yml takes it from here\n", report.Tag, report.Remote)
		case report.Ready:
			fmt.Fprintf(out, "ready. Nothing was tagged or pushed. Publish with: repo release --publish --remote %s\n", report.Remote)
		default:
			fmt.Fprintln(errOut, "  ! nothing was tagged")
		}
	}
	if len(report.Problems) > 0 {
		return 1
	}
	return 0
}
