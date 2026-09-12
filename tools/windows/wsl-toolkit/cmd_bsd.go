package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

func cmdBsd(ctx context.Context, args []string) (int, error) {
	if len(args) == 0 {
		return exitCannot, errors.New("bsd takes a subcommand: status, fetch or run")
	}
	switch args[0] {
	case "status":
		return cmdBsdStatus(ctx, args[1:])
	case "fetch":
		return cmdBsdFetch(ctx, args[1:])
	case "run":
		return cmdBsdRun(ctx, args[1:])
	}
	return exitCannot, fmt.Errorf("bsd has no subcommand %q. It takes status, fetch or run", args[0])
}

func cmdBsdStatus(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("bsd status")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	st := toolkit.BsdProbe(ctx)
	if *asJSON {
		return exitOK, writeJSON(st)
	}
	fmt.Println(map[bool]string{true: "ready", false: "not-ready"}[st.Ready])
	logf("  release     FreeBSD %s, amd64", toolkit.BsdRelease)
	logf("  accelerator whpx, the host's own hypervisor. No nesting and no elevation")
	if st.Qemu != "" {
		logf("  qemu        %s", st.Qemu)
		if st.QemuVersion != "" {
			logf("              %s", st.QemuVersion)
		}
	}
	logf("  whpx        %s", st.WhpxDetail)
	if st.Image != "" {
		logf("  image       %s, %s", st.Image, toolkit.HumanBytes(st.ImageBytes))
	}
	for _, p := range st.Problems {
		logf("  ! %s", p)
	}
	if !st.Ready {
		return exitCannot, nil
	}
	logf("\n  wsl-toolkit bsd run -c 'uname -a' runs one command in it.")
	return exitOK, nil
}

// cmdBsdFetch downloads the guest image and verifies it before it is usable.
//
// ⛔ THE DIGEST IS CHECKED BEFORE THE FILE IS PUT IN PLACE. A partial or
// substituted download that boots is worse than one that does not, and a file
// that arrived under the right name is not evidence of anything.
func cmdBsdFetch(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("bsd fetch")
	force := fs.Bool("force", false, "fetch again even when a verified image is already here")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	dir, err := toolkit.BsdDir()
	if err != nil {
		return exitCannot, err
	}
	target, err := toolkit.BsdImagePath()
	if err != nil {
		return exitCannot, err
	}
	if info, err := os.Stat(target); err == nil && !*force {
		if *asJSON {
			return exitOK, writeJSON(map[string]any{
				"schema": "wsl-toolkit-bsd-fetch/1", "image": target, "bytes": info.Size(), "fetched": false,
			})
		}
		fmt.Println(target)
		logf("  the image is already here, %s. Pass --force to fetch it again.", toolkit.HumanBytes(info.Size()))
		return exitOK, nil
	}
	if _, err := exec.LookPath("xz"); err != nil {
		return exitCannot, errors.New("xz is required to expand the published image and is not on PATH. Install it with: scoop install xz")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return exitCannot, err
	}
	archive := target + ".xz"
	logf("  fetching %s", toolkit.BsdImageURL)
	logf("  about 635 MB compressed, about 4 GB expanded")
	sum, err := fetchTo(ctx, toolkit.BsdImageURL, archive)
	if err != nil {
		return exitCannot, err
	}
	if sum != toolkit.BsdImagePinnedSha256 {
		_ = os.Remove(archive)
		return exitCannot, fmt.Errorf(
			"the published image does not match its pinned digest.\n  expected %s\n  measured %s\nThe partial file was removed and nothing was expanded",
			toolkit.BsdImagePinnedSha256, sum)
	}
	logf("  digest verified: %s", sum)
	logf("  expanding")
	// -k keeps the archive, so a re-run does not fetch 635 MB again.
	cmd := exec.CommandContext(ctx, "xz", "-dk", "-T0", "-f", filepath.Base(archive))
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return exitCannot, fmt.Errorf("expanding the image: %w", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return exitCannot, fmt.Errorf("the expanded image is not where it was expected: %w", err)
	}
	if *asJSON {
		return exitOK, writeJSON(map[string]any{
			"schema": "wsl-toolkit-bsd-fetch/1", "image": target, "bytes": info.Size(),
			"sha256": sum, "fetched": true,
		})
	}
	fmt.Println(target)
	logf("  %s expanded. wsl-toolkit bsd run -c 'uname -a' runs one command in it.", toolkit.HumanBytes(info.Size()))
	return exitOK, nil
}

