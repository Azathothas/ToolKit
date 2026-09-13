// SPDX-License-Identifier: 0BSD

package compat

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// The twelve actions, ported from the script's core/action-*.ps1.

// hostAddress is the address a distro reaches THIS host at, as a value.
//
// ⭐ ONE RESOLUTION, TWO CALLERS. -Action HostAddress prints it and -ScriptArg
// expands @hostaddress with it. Splitting the lookup from the report is what
// keeps those two from drifting: a second copy of the mode table would be a
// second place for the mirrored branch to be wrong, and the wrong answer there
// is 127.0.0.1, which is a plausible address that never connects.
//
// ⛔ IT REFUSES RATHER THAN GUESSING. A mode it cannot resolve to one address
// refuses with the candidates named. An address invented here is one a caller
// binds a fixture to and then debugs for an hour.
type hostAddress struct {
	Address   string
	Mode      string
	Source    string
	Path      string
	Interface string
}

func (s *session) resolveHostAddress() (hostAddress, error) {
	netMode := networkingMode()
	if netMode.Mode == "mirrored" {
		return hostAddress{Address: "127.0.0.1", Mode: netMode.Mode, Source: netMode.Source, Path: netMode.Path, Interface: "loopback"}, nil
	}
	if netMode.Mode != "nat" {
		// bridged, or something a later WSL adds. Both have more than one
		// right answer and this tool has no way to choose between them.
		return hostAddress{}, fmt.Errorf("Networking mode is '%s', and this tool can only answer for 'nat' "+
			"and 'mirrored'. In bridged mode the distro is on the LAN and reaches this host "+
			"at whichever host address is on that switch, which is a choice rather than a "+
			"lookup. Read it from inside a distro instead: "+
			"awk '$2 == 00000000 { print $3 }' /proc/net/route, little-endian hex.", netMode.Mode)
	}

	address, ifname, found := lookupWSLAdapter()
	if !found {
		return hostAddress{}, fmt.Errorf("NAT mode, and no WSL network adapter is up on this host. It is created when the " +
			"WSL utility VM first starts, so this is what an answer looks like before anything " +
			"has run: start a distro and ask again. Nothing was created to find that out.")
	}
	return hostAddress{
		Address: address, Mode: netMode.Mode, Source: netMode.Source,
		Path: netMode.Path, Interface: ifname,
	}, nil
}

// lookupWSLAdapter is the one hook that finds the host's WSL adapter address.
// It is a package variable for the same reason resolveWsl is.
var lookupWSLAdapter = scanWSLAdapter

// scanWSLAdapter walks this host's network interfaces for the WSL vEthernet
// adapter and its first IPv4 address.
func scanWSLAdapter() (address, ifname string, found bool) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", "", false
	}
	for _, n := range ifaces {
		// ⚠ MATCHED ON A PREFIX, NOT ON AN EXACT NAME. Windows has called this
		// adapter both 'vEthernet (WSL)' and 'vEthernet (WSL (Hyper-V
		// firewall))'.
		if !strings.HasPrefix(strings.ToLower(n.Name), "vethernet (wsl") {
			continue
		}
		if n.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := n.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			// Only IPv4: that is the family the mode table answers for.
			v4 := ipn.IP.To4()
			if v4 == nil {
				continue
			}
			return v4.String(), n.Name, true
		}
	}
	return "", "", false
}

func (s *session) actionHostAddress() (int, error) {
	addr, err := s.resolveHostAddress()
	if err != nil {
		return 1, err
	}
	s.log.noteLine("==> WSL networking mode: " + addr.Mode + " (from " + addr.Source + ")")
	if addr.Path != "" {
		s.log.noteLine("    " + addr.Path)
	}

	if addr.Mode == "mirrored" {
		s.log.noteLine("  * mirrored mode: the distro and this host share the loopback address.")
		s.log.noteLine("    A host service on 127.0.0.1 is reachable from inside the distro.")
		fmt.Fprintln(s.out, addr.Address)
		return 0, nil
	}

	s.log.noteLine("  * NAT mode: the distro reaches this host at " + addr.Address + ", on '" + addr.Interface + "'.")
	s.log.noteLine("  ! A HOST SERVICE ON 127.0.0.1 IS NOT REACHABLE FROM THE DISTRO IN THIS MODE.")
	s.log.noteLine("    Bind it to " + addr.Address + ", or to 0.0.0.0 if you accept the LAN as well.")
	s.log.noteLine("    The failure is silent: a fixture on loopback simply never receives a")
	s.log.noteLine("    connection, and nothing on either side says why.")
	s.log.noteLine("    This address is assigned by WSL and changes. Read it, never record it.")
	fmt.Fprintln(s.out, addr.Address)
	return 0, nil
}

