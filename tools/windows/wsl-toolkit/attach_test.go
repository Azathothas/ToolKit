// SPDX-License-Identifier: 0BSD

package main

import (
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// TestTheAttachLineUsesTheServersKeys holds the one flag WSL-76's attach line cannot
// lose: without it herdr's Windows client uses the keys configured on Windows, and
// the base's unbound close keys protect nobody who attaches that way. It also holds
// the selection the other lines carry, so a pasted line reaches the same base.
func TestTheAttachLineUsesTheServersKeys(t *testing.T) {
	prevInstance, prevConfig := toolkit.SelectedInstance, toolkit.ExplicitConfigPath
	t.Cleanup(func() { toolkit.SelectedInstance, toolkit.ExplicitConfigPath = prevInstance, prevConfig })
	toolkit.SelectedInstance = toolkit.Instance{Name: "base", Distro: "wsl-toolkit-base"}
	toolkit.ExplicitConfigPath = ""

	cfg := toolkit.DefaultConfig()
	cfg.Base.Name = "wsl-toolkit-base"
	cfg.Base.User = "herdr"
	herdr := toolkit.BaseAdapter{Name: "herdr"}
	cfg.Base.Adapters = []toolkit.BaseAdapter{herdr}
	ans := attachAnswer(cfg)
	if ans.Windows != "herdr --remote wsl-toolkit-base --remote-keybindings server" {
		t.Fatalf("the Windows line is %q", ans.Windows)
	}
	if ans.Linux[0] != "wsl-toolkit --instance base base shell" || !strings.HasPrefix(ans.Agents, "wsl-toolkit --instance base base herdr -- ") {
		t.Fatalf("the other lines do not carry the instance: %q, %q", ans.Linux, ans.Agents)
	}
}
