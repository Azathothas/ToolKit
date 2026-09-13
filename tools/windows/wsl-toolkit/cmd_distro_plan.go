// SPDX-License-Identifier: 0BSD

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// distroPlan is what a --dry-run prints: every step the real run would take, read
// from the same selections the real run reads, with nothing changed.
//
// ⛔ IT CARRIES NO COMMAND TEXT AND NO ENVIRONMENT VALUE. A command or a value can
// hold a credential, and a plan is written to a terminal or a log a caller keeps;
// the size, a digest and the variable names identify what would run without
// repeating it.
type distroPlan struct {
	Schema    string       `json:"schema"`
	Action    string       `json:"action"`
	DryRun    bool         `json:"dry_run"`
	Name      string       `json:"name,omitempty"`
	NameDrawn bool         `json:"name_drawn,omitempty"`
	Steps     []string     `json:"steps"`
	Command   *planCommand `json:"command,omitempty"`
	Log       *planLog     `json:"log,omitempty"`
}

type planCommand struct {
	Invocation string   `json:"invocation"`
	User       string   `json:"user"`
	Bytes      int      `json:"bytes"`
	SHA256     string   `json:"sha256"`
	EnvNames   []string `json:"env_names,omitempty"`
	UserEnv    bool     `json:"user_env"`
	Stdin      string   `json:"stdin"`
	Timeout    string   `json:"timeout"`
}

type planLog struct {
	toolkit.LogSettings
	Tick     string   `json:"tick,omitempty"`
	Escalate []string `json:"escalate,omitempty"`
	Renders  bool     `json:"renders_live_output"`
}

func newDistroPlan(action, name string) *distroPlan {
	return &distroPlan{Schema: "wsl-toolkit-distro-plan/1", Action: action, DryRun: true, Name: name, Steps: []string{}}
}

func (p *distroPlan) step(s string) { p.Steps = append(p.Steps, s) }

// addCommand describes the command half of new and run.
func (p *distroPlan) addCommand(name string, spec toolkit.ThrowawaySpec, payload commandPayload, s toolkit.LogSettings) {
	if payload.script == nil {
		return
	}
	composed := toolkit.ComposePayload(spec.Script, spec.Env, spec.UserEnv)
	sum := sha256.Sum256(composed)
	cmd := &planCommand{
		Invocation: "wsl.exe -d " + name + " -u " + spec.User + " -- /bin/sh -l",
		User:       spec.User, Bytes: len(composed), SHA256: hex.EncodeToString(sum[:]),
		UserEnv: spec.UserEnv, Stdin: "/dev/null", Timeout: spec.Timeout.String(),
	}
	for _, e := range spec.Env {
		cmd.EnvNames = append(cmd.EnvNames, e.Name)
	}
	if spec.Timeout == 0 {
		cmd.Timeout = "none"
	}
	p.Command = cmd
	p.step(fmt.Sprintf("run %d bytes from %s as %s in a login shell, framed on stdin, with /dev/null as its stdin", len(composed), payload.source, spec.User))
	if !s.Active() {
		p.step("forward the command's streams unchanged and answer its exit code")
		return
	}
	pl := &planLog{LogSettings: s, Renders: s.Renders()}
	if s.Tick > 0 {
		pl.Tick = s.Tick.String()
		for _, e := range s.Escalate {
			pl.Escalate = append(pl.Escalate, e.String())
		}
	}
	p.Log = pl
	if s.Renders() {
		p.step("relay the command line by line through the renderer: " + describeRender(s))
	} else {
		p.step("forward the live streams byte for byte, and split them into lines for the sinks and the heartbeat")
	}
	if s.TextPath != "" {
		verb := "append the uncoloured rendered lines to "
		if s.TextOverwrite {
			verb = "replace, when the command starts, the uncoloured rendered lines at "
		}
		p.step(verb + s.TextPath)
	}
	if s.EventPath != "" {
		p.step("append " + toolkit.EventLogSchema + " records to " + s.EventPath)
	}
	if s.RedactCount > 0 {
		p.step(fmt.Sprintf("replace the matches of %d pattern(s) with *** before any sink sees a line", s.RedactCount))
	}
	if s.Tick > 0 {
		p.step("write a heartbeat after " + s.Tick.String() + " of silence, reading the distribution's state and disk")
	}
}

