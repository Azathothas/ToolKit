// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// A caller's command reaches a guest shell through one channel, built here:
// the bytes the caller named, repaired in the copy being sent, composed with the
// environment the caller asked for, and framed so the shell reads all of it
// before it runs any of it.

// RepairReport says what RepairGuestScriptReport changed in the copy it made.
type RepairReport struct {
	CRLF       int  `json:"crlf,omitempty"`
	BOMRemoved bool `json:"bom_removed,omitempty"`
	NewlineAdd bool `json:"newline_added,omitempty"`
}

// Changed reports whether the copy differs from the bytes that were read.
func (r RepairReport) Changed() bool { return r.CRLF > 0 || r.BOMRemoved || r.NewlineAdd }

// EnvPair is one NAME=VALUE a caller passed, in the order it was passed.
//
// ⚠ ORDERED, NOT A MAP. A file of pairs and the flags after it are applied in
// that order, so a later value for the same name wins the way a shell would
// make it win, and a map would decide the winner by hash order.
type EnvPair struct {
	Name  string `json:"name"`
	Value string `json:"-"`
}

// ParseEnvPair splits one NAME=VALUE and refuses a name no shell can assign.
func ParseEnvPair(pair string) (EnvPair, error) {
	name, value, ok := strings.Cut(pair, "=")
	if !ok || name == "" {
		return EnvPair{}, fmt.Errorf("%q is not NAME=VALUE. The name comes first, then one equals sign, then the value, which may be empty", pair)
	}
	if !isShellName(name) {
		return EnvPair{}, fmt.Errorf("%q: %q is not a usable environment name. A name is letters, digits and underscores, and does not start with a digit", pair, name)
	}
	return EnvPair{Name: name, Value: value}, nil
}

// ParseEnvFile reads a file of NAME=VALUE lines.
//
// ⛔ A FILE RATHER THAN A DELIMITER, because a value is arbitrary text and has no
// safe separator: a URL query string carries commas and semicolons, and splitting
// on one would corrupt the value being passed. Blank lines and lines starting
// with # are skipped. The bytes get the repair a script gets, so a file written on
// Windows needs no conversion, and a line that is not a pair is refused by number.
func ParseEnvFile(raw []byte, source string) ([]EnvPair, error) {
	repaired, _, err := RepairGuestScriptReport(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", source, err)
	}
	var pairs []EnvPair
	for i, line := range strings.Split(string(repaired), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		p, err := ParseEnvPair(t)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", source, i+1, err)
		}
		pairs = append(pairs, p)
	}
	return pairs, nil
}

// ComposePayload is the one place a command, its environment and the user
// environment become the bytes a guest runs.
//
// The order is the contract: the user environment prologue, then the caller's
// assignments in the order given, then the command's bytes unchanged. A value the
// caller set explicitly therefore wins over what the prologue prepares, and the
// command is an exact suffix whichever spelling carried it.
func ComposePayload(command []byte, env []EnvPair, userEnv bool) []byte {
	var b bytes.Buffer
	if userEnv {
		b.WriteString(userEnvironmentPrelude)
	}
	for _, p := range env {
		b.WriteString(p.Name)
		b.WriteString("=")
		b.WriteString(shellQuote(p.Value))
		b.WriteString("; export ")
		b.WriteString(p.Name)
		b.WriteString("\n")
	}
	b.Write(command)
	return b.Bytes()
}

// payloadDelimiterPrefix begins the line that ends a framed payload.
const payloadDelimiterPrefix = "WTK_PAYLOAD_"

// ErrPayloadNUL refuses a payload the frame cannot carry.
var ErrPayloadNUL = errors.New("the command carries a NUL byte, which a here-document cannot carry and no POSIX shell reads past")

// FramePayload wraps a caller's payload for a guest shell reading its script
// from stdin.
//
// ⛔ WITHOUT THE FRAME, A COMMAND THAT READS STDIN EATS THE REST OF THE SCRIPT.
// The shell reads the script from the same pipe the command inherits, so `cat`,
// `read` or a prompting installer consumes the lines after it, and the run exits 0
// over commands that never ran. Measured on 2026-09-13 through wsl.exe into
// Alpine: `cat >/dev/null`, 20 KiB of comments and `echo` ran nothing after `cat`.
//
// The frame is one compound command. Its quoted here-document is parsed whole
// before anything executes, `.` sources it from a descriptor the shell duplicates
// for itself, and `</dev/null` is the command's stdin. Measured on busybox ash,
// dash, bash and Chimera's sh across the thirteen catalog images: a command
// reading stdin, reuse of descriptor 9, `return` at top level, `set -e`, and a
// payload with no final newline all behaved as the unframed script does.
func FramePayload(payload []byte) ([]byte, error) {
	if bytes.IndexByte(payload, 0) >= 0 {
		return nil, ErrPayloadNUL
	}
	delim, err := payloadDelimiter(payload)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	// A rootfs exported from a minimal image may have no /tmp, and a shell that
	// keeps a here-document in a temporary file fails naming nothing useful.
	b.WriteString("[ -d /tmp ] || mkdir -p /tmp 2>/dev/null || :\n")
	b.WriteString("{ . /dev/fd/9; } 9<<'")
	b.WriteString(delim)
	b.WriteString("' </dev/null\n")
	b.Write(payload)
	if len(payload) > 0 && payload[len(payload)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString(delim)
	b.WriteByte('\n')
	return b.Bytes(), nil
}

// payloadDelimiter draws a delimiter that appears nowhere in the payload.
//
// ⚠ A LINE EQUAL TO THE DELIMITER WOULD END THE PAYLOAD EARLY and hand the rest
// to the shell as ordinary script, so the check is on the whole payload rather
// than trusting 128 random bits to be enough by themselves.
func payloadDelimiter(payload []byte) (string, error) {
	return payloadDelimiterFrom(rand.Reader, payload)
}

func payloadDelimiterFrom(random io.Reader, payload []byte) (string, error) {
	for attempt := 0; attempt < 4; attempt++ {
		raw := make([]byte, 16)
		if _, err := io.ReadFull(random, raw); err != nil {
			return "", fmt.Errorf("no cryptographic randomness for the payload delimiter: %w", err)
		}
		delim := payloadDelimiterPrefix + hex.EncodeToString(raw)
		if !bytes.Contains(payload, []byte(delim)) {
			return delim, nil
		}
	}
	return "", errors.New("four drawn payload delimiters all occur in the command, which random draws make implausible")
}
