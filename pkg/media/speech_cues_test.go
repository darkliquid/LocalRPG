package media_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/provider/ttshttp"
	"github.com/darkliquid/localrpg/pkg/provider/ttssherpa"
)

// TestSpeechCueAdvertiserImplementations checks that each engine that declares
// speech-cue capabilities actually implements the advertiser interface.
func TestSpeechCueAdvertiserImplementations(t *testing.T) {
	eleven, err := media.NewElevenLabsTTSClient(config.TTSConfig{Type: "builtin", BuiltinName: "elevenlabs", APIKey: "test-key"})
	if err != nil {
		t.Fatalf("elevenlabs client: %v", err)
	}
	adv, ok := interface{}(eleven).(media.SpeechCueAdvertiser)
	if !ok {
		t.Fatal("expected the ElevenLabs client to implement SpeechCueAdvertiser")
	}
	caps := adv.SpeechCueCapabilities()
	if !caps.AudioTags {
		t.Errorf("expected ElevenLabs to support AudioTags, got false")
	}
	if len(caps.SupportedTags) == 0 {
		t.Errorf("expected ElevenLabs to advertise SupportedTags, got empty")
	}

	sherpa := ttssherpa.NewSherpaTTSClient("")
	advSherpa, ok := interface{}(sherpa).(media.SpeechCueAdvertiser)
	if !ok {
		t.Fatal("expected the Sherpa client to implement SpeechCueAdvertiser")
	}
	capsSherpa := advSherpa.SpeechCueCapabilities()
	if capsSherpa.AudioTags || capsSherpa.MarkdownEmphasis {
		t.Errorf("expected Sherpa to support neither AudioTags nor Markdown, got %+v", capsSherpa)
	}

	httpTTS := ttshttp.NewHTTPTTSClient(config.TTSConfig{Type: "http", Endpoint: "http://localhost:8880"})
	advHTTP, ok := interface{}(httpTTS).(media.SpeechCueAdvertiser)
	if !ok {
		t.Fatal("expected the HTTP TTS client to implement SpeechCueAdvertiser")
	}
	if !advHTTP.SpeechCueCapabilities().MarkdownEmphasis {
		t.Errorf("expected the HTTP TTS client to support MarkdownEmphasis, got false")
	}
}
