package media

import (
	"testing"
)

func TestSpeechCueAdvertiserImplementations(t *testing.T) {
	eleven := &ElevenLabsTTSClient{}
	adv, ok := interface{}(eleven).(SpeechCueAdvertiser)
	if !ok {
		t.Fatal("expected ElevenLabsTTSClient to implement SpeechCueAdvertiser")
	}
	caps := adv.SpeechCueCapabilities()
	if !caps.AudioTags {
		t.Errorf("expected ElevenLabs to support AudioTags, got false")
	}
	if len(caps.SupportedTags) == 0 {
		t.Errorf("expected ElevenLabs to advertise SupportedTags, got empty")
	}

	sherpa := &SherpaTTSClient{}
	advSherpa, ok := interface{}(sherpa).(SpeechCueAdvertiser)
	if !ok {
		t.Fatal("expected SherpaTTSClient to implement SpeechCueAdvertiser")
	}
	capsSherpa := advSherpa.SpeechCueCapabilities()
	if capsSherpa.AudioTags || capsSherpa.MarkdownEmphasis {
		t.Errorf("expected Sherpa to support neither AudioTags nor Markdown, got %+v", capsSherpa)
	}

	httpCli := &httpTTSClient{}
	advHTTP, ok := interface{}(httpCli).(SpeechCueAdvertiser)
	if !ok {
		t.Fatal("expected httpTTSClient to implement SpeechCueAdvertiser")
	}
	capsHTTP := advHTTP.SpeechCueCapabilities()
	if !capsHTTP.MarkdownEmphasis {
		t.Errorf("expected httpTTSClient to support MarkdownEmphasis, got false")
	}
}
