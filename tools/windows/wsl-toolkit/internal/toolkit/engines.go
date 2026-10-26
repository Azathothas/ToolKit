// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// What the base's container engines need beyond what provisioning builds: the
// cgroup delegation that makes a limit enforced, and three opt-in settings for a
// base whose own engines are used directly. WSL-130, WSL-136.
//
// ⭐ IT RUNS IN EVERY ENSURE, NOT ONLY AT A BUILD. Provisioning runs again only
// on a base that does not verify, so a step that lived there would never reach a
// base an earlier release built and that still works.

//go:embed engines.sh
var enginesScript []byte

// ensureEngines applies engines.sh to the base. engines.sh carries the rules;
// this only runs it.
func (b *Base) ensureEngines(ctx context.Context) error {
	// ⛔ NOT FROM A STATE DIRECTORY THAT DOES NOT OWN THE BASE, as applyAdapters
	// rules. Each opt-in setting turned off removes its file, so a guest's
	// configuration, which names none of them, took away what the owner turned
	// on. Said once, and not an error, so that ensure still succeeds. WSL-130.
	if err := b.assertOwner("changed", "base ensure"); err != nil {
		b.log("the engines are left as they are: " + err.Error())
		return nil
	}
	out := &prefixWriter{prefix: "", to: b.logWriter()}
	code, err := b.wsl.Exec(ctx, b.enginesRequest(out))
	out.Flush()
	if err != nil || code != 0 {
		return fmt.Errorf("the engines: engines.sh exited %d: %s", code, lastToolLine(out.seen.String(), "engines: "))
	}
	if !strings.Contains(out.seen.String(), "engines-complete") {
		return errors.New("the engines: engines.sh exited 0 without reaching its last line")
	}
	return nil
}

// enginesRequest is the one run of engines.sh: as root, with the configuration's
// settings in its environment.
//
// ⭐ IT CARRIES THE CHECK `run` SENDS, stale.sh and repair.sh behind the runtime
// prologue, which engines.sh installs for the start of each boot. One text, so
// the check at a start and the check before a job cannot drift. WSL-178.
func (b *Base) enginesRequest(out io.Writer) ExecRequest {
	return ExecRequest{
		Distro: b.cfg.Base.Name, User: "root", Script: enginesScript,
		Env: map[string]string{
			"TK_USER":        b.cfg.Base.User,
			"TK_SYSTEMD":     strconv.FormatBool(b.cfg.Base.Systemd),
			"TK_ROOT_ENGINE": strconv.FormatBool(b.cfg.Base.RootEngine),
			"TK_RUNTIME_DIR": strconv.FormatBool(b.cfg.Base.RuntimeDir),
			"TK_KVM":         strconv.FormatBool(b.cfg.Base.Kvm),

			// WSL-178: the run directory, the check a start runs, and the script
			// WSL runs at a start, which provisioning alone wrote.
			"TK_RUN_PREFIX":    guestRunRootPrefix,
			"TK_RUN_STATE_B64": base64.StdEncoding.EncodeToString(staleRunStatePayload()),
			"TK_BOOT_B64":      base64.StdEncoding.EncodeToString(bootScript),
		},
		Timeout: 10 * time.Minute, Stdout: out, Stderr: out,
	}
}

// RuntimeDirPrologue is what names the engine's runtime directory in a command
// `base exec` runs, where base.runtime_dir is on: the one prologue every job's
// engine call starts with. WSL-130.
func RuntimeDirPrologue() []byte { return guestRuntimePrologue() }

// The words verify.sh's kvm row answers with.
const (
	KvmUsable   = "usable"
	KvmUnusable = "unusable"
	KvmAbsent   = "absent"
)

// parseKvm reads verify.sh's kvm row: whether the account opens /dev/kvm for
// reading and writing. ⚠ Empty where the guest printed no row, which is not the
// same as absent.
func parseKvm(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "kvm "); ok {
			switch v = strings.TrimSpace(v); v {
			case KvmUsable, KvmUnusable, KvmAbsent:
				return v
			}
		}
	}
	return ""
}

// kvmProblem is what a base configured to keep /dev/kvm usable earns when the
// account cannot open it. ⛔ It is a problem and not an unusable base: the
// engine runs, and a job that does not ask for the device never meets it.
func kvmProblem(configured bool, measured, user string) string {
	switch {
	case !configured || measured == "" || measured == KvmUsable:
		return ""
	case measured == KvmAbsent:
		return "base.kvm is on and this base has no /dev/kvm. WSL exposes it only with nested virtualization on in .wslconfig"
	}
	return fmt.Sprintf("base.kvm is on and %s cannot open /dev/kvm for reading and writing. Run: %s", user, ToolCommand("base ensure"))
}
