// pullstall.go - how long a pull may be silent, sized from what it pulls.
//
// ⛔ WHAT WAS WRONG. A pull was given up after four minutes with no output, and
// the comment beside the limit said a working pull printed a line every few
// seconds. It does not. podman on Windows is a remote client with no terminal,
// and it prints a line as a layer starts and another as it ends, with nothing
// between. So the stall limit was a minimum bandwidth: the default base's one
// 193.6 MB layer failed below about 6.5 Mbit/s, and the refusal said it was not
// a slow link. Every retry started from zero. WSL-176.
//
// ⭐ THE LIMIT IS THE TIME THE LARGEST LAYER TAKES AT A FLOOR RATE, read from
// the image's registry before the pull, and never under four minutes. The
// export keeps four minutes, because it writes the archive as it moves.
// WSL_TOOLKIT_PULL_STALL names a pull's limit outright, for a slower link.
//
// ⛔ THE REGISTRY IS ASKED, NOT THE ENGINE. `podman manifest inspect` prints a
// list's shape for an image too: podman 4's API answers a Schema2List and
// podman 5 a ManifestListData, and neither carries `layers`. So a remote podman
// named no layer, and every pull on the engine this tool prefers kept four
// minutes. The registry's own answer is the same for every engine. WSL-176.
//
// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// PullFloorRate is the slowest link a pull's limits are sized for, in bytes a
// second. ⚠ About 2 Mbit/s, a quarter of the slowest link the old limit let the
// default base through, so a working link is never the one given up.
const PullFloorRate = 256 << 10

// PullStallEnv names a pull's stall limit outright, as a duration such as 45m.
const PullStallEnv = "WSL_TOOLKIT_PULL_STALL"

// pullStallFloor is the shortest a pull's stall limit is sized to, and a case's
// stand-in for it.
var pullStallFloor = EngineStallTimeout

// ManifestReadTimeout bounds every read of one image's manifests from its
// registry, its token included.
//
// ⚠ SHORT ON PURPOSE. It only sizes a limit, and a registry that does not
// answer in time leaves the pull the floor it always had.
const ManifestReadTimeout = 30 * time.Second

// maxManifestBytes bounds one manifest or token answer. ⚠ An index of every
// platform is a few kilobytes, and an unbounded read is a client a registry can
// exhaust.
const maxManifestBytes = 4 << 20

// registryTransport carries every read of a registry.
//
// ⛔ A CASE PUTS ITS OWN HERE, so no case reads a real registry.
var registryTransport http.RoundTripper = http.DefaultTransport

// pullLimit is how long one pull may be silent, how long it may run, the
// phrase the pulling line gives for the first, and the sentence a refusal for
// silence gives.
type pullLimit struct {
	Stall time.Duration
	Total time.Duration
	Why   string
	Hint  string
	// Sized says the stall came from the image's own layers.
	Sized bool
}

// pullStallSetting is what WSL_TOOLKIT_PULL_STALL says for one build, and
// pullStallKey carries it on a build's context.
type pullStallSetting struct {
	Stall time.Duration
	Set   bool
}

type pullStallKey struct{}

// readPullStall reads WSL_TOOLKIT_PULL_STALL from this process's environment,
// and refuses a value that is no positive duration.
func readPullStall() (time.Duration, bool, error) {
	if v := strings.TrimSpace(os.Getenv(PullStallEnv)); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return 0, false, fmt.Errorf("%s=%q is not a positive duration such as 45m, so nothing was pulled", PullStallEnv, v)
		}
		return d, true, nil
	}
	return 0, false, nil
}

// withPullStall answers ctx carrying a caller's own setting, where zero is a
// caller that set none.
//
// ⛔ THE HELPER BUILDS FOR ITS CALLER. Its own environment is whatever the
// process that started it held, so a client's variable reached no pull there,
// and a retry the refusal advised failed the same way. WSL-176.
func withPullStall(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, pullStallKey{}, pullStallSetting{Stall: d, Set: d > 0})
}

