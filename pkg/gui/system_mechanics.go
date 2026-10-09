package gui

import (
	"github.com/darkliquid/localrpg/pkg/core"
)

// validateMechanics reports non-fatal problems with a system's mechanics block,
// so a client that bypasses the editor cannot persist a broken system. It returns
// nil when the block is absent or sound.
func validateMechanics(spec *core.MechanicsSpec) []string {
	return spec.Validate()
}
