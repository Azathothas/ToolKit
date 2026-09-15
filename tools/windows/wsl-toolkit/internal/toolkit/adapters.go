// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ⭐ AN ADAPTER IS ONE PIECE OF SOFTWARE A BASE RUNS FOR ITS AGENTS, installed and
// proved by this tool. A configuration names it in base.adapters, `base ensure`
// applies it after provisioning, and `base status --probe` reads it back.
//
// ⛔ ITS DEFINITION HAS ONE HOME AND A GENERATED COPY. The home is
// tools/windows/wsl-toolkit/adapters/NAME/, where a reader finds it beside the
// examples; go:embed cannot reach out of this package, so the copy under
// ./adapters/ is what the executable carries, and the gate's `adapters` rule
// refuses the two disagreeing. TODO/RULES.md section 4, and the ruling in WSL-77.
//
// Each definition is a directory: install.sh runs as root in the base, probe.sh
// runs as root and prints `name value` lines with a `problem TEXT` line for each
// thing wrong, and every other file reaches install.sh base64-encoded in
// TK_FILE_<NAME>_B64. An adapter may also have a half on this machine, adapterHost.
//
//go:embed adapters
var adapterTree embed.FS

// BaseAdapter is one adapter a base configuration names.
type BaseAdapter struct {
	Name string `json:"name"`
	// InstallerSHA256 is an installer digest the operator approved after reading
	// the file, for an adapter that runs a provider's own installer. ⛔ It adds to
	// the digest pinned in the adapter and replaces nothing, and an adapter that
	// runs no installer refuses it rather than ignoring it.
	InstallerSHA256 string `json:"installer_sha256,omitempty"`
	// Version moves an adapter to a release its script does not pin, without
	// editing the script or rebuilding this executable.
	//
	// ⛔ A VERSION WITHOUT DIGESTS IS REFUSED. Taking a version from a
	// configuration and a digest from the script would download one release and
	// check it against another's, which fails confusingly at best and silently
	// installs nothing at worst. The two move together or neither moves.
	Version string `json:"version,omitempty"`
	// SHA256 is the digest of that release's asset for each machine architecture,
	// keyed as `uname -m` spells it: `x86_64`, `aarch64`.
	SHA256 map[string]string `json:"sha256,omitempty"`
	// Channel makes an adapter follow the newest build of a channel rather than the
	// release it pins. The one channel is `nightly`, herdr's nightly prereleases of this
	// repository. ⛔ A channel beside a version or digests is refused: the two answer the
	// same question differently. WSL-90.
	Channel string `json:"channel,omitempty"`
}

// AdapterState is one adapter as `base status --probe` read it back.
type AdapterState struct {
	Name     string            `json:"name"`
	Healthy  bool              `json:"healthy"`
	Version  string            `json:"version,omitempty"`
	Facts    map[string]string `json:"facts,omitempty"`
	Problems []string          `json:"problems,omitempty"`
}

// adapterSpec is what this executable knows about an adapter beyond its files.
type adapterSpec struct {
	Name    string
	Summary string
	// NeedsSystemd refuses a configuration that names the adapter without it.
	NeedsSystemd bool
	// Presets are the ones the adapter was driven on. ⛔ A base from any other
	// image is refused rather than guessed at.
	Presets []string
	// TakesInstallerDigest says the adapter runs a provider's installer, and reads
	// an operator's approval of one as TK_INSTALLER_SHA256.
	TakesInstallerDigest bool
	// TakesVersionPin says the adapter downloads a release it pins by version and
	// digest, so a configuration may move it to another without editing the script
	// or rebuilding this executable. It reads TK_ADAPTER_VERSION and
	// TK_ADAPTER_SHA256_<ARCH>.
	TakesVersionPin bool
	// TakesChannel says the adapter may follow AdapterChannelNightly.
	TakesChannel bool
	// Agent is the command an agent adapter installs in the base, which `base agent`
	// and the Windows launcher run, or "" for an adapter that is not an agent.
	Agent string
	// Host is the half that lives on this machine, or nil.
	Host adapterHost
}

