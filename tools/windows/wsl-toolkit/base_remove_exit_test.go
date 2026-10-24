// SPDX-License-Identifier: 0BSD

package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// ⛔ A REMOVAL REFUSED BY OWNERSHIP IS EXIT 2. It changed nothing, and a removal
// that was tried and failed is exit 1. WSL-167.
func TestARemovalRefusedByOwnershipExitsTwo(t *testing.T) {
	if got := removalExit(fmt.Errorf("%w: wsl-toolkit was NOT removed", toolkit.ErrNotOwner)); got != exitCannot {
		t.Fatalf("a removal refused by ownership exits %d; want %d", got, exitCannot)
	}
	if got := removalExit(errors.New("wsl --unregister failed")); got != exitFailed {
		t.Fatalf("a removal that failed exits %d; want %d", got, exitFailed)
	}
}
