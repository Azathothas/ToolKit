// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestANamedInstanceReadsItsOwnConfigurationFirst is WSL-75's order: for a named
// instance its own file comes before a project's, --config still comes before both,
// and with no own file, or no instance, the project's file wins as it did.
func TestANamedInstanceReadsItsOwnConfigurationFirst(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "instances", "base")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	projectFile := filepath.Join(project, WorkingConfigName)
	ownFile := filepath.Join(home, "config.json")
	writeImage := func(path, image string) {
		t.Helper()
		body := `{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit-base","image":"` + image + `"}}`
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeImage(projectFile, "docker.io/library/debian:latest")
	writeImage(ownFile, "docker.io/library/alpine:latest")
	restore := chdir(t, project)
	defer restore()
	previous, previousExplicit := SelectedInstance, ExplicitConfigPath
	t.Cleanup(func() { SelectedInstance, ExplicitConfigPath = previous, previousExplicit })
	t.Setenv("WSL_TOOLKIT_HOME", home)
	SelectedInstance = Instance{Name: "base", Distro: "wsl-toolkit-base", Home: home}
	ExplicitConfigPath = ""

	loads := func(label, wantPath, wantImage string) {
		t.Helper()
		cfg, err := LoadConfig()
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if !sameSourcePath(cfg.Path(), wantPath) || cfg.Base.Image != wantImage {
			t.Fatalf("%s: loaded %s with %s, want %s with %s", label, cfg.Path(), cfg.Base.Image, wantPath, wantImage)
		}
	}
	loads("a named instance with its own file", ownFile, "docker.io/library/alpine:latest")
	src, err := ResolveConfig()
	if err != nil || !strings.Contains(src.From, "instance's own configuration") {
		t.Fatalf("the source does not say why the instance's file won: %+v, %v", src, err)
	}

	ExplicitConfigPath = projectFile
	loads("--config over the instance's own file", projectFile, "docker.io/library/debian:latest")
	ExplicitConfigPath = ""

	if err := os.Remove(ownFile); err != nil {
		t.Fatal(err)
	}
	loads("a named instance with no own file", projectFile, "docker.io/library/debian:latest")
}

// grantBase is a configuration that accepts grants, with its file in a directory.
func grantBase(t *testing.T) Config {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Base.Automount = AutomountOff
	cfg.Base.Interop = BaseInteropOff
	cfg.path = filepath.Join(t.TempDir(), "config.json")
	return cfg
}

// TestAGrantIsRefusedWhereItWouldReplaceAnother holds PlanGrant's refusals and its
// one no-op: a target or a source already granted differently is refused, the same
// grant again changes nothing, and a base that can reach Windows drives is refused
// by the configuration's own rule.
func TestAGrantIsRefusedWhereItWouldReplaceAnother(t *testing.T) {
	cfg := grantBase(t)
	first, second := t.TempDir(), t.TempDir()
	change, err := PlanGrant(cfg, first, "/workspaces/first", "rw")
	if err != nil || change.Unchanged || len(change.Next.Base.Mounts) != 1 {
		t.Fatalf("a first grant answered %+v, %v", change, err)
	}
	cfg = change.Next
	if again, err := PlanGrant(cfg, first, "/workspaces/first", "rw"); err != nil || !again.Unchanged {
		t.Fatalf("the same grant again answered %+v, %v", again, err)
	}
	for label, attempt := range map[string]func() error{
		"another source at a granted target": func() error { _, err := PlanGrant(cfg, second, "/workspaces/first", "rw"); return err },
		"a granted source at another target": func() error { _, err := PlanGrant(cfg, first, "/workspaces/other", "rw"); return err },
		"a granted grant in another mode":    func() error { _, err := PlanGrant(cfg, first, "/workspaces/first", "ro"); return err },
	} {
		if err := attempt(); err == nil || !strings.Contains(err.Error(), "Revoke it first") {
			t.Errorf("%s answered %v, want a refusal naming base revoke", label, err)
		}
	}
	open := grantBase(t)
	open.Base.Automount = AutomountReadOnly
	if _, err := PlanGrant(open, first, "/workspaces/first", "rw"); err == nil || !strings.Contains(err.Error(), "automount") {
		t.Errorf("a grant into a base that mounts every drive answered %v", err)
	}
}

