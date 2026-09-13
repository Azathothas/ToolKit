// SPDX-License-Identifier: 0BSD

package toolkit

// Version is the product version, and it has ONE HOME: this constant.
//
// ⛔ THE HISTORY, so nobody re-derives it. It used to live in exactly one
// place, `$script:ToolkitVersion` in the PowerShell product, and this module
// READ it out of the script it embedded, so the executable and the script it
// carried could not disagree about what they were. The executable no longer
// embeds or launches a PowerShell script: it is a standalone product with its
// own release, so it declares its own version here and nothing in this tree
// reads a version out of a PowerShell file any more.
//
// The PowerShell product remains, for the callers who run it directly, and it
// keeps its own version in its own prelude. Two artefacts, two versions, and
// the release that ships them may cut one or the other or both.
//
// ⛔ Nothing else in this module may hold a copy of it.
const Version = "2.1.0"
