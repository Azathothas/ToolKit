package toolkit

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// Engine is a container engine on the HOST, used for exactly one job: turning
// an OCI image into a rootfs archive that wsl --import can read.
//
// ⚠ It is not the engine jobs run in. Jobs run in a rootless podman INSIDE the
// owned distribution, so nothing a job does can reach the machine's own engine
// or the images somebody else put in it.
type Engine struct {
	Name string // podman or docker
	Path string
	Arch string // the platform token this engine accepts, normalised
}

// FindEngine picks the host engine, preferring podman.
func FindEngine(ctx context.Context) (*Engine, error) {
	// ⛔ EVERY CANDIDATE'S REASON IS KEPT. Reporting only the last one names
	// whichever engine happened to be tried second, so a broken podman reads as
	// a missing docker and the next reader looks in the wrong place. That is
	// exactly what this function did on its first run here.
	var problems []string
	for _, name := range []string{"podman", "docker"} {
		exe, err := ResolveExecutable(name)
		if err != nil {
			problems = append(problems, name+": "+err.Error())
			continue
		}
		e := &Engine{Name: name, Path: exe.Resolved}
		arch, err := e.readArch(ctx)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s is installed at %s and did not answer: %v", name, exe.Resolved, err))
			continue
		}
		e.Arch = arch
		return e, nil
	}
	if len(problems) == 0 {
		problems = append(problems, "neither podman nor docker is installed")
	}
	return nil, fmt.Errorf("no usable container engine on this host: %s", strings.Join(problems, "; "))
}

// readArch asks the engine what architecture it runs, and normalises the answer.
//
// ⛔ THE TWO ENGINES SPELL THE FIELD DIFFERENTLY AND ASKING THE WRONG ONE FAILS
// THE WHOLE CALL. podman answers to {{.Host.Arch}} and docker to
// {{.Architecture}}; asking podman for the lower-case spelling returns
// "can't evaluate field host", which reads as a broken engine and is a wrong
// template. wsl-toolkit.ps1's ConvertTo-OciArch already knew this, and the first
// version of this function guessed instead of reading it.
//
// ⚠ They also disagree on the VALUE. podman answers amd64 and docker answers
// x86_64, and only the first is a token --platform accepts. A value neither
// recognises is passed through rather than guessed at, so the engine refuses it
// by name instead of this code inventing one.
func (e *Engine) readArch(ctx context.Context) (string, error) {
	bounded, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	field := "{{.Host.Arch}}"
	if e.Name == "docker" {
		field = "{{.Architecture}}"
	}
	out, stderr, err := Output(bounded, e.Path, "info", "--format", field)
	if err != nil {
		return "", fmt.Errorf("%s info --format %s: %w: %s", e.Name, field, err, firstLine(stderr))
	}
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("%s info --format %s printed nothing: %s", e.Name, field, firstLine(stderr))
	}
	arch := strings.TrimSpace(firstLine(out))
	switch arch {
	case "x86_64":
		return "amd64", nil
	case "aarch64":
		return "arm64", nil
	default:
		return arch, nil
	}
}

// Platform is the --platform value every pull and create passes.
//
// ⛔ NAMED ON EVERY CALL, NEVER INHERITED. The local image store is keyed by tag
// and not by architecture, so one `pull --platform linux/riscv64 alpine`
// repoints the shared local alpine:latest at that image and every later
// unqualified pull hands it back. Imported into WSL, that rootfs registers
// cleanly and then nothing in it executes.
func (e *Engine) Platform() string { return "linux/" + e.Arch }

// ExportRootfs turns an image reference into a rootfs archive on the host.
//
// It is the one place this executable talks to the host engine, and it removes
// the container it created whether or not the export succeeded.
func (e *Engine) ExportRootfs(ctx context.Context, ref, tarPath string, log func(string)) error {
	if err := ValidateImageRef(ref); err != nil {
		return err
	}
	pullCtx, cancelPull := context.WithTimeout(ctx, 30*time.Minute)
	defer cancelPull()
	log(fmt.Sprintf("pulling %s for %s", ref, e.Platform()))
	if out, stderr, err := Output(pullCtx, e.Path, "pull", "--platform", e.Platform(), ref); err != nil {
		return fmt.Errorf("%s pull %s: %w: %s", e.Name, ref, err, firstLine(out+stderr))
	}

	createCtx, cancelCreate := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelCreate()
	out, stderr, err := Output(createCtx, e.Path, "create", "--platform", e.Platform(), ref)
	if err != nil {
		return fmt.Errorf("%s create %s: %w: %s", e.Name, ref, err, firstLine(out+stderr))
	}
	id := strings.TrimSpace(firstLine(out))
	if id == "" {
		return fmt.Errorf("%s create %s printed no container id", e.Name, ref)
	}
	defer func() {
		rmCtx, cancelRm := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancelRm()
		// The container is this function's to remove and nothing else reads it,
		// so a failure here is reported and does not change the export's
		// verdict. It is discarded explicitly rather than by omission.
		if _, _, rmErr := Output(rmCtx, e.Path, "rm", "-f", id); rmErr != nil {
			log("could not remove the export container " + id + ": " + rmErr.Error())
		}
	}()

	log("exporting the rootfs of container " + id[:min(12, len(id))])
	f, err := os.OpenFile(tarPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	exportCtx, cancelExport := context.WithTimeout(ctx, 30*time.Minute)
	defer cancelExport()
	errBuf := &boundedBuffer{max: 64 << 10}
	cmd := newCommand(exportCtx, e.Path, "export", id)
	runErr := runCommand(exportCtx, cmd, nil, f, errBuf)
	closeErr := f.Close()
	if runErr != nil {
		_ = os.Remove(tarPath)
		return fmt.Errorf("%s export %s: %w: %s", e.Name, id, runErr, firstLine(errBuf.String()))
	}
	if closeErr != nil {
		return closeErr
	}
	size, ok := FileSize(tarPath)
	if !ok || size == 0 {
		// ⛔ An export that wrote nothing and exited 0 is the "step that exits 0
		// having done nothing" pattern. wsl --import would accept the empty
		// archive and register a distribution with no userland in it.
		_ = os.Remove(tarPath)
		return fmt.Errorf("%s export wrote an empty archive for %s", e.Name, ref)
	}
	log(fmt.Sprintf("rootfs: %s", HumanBytes(size)))
	return nil
}

// HumanBytes renders a byte count in binary units, labelled as binary.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 4; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTP"[exp])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
