// SPDX-License-Identifier: 0BSD

package release

import (
	"errors"
	"strings"
	"testing"
)

func TestVersionHasExactlyOneNativeHome(t *testing.T) {
	got, err := version([]byte("package toolkit\n\nconst Version = \"3.0.0\"\n"))
	if err != nil || got != "3.0.0" {
		t.Fatalf("version = %q, %v", got, err)
	}
	for _, body := range []string{"package toolkit\n", "const Version = \"3.0.0\"\nconst Version = \"3.0.1\"\n"} {
		if _, err := version([]byte(body)); err == nil || !strings.Contains(err.Error(), "exactly one") {
			t.Fatalf("ambiguous source was accepted: %v", err)
		}
	}
}

func TestOnlyThisRepositoryCanBeAReleaseRemote(t *testing.T) {
	at := "@"
	for _, raw := range []string{
		"https://github.com/Azathothas/ToolKit.git",
		"ssh://git" + at + "github.com/Azathothas/ToolKit.git",
		"git" + at + "github.com:Azathothas/ToolKit.git",
	} {
		if got := remoteRepository(raw); got != releaseRepository {
			t.Errorf("remoteRepository(%q) = %q", raw, got)
		}
	}
	for _, raw := range []string{
		"https://github.com/someone/ToolKit.git",
		"https://token" + at + "github.com/Azathothas/ToolKit.git",
		"git" + at + "example.com:Azathothas/ToolKit.git",
		"../ToolKit",
	} {
		if onlyReleaseRemoteURLs(raw) {
			t.Errorf("the release remote %q was accepted", raw)
		}
	}
	if !onlyReleaseRemoteURLs("https://github.com/Azathothas/ToolKit.git\n" +
		"git" + at + "github.com:Azathothas/ToolKit.git\n") {
		t.Error("two URLs for this repository were refused")
	}
}

func TestTheLiveRemoteMainMustBeExactlyHead(t *testing.T) {
	if !isReleaseBranch("main\n") || isReleaseBranch("feature/release") || isReleaseBranch("") {
		t.Error("the release branch predicate did not select main alone")
	}
	head := strings.Repeat("1", 40)
	other := strings.Repeat("2", 40)
	body := head + "\trefs/heads/main\n" + other + "\trefs/heads/other\n"
	if got := refSHA(body, "refs/heads/main"); got != head {
		t.Fatalf("main resolved to %q", got)
	}
	if got := refSHA(body, "refs/heads/missing"); got != "" {
		t.Fatalf("an absent branch resolved to %q", got)
	}
}

type fakeGit struct {
	replies map[string]struct {
		out string
		err error
	}
	calls []string
}

func (f *fakeGit) Git(args ...string) (string, error) {
	key := strings.Join(args, "\x00")
	f.calls = append(f.calls, key)
	r := f.replies[key]
	return r.out, r.err
}

func gitReply(out string, err error) struct {
	out string
	err error
} {
	return struct {
		out string
		err error
	}{out: out, err: err}
}

func TestAPushErrorIsReconciledAgainstTheRemoteBeforeRollback(t *testing.T) {
	remote, tag, head := "origin", "wsl-toolkit-v3.0.0", strings.Repeat("a", 40)
	read := strings.Join([]string{"ls-remote", "--tags", remote, "refs/tags/" + tag, "refs/tags/" + tag + "^{}"}, "\x00")
	deleteTag := strings.Join([]string{"tag", "-d", tag}, "\x00")

	accepted := &fakeGit{replies: map[string]struct {
		out string
		err error
	}{read: gitReply(strings.Repeat("b", 40)+"\trefs/tags/"+tag+"\n"+head+"\trefs/tags/"+tag+"^{}\n", nil)}}
	if published, err := reconcilePush(accepted, remote, tag, head, errors.New("connection closed")); err != nil || !published {
		t.Fatalf("an accepted tag was not reconciled: published=%v err=%v", published, err)
	}
	if len(accepted.calls) != 1 {
		t.Fatalf("an accepted remote tag triggered another mutation: %v", accepted.calls)
	}

	absent := &fakeGit{replies: map[string]struct {
		out string
		err error
	}{read: gitReply("", nil), deleteTag: gitReply("", nil)}}
	if published, err := reconcilePush(absent, remote, tag, head, errors.New("rejected")); err == nil || published || !strings.Contains(err.Error(), "removed again") {
		t.Fatalf("an absent remote tag was not rolled back locally: published=%v err=%v", published, err)
	}
	if len(absent.calls) != 2 || absent.calls[1] != deleteTag {
		t.Fatalf("the proved-absent tag was not the one local mutation: %v", absent.calls)
	}

	unknown := &fakeGit{replies: map[string]struct {
		out string
		err error
	}{read: gitReply("", errors.New("network unavailable"))}}
	if published, err := reconcilePush(unknown, remote, tag, head, errors.New("connection closed")); err == nil || published || !strings.Contains(err.Error(), "kept") {
		t.Fatalf("unknown remote state was treated as safe to roll back: published=%v err=%v", published, err)
	}
	if len(unknown.calls) != 1 {
		t.Fatalf("unknown remote state triggered a mutation: %v", unknown.calls)
	}
}
