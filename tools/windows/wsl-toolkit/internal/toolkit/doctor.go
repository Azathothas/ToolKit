package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"strings"
	"sync"
	"time"
)

type ToolSpec struct {
	ID, Group, Binary string
	Args              []string
}

// The regression suite compares this inventory with the standalone probe.
func ToolCatalog() []ToolSpec {
	groups := []struct{ name, tools string }{
		{"vcs", "git gh git-lfs jj hg svn"},
		{"runtime", "node deno bun python3 python ruby php java dotnet go rustc zig perl lua"},
		{"compiler", "gcc clang cl"},
		{"pkg-lang", "npm pnpm yarn pip pipx uv poetry cargo rustup gem composer maven gradle"},
		{"pkg-system", "scoop choco winget brew apt dnf pacman apk zypper nix"},
		{"container", "docker podman kubectl wsl"},
		{"build", "make cmake ninja just task msbuild"},
		{"quality", "shellcheck shfmt ruff eslint prettier golangci-lint"},
		{"cli", "jq yq rg fd curl wget aria2c tar 7z sqlite3 scc tokei hyperfine"},
		{"cloud", "wrangler aws gcloud az flyctl terraform"},
		{"shell", "bash zsh pwsh powershell"},
		{"agent", "codegraph"},
	}
	custom := map[string][]string{
		"cl": nil, "scoop": nil, "7z": nil, "wsl": nil,
		"java": {"-version"}, "go": {"version"}, "zig": {"version"},
		"lua": {"-v"}, "kubectl": {"version", "--client"},
		"msbuild": {"-version"}, "flyctl": {"version"},
		"powershell": {"-NoProfile", "-Command", "$PSVersionTable.PSVersion.ToString()"},
	}
	var catalog []ToolSpec
	for _, group := range groups {
		for _, name := range strings.Fields(group.tools) {
			args := []string{"--version"}
			if a, ok := custom[name]; ok {
				args = a
			}
			binary := name
			if name == "maven" {
				binary = "mvn"
			}
			catalog = append(catalog, ToolSpec{name, group.name, binary, args})
		}
	}
	return catalog
}

type ToolProbe struct {
	ID       string   `json:"id"`
	Group    string   `json:"group"`
	Found    bool     `json:"found"`
	Path     string   `json:"path"`
	Resolved string   `json:"resolved_path"`
	Kind     string   `json:"kind"`
	Version  string   `json:"version"`
	Status   string   `json:"status"`
	Notes    []string `json:"notes,omitempty"`
}

type DoctorOptions struct {
	Fast, Net bool
	Group     string
	// Wsl adds the section that answers whether THIS process may call wsl.exe.
	// It is on by default and off for --group, where the caller asked about one
	// group of tools rather than about the machine.
	SkipWsl bool
	Config  Config
}
type DoctorReport struct {
	Schema    string         `json:"schema"`
	Generated string         `json:"generated"`
	Probe     map[string]any `json:"probe"`
	Host      map[string]any `json:"host"`
	Repo      map[string]any `json:"repo"`
	Summary   map[string]int `json:"summary"`
	Tools     []ToolProbe    `json:"tools"`
	Wsl       *WslFacts      `json:"wsl,omitempty"`
	Notes     []string       `json:"notes"`
}

// versionToken finds a dotted version anywhere in a tool's own output.
// ⛔ THE BOUNDARY EXCLUDES A DOT AND ALLOWS A LEADING v, and both halves are
// load-bearing. Without them `yq ... version v4.35.1` matched at the second dot
// and the survey reported version 35.1: a number on a report that nothing
// measured, which is worse than a blank because a blank gets checked.
// ⛔ THE BOUNDARY EXCLUDES A DIGIT AND A DOT AND NOTHING ELSE, and each half was
// a wrong answer before it was there. Excluding letters as well made
// `go version go1.27.0` unmatchable, so go reported no version at all;
// including a dot made `version v4.35.1` match at the second dot and report
// 35.1. A number on a report that nothing measured is worse than a blank,
// because a blank gets checked.
var versionToken = regexp.MustCompile(`(?:^|[^0-9.])[vV]?([0-9]+\.[0-9]+[0-9A-Za-z.+_-]*)`)

