// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// releaseListServer answers the release list the way GitHub's API does, newest first,
// a page at a time, from the given entries, and counts the pages asked for.
func releaseListServer(t *testing.T, entries []map[string]any) (*httptest.Server, *int) {
	t.Helper()
	asked := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+ReleaseRepo+"/releases" {
			http.NotFound(w, r)
			return
		}
		asked++
		per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if per <= 0 {
			per = 30
		}
		if page <= 0 {
			page = 1
		}
		start := (page - 1) * per
		end := start + per
		if start > len(entries) {
			start = len(entries)
		}
		if end > len(entries) {
			end = len(entries)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(entries[start:end])
	}))
	t.Cleanup(srv.Close)
	prev := releaseAPI
	releaseAPI = srv.URL
	t.Cleanup(func() { releaseAPI = prev })
	return srv, &asked
}

func nightly(i int) map[string]any {
	return map[string]any{"tag_name": fmt.Sprintf("herdr-nightly-20260915-%012d", i), "prerelease": true, "draft": false}
}

// TestTheNewestReleaseIsFoundBehindAnyNumberOfNightlies holds the lookup to a list
// that this repository's herdr nightlies fill: a hundred and fifty prereleases stand in
// front of the newest wsl-toolkit release, which a single page of thirty never reaches.
// WSL-90.
func TestTheNewestReleaseIsFoundBehindAnyNumberOfNightlies(t *testing.T) {
	var entries []map[string]any
	for i := 0; i < 150; i++ {
		entries = append(entries, nightly(i))
	}
	entries = append(entries,
		map[string]any{"tag_name": "wsl-toolkit-v9.9.9", "prerelease": true},
		map[string]any{"tag_name": "wsl-toolkit-v9.9.8", "draft": true},
		map[string]any{"tag_name": "wsl-toolkit-v3.1.0", "assets": []map[string]any{
			{"name": "SHA256SUMS", "browser_download_url": "https://example.invalid/SHA256SUMS"},
		}},
		map[string]any{"tag_name": "wsl-toolkit-v3.0.0"},
	)
	_, asked := releaseListServer(t, entries)
	rel, err := LatestRelease(context.Background())
	if err != nil {
		t.Fatalf("the lookup behind 150 nightlies failed after %d page(s): %v", *asked, err)
	}
	if rel.Tag != "wsl-toolkit-v3.1.0" || rel.Version != "3.1.0" || rel.Assets["SHA256SUMS"] == "" {
		t.Fatalf("the lookup answered %+v, and the newest release that is neither a draft nor a prerelease is wsl-toolkit-v3.1.0", rel)
	}
	if *asked != 2 {
		t.Fatalf("the lookup read %d page(s), and the release is on the second page of a hundred", *asked)
	}
}

// TestAListWithNoReleaseOfThisToolEndsAndSaysSo holds the other end of the same loop:
// a list that runs out answers the refusal rather than reading forever.
func TestAListWithNoReleaseOfThisToolEndsAndSaysSo(t *testing.T) {
	var entries []map[string]any
	for i := 0; i < 120; i++ {
		entries = append(entries, nightly(i))
	}
	_, asked := releaseListServer(t, entries)
	_, err := LatestRelease(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no wsl-toolkit-v* release was published") {
		t.Fatalf("a list of nightlies alone answered %v", err)
	}
	if *asked != 2 {
		t.Fatalf("a list that ends on its second page was read for %d page(s)", *asked)
	}
}