// actionNew creates an ephemeral distro from an image or a tarball.
func (s *session) actionNew(ctx context.Context) int {
	if s.opts.Image == "" && s.opts.Tarball == "" {
		s.fail("Action New requires -Image (e.g. alpine:3.22) or -Tarball <path>.")
		return 1
	}
	if s.opts.Image != "" && s.opts.Tarball != "" {
		s.fail("Pass either -Image or -Tarball, not both.")
		return 1
	}

	// -Tarball takes a snapshot TAG as well as a path, so a caller who made
	// one names it rather than spelling out where this tool keeps it. A real
	// file always wins, so a caller with a tarball called `ready.tar` in the
	// working directory gets that file and not a snapshot of the same name.
	tarballArg := s.opts.Tarball
	if tarballArg != "" && !pathExists(tarballArg) {
		if asSnapshot, err := s.snapshotPath(tarballArg); err == nil && pathExists(asSnapshot) {
			s.log.step("-Tarball '" + tarballArg + "' names the snapshot at " + asSnapshot)
			s.log.warn("a snapshot carries whatever the distribution held when it was taken.")
			tarballArg = asSnapshot
		}
	}

	// ⛔ BEFORE ANY NAME IS DRAWN AND BEFORE ANY DIRECTORY IS MADE. A reuse
	// that had already created state would be a reuse that cost an import.
	if s.opts.Reuse {
		if s.opts.Tarball != "" {
			s.fail("-Reuse selects a distribution built from an -Image and does not apply to -Tarball.")
			return 1
		}
		if found := s.findReusableDistro(s.opts.Image); found != nil {
			// ⛔ IT SAYS WHICH IT DID, EVERY TIME. A reused distribution
			// carries whatever the last command left in it, and a caller who
			// did not ask for that is owed the warning rather than a silent
			// speed-up.
			s.log.step("Reusing '" + found.Name + "', built from " + found.Image + ", " + formatDistroAge(found))
			s.log.warn("it carries whatever the previous run left in it. Drop -Reuse for a clean one.")
			if s.opts.DryRun {
				steps := []string{
					"reuse      " + found.Name + " rather than importing",
				}
				if plan := s.commandPlanLine(found.Name, s.opts.User); plan != "" {
					steps = append(steps, "command    "+plan)
				}
				s.dryRunPlan("New", found.Name, steps)
				return 0
			}
			rc := 0
			if s.commandBytes != nil {
				s.log.step("Running command as '" + s.opts.User + "'")
				if err := s.invokeInDistro(ctx, found.Name, s.opts.User, s.commandBytes, &rc); err != nil {
					s.fail(err.Error())
					return 1
				}
				if rc != 0 {
					s.log.warn(fmt.Sprintf("command exited %d", rc))
				}
			}
			// ⚠ -Ephemeral AND -Reuse TOGETHER WOULD DESTROY THE THING THAT
			// WAS REUSED. Refused by name rather than silently ignored.
			if s.opts.Ephemeral {
				s.fail("-Ephemeral removes the distribution when the command ends and -Reuse keeps " +
					"one to run in again. Pass one.")
				return 1
			}
			return rc
		}
		s.log.step("-Reuse found no registered distribution built from " + s.opts.Image + "; importing one")
	}

	distro, err := s.resolveNewDistroName(s.opts.Name, s.opts.Image)
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	if isProtectedName(distro) {
		s.fail(fmt.Sprintf("Refusing to create a distro named '%s' (protected).", distro))
		return 1
	}

	target := filepath.Join(s.baseDir, distro)
	if err := s.assertInsideBaseDir(target); err != nil {
		s.fail(err.Error())
		return 1
	}
	if pathExists(target) {
		s.fail(fmt.Sprintf("Refusing to overwrite existing state at '%s'. Inspect the owned state first.", target))
		return 1
	}

	// ⛔ THE DRY RUN RETURNS BEFORE THE FIRST DIRECTORY CREATION, which is the
	// first thing on this path that changes the machine. The name it prints
	// carries a random suffix drawn just now, so a real run draws a different
	// one; that is said on the line rather than covered up with a fake
	// constant.
	if s.opts.DryRun {
		steps := []string{}
		if s.opts.Tarball != "" {
			steps = append(steps, "import     "+tarballArg)
		} else {
			engine := findContainerEngine()
			engineLine := "NONE FOUND -- -Image would be refused"
			if engine != nil {
				engineLine = engine.Name + " at " + engine.Path
			}
			steps = append(steps, "engine     "+engineLine)
			if engine != nil {
				if platform, err := s.enginePlatform(engine); err == nil {
					steps = append(steps, "platform   "+platform)
				} else {
					steps = append(steps, "platform   unreadable: "+err.Error())
				}
			}
			steps = append(steps, "pull       "+s.opts.Image)
			steps = append(steps, "export     "+filepath.Join(s.baseDir, distro+".tar"))
		}
		steps = append(steps, "directory  "+target)
		steps = append(steps, fmt.Sprintf("wsl.exe    --import %s %s <rootfs.tar> --version 2", distro, target))
		if s.opts.Systemd {
			steps = append(steps, "systemd    /etc/wsl.conf written, then the distro restarted")
		}
		if s.opts.OciEnv {
			steps = append(steps, "ocienv     /etc/profile.d/10-oci-env.sh written from the image config")
		}
		if plan := s.commandPlanLine(distro, s.opts.User); plan != "" {
			steps = append(steps, "command    "+plan)
		}
		if s.opts.Ephemeral {
			steps = append(steps, "ephemeral  the distro would then be unregistered and its disk deleted")
		}
		steps = append(steps, "⚠ the name above carries a random suffix drawn for this plan; a real run draws its own")
		s.dryRunPlan("New", distro, steps)
		return 0
	}

	var tarPath string
	tempTar := false
	rc := 0

	rollback := func(creationErr error) {
		s.log.warn("creation failed; rolling back")
		if known, err := s.distroNames(); err == nil && containsString(known, distro) {
			// ⛔ --terminate FIRST, exactly as removeEphemeralDistro does:
			// going straight to --unregister releases the disk
			// asynchronously, which is the race removePathWithRetry exists
			// to survive.
			_, _ = s.wslTerminate(distro)
			wsl, err := resolveWsl()
			if err == nil {
				_, _ = s.wslCapture(wsl, []string{"--unregister", distro}, true)
			}
		}
		if pathExists(target) {
			if err := s.removePathWithRetry(target, "partial disk for '"+distro+"'", ""); err != nil {
				s.log.warn("rollback incomplete: " + err.Error())
			}
		}
	}

	creationErr := func() (err error) {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}

		if s.opts.Tarball != "" {
			if !pathExists(tarballArg) {
				return fmt.Errorf("Tarball not found: %s. It is neither a file nor a snapshot tag; "+
					"-Action List names the snapshots this tool holds.", s.opts.Tarball)
			}
			tarPath = tarballArg
		} else {
			engine := findContainerEngine()
			if engine == nil {
				return fmt.Errorf("No container engine found. Install podman or docker, or pass -Tarball " +
					"with a rootfs archive instead.")
			}
			tarPath = filepath.Join(s.baseDir, distro+".tar")
			tempTar = true
			if err := s.exportImageRootfs(engine, s.opts.Image, tarPath); err != nil {
				return err
			}
		}

		if err := s.assertEnoughDiskSpace(tarPath, target); err != nil {
			return err
		}

		s.log.step(fmt.Sprintf("Importing as WSL2 distro '%s'", distro))
		wsl, err := resolveWsl()
		if err != nil {
			return err
		}
		if _, err := s.wslCapture(wsl, []string{"--import", distro, target, tarPath, "--version", "2"}, false); err != nil {
			return err
		}

		// Smoke test: a distro whose /bin/sh does not run is useless. Fail
		// loudly now. The loop also absorbs a first-boot race: drvfs
		// automount of /mnt/<drive> can lag the first shell by a second or
		// two.
		//
		// ⭐ IT ALSO PROVES THE COMMAND CHANNEL, because it goes through the
		// same transport every -Command does. A guest with no base64, or no
		// /dev/fd, fails HERE, at creation, with a message naming it.
		probeScript := strings.Join([]string{
			"echo __WSL_OK__",
			"for _ in 1 2 3 4 5 6 7 8 9 10; do",
			"    if [ -d /mnt/c ]; then break; fi",
			"    sleep 1",
			"done",
			`if [ ! -d /mnt/c ]; then echo "note: no /mnt/c (Windows drives not mounted)"; fi`,
			`head -2 /etc/os-release 2>/dev/null || echo "os-release: n/a"`,
		}, "\n")
		probeRc := 0
		probeText, err := s.distroOutput(distro, "root", []byte(probeScript), &probeRc, "the smoke probe")
		if err != nil {
			return err
		}
		if !strings.Contains(probeText, "__WSL_OK__") {
			return fmt.Errorf("Distro imported but /bin/sh did not run, or this rootfs cannot carry a "+
				"command: the channel needs base64 and /dev/fd inside the guest. "+
				"The probe exited %d. Output: %s", probeRc, strings.TrimSpace(probeText))
		}
		s.log.ok("'" + distro + "' is up")
		for _, l := range splitLines(probeText) {
			t := strings.TrimSpace(l)
			if t != "" && t != "__WSL_OK__" {
				s.log.dim("    " + t)
			}
		}

		// Before -OciEnv and before the command, so both run under systemd
		// when it was asked for. The profile script is on disk and survives
		// the restart either way.
		if s.opts.Systemd {
			if err := s.enableDistroSystemd(distro); err != nil {
				return err
			}
		}

		if s.opts.OciEnv {
			if s.opts.Tarball != "" {
				s.log.warn("-OciEnv ignored: a rootfs tarball carries no OCI configuration.")
			} else {
				s.log.step("Carrying the image's OCI configuration into the distro")
				cfgEngine := findContainerEngine()
				if cfgEngine == nil {
					return fmt.Errorf("-OciEnv needs the container engine that built this rootfs, and none is on PATH now.")
				}
				cfg, err := s.imageOciConfig(cfgEngine.Path, s.opts.Image)
				if err != nil {
					return err
				}
				if err := s.writeDistroFile(distro, "/etc/profile.d/10-oci-env.sh", newOciEnvScript(cfg, s.opts.Image), "0644"); err != nil {
					return err
				}
				s.log.ok("wrote /etc/profile.d/10-oci-env.sh")
			}
		}

		// Written before the caller's command runs, so a distro whose command
		// failed is still reusable: the import is what this records, and the
		// import succeeded.
		if s.opts.Image != "" {
			if err := s.writeDistroOrigin(distro, s.opts.Image); err != nil {
				return err
			}
		}

		if s.commandBytes != nil {
			s.log.step("Running command as '" + s.opts.User + "'")
			if err := s.invokeInDistro(ctx, distro, s.opts.User, s.commandBytes, &rc); err != nil {
				return err
			}
			if rc != 0 {
				s.log.warn(fmt.Sprintf("command exited %d", rc))
			}
		}

		if !s.opts.Ephemeral {
			s.log.plain("")
			s.log.plain("  Distro : " + distro)
			s.log.plain("  Disk   : " + target)
			s.log.plain("  Enter  : -Action Enter -Name " + distro)
			s.log.plain("  Remove : -Action Remove -Name " + distro + " -Force")
		}
		return nil
	}()

	if creationErr != nil {
		rollback(creationErr)
		s.fail(creationErr.Error())
		return 1
	}

	// The temporary tarball is removed in this cleanup, caught rather than
	// propagated: it runs before the teardown below, and a failure here must
	// not replace the real outcome of the action. A tarball left behind is an
	// orphan, which List reports and Purge removes.
	if tempTar && tarPath != "" && pathExists(tarPath) {
		// ⛔ THROUGH THE SAME DELETION AS EVERYTHING ELSE, containment guard
		// included.
		if err := s.removePathWithRetry(tarPath, "temporary rootfs tarball", ""); err != nil {
			s.log.warn("could not remove the temp tarball: " + err.Error())
		}
	}

	// ⛔ THE TEARDOWN IS OUTSIDE THE CREATION, ON PURPOSE. Inside it, a
	// teardown that could not delete the disk would be reported as "creation
	// failed; rolling back", which is false in a way that sends the reader
	// looking in the wrong place: the distro was created, the command ran, and
	// the only thing wrong is that several gigabytes are still on disk.
	if s.opts.Ephemeral {
		s.log.step("-Ephemeral set: tearing down '" + distro + "'")
		if err := s.removeEphemeralDistro(distro, true); err != nil {
			s.fail(err.Error())
			return 1
		}
	}

	// After the teardown, and after the cleanup removed the temp tarball. Both
	// of those are the reason this is at the bottom rather than beside the
	// command that produced the code.
	return rc
}

