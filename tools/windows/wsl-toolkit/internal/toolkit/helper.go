package toolkit

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The local helper is for a process that is refused when it calls wsl.exe. It is
// started ONCE through the approval path a session offers, holds the access, and
// takes job data from later commands.
//
// ⛔ A LIFETIME BOUNDARY, NOT A PRIVILEGE BOUNDARY. It listens on loopback, runs
// as whoever started it, and its token sits in that user's own state directory.
// It buys one approval instead of one per command and adds no permission.
//
// ⛔ What the protocol will not carry, each a refusal rather than an omission:
//
//	a host path          a workspace is UPLOADED and artifacts DOWNLOADED, so
//	                     the helper never opens a file the client named
//	a host command       no endpoint runs a program on this machine
//	a raw engine option  podman's flags are unreachable, so a mount cannot be
//	                     asked for
//	a WSL lifecycle call it builds and repairs the base and unregisters nothing

// HelperSchema versions the endpoint file and every response.
// ⚠ VERSION 2 CHANGED THE SHAPE OF TWO ROUTES. /v1/run and /v1/matrix used to
// answer with one JSON object once the work was over and now answer with a
// stream of newline-framed events. A client of version 1 reading a version 2
// answer sees the first event and no result, which is why the version is checked
// before a job is sent rather than after one comes back.
const HelperSchema = "wsl-toolkit-helper/2"

// HelperEndpoint is what a client reads to find a listening helper.
type HelperEndpoint struct {
	Schema  string    `json:"schema"`
	Address string    `json:"address"`
	Token   string    `json:"token"`
	PID     int       `json:"pid"`
	Version string    `json:"version"`
	Started time.Time `json:"started"`
}

// HelperEndpointPath is where the endpoint file lives.
func HelperEndpointPath() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "helper.json"), nil
}

// ReadHelperEndpoint returns the endpoint of a helper that is actually
// answering, or an error saying which of the three states this machine is in.
func ReadHelperEndpoint() (HelperEndpoint, error) {
	var ep HelperEndpoint
	path, err := HelperEndpointPath()
	if err != nil {
		return ep, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ep, fmt.Errorf("no helper has been started on this machine")
	}
	if err != nil {
		return ep, err
	}
	if err := json.Unmarshal(data, &ep); err != nil {
		return ep, fmt.Errorf("%s does not parse: %w", path, err)
	}
	if ep.Schema != HelperSchema {
		return ep, fmt.Errorf("%s declares schema %q and this build speaks %q", path, ep.Schema, HelperSchema)
	}
	return ep, nil
}

// HelperServer is the listening half.
type HelperServer struct {
	cfg    Config
	runner *Runner
	log    func(string)
	token  string
	stage  map[string]string
	mu     sync.Mutex
	server *http.Server
	stop   chan struct{}
	once   sync.Once
}

// maxRequestBytes bounds a JSON body. ⛔ A body with no ceiling is a memory
// ceiling reached in production. The archive endpoint streams and is bounded
// separately.
const maxRequestBytes = 4 << 20

// NewHelperServer prepares the listening half.
func NewHelperServer(cfg Config, log func(string)) (*HelperServer, error) {
	runner, err := NewRunner(cfg, log)
	if err != nil {
		return nil, err
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// ⛔ No fallback to a timestamp or a counter: a predictable token is
		// not a token.
		return nil, fmt.Errorf("no cryptographic randomness for a helper token: %w", err)
	}
	return &HelperServer{
		cfg: cfg, runner: runner, log: log,
		token: hex.EncodeToString(raw[:]),
		stage: map[string]string{},
		stop:  make(chan struct{}),
	}, nil
}

