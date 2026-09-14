// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
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

// The two ways grants.sh runs.
const (
	// GrantsWrite rewrites the tool-owned fstab block and creates each target,
	// for the restart that follows provisioning.
	GrantsWrite = "write"
	// GrantsLive also unmounts what the block no longer names and mounts what it
	// does, now, and reads each grant back as the account.
	GrantsLive = "live"
)

// applyGrants runs grants.sh in the base for a configuration's grants.
func (b *Base) applyGrants(ctx context.Context, cfg Config, mode string) (string, error) {
	fstab, checks, _, err := baseMountPayloads(cfg)
	if err != nil {
		return "", err
	}
	out := &prefixWriter{prefix: "", to: b.logWriter()}
	code, err := b.wsl.Exec(ctx, ExecRequest{
		Distro: cfg.Base.Name, User: "root", Script: grantsScript,
		Env: map[string]string{
			"TK_USER": cfg.Base.User, "TK_GRANTS_MODE": mode,
			"TK_FSTAB_B64": fstab, "TK_MOUNT_CHECKS": checks,
		},
		Timeout: 5 * time.Minute, Stdout: out, Stderr: out,
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
	current, err := cfg.ResolvedBaseMounts()
	if err != nil {
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

// ChangeGrantsLive makes the base's live mounts what next names, and reads them
// back. The configuration file is the caller's to write afterwards.
func (b *Base) ChangeGrantsLive(ctx context.Context, next Config) (string, error) {
	return b.applyGrants(ctx, next, GrantsLive)
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