// pullStallFor answers the setting a build reads: its caller's where ctx
// carries one, and this process's environment where it does not.
//
// ⭐ A BUILD ASKS THIS BEFORE THE ENGINE, so a value that is no duration refuses
// before anything is removed or pulled, and a dry run refuses it too.
func pullStallFor(ctx context.Context) (time.Duration, bool, error) {
	if s, ok := ctx.Value(pullStallKey{}).(pullStallSetting); ok {
		return s.Stall, s.Set, nil
	}
	return readPullStall()
}

// pullLimits answers one pull's limits: the variable where it is set, else the
// largest layer's time at PullFloorRate, else the floor.
//
// ⛔ A LAYER LIST THAT CANNOT BE READ IS NO REFUSAL. The pull still runs, with
// the four minutes it always had, and the line says why and names the variable.
func (e *Engine) pullLimits(ctx context.Context, ref string) (pullLimit, error) {
	named, set, err := pullStallFor(ctx)
	if err != nil {
		return pullLimit{}, err
	}
	// ⚠ READ WHERE THE VARIABLE IS SET TOO. It names the stall, and the layers
	// still size the ceiling, so a longer stall never shortens it.
	largest, total, readErr := registryLayerBytes(ctx, ref, e.Arch)
	switch {
	case set:
		return namedPullLimit(named, largest, total), nil
	case readErr != nil:
		return pullLimit{
			Stall: pullStallFloor, Total: EngineTransferTimeout,
			Why: fmt.Sprintf("its layers could not be read, %v. On a link slower than %s/s a layer can take longer, so set %s",
				readErr, HumanBytes(PullFloorRate), PullStallEnv),
			Hint: fmt.Sprintf(" This limit was the floor of %s, because its layers could not be read.", FormatSpan(pullStallFloor)),
		}, nil
	}
	return sizedPullLimit(largest, total), nil
}

// sizedPullLimit is the rule pullLimits applies to a layer list, with no
// process in it.
func sizedPullLimit(largest, total int64) pullLimit {
	rate := HumanBytes(PullFloorRate) + "/s"
	lim := pullLimit{Stall: atFloorRate(largest), Sized: true}
	lim.Why = fmt.Sprintf("the time its largest layer, %s, takes at %s", HumanBytes(largest), rate)
	lim.Hint = " This limit was " + lim.Why + "."
	if lim.Stall < pullStallFloor {
		lim.Stall = pullStallFloor
		// ⚠ THE FLOOR WON, and the sentence says so: the layer did not set it.
		lim.Why = fmt.Sprintf("the floor, because its largest layer, %s, takes less at %s", HumanBytes(largest), rate)
		lim.Hint = fmt.Sprintf(" This limit was the floor of %s, because its largest layer, %s, takes less at %s.",
			FormatSpan(pullStallFloor), HumanBytes(largest), rate)
	}
	// ⚠ THE CEILING GROWS WITH THE STALL, or a long stall could never fire
	// before the thirty minutes every transfer has.
	lim.Total = max(EngineTransferTimeout, atFloorRate(total), lim.Stall)
	return lim
}

// namedPullLimit is the limit WSL_TOOLKIT_PULL_STALL names, over layers of
// these sizes, where zero is a layer list that could not be read.
//
// ⛔ THE VARIABLE NEVER SHORTENS THE CEILING. It skipped the layers and set
// the ceiling to its own value, so 45m gave a 1 GB image less than the 63
// minutes its layers sized. The ceiling is now the longest of thirty minutes,
// every layer at the floor rate, and every layer at the rate the variable
// allows the largest one. WSL-176.
func namedPullLimit(stall time.Duration, largest, total int64) pullLimit {
	lim := pullLimit{
		Stall: stall, Total: max(EngineTransferTimeout, atFloorRate(total), stall),
		Why:  "as " + PullStallEnv + " says",
		Hint: " This limit was the one " + PullStallEnv + " names.",
	}
	if largest > 0 && total > largest {
		// ⚠ In floating point, bounded, so a long stall over many layers cannot
		// wrap into a negative ceiling.
		scaled := float64(stall) * float64(total) / float64(largest)
		if scaled > float64(math.MaxInt64/2) {
			scaled = float64(math.MaxInt64 / 2)
		}
		lim.Total = max(lim.Total, time.Duration(scaled))
	}
	return lim
}

