//go:build !windows

package toolkit

import "path/filepath"

// RealPath is EvalSymlinks everywhere that has no junctions to worry about.
func RealPath(path string) (string, error) { return filepath.EvalSymlinks(path) }

// finalPath is EvalSymlinks off Windows, which resolves every link there is.
func finalPath(path string) (string, error) { return filepath.EvalSymlinks(path) }
