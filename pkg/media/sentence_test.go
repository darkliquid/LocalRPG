package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	if _, err := pipeline.SynthesizeSegmentClips(context.Background(), segment, voice, nil, false); err != nil {
		t.Fatalf("SynthesizeSegmentClips: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want the final segment to reuse the provisional clip", client.calls)
	}
}

func TestSegmentClipKeysNamesTheClipsSynthesisWrites(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{VoiceID: "v1", Pitch: 1, SpeechRate: 1}
	segment := entity.TurnSegment{Kind: entity.SegmentNarration, Text: "One. Two."}

	keys, err := pipeline.SegmentClipKeys(segment, voice, nil)
	if err != nil {
		t.Fatalf("SegmentClipKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("keys = %d, want one per sentence", len(keys))
	}

	clips, err := pipeline.SynthesizeSegmentClips(context.Background(), segment, voice, nil, false)
	if err != nil {
		t.Fatalf("SynthesizeSegmentClips: %v", err)
	}
	if len(clips) != len(keys) {
		t.Fatalf("clips = %d, keys = %d", len(clips), len(keys))
	}
	for i, clip := range clips {
		if got := ClipKeyForPath(clip); got != keys[i] {
			t.Errorf("clip %d is named %q, want the key %q of its unit", i, got, keys[i])
		}
	}
	if client.calls != 2 {
		t.Errorf("calls = %d, want one per sentence", client.calls)
	}
}

func TestStreamedSentenceIsReusedByTheFinalSegment(t *testing.T) {
	client := &recordingTTSClient{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	voice := &entity.VoiceConfig{VoiceID: "v1", Pitch: 1, SpeechRate: 1}

	if _, err := pipeline.SynthesizeProvisional(context.Background(), entity.SegmentNarration, "", "The hall is quiet.", voice); err != nil {
		t.Fatalf("SynthesizeProvisional: %v", err)
	}

	segment := entity.TurnSegment{Kind: entity.SegmentNarration, Text: "The hall is quiet. Garrick nods."}
	clips, err := pipeline.SynthesizeSegmentClips(context.Background(), segment, voice, nil, false)
	if err != nil {
		t.Fatalf("SynthesizeSegmentClips: %v", err)
	}
	if len(clips) != 2 {
		t.Fatalf("clips = %d, want one per sentence", len(clips))
	}
	// One call for the streamed sentence, one for the second: the first is reused.
	if client.calls != 2 {
		t.Errorf("calls = %d, want the streamed sentence reused", client.calls)
	}

	again, err := pipeline.SynthesizeSegmentClips(context.Background(), segment, voice, nil, false)
	if err != nil {
		t.Fatalf("second SynthesizeSegmentClips: %v", err)
	}
	if client.calls != 2 {
		t.Errorf("calls = %d, want both clips cached", client.calls)
	}
	if len(again) != 2 || again[0] != clips[0] || again[1] != clips[1] {
		t.Errorf("second read = %#v, want the cached clips %#v", again, clips)
	}
}

func TestNoClipIsWrittenUnderTheWholeSegmentKey(t *testing.T) {
	client := &recordingTTSClient{}
	cache := NewContentCache(t.TempDir())
	pipeline := NewTTSPipeline(client, cache)
	voice := &entity.VoiceConfig{VoiceID: "v1", Pitch: 1, SpeechRate: 1}
	segment := entity.TurnSegment{Kind: entity.SegmentNarration, Text: "One. Two."}

	if _, err := pipeline.SynthesizeSegmentClips(context.Background(), segment, voice, nil, false); err != nil {
		t.Fatalf("SynthesizeSegmentClips: %v", err)
	}

	base := ComputeAudioCacheKeyForVoice(narratorSpeaker, voice, segment.Text)
	if _, err := os.Stat(filepath.Join(cache.Subdir("audio"), base+".opus")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a clip is stored under the whole-segment key %q; nothing concatenates any more", base)
	}
}

func TestSynthesizeSegmentClipsSkipsAFailedUnit(t *testing.T) {
	client := &testTTSClient{onSynthesize: func(_ context.Context, text string, _ *entity.VoiceConfig) ([]byte, error) {
		if text == "Two." {
			return nil, errors.New("provider said no")
		}
		return GenerateToneWAV(440, 0.02), nil
	}}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	clips, err := pipeline.SynthesizeSegmentClips(context.Background(), entity.TurnSegment{
		Kind: entity.SegmentNarration, Text: "One. Two.",
	}, nil, nil, false)
	if err != nil {
		t.Fatalf("a single failed unit must not fail the segment: %v", err)
	}
	if len(clips) != 1 {
		t.Fatalf("clips = %d, want the one unit that succeeded", len(clips))
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

	clips, err := pipeline.SynthesizeSegmentClips(context.Background(), entity.TurnSegment{
		Kind: entity.SegmentNarration, Text: "A *long* breath. Then another.",
	}, voice, nil, false)
	if err != nil {
		t.Fatalf("SynthesizeSegmentClips: %v", err)
	}
	if len(clips) != 1 {
		t.Fatalf("clips = %d, want the whole segment as one unit", len(clips))
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want the whole segment synthesized once", client.calls)
	}
}
