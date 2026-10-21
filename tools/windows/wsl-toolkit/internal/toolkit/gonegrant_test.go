// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// -- WSL-106, issue 37 item 2: a grant whose directory is gone ---------------

// goneGrantConfig is a configuration with one grant whose source existed when it
// was written and has since been removed, beside one that still exists.
func goneGrantConfig(t *testing.T) (cfg Config, gone, kept string) {
	t.Helper()
	cfg = grantBase(t)
	// ⚠ RESOLVED FIRST. A temporary directory can be named through an 8.3 short
	// form, a present source resolves to the long one, and a gone source cannot be
	// resolved at all, so the two spellings would differ for one directory.
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gone = filepath.Join(parent, "EPHM")
	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	kept = t.TempDir()
	cfg.Base.Mounts = []BaseMount{
		{Source: gone, Target: "/workspaces/EPHM", Mode: "rw"},
		{Source: kept, Target: "/workspaces/kept", Mode: "ro"},
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	return cfg, gone, kept
}

// ⛔ The configuration was refused as it loaded, so the one command that removes
// the grant could not run: the operator recreated the folder by hand to revoke it.
func TestAGrantWhoseSourceIsGoneIsKeptAndMarkedRatherThanRefused(t *testing.T) {
	cfg, gone, _ := goneGrantConfig(t)
	mounts, err := cfg.ResolvedBaseMounts()
	if err != nil {
		t.Fatalf("a configuration whose grant lost its directory was refused: %v", err)
	}
	if len(mounts) != 2 || !mounts[0].Missing || mounts[1].Missing {
		t.Fatalf("the grants resolved to %+v; want the first marked missing and the second not", mounts)
	}
	if !sameSourcePath(mounts[0].Source, gone) || mounts[0].Target != "/workspaces/EPHM" || mounts[0].Mode != "rw" {
		t.Fatalf("the gone grant lost its own source, target or mode: %+v", mounts[0])
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validation refused it: %v", err)
	}
}

func TestEverythingThatMountsTheGrantsRefusesAGoneOneByItsRevoke(t *testing.T) {
	cfg, _, _ := goneGrantConfig(t)
	_, _, _, err := baseMountPayloads(cfg)
	if !errors.Is(err, ErrGrantSourceGone) || !strings.Contains(err.Error(), "base revoke --target /workspaces/EPHM") {
		t.Fatalf("rendering the mounts answered %v; want the gone-source refusal naming the revoke", err)
	}
	other := t.TempDir()
	if _, err := PlanGrant(cfg, other, "/workspaces/other", "ro"); !errors.Is(err, ErrGrantSourceGone) {
		t.Fatalf("a new grant beside a gone one answered %v; want the gone-source refusal", err)
	}
	if _, err := PlanGrant(cfg, filepath.Join(other, "never"), "/workspaces/never", "ro"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("granting a directory that does not exist answered %v", err)
	}
	b := &Base{cfg: cfg, log: func(string) {}}
	if _, err := b.EnsureWith(context.Background(), false, false); !errors.Is(err, ErrGrantSourceGone) {
		t.Fatalf("base ensure answered %v; want the gone-source refusal before it reached anything else", err)
	}
}

func TestAGoneGrantCanBeRevokedAndTheRestStillMounts(t *testing.T) {
	cfg, _, kept := goneGrantConfig(t)
	change, err := PlanRevoke(cfg, "/workspaces/EPHM")
	if err != nil {
		t.Fatalf("revoking the gone grant was refused: %v", err)
	}
	if len(change.Next.Base.Mounts) != 1 || change.Next.Base.Mounts[0].Source != kept {
		t.Fatalf("the revoke left %+v", change.Next.Base.Mounts)
	}
	if _, _, _, err := baseMountPayloads(change.Next); err != nil {
		t.Fatalf("the configuration the revoke leaves cannot be mounted: %v", err)
	}
	checks, err := presentGrantChecks(cfg)
	if err != nil || strings.Count(checks, "\n")+1 != 1 || checks == "" {
		t.Fatalf("the verifier's read-back for a base with a gone grant is %q, %v; want the one grant that exists", checks, err)
	}
}

func TestAGoneMarkIsNeitherStoredNorFingerprinted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("WSL_TOOLKIT_HOME", home)
	cfg, gone, _ := goneGrantConfig(t)
	before := cfg.Fingerprint()
	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	if after := cfg.Fingerprint(); after != before {
		t.Fatalf("the fingerprint moved from %s to %s when only the disk changed", before, after)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Write(); err != nil {
		t.Fatalf("the configuration could not be written with a gone grant in it: %v", err)
	}
	stored, err := os.ReadFile(filepath.Join(home, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "missing") || !strings.Contains(string(stored), "/workspaces/EPHM") {
		t.Fatalf("the stored configuration carries the disk's answer, or lost the grant:\n%s", stored)
	}
}
