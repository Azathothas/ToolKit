// SPDX-License-Identifier: 0BSD

package checks

import (
	"path"
	"regexp"
	"strings"
)

// Secrets refuses a credential, or a fingerprint of a private system, reaching
// a remote.
//
// ⛔ ONCE IT DOES, A HISTORY REWRITE DOES NOT UNDO IT. The value was readable,
// and it may be cached, mirrored or already indexed. Rotation is the fix; this
// is what stops it needing one.
//
// ⛔ IT FINDS THE SHAPES IT KNOWS, AND A GREEN RUN IS NOT A CLEARANCE. It
// cannot find a password that looks like a word, a hostname that reads as
// prose, or a page of correct-looking examples that happens to describe a real
// system. It narrows the reading. It does not replace it.
//
// The public rules are on by default here, because this repository IS public.
// In a private project an email address and a home path are legitimate content,
// which is why the shell implementation kept them behind a flag.
func Secrets(t *Tree) Result {
	r := Result{Extra: map[string]any{"public_rules": true}}

	// 1. A credential FILE is tracked. The strongest signal there is: not a
	//    value that looks like a secret, but a file whose whole purpose is to
	//    hold one.
	for _, f := range t.Files {
		base := path.Base(f)
		if credentialSample.MatchString(base) {
			continue
		}
		if credentialFile.MatchString(base) {
			r.bad("%s is a credential file and it is tracked", f)
		}
	}

	// 2. Secret-shaped strings. Each pattern is a vendor's documented token
	//    shape. ⚠ A generic high-entropy rule is deliberately absent: it fires
	//    on hashes, minified code and base64 fixtures, and a check that cries
	//    wolf is a check somebody switches off.
	n := 0
	for _, f := range t.Files {
		b := t.Read(f)
		if len(b) == 0 || !looksTextual(b) {
			continue
		}
		n++
		self := strings.HasSuffix(f, "internal/checks/secrets.go")
		for _, ln := range Lines(b) {
			for _, rule := range secretShapes {
				if m := rule.re.FindString(ln.Text); m != "" {
					r.bad("%s:%d: %s", f, ln.N, rule.name)
				}
			}
			// ⛔ THE FILE THAT HOLDS THE PATTERNS IS EXEMPT FROM THE PUBLIC
			// RULES AND FROM NOTHING ELSE. It names generic home paths in its
			// own exclusions, and a check that reported itself would be one
			// nobody could make green.
			if self {
				continue
			}
			if m := emailRe.FindString(ln.Text); m != "" && !exemptEmail(m) {
				r.bad("%s:%d: an email address: %s", f, ln.N, m)
			}
			if m := homePathRe.FindString(ln.Text); m != "" && !genericHome(m) {
				r.bad("%s:%d: an absolute home path: %s", f, ln.N, m)
			}
			if m := longHexRe.FindString(ln.Text); m != "" && !declaredPin(ln.Text) {
				r.bad("%s:%d: a long hex identifier: %s", f, ln.N, m)
			}
		}
	}
	r.Extra["files"] = n
	return r
}

type secretShape struct {
	name string
	re   *regexp.Regexp
}

var secretShapes = []secretShape{
	{"a private key block", regexp.MustCompile(`BEGIN (RSA |OPENSSH |EC |DSA |PGP )?PRIVATE KEY`)},
	{"an aws access key id", regexp.MustCompile(`AKIA[0-9A-Z]{16}`)},
	{"a github token", regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{30,}`)},
	{"a slack token", regexp.MustCompile(`xox[abprs]-[0-9A-Za-z-]{10,}`)},
	{"a google api key", regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`)},
	{"a stripe key", regexp.MustCompile(`sk_(live|test)_[0-9A-Za-z]{16,}`)},
	{"a npm token", regexp.MustCompile(`npm_[A-Za-z0-9]{36}`)},
	{"a bearer literal", regexp.MustCompile(`Bearer [A-Za-z0-9._-]{24,}`)},
	{"a password in a url", regexp.MustCompile(`://[A-Za-z0-9._%+-]+:[^@/\s]{6,}@`)},
}

var (
	credentialFile   = regexp.MustCompile(`^(\.env(\..+)?|\.dev\.vars(\..+)?|.*\.(pem|key|p12|pfx|keystore|jks)|id_rsa|id_ed25519|id_ecdsa|credentials\.json|service-account.*\.json)$`)
	credentialSample = regexp.MustCompile(`\.(example|sample|template)$`)
	emailRe          = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	homePathRe       = regexp.MustCompile(`([A-Za-z]:[\/]Users[\/]|/home/|/Users/)[A-Za-z0-9._-]+`)
	longHexRe        = regexp.MustCompile(`\b[0-9a-f]{24,}\b`)
	pinnedAction     = regexp.MustCompile(`uses:\s*[A-Za-z0-9._-]+/[A-Za-z0-9._-]+@[0-9a-f]{40}`)
	declaredPinRe    = regexp.MustCompile(`[Pp]inned(Ref|Sha256|Commit|Digest)|PINNED_(REF|SHA256)`)
)

// ⚠ Narrowed rather than switched off. These are well-known generic paths, not
// a fingerprint of anybody's machine, and a check that fires on them is one
// somebody disables. `/home/toolkit/` is the same shape from this repository's
// own side: the account wsl-toolkit creates inside the distribution it owns,
// identical on every machine it runs on.
var genericHomeRe = regexp.MustCompile(`/home/(linuxbrew|runner|user|vagrant|ubuntu|node|toolkit)/|/Users/(runner|user)/`)

func genericHome(m string) bool { return genericHomeRe.MatchString(m + "/") }

// ⚠ A pinned GitHub Action is a 40-hex commit on a PUBLIC repository, and
// pinning is the SAFE practice this repository asks for: a tag moves and a
// moved tag runs unreviewed code. A rule that fires on correct hardening is a
// rule somebody disables, so the two shapes are excluded BY NAME rather than
// the whole hex rule being dropped.
func declaredPin(line string) bool {
	return pinnedAction.MatchString(line) || declaredPinRe.MatchString(line)
}

// exemptEmail passes the addresses that are the project's own published
// contact rather than a person's private one.
func exemptEmail(m string) bool {
	return strings.HasSuffix(m, "@users.noreply.github.com") ||
		strings.HasPrefix(m, "noreply@")
}

// looksTextual keeps the scan off binary content, which git tracks and grep -I
// skipped for the same reason.
func looksTextual(b []byte) bool {
	limit := len(b)
	if limit > 8000 {
		limit = 8000
	}
	for i := 0; i < limit; i++ {
		if b[i] == 0 {
			return false
		}
	}
	return true
}
