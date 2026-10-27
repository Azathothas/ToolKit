// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// The three ways grants.sh runs.
const (
	// GrantsWrite rewrites the tool-owned fstab block and creates each target,
	// for the restart that follows provisioning.
	GrantsWrite = "write"
	// GrantsLive also unmounts or mounts one target now, as the block names it,
	// and reads it back as the account.
	GrantsLive = "live"
	// GrantsBlock rewrites the tool-owned fstab block alone. It creates no
	// directory and changes no mount. WSL-179.
	GrantsBlock = "block"
	// GrantsHeld changes nothing. It names each live mount the guest's ledger
	// gives another configuration file. WSL-179.
	GrantsHeld = "held"
)

// grantsEnv is what grants.sh is handed for a configuration's grants. In live
// mode grant is the one grant the call makes or takes away: its target, and the
// Windows directory it names.
//
// ⛔ LIVE, ONLY THE GRANT'S TARGET IS UNMOUNTED OR MOUNTED. Every /workspaces
// mount the caller's configuration did not name was unmounted, and another
// caller's configuration, such as a project file or another copy of the home, can
// hold one. That caller's checkout went, and its writes landed in the guest.
// WSL-179.
func grantsEnv(cfg Config, mode string, grant BaseMount) (map[string]string, error) {
	var fstab, checks, target64, source64 string
	var err error
	if mode == GrantsLive {
		clean, targetErr := normalizeBaseMountTarget(grant.Target)
		if targetErr != nil {
			return nil, fmt.Errorf("a live grant change names its one target: %w", targetErr)
		}
		target64 = base64.StdEncoding.EncodeToString([]byte(clean))
		// ⛔ THE SOURCE TOO, SO A REVOKE UNMOUNTS ONLY A MOUNT FROM IT. A revoke
		// with no ledger entry to stop it unmounted whatever was mounted at its
		// target, another file's checkout included. Spelled as the block spells
		// it. WSL-179.
		if grant.Source != "" {
			source64 = base64.StdEncoding.EncodeToString([]byte(fstabEscape(filepath.ToSlash(grant.Source))))
		}
	}
	// ⚠ A TAKE-BACK'S BLOCK IS THE ONE A LIVE CHANGE WRITES, so it renders the
	// same way, with a gone grant left out rather than refused.
	if mode == GrantsWrite {
		fstab, checks, _, err = baseMountPayloads(cfg)
	} else {
		fstab, checks, err = liveMountPayloads(cfg)
	}
	if err != nil {
		return nil, err
	}
	ownerKey, ownerName := grantOwner(cfg)
	return map[string]string{
		"TK_USER": cfg.Base.User, "TK_GRANTS_MODE": mode,
		"TK_FSTAB_B64": fstab, "TK_MOUNT_CHECKS": checks,
		"TK_GRANT_TARGET": target64, "TK_GRANT_SOURCE": source64,
		"TK_GRANT_OWNER": ownerKey, "TK_GRANT_OWNER_NAME": ownerName,
		// WSL-198: the helper WSL's mount -a needs on alpine, written before a block.
		"TK_MOUNT_HELPER_B64": base64.StdEncoding.EncodeToString(mountHelperScript),
	}, nil
}

// grantOwner is how the guest's grant ledger names a configuration file: a key
// compared as Windows compares paths, and the file's own spelling to show. Both are
// base64, and both are empty for a configuration with no file. WSL-179.
func grantOwner(cfg Config) (key, name string) {
	p := ownerFile(cfg)
	if p == "" {
		return "", ""
	}
	k := p
	if runtime.GOOS == "windows" {
		k = strings.ToLower(p)
	}
	enc := base64.StdEncoding.EncodeToString
	return enc([]byte(k)), enc([]byte(p))
}

// ownerFile is the configuration file the grant ledger names a caller by, absolute
// and clean, or an empty string for a configuration with no file. The helper's
// client sends it, so the helper's base names the same file. WSL-179, WSL-190.
func ownerFile(cfg Config) string {
	p := cfg.Path()
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.Clean(p)
}

