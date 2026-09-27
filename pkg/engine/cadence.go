package engine

// forceCheckNudge is appended when the cadence floor fires but the provider
// cannot be told to force a tool call.
const forceCheckNudge = "The recent turns resolved without a check; if this action carries any consequence, call request_check before narrating."

// quietTurns counts the trailing turns that resolved no checks.
func quietTurns(turns []Turn) int {
	n := 0
	for i := len(turns) - 1; i >= 0; i-- {
		if len(turns[i].Checks) > 0 {
			break
		}
		n++
	}
	return n
}

// shouldForceCheck reports whether the engagement policy's cadence floor is due:
// auto only, a positive cadence, and enough quiet turns behind it.
func shouldForceCheck(engagement string, cadence int, turns []Turn) bool {
	if engagement != "auto" || cadence <= 0 {
		return false
	}
	return quietTurns(turns) >= cadence
}
