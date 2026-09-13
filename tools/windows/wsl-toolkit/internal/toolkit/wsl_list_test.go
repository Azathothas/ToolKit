// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAFailedListingNamesItsQueryAndIsAskedAgainOnlyWhenThatCanHelp(t *testing.T) {
	process := &ProcessError{Code: 2, Op: "process", Err: errors.New("exit status 0xffffffff")}
	err := listFailure([]string{"--list", "--running", "--quiet"}, "", "", process)
	if !strings.Contains(err.Error(), "wsl.exe --list --running --quiet failed and printed nothing") {
		t.Errorf("the failure does not name the query: %v", err)
	}
	var pe *ProcessError
	if !errors.As(err, &pe) || pe.Code != 2 {
		t.Errorf("naming the query lost the process error a caller reads the code from: %v", err)
	}
	if said := listFailure([]string{"--list", "--quiet"}, "\n", "Catastrophic failure\r\nError code: Wsl/0x8000ffff", process); !strings.Contains(said.Error(), "printed: Catastrophic failure") {
		t.Errorf("the failure does not carry what wsl.exe printed: %v", said)
	}
	if !retryableListFailure(err) {
		t.Error("a listing that failed for no stated reason is not asked once more")
	}
	for label, refused := range map[string]error{
		"a refusal of this process": fmt.Errorf("%w: access is denied", ErrWslDenied),
		"a listing that timed out":  &ProcessError{Code: ExitTimeout, Op: "process", Err: context.DeadlineExceeded},
	} {
		if retryableListFailure(refused) {
			t.Errorf("%s is asked again, which changes nothing and doubles the wait", label)
		}
	}
}
