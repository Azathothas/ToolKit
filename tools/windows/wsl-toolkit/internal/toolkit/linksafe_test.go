// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// -- WSL-201: root writes nothing in the account's home -------------------------------
//
// Every owner's `base ensure` runs each adapter's install.sh as root, and provisioning
// and engines.sh run as root too. chown, install -d, cp and a redirect each follow a
// link the account placed in its home. Where base.passwordless_sudo is off, or in a
// herdr pane under private_net, the account is not root, so a link it planted turned
// into root's write or root's chown. Every write in the home is now the account's.

// -- the class, read from the scripts ----------------------------------------------------

// shCommand is one simple command of a shell script, as rootHomeWrites reads it. It
// holds its words with the quotes taken off, the files its redirections name, the
// here-documents it is fed, its pipeline, its waiver and the line it starts on.
// ⚠ A $ that a single quote or a backslash makes literal is kept as a NUL, so it never
// reads as a variable. A ~ that starts a word unquoted is kept as \x01, so a quoted one
// never reads as a home.
type shCommand struct {
	line  int
	words []string
	outs  []string
	ins   []string
	docs  []*shDoc
	// pipe is shared by the commands one pipeline joins.
	pipe int
	// pattern is a case pattern, which runs nothing.
	pattern bool
	// waiver is the id of the `# linksafe:` comment that stands for this command, or 0.
	waiver int
	// scope is the shell the command runs in: 0 for the script, or a subshell's id.
	scope int
}

// shDoc is a here-document's body. A quoted delimiter keeps it as written.
type shDoc struct {
	body   string
	quoted bool
}

// shWaiver is one `# linksafe: REASON` comment: the line it stands on and its reason.
// ⭐ It stands for every command that starts on the next line holding one, or on its own
// line when it ends one, substitutions included. So a root step the scan cannot judge
// is reviewed where it is written.
type shWaiver struct {
	line   int
	reason string
}

// shScript is a script read into its commands and its waivers. A waiver's id is its
// index in waivers, plus one.
type shScript struct {
	cmds    []shCommand
	waivers []shWaiver
	pipes   int
	// parents holds the shell each subshell runs inside, by its id. The script's own is -1.
	parents []int
}

// subshell answers a new subshell's id, inside the shell parent.
func (s *shScript) subshell(parent int) int {
	s.parents = append(s.parents, parent)
	return len(s.parents) - 1
}

// The kinds of word a redirection takes.
const (
	shWord = iota
	shOut
	shIn
	shDrop
)

// shBreaks end a word that is not quoted, and a here-document's delimiter.
const shBreaks = " \t\n;&|<>()"

// shCommands reads src into its simple commands. A command substitution's own commands
// are read as commands too.
func shCommands(src string, line int) *shScript {
	s := &shScript{parents: []int{-1}}
	s.read(src, line, 0, 0)
	return s
}

// read reads src, which starts on line in the shell scope, and gives each command it
// finds the waiver inherit unless a comment of its own stands for it. ⭐ A ( opens a
// subshell and a substitution runs in one, so a cd in either ends where it closes.
func (s *shScript) read(src string, line, inherit, scope int) {
	s.pipes++
	cur := shCommand{line: line, pipe: s.pipes, waiver: inherit}
	scopes := []int{scope}
	var word strings.Builder
	inWord := false
	redirect := shWord
	type heredoc struct {
		doc   *shDoc
		delim string
		strip bool
	}
	var pending []heredoc
	// waiver waits for the next line that holds a command. lineWaiver stands for every
	// command that starts on lineWaiverAt.
	waiver, lineWaiver, lineWaiverAt := 0, 0, -1
	depth := 0
	flush := func() {
		if !inWord {
			return
		}
		w := word.String()
		word.Reset()
		inWord = false
		switch redirect {
		case shOut:
			cur.outs = append(cur.outs, w)
		case shIn:
			cur.ins = append(cur.ins, w)
		case shDrop:
		default:
			cur.words = append(cur.words, w)
		}
		redirect = shWord
	}
	end := func(piped bool) {
		flush()
		if len(cur.words) > 0 || len(cur.outs) > 0 || len(cur.ins) > 0 || len(cur.docs) > 0 {
			switch {
			case cur.waiver != inherit:
			case lineWaiver != 0 && cur.line == lineWaiverAt:
				cur.waiver = lineWaiver
			case waiver != 0:
				cur.waiver = waiver
				lineWaiver, lineWaiverAt, waiver = waiver, cur.line, 0
			}
			cur.scope = scopes[len(scopes)-1]
			s.cmds = append(s.cmds, cur)
		}
		if !piped {
			s.pipes++
		}
		cur = shCommand{line: line, pipe: s.pipes, waiver: inherit}
	}
	// sub is the waiver a substitution read now inherits.
	sub := func() int {
		switch {
		case waiver != 0:
			return waiver
		case lineWaiver != 0 && cur.line == lineWaiverAt:
			return lineWaiver
		}
		return cur.waiver
	}
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\\' && i+1 < len(src):
			if src[i+1] == '\n' {
				line++
			} else if src[i+1] == '$' {
				word.WriteByte(0)
				inWord = true
			} else {
				word.WriteByte(src[i+1])
				inWord = true
			}
			i += 2
		case c == '\'':
			j := strings.IndexByte(src[i+1:], '\'')
			if j < 0 {
				j = len(src) - i - 1
			}
			body := src[i+1 : i+1+j]
			word.WriteString(strings.ReplaceAll(body, "$", "\x00"))
			line += strings.Count(body, "\n")
			inWord = true
			i += j + 2
		case c == '"':
			j := shDoubleEnd(src, i+1)
			body := src[i+1 : j]
			if j > i+1 && src[j-1] == '"' {
				body = src[i+1 : j-1]
			}
			for _, inner := range shSubstitutions(body) {
				s.read(inner, line, sub(), s.subshell(scopes[len(scopes)-1]))
			}
			word.WriteString(strings.ReplaceAll(body, `\$`, "\x00"))
			line += strings.Count(body, "\n")
			inWord = true
			i = j
		case c == '$' && i+1 < len(src) && src[i+1] == '(':
			j := shParenEnd(src, i+2)
			inner := src[i+2 : j]
			if j > i+2 && src[j-1] == ')' {
				inner = src[i+2 : j-1]
			}
			// ⚠ ARITHMETIC IS NOT A COMMAND, and its words are numbers and names.
			if !strings.HasPrefix(inner, "(") {
				s.read(inner, line, sub(), s.subshell(scopes[len(scopes)-1]))
			}
			word.WriteString(src[i:j])
			line += strings.Count(inner, "\n")
			inWord = true
			i = j
		case c == '#' && !inWord:
			k := strings.IndexByte(src[i:], '\n')
			if k < 0 {
				k = len(src) - i
			}
			text := strings.TrimSpace(strings.TrimLeft(src[i:i+k], "#"))
			if reason, ok := strings.CutPrefix(text, "linksafe:"); ok {
				s.waivers = append(s.waivers, shWaiver{line: line, reason: strings.TrimSpace(reason)})
				id := len(s.waivers)
				if len(cur.words) > 0 || inWord {
					// ⭐ AT A LINE'S END, IT STANDS FOR EVERY COMMAND THAT STARTS ON IT.
					cur.waiver = id
					lineWaiver, lineWaiverAt = id, cur.line
					for k := len(s.cmds) - 1; k >= 0 && s.cmds[k].line == cur.line; k-- {
						if s.cmds[k].waiver == inherit {
							s.cmds[k].waiver = id
						}
					}
				} else {
					waiver = id
				}
			}
			i += k
		case c == '\n':
			end(false)
			line++
			i++
			for _, h := range pending {
				var body []string
				for i < len(src) {
					k := strings.IndexByte(src[i:], '\n')
					text := src[i:]
					if k >= 0 {
						text = src[i : i+k]
						i += k + 1
					} else {
						i = len(src)
					}
					line++
					if h.strip {
						text = strings.TrimLeft(text, "\t")
					}
					if text == h.delim {
						break
					}
					body = append(body, text)
				}
				h.doc.body = strings.Join(body, "\n")
			}
			pending = nil
			cur.line = line
		case c == ' ' || c == '\t':
			flush()
			i++
		case c == '|':
			if i+1 < len(src) && src[i+1] == '|' {
				end(false)
				i += 2
				continue
			}
			end(true)
			i++
		case c == ';' || c == '&':
			end(false)
			i++
		case c == '(':
			end(false)
			depth++
			scopes = append(scopes, s.subshell(scopes[len(scopes)-1]))
			i++
		case c == ')':
			if depth > 0 {
				// The command before it ran in the subshell, which closes after it.
				end(false)
				depth--
				scopes = scopes[:len(scopes)-1]
			} else {
				// ⭐ A ) THAT CLOSES NOTHING ENDS A CASE PATTERN, every alternative of it.
				flush()
				cur.pattern = true
				for k := len(s.cmds) - 1; k >= 0 && s.cmds[k].pipe == cur.pipe; k-- {
					s.cmds[k].pattern = true
				}
			}
			end(false)
			i++
		case c == '>' || c == '<':
			// ⚠ A word of digits right before it is the descriptor, not a word.
			if inWord && strings.Trim(word.String(), "0123456789") == "" {
				word.Reset()
				inWord = false
			} else {
				flush()
			}
			if c == '<' && i+1 < len(src) && src[i+1] == '<' {
				i += 2
				strip := false
				if i < len(src) && src[i] == '-' {
					strip = true
					i++
				}
				for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
					i++
				}
				start := i
				for i < len(src) && strings.IndexByte(shBreaks, src[i]) < 0 {
					i++
				}
				raw := src[start:i]
				doc := &shDoc{quoted: strings.ContainsAny(raw, `'"\`)}
				cur.docs = append(cur.docs, doc)
				pending = append(pending, heredoc{doc, strings.NewReplacer("'", "", `"`, "", `\`, "").Replace(raw), strip})
				continue
			}
			i++
			kind := shIn
			if c == '>' {
				kind = shOut
				if i < len(src) && (src[i] == '>' || src[i] == '|') {
					i++
				}
				if i < len(src) && src[i] == '&' {
					kind = shDrop
					i++
				}
			} else if i < len(src) && src[i] == '>' {
				// ⛔ <> OPENS ITS FILE FOR WRITING, and makes it.
				kind = shOut
				i++
			} else if i < len(src) && src[i] == '&' {
				kind = shDrop
				i++
			}
			for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
				i++
			}
			redirect = kind
		case c == '~' && !inWord:
			word.WriteByte(1)
			inWord = true
			i++
		default:
			word.WriteByte(c)
			inWord = true
			i++
		}
	}
	end(false)
}

// shDoubleEnd answers the index just past the double quote that closes the string
// opened just before i.
func shDoubleEnd(s string, i int) int {
	for i < len(s) {
		switch {
		case s[i] == '\\':
			i += 2
		case s[i] == '"':
			return i + 1
		case s[i] == '$' && i+1 < len(s) && s[i+1] == '(':
			i = shParenEnd(s, i+2)
		default:
			i++
		}
	}
	return len(s)
}

// shParenEnd answers the index just past the parenthesis that closes the one opened
// just before i.
func shParenEnd(s string, i int) int {
	depth := 1
	for i < len(s) {
		switch s[i] {
		case '\\':
			i += 2
		case '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				return len(s)
			}
			i += j + 2
		case '"':
			i = shDoubleEnd(s, i+1)
		case '(':
			depth++
			i++
		case ')':
			depth--
			i++
			if depth == 0 {
				return i
			}
		default:
			i++
		}
	}
	return len(s)
}

