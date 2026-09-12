package toolkit

import (
	"fmt"
	"syscall"
	"unsafe"
)

// whpxAvailable answers whether the Windows Hypervisor Platform is installed and
// its hypervisor is running.
//
// ⭐ UNELEVATED, IN ONE CALL, and that is the point. `Get-WindowsOptionalFeature`
// refuses a caller who is not an administrator, and so do Hyper-V's own APIs:
// measured 2026-08-27, `HcsEnumerateComputeSystems` - a READ - answers
// 0x8037011B, Hyper-V Administrators only. `WinHvPlatform.dll` resolves only
// when the optional feature is installed, so binding to it at all is half the
// answer, and capability 0 is HypervisorPresent, which is the other half.
func whpxAvailable() (bool, string) {
	dll := syscall.NewLazyDLL("WinHvPlatform.dll")
	if err := dll.Load(); err != nil {
		return false, "WinHvPlatform.dll did not load, so the optional feature is not installed"
	}
	proc := dll.NewProc("WHvGetCapability")
	if err := proc.Find(); err != nil {
		return false, "WHvGetCapability was not found in WinHvPlatform.dll"
	}
	var value uint32
	var written uint32
	// WHvCapabilityCodeHypervisorPresent is 0.
	hr, _, _ := proc.Call(
		uintptr(0),
		uintptr(unsafe.Pointer(&value)),
		uintptr(4),
		uintptr(unsafe.Pointer(&written)),
	)
	if hr == 0 && value == 1 {
		return true, "present, hypervisor running"
	}
	return false, fmt.Sprintf("hr=0x%08X value=%d", uint32(hr), value)
}