// actionRun runs a command inside an existing ephemeral distro.
func (s *session) actionRun(ctx context.Context) int {
	if s.opts.Name == "" {
		s.fail("Action Run requires -Name.")
		return 1
	}
	if s.commandBytes == nil {
		s.fail("Action Run requires -Command, -CommandFile or -CommandB64.")
		return 1
	}
	distro, err := s.resolveDistroName(s.opts.Name, "")
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	known, err := s.distroNames()
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	if !containsString(known, distro) {
		s.fail(fmt.Sprintf("Distro '%s' is not registered. Create it with -Action New.", distro))
		return 1
	}
	if s.opts.DryRun {
		s.dryRunPlan("Run", distro, []string{"command    " + s.commandPlanLine(distro, s.opts.User)})
		return 0
	}
	rc := 0
	if err := s.invokeInDistro(ctx, distro, s.opts.User, s.commandBytes, &rc); err != nil {
		s.fail(err.Error())
		return 1
	}
	return rc
}

// actionEnter attaches an interactive shell to an existing ephemeral distro.
//
// ⛔ IT SENDS NO COMMAND, and that is the whole difference from Run. No '--',
// no '/bin/sh -lc', and no base64 transport: wsl.exe is handed the distro and
// the user and nothing else, so the guest's login shell owns the terminal.
//
// ⛔ IT IS NOT BOUNDED BY -TimeoutSeconds either. A person sitting in a shell
// is not a wedged init.
//
// ⚠ The name is prefix-forced like every other action, so -Name
// podman-machine-default asks for 'eph-podman-machine-default' and is refused
// as unregistered. This action cannot reach a distro the tool did not create.
func (s *session) actionEnter(ctx context.Context) int {
	if s.opts.Name == "" {
		s.fail("Action Enter requires -Name.")
		return 1
	}
	distro, err := s.resolveDistroName(s.opts.Name, "")
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	known, err := s.distroNames()
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	if !containsString(known, distro) {
		s.fail(fmt.Sprintf("Distro '%s' is not registered. '-Action List' shows the ones that are, "+
			"and '-Action New' creates one.", distro))
		return 1
	}
	if s.opts.DryRun {
		wsl, _ := resolveWsl()
		s.dryRunPlan("Enter", distro, []string{
			"wsl.exe    " + wsl + " -d " + distro + " -u " + s.opts.User,
			"no command, no transport, no bound: the guest login shell would own the terminal",
		})
		return 0
	}

	s.log.step("Attaching to '" + distro + "' as '" + s.opts.User + "'. Leave it with exit, or Ctrl-D.")
	wsl, err := resolveWsl()
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	rc, runErr := s.foreground(ctx, wsl, []string{"-d", distro, "-u", s.opts.User})
	if runErr != nil && rc == 0 {
		s.fail(runErr.Error())
		return 1
	}
	return rc
}

