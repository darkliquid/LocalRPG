package harness

import (
	"strings"
	"testing"
)

func TestFormatSpeechFormattingInstructions(t *testing.T) {
	// Mode 1: Audio tags enabled (ElevenLabs)
	withAudio := FormatSpeechFormattingInstructions(SpeechCueContext{
		AudioTags:        true,
		MarkdownEmphasis: false,
		SampleTags:       []string{"[whispers]", "[sighs]", "[laughs]"},
	})
	if !strings.Contains(withAudio, "VOICE ACTING & SPEECH STEERING") {
		t.Errorf("expected instructions to include speech steering section")
	}
	if !strings.Contains(withAudio, "[whispers]") {
		t.Errorf("expected instructions to include sample tags")
	}

	// Mode 2: Audio tags disabled (Sherpa)
	withoutAudio := FormatSpeechFormattingInstructions(SpeechCueContext{
		AudioTags:        false,
		MarkdownEmphasis: false,
	})
	if strings.Contains(withoutAudio, "VOICE ACTING & SPEECH STEERING") {
		t.Errorf("expected no speech steering section when disabled")
	}
	if !strings.Contains(withoutAudio, "Do not write stage directions") && !strings.Contains(withoutAudio, "Do not write voice IDs, voice tags") {
		t.Errorf("expected instructions to forbid tags when disabled")
	}
}
