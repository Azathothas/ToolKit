package toolkit

import (
	"strings"
	"testing"
)

// ⛔ THE DEFAULT IS `on`, AND IT IS LOAD-BEARING. Every base built before this
// setting existed has no `shared_tmpfs` key at all; a default of `off` would
// unmount the directory /etc/resolv.conf resolves into on the next ensure of
// every one of them, and take DNS with it.
func TestTheSharedTmpfsDefaultsToOnSoAnOlderBaseKeepsItsResolver(t *testing.T) {
	got, err := NormalizeSharedTmpfs(DefaultConfig().Base.SharedTmpfs)
	if err != nil {
		t.Fatal(err)
	}
	if got != SharedTmpfsOn {
		t.Fatalf("a configuration that never names base.shared_tmpfs resolves to %q, want %q", got, SharedTmpfsOn)
	}
	if _, err := NormalizeSharedTmpfs("sealed"); err == nil || !strings.Contains(err.Error(), "base.shared_tmpfs") {
		t.Fatalf("NormalizeSharedTmpfs(\"sealed\") = %v, want a refusal naming the setting", err)
	}
}

// ⛔ Closing the shared tmpfs on a base whose Windows drives are mounted costs the
// resolver and seals nothing: the drives are a wider channel than the tmpfs was.
func TestClosingTheSharedTmpfsRequiresTheDrivesToBeOffToo(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Base.SharedTmpfs = SharedTmpfsOff
	cfg.Base.Automount = AutomountReadOnly
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "base.shared_tmpfs") {
		t.Fatalf("Validate() with drives mounted = %v, want a refusal naming base.shared_tmpfs", err)
	}
	cfg.Base.Automount = AutomountOff
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() with the drives off = %v, want it accepted", err)
	}
}

// ⭐ The claim is what makes `base doors` able to refuse a base where the boot
// script did not run. Without it the door is reported and nothing fails.
func TestTheSharedTmpfsDoorIsClaimedOnlyWhenTheSettingIsOff(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Base.Automount = AutomountOff
	cfg.Base.PasswordlessSudo = true
	claims, err := doorClaims(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range claims {
		if c.door == "fs.mnt-wsl-shared" {
			t.Fatal("fs.mnt-wsl-shared is claimed while base.shared_tmpfs is on, which promises a door this tool leaves open")
		}
	}
	cfg.Base.SharedTmpfs = SharedTmpfsOff
	claims, err = doorClaims(cfg)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range claims {
		if c.door == "fs.mnt-wsl-shared" {
			found = true
		}
	}
	if !found {
		t.Fatal("base.shared_tmpfs is off and fs.mnt-wsl-shared is not claimed, so base doors cannot fail over it")
	}
	doors, err := parseDoors(doorsSample(map[string]string{"fs.mnt-wsl-shared": DoorOpen}))
	if err != nil {
		t.Fatal(err)
	}
	_, problems, _ := judgeDoors(doors, claims)
	if len(problems) != 1 || !strings.Contains(problems[0], "fs.mnt-wsl-shared") {
		t.Fatalf("judgeDoors() problems = %v, want one naming fs.mnt-wsl-shared", problems)
	}
}

// ⛔ THE ORDER IS THE WHOLE FIX AND IT IS NOT INTERCHANGEABLE. The resolver is
// written while the shared file is still reachable, and the boot script refreshes
// it BEFORE the unmount. A provisioner that unmounted first would leave every
// base it built unable to resolve a name.
func TestTheProvisionerWritesTheResolverBeforeItClosesTheSharedTmpfs(t *testing.T) {
	script := string(provisionScript)
	resolver := strings.Index(script, "wrote /etc/resolv.conf,")
	boot := strings.Index(script, "installed $SEAL_BOOT")
	if resolver < 0 || boot < 0 {
		t.Fatal("the provisioner no longer writes a resolver and a boot script for a closed shared tmpfs")
	}
	if resolver > boot {
		t.Fatal("the provisioner installs the boot script before it writes the resolver, so a start could unmount the directory the resolver still lives in")
	}
	// ⛔ The symlink is removed before anything is written: writing through it
	// would put this base's resolver in the tmpfs every distribution can replace.
	rm := strings.Index(script, "rm -f /etc/resolv.conf")
	write := strings.Index(script, "> /etc/resolv.conf ||")
	if rm < 0 || write < 0 || rm > write {
		t.Fatal("the provisioner writes /etc/resolv.conf without removing the symlink first, so the write lands in the shared tmpfs")
	}
	if !strings.Contains(script, "generateResolvConf=false") {
		t.Fatal("the provisioner does not stop WSL regenerating the symlink, so the next start puts it back")
	}
}

// ⛔ A `command=` VALUE THAT wsl.conf CANNOT PARSE DOES NOT RUN AND SAYS NOTHING.
// Measured 2026-09-17: a value carrying nested quotes was silently ignored, and
// the only evidence was a marker file that never appeared. The value is a bare
// path for that reason, and nothing else may go in it.
func TestTheBootCommandIsABarePathWithNoShellInIt(t *testing.T) {
	script := string(provisionScript)
	const want = "SEAL_BOOT_LINE=\"command=$SEAL_BOOT\""
	if !strings.Contains(script, want) {
		t.Fatalf("the provisioner no longer sets the boot line to a bare path: want %s", want)
	}
	line := script[strings.Index(script, want):]
	line = line[:strings.Index(line, "\n")]
	for _, forbidden := range []string{";", "&&", "|", "$(", "'"} {
		if strings.Contains(strings.TrimPrefix(line, want), forbidden) {
			t.Fatalf("the boot line carries %q, which wsl.conf's parser is not reliable about: %s", forbidden, line)
		}
	}
}

// ⛔ THE VERIFIER READS THE MOUNT TABLE AND THE RESOLVER, not wsl.conf. A boot
// command wsl.conf rejected leaves the setting written and the tmpfs open, and a
// verifier that read the intention would pass that base.
func TestTheVerifierRefusesASharedTmpfsThatIsStillMountedOrALostResolver(t *testing.T) {
	script := string(verifyScript)
	for _, want := range []string{
		"grep -q ' /mnt/wsl ' /proc/mounts",
		"is still mounted",
		"is still a symlink",
		"took DNS with it",
		"cannot resolve a name",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the verifier no longer refuses over %q", want)
		}
	}
	if strings.Contains(script, "grep -q shared_tmpfs /etc/wsl.conf") {
		t.Error("the verifier reads wsl.conf for this, which is the intention rather than the result")
	}
}