// fetchTo downloads a URL to a file, RESUMING a partial one, and answers the
// SHA-256 of the complete result.
//
// ⛔ RESUMABLE, BECAUSE THE ALTERNATIVE IS PAYING 635 MB TWICE. The first
// version created the file, streamed into it and hashed as it went, so anything
// that interrupted it threw away everything already on disk. ⚠ It also carried
// no timeouts at all: measured on this host, it sat on a connection for
// twenty-five minutes having written nothing, and the only way to tell was to
// look at the file.
//
// ⚠ THE DIGEST IS TAKEN FROM THE FILE, not from the bytes that went past. With
// a resumed transfer this process never sees the earlier half, so hashing the
// stream would answer for a fragment and call it the file.
func fetchTo(ctx context.Context, url, dest string) (string, error) {
	var have int64
	if info, err := os.Stat(dest); err == nil {
		have = info.Size()
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
			IdleConnTimeout:       60 * time.Second,
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	// ⚠ A default Go user agent is refused or throttled by some mirrors, and the
	// symptom is a transfer that never starts rather than an error.
	req.Header.Set("User-Agent", "wsl-toolkit/"+versionString())
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	flags := os.O_CREATE | os.O_WRONLY
	switch resp.StatusCode {
	case http.StatusPartialContent:
		logf("  resuming at %s", toolkit.HumanBytes(have))
		flags |= os.O_APPEND
	case http.StatusOK:
		// The server ignored the range, or there was nothing to resume.
		have, flags = 0, flags|os.O_TRUNC
	case http.StatusRequestedRangeNotSatisfiable:
		// ⚠ THE ARCHIVE IS ALREADY WHOLE. Asking to resume at its own length is
		// what a run does when a previous fetch completed and the EXPANSION is
		// what failed, and the honest answer is the digest rather than an error
		// naming a status code. ⛔ Not trusted on its say-so: the digest is
		// still read off the file and still compared by the caller.
		logf("  the archive is already complete at %s", toolkit.HumanBytes(have))
		return sha256File(dest)
	default:
		return "", fmt.Errorf("%s answered %s", url, resp.Status)
	}
	f, err := os.OpenFile(dest, flags, 0o644)
	if err != nil {
		return "", err
	}
	total := have + resp.ContentLength
	_, copyErr := io.Copy(io.MultiWriter(f, &downloadProgress{have: have, total: total}), resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return sha256File(dest)
}

// sha256File reads a file back and answers its digest.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// downloadProgress says something every few seconds, because a 635 MB transfer
// that prints nothing is indistinguishable from one that has stopped.
type downloadProgress struct {
	have, total int64
	last        time.Time
}

func (p *downloadProgress) Write(b []byte) (int, error) {
	p.have += int64(len(b))
	if time.Since(p.last) < 5*time.Second {
		return len(b), nil
	}
	p.last = time.Now()
	if p.total > 0 {
		logf("  %s of %s", toolkit.HumanBytes(p.have), toolkit.HumanBytes(p.total))
	} else {
		logf("  %s", toolkit.HumanBytes(p.have))
	}
	return len(b), nil
}

func cmdBsdRun(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("bsd run")
	command := fs.String("c", "", "the command to run at a root shell in the guest")
	scriptFile := fs.String("script", "", "a file whose contents run at a root shell in the guest")
	timeout := fs.Duration("timeout", 15*time.Minute, "the whole session, the boot included")
	network := fs.Bool("network", false, "give the guest outbound user-mode networking. Nothing is forwarded inward")
	mem := fs.Int("memory", 2048, "guest memory in MiB")
	vcpus := fs.Int("cpus", 2, "guest processor count")
	noConsole := fs.Bool("no-console", false, "do not mirror the guest console while it boots")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	if *command != "" && *scriptFile != "" {
		return exitCannot, errors.New("-c and --script are two spellings of one argument, so passing both is refused")
	}
	var payload []byte
	var err error
	switch {
	case *command != "":
		payload, err = toolkit.RepairGuestScript([]byte(*command + "\n"))
	case *scriptFile != "":
		raw, readErr := os.ReadFile(*scriptFile)
		if readErr != nil {
			return exitCannot, readErr
		}
		payload, err = toolkit.RepairGuestScript(raw)
	default:
		return exitCannot, errors.New("nothing to run: pass -c COMMAND or --script FILE")
	}
	if err != nil {
		return exitCannot, err
	}

	var console io.Writer
	if !*noConsole && !quiet {
		// ⚠ The console goes to STDERR. It is progress, and stdout carries the
		// answer alone, so a caller can read one without the other.
		console = os.Stderr
	}
	logf("  booting FreeBSD %s under whpx. The first prompt takes about two minutes on this class of host.", toolkit.BsdRelease)
	res, runErr := toolkit.BsdRun(ctx, toolkit.BsdRunSpec{
		Script: payload, Timeout: *timeout,
		Network: *network, MemMiB: *mem, VCpus: *vcpus, Stdout: console,
	})
	if *asJSON {
		if err := writeJSON(res); err != nil {
			return exitCannot, err
		}
	} else if res.Output != "" {
		fmt.Println(res.Output)
	}
	if runErr != nil {
		return exitCannot, runErr
	}
	logf("  login at %s, session %s, exit %d",
		res.BootTime.Round(time.Second), res.Duration.Round(time.Second), res.Exit)
	return res.Exit, nil
}
