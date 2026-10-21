// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ReclaimSchema is what `base reclaim --json` answers.
const ReclaimSchema = "wsl-toolkit-reclaim/1"

// ErrDiskHeld is a sparse request WSL refused because its VM holds the disk.
var ErrDiskHeld = errors.New("WSL holds this base's disk")

// ReclaimResult is what one `base reclaim` did.
type ReclaimResult struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Disk   string `json:"disk"`
	// SparseBefore and SparseAfter are the disk's NTFS sparse attribute, read
	// before and after.
	SparseBefore bool `json:"sparse_before"`
	SparseAfter  bool `json:"sparse_after"`
	// AllocatedBefore and AllocatedAfter are the bytes the disk takes on its
	// volume, which is what a reclaim moves; its length can stay the same.
	AllocatedBefore int64 `json:"allocated_before"`
	AllocatedAfter  int64 `json:"allocated_after"`
	// Trimmed is what fstrim reported discarding in the guest, and TrimmedKnown
	// whether it said.
	Trimmed      int64 `json:"trimmed_bytes"`
	TrimmedKnown bool  `json:"trimmed_known"`
	// Refused is why the disk could not be made sparse, with the step that is
	// the operator's. Empty when nothing was refused.
	Refused string `json:"refused,omitempty"`
}

// diskHeldCodes are the error codes WSL answers when its VM holds a disk open.
// ⭐ THE CODE AND NOT THE SENTENCE: the sentence is localised, and the code is
// the same in every language.
//
// ⚠ TWO CODES FOR ONE CONDITION, measured on WSL 3.0.3: a running distribution
// answers WSL_E_DISTRO_NOT_STOPPED, and a stopped one whose disk the VM still
// holds answers ERROR_SHARING_VIOLATION. Stopping the distribution moves the
// answer from the first to the second and frees nothing. The driven pass found
// the first one, which the case had not carried.
var diskHeldCodes = []string{"WSL_E_DISTRO_NOT_STOPPED", "ERROR_SHARING_VIOLATION"}

// SetSparse asks WSL to make a distribution's disk sparse, so the space its guest
// frees and trims leaves the file.
//
// ⛔ WSL REFUSES WHILE ITS VM HOLDS THE DISK, and it holds every registered
// distribution's disk while the VM runs, which is while any distribution runs:
// measured on WSL 3.0.3, right after an import, after a terminate, and after a
// distribution was started and stopped. The one thing that frees it is `wsl
// --shutdown`, which this executable never runs. WSL-109.
func (w *Wsl) SetSparse(ctx context.Context, name string) error {
	if err := AssertOwnedDistro(name); err != nil {
		return err
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out, stderr, err := Output(bounded, w.Path, "--manage", name, "--set-sparse", "true")
	return sparseFailure(name, out, stderr, err)
}

// sparseFailure classifies what `wsl --manage --set-sparse` answered.
func sparseFailure(name, out, stderr string, err error) error {
	if err == nil {
		return nil
	}
	for _, code := range diskHeldCodes {
		if strings.Contains(out+stderr, code) {
			return fmt.Errorf("%w: %s (%s)", ErrDiskHeld, strings.TrimSpace(firstLine(out+"\n"+stderr)), code)
		}
	}
	return fmt.Errorf("wsl --manage %s --set-sparse true: %w: %s", name, classifyWslFailure(out, stderr, err), guestFailure(out, stderr))
}

// fstrimBytes reads the bytes `fstrim -v` reports discarding, in either of the
// two spellings util-linux and busybox write.
var fstrimBytes = regexp.MustCompile(`\((\d+) bytes\) trimmed|(\d+) bytes trimmed`)

// parseTrimmed answers the bytes one fstrim run reported, and whether it said.
func parseTrimmed(out string) (int64, bool) {
	var total int64
	found := false
	for _, m := range fstrimBytes.FindAllStringSubmatch(out, -1) {
		digits := m[1]
		if digits == "" {
			digits = m[2]
		}
		if n, err := strconv.ParseInt(digits, 10, 64); err == nil {
			total += n
			found = true
		}
	}
	return total, found
}

// DiskHeldAdvice is the operator's step when WSL holds the disk, and what it
// stops. ⛔ It is named and never taken: `wsl --shutdown` stops every
// distribution on this machine, the Podman machine's included.
func DiskHeldAdvice() string {
	return "WSL holds a distribution's disk while its VM runs, and the VM runs while any distribution does. " +
		"Stop it with `wsl --shutdown`, which stops EVERY distribution on this machine, the Podman machine too, " +
		"then run " + ToolCommand("base reclaim") + " again. This tool never runs it. " +
		"To have WSL create each new disk sparse, set sparseVhd=true under [experimental] in %USERPROFILE%\\.wslconfig"
}

// Reclaim gives the space the base's guest freed back to this machine. It makes
// the disk sparse when it is not, and trims the guest's root when it is.
func (b *Base) Reclaim(ctx context.Context) (ReclaimResult, error) {
	st, err := b.Status(ctx, false)
	res := ReclaimResult{Schema: ReclaimSchema, Name: b.cfg.Base.Name, Disk: st.DiskPath}
	if err != nil {
		return res, err
	}
	if !st.Registered {
		return res, fmt.Errorf("%s is not registered, so there is no disk to reclaim. Build it with: %s", b.cfg.Base.Name, ToolCommand("base ensure"))
	}
	sparse, allocated, ok := DiskAllocation(st.DiskPath)
	if !ok {
		return res, fmt.Errorf("the disk %s could not be read, so nothing was changed", st.DiskPath)
	}
	res.SparseBefore, res.AllocatedBefore = sparse, allocated
	if !sparse {
		b.log("making " + st.DiskPath + " sparse")
		if err := b.wsl.SetSparse(ctx, b.cfg.Base.Name); err != nil {
			res.SparseAfter, res.AllocatedAfter = sparse, allocated
			if errors.Is(err, ErrDiskHeld) {
				res.Refused = DiskHeldAdvice()
			}
			return res, err
		}
		sparse, _, _ = DiskAllocation(st.DiskPath)
		if !sparse {
			return res, fmt.Errorf("WSL answered that %s is sparse, and the file is not", st.DiskPath)
		}
	}
	b.log("trimming what the guest freed, as root")
	out, errOut, code, err := b.captureAs(ctx, "root", []byte("fstrim -v / 2>&1\n"), nil, 30*time.Minute)
	if err != nil || code != 0 {
		res.SparseAfter = sparse
		return res, fmt.Errorf("fstrim exited %d in %s: %s", code, b.cfg.Base.Name, guestFailure(errOut, out))
	}
	res.Trimmed, res.TrimmedKnown = parseTrimmed(out)
	res.SparseAfter = sparse
	if _, after, ok := DiskAllocation(st.DiskPath); ok {
		res.AllocatedAfter = after
	}
	return res, nil
}
