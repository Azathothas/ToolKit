package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Engine is a container engine on the HOST, used for exactly one job: turning
// an OCI image into a rootfs archive that wsl --import can read.
//
// ⚠ It is not the engine jobs run in. Jobs run in a rootless podman INSIDE the
// owned distribution, so nothing a job does can reach the machine's own engine
// or the images somebody else put in it.
type Engine struct {
	Name string // podman, docker or wslc
	Path string
	Arch string // the platform token this engine accepts, normalised
}

// hostEngineNames is the order a host engine is looked for.
//
// ⭐ wslc IS LAST, so a machine with podman or docker keeps the engine it had.
// It is WSL's own container tool, from WSL 3.0.1, so a machine with nothing else
// installed can still build a base. Ruled on 2026-10-21, ruling 30. WSL-110.
var hostEngineNames = []string{"podman", "docker", "wslc"}

// FindEngine picks the host engine, preferring podman.
func FindEngine(ctx context.Context) (*Engine, error) {
	// ⛔ EVERY CANDIDATE'S REASON IS KEPT. Reporting only the last one names
	// whichever engine happened to be tried second, so a broken podman reads as
	// a missing docker and the next reader looks in the wrong place. That is
	// exactly what this function did on its first run here.
	var problems []string
	for _, name := range hostEngineNames {
		exe, err := ResolveExecutable(name)
		if err != nil {
			problems = append(problems, name+": "+err.Error())
			continue
		}
		e := &Engine{Name: name, Path: exe.Resolved}
		arch, err := e.readArch(ctx)
		if err != nil {
			why := fmt.Sprintf("%s is installed at %s and did not answer: %v", name, exe.Resolved, err)
			// ⭐ PODMAN GETS A SECOND SENTENCE, because its own advice for this
			// state sends the reader to two commands that cannot help.
			if name == "podman" {
				if hint := DiagnosePodman(ctx, exe.Resolved, err); hint != "" {
					why += ". " + hint
				}
			}
			problems = append(problems, why)
			continue
		}
		e.Arch = arch
		return e, nil
	}
	if len(problems) == 0 {
		problems = append(problems, "none of podman, docker and wslc is installed")
	}
	return nil, fmt.Errorf("no usable container engine on this host: %s", strings.Join(problems, "; "))
}

// wslcVersion is the liveness question asked of wslc. ⚠ wslc's `info` names no
// architecture, and its containers run in a VM on this machine's own, so the
// architecture is this machine's and the call proves only that wslc answers.
func (e *Engine) wslcVersion(ctx context.Context) error {
	out, stderr, err := (engineCall{Timeout: 90 * time.Second, What: "wslc version"}).run(ctx, e.Path, "version")
	if err != nil {
		return fmt.Errorf("wslc version: %w: %s", err, firstLine(stderr))
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "wslc ") {
		return fmt.Errorf("wslc version answered %q, which is not a version", firstLine(out))
	}
	return nil
}

// pullArgs, createArgs and removeArgs are the one place the engines' spellings
// differ for an export.
//
// ⛔ wslc TAKES NO --platform, on its pull or its create, measured on wslc
// 3.0.3. So what it pulled is read back by checkArch before anything is created,
// rather than named and trusted. WSL-110.
func (e *Engine) pullArgs(ref string) []string {
	if e.Name == "wslc" {
		return []string{"pull", ref}
	}
	return []string{"pull", "--platform", e.Platform(), ref}
}

func (e *Engine) createArgs(ref string) []string {
	if e.Name == "wslc" {
		return []string{"create", ref}
	}
	return []string{"create", "--platform", e.Platform(), ref}
}

// wslcImage is the part of `wslc image inspect --format json` this tool reads.
type wslcImage struct {
	Architecture string      `json:"Architecture"`
	Os           string      `json:"Os"`
	Config       imageConfig `json:"Config"`
}

// inspectWslcImage reads one image's architecture and configuration from wslc.
func (e *Engine) inspectWslcImage(ctx context.Context, ref string) (wslcImage, error) {
	out, stderr, err := (engineCall{What: "wslc image inspect " + ref}).run(ctx, e.Path, "image", "inspect", "--format", "json", ref)
	if err != nil {
		return wslcImage{}, fmt.Errorf("wslc image inspect %s: %w: %s", ref, err, firstLine(stderr))
	}
	var images []wslcImage
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &images); err != nil || len(images) != 1 {
		return wslcImage{}, fmt.Errorf("wslc image inspect %s answered %d image(s) that do not parse as one: %v", ref, len(images), err)
	}
	return images[0], nil
}