// applyGrants runs grants.sh in the base for a configuration's grants. grant is
// read in live mode alone.
func (b *Base) applyGrants(ctx context.Context, cfg Config, mode string, grant BaseMount) (string, error) {
	env, err := grantsEnv(cfg, mode, grant)
	if err != nil {
		return "", err
	}
	out := &prefixWriter{prefix: "", to: b.logWriter()}
	code, err := b.wsl.Exec(ctx, ExecRequest{
		Distro: cfg.Base.Name, User: "root", Script: grantsScript,
		Env: env, Timeout: 5 * time.Minute, Stdout: out, Stderr: out,
	})
	out.Flush()
	seen := out.seen.String()
	if err != nil || code != 0 {
		// ⭐ THE SCRIPT'S OWN REASON IS THE ERROR, not its exit code alone. A grant
		// refused because a process is standing in the directory has to say so.
		reason := fmt.Sprintf("exited %d", code)
		for _, line := range strings.Split(seen, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "grants: ") {
				reason = strings.TrimPrefix(strings.TrimSpace(line), "grants: ")
			}
		}
		return seen, fmt.Errorf("the grants step in %s: %s", cfg.Base.Name, reason)
	}
	if !strings.Contains(seen, "grants-complete") {
		return seen, errors.New("the grants step exited 0 without reaching its last line")
	}
	return seen, nil
}

// grantTargetRE is what a default target's last element may carry.
var grantTargetRE = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// DefaultGrantTarget is where a directory is granted when no target is named:
// /workspaces/ and the directory's own name, with anything a target cannot carry
// turned into a dash.
func DefaultGrantTarget(source string) (string, error) {
	name := strings.Trim(grantTargetRE.ReplaceAllString(filepath.Base(filepath.Clean(source)), "-"), "-.")
	if name == "" {
		return "", fmt.Errorf("%s has no name a target can be made from. Pass --target /workspaces/NAME", source)
	}
	return normalizeBaseMountTarget("/workspaces/" + name)
}

// GrantChange is one grant added to or taken from a configuration.
type GrantChange struct {
	// Next is the configuration with the change made.
	Next Config
	// Mount is the grant, resolved: its source absolute and its target and mode
	// normalized.
	Mount BaseMount
	// Unchanged says the configuration already held exactly this grant, or did not
	// hold the one being taken away.
	Unchanged bool
}

// PlanGrant answers the configuration with one directory granted.
//
// ⛔ A TARGET OR A SOURCE ALREADY GRANTED DIFFERENTLY IS REFUSED, not replaced.
// Replacing it would unmount a directory an agent may be working in to mount
// another one at the same path, and `base revoke` is the command that says so.
func PlanGrant(cfg Config, source, target, mode string) (GrantChange, error) {
	abs, err := filepath.Abs(strings.TrimSpace(source))
	if err != nil {
		return GrantChange{}, err
	}
	if target == "" {
		if target, err = DefaultGrantTarget(abs); err != nil {
			return GrantChange{}, err
		}
	}
	mount := BaseMount{Source: abs, Target: target, Mode: mode}
	resolvedNew, err := (Config{Base: BaseConfig{Mounts: []BaseMount{mount}}}).ResolvedBaseMounts()
	if err != nil {
		return GrantChange{}, err
	}
	want := resolvedNew[0]
	if want.Missing {
		return GrantChange{}, fmt.Errorf("%s does not exist, so it cannot be granted", want.Source)
	}
	current, err := cfg.ResolvedBaseMounts()
	if err != nil {
		return GrantChange{}, err
	}
	// ⛔ REFUSED BEFORE ANYTHING CHANGES. The live step mounts every grant and
	// reads each back, and a gone source fails that after this grant is mounted.
	// WSL-106.
	if err := GoneGrantsError(current); err != nil {
		return GrantChange{}, err
	}
	for _, have := range current {
		sameTarget := have.Target == want.Target
		sameSource := sameSourcePath(have.Source, want.Source)
		switch {
		case sameTarget && sameSource && have.Mode == want.Mode:
			return GrantChange{Next: cfg, Mount: want, Unchanged: true}, nil
		case sameTarget:
			return GrantChange{}, fmt.Errorf("%s is already granted from %s, %s. Revoke it first: base revoke --target %s",
				have.Target, have.Source, have.Mode, have.Target)
		case sameSource:
			return GrantChange{}, fmt.Errorf("%s is already granted at %s, %s. Revoke it first: base revoke --target %s",
				have.Source, have.Target, have.Mode, have.Target)
		}
	}
	next := cfg
	next.Base.Mounts = append(append([]BaseMount(nil), cfg.Base.Mounts...), want)
	if err := next.Validate(); err != nil {
		return GrantChange{}, err
	}
	return GrantChange{Next: next, Mount: want}, nil
}

