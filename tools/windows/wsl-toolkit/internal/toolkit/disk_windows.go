// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"syscall"
	"unsafe"
)

var procGetCompressedFileSize = kernel32.NewProc("GetCompressedFileSizeW")

// fileAttributeSparse is FILE_ATTRIBUTE_SPARSE_FILE.
const fileAttributeSparse = 0x200

// DiskAllocation answers whether a file is sparse, how many bytes it takes on its
// volume, and whether both could be read.
//
// ⚠ THE SPACE A FILE TAKES IS NOT ITS LENGTH. A sparse disk whose guest trimmed
// what it freed is long and small, so the length alone reports a disk that did
// not give its space back as one that did not shrink, and the reverse. WSL-109.
func DiskAllocation(path string) (sparse bool, allocated int64, ok bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, 0, false
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return false, 0, false
	}
	var high uint32
	low, _, callErr := procGetCompressedFileSize.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&high)))
	// ⛔ 0xFFFFFFFF IS A SIZE AS WELL AS THE ERROR VALUE, so the error is read from
	// the call, as the API documents, and not from the number.
	if errno, isErrno := callErr.(syscall.Errno); uint32(low) == 0xFFFFFFFF && (!isErrno || errno != 0) {
		return attrs&fileAttributeSparse != 0, 0, false
	}
	return attrs&fileAttributeSparse != 0, int64(high)<<32 | int64(uint32(low)), true
}
