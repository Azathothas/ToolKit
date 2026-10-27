// SPDX-License-Identifier: 0BSD

//go:build windows

package checks

import (
	"strings"
	"syscall"
	"testing"
)

// otherSpelling is dir's short 8.3 form, as a root reached through DOWNLO~1 is
// spelled.
func otherSpelling(t *testing.T, dir string) string {
	t.Helper()
	from, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, 1024)
	n, err := syscall.GetShortPathName(from, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		t.Fatalf("no short form of %s: %v", dir, err)
	}
	short := syscall.UTF16ToString(buf[:n])
	if strings.EqualFold(short, dir) {
		t.Skipf("this volume gives %s no short form, so a short root cannot be spelled here", dir)
	}
	return short
}
