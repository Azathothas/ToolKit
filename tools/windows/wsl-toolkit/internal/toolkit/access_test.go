package toolkit

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaseAccessDefaultsAreReadOnlyAndInteroperable(t *testing.T) {
	cfg := DefaultConfig()
	automount, err := NormalizeAutomount(cfg.Base.Automount)
	if err != nil {
		t.Fatal(err)
	}
	interop, err := NormalizeBaseInterop(cfg.Base.Interop)
	if err != nil {
		t.Fatal(err)
	}
	toolset, err := NormalizeBaseToolset(cfg.Base.Toolset)
	if err != nil {
		t.Fatal(err)
	}
	if automount != AutomountReadOnly || interop != BaseInteropOn || cfg.Base.Systemd || toolset != BaseToolsetNone {
		t.Fatalf("default access = automount %q, interop %q, systemd %v, toolset %q", automount, interop, cfg.Base.Systemd, toolset)
	}
	if len(cfg.Base.Mounts) != 0 {
		t.Fatalf("default access grants %d explicit host directories", len(cfg.Base.Mounts))
	}
}

func TestUnknownBaseToolsetIsRefused(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Base.Toolset = "whatever-the-provider-installer-needs"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "base.toolset") {
		t.Fatalf("Validate() = %v, want a base.toolset refusal", err)
	}
}

func TestExplicitBaseMountsRequireClosedAmbientAccess(t *testing.T) {
	project := t.TempDir()
	cases := []struct {
		name      string
		automount string
		interop   string
		want      string
	}{
		{name: "drive automount remains", automount: AutomountReadOnly, interop: BaseInteropOff, want: "base.automount"},
		{name: "Windows interop remains", automount: AutomountOff, interop: BaseInteropOn, want: "base.interop"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Base.Automount = tc.automount
			cfg.Base.Interop = tc.interop
			cfg.Base.Mounts = []BaseMount{
				{Source: project, Target: "/workspaces/project"},
			}
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want a %s refusal", err, tc.want)
			}
		})
	}
}

func TestExplicitBaseMountTargetsStayBelowWorkspaces(t *testing.T) {
	project := t.TempDir()
	for _, target := range []string{
		"", "/workspaces", "/tmp/project", "/workspaces/../etc", "/workspaces/project name", "/workspaces/project;touch-pwned",
	} {
		t.Run(strings.ReplaceAll(target, "/", "_"), func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Base.Automount = AutomountOff
			cfg.Base.Interop = BaseInteropOff
			cfg.Base.Mounts = []BaseMount{
				{Source: project, Target: target},
			}
			if err := cfg.Validate(); err == nil {
				t.Fatalf("Validate accepted explicit target %q", target)
			}
		})
	}
}

func TestResolvedBaseMountsAnchorToTheConfigAndCanonicalize(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.path = filepath.Join(project, WorkingConfigName)
	cfg.Base.Automount = AutomountOff
	cfg.Base.Interop = BaseInteropOff
	cfg.Base.Mounts = []BaseMount{
		{Source: ".", Target: "/workspaces/./project", Mode: ""},
	}

	mounts, err := cfg.ResolvedBaseMounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(mounts) != 1 {
		t.Fatalf("resolved %d mounts, want 1", len(mounts))
	}
	wantSource, err := filepath.EvalSymlinks(project)
	if err != nil {
		t.Fatal(err)
	}
	if mounts[0].Source != filepath.Clean(wantSource) || mounts[0].Target != "/workspaces/project" || mounts[0].Mode != BaseMountReadOnly {
		t.Fatalf("resolved mount = %+v", mounts[0])
	}

	cfg.Base.Mounts = append(cfg.Base.Mounts,
		BaseMount{Source: project, Target: "/workspaces/duplicate", Mode: BaseMountReadWrite})
	if _, err := cfg.ResolvedBaseMounts(); err == nil || !strings.Contains(err.Error(), "source") || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("duplicate source resolution = %v", err)
	}
}

func TestExplicitBaseMountSourceRefusesAHostRoot(t *testing.T) {
	root := string(filepath.Separator)
	if volume := filepath.VolumeName(t.TempDir()); volume != "" {
		root = volume + string(filepath.Separator)
	}
	cfg := DefaultConfig()
	cfg.Base.Automount = AutomountOff
	cfg.Base.Interop = BaseInteropOff
	cfg.Base.Mounts = []BaseMount{
		{Source: root, Target: "/workspaces/everything"},
	}
	if _, err := cfg.ResolvedBaseMounts(); err == nil || !strings.Contains(err.Error(), "filesystem root") {
		t.Fatalf("ResolvedBaseMounts() = %v, want a filesystem-root refusal", err)
	}
}

func TestBaseMountPayloadIsEncodedAndEscaped(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project space#")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.path = filepath.Join(project, WorkingConfigName)
	cfg.Base.Automount = AutomountOff
	cfg.Base.Interop = BaseInteropOff
	cfg.Base.Mounts = []BaseMount{
		{Source: ".", Target: "/workspaces/provider", Mode: BaseMountReadWrite},
	}

	fstab64, checks, mounts, err := baseMountPayloads(cfg)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(fstab64)
	if err != nil {
		t.Fatal(err)
	}
	fstab := string(raw)
	if strings.Contains(fstab, "project space#") || !strings.Contains(fstab, `project\040space\043`) {
		t.Fatalf("fstab source was not field-escaped: %q", fstab)
	}
	if !strings.Contains(fstab, " /workspaces/provider drvfs metadata,rw,uid=1000,gid=1000,umask=022,fmask=011,nofail 0 0") {
		t.Fatalf("fstab options are incomplete: %q", fstab)
	}
	parts := strings.Fields(checks)
	if len(parts) != 2 || parts[1] != BaseMountReadWrite {
		t.Fatalf("mount checks = %q", checks)
	}
	target, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil || string(target) != "/workspaces/provider" {
		t.Fatalf("encoded mount target = %q, %v", target, err)
	}
	if len(mounts) != 1 || mounts[0].Source != filepath.Clean(project) {
		t.Fatalf("resolved mounts = %+v", mounts)
	}
}

func TestConfigFingerprintTracksBaseAccess(t *testing.T) {
	base := DefaultConfig()
	changes := map[string]func(*Config){
		"automount": func(c *Config) { c.Base.Automount = AutomountOff },
		"interop":   func(c *Config) { c.Base.Interop = BaseInteropOff },
		"systemd":   func(c *Config) { c.Base.Systemd = true },
		"toolset":   func(c *Config) { c.Base.Toolset = BaseToolsetDeveloper },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			changed := base
			change(&changed)
			if changed.Fingerprint() == base.Fingerprint() {
				t.Fatalf("%s did not change the configuration fingerprint", name)
			}
		})
	}

	project := t.TempDir()
	restricted := base
	restricted.path = filepath.Join(project, WorkingConfigName)
	restricted.Base.Automount = AutomountOff
	restricted.Base.Interop = BaseInteropOff
	withoutMount := restricted.Fingerprint()
	restricted.Base.Mounts = []BaseMount{
		{Source: ".", Target: "/workspaces/project", Mode: BaseMountReadWrite},
	}
	if restricted.Fingerprint() == withoutMount {
		t.Fatal("an explicit host mount did not change the configuration fingerprint")
	}
}
