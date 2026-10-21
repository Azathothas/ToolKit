// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-114 and WSL-115: what gc narrows to, and what it refuses -------------

// TestGCRefusesANarrowingItCannotHonour walks every refusal gc makes before it
// reaches WSL, and requires the refusal's own words.
//
// ⚠ EVERY CASE ALSO PASSES --via-helper, and no helper answers in an empty
// home. A refusal is made before a route is chosen, so the flag changes nothing
// here; a refusal that went missing fails at the absent helper, in milliseconds,
// rather than starting a real base for a dry run.
func TestGCRefusesANarrowingItCannotHonour(t *testing.T) {
	t.Setenv("WSL_TOOLKIT_HOME", t.TempDir())
	ctx := context.Background()
	for _, c := range []struct {
		name string
		args []string
		want string
	}{
		{"a shortened id", []string{"--job", "0b769b3dcc81"}, "is not a job id"},
		{"an id in capitals", []string{"--job", strings.ToUpper(cliJobID)}, "is not a job id"},
		{"a job and a project", []string{"--job", cliJobID, "--workspace", "."}, "Pass one"},
		{"images for one job", []string{"--job", cliJobID, "--images"}, "not one job's or one project's"},
		{"images for one project", []string{"--workspace", ".", "--images"}, "not one job's or one project's"},
		// ⛔ An unset variable in a wrapper cleaned up every project's jobs.
		{"an empty project", []string{"--workspace", ""}, "empty value is refused"},
		{"an empty job", []string{"--job", ""}, "empty value is refused"},
	} {
		code, err := cmdGC(ctx, append(c.args, "--via-helper"))
		if code != exitCannot || err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s answered %d, %v; want %d and %q", c.name, code, err, exitCannot, c.want)
		}
	}
}

// ⛔ An instance with no base still refuses an id nothing in it belongs to, and
// still answers for one it holds. A correctness review found the first exit 0.
func TestAnInstanceWithNoBaseStillRefusesAnUnknownJob(t *testing.T) {
	home := t.TempDir()
	policy := toolkit.CleanupPolicy{Job: cliJobID}
	if err := unregisteredJobRefusal(home, policy); !errors.Is(err, toolkit.ErrUnknownJob) {
		t.Fatalf("an id nothing here belongs to answered %v", err)
	}
	owner, err := toolkit.ClaimJob(home, cliJobID)
	if err != nil {
		t.Fatal(err)
	}
	owner.Release()
	if err := unregisteredJobRefusal(home, policy); err != nil {
		t.Fatalf("an id this instance holds was refused: %v", err)
	}
	if err := unregisteredJobRefusal(home, toolkit.CleanupPolicy{}); err != nil {
		t.Fatalf("a cleanup naming no job was refused: %v", err)
	}
}

// ⛔ A refusal prints no plan: an empty document beside it reads as a cleanup
// that found nothing to do.
func TestARefusedCleanupExitsTwoAndPrintsNoPlan(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("%w: %s", toolkit.ErrUnknownJob, cliJobID),
		toolkit.ErrWorkspaceViaHelper,
	} {
		var code int
		var got error
		out := captureStdout(t, func() {
			code, got = finishGC(toolkit.CleanupPlan{Schema: toolkit.CleanupSchema, DryRun: true}, err, true)
		})
		if code != exitCannot || got == nil || out != "" {
			t.Errorf("%v answered %d, %v, and printed %q", err, code, got, out)
		}
	}
	out := captureStdout(t, func() {
		if code, err := finishGC(toolkit.CleanupPlan{Schema: toolkit.CleanupSchema, DryRun: true}, nil, true); code != exitOK || err != nil {
			t.Errorf("an empty cleanup answered %d, %v", code, err)
		}
	})
	if !strings.Contains(out, toolkit.CleanupSchema) {
		t.Errorf("an empty cleanup printed no plan: %q", out)
	}
}
