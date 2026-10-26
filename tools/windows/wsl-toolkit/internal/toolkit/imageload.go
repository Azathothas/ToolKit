// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// `base image load`: an image archive, or an image this machine's own engine
// holds, loaded into the base's engine with no registry between. WSL-130.
//
// ⭐ THE ARCHIVE TRAVELS ON STDIN, through the data channel a workspace copy
// uses, into `podman load` itself. podbox moved its images with `podman save`
// and `docker load` by hand, through a file it had to place in the guest first.

// ImageLoadSchema versions `base image load --json`.
const ImageLoadSchema = "wsl-toolkit-image-load/1"

// ImageLoad is what one load did.
type ImageLoad struct {
	Schema string `json:"schema"`
	// Source is the archive or the reference as the caller named it, and Kind
	// says which: file, or host-image for one this machine's engine saved.
	Source     string   `json:"source"`
	Kind       string   `json:"kind"`
	HostEngine string   `json:"host_engine,omitempty"`
	Distro     string   `json:"distro"`
	User       string   `json:"user"`
	Bytes      int64    `json:"bytes"`
	Images     []string `json:"images"`
	// Error is why the load failed, and empty where it succeeded.
	Error string `json:"error,omitempty"`
}

// The two kinds of source a load reads.
const (
	ImageSourceFile = "file"
	ImageSourceHost = "host-image"
)

// LoadImageArchive streams an archive on this machine into `podman load` in the
// base, as user, and answers the images it loaded and the bytes it sent.
func (b *Base) LoadImageArchive(ctx context.Context, archive, user string) ([]string, int64, error) {
	f, err := os.Open(archive)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()
	argv, err := b.loadArgv(ctx, user)
	if err != nil {
		return nil, 0, err
	}
	stdout := &boundedBuffer{max: 1 << 20}
	stderr := &boundedBuffer{max: 256 << 10}
	sent := &countingReader{r: f}
	code, err := b.wsl.ExecDirect(ctx, b.cfg.Base.Name, user, "", argv, sent, stdout, stderr, 60*time.Minute)
	if err != nil || code != 0 {
		return nil, sent.n, fmt.Errorf("podman load in %s exited %d: %s", b.cfg.Base.Name, code, guestFailure(stderr.String(), stdout.String()))
	}
	images := parseLoadedImages(stdout.String())
	if len(images) == 0 {
		return nil, sent.n, fmt.Errorf("podman load exited 0 and named no image it loaded: %s", guestFailure(stdout.String(), stderr.String()))
	}
	return images, sent.n, nil
}

// loadArgv is `podman load` as the account runs it, with no shell between.
//
// ⛔ THE ACCOUNT'S ENGINE IS ASKED THROUGH ITS OWN RUNTIME DIRECTORY, which the
// prologue every job's engine call starts with creates and checks first. podman
// run with another refuses its own state as `database configuration mismatch`.
func (b *Base) loadArgv(ctx context.Context, user string) ([]string, error) {
	if user == "root" {
		return []string{"/usr/bin/env", "podman", "load"}, nil
	}
	out, stderr, code, err := b.captureAs(ctx, user, []byte("printf '%s\\n' \"$XDG_RUNTIME_DIR\"\n"), nil, 2*time.Minute)
	if err != nil || code != 0 {
		return nil, fmt.Errorf("could not prepare %s's runtime directory (exit %d): %s", user, code, guestFailure(stderr, out))
	}
	run := strings.TrimSpace(firstLine(out))
	if !strings.HasPrefix(run, guestRunRootPrefix) {
		return nil, fmt.Errorf("the guest named %q as %s's runtime directory, which is not the engine's", run, user)
	}
	return []string{"/usr/bin/env", "XDG_RUNTIME_DIR=" + run, "podman", "load"}, nil
}

// parseLoadedImages reads the references `podman load` names: `Loaded image:
// REF`, or `Loaded image(s): A, B` from an older podman. ⭐ A function with no
// process in it, so its case runs on any host.
func parseLoadedImages(out string) []string {
	var images []string
	for _, line := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(strings.TrimRight(line, "\r")), "Loaded image")
		if !ok {
			continue
		}
		_, list, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		for _, ref := range strings.Split(list, ",") {
			if ref = strings.TrimSpace(ref); ref != "" {
				images = append(images, ref)
			}
		}
	}
	return images
}

// SaveHostImage saves an image this machine's engine holds to an archive in the
// state directory, and answers the archive, the engine's name, and how to
// remove the archive once it is loaded.
func SaveHostImage(ctx context.Context, ref string) (string, string, func(), error) {
	if err := ValidateImageRef(ref); err != nil {
		return "", "", nil, err
	}
	home, err := EnsureHome()
	if err != nil {
		return "", "", nil, err
	}
	// ⛔ THE SPACE BEFORE THE ENGINE. A save wrote its archive into the state
	// directory with no check at all, as a build's export did. The archive's
	// size is the engine's to say, so this is the floor a throwaway's archive
	// keeps. WSL-180.
	if err := spaceFloor(home, ThrowawaySpaceFloor, 0, "before an image is saved into it", "Nothing was saved"); err != nil {
		return "", "", nil, err
	}
	engine, err := FindEngine(ctx)
	if err != nil {
		return "", "", nil, fmt.Errorf("an image that is not a file is read from this machine's engine, and there is none: %w", err)
	}
	dir, err := os.MkdirTemp(home, "image-load-")
	if err != nil {
		return "", "", nil, err
	}
	cleanup := func() { _ = RemoveInside(home, dir) }
	archive := filepath.Join(dir, "image.tar")
	if err := engine.SaveImage(ctx, ref, archive); err != nil {
		cleanup()
		return "", engine.Name, nil, err
	}
	if info, err := os.Stat(archive); err != nil || info.Size() == 0 {
		cleanup()
		return "", engine.Name, nil, errors.New(engine.Name + " save exited 0 and wrote no archive for " + ref)
	}
	return archive, engine.Name, cleanup, nil
}

// countingReader counts what passes through it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
