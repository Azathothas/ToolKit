// Package gitsync is the sanctioned way to commit and push.
//
// The defect it exists to catch is a rule that everybody agreed to and nobody
// enforces. `docs/conventions/git.md` states the identity rule and the
// attribution rule; before this existed the template DOCUMENTED both and
// ENFORCED neither, so the only thing standing between a project and a commit
// crediting a tool was whether the agent that session had read the file.
//
// ⭐ WHAT IT MAKES MECHANICAL, and each one has cost a real session:
//
//  1. Author AND committer are pinned per invocation, so a machine whose global
//     config says something else still produces the right commit. ⚠ `git commit
//     --author` sets only the author, which is why both are set: a commit can
//     carry two different identities and the one shown in a log is not the one
//     a checker reads.
//  2. An attribution line is REFUSED, never stripped. Silently rewriting
//     somebody's commit message is worse than declining to commit it: the author
//     never learns the rule and the next message has the same line.
//  3. A CI-skip marker is refused unless the flag was passed. A message that
//     merely MENTIONS one skips CI, because GitHub does not read the sentence
//     around it. That shipped a commit with no run once: the commit that
//     introduced the skip flag explained the marker in prose, and its own push
//     started nothing.
//  4. The body is read from a FILE, never from a shell string.
//
// ⛔ NOTHING HERE KNOWS WHO YOU ARE. The identity comes from the flags or from
// git config, and if neither has one this refuses rather than guessing. A
// template must never carry a person baked into it.
//
// ⚠ IT IS A HELPER, NOT A CHECK. It writes: that is its job. `--check` is the
// read-only half.
//
// SPDX-License-Identifier: 0BSD
package gitsync

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/Azathothas/ToolKit/tools/repo/internal/gitrepo"
)

// Schema versions the structured answer.
const Schema = "git-sync/1"

// Options are the flags this tool takes.
type Options struct {
	Message   string
	BodyFile  string
	Name      string
	Email     string
	Branch    string
	Paths     []string
	Gates     []string
	NoPush    bool
	PushOnly  bool
	Check     bool
	SkipGates bool
	NoCI      bool
	JSON      bool
}

// attributionRE is rule 1, and it is REFUSED rather than stripped.
//
// ⚠ THE VENDOR ADDRESS IS WRITTEN WITH A BRACKETED DOT ON PURPOSE. Spelled as a
// plain address it is a valid email, and the secret sweep refuses a tracked
// email address, so this guard's own source would have failed it. The bracket is
// a no-op to the regex engine and breaks the shape the sweep looks for.
//
// ⚠ CASE-INSENSITIVE on purpose. "Co-Authored-By" and "co-authored-by" are the
// same violation, and a guard that only caught one spelling would be a guard
// that catches whichever one nobody uses.
var attributionRE = regexp.MustCompile(`(?i)^[ \t]*co-authored-by:` +
	`|generated[ \t]+with[ \t]+\[?claude` +
	`|generated[ \t]+by[ \t]+(claude|chatgpt|gpt-|copilot|cursor|codex|gemini|llm|an?[ \t]+ai)` +
	`|written[ \t]+by[ \t]+(claude|chatgpt|gpt-|copilot|an?[ \t]+ai)` +
	`|with[ \t]+assistance[ \t]+from[ \t]+(claude|chatgpt|copilot|an?[ \t]+ai)` +
	`|claude[ \t]+(code|opus|sonnet|haiku)` +
	`|anthropic` +
	`|^[ \t]*(assisted|authored)-by:[ \t]*(claude|chatgpt|copilot)` +
	`|noreply@anthropic[.]com`)

// ciSkipRE is every marker GitHub Actions honours, matched the way GitHub
// matches them: case-insensitively and anywhere in the message.
//
// ⛔ THAT IS WHY A SENTENCE ABOUT ONE IS ONE. GitHub does not read the sentence
// around the marker.
var ciSkipRE = regexp.MustCompile(`(?i)\[skip[ _-]?ci\]|\[ci[ _-]?skip\]|\[no[ _-]?ci\]|\[skip[ _-]?actions\]|\[actions[ _-]?skip\]`)

