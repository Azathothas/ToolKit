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
)

// grantsEnv is what grants.sh is handed for a configuration's grants. In live
// mode target is the one grant the call makes or takes away.
//
// ⛔ LIVE, ONLY target IS UNMOUNTED OR MOUNTED. Every /workspaces mount the
// caller's configuration did not name was unmounted, and another caller's
// configuration, such as a project file or another copy of the home, can hold
// one. That caller's checkout went, and its writes landed in the guest. WSL-179.
func grantsEnv(cfg Config, mode, target string) (map[string]string, error) {
	var fstab, checks, target64 string
	var err error
	if mode == GrantsLive {
		clean, targetErr := normalizeBaseMountTarget(target)
		if targetErr != nil {
			return nil, fmt.Errorf("a live grant change names its one target: %w", targetErr)
		}
		target64 = base64.StdEncoding.EncodeToString([]byte(clean))
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
	return map[string]string{
		"TK_USER": cfg.Base.User, "TK_GRANTS_MODE": mode,
		"TK_FSTAB_B64": fstab, "TK_MOUNT_CHECKS": checks,
		"TK_GRANT_TARGET": target64,
	}, nil
}

// applyGrants runs grants.sh in the base for a configuration's grants. target is
// read in live mode alone.
func (b *Base) applyGrants(ctx context.Context, cfg Config, mode, target string) (string, error) {
	env, err := grantsEnv(cfg, mode, target)
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

// ChangeGrantsLive makes the live mount at target what next names, mounted or
// not, and reads it back. Every other mount stays as it is. The configuration
// file is the caller's to write afterwards. changed is false when target was
// already as next names it, so nothing was mounted or unmounted. WSL-179.
func (b *Base) ChangeGrantsLive(ctx context.Context, next Config, target string) (changed bool, err error) {
	out, err := b.applyGrants(ctx, next, GrantsLive, target)
	if err != nil {
		return false, err
	}
	return liveGrantChanged(out, target), nil
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
// written. prev is the configuration the file still holds, and changed is what
// ChangeGrantsLive answered.
func (b *Base) TakeGrantsBack(ctx context.Context, prev Config, target string, changed bool) error {
	_, err := b.applyGrants(ctx, prev, grantsTakeBackMode(changed), target)
	return err
}

// WriteTo stores the configuration at a path, atomically, in the tool's own
// format. ⚠ Mount sources keep their stored spelling: a relative one stays relative
// to the file it is written into.
func (c Config) WriteTo(path string) error {
	c.Schema = ConfigSchema
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'), 0o600)
}
