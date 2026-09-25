package embeddings

import (
	"context"
	"hash/fnv"
	"math"
	"strings"
)

// BuiltinHashProjectionProvider is a pure-Go, zero-dependency offline embedding
// provider. It projects word tokens and character n-grams into a fixed-dimension
// normalized unit vector using deterministic hashing.
type BuiltinHashProjectionProvider struct {
	id         string
	dimensions int
}

// NewBuiltinHashProjectionProvider creates a hash projection provider.
func NewBuiltinHashProjectionProvider(dims int) *BuiltinHashProjectionProvider {
	if dims <= 0 {
		dims = 384
	}
	return &BuiltinHashProjectionProvider{
		id:         "builtin-hash-projection",
		dimensions: dims,
	}
}

func (p *BuiltinHashProjectionProvider) ID() string {
	return p.id
}

func (p *BuiltinHashProjectionProvider) Dimensions() int {
	return p.dimensions
}

func (p *BuiltinHashProjectionProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	res := make([][]float32, len(texts))
	for i, text := range texts {
		res[i] = p.projectText(text)
	}
	return res, nil
}

var commonStopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "of": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "with": true,
	"by": true, "from": true, "as": true, "is": true, "was": true, "are": true,
	"were": true, "it": true, "its": true, "upon": true, "into": true,
}

func (p *BuiltinHashProjectionProvider) projectText(text string) []float32 {
	vec := make([]float32, p.dimensions)
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return vec
	}

	h := fnv.New64a()
	for _, word := range words {
		// Clean punctuation
		cleaned := strings.Trim(word, `.,:;!?'"()[]{}`)
		if len(cleaned) == 0 || commonStopWords[cleaned] {
			continue
		}

		// Word level projection
		h.Reset()
		_, _ = h.Write([]byte(cleaned))
		val := h.Sum64()
		dim1 := int(val % uint64(p.dimensions))
		sign1 := float32(1.0)
		if (val>>32)%2 == 1 {
			sign1 = -1.0
		}
		vec[dim1] += sign1 * 2.0

		// Character 3-gram projection for subword/morphological overlap
		runes := []rune(cleaned)
		for j := 0; j+2 < len(runes); j++ {
			gram := string(runes[j : j+3])
			h.Reset()
			_, _ = h.Write([]byte(gram))
			gval := h.Sum64()
			gdim := int(gval % uint64(p.dimensions))
			gsign := float32(1.0)
			if (gval>>32)%2 == 1 {
				gsign = -1.0
			}
			vec[gdim] += gsign * 0.5
		}
	}

	// L2 normalization to unit vector
	var sumSq float32
	for _, v := range vec {
		sumSq += v * v
	}
	norm := float32(math.Sqrt(float64(sumSq)))
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec
}

var _ Provider = (*BuiltinHashProjectionProvider)(nil)
