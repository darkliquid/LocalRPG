package gui

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestSegmentDTOsCarryCheckRef(t *testing.T) {
	segments := []entity.TurnSegment{
		{Kind: "narration", Text: "A roll.", CheckRef: "chk_1"},
		{Kind: "narration", Text: "No roll."},
	}
	dtos := segmentDTOs(segments, "test-game", clipPlan{}, nil)
	if dtos[0].CheckRef != "chk_1" {
		t.Fatalf("dtos[0].CheckRef = %q, want chk_1", dtos[0].CheckRef)
	}
	if dtos[1].CheckRef != "" {
		t.Fatalf("dtos[1].CheckRef = %q, want empty", dtos[1].CheckRef)
	}
}
