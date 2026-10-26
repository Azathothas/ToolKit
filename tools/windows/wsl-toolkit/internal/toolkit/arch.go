// SPDX-License-Identifier: 0BSD

package toolkit

import "runtime"

// nativeMachine answers the machine this Windows runs on, as an
// IMAGE_FILE_MACHINE value, and whether it could be read. ⚠ A variable so a case
// can answer for a host it does not have; production asks Windows.
var nativeMachine = readNativeMachine

// The IMAGE_FILE_MACHINE values IsWow64Process2 answers for a machine.
const (
	imageFileMachineI386  = 0x014c
	imageFileMachineARMNT = 0x01c4
	imageFileMachineAMD64 = 0x8664
	imageFileMachineARM64 = 0xaa64
)

// nativeArch is the architecture of the machine this executable runs on, as a
// Go and OCI platform token.
//
// ⛔ NOT runtime.GOARCH, WHICH IS THE EXECUTABLE'S. The amd64 build runs on an
// ARM64 Windows under emulation, and read its own GOARCH as the machine's. It
// then pulled amd64 images into an arm64 guest, read wslc's arm64 images as
// foreign, and selfupdate fetched the amd64 build again. WSL's guest follows
// the machine, not the executable. WSL-188.
//
// ⚠ A machine Windows cannot name, and every host that is not Windows, falls
// back to the executable's own architecture, which is what this answered before.
func nativeArch() string {
	if m, ok := nativeMachine(); ok {
		switch m {
		case imageFileMachineAMD64:
			return "amd64"
		case imageFileMachineARM64:
			return "arm64"
		case imageFileMachineI386:
			return "386"
		case imageFileMachineARMNT:
			return "arm"
		}
	}
	return executableArch
}

// executableArch is the executable's own architecture, which a machine Windows
// cannot name falls back to.
//
// ⚠ A VARIABLE, so a case can make it differ from every machine it answers for.
// On an arm64 host a lost ARM64 row fell back to runtime.GOARCH, arm64, and the
// case that guards it stayed green on the hardware it exists for. WSL-188.
var executableArch = runtime.GOARCH
