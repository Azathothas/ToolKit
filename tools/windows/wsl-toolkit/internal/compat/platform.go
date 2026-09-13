// SPDX-License-Identifier: 0BSD

package compat

import (
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// dur is the clock type the stream log works in.
type dur = time.Duration

// now is the wall clock, indirect so a test can hold it still.
var now = func() time.Time { return time.Now() }

// runtimeOS names the host family the way the doctor row reports it.
func runtimeOS() string { return runtime.GOOS + "/" + runtime.GOARCH }

// hostNow returns the wall clock as a time the stamp renderer formats.
func hostNow() time.Time { return now() }

// envValue reads the process environment.
func envValue(name string) string { return os.Getenv(name) }

// isInteractive reports whether a confirmation question can be asked at all.
//
// ⛔ Read-Host BLOCKS OR THROWS when stdin is not a console, and a tool that
// hangs on its own question is worse than one that refuses, so this is tested
// up front: a non-interactive session is told to pass -Force, in words, and
// nothing destructive happens.
func (s *session) interactive() bool {
	if s.in == nil {
		return false
	}
	if f, ok := s.in.(*os.File); ok {
		fi, err := f.Stat()
		if err != nil {
			return false
		}
		if fi.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return runtime.GOOS != "js"
}

// writerIsTerminal reports whether w is the process's own console, which is
// what the colour decision needs. ⛔ 'auto' IS DECIDED ONCE, at settings
// resolution, and not per line.
func writerIsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// volumeFreeBytes reports the bytes available to this caller on the volume a
// path lives on. The boolean is a THIRD answer: "could not measure" is not
// "there is room", and it is not a reason to refuse either.
func volumeFreeBytes(path string) (int64, bool) {
	return freeBytes(path)
}

// randomSuffix draws n characters from the script's 36-symbol alphabet. It is
// the only source of randomness the naming and transport paths use.
func randomSuffix(n int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(b)
}

// fullPathNormalise is the port of [IO.Path]::GetFullPath for the paths this
// interface builds: absolute, cleaned, with the host's separators.
func fullPathNormalise(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", errors.New("empty path")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// trimSeparators removes trailing path separators, both spellings, because a
// Windows path can carry either and the containment guard compares prefixes.
func trimSeparators(p string) string {
	return strings.TrimRight(p, `\/`)
}

// leafOfPath splits off the final component on BOTH separators, never on the
// host's alone: the rule that uses it is about Windows semantics whatever host
// is asking.
func leafOfPath(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[i+1:]
	}
	return p
}
