package engine

import "github.com/darkliquid/localrpg/pkg/harness"

// findPendingCheck returns the newest pending check with the given ref, or nil.
func findPendingCheck(turns []Turn, ref string) *harness.PendingCheck {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].PendingCheck != nil && turns[i].PendingCheck.Ref == ref {
			return turns[i].PendingCheck
		}
	}
	return nil
}