// atFloorRate is how long n bytes take at PullFloorRate, rounded up to a second.
func atFloorRate(n int64) time.Duration {
	if n <= 0 {
		return 0
	}
	return time.Duration((n+PullFloorRate-1)/PullFloorRate) * time.Second
}

// imageManifest is the part of an image manifest or an image index this reads.
type imageManifest struct {
	Layers []struct {
		Size int64 `json:"size"`
	} `json:"layers"`
	Manifests []struct {
		Digest   string `json:"digest"`
		Platform *struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
		} `json:"platform"`
	} `json:"manifests"`
}

// registryLayerBytes answers the largest layer and the sum of every layer of
// the image ref names for linux/arch, read from its registry anonymously.
func registryLayerBytes(ctx context.Context, ref, arch string) (int64, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, ManifestReadTimeout)
	defer cancel()
	host, repo, reference := splitImageRef(ref)
	reg := &registryReader{
		client: &http.Client{Transport: registryTransport},
		base:   registryOrigin(host) + "/v2/" + repo + "/manifests/",
		repo:   repo,
	}
	return layerBytes(ctx, reg.manifest, reference, arch)
}

// layerBytes answers the largest layer and the sum of every layer of the image
// a tag or digest names for linux/arch, reading at most two manifests: an
// index, then the image it names for this platform.
//
// ⚠ A REGISTRY THAT ANSWERS AN INDEX FOR THE IMAGE ITSELF NAMES NO LAYER, and a
// third read would answer the same, so that is an error and not a loop.
func layerBytes(ctx context.Context, read func(context.Context, string) ([]byte, error), reference, arch string) (int64, int64, error) {
	at := reference
	for range 2 {
		raw, err := read(ctx, at)
		if err != nil {
			return 0, 0, err
		}
		var doc imageManifest
		if err := json.Unmarshal(bytes.TrimSpace(raw), &doc); err != nil {
			return 0, 0, fmt.Errorf("the manifest of %s does not parse: %w", at, err)
		}
		if len(doc.Layers) > 0 {
			var largest, total int64
			for _, l := range doc.Layers {
				largest = max(largest, l.Size)
				total += l.Size
			}
			if largest <= 0 {
				return 0, 0, fmt.Errorf("the manifest of %s names no layer's size", at)
			}
			return largest, total, nil
		}
		digest := ""
		for _, m := range doc.Manifests {
			if m.Platform != nil && strings.EqualFold(m.Platform.OS, "linux") && strings.EqualFold(m.Platform.Architecture, arch) && m.Digest != "" {
				digest = m.Digest
				break
			}
		}
		if digest == "" {
			return 0, 0, fmt.Errorf("the manifest of %s names no layer and no linux/%s image", at, arch)
		}
		at = digest
	}
	return 0, 0, fmt.Errorf("the registry answered an index for %s itself, which names no layer", at)
}

// splitImageRef answers a fully qualified reference's registry host, its
// repository, and the tag or digest it names.
//
// ⚠ A DIGEST WINS OVER A TAG, as a pull reads them, and Docker Hub keeps a
// one-word repository under library/.
func splitImageRef(ref string) (host, repo, reference string) {
	host, repo, _ = strings.Cut(ref, "/")
	if at := strings.Index(repo, "@"); at >= 0 {
		repo, reference = repo[:at], repo[at+1:]
	}
	if colon := strings.LastIndex(repo, ":"); colon > strings.LastIndex(repo, "/") {
		if reference == "" {
			reference = repo[colon+1:]
		}
		repo = repo[:colon]
	}
	if reference == "" {
		reference = "latest"
	}
	if isDockerHub(host) && !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	return host, repo, reference
}

// isDockerHub says whether a registry host is Docker Hub by one of its names.
func isDockerHub(host string) bool {
	return host == "docker.io" || host == "index.docker.io"
}

// registryOrigin is the scheme and host a registry's API answers at.
func registryOrigin(host string) string {
	if isDockerHub(host) {
		host = "registry-1.docker.io"
	}
	return "https://" + host
}

