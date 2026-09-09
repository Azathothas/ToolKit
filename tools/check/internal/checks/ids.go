// SPDX-License-Identifier: 0BSD

package checks

import "regexp"

// entryIDRe is this repository's entry id: two to six uppercase letters, a
// hyphen, and digits. WSL-31, DOC-05, TOOL-12, BSD-01.
//
// ⚠ THE BOUNDARY ON EITHER SIDE IS LOAD BEARING. Without the leading one, a
// library name ending in capitals and a version reads as an entry id and gets
// reported as one; without the trailing one, WSL-3 matches inside WSL-31 and
// the wrong entry is named.
var entryIDRe = regexp.MustCompile(`(^|[^0-9A-Za-z-])([A-Z]{2,6}-[0-9]{2,})($|[^0-9A-Za-z-])`)

// entryIDs returns the entry ids a line names.
func entryIDs(s string) []string {
	var out []string
	for _, m := range entryIDRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[2])
	}
	return out
}
