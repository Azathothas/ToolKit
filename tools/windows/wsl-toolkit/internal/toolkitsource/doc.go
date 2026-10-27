// Package toolkitsource reads internal/toolkit's own source, type-checked, for
// the rules a run of its cases cannot show. TOOL-40.
//
// ⭐ ITS OWN PACKAGE, SO internal/toolkit's TEST BINARY DOES NOT LINK go/types.
// Every mutation row links that binary again, and go/types made each link
// slower. Only this package's test files hold code.
//
// SPDX-License-Identifier: 0BSD

package toolkitsource
