package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// ⭐ SIX THINGS `ready` MIGHT HAVE TO SAY, EACH WITH A COMMAND BEHIND IT.
// Without them the answer names a subsystem instead of a next step. WSL-52.
//
// ⛔ NONE OF THE SIX NEEDS A NEW CONCEPT, which is why they are one entry and
// why they are P2: each is a surface over data this tool already holds. That is
// reach rather than depth, and a surface that needed a new concept would belong
// in an entry of its own.

// -- artifacts ---------------------------------------------------------------

const artifactsUsage = `wsl-toolkit artifacts <retry> ID --to DIR

  retry   fetch a copy that was RETAINED because its transfer failed. The id is
          a job id for a copy kept in the guest, or an artifact set id for one
          the helper is still holding. Any result that retained a copy names it.

  --to    where to put it. Required
  --json  a structured answer

  It does NOT re-run the job. When nothing was retained it says so rather than
  offering to produce the output again.
`

func cmdArtifacts(ctx context.Context, args []string) (int, error) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, artifactsUsage)
		return exitCannot, nil
	}
	sub, rest := args[0], args[1:]
	fs := newFlagSet("artifacts " + sub)
	to := fs.String("to", "", "the directory to write the retained copy into")
	asJSON := fs.Bool("json", false, "write a structured answer")
	id, rest := splitLeadingID(rest)
	if err := parseArgs(fs, rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, artifactsUsage)
			return exitOK, nil
		}
		return exitCannot, err
	}
	if sub != "retry" {
		fmt.Fprint(os.Stderr, artifactsUsage)
		return exitCannot, fmt.Errorf("%q is not an artifacts subcommand", sub)
	}
	if id == "" {
		return exitCannot, errors.New("artifacts retry needs a job id or an artifact set id")
	}
	if *to == "" {
		return exitCannot, errors.New("artifacts retry needs --to DIR, because it writes files and will not guess where")
	}
	if err := toolkit.AssertArgvSafe([]string{id}); err != nil {
		return exitCannot, err
	}
	cfg, err := loadConfig()
	if err != nil {
		return exitCannot, err
	}
	dest, err := filepath.Abs(*to)
	if err != nil {
		return exitCannot, err
	}

	// A set id belongs to a helper and a job id belongs to the guest, so the
	// helper is asked first and only when one is listening.
	if c, dialErr := toolkit.DialHelper(ctx); dialErr == nil {
		got, err := c.DownloadArtifacts(ctx, id, dest, toolkit.DefaultWorkspaceLimits())
		if err == nil && got.Delivered > 0 {
			return reportRetry(*asJSON, id, "helper", dest, got.Delivered, "")
		}
	}

	runner, err := toolkit.NewRunner(cfg, note)
	if err != nil {
		return exitCannot, err
	}
	guestDir, why := runner.RetainedGuestDir(id)
	if guestDir == "" {
		// ⛔ IT DOES NOT OFFER TO RE-RUN THE JOB. Producing the output again is
		// a different act with a different cost, and a command that quietly did
		// it would have answered a question nobody asked.
		return reportRetry(*asJSON, id, "", dest, 0, why)
	}
	got, err := runner.FetchRetained(ctx, guestDir, dest)
	if err != nil {
		// ⛔ STILL AN OBJECT ON STDOUT. Returning the error here put NOTHING on
		// stdout under --json, which is exactly the defect WSL-46 closed in
		// `base ensure`, reintroduced in a command written afterwards. The
		// `--json` sweep in acceptance.ps1 is what caught it, on the first run
		// of the case, which is the whole argument for that sweep existing.
		//
		// ⚠ THE COPY IS STILL THERE. A retrieval that could not deliver has not
		// lost anything: the guest directory is untouched and `gc` collects it
		// under its own age policy, so a caller can fix the reason and ask again.
		return reportRetry(*asJSON, id, "guest", dest, got.Delivered,
			"the retained copy is still in the guest and could not be delivered: "+err.Error())
	}
	return reportRetry(*asJSON, id, "guest", dest, got.Delivered, "")
}