// shSubstitutions answers the text of each command substitution in a double-quoted
// string's body.
func shSubstitutions(body string) []string {
	var subs []string
	for i := 0; i < len(body); {
		switch {
		case body[i] == '\\':
			i += 2
		case body[i] == '$' && i+1 < len(body) && body[i+1] == '(':
			j := shParenEnd(body, i+2)
			end := j
			if end > i+2 && body[end-1] == ')' {
				end--
			}
			if inner := body[i+2 : end]; !strings.HasPrefix(inner, "(") {
				subs = append(subs, inner)
			}
			i = j
		default:
			i++
		}
	}
	return subs
}

func shSet(names ...string) map[string]bool {
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	return set
}

var (
	shAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	// shLeads are the words that can come before a command's own word.
	shLeads = shSet("if", "then", "else", "elif", "fi", "do", "done", "while", "until", "!", "{", "}",
		"local", "export", "readonly", "exec")
	// shAccountRoutes run what they are handed as the account. ⚠ Read by their names.
	// runuser and su are routes only by their user word, which shRunsAsAccount reads.
	shAccountRoutes = shSet("as_account", "account_dirs", "account_put", "account_same", "account_env",
		"account_read", "is_omp_wrapper", "first_lines", "write_manager")
	// shReaders write nothing and open no file they are handed: messages, tests, a
	// case, and what reads a path's own name, its link or its size. ⚠ By their names.
	shReaders = shSet("say", "die", "problem", "printf", "echo", "[", "test", ":", "true", "false",
		"return", "exit", "case", "esac", "readlink", "dirname", "basename", "du", "unset", "command -v")
	// shHomeItself may make or give the home itself, whose parent is root's, without
	// going inside it.
	shHomeItself = shSet("chown", "chgrp", "chmod", "mkdir")
	// shConsumers act on what they read: as paths, as words, or as a script.
	shConsumers = shSet("xargs", "read", "sh", "bash", "dash", "ash", "eval", "source", ".")
)

// shSplit answers a command's assignments, its command word and its arguments.
func shSplit(words []string) (assigns []string, name string, args []string) {
	i := 0
	for ; i < len(words); i++ {
		if shLeads[words[i]] {
			continue
		}
		if shAssignment.MatchString(words[i]) {
			assigns = append(assigns, words[i])
			continue
		}
		break
	}
	if i < len(words) {
		name, args = words[i], words[i+1:]
	}
	return assigns, name, args
}

// shUnwrap answers the command a wrapper runs, and its arguments: env, command, sudo,
// doas, busybox, nice, nohup, timeout, stdbuf, setsid, time and exec, each past its own
// options. `command -v` asks a question and runs nothing.
func shUnwrap(name string, args []string) (string, []string) {
	takes := func(opts map[string]bool) int {
		skip := 0
		for skip < len(args) && strings.HasPrefix(args[skip], "-") {
			if opts[args[skip]] {
				skip++
			}
			skip++
		}
		return skip
	}
	for name != "" {
		skip := 0
		switch path.Base(name) {
		case "env":
			for skip < len(args) {
				a := args[skip]
				if a == "-u" || a == "-C" || a == "-S" {
					skip += 2
					continue
				}
				if !strings.HasPrefix(a, "-") && !shAssignment.MatchString(a) {
					break
				}
				skip++
			}
		case "command":
			if len(args) > 0 && (args[0] == "-v" || args[0] == "-V") {
				return "command -v", nil
			}
			skip = takes(nil)
		case "sudo", "doas":
			skip = takes(shSet("-u", "-g", "-C", "-h", "-p", "-U", "-D", "-r", "-t"))
		case "nice":
			skip = takes(shSet("-n"))
		case "timeout":
			skip = takes(shSet("-s", "-k")) + 1
		case "stdbuf", "setsid", "nohup", "time", "busybox", "exec":
			skip = takes(nil)
		default:
			return name, args
		}
		if skip >= len(args) {
			return "", nil
		}
		name, args = args[skip], args[skip+1:]
	}
	return name, args
}

func shIdent(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// shRefs answers where each reference to the variable name in word ends.
func shRefs(word, name string) []int {
	var ends []int
	for i := strings.IndexByte(word, '$'); i >= 0; {
		rest := word[i+1:]
		switch {
		case strings.HasPrefix(rest, "{"+name+"}"):
			ends = append(ends, i+len(name)+3)
		case strings.HasPrefix(rest, "{"+name) && len(rest) > len(name)+1 && !shIdent(rest[len(name)+1]):
			if j := strings.IndexByte(rest, '}'); j >= 0 {
				ends = append(ends, i+j+2)
			}
		case strings.HasPrefix(rest, name) && (len(rest) == len(name) || !shIdent(rest[len(name)])):
			ends = append(ends, i+len(name)+1)
		}
		next := strings.IndexByte(word[i+1:], '$')
		if next < 0 {
			break
		}
		i += next + 1
	}
	return ends
}

// Where a word names, in the account's home.
const (
	placeNone = iota
	placeHome
	placeInside
)

// shHome is what one script calls the account's home. seeds hold the home itself, and
// inside holds the variables that name a path inside it, or a path the account
// supplied.
type shHome struct{ seeds, inside map[string]bool }

// place answers where word names. ⭐ The home itself is not inside it: its parent is
// root's, so the account cannot put a link in its place. A trailing slash on it, or a
// recursive flag, goes inside.
func (h shHome) place(word string) int {
	best := placeNone
	raise := func(p int) {
		if p > best {
			best = p
		}
	}
	// ~user is that account's home. A ~ alone is root's own.
	if rest, ok := strings.CutPrefix(word, "\x01"); ok {
		if name, _, slash := strings.Cut(rest, "/"); name != "" {
			if slash {
				raise(placeInside)
			}
			raise(placeHome)
		}
	}
	if i := strings.Index(word, "/home/"); i >= 0 {
		if user, _, slash := strings.Cut(word[i+len("/home/"):], "/"); user != "" {
			if slash {
				raise(placeInside)
			}
			raise(placeHome)
		}
	}
	for s := range h.seeds {
		for _, e := range shRefs(word, s) {
			if e < len(word) && (word[e] == '/' || word[e] == '$') {
				raise(placeInside)
			}
			raise(placeHome)
		}
	}
	for v := range h.inside {
		if len(shRefs(word, v)) > 0 {
			raise(placeInside)
		}
	}
	return best
}

// mentions is whether any word of c, a file it redirects or a here-document it expands
// names the home.
func (h shHome) mentions(c shCommand) bool {
	for _, w := range append(append(append([]string(nil), c.words...), c.outs...), c.ins...) {
		if h.place(w) != placeNone {
			return true
		}
	}
	for _, d := range c.docs {
		if !d.quoted && h.place(d.body) != placeNone {
			return true
		}
	}
	return false
}

// shPasswdHome is whether an assignment's value reads the home from the passwd
// database: its sixth field, by cut or by awk.
func shPasswdHome(value string) bool {
	if !strings.Contains(value, "getent passwd") {
		return false
	}
	for _, field := range []string{"-f6", "-f 6", "\x006", "$6"} {
		if strings.Contains(value, field) {
			return true
		}
	}
	return false
}

// shAccountSupplied is whether an assignment's value is what the account answered, a
// path it may have chosen.
func shAccountSupplied(value string) bool {
	if !strings.Contains(value, "$(") {
		return false
	}
	for _, route := range []string{"as_account", "account_env", "account_read", "runuser", "first_lines"} {
		if strings.Contains(value, route) {
			return true
		}
	}
	return false
}

// shHomeNames reads which variables a script's commands make the home, and which a
// path inside it.
func shHomeNames(cmds []shCommand) shHome {
	h := shHome{seeds: map[string]bool{}, inside: map[string]bool{}}
	mark := func(set map[string]bool, name string) bool {
		if set[name] {
			return false
		}
		set[name] = true
		return true
	}
	for changed := true; changed; {
		changed = false
		for _, c := range cmds {
			if len(c.words) >= 3 && c.words[0] == "for" && c.words[2] == "in" {
				for _, w := range c.words[3:] {
					if h.place(w) != placeNone && mark(h.inside, c.words[1]) {
						changed = true
					}
				}
				continue
			}
			assigns, _, _ := shSplit(c.words)
			for _, a := range assigns {
				name, value, _ := strings.Cut(a, "=")
				switch p := h.place(value); {
				case shPasswdHome(value) || p == placeHome:
					changed = mark(h.seeds, name) || changed
				case p == placeInside || shAccountSupplied(value):
					changed = mark(h.inside, name) || changed
				}
			}
		}
	}
	return h
}

// shRunsAsAccount is whether runuser or su, handed args, runs as the account: whether its
// user word is $TK_USER.
//
// ⛔ BY THE USER WORD, NOT BY THE NAME. Trusted by name, `runuser -u root -- chown x
// "$TK_HOME/.config"` passed the scan as the account's own step. A review found it.
func shRunsAsAccount(args []string) bool {
	takes := shSet("-g", "--group", "-G", "--supp-group", "-s", "--shell", "-c", "--command",
		"-w", "--whitelist-environment", "--session-command")
	account := func(w string) bool { return w == "$TK_USER" || w == "${TK_USER}" }
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-u" || a == "--user":
			return i+1 < len(args) && account(args[i+1])
		case strings.HasPrefix(a, "--user="):
			return account(strings.TrimPrefix(a, "--user="))
		case a == "--":
			return false
		case takes[a]:
			i++
		case strings.HasPrefix(a, "-"):
		default:
			return account(a)
		}
	}
	return false
}

// shRelative is whether a word names a path relative to where its shell stands. ⚠ A
// number is not one, nor a word that starts with a variable, a ~ or an option.
func shRelative(w string) bool {
	if w == "" || strings.Trim(w, "0123456789") == "" {
		return false
	}
	switch w[0] {
	case '/', '$', '-', 0, 1:
		return false
	}
	return true
}

// shRecursive is whether a command's arguments ask it to go down a tree.
func shRecursive(args []string) bool {
	for _, a := range args {
		if a == "--recursive" || strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsRune(a, 'R') {
			return true
		}
	}
	return false
}

// judge answers why root's command k of s reaches into the home, or nothing. inHome
// says its shell stands inside the home, where a relative name is a path there.
func (h shHome) judge(s *shScript, k int, inHome bool) []string {
	c := s.cmds[k]
	show := func(w string) string { return strings.NewReplacer("\x00", "$", "\x01", "~").Replace(w) }
	place := func(w string) int {
		if inHome && shRelative(w) {
			return placeInside
		}
		return h.place(w)
	}
	var why []string
	for _, o := range c.outs {
		if place(o) != placeNone {
			why = append(why, "root's redirect onto "+show(o))
		}
	}
	for _, in := range c.ins {
		if place(in) != placeNone {
			why = append(why, "root's read of "+show(in))
		}
	}
	if c.pattern || len(c.words) > 0 && c.words[0] == "for" {
		return why
	}
	_, name, args := shSplit(c.words)
	name, args = shUnwrap(name, args)
	base := ""
	if name != "" {
		base = path.Base(name)
	}
	route := shAccountRoutes[base] || (base == "runuser" || base == "su") && shRunsAsAccount(args)
	for _, d := range c.docs {
		// ⭐ cat WRITES THE TEXT WHERE ITS REDIRECT SAYS, and that redirect is judged above.
		if !d.quoted && h.place(d.body) != placeNone && !route && base != "cat" {
			why = append(why, fmt.Sprintf("root feeds %s a here-document that names the home", show(strings.TrimSpace(name+" "+strings.Join(c.words, " ")))))
		}
	}
	if name == "" || route {
		return why
	}
	// ⚠ A NAME WITHOUT A SLASH IS LOOKED UP ON PATH, wherever the shell stands.
	if h.place(name) != placeNone || strings.Contains(name, "/") && place(name) != placeNone {
		return append(why, "root runs "+show(name)+", a program in the account's home")
	}
	if shConsumers[base] {
		for j, o := range s.cmds {
			if j != k && o.pipe == c.pipe && h.mentions(o) {
				why = append(why, fmt.Sprintf("root's %s reads its pipeline, which names the home", show(name)))
				break
			}
		}
	}
	worst := placeNone
	for _, a := range args {
		worst = max(worst, place(a))
	}
	if worst == placeHome && shHomeItself[base] && !shRecursive(args) {
		worst = placeNone
	}
	if worst != placeNone && !shReaders[base] {
		why = append(why, fmt.Sprintf("root's %s %s", show(name), show(strings.Join(args, " "))))
	}
	return why
}