// TestARevokeTakesOneGrantAndKeepsTheRestAsWritten holds PlanRevoke: the named grant
// goes, a relative source left in the file keeps its spelling, and a target that
// is not granted is refused with the ones that are.
func TestARevokeTakesOneGrantAndKeepsTheRestAsWritten(t *testing.T) {
	cfg := grantBase(t)
	dir := filepath.Dir(cfg.path)
	if err := os.MkdirAll(filepath.Join(dir, "kept"), 0o755); err != nil {
		t.Fatal(err)
	}
	taken := t.TempDir()
	cfg.Base.Mounts = []BaseMount{
		{Source: "kept", Target: "/workspaces/kept", Mode: "ro"},
		{Source: taken, Target: "/workspaces/taken", Mode: "rw"},
	}
	change, err := PlanRevoke(cfg, "/workspaces/taken")
	if err != nil {
		t.Fatal(err)
	}
	if len(change.Next.Base.Mounts) != 1 || change.Next.Base.Mounts[0].Source != "kept" {
		t.Fatalf("revoking one grant left %+v", change.Next.Base.Mounts)
	}
	// ⚠ Against the resolved path: a temporary directory can be named through an 8.3
	// short form, and a grant's source is resolved to the long one.
	resolvedTaken, err := filepath.EvalSymlinks(taken)
	if err != nil {
		t.Fatal(err)
	}
	if !sameSourcePath(change.Mount.Source, resolvedTaken) || change.Mount.Mode != "rw" {
		t.Fatalf("the revoked grant was reported as %+v", change.Mount)
	}
	if _, err := PlanRevoke(cfg, "/workspaces/never"); err == nil || !strings.Contains(err.Error(), "/workspaces/kept, /workspaces/taken") {
		t.Fatalf("revoking a target never granted answered %v", err)
	}
}

// TestALiveGrantChangesTheBlockOnlyWhenNothingCanStillRefuse holds the order
// grants.sh keeps live, measured wrong on 2026-09-14: the removals come from the live
// mounts and run before the block is written, a failure after it puts the old block
// back, and write mode, which provisioning uses, writes the block and mounts nothing.
func TestALiveGrantChangesTheBlockOnlyWhenNothingCanStillRefuse(t *testing.T) {
	script := string(grantsScript)
	removal := strings.Index(script, `umount "$old"`)
	write := strings.Index(script, "if [ \"$TK_GRANTS_MODE\" = write ]; then\n  install_fstab \"$WORK/fstab\"\n")
	live := strings.LastIndex(script, "\ninstall_fstab \"$WORK/fstab\"\n")
	if removal < 0 || write < 0 || live < 0 || live < removal {
		t.Fatalf("the live block is not written after the removals: removal at %d, write-mode block at %d, live block at %d", removal, write, live)
	}
	if !strings.Contains(script, "done < \"$WORK/live-targets\"") || strings.Contains(script, "old-targets") {
		t.Fatal("the removals are not read from the live mounts under /workspaces")
	}
	for _, refusal := range []string{`fail_back "$old is still in use`, `fail_back "$target could not be mounted`, `install_fstab "$WORK/fstab.before"`} {
		if !strings.Contains(script, refusal) {
			t.Errorf("grants.sh does not carry %s", refusal)
		}
	}
}

// TestADefaultTargetIsTheDirectorysOwnName holds the name a grant gets when none is
// passed: the directory's last element, with anything a target cannot carry turned
// into a dash.
func TestADefaultTargetIsTheDirectorysOwnName(t *testing.T) {
	for source, want := range map[string]string{
		filepath.Join("x", "ToolKit"):     "/workspaces/ToolKit",
		filepath.Join("x", "my project"):  "/workspaces/my-project",
		filepath.Join("x", "cafe & bar"):  "/workspaces/cafe-bar",
		filepath.Join("x", "repo.v2_old"): "/workspaces/repo.v2_old",
	} {
		if got, err := DefaultGrantTarget(source); err != nil || got != want {
			t.Errorf("%q answered %q (%v), want %q", source, got, err, want)
		}
	}
}
