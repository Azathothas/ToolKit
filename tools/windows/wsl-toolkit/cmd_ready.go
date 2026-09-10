package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// ⭐ ONE COMMAND, ONE ANSWER. A first-run agent used to assemble readiness from
// separate concepts: read the manual, diagnose WSL access, start or find a
// helper, ensure and probe the base, work out which route it got, run a
// representative container, and find where output went. An experienced operator
// can compose that; an agent pointed at the manual should not have to infer it,
// especially when its own process cannot call wsl.exe while an approved helper
// can. WSL-49.
//
// ⛔ NOTHING NEW IS MEASURED HERE. `doctor`, `helper status`, `base status
// --probe`, `run` and `logs` already answer every part of it. What was missing
// is one command that runs them in order and reports one answer.
//
// ⛔ WHEN IT CANNOT PROCEED IT PRINTS ONE EXACT COMMAND and does not pretend it
// can self-elevate. A restricted process that cannot start a helper says so and
// names the approval command, which is the whole reason this product exists.

const readyUsage = `wsl-toolkit ready [--smoke] [--ensure] [--json]

  One answer to "can this agent run isolated Linux jobs here, and if not, what
  is the one command that fixes it".

  --smoke     run one tiny container proving the whole path end to end
  --ensure    build or repair the base if it is not usable. Off by default:
              a readiness check that builds a distribution has changed the
              thing it was asked to measure
  --json      one stable object
  --no-update do not ask whether a newer release exists
`

// ReadyReport is the stable object. ⛔ Every field is always present except the
// ones whose absence is itself the answer, and those are documented as
// omitempty in the manual rather than left to be discovered.
type ReadyReport struct {
	Schema     string               `json:"schema"`
	Ready      bool                 `json:"ready"`
	Verdict    string               `json:"verdict"`
	Executable string               `json:"executable"`
	Version    string               `json:"version"`
	Instance   string               `json:"instance"`
	Home       string               `json:"home"`
	Config     readyConfig          `json:"config"`
	Route      readyRoute           `json:"route"`
	Helper     readyHelper          `json:"helper"`
	Base       readyBase            `json:"base"`
	Catalog    int                  `json:"catalog"`
	Smoke      *readySmoke          `json:"smoke,omitempty"`
	Update     toolkit.UpdateStatus `json:"update"`
	Locations  readyLocations       `json:"locations"`
	// Remediation is the ordered list of EXACT commands that would move this
	// machine forward. ⛔ Empty when ready; the first entry is the one to run.
	Remediation []string `json:"remediation,omitempty"`
	Problems    []string `json:"problems,omitempty"`
}

type readyConfig struct {
	Path        string `json:"path"`
	From        string `json:"from"`
	Exists      bool   `json:"exists"`
	Fingerprint string `json:"fingerprint"`
	Valid       bool   `json:"valid"`
	Reason      string `json:"reason,omitempty"`
}

type readyRoute struct {
	Selected     string `json:"selected"`
	Reason       string `json:"reason"`
	WslCallable  bool   `json:"wsl_callable"`
	WslInstalled bool   `json:"wsl_installed"`
}

type readyHelper struct {
	Listening  bool   `json:"listening"`
	Address    string `json:"address,omitempty"`
	Version    string `json:"version,omitempty"`
	Compatible bool   `json:"compatible"`
	Reason     string `json:"reason,omitempty"`
}

type readyBase struct {
	Name       string `json:"name"`
	Image      string `json:"image"`
	BuiltFrom  string `json:"built_from,omitempty"`
	Registered bool   `json:"registered"`
	Healthy    bool   `json:"healthy"`
	Engine     string `json:"engine,omitempty"`
	Identified bool   `json:"identified"`
}