// PlanRevoke answers the configuration with the grant at a target taken away.
func PlanRevoke(cfg Config, target string) (GrantChange, error) {
	clean, err := normalizeBaseMountTarget(target)
	if err != nil {
		return GrantChange{}, err
	}
	current, err := cfg.ResolvedBaseMounts()
	if err != nil {
		return GrantChange{}, err
	}
	next := cfg
	next.Base.Mounts = nil
	var taken *BaseMount
	for i, have := range current {
		if have.Target == clean {
			taken = &current[i]
			continue
		}
		// ⚠ THE STORED SPELLING IS KEPT for every grant left, so a relative source
		// in somebody's project file stays relative.
		next.Base.Mounts = append(next.Base.Mounts, cfg.Base.Mounts[i])
	}
	if taken == nil {
		granted := make([]string, 0, len(current))
		for _, have := range current {
			granted = append(granted, have.Target)
		}
		if len(granted) == 0 {
			return GrantChange{}, fmt.Errorf("%s is not granted, and this base has no grant", clean)
		}
		return GrantChange{}, fmt.Errorf("%s is not granted. The grants are: %s", clean, strings.Join(granted, ", "))
	}
	return GrantChange{Next: next, Mount: *taken}, nil
}

func sameSourcePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// ChangeGrantsLive makes the live mount at grant's target what next names,
// mounted or not, and reads it back. Every other mount stays as it is. The
// configuration file is the caller's to write afterwards. changed is false when
// the target was already as next names it, so nothing was mounted or unmounted.
// WSL-179.
func (b *Base) ChangeGrantsLive(ctx context.Context, next Config, grant BaseMount) (changed bool, err error) {
	out, err := b.applyGrants(ctx, next, GrantsLive, grant)
	if err != nil {
		return false, err
	}
	return liveGrantChanged(out, grant.Target), nil
}

// liveGrantChanged reads whether a live run of grants.sh mounted or unmounted
// target. The script prints "unchanged TARGET" when it did neither.
func liveGrantChanged(out, target string) bool {
	clean, err := normalizeBaseMountTarget(target)
	if err != nil {
		return true
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "unchanged "+clean {
			return false
		}
	}
	return true
}

// grantsTakeBackMode is how a live change the file could not record is taken
// back. ⛔ A CHANGE THAT MOUNTED NOTHING TAKES BACK THE BLOCK ALONE. Its target
// was already live as asked, so another configuration had mounted it, and a
// live take-back by the previous configuration unmounted that mount. WSL-179.
func grantsTakeBackMode(changed bool) string {
	if changed {
		return GrantsLive
	}
	return GrantsBlock
}

// TakeGrantsBack undoes a live change whose configuration file could not be
// written. prev is the configuration the file still holds, grant is the one the
// change made or took away, and changed is what ChangeGrantsLive answered.
func (b *Base) TakeGrantsBack(ctx context.Context, prev Config, grant BaseMount, changed bool) error {
	_, err := b.applyGrants(ctx, prev, grantsTakeBackMode(changed), grant)
	return err
}

// HeldGrant is a live mount the guest's ledger gives another configuration file.
type HeldGrant struct {
	Target string
	File   string
}