// checkArch refuses an image whose architecture is not the one this engine runs.
// ⛔ A ROOTFS FOR ANOTHER ARCHITECTURE IMPORTS CLEANLY, and then nothing in it
// executes. WSL-64 named the variant on every fetch; an engine that cannot name
// it has to be read back instead.
func (e *Engine) checkArch(ctx context.Context, ref string) error {
	if e.Name != "wslc" {
		return nil
	}
	img, err := e.inspectWslcImage(ctx, ref)
	if err != nil {
		return err
	}
	return wslcArchVerdict(ref, img, e.Arch)
}

// wslcArchVerdict is the rule checkArch applies, with no process in it.
func wslcArchVerdict(ref string, img wslcImage, want string) error {
	if !strings.EqualFold(img.Os, "linux") || !strings.EqualFold(img.Architecture, want) {
		return fmt.Errorf("wslc pulled %s for %s/%s, and this machine runs linux/%s. wslc takes no --platform, so it cannot be asked for another; "+
			"remove the image with `wslc rmi %s` and pull it again, or use podman or docker", ref, img.Os, img.Architecture, want, ref)
	}
	return nil
}

// ImageConfig reads an image's configuration, ENV and WORKDIR, from the engine.
func (e *Engine) ImageConfig(ctx context.Context, ref string) (imageConfig, error) {
	if e.Name == "wslc" {
		img, err := e.inspectWslcImage(ctx, ref)
		return img.Config, err
	}
	out, stderr, err := (engineCall{What: e.Name + " image inspect " + ref}).run(ctx, e.Path, "image", "inspect", ref, "--format", "{{json .Config}}")
	if err != nil {
		return imageConfig{}, fmt.Errorf("%s image inspect %s: %w: %s", e.Name, ref, err, firstLine(stderr))
	}
	var cfg imageConfig
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &cfg); err != nil {
		return imageConfig{}, fmt.Errorf("the configuration of %s does not parse: %w", ref, err)
	}
	return cfg, nil
}

