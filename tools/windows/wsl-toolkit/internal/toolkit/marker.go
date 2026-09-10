// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
)

// ⛔ WHY A MARKER EXISTS AT ALL. podman reports one status for pulling an image,
// creating a container and running its payload, so a tag the registry does not
// have and a payload that exits 125 are the same number. The tool reported both
// as a container that ran and failed, and `matrix` counted both under `ran`,
// which is the exact distinction its three counts exist to make.
//
// ⛔ NOT A TEST ON 125, 126 OR 127. Those are podman's conventions AND legal
// values for a real payload, so a classifier keyed to them calls a working job
// unreached. The marker is written by the payload's own wrapper INSIDE the
// container, so seeing it is proof the container started and the wrapper ran.

// newMarkerToken is the per-job value the wrapper echoes.
//
// ⚠ It is random per job rather than a constant, so a payload printing the
// tool's own marker cannot make an unreached job look like a job that ran. A
// payload can read it out of its own command line, which is not a boundary this
// claims to hold: a payload that ran is a payload that ran.
func newMarkerToken() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("no cryptographic randomness for a job marker: %w", err)
	}
	return "wtk-started-" + hex.EncodeToString(raw[:]), nil
}

// markerStripper removes the marker line from a stream and remembers that it
// was there.
//
// ⚠ IT HOLDS BACK A PARTIAL TAIL. The marker can arrive split across two writes,
// and a filter that only looked at each write on its own would pass half of it
// through and never match. The held-back tail is at most the marker's own
// length, so a stream that never carries one is delayed by that many bytes and
// no more. Flush releases it when the stream ends.
//
// ⛔ FRAMING DOES NOT ADD A BYTE TO THE PAYLOAD. The token is written with a
// leading newline so it cannot cut into an unterminated line the engine wrote,
// and this filter used to put that newline back unconditionally. The token is
// written BEFORE the payload runs, so on almost every job there was nothing in
// front of it to terminate: a payload writing no stderr at all came back as
// `"stderr":"\n"` with `stderr_bytes: 1`, and `matrix` reported one byte of
// error output for twelve rows that wrote none. WSL-46, issue 23. The newline
// is now put back only where there is an unterminated tail for it to terminate.
type markerStripper struct {
	mu   sync.Mutex
	dst  io.Writer
	tok  []byte
	buf  []byte
	seen bool
	// last is the last byte handed to dst, and any says whether there has been
	// one. Together they answer "was the stream mid-line when the token
	// arrived", which the buffer alone cannot: everything before the token may
	// already have been flushed.
	last byte
	any  bool
}

func newMarkerStripper(dst io.Writer, token string) *markerStripper {
	// The newlines are part of the token: a marker on its own line cannot cut
	// into a line the payload wrote.
	return &markerStripper{dst: dst, tok: []byte("\n" + token + "\n")}
}

func (m *markerStripper) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(p)
	m.buf = append(m.buf, p...)
	for {
		i := bytes.Index(m.buf, m.tok)
		if i < 0 {
			break
		}
		m.seen = true
		// Whatever sits immediately in front of the token in the WHOLE stream,
		// which is the pending buffer where there is one and the last byte
		// already written where there is not.
		keepNewline := false
		switch {
		case i > 0:
			keepNewline = m.buf[i-1] != '\n'
		case m.any:
			keepNewline = m.last != '\n'
		}
		cut := i
		if keepNewline {
			cut = i + 1
		}
		m.buf = append(m.buf[:cut], m.buf[i+len(m.tok):]...)
	}
	keep := partialSuffix(m.buf, m.tok)
	flush := m.buf[:len(m.buf)-keep]
	var err error
	if len(flush) > 0 {
		m.last, m.any = flush[len(flush)-1], true
		_, err = m.dst.Write(flush)
	}
	m.buf = append(m.buf[:0], m.buf[len(m.buf)-keep:]...)
	return n, err
}

// Flush writes whatever was held back. ⛔ Called once the stream is over: the
// tail is only ambiguous while more bytes could still arrive.
func (m *markerStripper) Flush() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.buf) == 0 {
		return nil
	}
	m.last, m.any = m.buf[len(m.buf)-1], true
	_, err := m.dst.Write(m.buf)
	m.buf = m.buf[:0]
	return err
}

func (m *markerStripper) Seen() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.seen
}

// partialSuffix is the length of the longest suffix of b that is a prefix of
// tok. That is exactly what must be held back.
func partialSuffix(b, tok []byte) int {
	max := len(tok) - 1
	if len(b) < max {
		max = len(b)
	}
	for n := max; n > 0; n-- {
		if bytes.Equal(b[len(b)-n:], tok[:n]) {
			return n
		}
	}
	return 0
}
