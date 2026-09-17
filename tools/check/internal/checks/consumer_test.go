// SPDX-License-Identifier: 0BSD

package checks

import (
	"strings"
	"testing"
)

// detail is one row of the report, spelled out here because the field is an
// anonymous struct on consumerReport and a case cannot name that type.
func detail(name string, pass, skipped bool, actual string) struct {
	Name    string `json:"name"`
	Pass    bool   `json:"pass"`
	Skipped bool   `json:"skipped"`
	Why     string `json:"why"`
	Actual  string `json:"actual"`
} {
	return struct {
		Name    string `json:"name"`
		Pass    bool   `json:"pass"`
		Skipped bool   `json:"skipped"`
		Why     string `json:"why"`
		Actual  string `json:"actual"`
	}{Name: name, Pass: pass, Skipped: skipped, Actual: actual}
}

func goodReport() consumerReport {
	r := consumerReport{
		Schema: "wsl-toolkit-consumer/1", OK: true, Exe: "/built/wsl-toolkit",
		Complete: true, Cases: 2, Failed: 0, Skipped: 1, Version: "3.1.0",
	}
	r.Detail = append(r.Detail, detail("a case that passed", true, false, "True"))
	r.Detail = append(r.Detail, detail("a case that skipped", false, true, ""))
	return r
}

// TestAConsumerRunThatProvedNothingIsRefused is WSL-91's guard, and every row
// here is a way a green-looking report can mean nothing.
//
// ⛔ THE VERDICT IS A FUNCTION WITH NO PROCESS IN IT so that this case can exist
// at all. Reaching it through Consumer would need a Go toolchain, PowerShell and
// a built binary, which is a verdict nothing can test on the host that would
// catch a mistake in it.
func TestAConsumerRunThatProvedNothingIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spoil func(*consumerReport)
		want  string
	}{
		{"a run that reached no case", func(r *consumerReport) {
			r.Cases, r.Detail = 0, nil
		}, "reached no case at all"},
		{"a run that did not reach every declared case", func(r *consumerReport) {
			r.Complete = false
		}, "did not reach every case it declares"},
		{"a run against some other binary", func(r *consumerReport) {
			r.Exe = "/somewhere/else/wsl-toolkit"
		}, "drove"},
		{"a schema this check does not know", func(r *consumerReport) {
			r.Schema = "wsl-toolkit-consumer/9"
		}, "answered schema"},
		{"a case the released-binary contract broke", func(r *consumerReport) {
			r.Detail = append(r.Detail, detail("the state directory it names is the one it was told to use",
				false, false, "config exited 2: a refusal\nand a second line"))
			r.Failed = 1
		}, "the released-binary contract is broken by this tree"},
		// ⛔ THE COUNT AND THE ROWS CAN DISAGREE, and believing they cannot is
		// how a report that lost its rows reads as a report with none.
		{"a failure count no row accounts for", func(r *consumerReport) {
			r.Failed = 3
		}, "named 0 of them"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rep := goodReport()
			tc.spoil(&rep)
			found := consumerFindings(rep, "/built/wsl-toolkit")
			if len(found) == 0 {
				t.Fatalf("this report was accepted and it should not have been")
			}
			if !strings.Contains(strings.Join(found, " | "), tc.want) {
				t.Fatalf("the finding does not say %q: %v", tc.want, found)
			}
		})
	}
}

// TestAGoodConsumerRunIsAccepted is the other half, and without it every row
// above would pass over a function that refuses everything.
func TestAGoodConsumerRunIsAccepted(t *testing.T) {
	if found := consumerFindings(goodReport(), "/built/wsl-toolkit"); len(found) != 0 {
		t.Fatalf("a clean report was refused: %v", found)
	}
}

// TestTheReportIsFoundBehindWhateverTheHostPrintedFirst holds trimToJSON to its
// job. A PowerShell host can put a banner ahead of a script's own output, and a
// check that reads the whole stream as JSON would report "not its report" about
// a run that answered perfectly.
func TestTheReportIsFoundBehindWhateverTheHostPrintedFirst(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"a clean document", `{"schema":"x"}`, `{"schema":"x"}`},
		{"a banner ahead of it", "a warning line\r\n{\"schema\":\"x\"}\r\n", `{"schema":"x"}`},
		{"trailing blank lines", "{\"schema\":\"x\"}\n\n", `{"schema":"x"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(trimToJSON([]byte(tc.in))); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
