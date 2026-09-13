// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"strings"
	"testing"
)

func TestWithUserEnvironmentLeavesPayloadAsExactSuffix(t *testing.T) {
	payload := []byte("printf '$HOME @hostaddress'\n")
	got := WithUserEnvironment(payload)
	if !bytes.HasSuffix(got, payload) {
		t.Fatalf("payload is not the exact suffix: %q", got)
	}
	for _, want := range []string{"umask 077", "XDG_RUNTIME_DIR", "TMPDIR", "$HOME/.local/bin", "/usr/local/go/bin", "unset -f _wtk_add"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("prologue does not carry %q", want)
		}
	}
}

func TestUserEnvironmentKeepsThePathOrderContract(t *testing.T) {
	got := userEnvironmentPrelude
	want := `for _wtk_dir in "$HOME/.local/bin" "$HOME/bin" "$HOME/.cargo/bin" "$HOME/go/bin" "$HOME/.bun/bin" "$HOME/.deno/bin" "$HOME/.nix-profile/bin" /nix/var/nix/profiles/default/bin /usr/local/go/bin /usr/local/cargo/bin /usr/local/bin /usr/bin /bin /usr/local/sbin /usr/sbin /sbin; do`
	if !strings.Contains(got, want) {
		t.Fatalf("ordered path contract is absent:\n%s", got)
	}
}