// quotedVersion is the second pattern, and it exists because a dotted one is
// not universal. ⚠ Measured on this host: `java -version` on JDK 25 prints
// `openjdk version "25" 2025-09-16`, with no dot at all, so the dotted pattern
// found nothing and the row read version-unknown over a working java.
var quotedVersion = regexp.MustCompile(`(?i)version\s+"?v?([0-9][0-9A-Za-z.+_-]*)"?`)

// probeInvocation decides how to ASK a tool its version.
//
// ⛔ IT RUNS THE PATH THAT WAS FOUND, NOT THE ONE IT RESOLVES TO, and the
// difference is a wrong answer rather than a style point. Measured on this host
// on 2026-09-09: `~/.cargo/bin/rustc.exe` resolves to `rustup.exe`, because
// rustup installs a multiplexer that decides what to do from the name it was
// invoked under. Running the resolved target reported `rustc 1.29.0`, which is
// rustup's own version, on a machine whose rustc is 1.98.0. The RESOLVED path is
// what the report shows a reader; the FOUND path is what gets executed.
func probeInvocation(exe Executable, args []string) (string, []string, rawCommandLine, error) {
	switch strings.ToLower(filepath.Ext(exe.Path)) {
	case ".ps1":
		// ⚠ A .ps1 is not an executable. Process creation with UseShellExecute
		// off throws "not a valid application for this OS platform" on one, so
		// it is routed to a PowerShell host rather than run.
		ps, err := ResolveExecutable("pwsh")
		if err != nil {
			ps, err = ResolveExecutable("powershell")
		}
		if err != nil {
			return "", nil, "", err
		}
		return ps.Path, append([]string{"-NoProfile", "-NonInteractive", "-File", exe.Path}, args...), "", nil
	case ".cmd", ".bat":
		// ⛔ A COMMAND SCRIPT NEEDS A RAW COMMAND LINE. Go escapes an argument
		// list for CreateProcess, and cmd.exe does not parse what CreateProcess
		// produces the way Go assumes: the escaped form reached codegraph.cmd as
		// something it exited 1 on, so the survey reported a broken tool over a
		// working one. /s plus one pair of outer quotes is cmd's own documented
		// shape, and it is built here as a string rather than as arguments.
		//
		// ⚠ cmd expands a percent and an exclamation mark even inside quotes, so
		// a path carrying one is refused rather than run.
		all := append([]string{exe.Path}, args...)
		for _, arg := range all {
			if strings.ContainsAny(arg, "\"\r\n%!^&|<>") {
				return "", nil, "", errors.New("the path or an argument carries a character cmd.exe would read as syntax")
			}
		}
		quoted := make([]string, len(all))
		for i, arg := range all {
			quoted[i] = `"` + arg + `"`
		}
		shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
		line := `"` + shell + `" /d /s /c "` + strings.Join(quoted, " ") + `"`
		return shell, nil, rawCommandLine(line), nil
	default:
		return exe.Path, args, "", nil
	}
}

