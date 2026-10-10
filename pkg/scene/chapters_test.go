package scene

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/engine"
	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestChaptersFromScenes(t *testing.T) {
	compiler := NewCompiler(twoLocationSource())
	script, err := compiler.Compile(context.Background(), "campaign-01", Options{})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if len(script.Chapters) != 2 {
		t.Fatalf("chapters = %+v, want 2", script.Chapters)
	}
	if script.Chapters[0].Title == "" {
		t.Fatalf("chapter 0 has no title: %+v", script.Chapters)
	}
	if script.Chapters[1].Start <= script.Chapters[0].Start {
		t.Fatalf("chapter starts must increase: %+v", script.Chapters)
	}
	if script.Chapters[0].Title != "Alden Tavern" || script.Chapters[1].Title != "Aldon Harbour" {
		t.Fatalf("unexpected chapter titles: %+v", script.Chapters)
	}
}

// TestSingleSceneHasOneChapter is the regression guard: a story in one place has a
// single chapter and no boundary.
func TestSingleSceneHasOneChapter(t *testing.T) {
	source := &fakeSource{
		turns: []engine.Turn{turn(1, "alden-tavern", entity.TurnSegment{Kind: entity.SegmentNarration, Text: "Warm light."})},
		locations: map[string]*entity.Entity{
			"alden-tavern": {ID: "alden-tavern", Name: "Alden Tavern", Type: "location"},
		},
	}
	script, err := NewCompiler(source).Compile(context.Background(), "campaign-01", Options{})
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if len(script.Chapters) != 1 {
		t.Fatalf("chapters = %+v, want one", script.Chapters)
	}
	if script.Chapters[0].Start != 0 {
		t.Fatalf("the first chapter should start at zero, got %v", script.Chapters[0].Start)
	}
}

func TestChaptersVTTCoversEveryScene(t *testing.T) {
	chapters := []Chapter{
		{Title: "Alden Tavern", Start: 0},
		{Title: "Aldon Harbour", Start: 4_000_000_000},
	}
	vtt := ChaptersVTT(chapters, 9_000_000_000)
	if !strings.HasPrefix(vtt, "WEBVTT") {
		t.Fatalf("vtt = %q, want a WEBVTT header", vtt)
	}
	if !strings.Contains(vtt, "Alden Tavern") || !strings.Contains(vtt, "Aldon Harbour") {
		t.Fatalf("vtt = %q, want both chapter titles", vtt)
	}
	if !strings.Contains(vtt, "00:00:00.000 --> 00:00:04.000") {
		t.Fatalf("vtt = %q, want the first chapter bounded by the second's start", vtt)
	}
	if !strings.Contains(vtt, "00:00:04.000 --> 00:00:09.000") {
		t.Fatalf("vtt = %q, want the last chapter bounded by the total", vtt)
	}
}
