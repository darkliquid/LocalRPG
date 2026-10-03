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

// findResolvedCheck returns the check a previous turn already resolved for the
// given pending ref, so a retried request reuses it rather than rolling again.
func findResolvedCheck(turns []Turn, ref string) *harness.CheckResult {
	if ref == "" {
		return nil
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].ResolvesCheckRef != ref || len(turns[i].Checks) == 0 {
			continue
		}
		resolved := turns[i].Checks[0]
		return &resolved
	}
	return nil
}

// findPendingTurn returns the number of the turn carrying the pending check, or 0
// when none does.
func findPendingTurn(turns []Turn, ref string) int {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].PendingCheck != nil && turns[i].PendingCheck.Ref == ref {
			return turns[i].Number
		}
	}
	return 0
}
