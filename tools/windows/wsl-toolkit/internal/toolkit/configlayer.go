// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// How a configuration file is laid over what is beneath it. LoadConfig reads one
// file over the compiled-in defaults; a named instance's own file can then have a
// working tree's file laid over its job defaults.

// jobKeys is every jobs key: the defaults a job takes where its command line
// names nothing. A file lays each one it writes over what is beneath it, and a
// working tree's file lends each one to a named instance. WSL-122, WSL-131.
//
// ⛔ EVERY KEY CROSSES, ruling 34. 7.1 lent a named instance only the keys it
// added, and kept container_lifecycle, platform, workspace, exclude and
// gitignore as the instance's own file said. A case walks JobConfig, so a key
// added later must be listed here and must reach the instance.
var jobKeys = []string{
	"container_lifecycle", "platform", "workspace", "exclude", "gitignore",
	"image", "timeout", "env", "env_file", "user", "max_bytes", "max_entries", "cache",
	"max_running", "max_running_wait",
}

// lentKeys reads which jobs keys a file writes, and whether it writes requires.
// A file that does not parse as an object writes none.
//
// ⚠ The decoder matches a key in any case, so the keys are compared in lower
// case. A key written as null is one the decoder leaves alone.
func lentKeys(b []byte) (jobs map[string]bool, requires bool) {
	return writtenKeys(b, jobKeys)
}

// writtenKeys reads which of keys a file writes under jobs, and whether it
// writes requires.
func writtenKeys(b []byte, keys []string) (jobs map[string]bool, requires bool) {
	jobs = map[string]bool{}
	var top map[string]json.RawMessage
	if json.Unmarshal(b, &top) != nil {
		return jobs, false
	}
	set := func(v json.RawMessage) bool { return strings.TrimSpace(string(v)) != "null" }
	for key, raw := range top {
		switch strings.ToLower(key) {
		case "requires":
			requires = requires || set(raw)
		case "jobs":
			var inner map[string]json.RawMessage
			if json.Unmarshal(raw, &inner) != nil {
				continue
			}
			for k, v := range inner {
				for _, lent := range keys {
					if strings.ToLower(k) == lent && set(v) {
						jobs[lent] = true
					}
				}
			}
		}
	}
	return jobs, requires
}

// overlayJobDefaults lays the jobs keys a file writes over what is beneath them,
// key by key. written is lentKeys' answer for that file.
//
// ⛔ WHAT THE FILE WRITES, NOT WHAT IS NONZERO. A project that writes gitignore
// false, exclude [] or max_running 0 states a value, and the instance's own
// gives way to it. A key the file leaves out keeps what is beneath. ⚠ An empty
// string is not set, as validateJobDefaults reads it, so it keeps what is
// beneath too.
//
// ⛔ ONE COPY FOR BOTH LAYERS. The file the search resolved and a working tree's
// file lending to a named instance go through this one list, so a key one of
// them carries cannot be dropped by the other. A case sets every field in a
// lending file and reads it back. WSL-131.
func overlayJobDefaults(dst *JobConfig, stored JobConfig, written map[string]bool) {
	text := func(key, v string, to *string) {
		if written[key] && v != "" {
			*to = v
		}
	}
	text("container_lifecycle", stored.ContainerLifecycle, &dst.ContainerLifecycle)
	text("platform", stored.Platform, &dst.Platform)
	text("workspace", stored.Workspace, &dst.Workspace)
	if written["exclude"] {
		dst.Exclude = stored.Exclude
	}
	if written["gitignore"] {
		dst.Gitignore = stored.Gitignore
	}
	text("image", stored.Image, &dst.Image)
	text("timeout", stored.Timeout, &dst.Timeout)
	if written["env"] {
		dst.Env = stored.Env
	}
	text("env_file", stored.EnvFile, &dst.EnvFile)
	text("user", stored.User, &dst.User)
	if written["max_bytes"] {
		dst.MaxBytes = stored.MaxBytes
	}
	if written["max_entries"] {
		dst.MaxEntries = stored.MaxEntries
	}
	if written["cache"] {
		dst.Cache = stored.Cache
	}
	// WSL-136: the running-jobs cap is a project's to set, as the rest are.
	if written["max_running"] {
		dst.MaxRunning = stored.MaxRunning
	}
	text("max_running_wait", stored.MaxRunningWait, &dst.MaxRunningWait)
}

// decodeStored reads one configuration file strictly.
func decodeStored(path string, b []byte) (Config, error) {
	var stored Config
	if err := checkFloor(path, b); err != nil {
		return stored, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&stored); err != nil {
		return stored, unreadable(path, err)
	}
	if stored.Schema != ConfigSchema {
		return stored, fmt.Errorf("%s declares schema %q and this build reads %q", path, stored.Schema, ConfigSchema)
	}
	return stored, nil
}