func probeTool(ctx context.Context, spec ToolSpec, fast bool) ToolProbe {
	p := ToolProbe{ID: spec.ID, Group: spec.Group, Status: "absent"}
	exe, err := ResolveExecutable(spec.Binary)
	p.Notes = exe.Notes
	if err != nil {
		if len(exe.Notes) > 0 {
			p.Status = "inaccessible"
		}
		return p
	}
	p.Found, p.Path, p.Resolved, p.Kind = true, exe.Path, exe.Resolved, exe.Kind
	p.Status = "discovered"
	if fast || len(spec.Args) == 0 {
		return p
	}
	file, args, raw, err := probeInvocation(exe, spec.Args)
	if err != nil {
		p.Status = "unusable"
		p.Notes = append(p.Notes, err.Error())
		return p
	}
	bounded, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	out, stderr, err := outputRaw(bounded, raw, file, args...)
	if bounded.Err() != nil {
		p.Status = "timeout"
		return p
	}
	if err != nil {
		p.Status = "unusable"
		p.Notes = append(p.Notes, fmt.Sprintf("version probe exited %d", ExitCode(err)))
		if exe.Kind == "app-execution-alias" && ExitCode(err) == 9009 {
			// ⚠ 9009 FROM AN APP EXECUTION ALIAS IS NOT A BROKEN TOOL. It is
			// Windows saying the Store package behind the alias is not
			// installed, and the alias itself is a zero-byte stub that exists
			// only to open the Store. A reader told "exited 9009" and nothing
			// else goes looking for a corrupted install.
			p.Notes = append(p.Notes, "this is a Windows app execution alias with no package behind it: it opens the Store rather than running anything")
		}
		return p
	}
	// Java reports its version on stderr; version discovery intentionally reads
	// both. Value-bearing commands elsewhere read stdout alone.
	// ⭐ THE DOTTED PATTERN FIRST, THEN THE QUOTED ONE. Two patterns rather than
	// one loose expression: a loose one matches a date or a build number in the
	// same line and reports it as the version, which is a number on a report
	// that was not measured.
	joined := out + " " + stderr
	if m := versionToken.FindStringSubmatch(joined); m != nil {
		p.Version, p.Status = m[1], "working"
		return p
	}
	if m := quotedVersion.FindStringSubmatch(joined); m != nil {
		p.Version, p.Status = m[1], "working"
		return p
	}
	p.Status = "version-unknown"
	return p
}

func moduleProbe() ToolProbe {
	p := ToolProbe{ID: "psscriptanalyzer", Group: "quality", Kind: "module", Status: "absent"}
	var dirs []string
	for _, root := range filepath.SplitList(os.Getenv("PSModulePath")) {
		if root != "" {
			dirs = append(dirs, filepath.Join(root, "PSScriptAnalyzer"))
		}
	}
	if runtime.GOOS == "windows" {
		for _, sub := range []string{"PowerShell/Modules", "WindowsPowerShell/Modules"} {
			dirs = append(dirs, filepath.Join(os.Getenv("ProgramFiles"), filepath.FromSlash(sub), "PSScriptAnalyzer"))
		}
	}
	pattern := regexp.MustCompile(`(?im)^\s*ModuleVersion\s*=\s*['"]([^'"]+)`)
	for _, dir := range dirs {
		files, _ := filepath.Glob(filepath.Join(dir, "*", "PSScriptAnalyzer.psd1"))
		files = append(files, filepath.Join(dir, "PSScriptAnalyzer.psd1"))
		for _, file := range files {
			b, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			if m := pattern.FindSubmatch(b); m != nil {
				p.Found, p.Path, p.Resolved, p.Version, p.Status = true, filepath.Dir(file), filepath.Dir(file), string(m[1]), "discovered"
				return p
			}
		}
	}
	return p
}

