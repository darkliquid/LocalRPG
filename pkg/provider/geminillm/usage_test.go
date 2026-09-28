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
	if u.Model != "gemini-2.5-pro" {
		t.Fatalf("usage identity = %+v", u)
	}
	if u.Provider != "" {
		t.Fatalf("the adapter must not name itself; the resolver stamps the key: %+v", u)
	}
	if usageFromMetadata("m", nil) != nil {
		t.Fatal("nil metadata must map to nil usage")
	}
}
