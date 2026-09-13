// SPDX-License-Identifier: 0BSD

package compat

import (
	"strings"
	"testing"
)

// -- the binder --------------------------------------------------------------

func TestParseBindsTheScriptSpellings(t *testing.T) {
	o, err := Parse([]string{"-Action", "New", "-Image", "alpine:3.22", "-Ephemeral", "-User", "agent"})
	if err != nil {
		t.Fatalf("the shape every existing caller spells was refused: %v", err)
	}
	if o.Action != ActionNew || o.Image != "alpine:3.22" || !o.Ephemeral || o.User != "agent" {
		t.Fatalf("the parameters did not bind: %+v", o)
	}
}

func TestParseBindsInlineAndCaseForms(t *testing.T) {
	// The script's host bound -Name:value, -Name=value and case-insensitive
	// spellings. A caller moving here gets the same latitude.
	cases := [][]string{
		{"-Action:List"},
		{"-Action=List"},
		{"-action", "List"},
		{"-Ac", "List"},
	}
	for _, args := range cases {
		o, err := Parse(args)
		if err != nil {
			t.Errorf("%v was refused: %v", args, err)
			continue
		}
		if o.Action != ActionList {
			t.Errorf("%v bound Action to %q", args, o.Action)
		}
	}
}

func TestParseResolvesUnambiguousPrefixesAndRefusesAmbiguousOnes(t *testing.T) {
	if o, err := Parse([]string{"-Act", "List"}); err != nil || o.Action != ActionList {
		t.Errorf("-Act is unambiguous and was not resolved to Action: %v, %v", o.Action, err)
	}
	// -Com is Command, CommandFile and CommandB64 at once. A host that picked
	// one for the caller would be a host guessing what somebody meant.
	if _, err := Parse([]string{"-Com", "x", "-Action", "Run"}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("an ambiguous prefix was not refused: %v", err)
	}
}

func TestParseRefusesWhatTheScriptRefused(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"a scalar named twice", []string{"-Action", "List", "-Image", "a", "-Image", "b"}, "more than once"},
		{"a value outside the ValidateSet", []string{"-Action", "Delete"}, "does not belong to the set"},
		{"a value outside an int range", []string{"-Action", "Run", "-Name", "x", "-Command", "true", "-TimeoutSeconds", "9999"}, "allowed range"},
		{"an unknown parameter", []string{"-Action", "List", "-Image2", "a"}, "cannot be found"},
		{"a positional argument", []string{"New"}, "positional"},
		{"no action at all", []string{"-Image", "alpine:3.22"}, "mandatory"},
		{"a trailing flag with no value", []string{"-Action", "List", "-Name"}, "missing an argument"},
		{"a bad switch value", []string{"-Action", "List", "-Force=maybe"}, "cannot convert"},
	}
	for _, c := range cases {
		if _, err := Parse(c.args); err == nil {
			t.Errorf("%s: %v was accepted", c.name, c.args)
		} else if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: the refusal does not say %q: %v", c.name, c.want, err)
		}
	}
}

func TestParseDefaultsMatchTheParameterBlock(t *testing.T) {
	o, err := Parse([]string{"-Action", "List"})
	if err != nil {
		t.Fatal(err)
	}
	if o.User != "root" {
		t.Errorf("User = %q, want the script's default root", o.User)
	}
	if o.TimeoutSeconds != 120 {
		t.Errorf("TimeoutSeconds = %d, want 120: the bound on the tool's own questions moved", o.TimeoutSeconds)
	}
	if o.TickSeconds != 30 {
		t.Errorf("TickSeconds = %d, want 30", o.TickSeconds)
	}
	if o.TimestampMode != "Relative" || o.Color != "auto" || o.TimestampSeparator != " " {
		t.Errorf("the rendering defaults moved: %+v", o)
	}
	if len(o.TickEscalateSeconds) != 3 || o.TickEscalateSeconds[0] != "120" {
		t.Errorf("the escalation thresholds moved: %v", o.TickEscalateSeconds)
	}
	if o.MaxLineBytes != 0 || o.CommandTimeoutSeconds != 0 {
		t.Errorf("the off switches no longer default to off: %+v", o)
	}
}

