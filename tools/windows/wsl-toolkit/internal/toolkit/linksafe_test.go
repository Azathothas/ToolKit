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
	"strings"
	"testing"
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
// holds its words with the quotes taken off, the files its output redirections name, and
// the line it starts on. ⚠ A $ that a single quote or a backslash makes literal is kept
// as a NUL, so it never reads as a variable.
type shCommand struct {
	line  int
	words []string
	outs  []string
}

// shBreaks end a word that is not quoted, and a here-document's delimiter.
const shBreaks = " \t\n;&|<>()"

// shCommands reads src into its simple commands. A command substitution's own commands
// are read as commands too, and a here-document's body is skipped as text.
func shCommands(src string, line int) []shCommand {
	var cmds []shCommand
	cur := shCommand{line: line}
	var word strings.Builder
	inWord := false
	// redirect is 1 when the next word is the file an output goes to, and 2 when it is a
	// word to drop: an input's file, or a descriptor an output is joined to.
	redirect := 0
	type heredoc struct {
		delim string
		strip bool
	}
	var pending []heredoc
	flush := func() {
		if !inWord {
			return
		}
		w := word.String()
		word.Reset()
		inWord = false
		switch redirect {
		case 1:
			cur.outs = append(cur.outs, w)
		case 2:
		default:
			cur.words = append(cur.words, w)
		}
		redirect = 0
	}
	end := func() {
		flush()
		if len(cur.words) > 0 || len(cur.outs) > 0 {
			cmds = append(cmds, cur)
		}
		cur = shCommand{line: line}
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
			for _, sub := range shSubstitutions(body) {
				cmds = append(cmds, shCommands(sub, line)...)
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
			cmds = append(cmds, shCommands(inner, line)...)
			word.WriteString(src[i:j])
			line += strings.Count(inner, "\n")
			inWord = true
			i = j
		case c == '#' && !inWord:
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '\n':
			end()
			line++
			i++
			for _, h := range pending {
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
				}
			}
			pending = nil
			cur.line = line
		case c == ' ' || c == '\t':
			flush()
			i++
		case strings.IndexByte(";&|()", c) >= 0:
			end()
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
				pending = append(pending, heredoc{strings.NewReplacer("'", "", `"`, "", `\`, "").Replace(src[start:i]), strip})
				continue
			}
			i++
			kind := 2
			if c == '>' {
				kind = 1
				if i < len(src) && (src[i] == '>' || src[i] == '|') {
					i++
				}
				if i < len(src) && src[i] == '&' {
					kind = 2
					i++
				}
			} else if i < len(src) && (src[i] == '>' || src[i] == '&') {
				i++
			}
			for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
				i++
			}
			redirect = kind
		default:
			word.WriteByte(c)
			inWord = true
			i++
		}
	}
	end()
	return cmds
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
			subs = append(subs, body[i+2:end])
			i = j
		default:
			i++
		}
	}
	return subs
}

var (
	shAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	// shLeads are the words that can come before a command's own word.
	shLeads = map[string]bool{"if": true, "then": true, "else": true, "elif": true, "fi": true,
		"do": true, "done": true, "while": true, "until": true, "!": true, "{": true, "}": true,
		"local": true, "export": true, "readonly": true, "exec": true}
	// shWrites are the commands that write a file they are given.
	shWrites = map[string]bool{"chown": true, "chgrp": true, "chmod": true, "install": true,
		"mkdir": true, "mktemp": true, "mv": true, "cp": true, "ln": true, "rm": true,
		"rmdir": true, "touch": true, "tee": true, "truncate": true, "curl": true}
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

// shHome is what one script calls the account's home. seeds hold the home itself, read
// from getent, and inside holds the variables built on a path inside it.
type shHome struct{ seeds, inside map[string]bool }

// names is true when word is a path inside the home. ⭐ The home itself is not: its
// parent is root's, so the account cannot put a link in its place.
func (h shHome) names(word string) bool {
	for s := range h.seeds {
		for _, e := range shRefs(word, s) {
			if e < len(word) && (word[e] == '/' || word[e] == '$') {
				return true
			}
		}
	}
	for v := range h.inside {
		if len(shRefs(word, v)) > 0 {
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
					if h.names(w) && mark(h.inside, c.words[1]) {
						changed = true
					}
				}
				continue
			}
			assigns, _, _ := shSplit(c.words)
			for _, a := range assigns {
				name, value, _ := strings.Cut(a, "=")
				switch {
				case strings.Contains(value, "getent passwd") && strings.Contains(value, "-f6"):
					changed = mark(h.seeds, name) || changed
				case h.copyOfHome(value):
					changed = mark(h.seeds, name) || changed
				case h.names(value):
					changed = mark(h.inside, name) || changed
				}
			}
		}
	}
	return h
}

func (h shHome) copyOfHome(value string) bool {
	for s := range h.seeds {
		if value == "$"+s || value == "${"+s+"}" {
			return true
		}
	}
	return false
}

// rootHomeWrites answers each write a root script makes inside the account's home: a
// command of shWrites whose argument is a path there, or a redirect onto one. ⭐ A
// command as_account runs is the account's. Its redirect is still root's, because root's
// shell opens that file.
func rootHomeWrites(src string) ([]string, shHome) {
	cmds := shCommands(src, 1)
	h := shHomeNames(cmds)
	show := func(s string) string { return strings.ReplaceAll(s, "\x00", "$") }
	var found []string
	for _, c := range cmds {
		for _, o := range c.outs {
			if h.names(o) {
				found = append(found, fmt.Sprintf("line %d: root's redirect onto %s", c.line, show(o)))
			}
		}
		if len(c.words) > 0 && c.words[0] == "for" {
			continue
		}
		_, name, args := shSplit(c.words)
		if !shWrites[path.Base(name)] {
			continue
		}
		for _, a := range args {
			if h.names(a) {
				found = append(found, fmt.Sprintf("line %d: root's %s %s", c.line, name, show(strings.Join(args, " "))))
				break
			}
		}
	}
	return found, h
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
// base, and refuses a write by root onto a path in the account's home. WSL-201.
//
// ⛔ THE CLASS, NOT THE SITES IT WAS FOUND AT. Four adapters and the provisioner each
// wrote there as root, and a fifth script would. A path is in the home when it is built
// on the variable a script reads the home into from getent, or on a variable built on
// one. The home itself is root's to make and give.
// ⚠ IT READS WRITES. A comparison root makes in the home writes nothing, and stays root's.
func TestNoRootWriteLandsInTheAccountsHome(t *testing.T) {
	scripts := rootScripts(t)
	for name, body := range scripts {
		found, _ := rootHomeWrites(string(body))
		for _, f := range found {
			t.Errorf("%s writes in the account's home as root, %s. Write it through as_account, account_dirs or account_put", name, f)
		}
	}
	// ⚠ A SCAN THAT READ NO HOME PROVES NOTHING. Each of these reads the home and names
	// paths inside it.
	for _, name := range []string{"provision.sh", "engines.sh", "adapters/herdr/install.sh", "adapters/muse/install.sh",
		"adapters/omp/install.sh", "adapters/pi/install.sh"} {
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
	cmds := shCommands(src, 1)
	h := shHomeNames(cmds)
	n := 0
	for _, c := range cmds {
		for _, w := range append(append([]string(nil), c.words...), c.outs...) {
			if h.names(w) {
				n++
			}
		}
	}
	return n
}

// TestTheHomeWriteScanReadsWhatRootWrites holds the scan above to the writes it refuses
// and the ones it lets by. WSL-201.
//
// ⛔ EACH REFUSED LINE IS ONE THE ADAPTERS CARRIED, or the shape of one: install -d -o,
// chown after a mv, a redirect, mktemp in the home, a loop over the home's directories,
// a here-document, rm in engines.sh, and the redirect of a command the account runs.
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
	}
	for _, src := range refused {
		if found, _ := rootHomeWrites(src); len(found) == 0 {
			t.Errorf("the scan let a root write in the home by:\n%s", src)
		}
	}
	allowed := []string{
		home + "chown \"$TK_USER:$TK_GROUP\" \"$TK_HOME\"\nchmod 0700 \"$TK_HOME\"",
		home + `as_account mkdir -p "$TK_HOME/.config"`,
		home + `printf x | as_account sh -c 'cat > "$1"' sh "$TK_HOME/.config/x"`,
		home + "cat > /usr/local/bin/omp <<WRAPPER\nexec \"$TK_HOME/.local/bin/omp\" \"\\$@\"\nWRAPPER\nchmod 0755 /usr/local/bin/omp",
		home + `say "wrote $TK_HOME/.config/herdr/config.toml"`,
		home + "t=$(mktemp)\nas_account jq . \"$TK_HOME/.config/x\" > \"$t\" 2>/dev/null",
		home + "f=$TK_HOME/.config/x\naccount_put \"$f\" 0600 < \"$t\" || { rm -f \"$t\"; die \"the account could not write $f\"; }",
		home + "# chown \"$TK_USER\" \"$TK_HOME/.config\" in a comment\n:",
	}
	for _, src := range allowed {
		if found, _ := rootHomeWrites(src); len(found) != 0 {
			t.Errorf("the scan refused what root may do, %q, in:\n%s", found, src)
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

// rootHandTools are root's commands that write a file, and jq, which reads one for a
// write. rootHands records each call that names a path in the account's home, or in the
// directory outside it that the case's links name.
var rootHandTools = []string{"chown", "chgrp", "chmod", "install", "mkdir", "mktemp", "mv", "cp", "ln", "rm", "rmdir", "touch", "tee", "jq"}

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
// ⭐ THE ACCOUNT'S COMMANDS COME FROM ITS OWN PATH, so a write the account makes is not
// recorded. ⚠ chown records and changes nothing, because the host has no account to give
// a file to. install drops its owner flags for the same reason. stop is a path install
// stops the script at with exit 7, as the herdr cases stop it, and jq is jq's body.
func rootHands(t *testing.T, bin, stop, jq string) {
	t.Helper()
	const record = `for a do
  case $a in
    "$STUB_HOME"/*|"$STUB_OUTSIDE"|"$STUB_OUTSIDE"/*) printf '%s\n' "TOOL $*" >> "$STUB_HANDS"; break ;;
  esac
done
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
		t.Fatalf("root wrote in the account's home, where only the account writes:\n%s\n%s", b, out)
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

// untouched fails the case when what a planted link names changed: a mode, a byte, or a
// file that was not there.
func (p planted) untouched(t *testing.T, out string) {
	t.Helper()
	info, err := os.Lstat(p.target)
	if p.absent {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s was made through the link that named it (%v):\n%s", p.target, err, out)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s is gone (%v):\n%s", p.target, err, out)
	}
	if info.Mode().Perm() != p.mode || info.IsDir() != p.dir {
		t.Fatalf("%s became %v through the link that named it, and it was %v:\n%s", p.target, info.Mode(), p.mode, out)
	}
	if !p.dir {
		if b, _ := os.ReadFile(p.target); string(b) != p.body {
			t.Fatalf("%s was written through the link that named it: %q:\n%s", p.target, b, out)
		}
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
	// ⭐ A SECOND ENSURE READS THE TRUST STORE IT FINDS, and the account reads it.
	code, out = r.runLinked(script, env...)
	handsWrote(t, r.hands, out)
	if code != 0 || !strings.HasSuffix(strings.TrimSpace(out), adapterCompleteLine("muse")) {
		t.Fatalf("a second ensure over the files the account wrote answered exit %d:\n%s", code, out)
	}
}

// TestThePiAdapterWritesNothingInTheHomeAsRoot runs pi's install.sh with ~/.pi a link to
// a directory outside the home, and its settings file a link to nothing. WSL-201.
//
// ⛔ ROOT'S install -d CHANGED THE MODE AND OWNER OF THAT DIRECTORY, and the settings
// file was made through the link, where it named.
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
	needPosix(t, "cat", "cut", "mktemp", "sha256sum", "base64", "cp", "mkdir", "chmod", "mv", "rm", "rmdir", "sed")
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

// TestProvisioningLeavesTheHomeToTheAccount runs the provisioner's XDG directories and
// the engine's settings with links planted in the home. WSL-201.
//
// ⛔ A RE-PROVISIONING RUNS AFTER THE ACCOUNT HAS RUN AGENTS AND JOBS. Root's chmod 0700
// changed the mode of whatever .cache named, root's redirect wrote the engine's settings
// through a link, and chown -R ran over a tree the account owns.
// ⭐ Root still gives back a directory that is not the account's, from inside the home,
// with chown -h, and never through a link the account made of a directory above it.
func TestProvisioningLeavesTheHomeToTheAccount(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the provisioner runs in a Linux base, on symbolic links, so the Linux job runs this case")
	}
	sh := posixShell(t)
	dir := t.TempDir()
	home, outside, hands := filepath.Join(dir, "home"), filepath.Join(dir, "outside"), filepath.Join(dir, "root-hands")
	bin, account := filepath.Join(dir, "bin"), filepath.Join(dir, "account-path")
	for _, d := range []string{filepath.Join(home, ".config", "containers", "containers.conf.d"), bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	rootHands(t, bin, "", "")
	// ⚠ root's chown here names what it gives by a name inside the directory it stands in.
	if err := os.WriteFile(filepath.Join(bin, "chown"), []byte("#!/bin/sh\nprintf 'chown %s in %s\\n' \"$*\" \"$(pwd -P)\" >> \"$STUB_HANDS\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	accountTools(t, account)
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
	section := strings.ReplaceAll(string(mirrorsScript)+"\n"+provisionerSection(t, "the engine's settings"),
		"/etc/containers/", filepath.ToSlash(dir)+"/etc/containers/")
	script := "set -eu\nTK_USER=agent\nTK_GROUP=agent\nTK_HOME=" + shellQuote(home) + "\n" +
		"say() { printf '%s\\n' \"$*\"; }\ndie() { printf 'die: %s\\n' \"$*\"; exit 3; }\npasta() { :; }\n" +
		// ⭐ runuser is the account: the host's own commands, on a PATH without root's hands.
		"runuser() { shift 3; PATH=" + shellQuote(account) + " \"$@\"; }\n" +
		markedSection(t, provisionScript, "as the account") +
		markedSection(t, provisionScript, "the account's XDG directories") + section
	cmd := exec.Command(sh, "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STUB_HOME="+home, "STUB_OUTSIDE="+outside, "STUB_HANDS="+hands)
	raw, err := cmd.CombinedOutput()
	out := string(raw)
	if err != nil {
		t.Fatalf("the provisioner's home failed: %v\n%s", err, out)
	}
	real, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	recorded, _ := os.ReadFile(hands)
	for _, line := range strings.Split(strings.TrimSpace(string(recorded)), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "chown -h ") || !strings.HasSuffix(line, " in "+real) {
			t.Errorf("root wrote in the account's home other than with chown -h from inside it: %s\n%s", line, out)
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
