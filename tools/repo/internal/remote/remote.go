// Package remote answers what is open against a repository, and whether any of
// it says something that survives being checked.
//
// The defect it exists to catch is a change accepted on the strength of its own
// description. A bot's pull request title says what it believes it is doing. A
// contributor's issue says what they believe is wrong. Both are CLAIMS, and both
// are usually right, which is exactly what makes the wrong one expensive: nobody
// is looking by the hundredth bump.
//
// ⭐ THIS WAS PAID FOR ON THIS REPOSITORY, TWICE, IN ONE HOUR.
//  1. `actions/checkout` was pinned to v4, and v4 targets Node 20, which GitHub
//     had deprecated. The runs were being force-migrated with a warning in a log
//     nobody reads. Resolving a tag is not the same as checking what it declares.
//  2. The replacement pin was v5, chosen by looking only at v5 and v4. v7
//     already existed. A tag resolving cleanly says nothing about whether it is
//     current.
//
// ⛔ IT IS READ ONLY. It never merges, never closes, never comments, never
// approves. Deciding is the operator's.
//
// ⚠ IT CANNOT TELL YOU WHETHER A CHANGE IS A GOOD IDEA. It checks the facts an
// item asserts about the world. Whether you want the change is a reading.
//
// SPDX-License-Identifier: 0BSD
package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Schema versions the structured answer.
const Schema = "check-remote-items/1"

// Options are the flags this tool takes.
type Options struct {
	JSON bool
	Repo string
}

// Report is the verdict, and the two modes compute it from this one value.
//
// ⛔ THE TWO MODES REPORTED DIFFERENT VERDICTS ONCE: text exited 1 whenever
// anything needed a reading and json exited 0 over the same tree, so a gate
// runner saw green where a person saw red. Both shell twins carried it, so
// check-twins compared them and passed. One value, one exit expression.
type Report struct {
	Schema     string `json:"schema"`
	Problems   int    `json:"problems"`
	NeedsHuman int    `json:"needs_human"`
	OpenPRs    int    `json:"open_prs"`
}

// Verdict is the exit code.
//
// ⚠ AN UNREAD ITEM IS NOT A FAILED CHECK, and this used to exit 1 for one. Any
// repository with an open issue was then permanently red, which is how a check
// stops being read: the one state it cannot report is the one it exists for. An
// item needing a reading is counted, named, and exits 0. Only a claim that was
// checked and did not hold exits 1.
func (r Report) Verdict() int {
	if r.Problems > 0 {
		return 1
	}
	return 0
}

type reporter struct {
	out    io.Writer
	report Report
}

func (r *reporter) note(format string, a ...any) { fmt.Fprintf(r.out, "  "+format+"\n", a...) }
func (r *reporter) bad(format string, a ...any) {
	fmt.Fprintf(r.out, "  ⛔ "+format+"\n", a...)
	r.report.Problems++
}
func (r *reporter) human(format string, a ...any) {
	fmt.Fprintf(r.out, "  ⚠ "+format+"\n", a...)
	r.report.NeedsHuman++
}

type issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
}

type pull struct {
	issue
	Files []struct {
		Path string `json:"path"`
	} `json:"files"`
}

