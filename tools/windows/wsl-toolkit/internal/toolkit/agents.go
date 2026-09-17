// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ⭐ AN AGENT RUNS IN THE BASE, AND WINDOWS REACHES IT FROM THE PROJECT IT STANDS IN.
// The operator's ruling of 2026-09-14 on WSL-78: the agents "should run inside
// wsl-toolkit base to begin with", and Windows gets launchers. An agent adapter names
// the command it installs; `base agent` runs that command in the guest directory the
// caller's Windows directory is granted at, through the same framed channel `base
// exec` uses, so there is no second path into the guest.

// toolModulePath is this tool's module, which a launcher's build information carries.
const toolModulePath = "github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit"

// launcherBaseInstance is the instance a launcher with no suffix reaches: the one base
// the operator's ruling of 2026-09-14 names.
const launcherBaseInstance = "base"

// BinDirEnv names a directory that stands in for the account's bin directory.
//
// ⚠ FOR ISOLATION, NOT FOR USE, as SSHDirEnv is: a throwaway base's launcher must
// not be written beside the operator's own.
const BinDirEnv = "WSL_TOOLKIT_BIN_DIR"

// ErrNotGranted is a directory no grant of the base covers.
var ErrNotGranted = errors.New("not granted to the base")

// notGrantedError is ErrNotGranted for one directory, with the line that grants it.
type notGrantedError struct{ dir, distro string }

func (e *notGrantedError) Error() string {
	return e.dir + " is not granted to " + e.distro + ", so an agent there cannot see it. Grant it, then run the agent again:\n  " + grantCommand(e.dir)
}

func (e *notGrantedError) Is(target error) bool { return target == ErrNotGranted }

// AgentCommand answers the command an agent adapter runs in the base, for a name the
// configuration's base.adapters carries.
func AgentCommand(cfg Config, name string) (string, error) {
	var agents []string
	for _, a := range cfg.Base.Adapters {
		spec, ok := lookupAdapter(a.Name)
		if !ok || spec.Agent == "" {
			continue
		}
		if a.Name == name {
			return spec.Agent, nil
		}
		agents = append(agents, a.Name)
	}
	if len(agents) == 0 {
		return "", fmt.Errorf("%s's configuration names no agent adapter, so there is no %q to run. Add {\"name\": %q} to base.adapters, and run base ensure", cfg.Base.Name, name, name)
	}
	return "", fmt.Errorf("%q is not an agent this base carries. It carries: %s", name, strings.Join(agents, ", "))
}

// GrantedGuestDir answers the guest directory a Windows directory is seen at, through
// the grant that covers it.
//
// ⛔ A GRANT COVERS ITS DIRECTORY AND WHAT IS BENEATH IT, AND NOTHING THAT MERELY
// STARTS WITH THE SAME LETTERS. `C:\work\project` does not cover
// `C:\work\projector`, and a match on the string alone would hand an agent a guest
// path that does not exist and a caller a success that did not happen.
func GrantedGuestDir(cfg Config, dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("%s is not accessible: %w", abs, err)
	}
	resolved = filepath.Clean(resolved)
	grants, err := cfg.ResolvedBaseMounts()
	if err != nil {
		return "", err
	}
	for _, g := range grants {
		rel, ok := beneath(g.Source, resolved)
		if !ok {
			continue
		}
		if rel == "" {
			return g.Target, nil
		}
		return g.Target + "/" + filepath.ToSlash(rel), nil
	}
	return "", &notGrantedError{dir: resolved, distro: cfg.Base.Name}
}

// beneath answers the path of child relative to parent, and whether child is parent
// or inside it, comparing as the host's filesystem does.
func beneath(parent, child string) (string, bool) {
	parent, child = filepath.Clean(parent), filepath.Clean(child)
	same := parent == child
	if runtime.GOOS == "windows" {
		same = strings.EqualFold(parent, child)
	}
	if same {
		return "", true
	}
	prefix := parent
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if runtime.GOOS == "windows" {
		if len(child) <= len(prefix) || !strings.EqualFold(child[:len(prefix)], prefix) {
			return "", false
		}
	} else if !strings.HasPrefix(child, prefix) {
		return "", false
	}
	return child[len(prefix):], true
}

