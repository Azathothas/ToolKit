// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
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

// ⛔ A refusal prints no plan: an empty document beside it reads as a cleanup
// that found nothing to do.
//
// ⚠ AND A CLEANUP THAT COULD NOT ASK WHETHER THE BASE IS REGISTERED is one. The
// command asked before it chose, and exited 2 with no plan. WSL-200.
func TestARefusedCleanupExitsTwoAndPrintsNoPlan(t *testing.T) {
	for _, err := range []error{
		fmt.Errorf("%w: %s", toolkit.ErrUnknownJob, cliJobID),
		toolkit.ErrWorkspaceViaHelper,
		fmt.Errorf("%w: %w", toolkit.ErrRegistrationUnread, toolkit.ErrWslDenied),
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