// adapterSpecs is every adapter this executable carries.
var adapterSpecs = []adapterSpec{
	{
		Name: "herdr",
		// ⛔ NO VERSION IN THIS SENTENCE. The adapter pins one and a configuration
		// may move it, so a number here would be a second home for that fact and
		// the one nobody updates. `base status --probe` reads back what is there.
		Summary:         "herdr with its server as a system unit, and an SSH door the operator's Windows herdr client attaches through",
		NeedsSystemd:    true,
		Presets:         []string{"arch"},
		TakesVersionPin: true,
		TakesChannel:    true,
		Host:            &herdrHost{},
	},
	{
		Name:                 "muse",
		Summary:              "Muse Code for the base's account, from Meta's installer, run only while that file's digest is one the operator approved, a launcher on Windows, and a herdr reporter for its lifecycle",
		Presets:              []string{"arch"},
		TakesInstallerDigest: true,
		Agent:                "muse",
		Host:                 &agentLauncherHost{agent: "muse"},
	},
	{
		Name: "pi",
		// ⭐ NO DIGEST PIN AND NO INSTALLER APPROVAL, and the difference from muse is
		// the install route rather than a lower standard: pi is an npm package, and
		// npm checks a package against the registry's own integrity value. The
		// version is pinnable from the configuration for an operator who wants one.
		Summary:         "the Pi coding agent for the base's account from npm, with herdr's own integration so herdr has lifecycle authority",
		Presets:         []string{"arch"},
		TakesVersionPin: true,
		Agent:           "pi",
		Host:            &agentLauncherHost{agent: "pi"},
	},
	{
		Name:            "omp",
		Summary:         "Oh My Pi for the base's account from npm, with herdr's own integration, and a refusal when it and pi resolve to one extension directory",
		Presets:         []string{"arch"},
		TakesVersionPin: true,
		Agent:           "omp",
		Host:            &agentLauncherHost{agent: "omp"},
	},
}

// installerDigestRE is a SHA-256 as sha256sum prints it.
var installerDigestRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

func lookupAdapter(name string) (adapterSpec, bool) {
	for _, s := range adapterSpecs {
		if s.Name == name {
			return s, true
		}
	}
	return adapterSpec{}, false
}

// ConfigNamesAdapter says whether a configuration names an adapter.
func ConfigNamesAdapter(c Config, name string) bool {
	for _, a := range c.Base.Adapters {
		if a.Name == name {
			return true
		}
	}
	return false
}

