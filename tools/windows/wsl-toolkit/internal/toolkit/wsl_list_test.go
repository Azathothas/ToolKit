// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// ⛔ WSL-188: AN EMPTY MACHINE IS READ IN ANY LANGUAGE. The check matched the
// English sentence alone, so a Windows in another language read its empty
// machine as a failed listing. The error code is read too, and where wsl.exe
// names neither, WSL's own registry says whether anything is registered.
func TestAnEmptyMachineIsReadInAnyLanguage(t *testing.T) {
	for said, empty := range map[string]bool{
		"Windows Subsystem for Linux has no installed distributions.":                                                  true,
		"Das Windows-Subsystem fuer Linux hat keine Distributionen.\r\nFehlercode: Wsl/WSL_E_DEFAULT_DISTRO_NOT_FOUND": true,
		"Catastrophic failure\r\nError code: Wsl/0x8000ffff":                                                           false,
	} {
		if got := emptyMachine("", said); got != empty {
			t.Errorf("emptyMachine(%q) = %v, want %v", said, got, empty)
		}
	}
	h := newFakeHost(t)
	h.say(t, "listing", "localized-empty")
	w := &Wsl{Path: h.wsl}
	if names, err := w.listNames(context.Background()); err != nil || len(names) != 0 {
		t.Errorf("a German empty machine, with no distribution in the registry, listed %v, %v; want nothing and no failure", names, err)
	}
	// ⛔ A REGISTERED DISTRIBUTION KEEPS THE FAILURE A FAILURE.
	previous := readRegistrations
	t.Cleanup(func() { readRegistrations = previous })
	readRegistrations = func() (map[string]string, error) { return map[string]string{"other": `C:\other`}, nil }
	if _, err := w.listNames(context.Background()); err == nil {
		t.Error("a failed listing on a machine with a registered distribution read as an empty machine")
	}
	// ⛔ A wsl.exe THAT NEVER STARTED SAYS NOTHING ABOUT WHAT IS REGISTERED.
	readRegistrations = func() (map[string]string, error) { return map[string]string{}, nil }
	missing := &Wsl{Path: filepath.Join(t.TempDir(), "no-wsl.exe")}
	if _, err := missing.listNames(context.Background()); err == nil {
		t.Error("a wsl.exe that never started read as an empty machine")
	}
}

// ⛔ WSL-188: DOCTOR READS AN EMPTY OR A SLOW LIST AS NO REFUSAL. It read every
// failed listing as a process that cannot enumerate, and sent the reader to the
// approval flow, over a machine with no distribution or a WSL still starting.
func TestDoctorReadsAnEmptyOrSlowListAsNoRefusal(t *testing.T) {
	exited := &ProcessError{Code: 1, Op: "process", Err: errors.New("exit status 1"), Started: true}
	slow := &ProcessError{Code: ExitTimeout, Op: "process", Err: context.DeadlineExceeded, Started: true}
	previous := readRegistrations
	t.Cleanup(func() { readRegistrations = previous })
	for _, c := range []struct {
		name, out, stderr string
		err               error
		registered        bool
		state             string
		access            bool
		note              string
	}{
		{"listed", "Arch\nwsl-toolkit\n", "", nil, true, "listed", true, ""},
		// ⚠ A REGISTRY THAT HOLDS A DISTRIBUTION, so each sentence is read on its own.
		{"empty, in English", "Windows Subsystem for Linux has no installed distributions.", "", exited, true, "empty", true, ""},
		{"empty, by its code", "", "Fehlercode: Wsl/WSL_E_DEFAULT_DISTRO_NOT_FOUND", exited, true, "empty", true, ""},
		{"empty, by the registry", "Das Windows-Subsystem fuer Linux hat keine Distributionen.", "", exited, false, "empty", true, ""},
		{"refused", "", "Wsl/EnumerateDistros/Service/E_ACCESSDENIED", exited, false, "refused", false, "cannot enumerate"},
		{"slow", "", "", slow, false, "slow", false, "did not answer the listing in time"},
		{"failed", "Catastrophic failure", "", exited, true, "failed", false, "not as a refusal"},
	} {
		disks := map[string]string{}
		if c.registered {
			disks["other"] = `C:\other`
		}
		readRegistrations = func() (map[string]string, error) { return disks, nil }
		got := readDoctorWsl(c.out, c.stderr, c.err)
		if got.State != c.state || got.Access != c.access || !strings.Contains(got.Note, c.note) {
			t.Errorf("%s: doctor read %+v; want %s, access %v, a note saying %q", c.name, got, c.state, c.access, c.note)
		}
		if c.state != "refused" && strings.Contains(got.Note, "cannot enumerate") {
			t.Errorf("%s: doctor sent the reader to the approval flow: %s", c.name, got.Note)
		}
	}
}