// rootHomeWrites answers each step a root script takes inside the account's home: a
// command handed a path there, a redirect onto one or from one, a here-document or a
// pipeline that names one and feeds a command that acts on it, and a program there
// run. ⭐ A command an account route runs is the account's. Its redirect is still root's,
// because root's shell opens that file. A command a waiver stands for is left out, and
// stale answers each waiver that stood for nothing the scan refuses.
//
// ⛔ A cd INTO THE HOME PUTS ITS SHELL THERE UNTIL THAT SHELL CLOSES, so a relative name
// is a path in the home. A waiver on the cd silenced every later step in the subshell,
// and `chown -R agent .` after a waived `cd -P "$TK_HOME/.config"` passed. A review
// found it.
func rootHomeWrites(src string) (found, stale []string, h shHome) {
	s := shCommands(src, 1)
	h = shHomeNames(s.cmds)
	used := map[int]bool{}
	// at holds where each shell's cd left it: in the home or not. A shell no cd has
	// moved stands where the shell around it does.
	at := map[int]bool{}
	inHome := func(scope int) bool {
		for ; scope >= 0; scope = s.parents[scope] {
			if v, ok := at[scope]; ok {
				return v
			}
		}
		return false
	}
	for k, c := range s.cmds {
		here := inHome(c.scope)
		why := h.judge(s, k, here)
		if _, name, args := shSplit(c.words); name == "cd" {
			to := ""
			for _, a := range args {
				if !strings.HasPrefix(a, "-") {
					to = a
					break
				}
			}
			at[c.scope] = h.place(to) != placeNone || here && shRelative(to)
		}
		if len(why) == 0 {
			continue
		}
		if c.waiver != 0 {
			used[c.waiver] = true
			continue
		}
		found = append(found, fmt.Sprintf("line %d: %s", c.line, strings.Join(why, "; ")))
	}
	for i, w := range s.waivers {
		switch {
		case w.reason == "":
			stale = append(stale, fmt.Sprintf("line %d: a linksafe waiver gives no reason", w.line))
		case !used[i+1]:
			stale = append(stale, fmt.Sprintf("line %d: a linksafe waiver stands for nothing the scan refuses", w.line))
		}
	}
	return found, stale, h
}

// rootScripts answers every shell script the executable runs in a base, by its path.
func rootScripts(t *testing.T) map[string][]byte {
	t.Helper()
	scripts := map[string][]byte{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sh") {
			body, err := os.ReadFile(e.Name())
			if err != nil {
				t.Fatal(err)
			}
			scripts[e.Name()] = body
		}
	}
	err = fs.WalkDir(adapterTree, "adapters", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".sh") {
			return err
		}
		body, err := adapterTree.ReadFile(p)
		scripts[p] = body
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return scripts
}

// TestNoRootWriteLandsInTheAccountsHome reads every script the executable runs in a
// base, and refuses a step root takes inside the account's home. WSL-201.
//
// ⛔ THE CLASS, NOT THE SITES IT WAS FOUND AT. Four adapters and the provisioner each
// wrote there as root, and a fifth script would. A path is in the home when it is built
// on the variable a script reads the home into from getent, on `/home/`, on `~user`, on
// a variable built on one of those, or on what the account answered. The home itself
// is root's to make and give.
// ⛔ AN ALLOW-LIST, NOT A LIST OF WRITERS. Every command handed such a path is refused,
// unless the account runs it, it writes nothing and opens nothing it is handed, or a
// `# linksafe:` comment gives the reason it is safe. A list of writers let dd, sed -i,
// tar -C, find -exec and a helper of the script's own through. A waiver that stands
// for nothing is refused too, so none outlives the step it was written for.
func TestNoRootWriteLandsInTheAccountsHome(t *testing.T) {
	scripts := rootScripts(t)
	for name, body := range scripts {
		found, stale, _ := rootHomeWrites(string(body))
		for _, f := range found {
			t.Errorf("%s takes a step in the account's home as root, %s. Take it through as_account, account_dirs, account_put or account_same, or give the reason it is safe in a `# linksafe:` comment", name, f)
		}
		for _, s := range stale {
			t.Errorf("%s: %s", name, s)
		}
	}
	// ⚠ A SCAN THAT READ NO HOME PROVES NOTHING. Each of these reads the home and names
	// paths inside it.
	for _, name := range []string{"provision.sh", "engines.sh", "adapters/herdr/install.sh", "adapters/muse/install.sh",
		"adapters/omp/install.sh", "adapters/pi/install.sh", "adapters/muse/probe.sh", "adapters/omp/probe.sh",
		"adapters/pi/probe.sh"} {
		body, ok := scripts[name]
		if !ok {
			t.Fatalf("%s was not read", name)
		}
		if n := homePaths(string(body)); n == 0 {
			t.Errorf("%s reads as naming no path in the account's home, so the scan of it proved nothing", name)
		}
	}
}

// homePaths counts the words of src that name a path in the account's home.
func homePaths(src string) int {
	s := shCommands(src, 1)
	h := shHomeNames(s.cmds)
	n := 0
	for _, c := range s.cmds {
		for _, w := range append(append([]string(nil), c.words...), c.outs...) {
			if h.place(w) == placeInside {
				n++
			}
		}
	}
	return n
}

// TestTheHomeWriteScanReadsWhatRootWrites holds the scan above to the steps it refuses
// and the ones it lets by. WSL-201.
//
// ⛔ EACH REFUSED LINE IS ONE THE ADAPTERS CARRIED, OR ONE A REVIEW FOUND THE SCAN LET
// BY. A list of writers missed a cd into the home and a relative name, a wrapper before
// the writer, a writer it did not list, a helper writing what it was handed, set --, a
// here-document fed to read, <>, a recursive flag on the home itself, the home read
// from the sixth field another way, `/home/`, ~user, and a path the account supplies.
func TestTheHomeWriteScanReadsWhatRootWrites(t *testing.T) {
	const home = "TK_HOME=$(getent passwd \"$TK_USER\" | cut -d: -f6)\n"
	refused := []string{
		home + `install -d -o "$TK_USER" -g "$TK_GROUP" -m 0700 "$TK_HOME/.config/muse"`,
		home + "OMP_REAL=$TK_HOME/.local/bin/omp-npm\nmv -f \"$x\" \"$OMP_REAL\" || die \"no\"\nchown \"$TK_USER\" \"$OMP_REAL\"",
		home + "trust=$TK_HOME/.config/muse/trust.json\nprintf '{}' > \"$trust\"",
		home + `config_tmp=$(mktemp "$TK_HOME/.config/herdr/config.toml.XXXXXX")`,
		home + "for d in \"$TK_HOME/.config\" \"$TK_HOME/.cache\"; do\n  chmod 0700 \"$d\"\ndone",
		home + "cat > \"$TK_HOME/.local/bin/omp\" <<WRAPPER\nexec omp\nWRAPPER",
		"home=$(getent passwd \"$TK_USER\" 2>/dev/null | cut -d: -f6) || home=\nwrite_manager remove || rm -f \"$home/$MANAGER_REL\"",
		home + "s=$TK_HOME/.pi/agent/settings.json\nif as_account jq . \"$s\" > \"$s.new\"; then :; fi",
		home + "d=\"$(dirname \"$TK_HOME/.omp/agent\")\"\nchown -R \"$TK_USER\" \"$d\"",
		home + "chown -R \"$TK_USER\" \"$TK_HOME/.config\"",
		home + `chown "$TK_USER" "$TK_HOME/.config/"`,
		home + `chown "$TK_USER" "$TK_HOME"/.config`,
		home + `cp -a /tmp/x "$TK_HOME/.config/x"`,
		home + `ln -sf /tmp/x "$TK_HOME/.config/x"`,
		home + `install -m 0600 /tmp/x "$TK_HOME/.config/x"`,
		// The ones a list of writers let by.
		home + "cd \"$TK_HOME\" && chown agent .config",
		home + "( cd \"$TK_HOME\" && chown agent .config )",
		home + `env chown agent "$TK_HOME/.config"`,
		home + `command chown agent "$TK_HOME/.config"`,
		home + `busybox chown agent "$TK_HOME/.config"`,
		home + `sudo -u root chown agent "$TK_HOME/.config"`,
		home + `dd if=/tmp/x of="$TK_HOME/.config/x"`,
		home + `sed -i 's/a/b/' "$TK_HOME/.config/x"`,
		home + `tar -C "$TK_HOME" -xf /tmp/x.tar`,
		home + `find "$TK_HOME/.config" -exec chown agent {} +`,
		home + "printf '%s\\n' \"$TK_HOME/.config\" | xargs chown agent",
		home + `sh -c 'cat > "$1"' sh "$TK_HOME/x"`,
		home + "put() { cp \"$1\" \"$2\"; }\nput /tmp/x \"$TK_HOME/.config/x\"",
		home + "set -- \"$TK_HOME/.config\"\nchown agent \"$1\"",
		home + "while read -r p; do chown agent \"$p\"; done <<EOF\n$TK_HOME/.config\nEOF",
		home + `exec 3<>"$TK_HOME/x"`,
		home + `chown -R agent "$TK_HOME"`,
		"h=$(getent passwd agent | cut -d: -f 6)\nchown agent \"$h/.config\"",
		"h=$(getent passwd agent | awk -F: '{print $6}')\nchown agent \"$h/.config\"",
		`chown agent "/home/$TK_USER/.config"`,
		`chown agent ~agent/.config`,
		home + "d=$(account_env PI_CONFIG_DIR)\nmkdir -p \"$d\"",
		// And what root read in the home, or ran there.
		home + `same=$(sha256sum < "$TK_HOME/.config/x")`,
		home + `head -n 1 "$TK_HOME/.config/x" | grep -q mark`,
		home + `"$TK_HOME/.local/bin/muse" --version`,
		home + "p=$(runuser -l agent -c 'command -v pi')\nhead -3 \"$p\"",
		home + "s=$(as_account mktemp -d /tmp/s.XXXXXX)\nrm -rf \"$s\"",
		// ⛔ runuser and su as root, which the scan trusted by name. A review found it.
		home + `runuser -u root -- chown agent "$TK_HOME/.config"`,
		home + `su -c "chown agent $TK_HOME/.config" root`,
		// ⛔ A relative name after a waived cd into the home, which the waiver silenced.
		home + "( # linksafe: cd -P resolves every link\n  cd -P \"$TK_HOME/.config\" || exit 0\n  chown -R agent .\n)",
		home + "# linksafe: the cd alone is reviewed\ncd \"$TK_HOME\"\nchown agent .config",
		home + "( # linksafe: the cd alone is reviewed\n  cd \"$TK_HOME\" || exit 0\n  cd .config\n  rm -f x\n)",
	}
	for _, src := range refused {
		if found, _, _ := rootHomeWrites(src); len(found) == 0 {
			t.Errorf("the scan let a root step in the home by:\n%s", src)
		}
	}
	allowed := []string{
		home + "chown \"$TK_USER:$TK_GROUP\" \"$TK_HOME\"\nchmod 0700 \"$TK_HOME\"",
		home + "[ -d \"$TK_HOME\" ] || { mkdir -p \"$TK_HOME\"; chown \"$TK_USER\" \"$TK_HOME\"; }",
		home + `as_account mkdir -p "$TK_HOME/.config"`,
		home + `printf x | as_account sh -c 'cat > "$1"' sh "$TK_HOME/.config/x"`,
		home + "cat > /usr/local/bin/omp <<WRAPPER\nexec \"$TK_HOME/.local/bin/omp\" \"\\$@\"\nWRAPPER\nchmod 0755 /usr/local/bin/omp",
		home + `say "wrote $TK_HOME/.config/herdr/config.toml"`,
		home + "t=$(mktemp)\nas_account jq . \"$TK_HOME/.config/x\" > \"$t\" 2>/dev/null",
		home + "f=$TK_HOME/.config/x\naccount_put \"$f\" 0600 < \"$t\" || { rm -f \"$t\"; die \"the account could not write $f\"; }",
		home + "# chown \"$TK_USER\" \"$TK_HOME/.config\" in a comment\n:",
		home + "f=$TK_HOME/.config/x\nif [ -f \"$f\" ] && account_same \"$t\" \"$f\"; then :; fi",
		home + "account_put \"$TK_HOME/.local/bin/omp\" 0755 <<WRAPPER\nexec \"$TK_HOME/.local/bin/omp-npm\"\nWRAPPER",
		home + "real=$(readlink -f \"$TK_HOME/.local/bin/muse\")\nk=$(du -sk \"$TK_HOME/.config/muse\" | cut -f1)",
		home + "case $real in\n  \"$TK_HOME\"/.local/share/*) v=$(dirname \"$real\") ;;\nesac",
		// ⭐ A cd ends where its shell closes: a substitution, or a subshell.
		home + "# linksafe: root enters the home itself and reads where it stands\nh=$(cd -P \"$TK_HOME\" && pwd -P)\nmkdir -p work",
		home + "( # linksafe: cd -P resolves every link, and pwd -P reads it back\n  cd -P \"$TK_HOME/.config\" || exit 0\n" +
			"  # linksafe: `.` is the directory pwd -P read back\n  chown agent .\n)\nmkdir -p work",
		home + "( # linksafe: the cd alone is reviewed\n  cd \"$TK_HOME\" || exit 0\n  cd /tmp\n  rm -f x\n)",
		// ⭐ runuser and su as the account, by their user word.
		home + `runuser -u "$TK_USER" -- chown agent "$TK_HOME/.config"`,
		home + `su -s /bin/sh -c "chown agent $TK_HOME/.config" "$TK_USER"`,
		home + "t=$(mktemp)\naccount_read jq . \"$TK_HOME/.config/x\" > \"$t\"",
		home + "p=$(runuser -l agent -c 'command -v pi')\nfirst_lines 3 \"$p\" | grep -q mark",
		home + "case $ws in '~') ws=$TK_HOME ;; esac",
	}
	for _, src := range allowed {
		if found, stale, _ := rootHomeWrites(src); len(found) != 0 || len(stale) != 0 {
			t.Errorf("the scan refused what root may do, %q %q, in:\n%s", found, stale, src)
		}
	}
	// ⛔ A WAIVER STANDS FOR A STEP THE SCAN REFUSES, AND SAYS WHY. One over nothing, or
	// one with no reason, is refused.
	for _, src := range []string{
		home + "# linksafe: nothing here needs it\nsay \"$TK_HOME\"",
		home + "# linksafe:\ncd \"$TK_HOME\"",
	} {
		if _, stale, _ := rootHomeWrites(src); len(stale) == 0 {
			t.Errorf("the scan kept a waiver it should refuse, in:\n%s", src)
		}
	}
}

