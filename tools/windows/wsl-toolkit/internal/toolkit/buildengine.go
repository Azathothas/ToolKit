// SPDX-License-Identifier: 0BSD

package toolkit

import "fmt"

// BuildEngine is the host engine a build exported the base's rootfs through, and
// the host that engine answered from. WSL-188.
//
// ⛔ THE BUILD'S LOG SAID IT, AND ONLY ON THE DIRECT ROUTE. Through the helper that
// log is the helper's, and no field said it, so a caller whose podman connection
// or Docker context named another machine of this architecture got a base built
// there without a word. The state carries it, so the helper's answer, `--json` and
// `ready --ensure` do.
type BuildEngine struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
	// Host is the host name the engine's `info` answered with. ⚠ Empty where the
	// engine names none, as wslc does, which is not the same as this machine.
	Host string `json:"host,omitempty"`
}

// String is the engine as a reader sees it in one line.
func (e BuildEngine) String() string {
	if e.Host == "" {
		return fmt.Sprintf("%s for %s", e.Name, e.Platform)
	}
	return fmt.Sprintf("%s for %s, answering from host %q", e.Name, e.Platform, e.Host)
}

// useEngine says which engine a build asks for its rootfs, and keeps it for the
// state the build answers. WSL-188.
func (b *Base) useEngine(e *Engine) {
	b.log(fmt.Sprintf("host engine: %s, platform %s", e.Name, e.Platform()))
	b.builtWith = &BuildEngine{Name: e.Name, Platform: e.Platform(), Host: e.Host}
}

// withBuild puts the engine this call built with into st. ⚠ Nil where this call
// built nothing, so a plain ensure of a registered base adds no field. WSL-188.
func (b *Base) withBuild(st BaseState) BaseState {
	if b.builtWith != nil {
		built := *b.builtWith
		st.BuiltWith = &built
	}
	return st
}
