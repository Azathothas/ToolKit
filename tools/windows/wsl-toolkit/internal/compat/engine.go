// SPDX-License-Identifier: 0BSD

package compat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Rootfs acquisition, ported from the script's rootfs.ps1.

// containerEngine is an engine this interface can pull and export with.
type containerEngine struct {
	Name string
	Path string
}

// findContainerEngine is Get-ContainerEngine: podman first, then docker, on
// PATH and then at the usual install locations.
func findContainerEngine() *containerEngine {
	for _, name := range []string{"podman", "docker"} {
		if p, err := executablePath(name + ".exe"); err == nil {
			return &containerEngine{Name: name, Path: p}
		}
		if p, err := executablePath(name); err == nil {
			return &containerEngine{Name: name, Path: p}
		}
	}
	type candidate struct {
		name, path string
	}
	candidates := []candidate{
		{"podman", filepath.Join(envValue("LOCALAPPDATA"), "Programs", "Podman", "podman.exe")},
		{"podman", filepath.Join(envValue("ProgramFiles"), "RedHat", "Podman", "podman.exe")},
		{"docker", filepath.Join(envValue("ProgramFiles"), "Docker", "Docker", "resources", "bin", "docker.exe")},
	}
	for _, c := range candidates {
		if c.path == "" || filepath.Base(c.path) == c.path {
			// An unset variable collapses to a bare file name, which LookPath
			// would find on PATH and report as an install location. Refuse
			// that: a candidate is only a candidate when it is a full path.
			continue
		}
		if st, err := os.Stat(c.path); err == nil && !st.IsDir() {
			return &containerEngine{Name: c.name, Path: c.path}
		}
	}
	return nil
}

// convertToOciArch maps whatever an engine calls the host architecture onto
// the name --platform wants. The two engines disagree with each other AND
// with the OCI names: podman info says 'amd64', docker info says 'x86_64',
// and --platform accepts only the first.
//
// An unrecognised value passes through lowercased rather than being rejected:
// a riscv64 host is a legitimate answer this table has not been taught, and
// the caller validates the shape before using it.
func convertToOciArch(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "x86_64", "x86-64":
		return "amd64"
	case "aarch64", "armv8l":
		return "arm64"
	case "armv7l", "armhf", "armv6l":
		return "arm"
	case "i386", "i486", "i586", "i686", "x86":
		return "386"
	}
	return v
}

var archShape = regexp.MustCompile(`^[a-z0-9_]+$`)

// enginePlatform is the linux/ARCH string every pull and every create names,
// read from the engine rather than assumed.
//
// ⭐ ONE READ PATH. exportImageRootfs pins --platform with it and Doctor
// reports it, and both go through here: an answer computed in two places is an
// answer that will one day be two answers.
//
// IT DOUBLES AS THE READINESS PROBE, and its answer is USED rather than
// discarded: a stopped podman machine otherwise yields a cryptic pull error
// with no hint that the VM is down.
func (s *session) enginePlatform(engine *containerEngine) (string, error) {
	archField := "{{.Host.Arch}}"
	if engine.Name == "docker" {
		archField = "{{.Architecture}}"
	}
	// ⛔ THREE OUTCOMES, NOT ONE. A probe that failed for its OWN reasons must
	// not be credited to the engine: the child's exit code and output decide,
	// and a failure that never reached the child says so.
	probe := s.boundedCapture(engine.Path, []string{"info", "--format", archField}, time.Duration(s.opts.TimeoutSeconds)*time.Second)
	if probe.Err != nil {
		return "", fmt.Errorf("could not ask %s at %s what architecture this host is; the engine was never reached, "+
			"so this says nothing about whether it works.\nUnderlying error: %v", engine.Name, engine.Path, probe.Err)
	}
	if probe.TimedOut {
		return "", fmt.Errorf("%s answered nothing to 'info' inside %ds. If you use podman on Windows, start its VM with:  podman machine start",
			engine.Name, int(s.opts.TimeoutSeconds))
	}
	if probe.Exit != 0 {
		return "", fmt.Errorf("%s answered exit %d to 'info'. If you use podman on Windows, start its VM with:  podman machine start\nIt said: %s",
			engine.Name, probe.Exit, strings.TrimSpace(probe.Text))
	}
	rawArch := ""
	for _, line := range splitLines(probe.Text) {
		if strings.TrimSpace(line) != "" {
			rawArch = strings.TrimSpace(line)
		}
	}
	arch := convertToOciArch(rawArch)
	// Refusing here rather than pulling unqualified: an unqualified pull takes
	// whatever the shared local tag currently points at, and the rootfs then
	// imports and every binary in it fails to execute.
	if !archShape.MatchString(arch) {
		return "", fmt.Errorf("Could not read the host architecture from '%s info --format %s'; it answered '%s'. "+
			"Refusing to pull without --platform, because an unqualified pull can silently export the wrong architecture.",
			engine.Name, archField, rawArch)
	}
	return "linux/" + arch, nil
}

