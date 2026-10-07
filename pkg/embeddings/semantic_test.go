package embeddings

import (
	"context"
	"testing"
)

// TestSemanticNeighbours is the check the hash projection fails: with a real
// encoder, "blade" is closer to "sword" than to "cabbage". It skips when the
// model or its runtime is not installed.
func TestSemanticNeighbours(t *testing.T) {
	p, err := NewONNXProvider(testModelDir(t))
	if err != nil {
		t.Skipf("embedding runtime unavailable: %v", err)
	}

	vecs, err := p.Embed(context.Background(), []string{"blade", "sword", "cabbage"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	bladeSword := CosineSimilarity(vecs[0], vecs[1])
	bladeCabbage := CosineSimilarity(vecs[0], vecs[2])
	if bladeSword <= bladeCabbage {
		t.Fatalf("blade/sword (%.4f) is not closer than blade/cabbage (%.4f): the encoder has no semantic generalisation",
			bladeSword, bladeCabbage)
	}
}