// Run reads the repository and returns the exit code.
//
// Exit codes: 0 nothing open, or nothing open failed a check; 1 an item's claim
// did not survive checking; 2 could not run.
func Run(opts Options, out, errOut io.Writer) int {
	gh, err := exec.LookPath("gh")
	if err != nil {
		fmt.Fprintln(errOut, "check-remote-items: gh not found")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c := &client{gh: gh, repo: opts.Repo, ctx: ctx}
	if _, err := c.raw("auth", "status"); err != nil {
		fmt.Fprintln(errOut, "check-remote-items: gh is not authenticated")
		return 2
	}

	// ⛔ IN JSON MODE, STDOUT IS RESERVED FOR THE DOCUMENT. The report still
	// goes out, on stderr, where a person reading a terminal sees it and a gate
	// runner reading stdout does not. It used to print the whole human report to
	// stdout first, so `check | jq` failed to parse and every other check in
	// this directory was machine-readable while this one was not.
	human := out
	if opts.JSON {
		human = errOut
	}
	r := &reporter{out: human, report: Report{Schema: Schema}}

	fmt.Fprintln(human, "\nOPEN ISSUES")
	var issues []issue
	if err := c.list(&issues, "issue", "number,title,author,createdAt"); err != nil {
		fmt.Fprintf(errOut, "check-remote-items: could not list issues: %s\n", err)
		return 2
	}
	if len(issues) == 0 {
		r.note("none")
	} else {
		for _, i := range issues {
			r.note("#%d [%s] %s", i.Number, i.Author.Login, i.Title)
		}
		// ⚠ Reported, not judged. An issue is a person's account of a problem
		// and nothing here can verify it. What this can do is stop one going
		// unnoticed.
		r.human("%d open issue(s). Read them; nothing here can verify a report.", len(issues))
	}

	fmt.Fprintln(human, "\nOPEN PULL REQUESTS")
	var prs []pull
	if err := c.list(&prs, "pr", "number,title,author,headRefName,files"); err != nil {
		fmt.Fprintf(errOut, "check-remote-items: could not list pull requests: %s\n", err)
		return 2
	}
	r.report.OpenPRs = len(prs)
	if len(prs) == 0 {
		r.note("none")
	}
	for _, pr := range prs {
		fmt.Fprintf(human, "\n  #%d [%s] %s\n", pr.Number, pr.Author.Login, pr.Title)
		diff, err := c.raw("pr", "diff", fmt.Sprint(pr.Number))
		if err != nil {
			r.human("#%d: could not read the diff", pr.Number)
			continue
		}
		pins := PinsAdded(diff)
		if len(pins) == 0 {
			var paths []string
			for _, f := range pr.Files {
				paths = append(paths, f.Path)
			}
			r.note("touches: %s", strings.Join(paths, ", "))
			r.human("#%d: nothing mechanically checkable here. Read it.", pr.Number)
			continue
		}
		for _, p := range pins {
			checkPin(c, r, human, p)
		}
	}

	fmt.Fprintln(human, "")
	switch {
	case r.report.Problems > 0:
		fmt.Fprintf(human, "⛔ %d claim(s) did not survive checking. Do not merge on the description.\n", r.report.Problems)
	case r.report.NeedsHuman > 0:
		fmt.Fprintf(human, "⚠ %d item(s) need a reading. Nothing failed a check; nothing was verified either.\n", r.report.NeedsHuman)
	default:
		fmt.Fprintln(human, "✅ every mechanically checkable claim held.")
		fmt.Fprintln(human, "⚠ That is not approval. Whether you want a change is a reading, not a check.")
	}

	if opts.JSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(r.report); err != nil {
			fmt.Fprintf(errOut, "check-remote-items: %s\n", err)
			return 2
		}
	}
	return r.report.Verdict()
}

// Pin is one pinned action a diff proposes.
type Pin struct {
	Action string // owner/name
	SHA    string // the 40 hex characters
	Tag    string // the trailing comment's label, empty when there is none
}

// pinRE matches a pinned action and the trailing comment beside it.
//
// ⛔ The comment is captured too, because a pin whose LABEL disagrees with it is
// its own defect: the comment drifts and a reader trusts the comment.
var pinRE = regexp.MustCompile(`uses:[ \t]*([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)@([0-9a-f]{40})(?:[ \t]*#[ \t]*(\S+))?`)

// PinsAdded finds every action pin the diff ADDS.
//
// ⚠ ADDED LINES ONLY. A pin the diff removes is not a claim this change is
// making, and checking it would report a defect the change is fixing.
func PinsAdded(diff string) []Pin {
	var out []Pin
	seen := map[string]bool{}
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			continue
		}
		for _, m := range pinRE.FindAllStringSubmatch(line, -1) {
			p := Pin{Action: m[1], SHA: m[2], Tag: m[3]}
			key := p.Action + "@" + p.SHA + "#" + p.Tag
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, p)
		}
	}
	return out
}

func checkPin(c *client, r *reporter, human io.Writer, p Pin) {
	label := p.Tag
	if label == "" {
		label = "no label"
	}
	fmt.Fprintf(human, "    %s@%s  (labelled %s)\n", p.Action, short(p.SHA), label)

	// 1. does the commit exist, and in THAT repository?
	if _, err := c.api("repos/" + p.Action + "/commits/" + p.SHA); err != nil {
		r.bad("%s@%s does not exist in that repository. A pin naming a commit the repo does not have is not a bump.", p.Action, p.SHA)
		return
	}
	r.note("    commit exists in %s", p.Action)

	// 2. does the label resolve to that same commit?
	if p.Tag == "" {
		r.human("    no tag comment beside the pin. A bare SHA tells a reader nothing.")
	} else {
		switch sha, err := c.tagSHA(p.Action, p.Tag); {
		case err != nil || sha == "":
			r.human("    the label %s is not a tag in %s", p.Tag, p.Action)
		case sha != p.SHA:
			r.bad("the label says %s but that tag is %s, not the pinned commit. The comment has drifted from the pin.", p.Tag, short(sha))
		default:
			r.note("    label %s matches the pin", p.Tag)
		}
	}

	// 3. ⭐ what runtime does the PINNED COMMIT declare? This is the check the
	//    Node 20 deprecation got past.
	body, err := c.file(p.Action, p.SHA, "action.yml")
	if err != nil {
		body, err = c.file(p.Action, p.SHA, "action.yaml")
	}
	if err != nil {
		r.human("    could not read action.yml at that commit; runtime unverified")
	} else {
		switch rt := DeclaredRuntime(body); rt {
		case "":
			r.human("    could not read action.yml at that commit; runtime unverified")
		case "node12", "node16", "node20":
			r.bad("it declares %s, which GitHub has deprecated. It will run under a forced newer runtime, with a warning nobody reads, until it does not.", rt)
		case "node24", "docker", "composite":
			r.note("    runtime: %s", rt)
		default:
			r.human("    runtime: %s (unrecognised; check it)", rt)
		}
	}

	// 4. is anything newer already out?
	latest, err := c.latestRelease(p.Action)
	switch {
	case err != nil || latest == "":
	case p.Tag != "" && latest != p.Tag:
		r.human("    %s is already released; this proposes %s", latest, p.Tag)
	default:
		r.note("    %s is the latest release", latest)
	}
}