// ⛔ WSL-188: AND DOCTOR'S REPORT CARRIES WHAT IT READ. An empty machine in another
// language reads `wsl_access` true, `wsl_state` empty and no distribution, with no
// note sending the reader to the approval flow. ⚠ The report is filled from the
// listing's answer, so this runs on every host. Doctor asks wsl.exe on Windows
// alone, and the case that went through Doctor was skipped everywhere else.
func TestDoctorReportsAnEmptyMachineAsEmpty(t *testing.T) {
	previous := readRegistrations
	t.Cleanup(func() { readRegistrations = previous })
	readRegistrations = func() (map[string]string, error) { return map[string]string{}, nil }
	report := DoctorReport{Host: map[string]any{}, Notes: []string{}}
	exited := &ProcessError{Code: 1, Op: "process", Err: errors.New("exit status 1"), Started: true}
	report.addWslListing("Das Windows-Subsystem fuer Linux hat keine installierten Distributionen.", "", exited)
	if report.Host["wsl_state"] != "empty" || report.Host["wsl_access"] != true {
		t.Errorf("doctor reported an empty machine as wsl_state %v, wsl_access %v", report.Host["wsl_state"], report.Host["wsl_access"])
	}
	if distros, ok := report.Host["wsl_distros"].([]string); !ok || len(distros) != 0 {
		t.Errorf("doctor reported an empty machine's distributions as %#v", report.Host["wsl_distros"])
	}
	for _, note := range report.Notes {
		if strings.Contains(note, "cannot enumerate") {
			t.Errorf("doctor sent the reader of an empty machine to the approval flow: %s", note)
		}
	}
}

func TestAFailedListingNamesItsQueryAndIsAskedAgainOnlyWhenThatCanHelp(t *testing.T) {
	process := &ProcessError{Code: 2, Op: "process", Err: errors.New("exit status 0xffffffff")}
	err := listFailure([]string{"--list", "--running", "--quiet"}, "", "", process)
	if !strings.Contains(err.Error(), "wsl.exe --list --running --quiet failed and printed nothing") {
		t.Errorf("the failure does not name the query: %v", err)
	}
	var pe *ProcessError
	if !errors.As(err, &pe) || pe.Code != 2 {
		t.Errorf("naming the query lost the process error a caller reads the code from: %v", err)
	}
	if said := listFailure([]string{"--list", "--quiet"}, "\n", "Catastrophic failure\r\nError code: Wsl/0x8000ffff", process); !strings.Contains(said.Error(), "printed: Catastrophic failure") {
		t.Errorf("the failure does not carry what wsl.exe printed: %v", said)
	}
	if !retryableListFailure(err) {
		t.Error("a listing that failed for no stated reason is not asked once more")
	}
	for label, refused := range map[string]error{
		"a refusal of this process": fmt.Errorf("%w: access is denied", ErrWslDenied),
		"a listing that timed out":  &ProcessError{Code: ExitTimeout, Op: "process", Err: context.DeadlineExceeded},
	} {
		if retryableListFailure(refused) {
			t.Errorf("%s is asked again, which changes nothing and doubles the wait", label)
		}
	}
}
