// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// ⛔ WHY THIS EXISTS. A consumer that fetched this executable had no way to move
// to a newer one except by knowing the release URL scheme, resolving the latest
// tag, picking the asset for its architecture and verifying the digest by hand.
// That is the work WSL-17 removed for the launcher and never removed here.
// Thirteen defects were filed against v1.3.0 by an agent with no way to learn a
// newer release existed, and the answer to most of them is "upgrade". WSL-53.
//
// ⛔ NOTHING HERE READS A CREDENTIAL. The release assets are public, so the
// fetch is anonymous; a GITHUB_TOKEN in the environment is deliberately not
// read. A tool that reached for a token to update itself would be a tool that
// could reach for one for anything.
//
// ⛔ AND NOTHING HERE UPDATES WITHOUT BEING ASKED. `ready` REPORTS; `selfupdate`
// acts. An auto-updater changes the binary under a running pipeline, which is
// the one thing an agent cannot recover from.

// ReleaseRepo is where this tool's own releases live.
const ReleaseRepo = "Azathothas/ToolKit"

// ReleaseTagPrefix is what marks a release of this tool rather than of anything
// else the repository might publish later.
const ReleaseTagPrefix = "wsl-toolkit-v"

// UpdateCheckTimeout bounds the whole resolve. ⚠ SHORT ON PURPOSE: an update
// check is never the reason a caller is waiting, and `ready` must not become
// slow because GitHub is.
const UpdateCheckTimeout = 8 * time.Second

// UpdateStatus is what an update check found.
//
// ⛔ `Checked` IS ALWAYS PRESENT AND `Available` IS ONLY MEANINGFUL WITH IT.
// An absent field and a field saying "no update" are different facts, and a
// caller reading `.update.available` must not see the same value for "there is
// no newer release" and "nobody looked". WSL-53's first fork, settled that way.
type UpdateStatus struct {
	Checked   bool   `json:"checked"`
	Reason    string `json:"reason,omitempty"`
	Running   string `json:"running"`
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`
	Command   string `json:"command,omitempty"`
}

// Release is one published release of this tool.
type Release struct {
	Tag     string
	Version string
	Assets  map[string]string
}

// LatestRelease resolves the newest `wsl-toolkit-v*` release.
//
// ⭐ IT USES THE PUBLIC API AND NO CLIENT. `gh` is not required, because the
// machine this runs on is a consumer's rather than a developer's.
func LatestRelease(ctx context.Context) (Release, error) {
	var rel Release
	bounded, cancel := context.WithTimeout(ctx, UpdateCheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(bounded, http.MethodGet,
		"https://api.github.com/repos/"+ReleaseRepo+"/releases?per_page=30", nil)
	if err != nil {
		return rel, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "wsl-toolkit")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return rel, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return rel, fmt.Errorf("the release list answered %s", resp.Status)
	}
	var list []struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&list); err != nil {
		return rel, err
	}
	for _, r := range list {
		if r.Draft || r.Prerelease || !strings.HasPrefix(r.TagName, ReleaseTagPrefix) {
			continue
		}
		// ⚠ THE FIRST MATCH, because the API returns newest first. Comparing
		// version strings ourselves would mean writing a version comparator for
		// a list that is already ordered by the thing that published it.
		rel.Tag = r.TagName
		rel.Version = strings.TrimPrefix(r.TagName, ReleaseTagPrefix)
		rel.Assets = map[string]string{}
		for _, a := range r.Assets {
			rel.Assets[a.Name] = a.URL
		}
		return rel, nil
	}
	return rel, fmt.Errorf("no %s* release was published", ReleaseTagPrefix)
}

// CheckUpdate answers whether a newer release exists.
//
// ⛔ A NETWORK THAT CANNOT BE REACHED IS NOT A MACHINE THAT IS NOT READY. The
// failure is reported as `checked: false` with a reason, because a tool that
// refused to run isolated Linux jobs because GitHub was down would have invented
// a dependency it does not have.
func CheckUpdate(ctx context.Context, running string) UpdateStatus {
	st := UpdateStatus{Running: running}
	rel, err := LatestRelease(ctx)
	if err != nil {
		st.Reason = err.Error()
		return st
	}
	st.Checked = true
	st.Latest = rel.Version
	// ⛔ NEWER, NOT MERELY DIFFERENT. The first version compared the two strings
	// for inequality, so a build AHEAD of the newest release - which is every
	// development build between two releases - was told an update was available,
	// and running it would have DOWNGRADED the executable. Found by running it
	// on the working tree the moment the version was bumped to 2.0.0 against a
	// published 1.3.0.
	switch CompareVersions(rel.Version, running) {
	case 1:
		st.Available = true
		st.Command = "wsl-toolkit selfupdate"
	case 0:
		// The newest release is what is running.
	default:
		// ⚠ AHEAD OF THE NEWEST RELEASE IS NOT AN ERROR AND NOT AN UPDATE. It is
		// what a development build looks like, and saying so beats both silence
		// and a wrong offer.
		st.Reason = "this build is ahead of the newest published release, " + rel.Version
	}
	return st
}

// CompareVersions orders two dotted numeric versions: 1 when a is newer than b,
// -1 when it is older, 0 when they are the same.
//
// ⛔ A COMPONENT THAT IS NOT A NUMBER MAKES THE ANSWER "the same", which reads
// as "no update" and is the safe direction. Guessing an order for a version
// scheme this tool does not use would offer a caller a download on a comparison
// nobody defined; refusing to guess costs one manual upgrade.
func CompareVersions(a, b string) int {
	if a == b {
		return 0
	}
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		av, aok := versionPart(as, i)
		bv, bok := versionPart(bs, i)
		if !aok || !bok {
			return 0
		}
		if av != bv {
			if av > bv {
				return 1
			}
			return -1
		}
	}
	return 0
}

func versionPart(parts []string, i int) (int, bool) {
	if i >= len(parts) {
		// A missing component is zero: 2.0 and 2.0.0 are the same version.
		return 0, true
	}
	v, err := strconv.Atoi(strings.TrimSpace(parts[i]))
	if err != nil {
		return 0, false
	}
	return v, true
}

// AssetName is what this host's executable is called in a release.
func AssetName() string {
	return "wsl-toolkit-" + runtime.GOOS + "-" + runtime.GOARCH + ".exe"
}

// UpdateResult is what one selfupdate did.
type UpdateResult struct {
	Schema      string `json:"schema"`
	Running     string `json:"running"`
	Latest      string `json:"latest,omitempty"`
	Tag         string `json:"tag,omitempty"`
	Asset       string `json:"asset,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	Replaced    bool   `json:"replaced"`
	Executable  string `json:"executable,omitempty"`
	PreviousAt  string `json:"previous_at,omitempty"`
	CheckedOnly bool   `json:"checked_only"`
	Reason      string `json:"reason,omitempty"`
}

