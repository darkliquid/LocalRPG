package embeddings

import (
	"math"
	"testing"
)

func TestVectorEncodeDecodeRoundTrip(t *testing.T) {
	orig := []float32{0.0, 1.5, -2.25, 3.14159, -0.0001}
	encoded := EncodeVector(orig)
	if len(encoded) != len(orig)*4 {
		t.Fatalf("expected encoded length %d, got %d", len(orig)*4, len(encoded))
	}
	decoded, err := DecodeVector(encoded)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(decoded) != len(orig) {
		t.Fatalf("expected decoded length %d, got %d", len(orig), len(decoded))
	}
	for i := range orig {
		if math.Abs(float64(orig[i]-decoded[i])) > 1e-6 {
			t.Errorf("at index %d: expected %f, got %f", i, orig[i], decoded[i])
		}
	}
}

func TestDecodeVectorInvalidLength(t *testing.T) {
	_, err := DecodeVector([]byte{1, 2, 3}) // not a multiple of 4
	if err == nil {
		t.Fatal("expected error on invalid byte length, got nil")
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "identical vectors",
			a:        []float32{1.0, 0.0, 0.0},
			b:        []float32{1.0, 0.0, 0.0},
			expected: 1.0,
		},
		{
			name:     "orthogonal vectors",
			a:        []float32{1.0, 0.0},
			b:        []float32{0.0, 1.0},
			expected: 0.0,
		},
		{
			name:     "opposite vectors",
			a:        []float32{1.0, 2.0},
			b:        []float32{-1.0, -2.0},
			expected: -1.0,
		},
		{
			name:     "zero vector safe",
			a:        []float32{0.0, 0.0},
			b:        []float32{1.0, 2.0},
			expected: 0.0,
		},
		{
			name:     "dimension mismatch safe",
			a:        []float32{1.0},
			b:        []float32{1.0, 2.0},
			expected: 0.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sim := CosineSimilarity(tc.a, tc.b)
			if math.Abs(float64(sim-tc.expected)) > 1e-5 {
				t.Errorf("expected similarity %f, got %f", tc.expected, sim)
			}
		})
	}
}
