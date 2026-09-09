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
const HelperSchema = "wsl-toolkit-helper/1"

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
	return filepath.Join(h.runner.home, "artifacts", id)
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
	res := h.runner.Run(r.Context(), JobSpec{
		Image: req.Image, Script: payload, Workspace: staged, Env: req.Env,
		Timeout: time.Duration(req.TimeoutMS) * time.Millisecond, Network: req.Network,
		Limits: req.limits(), ArtifactDir: h.artifactDir(artifactID, req.Artifacts),
		User: req.User,
	})
	writeHelperJSON(w, http.StatusOK, map[string]any{
		"schema": HelperSchema, "result": res, "artifacts_id": artifactID,
	})
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
	report, err := h.runner.RunMatrix(r.Context(), MatrixSpec{
		Images: selected, Script: payload, Workspace: staged, Env: req.Env,
		Timeout: time.Duration(req.TimeoutMS) * time.Millisecond, Network: req.Network,
		Parallel: req.Parallel, Limits: req.limits(),
		ArtifactDir: h.artifactDir(artifactID, req.Artifacts),
		User:        req.User,
	})
	if err != nil {
		writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeHelperJSON(w, http.StatusOK, map[string]any{
		"schema": HelperSchema, "report": report, "artifacts_id": artifactID,
	})
}

// handleArtifacts streams one job's artifact directory back as an archive.
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
		http.Error(w, "no artifacts under that id", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/x-tar")
	if _, _, err := writeWorkspaceTar(w, dir, DefaultWorkspaceLimits(), nil); err != nil {
		h.log("streaming artifacts " + id + ": " + err.Error())
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
	}
	if r.ContentLength > 0 {
		if err := decodeHelperBody(r, &req); err != nil {
			writeHelperJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	plan, err := h.runner.Cleanup(r.Context(), req.Apply, time.Duration(req.OlderThanMS)*time.Millisecond, req.Images)
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
