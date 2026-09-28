package media

import (
	"context"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestSplitSentences(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"One. Two! Three?", []string{"One.", "Two!", "Three?"}},
		{"Dr. Smith arrived. Then he left.", []string{"Dr. Smith arrived.", "Then he left."}},
		{"Pi is 3.14 roughly.", []string{"Pi is 3.14 roughly."}},
		{"Run `echo hi.` now. Done.", []string{"Run `echo hi.` now.", "Done."}},
		{"Line one\nLine two", []string{"Line one", "Line two"}},
		{`He said "Go." Then left.`, []string{`He said "Go."`, "Then left."}},
		{"No terminator here", []string{"No terminator here"}},
		{"", nil},
	}

	for _, tc := range cases {
		got := SplitSentences(tc.text)
		if len(got) != len(tc.want) {
			t.Errorf("SplitSentences(%q) = %#v, want %#v", tc.text, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("SplitSentences(%q)[%d] = %q, want %q", tc.text, i, got[i], tc.want[i])
			}
		}
	}
}

func TestSplitCompleteSentencesHoldsTheTail(t *testing.T) {
	complete, remainder := SplitCompleteSentences("One. Tw")
	if len(complete) != 1 || complete[0] != "One." {
		t.Fatalf("complete = %#v, want [One.]", complete)
	}
	if remainder != " Tw" {
		t.Fatalf("remainder = %q, want %q", remainder, " Tw")
	}
}

func TestSynthesizeProvisionalSharesTheFinalSegmentCacheKey(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{VoiceID: "v1", Pitch: 1, SpeechRate: 1}

	if _, err := pipeline.SynthesizeProvisional(context.Background(), entity.SegmentNarration, "", "The hall is quiet.", voice); err != nil {
		t.Fatalf("SynthesizeProvisional: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("provisional calls = %d, want 1", client.calls)
	}

	segment := entity.TurnSegment{Kind: entity.SegmentNarration, Text: "The hall is quiet."}
	if _, err := pipeline.SynthesizeSegment(context.Background(), segment, voice, nil); err != nil {
		t.Fatalf("SynthesizeSegment: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want the final segment to reuse the provisional clip", client.calls)
	}
}
