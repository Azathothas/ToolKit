// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAHelperServesFromAStateDirectoryThatDoesNotExistYet: a fresh instance has
// no state directory until something writes one, and the helper is often the
// first writer. WSL-103.
func TestAHelperServesFromAStateDirectoryThatDoesNotExistYet(t *testing.T) {
	home := filepath.Join(t.TempDir(), "instances", "fresh")
	t.Setenv("WSL_TOOLKIT_HOME", home)
	h := &HelperServer{
		token: "test", stop: make(chan struct{}), log: func(string) {},
		stage: map[string]string{}, runners: map[string]*Runner{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.Serve(ctx) }()
	endpoint := filepath.Join(home, "helper.json")
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(endpoint); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("the helper stopped before it wrote its endpoint: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the helper wrote no endpoint file")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("the helper did not stop when its context ended")
	}
	if _, err := os.Stat(endpoint); !os.IsNotExist(err) {
		t.Errorf("the endpoint file outlived the helper: %v", err)
	}
}
