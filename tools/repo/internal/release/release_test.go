// SPDX-License-Identifier: 0BSD

package release

import (
	"strings"
	"testing"
)

func TestVersionHasExactlyOneNativeHome(t *testing.T) {
	got, err := version([]byte("package toolkit\n\nconst Version = \"3.0.0\"\n"))
	if err != nil || got != "3.0.0" {
		t.Fatalf("version = %q, %v", got, err)
	}
	for _, body := range []string{"package toolkit\n", "const Version = \"3.0.0\"\nconst Version = \"3.0.1\"\n"} {
		if _, err := version([]byte(body)); err == nil || !strings.Contains(err.Error(), "exactly one") {
			t.Fatalf("ambiguous source was accepted: %v", err)
		}
	}
}
