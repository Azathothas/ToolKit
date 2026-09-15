// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The general-purpose files this repository publishes, carried INSIDE the
// executable so a machine with the binary needs nothing else.
//
// ⛔ WHY THEY ARE HERE AT ALL. `bootstrap.sh` and the two configuration files it
// installs are fetched by URL from outside this tree, which is the contract
// ../../../../docs/consumers.md owns and nothing here changes. What this removes
// is the OTHER way they were reached: an operator copying `bootstrap.sh` into a
// checkout by hand before a base could be provisioned, with no way to tell which
// version they copied. An executable that carries them can run them, and
// `shipped` says exactly which bytes it holds.
//
// ⚠ SO THERE IS A COPY, and the gate's `shipped` check refuses it disagreeing
// with the definition, as `adapters` and `package-table` already do for theirs.
// The copies live here because go:embed cannot reach out of its own package
// directory. Rewrite them with:
//
//	sh scripts/common/check.sh shipped --fix
//
//go:embed shipped
var shippedTree embed.FS

// ShippedFile is one file the executable carries, with the path it came from.
type ShippedFile struct {
	// Name is how a caller asks for it: `bootstrap.sh`, `tmux.conf`.
	Name string `json:"name"`
	// Source is where it lives in this repository, for a reader who wants the
	// original rather than the copy.
	Source string `json:"source"`
	// Bytes is its length, and SHA256 its digest, so a caller can say which
	// version they have without this tool being asked to trust its own name.
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
	// Mode is 0755 for a file that is run and 0644 for one that is read.
	Mode fs.FileMode `json:"-"`
}

// shippedModes says which carried files are executable. ⛔ A configuration file
// written executable is a file somebody eventually runs.
var shippedModes = map[string]fs.FileMode{
	"bootstrap.sh":     0o755,
	"shell-profile.sh": 0o644,
	"tmux.conf":        0o644,
}

// ShippedSource is where each carried file's definition lives.
const ShippedSource = "scripts/common/"

