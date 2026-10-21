package toolkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ⛔ NTFS CARRIES NO POSIX MODE BIT, so every file in a Windows checkout arrives
// in a Linux container at 0644 and every script in it is unrunnable. Measured by
// a consumer of this tool on 2026-09-11: 396 of 396 scripts had to be repaired
// inside the job, and the first failure reads
// `./scripts/common/bootstrap-env.sh: Permission denied`, which names the script
// and not the transfer. That consumer carries a `restore-modes.sh` and a wrapper
// to call it, which is the class of workaround issue 29 exists to remove.
//
// ⭐ THE GIT INDEX IS THE ONLY RECORD OF WHICH FILES ARE MEANT TO RUN on a host
// whose filesystem cannot hold it. `git ls-files -s` reports mode 100755 for
// each one, and that is what this reads.
//
// ⚠ THE INDEX IS NOT THE WHOLE ANSWER, and the same consumer measured the gap
// three weeks later: a NEW script, written and not yet staged, is in no index,
// so the repair reported 396 of 396 fixed in the same run that failed with
// `Permission denied`, rc 126. A shebang closes that one. ⛔ It is NOT
// `chmod -R +x`, which marks data executable and reports nothing: a file whose
// first two bytes are `#!` is a script by its own declaration, and the count of
// files promoted this way is reported rather than applied in silence.

// execBits answers which workspace files must arrive executable.
//
// ⚠ It never fails a job. A workspace with no git in it, or no git on PATH, is
// an ordinary workspace, and the shebang pass still covers the scripts in it.
type execBits struct {
	index   map[string]bool // slash-relative paths the git index marks 100755
	fromGit int
	fromHdr int
	// from names where index came from, for the report. Empty means the git
	// index; an untrusted tree's comes from the archive that wrote it.
	from string
}

// gitIndexTimeout bounds the one external call. ⚠ A repository large enough to
// be slow here is a repository whose copy is slower still, so the bound is
// generous rather than tight.
const gitIndexTimeout = 60 * time.Second

// gitArgs are the arguments every git call here starts with.
//
// ⛔ core.fsmonitor IS OFF ON EVERY CALL. A repository's own configuration
// names a program, and git ran it on `ls-files -s`, on `check-ignore` and on
// `ls-files --others --ignored`: measured with git 2.55 by a safety review. A
// .git the caller did not write is how a job or a sandboxed client would run a
// program on this machine. A tree a container or a helper client wrote is never
// given to git at all; this is the second guard, for the caller's own.
func gitArgs(root string, args ...string) []string {
	return append([]string{"-c", "core.fsmonitor=false", "-C", root}, args...)
}

// newExecBits reads the git index for root. An error is never returned: an
// unreadable index means an empty set, which degrades to the shebang pass.
func newExecBits(ctx context.Context, root string) *execBits {
	b := &execBits{index: map[string]bool{}}
	git, err := exec.LookPath("git")
	if err != nil {
		return b
	}
	ctx, cancel := context.WithTimeout(ctx, gitIndexTimeout)
	defer cancel()
	// -z, because a path with a newline or a quote in it is reported escaped
	// and re-quoted without it, and the escaped form is not a path.
	cmd := exec.CommandContext(ctx, git, gitArgs(root, "ls-files", "-s", "-z")...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return b
	}
	for _, rec := range strings.Split(out.String(), "\x00") {
		// "100755 <sha> <stage>\t<path>"
		tab := strings.IndexByte(rec, '\t')
		if tab < 0 || !strings.HasPrefix(rec, "100755 ") {
			continue
		}
		b.index[rec[tab+1:]] = true
	}
	return b
}

// mode answers the tar member mode for one regular file.
//
// ⭐ THE HOST'S OWN BIT WINS WHERE IT EXISTS. This code also runs where the
// filesystem does carry a mode, and overriding a real bit with a guess would be
// worse than the problem.
func (b *execBits) mode(hostMode os.FileMode, slashRel, absPath string) int64 {
	if hostMode&0o111 != 0 {
		return 0o755
	}
	if b.index[slashRel] {
		b.fromGit++
		return 0o755
	}
	if hasShebang(absPath) {
		b.fromHdr++
		return 0o755
	}
	return 0o644
}

