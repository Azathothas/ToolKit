// Package binfmt answers whether binfmt_misc handlers are actually registered
// in the kernel that containers on this machine run against.
//
// The defect it exists to catch is cross-architecture execution that has never
// once worked while every visible signal says the machine is healthy. Measured
// on the reporting machine on 2026-08-27: `systemd-binfmt.service` reported
// `status=0/SUCCESS` having registered ZERO handlers, because the path it writes
// to had a systemd autofs stacked on the binfmt_misc mount and every read of it
// returned ELOOP. The unit was green, the config was complete, the emulators
// were installed, and `podman run --platform linux/arm64` failed with
// `Exec format error` that reads like an unrelated breakage.
//
// ⭐ IT READS THE KERNEL, not a unit's exit code. That is the whole point: the
// unit is the thing that lied.
//
// SPDX-License-Identifier: 0BSD
package binfmt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Schema versions the structured answer.
const Schema = "check-binfmt/1"

// Dir is the one path this reads, and it is a kernel interface rather than a
// file anything else writes.
const Dir = "/proc/sys/fs/binfmt_misc"

// Options are the flags this tool takes.
type Options struct {
	JSON    bool
	Distro  string
	Require int
}

// Report is what one read found.
type Report struct {
	Schema     string `json:"schema"`
	Source     string `json:"source"`
	Kernel     string `json:"kernel"`
	Handlers   int    `json:"handlers"`
	StatusFile string `json:"status_file"`
	Stacked    bool   `json:"stacked"`
	Problem    string `json:"problem"`
	readErr    string
}

// Run reads the kernel and returns the exit code.
//
// Exit codes: 0 read it, 1 the kernel state is broken or below --require,
// 2 could not run.
func Run(opts Options, out, errOut io.Writer) int {
	if opts.Require < 0 {
		fmt.Fprintf(errOut, "check-binfmt: --require wants a number that is not negative, got %d\n", opts.Require)
		return 2
	}
	rep, code := read(opts, errOut)
	if code != 0 {
		return code
	}

	if opts.JSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(errOut, "check-binfmt: %s\n", err)
			return 2
		}
		if rep.Problem != "" {
			return 1
		}
		return 0
	}
	return render(rep, opts, out)
}

// read finds a Linux kernel to ask and asks it.
//
// ⭐ IN ORDER: /proc/sys/fs/binfmt_misc directly when this host has one (Linux,
// WSL); `wsl.exe -d DISTRO` when it does not (a Windows host); then exit 2,
// because no Linux kernel is reachable from here.
func read(opts Options, errOut io.Writer) (Report, int) {
	rep := Report{Schema: Schema, StatusFile: "unknown"}

	if st, err := os.Stat(Dir); err == nil && st.IsDir() {
		rep.Source = "local"
		rep.Kernel = localKernel()
		entries, err := os.ReadDir(Dir)
		if err != nil {
			// ⚠ THE ERROR TEXT IS KEPT, not discarded. ELOOP is the whole
			// diagnosis, and a version of this that swallowed it would report
			// "no handlers" over the one state it exists to name.
			rep.readErr = err.Error()
		} else {
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			rep.Handlers, rep.StatusFile = classify(names)
		}
		return verdict(rep, opts), 0
	}

	wsl := findWsl()
	if wsl == "" {
		fmt.Fprintf(errOut, "check-binfmt: no %s on this host and no wsl.exe to reach one.\n", Dir)
		fmt.Fprintln(errOut, "check-binfmt: nothing to read. This is not a failure, it is not applicable here.")
		return rep, 2
	}
	rep.Source = "wsl:" + opts.Distro

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := wslRun(ctx, wsl, opts.Distro, "exit 0"); err != nil {
		fmt.Fprintf(errOut, "check-binfmt: distro %q is not registered or would not start.\n", opts.Distro)
		fmt.Fprintln(errOut, "check-binfmt: start it, or name another with --distro. Could not run.")
		return rep, 2
	}
	rep.Kernel = strings.TrimSpace(mustOut(wslRun(ctx, wsl, opts.Distro, "uname -r")))
	listing, err := wslRun(ctx, wsl, opts.Distro, "ls -1 "+Dir)
	listing = strings.ReplaceAll(listing, "\r", "")
	if err != nil || looksUnreadable(listing) {
		rep.readErr = strings.TrimSpace(listing)
		if rep.readErr == "" && err != nil {
			rep.readErr = err.Error()
		}
	} else {
		rep.Handlers, rep.StatusFile = classify(strings.Split(listing, "\n"))
	}
	return verdict(rep, opts), 0
}

// classify counts the qemu handlers and says whether the enable switch is there.
func classify(names []string) (handlers int, statusFile string) {
	statusFile = "absent"
	for _, n := range names {
		n = strings.TrimSpace(n)
		if strings.HasPrefix(n, "qemu-") {
			handlers++
		}
		if n == "status" {
			statusFile = "present"
		}
	}
	return handlers, statusFile
}