// SelfUpdate downloads a release, verifies it, and replaces this executable.
//
// ⛔ THE DIGEST IS CHECKED AGAINST THE BYTES THAT ARRIVED, before anything is
// replaced. A mismatch leaves the running executable untouched and is a refusal,
// never a warning.
func SelfUpdate(ctx context.Context, running, tag string, log func(string)) (UpdateResult, error) {
	res := UpdateResult{Schema: "wsl-toolkit-selfupdate/1", Running: running}
	if log == nil {
		log = func(string) {}
	}
	self, err := os.Executable()
	if err != nil {
		return res, fmt.Errorf("this process cannot say where its own executable is: %w", err)
	}
	self, err = filepath.Abs(self)
	if err != nil {
		return res, err
	}
	res.Executable = self

	rel, err := LatestRelease(ctx)
	if err != nil {
		return res, err
	}
	if tag != "" && tag != rel.Tag {
		// A named tag is honoured, so a caller can move DOWN as well as up.
		rel.Tag = tag
		rel.Version = strings.TrimPrefix(tag, ReleaseTagPrefix)
		rel.Assets = nil
	}
	res.Tag, res.Latest = rel.Tag, rel.Version

	name := AssetName()
	res.Asset = name
	base := "https://github.com/" + ReleaseRepo + "/releases/download/" + rel.Tag
	assetURL, sumsURL := base+"/"+name, base+"/SHA256SUMS"
	if rel.Assets != nil {
		if u, ok := rel.Assets[name]; ok {
			assetURL = u
		}
		if u, ok := rel.Assets["SHA256SUMS"]; ok {
			sumsURL = u
		}
	}

	log("fetching " + rel.Tag + " " + name)
	body, err := fetchBytes(ctx, assetURL, 256<<20)
	if err != nil {
		return res, fmt.Errorf("could not download %s: %w", name, err)
	}
	sums, err := fetchBytes(ctx, sumsURL, 1<<20)
	if err != nil {
		return res, fmt.Errorf("could not download SHA256SUMS: %w", err)
	}
	want, err := digestFor(string(sums), name)
	if err != nil {
		return res, err
	}
	got := sha256.Sum256(body)
	res.SHA256 = hex.EncodeToString(got[:])
	if res.SHA256 != want {
		// ⛔ REFUSED, AND NOTHING IS REPLACED. The running executable is
		// untouched, which is the property that makes this safe to run at all.
		return res, fmt.Errorf("the download does not match SHA256SUMS. It published %s and %d bytes arrived digesting %s. "+
			"Nothing was replaced", want, len(body), res.SHA256)
	}
	log("verified " + res.SHA256)

	if rel.Version == running {
		res.Reason = "the newest release is the version already running"
		return res, nil
	}

	// The replacement is written BESIDE the running executable and renamed into
	// place, so a killed process leaves either the old file or the new one and
	// never half of either.
	dir := filepath.Dir(self)
	staged := filepath.Join(dir, ".wsl-toolkit-update-"+strconv.FormatInt(time.Now().UnixNano(), 36)+".exe")
	if err := os.WriteFile(staged, body, 0o755); err != nil {
		return res, fmt.Errorf("could not write the replacement beside %s: %w", self, err)
	}
	// The staged file is this function's own and is removed on every path out
	// of it, so it goes through the same helper as everything else.
	removeStaged := func() {
		if err := RemoveInside(dir, staged); err != nil && !errors.Is(err, os.ErrNotExist) {
			log("the staged replacement is still on disk: " + staged)
		}
	}
	previous := filepath.Join(dir, previousPrefix+running+".exe")
	// ⛔ THROUGH THE ONE DELETION, which contains the target and reads the
	// state back. TODO/RULES.md section 3: the guard runs INSIDE the helper
	// rather than beside each caller, because a guard applied at four call
	// sites is one that will be applied at three. A leftover from an earlier
	// run at this exact path is the only thing this can reach.
	if err := RemoveInside(dir, previous); err != nil && !errors.Is(err, os.ErrNotExist) {
		return res, fmt.Errorf("a copy from an earlier update is in the way and could not be removed: %w", err)
	}
	if err := os.Rename(self, previous); err != nil {
		removeStaged()
		return res, fmt.Errorf("could not move the running executable aside: %w", err)
	}
	if err := os.Rename(staged, self); err != nil {
		// Put it back rather than leaving the caller with no executable at all.
		_ = os.Rename(previous, self)
		removeStaged()
		return res, fmt.Errorf("could not put the new executable in place: %w", err)
	}
	res.Replaced, res.PreviousAt = true, previous
	log("replaced " + self + " with " + rel.Version)
	// ⛔ IT SAYS WHICH COMMAND, because only one of them does it. The sweep runs
	// from `selfupdate` and nowhere else, so "the next run removes it" was false
	// for every other command including `version`, which is the one this tool
	// tells a caller to run next. Measured by following that advice: the copy
	// was still there.
	//
	// ⚠ The sweep is NOT moved into every command on purpose. A read-only
	// report that deletes something is the `WSL-55` shape, and `version` and
	// `doctor` create and remove nothing by design. So the message names the
	// command that acts and the file, and a caller who wants the space back now
	// has both.
	log("the previous copy is " + previous)
	log("the next `wsl-toolkit selfupdate` removes it, or delete it yourself")
	return res, nil
}

