package embeddings

import "context"

// Provider computes dense vector embeddings for input strings.
type Provider interface {
	ID() string
	Dimensions() int
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