// Serve listens on the loopback interface and blocks until stopped.
func (h *HelperServer) Serve(ctx context.Context) error {
	// ⛔ 127.0.0.1 explicitly, never a wildcard. On 0.0.0.0 the token would be
	// the only thing between another machine and this one's WSL.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", h.guard(h.handleStatus))
	mux.HandleFunc("/v1/workspace", h.guard(h.handleWorkspace))
	mux.HandleFunc("/v1/base/status", h.guard(h.handleBaseStatus))
	mux.HandleFunc("/v1/base/ensure", h.guard(h.handleBaseEnsure))
	mux.HandleFunc("/v1/run", h.guard(h.handleRun))
	mux.HandleFunc("/v1/matrix", h.guard(h.handleMatrix))
	mux.HandleFunc("/v1/artifacts", h.guard(h.handleArtifacts))
	mux.HandleFunc("/v1/resources", h.guard(h.handleResources))
	mux.HandleFunc("/v1/gc", h.guard(h.handleGC))
	mux.HandleFunc("/v1/stop", h.guard(h.handleStop))

	h.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		// ⚠ No write timeout. A fleet legitimately runs for an hour, and a
		// deadline here would report a network failure over a working job. The
		// per-job deadline is where a runaway is bounded.
	}
	version, err := ScriptVersion()
	if err != nil {
		return err
	}
	ep := HelperEndpoint{
		Schema: HelperSchema, Address: ln.Addr().String(), Token: h.token,
		PID: os.Getpid(), Version: version, Started: time.Now().UTC(),
	}
	path, err := HelperEndpointPath()
	if err != nil {
		return err
	}
	data, err := jsonMarshalIndent(ep)
	if err != nil {
		return err
	}
	// ⚠ On Windows 0600 is not an access control list. What protects this file
	// is that %LOCALAPPDATA% is the user's own directory.
	if err := writeFileAtomic(path, data, 0o600); err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			h.log("the endpoint file is still on disk: " + path)
		}
	}()

	go func() {
		select {
		case <-ctx.Done():
		case <-h.stop:
		}
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.server.Shutdown(shutdown); err != nil {
			h.log("shutdown: " + err.Error())
		}
	}()

	h.log("listening on " + ep.Address + " as pid " + strconv.Itoa(ep.PID))
	h.log("endpoint file: " + path)
	if err := h.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	h.cleanupStaging()
	return nil
}

func (h *HelperServer) cleanupStaging() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, dir := range h.stage {
		if err := RemoveInside(h.runner.home, dir); err != nil {
			h.log("could not remove staged upload " + id + ": " + err.Error())
		}
		delete(h.stage, id)
	}
}

// guard is the one place a request is authenticated, so no endpoint can be
// added without it.
func (h *HelperServer) guard(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		// ⛔ Constant time. Reachable only from loopback does not make an
		// equality comparison on a token right.
		if subtle.ConstantTimeCompare([]byte(token), []byte(h.token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func writeHelperJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		// The response is already committed, so this can only be recorded.
		_ = err
	}
}

func decodeHelperBody(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxRequestBytes))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (h *HelperServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	version, _ := ScriptVersion()
	writeHelperJSON(w, http.StatusOK, map[string]any{
		"schema": HelperSchema, "version": version, "pid": os.Getpid(),
		"base": h.cfg.Base.Name, "user": h.cfg.Base.User,
	})
}

func (h *HelperServer) handleStop(w http.ResponseWriter, r *http.Request) {
	writeHelperJSON(w, http.StatusOK, map[string]any{"schema": HelperSchema, "stopping": true})
	h.once.Do(func() { close(h.stop) })
}

// handleWorkspace takes an uploaded archive and unpacks it into the guest.
//
// ⛔ The client sends BYTES, not a path. A path here would let a sandboxed
// caller read anything the helper can.
func (h *HelperServer) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	id, err := newJobID()
	if err != nil {
		writeHelperJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	dir := filepath.Join(h.runner.home, "uploads", id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		writeHelperJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	limits := DefaultWorkspaceLimits()
	if v := r.Header.Get("X-Max-Bytes"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			limits.MaxBytes = n
		}
	}
	// ⚠ Both ceilings, or the pair disagrees. Sending one and not the other
	// refuses an upload the direct path accepts, and the message names the
	// entry count the caller did not set.
	if v := r.Header.Get("X-Max-Entries"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limits.MaxEntries = n
		}
	}
	entries, total, err := extractInto(io.LimitReader(r.Body, limits.MaxBytes+1<<20), dir, limits)
	if err != nil {
		if rmErr := RemoveInside(h.runner.home, dir); rmErr != nil {
			h.log("could not remove a refused upload: " + rmErr.Error())
		}
		writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.mu.Lock()
	h.stage[id] = dir
	h.mu.Unlock()
	// ⛔ RECORDED, so cleanup can see it without this process. The map is a
	// convenience for the running helper; the ledger is what survives it being
	// killed, and what stops a concurrent `gc --apply` removing an upload that a
	// job is about to use.
	if err := h.runner.ledger.Append(LedgerEntry{Event: "open", Kind: "upload", ID: id, HostDir: dir}); err != nil {
		h.log("could not record the uploaded workspace " + id + ": " + err.Error())
	}
	writeHelperJSON(w, http.StatusOK, map[string]any{
		"schema": HelperSchema, "id": id, "entries": entries, "bytes": total,
	})
}