// actionList lists ephemeral distros, and shows what else exists (never
// touched).
//
// ⛔ A PARTIAL INVENTORY IS NEVER PRESENTED AS COMPLETE. An enumeration that
// was refused is reported as one, with exit 2, rather than describing a
// machine this process could not see.
func (s *session) actionList() int {
	all, err := s.distroNames()
	if err != nil {
		s.log.noteLine("could not list the distributions on this machine, so nothing below would be complete.")
		s.log.noteLine(err.Error())
		s.log.noteLine("This process may be refused where wsl.exe itself works. Try the approval path this session offers.")
		return 2
	}
	mine := filterPrefix(all, prefix)
	s.log.step("Ephemeral distros (prefix '" + prefix + "')")
	if len(mine) == 0 {
		s.log.dim("  (none)")
	} else {
		for _, m := range mine {
			s.log.plain("  " + m)
		}
	}

	s.log.step("Other distros on this system -- never touched by this tool")
	others := filterNotPrefix(all, prefix)
	if len(others) == 0 {
		s.log.dim("  (none)")
	} else {
		for _, o := range others {
			tag := ""
			if isProtectedName(o) {
				tag = "   [PROTECTED]"
			}
			s.log.dim("  " + o + tag)
		}
	}

	s.log.step("Orphaned rootfs tarballs in " + s.baseDir)
	orphans := s.orphanTarballs()
	if len(orphans) == 0 {
		s.log.dim("  (none)")
	} else {
		for _, t := range orphans {
			s.log.plain(fmt.Sprintf("  %s   %.1f MiB   written %s",
				t.Name, float64(t.Bytes)/(1024*1024), t.Written.Format("2006-01-02 15:04:05Z")))
		}
		var sum int64
		for _, t := range orphans {
			sum += t.Bytes
		}
		s.log.warn(fmt.Sprintf("%.1f MiB total. Remove them with -Action Purge.", float64(sum)/(1024*1024)))
		s.log.warn("a New running right now also has a .tar here: check the time before purging.")
	}
	return 0
}

// actionRemove unregisters one ephemeral distro and deletes its disk.
func (s *session) actionRemove() int {
	if s.opts.Name == "" {
		s.fail("Action Remove requires -Name.")
		return 1
	}
	target, err := s.resolveDistroName(s.opts.Name, "")
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	if s.opts.DryRun {
		s.dryRunPlan("Remove", target, []string{
			"unregister " + target,
			"delete     " + filepath.Join(s.baseDir, target),
			"the name was prefix-forced first, so a protected distro cannot be reached by asking for it",
		})
		return 0
	}
	if err := s.removeEphemeralDistro(target, false); err != nil {
		s.fail(err.Error())
		return 1
	}
	return 0
}

