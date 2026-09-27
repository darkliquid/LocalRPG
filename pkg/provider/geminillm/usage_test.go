package geminillm

import (
	"testing"

	"google.golang.org/genai"
)

func TestUsageFromMetadataMapsTokens(t *testing.T) {
	u := usageFromMetadata("gemini-2.5-pro", &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     7,
		CandidatesTokenCount: 3,
	})
	if u == nil || u.InputTokens != 7 || u.OutputTokens != 3 {
		t.Fatalf("usage = %+v, want 7/3", u)
	}
	if u.Provider != "gemini" || u.Model != "gemini-2.5-pro" {
		t.Fatalf("usage identity = %+v", u)
	}
	if usageFromMetadata("m", nil) != nil {
		t.Fatal("nil metadata must map to nil usage")
	}
}
