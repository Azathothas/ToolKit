// SPDX-License-Identifier: 0BSD

//go:build !windows

package toolkit

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// The executable is Windows-only; this half reads /proc, so the suite proves
// the watcher on the second host it runs on.

// procStat reads a process's name, parent, start time and state from /proc.
// ok is false for a process that is gone or a zombie, which has ended.
func procStat(pid int) (name string, ppid int, start int64, ok bool) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", 0, 0, false
	}
	s := string(b)
	open, closeAt := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if open < 0 || closeAt < open {
		return "", 0, 0, false
	}
	name = s[open+1 : closeAt]
	fields := strings.Fields(s[closeAt+1:])
	// state is field 3, ppid field 4 and starttime field 22 of proc(5).
	if len(fields) < 20 || fields[0] == "Z" || fields[0] == "X" {
		return name, 0, 0, false
	}
	ppid, _ = strconv.Atoi(fields[1])
	start, err = strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return name, 0, 0, false
	}
	return name, ppid, start, true
}

func callerChain(max int) ([]ProcessIdentity, error) {
	_, ppid, childStart, ok := procStat(os.Getpid())
	if !ok {
		return nil, errNoChain
	}
	var out []ProcessIdentity
	pid := ppid
	for len(out) < max && pid > 1 {
		name, next, start, ok := procStat(pid)
		if !ok || chainEnds[name] || start > childStart {
			break
		}
		out = append(out, ProcessIdentity{PID: pid, Start: start, Name: name, Role: RoleCaller})
		childStart, pid = start, next
	}
	return out, nil
}

func selfIdentity() (ProcessIdentity, error) { return processIdentity(os.Getpid()) }

func processIdentity(pid int) (ProcessIdentity, error) {
	name, _, start, ok := procStat(pid)
	if !ok {
		return ProcessIdentity{}, fmt.Errorf("process %d is not running", pid)
	}
	return ProcessIdentity{PID: pid, Start: start, Name: name}, nil
}

// procWatch reads each watched process's start time again until one differs.
type procWatch struct {
	ids  []ProcessIdentity
	gone *ProcessIdentity
}

func watchProcesses(ps []ProcessIdentity) (ProcessWatch, error) {
	return &procWatch{ids: ps}, nil
}

func (w *procWatch) Exited(timeout time.Duration) (*ProcessIdentity, error) {
	deadline := time.Now().Add(timeout)
	for {
		if w.gone != nil {
			return w.gone, nil
		}
		for i, p := range w.ids {
			if _, _, start, ok := procStat(p.PID); !ok || start != p.Start {
				w.gone = &w.ids[i]
				return w.gone, nil
			}
		}
		if !time.Now().Before(deadline) {
			return nil, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (w *procWatch) Close() {}
