package media

import (
	"slices"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestGroupFolderMatchesPlanGroupsWithoutABudget(t *testing.T) {
	lines := []SpeakerLine{
		{SpeakerID: "narrator", Label: "Narrator", Text: "The hall is quiet."},
		{SpeakerID: "narrator", Label: "Narrator", Text: "Cold air rushes in."},
		{SpeakerID: "garrick", Label: "Garrick", Text: "Keep walking."},
	}
	caps := TTSCapabilities{MaxSpeakers: 1}

	folder := NewGroupFolder(caps, 0)
	var flushed [][]SpeakerLine
	for _, line := range lines {
		flushed = append(flushed, folder.Add(line)...)
	}
	if group := folder.Flush(); group != nil {
		flushed = append(flushed, group)
	}

	if len(flushed) != 2 {
		t.Fatalf("folded %d groups, want 2: %#v", len(flushed), flushed)
	}
	if len(flushed[0]) != 2 || len(flushed[1]) != 1 {
		t.Fatalf("group sizes = %d,%d, want 2,1", len(flushed[0]), len(flushed[1]))
	}
}

func TestGroupFolderFlushesAtTheBudget(t *testing.T) {
	folder := NewGroupFolder(TTSCapabilities{MaxSpeakers: 1}, 10)
	if out := folder.Add(SpeakerLine{SpeakerID: "n", Label: "Narrator", Text: "One two."}); len(out) != 0 {
		t.Fatalf("premature flush: %#v", out)
	}
	out := folder.Add(SpeakerLine{SpeakerID: "n", Label: "Narrator", Text: "Three four five."})
	if len(out) == 0 {
		t.Fatal("the budget must force a flush")
	}
}

func TestGroupFolderFlushesWhenTheSpeakerChanges(t *testing.T) {
	folder := NewGroupFolder(TTSCapabilities{MaxSpeakers: 1}, 0)
	if out := folder.Add(SpeakerLine{SpeakerID: "a", Label: "A", Text: "One."}); len(out) != 0 {
		t.Fatalf("premature flush: %#v", out)
	}
	out := folder.Add(SpeakerLine{SpeakerID: "b", Label: "B", Text: "Two."})
	if len(out) != 1 || len(out[0]) != 1 || out[0][0].SpeakerID != "a" {
		t.Fatalf("a speaker change must close the previous group, got %#v", out)
	}
	if rest := folder.Flush(); len(rest) != 1 || rest[0].SpeakerID != "b" {
		t.Fatalf("the new speaker's line must remain pending, got %#v", rest)
	}
}

// TestGroupFolderMatchesPlanGroups proves the streaming fold and the batch plan
// agree, so a streamed group and a finalised group share a cache key.
func TestGroupFolderMatchesPlanGroups(t *testing.T) {
	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The hall is quiet."},
		{Kind: entity.SegmentNarration, Text: "Cold air rushes in."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
		{Kind: entity.SegmentNarration, Text: strings.Repeat("Long. ", 200)},
	}
	caps := TTSCapabilities{MaxSpeakers: 1, MaxCharsPerRequest: 40}

	resolve := func(segment entity.TurnSegment) (SpeakerLine, bool) {
		label := narratorLabel
		if segment.Kind == entity.SegmentSpeech {
			label = segment.Speaker
		}
		return SpeakerLine{SpeakerID: segment.SpeakerID, Label: label, Text: segment.Text}, true
	}

	want := planGroups(segments, caps, resolve)

	folder := NewGroupFolder(caps, 0)
	var got [][]SpeakerLine
	for _, segment := range segments {
		line, _ := resolve(segment)
		got = append(got, folder.Add(line)...)
	}
	if group := folder.Flush(); group != nil {
		got = append(got, group)
	}

	if len(got) != len(want) {
		t.Fatalf("folded %d groups, want %d", len(got), len(want))
	}
	for i := range want {
		if len(got[i]) != len(want[i].Lines) {
			t.Fatalf("group %d has %d lines, want %d", i, len(got[i]), len(want[i].Lines))
		}
		for j := range want[i].Lines {
			if got[i][j].Text != want[i].Lines[j].Text || got[i][j].SpeakerID != want[i].Lines[j].SpeakerID {
				t.Fatalf("group %d line %d = %#v, want %#v", i, j, got[i][j], want[i].Lines[j])
			}
		}
	}
}

// TestGroupFolderSegmentIndexesMatchPlan proves the streamer's segment-index
// attribution and the plan's group indexes agree, so a streamed group and a
// finalised group cover the same segments.
func TestGroupFolderSegmentIndexesMatchPlan(t *testing.T) {
	segments := []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "The hall is quiet."},
		{Kind: entity.SegmentNarration, Text: "Cold air rushes in."},
		{Kind: entity.SegmentSpeech, Speaker: "Garrick", SpeakerID: "garrick", Text: "Keep walking."},
		{Kind: entity.SegmentNarration, Text: "Then silence."},
	}
	caps := TTSCapabilities{MaxSpeakers: 1}

	resolve := func(segment entity.TurnSegment) (SpeakerLine, bool) {
		label := narratorLabel
		if segment.Kind == entity.SegmentSpeech {
			label = segment.Speaker
		}
		return SpeakerLine{SpeakerID: segment.SpeakerID, Label: label, Text: segment.Text}, true
	}

	want := planGroups(segments, caps, resolve)

	folder := NewGroupFolder(caps, 0)
	var gotGroups [][]SpeakerLine
	for _, segment := range segments {
		line, _ := resolve(segment)
		gotGroups = append(gotGroups, folder.Add(line)...)
	}
	if group := folder.Flush(); group != nil {
		gotGroups = append(gotGroups, group)
	}

	// Attribute one segment index per line, in feed order, as the streamer does.
	queue := make([]int, 0, len(segments))
	for i := range segments {
		queue = append(queue, i)
	}
	last := 0
	gotIndexes := make([][]int, 0, len(gotGroups))
	for _, group := range gotGroups {
		indexes := make([]int, 0, len(group))
		for range group {
			if len(queue) > 0 {
				last = queue[0]
				queue = queue[1:]
			}
			indexes = append(indexes, last)
		}
		gotIndexes = append(gotIndexes, indexes)
	}

	if len(gotIndexes) != len(want) {
		t.Fatalf("folded %d groups, want %d", len(gotIndexes), len(want))
	}
	for i := range want {
		if !slices.Equal(gotIndexes[i], want[i].SegmentIndexes) {
			t.Fatalf("group %d indexes = %v, want %v", i, gotIndexes[i], want[i].SegmentIndexes)
		}
	}
}