// ShippedFiles is every file this executable carries, sorted by name.
func ShippedFiles() ([]ShippedFile, error) {
	entries, err := shippedTree.ReadDir("shipped")
	if err != nil {
		return nil, err
	}
	out := make([]ShippedFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		body, err := shippedTree.ReadFile("shipped/" + e.Name())
		if err != nil {
			return nil, err
		}
		mode, ok := shippedModes[e.Name()]
		if !ok {
			// ⛔ A FILE WITH NO DECLARED MODE IS A FILE NOBODY DECIDED ABOUT, and
			// guessing executable is the guess that costs something.
			mode = 0o644
		}
		out = append(out, ShippedFile{
			Name:   e.Name(),
			Source: ShippedSource + e.Name(),
			Bytes:  len(body),
			SHA256: sha256Hex(body),
			Mode:   mode,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ShippedBody answers one carried file's bytes.
func ShippedBody(name string) ([]byte, error) {
	if err := assertShippedName(name); err != nil {
		return nil, err
	}
	body, err := shippedTree.ReadFile("shipped/" + name)
	if err != nil {
		names, _ := ShippedFiles()
		have := make([]string, 0, len(names))
		for _, f := range names {
			have = append(have, f.Name)
		}
		return nil, fmt.Errorf("this executable carries no %q. It carries: %s", name, strings.Join(have, ", "))
	}
	return body, nil
}

// assertShippedName refuses anything but a bare file name.
//
// ⛔ IT IS AN EMBEDDED FILESYSTEM AND A PATH STILL MATTERS. `ReadFile` on an
// embed.FS does not walk out of the tree, so this is not a traversal guard; it is
// here so that a caller who passes a path gets a refusal naming the rule rather
// than a "no such file" about a name they think they gave correctly.
func assertShippedName(name string) error {
	if name == "" {
		return fmt.Errorf("name one of the files this executable carries")
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return fmt.Errorf("%q is a path, and this takes the bare name of a carried file", name)
	}
	return nil
}

// WriteShipped writes one carried file into a directory and answers where.
//
// ⛔ IT REFUSES TO OVERWRITE SOMETHING THAT IS NOT ITS OWN. A file already there
// whose content differs is a file somebody edited, and replacing it silently is
// how an operator's change disappears. Identical content is left alone rather
// than rewritten, so a second run changes nothing and no timestamp moves.
func WriteShipped(name, dir string, force bool) (string, bool, error) {
	body, err := ShippedBody(name)
	if err != nil {
		return "", false, err
	}
	files, err := ShippedFiles()
	if err != nil {
		return "", false, err
	}
	mode := fs.FileMode(0o644)
	for _, f := range files {
		if f.Name == name {
			mode = f.Mode
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, err
	}
	dest := filepath.Join(dir, name)
	if current, err := os.ReadFile(dest); err == nil {
		if string(current) == string(body) {
			return dest, false, nil
		}
		if !force {
			return dest, false, fmt.Errorf("%s is already there and differs from the one this executable carries. Read it, then pass --force to replace it", dest)
		}
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, body, mode); err != nil {
		return "", false, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return "", false, err
	}
	// ⚠ Rename keeps the temporary file's mode on some filesystems and not
	// others, so the mode is set again rather than assumed.
	if err := os.Chmod(dest, mode); err != nil {
		return dest, true, err
	}
	return dest, true, nil
}

// sha256Hex is a file's digest as sha256sum prints it. ⚠ Lowercase hex, so a
// caller can compare it with what the guest's own sha256sum answers without
// normalising either side.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ShippedScript is a carried script and a caller's arguments, as one payload.
//
// ⛔ THE SCRIPT IS DATA AND THE ARGUMENTS ARE QUOTED. The body reaches the guest
// inside a here-document with a quoted delimiter, so nothing in it is expanded on
// the way; the arguments are quoted one by one into the line that runs it. A
// caller's `--with 'a b'` is one argument, and a caller's `$(id)` is four
// characters.
//
// ⭐ NO TEMPORARY FILE, AND THAT IS A DOOR SWEEP FINDING RATHER THAN A STYLE.
// An earlier draft wrote the body to `$(mktemp)` and removed it in a trap. The
// base this runs in may carry `base.toolset: "none"`, where coreutils is not
// installed and `mktemp` is absent - and a fallback to a predictable path under
// /tmp is a file another process can put a symlink at first. Reading the script
// from a file descriptor needs neither.
//
// ⚠ DESCRIPTOR 8, BECAUSE THE FRAME AROUND THIS USES 9. Every payload this tool
// sends goes through FramePayload, which sources the whole of it from `/dev/fd/9`
// with the command's stdin on /dev/null. So `/dev/fd` is already a dependency and
// already proved across the thirteen catalogue images; this takes the next
// descriptor down and leaves the script's own stdin as the frame set it.
//
// ⚠ THE DELIMITER IS NOT A GUESS. It carries a random component, because a script
// that happened to contain the delimiter on a line of its own would otherwise end
// its own here-document and the rest would run as commands.
func ShippedScript(body []byte, args []string) []byte {
	delim := "TK_SHIPPED_" + randomToken()
	var b strings.Builder
	b.WriteString("{ sh /dev/fd/8")
	for _, a := range args {
		b.WriteString(" ")
		b.WriteString(shellQuote(a))
	}
	b.WriteString("; } 8<<'" + delim + "'\n")
	b.Write(body)
	if len(body) > 0 && body[len(body)-1] != '\n' {
		b.WriteString("\n")
	}
	b.WriteString(delim + "\n")
	return []byte(b.String())
}

// randomToken is 16 hex characters from the system's own source.
//
// ⛔ NOT A TIMESTAMP AND NOT A COUNTER. This ends a here-document, so a value a
// script's own content could contain is a value that ends it early.
func randomToken() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// ⚠ crypto/rand failing is a broken machine, and a fixed delimiter here
		// would be the one case where this is unsafe. Refuse by making the
		// delimiter impossible to type instead.
		return "READ_FAILED_" + hex.EncodeToString([]byte(err.Error()))
	}
	return strings.ToUpper(hex.EncodeToString(b))
}