// AdapterNames answers the adapters this executable carries, sorted.
func AdapterNames() []string {
	names := make([]string, 0, len(adapterSpecs))
	for _, s := range adapterSpecs {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return names
}

// validateAdapters is Config.Validate's part for base.adapters.
func validateAdapters(c Config) error {
	seen := map[string]bool{}
	for i, a := range c.Base.Adapters {
		spec, ok := lookupAdapter(a.Name)
		if !ok {
			return fmt.Errorf("base.adapters[%d] names %q, and the adapters this executable carries are: %s",
				i, a.Name, strings.Join(AdapterNames(), ", "))
		}
		if seen[a.Name] {
			return fmt.Errorf("base.adapters names %q more than once", a.Name)
		}
		seen[a.Name] = true
		if a.InstallerSHA256 != "" && !spec.TakesInstallerDigest {
			return fmt.Errorf("base.adapters[%d] gives %q an installer_sha256, and it runs no installer that one could approve", i, a.Name)
		}
		if a.InstallerSHA256 != "" && !installerDigestRE.MatchString(a.InstallerSHA256) {
			return fmt.Errorf("base.adapters[%d].installer_sha256 is not a SHA-256. Give the 64 lowercase hex characters sha256sum prints for the installer you read", i)
		}
		if err := validateAdapterPin(i, a, spec); err != nil {
			return err
		}
		if a.Channel != "" {
			if !spec.TakesChannel {
				return fmt.Errorf("base.adapters[%d] gives %q a channel, and only herdr follows one", i, a.Name)
			}
			if a.Channel != AdapterChannelNightly {
				return fmt.Errorf("base.adapters[%d].channel is %q, and the one channel is %q", i, a.Channel, AdapterChannelNightly)
			}
			if a.Version != "" || len(a.SHA256) > 0 {
				return fmt.Errorf("base.adapters[%d] gives %q a channel and a version pin, and it follows one or the other. Remove \"channel\" to keep the pin, or \"version\" and \"sha256\" to follow the channel", i, a.Name)
			}
		}
		if spec.NeedsSystemd && !c.Base.Systemd {
			return fmt.Errorf("base.adapters names %q, whose server runs as a system unit, so it needs base.systemd to be true", a.Name)
		}
		if len(spec.Presets) > 0 {
			preset := PresetForRef(c.Base.Image)
			found := false
			for _, p := range spec.Presets {
				found = found || p == preset
			}
			if !found {
				return fmt.Errorf("base.adapters names %q, which is driven on the %s preset only, and base.image %q is not it",
					a.Name, strings.Join(spec.Presets, " and "), c.Base.Image)
			}
		}
	}
	return nil
}

// adapterVersionRE is a release as an adapter names one: digits, dots and the
// letters a release tag carries. ⛔ Not a free string: it becomes part of a URL.
var adapterVersionRE = regexp.MustCompile(`^[0-9][0-9A-Za-z.+-]{0,31}$`)

// validateAdapterPin holds the rule that a version and its digests move together.
//
// ⛔ EITHER ALONE IS A REFUSAL. A version with no digest is a download this
// repository will not make; a digest with no version is a value nothing reads,
// and silently ignoring it is how an operator believes they pinned something.
func validateAdapterPin(i int, a BaseAdapter, spec adapterSpec) error {
	if !spec.TakesVersionPin {
		if a.Version != "" || len(a.SHA256) > 0 {
			return fmt.Errorf("base.adapters[%d] pins a version for %q, and that adapter installs nothing this pins", i, a.Name)
		}
		return nil
	}
	if a.Version == "" && len(a.SHA256) == 0 {
		return nil
	}
	if a.Version == "" {
		return fmt.Errorf("base.adapters[%d] gives %q digests and no version, so nothing would read them. Add \"version\"", i, a.Name)
	}
	if len(a.SHA256) == 0 {
		return fmt.Errorf("base.adapters[%d] moves %q to version %q and gives no digests. Add \"sha256\", keyed by architecture as `uname -m` spells it, because this tool downloads nothing it cannot check", i, a.Name, a.Version)
	}
	if !adapterVersionRE.MatchString(a.Version) {
		return fmt.Errorf("base.adapters[%d].version %q is not a release this adapter can name. It becomes part of a download URL, so it is digits, dots, letters, plus and hyphen", i, a.Version)
	}
	for arch, digest := range a.SHA256 {
		if !adapterArchRE.MatchString(arch) {
			return fmt.Errorf("base.adapters[%d].sha256 is keyed %q, and a key is an architecture as `uname -m` spells it, such as x86_64 or aarch64", i, arch)
		}
		if !installerDigestRE.MatchString(digest) {
			return fmt.Errorf("base.adapters[%d].sha256[%q] is not a SHA-256. Give the 64 lowercase hex characters sha256sum prints for that release's asset", i, arch)
		}
	}
	return nil
}

// adapterArchRE is a machine architecture as `uname -m` spells one.
var adapterArchRE = regexp.MustCompile(`^[a-z0-9_]{3,16}$`)

// adapterScript answers one of an adapter's two scripts.
func adapterScript(name, script string) ([]byte, error) {
	return adapterTree.ReadFile("adapters/" + name + "/" + script)
}

// adapterFileEnv answers every file of an adapter other than its two scripts, each
// as TK_FILE_<NAME>_B64.
//
// ⛔ BASE64 IN THE ENVIRONMENT, NOT TEXT IN THE SCRIPT. A file substituted into
// shell source is a file whose quote or dollar sign becomes code, which is the
// defect the command channel exists to remove.
func adapterFileEnv(name string) (map[string]string, error) {
	entries, err := adapterTree.ReadDir("adapters/" + name)
	if err != nil {
		return nil, fmt.Errorf("this executable carries no adapter %q: %w", name, err)
	}
	env := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || e.Name() == "install.sh" || e.Name() == "probe.sh" {
			continue
		}
		body, err := adapterTree.ReadFile("adapters/" + name + "/" + e.Name())
		if err != nil {
			return nil, err
		}
		env["TK_FILE_"+adapterEnvName(e.Name())+"_B64"] = base64.StdEncoding.EncodeToString(body)
	}
	return env, nil
}

var adapterEnvNameRE = regexp.MustCompile(`[^A-Za-z0-9]+`)

func adapterEnvName(file string) string {
	return strings.ToUpper(adapterEnvNameRE.ReplaceAllString(file, "_"))
}

// parseAdapterProbe reads probe.sh's lines.
func parseAdapterProbe(name, out string) AdapterState {
	st := AdapterState{Name: name, Facts: map[string]string{}}
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, _ := strings.Cut(line, " ")
		value = strings.TrimSpace(value)
		switch key {
		case "problem":
			st.Problems = append(st.Problems, value)
		case "version":
			// ⚠ In the facts too, because the machine half compares against it.
			st.Version = value
			st.Facts[key] = value
		default:
			st.Facts[key] = value
		}
	}
	if st.Version == "" {
		st.Problems = append(st.Problems, "the probe named no version")
	}
	st.Healthy = len(st.Problems) == 0
	return st
}

