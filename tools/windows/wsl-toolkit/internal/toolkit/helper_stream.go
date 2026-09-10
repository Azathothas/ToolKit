// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// ⛔ WHY THE HELPER STREAMS. A restricted caller has no alternative to this
// route, and it answered with one JSON object once the work was over. A fleet of
// twelve images was seventy seconds in which the client received nothing at all:
// not a row label, not a pull, not a byte of the command's output. A caller
// cannot tell a compiler that is working from one that has hung, and the only
// remedy available to them was to wait and hope.
//
// ⭐ THE FRAMING IS ONE JSON OBJECT PER LINE. It needs no length prefix, no
// multipart boundary and no second parser: a client reads lines and decodes each
// one. A truncated stream ends without a `result` event, which is exactly how a
// client tells "the helper died" from "the job failed".

// HelperEventSchema versions the event stream, because its reader is a program.
const HelperEventSchema = "wsl-toolkit-helper-event/1"

// HelperEvent is one line of a streamed response.
type HelperEvent struct {
	Schema string `json:"schema"`
	// Kind is one of: log, stdout, stderr, row, result, error.
	Kind string `json:"kind"`
	// Text carries a log line or an error message.
	Text string `json:"text,omitempty"`
	// B64 carries a chunk of the command's own bytes. ⛔ Base64 because the
	// bytes are the container's and JSON cannot hold arbitrary ones: a chunk
	// split mid-rune would otherwise be re-encoded into something the container
	// did not write.
	B64 string `json:"b64,omitempty"`
	// Row is one finished fleet row, sent as it finishes.
	Row *JobResult `json:"row,omitempty"`
	// Result and Report are the last event, and only one of them appears.
	Result      *JobResult    `json:"result,omitempty"`
	Report      *MatrixReport `json:"report,omitempty"`
	ArtifactsID string        `json:"artifacts_id,omitempty"`
}

// eventWriter serialises events onto one response and flushes each.
//
// ⚠ THE FLUSH IS THE WHOLE POINT. Without it Go buffers the response and the
// client receives everything at the end, which is the behaviour being fixed. A
// ResponseWriter that cannot flush is reported once rather than silently
// batching.
type eventWriter struct {
	mu      sync.Mutex
	enc     *json.Encoder
	flusher http.Flusher
	err     error
}

func newEventWriter(w http.ResponseWriter) *eventWriter {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	e := &eventWriter{enc: json.NewEncoder(w)}
	if f, ok := w.(http.Flusher); ok {
		e.flusher = f
		f.Flush()
	}
	return e
}

func (e *eventWriter) send(ev HelperEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil {
		return
	}
	ev.Schema = HelperEventSchema
	if err := e.enc.Encode(ev); err != nil {
		e.err = err
		return
	}
	if e.flusher != nil {
		e.flusher.Flush()
	}
}

// chunkWriter turns writes into stdout or stderr events.
//
// ⚠ IT DOES NOT WAIT FOR A NEWLINE. A build that prints a progress bar with
// carriage returns and no newline would otherwise arrive in one piece at the
// end, which is the case a caller most wants to watch.
type chunkWriter struct {
	out  *eventWriter
	kind string
}

func (c *chunkWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	c.out.send(HelperEvent{Kind: c.kind, B64: base64.StdEncoding.EncodeToString(p)})
	return len(p), nil
}

// ReadHelperEvents decodes a streamed response and hands each event to fn.
//
// ⛔ THE STREAM MUST END WITH A result OR report EVENT. A stream that stops
// before one is a helper that died mid-job, and returning what arrived so far as
// though it were an answer is how a killed run reads as a finished one.
func ReadHelperEvents(body io.Reader, fn func(HelperEvent) error) error {
	sc := bufio.NewScanner(body)
	// A single event carries at most one write's worth of output plus base64
	// overhead. The cap is generous and bounded rather than absent: an
	// unbounded reader here is a client a helper can exhaust.
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	seenFinal := false
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev HelperEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			return fmt.Errorf("the helper sent something that is not an event: %w", err)
		}
		if ev.Kind == "error" {
			return errors.New(ev.Text)
		}
		if ev.Kind == "result" || ev.Kind == "report" {
			seenFinal = true
		}
		if err := fn(ev); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("the helper's answer stopped early: %w", err)
	}
	if !seenFinal {
		return errors.New("the helper's answer ended without a result, so the job's outcome is unknown. Check that the helper is still running with: wsl-toolkit helper status")
	}
	return nil
}

// stream opens a streaming request and reads its events.
func (c *HelperClient) stream(ctx context.Context, path string, body any, fn func(HelperEvent) error) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+c.endpoint.Address+path, bytesReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.endpoint.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var problem struct {
			Error string `json:"error"`
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if json.Unmarshal(raw, &problem) == nil && problem.Error != "" {
			return fmt.Errorf("the helper refused: %s", problem.Error)
		}
		return fmt.Errorf("the helper answered %s", resp.Status)
	}
	return ReadHelperEvents(resp.Body, fn)
}

// HelperSinks are where a streaming client puts what arrives.
//
// ⚠ A NIL WRITER DISCARDS THAT STREAM RATHER THAN FAILING. A caller whose own
// stdout carries a structured answer passes nil for Stdout on purpose, and the
// complete text is still on the result and in the helper's transcript.
type HelperSinks struct {
	Stdout io.Writer
	Stderr io.Writer
	Log    func(string)
	Row    func(JobResult)
}

func (s HelperSinks) apply(ev HelperEvent) error {
	switch ev.Kind {
	case "stdout", "stderr":
		raw, err := base64.StdEncoding.DecodeString(ev.B64)
		if err != nil {
			return fmt.Errorf("the helper sent output this build cannot decode: %w", err)
		}
		w := s.Stdout
		if ev.Kind == "stderr" {
			w = s.Stderr
		}
		if w == nil {
			return nil
		}
		_, err = w.Write(raw)
		return err
	case "log":
		if s.Log != nil {
			s.Log(ev.Text)
		}
	case "row":
		if s.Row != nil && ev.Row != nil {
			s.Row(*ev.Row)
		}
	}
	return nil
}

// RunStream asks the helper to execute one job and receives it as it happens.
func (c *HelperClient) RunStream(ctx context.Context, req HelperRunRequest, sinks HelperSinks) (JobResult, string, error) {
	var res JobResult
	var artifacts string
	// ⛔ ATTACHED HERE, not by the caller. Every job that reaches a helper goes
	// through this function or MatrixStream, so this is the one door where the
	// effective configuration joins the request. WSL-44.
	req.Config = &c.cfg
	err := c.stream(ctx, "/v1/run", req, func(ev HelperEvent) error {
		if ev.Kind == "result" && ev.Result != nil {
			res, artifacts = *ev.Result, ev.ArtifactsID
			return nil
		}
		return sinks.apply(ev)
	})
	return res, artifacts, err
}

// MatrixStream asks the helper to execute a fleet and receives each row as it
// finishes.
func (c *HelperClient) MatrixStream(ctx context.Context, req HelperMatrixRequest, sinks HelperSinks) (MatrixReport, string, error) {
	var report MatrixReport
	var artifacts string
	req.Config = &c.cfg
	err := c.stream(ctx, "/v1/matrix", req, func(ev HelperEvent) error {
		if ev.Kind == "report" && ev.Report != nil {
			report, artifacts = *ev.Report, ev.ArtifactsID
			return nil
		}
		return sinks.apply(ev)
	})
	return report, artifacts, err
}
