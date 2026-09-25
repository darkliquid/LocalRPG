package embeddings

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeVector serializes a float32 slice into a little-endian byte slice.
func EncodeVector(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// DecodeVector parses a little-endian byte slice into a float32 slice.
func DecodeVector(buf []byte) ([]float32, error) {
	if len(buf)%4 != 0 {
		return nil, fmt.Errorf("invalid vector byte length %d (must be multiple of 4)", len(buf))
	}
	n := len(buf) / 4
	vec := make([]float32, n)
	for i := 0; i < n; i++ {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return vec, nil
}

// CosineSimilarity computes the cosine similarity between two float32 vectors.
// If vectors have different lengths or zero magnitude, 0.0 is returned.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dot, normA, normB float32
	i := 0
	// 4-way unrolled loop for SIMD-friendly instruction pipelining
	for ; i+3 < len(a); i += 4 {
		dot += a[i]*b[i] + a[i+1]*b[i+1] + a[i+2]*b[i+2] + a[i+3]*b[i+3]
		normA += a[i]*a[i] + a[i+1]*a[i+1] + a[i+2]*a[i+2] + a[i+3]*a[i+3]
		normB += b[i]*b[i] + b[i+1]*b[i+1] + b[i+2]*b[i+2] + b[i+3]*b[i+3]
	}
	for ; i < len(a); i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	denom := float32(math.Sqrt(float64(normA)) * math.Sqrt(float64(normB)))
	if denom <= 0 {
		return 0.0
	}
	return dot / denom
}