func verdict(rep Report, opts Options) Report {
	switch {
	case rep.readErr != "":
		// ⛔ ELOOP IS ITS OWN VERDICT. A directory that exists and cannot be
		// read is the stacked mount this check was written for, and calling it
		// "unreadable" alongside a permissions error would lose the diagnosis.
		if isELOOP(rep.readErr) {
			rep.Stacked = true
			rep.Problem = "stacked-mount"
		} else {
			rep.Problem = "unreadable"
		}
	case rep.Handlers < opts.Require:
		rep.Problem = "below-require"
	}
	return rep
}

// isELOOP matches the reading rather than an errno, because the answer arrives
// as a Windows-side string when it comes through wsl.exe.
func isELOOP(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "too many levels of symbolic links") || strings.Contains(l, "eloop")
}

func looksUnreadable(s string) bool {
	l := strings.ToLower(s)
	for _, m := range []string{"too many levels of symbolic links", "eloop", "no such file", "not a directory"} {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}

func render(rep Report, opts Options, out io.Writer) int {
	fmt.Fprintln(out, "check-binfmt")
	fmt.Fprintf(out, "  read from      %s\n", rep.Source)
	fmt.Fprintf(out, "  kernel         %s\n", rep.Kernel)

	if rep.Stacked {
		fmt.Fprint(out, "  handlers       UNREADABLE\n\n")
		fmt.Fprintf(out, "⛔ %s exists and CANNOT BE READ: ELOOP.\n", Dir)
		fmt.Fprint(out, "   That is a second filesystem stacked on the same path, and it is the\n")
		fmt.Fprint(out, "   state this check exists to name. systemd-binfmt.service writes into\n")
		fmt.Fprint(out, "   the path underneath and reports status=0/SUCCESS while registering\n")
		fmt.Fprint(out, "   nothing, so the unit is green and cross-architecture execution has\n")
		fmt.Fprint(out, "   never once worked.\n\n")
		fmt.Fprintln(out, "   The reading, verbatim:")
		indent(out, rep.readErr)
		return 1
	}
	if rep.readErr != "" {
		fmt.Fprint(out, "  handlers       UNREADABLE\n\n")
		fmt.Fprintf(out, "⛔ %s could not be read, and NOT with the ELOOP this check knows:\n", Dir)
		indent(out, rep.readErr)
		return 1
	}

	fmt.Fprintf(out, "  qemu handlers  %d\n", rep.Handlers)
	fmt.Fprintf(out, "  status file    %s\n", rep.StatusFile)

	if rep.Handlers == 0 {
		fmt.Fprint(out, "\n⚠ ZERO handlers are registered. Nothing is broken if this machine never\n")
		fmt.Fprint(out, "  wanted cross-architecture execution. If it did, this is why\n")
		fmt.Fprint(out, "  \"podman run --platform linux/ARCH\" fails with Exec format error, and\n")
		fmt.Fprint(out, "  a green systemd-binfmt.service does not contradict it.\n")
	}
	if rep.Problem == "below-require" {
		fmt.Fprintf(out, "\n⛔ %d handler(s) registered, --require asked for %d.\n", rep.Handlers, opts.Require)
		return 1
	}

	fmt.Fprint(out, "\n⚠ Registered is not the same as reaching a container. A handler registered\n")
	fmt.Fprint(out, "  WITHOUT the F flag needs its interpreter to exist inside the mount\n")
	fmt.Fprint(out, "  namespace that runs; with F the kernel holds the interpreter open and it\n")
	fmt.Fprint(out, "  does not. Read one to see which:\n")
	fmt.Fprintf(out, "    cat %s/qemu-aarch64\n", Dir)
	return 0
}

func indent(out io.Writer, s string) {
	for _, line := range strings.Split(s, "\n") {
		fmt.Fprintf(out, "     %s\n", line)
	}
}

func localKernel() string {
	if runtime.GOOS != "linux" {
		return "unknown"
	}
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func findWsl() string {
	for _, name := range []string{"wsl.exe", "wsl"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// wslRun asks one distribution a question.
//
// ⭐ NOTHING HERE GOES THROUGH A SHELL ON THIS SIDE, which removes the whole
// class the shell implementation had to work around: Git Bash rewrites any
// argument that looks like a POSIX path into a Windows path before the target
// process sees it, so `/bin/sh` arrived at the Linux side as
// `C:/Program Files/Git/bin/sh` and a running distro reported as unstartable.
// The shell version needed MSYS_NO_PATHCONV and MSYS2_ARG_CONV_EXCL to disable
// it; exec.Command passes the argument list to CreateProcess untouched.
//
// ⚠ WSL_UTF8 is still needed. Without it wsl.exe emits UTF-16LE and a
// redirected stdout reads as empty or as mojibake.
func wslRun(ctx context.Context, wsl, distro, script string) (string, error) {
	cmd := exec.CommandContext(ctx, wsl, "-d", distro, "-u", "root", "--", "/bin/sh", "-lc", script)
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func mustOut(s string, _ error) string { return s }