// grantCommand is the line that grants a directory to the selected instance's base.
func grantCommand(dir string) string {
	instance := ""
	if SelectedInstance.Name != DefaultInstance {
		instance = "--instance " + SelectedInstance.Name + " "
	}
	return "wsl-toolkit " + instance + "base grant --source " + quoteForDisplay(dir) + " --mode rw"
}

// quoteForDisplay quotes a Windows path for a line a person pastes into PowerShell.
func quoteForDisplay(p string) string {
	if strings.ContainsAny(p, " '\t") {
		return "'" + strings.ReplaceAll(p, "'", "''") + "'"
	}
	return p
}

// AgentScript is the POSIX command that runs an agent with a caller's arguments.
//
// ⛔ EVERY ARGUMENT IS QUOTED, SO NOTHING IN ONE IS READ AS SHELL. An agent's prompt
// is prose, and prose carries quotes, dollar signs and backticks; a prompt whose
// backtick ran is the defect this tool's command channel exists to remove.
func AgentScript(command string, args []string) []byte {
	var b strings.Builder
	b.WriteString("exec ")
	b.WriteString(command)
	for _, a := range args {
		b.WriteString(" ")
		b.WriteString(shellQuote(a))
	}
	b.WriteString("\n")
	return []byte(b.String())
}

// AgentInteractive says whether an agent's arguments ask for its own screen, which
// runs in a herdr pane rather than through a channel with no terminal.
func AgentInteractive(args []string) bool {
	return len(args) == 0 || args[0] == "resume"
}

// LauncherFile answers the file name of an agent's launcher for an instance, or "" for
// the default instance, which has none.
func LauncherFile(agent, instance string) string {
	switch instance {
	case DefaultInstance:
		return ""
	case launcherBaseInstance:
		return agent + ".exe"
	}
	return agent + "-" + instance + ".exe"
}

