// Package release verifies and tags the native wsl-toolkit product.
//
// SPDX-License-Identifier: 0BSD
package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Azathothas/ToolKit/tools/repo/internal/gitrepo"
)

const (
	Schema            = "repo-release/1"
	versionFile       = "tools/windows/wsl-toolkit/internal/toolkit/version.go"
	releaseRepository = "github.com/azathothas/toolkit"
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

type gitRunner interface {
	Git(args ...string) (string, error)
}

func remoteRepository(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			return ""
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "https" && scheme != "ssh" {
			return ""
		}
		// ⛔ A CREDENTIAL IN A REMOTE URL WOULD REACH GIT'S ERROR TEXT. Release
		// remotes use the credential helper or SSH agent, never embedded userinfo.
		if scheme == "https" && u.User != nil {
			return ""
		}
		return cleanRemoteRepository(u.Hostname(), u.Path)
	}
	colon := strings.IndexByte(raw, ':')
	if colon <= 0 || strings.ContainsAny(raw[:colon], `/\\`) {
		return ""
	}
	host := raw[:colon]
	if at := strings.LastIndexByte(host, '@'); at >= 0 {
		host = host[at+1:]
	}
	return cleanRemoteRepository(host, raw[colon+1:])
}

func cleanRemoteRepository(host, path string) string {
	path = strings.Trim(strings.TrimSpace(path), "/")
	path = strings.TrimSuffix(path, ".git")
	if len(strings.Split(path, "/")) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(host) + "/" + path)
}

func onlyReleaseRemoteURLs(body string) bool {
	lines := strings.Fields(body)
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if remoteRepository(line) != releaseRepository {
			return false
		}
	}
	return true
}

func lineIsListed(body, want string) bool {
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

func isReleaseBranch(body string) bool { return strings.TrimSpace(body) == "main" }

func refSHA(body, ref string) string {
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == ref {
			return fields[0]
		}
	}
	return ""
}

func tagTarget(body, tag string) (string, bool) {
	ref := "refs/tags/" + tag
	if peeled := refSHA(body, ref+"^{}"); peeled != "" {
		return peeled, true
	}
	direct := refSHA(body, ref)
	return direct, direct != ""
}

// reconcilePush distinguishes an error returned before a push from one returned
// after the server accepted it. A local tag is removed only after a successful
// read-back proves the remote has no tag to reconcile.
func reconcilePush(repo gitRunner, remote, tag, head string, pushErr error) (bool, error) {
	body, err := repo.Git("ls-remote", "--tags", remote, "refs/tags/"+tag, "refs/tags/"+tag+"^{}")
	if err != nil {
		return false, fmt.Errorf("could not read %s back from %s after the push; the local tag was kept because remote state is unknown: %w", tag, remote, err)
	}
	if target, exists := tagTarget(body, tag); exists {
		if target == head {
			return true, nil
		}
		return false, fmt.Errorf("%s exists on %s but does not resolve to HEAD; the local tag was kept for reconciliation", tag, remote)
	}
	if pushErr == nil {
		return false, fmt.Errorf("git push exited 0 but %s is absent from %s; the local tag was kept for reconciliation", tag, remote)
	}
	if _, err := repo.Git("tag", "-d", tag); err != nil {
		return false, fmt.Errorf("the push failed and %s is absent from %s, but the local tag could not be removed: %w", tag, remote, err)
	}
	return false, fmt.Errorf("the push failed and %s is absent from %s; the local tag was removed again", tag, remote)
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
	branch, err := repo.Git("branch", "--show-current")
	if err != nil {
		fmt.Fprintf(errOut, "release: could not read the current branch: %v\n", err)
		return 2
	}
	if !isReleaseBranch(branch) {
		report.Problems = append(report.Problems, "the release checkout is not on main")
	}
	head, err := repo.Git("rev-parse", "HEAD")
	if err != nil {
		fmt.Fprintf(errOut, "release: %v\n", err)
		return 2
	}
	head = strings.TrimSpace(head)
	remotes, err := repo.Git("remote")
	if err != nil {
		fmt.Fprintf(errOut, "release: could not list repository remotes: %v\n", err)
		return 2
	}
	remoteOK := lineIsListed(remotes, opts.Remote)
	if !remoteOK {
		report.Problems = append(report.Problems, "the configured remote "+opts.Remote+" does not exist")
	} else {
		fetchURLs, fetchErr := repo.Git("remote", "get-url", "--all", opts.Remote)
		pushURLs, pushErr := repo.Git("remote", "get-url", "--push", "--all", opts.Remote)
		if fetchErr != nil || pushErr != nil {
			fmt.Fprintf(errOut, "release: could not read every URL for %s\n", opts.Remote)
			return 2
		}
		remoteOK = onlyReleaseRemoteURLs(fetchURLs) && onlyReleaseRemoteURLs(pushURLs)
		if !remoteOK {
			report.Problems = append(report.Problems, "every fetch and push URL for "+opts.Remote+" must name github.com/Azathothas/ToolKit and carry no embedded credential")
		}
	}
	if remoteOK {
		remoteMain, err := repo.Git("ls-remote", "--heads", opts.Remote, "refs/heads/main")
		if err != nil {
			fmt.Fprintf(errOut, "release: could not prove HEAD is on %s/main: %v\n", opts.Remote, err)
			return 2
		}
		if refSHA(remoteMain, "refs/heads/main") != head {
			report.Problems = append(report.Problems, "HEAD is not the live "+opts.Remote+"/main, so CI cannot check it out from the release remote")
		}
	}
	if report.Tag != "" {
		if local, err := repo.Git("tag", "--list", report.Tag); err != nil {
			fmt.Fprintf(errOut, "release: %v\n", err)
			return 2
		} else if strings.TrimSpace(local) != "" {
			report.Problems = append(report.Problems, "tag "+report.Tag+" already exists locally; bump Version instead of moving it")
		}
		if remoteOK {
			remote, err := repo.Git("ls-remote", "--tags", opts.Remote, "refs/tags/"+report.Tag)
			if err != nil {
				fmt.Fprintf(errOut, "release: could not prove the remote tag is free: %v\n", err)
				return 2
			}
			if strings.TrimSpace(remote) != "" {
				report.Problems = append(report.Problems, "tag "+report.Tag+" already exists on "+opts.Remote+"; bump Version instead of moving it")
			}
		}
	}

	report.Ready = len(report.Problems) == 0
	if report.Ready && opts.Publish {
		if _, err := repo.Git("tag", "-a", report.Tag, "-m", "wsl-toolkit "+report.Version); err != nil {
			fmt.Fprintf(errOut, "release: creating %s: %v\n", report.Tag, err)
			return 2
		}
		_, pushErr := repo.Git("push", opts.Remote, "refs/tags/"+report.Tag)
		published, err := reconcilePush(repo, opts.Remote, report.Tag, head, pushErr)
		if err != nil {
			fmt.Fprintf(errOut, "release: %v\n", err)
			return 2
		}
		if pushErr != nil {
			fmt.Fprintf(errOut, "release: git push returned an error, but remote read-back proves %s resolves to HEAD\n", report.Tag)
		}
		report.Published = published
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
