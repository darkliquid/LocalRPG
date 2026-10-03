package media

import "testing"

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
		if out := folder.Add(line); out != nil {
			flushed = append(flushed, out)
		}
	}
	if out := folder.Flush(); out != nil {
		flushed = append(flushed, out)
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
	if out := folder.Add(SpeakerLine{SpeakerID: "n", Label: "Narrator", Text: "One two."}); out != nil {
		t.Fatalf("premature flush: %#v", out)
	}
	out := folder.Add(SpeakerLine{SpeakerID: "n", Label: "Narrator", Text: "Three four five."})
	if len(out) == 0 {
		t.Fatal("the budget must force a flush")
	}
}

func TestGroupFolderFlushesWhenTheSpeakerChanges(t *testing.T) {
	folder := NewGroupFolder(TTSCapabilities{MaxSpeakers: 1}, 0)
	if out := folder.Add(SpeakerLine{SpeakerID: "a", Label: "A", Text: "One."}); out != nil {
		t.Fatalf("premature flush: %#v", out)
	}
	out := folder.Add(SpeakerLine{SpeakerID: "b", Label: "B", Text: "Two."})
	if len(out) != 1 || out[0].SpeakerID != "a" {
		t.Fatalf("a speaker change must close the previous group, got %#v", out)
	}
	if rest := folder.Flush(); len(rest) != 1 || rest[0].SpeakerID != "b" {
		t.Fatalf("the new speaker's line must remain pending, got %#v", rest)
	}
}
