// SPDX-License-Identifier: 0BSD

package compat

import (
	"fmt"
	"strings"
)

// The compatibility interface, ported from wsl-toolkit.ps1 into this
// executable. The PowerShell product remains a separate artefact for the
// callers who run it directly; the executable no longer carries or launches a
// copy of it. Everything the script's command line accepted is accepted here,
// with the same refusals, the same streams and the same exit codes, so a
// caller who drove one drives the other.

// Action is one of the twelve things the interface can do. The names are the
// PowerShell action names, because they are what existing callers spell.
type Action string

// The twelve actions, with the spellings the PowerShell surface defined.
const (
	ActionNew         Action = "New"
	ActionRun         Action = "Run"
	ActionEnter       Action = "Enter"
	ActionList        Action = "List"
	ActionRemove      Action = "Remove"
	ActionPurge       Action = "Purge"
	ActionResources   Action = "Resources"
	ActionHostAddress Action = "HostAddress"
	ActionDoctor      Action = "Doctor"
	ActionSnapshot    Action = "Snapshot"
	ActionReplay      Action = "Replay"
	ActionCompare     Action = "Compare"
)

// validActions is the ValidateSet of the $Action parameter.
var validActions = []Action{
	ActionNew, ActionRun, ActionEnter, ActionList, ActionRemove, ActionPurge,
	ActionResources, ActionHostAddress, ActionDoctor, ActionSnapshot,
	ActionReplay, ActionCompare,
}

// Options is every parameter the interface accepts, with the script's
// defaults. Which of them each action READS is the table in
// applicability.go, and a parameter handed to an action that does not read it
// is refused rather than ignored.
type Options struct {
	Action Action

	Image       string
	Tarball     string
	Name        string
	Command     string
	CommandFile string
	CommandB64  string
	User        string
	StateDir    string

	UserEnv   bool
	Ephemeral bool
	OciEnv    bool
	Systemd   bool

	// TimeoutSeconds bounds the questions THIS implementation asks a distro
	// for itself. The caller's own command is bounded by CommandTimeoutSeconds,
	// and deliberately not by this.
	TimeoutSeconds int

	NoTimestamps          bool
	TimestampMode         string
	TimestampFormat       string
	TickSeconds           int
	CommandTimeoutSeconds int
	Verbatim              bool
	ScriptArg             []string
	ScriptArgFile         string

	TimestampColumns    []string
	TimestampSeparator  string
	PrefixOnly          bool
	Color               string
	TimestampProfile    string
	StreamLogPath       string
	StreamLogOverwrite  bool
	EventLog            string
	Redact              []string
	MaxLineBytes        int
	TickEscalateSeconds []string

	DryRun         bool
	ProgressPrefix string
	As             string
	From           string
	Against        string
	Reuse          bool
	Force          bool

	// passed records the parameters the caller actually set, lower-cased,
	// which is what the applicability refusal and the preset expansion read.
	passed map[string]bool
}

// wasPassed reports whether the caller set a parameter explicitly, which is
// how a profile stays a starting point rather than a mode.
func (o *Options) wasPassed(name string) bool { return o.passed[strings.ToLower(name)] }

// markPassed records an explicit parameter during binding.
func (o *Options) markPassed(name string) {
	if o.passed == nil {
		o.passed = map[string]bool{}
	}
	o.passed[strings.ToLower(name)] = true
}

// defaultOptions is the script's parameter block.
func defaultOptions() Options {
	return Options{
		User:                "root",
		StateDir:            getenv("WSL_TOOLKIT_STATE_DIR"),
		TimeoutSeconds:      120,
		TimestampMode:       "Relative",
		TickSeconds:         30,
		ScriptArg:           []string{},
		TimestampSeparator:  " ",
		Color:               "auto",
		Redact:              []string{},
		MaxLineBytes:        0,
		TickEscalateSeconds: []string{"120", "300", "900"},
	}
}

// binding describes one bindable parameter.
type binding struct {
	name    string
	kind    kind
	setS    func(o *Options, value string) error
	setB    func(o *Options, v bool) error
	allowed []string // ValidateSet, for string kinds
	ranges  [2]int   // ValidateRange, for int kinds
}

