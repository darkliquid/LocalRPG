package media

import (
	"context"
	"strings"
	"testing"

	"github.com/darkliquid/localrpg/pkg/entity"
)

type mockCueClient struct {
	audioTags bool
	markdown  bool
}

func (m *mockCueClient) Synthesize(ctx context.Context, text string, voice *entity.VoiceConfig) ([]byte, error) {
	return []byte("audio"), nil
}

func (m *mockCueClient) SpeechCueCapabilities() SpeechCueCapabilities {
	return SpeechCueCapabilities{
		AudioTags:        m.audioTags,
		MarkdownEmphasis: m.markdown,
	}
}

func TestStripAudioTags(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{input: `[whispers] "Be quiet!"`, expected: `"Be quiet!"`},
		{input: `[sighs] We made it.`, expected: `We made it.`},
		{input: `Garrick [clears throat] answered.`, expected: `Garrick answered.`},
		{input: `[laughs] [chuckles] "That is good."`, expected: `"That is good."`},
		{input: `[[alden-tavern|The Tavern]] was warm.`, expected: `[[alden-tavern|The Tavern]] was warm.`}, // Wikilinks preserved
		{input: `[whispers]`, expected: ``},
	}

	for _, c := range cases {
		got := StripAudioTags(c.input)
		if got != c.expected {
			t.Errorf("StripAudioTags(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestSpeakableTextForSpeechCues(t *testing.T) {
	tagClient := &mockCueClient{audioTags: true, markdown: false}
	plainClient := &mockCueClient{audioTags: false, markdown: false}

	input := `[whispers] "Careful, *adventurer*!"`

	// Client supporting audio tags keeps bracketed cue, strips markdown asterisks
	gotTagged := SpeakableTextFor(TextPolicyAuto, tagClient, input)
	if !strings.Contains(gotTagged, "[whispers]") {
		t.Errorf("expected tagged client to keep [whispers], got %q", gotTagged)
	}
	if strings.Contains(gotTagged, "*") {
		t.Errorf("expected markdown asterisks to be stripped, got %q", gotTagged)
	}

	// Client not supporting audio tags strips both
	gotPlain := SpeakableTextFor(TextPolicyAuto, plainClient, input)
	if strings.Contains(gotPlain, "[whispers]") {
		t.Errorf("expected plain client to strip [whispers], got %q", gotPlain)
	}
	if !strings.Contains(gotPlain, `"Careful, adventurer!"`) {
		t.Errorf("expected clean prose, got %q", gotPlain)
	}
}
