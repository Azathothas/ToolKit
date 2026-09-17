// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// nightlyServer answers both halves a nightly is read from: the release list, a page at
// a time, and each release's files by tag and name. It counts downloads by name.
type nightlyServer struct {
	mu        sync.Mutex
	releases  []map[string]any
	files     map[string][]byte
	downloads map[string]int
}

func newNightlyServer(t *testing.T) *nightlyServer {
	t.Helper()
	s := &nightlyServer{files: map[string][]byte{}, downloads: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.URL.Path == "/repos/"+ReleaseRepo+"/releases" {
			per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			start, end := (page-1)*per, page*per
			if start > len(s.releases) {
				start = len(s.releases)
			}
			if end > len(s.releases) {
				end = len(s.releases)
			}
			_ = json.NewEncoder(w).Encode(s.releases[start:end])
			return
		}
		prefix := "/" + ReleaseRepo + "/releases/download/"
		if strings.HasPrefix(r.URL.Path, prefix) {
			key := strings.TrimPrefix(r.URL.Path, prefix)
			if body, ok := s.files[key]; ok {
				s.downloads[key]++
				_, _ = w.Write(body)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	prevAPI, prevDownload := releaseAPI, releaseDownload
	releaseAPI, releaseDownload = srv.URL, srv.URL
	t.Cleanup(func() { releaseAPI, releaseDownload = prevAPI, prevDownload })
	return s
}

func (s *nightlyServer) publish(tag string, files map[string][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var sums strings.Builder
	for name, body := range files {
		s.files[tag+"/"+name] = body
		fmt.Fprintf(&sums, "%s  %s\n", sha256Hex(body), name)
	}
	s.files[tag+"/SHA256SUMS"] = []byte(sums.String())
}

func clientZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestAChannelIsHerdrsAloneAndNeverBesideAPin holds the configuration rule: only herdr
// follows a channel, the one channel is nightly, and a channel beside a version pin is
// refused rather than one of them silently winning. WSL-90.
func TestAChannelIsHerdrsAloneAndNeverBesideAPin(t *testing.T) {
	ok := baseConfigWithAdapters(BaseAdapter{Name: "herdr", Channel: AdapterChannelNightly})
	if err := ok.Validate(); err != nil {
		t.Fatalf("herdr on the nightly channel was refused: %v", err)
	}
	museOnChannel := BaseAdapter{Name: "muse", Channel: AdapterChannelNightly}
	weekly := BaseAdapter{Name: "herdr", Channel: "weekly"}
	pinned := BaseAdapter{Name: "herdr", Channel: AdapterChannelNightly, Version: "0.10.1",
		SHA256: map[string]string{"x86_64": strings.Repeat("a", 64)}}
	cases := map[string]struct {
		adapters []BaseAdapter
		want     string
	}{
		"a channel on muse":              {[]BaseAdapter{herdrAdapter, museOnChannel}, "only herdr follows one"},
		"an unknown channel":             {[]BaseAdapter{weekly}, `the one channel is "nightly"`},
		"a channel beside a version pin": {[]BaseAdapter{pinned}, "follows one or the other"},
	}
	for name, tc := range cases {
		cfg := baseConfigWithAdapters(tc.adapters...)
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s answered %v, and it must be refused naming %q", name, err, tc.want)
		}
	}
}

// TestTheNightlyChannelInstallsWhatTheNewestNightlyPublished holds what the channel
// hands install.sh: the newest release that is a prerelease with a nightly's tag, each
// digest from that release's own SHA256SUMS, and its download base. A hand-made tag with
// the prefix, a nightly published as a full release, and a draft are passed over.
func TestTheNightlyChannelInstallsWhatTheNewestNightlyPublished(t *testing.T) {
	s := newNightlyServer(t)
	const tag = "herdr-nightly-20260915-052779c4159e"
	s.releases = []map[string]any{
		{"tag_name": "herdr-nightly-by-hand", "prerelease": true},
		{"tag_name": "herdr-nightly-20260916-aaaaaaaaaaaa", "prerelease": true, "draft": true},
		{"tag_name": "herdr-nightly-20260916-bbbbbbbbbbbb", "prerelease": false},
		{"tag_name": "wsl-toolkit-v3.0.0"},
		{"tag_name": tag, "prerelease": true},
	}
	x86, arm := []byte("linux x86_64 build"), []byte("linux aarch64 build")
	s.publish(tag, map[string][]byte{"herdr-linux-x86_64": x86, "herdr-linux-aarch64": arm})
	env, err := herdrNightlyEnv(context.Background())
	if err != nil {
		t.Fatalf("the channel failed: %v", err)
	}
	want := map[string]string{
		"TK_ADAPTER_VERSION":        tag,
		"TK_ADAPTER_URL_BASE":       releaseDownload + "/" + ReleaseRepo + "/releases/download/" + tag,
		"TK_ADAPTER_SHA256_X86_64":  sha256Hex(x86),
		"TK_ADAPTER_SHA256_AARCH64": sha256Hex(arm),
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s is %q, and the newest nightly makes it %q", k, env[k], v)
		}
	}
	if len(env) != len(want) {
		t.Errorf("the channel passes %d variables, and install.sh reads %d: %v", len(env), len(want), env)
	}
}

// TestTheWindowsClientIsWrittenOnlyOverAMatchingDigest holds the client this machine
// keeps: nothing is written when the zip's digest disagrees with SHA256SUMS or an entry
// climbs out of its directory; a matching zip is written once, recorded, and not fetched
// again; and a newer nightly's client replaces the older one.
func TestTheWindowsClientIsWrittenOnlyOverAMatchingDigest(t *testing.T) {
	arch, err := herdrClientArch()
	if err != nil {
		t.Skip(err.Error())
	}
	name := "herdr-windows-" + arch + ".zip"
	s := newNightlyServer(t)
	home := t.TempDir()
	logf := func(string) {}
	const first, second = "herdr-nightly-20260915-052779c4159e", "herdr-nightly-20260916-0123456789ab"

	good := clientZip(t, map[string]string{"herdr.exe": "client", "LICENSE": "Apache-2.0"})
	s.publish(first, map[string][]byte{name: good})
	s.mu.Lock()
	s.files[first+"/"+name] = clientZip(t, map[string]string{"herdr.exe": "a different client"})
	s.mu.Unlock()
	if _, err := ensureHerdrClient(context.Background(), home, first, logf); err == nil || !strings.Contains(err.Error(), "does not match its SHA256SUMS") {
		t.Fatalf("a zip whose digest disagrees answered %v", err)
	}
	if _, err := os.Stat(herdrClientDir(home)); err == nil {
		if entries, _ := os.ReadDir(herdrClientDir(home)); len(entries) > 0 {
			t.Fatalf("a refused zip left %d entries under %s", len(entries), herdrClientDir(home))
		}
	}

	s.publish(first, map[string][]byte{name: clientZip(t, map[string]string{"herdr.exe": "client", "../escape.txt": "out"})})
	if _, err := ensureHerdrClient(context.Background(), home, first, logf); err == nil || !strings.Contains(err.Error(), "outside the directory this tool owns") {
		t.Fatalf("a zip with an entry outside its directory answered %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "herdr", "escape.txt")); err == nil {
		t.Fatal("an entry outside the client's directory was written")
	}

	s.publish(first, map[string][]byte{name: good})
	exe, err := ensureHerdrClient(context.Background(), home, first, logf)
	if err != nil {
		t.Fatalf("a matching zip was refused: %v", err)
	}
	if body, err := os.ReadFile(exe); err != nil || string(body) != "client" || exe != filepath.Join(home, "herdr", first, "herdr.exe") {
		t.Fatalf("the client is %s holding %q (%v)", exe, body, err)
	}
	cfg := baseConfigWithAdapters(BaseAdapter{Name: "herdr", Channel: AdapterChannelNightly})
	t.Setenv("WSL_TOOLKIT_HOME", home)
	if got := HerdrWindowsClient(cfg); got != exe {
		t.Fatalf("base attach would name %q, and the client is %s", got, exe)
	}
	if got := HerdrWindowsClient(baseConfigWithAdapters(herdrAdapter)); got != "" {
		t.Fatalf("a herdr adapter on no channel named the client %q", got)
	}
	before := s.downloads[first+"/"+name]
	if _, err := ensureHerdrClient(context.Background(), home, first, logf); err != nil || s.downloads[first+"/"+name] != before {
		t.Fatalf("a second ensure of the same nightly answered %v and downloaded %d more time(s)", err, s.downloads[first+"/"+name]-before)
	}

	s.publish(second, map[string][]byte{name: clientZip(t, map[string]string{"herdr.exe": "newer client"})})
	if _, err := ensureHerdrClient(context.Background(), home, second, logf); err != nil {
		t.Fatalf("a newer nightly's client was refused: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "herdr", first)); err == nil {
		t.Fatalf("the older nightly's client is still under %s", herdrClientDir(home))
	}
}

// TestTheNightlyChannelReachesInstallThroughTheHostHalf holds the seam between the two:
// the herdr host half hands install.sh the channel's variables beside the door's key, and
// a base on no channel asks for none.
func TestTheNightlyChannelReachesInstallThroughTheHostHalf(t *testing.T) {
	home := t.TempDir()
	keyDir := filepath.Join(home, ".ssh", "wsl-toolkit")
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keyDir, "id_ed25519.pub"), []byte("ssh-ed25519 AAAAC3Nza key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	asked := 0
	h := &herdrHost{home: home, nightly: func(context.Context) (map[string]string, error) {
		asked++
		return map[string]string{"TK_ADAPTER_VERSION": "herdr-nightly-20260915-052779c4159e"}, nil
	}}
	b := &Base{cfg: baseConfigWithAdapters(BaseAdapter{Name: "herdr", Channel: AdapterChannelNightly}), log: func(string) {}}
	env, err := h.prepare(context.Background(), b)
	if err != nil || env["TK_ADAPTER_VERSION"] == "" || env["TK_SSH_CLIENT_KEY"] == "" || asked != 1 {
		t.Fatalf("a nightly base prepared %v after %d resolve(s): %v", env, asked, err)
	}
	b.cfg = baseConfigWithAdapters(herdrAdapter)
	env, err = h.prepare(context.Background(), b)
	if err != nil || env["TK_ADAPTER_VERSION"] != "" || asked != 1 {
		t.Fatalf("a base on no channel prepared %v after %d resolve(s): %v", env, asked, err)
	}

	// ⛔ A NIGHTLY BASE WITH NO MATCHING CLIENT ON THIS MACHINE IS NOT HEALTHY.
	b.cfg = baseConfigWithAdapters(BaseAdapter{Name: "herdr", Channel: AdapterChannelNightly})
	b.home = t.TempDir()
	b.wsl = &Wsl{Path: `C:\Windows\System32\wsl.exe`}
	facts := map[string]string{"version": "0.9.0", "ssh-host-key": "ssh-ed25519 AAAA", "release": "herdr-nightly-20260915-052779c4159e"}
	found := false
	checkProblems, _ := h.check(context.Background(), b, facts)
	for _, p := range checkProblems {
		found = found || strings.Contains(p, "no herdr client for the base's herdr-nightly-20260915-052779c4159e")
	}
	if !found {
		t.Fatalf("a nightly base with no client on this machine was not named: %v", checkProblems)
	}
}
