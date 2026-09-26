package engine

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/harness"
)

// TestNonStructuredTurnStillUsesExtraction asserts the extraction result reaches
// segment building, which the overlap refactor must not break.
func TestNonStructuredTurnStillUsesExtraction(t *testing.T) {
	extraction := harness.Extraction{
		Entities: []harness.ExtractedEntity{{ID: "mira", Name: "Mira", Type: "character", Body: "A scout."}},
		Dialogue: []harness.ExtractedDialogue{{Speaker: "Mira", Text: "Hold the line."}},
	}

	segments := buildTurnSegments(nil, "Mira says, \"Hold the line.\"", extraction)
	found := false
	for _, seg := range segments {
		if seg.Kind == entity.SegmentSpeech && seg.Speaker == "Mira" {
			found = true
		}
	}
	if !found {
		t.Fatalf("extracted dialogue missing from segments: %+v", segments)
	}
}