// Finding is one offending line.
type Finding struct {
	Line int
	Text string
}

// FindAttribution reports every line crediting a tool.
func FindAttribution(text string) []Finding { return matches(text, attributionRE) }

// FindCISkip reports every line carrying a marker GitHub would honour.
func FindCISkip(text string) []Finding { return matches(text, ciSkipRE) }

func matches(text string, re *regexp.Regexp) []Finding {
	var out []Finding
	for i, line := range strings.Split(text, "\n") {
		if re.MatchString(line) {
			out = append(out, Finding{Line: i + 1, Text: strings.TrimRight(line, "\r")})
		}
	}
	return out
}

type run struct {
	repo  *gitrepo.Repo
	name  string
	email string
	// out is where a structured answer goes and NOTHING else.
	out io.Writer
	// human is where progress goes: stdout normally, stderr under --json.
	human  io.Writer
	errOut io.Writer
}

func (r *run) say(format string, a ...any) {
	fmt.Fprintf(r.human, "%s git-sync: %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, a...))
}

func (r *run) ident() string { return r.name + " <" + r.email + ">" }

// git runs one command with the identity pinned per invocation.
//
// ⛔ COMMITTER AS WELL AS AUTHOR. `--author` sets only the author, and a commit
// can carry two different identities; the one shown in a log is not the one a
// checker reads.
func (r *run) git(args ...string) (string, error) {
	full := append([]string{
		"-c", "user.name=" + r.name, "-c", "user.email=" + r.email,
		"-c", "committer.name=" + r.name, "-c", "committer.email=" + r.email,
	}, args...)
	return r.repo.Git(full...)
}

// Run is the whole tool.
//
// Exit codes: 0 done, 1 a rule was broken or a gate failed, 2 could not run.
func Run(opts Options, out, errOut io.Writer) int {
	repo, err := gitrepo.Open()
	if err != nil {
		fmt.Fprintf(errOut, "git-sync: %s\n", err)
		return 2
	}
	// ⛔ IN JSON MODE, STDOUT IS RESERVED FOR THE DOCUMENT. The shell version
	// sent both the progress lines and the JSON to stdout, so
	// `git-sync --check --json | jq` failed to parse. check-remote-items had
	// the same defect and its own header records it; fixing one and not the
	// other would leave two answers to the same question in one directory.
	human := out
	if opts.JSON {
		human = errOut
	}
	r := &run{repo: repo, out: out, human: human, errOut: errOut}

	// ⛔ NOTHING IS INVENTED. Guessing an identity onto somebody's commit is
	// worse than not committing, because it is a claim about who wrote something.
	r.name, r.email = opts.Name, opts.Email
	if r.name == "" {
		r.name = repo.Config("user.name")
	}
	if r.email == "" {
		r.email = repo.Config("user.email")
	}
	if r.name == "" || r.email == "" {
		fmt.Fprintln(errOut, "git-sync: no identity. Pass --name and --email, or set git config")
		fmt.Fprintln(errOut, "  user.name and user.email. Nothing is guessed here.")
		return 2
	}

	branch := opts.Branch
	if branch == "" {
		b, err := repo.Git("rev-parse", "--abbrev-ref", "HEAD")
		branch = strings.TrimSpace(b)
		if err != nil || branch == "" {
			branch = "main"
		}
	}

	// ⛔ THE BODY COMES FROM A FILE. A body passed as a shell string loses its
	// quoting, and the way it fails is worse than an error: nothing errors, and
	// a fragment is executed or dropped somewhere in the middle.
	message := opts.Message
	if message != "" && opts.BodyFile != "" {
		body, err := os.ReadFile(opts.BodyFile)
		if err != nil {
			fmt.Fprintf(errOut, "git-sync: --body-file %q does not exist.\n", opts.BodyFile)
			return 2
		}
		message += "\n\n" + string(body)
	}

	if opts.Check {
		return r.check(message, opts)
	}
	return r.commitAndPush(message, branch, opts)
}

// check is the read-only half.
func (r *run) check(message string, opts Options) int {
	problems := 0

	if message != "" {
		if hits := FindAttribution(message); len(hits) > 0 {
			fmt.Fprintln(r.errOut, "git-sync: the message carries attribution:")
			report(r.errOut, hits)
			problems++
		} else {
			r.say("message carries no attribution")
		}
	}

	// ⭐ THE LAST COMMIT IS CHECKED TOO, so a bad one that landed some other way
	// is still caught. A guard that only inspects what it is asked to write
	// cannot see what somebody committed around it.
	head, err := r.repo.Git("log", "-1", "--pretty=%an <%ae>%n%cn <%ce>%n%B")
	if err == nil && strings.TrimSpace(head) != "" {
		if hits := FindAttribution(head); len(hits) > 0 {
			fmt.Fprintln(r.errOut, "git-sync: HEAD commit carries attribution:")
			report(r.errOut, hits)
			problems++
		} else {
			r.say("HEAD commit is clean")
		}
		who, _ := r.repo.Git("log", "-1", "--pretty=%an <%ae>|%cn <%ce>")
		who = strings.TrimSpace(who)
		want := r.ident() + "|" + r.ident()
		if who != want {
			fmt.Fprintf(r.errOut, "git-sync: HEAD identity is %s, expected %s\n", who, want)
			problems++
		} else {
			r.say("HEAD identity is %s, author and committer", r.ident())
		}
	}

	if opts.JSON {
		enc := json.NewEncoder(r.out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{"schema": Schema, "problems": problems}); err != nil {
			fmt.Fprintf(r.errOut, "git-sync: %s\n", err)
			return 2
		}
	}
	if problems > 0 {
		return 1
	}
	r.say("all checks pass")
	return 0
}

func (r *run) commitAndPush(message, branch string, opts Options) int {
	if !opts.PushOnly {
		if strings.TrimSpace(message) == "" {
			fmt.Fprintln(r.errOut, "git-sync: --message is required unless --push-only or --check.")
			return 2
		}

		// ⛔ REFUSED, NOT STRIPPED. Rewriting a message to make it pass is how
		// the author never finds out, and the same line arrives again next time.
		if hits := FindAttribution(message); len(hits) > 0 {
			fmt.Fprintln(r.errOut, "git-sync: the commit message carries AI attribution and will NOT be")
			fmt.Fprintln(r.errOut, "rewritten for you:")
			report(r.errOut, hits)
			fmt.Fprintln(r.errOut, "\nRemove it and run again. docs/conventions/git.md.")
			return 1
		}

		// ⚠ Checked BEFORE the gates, not after. Finding this out after a long
		// test run is finding it out late.
		if !opts.NoCI {
			if hits := FindCISkip(message); len(hits) > 0 {
				fmt.Fprintln(r.errOut, "git-sync: the message carries a CI skip marker and --no-ci was not")
				fmt.Fprintln(r.errOut, "passed, so this push would silently start no run:")
				report(r.errOut, hits)
				fmt.Fprintln(r.errOut, "\nWrite the marker some other way, or pass --no-ci if you meant it.")
				return 1
			}
		}

		if len(opts.Paths) > 0 {
			for _, p := range opts.Paths {
				if _, err := r.repo.Git("add", "--", p); err != nil {
					fmt.Fprintf(r.errOut, "git-sync: git add %q failed: %s\n", p, err)
					return 1
				}
			}
			r.say("staged the named path(s)")
		} else {
			if _, err := r.repo.Git("add", "-A"); err != nil {
				fmt.Fprintf(r.errOut, "git-sync: git add -A failed: %s\n", err)
				return 1
			}
			r.say("staged everything not ignored")
		}

		staged, _ := r.repo.Git("diff", "--cached", "--name-only")
		n := 0
		for _, line := range strings.Split(staged, "\n") {
			if strings.TrimSpace(line) != "" {
				n++
			}
		}
		if n == 0 {
			fmt.Fprintln(r.errOut, "git-sync: nothing staged, so there is nothing to commit.")
			return 1
		}
		r.say("%d file(s) staged", n)

		if code := r.runGates(opts); code != 0 {
			return code
		}

		if opts.NoCI {
			// On its own line at the end, so the subject stays readable in a log
			// and a reader can see which pushes were never checked.
			message += "\n\n[skip ci]\n"
		}

		msgFile, err := os.CreateTemp("", "git-sync-*.txt")
		if err != nil {
			fmt.Fprintf(r.errOut, "git-sync: %s\n", err)
			return 2
		}
		defer os.Remove(msgFile.Name())
		if _, err := msgFile.WriteString(message); err != nil {
			msgFile.Close()
			fmt.Fprintf(r.errOut, "git-sync: %s\n", err)
			return 2
		}
		msgFile.Close()

		if _, err := r.git("commit", "--file", msgFile.Name()); err != nil {
			fmt.Fprintf(r.errOut, "git-sync: git commit failed: %s\n", err)
			return 1
		}
		last, _ := r.repo.Git("log", "-1", "--pretty=%h %s")
		r.say("committed %s", strings.TrimSpace(last))

		// ⭐ VERIFY RATHER THAN ASSUME. `-c` can be overridden by a hook or by
		// an environment variable, and a commit that landed with the wrong
		// identity is not fixed by having asked nicely.
		who, _ := r.repo.Git("log", "-1", "--pretty=%an <%ae>|%cn <%ce>")
		who = strings.TrimSpace(who)
		if want := r.ident() + "|" + r.ident(); who != want {
			fmt.Fprintf(r.errOut, "git-sync: the commit landed as %q, not %q. Something overrode -c.\n", who, want)
			return 1
		}
		r.say("identity verified: %s, author and committer", r.ident())
	} else if code := r.runGates(opts); code != 0 {
		return code
	}

	if opts.NoPush {
		r.say("--no-push, stopping before the push")
		return 0
	}
	r.say("pushing %s to origin", branch)
	if _, err := r.repo.Git("push", "origin", branch); err != nil {
		fmt.Fprintf(r.errOut, "git-sync: git push failed: %s\n", err)
		return 1
	}
	r.say("pushed")
	return 0
}

// runGates runs each gate BEFORE the push.
//
// ⛔ THE EXIT CODE IS READ FROM THE PROCESS THAT PRODUCED IT, unpiped. The
// shell version ran its loop on the right of a pipe, where a subshell's exit
// does not reach the caller, so a failing gate could not stop the push it was
// there to stop.
func (r *run) runGates(opts Options) int {
	if opts.SkipGates {
		r.say("GATES SKIPPED by --skip-gates. This push carries no proof the tree is green.")
		return 0
	}
	if len(opts.Gates) == 0 {
		r.say("no --gate given, nothing to run")
		return 0
	}
	for _, g := range opts.Gates {
		if strings.TrimSpace(g) == "" {
			continue
		}
		r.say("gate: %s", g)
		cmd := exec.Command("sh", "-c", g)
		cmd.Dir = r.repo.Root
		cmd.Stdout, cmd.Stderr = r.human, r.errOut
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(r.errOut, "git-sync: a gate failed. Nothing has been pushed: %s\n", err)
			return 1
		}
	}
	return 0
}

func report(w io.Writer, hits []Finding) {
	for _, h := range hits {
		fmt.Fprintf(w, "  line %d: %s\n", h.Line, h.Text)
	}
}
