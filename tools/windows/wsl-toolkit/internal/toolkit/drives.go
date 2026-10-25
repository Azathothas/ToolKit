// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"
)

// drivesScript removes the empty drive mount points a base with `automount off`
// carries after the restart that applied it. WSL-153, WSL-155.
//
//go:embed drives.sh
var drivesScript []byte

// sweepDrivesWhenOff sweeps the drive mount points of a base configured with
// `automount off`, and does nothing for any other setting.
func (b *Base) sweepDrivesWhenOff(ctx context.Context) error {
	automount, err := NormalizeAutomount(b.cfg.Base.Automount)
	if err != nil {
		return err
	}
	if automount != AutomountOff {
		return nil
	}
	return b.sweepDriveMountPoints(ctx)
}

// sweepDriveMountPoints runs drives.sh as root, after the restart that applies
// `automount off` and before the verifier reads the drives back.
//
// ⛔ A REFUSAL HERE IS A PROVISIONING FAILURE, as it was while the sweep lived
// in provision.sh: a one-letter directory under /mnt that is not empty cannot be
// made absent, and `automount off` cannot then be true.
func (b *Base) sweepDriveMountPoints(ctx context.Context) error {
	out := &prefixWriter{prefix: "", to: b.logWriter()}
	code, err := b.wsl.Exec(ctx, ExecRequest{
		Distro: b.cfg.Base.Name, User: "root", Script: drivesScript,
		Timeout: 5 * time.Minute, Stdout: out, Stderr: out,
	})
	out.Flush()
	if err != nil || code != 0 {
		return fmt.Errorf("the drive mount points: drives.sh exited %d: %s", code, lastToolLine(out.seen.String(), "drives: "))
	}
	if !strings.Contains(out.seen.String(), "drives-complete") {
		return errors.New("the drive mount points: drives.sh exited 0 without reaching its last line")
	}
	return nil
}
