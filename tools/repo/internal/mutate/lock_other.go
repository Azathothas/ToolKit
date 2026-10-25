//go:build !(windows || linux || darwin || freebsd || netbsd || openbsd || dragonfly)

// lock_other.go - a host with no lock this package knows how to take.
//
// ⚠ NO LOCK MEANS NO SHARED SLOT. Every worker stages into a private temporary
// directory instead, which is correct and only misses the build cache.
//
// SPDX-License-Identifier: 0BSD

package mutate

func lockSlot(string) (unlock func(), ok bool) { return nil, false }
