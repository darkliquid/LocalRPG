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

func TestMultiSentenceSegmentReusesSentenceClips(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{VoiceID: "v1", Pitch: 1, SpeechRate: 1}

	// The streaming path writes the first sentence as its own clip.
	if _, err := pipeline.SynthesizeProvisional(context.Background(), entity.SegmentNarration, "", "The hall is quiet.", voice); err != nil {
		t.Fatalf("SynthesizeProvisional: %v", err)
	}

	segment := entity.TurnSegment{Kind: entity.SegmentNarration, Text: "The hall is quiet. Garrick nods."}
	if _, err := pipeline.SynthesizeSegment(context.Background(), segment, voice, nil); err != nil {
		t.Fatalf("SynthesizeSegment: %v", err)
	}
	// One call for the provisional sentence and one for the second; the first is
	// reused rather than synthesized again.
	if client.calls != 2 {
		t.Fatalf("calls = %d, want 2 (the first sentence reused)", client.calls)
	}

	// A second read is served from the concatenated segment clip.
	if _, err := pipeline.SynthesizeSegment(context.Background(), segment, voice, nil); err != nil {
		t.Fatalf("second SynthesizeSegment: %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("calls = %d, want the segment clip to be cached", client.calls)
	}
}

func TestSingleSentenceSegmentTakesTheDirectPath(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{VoiceID: "v1"}

	path, err := pipeline.SynthesizeSegment(context.Background(), entity.TurnSegment{
		Kind: entity.SegmentNarration, Text: "One line.",
	}, voice, nil)
	if err != nil {
		t.Fatalf("SynthesizeSegment: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want 1", client.calls)
	}
	if path == "" {
		t.Fatal("expected a clip path")
	}
}

// markdownTTSClient consumes Markdown itself, so its text must not be split.
type markdownTTSClient struct {
	calls int
}

func (c *markdownTTSClient) Synthesize(context.Context, string, *entity.VoiceConfig) ([]byte, error) {
	c.calls++
	return GenerateToneWAV(440, 0.01), nil
}

func (c *markdownTTSClient) SupportsMarkdown() bool { return true }

func TestMarkdownAwareClientIsNotSplit(t *testing.T) {
	client := &markdownTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{VoiceID: "v1"}

	if _, err := pipeline.SynthesizeSegment(context.Background(), entity.TurnSegment{
		Kind: entity.SegmentNarration, Text: "A *long* breath. Then another.",
	}, voice, nil); err != nil {
		t.Fatalf("SynthesizeSegment: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want the whole segment synthesized once", client.calls)
	}
}
