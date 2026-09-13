// SPDX-License-Identifier: 0BSD

//go:build windows

package toolkit

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

// lxssKey is where WSL records every distribution registered for this user.
const lxssKey = `Software\Microsoft\Windows\CurrentVersion\Lxss`

// errNoMoreItems is ERROR_NO_MORE_ITEMS, which ends a registry enumeration. The
// syscall package does not name it.
const errNoMoreItems syscall.Errno = 259

// registeredDisks answers, for every distribution registered to this user, the
// directory WSL keeps its disk in.
//
// ⭐ WSL's OWN RECORD, READ WITHOUT STARTING ANYTHING. `wsl --list` names a
// distribution and never says where it lives, and where it lives is the one fact
// that separates a throwaway distribution this state directory made from one
// that merely shares the prefix.
//
// ⛔ A KEY THAT DOES NOT EXIST IS AN EMPTY MACHINE, and any other failure is a
// refusal. Folding a registry that could not be read into "nothing registered"
// would make an ownership check pass over distributions nobody looked at.
func registeredDisks() (map[string]string, error) {
	path, err := syscall.UTF16PtrFromString(lxssKey)
	if err != nil {
		return nil, err
	}
	var root syscall.Handle
	if err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, path, 0, syscall.KEY_READ, &root); err != nil {
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("WSL's registration key HKCU\\%s could not be opened: %w", lxssKey, err)
	}
	defer syscall.RegCloseKey(root)

	out := map[string]string{}
	for i := uint32(0); ; i++ {
		var buf [256]uint16
		n := uint32(len(buf))
		err := syscall.RegEnumKeyEx(root, i, &buf[0], &n, nil, nil, nil, nil)
		if errors.Is(err, errNoMoreItems) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("WSL's registration key could not be enumerated: %w", err)
		}
		sub := syscall.UTF16ToString(buf[:n])
		name, err := registryString(root, sub, "DistributionName")
		if registrationGone(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(name) == "" {
			continue
		}
		base, err := registryString(root, sub, "BasePath")
		if registrationGone(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[name] = cleanBasePath(base)
	}
	return out, nil
}

// registryString reads one string value of one subkey. An absent value is an
// empty string; an unreadable one is an error.
func registryString(root syscall.Handle, sub, value string) (string, error) {
	subPtr, err := syscall.UTF16PtrFromString(sub)
	if err != nil {
		return "", err
	}
	var key syscall.Handle
	if err := syscall.RegOpenKeyEx(root, subPtr, 0, syscall.KEY_READ, &key); err != nil {
		return "", fmt.Errorf("WSL registration %s could not be opened: %w", sub, err)
	}
	defer syscall.RegCloseKey(key)
	valuePtr, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		return "", err
	}
	var kind, size uint32
	if err := syscall.RegQueryValueEx(key, valuePtr, nil, &kind, nil, &size); err != nil {
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
			return "", nil
		}
		return "", fmt.Errorf("WSL registration %s has an unreadable %s: %w", sub, value, err)
	}
	if kind != syscall.REG_SZ && kind != syscall.REG_EXPAND_SZ {
		return "", fmt.Errorf("WSL registration %s stores %s as registry type %d rather than a string", sub, value, kind)
	}
	if size == 0 {
		return "", nil
	}
	buf := make([]uint16, (size+1)/2+1)
	if err := syscall.RegQueryValueEx(key, valuePtr, nil, &kind, (*byte)(unsafe.Pointer(&buf[0])), &size); err != nil {
		return "", fmt.Errorf("WSL registration %s has an unreadable %s: %w", sub, value, err)
	}
	return syscall.UTF16ToString(buf), nil
}
