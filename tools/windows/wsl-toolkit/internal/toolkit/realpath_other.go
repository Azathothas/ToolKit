//go:build !windows

package toolkit

import "path/filepath"

// RealPath is EvalSymlinks everywhere that has no junctions to worry about.
func RealPath(path string) (string, error) { return filepath.EvalSymlinks(path) }