// adapterCompleteLine is what install.sh prints last. ⛔ Its absence over a zero
// exit is a script that stopped early and said nothing.
func adapterCompleteLine(name string) string { return "adapter-complete " + name }

// adapterInstallEnv is what an adapter's install.sh receives, beside what its
// machine half prepares: its files, the account and the distribution, and an
// installer digest the operator approved.
func adapterInstallEnv(cfg Config, a BaseAdapter) (map[string]string, error) {
	env, err := adapterFileEnv(a.Name)
	if err != nil {
		return nil, err
	}
	env["TK_USER"] = cfg.Base.User
	env["TK_DISTRO"] = cfg.Base.Name
	if a.InstallerSHA256 != "" {
		env["TK_INSTALLER_SHA256"] = a.InstallerSHA256
	}
	// ⭐ A RELEASE THIS EXECUTABLE HAS NEVER HEARD OF, pinned by the configuration
	// that asked for it. The script keeps its own default, so a base that names
	// neither is unchanged.
	if a.Version != "" {
		env["TK_ADAPTER_VERSION"] = a.Version
		for arch, digest := range a.SHA256 {
			env["TK_ADAPTER_SHA256_"+strings.ToUpper(arch)] = digest
		}
	}
	return env, nil
}

// applyAdapters installs every adapter the configuration names, in order, and
// proves each one, and takes the machine half of every other one away.
func (b *Base) applyAdapters(ctx context.Context) error {
	named := map[string]bool{}
	for _, a := range b.cfg.Base.Adapters {
		named[a.Name] = true
	}
	for _, spec := range adapterSpecs {
		if !named[spec.Name] && spec.Host != nil {
			// ⚠ THE GUEST HALF STAYS. Removing an adapter from a configuration
			// takes away this machine's way in; what it installed in the base goes
			// with `base recreate`, and the manual says so.
			if err := spec.Host.remove(b); err != nil {
				return fmt.Errorf("adapter %s: %w", spec.Name, err)
			}
		}
	}
	for _, a := range b.cfg.Base.Adapters {
		spec, ok := lookupAdapter(a.Name)
		if !ok {
			return fmt.Errorf("this executable carries no adapter %q", a.Name)
		}
		b.log("adapter " + a.Name + ": installing")
		install, err := adapterScript(a.Name, "install.sh")
		if err != nil {
			return err
		}
		env, err := adapterInstallEnv(b.cfg, a)
		if err != nil {
			return err
		}
		if spec.Host != nil {
			extra, err := spec.Host.prepare(ctx, b)
			if err != nil {
				return fmt.Errorf("adapter %s: %w", a.Name, err)
			}
			for k, v := range extra {
				env[k] = v
			}
		}
		out := &prefixWriter{prefix: "", to: b.logWriter()}
		code, err := b.wsl.Exec(ctx, ExecRequest{
			Distro: b.cfg.Base.Name, User: "root", Script: install, Env: env,
			Timeout: 20 * time.Minute, Stdout: out, Stderr: out,
		})
		out.Flush()
		if err != nil || code != 0 {
			return fmt.Errorf("adapter %s: install exited %d: %w", a.Name, code, err)
		}
		if !strings.Contains(out.seen.String(), adapterCompleteLine(a.Name)) {
			return fmt.Errorf("adapter %s: install exited 0 without reaching its last line", a.Name)
		}
		st := b.probeAdapter(ctx, spec)
		if spec.Host != nil && st.Facts != nil {
			if err := spec.Host.apply(ctx, b, st.Facts); err != nil {
				return fmt.Errorf("adapter %s: %w", a.Name, err)
			}
			st.Problems = append(st.Problems, spec.Host.check(ctx, b, st.Facts)...)
		}
		if len(st.Problems) > 0 {
			return fmt.Errorf("adapter %s is installed, and reading it back found: %s", a.Name, strings.Join(st.Problems, "; "))
		}
		b.log("adapter " + a.Name + ": healthy, version " + st.Version)
	}
	return nil
}

// probeAdapter runs an adapter's probe in the base and reads it, without this
// machine's half.
func (b *Base) probeAdapter(ctx context.Context, spec adapterSpec) AdapterState {
	probe, err := adapterScript(spec.Name, "probe.sh")
	if err != nil {
		return AdapterState{Name: spec.Name, Problems: []string{err.Error()}}
	}
	out, errOut, code, err := b.captureAs(ctx, "root", probe, map[string]string{
		"TK_USER": b.cfg.Base.User, "TK_DISTRO": b.cfg.Base.Name,
	}, 2*time.Minute)
	if err != nil || code != 0 {
		return AdapterState{Name: spec.Name, Problems: []string{fmt.Sprintf("the probe exited %d: %s", code, firstLine(errOut))}}
	}
	return parseAdapterProbe(spec.Name, out)
}

