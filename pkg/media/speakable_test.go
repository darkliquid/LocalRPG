package media

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/entity"
)

func TestSpeakableText(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"emphasis", "She *hesitates*, then **commits**.", "She hesitates, then commits."},
		{"strong emphasis", "***Now*** or never.", "Now or never."},
		{"underscore words survive", "The snake_case_field stays intact.", "The snake_case_field stays intact."},
		{"underscore emphasis", "A _soft_ word.", "A soft word."},
		{"wikilink labelled", "Ask [[lady-evelyn|Lady Evelyn]] about it.", "Ask Lady Evelyn about it."},
		{"wikilink bare", "See [[the-ashen-bastion]].", "See the-ashen-bastion."},
		{"inline code", "Type `1d20+5` to roll.", "Type 1d20+5 to roll."},
		{"heading", "## The Gate\nIt looms.", "The Gate It looms."},
		{"list", "- First\n- Second", "First Second"},
		{"ordered list", "1. First\n2. Second", "First Second"},
		{"blockquote", "> Beware the mist.", "Beware the mist."},
		{"scene break", "Before\n\n---\n\nAfter", "Before After"},
		{"entities", "Salt &amp; iron, &quot;cold&quot;.", "Salt & iron, \"cold\"."},
		{"keeps dashes and ellipses", "Wait—no… perhaps.", "Wait—no… perhaps."},
		{"blank reduces empty", "   \n\n  ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SpeakableText(tt.in); got != tt.want {
				t.Errorf("SpeakableText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// markdownFake records every string an engine is asked to speak and can claim to
// understand Markdown itself.
type markdownFake struct {
	got   []string
	aware bool
}

func (m *markdownFake) Synthesize(_ context.Context, text string, _ *entity.VoiceConfig) ([]byte, error) {
	m.got = append(m.got, text)
	return GenerateToneWAV(440, 0.02), nil
}

func (m *markdownFake) SupportsMarkdown() bool { return m.aware }

func TestPipelineStripsMarkdownByDefault(t *testing.T) {
	client := &markdownFake{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	if _, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentNarration, Text: "A *soft* word."}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.got) != 1 || client.got[0] != "A soft word." {
		t.Errorf("engine received %q, want %q", client.got, "A soft word.")
	}
}

func TestPipelineKeepsMarkdownForAwareClient(t *testing.T) {
	client := &markdownFake{aware: true}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	if _, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentNarration, Text: "A *soft* word."}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.got) != 1 || client.got[0] != "A *soft* word." {
		t.Errorf("aware engine received %q, want raw markdown", client.got)
	}
}

func TestPipelineKeepPolicySendsRawText(t *testing.T) {
	client := &markdownFake{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))
	pipeline.SetTextPolicy(TextPolicyKeep)

	if _, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentNarration, Text: "A *soft* word."}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.got) != 1 || client.got[0] != "A *soft* word." {
		t.Errorf("keep policy received %q, want raw markdown", client.got)
	}
}

func TestPipelineSkipsEmptyAfterStrip(t *testing.T) {
	client := &markdownFake{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	_, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentNarration, Text: "---"}, nil, nil)
	if !errors.Is(err, ErrNoSpeakableText) {
		t.Fatalf("expected ErrNoSpeakableText, got %v", err)
	}
	if len(client.got) != 0 {
		t.Errorf("empty segment reached the engine: %q", client.got)
	}
}

func TestSynthesizeSegmentsSkipsUnspeakableBeats(t *testing.T) {
	client := &markdownFake{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	clips, err := pipeline.SynthesizeSegments(context.Background(), []entity.TurnSegment{
		{Kind: entity.SegmentNarration, Text: "---"},
		{Kind: entity.SegmentNarration, Text: "Hello."},
	}, nil, nil)
	if err != nil {
		t.Fatalf("SynthesizeSegments failed: %v", err)
	}
	if len(clips) != 1 {
		t.Fatalf("expected one clip, got %d", len(clips))
	}
}

func TestPipelineSharesCacheForEquivalentText(t *testing.T) {
	client := &markdownFake{}
	pipeline := NewTTSPipeline(client, NewContentCache(t.TempDir()))

	first, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentNarration, Text: "**bold**"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pipeline.SynthesizeSegment(context.Background(),
		entity.TurnSegment{Kind: entity.SegmentNarration, Text: "bold"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("equivalent text produced different clips: %q vs %q", first, second)
	}
	if len(client.got) != 1 {
		t.Errorf("expected one synthesis call, got %d (%q)", len(client.got), strings.Join(client.got, "|"))
	}
}

func TestTextPolicyFromConfig(t *testing.T) {
	cases := map[string]TextPolicy{
		"":         TextPolicyAuto,
		"auto":     TextPolicyAuto,
		"strip":    TextPolicyStrip,
		"keep":     TextPolicyKeep,
		"nonsense": TextPolicyAuto,
	}
	for in, want := range cases {
		var cfg config.TTSConfig
		cfg.Markdown = in
		if got := TextPolicyFromConfig(cfg); got != want {
			t.Errorf("TextPolicyFromConfig(%q) = %v, want %v", in, got, want)
		}
	}
}
