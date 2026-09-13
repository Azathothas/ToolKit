// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"syscall"
)

// The two registry answers that mean a key is not there any more. They are
// numbered here rather than named from the syscall package, which spells them
// only on Windows, so the rule is one function whichever host runs the suite.
const (
	registryFileNotFound syscall.Errno = 2    // ERROR_FILE_NOT_FOUND
	registryKeyDeleted   syscall.Errno = 1018 // ERROR_KEY_DELETED
)

// registrationGone reports a registry answer meaning a distribution's key was
// removed after the enumeration listed it.
//
// ⛔ A CONCURRENT UNREGISTER, NOT AN UNREADABLE REGISTRY. Another run removing
// its own distribution deletes a subkey between this enumeration and the open,
// and failing the whole read for that would fail every command of this run,
// including a removal's own read-back. Every other failure still refuses.
func registrationGone(err error) bool {
	return errors.Is(err, registryFileNotFound) || errors.Is(err, registryKeyDeleted)
}