// probeAdapters reads back every adapter the configuration names, both halves.
func (b *Base) probeAdapters(ctx context.Context) []AdapterState {
	var states []AdapterState
	for _, a := range b.cfg.Base.Adapters {
		spec, ok := lookupAdapter(a.Name)
		if !ok {
			states = append(states, AdapterState{Name: a.Name, Problems: []string{"this executable carries no such adapter"}})
			continue
		}
		st := b.probeAdapter(ctx, spec)
		if spec.Host != nil && st.Facts != nil {
			st.Problems = append(st.Problems, spec.Host.check(ctx, b, st.Facts)...)
		}
		st.Healthy = len(st.Problems) == 0
		states = append(states, st)
	}
	return states
}

// removeAdapterHosts takes every adapter's machine half away, for `base remove`.
func (b *Base) removeAdapterHosts() error {
	var errs []error
	for _, spec := range adapterSpecs {
		if spec.Host == nil {
			continue
		}
		if err := spec.Host.remove(b); err != nil {
			errs = append(errs, fmt.Errorf("adapter %s: %w", spec.Name, err))
		}
	}
	return errors.Join(errs...)
}

// adapterHost is the half of an adapter that lives on this machine.
type adapterHost interface {
	// prepare answers the environment the guest half needs from this machine.
	prepare(ctx context.Context, b *Base) (map[string]string, error)
	// apply writes what this machine needs from the guest's facts.
	apply(ctx context.Context, b *Base, facts map[string]string) error
	// check answers what is wrong with this machine's half.
	check(ctx context.Context, b *Base, facts map[string]string) []string
	// remove takes this machine's half away, and is a success when nothing was there.
	remove(b *Base) error
}

// -- herdr's half on this machine ------------------------------------------------

// herdrDoorWrapper is the guest path install.sh writes the door's wrapper to.
const herdrDoorWrapper = "/usr/local/lib/wsl-toolkit/sshd-stdio"

// herdrHost writes one dedicated key, one known-hosts line and one marked Host
// block, all under the Windows account's own .ssh directory, which is exactly
// what the operator's ruling of 2026-09-14 allows.
type herdrHost struct {
	// home is the account home the .ssh directory is under. Empty means
	// SSHDirEnv, then this process's own home; a case points it at a temporary
	// directory.
	home string
	// connect answers what `ssh -F CONFIG ALIAS herdr --version` printed. Nil
	// means run it.
	connect func(ctx context.Context, config, alias string) (string, error)
	// nightly answers the install environment for the newest nightly. Nil means resolve
	// it from this repository's releases; a case stands in for the network.
	nightly func(ctx context.Context) (map[string]string, error)
}

// SSHDirEnv names a directory that stands in for the account's .ssh directory.
//
// ⚠ FOR ISOLATION, NOT FOR USE. The acceptance runner builds a base whose door
// must not be written into the operator's own SSH configuration, and herdr's
// client reads only that one, so a door written elsewhere is reachable by
// `ssh -F` and by nothing that attaches.
const SSHDirEnv = "WSL_TOOLKIT_SSH_DIR"

type herdrPaths struct {
	sshDir, key, knownHosts, config string
}

func (h *herdrHost) paths() (herdrPaths, error) {
	ssh := ""
	switch {
	case h.home != "":
		ssh = filepath.Join(h.home, ".ssh")
	case strings.TrimSpace(os.Getenv(SSHDirEnv)) != "":
		abs, err := filepath.Abs(strings.TrimSpace(os.Getenv(SSHDirEnv)))
		if err != nil {
			return herdrPaths{}, err
		}
		ssh = abs
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return herdrPaths{}, fmt.Errorf("no account home for the SSH door's key: %w", err)
		}
		ssh = filepath.Join(home, ".ssh")
	}
	return herdrPaths{
		sshDir:     ssh,
		key:        filepath.Join(ssh, "wsl-toolkit", "id_ed25519"),
		knownHosts: filepath.Join(ssh, "wsl-toolkit", "known_hosts"),
		config:     filepath.Join(ssh, "config"),
	}, nil
}

