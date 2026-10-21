// SPDX-License-Identifier: 0BSD

//go:build !windows

package toolkit

// DiskAllocation is unanswerable on a host this executable does not manage disks
// on, and says so rather than inventing a number.
func DiskAllocation(string) (sparse bool, allocated int64, ok bool) { return false, 0, false }
