package sysgen

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/systemtest"
)

// GateResult reports whether a system may be saved. A failure means the system
// does not load or its declared check does not resolve; the gate makes no model
// call, so it is cheap enough to run on every save.
type GateResult struct {
	OK       bool                 `json:"ok"`
	Failures []systemtest.Failure `json:"failures,omitempty"`
	Script   bool                 `json:"script,omitempty"`
}

// Gate runs the smoke scenario against a system and reports whether it may be
// saved. A system with no declared mechanics passes trivially, because the gate
// is about a declared system working, not about requiring declarations.
func Gate(sys System) GateResult {
	runner := systemtest.System{ID: sys.ID, Script: sys.Script, Mechanics: sys.Mechanics}
	failures := systemtest.Run(runner, systemtest.SmokeScenario(sys.Mechanics))
	return GateResult{
		OK:       len(failures) == 0,
		Failures: failures,
		Script:   strings.TrimSpace(sys.Script) != "",
	}
}

// FailureText renders a gate's failures as one line, for a warning or an error.
func (r GateResult) FailureText() string {
	if len(r.Failures) == 0 {
		return ""
	}
	parts := make([]string, 0, len(r.Failures))
	for _, failure := range r.Failures {
		if failure.Detail != "" {
			parts = append(parts, failure.Detail)
		}
	}
	return strings.Join(parts, "; ")
}