// -- WSL-129: a version floor --------------------------------------------------

// ErrVersionFloor is a configuration whose `requires` names a newer build than
// this one. Every command that reads it refuses with exit 2. WSL-129.
var ErrVersionFloor = errors.New("a newer wsl-toolkit is required")

// parseFloor reads a `requires` value: a version of this tool, such as 7.1.0. A
// leading v is accepted, as a release tag spells it.
func parseFloor(v string) (string, error) {
	s := strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(s, ".")
	if s == "" || len(parts) > 3 {
		return "", fmt.Errorf("requires %q is not a version of this tool, such as 7.1.0", v)
	}
	for _, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return "", fmt.Errorf("requires %q is not a version of this tool, such as 7.1.0", v)
		}
	}
	return s, nil
}

// checkFloor refuses a file whose `requires` names a build newer than this one.
//
// ⛔ THE FLOOR IS READ BEFORE THE STRICT DECODE. A file a newer build wrote can
// carry a key this build has never heard of, and the decode would refuse it over
// that key. The floor is the true reason, and it names the remedy. ⚠ The remedy
// carries no --instance: one executable serves every instance.
func checkFloor(path string, b []byte) error {
	var probe struct {
		Requires json.RawMessage `json:"requires"`
	}
	if json.Unmarshal(b, &probe) != nil || len(probe.Requires) == 0 || string(probe.Requires) == "null" {
		return nil
	}
	var floor string
	if err := json.Unmarshal(probe.Requires, &floor); err != nil {
		return fmt.Errorf("%s: requires is %s, and it must be a version of this tool, such as \"7.1.0\"", path, probe.Requires)
	}
	want, err := parseFloor(floor)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if CompareVersions(Version, want) < 0 {
		return fmt.Errorf("%w: %s requires wsl-toolkit %s or newer, and this build is %s. Update it with: %s",
			ErrVersionFloor, path, want, Version, ToolCommandFor(DefaultInstance, "selfupdate"))
	}
	return nil
}

// higherFloor is the later of two floors, either of which may be empty.
func higherFloor(a, b string) string {
	pa, errA := parseFloor(a)
	pb, errB := parseFloor(b)
	switch {
	case errA != nil:
		return b
	case errB != nil:
		return a
	case CompareVersions(pb, pa) > 0:
		return b
	}
	return a
}

// unreadable names a file the strict decode refused.
//
// ⭐ AN UNKNOWN KEY NAMES THE LIKELY CAUSE AND THE REMEDY. Every build refuses a
// key it does not know, so a file a newer build wrote is refused by every older
// one, and the decoder's own words say only that the key is unknown. WSL-129.
func unreadable(path string, err error) error {
	readable := fmt.Errorf("%s is not readable as %s: %w", path, ConfigSchema, err)
	if !strings.HasPrefix(err.Error(), "json: unknown field ") {
		return readable
	}
	return fmt.Errorf("%w. A newer build of wsl-toolkit may have written it, and this build is %s. Update it with: %s",
		readable, Version, ToolCommandFor(DefaultInstance, "selfupdate"))
}

// -- WSL-131: a working tree's job defaults for a named instance --------------

// ownLayer is what a named instance's own file says about jobs, kept apart from
// what a working tree contributed, so a write stores the instance's own.
type ownLayer struct {
	jobs     JobConfig
	requires string
}

// lendingProjectConfig is the working tree's wsl-toolkit.json nearest the
// working directory where that file writes a jobs key or requires, and empty
// otherwise. WSL-131.
//
// ⛔ A FILE THAT WRITES NEITHER IS NOT READ, as before this release. A named
// instance with its own file ignored the working tree's file whole, so one that
// does not parse, or carries a key this build does not know, refused nothing.
// It still refuses nothing. Only a file that lends is held strictly.
func lendingProjectConfig() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, WorkingConfigName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			b, err := os.ReadFile(candidate)
			if err != nil {
				return ""
			}
			if jobs, requires := lentKeys(b); len(jobs) == 0 && !requires {
				return ""
			}
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// contribute lays a working tree's file over a named instance's configuration:
// its job defaults, key by key, and its requires. WSL-131.
//
// ⭐ ONE BASE SERVES EVERY PROJECT, AND A PROJECT STILL STATES ITS JOBS. The
// instance's own file says what the base is, which WSL-75 settled. Before this,
// a project's image, timeout and environment were ignored under a named
// instance, so every wrapper passed them on each call.
//
// ⛔ ITS BASE IS NOT READ. The instance's own file says what the base is. Every
// jobs key it writes is lent, ruling 34, and jobKeys lists them.
func (c *Config) contribute(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	stored, err := decodeStored(path, b)
	if err != nil {
		return err
	}
	c.own = &ownLayer{jobs: c.Jobs, requires: c.Requires}
	written, requires := lentKeys(b)
	overlayJobDefaults(&c.Jobs, stored.Jobs, written)
	c.Requires = higherFloor(c.Requires, stored.Requires)
	c.project = path
	c.projectKeys = written
	c.projectLends = nil
	for _, k := range jobKeys {
		if written[k] {
			c.projectLends = append(c.projectLends, "jobs."+k)
		}
	}
	if requires {
		c.projectLends = append(c.projectLends, "requires")
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("%s, whose job defaults this instance takes: %w", path, err)
	}
	return nil
}