type kind int

const (
	kindString kind = iota
	kindInt
	kindSwitch
	kindStrings
)

// parameters is the parameter table, in the order the script declares them.
// ⛔ A PARAMETER HERE WITHOUT A ROW IN applicability.go IS REFUSED ON EVERY
// ACTION, which is loud, and that is the rule working rather than failing.
func parameters() []binding {
	return []binding{
		{name: "Action", kind: kindString, allowed: actionNames(), setS: func(o *Options, v string) error {
			o.Action = Action(v)
			return nil
		}},
		{name: "Image", kind: kindString, setS: func(o *Options, v string) error { o.Image = v; return nil }},
		{name: "Tarball", kind: kindString, setS: func(o *Options, v string) error { o.Tarball = v; return nil }},
		{name: "Name", kind: kindString, setS: func(o *Options, v string) error { o.Name = v; return nil }},
		{name: "Command", kind: kindString, setS: func(o *Options, v string) error { o.Command = v; return nil }},
		{name: "CommandFile", kind: kindString, setS: func(o *Options, v string) error { o.CommandFile = v; return nil }},
		{name: "CommandB64", kind: kindString, setS: func(o *Options, v string) error { o.CommandB64 = v; return nil }},
		{name: "User", kind: kindString, setS: func(o *Options, v string) error { o.User = v; return nil }},
		{name: "StateDir", kind: kindString, setS: func(o *Options, v string) error { o.StateDir = v; return nil }},
		{name: "UserEnv", kind: kindSwitch, setB: func(o *Options, v bool) error { o.UserEnv = v; return nil }},
		{name: "Ephemeral", kind: kindSwitch, setB: func(o *Options, v bool) error { o.Ephemeral = v; return nil }},
		{name: "OciEnv", kind: kindSwitch, setB: func(o *Options, v bool) error { o.OciEnv = v; return nil }},
		{name: "Systemd", kind: kindSwitch, setB: func(o *Options, v bool) error { o.Systemd = v; return nil }},
		{name: "TimeoutSeconds", kind: kindInt, ranges: [2]int{5, 3600}, setS: func(o *Options, v string) error {
			return setInt(&o.TimeoutSeconds, v)
		}},
		{name: "NoTimestamps", kind: kindSwitch, setB: func(o *Options, v bool) error { o.NoTimestamps = v; return nil }},
		{name: "TimestampMode", kind: kindString, allowed: []string{"Relative", "Delta", "Wall", "Iso", "Epoch"}, setS: func(o *Options, v string) error {
			o.TimestampMode = v
			return nil
		}},
		{name: "TimestampFormat", kind: kindString, setS: func(o *Options, v string) error { o.TimestampFormat = v; return nil }},
		{name: "TickSeconds", kind: kindInt, ranges: [2]int{0, 86400}, setS: func(o *Options, v string) error {
			return setInt(&o.TickSeconds, v)
		}},
		{name: "CommandTimeoutSeconds", kind: kindInt, ranges: [2]int{0, 604800}, setS: func(o *Options, v string) error {
			return setInt(&o.CommandTimeoutSeconds, v)
		}},
		{name: "Verbatim", kind: kindSwitch, setB: func(o *Options, v bool) error { o.Verbatim = v; return nil }},
		{name: "ScriptArg", kind: kindStrings, setS: func(o *Options, v string) error { o.ScriptArg = append(o.ScriptArg, v); return nil }},
		{name: "ScriptArgFile", kind: kindString, setS: func(o *Options, v string) error { o.ScriptArgFile = v; return nil }},
		{name: "TimestampColumns", kind: kindStrings, setS: func(o *Options, v string) error { o.TimestampColumns = append(o.TimestampColumns, v); return nil }},
		{name: "TimestampSeparator", kind: kindString, setS: func(o *Options, v string) error { o.TimestampSeparator = v; return nil }},
		{name: "PrefixOnly", kind: kindSwitch, setB: func(o *Options, v bool) error { o.PrefixOnly = v; return nil }},
		{name: "Color", kind: kindString, allowed: []string{"auto", "always", "never"}, setS: func(o *Options, v string) error {
			o.Color = v
			return nil
		}},
		{name: "TimestampProfile", kind: kindString, allowed: []string{"human", "ci", "forensic", "wall", "raw"}, setS: func(o *Options, v string) error {
			o.TimestampProfile = v
			return nil
		}},
		{name: "StreamLogPath", kind: kindString, setS: func(o *Options, v string) error { o.StreamLogPath = v; return nil }},
		{name: "StreamLogOverwrite", kind: kindSwitch, setB: func(o *Options, v bool) error { o.StreamLogOverwrite = v; return nil }},
		{name: "EventLog", kind: kindString, setS: func(o *Options, v string) error { o.EventLog = v; return nil }},
		{name: "Redact", kind: kindStrings, setS: func(o *Options, v string) error { o.Redact = append(o.Redact, v); return nil }},
		{name: "MaxLineBytes", kind: kindInt, ranges: [2]int{0, 1048576}, setS: func(o *Options, v string) error {
			return setInt(&o.MaxLineBytes, v)
		}},
		{name: "TickEscalateSeconds", kind: kindStrings, setS: func(o *Options, v string) error {
			// ⛔ THE DEFAULT IS REPLACED BY THE FIRST EXPLICIT VALUE, not
			// appended to. The script's parameter default was @('120','300',
			// '900'), and passing a value BOUND the parameter; a caller who
			// passed 5 got 5 alone, not 5 after the three defaults.
			if !o.wasPassed("TickEscalateSeconds") {
				o.TickEscalateSeconds = nil
			}
			o.TickEscalateSeconds = append(o.TickEscalateSeconds, v)
			return nil
		}},
		{name: "DryRun", kind: kindSwitch, setB: func(o *Options, v bool) error { o.DryRun = v; return nil }},
		{name: "ProgressPrefix", kind: kindString, setS: func(o *Options, v string) error { o.ProgressPrefix = v; return nil }},
		{name: "As", kind: kindString, setS: func(o *Options, v string) error { o.As = v; return nil }},
		{name: "From", kind: kindString, setS: func(o *Options, v string) error { o.From = v; return nil }},
		{name: "Against", kind: kindString, setS: func(o *Options, v string) error { o.Against = v; return nil }},
		{name: "Reuse", kind: kindSwitch, setB: func(o *Options, v bool) error { o.Reuse = v; return nil }},
		{name: "Force", kind: kindSwitch, setB: func(o *Options, v bool) error { o.Force = v; return nil }},
	}
}