// report is the line a caller reads, or empty when nothing was restored.
func (b *execBits) report() string {
	switch {
	case b.fromGit == 0 && b.fromHdr == 0:
		return ""
	case b.fromHdr == 0:
		return plural(b.fromGit, "file") + carries(b.fromGit) + " the executable bit from " + b.source()
	case b.fromGit == 0:
		return plural(b.fromHdr, "file") + carries(b.fromHdr) + " the executable bit from a shebang, and " + isAre(b.fromHdr) + " in no " + strings.TrimPrefix(b.source(), "the ")
	}
	return plural(b.fromGit, "file") + carries(b.fromGit) + " the executable bit from " + b.source() + ", and " +
		plural(b.fromHdr, "file") + " from a shebang"
}

// source is where index came from, as the report names it.
func (b *execBits) source() string {
	if b.from != "" {
		return b.from
	}
	return "the git index"
}

// carries agrees the verb with the count, because a report that reads as
// broken English reads as a tool that is guessing.
func carries(n int) string {
	if n == 1 {
		return " carries"
	}
	return " carry"
}

// isAre agrees the second verb in the shebang sentence with its count.
func isAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// hasShebang reads the first two bytes of a file.
//
// ⚠ An unreadable file answers false rather than failing. The walker opens the
// same file immediately afterwards and reports the real error there, with the
// path in it.
func hasShebang(absPath string) bool {
	f, err := os.Open(deviceSafePath(absPath))
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 2)
	n, _ := f.Read(head)
	return n == 2 && head[0] == '#' && head[1] == '!'
}

// isGitWorkspace reports whether root is inside a git working tree, which is
// what makes the index worth asking for.
func isGitWorkspace(root string) bool {
	for dir := filepath.Clean(root); ; {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// gitIgnored answers the slash-relative paths git ignores under root, a
// directory with a trailing slash, for a workspace copy that leaves them out.
//
// ⭐ GIT DECIDES, NOT A SECOND READER OF .gitignore. A project's ignore rules
// nest, negate and anchor, and a second implementation of them is the second
// copy that drifts. `--directory` names a wholly ignored directory once rather
// than every file in it. WSL-112.
//
// ⛔ UNLIKE THE INDEX READ ABOVE, THIS FAILS LOUD. A caller who asked for git's
// rules and got none would copy every ignored build tree and be refused for its
// size, or copy a credential git keeps out of the repository.
func gitIgnored(ctx context.Context, root string) (map[string]bool, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil, errors.New("--gitignore reads git's own rules, and no git is on PATH")
	}
	ctx, cancel := context.WithTimeout(ctx, gitIndexTimeout)
	defer cancel()
	// ⛔ GIT ANSWERS WHETHER THIS IS A WORK TREE, not a walk for a `.git` above
	// it. git honours GIT_CEILING_DIRECTORIES, a worktree's `.git` file and
	// GIT_DIR, and a walk does not, so the two could disagree about one directory.
	probe := exec.CommandContext(ctx, git, gitArgs(root, "rev-parse", "--is-inside-work-tree")...)
	if out, err := probe.Output(); err != nil || strings.TrimSpace(string(out)) != "true" {
		return nil, fmt.Errorf("--gitignore reads git's own rules, and %s is not in a git work tree", root)
	}
	// ⛔ A WORKSPACE ITS OWN REPOSITORY IGNORES is refused by name. git's rules
	// leave out all of it, and git ls-files --directory then fails with "internal
	// error - directory entry not superset of prefix", measured with git 2.55 on a
	// directory under this repository's ignored .tmp.
	if exec.CommandContext(ctx, git, gitArgs(root, "check-ignore", "-q", ".")...).Run() == nil {
		return nil, fmt.Errorf("--gitignore reads git's own rules, and %s is itself ignored by the repository it is in, so they would leave out all of it", root)
	}
	cmd := exec.CommandContext(ctx, git, gitArgs(root, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")...)
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git ls-files could not list what git ignores in %s: %w: %s", root, err, firstLine(stderr.String()))
	}
	ignored := map[string]bool{}
	for _, rec := range strings.Split(out.String(), "\x00") {
		if rec != "" {
			ignored[rec] = true
		}
	}
	return ignored, nil
}
