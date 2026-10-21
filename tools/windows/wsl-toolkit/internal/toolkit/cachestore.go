// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// What the base's engine holds as named caches, and their removal. WSL-122.

// CacheVolume is one named cache the base's engine holds.
type CacheVolume struct {
	Name   string `json:"name"`
	Volume string `json:"volume"`
	// Bytes is what it holds, read in the account's user namespace, where a job
	// that ran as another user left files the account cannot read. -1 where it
	// could not be read.
	Bytes int64 `json:"bytes"`
	// Containers are every container that names it, and Running the ones that
	// run now.
	Containers []string `json:"containers,omitempty"`
	Running    []string `json:"running,omitempty"`
}

// cacheVolumes lists every cache volume this tool made.
//
// ⛔ ONLY A VOLUME CARRYING THIS TOOL'S LABEL AND NAME IS ONE. A volume an image
// declared, or one somebody made by hand, is not this tool's to report as held
// or to remove.
func (r *Runner) cacheVolumes(ctx context.Context) ([]CacheVolume, error) {
	script := `podman volume ls --filter label=` + JobLabel + ` --format '{{.Name}}' 2>/dev/null | while IFS= read -r v; do
  case "$v" in ` + CacheVolumePrefix + `*) : ;; *) continue ;; esac
  mp=$(podman volume inspect --format '{{.Mountpoint}}' "$v" 2>/dev/null) || mp=
  kb=
  if [ -n "$mp" ]; then kb=$(podman unshare du -sk "$mp" 2>/dev/null | cut -f1); fi
  [ -n "$kb" ] || kb=-1
  every=$(podman ps -a --filter volume="$v" --format '{{.Names}}' 2>/dev/null | tr '\n' ' ')
  now=$(podman ps --filter volume="$v" --format '{{.Names}}' 2>/dev/null | tr '\n' ' ')
  printf 'cache\t%s\t%s\t%s\t%s\n' "$v" "$kb" "$now" "$every"
done
printf 'caches-listed\n'
`
	out, stderr, code, err := r.baseCapture(ctx, []byte(script), 5*time.Minute)
	if err != nil || code != 0 || !strings.Contains(out, "caches-listed") {
		return nil, fmt.Errorf("the caches could not be listed (exit %d): %s", code, guestFailure(stderr, out))
	}
	return parseCacheVolumes(out), nil
}

// parseCacheVolumes reads cacheVolumes' lines.
func parseCacheVolumes(out string) []CacheVolume {
	var caches []CacheVolume
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) != 5 || f[0] != "cache" || !strings.HasPrefix(f[1], CacheVolumePrefix) {
			continue
		}
		c := CacheVolume{Name: strings.TrimPrefix(f[1], CacheVolumePrefix), Volume: f[1], Bytes: -1}
		if kb, err := strconv.ParseInt(strings.TrimSpace(f[2]), 10, 64); err == nil && kb >= 0 {
			c.Bytes = kb * 1024
		}
		c.Running = strings.Fields(f[3])
		c.Containers = strings.Fields(f[4])
		caches = append(caches, c)
	}
	return caches
}

// planCaches splits the caches into what `gc --caches` removes and what it keeps.
//
// ⛔ A CACHE A CONTAINER NAMES IS KEPT, running or stopped. podman refuses to
// remove such a volume, so a plan that listed it would be a dry run the apply
// then fails, which is the defect cleanup.go exists to prevent.
func planCaches(caches []CacheVolume) (remove []string, kept []string) {
	for _, c := range caches {
		if len(c.Containers) > 0 {
			kept = append(kept, c.Volume+": container "+strings.Join(c.Containers, ", ")+
				" names it. Remove that container first, as "+ToolCommand("gc --job ID --apply")+" does for a job's")
			continue
		}
		remove = append(remove, c.Volume)
	}
	return remove, kept
}

// cleanupCaches is `gc --caches`: the named caches, and nothing else.
//
// ⭐ IT TOUCHES NO JOB. A caller dropping a cache whose toolchain went stale has
// not asked for every retained job to go with it, so the job store is not
// enumerated here at all. WSL-122.
func (r *Runner) cleanupCaches(ctx context.Context, apply bool) (CleanupPlan, error) {
	plan := CleanupPlan{Schema: CleanupSchema, DryRun: !apply}
	caches, err := r.cacheVolumes(ctx)
	if err != nil {
		return plan, err
	}
	plan.Caches, plan.Kept = planCaches(caches)
	if !apply || len(plan.Caches) == 0 {
		return plan, nil
	}
	var b strings.Builder
	for _, name := range plan.Caches {
		q := shellQuote(name)
		b.WriteString("if podman volume rm " + q + " >/dev/null 2>&1; then printf 'removed cache %s\\n' " + q +
			"; else printf 'FAILED cache %s\\n' " + q + "; fi\n")
	}
	b.WriteString("printf 'caches-removed\\n'\n")
	out, stderr, code, err := r.baseCapture(ctx, []byte(b.String()), 5*time.Minute)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "removed "):
			plan.Removed = append(plan.Removed, strings.TrimPrefix(line, "removed "))
		case strings.HasPrefix(line, "FAILED "):
			plan.Failed = append(plan.Failed, strings.TrimPrefix(line, "FAILED "))
		}
	}
	if err != nil || code != 0 || !strings.Contains(out, "caches-removed") {
		return plan, fmt.Errorf("removing the caches in the guest exited %d: %s", code, guestFailure(stderr, out))
	}
	if len(plan.Failed) > 0 {
		return plan, fmt.Errorf("%d cache(s) could not be removed", len(plan.Failed))
	}
	return plan, nil
}
