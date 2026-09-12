package toolkit

import (
	"bytes"
	"context"
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
}

// gitIndexTimeout bounds the one external call. ⚠ A repository large enough to
// be slow here is a repository whose copy is slower still, so the bound is
// generous rather than tight.
const gitIndexTimeout = 60 * time.Second

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
	cmd := exec.CommandContext(ctx, git, "-C", root, "ls-files", "-s", "-z")
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
		return plural(b.fromGit, "file") + carries(b.fromGit) + " the executable bit from the git index"
	case b.fromGit == 0:
		return plural(b.fromHdr, "file") + carries(b.fromHdr) + " the executable bit from a shebang, and " + isAre(b.fromHdr) + " in no git index"
	}
	return plural(b.fromGit, "file") + carries(b.fromGit) + " the executable bit from the git index, and " +
		plural(b.fromHdr, "file") + " from a shebang"
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
	f, err := os.Open(absPath)
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
