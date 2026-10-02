package engine

import (
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// buildSegments maps authored segments to turn segments, joining narration in
// order. A speech segment whose speaker cannot be resolved falls back to
// narration rather than being dropped.
func buildSegments(sub *harness.TurnSubmission, resolve func(string) (string, bool)) (string, []entity.TurnSegment) {
	var narration strings.Builder
	segments := make([]entity.TurnSegment, 0, len(sub.Segments))
	for _, spec := range sub.Segments {
		if spec.Kind == "speech" {
			// The GM often names a speaker the way the narration links them, so
			// the wikilink is unwrapped before it is resolved and displayed.
			speaker := entity.WikilinkTarget(spec.Speaker)
			id, ok := resolve(speaker)
			if !ok {
				appendNarration(&narration, spec.Text)
				segments = append(segments, entity.TurnSegment{Kind: entity.SegmentNarration, Text: spec.Text, CheckRef: spec.CheckRef})
				continue
			}
			segments = append(segments, entity.TurnSegment{
				Kind:      entity.SegmentSpeech,
				Speaker:   speaker,
				SpeakerID: id,
				Text:      spec.Text,
				CheckRef:  spec.CheckRef,
			})
			continue
		}
		appendNarration(&narration, spec.Text)
		segments = append(segments, entity.TurnSegment{Kind: entity.SegmentNarration, Text: spec.Text, CheckRef: spec.CheckRef})
	}
	return strings.TrimSpace(narration.String()), segments
}

func appendNarration(b *strings.Builder, text string) {
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(strings.TrimSpace(text))
}

// extractionFromSubmission carries only the turn-level location; personae are
// staged separately so their state survives.
func extractionFromSubmission(sub *harness.TurnSubmission) harness.Extraction {
	return harness.Extraction{PlayerLocation: sub.PlayerLocation}
}

// speakerResolver resolves a speaker name against declared personae first, then
// the entity store.
func (o *TurnOrchestrator) speakerResolver(sub *harness.TurnSubmission) func(string) (string, bool) {
	declared := make(map[string]string, len(sub.Personae))
	for _, p := range sub.Personae {
		if id := entity.Slugify(p.Name); id != "" {
			declared[strings.ToLower(strings.TrimSpace(p.Name))] = id
		}
	}
	return func(name string) (string, bool) {
		if id, ok := declared[strings.ToLower(strings.TrimSpace(name))]; ok {
			return id, true
		}
		id := entity.Slugify(name)
		if id == "" {
			return "", false
		}
		if ent, err := o.store.GetEntity(id); err == nil && ent != nil {
			return ent.ID, true
		}
		if summaries, err := o.store.ListEntities(); err == nil {
			for _, summary := range summaries {
				if summary.ID == id || strings.EqualFold(summary.Name, name) {
					return summary.ID, true
				}
			}
		}
		return "", false
	}
}

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
// declaredStats is the mechanics schema's declared stats (nil when the system
// declares none, in which case any state path is allowed). proposed is the
// player's explicit roll, when this turn had one. engagement is the resolved
// mechanics policy, which decides whether resolved checks are allowed at all.
func validateSubmission(sub *harness.TurnSubmission, checks []harness.CheckResult, declaredStats map[string]core.StatSpec, proposed *harness.ProposedCheck, engagement string) error {
	if sub == nil {
		return &submissionError{Code: "no_submission", Detail: "empty submission"}
	}
	if len(sub.Segments) == 0 {
		return &submissionError{Code: "no_segments", Detail: "submission has no segments"}
	}

	// The off policy resolves nothing; the ask policy leaves resolution to the
	// player's roll, so a model-resolved check in either is a protocol error.
	if engagement == "off" && len(checks) > 0 {
		return &submissionError{Code: "checks_disabled", Detail: "mechanics are off"}
	}
	if engagement == "ask" && len(checks) > 0 {
		return &submissionError{Code: "check_not_player_rolled", Detail: "checks are resolved by the player's roll"}
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
		if len(declaredStats) > 0 {
			if _, ok := declaredStats[change.Path]; !ok {
				return &submissionError{Code: "undeclared_stat", Detail: change.Path}
			}
		}
	}

	for _, dismissed := range sub.DismissedChecks {
		if strings.TrimSpace(dismissed.Reason) == "" {
			return &submissionError{Code: "unjustified_dismissal", Detail: dismissed.CheckRef}
		}
	}

	// A player's explicit roll must be answered: either a check resolves it, or
	// the GM dismisses that exact proposal with a reason. An impossible action
	// needs no roll, so it is exempt.
	if proposed != nil && sub.Verdict.Feasibility != harness.FeasibilityImpossible {
		resolved := len(checks) > 0
		dismissed := false
		for _, entry := range sub.DismissedChecks {
			if entry.CheckRef == proposed.Ref && strings.TrimSpace(entry.Reason) != "" {
				dismissed = true
			}
		}
		if !resolved && !dismissed {
			return &submissionError{Code: "unresolved_proposed_check", Detail: proposed.Ref}
		}
	}

	return nil
}