func reportRetry(asJSON bool, id, kind, dest string, delivered int, why string) (int, error) {
	// ⛔ A REASON MEANS IT DID NOT WORK, whether or not a copy was found. The
	// first version keyed the verdict on `kind` alone, so a retrieval that
	// located the copy and failed to deliver it exited 0.
	code := exitOK
	if kind == "" || why != "" {
		code = exitFailed
	}
	if asJSON {
		return code, writeJSON(map[string]any{
			"schema": "wsl-toolkit-artifacts/1", "id": id, "retained_kind": kind,
			"to": dest, "delivered": delivered, "reason": why,
		})
	}
	if kind == "" {
		logf("  nothing was retained for %s: %s", id, why)
		return code, nil
	}
	if why != "" {
		logf("  the %s copy of %s was found and not delivered: %s", kind, id, why)
		return code, nil
	}
	logf("  %d entr(y|ies) from the %s copy of %s written to %s", delivered, kind, id, dest)
	return code, nil
}

// splitLeadingID takes a bare id written before the flags, the way `logs` does.
func splitLeadingID(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

// -- examples ----------------------------------------------------------------

const examplesUsage = `wsl-toolkit examples [--json]

  The canonical command patterns, so they live in the binary rather than only in
  the manual. An agent that has the executable has these.
`

// examples are the patterns the manual teaches, in the binary.
//
// ⚠ THEY ARE A COPY OF WHAT THE MANUAL SHOWS, and that is a duplication with a
// reason: an agent holding the executable and no manual can still be told the
// shape of a correct call. The manual is the explanation; this is the shape.
var examples = []struct{ What, Command string }{
	{"is this machine ready", "wsl-toolkit ready --smoke"},
	{"one command in one container", `wsl-toolkit run --image alpine -c 'uname -a'`},
	{"a script file rather than a string", `wsl-toolkit run --image debian --script .\build.sh`},
	{"a workspace copied in and artifacts back", `wsl-toolkit run --image debian --workspace . --artifacts .\out -c 'make && cp build/x /out/'`},
	{"the same command across every catalog image", `wsl-toolkit matrix --images all -c 'cc --version'`},
	{"no network for the container", `wsl-toolkit run --image alpine --no-network -c 'wget -T2 example.com'`},
	{"as an account that is not root", `wsl-toolkit run --image alpine --user 1000:1000 -c 'id -u'`},
	{"read a job's complete output back", "wsl-toolkit logs JOB-ID"},
	{"two agents, isolated from each other", "wsl-toolkit --instance two ready --ensure"},
	{"what this tool is holding, and release it", "wsl-toolkit resources ; wsl-toolkit gc --apply"},
	{"a caller that cannot reach wsl.exe", "wsl-toolkit helper serve --detach"},
	{"move to a newer release", "wsl-toolkit selfupdate --check"},
}

func cmdExamples(args []string) (int, error) {
	fs := newFlagSet("examples")
	asJSON := fs.Bool("json", false, "write a structured answer")
	if err := parseArgs(fs, args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(os.Stderr, examplesUsage)
			return exitOK, nil
		}
		return exitCannot, err
	}
	if *asJSON {
		rows := make([]map[string]string, 0, len(examples))
		for _, e := range examples {
			rows = append(rows, map[string]string{"what": e.What, "command": e.Command})
		}
		return exitOK, writeJSON(map[string]any{"schema": "wsl-toolkit-examples/1", "examples": rows})
	}
	for _, e := range examples {
		fmt.Printf("# %s\n%s\n\n", e.What, e.Command)
	}
	return exitOK, nil
}

// -- images pull and warm ----------------------------------------------------

// imagesReach is what one image's readiness looks like.
type imagesReach struct {
	ID        string  `json:"id"`
	Ref       string  `json:"ref"`
	Cached    bool    `json:"cached"`
	Pulled    bool    `json:"pulled"`
	Reason    string  `json:"reason,omitempty"`
	Seconds   float64 `json:"seconds"`
	Reachable bool    `json:"reachable"`
}