// actionPurge removes ALL ephemeral distros (prefix-matched only) and the
// orphaned tarballs. Snapshots are reported and never removed.
func (s *session) actionPurge() int {
	all, err := s.distroNames()
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	mine := filterPrefix(all, prefix)
	orphans := s.orphanTarballs()
	// ⛔ SNAPSHOTS ARE REPORTED AND NEVER REMOVED. Purge exists to collect
	// what was LEFT BEHIND. A snapshot is the one durable thing this tool
	// makes on purpose, and a caller who believed it was durable losing it to
	// a routine cleanup is the worse failure by far.
	snaps := s.snapshots()
	snapLines := func() {
		if len(snaps) == 0 {
			return
		}
		var ssum int64
		for _, f := range snaps {
			ssum += f.Bytes
		}
		dir, _ := s.snapshotDir()
		s.log.noteLine(fmt.Sprintf("  %d snapshot(s), %.1f MiB, KEPT in %s", len(snaps), float64(ssum)/(1024*1024), dir))
		s.log.noteLine("  Purge never removes those. Delete the file to remove one.")
	}

	if len(mine) == 0 && len(orphans) == 0 {
		s.log.ok("nothing to purge")
		snapLines()
		return 0
	}

	var what []string
	if len(mine) > 0 {
		s.log.step("Ephemeral distro(s): " + strings.Join(mine, ", "))
		what = append(what, fmt.Sprintf("%d distro(s)", len(mine)))
	}
	if len(orphans) > 0 {
		var sum int64
		for _, t := range orphans {
			sum += t.Bytes
		}
		names := make([]string, 0, len(orphans))
		for _, t := range orphans {
			names = append(names, t.Name)
		}
		s.log.step(fmt.Sprintf("Orphaned rootfs tarball(s), %.1f MiB: %s", float64(sum)/(1024*1024), strings.Join(names, ", ")))
		what = append(what, fmt.Sprintf("%d tarball(s)", len(orphans)))
	}

	// ⛔ BEFORE THE CONFIRMATION AND BEFORE THE FIRST DELETION. A dry run that
	// asked for confirmation would be teaching a caller to answer yes to a
	// prompt that sometimes deletes and sometimes does not.
	if s.opts.DryRun {
		steps := []string{}
		for _, d := range mine {
			steps = append(steps, "unregister "+d+", and delete "+filepath.Join(s.baseDir, d))
		}
		for _, t := range orphans {
			steps = append(steps, "delete     "+t.Path)
		}
		s.dryRunPlan("Purge", "", steps)
		return 0
	}

	snapLines()

	// ONE confirmation covering both classes. Two prompts over one -Force is
	// how somebody learns to pass -Force without reading either of them.
	if !s.confirmDestructive(strings.Join(what, " and "), "Purge") {
		return 0
	}

	// Counted, not thrown on at the first failure. One stuck item must not
	// hide the state of the rest.
	failed := 0
	for _, d := range mine {
		if err := s.removeEphemeralDistro(d, true); err != nil {
			s.log.warn(fmt.Sprintf("skip %s: %s", d, err.Error()))
			failed++
		}
	}
	for _, t := range orphans {
		// Through the SAME deletion as a distro disk, so the containment guard
		// covers both.
		if err := s.removePathWithRetry(t.Path, "orphaned rootfs tarball", ""); err != nil {
			s.log.warn(fmt.Sprintf("skip %s: %s", t.Name, err.Error()))
			failed++
		}
	}

	// A Purge that could not remove something must NOT exit 0.
	if failed > 0 {
		s.fail(fmt.Sprintf("%d of %d item(s) were NOT removed. Each is named in a warning above.",
			failed, len(mine)+len(orphans)))
		return 1
	}
	return 0
}

// actionSnapshot exports a registered distro back to a rootfs tarball that
// -Action New can import.
//
// ⭐ THE THIRD CALLER OF ONE PATH. exportImageRootfs writes a tarball and
// actionNew imports one; this writes one from a distro rather than from an
// image, and New reads it back through the -Tarball it already has.
//
// ⚠ A SNAPSHOT CARRIES WHATEVER THE LAST COMMAND LEFT IN IT, including a
// credential a caller passed with -ScriptArg. It is a plain tarball on this
// machine's disk and nothing in it is encrypted.
func (s *session) actionSnapshot() int {
	if s.opts.Name == "" {
		s.fail("Action Snapshot requires -Name <distro>.")
		return 1
	}
	tag, err := snapshotTag(s.opts.As)
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	distro, err := s.resolveDistroName(s.opts.Name, "")
	if err != nil {
		s.fail(err.Error())
		return 1
	}

	// ⛔ THROUGH THE SAME OWNERSHIP CHECKS AS EVERY DESTRUCTIVE PATH, even
	// though this removes nothing. An export READS a distribution whole and
	// writes it to a file the caller keeps, so exporting one this tool did not
	// create would be this tool copying somebody else's disk out.
	// assertRemovable is where the prefix rule and the protected-name list
	// already live, and a second copy of either is how one of them stops being
	// applied.
	if err := s.assertRemovable(distro); err != nil {
		s.fail(err.Error())
		return 1
	}

	known, err := s.distroNames()
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	if !containsString(known, distro) {
		s.fail(fmt.Sprintf("'%s' is not a registered distribution. -Action List names the ones this tool made.", distro))
		return 1
	}

	dir, err := s.snapshotDir()
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	out := filepath.Join(dir, tag+".tar")
	wsl, err := resolveWsl()
	if err != nil {
		s.fail(err.Error())
		return 1
	}

	if s.opts.DryRun {
		s.dryRunPlan("Snapshot", distro, []string{"export     " + distro + " -> " + out})
		return 0
	}

	if pathExists(out) && !s.opts.Force {
		s.fail(fmt.Sprintf("A snapshot tagged '%s' already exists at %s. Pass -Force to replace it.", tag, out))
		return 1
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.fail(err.Error())
		return 1
	}

	// ⚠ WRITTEN TO A TEMPORARY AND RENAMED, in the same directory. A killed
	// export otherwise leaves a truncated tarball under the tag, and the next
	// New -Tarball would import it: a rename across volumes is a copy and
	// loses the guarantee, which is why the temporary is a sibling.
	temp := filepath.Join(dir, "."+randomSuffix(12)+".tmp")
	s.log.step(fmt.Sprintf("Exporting '%s' as snapshot '%s'", distro, tag))
	_, exportErr := s.wslCapture(wsl, []string{"--export", distro, temp}, false)
	if exportErr == nil {
		st, statErr := os.Stat(temp)
		if statErr != nil {
			exportErr = fmt.Errorf("The export produced no file at %s", temp)
		} else if st.Size() < 1024 {
			// ⛔ THE EFFECT IS READ BACK. wsl.exe --export has been seen to
			// exit 0 over a file nobody could import; a size floor turns that
			// into a refusal here rather than into a failed import days
			// later.
			exportErr = fmt.Errorf("The exported snapshot is implausibly small (%d bytes).", st.Size())
		} else {
			if err := os.Rename(temp, out); err != nil {
				exportErr = err
			}
		}
	}
	if exportErr == nil {
		st, _ := os.Stat(out)
		s.log.ok(fmt.Sprintf("snapshot '%s': %.1f MiB at %s", tag, float64(st.Size())/(1024*1024), out))
		s.log.warn("it carries whatever that distribution held, including anything a previous " +
			"-Command or -ScriptArg left in it.")
		s.log.noteLine("  reuse it with: -Action New -Tarball " + tag)
	}
	// The temporary is always collected, whatever happened.
	if pathExists(temp) {
		_ = os.Remove(temp)
	}
	if exportErr != nil {
		s.fail(exportErr.Error())
		return 1
	}
	return 0
}

