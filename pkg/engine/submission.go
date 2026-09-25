package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/harness"
)

// submissionError is a bounded reason a structured turn was rejected.
type submissionError struct {
	Code   string
	Detail string
}

func (e *submissionError) Error() string {
	if e.Detail == "" {
		return e.Code
	}
	return e.Code + ": " + e.Detail
}

// validateSubmission audits a structured turn against the checks it resolved.
// declaredStats is the mechanics schema's declared stat ids (nil when the system
// declares none, in which case any state path is allowed).
func validateSubmission(sub *harness.TurnSubmission, checks []harness.CheckResult, declaredStats map[string]bool) error {
	if sub == nil {
		return &submissionError{Code: "no_submission", Detail: "empty submission"}
	}
	if len(sub.Segments) == 0 {
		return &submissionError{Code: "no_segments", Detail: "submission has no segments"}
	}

	switch sub.Verdict.Feasibility {
	case harness.FeasibilityUncertain:
		if len(checks) == 0 {
			return &submissionError{Code: "no_check", Detail: "uncertain action with no resolved check"}
		}
	case harness.FeasibilityImpossible:
		if len(checks) > 0 {
			return &submissionError{Code: "impossible_with_check", Detail: "impossible action resolved a check"}
		}
	}

	ids := make(map[string]bool, len(checks))
	for _, check := range checks {
		ids[check.CheckID] = true
	}
	for _, segment := range sub.Segments {
		if segment.CheckRef != "" && !ids[segment.CheckRef] {
			return &submissionError{Code: "unknown_check", Detail: "segment references unknown check " + segment.CheckRef}
		}
	}

	for _, change := range sub.StateChanges {
		if len(declaredStats) > 0 && !declaredStats[change.Path] {
			return &submissionError{Code: "undeclared_stat", Detail: change.Path}
		}
	}

	for _, dismissed := range sub.DismissedChecks {
		if strings.TrimSpace(dismissed.Reason) == "" {
			return &submissionError{Code: "unjustified_dismissal", Detail: dismissed.CheckRef}
		}
	}

	return nil
}