// runImagesWarm pulls a selection ahead of time.
//
// ⛔ IT FAILS FIRST RATHER THAN SLOWLY. An image is pulled when a job needs it,
// so a twelve-row matrix on a slow link discovers an unreachable reference on
// row nine, an hour in. This asks every reference up front and reports which
// ones answered. WSL-52.
func runImagesWarm(ctx context.Context, cfg toolkit.Config, selectors []string, pull bool, asJSON bool) (int, error) {
	selected, err := cfg.SelectImages(selectors)
	if err != nil {
		return exitCannot, err
	}
	runner, err := toolkit.NewRunner(cfg, note)
	if err != nil {
		return exitCannot, err
	}
	rows := make([]imagesReach, 0, len(selected))
	failed := 0
	for _, img := range selected {
		started := time.Now()
		r := imagesReach{ID: img.ID, Ref: img.Ref}
		cached, pulled, reason := runner.ReachImage(ctx, img.Ref, pull)
		r.Cached, r.Pulled, r.Reason = cached, pulled, reason
		r.Reachable = reason == ""
		r.Seconds = time.Since(started).Round(time.Millisecond).Seconds()
		if !r.Reachable {
			failed++
		}
		rows = append(rows, r)
		if !asJSON {
			switch {
			case !r.Reachable:
				logf("  ! %-12s %s", img.ID, reason)
			case r.Pulled:
				logf("  %-12s pulled in %.1fs", img.ID, r.Seconds)
			case r.Cached:
				logf("  %-12s already here", img.ID)
			default:
				logf("  %-12s reachable, not pulled", img.ID)
			}
		}
	}
	code := exitOK
	if failed > 0 {
		code = exitFailed
	}
	if asJSON {
		return code, writeJSON(map[string]any{
			"schema": "wsl-toolkit-images-reach/1", "pulled": pull,
			"images": rows, "unreachable": failed,
		})
	}
	return code, nil
}

// -- config validate and --effective -----------------------------------------

// runConfigValidate reads the configuration the search resolves and refuses one
// the loader refuses, WITHOUT writing anything.
//
// ⛔ IT WRITES NOTHING, which is the whole point of it existing beside
// `config --write`. A caller checking a configuration before using it must not
// have the check create the file it was asking about.
func runConfigValidate(asJSON bool, path string) (int, error) {
	if path != "" {
		toolkit.ExplicitConfigPath = path
	}
	src, srcErr := toolkit.ResolveConfig()
	cfg, err := toolkit.LoadConfig()
	reason := ""
	switch {
	case srcErr != nil:
		reason = srcErr.Error()
	case err != nil:
		reason = err.Error()
	}
	ok := reason == ""
	code := exitOK
	if !ok {
		code = exitFailed
	}
	if asJSON {
		return code, writeJSON(map[string]any{
			"schema": "wsl-toolkit-config-validate/1", "valid": ok, "reason": reason,
			"path": src.Path, "from": src.From, "searched": src.Searched,
			"fingerprint": cfg.Fingerprint(),
		})
	}
	if ok {
		logf("  %s is usable, fingerprint %s", src.Path, cfg.Fingerprint())
	} else {
		logf("  %s is not usable: %s", src.Path, reason)
	}
	return code, nil
}

// -- resources --job ---------------------------------------------------------

// narrowResourcesToJob keeps only what belongs to one job.
//
// ⛔ A POST-FILTER RATHER THAN A SECOND READ PATH. The report is produced once,
// by the same code on both routes, and narrowing it here means `--job` cannot
// answer differently from the whole-store view it is a subset of. WSL-52.
func narrowResourcesToJob(rep toolkit.ResourceReport, id string) toolkit.ResourceReport {
	out := rep
	out.Owned.GuestJobs = nil
	for _, j := range rep.Owned.GuestJobs {
		if strings.Contains(j.Path, id) {
			out.Owned.GuestJobs = append(out.Owned.GuestJobs, j)
		}
	}
	out.Owned.Containers = nil
	for _, c := range rep.Owned.Containers {
		if strings.Contains(c.Name, id) {
			out.Owned.Containers = append(out.Owned.Containers, c)
		}
	}
	out.Owned.OpenRecords = nil
	for _, e := range rep.Owned.OpenRecords {
		if e.ID == id {
			out.Owned.OpenRecords = append(out.Owned.OpenRecords, e)
		}
	}
	// ⚠ THE TOTALS ARE WITHHELD RATHER THAN RECOMPUTED. They measure the whole
	// state directory, and reporting them beside one job's rows would be a
	// number that looks like it belongs to the job and does not.
	out.Owned.HomeBytes, out.Owned.HomeKnown = 0, false
	out.Owned.GuestBytes, out.Owned.GuestKnown = 0, false
	out.Warnings = append(out.Warnings,
		"narrowed to job "+id+"; the byte totals measure the whole store and are withheld here")
	sort.SliceStable(out.Owned.GuestJobs, func(i, j int) bool {
		return out.Owned.GuestJobs[i].Path < out.Owned.GuestJobs[j].Path
	})
	return out
}