// actionResources reports what WSL and the container engine are holding on
// this machine, and PRINTs the cleanup commands without running any of them.
// Read-only. Nothing it reports is this tool's to remove except what the
// prefix names.
func (s *session) actionResources() int {
	all, err := s.distroNames()
	if err != nil {
		s.fail(err.Error())
		return 1
	}
	mine := filterPrefix(all, prefix)

	s.log.step("What this tool made, under " + s.baseDir)
	var mineBytes int64
	unmeasured := 0
	if len(mine) == 0 {
		s.log.dim("  (no ephemeral distros)")
	}
	for _, d := range mine {
		size, ok := s.directorySizeBytes(filepath.Join(s.baseDir, d))
		if !ok {
			unmeasured++
			s.log.warn(fmt.Sprintf("  %-40s size could not be read", d))
			continue
		}
		mineBytes += size
		s.log.plain(fmt.Sprintf("  %-40s %10.1f MiB", d, float64(size)/(1024*1024)))
	}

	orphans := s.orphanTarballs()
	var orphanBytes int64
	if len(orphans) > 0 {
		for _, t := range orphans {
			orphanBytes += t.Bytes
			s.log.plain(fmt.Sprintf("  %-40s %10.1f MiB   rootfs tarball, written %s",
				t.Name, float64(t.Bytes)/(1024*1024), t.Written.Format("2006-01-02 15:04:05Z")))
		}
		s.log.warn("a New running right now also has a .tar here: check the time before purging.")
	}

	// ⛔ A dash rather than a number when something could not be measured. A
	// total that silently counts an unreadable directory as zero is a number
	// somebody acts on.
	if unmeasured > 0 {
		s.log.warn(fmt.Sprintf("total not stated: %d director(y/ies) could not be measured. "+
			"Measured so far: %.1f MiB", unmeasured, float64(mineBytes+orphanBytes)/(1024*1024)))
	} else {
		s.log.ok(fmt.Sprintf("%.1f MiB held by this tool, across %d distro(s) and %d tarball(s)",
			float64(mineBytes+orphanBytes)/(1024*1024), len(mine), len(orphans)))
	}

	s.log.step("What else is registered with WSL, which this tool never touches")
	others := filterNotPrefix(all, prefix)
	if len(others) == 0 {
		s.log.dim("  (none)")
	}
	for _, o := range others {
		tag := ""
		if isProtectedName(o) {
			tag = "   [PROTECTED]"
		}
		s.log.dim("  " + o + tag)
	}
	s.log.dim("  their disks are wherever they were imported to, which this tool does not know.")

	s.log.step("What the container engine is holding, which this tool never made")
	engine := findContainerEngine()
	if engine == nil {
		s.log.dim("  (no podman or docker on this host)")
	} else {
		s.log.dim(fmt.Sprintf("  engine: %s (%s)", engine.Name, engine.Path))
		usage := s.engineUsage(engine)
		if !usage.reachable {
			s.log.warn("the engine did not answer, so nothing about it is reported.")
			s.log.warn("on Windows podman runs in its own VM: try  podman machine start")
			if usage.reason != "" {
				s.log.dim("    " + usage.reason)
			}
		} else {
			s.log.plain(fmt.Sprintf("  %-15s %6s %7s %12s  %s", "TYPE", "TOTAL", "ACTIVE", "SIZE", "RECLAIMABLE"))
			for _, r := range usage.rows {
				s.log.plain(fmt.Sprintf("  %-15s %6s %7s %12s  %s", r.Type, r.Total, r.Active, r.Size, r.Reclaimable))
			}
			if usage.dangling >= 0 {
				s.log.plain(fmt.Sprintf("  dangling images: %d", usage.dangling))
			}
			if usage.unused >= 0 {
				s.log.plain(fmt.Sprintf("  unused volumes:  %d", usage.unused))
			}
			s.log.warn("RECLAIMABLE is not the whole prize. This engine reports three rows " +
				"from system df, Images, Containers and Local Volumes, and no build cache " +
				"row, so a prune can free considerably more than the figure above.")
		}
	}

	s.log.step("The commands that would free it. NONE of them was run.")
	s.log.plain("")
	s.log.dim("  # this tool's own, and the only ones it will ever remove:")
	s.log.plain("  wsl-toolkit script -Action Purge -Force")
	s.log.plain("")
	s.log.dim("  # the engine's, and NOT this tool's to run. Read them before pasting one:")
	s.log.plain("  podman system df                      # the numbers above, again")
	s.log.plain("  podman image prune --force            # dangling images only")
	s.log.plain("  podman volume prune --force           # volumes nothing references")
	s.log.plain("  podman system prune -a --volumes --force")
	s.log.plain("")
	s.log.warn("the last one removes every image no RUNNING container uses, which is not the")
	s.log.warn("same as unused: an image you pulled this morning goes too, and so does every")
	s.log.warn("named volume holding data somebody kept on purpose.")
	return 0
}