// manifestAccept names every manifest shape a pull can meet.
var manifestAccept = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

// registryReader reads one repository's manifests, with the anonymous token
// its registry hands out for a pull once it asks for one.
//
// ⚠ ANONYMOUS ONLY. A private image answers 401 to this, and its pull keeps the
// floor and says why. Nothing here reads a credential.
type registryReader struct {
	client *http.Client
	base   string
	repo   string
	token  string
}

// manifest reads the manifest a tag or digest names.
func (r *registryReader) manifest(ctx context.Context, reference string) ([]byte, error) {
	for asked := false; ; asked = true {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base+reference, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", manifestAccept)
		if r.token != "" {
			req.Header.Set("Authorization", "Bearer "+r.token)
		}
		resp, err := r.client.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusUnauthorized && !asked:
			if r.token, err = r.anonymousToken(ctx, resp.Header.Get("WWW-Authenticate")); err != nil {
				return nil, err
			}
			continue
		case resp.StatusCode != http.StatusOK:
			return nil, fmt.Errorf("%s answered %s", req.URL.Redacted(), resp.Status)
		}
		return body, readErr
	}
}

// anonymousToken asks a registry's token service for a pull token with no
// credential, as a Bearer challenge directs.
func (r *registryReader) anonymousToken(ctx context.Context, challenge string) (string, error) {
	params, ok := bearerChallenge(challenge)
	if !ok {
		return "", fmt.Errorf("the registry asks for a credential this tool does not send: %q", challenge)
	}
	realm, err := url.Parse(params["realm"])
	if err != nil || (realm.Scheme != "https" && realm.Scheme != "http") {
		return "", fmt.Errorf("the registry's token service %q is not an address", params["realm"])
	}
	scope := params["scope"]
	if scope == "" {
		scope = "repository:" + r.repo + ":pull"
	}
	q := realm.Query()
	if params["service"] != "" {
		q.Set("service", params["service"])
	}
	q.Set("scope", scope)
	realm.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the registry's token service answered %s", resp.Status)
	}
	var answer struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxManifestBytes)).Decode(&answer); err != nil {
		return "", fmt.Errorf("the registry's token does not parse: %w", err)
	}
	if answer.Token == "" {
		answer.Token = answer.AccessToken
	}
	if answer.Token == "" {
		return "", errors.New("the registry's token service answered no token")
	}
	return answer.Token, nil
}

// bearerChallenge reads a WWW-Authenticate Bearer challenge's parameters.
func bearerChallenge(h string) (map[string]string, bool) {
	scheme, rest, _ := strings.Cut(strings.TrimSpace(h), " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return nil, false
	}
	params := map[string]string{}
	for rest = strings.TrimSpace(rest); rest != ""; rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), ",")) {
		key, after, ok := strings.Cut(rest, "=")
		if !ok {
			return nil, false
		}
		var val string
		if strings.HasPrefix(after, `"`) {
			end := strings.Index(after[1:], `"`)
			if end < 0 {
				return nil, false
			}
			val, rest = after[1:1+end], after[2+end:]
		} else {
			val, rest, _ = strings.Cut(after, ",")
		}
		params[strings.ToLower(strings.TrimSpace(key))] = val
	}
	return params, params["realm"] != ""
}

// engineStatusCommand is the command that says whether an engine is answering.
func engineStatusCommand(engine string) string {
	switch engine {
	case "docker":
		return "docker info"
	case "wslc":
		return "wslc version"
	}
	return "podman system connection list"
}

// pullStallHint is where to look when a pull stalls, for the engine that pulled.
func pullStallHint(engine string, lim pullLimit) string {
	return fmt.Sprintf("%s prints nothing while one layer downloads, so a link slower than %s/s reads as silence too.%s "+
		"On a slow link, set %s to a longer span, such as 45m, and retry. Otherwise the registry or the network stopped: "+
		"%s", engine, HumanBytes(PullFloorRate), lim.Hint, PullStallEnv, engineStatusCommand(engine))
}