// readArch asks the engine what architecture it runs, and normalises the answer.
//
// ⛔ THE TWO ENGINES SPELL THE FIELD DIFFERENTLY AND ASKING THE WRONG ONE FAILS
// THE WHOLE CALL. podman answers to {{.Host.Arch}} and docker to
// {{.Architecture}}; asking podman for the lower-case spelling returns
// "can't evaluate field host", which reads as a broken engine and is a wrong
// template.
//
// ⚠ They also disagree on the VALUE. podman answers amd64 and docker answers
// x86_64, and only the first is a token --platform accepts. A value neither
// recognises is passed through rather than guessed at, so the engine refuses it
// by name instead of this code inventing one.
func (e *Engine) readArch(ctx context.Context) (string, error) {
	if e.Name == "wslc" {
		if err := e.wslcVersion(ctx); err != nil {
			return "", err
		}
		return runtime.GOARCH, nil
	}
	field := "{{.Host.Arch}}"
	if e.Name == "docker" {
		field = "{{.Architecture}}"
	}
	// ⚠ 90 SECONDS RATHER THAN THE DEFAULT, and it is a measurement: `info` on
	// a machine that has just started is the slowest question this tool asks.
	out, stderr, err := (engineCall{Timeout: 90 * time.Second, What: e.Name + " info"}).
		run(ctx, e.Path, "info", "--format", field)
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

// ExportedImage is what a base's export read about the image it exported.
type ExportedImage struct {
	// Digest is the image's digest as the engine named it, or empty where it
	// named none. WSL-186.
	Digest string
}

// ExportRootfs turns an image reference into a rootfs archive on the host.
//
// It is the one place this executable talks to the host engine, and it removes
// the container it created whether or not the export succeeded.
func (e *Engine) ExportRootfs(ctx context.Context, ref, tarPath string, log func(string)) error {
	_, err := e.export(ctx, ref, tarPath, log, false)
	return err
}

// ExportBaseRootfs is ExportRootfs for the base, which records what it was built
// from. It reads the image's digest. ⭐ Where the pull fails, it exports the copy
// the engine already holds for this machine's platform, and says so. A host the
// registry does not answer could not rebuild its base at all. WSL-186, WSL-192.
func (e *Engine) ExportBaseRootfs(ctx context.Context, ref, tarPath string, log func(string)) (ExportedImage, error) {
	return e.export(ctx, ref, tarPath, log, true)
}

// export is ExportRootfs, and ExportBaseRootfs where forBase is set.
func (e *Engine) export(ctx context.Context, ref, tarPath string, log func(string), forBase bool) (ExportedImage, error) {
	var got ExportedImage
	if err := ValidateImageRef(ref); err != nil {
		return got, err
	}
	// ⭐ THE STALL IS SIZED BEFORE THE PULL, from the layers it will move, and
	// the line says how long the pull may be silent and why. WSL-176.
	// ⚠ THE READ IS SAID BEFORE IT IS MADE. It is bounded, and a registry that
	// is slow to answer was a silence before the first line. WSL-176.
	log(fmt.Sprintf("reading the layers of %s from its registry, for at most %s, to size the pull's limits", ref, FormatSpan(ManifestReadTimeout)))
	lim, err := e.pullLimits(ctx, ref)
	if err != nil {
		return got, err
	}
	log(fmt.Sprintf("pulling %s for %s. It may be silent for %s, %s", ref, e.Platform(), FormatSpan(lim.Stall), lim.Why))
	// ⛔ NEVER EMIT NOTHING: RENDER SILENCE, WITH A TIME ON IT. The pull is
	// bounded at thirty minutes or more below, which is the right ceiling and the whole
	// of what this used to say. Between the line before and the pull returning,
	// a caller saw NOTHING, so a stalled pull and a slow one were the same
	// picture for up to half an hour.
	//
	// ⚠ MEASURED 2026-09-17, and it is why this exists. A
	// `podman pull ghcr.io/pkgforge-dev/archlinux:latest` on this host sat for
	// 28 minutes with ZERO bytes read, ZERO written and ZERO processor time over
	// a 25-second window; the same pull, run again minutes later, finished in
	// 221 seconds. The stall is not reproducible and is not diagnosed. The
	// silence was ours, and this is WSL-18's own rule applied where it was not.
	stopBeat := beat(ctx, log, "still pulling "+ref)
	// ⭐ THE STALL DEADLINE IS WHAT CATCHES A HANG. The total ceiling has to be
	// set for the worst legitimate pull, which makes it useless against a pull
	// that has simply stopped; this one asks whether anything arrived recently.
	// ⚠ A pull is silent while a layer downloads, so its stall is pullLimits',
	// and the hint says a slow link reads the same. WSL-176.
	out, stderr, err := (engineCall{
		Timeout: lim.Total, Stall: lim.Stall,
		What: e.Name + " pull " + ref,
		Hint: pullStallHint(e.Name, lim),
	}).run(ctx, e.Path, e.pullArgs(ref)...)
	stopBeat()
	if err != nil {
		pullErr := fmt.Errorf("%s pull %s: %w: %s", e.Name, ref, err, guestFailure(out, stderr))
		// ⚠ A CALL THAT WAS STOPPED IS NOT A PULL THAT FAILED, so nothing stands in
		// for it.
		if !forBase || ctx.Err() != nil {
			return got, pullErr
		}
		// ⛔ A FAILED PULL ENDED THE REBUILD, even where the engine already held the
		// image for this machine. So a host the registry does not answer could not
		// rebuild its base at all. The engine's own copy is used now, and the line
		// says it is a local copy, and which digest. WSL-192.
		img, localErr := e.localImage(ctx, ref)
		if localErr != nil {
			return got, fmt.Errorf("%w%s %v", pullErr, stopAfter(pullErr.Error()), localErr)
		}
		got = ExportedImage{Digest: img.digestFor(ref)}
		log("⚠ " + pullErr.Error())
		log(fmt.Sprintf("⚠ building from the copy of %s that %s already holds, for linux/%s, %s. It may be older than the registry's",
			ref, e.Name, e.Arch, digestWords(got.Digest)))
	} else {
		if err := e.checkArch(ctx, ref); err != nil {
			return got, err
		}
		if forBase {
			got.Digest = e.digestOf(ctx, ref, log)
		}
	}

	out, stderr, err = (engineCall{Timeout: 5 * time.Minute, What: e.Name + " create " + ref}).
		run(ctx, e.Path, e.createArgs(ref)...)
	if err != nil {
		return got, fmt.Errorf("%s create %s: %w: %s", e.Name, ref, err, guestFailure(out, stderr))
	}
	id := strings.TrimSpace(firstLine(out))
	if id == "" {
		return got, fmt.Errorf("%s create %s printed no container id", e.Name, ref)
	}
	defer func() {
		// The container is this function's to remove and nothing else reads it,
		// so a failure here is reported and does not change the export's
		// verdict. It is discarded explicitly rather than by omission.
		//
		// ⚠ WithoutCancel, because the caller's context may already have ended
		// and this cleanup still has to run; the deadline is the engine call's.
		if _, _, rmErr := (engineCall{What: e.Name + " rm " + id}).
			run(context.WithoutCancel(ctx), e.Path, "rm", "-f", id); rmErr != nil {
			log("could not remove the export container " + id + ": " + rmErr.Error())
		}
	}()

	log("exporting the rootfs of container " + id[:min(12, len(id))])
	f, err := os.OpenFile(tarPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return got, err
	}
	errBuf := &boundedBuffer{max: 64 << 10}
	// ⭐ THE EXPORT GETS THE STALL DEADLINE TOO. It is the other call here that
	// moves bytes for minutes, and a rootfs export that stops is exactly as
	// invisible as a pull that stops.
	runErr := (engineCall{
		Timeout: EngineTransferTimeout, Stall: EngineStallTimeout,
		What: e.Name + " export " + id[:min(12, len(id))],
		Hint: "An export writes a rootfs to this host and touches no registry, so read disk space and the engine machine",
	}).runStream(ctx, f, errBuf, e.Path, "export", id)
	closeErr := f.Close()
	if runErr != nil {
		_ = os.Remove(tarPath)
		return got, fmt.Errorf("%s export %s: %w: %s", e.Name, id, runErr, guestFailure(errBuf.String()))
	}
	if closeErr != nil {
		return got, closeErr
	}
	size, ok := FileSize(tarPath)
	if !ok || size == 0 {
		// ⛔ An export that wrote nothing and exited 0 is the "step that exits 0
		// having done nothing" pattern. wsl --import would accept the empty
		// archive and register a distribution with no userland in it.
		_ = os.Remove(tarPath)
		return got, fmt.Errorf("%s export wrote an empty archive for %s", e.Name, ref)
	}
	log(fmt.Sprintf("rootfs: %s", HumanBytes(size)))
	return got, nil
}

// engineImage is the part of an engine's `image inspect` this tool reads. podman
// and docker answer a list of one by default, and wslc with `--format json`.
type engineImage struct {
	// Digest is podman's digest of the image for this platform. docker and wslc
	// name none here.
	Digest string `json:"Digest"`
	// RepoDigests is each repository and digest the image was pulled by, as
	// `REPOSITORY@sha256:HEX`.
	RepoDigests  []string `json:"RepoDigests"`
	Architecture string   `json:"Architecture"`
	Os           string   `json:"Os"`
}

// inspectImage reads one image from the engine's own store. It pulls nothing.
func (e *Engine) inspectImage(ctx context.Context, ref string) (engineImage, error) {
	args := []string{"image", "inspect", ref}
	if e.Name == "wslc" {
		args = []string{"image", "inspect", "--format", "json", ref}
	}
	out, stderr, err := (engineCall{What: e.Name + " image inspect " + ref}).run(ctx, e.Path, args...)
	if err != nil {
		return engineImage{}, fmt.Errorf("%s image inspect %s: %w: %s", e.Name, ref, err, guestFailure(out, stderr))
	}
	var images []engineImage
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &images); err != nil || len(images) != 1 {
		return engineImage{}, fmt.Errorf("%s image inspect %s answered %d image(s) that do not parse as one: %v", e.Name, ref, len(images), err)
	}
	return images[0], nil
}