func actionNames() []string {
	out := make([]string, 0, len(validActions))
	for _, a := range validActions {
		out = append(out, string(a))
	}
	return out
}

func setInt(dst *int, v string) error {
	n, err := parseIntStrict(v)
	if err != nil {
		return fmt.Errorf("cannot convert value %q to type %q", v, "System.Int32")
	}
	*dst = n
	return nil
}

// Parse binds arguments the way the script's parameter block binds them: every
// value is named, names are case-insensitive with unambiguous-prefix
// matching, `-Name value` and `-Name:value` both bind, and a scalar parameter
// named twice is refused rather than resolved by precedence.
func Parse(args []string) (Options, error) {
	o := defaultOptions()
	table := parameters()
	byName := map[string]*binding{}
	for i := range table {
		byName[strings.ToLower(table[i].name)] = &table[i]
	}
	seen := map[string]bool{}

	for i := 0; i < len(args); i++ {
		tok := args[i]
		if !strings.HasPrefix(tok, "-") {
			return o, fmt.Errorf("a positional parameter cannot be found that accepts argument %q", tok)
		}
		name := tok[1:]
		value := ""
		hasValue := false
		for sepIdx := 0; sepIdx < len(name); sepIdx++ {
			// ⛔ THE FIRST SEPARATOR WINS, and only ':' or '=' count. A name
			// cannot contain either, so the scan is safe; the first one is
			// where the inline value starts, and what follows may itself be
			// empty, which is how `-Redact:` refuses rather than binding "".
			if c := name[sepIdx]; c == ':' || c == '=' {
				value = name[sepIdx+1:]
				name = name[:sepIdx]
				hasValue = true
				break
			}
		}
		b, err := resolveParameter(byName, name)
		if err != nil {
			return o, err
		}
		isSwitch := b.kind == kindSwitch
		switch {
		case hasValue:
			// `-Switch:$false` binds the boolean; that is the one way a switch
			// takes a value.
			if isSwitch {
				v, err := parseSwitchValue(value)
				if err != nil {
					return o, fmt.Errorf("parameter '%s' cannot be processed: %w", b.name, err)
				}
				if err := b.setB(&o, v); err != nil {
					return o, err
				}
				seen[strings.ToLower(b.name)] = true
				o.markPassed(b.name)
				continue
			}
			if err := bindValue(&o, b, value); err != nil {
				return o, err
			}
		case isSwitch:
			if err := b.setB(&o, true); err != nil {
				return o, err
			}
		default:
			// ⛔ A MISSING ARGUMENT IS A REFUSAL, NOT AN EMPTY STRING. A
			// trailing `-Image` bound as "" would fail much later, in a
			// message about a value nobody typed.
			if i+1 >= len(args) {
				return o, fmt.Errorf("missing an argument for parameter '%s'", b.name)
			}
			i++
			if err := bindValue(&o, b, args[i]); err != nil {
				return o, err
			}
		}
		// ⛔ A SCALAR NAMED TWICE IS REFUSED. PowerShell refuses it and so does
		// this: a precedence between two spellings of one parameter is a rule
		// nobody would remember. The slice kinds take every occurrence.
		key := strings.ToLower(b.name)
		if b.kind != kindStrings {
			if seen[key] {
				return o, fmt.Errorf("parameter '%s' is specified more than once", b.name)
			}
			seen[key] = true
		}
		o.markPassed(b.name)
	}

	if o.Action == "" {
		return o, fmt.Errorf("cannot process the command because of one or more missing mandatory parameters: Action")
	}
	if o.User == "" {
		// An explicitly empty -User falls back to the script's default rather
		// than reaching the guest as an argumentless -u.
		o.User = "root"
	}
	return o, nil
}

