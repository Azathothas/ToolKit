// SPDX-License-Identifier: 0BSD

//go:build !windows

package checks

import (
	"os"
	"path/filepath"
	"testing"
)

// otherSpelling is a link to dir, as a root reached through a link is spelled.
func otherSpelling(t *testing.T, dir string) string {
	t.Helper()
	link := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	return link
}