// engineUsage is what the container engine is holding, read only.
//
// ⛔ NOTHING HERE WRITES. Every argument list is a report. -Action Resources
// prints removal commands and runs none of them.
//
// ⚠ THE FIELD SEPARATOR IS A PIPE AND NOT A TAB. A Go template written as
// `{{.Type}}\t{{.Size}}` would put a literal backslash-t into the argument;
// a pipe needs no escape and no shell sees it.
type engineUsageReport struct {
	reachable bool
	reason    string
	rows      []engineRow
	dangling  int
	unused    int
}

type engineRow struct {
	Type, Total, Active, Size, Reclaimable string
}

func (s *session) engineUsage(engine *containerEngine) engineUsageReport {
	timeout := time.Duration(s.opts.TimeoutSeconds) * time.Second
	df := s.boundedCapture(engine.Path, []string{
		"system", "df", "--format", "{{.Type}}|{{.Total}}|{{.Active}}|{{.Size}}|{{.Reclaimable}}"}, timeout)
	if df.Err != nil || df.TimedOut {
		return engineUsageReport{reachable: false, reason: strings.TrimSpace(df.Text), dangling: -1, unused: -1}
	}
	if df.Exit != 0 {
		return engineUsageReport{reachable: false, reason: strings.TrimSpace(df.Text), dangling: -1, unused: -1}
	}
	report := engineUsageReport{reachable: true, dangling: -1, unused: -1}
	for _, line := range splitLines(df.Text) {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		p := strings.Split(t, "|")
		if len(p) < 5 {
			continue // a warning on stderr is not a row
		}
		report.rows = append(report.rows, engineRow{Type: p[0], Total: p[1], Active: p[2], Size: p[3], Reclaimable: p[4]})
	}
	dang := s.boundedCapture(engine.Path, []string{"images", "--filter", "dangling=true", "--format", "{{.ID}}"}, timeout)
	if dang.Err == nil && !dang.TimedOut && dang.Exit == 0 {
		report.dangling = countNonEmpty(splitLines(dang.Text))
	}
	vol := s.boundedCapture(engine.Path, []string{"volume", "ls", "--filter", "dangling=true", "--format", "{{.Name}}"}, timeout)
	if vol.Err == nil && !vol.TimedOut && vol.Exit == 0 {
		report.unused = countNonEmpty(splitLines(vol.Text))
	}
	return report
}

