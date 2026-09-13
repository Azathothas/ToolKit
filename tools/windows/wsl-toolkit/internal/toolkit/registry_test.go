// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"fmt"
	"syscall"
	"testing"
)

func TestARegistrationRemovedMidEnumerationIsSkippedAndOtherFailuresRefuse(t *testing.T) {
	for label, err := range map[string]error{
		"a subkey that is gone":        registryFileNotFound,
		"a subkey marked for deletion": registryKeyDeleted,
		"the open's wrapped answer":    fmt.Errorf("WSL registration {1234} could not be opened: %w", registryFileNotFound),
	} {
		if !registrationGone(err) {
			t.Errorf("%s was not read as a registration that went away: %v", label, err)
		}
	}
	for label, err := range map[string]error{
		"access denied":        syscall.Errno(5),
		"an unexpected answer": errors.New("WSL registration {1234} stores BasePath as registry type 3 rather than a string"),
		"no error":             nil,
	} {
		if registrationGone(err) {
			t.Errorf("%s was skipped as if the registration had gone: %v", label, err)
		}
	}
}