func (h *herdrHost) prepare(ctx context.Context, b *Base) (map[string]string, error) {
	p, err := h.paths()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(p.key + ".pub"); errors.Is(err, os.ErrNotExist) {
		keygen, lookErr := exec.LookPath("ssh-keygen")
		if lookErr != nil {
			return nil, errors.New("ssh-keygen is not on PATH, and the SSH door needs a key made on this machine. Windows carries OpenSSH as an optional feature, or: scoop install openssh")
		}
		if err := os.MkdirAll(filepath.Dir(p.key), 0o700); err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, keygen, "-q", "-t", "ed25519", "-N", "", "-C", "wsl-toolkit herdr client", "-f", p.key)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("ssh-keygen could not make the door's key: %v: %s", err, firstLine(string(out)))
		}
		b.log("made the SSH door's key, " + p.key)
	}
	pub, err := os.ReadFile(p.key + ".pub")
	if err != nil {
		return nil, err
	}
	line := strings.TrimSpace(string(pub))
	if !strings.HasPrefix(line, "ssh-ed25519 ") || strings.ContainsAny(line, "\r\n") {
		return nil, fmt.Errorf("%s is not one ed25519 public key", p.key+".pub")
	}
	env := map[string]string{"TK_SSH_CLIENT_KEY": line}
	if herdrChannel(b.cfg) == AdapterChannelNightly {
		nightly := h.nightly
		if nightly == nil {
			nightly = herdrNightlyEnv
		}
		extra, err := nightly(ctx)
		if err != nil {
			return nil, err
		}
		for k, v := range extra {
			env[k] = v
		}
		b.log("the herdr adapter follows the nightly channel: " + extra["TK_ADAPTER_VERSION"])
	}
	return env, nil
}

func (h *herdrHost) apply(ctx context.Context, b *Base, facts map[string]string) error {
	p, err := h.paths()
	if err != nil {
		return err
	}
	hostKey := facts["ssh-host-key"]
	if !strings.HasPrefix(hostKey, "ssh-ed25519 ") {
		return errors.New("the base named no ed25519 host key for its SSH door")
	}
	alias := b.cfg.Base.Name
	if err := os.MkdirAll(filepath.Dir(p.knownHosts), 0o700); err != nil {
		return err
	}
	known, err := readOptional(p.knownHosts)
	if err != nil {
		return err
	}
	if next := withKnownHost(known, alias, hostKey); !bytes.Equal(next, known) {
		if err := writeFileAtomic(p.knownHosts, next, 0o600); err != nil {
			return err
		}
	}
	current, err := readOptional(p.config)
	if err != nil {
		return err
	}
	block := herdrHostBlock(alias, b.cfg.Base.User, b.wsl.Path, p)
	if found, ok := markedBlock(current, alias); !ok || !equalLines(found, block) {
		if err := os.MkdirAll(p.sshDir, 0o700); err != nil {
			return err
		}
		if err := writeFileAtomic(p.config, withMarkedBlock(current, alias, block), 0o600); err != nil {
			return err
		}
		b.log("wrote the Host " + alias + " block at the top of " + p.config)
	}
	// ⭐ THE CLIENT OF THE BUILD THE BASE RUNS, read from the base's own facts, so a
	// nightly published between install and here is not fetched in its place.
	if herdrChannel(b.cfg) == AdapterChannelNightly {
		if _, err := ensureHerdrClient(ctx, b.home, facts["release"], b.log); err != nil {
			return err
		}
	}
	return nil
}

func (h *herdrHost) check(ctx context.Context, b *Base, facts map[string]string) []string {
	p, err := h.paths()
	if err != nil {
		return []string{err.Error()}
	}
	alias := b.cfg.Base.Name
	var problems []string
	if _, err := os.Stat(p.key); err != nil {
		problems = append(problems, "this machine has no key for the SSH door at "+p.key)
	}
	known, _ := readOptional(p.knownHosts)
	if hostKey := facts["ssh-host-key"]; hostKey == "" || !knownHostIs(known, alias, hostKey) {
		problems = append(problems, p.knownHosts+" does not carry the base's host key. Run: wsl-toolkit base ensure")
	}
	current, _ := readOptional(p.config)
	if found, ok := markedBlock(current, alias); !ok || !equalLines(found, herdrHostBlock(alias, b.cfg.Base.User, b.wsl.Path, p)) {
		problems = append(problems, p.config+" does not carry this base's Host block as this tool writes it. Run: wsl-toolkit base ensure")
	}
	if herdrChannel(b.cfg) == AdapterChannelNightly {
		if st, ok := readHerdrClient(b.home); !ok || st.Release != facts["release"] {
			problems = append(problems, "this machine has no herdr client for the base's "+facts["release"]+". Run: wsl-toolkit base ensure")
		}
	}
	if len(problems) > 0 {
		return problems
	}
	connect := h.connect
	if connect == nil {
		connect = sshHerdrVersion
	}
	answer, err := connect(ctx, p.config, alias)
	if err != nil {
		return []string{fmt.Sprintf("ssh %s did not reach herdr through wsl.exe: %v", alias, err)}
	}
	if want := facts["version"]; want == "" || answer != "herdr "+want {
		return []string{fmt.Sprintf("ssh %s answered %q where the base reports herdr %s", alias, answer, facts["version"])}
	}
	return nil
}

