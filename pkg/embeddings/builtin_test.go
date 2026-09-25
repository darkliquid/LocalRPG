package embeddings

import (
	"context"
	"math"
	"testing"
)

func TestBuiltinHashProjectionProvider(t *testing.T) {
	p := NewBuiltinHashProjectionProvider(384)
	if p.Dimensions() != 384 {
		t.Fatalf("expected 384 dimensions, got %d", p.Dimensions())
	}
	if p.ID() == "" {
		t.Fatal("expected non-empty ID")
	}

	ctx := context.Background()
	texts := []string{
		"The ancient dragon of Mount Dread slept upon heaps of gold.",
		"A fiery dragon rested in its mountain cavern filled with treasure.",
		"A quiet apothecary sells healing herbs and minor salves in the village.",
	}

	vecs, err := p.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(vecs) != len(texts) {
		t.Fatalf("expected %d vectors, got %d", len(texts), len(vecs))
	}

	for i, v := range vecs {
		if len(v) != 384 {
			t.Fatalf("vector %d length = %d, expected 384", i, len(v))
		}
		// Check that vectors are normalized to unit length
		var normSq float32
		for _, val := range v {
			normSq += val * val
		}
		norm := math.Sqrt(float64(normSq))
		if math.Abs(norm-1.0) > 1e-3 {
			t.Errorf("vector %d norm = %f, expected ~1.0", i, norm)
		}
	}

	// Semantically closer texts (both about dragons/cavern/gold) should have higher similarity
	sim12 := CosineSimilarity(vecs[0], vecs[1])
	sim13 := CosineSimilarity(vecs[0], vecs[2])
	if sim12 <= sim13 {
		t.Errorf("expected similar dragon texts to have higher similarity than apothecary text; sim12=%f, sim13=%f", sim12, sim13)
	}
}
