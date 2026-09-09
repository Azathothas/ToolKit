package toolkit

import (
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// FreeSpace reports the bytes available to this caller on the volume holding a
// path, and whether the question could be answered at all.
//
// ⚠ THREE ANSWERS, NOT TWO. A volume whose free space cannot be read is not the
// same fact as a volume with no space, and treating it as either is a lie in
// one direction or a needless refusal in the other. The boolean is what keeps
// them apart.
//
// It walks up to the nearest existing ancestor, because the directory an import
// is about to create does not exist yet and the volume is the same either way.
func FreeSpace(path string) (int64, bool, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, false, err
	}
	for {
		p, err := syscall.UTF16PtrFromString(abs)
		if err != nil {
			return 0, false, err
		}
		var free, total, totalFree uint64
		r, _, _ := procGetDiskFreeSpaceEx.Call(
			uintptr(unsafe.Pointer(p)),
			uintptr(unsafe.Pointer(&free)),
			uintptr(unsafe.Pointer(&total)),
			uintptr(unsafe.Pointer(&totalFree)),
		)
		if r != 0 {
			return int64(free), true, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return 0, false, nil
		}
		abs = parent
	}
}
