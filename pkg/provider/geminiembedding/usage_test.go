package geminiembedding

import (
	"testing"

	"google.golang.org/genai"
)

func TestEmbeddingUsagePrefersBillableCharacters(t *testing.T) {
	u := embeddingUsage("text-embedding-004", &genai.EmbedContentResponse{
		Metadata: &genai.EmbedContentMetadata{BillableCharacterCount: 42},
	})
	if u.Characters != 42 || u.Requests != 0 || u.Estimated {
		t.Fatalf("usage = %+v, want 42 exact characters", u)
	}
	if u.Model != "text-embedding-004" {
		t.Fatalf("model = %q", u.Model)
	}
}

func TestEmbeddingUsageFallsBackToAnEstimatedRequest(t *testing.T) {
	for name, resp := range map[string]*genai.EmbedContentResponse{
		"no metadata":     {},
		"zero characters": {Metadata: &genai.EmbedContentMetadata{}},
	} {
		u := embeddingUsage("gemini-embedding", resp)
		if u.Characters != 0 || u.Requests != 1 || !u.Estimated {
			t.Fatalf("%s: usage = %+v, want one estimated request", name, u)
		}
	}
}