// localImage is the engine's own copy of ref, where it holds one for this
// machine's platform. WSL-192.
//
// ⛔ THE PLATFORM IS READ BACK. The store is keyed by tag and not by
// architecture, so the copy under a tag can be another platform's after one
// `--platform` pull. Imported, it registers cleanly and nothing in it runs.
func (e *Engine) localImage(ctx context.Context, ref string) (engineImage, error) {
	img, err := e.inspectImage(ctx, ref)
	if err != nil {
		return img, fmt.Errorf("%s holds no copy of it to build from: %v", e.Name, err)
	}
	if !strings.EqualFold(img.Os, "linux") || !strings.EqualFold(img.Architecture, e.Arch) {
		return img, fmt.Errorf("the copy %s holds is for %s/%s, and this machine runs linux/%s, so it is not built from",
			e.Name, img.Os, img.Architecture, e.Arch)
	}
	return img, nil
}

// digestOf is the digest the engine names for an image it has just pulled, or
// empty, said either way. ⚠ A digest that cannot be read leaves the build as it
// was before digests were recorded. Refusing a base over it would trade a working
// build for a field. WSL-186.
func (e *Engine) digestOf(ctx context.Context, ref string, log func(string)) string {
	img, err := e.inspectImage(ctx, ref)
	if err != nil {
		log("⚠ the digest of " + ref + " could not be read, so none is recorded: " + err.Error())
		return ""
	}
	d := img.digestFor(ref)
	if d == "" {
		log("⚠ " + e.Name + " names no digest for " + ref + ", so none is recorded")
		return ""
	}
	log("digest: " + d)
	return d
}

