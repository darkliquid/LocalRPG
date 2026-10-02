package ttsgemini

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/genai"

	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
)

func TestDecodeBatchOutput(t *testing.T) {
	client := &GeminiTTSClient{model: "gemini-3.8-flash-tts", defaultVoice: "Aoede"}
	audio := base64.StdEncoding.EncodeToString([]byte("RIFF....WAVE"))
	input := strings.Join([]string{
		`{"key":"k1","response":{"candidates":[{"content":{"parts":[{"inlineData":{"data":"` + audio + `","mimeType":"audio/wav"}}],"role":"model"}}]}}`,
		`{"key":"k2","error":{"message":"boom"}}`,
	}, "\n")

	results, err := client.decodeBatchOutput([]byte(input))
	if err != nil {
		t.Fatalf("decodeBatchOutput: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %#v", len(results), results)
	}
	if results[0].Key != "k1" || len(results[0].Audio) == 0 || results[0].Err != nil {
		t.Errorf("unexpected first result %#v", results[0])
	}
	if results[1].Key != "k2" || results[1].Err == nil {
		t.Errorf("expected the error line to be a failed result, got %#v", results[1])
	}
}

func TestBatchStateMapping(t *testing.T) {
	cases := map[genai.JobState]string{
		genai.JobStateQueued:    "pending",
		genai.JobStatePending:   "pending",
		genai.JobStateRunning:   "running",
		genai.JobStateSucceeded: "succeeded",
		genai.JobStateFailed:    "failed",
		genai.JobStateCancelled: "cancelled",
		genai.JobStateExpired:   "expired",
	}
	for state, want := range cases {
		if got := batchState(state); got != want {
			t.Errorf("batchState(%q) = %q, want %q", state, got, want)
		}
	}
}

// The batch file must be a JSONL GenerateContentRequest per line, with the audio
// config under generation_config, which is what the Developer API parses.
func TestBatchInputLineShape(t *testing.T) {
	client := &GeminiTTSClient{model: "gemini-3.8-flash-tts", defaultVoice: "Aoede"}
	line, err := client.batchInputLine(media.BatchRequest{
		Key: "k1",
		Lines: []media.SpeakerLine{
			{SpeakerID: "narrator", Label: "Narrator", Voice: &entity.VoiceConfig{VoiceID: "Aoede"}, Text: "Hello."},
		},
	})
	if err != nil {
		t.Fatalf("batchInputLine: %v", err)
	}

	var decoded struct {
		Key     string `json:"key"`
		Request struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
				Role string `json:"role"`
			} `json:"contents"`
			GenerationConfig struct {
				ResponseModalities []string `json:"responseModalities"`
				SpeechConfig       *struct {
					VoiceConfig struct {
						PrebuiltVoiceConfig struct {
							VoiceName string `json:"voiceName"`
						} `json:"prebuiltVoiceConfig"`
					} `json:"voiceConfig"`
				} `json:"speechConfig"`
			} `json:"generation_config"`
		} `json:"request"`
	}
	if err := json.Unmarshal(line, &decoded); err != nil {
		t.Fatalf("decode line: %v (raw %s)", err, line)
	}
	if decoded.Key != "k1" {
		t.Errorf("key = %q, want k1", decoded.Key)
	}
	if len(decoded.Request.Contents) != 1 || len(decoded.Request.Contents[0].Parts) != 1 || decoded.Request.Contents[0].Parts[0].Text != "Hello." {
		t.Errorf("unexpected contents %#v", decoded.Request.Contents)
	}
	if mods := decoded.Request.GenerationConfig.ResponseModalities; len(mods) != 1 || mods[0] != "AUDIO" {
		t.Errorf("responseModalities = %#v, want [AUDIO]", mods)
	}
	if decoded.Request.GenerationConfig.SpeechConfig == nil {
		t.Fatalf("expected a speechConfig in the line: %s", line)
	}
	if name := decoded.Request.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig.VoiceName; name != "Aoede" {
		t.Errorf("voiceName = %q, want Aoede", name)
	}
}