// TestTheAccountsOwnWritesAreOneText holds the writers the adapters share to one text,
// and engines.sh's and the provisioner's as_account to one. WSL-201.
//
// ⚠ EACH SCRIPT CARRIES ITS OWN COPY, because each arrives alone on stdin. A copy that
// drifts would hold one adapter to a rule the others no longer keep.
func TestTheAccountsOwnWritesAreOneText(t *testing.T) {
	var first, from string
	for _, name := range []string{"herdr", "muse", "omp", "pi"} {
		body, err := adapterScript(name, "install.sh")
		if err != nil {
			t.Fatal(err)
		}
		block := markedSection(t, body, "the account's own writes")
		if first == "" {
			first, from = block, name
			continue
		}
		if block != first {
			t.Errorf("%s/install.sh's own writes are not %s/install.sh's", name, from)
		}
	}
	if markedSection(t, enginesScript, "as the account") != markedSection(t, provisionScript, "as the account") {
		t.Error("provision.sh's as_account is not engines.sh's")
	}
}

// -- the adapters, run with links planted in the home ------------------------------------

// rootHandTools are root's commands that write a file, jq, which reads one for a write,
// and head and sha256sum, which read one to compare it. rootHands records each call that
// names a path in the account's home, or in the directory outside it that the case's
// links name, as an argument or as its standard input.
var rootHandTools = []string{"chown", "chgrp", "chmod", "install", "mkdir", "mktemp", "mv", "cp", "ln", "rm", "rmdir", "touch", "tee", "jq", "head", "sha256sum"}

// jqLinkStandIn answers each program the adapters run with jq, well enough for a case
// that asks where a file lands. A test answers true, except whether a workspace is
// trusted yet. An edit answers the file it read with one key more, so what the file
// held still shows.
const jqLinkStandIn = `e=
prog=
file=
while [ $# -gt 0 ]; do
  case $1 in
    -e) e=1 ;;
    -r|-c) ;;
    --arg) shift 2 ;;
    *) if [ -z "$prog" ]; then prog=$1; else file=$1; fi ;;
  esac
  shift
done
case $prog in *'.projects | has($w)'*) exit 1 ;; esac
[ -z "$e" ] || exit 0
if [ -f "$file" ]; then
  sed '$ s/}$/,"edited":true}/' "$file"
else
  printf '{"edited":true}\n'
fi
`

