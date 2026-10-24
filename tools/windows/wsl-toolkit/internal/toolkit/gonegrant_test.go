// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
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

// ⛔ Two grants gone: the live step of each revoke refused over the other, each
// refusal naming the other's revoke, so neither could be removed. A correctness
// review found it.
func TestEachOfTwoGoneGrantsCanBeRevoked(t *testing.T) {
	cfg, _, kept := goneGrantConfig(t)
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(parent, "GONE2")
	if err := os.Mkdir(second, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg.Base.Mounts = append(cfg.Base.Mounts, BaseMount{Source: second, Target: "/workspaces/GONE2", Mode: "ro"})
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/workspaces/EPHM", "/workspaces/GONE2"} {
		change, err := PlanRevoke(cfg, target)
		if err != nil {
			t.Fatalf("planning the revoke of %s refused: %v", target, err)
		}
		fstab64, checks, err := liveMountPayloads(change.Next)
		if err != nil {
			t.Fatalf("the live step of revoking %s refused: %v", target, err)
		}
		fstab, err := base64.StdEncoding.DecodeString(fstab64)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(checks, "\n")+1 != 1 || strings.Contains(string(fstab), "GONE2") || strings.Contains(string(fstab), "EPHM") ||
			!strings.Contains(string(fstab), "/workspaces/kept") {
			t.Fatalf("revoking %s renders %q and checks %q; want the one grant that exists, %s, and no gone one", target, fstab, checks, kept)
		}
	}
	if _, _, _, err := baseMountPayloads(cfg); !errors.Is(err, ErrGrantSourceGone) {
		t.Fatalf("base ensure's rendering answered %v beside two gone grants; want the refusal", err)
	}
}

// ⛔ A gone grant's mount can outlive its directory until the base restarts. The
// verifier is told of it, allows it and checks nothing in it, and still refuses
// any other Windows mount it was not told of. A correctness review found it.
func TestTheVerifierAllowsAGoneGrantsMountAndNoOther(t *testing.T) {
	cfg, _, _ := goneGrantConfig(t)
	gone, err := goneGrantTargets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if target, err := base64.StdEncoding.DecodeString(gone); err != nil || string(target) != "/workspaces/EPHM" {
		t.Fatalf("the gone targets are %q (%v); want /workspaces/EPHM alone", gone, err)
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell on this host")
	}
	dir := t.TempDir()
	mounts := filepath.Join(dir, "mounts")
	if err := os.WriteFile(mounts, []byte("drvfs /workspaces/EPHM drvfs rw 0 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	block := mustExtract(t, "verify.sh", "mount_count=0", "done < /proc/mounts") + "\nfi\n"
	script := strings.ReplaceAll(block, "/proc/mounts", filepath.ToSlash(mounts))
	run := func(goneVar string) (string, error) {
		cmd := exec.Command(sh, "-c", script)
		cmd.Env = append(os.Environ(), "TK_AUTOMOUNT=off", "TK_MOUNT_CHECKS=", "TK_MOUNT_GONE="+goneVar, "TMPDIR="+filepath.ToSlash(dir))
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run(""); err == nil || !strings.Contains(out, "unconfigured Windows directory is mounted at /workspaces/EPHM") {
		t.Fatalf("a Windows mount nobody named answered %v: %s", err, out)
	}
	if out, err := run(gone); err != nil {
		t.Fatalf("a gone grant's mount was refused: %v: %s", err, out)
	}
}

// ⛔ A grant gone mid-ensure is not a broken base: removing the base to rebuild it
// would lose it, and the rebuild refuses over the same grant. Its refusal names
// the revoke and not the rebuild. ⚠ Since WSL-171 no failed re-provision removes
// the base; the others name `base recreate`.
func TestAGrantGoneWhileEnsureRunsDoesNotCostTheBase(t *testing.T) {
	cfg, _, _ := goneGrantConfig(t)
	_, _, _, err := baseMountPayloads(cfg)
	if got := reprovisionFailed(cfg.Base.Name, err); got != err {
		t.Fatalf("the gone-grant refusal %v was rewritten as %v", err, got)
	}
	other := errors.New("the engine did not start")
	if got := reprovisionFailed(cfg.Base.Name, other); !errors.Is(got, other) || !strings.Contains(got.Error(), "base recreate") {
		t.Fatalf("another failure answered %v; want its own error and the rebuild named", got)
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