// digestFor is the digest img names for ref. It is the engine's own digest where
// it names one, as podman does, or else the digest of ref's repository among its
// repository digests, as docker names it.
//
// ⚠ THE TWO ARE NOT THE SAME KIND, and either pins a pull to this image. Measured
// with the base's own podman and docker.io/library/alpine:latest: `Digest` is the
// amd64 manifest's, and `RepoDigests` holds it beside the index's. WSL-186.
func (img engineImage) digestFor(ref string) string {
	if isImageDigest(img.Digest) {
		return img.Digest
	}
	want := repositoryOf(ref)
	for _, rd := range img.RepoDigests {
		name, d, ok := strings.Cut(rd, "@")
		if ok && isImageDigest(d) && repositoryOf(name) == want {
			return d
		}
	}
	return ""
}

// isImageDigest says whether s is a sha256 digest as an image reference spells it.
func isImageDigest(s string) bool {
	hex, ok := strings.CutPrefix(s, "sha256:")
	if !ok || len(hex) != 64 {
		return false
	}
	for _, r := range hex {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// repositoryOf is an image name's registry and repository, with no tag and no
// digest, the way an engine resolves a short name: `alpine` is
// `docker.io/library/alpine`.
func repositoryOf(name string) string {
	name, _, _ = strings.Cut(name, "@")
	if colon := strings.LastIndex(name, ":"); colon > strings.LastIndex(name, "/") {
		name = name[:colon]
	}
	host, rest, ok := strings.Cut(name, "/")
	if !ok || (!strings.ContainsAny(host, ".:") && host != "localhost") {
		host, rest = "docker.io", name
	}
	if isDockerHub(host) {
		host = "docker.io"
		if !strings.Contains(rest, "/") {
			rest = "library/" + rest
		}
	}
	return strings.ToLower(host + "/" + rest)
}

// PinnedImage is image at digest, as `base.image` would name it to build that
// image again. ⚠ Empty where digest is.
func PinnedImage(image, digest string) string {
	if !isImageDigest(digest) {
		return ""
	}
	return repositoryOf(image) + "@" + digest
}

// digestWords is a digest as a sentence names it.
func digestWords(digest string) string {
	if digest == "" {
		return "with no digest the engine names"
	}
	return "digest " + digest
}

// PullBeatEvery is how often a long engine call says it is still there.
//
// ⚠ IT IS NOT A PROGRESS BAR AND CANNOT BECOME ONE. podman writes its progress
// to stderr and Output CAPTURES that, so nothing here knows how many bytes have
// moved. What it can say honestly is how long it has been waiting, which is the
// difference between "this is slow" and "this has stopped".
const PullBeatEvery = 30 * time.Second

// beat writes one line every PullBeatEvery until the returned function is
// called, and returns a function that is safe to call exactly once.
//
// ⭐ THE ELAPSED TIME IS THE CONTENT. A heartbeat that only said "still working"
// would be the watcher whose output is the thing it is watching, which
// docs/conventions/forbidden-patterns.md already has a row for.
func beat(ctx context.Context, log func(string), what string) func() {
	if log == nil {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	started := time.Now()
	go func() {
		defer close(done)
		t := time.NewTicker(PullBeatEvery)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case now := <-t.C:
				log(fmt.Sprintf("%s: %s so far, no output yet", what, FormatSpan(now.Sub(started))))
			}
		}
	}()
	// ⛔ THE STOP WAITS, for the reason tick.go gives: a line already in flight
	// would otherwise print after the one saying the step finished.
	var once sync.Once
	return func() {
		once.Do(func() { close(stop) })
		<-done
	}
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

// ParseHumanBytes reads a size the way an engine prints one, and it lives
// beside HumanBytes because it is its inverse and the two vocabularies have to
// agree.
//
// ⛔ THE TWO UNIT FAMILIES MEAN DIFFERENT NUMBERS AND BOTH ARE IN USE. podman's
// `{{.MemUsage}}` goes through docker's units.HumanSize, which is DECIMAL: the
// `33.44GB` measured on this host on 2026-09-17 is 33,440,000,000 bytes, which
// is 31.1 GiB on a 32 GiB machine. Reading it as binary would report a tenth
// less memory than there is, with nothing saying so. A `iB` suffix is binary,
// a bare `B` suffix is decimal, and that is the rule both tools follow.
func ParseHumanBytes(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	if err != nil || n < 0 {
		return 0, false
	}
	suffix := strings.TrimSpace(s[i:])
	binary := strings.HasSuffix(suffix, "iB")
	letter := strings.TrimSuffix(strings.TrimSuffix(suffix, "iB"), "B")
	step := float64(1000)
	if binary {
		step = 1024
	}
	mul := float64(1)
	switch strings.ToUpper(strings.TrimSpace(letter)) {
	case "":
	case "K":
		mul = step
	case "M":
		mul = step * step
	case "G":
		mul = step * step * step
	case "T":
		mul = step * step * step * step
	case "P":
		mul = step * step * step * step * step
	default:
		return 0, false
	}
	// ⛔ ROUNDED, NOT TRUNCATED. 33.44 times a billion is 33439999999.999996 in
	// binary floating point, and int64() of that is 33,439,999,999 - a byte
	// short of the figure the engine printed, every time, with nothing saying
	// so. It is docs/conventions/shell.md section 8's [int](2.65) in Go, and
	// the case is what found it.
	return int64(math.Round(n * mul)), true
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// podmanSocketFailure matches a podman that started and cannot be talked to.
//
// ⚠ THE TWO SPELLINGS ARE ONE STATE. The transport error surfaces as an SSH
// channel refusal when the machine is up and the socket behind it is not
// served, and as a plain dial refusal when the forward itself is gone.
var podmanSocketFailure = regexp.MustCompile(`(?i)unable to connect to podman socket|ssh: rejected|connection refused|actively refused|cannot connect to podman`)

// DiagnosePodman explains a podman that is installed and will not answer, and
// names a recovery it has MEASURED rather than one it believes.
//
// ⛔ PODMAN'S OWN ADVICE IS WRONG FOR THIS STATE AND A CONSUMER WILL BLAME US.
// When the default connection's socket is not served, podman says to try
// `podman machine init` and `podman machine start`. The machine is already
// running, so start does nothing, and init would build a second machine. The
// reader runs both, neither helps, and the tool that relayed that advice is the
// one they came from. Measured on this host 2026-09-17: the machine reported
// Running, `podman machine ssh` worked, and `user@1000.service` had failed with
// "Failed to spawn executor: Device or resource busy" on systemd 259, so the
// ROOTLESS socket was never created while the ROOTFUL one was healthy.
//
// ⭐ IT NAMES A CONNECTION IT HAS JUST DRIVEN. podman registers a root
// connection beside the rootless one for every machine, and when the rootless
// user manager is dead the root one usually answers. Trying each and reporting
// the one that worked turns a dead end into one command.
//
// ⚠ IT RUNS ONLY ON THE FAILURE PATH and is bounded, so a healthy host pays
// nothing for it. An empty answer means nothing was recognised, and the caller
// then reports podman's own words alone rather than a guess of ours.
func DiagnosePodman(ctx context.Context, exe string, failure error) string {
	if failure == nil || !podmanSocketFailure.MatchString(failure.Error()) {
		return ""
	}
	bounded, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	running, machine := podmanMachineRunning(bounded, exe)
	return podmanHint(running, machine, podmanWorkingConnection(bounded, exe))
}

// podmanHint is what the diagnosis SAYS, with no process in it.
//
// ⛔ SEPARATED FROM THE MEASURING so the sentences can be driven on a host with
// no podman at all. They could not be before, and repo mutate said so: removing
// the whole guard left the cases green, because every one of them asserted only
// that the diagnosis stayed SILENT and none that it ever spoke.
func podmanHint(running bool, machine, conn string) string {
	var notes []string
	if running {
		notes = append(notes, fmt.Sprintf("the machine %q reports Running, so `podman machine start` will not help and "+
			"`podman machine init` would build a SECOND machine", machine))
	}
	if conn != "" {
		notes = append(notes, fmt.Sprintf("connection %q DOES answer, so the socket is fine and the default connection is "+
			"the broken half. Run: podman system connection default %s", conn, conn))
	} else {
		notes = append(notes, "no registered connection answers. Inside the machine, `systemctl is-active user@1000.service` "+
			"reports whether the user manager that serves the rootless socket came up at all; a failed one leaves no socket "+
			"to connect to and no amount of restarting the machine creates one")
	}
	return strings.Join(notes, "; ")
}

// podmanMachineRunning reports whether any machine is running, and its name.
func podmanMachineRunning(ctx context.Context, exe string) (bool, string) {
	out, _, err := (engineCall{What: "podman machine list"}).run(ctx, exe, "machine", "list", "--format", "{{.Name}} {{.Running}}")
	if err != nil {
		return false, ""
	}
	for _, ln := range strings.Split(out, "\n") {
		fields := strings.Fields(ln)
		if len(fields) == 2 && strings.EqualFold(fields[1], "true") {
			return true, strings.TrimSuffix(fields[0], "*")
		}
	}
	return false, ""
}

// podmanWorkingConnection returns the first registered connection that answers.
//
// ⛔ IT DRIVES EACH ONE rather than reading a status field. A connection listed
// as present says nothing about whether the socket behind it is served, which is
// the entire failure being diagnosed.
func podmanWorkingConnection(ctx context.Context, exe string) string {
	out, _, err := (engineCall{What: "podman system connection list"}).run(ctx, exe, "system", "connection", "list", "--format", "{{.Name}}")
	if err != nil {
		return ""
	}
	for _, ln := range strings.Split(out, "\n") {
		name := strings.TrimSpace(ln)
		if name == "" || strings.EqualFold(name, "name") {
			continue
		}
		// ⚠ FIFTEEN SECONDS, NOT THE DEFAULT. This runs once per registered
		// connection while the engine is already known to be broken, so a
		// diagnosis over four dead connections must not cost four ceilings.
		_, _, probeErr := (engineCall{Timeout: 15 * time.Second, What: "podman --connection " + name + " info"}).
			run(ctx, exe, "--connection", name, "info", "--format", "{{.Host.Arch}}")
		if probeErr == nil {
			return name
		}
	}
	return ""
}

// SaveImage writes an image this machine's engine holds to an archive file, for
// `base image load` to carry into the base. WSL-130.
//
// ⚠ NO STALL DEADLINE, because `docker save` writes its archive and prints
// nothing while it does, so a quiet save is a working one. The transfer ceiling
// still bounds it. `-o` is the spelling all three engines read.
func (e *Engine) SaveImage(ctx context.Context, ref, path string) error {
	_, stderr, err := (engineCall{Timeout: EngineTransferTimeout, What: e.Name + " save " + ref}).
		run(ctx, e.Path, "save", "-o", path, ref)
	if err != nil {
		return fmt.Errorf("%s save %s: %w: %s", e.Name, ref, err, guestFailure(stderr))
	}
	return nil
}
