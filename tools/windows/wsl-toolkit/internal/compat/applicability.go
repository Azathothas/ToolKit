// SPDX-License-Identifier: 0BSD

package compat

import (
	"fmt"
	"strings"
)

// parameterApplicability is which parameters each action actually READS.
//
// ⛔ THIS TABLE IS THE ANSWER TO A REAL DEFECT, not tidiness. -Image passed to
// -Action List did nothing and said nothing, so a caller who typed it believed
// something was happening. A parameter silently ignored by the action it was
// handed to is the caller's half of "a setting no code reads", and the refusal
// is the fix.
//
// ⭐ IT IS DERIVED FROM WHERE THE VALUE IS READ, not from what the help says.
// -TimeoutSeconds reaches the bounded capture and nothing else, so it belongs
// to New and Resources and is REFUSED on Run, where the parameter a caller
// actually wants is -CommandTimeoutSeconds. The refusal says that rather than
// only saying no.
func parameterApplicability() map[string][]Action {
	relay := []Action{ActionNew, ActionRun}
	// Replay RENDERS, so every parameter that decides how a line is rendered
	// applies to it, and the ones that decide what is CAPTURED do not.
	render := append(append([]Action{}, relay...), ActionReplay)
	return map[string][]Action{
		"image":                 {ActionNew},
		"tarball":               {ActionNew},
		"name":                  {ActionNew, ActionRun, ActionEnter, ActionRemove, ActionSnapshot},
		"command":               relay,
		"commandfile":           relay,
		"commandb64":            relay,
		"user":                  {ActionNew, ActionRun, ActionEnter},
		"statedir":              {ActionNew, ActionRun, ActionEnter, ActionList, ActionRemove, ActionPurge, ActionResources, ActionHostAddress, ActionDoctor, ActionSnapshot},
		"userenv":               relay,
		"ephemeral":             {ActionNew},
		"ocienv":                {ActionNew},
		"systemd":               {ActionNew},
		"timeoutseconds":        {ActionNew, ActionResources},
		"verbatim":              relay,
		"scriptarg":             relay,
		"scriptargfile":         relay,
		"notimestamps":          render,
		"timestampmode":         render,
		"timestampformat":       render,
		"timestampcolumns":      render,
		"timestampseparator":    render,
		"timestampprofile":      render,
		"prefixonly":            render,
		"color":                 render,
		"streamlogpath":         relay,
		"streamlogoverwrite":    relay,
		"eventlog":              relay,
		"redact":                render,
		"maxlinebytes":          render,
		"tickseconds":           relay,
		"tickescalateseconds":   relay,
		"commandtimeoutseconds": relay,
		"progressprefix":        relay,
		// Snapshot names the distro with -Name and the tag with -As; New reads a
		// tag back through -Tarball, which already applies to it.
		"as":      {ActionSnapshot},
		"from":    {ActionReplay, ActionCompare},
		"against": {ActionCompare},
		"reuse":   {ActionNew},
		"dryrun":  {ActionNew, ActionRun, ActionEnter, ActionRemove, ActionPurge, ActionSnapshot},
		"force":   {ActionNew, ActionRemove, ActionPurge, ActionSnapshot},
	}
}

// assertParametersApplyToAction refuses a parameter the chosen action does not
// read.
//
// ⛔ THIS IS A BREAK AND IT IS MEANT TO BE. A caller who was passing a
// parameter that did nothing gets a refusal on the first run, which is the
// point: they were not getting what they asked for and nothing told them.
func assertParametersApplyToAction(o *Options) error {
	table := parameterApplicability()
	var bad []string
	for name := range o.passed {
		if name == "action" {
			// -Action is not in the table because it IS the choice.
			continue
		}
		rows, ok := table[name]
		if !ok {
			b := findParameter(name)
			label := "-" + name
			if b != nil {
				label = "-" + b.name
			}
			bad = append(bad, fmt.Sprintf("%s has no entry in this interface's parameter table, so it cannot be checked against -Action %s", label, o.Action))
			continue
		}
		if !containsAction(rows, o.Action) {
			names := make([]string, 0, len(rows))
			for _, a := range rows {
				names = append(names, string(a))
			}
			b := findParameter(name)
			label := "-" + name
			if b != nil {
				label = "-" + b.name
			}
			bad = append(bad, fmt.Sprintf("%s is read by -Action %s and not by -Action %s", label, strings.Join(names, ", "), o.Action))
		}
	}
	if len(bad) == 0 {
		return nil
	}
	hint := ""
	if o.wasPassed("TimeoutSeconds") && o.Action == ActionRun {
		hint = " -TimeoutSeconds bounds the questions this tool asks a distro for itself. The bound on YOUR command is -CommandTimeoutSeconds."
	}
	return fmt.Errorf("-Action %s ignores parameters you passed, so it would have done something other than what you asked: %s.%s",
		o.Action, strings.Join(bad, "; "), hint)
}

func findParameter(lower string) *binding {
	for _, b := range parameters() {
		if strings.ToLower(b.name) == lower {
			return &b
		}
	}
	return nil
}

func containsAction(rows []Action, a Action) bool {
	for _, r := range rows {
		if r == a {
			return true
		}
	}
	return false
}