func describeRender(s toolkit.LogSettings) string {
	var parts []string
	if len(s.Columns) > 0 {
		parts = append(parts, "columns "+strings.Join(s.Columns, ","))
	}
	if s.MaxLineBytes > 0 {
		parts = append(parts, fmt.Sprintf("lines cut at %d bytes", s.MaxLineBytes))
	}
	if s.ProgressPrefix != "" {
		parts = append(parts, "progress lines starting "+s.ProgressPrefix+" consumed")
	}
	if s.RedactCount > 0 {
		parts = append(parts, "redaction")
	}
	return strings.Join(parts, ", ")
}

// planNew describes `distro new` from read-only questions: which engine and
// platform an image would be pulled with, which distribution a reuse would pick,
// and where the disk would go.
func planNew(ctx context.Context, t *toolkit.Throwaways, spec toolkit.ThrowawaySpec, payload commandPayload, s toolkit.LogSettings) (*distroPlan, error) {
	plan := newDistroPlan("new", spec.Name)
	if spec.Reuse {
		found, err := t.FindReusable(ctx, spec.Image)
		if err != nil {
			return nil, err
		}
		if found != nil {
			plan.Name = found.Name
			plan.step("reuse " + found.Name + ", built from " + spec.Image + ". It carries whatever the previous run left in it")
			plan.addCommand(found.Name, spec, payload, s)
			return plan, nil
		}
		plan.step("no owned distribution was built from " + spec.Image + ", so one is imported")
	}
	// ⛔ THE REAL RUN'S OWN REFUSALS: the archive or snapshot, the host engine,
	// and a --name already taken. A plan that passed what the run refuses would
	// be a dry run describing a different run.
	pre, err := t.Preflight(ctx, spec)
	if err != nil {
		return nil, err
	}
	if plan.Name == "" {
		builtFrom := firstNonEmpty(spec.Image, spec.Tarball)
		drawn, _, err := toolkit.ThrowawayName("", builtFrom)
		if err != nil {
			return nil, err
		}
		plan.Name, plan.NameDrawn = drawn, true
		plan.step("draw a name like " + drawn + ". A real run draws its own suffix")
	} else if name, _, err := toolkit.ThrowawayName(spec.Name, ""); err != nil {
		return nil, err
	} else {
		plan.Name = name
	}
	dir := filepath.Join(t.Dir(), plan.Name)
	switch {
	case pre.Engine != nil:
		plan.step(fmt.Sprintf("pull %s with %s for %s, and export its filesystem to %s", spec.Image, pre.Engine.Name, pre.Engine.Platform(),
			filepath.Join(t.Dir(), plan.Name+".tar")))
	case pre.Snapshot != "":
		plan.step("import the snapshot " + pre.Snapshot + " at " + pre.Rootfs + ", which carries whatever that distribution held")
	default:
		plan.step("import the rootfs archive " + pre.Rootfs)
	}
	plan.step(fmt.Sprintf("refuse unless %s is free beside twice the archive on the volume holding %s", toolkit.HumanBytes(toolkit.ThrowawaySpaceFloor), dir))
	plan.step("register it: wsl.exe --import " + plan.Name + " " + dir + " ROOTFS --version 2")
	plan.step("prove a login shell runs a framed command, within " + spec.ProbeTimeout.String())
	if spec.Systemd {
		plan.step("write /etc/wsl.conf with systemd=true, restart it, and refuse unless PID 1 is systemd")
	}
	if spec.OciEnv {
		plan.step("write the image's ENV and WORKDIR to /etc/profile.d/10-oci-env.sh")
	}
	plan.addCommand(plan.Name, spec, payload, s)
	if spec.Ephemeral {
		plan.step("unregister it and delete its directory once the command ends, even if the run is interrupted")
	}
	return plan, nil
}

func renderDistroPlan(plan *distroPlan, asJSON bool) error {
	if asJSON {
		return writeJSON(plan)
	}
	fmt.Fprintf(os.Stderr, "==> Dry-run: distro %s. Nothing is changed.\n", plan.Action)
	if plan.Name != "" {
		// The name is the one thing on stdout, as it is for a real `distro new`.
		fmt.Println(plan.Name)
	}
	for _, s := range plan.Steps {
		fmt.Fprintln(os.Stderr, "  would "+s)
	}
	if plan.Command != nil {
		env := "none"
		if len(plan.Command.EnvNames) > 0 {
			env = strings.Join(plan.Command.EnvNames, ",")
		}
		fmt.Fprintf(os.Stderr, "  command %d bytes, sha256 %s, timeout %s, env %s\n", plan.Command.Bytes, plan.Command.SHA256,
			plan.Command.Timeout, env)
	}
	return nil
}
