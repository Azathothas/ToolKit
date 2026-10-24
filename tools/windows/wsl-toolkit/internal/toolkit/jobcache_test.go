// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestACacheIsNamedAndMountedWhereAJobCanUseIt is WSL-122's spelling: a name
// `[a-z0-9][a-z0-9_.-]*` of at most 63, then an absolute clean path that is not
// one of the job's own or a kernel filesystem.
func TestACacheIsNamedAndMountedWhereAJobCanUseIt(t *testing.T) {
	for spec, want := range map[string]JobCache{
		"cargo:/usr/local/cargo":  {Name: "cargo", Path: "/usr/local/cargo"},
		"pip.3-11_x:/root/.cache": {Name: "pip.3-11_x", Path: "/root/.cache"},
		"t:/work/target":          {Name: "t", Path: "/work/target"},
	} {
		got, err := ParseCache(spec)
		if err != nil || got != want {
			t.Errorf("ParseCache(%q) = %+v, %v; want %+v", spec, got, err, want)
		}
	}
	refused := map[string]string{
		"cargo":                         "not NAME:/path",
		"Cargo:/c":                      "lower-case",
		"-x:/c":                         "lower-case",
		strings.Repeat("a", 64) + ":/c": "1 to 63",
		"c:relative":                    "not an absolute path",
		"c:/":                           "not an absolute path",
		"c:/a/../b":                     "clean form",
		"c:/a b":                        "does not hold",
		"c:/a:ro":                       "does not hold",
		"c:/work":                       "/work is the job's own",
		"c:/out":                        "/out is the job's own",
		"c:/in/x":                       "/in is the job's own",
		"c:/job.sh":                     "/job.sh is the job's own",
		"c:/proc/x":                     "kernel filesystem",
		"c:/dev":                        "kernel filesystem",
	}
	for spec, want := range refused {
		if _, err := ParseCache(spec); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseCache(%q) = %v, want a refusal naming %q", spec, err, want)
		}
	}
	if _, err := ParseCaches([]string{"a:/x", "b:/x"}, "--cache"); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("two caches at one path answered %v", err)
	}
	if got, err := ParseCaches([]string{"a:/x", "a:/y"}, "--cache"); err != nil || len(got) != 2 {
		t.Errorf("one cache at two paths answered %v, %v", got, err)
	}
}

// TestACacheIsMadeWithTheLabelAndMountedForTheUser holds the engine's half: the
// volume is made with this tool's label before the engine runs, once however
// many paths name it, and is re-owned for --user as the job's own mounts are.
func TestACacheIsMadeWithTheLabelAndMountedForTheUser(t *testing.T) {
	runner := &Runner{}
	spec := JobSpec{
		Image: "docker.io/library/alpine:latest", ContainerLifecycle: ContainerEphemeral,
		Caches: []JobCache{
			{Name: "cargo", Path: "/usr/local/cargo"},
			{Name: "cargo", Path: "/root/.cargo"},
		},
	}
	script := string(runner.containerScript(spec, "wtk-one", jobLayout("/home/toolkit", "one"), "marker"))
	create := "podman volume create --label 'wsl-toolkit.owner=wsl-toolkit' --label 'wsl-toolkit.cache=cargo' 'wtk-cache-cargo'"
	if strings.Count(script, create) != 1 {
		t.Errorf("the volume is not made once with this tool's label:\n%s", script)
	}
	if made, engine := strings.Index(script, create), strings.Index(script, "exec podman"); made < 0 || made > engine {
		t.Errorf("the volume is not made before the engine starts:\n%s", script)
	}
	for _, want := range []string{"'--volume' 'wtk-cache-cargo:/usr/local/cargo:Z'", "'--volume' 'wtk-cache-cargo:/root/.cargo:Z'"} {
		if !strings.Contains(script, want) {
			t.Errorf("the podman command does not carry %s:\n%s", want, script)
		}
	}
	spec.User = "1000:1000"
	owned := string(runner.containerScript(spec, "wtk-two", jobLayout("/home/toolkit", "two"), "marker"))
	if !strings.Contains(owned, "'--volume' 'wtk-cache-cargo:/usr/local/cargo:U,Z'") {
		t.Errorf("a cache is not re-owned for --user:\n%s", owned)
	}
	plain := string(runner.containerScript(JobSpec{Image: "docker.io/library/alpine:latest", ContainerLifecycle: ContainerEphemeral},
		"wtk-three", jobLayout("/home/toolkit", "three"), "marker"))
	if strings.Contains(plain, "volume create") || strings.Contains(plain, CacheVolumePrefix) {
		t.Errorf("a job with no cache makes one:\n%s", plain)
	}
}

