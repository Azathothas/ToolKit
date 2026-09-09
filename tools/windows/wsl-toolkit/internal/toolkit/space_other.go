//go:build !windows

package toolkit

// FreeSpace is unanswerable on a host this executable does not manage disks on.
// It reports "could not measure" rather than a number, because a fabricated
// figure is worse than an absence: a blank gets checked and a number gets used.
func FreeSpace(string) (int64, bool, error) { return 0, false, nil }