func Doctor(ctx context.Context, o DoctorOptions) (DoctorReport, error) {
	catalog := ToolCatalog()
	valid := o.Group == ""
	var selected []ToolSpec
	for _, spec := range catalog {
		if o.Group == "" || o.Group == spec.Group {
			valid = true
			selected = append(selected, spec)
		}
	}
	if !valid {
		return DoctorReport{}, fmt.Errorf("unknown doctor group %q", o.Group)
	}
	report := DoctorReport{Schema: "agent-doctor/1", Generated: time.Now().UTC().Format(time.RFC3339),
		Probe: map[string]any{"impl": "wsl-toolkit", "fast": o.Fast, "group": o.Group},
		Host:  map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH, "flavor": "native", "wsl": false, "container": false, "kernel": "", "distro": runtime.GOOS, "distro_version": "", "shell": "native Go", "writable_tmp": "", "network": "unknown"},
		Repo:  map[string]any{"is_git": false}, Notes: []string{}, Tools: make([]ToolProbe, len(selected))}
	if runtime.GOARCH == "amd64" {
		report.Host["arch"] = "x86_64"
	}
	if runtime.GOARCH == "arm64" {
		report.Host["arch"] = "aarch64"
	}
	if runtime.GOOS == "linux" {
		if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
			report.Host["wsl"], report.Host["flavor"] = true, "wsl"
		}
		for _, file := range []string{"/.dockerenv", "/run/.containerenv"} {
			if _, err := os.Stat(file); err == nil {
				report.Host["container"] = true
			}
		}
		if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
			report.Host["kernel"] = strings.TrimSpace(string(b))
		}
		if b, err := os.ReadFile("/etc/os-release"); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				key, value, _ := strings.Cut(line, "=")
				if key == "ID" {
					report.Host["distro"] = strings.Trim(value, `"`)
				}
				if key == "VERSION_ID" {
					report.Host["distro_version"] = strings.Trim(value, `"`)
				}
			}
		}
	}
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, spec := range selected {
		wg.Add(1)
		go func(i int, spec ToolSpec) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			report.Tools[i] = probeTool(ctx, spec, o.Fast)
		}(i, spec)
	}
	wg.Wait()
	if o.Group == "" || o.Group == "quality" {
		report.Tools = append(report.Tools, moduleProbe())
	}
	report.Summary = map[string]int{"tools_found": 0, "tools_missing": 0}
	for _, tool := range report.Tools {
		if tool.Found {
			report.Summary["tools_found"]++
		} else {
			report.Summary["tools_missing"]++
		}
	}
	if git, err := ResolveExecutable("git"); err == nil {
		bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		value := func(args ...string) string {
			out, _, err := Output(bounded, git.Resolved, args...)
			if err != nil {
				return ""
			}
			return strings.TrimSpace(out)
		}
		if root := value("rev-parse", "--show-toplevel"); root != "" {
			report.Repo = map[string]any{"is_git": true, "root": root, "branch": value("rev-parse", "--abbrev-ref", "HEAD"), "dirty": value("status", "--porcelain") != "", "commits": value("rev-list", "--count", "HEAD")}
		}
	}
	if runtime.GOOS == "windows" {
		if wsl, err := ResolveExecutable("wsl.exe"); err == nil {
			bounded, cancel := context.WithTimeout(ctx, 6*time.Second)
			out, stderr, err := Output(bounded, wsl.Resolved, "--list", "--quiet")
			cancel()
			report.Host["wsl_access"] = err == nil
			if err == nil {
				report.Host["wsl_distros"] = strings.Fields(out)
			} else {
				report.Host["wsl_error"] = strings.TrimSpace(out + " " + stderr)
				report.Notes = append(report.Notes, "WSL is installed but this process cannot enumerate distributions. Retry the requested command through the agent approval flow, or use an explicitly started local helper.")
			}
		}
	}
	if o.Net {
		client := &http.Client{Timeout: 8 * time.Second}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://example.com", nil)
		if err != nil {
			return report, err
		}
		resp, err := client.Do(req)
		report.Host["network"] = "no"
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				report.Host["network"] = "yes"
			}
		}
	}
	if !o.SkipWsl && o.Group == "" {
		facts := ReadWslFacts(ctx, o.Config)
		report.Wsl = &facts
	}
	report.Host["shell"] = hostShell()
	report.Notes = append(report.Notes,
		"Writable temporary storage was not probed: this report creates no files.",
		"The resolved column is what would actually run. A scoop shim, a node .ps1 wrapper and a .cmd launcher all answer to --version and only one can be handed to CreateProcess.")
	return report, nil
}

func WriteDoctor(w io.Writer, report DoctorReport, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	return WriteDoctorText(w, report)
}