// Portable is this configuration as another process reads it: each grant's
// source resolved against the file that names it.
//
// ⛔ THE FILE'S PATH DOES NOT CROSS THE HELPER PROTOCOL, so a relative source
// sent as it was written resolved against the helper's own working directory,
// and `base ensure` through the helper mounted the wrong directory or none.
// WSL-131.
func (c Config) Portable() Config {
	resolved, err := c.ResolvedBaseMounts()
	if err != nil || len(resolved) == 0 {
		return c
	}
	for i := range resolved {
		// ⚠ The disk's answer today, which the reader takes again.
		resolved[i].Missing = false
	}
	c.Base.Mounts = resolved
	return c
}

// Project is the working tree's file whose job defaults and requires a named
// instance's configuration took, or empty. WSL-131.
func (c Config) Project() string { return c.project }

// ProjectLends is each key Project lent this instance, as jobs.KEY in jobKeys'
// order and then requires. `config` names them beside the file. WSL-131.
func (c Config) ProjectLends() []string { return c.projectLends }

// JobsDir is the directory a relative path in jobs.KEY resolves against: that of
// the file that named the key. WSL-131.
func (c Config) JobsDir(key string) string {
	if c.project != "" && c.projectKeys[key] {
		return filepath.Dir(c.project)
	}
	return filepath.Dir(c.path)
}

// JobTimeout is jobs.timeout as a duration, and whether it is set.
func (j JobConfig) JobTimeout() (time.Duration, bool) {
	if j.Timeout == "" {
		return 0, false
	}
	d, err := time.ParseDuration(j.Timeout)
	return d, err == nil
}

// validateJobDefaults holds the job defaults a configuration may carry beside
// the lifecycle, the platform and the excludes. An empty value is not set.
// WSL-122, WSL-131.
func validateJobDefaults(j JobConfig, catalog []Image) error {
	if j.Image != "" {
		known := false
		for _, img := range catalog {
			known = known || img.ID == j.Image
		}
		if !known {
			if err := ValidateImageRef(j.Image); err != nil {
				return fmt.Errorf("jobs.image %q is not a catalog id and not a usable reference: %w", j.Image, err)
			}
		}
	}
	if j.Timeout != "" {
		d, err := time.ParseDuration(j.Timeout)
		if err != nil {
			return fmt.Errorf("jobs.timeout %q is not a duration such as 45m: %w", j.Timeout, err)
		}
		if d < 0 {
			return fmt.Errorf("jobs.timeout %q is negative. Write 0 for no deadline, or a positive duration", j.Timeout)
		}
	}
	for name := range j.Env {
		if !ValidEnvName(name) {
			return fmt.Errorf("jobs.env names %q, which is not a usable environment name. A name is letters, digits and underscores, and does not start with a digit", name)
		}
	}
	if j.EnvFile != "" && strings.TrimSpace(j.EnvFile) == "" {
		return errors.New("jobs.env_file is blank. Name a file, or leave the key out")
	}
	if j.User != "" && strings.ContainsAny(j.User, " \t\r\n") {
		return fmt.Errorf("jobs.user %q carries white space, and an account, a uid or uid:gid has none", j.User)
	}
	if j.MaxBytes < 0 {
		return fmt.Errorf("jobs.max_bytes %d is negative. Write a positive number of bytes, or leave the key out", j.MaxBytes)
	}
	if j.MaxEntries < 0 {
		return fmt.Errorf("jobs.max_entries %d is negative. Write a positive number of entries, or leave the key out", j.MaxEntries)
	}
	if _, err := ParseCaches(j.Cache, "jobs.cache"); err != nil {
		return err
	}
	return nil
}
