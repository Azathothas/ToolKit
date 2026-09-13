// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// cmdHostAddress answers the address a WSL distribution reaches this host at.
//
// ⭐ THE ADDRESS IS THE ONLY THING ON STDOUT, so `$addr = wsl-toolkit
// hostaddress` assigns it. What it means and where it came from go to stderr.
func cmdHostAddress(_ context.Context, args []string) (int, error) {
	fs := newFlagSet("hostaddress")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	ans, err := toolkit.ResolveHostAddress()
	if err != nil {
		if *asJSON {
			_ = writeJSON(ans)
		}
		return exitCannot, err
	}
	if *asJSON {
		return exitOK, writeJSON(ans)
	}
	fmt.Println(ans.Address)
	fmt.Fprintf(os.Stderr, "  networking mode %s, from %s\n", ans.Mode, ans.Source)
	if ans.Loopback {
		fmt.Fprintln(os.Stderr, "  a host service bound to 127.0.0.1 is reachable from inside a distribution")
		return exitOK, nil
	}
	fmt.Fprintf(os.Stderr, "  the address of %s. A HOST SERVICE BOUND TO 127.0.0.1 IS NOT REACHABLE from a distribution in this mode:\n", ans.Interface)
	fmt.Fprintf(os.Stderr, "  bind it to %s, or to 0.0.0.0 if the local network may reach it too. WSL assigns this address and changes it, so read it rather than recording it\n", ans.Address)
	return exitOK, nil
}
