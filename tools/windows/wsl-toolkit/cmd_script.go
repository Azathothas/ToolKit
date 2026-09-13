package main

import (
	"context"
	"os"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/compat"
)

// cmdScript runs the compatibility interface with every argument forwarded
// unchanged.
//
// ⭐ THE SURFACE IS IMPLEMENTED HERE, NOT LAUNCHED. It used to carry a copy of
// the PowerShell product inside this executable and run it through a
// PowerShell host, so the binary depended on a host being installed. The
// interface is now native Go: one downloaded artefact, no PowerShell
// dependency, and the same arguments, streams, refusals and exit codes as the
// product it ports.
//
// ⛔ THE EXIT CODE IS FORWARDED, WHATEVER IT IS. An inner command that exited 1
// and a tool failure that exits 1 are the same number out of here, exactly as
// they were out of the product this ports; distinguishing them would be a
// break the interface never made.
//
// ⛔ THE STREAMS ARE THE PROCESS'S OWN: the report stream carries the action's
// report, and the notes go to this process's stderr.
func cmdScript(ctx context.Context, args []string) (int, error) {
	return compat.Run(ctx, args, os.Stdout, os.Stderr, os.Stdin), nil
}