type readySmoke struct {
	Ran        bool   `json:"ran"`
	UID        string `json:"uid,omitempty"`
	Kernel     string `json:"kernel,omitempty"`
	WorkWrite  bool   `json:"work_writable"`
	Artifact   bool   `json:"artifact_returned"`
	Transcript bool   `json:"transcript_readable"`
	NoHostMnt  bool   `json:"no_host_mount"`
	JobID      string `json:"job_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type readyLocations struct {
	State       string `json:"state"`
	Transcripts string `json:"transcripts"`
	Config      string `json:"config"`
}

func cmdReady(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("ready")
	asJSON := fs.Bool("json", false, "write a structured answer")
	smoke := fs.Bool("smoke", false, "run one tiny container proving the whole path")
	ensure := fs.Bool("ensure", false, "build or repair the base if it is not usable")
	noUpdate := fs.Bool("no-update", false, "do not ask whether a newer release exists")
	if err := parseArgs(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, readyUsage)
			return exitOK, nil
		}
		return exitCannot, err
	}
	r := assembleReady(ctx, *smoke, *ensure, !*noUpdate)
	if *asJSON {
		return readyVerdict(r), writeJSON(r)
	}
	renderReady(r)
	return readyVerdict(r), nil
}

func readyVerdict(r ReadyReport) int {
	if r.Ready {
		return exitOK
	}
	return exitFailed
}

// assembleReady runs the parts in the order a first run needs them, and never
// stops at the first problem: an agent that learns three things are wrong in one
// pass fixes them in one pass.
func assembleReady(ctx context.Context, smoke, ensure, wantUpdate bool) ReadyReport {
	r := ReadyReport{Schema: "wsl-toolkit-ready/1", Instance: toolkit.SelectedInstance.Name}
	if exe, err := os.Executable(); err == nil {
		r.Executable, _ = filepath.Abs(exe)
	}
	r.Version = versionString()

	// -- the configuration, validated and never changed ----------------------
	src, srcErr := toolkit.ResolveConfig()
	r.Config.Path, r.Config.From = src.Path, src.From
	if srcErr != nil {
		r.Config.Reason = srcErr.Error()
		r.Problems = append(r.Problems, srcErr.Error())
		r.Remediation = append(r.Remediation, "wsl-toolkit config")
	}
	cfg, cfgErr := toolkit.LoadConfig()
	if cfgErr != nil {
		r.Config.Reason = cfgErr.Error()
		r.Problems = append(r.Problems, cfgErr.Error())
		r.Remediation = append(r.Remediation, "wsl-toolkit config")
	} else {
		r.Config.Valid = true
	}
	if _, err := os.Stat(r.Config.Path); err == nil {
		r.Config.Exists = true
	}
	r.Config.Fingerprint = cfg.Fingerprint()
	r.Catalog = len(cfg.Catalog())
	r.Base.Name, r.Base.Image = cfg.Base.Name, cfg.Base.Image

	if home, err := toolkit.Home(); err == nil {
		r.Home = home
		r.Locations.State = home
		r.Locations.Transcripts = filepath.Join(home, "jobs")
	}
	r.Locations.Config = r.Config.Path

	// -- the route, and the reason for it ------------------------------------
	probeErr := toolkit.ProbeWsl(ctx)
	r.Route.WslCallable = probeErr == nil
	r.Route.WslInstalled = true
	if probeErr != nil && errors.Is(probeErr, toolkit.ErrWslMissing) {
		r.Route.WslInstalled = false
	}
	client, dialErr := toolkit.DialHelper(ctx)
	if dialErr == nil {
		r.Helper.Listening = true
		r.Helper.Address = client.Endpoint().Address
		r.Helper.Version = client.Endpoint().Version
		// ⚠ COMPATIBLE MEANS THE SCHEMA MATCHED, which DialHelper already
		// asserted by refusing anything else. A version that differs is not
		// necessarily incompatible, and saying so is more honest than an
		// equality test dressed as a compatibility answer.
		r.Helper.Compatible = true
		if r.Helper.Version != r.Version {
			r.Helper.Reason = "it is running " + r.Helper.Version + " and this client is " + r.Version +
				"; the protocol matched, so they can work together"
		}
	} else {
		r.Helper.Reason = dialErr.Error()
	}

	switch {
	case r.Route.WslCallable:
		r.Route.Selected, r.Route.Reason = "direct", "this process can call wsl.exe itself"
	case r.Helper.Listening:
		r.Route.Selected = "helper"
		r.Route.Reason = "this process cannot call wsl.exe and a helper is listening on " + r.Helper.Address
	case !r.Route.WslInstalled:
		r.Route.Selected, r.Route.Reason = "none", "WSL is not installed on this machine"
		r.Problems = append(r.Problems, "WSL is not installed, and nothing here can install it")
		r.Remediation = append(r.Remediation, "wsl --install")
	default:
		r.Route.Selected = "none"
		r.Route.Reason = "this process cannot call wsl.exe and no helper is listening: " + probeErr.Error()
		r.Problems = append(r.Problems, r.Route.Reason)
		// ⛔ ONE EXACT COMMAND, and it is the approval path rather than a
		// suggestion that this process try harder. It cannot self-elevate and
		// saying otherwise would be the lie this whole product exists to avoid.
		r.Remediation = append(r.Remediation, "wsl-toolkit helper serve --detach")
	}

	// -- the base ------------------------------------------------------------
	if r.Route.Selected != "none" {
		r.fillBase(ctx, cfg, client, ensure)
	}

	// -- one container, end to end -------------------------------------------
	if smoke {
		r.Smoke = runReadySmoke(ctx, cfg, r.Base.Healthy)
	}

	// -- is there a newer release --------------------------------------------
	if wantUpdate {
		r.Update = toolkit.CheckUpdate(ctx, r.Version)
	} else {
		r.Update = toolkit.UpdateStatus{Running: r.Version, Reason: "--no-update"}
	}

	r.Ready = len(r.Problems) == 0 && r.Base.Healthy
	if r.Smoke != nil && !r.Smoke.Ran {
		r.Ready = false
	}
	switch {
	case r.Ready:
		r.Verdict = "ready"
	case r.Route.Selected == "none":
		r.Verdict = "no-route"
	case !r.Base.Healthy:
		r.Verdict = "no-base"
	default:
		r.Verdict = "not-ready"
	}
	return r
}

func (r *ReadyReport) fillBase(ctx context.Context, cfg toolkit.Config, client *toolkit.HelperClient, ensure bool) {
	read := func(probe bool) (toolkit.BaseState, error) {
		if r.Route.Selected == "helper" && client != nil {
			return client.BaseStatus(ctx, probe)
		}
		base, err := toolkit.NewBase(cfg, note)
		if err != nil {
			return toolkit.BaseState{}, err
		}
		return base.Status(ctx, probe)
	}
	st, err := read(true)
	if err != nil {
		r.Problems = append(r.Problems, err.Error())
		r.Remediation = append(r.Remediation, "wsl-toolkit base status --probe")
		return
	}
	if !st.Healthy && ensure {
		// ⛔ ONLY WITH --ensure. A readiness check that builds a distribution
		// has changed the thing it was asked to measure, and a first run that
		// silently spent four minutes pulling a rootfs is not an answer.
		note("the base is not usable and --ensure was passed, so this builds it")
		if r.Route.Selected == "helper" && client != nil {
			st, err = client.BaseEnsure(ctx, false)
		} else {
			var base *toolkit.Base
			if base, err = toolkit.NewBase(cfg, note); err == nil {
				var runner *toolkit.Runner
				if runner, err = toolkit.NewRunner(cfg, note); err == nil {
					st, err = runner.EnsureBase(ctx, false)
				}
				_ = base
			}
		}
		if err != nil {
			r.Problems = append(r.Problems, err.Error())
			r.Remediation = append(r.Remediation, "wsl-toolkit base ensure")
			return
		}
	}
	r.Base.Registered, r.Base.Healthy = st.Registered, st.Healthy
	r.Base.Engine, r.Base.BuiltFrom = st.Engine, st.BuiltFrom
	r.Base.Identified = st.Identity != nil
	for _, p := range st.Problems {
		r.Problems = append(r.Problems, p)
	}
	if !st.Healthy {
		r.Remediation = append(r.Remediation, "wsl-toolkit base ensure")
	}
}

// readySmokeScript is deliberately one line per fact, so a partial answer says
// which fact was missing rather than failing as a whole.
const readySmokeScript = `set -u
printf 'uid=%s\n' "$(id -u)"
printf 'kernel=%s\n' "$(uname -r)"
if : > /work/.wsl-toolkit-ready 2>/dev/null; then printf 'work=yes\n'; else printf 'work=no\n'; fi
rm -f /work/.wsl-toolkit-ready 2>/dev/null || :
printf 'ready\n' > /out/ready.txt
if [ -d /mnt/c ]; then printf 'hostmnt=yes\n'; else printf 'hostmnt=no\n'; fi
`

func runReadySmoke(ctx context.Context, cfg toolkit.Config, healthy bool) *readySmoke {
	s := &readySmoke{}
	if !healthy {
		s.Reason = "the base is not usable, so nothing was run"
		return s
	}
	runner, err := toolkit.NewRunner(cfg, note)
	if err != nil {
		s.Reason = err.Error()
		return s
	}
	home, err := toolkit.EnsureHome()
	if err != nil {
		s.Reason = err.Error()
		return s
	}
	art := filepath.Join(home, "ready-smoke")
	// ⛔ THROUGH THE ONE DELETION, which contains the target and reads the state
	// back. TODO/RULES.md section 3, and this is a path this command builds
	// rather than one a caller named, which is exactly the case where a bare
	// RemoveAll looks harmless and sets the precedent.
	clear := func() {
		if err := toolkit.RemoveInside(home, art); err != nil && !errors.Is(err, os.ErrNotExist) {
			note("the smoke directory is still on disk: " + art)
		}
	}
	clear()
	defer clear()

	res := runner.Run(ctx, toolkit.JobSpec{
		Image: toolkit.VerifyImage, Script: []byte(readySmokeScript),
		ArtifactDir: art, Timeout: 5 * time.Minute, Label: "ready --smoke",
	})
	s.JobID = res.ID
	if res.Verdict() != 0 {
		s.Reason = fmt.Sprintf("the smoke job exited %d: %s", res.Verdict(), strings.TrimSpace(res.Error+" "+res.ArtifactError))
		return s
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "uid":
			s.UID = v
		case "kernel":
			s.Kernel = v
		case "work":
			s.WorkWrite = v == "yes"
		case "hostmnt":
			// ⛔ THE ASSERTION IS THE ABSENCE, and it is asked rather than
			// inferred from what was left off the command line.
			// docs/conventions/forbidden-patterns.md: a header asserting a
			// property the command line does not enforce is a claim that is
			// simply false.
			s.NoHostMnt = v == "no"
		}
	}
	if _, err := os.Stat(filepath.Join(art, "ready.txt")); err == nil {
		s.Artifact = true
	}
	if res.Transcript != "" {
		if b, err := os.ReadFile(filepath.Join(res.Transcript, "stdout.log")); err == nil && len(b) > 0 {
			s.Transcript = true
		}
	}
	s.Ran = s.UID != "" && s.Kernel != "" && s.WorkWrite && s.Artifact && s.Transcript && s.NoHostMnt
	if !s.Ran {
		s.Reason = "the container ran and one of the six facts did not hold"
	}
	return s
}

func renderReady(r ReadyReport) {
	out := os.Stderr
	fmt.Fprintf(out, "  verdict     %s\n", r.Verdict)
	fmt.Fprintf(out, "  version     %s\n", r.Version)
	fmt.Fprintf(out, "  executable  %s\n", r.Executable)
	if r.Instance != "" {
		fmt.Fprintf(out, "  instance    %s\n", r.Instance)
	}
	fmt.Fprintf(out, "  state       %s\n", r.Home)
	fmt.Fprintf(out, "  config      %s (%s), fingerprint %s\n", r.Config.Path, r.Config.From, r.Config.Fingerprint)
	fmt.Fprintf(out, "  route       %s -- %s\n", r.Route.Selected, r.Route.Reason)
	if r.Helper.Listening {
		fmt.Fprintf(out, "  helper      %s, version %s\n", r.Helper.Address, r.Helper.Version)
	} else {
		fmt.Fprintf(out, "  helper      none listening\n")
	}
	fmt.Fprintf(out, "  base        %s, registered %v, usable %v\n", r.Base.Name, r.Base.Registered, r.Base.Healthy)
	if r.Base.BuiltFrom != "" {
		fmt.Fprintf(out, "  built from  %s\n", r.Base.BuiltFrom)
	}
	if r.Base.Engine != "" {
		fmt.Fprintf(out, "  engine      %s\n", r.Base.Engine)
	}
	fmt.Fprintf(out, "  catalog     %d image(s)\n", r.Catalog)
	if r.Smoke != nil {
		if r.Smoke.Ran {
			fmt.Fprintf(out, "  smoke       ran as uid %s on kernel %s, /work writable, artifact returned, transcript readable, no host mount\n",
				r.Smoke.UID, r.Smoke.Kernel)
		} else {
			fmt.Fprintf(out, "  smoke       DID NOT PROVE IT: %s\n", r.Smoke.Reason)
		}
	}
	switch {
	case r.Update.Checked && r.Update.Available:
		fmt.Fprintf(out, "  update      %s is published and this is %s. Run: %s\n", r.Update.Latest, r.Update.Running, r.Update.Command)
	case r.Update.Checked && r.Update.Reason != "":
		// ⚠ A build AHEAD of the newest release. Saying "this is the newest
		// published release" here would name a version that is not published.
		fmt.Fprintf(out, "  update      none: %s\n", r.Update.Reason)
	case r.Update.Checked:
		fmt.Fprintf(out, "  update      none: %s is the newest published release\n", r.Update.Running)
	default:
		fmt.Fprintf(out, "  update      not checked: %s\n", r.Update.Reason)
	}
	for _, p := range r.Problems {
		fmt.Fprintf(out, "  ! %s\n", p)
	}
	if len(r.Remediation) > 0 {
		// ⛔ ONE COMMAND, NAMED FIRST. A list of six things to try is a list
		// nobody runs; the first line is the one to run now.
		fmt.Fprintf(out, "\n  run this next:\n    %s\n", r.Remediation[0])
		for _, c := range r.Remediation[1:] {
			fmt.Fprintf(out, "    then: %s\n", c)
		}
	}
}
