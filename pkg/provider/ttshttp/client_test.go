package ttshttp_test

import (
	"testing"

	"github.com/darkliquid/localrpg/pkg/provider/ttshttp"
)

func TestResolveHTTPEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantSpeech string
		wantVoices string
	}{
		{
			name:       "bare base url",
			input:      "http://localhost:8880",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "base url with trailing slash",
			input:      "http://localhost:8880/",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "full speech endpoint",
			input:      "http://localhost:8880/v1/audio/speech",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "v1 endpoint",
			input:      "http://localhost:8880/v1",
			wantSpeech: "http://localhost:8880/v1/audio/speech",
			wantVoices: "http://localhost:8880/v1/audio/voices",
		},
		{
			name:       "alltalk endpoint preserved",
			input:      "http://localhost:7851/api/tts-generate",
			wantSpeech: "http://localhost:7851/api/tts-generate",
			wantVoices: "",
		},
		{
			name:       "empty endpoint",
			input:      "",
			wantSpeech: "",
			wantVoices: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpeech, gotVoices := ttshttp.ResolveHTTPEndpoints(tt.input)
			if gotSpeech != tt.wantSpeech {
				t.Errorf("speechURL = %q, want %q", gotSpeech, tt.wantSpeech)
			}
			if gotVoices != tt.wantVoices {
				t.Errorf("voicesURL = %q, want %q", gotVoices, tt.wantVoices)
			}
		})
	}
}