// heldGrants asks the guest which live mounts its ledger gives a configuration
// file other than this base's. It reads under the guest's grant lock, so a grant
// still running is read once it has finished. WSL-179.
func (b *Base) heldGrants(ctx context.Context) ([]HeldGrant, error) {
	env, err := grantsEnv(b.cfg, GrantsHeld, BaseMount{})
	if err != nil {
		return nil, err
	}
	out := &boundedBuffer{max: 1 << 20}
	errBuf := &boundedBuffer{max: 64 << 10}
	code, err := b.wsl.Exec(ctx, ExecRequest{
		Distro: b.cfg.Base.Name, User: "root", Script: grantsScript,
		Env: env, Timeout: 5 * time.Minute, Stdout: out, Stderr: errBuf,
	})
	if err != nil || code != 0 {
		return nil, fmt.Errorf("exited %d: %s", code, guestFailure(out.String(), errBuf.String()))
	}
	if !strings.Contains(out.String(), "grants-complete") {
		return nil, errors.New("the grants step exited 0 without reaching its last line")
	}
	return parseHeldGrants(out.String()), nil
}

// parseHeldGrants reads grants.sh's and verify.sh's `held-mount TARGET FILE64` rows.
func parseHeldGrants(out string) []HeldGrant {
	var held []HeldGrant
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[0] != "held-mount" {
			continue
		}
		name, err := base64.StdEncoding.DecodeString(f[2])
		if err != nil {
			continue
		}
		held = append(held, HeldGrant{Target: f[1], File: string(name)})
	}
	return held
}

// HeldGrantsError is the refusal of a restart over live mounts another
// configuration file holds, or nil when there are none. WSL-179.
//
// ⚠ IT SAYS THE RESTART ENDS THAT FILE'S WORK, NOT THAT IT DROPS ITS GRANTS. The
// fstab block keeps each line the ledger gives another file, so a restart mounts
// them again, and the work running in them still ends. WSL-190.
func HeldGrantsError(name string, held []HeldGrant) error {
	if len(held) == 0 {
		return nil
	}
	var each, revoke []string
	for _, h := range held {
		each = append(each, h.Target+", granted by "+h.File)
		revoke = append(revoke, "wsl-toolkit --config "+h.File+" base revoke --target "+h.Target)
	}
	return fmt.Errorf("%s would be re-provisioned and restarted, and the restart ends the work under grants another configuration holds live: %s. Nothing was changed. Revoke each with its file first: %s. Or run base ensure --repair to restart over them",
		name, strings.Join(each, "; "), strings.Join(revoke, "; "))
}

// grantLockPoll is how often a grant or a revoke waiting for another asks again,
// and grantLockWait how long it waits before it refuses. The live step's own
// limit is 5 minutes, so a holder that is working lets go well within it.
var (
	grantLockPoll = 200 * time.Millisecond
	grantLockWait = 10 * time.Minute
)

// LockGrants holds the selected instance's grant lock until the returned release
// runs or this process ends. `base grant` and `base revoke` read the configuration
// file under it, change the base, and write the file before letting it go.
//
// ⛔ TWO GRANTS AT ONCE EACH WROTE A FILE WITHOUT THE OTHER'S GRANT. Each loaded
// the file, mounted its own target, and wrote back what it had loaded plus its
// own, so the last write won and the other grant became an unconfigured live
// mount. WSL-179.
func LockGrants(ctx context.Context, log func(string)) (func(), error) {
	root, err := StateRoot()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "locks")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	name := SelectedInstance.Name
	if name == "" {
		name = DefaultInstance
	}
	lock := filepath.Join(dir, "grants-"+name+".lock")
	deadline := time.Now().Add(grantLockWait)
	said := false
	for {
		release, err := lockExclusive(lock)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, errLockHeld) {
			return nil, fmt.Errorf("the grant lock %s: %w", lock, err)
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("another grant or revoke on instance %s held its lock for %s, so nothing was changed. Run this again once it has finished", name, grantLockWait)
		}
		if !said && log != nil {
			log("waiting for another grant or revoke on instance " + name + " to finish")
			said = true
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for another grant or revoke on instance %s: %w", name, ctx.Err())
		case <-time.After(grantLockPoll):
		}
	}
}

// WriteTo stores the configuration at a path, atomically, in the tool's own
// format. ⚠ Mount sources keep their stored spelling: a relative one stays relative
// to the file it is written into.
func (c Config) WriteTo(path string) error {
	c.Schema = ConfigSchema
	// ⛔ A working tree's jobs are not written into an instance's file. WSL-131.
	if c.own != nil {
		c.Jobs, c.Requires = c.own.jobs, c.own.requires
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0o600)
}
