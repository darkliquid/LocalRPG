package media_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/media"
)

func TestDescribeGeminiTTSCapabilities(t *testing.T) {
	client := media.NewGeminiTTSClientOffline("gemini-3.8-flash-tts", "Aoede")
	caps := media.Describe(client)
	if !caps.VoiceCatalog || !caps.VoiceOptions || !caps.SpeechCues || !caps.Metered || !caps.ExtendedVoices {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
	if caps.MarkdownEmphasis {
		t.Fatalf("gemini tts does not interpret markdown: %+v", caps)
	}
}