func (h *herdrHost) remove(b *Base) error {
	p, err := h.paths()
	if err != nil {
		return err
	}
	alias := b.cfg.Base.Name
	if current, err := readOptional(p.config); err != nil {
		return err
	} else if next := withMarkedBlock(current, alias, nil); !bytes.Equal(next, current) {
		if err := writeFileAtomic(p.config, next, 0o600); err != nil {
			return err
		}
		b.log("removed the Host " + alias + " block from " + p.config)
	}
	if known, err := readOptional(p.knownHosts); err != nil {
		return err
	} else if next := withKnownHost(known, alias, ""); !bytes.Equal(next, known) {
		if err := writeFileAtomic(p.knownHosts, next, 0o600); err != nil {
			return err
		}
	}
	// The instance's herdr client goes with its base's half, whatever the channel says now.
	if client := herdrClientDir(b.home); b.home != "" {
		if _, err := os.Lstat(client); err == nil {
			if err := RemoveInside(b.home, client); err != nil {
				return err
			}
			b.log("removed the herdr client under " + client)
		}
	}
	return nil
}

// HerdrDoor answers the SSH alias the herdr adapter writes for a base, and what is
// missing on this machine for that alias to work. It reads files and starts
// nothing, so it can answer before `base ensure` has ever run.
func HerdrDoor(cfg Config) (string, []string) {
	alias := cfg.Base.Name
	named := false
	for _, a := range cfg.Base.Adapters {
		named = named || a.Name == "herdr"
	}
	if !named {
		return alias, []string{fmt.Sprintf("%s's configuration names no herdr adapter. Add {\"name\": \"herdr\"} to base.adapters, and run base ensure", alias)}
	}
	h := &herdrHost{}
	p, err := h.paths()
	if err != nil {
		return alias, []string{err.Error()}
	}
	var problems []string
	if _, err := os.Stat(p.key); err != nil {
		problems = append(problems, "this machine has no key for the SSH door at "+p.key+". base ensure makes it")
	}
	current, _ := readOptional(p.config)
	w, err := FindWsl()
	if err != nil {
		return alias, append(problems, err.Error())
	}
	if found, ok := markedBlock(current, alias); !ok || !equalLines(found, herdrHostBlock(alias, cfg.Base.User, w.Path, p)) {
		problems = append(problems, p.config+" does not carry the Host "+alias+" block as base ensure writes it")
	}
	return alias, problems
}

// sshHerdrVersion asks the base's herdr for its version through the Host block,
// non-interactively, the way herdr's own client connects.
//
// ⚠ -F NAMES THE FILE THE BLOCK WAS WRITTEN TO, so the answer is about that file
// and not about whichever configuration ssh would otherwise have read.
func sshHerdrVersion(ctx context.Context, config, alias string) (string, error) {
	ssh, err := exec.LookPath("ssh")
	if err != nil {
		return "", errors.New("ssh is not on PATH")
	}
	bounded, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, ssh, "-F", config, "-o", "BatchMode=yes", "-T", alias, "/usr/local/bin/herdr --version")
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, firstLine(errOut.String()))
	}
	return firstLine(out.String()), nil
}

