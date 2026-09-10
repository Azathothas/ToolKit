package toolkit

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// HelperClient is the calling half.
//
// ⚠ IT IS THE SAME BINARY ON BOTH ENDS. The client marshals an operation this
// executable already defines rather than a command, which is what keeps the
// protocol from becoming a way to run anything.
type HelperClient struct {
	endpoint HelperEndpoint
	http     *http.Client
	// cfg is the EFFECTIVE configuration this client read, and it travels with
	// every request that depends on one.
	//
	// ⛔ IT IS LOADED HERE RATHER THAN PASSED IN BY EACH CALLER. A helper that
	// acts on its own startup config is WSL-44, and "every call site remembers
	// to attach the config" is the shape of guard that will one day be applied
	// at three of four call sites. One door.
	cfg Config
}

// DialHelper connects to a helper that is actually answering.
//
// ⛔ IT ASKS RATHER THAN TRUSTING THE FILE. An endpoint file left behind by a
// process that died names a port nothing is listening on, and a client that
// trusted it would report a connection error where the honest answer is that no
// helper is running.
func DialHelper(ctx context.Context) (*HelperClient, error) {
	ep, err := ReadHelperEndpoint()
	if err != nil {
		return nil, err
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	c := &HelperClient{
		endpoint: ep,
		cfg:      cfg,
		// No overall timeout: a matrix legitimately runs for an hour. The
		// per-job deadline is where a runaway is bounded, and a client timeout
		// here would report a network failure over a job that was working.
		http: &http.Client{},
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var status struct {
		Schema  string `json:"schema"`
		Version string `json:"version"`
		PID     int    `json:"pid"`
	}
	if err := c.call(bounded, http.MethodGet, "/v1/status", nil, &status); err != nil {
		return nil, fmt.Errorf("a helper endpoint file exists and nothing is answering on %s: %w", ep.Address, err)
	}
	if status.PID != ep.PID {
		return nil, fmt.Errorf("something else is listening on %s: it reports pid %d and the endpoint file says %d", ep.Address, status.PID, ep.PID)
	}
	return c, nil
}

// Endpoint is what this client is talking to, for a report that names it.
func (c *HelperClient) Endpoint() HelperEndpoint { return c.endpoint }

// Config is the effective configuration this client will send, so a caller can
// report which one a helper is being asked to act on.
func (c *HelperClient) Config() Config { return c.cfg }

func (c *HelperClient) call(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+c.endpoint.Address+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.endpoint.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		// ⛔ THE STRUCTURED ANSWER IS READ EVEN ON A REFUSAL. A failure is when
		// a caller most needs it: `base ensure` refusing a stale engine sends
		// back a state carrying the exact command to run next, and this used to
		// throw that away and hand the caller a string. The error is still what
		// is returned; `out` is filled in first so a caller that looks at it
		// finds what the server sent. WSL-61.
		if out != nil {
			_ = json.Unmarshal(data, out)
		}
		var problem struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &problem) == nil && problem.Error != "" {
			return fmt.Errorf("the helper refused: %s", problem.Error)
		}
		return fmt.Errorf("the helper answered %s", resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// UploadWorkspace sends a host directory to the helper as an archive and returns
// the id a run refers to it by.
func (c *HelperClient) UploadWorkspace(ctx context.Context, hostDir string, limits WorkspaceLimits, excludes []string) (string, error) {
	pr, pw := io.Pipe()
	go func() {
		_, err := writeWorkspaceTar(pw, hostDir, limits, excludes)
		_ = pw.CloseWithError(err)
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+c.endpoint.Address+"/v1/workspace", pr)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.endpoint.Token)
	req.Header.Set("Content-Type", "application/x-tar")
	req.Header.Set("X-Max-Bytes", strconv.FormatInt(limits.MaxBytes, 10))
	req.Header.Set("X-Max-Entries", strconv.Itoa(limits.MaxEntries))
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var out struct {
		ID      string `json:"id"`
		Entries int    `json:"entries"`
		Bytes   int64  `json:"bytes"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("the helper answered %s: %s", resp.Status, firstLine(string(data)))
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the helper refused the workspace: %s", out.Error)
	}
	return out.ID, nil
}

// DownloadArtifacts pulls one run's artifacts and extracts them, with the same
// per-entry validation the direct path uses.
func (c *HelperClient) DownloadArtifacts(ctx context.Context, id, hostDir string, limits WorkspaceLimits) (ArtifactTransfer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://"+c.endpoint.Address+"/v1/artifacts?id="+id, nil)
	if err != nil {
		return ArtifactTransfer{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.endpoint.Token)
	resp, err := c.http.Do(req)
	if err != nil {
		return ArtifactTransfer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ArtifactTransfer{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return ArtifactTransfer{}, fmt.Errorf("the helper answered %s for the artifacts", resp.Status)
	}
	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		return ArtifactTransfer{}, err
	}
	dest, err := resolveExisting(hostDir)
	if err != nil {
		return ArtifactTransfer{}, err
	}
	got, err := extractInto(resp.Body, dest, limits)
	if err != nil {
		// ⛔ NOT ACKNOWLEDGED. The helper keeps the set so a caller can go
		// back for it, which is the same rule the guest directory follows when
		// its transfer fails. What was missing is the caller being TOLD: the
		// set id travels back on the result now. WSL-46, issue 24.
		return got, err
	}
	if relErr := c.ReleaseArtifacts(ctx, id); relErr != nil {
		// A set that could not be released is a leak, not a failed job. Cleanup
		// collects it by age, so this is reported and not raised.
		return got, nil
	}
	return got, nil
}

// ReleaseArtifacts tells the helper the set arrived and may go.
func (c *HelperClient) ReleaseArtifacts(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		"http://"+c.endpoint.Address+"/v1/artifacts?id="+id, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.endpoint.Token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("the helper answered %s when releasing the artifacts", resp.Status)
	}
	return nil
}

// Status asks what the helper is.
func (c *HelperClient) Status(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	err := c.call(ctx, http.MethodGet, "/v1/status", nil, &out)
	return out, err
}

// Stop asks the helper to shut down.
func (c *HelperClient) Stop(ctx context.Context) error {
	return c.call(ctx, http.MethodPost, "/v1/stop", nil, nil)
}

// BaseStatus asks about the owned distribution.
func (c *HelperClient) BaseStatus(ctx context.Context, probe bool) (BaseState, error) {
	var out struct {
		State BaseState `json:"state"`
	}
	path := "/v1/base/status"
	sep := "?"
	if probe {
		path += "?probe=1"
		sep = "&"
	}
	// The config travels base64 in the query because this route is a GET, and a
	// GET carrying a body is something an intermediary may drop.
	if raw, err := json.Marshal(c.cfg); err == nil {
		path += sep + "config_b64=" + url.QueryEscape(base64.StdEncoding.EncodeToString(raw))
	}
	err := c.call(ctx, http.MethodGet, path, nil, &out)
	return out.State, err
}

// BaseEnsure asks the helper to build or repair the base.
//
// ⛔ THE CONFIG GOES WITH IT. This request used to carry {force} alone, so
// `base ensure --preset alpine` announced Alpine, wrote Alpine to the client's
// config, and had the helper rebuild from the image ITS startup config named.
// WSL-44, issue 17.
func (c *HelperClient) BaseEnsure(ctx context.Context, force bool) (BaseState, error) {
	return c.BaseEnsureWith(ctx, force, false)
}

// BaseEnsureWith carries the repair switch over the protocol.
//
// ⚠ THE FIELD IS ADDITIVE AND THAT IS SAFE HERE FOR ONE REASON ONLY: the client
// refuses an endpoint whose schema is not the one this build speaks, so a new client
// cannot reach a helper that would ignore the field. Without that refusal an
// omitted field would read as "do not repair" on one side and "was never asked"
// on the other, which is the same answer for two different reasons.
func (c *HelperClient) BaseEnsureWith(ctx context.Context, force, repair bool) (BaseState, error) {
	var out struct {
		State BaseState `json:"state"`
		Error string    `json:"error"`
	}
	body := map[string]any{"force": force, "repair": repair, "config": c.cfg}
	if err := c.call(ctx, http.MethodPost, "/v1/base/ensure", body, &out); err != nil {
		return out.State, err
	}
	if out.Error != "" {
		return out.State, errors.New(out.Error)
	}
	return out.State, nil
}

// Run asks the helper to execute one job.
func (c *HelperClient) Run(ctx context.Context, req HelperRunRequest) (JobResult, string, error) {
	return c.RunStream(ctx, req, HelperSinks{})
}

// Matrix asks the helper to execute a fleet.
func (c *HelperClient) Matrix(ctx context.Context, req HelperMatrixRequest) (MatrixReport, string, error) {
	return c.MatrixStream(ctx, req, HelperSinks{})
}

// bytesReader is bytes.NewReader, named here so the streaming file does not need
// its own import of the package for one call.
func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// Resources asks what the machine is holding.
func (c *HelperClient) Resources(ctx context.Context) (ResourceReport, error) {
	var out struct {
		Report ResourceReport `json:"report"`
	}
	err := c.call(ctx, http.MethodGet, "/v1/resources", nil, &out)
	return out.Report, err
}

// Inspect asks what one job was, and what the machine was doing when it ran.
//
// ⛔ THE UNKNOWN-JOB REFUSAL IS REBUILT AS THE SAME TYPED ERROR the direct path
// returns, from the flag the helper sets. Without it the two routes would give
// different exit codes for the same question: `inspect` reports an unknown id
// as exitFailed and everything else as exitCannot, and a generic "the helper
// refused" would land in the second. WSL-58.
func (c *HelperClient) Inspect(ctx context.Context, id string, since time.Duration) (InspectReport, error) {
	var out struct {
		Report     InspectReport `json:"report"`
		UnknownJob bool          `json:"unknown_job"`
		Error      string        `json:"error"`
	}
	q := url.Values{}
	if id != "" {
		q.Set("id", id)
	}
	if since > 0 {
		q.Set("since_ms", strconv.FormatInt(since.Milliseconds(), 10))
	}
	// The configuration travels base64 in the query, as base/status does: a GET
	// with a body is something an intermediary is free to drop.
	cfg, err := json.Marshal(c.cfg)
	if err != nil {
		return out.Report, err
	}
	q.Set("config_b64", base64.StdEncoding.EncodeToString(cfg))
	if err := c.call(ctx, http.MethodGet, "/v1/inspect?"+q.Encode(), nil, &out); err != nil {
		return out.Report, err
	}
	if out.UnknownJob {
		message := out.Error
		if message == "" {
			message = id
		}
		return out.Report, fmt.Errorf("%w: %s", ErrUnknownJob, message)
	}
	if out.Error != "" {
		return out.Report, errors.New(out.Error)
	}
	return out.Report, nil
}

// Cleanup asks the helper to remove what this tool made.
func (c *HelperClient) Cleanup(ctx context.Context, apply bool, policy CleanupPolicy, images bool) (CleanupPlan, error) {
	var out struct {
		Plan  CleanupPlan `json:"plan"`
		Error string      `json:"error"`
	}
	body := map[string]any{
		"apply": apply, "older_than_ms": policy.OlderThan.Milliseconds(),
		"images": images, "include_live": policy.IncludeLive,
	}
	if err := c.call(ctx, http.MethodPost, "/v1/gc", body, &out); err != nil {
		return out.Plan, err
	}
	if out.Error != "" {
		return out.Plan, errors.New(out.Error)
	}
	return out.Plan, nil
}

// EncodeScript is how a payload crosses the protocol.
//
// ⭐ Base64 for the same reason the script's own command channel uses it: it is
// the one encoding nothing between here and there interprets.
func EncodeScript(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
