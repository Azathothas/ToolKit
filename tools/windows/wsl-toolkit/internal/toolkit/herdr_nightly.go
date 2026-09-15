// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ⭐ THE HERDR ADAPTER MAY FOLLOW A CHANNEL, and the one channel is this repository's
// own herdr nightlies, built by .github/workflows/herdr-nightly.yml from herdr's
// development branch. Ruled by the operator on 2026-09-15 over the recommended pin by
// version and digest. WSL-90.
//
// ⚠ WHAT A NIGHTLY'S DIGEST PROVES. Each one is read from the nightly's own SHA256SUMS,
// which ships in the same release as the files it covers, so it proves the bytes arrived
// as they were published and not who published them. Each file's keyless bundle proves
// that, and this executable does not verify one: it would need a Sigstore client this
// module does not carry.

// AdapterChannelNightly is the one channel an adapter follows.
const AdapterChannelNightly = "nightly"

// herdrNightlyTagRE is the tag herdr-nightly.yml gives a nightly: the day, and the first
// twelve characters of herdr's commit.
var herdrNightlyTagRE = regexp.MustCompile(`^herdr-nightly-[0-9]{8}-[0-9a-f]{12}$`)

// releaseDownload is the host a release's files are downloaded from. A case points it at
// a local server; nothing else sets it.
var releaseDownload = "https://github.com"

// herdrChannel answers the channel the configuration's herdr adapter follows, or "".
func herdrChannel(cfg Config) string {
	for _, a := range cfg.Base.Adapters {
		if a.Name == "herdr" {
			return a.Channel
		}
	}
	return ""
}

// LatestHerdrNightly resolves the newest herdr nightly this repository published.
//
// ⛔ ONLY A PRERELEASE WHOSE TAG IS A NIGHTLY'S. A tag somebody made by hand with the
// same prefix, a release that is not a prerelease, and a draft are all passed over,
// because the channel installs what it resolves into a base.
func LatestHerdrNightly(ctx context.Context) (string, error) {
	bounded, cancel := context.WithTimeout(ctx, UpdateCheckTimeout)
	defer cancel()
	for page := 1; page <= releaseListPages; page++ {
		list, err := releaseListPageAt(bounded, page)
		if err != nil {
			return "", err
		}
		for _, r := range list {
			if r.Draft || !r.Prerelease || !herdrNightlyTagRE.MatchString(r.TagName) {
				continue
			}
			return r.TagName, nil
		}
		if len(list) < releaseListPage {
			break
		}
	}
	return "", errors.New("this repository has published no herdr nightly, so the nightly channel has nothing to install. Remove \"channel\" from the herdr adapter to install the release it pins")
}

// herdrReleaseBase is where one of this repository's releases' files are downloaded.
func herdrReleaseBase(tag string) string {
	return releaseDownload + "/" + ReleaseRepo + "/releases/download/" + tag
}

// herdrNightlySums reads a nightly's SHA256SUMS.
func herdrNightlySums(ctx context.Context, tag string) (string, error) {
	sums, err := fetchBytes(ctx, herdrReleaseBase(tag)+"/SHA256SUMS", 1<<20)
	if err != nil {
		return "", fmt.Errorf("could not read %s's SHA256SUMS: %w", tag, err)
	}
	return string(sums), nil
}

// herdrNightlyEnv answers what the herdr adapter's install.sh reads to install the
// newest nightly's Linux build: its tag as the version, each architecture's digest from
// its SHA256SUMS, and its download base.
func herdrNightlyEnv(ctx context.Context) (map[string]string, error) {
	tag, err := LatestHerdrNightly(ctx)
	if err != nil {
		return nil, err
	}
	sums, err := herdrNightlySums(ctx, tag)
	if err != nil {
		return nil, err
	}
	env := map[string]string{"TK_ADAPTER_VERSION": tag, "TK_ADAPTER_URL_BASE": herdrReleaseBase(tag)}
	for _, arch := range []string{"x86_64", "aarch64"} {
		digest, err := digestFor(sums, "herdr-linux-"+arch)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", tag, err)
		}
		if !installerDigestRE.MatchString(digest) {
			return nil, fmt.Errorf("%s's SHA256SUMS gives herdr-linux-%s the digest %q, which is not a SHA-256", tag, arch, digest)
		}
		env["TK_ADAPTER_SHA256_"+strings.ToUpper(arch)] = digest
	}
	return env, nil
}

// -- the Windows client --------------------------------------------------------------