var usingRE = regexp.MustCompile(`(?m)^[ \t]*using:[ \t]*(.+?)[ \t]*$`)

// DeclaredRuntime reads the `using:` value out of an action's manifest.
//
// ⚠ THE VALUE MAY BE QUOTED, AND THE CALLER MATCHES BARE WORDS. `using: "node24"`
// is valid YAML and real actions write it that way: astral-sh/setup-uv does.
// The shell version's capture kept its quotes, so a quoted "node20" matched no
// arm, fell through to the catch-all, and was reported as "unrecognised; check
// it" instead of the refusal this whole check exists to raise. A deprecated
// runtime evaded the one rule written for it by being spelled the other legal
// way. Found by running the check against a real third-party pull request.
func DeclaredRuntime(manifest string) string {
	inRuns := false
	for _, line := range strings.Split(manifest, "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if strings.HasPrefix(trimmed, "runs:") {
			inRuns = true
			continue
		}
		if inRuns && len(trimmed) > 0 && trimmed[0] != ' ' && trimmed[0] != '\t' {
			break
		}
		if !inRuns {
			continue
		}
		if m := usingRE.FindStringSubmatch(trimmed); m != nil {
			return strings.Trim(strings.TrimSpace(m[1]), `"'`)
		}
	}
	return ""
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// client is the one place gh is invoked, so every value is stripped of the
// carriage return gh emits on Windows in exactly one place.
//
// ⛔ gh ON WINDOWS EMITS CRLF, and a carriage return riding on a value is
// invisible until something types it. It made a pull request number read as
// "1\r", and the diff fetch for it failed with no useful message.
type client struct {
	gh   string
	repo string
	ctx  context.Context
}

func (c *client) raw(args ...string) (string, error) {
	if c.repo != "" && len(args) > 0 && args[0] != "auth" {
		args = append(args, "--repo", c.repo)
	}
	cmd := exec.CommandContext(c.ctx, c.gh, args...)
	out, err := cmd.Output()
	return strings.ReplaceAll(string(out), "\r", ""), err
}

func (c *client) list(into any, kind, fields string) error {
	out, err := c.raw(kind, "list", "--state", "open", "--limit", "50", "--json", fields)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(out), into)
}

func (c *client) api(path string, extra ...string) (string, error) {
	args := append([]string{"api", path}, extra...)
	cmd := exec.CommandContext(c.ctx, c.gh, args...)
	out, err := cmd.Output()
	return strings.ReplaceAll(string(out), "\r", ""), err
}

// tagSHA resolves a tag to the commit it names.
//
// ⚠ AN ANNOTATED TAG POINTS AT A TAG OBJECT, not at a commit, so one hop is not
// enough. Most released actions use annotated tags, which is precisely the case
// a one-hop resolver gets wrong.
func (c *client) tagSHA(action, tag string) (string, error) {
	out, err := c.api("repos/" + action + "/git/ref/tags/" + tag)
	if err != nil {
		return "", err
	}
	var ref struct {
		Object struct {
			SHA  string `json:"sha"`
			Type string `json:"type"`
		} `json:"object"`
	}
	if err := json.Unmarshal([]byte(out), &ref); err != nil {
		return "", err
	}
	if ref.Object.Type != "tag" {
		return ref.Object.SHA, nil
	}
	out, err = c.api("repos/" + action + "/git/tags/" + ref.Object.SHA)
	if err != nil {
		return "", err
	}
	var obj struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return "", err
	}
	return obj.Object.SHA, nil
}

// file reads one path at one commit.
//
// ⭐ THROUGH gh, not through a bare HTTPS fetch. The shell version used curl
// against raw.githubusercontent.com, which carries no credential, so a private
// action or a rate-limited runner read as "runtime unverified" rather than as
// what it declares. gh already holds the token this needs.
func (c *client) file(action, ref, path string) (string, error) {
	return c.api("repos/"+action+"/contents/"+path,
		"-H", "Accept: application/vnd.github.raw", "-f", "ref="+ref)
}

func (c *client) latestRelease(action string) (string, error) {
	out, err := c.api("repos/" + action + "/releases/latest")
	if err != nil {
		return "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal([]byte(out), &rel); err != nil {
		return "", err
	}
	return rel.TagName, nil
}
