// SPDX-License-Identifier: 0BSD

package toolkit

// Version is the product's semantic version, and its one home. Release tags, the
// helper's compatibility check and `wsl-toolkit version` all read it, and both
// `repo release` and the release workflow read it from this file and refuse one
// that declares it other than exactly once.
const Version = "4.0.0"
