// SPDX-License-Identifier: 0BSD

package compat

import (
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// The command channel, ported from the script's command-channel.ps1 and
// distro-exec.ps1.
//
// -Command, -CommandFile and -CommandB64 are three ways of handing over the
// same thing: bytes. They travel to the guest as base64 and are decoded INSIDE
// the distro, so a quote, a dollar sign, a backtick or a tab arrives
// byte-exact instead of being re-parsed in transit.
//
// WHAT DOES NOT SURVIVE the un-mediated trip, measured against real distros
// under both PowerShell hosts, with every hazard already correctly
// single-quoted for sh before it was passed:
//
//	$VAR       expanded in transit, and the RESULT then re-parsed
//	backtick   opens a command substitution
//	"          survives on PowerShell 7 and does NOT on 5.1
//
// The single quotes never reach the guest, so a caller cannot fix this by
// quoting harder. What survives is base64: [A-Za-z0-9+/=], plus the operators
// the transport skeleton needs.

// guestScratchPath is a path for the transport file inside the guest. It is
// random per call because two concurrent Run commands against one distro must
// not write each other's file. The window is microseconds wide and it costs
// eight characters to close it.
func guestScratchPath() string {
	return "/tmp/.wsl-eph-" + randomSuffix(8)
}

// guestUserEnvironmentPrelude is the shell prologue -UserEnv prepends, as ONE
// home for both of the defects it answers.
//
//  1. ⛔ ROOTLESS PODMAN NEEDS XDG_RUNTIME_DIR AND runuser LEAVES IT UNSET.
//     A build then dies at `cannot create state directory for buildah-...`,
//     which reads as a broken image and is a missing directory.
//  2. ⚠ A GUEST'S PATH IS NOT THE PATH ITS TOOLS WERE INSTALLED ONTO. A login
//     shell in an imported distro carries WSL's PATH, not the image's.
//
// ⛔ It installs nothing, sources no account rc file and creates exactly one
// directory. Preparing an environment is not the same act as provisioning a
// machine.
func guestUserEnvironmentPrelude() string {
	// ⚠ THE DIRECTORY ORDER IS THE CONTRACT. A tool a user installed for
	// themselves wins over a system copy of the same name. sbin comes after
	// bin so an unprivileged caller reaches the ordinary tool first.
	dirs := []string{
		"$HOME/.local/bin", "$HOME/bin", "$HOME/.cargo/bin", "$HOME/go/bin",
		"$HOME/.bun/bin", "$HOME/.deno/bin", "$HOME/.nix-profile/bin",
		"/nix/var/nix/profiles/default/bin",
		"/usr/local/go/bin", "/usr/local/cargo/bin",
		"/usr/local/bin", "/usr/bin", "/bin",
		"/usr/local/sbin", "/usr/sbin", "/sbin",
	}
	lines := []string{
		"_wtk_uid=$(id -u) 2>/dev/null || { echo \"wsl-toolkit: this guest has no id command\" >&2; exit 2; }",
		"_wtk_run=/tmp/wsl-toolkit-run-$_wtk_uid",
		// umask rather than a chmod afterwards: a directory created 0755 and
		// narrowed a moment later is world-readable for that moment.
		"(umask 077; mkdir \"$_wtk_run\") 2>/dev/null || :",
		"if [ -L \"$_wtk_run\" ]; then echo \"wsl-toolkit: $_wtk_run is a symlink and will not be used\" >&2; exit 2; fi",
		"if [ ! -d \"$_wtk_run\" ]; then echo \"wsl-toolkit: could not create $_wtk_run\" >&2; exit 2; fi",
		// ⛔ THREE OUTCOMES, NOT TWO. A guest with no `stat` and a directory
		// owned by somebody else are different facts.
		"if command -v stat >/dev/null 2>&1; then",
		"    _wtk_own=$(stat -c %u \"$_wtk_run\" 2>/dev/null) || _wtk_own=",
		"    if [ -z \"$_wtk_own\" ]; then echo \"wsl-toolkit: stat cannot read $_wtk_run\" >&2; exit 2; fi",
		"    if [ \"$_wtk_own\" != \"$_wtk_uid\" ]; then echo \"wsl-toolkit: $_wtk_run belongs to uid $_wtk_own, not $_wtk_uid\" >&2; exit 2; fi",
		"fi",
		"chmod 700 \"$_wtk_run\" || exit 2",
		"XDG_RUNTIME_DIR=$_wtk_run; export XDG_RUNTIME_DIR",
		// A per-uid TMPDIR under the runtime directory, so what a job leaves
		// behind is one directory the host can measure and remove.
		"(umask 077; mkdir \"$_wtk_run/tmp\") 2>/dev/null || :",
		"if [ -d \"$_wtk_run/tmp\" ] && [ ! -L \"$_wtk_run/tmp\" ]; then TMPDIR=$_wtk_run/tmp; export TMPDIR; fi",
		"_wtk_path=",
		"_wtk_add() {",
		"    case \":$_wtk_path:\" in *\":$1:\"*) return 0 ;; esac",
		"    [ -d \"$1\" ] || return 0",
		"    _wtk_path=${_wtk_path:+$_wtk_path:}$1",
		"}",
		"for _wtk_dir in " + strings.Join(dirs, " ") + "; do _wtk_add \"$_wtk_dir\"; done",
		// The inherited PATH goes through the same function, which both keeps
		// its entries and removes the copies this prologue has already added.
		"_wtk_old=$PATH",
		"while [ -n \"$_wtk_old\" ]; do",
		"    case \"$_wtk_old\" in *:*) _wtk_one=${_wtk_old%%:*}; _wtk_old=${_wtk_old#*:} ;; *) _wtk_one=$_wtk_old; _wtk_old= ;; esac",
		"    [ -n \"$_wtk_one\" ] && _wtk_add \"$_wtk_one\"",
		"done",
		"PATH=$_wtk_path; export PATH",
		"unset -f _wtk_add 2>/dev/null || :",
		"unset _wtk_uid _wtk_run _wtk_own _wtk_path _wtk_dir _wtk_old _wtk_one",
	}
	return strings.Join(lines, "\n")
}

// addGuestUserEnvironment prepends the prologue above, then the caller's
// bytes, unchanged.
//
// ⛔ THE CALLER'S BYTES ARE THE SUFFIX AND NOTHING IS SUBSTITUTED INTO THEM.
func addGuestUserEnvironment(script []byte) []byte {
	pre := append([]byte(guestUserEnvironmentPrelude()), '\n')
	out := make([]byte, 0, len(pre)+len(script))
	return append(append(out, pre...), script...)
}

// convertFromCommandFileBytes is a file's bytes, made into something /bin/sh
// can read, with every change named on the way past.
//
// ⛔ THE FILE ON DISK IS NEVER WRITTEN TO. The distinction this function draws
// is between THE FILE, which is the caller's and is never touched, and THE
// COPY IN TRANSIT, which is this tool's to make correct.
//
//	UTF-16     REFUSED by name: its bytes are NUL-interleaved, so /bin/sh
//	           stops at the first one and the command does nothing, silently.
//	UTF-8 BOM  removed: it is three bytes ahead of the first line, so a
//	           leading `#!/bin/sh` is not a comment any more.
//	CRLF       turned into LF: /bin/sh reads the carriage return as part of
//	           the last word on every line.
//
// ⚠ verbatim TURNS ALL THREE OFF and sends the bytes exactly as they are. The
// UTF-16 refusal becomes a warning there, because refusing bytes somebody
// explicitly asked to send would be this function overriding them.
func (s *session) convertFromCommandFileBytes(bytes []byte, source string, verbatim bool) ([]byte, error) {
	utf16 := len(bytes) >= 2 &&
		((bytes[0] == 0xFF && bytes[1] == 0xFE) || (bytes[0] == 0xFE && bytes[1] == 0xFF))

	if verbatim {
		if utf16 {
			s.log.ok(source + " starts with a UTF-16 byte order mark and -Verbatim was passed, so it is sent as-is. /bin/sh will stop at the first NUL byte.")
		}
		if containsByte(bytes, 13) {
			s.log.ok(source + " has carriage returns and -Verbatim was passed, so they are sent as-is. /bin/sh reads a CR as part of the last word on the line.")
		}
		return bytes, nil
	}

	if utf16 {
		return nil, fmt.Errorf("%s is UTF-16: its bytes carry a NUL after nearly every character, and "+
			"/bin/sh stops at the first one, so the command would do nothing and say nothing. "+
			"Save it as UTF-8, or pass -Verbatim if sending it unchanged is what you meant.", source)
	}

	out := bytes
	bom := false
	if len(out) >= 3 && out[0] == 0xEF && out[1] == 0xBB && out[2] == 0xBF {
		out = out[3:]
		bom = true
	}

	// ⛔ CRLF ONLY, never a lone CR. A carriage return that is not followed by
	// a newline is a deliberate one: a progress meter, or a literal in a
	// here-doc the script writes out. Turning that into a newline would edit
	// the payload rather than repair it, which is exactly the line this
	// function does not cross.
	crlf := 0
	keep := make([]byte, len(out))
	n := 0
	for i := 0; i < len(out); i++ {
		if out[i] == 13 && i+1 < len(out) && out[i+1] == 10 {
			crlf++
			continue
		}
		keep[n] = out[i]
		n++
	}
	if crlf > 0 {
		out = keep[:n]
	}

	if bom {
		s.log.ok(source + " carried a UTF-8 byte order mark; it was left out of the copy being sent.")
	}
	if crlf > 0 {
		s.log.ok(fmt.Sprintf("%s has %d CRLF line ending(s); the copy being sent uses LF.", source, crlf))
		s.log.ok("the file on disk was NOT modified. Pass -Verbatim to send its bytes exactly.")
	}
	return out, nil
}

func containsByte(b []byte, c byte) bool {
	for _, x := range b {
		if x == c {
			return true
		}
	}
	return false
}

// scriptArgPrologue is -ScriptArg NAME=VALUE, as POSIX shell assignments ahead
// of the command.
//
// ⭐ VALUES ARE ASSIGNED, NEVER SUBSTITUTED INTO THE SCRIPT. The alternative is
// a caller running sed over their own file to inject a URL. Nothing in the
// caller's bytes is rewritten here: text is added in front of them,
// single-quoted, so a value can contain anything at all.
//
// @hostaddress IS EXPANDED, and it is the one substitution that happens: a
// caller wiring a guest at a fixture on this host needs the NAT address,
// which -Action HostAddress already answers without creating a distro.
func (s *session) scriptArgPrologue(pairs []string) (string, error) {
	var lines []string
	addr := ""
	for _, pair := range pairs {
		eq := strings.Index(pair, "=")
		if eq < 1 {
			return "", fmt.Errorf("-ScriptArg '%s' is not NAME=VALUE. The name comes first, then one equals sign, then the value, which may be empty.", pair)
		}
		name, value := pair[:eq], pair[eq+1:]
		if !isShellIdentifier(name) {
			return "", fmt.Errorf("-ScriptArg name '%s' is not a POSIX shell variable name. It must start with "+
				"a letter or an underscore and carry only letters, digits and underscores.", name)
		}
		if strings.Contains(value, "@hostaddress") {
			if addr == "" {
				a, err := s.resolveHostAddress()
				if err != nil {
					return "", err
				}
				addr = a.Address
			}
			value = strings.ReplaceAll(value, "@hostaddress", addr)
			s.log.ok("-ScriptArg " + name + " : @hostaddress resolved to " + addr + ". WSL assigns it and it changes; it is read here and not recorded.")
		}
		lines = append(lines, name+"="+shellSingleQuoted(value))
		lines = append(lines, "export "+name)
	}
	if len(lines) == 0 {
		return "", nil
	}
	return strings.Join(lines, "\n") + "\n", nil
}

// scriptArgPairs is every NAME=VALUE pair for this run, from the file first
// and then the flag.
//
// ⭐ THE FILE EXISTS BECAUSE A .ps1 RUN THROUGH -File CANNOT HAVE A PARAMETER
// REPEATED, which is how every consumer ran the script. The Go parser accepts
// repeats, and the file keeps working, so a caller moving here loses nothing.
//
// ⛔ A FILE RATHER THAN A DELIMITER, because there is no safe delimiter: a
// VALUE is arbitrary, and a URL query string carries commas. THE FILE'S BYTES
// GO THROUGH the same repair -CommandFile gets. ⛔ The file on disk is never
// written to.
func (s *session) scriptArgPairs(fromFile string, pairs []string) ([]string, error) {
	var out []string
	if fromFile != "" {
		st, err := os.Stat(fromFile)
		if err != nil || st.IsDir() {
			return nil, fmt.Errorf("-ScriptArgFile not found: %s", fromFile)
		}
		raw, err := os.ReadFile(fromFile)
		if err != nil {
			return nil, err
		}
		repaired, err := s.convertFromCommandFileBytes(raw, fromFile, s.opts.Verbatim)
		if err != nil {
			return nil, err
		}
		text := string(repaired)
		for n, line := range strings.Split(text, "\n") {
			t := strings.TrimSpace(line)
			if t == "" || strings.HasPrefix(t, "#") {
				continue
			}
			if strings.Index(t, "=") < 1 {
				return nil, fmt.Errorf("-ScriptArgFile %s line %d: '%s' is not NAME=VALUE.", fromFile, n+1, t)
			}
			out = append(out, t)
		}
	}
	for _, p := range pairs {
		if p != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

// resolveCommandBytes collapses the three spellings to one thing, so every
// action downstream sees bytes and no action has to know which switch the
// caller used.
//
// nil means NO command was given, which is different from an empty one: New
// with no -Command is a distro that gets created and kept.
//
// -ScriptArg IS PREPENDED HERE, in this one place, so a value reaches a
// command written as text, as a file and as base64 identically.
func (s *session) resolveCommandBytes() ([]byte, error) {
	o := s.opts
	var given []string
	if o.Command != "" {
		given = append(given, "-Command")
	}
	if o.CommandFile != "" {
		given = append(given, "-CommandFile")
	}
	if o.CommandB64 != "" {
		given = append(given, "-CommandB64")
	}
	pairs, err := s.scriptArgPairs(o.ScriptArgFile, o.ScriptArg)
	if err != nil {
		return nil, err
	}
	if len(given) > 1 {
		return nil, fmt.Errorf("Pass only one of %s. They are three spellings of the same argument.", strings.Join(given, ", "))
	}
	if len(given) == 0 {
		// ⛔ REFUSED RATHER THAN IGNORED. A caller who passed -ScriptArg and no
		// command has made a mistake this tool can see.
		if len(pairs) > 0 {
			return nil, fmt.Errorf("-ScriptArg or -ScriptArgFile was given with no command to pass it to. Add -Command, -CommandFile or -CommandB64.")
		}
		return nil, nil
	}
	if o.Verbatim && o.CommandFile == "" {
		return nil, fmt.Errorf("-Verbatim applies to -CommandFile, which is the only spelling whose bytes come off disk. %s is already exactly what you passed.", given[0])
	}

	var body []byte
	switch {
	case o.Command != "":
		body = []byte(o.Command)
	case o.CommandB64 != "":
		decoded, err := base64.StdEncoding.DecodeString(o.CommandB64)
		if err != nil {
			return nil, fmt.Errorf("-CommandB64 is not valid base64: %v", err)
		}
		body = decoded
	default:
		st, err := os.Stat(o.CommandFile)
		if err != nil || st.IsDir() {
			return nil, fmt.Errorf("-CommandFile not found: %s", o.CommandFile)
		}
		raw, err := os.ReadFile(o.CommandFile)
		if err != nil {
			return nil, err
		}
		body, err = s.convertFromCommandFileBytes(raw, o.CommandFile, o.Verbatim)
		if err != nil {
			return nil, err
		}
	}

	if len(pairs) > 0 {
		prologue, err := s.scriptArgPrologue(pairs)
		if err != nil {
			return nil, err
		}
		pre := []byte(prologue)
		merged := make([]byte, 0, len(pre)+len(body))
		merged = append(merged, pre...)
		merged = append(merged, body...)
		body = merged
	}
	return body, nil
}

// transportPathShape is the alphabet a guest path is allowed to carry. Every
// path this interface writes is one it chose, and a path arriving with
// anything else in it is a bug rather than an intention.
var transportPathShape = regexp.MustCompile(`^/[A-Za-z0-9_.\-]+(/[A-Za-z0-9_.\-]+)*$`)

// distroScriptCommand is THE ONE PLACE THE TRANSPORT SKELETON IS BUILT, and
// the reason there is only one is that a payload hand-written inside the safe
// alphabet was a constraint nothing enforced.
//
// THE SKELETON, and each link earns its place:
//
//	mkdir -p /tmp   a rootfs exported from a scratch image may have no /tmp.
//	exec 8>PATH     create the transport for writing...
//	exec 9<PATH     ...open a reader on it...
//	rm -f PATH      ...and UNLINK IT BEFORE ANY CONTENT EXISTS. Both
//	                descriptors keep the inode alive, so the command's text
//	                is never a file anybody can read.
//	base64 -d>&8    the decode. && means a guest with no base64 STOPS here
//	                instead of sourcing an empty file and reporting success
//	                over a command that never ran.
//	. /dev/fd/9     source it from the open descriptor. The login shell runs
//	                it, so /etc/profile and -OciEnv still apply.
//
// ⚠ THE ORDER IS THE POINT. Writing the file and then unlinking it reads the
// same and is not: a redirect CREATES the file before the decode runs, so a
// guest with no base64 was left holding an empty /tmp file that nothing
// removed.
func distroScriptCommand(script []byte, guestPath string) (string, error) {
	if !transportPathShape.MatchString(guestPath) {
		return "", fmt.Errorf("Transport path '%s' is outside the alphabet that survives the trip.", guestPath)
	}
	b64 := base64.StdEncoding.EncodeToString(script)
	line := "mkdir -p /tmp&&exec 8>" + guestPath + "&&exec 9<" + guestPath + "&&rm -f " + guestPath +
		"&&echo " + b64 + "|base64 -d>&8&&. /dev/fd/9"

	// ⛔ THE CHECK THAT DID NOT EXIST. Every payload now reaches the guest as
	// base64, so nothing a caller writes can break the alphabet; this catches
	// the OTHER direction, an edit to the skeleton that adds a character the
	// measurement never cleared.
	if bad := transportAlphabet.FindString(line); bad != "" {
		return "", fmt.Errorf("Transport skeleton carries '%s', which is outside the alphabet measured "+
			"to survive a command line to /bin/sh. Re-measure before widening it.", bad)
	}
	return line, nil
}

var transportAlphabet = regexp.MustCompile(`[^A-Za-z0-9+/=|<>&;. _-]`)
