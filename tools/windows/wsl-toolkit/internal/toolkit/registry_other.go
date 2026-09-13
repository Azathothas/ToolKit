// SPDX-License-Identifier: 0BSD

//go:build !windows

package toolkit

import "fmt"

// registeredDisks is unanswerable off Windows: WSL records its distributions in
// the Windows registry. It refuses rather than reporting an empty machine,
// because an empty answer would let an ownership check pass over nothing.
func registeredDisks() (map[string]string, error) {
	return nil, fmt.Errorf("%w: WSL's registrations live in the Windows registry", ErrWslMissing)
}