func (h *HelperServer) stagedDir(id string) (string, error) {
	if id == "" {
		return "", nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	dir, ok := h.stage[id]
	if !ok {
		return "", fmt.Errorf("no uploaded workspace with id %q", id)
	}
	return dir, nil
}

// HelperRunRequest is the wire shape of one job. ⛔ There is no field for a host
// path: a workspace arrives by upload and artifacts leave by download.
type HelperRunRequest struct {
	Image     string            `json:"image"`
	ScriptB64 string            `json:"script_b64"`
	Env       map[string]string `json:"env,omitempty"`
	// ⛔ User IS ON THE WIRE because the direct path has it. A field the
	// client can set and this side ignores is a job that runs as root after a
	// caller asked for another account, with nothing said either way.
	User       string `json:"user,omitempty"`
	TimeoutMS  int64  `json:"timeout_ms,omitempty"`
	Network    bool   `json:"network"`
	StagingID  string `json:"staging_id,omitempty"`
	Artifacts  bool   `json:"artifacts,omitempty"`
	MaxBytes   int64  `json:"max_bytes,omitempty"`
	MaxEntries int    `json:"max_entries,omitempty"`
	// MaxOutput is here for the same reason User is: the direct path has it. A
	// door sweep found it missing before this shipped, which is the SECOND time
	// a job flag has been dropped between the two routes.
	MaxOutput int64 `json:"max_output,omitempty"`
}

// HelperMatrixRequest is the wire shape of a fleet run.
type HelperMatrixRequest struct {
	HelperRunRequest
	Images   []string `json:"images,omitempty"`
	Parallel int      `json:"parallel,omitempty"`
}

func (req HelperRunRequest) limits() WorkspaceLimits {
	l := DefaultWorkspaceLimits()
	if req.MaxBytes > 0 {
		l.MaxBytes = req.MaxBytes
	}
	if req.MaxEntries > 0 {
		l.MaxEntries = req.MaxEntries
	}
	return l
}

func (h *HelperServer) artifactDir(id string, want bool) string {
	if !want {
		return ""
	}
	dir := filepath.Join(h.runner.home, "artifacts", id)
	// Recorded BEFORE the job fills it, for the reason every other record here
	// is: a set written by a run that was killed is only findable if the record
	// went in first.
	if err := h.runner.ledger.Append(LedgerEntry{Event: "open", Kind: "artifacts", ID: id, HostDir: dir}); err != nil {
		h.log("could not record the artifact set " + id + ": " + err.Error())
	}
	return dir
}

func (h *HelperServer) handleRun(w http.ResponseWriter, r *http.Request) {
	var req HelperRunRequest
	if err := decodeHelperBody(r, &req); err != nil {
		writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	payload, err := base64.StdEncoding.DecodeString(req.ScriptB64)
	if err != nil {
		writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": "script_b64 is not base64: " + err.Error()})
		return
	}
	staged, err := h.stagedDir(req.StagingID)
	if err != nil {
		writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	artifactID, err := newJobID()
	if err != nil {
		writeHelperJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// ⛔ THE UPLOAD IS CONSUMED BY THE JOB THAT NAMED IT. One upload, one
	// job: nothing in the protocol reuses a staging id, and a helper that kept
	// them accumulated every workspace any client had ever sent, none of which
	// cleanup could see.
	defer h.releaseStaging(req.StagingID)

	// ⭐ THE HEADER GOES OUT BEFORE THE JOB STARTS. Once it has, the client
	// is reading, and every byte the container writes reaches it as it is
	// written rather than when the job is over.
	events := newEventWriter(w)
	res := h.runner.Run(r.Context(), JobSpec{
		Image: req.Image, Script: payload, Workspace: staged, Env: req.Env,
		Timeout: time.Duration(req.TimeoutMS) * time.Millisecond, Network: req.Network,
		Limits: req.limits(), ArtifactDir: h.artifactDir(artifactID, req.Artifacts),
		User: req.User, MaxOutput: req.MaxOutput,
		Stdout: &chunkWriter{out: events, kind: "stdout"},
		Stderr: &chunkWriter{out: events, kind: "stderr"},
	})
	events.send(HelperEvent{Kind: "result", Result: &res, ArtifactsID: artifactID})
}

func (h *HelperServer) handleMatrix(w http.ResponseWriter, r *http.Request) {
	var req HelperMatrixRequest
	if err := decodeHelperBody(r, &req); err != nil {
		writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	payload, err := base64.StdEncoding.DecodeString(req.ScriptB64)
	if err != nil {
		writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": "script_b64 is not base64: " + err.Error()})
		return
	}
	selected, err := h.cfg.SelectImages(req.Images)
	if err != nil {
		writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	staged, err := h.stagedDir(req.StagingID)
	if err != nil {
		writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	artifactID, err := newJobID()
	if err != nil {
		writeHelperJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer h.releaseStaging(req.StagingID)

	events := newEventWriter(w)
	report, err := h.runner.RunMatrix(r.Context(), MatrixSpec{
		Images: selected, Script: payload, Workspace: staged, Env: req.Env,
		Timeout: time.Duration(req.TimeoutMS) * time.Millisecond, Network: req.Network,
		Parallel: req.Parallel, Limits: req.limits(),
		ArtifactDir: h.artifactDir(artifactID, req.Artifacts),
		User:        req.User,
		MaxOutput:   req.MaxOutput,
		// ⛔ A ROW IS SENT WHEN IT FINISHES, not when the fleet does. A
		// caller watching twelve images can see eleven succeed while the
		// twelfth is still pulling, which is the difference between a fleet
		// that is working and one that is stuck.
		OnRow: func(row JobResult) {
			// The row's own streams are not forwarded: twelve containers
			// interleaved on one stdout is unreadable, and the transcripts hold
			// the complete text of each.
			row.Stdout, row.Stderr = "", ""
			events.send(HelperEvent{Kind: "row", Row: &row})
		},
	})
	if err != nil {
		events.send(HelperEvent{Kind: "error", Text: err.Error()})
		return
	}
	events.send(HelperEvent{Kind: "report", Report: &report, ArtifactsID: artifactID})
}

// handleArtifacts streams one job's artifact directory back as an archive, and
// releases it when the client says it arrived.
//
// ⛔ THE HELPER DOES NOT DELETE ON A SUCCESSFUL WRITE TO THE SOCKET. Bytes
// leaving here is not the same fact as bytes landing on the client's disk, and
// deleting on the first is the same shape as the defect where a job's guest
// output was torn down after a transfer that had failed. The client sends DELETE
// once it has extracted; anything nobody acknowledges is collected by age.
func (h *HelperServer) handleArtifacts(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if _, err := hex.DecodeString(id); err != nil || len(id) != 16 {
		// ⛔ Validated by SHAPE before it is joined to a path. A caller-supplied
		// path component is how a download endpoint becomes a file reader.
		http.Error(w, "an artifacts id is 16 hex characters", http.StatusBadRequest)
		return
	}
	dir := filepath.Join(h.runner.home, "artifacts", id)
	if _, err := ResolveInside(h.runner.home, dir); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(dir); err != nil {
		if r.Method == http.MethodDelete {
			// ⚠ RELEASING SOMETHING ALREADY GONE IS A SUCCESS. A client that
			// retried, or one whose set cleanup collected first, would otherwise
			// get an error for a state it was asking for.
			writeHelperJSON(w, http.StatusOK, map[string]any{"schema": HelperSchema, "released": id})
			return
		}
		http.Error(w, "no artifacts under that id", http.StatusNotFound)
		return
	}
	if r.Method == http.MethodDelete {
		if err := RemoveInside(h.runner.home, dir); err != nil {
			writeHelperJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if err := h.runner.ledger.Append(LedgerEntry{Event: "close", Kind: "artifacts", ID: id}); err != nil {
			h.log("could not close the record for the artifact set " + id + ": " + err.Error())
		}
		writeHelperJSON(w, http.StatusOK, map[string]any{"schema": HelperSchema, "released": id})
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	if _, _, err := writeWorkspaceTar(w, dir, DefaultWorkspaceLimits(), nil); err != nil {
		h.log("streaming artifacts " + id + ": " + err.Error())
	}
}

// releaseStaging removes one uploaded workspace and forgets it.
//
// ⚠ THE MAP IS NOT THE RECORD. It is removed from both, and the directory
// is what cleanup reads, so a helper that was killed between the upload and the
// job still leaves something collectable rather than something only a lost map
// could have named.
func (h *HelperServer) releaseStaging(id string) {
	if id == "" {
		return
	}
	h.mu.Lock()
	dir, ok := h.stage[id]
	delete(h.stage, id)
	h.mu.Unlock()
	if !ok {
		return
	}
	if err := RemoveInside(h.runner.home, dir); err != nil {
		h.log("could not release the uploaded workspace " + id + ": " + err.Error())
		return
	}
	if err := h.runner.ledger.Append(LedgerEntry{Event: "close", Kind: "upload", ID: id}); err != nil {
		h.log("could not close the record for the uploaded workspace " + id + ": " + err.Error())
	}
}

func (h *HelperServer) handleBaseStatus(w http.ResponseWriter, r *http.Request) {
	probe := r.URL.Query().Get("probe") == "1"
	st, err := h.runner.Base().Status(r.Context(), probe)
	if err != nil {
		writeHelperJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeHelperJSON(w, http.StatusOK, map[string]any{"schema": HelperSchema, "state": st})
}

func (h *HelperServer) handleBaseEnsure(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Force bool `json:"force"`
	}
	if r.ContentLength > 0 {
		if err := decodeHelperBody(r, &req); err != nil {
			writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	st, err := h.runner.Base().Ensure(r.Context(), req.Force)
	if err != nil {
		writeHelperJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error(), "state": ""})
		return
	}
	writeHelperJSON(w, http.StatusOK, map[string]any{"schema": HelperSchema, "state": st})
}

func (h *HelperServer) handleResources(w http.ResponseWriter, r *http.Request) {
	writeHelperJSON(w, http.StatusOK, map[string]any{
		"schema": HelperSchema, "report": h.runner.Resources(r.Context()),
	})
}

func (h *HelperServer) handleGC(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Apply       bool  `json:"apply"`
		OlderThanMS int64 `json:"older_than_ms"`
		Images      bool  `json:"images"`
		// IncludeLive is on the wire because the direct path has it. ⛔ A
		// flag one route honours and the other drops is the defect this
		// protocol has already had once.
		IncludeLive bool `json:"include_live,omitempty"`
	}
	if r.ContentLength > 0 {
		if err := decodeHelperBody(r, &req); err != nil {
			writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	policy := CleanupPolicy{
		OlderThan:   time.Duration(req.OlderThanMS) * time.Millisecond,
		IncludeLive: req.IncludeLive,
	}
	plan, err := h.runner.Cleanup(r.Context(), req.Apply, policy, req.Images)
	payload := map[string]any{"schema": HelperSchema, "plan": plan}
	if err != nil {
		payload["error"] = err.Error()
	}
	writeHelperJSON(w, http.StatusOK, payload)
}

// ScriptVersion is the product version, read from the embedded script through
// the one function that owns it. It is a hook so this package does not import
// the script package directly and create a cycle with the command layer.
var ScriptVersion = func() (string, error) { return "", nil }
