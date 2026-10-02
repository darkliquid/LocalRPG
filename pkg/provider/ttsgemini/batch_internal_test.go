package ttsgemini

import (
	"encoding/base64"
	"strings"
	"testing"

	"google.golang.org/genai"
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