// rootHands writes root's hands into bin, which only root's PATH starts with. Each does
// what the command does once it has recorded the call.
//
// ⭐ THE ACCOUNT'S COMMANDS COME FROM ITS OWN PATH, so a step the account takes is not
// recorded. ⚠ chown records and changes nothing, because the host has no account to give
// a file to. install drops its owner flags for the same reason. stop is a path install
// stops the script at with exit 7, as the herdr cases stop it, and jq is jq's body.
// ⛔ A REDIRECT INTO A HAND IS READ BACK FROM /proc, so root's `sha256sum < FILE` in
// the home is recorded as root's read of FILE. WSL-201.
func rootHands(t *testing.T, bin, stop, jq string) {
	t.Helper()
	const record = `for a do
  case $a in
    "$STUB_HOME"/*|"$STUB_OUTSIDE"|"$STUB_OUTSIDE"/*) printf '%s\n' "TOOL $*" >> "$STUB_HANDS"; break ;;
  esac
done
hand_in=$(readlink /proc/$$/fd/0 2>/dev/null) || hand_in=
case $hand_in in
  "$STUB_HOME"/*|"$STUB_OUTSIDE"|"$STUB_OUTSIDE"/*) printf '%s\n' "TOOL < $hand_in" >> "$STUB_HANDS" ;;
esac
`
	for _, tool := range rootHandTools {
		body := "#!/bin/sh\n"
		if tool == "install" && stop != "" {
			body += "case \" $* \" in *\" " + stop + " \"*) echo 'install: stopped here by the case' >&2; exit 7 ;; esac\n"
		}
		body += strings.ReplaceAll(record, "TOOL", tool)
		switch tool {
		case "chown", "chgrp":
			body += "exit 0\n"
		case "jq":
			if jq == "" {
				continue
			}
			body += jq
		default:
			real, err := exec.LookPath(tool)
			if err != nil {
				t.Skipf("%s is not on PATH", tool)
			}
			if tool == "install" {
				body += "skip=\nfor a do\n  shift\n  if [ -n \"$skip\" ]; then skip=; continue; fi\n" +
					"  case $a in -o|-g) skip=1; continue ;; esac\n  set -- \"$@\" \"$a\"\ndone\n"
			}
			body += "exec " + shellQuote(real) + " \"$@\"\n"
		}
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// handsWrote fails the case when root's hands named a path in the home or outside it.
func handsWrote(t *testing.T, hands, out string) {
	t.Helper()
	if b, _ := os.ReadFile(hands); len(b) > 0 {
		t.Fatalf("root wrote or read in the account's home, where only the account does:\n%s\n%s", b, out)
	}
}

// planted is what a link planted in the home names outside it, and what that held.
type planted struct {
	target string
	mode   fs.FileMode
	body   string
	dir    bool
	absent bool
}

// plant makes link name target. A dir target is made with mode, a file target holds body
// with mode, and an absent one is never made.
func plant(t *testing.T, link string, p planted) planted {
	t.Helper()
	for _, d := range []string{filepath.Dir(link), filepath.Dir(p.target)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	switch {
	case p.dir:
		if err := os.MkdirAll(p.target, 0o755); err != nil {
			t.Fatal(err)
		}
	case !p.absent:
		if err := os.WriteFile(p.target, []byte(p.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !p.absent {
		if err := os.Chmod(p.target, p.mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(p.target, link); err != nil {
		t.Fatal(err)
	}
	return p
}

// changed answers how what a planted link names changed: a mode, a byte, or a file that
// was not there. It answers nil when nothing did.
func (p planted) changed() error {
	info, err := os.Lstat(p.target)
	if p.absent {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s was made through the link that named it (%v)", p.target, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s is gone (%v)", p.target, err)
	}
	if info.Mode().Perm() != p.mode || info.IsDir() != p.dir {
		return fmt.Errorf("%s became %v through the link that named it, and it was %v", p.target, info.Mode(), p.mode)
	}
	if !p.dir {
		if b, _ := os.ReadFile(p.target); string(b) != p.body {
			return fmt.Errorf("%s was written through the link that named it: %q", p.target, b)
		}
	}
	return nil
}

// untouched fails the case when what a planted link names changed.
func (p planted) untouched(t *testing.T, out string) {
	t.Helper()
	if err := p.changed(); err != nil {
		t.Fatalf("%v:\n%s", err, out)
	}
}

// replaced fails the case unless path is a regular file now, where a link was planted.
func replaced(t *testing.T, path, out string) {
	t.Helper()
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("%s is not the file the account wrote in the link's place (%v):\n%s", path, err, out)
	}
}

// linkRig is a releaseRig whose root records each write it makes in the home, beside a
// directory outside the home for the links a case plants.
type linkRig struct {
	*releaseRig
	hands, outside string
}

func newLinkRig(t *testing.T, binName, versionPrefix string) *linkRig {
	t.Helper()
	r := &linkRig{releaseRig: newReleaseRig(t, binName, versionPrefix)}
	r.hands = filepath.Join(r.root, "root-hands")
	r.outside = filepath.Join(r.root, "outside")
	rootHands(t, r.bin, "", jqLinkStandIn)
	r.write(filepath.Join(r.accountPath, "jq"), "#!/bin/sh\n"+jqLinkStandIn)
	return r
}

func (r *linkRig) runLinked(script []byte, env ...string) (int, string) {
	r.t.Helper()
	return r.run(script, append([]string{"STUB_OUTSIDE=" + r.outside, "STUB_HANDS=" + r.hands}, env...)...)
}

// TestTheLinkRigSeesAWriterItDoesNotWrap runs root writers the rig has no hand for
// through links planted in a home, and finds each change. WSL-201.
//
// ⛔ THE RIG RUNS ROOT AND THE ACCOUNT AS ONE USER, and records root only through its
// hands. A writer it does not wrap shows only as a change to what a planted link names,
// so this holds that the change is seen: a redirect, an append and dd, and a file made
// through a link to nothing.
// ⚠ A WRITER THAT REPLACES THE LINK, as sed -i does, changes nothing it names. The rig
// cannot tell its file from the account's, and the scan above refuses it.
func TestTheLinkRigSeesAWriterItDoesNotWrap(t *testing.T) {
	needPosix(t, "dd")
	root := t.TempDir()
	home, outside := filepath.Join(root, "home"), filepath.Join(root, "outside")
	for i, writer := range []string{
		`printf 'root\n' > "$1"`,
		`printf 'root\n' >> "$1"`,
		`printf 'root\n' | dd of="$1" conv=notrunc 2>/dev/null`,
	} {
		name := fmt.Sprintf("file-%d", i)
		p := plant(t, filepath.Join(home, name), planted{target: filepath.Join(outside, name), body: "a file outside the home\n", mode: 0o640})
		if out, err := exec.Command("sh", "-c", writer, "sh", filepath.Join(home, name)).CombinedOutput(); err != nil {
			t.Fatalf("%s did not run: %v %s", writer, err, out)
		}
		if p.changed() == nil {
			t.Errorf("the rig did not see root's %s write through a link", writer)
		}
	}
	absent := plant(t, filepath.Join(home, "absent"), planted{target: filepath.Join(outside, "absent"), absent: true})
	if out, err := exec.Command("sh", "-c", `: > "$1"`, "sh", filepath.Join(home, "absent")).CombinedOutput(); err != nil {
		t.Fatalf("the redirect did not run: %v %s", err, out)
	}
	if absent.changed() == nil {
		t.Error("the rig did not see a file root made through a link to nothing")
	}
}

// TestTheOmpAdapterWritesNothingInTheHomeAsRoot runs omp's install.sh on a base that
// separated omp's directory, with links planted where it writes. WSL-201.
//
// ⛔ THE ACCOUNT'S omp IS A LINK TO A FILE OUTSIDE THE HOME, and the separating wrapper
// moved it aside and chowned it as root, which changed the owner of that file. ~/.omp is
// a link to a directory outside, and root's install -d changed its owner and mode.
func TestTheOmpAdapterWritesNothingInTheHomeAsRoot(t *testing.T) {
	agentScriptsRun(t, "cp", "chmod", "mv")
	script, err := adapterScript("omp", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	r := newLinkRig(t, "omp", "omp/")
	binary := "#!/bin/sh\necho omp/9.9.9\n"
	digest := r.serve([]byte(binary))
	r.write(filepath.Join(r.bin, "herdr"), "#!/bin/sh\nexit 0\n")
	r.write(filepath.Join(r.home, ".profile"), "PI_CODING_AGENT_DIR=$HOME/.pi/agent\nexport PI_CODING_AGENT_DIR\n")
	omp := plant(t, filepath.Join(r.prefixBin, "omp"), planted{target: filepath.Join(r.outside, "omp"), body: binary, mode: 0o755})
	agentDir := plant(t, filepath.Join(r.home, ".omp"), planted{target: filepath.Join(r.outside, "omp-dir"), dir: true, mode: 0o750})
	code, out := r.runLinked(script, "TK_ADAPTER_SEPARATE_AGENT_DIR=1",
		"TK_ADAPTER_VERSION=9.9.9", "TK_ADAPTER_SHA256_X86_64="+digest, "TK_ADAPTER_SHA256_AARCH64="+digest)
	handsWrote(t, r.hands, out)
	if code != 0 || !strings.HasSuffix(strings.TrimSpace(out), adapterCompleteLine("omp")) ||
		!strings.Contains(out, "wrote the wrapper, which gives omp alone") || !strings.Contains(out, "installed herdr's omp integration") {
		t.Fatalf("omp over links in the home answered exit %d:\n%s", code, out)
	}
	omp.untouched(t, out)
	agentDir.untouched(t, out)
	replaced(t, filepath.Join(r.prefixBin, "omp"), out)
	if wrapper := r.read(filepath.Join(r.prefixBin, "omp")); !strings.Contains(wrapper, "# wsl-toolkit omp adapter wrapper") {
		t.Fatalf("the account's omp is not the wrapper:\n%s", wrapper)
	}
}

// TestTheMuseAdapterWritesNothingInTheHomeAsRoot runs muse's install.sh with links
// planted at each path it writes in the home. WSL-201.
//
// ⛔ ROOT'S REDIRECT MADE A TRUST STORE THROUGH A LINK, and its install -d changed the
// mode and owner of a directory outside the home. Its install wrote the reporter and the
// settings, and its chown gave the account whatever a link named.
// ⛔ AND ROOT READ THE REPORTER, THE SETTINGS AND herdr's RULES to compare them, through
// whatever link stood there. A second ensure finds each, and only the account reads them.
func TestTheMuseAdapterWritesNothingInTheHomeAsRoot(t *testing.T) {
	agentScriptsRun(t, "bash", "base64", "sed", "wc", "date", "mv", "chmod")
	script, err := adapterScript("muse", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	r := newLinkRig(t, "muse", "")
	r.write(filepath.Join(r.bin, "cmp"), "#!/bin/sh\nexit 0\n")
	herdr := filepath.Join(r.root, "herdr-bin", "herdr")
	r.write(herdr, "#!/bin/sh\nexit 0\n")
	r.write(filepath.Join(r.prefixBin, "muse"), "#!/bin/sh\necho \"Muse Code 9.9.9 (9.9.9-R1)\"\n")
	muse := filepath.Join(r.home, ".config", "muse")
	trust := plant(t, filepath.Join(muse, "trust.json"), planted{target: filepath.Join(r.outside, "trust.json"), absent: true})
	settings := plant(t, filepath.Join(muse, "settings.json"), planted{target: filepath.Join(r.outside, "settings.json"), absent: true})
	hookPath := filepath.Join(r.home, ".local", "share", "wsl-toolkit", "herdr-agent-state.sh")
	hook := plant(t, hookPath, planted{target: filepath.Join(r.outside, "hook.sh"), body: "#!/bin/sh\n# not the reporter\n", mode: 0o644})
	herdrDir := plant(t, filepath.Join(r.home, ".config", "herdr"), planted{target: filepath.Join(r.outside, "herdr"), dir: true, mode: 0o750})
	enc := base64.StdEncoding.EncodeToString
	env := []string{"TK_MUSE_STAGE_DIR=" + r.stage, "TK_MUSE_WRAPPER=" + filepath.Join(r.system, "muse"), "TK_HERDR_BIN=" + herdr,
		"TK_TRUSTED_WORKSPACES=~:/workspaces/src",
		"TK_FILE_HERDR_AGENT_STATE_SH_B64=" + enc([]byte("#!/bin/sh\nexit 0\n")),
		"TK_FILE_HERDR_DETECTION_TOML_B64=" + enc([]byte("# from wsl-toolkit's muse adapter\nid = \"muse\"\n"))}
	code, out := r.runLinked(script, env...)
	handsWrote(t, r.hands, out)
	if code != 0 || !strings.HasSuffix(strings.TrimSpace(out), adapterCompleteLine("muse")) ||
		!strings.Contains(out, "pre-trusted 2 workspace(s)") || !strings.Contains(out, "wrote "+hookPath) ||
		!strings.Contains(out, "registered the herdr reporter") || !strings.Contains(out, "agent-detection/muse.toml") {
		t.Fatalf("muse over links in the home answered exit %d:\n%s", code, out)
	}
	for _, p := range []planted{trust, settings, hook, herdrDir} {
		p.untouched(t, out)
	}
	for _, p := range []string{filepath.Join(muse, "trust.json"), filepath.Join(muse, "settings.json"), hookPath} {
		replaced(t, p, out)
	}
	// ⛔ A NEW SETTINGS FILE CARRIES schema_version, which Muse refuses to start without.
	if got := r.read(filepath.Join(muse, "settings.json")); !strings.Contains(got, `"schema_version":1`) {
		t.Fatalf("the settings file the account wrote carries no schema_version: %q", got)
	}
	// ⭐ A SECOND ENSURE READS WHAT IT FINDS, and the account reads it.
	code, out = r.runLinked(script, env...)
	handsWrote(t, r.hands, out)
	if code != 0 || !strings.HasSuffix(strings.TrimSpace(out), adapterCompleteLine("muse")) ||
		!strings.Contains(out, "herdr's detection rules for Muse are in place") {
		t.Fatalf("a second ensure over the files the account wrote answered exit %d:\n%s", code, out)
	}
}

// TestThePiAdapterWritesNothingInTheHomeAsRoot runs pi's install.sh with ~/.pi a link to
// a directory outside the home, and its settings file a link to nothing. WSL-201.
//
// ⛔ ROOT'S install -d CHANGED THE MODE AND OWNER OF THAT DIRECTORY, and the settings
// file was made through the link, where it named. Root then read it there to compare it.
func TestThePiAdapterWritesNothingInTheHomeAsRoot(t *testing.T) {
	agentScriptsRun(t, "tar", "gzip", "cp", "ln", "mv", "chmod")
	script, err := adapterScript("pi", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	r := newLinkRig(t, "pi", "")
	digest := r.serve(piReleaseAsset(t, "9.9.9"))
	r.write(filepath.Join(r.bin, "herdr"), "#!/bin/sh\nexit 0\n")
	piDir := plant(t, filepath.Join(r.home, ".pi"), planted{target: filepath.Join(r.outside, "pi"), dir: true, mode: 0o750})
	settingsPath := filepath.Join(r.outside, "pi", "agent", "settings.json")
	settings := plant(t, settingsPath, planted{target: filepath.Join(r.outside, "settings.json"), absent: true})
	code, out := r.runLinked(script, "TK_ADAPTER_VERSION=9.9.9", "TK_ADAPTER_SHA256_X86_64="+digest,
		"TK_ADAPTER_SHA256_AARCH64="+digest, "TK_ADAPTER_MODEL=gateway/model-1", "TK_ADAPTER_EFFORT=high")
	handsWrote(t, r.hands, out)
	if code != 0 || !strings.HasSuffix(strings.TrimSpace(out), adapterCompleteLine("pi")) ||
		!strings.Contains(out, "installed herdr's pi integration") || !strings.Contains(out, "pi starts on gateway/model-1 at effort high") {
		t.Fatalf("pi over links in the home answered exit %d:\n%s", code, out)
	}
	piDir.untouched(t, out)
	settings.untouched(t, out)
	replaced(t, settingsPath, out)
}

// TestTheHerdrAdapterWritesNothingInTheHomeAsRoot runs herdr's install.sh as far as the
// door, with ~/.config/herdr a link to a directory outside the home and the
// configuration a link to a file there. WSL-201.
//
// ⛔ ROOT MADE ITS TEMPORARY FILE IN A DIRECTORY THE ACCOUNT OWNS, chowned and moved it,
// and its install -d changed the mode and owner of what ~/.config/herdr named.
func TestTheHerdrAdapterWritesNothingInTheHomeAsRoot(t *testing.T) {
	needPosix(t, "cat", "cut", "mktemp", "sha256sum", "base64", "cp", "mkdir", "chmod", "mv", "rm", "rmdir", "sed", "head", "readlink")
	script, err := adapterScript("herdr", "install.sh")
	if err != nil {
		t.Fatal(err)
	}
	files, err := adapterFileEnv("herdr")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	home, lib, units, dest := filepath.Join(root, "home"), filepath.Join(root, "lib"), filepath.Join(root, "units"), filepath.Join(root, "bin-herdr")
	outside, hands := filepath.Join(root, "outside"), filepath.Join(root, "root-hands")
	for _, dir := range []string{home, lib, units, dest} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	served := []byte("a herdr the case serves\n")
	bin, write := stubDir(t)
	realCat, _ := exec.LookPath("cat")
	for _, name := range []string{"pacman", "sshd", "ssh-keygen", "ss"} {
		write(name, "exit 0\n")
	}
	write("cat", "if [ \"$1\" = /proc/1/comm ]; then echo systemd; exit 0; fi\nexec "+realCat+" \"$@\"\n")
	write("getent", "printf 'herdr:x:1000:1000::%s:/bin/bash\\n' \"$STUB_HOME\"\n")
	write("id", "echo herdr\n")
	write("uname", "echo x86_64\n")
	write("curl", "while [ $# -gt 0 ]; do if [ \"$1\" = --output ]; then printf 'a herdr the case serves\\n' > \"$2\"; shift; fi; shift; done\n")
	write("systemctl", "case $1 in is-active) exit 0 ;; is-enabled) echo masked ;; esac\nexit 0\n")
	write("ip", "exit 0\n")
	write("runuser", "[ \"$1\" = -u ] && [ \"$2\" = herdr ] && [ \"$3\" = -- ] || exit 9\nshift 3\nexec \"$@\"\n")
	rootHands(t, bin, "/etc/wsl-toolkit", "")
	herdrDir := plant(t, filepath.Join(home, ".config", "herdr"), planted{target: filepath.Join(outside, "herdr"), dir: true, mode: 0o750})
	configPath := filepath.Join(outside, "herdr", "config.toml")
	config := plant(t, configPath, planted{target: filepath.Join(outside, "config.toml"), body: "a file outside the home\n", mode: 0o640})
	env := []string{"TK_USER=herdr", "TK_DISTRO=wsl-toolkit-base", "TK_SSH_CLIENT_KEY=ssh-ed25519 AAAA",
		"TK_HERDR_BIN=" + filepath.Join(dest, "herdr"), "TK_HERDR_LIB_DIR=" + lib, "TK_HERDR_UNIT_DIR=" + units,
		"TK_ADAPTER_VERSION=0.10.1", "TK_ADAPTER_SHA256_X86_64=" + sha256Hex(served),
		"STUB_HOME=" + home, "STUB_OUTSIDE=" + outside, "STUB_HANDS=" + hands}
	for k, v := range files {
		env = append(env, k+"="+v)
	}
	code, out := runSh(t, script, bin, nil, env...)
	handsWrote(t, hands, out)
	if code != 7 || !strings.Contains(out, "stopped here by the case") || !strings.Contains(out, "wrote the tracked herdr configuration") {
		t.Fatalf("herdr over links in the home answered exit %d, and the case stops it at the door's directory:\n%s", code, out)
	}
	herdrDir.untouched(t, out)
	config.untouched(t, out)
	replaced(t, configPath, out)
	if got, _ := os.ReadFile(configPath); string(got) != herdrFile(t, "config.toml") {
		t.Fatalf("the configuration the account wrote is not the adapter's:\n%s", got)
	}
}

// -- the provisioner and engines.sh ------------------------------------------------------

// accountTools links the host's own commands into dir, for the PATH the account's side of
// a case runs on, where root's hands are not.
func accountTools(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range rigAccountTools {
		if p, err := exec.LookPath(tool); err == nil {
			if err := os.Symlink(p, filepath.Join(dir, tool)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// inodeChown is root's chown in the provisioner's cases. It changes nothing, because the
// host has no account to give a file to, and records the inode of each file it was
// handed, as chown would change it: a link's own with -h, and what it names without.
// ⛔ AN INODE, NOT A SPELLING. A hard link is the file it names, so only the inode tells
// whether root gave the account a file outside the home. WSL-201.
// ⚠ A RECURSIVE ONE IS ROOT WALKING A TREE THE ACCOUNT OWNS, and is one of root's hands.
const inodeChown = `#!/bin/sh
deref=L
owner=
case " $* " in *" -R "*) printf 'chown %s\n' "$*" >> "$STUB_HANDS" ;; esac
for a do
  case $a in
    -h) deref= ;;
    -*) ;;
    *)
      if [ -z "$owner" ]; then owner=$a; continue; fi
      ls -di$deref -- "$a" | awk '{ print $1 }' >> "$STUB_CHOWNS"
      ;;
  esac
done
exit 0
`

// inode answers the inode of p itself, never of what a link there names.
func inode(t *testing.T, p string) string {
	t.Helper()
	out, err := exec.Command("ls", "-di", "--", p).Output()
	if err != nil {
		t.Fatalf("ls -di %s: %v", p, err)
	}
	return strings.Fields(string(out))[0]
}

// provisionHome runs the provisioner's XDG directories, and then its engine settings
// when settings is set, as root over home, with chown recording inodes. The account is
// the host's own commands, on a PATH without root's hands. It answers what the script
// printed, the inodes root's chown changed, and whether it exited 0. Root's other hands
// record in dir's root-hands.
func provisionHome(t *testing.T, dir, home, outside string, settings bool) (string, []string, bool) {
	t.Helper()
	sh := posixShell(t)
	hands, bin, account := filepath.Join(dir, "root-hands"), filepath.Join(dir, "bin"), filepath.Join(dir, "account-path")
	chowns := filepath.Join(dir, "root-chowns")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	rootHands(t, bin, "", "")
	if err := os.WriteFile(filepath.Join(bin, "chown"), []byte(inodeChown), 0o755); err != nil {
		t.Fatal(err)
	}
	accountTools(t, account)
	script := "set -eu\nTK_USER=agent\nTK_GROUP=agent\nTK_HOME=" + shellQuote(home) + "\n" +
		"say() { printf '%s\\n' \"$*\"; }\ndie() { printf 'die: %s\\n' \"$*\"; exit 3; }\npasta() { :; }\n" +
		// ⭐ runuser is the account: the host's own commands, on a PATH without root's hands.
		"runuser() { shift 3; PATH=" + shellQuote(account) + " \"$@\"; }\n" +
		markedSection(t, provisionScript, "as the account") +
		markedSection(t, provisionScript, "the account's XDG directories")
	if settings {
		script += strings.ReplaceAll(string(mirrorsScript)+"\n"+provisionerSection(t, "the engine's settings"),
			"/etc/containers/", filepath.ToSlash(dir)+"/etc/containers/")
	}
	cmd := exec.Command(sh, "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STUB_HOME="+home, "STUB_OUTSIDE="+outside, "STUB_HANDS="+hands, "STUB_CHOWNS="+chowns)
	raw, err := cmd.CombinedOutput()
	recorded, _ := os.ReadFile(chowns)
	return string(raw), strings.Fields(string(recorded)), err == nil
}

// homeDirs answers the inodes of the directories that stand at the XDG paths of home
// themselves, links left out: what root may give back.
func homeDirs(t *testing.T, home string) map[string]string {
	t.Helper()
	dirs := map[string]string{}
	for _, rel := range []string{".config", ".cache", ".local", ".local/share", ".local/state", ".config/containers", ".config/containers/containers.conf.d"} {
		p := filepath.Join(home, filepath.FromSlash(rel))
		if info, err := os.Lstat(p); err == nil && info.IsDir() {
			dirs[inode(t, p)] = rel
		}
	}
	return dirs
}

// TestProvisioningLeavesTheHomeToTheAccount runs the provisioner's XDG directories and
// the engine's settings with links planted in the home. WSL-201.
//
// ⛔ A RE-PROVISIONING RUNS AFTER THE ACCOUNT HAS RUN AGENTS AND JOBS. Root's chmod 0700
// changed the mode of whatever .cache named, root's redirect wrote the engine's settings
// through a link, and chown -R ran over a tree the account owns.
// ⭐ Root still gives back a directory at an XDG path that is not the account's, and only
// a directory that stands at that path in the home itself.
func TestProvisioningLeavesTheHomeToTheAccount(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the provisioner runs in a Linux base, on symbolic links, so the Linux job runs this case")
	}
	dir := t.TempDir()
	home, outside := filepath.Join(dir, "home"), filepath.Join(dir, "outside")
	if err := os.MkdirAll(filepath.Join(home, ".config", "containers", "containers.conf.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	cache := plant(t, filepath.Join(home, ".cache"), planted{target: filepath.Join(outside, "cache"), dir: true, mode: 0o750})
	local := plant(t, filepath.Join(home, ".local"), planted{target: filepath.Join(outside, "local"), dir: true, mode: 0o750})
	// ⚠ What .local names already holds share, so a root that entered it would find
	// something there to give away. state is left for the account to make.
	if err := os.Mkdir(filepath.Join(local.target, "share"), 0o755); err != nil {
		t.Fatal(err)
	}
	confPath := filepath.Join(home, ".config", "containers", "containers.conf.d", "10-wsl-toolkit.conf")
	conf := plant(t, confPath, planted{target: filepath.Join(outside, "conf"), body: "a file outside the home\n", mode: 0o640})
	old := filepath.Join(home, ".config", "containers", "containers.conf")
	if err := os.WriteFile(old, []byte("# Written by wsl-toolkit. cgroupfs works with either configured init.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	given := homeDirs(t, home)
	out, changed, ok := provisionHome(t, dir, home, outside, true)
	if !ok {
		t.Fatalf("the provisioner's home failed:\n%s", out)
	}
	handsWrote(t, filepath.Join(dir, "root-hands"), out)
	for _, n := range changed {
		if _, mine := given[n]; !mine {
			t.Errorf("root's chown changed inode %s, which is no directory at an XDG path of the home:\n%s", n, out)
		}
	}
	cache.untouched(t, out)
	local.untouched(t, out)
	conf.untouched(t, out)
	replaced(t, confPath, out)
	if got, _ := os.ReadFile(confPath); !strings.Contains(string(got), `cgroup_manager = "cgroupfs"`) {
		t.Errorf("the account's engine settings carry no cgroupfs:\n%s", got)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) || !strings.Contains(out, "removed the containers.conf an earlier release wrote") {
		t.Errorf("the containers.conf an earlier release wrote is still there (%v):\n%s", err, out)
	}
	if info, err := os.Stat(filepath.Join(home, ".config")); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the account's .config is not 0700 (%v):\n%s", err, out)
	}
	if info, err := os.Stat(filepath.Join(outside, "local", "state")); err != nil || !info.IsDir() {
		t.Errorf("the account did not make .local/state where its .local names (%v):\n%s", err, out)
	}
}

// TestProvisioningGivesTheAccountNoFileAHardLinkNames runs the provisioner's XDG
// directories over a home whose .cache is a hard link to a file outside it, and whose
// .local/state is a regular file. WSL-201.
//
// ⛔ ROOT'S chown -h GAVE THE ACCOUNT THE FILE A HARD LINK IS. -h keeps a symbolic link
// from being followed, and a hard link is the file itself, so a hard link at .cache to
// a file of root's gave the account that file. Root gives back only a directory now,
// from inside it, and a name that is not a directory is named and left as it is.
// ⭐ AND A DIRECTORY AN OLDER RELEASE LEFT ROOT'S IS GIVEN BACK. Its provisioning died
// before its chown -R, and the account's own mktemp in .config/containers then failed.
func TestProvisioningGivesTheAccountNoFileAHardLinkNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the provisioner runs in a Linux base, on hard links, so the Linux job runs this case")
	}
	dir := t.TempDir()
	home, outside := filepath.Join(dir, "home"), filepath.Join(dir, "outside")
	for _, d := range []string{filepath.Join(home, ".config", "containers"), filepath.Join(home, ".local"), outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	roots := filepath.Join(outside, "roots-own-file")
	const rootsBody = "a file of root's\n"
	if err := os.WriteFile(roots, []byte(rootsBody), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(roots, filepath.Join(home, ".cache")); err != nil {
		t.Skipf("no hard link could be made here: %v", err)
	}
	state := filepath.Join(home, ".local", "state")
	if err := os.WriteFile(state, []byte("a file of the account's\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	refused := map[string]string{inode(t, roots): "the file .cache is a hard link to", inode(t, state): ".local/state"}
	containers := inode(t, filepath.Join(home, ".config", "containers"))
	out, changed, ok := provisionHome(t, dir, home, outside, false)
	if ok || !strings.Contains(out, "die: "+filepath.Join(home, ".cache")+" is not a directory, and a directory of agent's belongs there") {
		t.Fatalf("a hard link at .cache did not stop the provisioner naming it:\n%s", out)
	}
	handsWrote(t, filepath.Join(dir, "root-hands"), out)
	gave := false
	for _, n := range changed {
		if what, bad := refused[n]; bad {
			t.Errorf("root's chown changed inode %s, %s:\n%s", n, what, out)
		}
		gave = gave || n == containers
	}
	if !gave {
		t.Errorf("root did not give back .config/containers, a directory in the home: it changed %q\n%s", changed, out)
	}
	if b, err := os.ReadFile(roots); err != nil || string(b) != rootsBody {
		t.Errorf("the file a hard link named was changed: %q %v", b, err)
	}
	if info, err := os.Lstat(state); err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Errorf("the account's file at .local/state was changed: %v %v", info, err)
	}
}

// TestProvisioningNamesAnAccountNothingCanBecome runs the provisioner's XDG directories
// where neither runuser nor su is installed. WSL-201.
//
// ⛔ PROVISIONING NOW BECOMES THE ACCOUNT, so a userland with neither stops there. Its
// refusal said the account could not make its directories, which reads as a full disk
// or a link. It names the two commands now.
func TestProvisioningNamesAnAccountNothingCanBecome(t *testing.T) {
	sh, _ := grantsShell(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(filepath.Join(dir, "home"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "chown"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "set -eu\nTK_USER=agent\nTK_GROUP=agent\nTK_HOME=" + shellPath(t, filepath.Join(dir, "home")) + "\n" +
		"say() { printf '%s\\n' \"$*\"; }\ndie() { printf 'die: %s\\n' \"$*\"; exit 3; }\n" +
		markedSection(t, provisionScript, "as the account") +
		markedSection(t, provisionScript, "the account's XDG directories")
	file := filepath.Join(dir, "xdg.sh")
	if err := os.WriteFile(file, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, filepath.ToSlash(file))
	// ⚠ A PATH OF ONE STAND-IN, so neither runuser nor su is found.
	cmd.Env = append(os.Environ(), "PATH="+shellPath(t, bin))
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "die: neither runuser nor su is installed, so nothing can act as agent in its home") {
		t.Fatalf("a userland with neither runuser nor su answered %v:\n%s", err, out)
	}
}

// TestProvisioningNamesAFileWhereTheEngineSettingsGo runs the provisioner's XDG
// directories over a home with a file at .config/containers, and then at its drop-in
// directory. WSL-201.
//
// ⛔ THE ACCOUNT CHECKED THE FIVE XDG NAMES ALONE. A file, a pipe or a dangling link at
// either directory the engine's settings go in failed a later step, with a message that
// named neither the path nor the remedy, so the owner's ensure stayed stuck. Both are
// named now, as the others are.
func TestProvisioningNamesAFileWhereTheEngineSettingsGo(t *testing.T) {
	sh, bin := grantsShell(t)
	for _, rel := range []string{".config/containers", ".config/containers/containers.conf.d"} {
		dir := t.TempDir()
		home, standIns := filepath.Join(dir, "home"), filepath.Join(dir, "bin")
		if err := os.MkdirAll(filepath.Dir(filepath.Join(home, filepath.FromSlash(rel))), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, filepath.FromSlash(rel)), []byte("a file of the account's\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(standIns, 0o755); err != nil {
			t.Fatal(err)
		}
		// ⭐ Root's chown changes nothing here, and runuser is the account: the host itself.
		if err := os.WriteFile(filepath.Join(standIns, "chown"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		at := shellPath(t, home)
		script := "set -eu\nTK_USER=agent\nTK_GROUP=agent\nTK_HOME=" + at + "\n" +
			"say() { printf '%s\\n' \"$*\"; }\ndie() { printf 'die: %s\\n' \"$*\"; exit 3; }\n" +
			"runuser() { shift 3; \"$@\"; }\n" +
			markedSection(t, provisionScript, "as the account") +
			markedSection(t, provisionScript, "the account's XDG directories")
		file := filepath.Join(dir, "xdg.sh")
		if err := os.WriteFile(file, []byte(script), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(sh, filepath.ToSlash(file))
		cmd.Env = append(os.Environ(), "PATH="+standIns+string(os.PathListSeparator)+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		want := "die: " + at + "/" + rel + " is not a directory, and a directory of agent's belongs there. Move it aside as agent, then run base ensure again"
		if err == nil || !strings.Contains(string(out), want) {
			t.Errorf("a file at %s answered %v, want %q:\n%s", rel, err, want, out)
		}
		if b, _ := os.ReadFile(filepath.Join(home, filepath.FromSlash(rel))); string(b) != "a file of the account's\n" {
			t.Errorf("the account's file at %s was changed: %q", rel, b)
		}
	}
}

// accountReaders are the adapter scripts' sections that hold account_read, and the call
// each case makes of a reader built on it. $1 is root's own file, and $2 the account's.
var accountReaders = []struct{ adapter, script, section, call string }{
	{"muse", "install.sh", "the account's own writes", `account_same "$1" "$2" && echo same || echo differs`},
	{"herdr", "install.sh", "the account's own writes", `account_same "$1" "$2" && echo same || echo differs`},
	{"omp", "install.sh", "the account's own writes", `account_same "$1" "$2" && echo same || echo differs`},
	{"pi", "install.sh", "the account's own writes", `account_same "$1" "$2" && echo same || echo differs`},
	{"muse", "probe.sh", "the account's reads", `first_lines 1 "$2"; echo read`},
	{"omp", "probe.sh", "the account's reads", `first_lines 1 "$2"; echo read`},
	{"pi", "probe.sh", "the account's reads", `first_lines 1 "$2"; echo read`},
}

// TestAnAccountReadThatHangsIsStopped runs account_same and first_lines where the
// account's read of a file never returns, as one of a named pipe or of its own FUSE
// mount does. WSL-201.
//
// ⛔ THE ACCOUNT'S READ HELD base ensure UNTIL THE ADAPTER'S 20-MINUTE LIMIT. It won the
// race after the `[ -f ]` test with a named pipe, or served the file from its own FUSE
// mount and never answered. Root's timeout runs runuser now, so the read stops and reads
// as nothing. ⭐ The case stands in a reader that sleeps, and a second for the bound.
func TestAnAccountReadThatHangsIsStopped(t *testing.T) {
	sh, bin := grantsShell(t)
	if _, err := exec.LookPath("timeout"); err != nil {
		t.Skip("no timeout on PATH, and the bound is taken only where the userland carries one")
	}
	dir := t.TempDir()
	standIns, hangs := filepath.Join(dir, "bin"), filepath.Join(dir, "hangs")
	for _, d := range []string{standIns, hangs} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// ⭐ runuser is the account: the host, with readers that never answer first on its PATH.
	write := func(p, body string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// ⚠ FOUND BESIDE ITSELF, as the shell spells it. A Windows path in PATH is split at
	// its drive's colon.
	write(filepath.Join(standIns, "runuser"), "#!/bin/sh\nshift 3\nPATH=${0%/*}/../hangs:$PATH\nexport PATH\nexec \"$@\"\n")
	for _, reader := range []string{"sha256sum", "head"} {
		write(filepath.Join(hangs, reader), "#!/bin/sh\nexec sleep 20\n")
	}
	src, dest := filepath.Join(dir, "src"), filepath.Join(dir, "dest")
	write(src, "the adapter's bytes\n")
	write(dest, "the adapter's bytes\n")
	for _, r := range accountReaders {
		body, err := adapterScript(r.adapter, r.script)
		if err != nil {
			t.Fatal(err)
		}
		section := strings.ReplaceAll(markedSection(t, body, r.section), "timeout 30 ", "timeout 1 ")
		script := "as_account() { runuser -u agent -- \"$@\"; }\n" + section + r.call + "\n"
		file := filepath.Join(dir, r.adapter+"-"+r.script)
		write(file, script)
		cmd := exec.Command(sh, filepath.ToSlash(file), shellPath(t, src), shellPath(t, dest))
		cmd.Env = append(os.Environ(), "PATH="+standIns+string(os.PathListSeparator)+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		start := time.Now()
		out, err := cmd.CombinedOutput()
		took := time.Since(start)
		if err != nil || took > 12*time.Second {
			t.Errorf("%s/%s: a read that never answers held its reader %s (%v):\n%s", r.adapter, r.script, took.Round(time.Second), err, out)
		}
		if r.script == "install.sh" && !strings.Contains(string(out), "differs") {
			t.Errorf("%s/%s: a read that was stopped read as the same file:\n%s", r.adapter, r.script, out)
		}
	}
}

// TestEveryAccountReadInTheAdaptersIsBounded reads every adapter script for a read of a
// file the account owns that does not go through account_read. WSL-201.
//
// ⛔ ONE UNBOUNDED READ IS ENOUGH TO HOLD base ensure, so the class is held, not the sites
// it was found at: jq, sha256sum, head, sed, cat, grep and readlink run by as_account, a
// script of its own that reads a file it tests, and the login shell that reads its
// profile.
func TestEveryAccountReadInTheAdaptersIsBounded(t *testing.T) {
	unbounded := regexp.MustCompile(`\bas_account (jq|sha256sum|head|sed|cat|grep|readlink)\b|\bas_account sh -c '\[ -f |\bas_account sh -lc\b`)
	seen := 0
	err := fs.WalkDir(adapterTree, "adapters", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".sh") {
			return err
		}
		body, err := adapterTree.ReadFile(p)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(body), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				continue
			}
			if unbounded.MatchString(line) {
				t.Errorf("%s:%d reads a file of the account's without a bound. Run it through account_read:\n%s", p, i+1, line)
			}
			seen += strings.Count(line, "account_read ")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Fatal("no adapter script reads through account_read, so this case is reading scripts that moved")
	}
}

// TestEnginesShLeavesTheAccountsDropInToTheAccount runs engines.sh's removal of the
// account's engine drop-in, on a base without systemd, where the account cannot be
// become. WSL-201.
//
// ⛔ ROOT REMOVED IT ITSELF, and rm -f follows a link the account made of a directory
// above the file. So root removed a file of that name outside the home. One the account
// cannot remove is named instead.
func TestEnginesShLeavesTheAccountsDropInToTheAccount(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("engines.sh runs in a Linux base, on symbolic links, so the Linux job runs this case")
	}
	sh := posixShell(t)
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	dropIn := plant(t, filepath.Join(home, ".config", "containers", "containers.conf.d"),
		planted{target: filepath.Join(dir, "outside", "conf.d"), dir: true, mode: 0o755})
	target := filepath.Join(dropIn.target, "50-wsl-toolkit-cgroup.conf")
	if err := os.WriteFile(target, []byte("a file outside the home\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "say() { printf '  * %s\\n' \"$*\"; }\nrunuser() { return 1; }\nsu() { return 1; }\n" +
		"TK_USER=agent\nhome=" + shellQuote(home) + "\n" +
		markedSection(t, enginesScript, "as the account") +
		markedSection(t, enginesScript, "the account's engine drop-in") +
		markedSection(t, enginesScript, "the account's drop-in without systemd")
	out, err := exec.Command(sh, "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("the drop-in's removal failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("root removed %s through the link the account made (%v):\n%s", target, err, out)
	}
	if !strings.Contains(string(out), "agent could not remove "+home+"/.config/containers/containers.conf.d/50-wsl-toolkit-cgroup.conf") {
		t.Fatalf("a drop-in the account could not remove was not named:\n%s", out)
	}
}

// enginesBus runs engines.sh's choice of the account's drop-in by its session bus, as root
// over dir's home, and answers what it said. bus says whether the session bus answers and
// the user manager is active, and become stands in for runuser.
func enginesBus(t *testing.T, dir string, bus bool, become string) string {
	t.Helper()
	sh, bin := grantsShell(t)
	at := shellPath(t, dir)
	dropIn := filepath.Join(dir, "home", ".config", "containers", "containers.conf.d", "50-wsl-toolkit-cgroup.conf")
	if err := os.MkdirAll(filepath.Dir(dropIn), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dropIn, []byte("[engine]\ncgroup_manager = \"systemd\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	section := strings.ReplaceAll(markedSection(t, enginesScript, "the account's drop-in, by its session bus"),
		`[ -S "/run/user/$uid/bus" ]`, "[ "+strconv.FormatBool(bus)+" = true ]")
	script := "say() { printf '  * %s\\n' \"$*\"; }\n" + become + "su() { return 1; }\nsystemctl() { return 0; }\n" +
		"TK_USER=agent\nuid=1000\ndelegated=\nhome=" + at + "/home\n" +
		markedSection(t, enginesScript, "as the account") +
		markedSection(t, enginesScript, "the account's engine drop-in") + section
	file := filepath.Join(t.TempDir(), "bus.sh")
	if err := os.WriteFile(file, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, filepath.ToSlash(file))
	cmd.Env = shellEnv(bin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the drop-in's branch failed: %v\n%s", err, out)
	}
	return string(out)
}

// TestEnginesShSaysCgroupfsOnlyOnceTheDropInIsGone runs engines.sh's branch for a
// systemd base whose user manager has no session bus. WSL-201.
//
// ⛔ IT SAID THE ENGINE KEPT cgroupfs WHILE THE DROP-IN STILL NAMED systemd. The
// account's removal failed, the failure was ignored, and the line went on as if the
// drop-in were gone. One the account cannot remove is named, as without systemd.
// ⚠ AND NOTHING IS CLAIMED OF WHAT IT HOLDS. A removal fails where nothing can become the
// account, whether or not a drop-in is there.
func TestEnginesShSaysCgroupfsOnlyOnceTheDropInIsGone(t *testing.T) {
	dir := t.TempDir()
	at := shellPath(t, dir)
	dropIn := filepath.Join(dir, "home", ".config", "containers", "containers.conf.d", "50-wsl-toolkit-cgroup.conf")
	out := enginesBus(t, dir, false, "runuser() { return 1; }\n")
	if !strings.Contains(out, "agent could not remove "+at+"/home/.config/containers/containers.conf.d/50-wsl-toolkit-cgroup.conf, and its user manager has no session bus") ||
		strings.Contains(out, "keeps cgroupfs") || strings.Contains(out, "still names") {
		t.Fatalf("a drop-in the account could not remove read as gone, or was said to name a manager:\n%s", out)
	}
	if _, err := os.Stat(dropIn); err != nil {
		t.Fatalf("the drop-in is gone, and nobody could remove it: %v", err)
	}
	out = enginesBus(t, dir, false, "runuser() { shift 3; \"$@\"; }\n")
	if !strings.Contains(out, "so its engine keeps cgroupfs") || strings.Contains(out, "could not remove") {
		t.Fatalf("a drop-in the account removed did not read as cgroupfs:\n%s", out)
	}
	if _, err := os.Stat(dropIn); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the account's removal left the drop-in: %v", err)
	}
}

// TestEnginesShSaysAWriteThatFailedBesideALiveBus runs engines.sh's branch for a systemd
// base whose session bus answers. WSL-201.
//
// ⛔ THE WRITE WAS A THIRD TEST BESIDE THE BUS, so a write that failed beside a live bus
// went to the bus-less branch, and both its lines blamed a session bus that answered.
// The bus is read once, and a write that failed is said as one.
func TestEnginesShSaysAWriteThatFailedBesideALiveBus(t *testing.T) {
	dir := t.TempDir()
	at := shellPath(t, dir)
	out := enginesBus(t, dir, true, "runuser() { return 1; }\n")
	if !strings.Contains(out, "agent could not write "+at+"/home/.config/containers/containers.conf.d/50-wsl-toolkit-cgroup.conf") ||
		strings.Contains(out, "no session bus") {
		t.Fatalf("a write that failed beside a live bus was not said as one:\n%s", out)
	}
	out = enginesBus(t, dir, true, "runuser() { shift 3; \"$@\"; }\n")
	if !strings.Contains(out, "cgroup delegation: none read to agent's user manager") || strings.Contains(out, "could not") {
		t.Fatalf("a write beside a live bus did not move the engine to the user manager:\n%s", out)
	}
}

// -- WSL-202: what root writes outside the home, at a path the account can shape ----------

// TestTheMuseProbeLeavesItsScratchToTheAccount runs the muse probe's Muse session with
// root's rm on PATH, where the account's PATH does not reach it. WSL-202.
//
// ⛔ ROOT REMOVED THE ACCOUNT'S SCRATCH DIRECTORY ITSELF. The directory is the account's,
// and root's rm -rf walked whatever the account put in it. The account removes it now.
// ⚠ NO jq IS NEEDED: root's is a stand-in that reads the session's id, as the probe's
// own case cannot be run without one.
func TestTheMuseProbeLeavesItsScratchToTheAccount(t *testing.T) {
	needPosix(t, "mktemp", "timeout", "sed")
	probe, err := adapterScript("muse", "probe.sh")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin, account := filepath.Join(dir, "bin"), filepath.Join(dir, "account-path")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	accountTools(t, account)
	if p, err := exec.LookPath("timeout"); err == nil {
		if err := os.Symlink(p, filepath.Join(account, "timeout")); err != nil {
			t.Fatal(err)
		}
	}
	realRm, err := exec.LookPath("rm")
	if err != nil {
		t.Fatal(err)
	}
	rootRm, seen := filepath.Join(dir, "root-rm"), filepath.Join(dir, "seen")
	logFile, hook, launcher := filepath.Join(dir, "reporter.log"), filepath.Join(dir, "hook.sh"), filepath.Join(dir, "muse")
	for name, body := range map[string]string{
		filepath.Join(bin, "rm"): "#!/bin/sh\nfor a do\n  case $a in */wsl-toolkit-muse-probe.*) printf 'rm %s\\n' \"$*\" >> " + shellQuote(rootRm) +
			"; break ;; esac\ndone\nexec " + shellQuote(realRm) + " \"$@\"\n",
		filepath.Join(bin, "jq"):    "#!/bin/sh\nsed -n 's/.*\"kind\":\"session\",\"id\":\"\\([^\"]*\\)\".*/\\1/p'\n",
		filepath.Join(bin, "herdr"): "#!/bin/sh\nexit 0\n",
		hook:                        "#!/bin/sh\nexit 0\n",
		// The stand-in for Muse: it says where it ran, names its session, and the reporter logs it.
		launcher: "#!/bin/sh\nprintf '%s\\n' \"$PWD\" > " + shellQuote(seen) +
			"\nprintf '{\"stream\":{\"kind\":\"session\",\"id\":\"s-1\"}}\\n'\nprintf 'SessionStart for s-1 \\n' >> " + shellQuote(logFile) + "\n",
	} {
		if err := os.WriteFile(name, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script := "problem() { printf 'problem %s\\n' \"$*\"; }\n" +
		"as_account() { env PATH=" + shellQuote(account) + " \"$@\"; }\n" +
		"TK_PROBE_SESSION=1\nHOOK=" + shellQuote(hook) + "\nLAUNCHER=" + shellQuote(launcher) +
		"\nREPORTER_LOG=" + shellQuote(logFile) + "\nMUSE_SETTINGS=/nowhere\nregistered=SessionStart\nversion=9.9.9\n" +
		markedSection(t, probe, "the account's reads") + markedSection(t, probe, "the probe's Muse session")
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "herdr_reporter_session_start reached") {
		t.Fatalf("the probe's session answered %v:\n%s", err, out)
	}
	if b, _ := os.ReadFile(rootRm); len(b) != 0 {
		t.Fatalf("root removed the account's scratch directory itself:\n%s\n%s", b, out)
	}
	ran, err := os.ReadFile(seen)
	if err != nil {
		t.Fatalf("the stand-in for Muse never ran: %v\n%s", err, out)
	}
	scratch := filepath.Dir(strings.TrimSpace(string(ran)))
	if !strings.HasPrefix(filepath.Base(scratch), "wsl-toolkit-muse-probe.") {
		t.Fatalf("Muse ran in %q, not in the probe's scratch directory", ran)
	}
	if _, err := os.Stat(scratch); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the account did not remove its scratch directory %s (%v)", scratch, err)
	}
}

// TestTheMuseReporterSelfTestMakesADirectoryOfItsOwn runs the reporter's self-test where
// the name it used is taken by a directory another account made, holding links. WSL-202.
//
// ⛔ THE MUSE ADAPTER RUNS THE SELF-TEST AS ROOT, and it took the fixed name
// herdr-muse-selftest.PID in /tmp with mkdir -p. A directory any account made there
// first was taken, root wrote its bindings through the links it held, and then removed
// it. ⭐ The case runs the self-test with exec, so its PID is the one it plants under.
func TestTheMuseReporterSelfTestMakesADirectoryOfItsOwn(t *testing.T) {
	needPosix(t, "mktemp", "ln")
	reporter, err := adapterTree.ReadFile("adapters/muse/herdr-agent-state.sh")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	tmp, outside := filepath.Join(dir, "tmp"), filepath.Join(dir, "outside")
	for _, d := range []string{tmp, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(dir, "reporter.sh")
	if err := os.WriteFile(file, reporter, 0o644); err != nil {
		t.Fatal(err)
	}
	roots := filepath.Join(outside, "roots-own-file")
	const rootsBody = "a file of root's\n"
	if err := os.WriteFile(roots, []byte(rootsBody), 0o644); err != nil {
		t.Fatal(err)
	}
	absent := filepath.Join(outside, "made-through-a-link")
	// The other account's directory, under the self-test's own PID, then the self-test.
	const plantAndRun = `d=$TMPDIR/herdr-muse-selftest.$$
mkdir "$d" && ln -s "$1" "$d/bindings" && ln -s "$2" "$d/bindings.$$" && printf planted > "$d/planted" || exit 9
exec sh "$3" --selftest`
	cmd := exec.Command("sh", "-c", plantAndRun, "sh", absent, roots, file)
	cmd.Env = append(os.Environ(), "TMPDIR="+tmp)
	out, err := cmd.CombinedOutput()
	// ⚠ WITHOUT jq ITS CASES THAT READ A PAYLOAD FAIL, and it still runs to its end, which
	// is what this case reads.
	if _, jq := exec.LookPath("jq"); jq == nil && err != nil || !strings.Contains(string(out), "selftest: ") {
		t.Fatalf("the self-test answered %v:\n%s", err, out)
	}
	if _, err := os.Lstat(absent); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the self-test made a file through a link in another account's directory (%v)", err)
	}
	if b, _ := os.ReadFile(roots); string(b) != rootsBody {
		t.Errorf("the self-test wrote through a link in another account's directory: %q", b)
	}
	left, _ := filepath.Glob(filepath.Join(tmp, "herdr-muse-selftest.*"))
	if len(left) != 1 {
		t.Fatalf("the self-test's directories are %q; want the other account's alone, its own removed", left)
	}
	if b, err := os.ReadFile(filepath.Join(left[0], "planted")); err != nil || string(b) != "planted" {
		t.Errorf("the self-test removed or changed another account's directory: %q %v", b, err)
	}
}
