// SPDX-License-Identifier: 0BSD

package checks

import (
	"fmt"
	"strconv"
	"strings"
)

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }

func itoa(i int) string { return strconv.Itoa(i) }

// isComment reports whether a line is a comment in any of the languages this
// tree carries. ⚠ It reads the line's FIRST token only: a trailing comment is
// on a line with code, and reading such a line as code is the safe direction.
func isComment(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "#") || strings.HasPrefix(s, "//") ||
		strings.HasPrefix(s, "*") || strings.HasPrefix(s, "/*")
}