// exportImageRootfs pulls an OCI image and flattens it to a rootfs tarball.
func (s *session) exportImageRootfs(engine *containerEngine, imageRef, outFile string) error {
	s.log.step(fmt.Sprintf("Engine: %s (%s)", engine.Name, engine.Path))

	platform, err := s.enginePlatform(engine)
	if err != nil {
		return err
	}
	s.log.step("Platform: " + platform)

	s.log.step("Pulling " + imageRef)
	if out, err := s.engineCapture(engine, []string{"pull", "--platform", platform, imageRef}); err != nil {
		return err
	} else if out == "" {
		// Pull output is streamed to the merged capture; nothing to add.
		_ = out
	}

	var cid string
	{
		out, err := s.engineCapture(engine, []string{"create", "--platform", platform, imageRef})
		if err != nil {
			// Images with no CMD/ENTRYPOINT reject a bare create, so fall back
			// to naming one.
			s.log.warn("bare create failed; retrying with an explicit command")
			out, err = s.engineCapture(engine, []string{"create", "--platform", platform, imageRef, "/bin/sh"})
			if err != nil {
				return err
			}
		}
		for _, line := range splitLines(out) {
			if strings.TrimSpace(line) != "" {
				cid = strings.TrimSpace(line)
			}
		}
		if strings.TrimSpace(cid) == "" {
			return fmt.Errorf("Container id was empty.")
		}
	}

	short := cid
	if len(short) > 12 {
		short = short[:12]
	}
	s.log.step(fmt.Sprintf("Exporting rootfs (container %s)", short))
	// -o is mandatory: a shell redirect would corrupt the binary stream.
	if _, err := s.engineCapture(engine, []string{"export", "-o", outFile, cid}); err != nil {
		// The container is removed in the cleanup below regardless; report the
		// export failure itself.
		_, _ = s.engineCapture(engine, []string{"rm", "-f", cid})
		return err
	}
	_, _ = s.engineCapture(engine, []string{"rm", "-f", cid})

	st, err := os.Stat(outFile)
	if err != nil {
		return fmt.Errorf("Export produced no file at %s", outFile)
	}
	if st.Size() < 1024 {
		return fmt.Errorf("Exported rootfs is implausibly small (%d bytes).", st.Size())
	}
	s.log.ok(fmt.Sprintf("rootfs: %.1f MiB", float64(st.Size())/(1024*1024)))
	return nil
}

// engineCapture is Invoke-Native for the container engine: both streams
// merged, a non-zero exit refused with what the engine said.
func (s *session) engineCapture(engine *containerEngine, args []string) (string, error) {
	var buf strings.Builder
	cmd := engineCommand(engine.Path, args)
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	text := buf.String()
	if err != nil {
		var ee *exitStatus
		if asExitStatus(err, &ee) {
			return text, fmt.Errorf("%s %s failed (exit %d): %s", engine.Name, strings.Join(args, " "), ee.code, strings.TrimSpace(text))
		}
		return text, fmt.Errorf("%s %s could not be started: %v", engine.Name, strings.Join(args, " "), err)
	}
	return text, nil
}

// imageOciConfig is the image's OCI configuration. 'podman export' writes a
// FILESYSTEM and no configuration by definition, so ENV and WORKDIR are not in
// the rootfs and have to be read from the image separately.
//
// '{{json .Config}}' is the one spelling both engines share.
func (s *session) imageOciConfig(enginePath, imageRef string) (imageConfig, error) {
	out, err := captureEngine(enginePath, []string{"image", "inspect", imageRef, "--format", "{{json .Config}}"})
	if err != nil {
		return imageConfig{}, err
	}
	text := strings.TrimSpace(out)
	if text == "" || text == "null" {
		return imageConfig{}, fmt.Errorf("Could not read the OCI configuration of '%s'; the engine answered '%s'.", imageRef, text)
	}
	var cfg imageConfig
	if err := json.Unmarshal([]byte(text), &cfg); err != nil {
		return imageConfig{}, fmt.Errorf("Could not read the OCI configuration of '%s': %v", imageRef, err)
	}
	return cfg, nil
}

type imageConfig struct {
	Env        []string `json:"Env"`
	WorkingDir string   `json:"WorkingDir"`
}

// newOciEnvScript turns an image config into a /etc/profile.d snippet.
//
// ENV and WORKDIR are carried. USER and ENTRYPOINT ARE NOT, and that is a
// decision rather than an omission: WSL fixes the login user at import time
// and -User selects it per call, and a login shell has no entrypoint to run.
// Writing either into profile.d would be a setting that looks like it works.
func (s *session) ociEnvScript(cfg imageConfig, imageRef string) string {
	lines := []string{
		"# Written by wsl-toolkit -OciEnv, from the OCI config of:",
		"#   " + imageRef,
		"# The rootfs came from a filesystem export, which carries no config, so",
		"# without this file the environment here is WSL default and not the",
		"# image environment.",
	}
	for _, e := range cfg.Env {
		i := strings.Index(e, "=")
		if i < 1 {
			// ⛔ SAID, never silently dropped: a skipped entry is an env the
			// caller asked to carry, and silence reads as carried.
			s.log.warn("skipping malformed image env entry: " + e)
			continue
		}
		k, v := e[:i], e[i+1:]
		if !isShellIdentifier(k) {
			s.log.warn("skipping image env name that is not a shell identifier: " + k)
			continue
		}
		lines = append(lines, "export "+k+"="+shellSingleQuoted(v))
	}
	if cfg.WorkingDir != "" && cfg.WorkingDir != "/" {
		// ':' rather than 'true': it is a shell built-in everywhere, and the
		// guard is there so a WORKDIR the image creates at runtime does not
		// make every login shell fail.
		lines = append(lines, "cd "+shellSingleQuoted(cfg.WorkingDir)+" 2>/dev/null || :")
	}
	return strings.Join(lines, "\n") + "\n"
}

var shellIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func isShellIdentifier(s string) bool { return shellIdentifier.MatchString(s) }

// shellSingleQuoted is POSIX single-quoting. The only character a
// single-quoted string cannot contain is a single quote, so it is written by
// closing, escaping and reopening. Used for values that end up INSIDE a file
// in the guest, never for the command that carries them there.
func shellSingleQuoted(raw string) string {
	return "'" + strings.ReplaceAll(raw, "'", `'\''`) + "'"
}