// previousPrefix names a superseded executable. ⚠ It has to be recognisable
// from a later run, which is the only process that can remove it.
const previousPrefix = ".wsl-toolkit-previous-"

// SweepPreviousExecutables removes what an earlier update left behind.
//
// ⛔ IT IS CALLED BY EVERY `selfupdate`, INCLUDING `--check`. The first version
// swept inside SelfUpdate alone, so a caller that only ever asked whether an
// update existed never collected the copy its last real update had left. Found
// by running the two in sequence and reading the directory, which is the only
// way that gap is visible: nothing fails, a file simply stays.
//
// ⚠ FAILURE IS REPORTED AND NOT RAISED. A leftover this run cannot remove is a
// few megabytes, and refusing to update over one would be worse than the file.
func SweepPreviousExecutables(log func(string)) {
	self, err := os.Executable()
	if err != nil {
		return
	}
	if abs, err := filepath.Abs(self); err == nil {
		self = abs
	}
	sweepPreviousExecutables(filepath.Dir(self), log)
}

func sweepPreviousExecutables(dir string, log func(string)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), previousPrefix) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		// ⛔ Through the one deletion, for the same reason as above. It is also
		// what reads the state back, so "removed" is a fact rather than the
		// absence of an error.
		if err := RemoveInside(dir, p); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			log("a previous copy is still here and could not be removed: " + p)
			continue
		}
		log("removed the previous copy " + p)
	}
}

func fetchBytes(ctx context.Context, url string, max int64) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(bounded, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wsl-toolkit")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, max))
}

// digestFor reads one name's digest out of a SHA256SUMS file.
//
// ⛔ A NAME THAT IS NOT THERE IS AN ERROR. Falling back to "no digest published,
// carry on" would make the whole verification optional the day the file changed
// shape, which is the shape of check that reports success over a failure.
func digestFor(sums, name string) (string, error) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		// `sha256sum` writes "<hex>  <name>", with the name possibly marked
		// binary by a leading asterisk.
		if strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS names no digest for %s, so nothing could be verified", name)
}
