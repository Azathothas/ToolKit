// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-120: a prefix the client cannot read, and what gc --json says --------

// ⭐ A prefix nothing here starts is a refusal a client can tell apart, so a
// command going through the helper hands the prefix on instead.
func TestAPrefixNothingHereStartsIsNoJobHere(t *testing.T) {
	home := t.TempDir()
	selectInstance(t, DefaultInstance)
	_, err := ResolveJobID(home, "beef1234", nil)
	if !errors.Is(err, ErrNoJobHere) || !errors.Is(err, ErrJobPrefix) {
		t.Fatalf("a prefix of no job answered %v; want ErrNoJobHere and ErrJobPrefix", err)
	}
	claimEnded(t, home, "beef123400000000")
	claimEnded(t, home, "beef123411111111")
	if _, err := ResolveJobID(home, "beef1234", nil); errors.Is(err, ErrNoJobHere) || !errors.Is(err, ErrJobPrefix) {
		t.Errorf("a prefix of two jobs answered %v; want ErrJobPrefix alone", err)
	}
}

// ⛔ A job whose host directory is gone and whose record is still open is found
// by a prefix: its container can outlive its directory.
func TestAnOpenRecordIsFoundByAPrefix(t *testing.T) {
	home := t.TempDir()
	selectInstance(t, DefaultInstance)
	led := &Ledger{path: filepath.Join(home, "ledger.jsonl")}
	if err := led.Append(LedgerEntry{Event: "open", Kind: "job", ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveJobID(home, testJobID[:6], nil); err != nil || got != testJobID {
		t.Errorf("a prefix of an open record answered %q, %v", got, err)
	}
	if err := led.Append(LedgerEntry{Event: "close", Kind: "job", ID: testJobID}); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveJobID(home, testJobID[:6], nil); !errors.Is(err, ErrNoJobHere) {
		t.Errorf("a prefix of a closed record answered %v; want no job here", err)
	}
}

// helperCall sends one request to a handler and decodes its answer.
func helperCall(t *testing.T, handler http.HandlerFunc, body any) map[string]any {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(b))))
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("the helper answered %d %q", rec.Code, rec.Body.String())
	}
	return out
}

// ⛔ The helper reads a prefix of its own jobs. A client under a state
// directory of its own found nothing for it and was refused, though the helper
// held the job. WSL-120.
func TestTheHelperReadsAPrefixOfItsOwnJobs(t *testing.T) {
	home := t.TempDir()
	selectInstance(t, DefaultInstance)
	claimEnded(t, home, testJobID)
	r := &Runner{cfg: DefaultConfig(), wsl: &Wsl{Path: filepath.Join(home, "no-wsl.exe")}, home: home,
		ledger: &Ledger{path: filepath.Join(home, "ledger.jsonl")}, log: func(string) {}}
	h := &HelperServer{runner: r}

	out := helperCall(t, h.handleJobStop, HelperStopRequest{ID: "beef1234"})
	if out["unknown"] != true || !strings.Contains(out["error"].(string), "beef1234") {
		t.Errorf("a stop of a prefix the helper has no job for answered %v", out)
	}
	out = helperCall(t, h.handleJobStop, HelperStopRequest{ID: testJobID[:6]})
	if out["unknown"] == true {
		t.Fatalf("a stop of a prefix of the helper's job was answered as unknown: %v", out)
	}
	if report, _ := out["report"].(map[string]any); report["id"] != testJobID {
		t.Errorf("the stop reached %v; want %s", out["report"], testJobID)
	}
	out = helperCall(t, h.handleGC, HelperGCRequest{Job: "beef1234"})
	if out["unknown"] != true {
		t.Errorf("a cleanup of a prefix the helper has no job for answered %v", out)
	}
}

// ⛔ The two images `gc --images` keeps read as kept in its entries, never as
// pruned, and the settle after the prune leaves them so. WSL-174 and WSL-120.
func TestAKeptImageReadsKeptInTheEntries(t *testing.T) {
	kept := OwnedThing{ID: "kept0001", Name: VerifyImage}
	images := []OwnedThing{
		kept,
		{ID: "gone0001", Name: "docker.io/library/old:1"},
	}
	entries, at, ids := appendImageEntries(nil, images, DefaultInstance)
	if len(entries) != 2 || entries[0].Action != CleanupKeep || entries[0].Result != CleanupKept || entries[0].Reason == "" {
		t.Fatalf("the probe's image reads %+v", entries)
	}
	if len(at) != 1 || at[0] != 1 || len(ids) != 1 || ids[0] != "gone0001" || entries[1].Action != CleanupPrune {
		t.Fatalf("the pruned image reads %+v at %v with %v", entries[1], at, ids)
	}
	settleImageEntries(entries, at, ids, []OwnedThing{kept}, nil)
	if entries[0].Result != CleanupKept || entries[1].Result != CleanupRemoved {
		t.Errorf("after the prune the entries read %+v", entries)
	}
}
