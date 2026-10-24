// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	waitTimeout                    = 0x102
	errInvalidParameter            = syscall.Errno(87)
)

var procWaitForMultipleObjects = kernel32.NewProc("WaitForMultipleObjects")

// processStart is when a process was created, in 100 ns ticks.
func processStart(h syscall.Handle) (int64, error) {
	var created, exited, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0, err
	}
	return int64(created.HighDateTime)<<32 | int64(created.LowDateTime), nil
}

// openWatchable opens a process for a wait, or fails.
func openWatchable(pid uint32) (syscall.Handle, error) {
	return syscall.OpenProcess(syscall.SYNCHRONIZE|processQueryLimitedInformation, false, pid)
}

// callerChain walks the processes above this one, nearest first.
//
// ⛔ A PARENT ID CAN NAME A PROCESS THAT IS NOT THE PARENT. Windows keeps the id
// of a parent that has exited, and gives the number to the next process it
// starts. So each step is held to its creation time: a parent created after
// its child is a stranger, and the chain ends there.
func callerChain(max int) ([]ProcessIdentity, error) {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("the process list: %w", err)
	}
	defer func() { _ = syscall.CloseHandle(snap) }()
	parents := map[uint32]uint32{}
	names := map[uint32]string{}
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		parents[e.ProcessID] = e.ParentProcessID
		names[e.ProcessID] = syscall.UTF16ToString(e.ExeFile[:])
	}
	self, err := syscall.GetCurrentProcess()
	if err != nil {
		return nil, err
	}
	childStart, err := processStart(self)
	if err != nil {
		return nil, err
	}
	var out []ProcessIdentity
	pid, ok := parents[uint32(os.Getpid())]
	for ok && len(out) < max && pid > 4 {
		name := names[pid]
		if chainEnds[strings.ToLower(name)] {
			break
		}
		h, err := openWatchable(pid)
		if err != nil {
			break
		}
		start, err := processStart(h)
		_ = syscall.CloseHandle(h)
		if err != nil || start > childStart {
			break
		}
		out = append(out, ProcessIdentity{PID: int(pid), Start: start, Name: name, Role: RoleCaller})
		next, known := parents[pid]
		if !known || next == pid {
			break
		}
		childStart, pid = start, next
	}
	return out, nil
}

// selfIdentity is this process.
func selfIdentity() (ProcessIdentity, error) {
	self, err := syscall.GetCurrentProcess()
	if err != nil {
		return ProcessIdentity{}, err
	}
	start, err := processStart(self)
	if err != nil {
		return ProcessIdentity{}, err
	}
	name := "wsl-toolkit.exe"
	if exe, err := os.Executable(); err == nil {
		name = filepath.Base(exe)
	}
	return ProcessIdentity{PID: os.Getpid(), Start: start, Name: name}, nil
}

// processIdentity is one running process, by pid.
func processIdentity(pid int) (ProcessIdentity, error) {
	h, err := openWatchable(uint32(pid))
	if err != nil {
		return ProcessIdentity{}, err
	}
	defer func() { _ = syscall.CloseHandle(h) }()
	start, err := processStart(h)
	return ProcessIdentity{PID: pid, Start: start, Name: fmt.Sprintf("pid %d", pid)}, err
}

// winWatch waits on a handle to each watched process.
type winWatch struct {
	ids     []ProcessIdentity
	handles []syscall.Handle
	index   []int
	gone    *ProcessIdentity
}

func watchProcesses(ps []ProcessIdentity) (ProcessWatch, error) {
	w := &winWatch{ids: ps}
	for i, p := range ps {
		h, err := openWatchable(uint32(p.PID))
		if err != nil {
			if errors.Is(err, errInvalidParameter) && w.gone == nil {
				// No process has that id any more.
				w.gone = &w.ids[i]
			}
			continue
		}
		if start, err := processStart(h); err != nil || start != p.Start {
			// ⛔ ANOTHER PROCESS HAS THE ID NOW, so the one watched is gone.
			_ = syscall.CloseHandle(h)
			if w.gone == nil {
				w.gone = &w.ids[i]
			}
			continue
		}
		w.handles = append(w.handles, h)
		w.index = append(w.index, i)
	}
	return w, nil
}

func (w *winWatch) Exited(timeout time.Duration) (*ProcessIdentity, error) {
	if w.gone != nil {
		return w.gone, nil
	}
	if len(w.handles) == 0 {
		time.Sleep(timeout)
		return nil, nil
	}
	ms := uintptr(timeout / time.Millisecond)
	r, _, callErr := procWaitForMultipleObjects.Call(uintptr(len(w.handles)), uintptr(unsafe.Pointer(&w.handles[0])), 0, ms)
	switch {
	case r < uintptr(len(w.handles)):
		w.gone = &w.ids[w.index[r]]
		return w.gone, nil
	case r == waitTimeout:
		return nil, nil
	}
	return nil, fmt.Errorf("waiting on the watched processes: %v", callErr)
}

func (w *winWatch) Close() {
	for _, h := range w.handles {
		_ = syscall.CloseHandle(h)
	}
	w.handles = nil
}