// herdrClientStamp is what this machine records about the herdr client it wrote.
type herdrClientStamp struct {
	Schema  string `json:"schema"`
	Release string `json:"release"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
}

const herdrClientStampSchema = "wsl-toolkit-herdr-client/1"

// herdrClientDir is where an instance keeps the herdr client its base's nightly matches.
func herdrClientDir(home string) string { return filepath.Join(home, "herdr") }

func herdrClientStampPath(home string) string {
	return filepath.Join(herdrClientDir(home), "client.json")
}

// readHerdrClient answers the client this machine recorded, and whether its file is there.
func readHerdrClient(home string) (herdrClientStamp, bool) {
	var st herdrClientStamp
	body, err := os.ReadFile(herdrClientStampPath(home))
	if err != nil || json.Unmarshal(body, &st) != nil || st.Schema != herdrClientStampSchema {
		return herdrClientStamp{}, false
	}
	if _, err := os.Stat(st.Path); err != nil {
		return st, false
	}
	return st, true
}

// HerdrWindowsClient answers the herdr client this instance's base is matched with, or ""
// when its herdr adapter follows no channel or none has been written. It reads and starts
// nothing else, so `base attach` can answer before a base exists.
func HerdrWindowsClient(cfg Config) string {
	if herdrChannel(cfg) != AdapterChannelNightly {
		return ""
	}
	home, err := Home()
	if err != nil {
		return ""
	}
	if st, ok := readHerdrClient(home); ok {
		return st.Path
	}
	return ""
}

// herdrClientArch names this process's architecture as a herdr release asset does.
func herdrClientArch() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64", nil
	case "arm64":
		return "aarch64", nil
	}
	return "", fmt.Errorf("herdr's nightlies carry no Windows client for %s", runtime.GOARCH)
}

// ensureHerdrClient writes the Windows herdr client of one nightly under home, and
// removes every other nightly's.
//
// ⛔ NOTHING IS WRITTEN UNTIL THE ZIP'S DIGEST MATCHES THE NIGHTLY'S SHA256SUMS, and an
// entry that would land outside the client's directory refuses the whole zip.
func ensureHerdrClient(ctx context.Context, home, tag string, log func(string)) (string, error) {
	if !herdrNightlyTagRE.MatchString(tag) {
		return "", fmt.Errorf("the base reports herdr release %q, which is not a nightly's tag. Run: wsl-toolkit base ensure", tag)
	}
	arch, err := herdrClientArch()
	if err != nil {
		return "", err
	}
	dir := herdrClientDir(home)
	target := filepath.Join(dir, tag)
	exe := filepath.Join(target, "herdr.exe")
	if st, ok := readHerdrClient(home); ok && st.Release == tag && st.Path == exe {
		return exe, nil
	}
	name := "herdr-windows-" + arch + ".zip"
	sums, err := herdrNightlySums(ctx, tag)
	if err != nil {
		return "", err
	}
	want, err := digestFor(sums, name)
	if err != nil {
		return "", fmt.Errorf("%s: %w", tag, err)
	}
	body, err := fetchBytes(ctx, herdrReleaseBase(tag)+"/"+name, 256<<20)
	if err != nil {
		return "", fmt.Errorf("could not download %s from %s: %w", name, tag, err)
	}
	if got := sha256Hex(body); got != want {
		return "", fmt.Errorf("%s from %s does not match its SHA256SUMS: it publishes %s and %d bytes arrived digesting %s. Nothing was written", name, tag, want, len(body), got)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	partial := filepath.Join(dir, "."+tag+".partial")
	for _, stale := range []string{partial, target} {
		if _, err := os.Lstat(stale); err == nil {
			if err := RemoveInside(dir, stale); err != nil {
				return "", err
			}
		}
	}
	if err := extractZipInside(body, partial); err != nil {
		_ = RemoveInside(dir, partial)
		return "", fmt.Errorf("%s from %s: %w", name, tag, err)
	}
	if _, err := os.Stat(filepath.Join(partial, "herdr.exe")); err != nil {
		_ = RemoveInside(dir, partial)
		return "", fmt.Errorf("%s from %s carries no herdr.exe at its root", name, tag)
	}
	if err := os.Rename(partial, target); err != nil {
		_ = RemoveInside(dir, partial)
		return "", err
	}
	stamp, err := json.MarshalIndent(herdrClientStamp{Schema: herdrClientStampSchema, Release: tag, Path: exe, SHA256: want}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(herdrClientStampPath(home), append(stamp, '\n'), 0o644); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != tag && herdrNightlyTagRE.MatchString(e.Name()) {
			if err := RemoveInside(dir, filepath.Join(dir, e.Name())); err != nil {
				return "", err
			}
			log("removed the herdr client of " + e.Name())
		}
	}
	log("wrote herdr's Windows client from " + tag + ", " + exe)
	return exe, nil
}

// extractZipInside writes every file of a zip under root.
//
// ⛔ AN ENTRY THAT RESOLVES OUTSIDE ROOT REFUSES THE ZIP. A name carrying `..` would
// otherwise write wherever it says, and one naming a drive or starting at a root is
// refused before that.
func extractZipInside(body []byte, root string) error {
	// ⚠ archive/zip may answer ErrInsecurePath beside a usable reader, depending on
	// GODEBUG; the entries are checked below either way, and that check names the entry.
	r, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		name := strings.ReplaceAll(f.Name, `\`, "/")
		if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
			return fmt.Errorf("the zip names %q, which is not a relative path", f.Name)
		}
		// ⛔ THE ONE CONTAINMENT CHECK: a name that climbs out resolves outside root.
		dest, err := ResolveInside(root, filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return fmt.Errorf("the zip names %q: %w", f.Name, err)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
		if err != nil {
			_ = src.Close()
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(src, 256<<20))
		closeErr := out.Close()
		_ = src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
