package engine

import (
	"strings"

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
			id, ok := resolve(spec.Speaker)
			if !ok {
				appendNarration(&narration, spec.Text)
				segments = append(segments, entity.TurnSegment{Kind: entity.SegmentNarration, Text: spec.Text})
				continue
			}
			segments = append(segments, entity.TurnSegment{
				Kind:      entity.SegmentSpeech,
				Speaker:   spec.Speaker,
				SpeakerID: id,
				Text:      spec.Text,
			})
			continue
		}
		appendNarration(&narration, spec.Text)
		segments = append(segments, entity.TurnSegment{Kind: entity.SegmentNarration, Text: spec.Text})
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