// TestTheCacheCheckMakesAMissingVolumeInARealShell runs the generated lines
// against a stand-in engine: a missing volume is made, one that exists is
// left, and one that cannot be made is a refusal that names it.
func TestTheCacheCheckMakesAMissingVolumeInARealShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the check is POSIX shell, and this host has none to run it in")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	// ⚠ With `race` present, the create fails because another job made the
	// volume a moment before, which then exists.
	fake := "#!/bin/sh\necho \"$*\" >> " + shellQuote(log) + "\ncase \"$1 $2\" in\n" +
		"  'volume exists') [ -e " + shellQuote(filepath.Join(dir, "exists")) + " ] ;;\n" +
		"  'volume create') if [ -e " + shellQuote(filepath.Join(dir, "race")) + " ]; then : > " + shellQuote(filepath.Join(dir, "exists")) +
		"; echo 'already exists' >&2; exit 1; fi\n" +
		"    [ ! -e " + shellQuote(filepath.Join(dir, "refuse")) + " ] || { echo 'no space' >&2; exit 1; } ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "podman"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func() (int, string, string) {
		_ = os.Remove(log)
		caches := []JobCache{
			{Name: "pip", Path: "/c"},
		}
		cmd := exec.Command(sh, "-c", cacheCreateScript(caches)+"exit 0\n")
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
		out, _ := cmd.CombinedOutput()
		calls, _ := os.ReadFile(log)
		return cmd.ProcessState.ExitCode(), string(out), string(calls)
	}
	if code, out, calls := run(); code != 0 || !strings.Contains(calls, "volume create --label wsl-toolkit.owner=wsl-toolkit --label wsl-toolkit.cache=pip wtk-cache-pip") {
		t.Errorf("a missing volume: exit %d, %s, calls:\n%s", code, out, calls)
	}
	if err := os.WriteFile(filepath.Join(dir, "exists"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, calls := run(); code != 0 || strings.Contains(calls, "create") {
		t.Errorf("a volume that exists was made again: exit %d, calls:\n%s", code, calls)
	}
	if err := os.Remove(filepath.Join(dir, "exists")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "refuse"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run(); code != 125 || !strings.Contains(out, "--cache pip") || !strings.Contains(out, "no space") {
		t.Errorf("a volume that cannot be made: exit %d, %q", code, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "race"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, out, _ := run(); code != 0 {
		t.Errorf("a volume another job made first was refused: exit %d, %q", code, out)
	}
}

// TestACacheWithNoLabelIsStillListed runs the listing against a stand-in
// engine. A volume podman made itself, after `gc --caches --apply` removed the
// one a job had just made, carries no label, and it is still a cache. WSL-122.
func TestACacheWithNoLabelIsStillListed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the listing is POSIX shell, and this host has none to run it in")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	dir := t.TempDir()
	fake := "#!/bin/sh\ncase \"$1 $2\" in\n" +
		"  'volume ls') case \"$*\" in *--filter*) echo wtk-cache-labelled ;; *) printf 'wtk-cache-labelled\\nwtk-cache-bare\\nsomebody\\n' ;; esac ;;\n" +
		"  'volume inspect') echo " + shellQuote(dir) + " ;;\n" +
		"  'unshare du') shift; exec \"$@\" ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "podman"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, "-c", cacheListScript())
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("the listing failed: %v\n%s", err, out)
	}
	var names []string
	for _, c := range parseCacheVolumes(string(out)) {
		names = append(names, c.Volume)
	}
	if strings.Join(names, " ") != "wtk-cache-labelled wtk-cache-bare" {
		t.Errorf("the listing read %v, want both caches and nothing else:\n%s", names, out)
	}
}

// TestCacheVolumesAreReadFromTheirLines reads the guest's listing, and keeps
// nothing that is not a cache's own volume.
func TestCacheVolumesAreReadFromTheirLines(t *testing.T) {
	out := "cache\twtk-cache-cargo\t1032\t\twtk-0123456789abcdef \n" +
		"cache\twtk-cache-pip\t-1\twtk-aaaa wtk-bbbb \twtk-aaaa wtk-bbbb \r\n" +
		"cache\tsomebody-else\t4\t\t\n" +
		"caches-listed\n"
	got := parseCacheVolumes(out)
	if len(got) != 2 {
		t.Fatalf("read %+v, want the two caches", got)
	}
	if got[0].Name != "cargo" || got[0].Bytes != 1032*1024 || len(got[0].Running) != 0 || strings.Join(got[0].Containers, " ") != "wtk-0123456789abcdef" {
		t.Errorf("the first cache read as %+v", got[0])
	}
	if got[1].Bytes != -1 || len(got[1].Running) != 2 {
		t.Errorf("an unmeasured cache read as %+v", got[1])
	}
}

// TestACacheAContainerNamesIsKept: podman refuses to remove such a volume, so
// the plan keeps it and names the container, and gc --apply does not fail.
func TestACacheAContainerNamesIsKept(t *testing.T) {
	remove, kept := planCaches([]CacheVolume{
		{Name: "free", Volume: "wtk-cache-free"},
		{Name: "held", Volume: "wtk-cache-held", Containers: []string{"wtk-0123456789abcdef"}},
	})
	if strings.Join(remove, " ") != "wtk-cache-free" {
		t.Errorf("the plan removes %v", remove)
	}
	if len(kept) != 1 || !strings.Contains(kept[0], "wtk-cache-held") || !strings.Contains(kept[0], "wtk-0123456789abcdef") {
		t.Errorf("the plan keeps %v", kept)
	}
}

// TestACapabilityIsReadInPodmansSpelling is WSL-130's --cap-add: any case, with
// or without CAP_, or ALL, and nothing the kernel does not name.
func TestACapabilityIsReadInPodmansSpelling(t *testing.T) {
	for in, want := range map[string]string{"net_admin": "CAP_NET_ADMIN", "CAP_SYS_PTRACE": "CAP_SYS_PTRACE", " all ": "ALL"} {
		if got, err := ParseCapability(in); err != nil || got != want {
			t.Errorf("ParseCapability(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"NET_WIZARD", "", "CAP_", "net admin"} {
		if _, err := ParseCapability(bad); err == nil {
			t.Errorf("--cap-add %q was accepted", bad)
		}
	}
	runner := &Runner{}
	script := string(runner.containerScript(JobSpec{
		Image: "docker.io/library/alpine:latest", ContainerLifecycle: ContainerEphemeral, CapAdd: []string{"CAP_NET_ADMIN"},
	}, "wtk-one", jobLayout("/home/toolkit", "one"), "marker"))
	if !strings.Contains(script, "'--cap-add' 'CAP_NET_ADMIN'") {
		t.Errorf("the capability does not reach podman:\n%s", script)
	}
	plain := string(runner.containerScript(JobSpec{Image: "docker.io/library/alpine:latest", ContainerLifecycle: ContainerEphemeral},
		"wtk-two", jobLayout("/home/toolkit", "two"), "marker"))
	if strings.Contains(plain, "--cap-add") {
		t.Errorf("a job that named no capability is given one:\n%s", plain)
	}
}

// TestCachesAndCapabilitiesCrossTheHelper: the helper parses both again, holding
// them to the rules the direct path applies.
func TestCachesAndCapabilitiesCrossTheHelper(t *testing.T) {
	caches, caps, err := HelperRunRequest{Caches: []string{"cargo:/c"}, CapAdd: []string{"net_admin"}}.jobOptions()
	if err != nil || len(caches) != 1 || caches[0].Volume() != "wtk-cache-cargo" || len(caps) != 1 || caps[0] != "CAP_NET_ADMIN" {
		t.Errorf("the helper read %+v and %v, %v", caches, caps, err)
	}
	if _, _, err := (HelperRunRequest{Caches: []string{"cargo:/work"}}).jobOptions(); err == nil {
		t.Error("the helper took a cache over /work")
	}
	if _, _, err := (HelperRunRequest{CapAdd: []string{"NET_WIZARD"}}).jobOptions(); err == nil {
		t.Error("the helper took a capability the kernel does not name")
	}
}
