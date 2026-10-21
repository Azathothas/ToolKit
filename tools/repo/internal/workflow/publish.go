// SPDX-License-Identifier: 0BSD

package workflow

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// bom is U+FEFF, spelt as a rune so the source file carries no byte order mark.
var bom = string(rune(0xFEFF))

// ReadNames reads one name per line, as `gh ... --jq '.assets[].name'` writes
// them. ⚠ A BOM, a carriage return and blank lines are dropped, because a
// PowerShell step writes the first two and a name never carries either.
func ReadNames(r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(r)
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			line = strings.TrimPrefix(line, bom)
			first = false
		}
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out, sc.Err()
}

// Missing is every staged name the release does not report.
//
// ⛔ READ BACK WHAT IS ACTUALLY ON THE RELEASE. Every step before this can
// report success and still leave a release short, which is the whole class
// these workflows keep meeting. An upload loop that exits 0 is a claim; the
// names the release reports are the evidence.
func Missing(staged, reported []string) []string {
	on := map[string]bool{}
	for _, r := range reported {
		on[r] = true
	}
	var out []string
	for _, s := range staged {
		if !on[s] {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// ReadBack is the decision `repo release-readback` prints: nil when every
// staged asset is on the release.
//
// ⛔ NOTHING STAGED IS A FAILURE, NOT A PASS. With no names to look for, every
// name is present, and a read-back that can only agree is the `exits 0 having
// done nothing` row in docs/conventions/forbidden-patterns.md.
func ReadBack(staged, reported []string) error {
	if len(staged) == 0 {
		return fmt.Errorf("nothing is staged, so there is nothing to read back and no proof the release holds anything")
	}
	if missing := Missing(staged, reported); len(missing) > 0 {
		return fmt.Errorf("the release is missing %d asset(s): %s", len(missing), strings.Join(missing, ", "))
	}
	return nil
}

// NightlyPrefix is the only tag the prune may ever delete.
const NightlyPrefix = "herdr-nightly-"

// Release is the part of one GitHub release object the prune reads.
type Release struct {
	Tag         string `json:"tag_name"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	CreatedAt   string `json:"created_at"`
}

// ParseReleases reads `gh api repos/OWNER/NAME/releases` as it answers.
func ParseReleases(body []byte) ([]Release, error) {
	var out []Release
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("the releases answer is not a JSON array of releases: %w", err)
	}
	return out, nil
}

// Deletable refuses any tag that is not a nightly's.
//
// ⛔ NOTHING BUT A herdr-nightly- TAG IS EVER DELETED, checked on the tag the
// delete is about to be handed and not only in the selection above it. This
// workflow holds contents:write on the repository whose wsl-toolkit releases
// consumers download.
func Deletable(tag string) error {
	if !strings.HasPrefix(tag, NightlyPrefix) {
		return fmt.Errorf("refusing to delete %q, which is not a %s tag", tag, NightlyPrefix)
	}
	return nil
}

// Prune is the nightlies older than the newest keep, oldest last.
//
// ⛔ NEWEST BY published_at, NEVER created_at. `gh release create --target SHA`
// makes GitHub date the release from THE TARGET COMMIT, not from the publish.
// Measured on 2026-09-16 on the first nightly: created_at 2026-09-15T12:05:05Z,
// which is cfa3252's own commit date, against published_at 2026-09-16T07:15:16Z,
// nineteen hours later. Sorting by created_at therefore orders these by when
// ToolKit's main last moved, so a nightly published today from an older main
// sorts BELOW one published last week from a newer main, and the prune deletes
// the NEWEST, which is the one the adapter's `nightly` channel resolves. WSL-90.
//
// ⚠ A RELEASE WITH NO published_at IS A DRAFT and sorts as the oldest, as jq's
// sort_by put a null first; a published_at that is present and unreadable is
// refused rather than guessed at.
func Prune(releases []Release, keep int) ([]string, error) {
	if keep < 1 {
		return nil, fmt.Errorf("keep is %d; at least the newest nightly stays, because the adapter's nightly channel resolves it", keep)
	}
	type dated struct {
		tag string
		at  time.Time
	}
	var nightlies []dated
	for _, r := range releases {
		if !r.Prerelease || !strings.HasPrefix(r.Tag, NightlyPrefix) {
			continue
		}
		var at time.Time
		if r.PublishedAt != "" {
			t, err := time.Parse(time.RFC3339, r.PublishedAt)
			if err != nil {
				return nil, fmt.Errorf("%s has published_at %q, which is not a time", r.Tag, r.PublishedAt)
			}
			at = t
		}
		nightlies = append(nightlies, dated{tag: r.Tag, at: at})
	}
	sort.SliceStable(nightlies, func(i, j int) bool { return nightlies[i].at.After(nightlies[j].at) })
	var old []string
	for i := keep; i < len(nightlies); i++ {
		old = append(old, nightlies[i].tag)
	}
	return old, nil
}

// DeleteAll hands each tag to del, after every one of them has passed
// Deletable.
//
// ⛔ ALL ARE CHECKED BEFORE ANY IS DELETED. A refusal halfway down the list
// would leave the run having deleted some releases and failed, which is a
// state nobody asked for and a re-run cannot undo.
func DeleteAll(tags []string, del func(tag string) error) error {
	for _, tag := range tags {
		if err := Deletable(tag); err != nil {
			return err
		}
	}
	for _, tag := range tags {
		if err := del(tag); err != nil {
			return fmt.Errorf("deleting %s: %w", tag, err)
		}
	}
	return nil
}