// TestParseTakesRepeatedListParameters is the deliberate IMPROVEMENT over the
// -File channel: a .ps1 run through -File could not have a parameter repeated,
// which is why -ScriptArgFile exists. The Go parser binds every occurrence,
// and the file form keeps working, so a caller moving here loses nothing.
func TestParseTakesRepeatedListParameters(t *testing.T) {
	o, err := Parse([]string{"-Action", "Run", "-Name", "x", "-Command", "true",
		"-ScriptArg", "A=1", "-ScriptArg", "B=2"})
	if err != nil {
		t.Fatalf("repeated -ScriptArg was refused: %v", err)
	}
	if len(o.ScriptArg) != 2 || o.ScriptArg[1] != "B=2" {
		t.Fatalf("ScriptArg = %v, want both pairs", o.ScriptArg)
	}
	// And the comma spelling splits, because a number list through -File
	// arrived as one string and a culture turned "5,9" into 59.
	o, err = Parse([]string{"-Action", "Run", "-Name", "x", "-Command", "true", "-TickEscalateSeconds", "5,9"})
	if err != nil {
		t.Fatal(err)
	}
	if len(o.TickEscalateSeconds) != 1 || o.TickEscalateSeconds[0] != "5,9" {
		t.Fatalf("the raw value moved: %v; the split belongs to the settings, which the selftest asserts", o.TickEscalateSeconds)
	}
}

func TestParseTracksWhatTheCallerActuallyPassed(t *testing.T) {
	o, err := Parse([]string{"-Action", "New", "-Image", "alpine:3.22", "-Force"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.wasPassed("Image") || !o.wasPassed("image") || !o.wasPassed("FORCE") {
		t.Error("an explicitly passed parameter was not recorded, so a preset would overrule it")
	}
	if o.wasPassed("Name") {
		t.Error("a parameter the caller never typed was recorded as passed")
	}
}

// -- the applicability table ---------------------------------------------------

func TestEveryParameterHasAnApplicabilityRow(t *testing.T) {
	// ⛔ A PARAMETER WITH NO ROW IS REFUSED EVERYWHERE. That is the failure
	// mode of forgetting, and it is loud by design; this case is what makes
	// the loudness impossible to miss before a release instead of after it.
	table := parameterApplicability()
	for _, b := range parameters() {
		if strings.EqualFold(b.name, "Action") {
			// -Action has no row because it IS the choice, exactly as the
			// script's own common parameters were skipped.
			continue
		}
		if _, ok := table[strings.ToLower(b.name)]; !ok {
			t.Errorf("-%s has no applicability row, so it is refused on every action. Add the row", b.name)
		}
	}
	// And the table must not outlive the parameter it names.
	for name := range table {
		found := false
		for _, b := range parameters() {
			if strings.ToLower(b.name) == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the applicability table names %q and no parameter has that name. Delete the row", name)
		}
	}
}

func TestAParameterTheActionDoesNotReadIsRefused(t *testing.T) {
	// The defect the table exists for: -Image passed to -Action List did
	// nothing and said nothing, so a caller who typed it believed something
	// was happening.
	o, err := Parse([]string{"-Action", "List", "-Image", "alpine:3.22"})
	if err != nil {
		t.Fatal(err)
	}
	err = assertParametersApplyToAction(&o)
	if err == nil {
		t.Fatal("-Image on -Action List was accepted, which is the defect back again")
	}
	if !strings.Contains(err.Error(), "-Image is read by -Action New") {
		t.Errorf("the refusal does not say who reads -Image: %v", err)
	}
}

func TestTimeoutSecondsOnRunNamesTheParameterTheCallerWants(t *testing.T) {
	o, err := Parse([]string{"-Action", "Run", "-Name", "x", "-Command", "true", "-TimeoutSeconds", "30"})
	if err != nil {
		t.Fatal(err)
	}
	err = assertParametersApplyToAction(&o)
	if err == nil {
		t.Fatal("-TimeoutSeconds on Run was accepted; the caller's bound is -CommandTimeoutSeconds")
	}
	if !strings.Contains(err.Error(), "-CommandTimeoutSeconds") {
		t.Errorf("the refusal does not name the parameter the caller actually wants: %v", err)
	}
}

func TestTheRenderParametersReachReplay(t *testing.T) {
	// Replay RENDERS, so every parameter that decides how a line is rendered
	// applies to it, and the ones that decide what is CAPTURED do not.
	values := map[string]string{
		"NoTimestamps": "", "TimestampMode": "Wall", "Redact": "s", "MaxLineBytes": "5", "Color": "never",
	}
	for _, name := range []string{"NoTimestamps", "TimestampMode", "Redact", "MaxLineBytes", "Color"} {
		spell := "-" + name
		if values[name] != "" {
			spell += ":" + values[name]
		}
		o, err := Parse(append([]string{"-Action", "Replay", "-From", "x.jsonl"}, spell))
		if err != nil {
			t.Fatalf("-%s on Replay was refused at binding: %v", name, err)
		}
		if err := assertParametersApplyToAction(&o); err != nil {
			t.Errorf("-%s on Replay: %v", name, err)
		}
	}
	o, err := Parse([]string{"-Action", "Replay", "-From", "x.jsonl", "-TickSeconds", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if err := assertParametersApplyToAction(&o); err == nil {
		t.Error("-TickSeconds on Replay was accepted; a heartbeat over a file that has already been read is not a heartbeat")
	}
}