func bindValue(o *Options, b *binding, value string) error {
	if len(b.allowed) > 0 {
		ok := false
		for _, a := range b.allowed {
			if strings.EqualFold(a, value) {
				value = a
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("cannot validate argument on parameter '%s'. The argument %q does not belong to the set %q",
				b.name, value, strings.Join(b.allowed, ","))
		}
	}
	if b.kind == kindInt {
		n, err := parseIntStrict(value)
		if err != nil {
			return fmt.Errorf("cannot convert value %q to type %q. The value must be a whole number of seconds", value, "System.Int32")
		}
		if n < b.ranges[0] || n > b.ranges[1] {
			return fmt.Errorf("cannot validate argument on parameter '%s'. The %d argument is greater than the maximum allowed range of %d. Supply an argument that is greater than or equal to %d and less than or equal to %d",
				b.name, n, b.ranges[1], b.ranges[0], b.ranges[1])
		}
	}
	if b.kind == kindSwitch {
		return b.setB(o, true)
	}
	return b.setS(o, value)
}

// resolveParameter matches a name case-insensitively, then by unambiguous
// prefix, which is what the script's host did for every caller.
func resolveParameter(byName map[string]*binding, name string) (*binding, error) {
	if b, ok := byName[strings.ToLower(name)]; ok {
		return b, nil
	}
	var found *binding
	ambiguous := false
	for lower, b := range byName {
		if strings.HasPrefix(lower, strings.ToLower(name)) {
			if found != nil && found.name != b.name {
				ambiguous = true
			}
			found = b
		}
	}
	if ambiguous {
		return nil, fmt.Errorf("parameter cannot be processed because the parameter name %q is ambiguous. Possible matches: pick a longer spelling", name)
	}
	if found == nil {
		return nil, fmt.Errorf("a parameter cannot be found that matches parameter name %q", name)
	}
	return found, nil
}

func parseSwitchValue(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no", "":
		return false, nil
	}
	return false, fmt.Errorf("cannot convert value %q to type %q", v, "System.Management.Automation.SwitchParameter")
}