// herdrHostBlock is the Host block's lines, without the markers.
//
// ⛔ THE PROXYCOMMAND STARTS sshd FOR ONE CONNECTION, AND NOTHING LISTENS. Measured
// on 2026-09-14 with Windows OpenSSH 10.0p2 against a throwaway Arch base: key
// authentication and a command round trip in 0.2 s, and herdr's own remote attach
// connected through it.
func herdrHostBlock(alias, user, wslPath string, p herdrPaths) []string {
	quote := func(path string) string {
		// ⚠ Not filepath.ToSlash, which converts only the RUNNING host's separator:
		// these are Windows paths, and the suite also runs on Linux.
		path = strings.ReplaceAll(path, `\`, "/")
		if strings.ContainsAny(path, " \t") {
			return `"` + path + `"`
		}
		return path
	}
	return []string{
		"Host " + alias,
		"  User " + user,
		"  ProxyCommand " + quote(wslPath) + " -d " + alias + " -u root --exec " + herdrDoorWrapper,
		"  IdentityFile " + quote(p.key),
		"  IdentitiesOnly yes",
		"  UserKnownHostsFile " + quote(p.knownHosts),
		"  HostKeyAlias " + alias,
		"  StrictHostKeyChecking yes",
		"  PasswordAuthentication no",
		"  KbdInteractiveAuthentication no",
		"  PreferredAuthentications publickey",
	}
}

func markerBegin(alias string) string { return "# wsl-toolkit begin " + alias }
func markerEnd(alias string) string   { return "# wsl-toolkit end " + alias }

// herdrBlockNote is the first line inside every marked block.
const herdrBlockNote = "# Written by wsl-toolkit base ensure, and removed by base remove."

// markedBlock answers the lines between this alias's markers, wherever the block
// sits, and whether there was one.
func markedBlock(current []byte, alias string) ([]string, bool) {
	var found []string
	inBlock, ok := false, false
	for _, ln := range strings.Split(string(current), "\n") {
		bare := strings.TrimRight(ln, "\r")
		switch {
		case !inBlock && bare == markerBegin(alias):
			inBlock, ok = true, true
		case inBlock && bare == markerEnd(alias):
			return found, ok
		case inBlock:
			found = append(found, bare)
		}
	}
	// ⛔ A BEGIN WITH NO END IS NOT A BLOCK. It is answered as absent, so ensure
	// rewrites the file rather than trusting half of one.
	return nil, false
}

// equalLines says whether a marked block holds exactly the lines this tool writes.
func equalLines(found, block []string) bool {
	want := append([]string{herdrBlockNote}, block...)
	if len(found) != len(want) {
		return false
	}
	for i := range want {
		if found[i] != want[i] {
			return false
		}
	}
	return true
}

// withMarkedBlock answers a configuration file with this alias's marked block
// replaced by block and put at the top, or removed when block is nil.
//
// ⛔ THE BLOCK GOES AT THE TOP. OpenSSH takes the first value it reads for each
// setting, so a `Host *` above it would decide this alias's user and proxy.
// ⭐ Everything outside the markers keeps its bytes, and its line endings decide
// the block's.
func withMarkedBlock(current []byte, alias string, block []string) []byte {
	eol := "\n"
	if bytes.Contains(current, []byte("\r\n")) {
		eol = "\r\n"
	}
	// ⛔ ONLY A COMPLETE PAIR OF MARKERS IS TAKEN OUT. A begin line with no end after
	// it is left where it is, with everything below it: removing to the end of the
	// file would delete somebody's own hosts over one stray comment.
	lines := strings.SplitAfter(string(current), "\n")
	var kept strings.Builder
	for i := 0; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r\n") == markerBegin(alias) {
			end := -1
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimRight(lines[j], "\r\n") == markerEnd(alias) {
					end = j
					break
				}
			}
			if end >= 0 {
				i = end
				continue
			}
		}
		kept.WriteString(lines[i])
	}
	if block == nil {
		return []byte(kept.String())
	}
	var out strings.Builder
	out.WriteString(markerBegin(alias) + eol)
	out.WriteString(herdrBlockNote + eol)
	for _, ln := range block {
		out.WriteString(ln + eol)
	}
	out.WriteString(markerEnd(alias) + eol)
	out.WriteString(kept.String())
	return []byte(out.String())
}

// knownHostIs says whether the alias has exactly one line, carrying this key.
func knownHostIs(current []byte, alias, hostKey string) bool {
	matches := 0
	for _, ln := range strings.Split(string(current), "\n") {
		f := strings.Fields(ln)
		if len(f) == 0 || f[0] != alias {
			continue
		}
		matches++
		if strings.Join(f[1:], " ") != hostKey {
			return false
		}
	}
	return matches == 1
}

// withKnownHost answers a known-hosts file with this alias's line replaced, or
// removed when hostKey is empty. ⚠ The file is this tool's own, so its other lines
// are other bases' and are kept as they are.
func withKnownHost(current []byte, alias, hostKey string) []byte {
	var out strings.Builder
	for _, ln := range strings.SplitAfter(string(current), "\n") {
		if ln == "" {
			continue
		}
		if f := strings.Fields(ln); len(f) > 0 && f[0] == alias {
			continue
		}
		out.WriteString(ln)
		if !strings.HasSuffix(ln, "\n") {
			out.WriteString("\n")
		}
	}
	if hostKey != "" {
		out.WriteString(alias + " " + hostKey + "\n")
	}
	return []byte(out.String())
}

func readOptional(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return b, err
}