// actionDoctor reports what this host can and cannot do, before anything is
// created.
//
// ⭐ THE DEFECT IT EXISTS FOR IS A CRYPTIC FAILURE HALFWAY IN. Each row is
// knowable in under a second and none of them was said at the failure.
//
// ⛔ IT IS READ-ONLY AND IT CREATES NOTHING. No distro, no pull, no import, no
// file.
//
// ⭐ EVERY ROW SAYS HOW IT WAS OBTAINED. `obs` was read from an interface,
// `der` was computed from readings, `abs` means this machine cannot answer at
// all. A row that could not be measured says so instead of carrying a number
// nobody took.
func (s *session) actionDoctor() int {
	type row struct{ name, prov, value string }
	var rows []row
	add := func(name, prov, value string) { rows = append(rows, row{name, prov, value}) }

	add("tool", "obs", "wsl-toolkit "+toolkit.Version)
	add("host", "obs", runtimeOS())

	wsl, wslErr := resolveWsl()
	if wslErr != nil {
		add("wsl.exe", "abs", "not on PATH. Nothing this tool does can work without it.")
	} else {
		add("wsl.exe", "obs", wsl)
		res := s.boundedCapture(wsl, []string{"--version"}, 15*time.Second)
		switch {
		case res.TimedOut:
			add("wsl version", "abs", "wsl --version did not answer inside 15s")
		case res.Err != nil:
			add("wsl version", "abs", "wsl --version could not be started")
		default:
			first := ""
			for _, l := range splitLines(res.Text) {
				if strings.TrimSpace(l) != "" {
					first = strings.TrimSpace(l)
					break
				}
			}
			if first == "" {
				add("wsl version", "abs", "wsl --version printed nothing")
			} else {
				add("wsl version", "obs", first)
			}
		}

		all, err := s.distroNames()
		if err != nil {
			add("distros", "abs", err.Error())
		} else {
			mine := filterPrefix(all, prefix)
			add("distros", "obs", fmt.Sprintf("%d registered, %d with the '%s' prefix", len(all), len(mine), prefix))
			var prot []string
			for _, d := range all {
				if isProtectedName(d) {
					prot = append(prot, d)
				}
			}
			if len(prot) == 0 {
				add("protected", "obs", "none of the registered distros is on the protected list")
			} else {
				add("protected", "obs", strings.Join(prot, ", ")+" -- this tool refuses to remove these")
			}
		}
	}

	// -- networking, which is where a guest silently fails to reach the host
	if addr, err := s.resolveHostAddress(); err == nil {
		add("networkingMode", "obs", addr.Mode+"  ("+addr.Source+")")
		add("host address", "der", addr.Address+"  -- what a distro reaches this host at. Read, never recorded: WSL reassigns it.")
	} else {
		add("host address", "abs", err.Error())
	}

	// -- the container engine, which only -Image needs
	engine := findContainerEngine()
	if engine == nil {
		add("container engine", "abs", "no podman or docker on PATH. -Image cannot work; -Tarball still can.")
	} else {
		add("container engine", "obs", engine.Name+"  ("+engine.Path+")")
		// ⭐ The SAME function exportImageRootfs pins --platform with. Asking
		// a second way here would let this row say one thing while a pull did
		// another.
		if platform, err := s.enginePlatform(engine); err == nil {
			add("platform", "obs", platform+"  -- named on every pull, never inherited")
		} else {
			add("platform", "abs", err.Error())
		}
	}

	// -- where this tool keeps things, and what is left there
	add("base directory", "obs", s.baseDir)
	if free, measurable := volumeFreeBytes(s.baseDir); !measurable {
		add("free space", "abs", "the volume did not report free space")
	} else {
		add("free space", "obs", fmt.Sprintf("%.1f GiB, against a floor of %.0f MiB per import",
			float64(free)/(1024*1024*1024), float64(importSpaceFloor)/(1024*1024)))
	}
	orphans := s.orphanTarballs()
	if len(orphans) == 0 {
		add("orphan tarballs", "obs", "none")
	} else {
		var sum int64
		for _, t := range orphans {
			sum += t.Bytes
		}
		add("orphan tarballs", "obs", fmt.Sprintf("%d, %.1f MiB. -Action Purge removes them.",
			len(orphans), float64(sum)/(1024*1024)))
	}

	// -- the console, which decides whether a log is readable
	add("stdout", "obs", map[bool]string{true: "redirected, so the stream log will not colour it", false: "a console"}[!writerIsTerminal(s.out)])

	// ⭐ MEASURED, NOT ASSUMED. The stream log renders %9f, and the tick is
	// 100ns while the system clock's own resolution is coarser still. Printing
	// nine digits without saying which of them were measured is nine digits of
	// invented precision, so the figure is taken here, on this host, now.
	start := time.Now()
	seen := map[int64]bool{}
	spins := 0
	for time.Since(start) < 40*time.Millisecond && spins < 2_000_000 {
		seen[time.Now().UnixNano()] = true
		spins++
	}
	if len(seen) < 2 {
		add("clock resolution", "abs", "could not be measured in the sampling window")
	} else {
		keys := make([]int64, 0, len(seen))
		for k := range seen {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		smallest := keys[1] - keys[0]
		for i := 2; i < len(keys); i++ {
			if gap := keys[i] - keys[i-1]; gap < smallest {
				smallest = gap
			}
		}
		// ⛔ REPORTED IN NANOSECONDS, because milliseconds rounds the answer to
		// zero on a host whose clock is finer than a millisecond. The count of
		// distinct readings is printed beside it so a reader can see the
		// sample the figure came from.
		add("clock resolution", "der", fmt.Sprintf("%d ns smallest gap, over %d distinct readings in 40 ms. "+
			"⚠ -TimestampFormat %%9f pads below this rather than measuring below it.", smallest, len(keys)))
	}

	s.log.step("wsl-toolkit doctor -- read-only, and it created nothing")
	for _, r := range rows {
		s.log.plain(fmt.Sprintf("  %-18s %s  %s", r.name, r.prov, r.value))
	}
	var absent []string
	for _, r := range rows {
		if r.prov == "abs" {
			absent = append(absent, r.name)
		}
	}
	if len(absent) == 0 {
		s.log.ok("every question this host was asked, it answered")
	} else {
		s.log.warn(fmt.Sprintf("%d question(s) this host cannot answer: %s", len(absent), strings.Join(absent, ", ")))
		s.log.warn("an absent row is this tool refusing to fabricate, not a failure of the tool.")
	}
	return 0
}

// -- dry-run plan -------------------------------------------------------------

// dryRunPlan is what would happen, printed, with nothing done.
//
// ⭐ IT GOES TO THE REPORT STREAM, so a caller can capture and audit it. A plan
// a person can only read on a terminal is a plan nothing can check.
//
// ⛔ IT CREATES, WRITES AND DELETES NOTHING.
func (s *session) dryRunPlan(action, distro string, steps []string) {
	fmt.Fprintf(s.out, "DRY RUN: -Action %s. Nothing below has been done.\n", action)
	if distro != "" {
		fmt.Fprintf(s.out, "  distro     %s\n", distro)
	}
	for _, step := range steps {
		fmt.Fprintf(s.out, "  %s\n", step)
	}
	fmt.Fprintln(s.out, "  ⛔ nothing was created, imported, written or removed. Drop -DryRun to run it.")
}

// commandPlanLine is the exact wsl.exe argument string a run would use, for a
// dry run to print.
//
// ⭐ IT IS BUILT BY THE SAME FUNCTION THE REAL RUN USES, so the plan cannot
// describe a command line the run would not produce.
//
// ⚠ The guest scratch path carries a random component, so the plan's path and
// the run's path differ. That is said on the plan rather than hidden by
// printing a fake constant.
func (s *session) commandPlanLine(distro, runAs string) string {
	if s.commandBytes == nil {
		return ""
	}
	line, err := distroScriptCommand(s.commandBytes, guestScratchPath())
	if err != nil {
		return ""
	}
	wsl, err := resolveWsl()
	if err != nil {
		return ""
	}
	// ⛔ The plan is an ARGUMENT LIST joined for reading, and every argument
	// this tool passes is one it built, which is what makes the join safe.
	return wsl + " -d " + distro + " -u " + runAs + " -- /bin/sh -lc " + line
}

func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func filterPrefix(list []string, pre string) []string {
	var out []string
	for _, s := range list {
		if strings.HasPrefix(s, pre) {
			out = append(out, s)
		}
	}
	return out
}

func filterNotPrefix(list []string, pre string) []string {
	var out []string
	for _, s := range list {
		if !strings.HasPrefix(s, pre) {
			out = append(out, s)
		}
	}
	return out
}

func countNonEmpty(list []string) int {
	n := 0
	for _, s := range list {
		if strings.TrimSpace(s) != "" {
			n++
		}
	}
	return n
}
