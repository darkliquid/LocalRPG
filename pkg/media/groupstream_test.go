package media

import (
	"math/rand"
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

// propertyVoices gives the property test's speakers distinct voices, so the
// same-voice rule in canJoinGroup is exercised.
var propertyVoices = map[string]*entity.VoiceConfig{
	"garrick": {Provider: "mock", VoiceID: "v-garrick"},
	"evelyn":  {Provider: "mock", VoiceID: "v-evelyn"},
	"mara":    {Provider: "mock", VoiceID: "v-mara"},
}

// propertyResolve is the property test's segment-to-line resolver, mirroring
// TTSPipeline.SegmentLine: empty text resolves to nothing, speech carries its
// speaker and voice, narration is read by the narrator.
func propertyResolve(segment entity.TurnSegment) (SpeakerLine, bool) {
	if strings.TrimSpace(segment.Text) == "" {
		return SpeakerLine{}, false
	}
	if segment.Kind == entity.SegmentSpeech {
		return SpeakerLine{
			SpeakerID: segment.SpeakerID,
			Label:     segment.Speaker,
			Voice:     propertyVoices[segment.SpeakerID],
			Text:      segment.Text,
		}, true
	}
	return SpeakerLine{Label: narratorLabel, Text: segment.Text}, true
}

// foldSegments runs the streaming fold over a segment sequence and attributes a
// segment index to every emitted group the way the sentence streamer does: one
// index is queued per resolved line, and a group consumes one per line, reusing
// the last when a split line yields more groups than indexes.
func foldSegments(segments []entity.TurnSegment, caps TTSCapabilities, resolve func(entity.TurnSegment) (SpeakerLine, bool)) ([][]SpeakerLine, [][]int) {
	folder := NewGroupFolder(caps, 0)
	var groups [][]SpeakerLine
	var indexes [][]int
	queue := make([]int, 0, len(segments))
	last := 0
	take := func(n int) []int {
		out := make([]int, 0, n)
		for i := 0; i < n; i++ {
			if len(queue) > 0 {
				last = queue[0]
				queue = queue[1:]
			}
			out = append(out, last)
		}
		return out
	}
	emit := func(flushed [][]SpeakerLine) {
		for _, group := range flushed {
			groups = append(groups, group)
			indexes = append(indexes, take(len(group)))
		}
	}
	for i, segment := range segments {
		line, ok := resolve(segment)
		if !ok {
			continue
		}
		queue = append(queue, i)
		emit(folder.Add(line))
	}
	if group := folder.Flush(); group != nil {
		emit([][]SpeakerLine{group})
	}
	return groups, indexes
}

// randomSegments builds an arbitrary sequence of narration and speech segments,
// including empty and multi-sentence lines, so the fold and the plan meet the
// skip and split paths too.
func randomSegments(rng *rand.Rand) []entity.TurnSegment {
	speakers := []struct{ name, id string }{
		{"Garrick", "garrick"},
		{"Evelyn", "evelyn"},
		{"Mara", "mara"},
	}
	segments := make([]entity.TurnSegment, 0, 30)
	for i := 0; i < rng.Intn(31); i++ {
		text := randomText(rng)
		if rng.Intn(3) == 0 {
			speaker := speakers[rng.Intn(len(speakers))]
			segments = append(segments, entity.TurnSegment{
				Kind: entity.SegmentSpeech, Speaker: speaker.name, SpeakerID: speaker.id, Text: text,
			})
			continue
		}
		segments = append(segments, entity.TurnSegment{Kind: entity.SegmentNarration, Text: text})
	}
	return segments
}

// randomText returns a short phrase, an empty line, or a long multi-sentence run.
func randomText(rng *rand.Rand) string {
	if rng.Intn(5) == 0 {
		return strings.Repeat("A sentence about the hall. ", 1+rng.Intn(30))
	}
	words := []string{"the", "hall", "is", "quiet", "cold", "air", "rushes", "in"}
	parts := make([]string, 0, 12)
	for i := 0; i < rng.Intn(12); i++ {
		parts = append(parts, words[rng.Intn(len(words))])
	}
	return strings.Join(parts, " ")
}

// randomCaps varies the speaker budget and the request limits so every grouping
// boundary is exercised.
func randomCaps(rng *rand.Rand) TTSCapabilities {
	caps := TTSCapabilities{MaxSpeakers: 1 + rng.Intn(3)}
	if rng.Intn(2) == 0 {
		caps.MaxCharsPerRequest = 10 + rng.Intn(70)
	}
	if rng.Intn(3) == 0 {
		caps.MaxTokensPerRequest = 3 + rng.Intn(20)
	}
	return caps
}

// TestGroupFoldEqualsPlanProperty generalises TestGroupFolderMatchesPlanGroups to
// arbitrary segment sequences: the streaming fold and the batch plan must agree
// on the groups, their lines, and the segments each group covers, so a streamed
// clip and a finalised clip share a cache key.
func TestGroupFoldEqualsPlanProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 500; i++ {
		segments := randomSegments(rng)
		caps := randomCaps(rng)
		want := planGroups(segments, caps, propertyResolve)
		gotGroups, gotIndexes := foldSegments(segments, caps, propertyResolve)

		if len(gotGroups) != len(want) {
			t.Fatalf("case %d: folded %d groups, want %d for %#v", i, len(gotGroups), len(want), segments)
		}
		for j := range want {
			if len(gotGroups[j]) != len(want[j].Lines) {
				t.Fatalf("case %d group %d has %d lines, want %d", i, j, len(gotGroups[j]), len(want[j].Lines))
			}
			for k := range want[j].Lines {
				if gotGroups[j][k] != want[j].Lines[k] {
					t.Fatalf("case %d group %d line %d = %#v, want %#v", i, j, k, gotGroups[j][k], want[j].Lines[k])
				}
			}
			if !slices.Equal(gotIndexes[j], want[j].SegmentIndexes) {
				t.Fatalf("case %d group %d indexes = %v, want %v", i, j, gotIndexes[j], want[j].SegmentIndexes)
			}
		}
	}
}

// TestGroupFoldKeyEqualsPlanKeyProperty proves the streamed group key equals the
// finalised group key for every group of an arbitrary sequence, which is the
// guarantee that makes a streamed clip reusable by the turn.
func TestGroupFoldKeyEqualsPlanKeyProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for i := 0; i < 300; i++ {
		segments := randomSegments(rng)
		caps := randomCaps(rng)
		want := planGroups(segments, caps, propertyResolve)
		gotGroups, _ := foldSegments(segments, caps, propertyResolve)

		if len(gotGroups) != len(want) {
			t.Fatalf("case %d: folded %d groups, want %d", i, len(gotGroups), len(want))
		}
		for j := range want {
			foldKey := ComputeGroupCacheKey("mock", "m", gotGroups[j])
			planKey := ComputeGroupCacheKey("mock", "m", want[j].Lines)
			if foldKey != planKey {
				t.Fatalf("case %d group %d key = %s, want %s", i, j, foldKey, planKey)
			}
		}
	}
}