// LauncherIdentity reads what a launcher is from the name it was started under: the
// agent it runs and the instance it reaches.
func LauncherIdentity(argv0 string) (agent, instance string, ok bool) {
	name := strings.ToLower(filepath.Base(strings.ReplaceAll(argv0, `\`, "/")))
	name = strings.TrimSuffix(name, ".exe")
	for _, spec := range adapterSpecs {
		if spec.Agent == "" {
			continue
		}
		if name == spec.Agent {
			return spec.Agent, launcherBaseInstance, true
		}
		if suffix, found := strings.CutPrefix(name, spec.Agent+"-"); found && suffix != "" && ValidateInstanceName(suffix) == nil {
			return spec.Agent, suffix, true
		}
	}
	return "", "", false
}

// isToolBuild says whether an executable is a build of this tool, from the build
// information Go writes into it, without running it.
func isToolBuild(path string) bool {
	info, err := buildinfo.ReadFile(path)
	return err == nil && info.Main.Path == toolModulePath
}

// -- an agent's launcher on this machine ---------------------------------------------

// agentLauncherHost writes one launcher for an agent adapter, in the Windows account's
// bin directory, which the operator's ruling of 2026-09-14 allows.
type agentLauncherHost struct {
	agent string
	// dir stands in for the bin directory, and self for this executable. Empty means
	// BinDirEnv then the account's bin directory, and os.Executable; a case sets both.
	dir, self string
}

func (h *agentLauncherHost) paths() (string, string, error) {
	file := LauncherFile(h.agent, SelectedInstance.Name)
	if file == "" {
		return "", "", nil
	}
	dir := h.dir
	switch {
	case dir != "":
	case strings.TrimSpace(os.Getenv(BinDirEnv)) != "":
		abs, err := filepath.Abs(strings.TrimSpace(os.Getenv(BinDirEnv)))
		if err != nil {
			return "", "", err
		}
		dir = abs
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", fmt.Errorf("no account home for the %s launcher: %w", h.agent, err)
		}
		dir = filepath.Join(home, "bin")
	}
	return dir, filepath.Join(dir, file), nil
}

func (h *agentLauncherHost) executable() (string, error) {
	if h.self != "" {
		return h.self, nil
	}
	return os.Executable()
}

func (h *agentLauncherHost) prepare(context.Context, *Base) (map[string]string, error) {
	return nil, nil
}

// apply writes the launcher as a copy of this executable.
//
// ⛔ A FILE THAT IS NOT A BUILD OF THIS TOOL IS NEVER OVERWRITTEN. The bin directory
// is the operator's, and a `muse.exe` somebody else put there is theirs.
func (h *agentLauncherHost) apply(_ context.Context, b *Base, _ map[string]string) error {
	dir, path, err := h.paths()
	if err != nil {
		return err
	}
	if path == "" {
		b.log("the default instance gets no " + h.agent + " launcher; run it with: wsl-toolkit base agent " + h.agent + " -- ARGS")
		return nil
	}
	self, err := h.executable()
	if err != nil {
		return err
	}
	want, err := os.ReadFile(self)
	if err != nil {
		return err
	}
	if have, err := os.ReadFile(path); err == nil {
		if !isToolBuild(path) {
			return fmt.Errorf("%s exists and is not a build of wsl-toolkit, so it was left as it is. Move it, then run base ensure again", path)
		}
		if bytes.Equal(have, want) {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(path, want, 0o755); err != nil {
		return err
	}
	b.log("wrote the " + h.agent + " launcher, " + path)
	return nil
}

// check reads this machine's half back.
//
// ⛔ IT ACCUMULATES RATHER THAN BAILING. The first version returned at each
// problem, so the shadow note below sat behind four early returns and could only
// ever be reported on a machine where everything else was already right. A guard
// that only runs when nothing is wrong is a guard for the case nobody needs.
// Whether the launcher is the current build and whether its NAME reaches it are
// independent facts, and a reader wants both.
func (h *agentLauncherHost) check(_ context.Context, _ *Base, _ map[string]string) (problems, notes []string) {
	_, path, err := h.paths()
	if err != nil {
		return []string{err.Error()}, nil
	}
	if path == "" {
		// The default instance gets no launcher, so there is no name to resolve.
		return nil, nil
	}
	switch {
	case !fileExists(path):
		problems = append(problems, "this machine has no "+h.agent+" launcher at "+path+". Run: wsl-toolkit base ensure")
	case !isToolBuild(path):
		problems = append(problems, path+" is not a build of wsl-toolkit")
	default:
		self, err := h.executable()
		switch {
		case err != nil:
			problems = append(problems, err.Error())
		case !sameFileBytes(path, self):
			problems = append(problems, path+" is another build of wsl-toolkit than this one. Run: wsl-toolkit base ensure")
		}
	}
	// ⭐ AND THE NAME HAS TO REACH IT, whatever the answers above were. Finding 69.
	return problems, h.shadowNote(path)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// shadowNote says whether typing the agent's name on this machine reaches the
// launcher, and is finding 69.
//
// ⛔ THE LAUNCHER EXISTED, MATCHED THIS BUILD, AND THE NAME REACHED SOMETHING
// ELSE. Measured on the operator's host on 2026-09-17: muse.exe, pi.exe and
// omp.exe were all present in the bin directory and all one digest, and `omp`
// resolved to a native Windows build because its directory sat at PATH position
// 3 and the bin directory at 66. The native program answered omp/18.1.19 while
// the base held omp/18.2.3, and it can see neither the base nor the grant.
//
// ⭐ IT IS FINDING 62 ON THE OTHER SIDE OF THE BRIDGE. That one made all three
// guest probes read the agent's name back on a LOGIN shell inside the base,
// because the account's own prefix had got in front of the wrapper. Nothing read
// the name back on WINDOWS, which is where the operator types it.
//
// ⚠ A NOTE AND NOT A PROBLEM. `base agent NAME` and herdr both still reach the
// agent, so the base is not broken and must not be reported not-ready over the
// operator's PATH order, which `base ensure` cannot fix anyway.
func (h *agentLauncherHost) shadowNote(launcher string) []string {
	if runtime.GOOS != "windows" {
		// A launcher is a Windows file. There is nothing to resolve elsewhere.
		return nil
	}
	resolved, err := exec.LookPath(h.agent)
	if err != nil {
		return []string{"typing " + h.agent + " on this machine reaches nothing: " +
			filepath.Dir(launcher) + " is not on PATH. Add it, or run the launcher by its full path, " + launcher}
	}
	if note := launcherShadow(h.agent, resolved, launcher); note != "" {
		return []string{note}
	}
	return nil
}

// launcherShadow is the rule, with no host in it.
//
// ⭐ SEPARATED SO ITS CASE RUNS EVERYWHERE. Finding 42 is a case that needed
// Windows, passed here, and went red on CI's Linux host. The lookup above needs
// a real PATH; deciding what the lookup MEANS does not.
func launcherShadow(agent, resolved, launcher string) string {
	if samePath(resolved, launcher) {
		return ""
	}
	return "typing " + agent + " on this machine runs " + resolved + ", which is not this tool's launcher at " +
		launcher + ". That program cannot see this base or its grants. Run the launcher by its full path, " +
		"or put " + windowsDir(launcher) + " earlier on PATH than " + windowsDir(resolved)
}

// windowsDir is the directory half of a Windows path, decided here for the
// reason normalizeWindowsPath is.
//
// ⚠ filepath.Dir ANSWERS ABOUT THE HOST. On Linux it reads a backslash as an
// ordinary character, so it returns "." for every Windows path and the note would
// tell a reader to reorder two directories both called ".". Nothing computes this
// note on Linux today, and a rule that is only right because nothing calls it is
// a rule waiting to be called.
func windowsDir(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i > 0 {
		return strings.ReplaceAll(p[:i], "/", "\\")
	}
	return p
}

// samePath compares two Windows paths the way Windows does.
//
// ⚠ CASE-INSENSITIVE AND SEPARATOR-INSENSITIVE. PATH holds whatever a user
// typed, so one file arrives spelled with either separator and in either case,
// and a plain string comparison calls those two different files.
//
// ⛔ IT USES NO path/filepath, AND THAT IS THE WHOLE POINT. The first version
// did, and its case went RED in golang:1.25 while passing here, because Clean,
// Abs and Separator all answer about the host they run on and these are Windows
// paths wherever this is compiled. That is finding 42, in the commit whose own
// comment claimed to have applied finding 42's lesson. The container check caught
// it before the push, which is the check finding 42 says exists for this.
func samePath(a, b string) bool {
	return normalizeWindowsPath(a) == normalizeWindowsPath(b)
}

// normalizeWindowsPath is Windows path equality, decided here rather than by the
// host: one separator, one case, and no redundant segment.
//
// ⚠ IT RESOLVES NO "..", because neither input is relative. PATH holds
// absolute entries and the launcher path is built from the account's home, so a
// parent segment would be a shape this never sees and a rule it cannot check.
func normalizeWindowsPath(p string) string {
	p = strings.ToLower(strings.ReplaceAll(p, "\\", "/"))
	parts := strings.Split(p, "/")
	kept := parts[:0]
	for i, part := range parts {
		// An empty part is a doubled separator, except at the start, where it is
		// the leading slash of a UNC or rooted path and carries meaning.
		if part == "." || (part == "" && i > 0) {
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, "/")
}

// remove deletes this instance's launcher when it is a build of this tool.
func (h *agentLauncherHost) remove(b *Base) error {
	dir, path, err := h.paths()
	if err != nil || path == "" {
		return err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if !isToolBuild(path) {
		b.log(path + " is not a build of wsl-toolkit, so it was left as it is")
		return nil
	}
	if err := RemoveInside(dir, path); err != nil {
		return err
	}
	b.log("removed the " + h.agent + " launcher, " + path)
	return nil
}

// sameFileBytes says whether two files hold the same bytes, reading both in step.
func sameFileBytes(a, b string) bool {
	fa, err := os.Open(a)
	if err != nil {
		return false
	}
	defer func() { _ = fa.Close() }()
	fb, err := os.Open(b)
	if err != nil {
		return false
	}
	defer func() { _ = fb.Close() }()
	bufA, bufB := make([]byte, 64<<10), make([]byte, 64<<10)
	for {
		na, errA := io.ReadFull(fa, bufA)
		nb, errB := io.ReadFull(fb, bufB)
		if na != nb || !bytes.Equal(bufA[:na], bufB[:nb]) {
			return false
		}
		endA := errors.Is(errA, io.EOF) || errors.Is(errA, io.ErrUnexpectedEOF)
		endB := errors.Is(errB, io.EOF) || errors.Is(errB, io.ErrUnexpectedEOF)
		if endA || endB {
			return endA && endB
		}
		if errA != nil || errB != nil {
			return false
		}
	}
}
